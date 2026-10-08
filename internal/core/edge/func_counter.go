package edge

import (
	"fmt"

	"go.uber.org/zap"
)

// ===========================================================================
// counter_digital：数字信号计数（单点位 / 多点位统一）
//
// 面向开关量 / 数字电平（0/1）信号。典型波形 0-峰值-0-峰值-0 计为 2 次：
// 对“是否达到阈值”的电平做边沿检测，默认上升沿（0→峰值）计一次，
// 避免高电平持续期间每个采样周期重复计数。
//
// 规则引用 1 个变量时为单点位计数；引用多个变量时，每个点位独立按
// （本点阈值或统一阈值）判定高/低，再按 logic 组合：
//   and    —— 所有点位均高（多点位时默认）
//   or     —— 任意点位高
//   n_of_m —— 高电平点位数 ≥ requireCount
// 对组合电平做边沿计数：组合条件从“不满足→满足”一次计 1。
//
// 计数仅存内存（进程重启清零），跨自然日首次采样自动清零；
// 引擎层通过 SetInitialCount 从 DB 加载当日已记录计数，重启后从断点继续。
//
// 兼容旧类型：counter_pulse（单点位）与 counter_multi（多点位组合）
// 在引擎层归一化为本算子，参数完全兼容。
// ===========================================================================

type digitalCounterFunc struct{}

func (digitalCounterFunc) Meta() FuncMeta {
	return FuncMeta{
		Type:    "counter_digital",
		CName:   "数字信号计数",
		Desc:    "对开关量/数字电平(0/1)计数：引用1个点位为单点计数；继续添加点位即变为多点位计数，按 与/或/满足N个 组合（如 0-峰值-0-峰值-0 计为 2）",
		MultiIn: true,
		Params: []ParamMeta{
			{Name: "edge", CName: "计数边沿", Type: "select", Default: "rising",
				Options: []ParamOption{
					{Label: "上升沿(0→峰值)", Value: "rising"},
					{Label: "下降沿(峰值→0)", Value: "falling"},
					{Label: "双向沿", Value: "both"},
				}},
			// logic / requireCount / threshold 仅多点位（Vars>1）时在前端展示，
			// 单点位时后端忽略 logic；threshold 单点位时作为缺省阈值。
			{Name: "logic", CName: "组合条件", Type: "select", Default: "and",
				Desc: "仅多个点位时生效",
				Options: []ParamOption{
					{Label: "与(全部满足)", Value: "and"},
					{Label: "或(任一满足)", Value: "or"},
					{Label: "满足N个", Value: "n_of_m"},
				}},
			{Name: "requireCount", CName: "满足个数N", Type: "number", Default: 1,
				Desc: "仅组合条件为“满足N个”时生效"},
			{Name: "threshold", CName: "统一峰值阈值", Type: "number", Default: 1,
				Desc: "未单独配置阈值的点位使用此阈值；当前值 ≥ 阈值视为高电平"},
		},
	}
}

func (digitalCounterFunc) NewState(rule Rule) (State, error) {
	if len(rule.Vars) == 0 {
		return nil, fmt.Errorf("数字信号计数至少需要 1 个引用变量")
	}
	uni := rule.Params.Float("threshold", 1)
	cfgs := make([]multiInputCfg, 0, len(rule.Vars))
	for _, vr := range rule.Vars {
		th := uni
		if vr.HasThreshold {
			th = vr.Threshold
		}
		cfgs = append(cfgs, multiInputCfg{varName: vr.Var, threshold: th})
	}
	n := rule.Params.Int("requireCount", 1)
	if n < 1 {
		n = 1
	}
	if n > len(cfgs) {
		n = len(cfgs)
	}
	return &digitalCounterState{
		cfgs:         cfgs,
		edge:         rule.Params.String("edge", "rising"),
		logic:        rule.Params.String("logic", "and"),
		requireCount: n,
	}, nil
}

type multiInputCfg struct {
	varName   string
	threshold float64
}

type digitalCounterState struct {
	cfgs         []multiInputCfg
	edge         string
	logic        string
	requireCount int
	lastHigh     *bool
	count        int64
	day          string
	logger       *zap.Logger
}

// SetLogger 实现 LoggerAware
func (s *digitalCounterState) SetLogger(l *zap.Logger) { s.logger = l }

// Count 实现 CounterState
func (s *digitalCounterState) Count() int64 { return s.count }

// SetInitialCount 实现 CounterState：从 DB 恢复当日已记录计数；
// 同时清空 lastHigh，使下次采样仅建立基线，避免首拍误判边沿
func (s *digitalCounterState) SetInitialCount(count int64) {
	s.count = count
	s.lastHigh = nil
}

func (s *digitalCounterState) rollDay(day string) {
	if day != "" && s.day != "" && day != s.day {
		s.count = 0
		s.lastHigh = nil // 跨日重新建立基线，避免零点瞬间误判
	}
	s.day = day
}

// combo 计算当前采样拍各点位高/低状态与组合电平
func (s *digitalCounterState) combo(in map[string]float64) (bool, int, []map[string]interface{}) {
	highCount := 0
	status := make([]map[string]interface{}, 0, len(s.cfgs))
	for _, c := range s.cfgs {
		val := in[c.varName]
		isHigh := val >= c.threshold
		if isHigh {
			highCount++
		}
		status = append(status, map[string]interface{}{
			"var":       c.varName,
			"value":     val,
			"threshold": c.threshold,
			"high":      isHigh,
		})
	}
	var comboHigh bool
	switch s.logic {
	case "or":
		comboHigh = highCount >= 1
	case "n_of_m":
		comboHigh = highCount >= s.requireCount
	default: // and
		comboHigh = highCount == len(s.cfgs)
	}
	return comboHigh, highCount, status
}

func (s *digitalCounterState) Evaluate(in map[string]float64, day string) (interface{}, bool, error) {
	s.rollDay(day)
	high, highCount, status := s.combo(in)

	triggered := false
	reason := "baseline-init" // 首拍建基线，不计数
	if s.lastHigh != nil {
		prev := *s.lastHigh
		rising := !prev && high
		falling := prev && !high
		switch s.edge {
		case "falling":
			if falling {
				s.count++
				triggered = true
				reason = "falling-edge"
			} else {
				reason = "no-edge"
			}
		case "both":
			if rising || falling {
				s.count++
				triggered = true
				reason = "both-edge"
			} else {
				reason = "no-edge"
			}
		default: // rising
			if rising {
				s.count++
				triggered = true
				reason = "rising-edge"
			} else {
				reason = "no-edge"
			}
		}
	}

	if s.logger != nil {
		s.logger.Debug("digital-counter evaluate",
			zap.Any("inputs", status),
			zap.Int("highCount", highCount),
			zap.Int("requireCount", s.requireCount),
			zap.String("logic", s.logic),
			zap.Bool("high", high),
			zap.Any("lastHigh", s.lastHigh),
			zap.String("edge", s.edge),
			zap.String("reason", reason),
			zap.Bool("triggered", triggered),
			zap.Int64("count", s.count),
			zap.String("day", day),
		)
	}

	h := high
	s.lastHigh = &h
	return s.count, true, nil
}
