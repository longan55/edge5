package edge

import (
	"fmt"
	"time"

	"go.uber.org/zap"
)

// SamplePoint 去重后待采样的点位
type SamplePoint struct {
	Address   string
	ParseType string
}

type ruleRuntime struct {
	rule  Rule
	state State
	// lastCount 上一次 Evaluate 后该计数器返回的 count；nil 表示尚未初始化或跨日重置，
	// 此时本次 Evaluate 不计算 delta（避免昨日 50 → 今日 0 产生 -49 的负 delta）
	lastCount    *int64
	lastCountDay string
}

// EvalResult 单次 Evaluate 的结果
type EvalResult struct {
	// Outputs 输出名 -> 值（仅含 emit=true 的规则）
	Outputs map[string]interface{}
	// CounterDeltas 计数类规则名 -> 自上次以来的增量；正数表示计数累加，仅包含 != 0 的项
	CounterDeltas map[string]int64
}

// Engine 单个边缘计算任务的运行引擎，持有全部规则的有状态运行时。
// 计数状态仅存在内存：进程重启自然清零；跨自然日由算子在 Evaluate 时清零。
// 引擎通过 lastCount/lastCountDay 跟踪 delta，供调度层持久化到 edge_counter_daily。
type Engine struct {
	runtimes []*ruleRuntime
	logger   *zap.Logger
	inputs   []Input // 顶层点位池（含表达式，由 service 在采样后求值）
}

// NormalizeFuncType 把历史算子类型归一化为当前类型，保证旧任务无需迁移即可运行：
//   - counter_pulse / counter_multi → counter_digital（数字信号计数，单/多点位统一）
//   - counter_ac                    → counter_analog（模拟峰值计数）
func NormalizeFuncType(t string) string {
	switch t {
	case "counter_pulse", "counter_multi":
		return "counter_digital"
	case "counter_ac":
		return "counter_analog"
	default:
		return t
	}
}

// NewEngine 根据规则与点位池构建引擎：校验每条规则引用的 Var 都在点位池中存在，
// 为每条规则创建算子运行时。
func NewEngine(rules []Rule, inputs []Input, logger *zap.Logger) (*Engine, error) {
	// 构建 Var->Input 索引，校验 Var 唯一性
	varMap := make(map[string]Input, len(inputs))
	for _, in := range inputs {
		if in.Var == "" {
			return nil, fmt.Errorf("点位池中存在空变量名")
		}
		if _, exists := varMap[in.Var]; exists {
			return nil, fmt.Errorf("点位池变量名重复: %s", in.Var)
		}
		varMap[in.Var] = in
	}

	rt := make([]*ruleRuntime, 0, len(rules))
	for _, r := range rules {
		// 历史类型归一化（复制后修改，避免污染调用方传入的规则）
		r.FuncType = NormalizeFuncType(r.FuncType)
		// 校验规则引用的 Var 全部在点位池中
		for _, vr := range r.Vars {
			if _, ok := varMap[vr.Var]; !ok {
				return nil, fmt.Errorf("规则 %s 引用了未定义的变量 %s", r.Name, vr.Var)
			}
		}
		fn, err := GetFunc(r.FuncType)
		if err != nil {
			return nil, err
		}
		st, err := fn.NewState(r)
		if err != nil {
			return nil, err
		}
		// 注入 logger 给支持 LoggerAware 的算子（如计数类，便于打印决策过程）
		if la, ok := st.(LoggerAware); ok && logger != nil {
			la.SetLogger(logger)
		}
		rt = append(rt, &ruleRuntime{rule: r, state: st})
	}
	return &Engine{runtimes: rt, logger: logger, inputs: inputs}, nil
}

// SamplePoints 返回点位池按地址去重后的采样清单
func (e *Engine) SamplePoints() []SamplePoint {
	seen := make(map[string]bool)
	out := make([]SamplePoint, 0, len(e.inputs))
	for _, in := range e.inputs {
		if in.Address == "" || seen[in.Address] {
			continue
		}
		seen[in.Address] = true
		out = append(out, SamplePoint{Address: in.Address, ParseType: in.ParseType})
	}
	return out
}

// Evaluate 输入为已应用 Scale 归一化后的采样值（key=Var 名），按规则逐条计算。
// 某条规则的任一引用变量缺失（质量异常/未读到）时，本周期跳过该规则、
// 不动其 lastCount，避免下次成功时产生虚假 delta。
func (e *Engine) Evaluate(values map[string]float64, now time.Time) EvalResult {
	day := DayKey(now)
	out := make(map[string]interface{})
	deltas := make(map[string]int64)
	for _, rt := range e.runtimes {
		if len(rt.rule.Vars) == 0 {
			continue
		}
		in := make(map[string]float64, len(rt.rule.Vars))
		ready := true
		for _, vr := range rt.rule.Vars {
			v, ok := values[vr.Var]
			if !ok {
				ready = false
				break
			}
			in[vr.Var] = v
		}
		if !ready {
			continue
		}
		val, emit, err := rt.state.Evaluate(in, day)
		if err != nil {
			if e.logger != nil {
				e.logger.Warn("边缘计算规则执行失败",
					zap.String("rule", rt.rule.Name),
					zap.String("func", rt.rule.FuncType),
					zap.Error(err))
			}
			continue
		}
		if emit {
			out[rt.rule.Name] = val
		}
		// 计数类算子：计算 delta
		if cs, ok := rt.state.(CounterState); ok {
			cur := cs.Count()
			// 跨日：算子内部 rollDay 已清零 count，引擎层也重置基线，
			// 本拍不计算 delta（今日首拍若未触发边沿则 cur=0，delta=0 也无意义写入）
			if rt.lastCountDay != day {
				rt.lastCountDay = day
				rt.lastCount = nil
			}
			if rt.lastCount != nil {
				delta := cur - *rt.lastCount
				if delta != 0 {
					deltas[rt.rule.Name] = delta
				}
			}
			c := cur
			rt.lastCount = &c
		}
	}
	return EvalResult{Outputs: out, CounterDeltas: deltas}
}

// SetInitialCount 为指定规则恢复内存计数（启动时从 DB 加载当日已记录）。
// 同时设置 lastCount=count、lastCountDay=今日，使首次 Evaluate 不产生 spurious delta。
// lastHigh 由算子 SetInitialCount 内部清空，首拍仅建立基线。
func (e *Engine) SetInitialCount(ruleName string, count int64, day string) {
	for _, rt := range e.runtimes {
		if rt.rule.Name != ruleName {
			continue
		}
		if cs, ok := rt.state.(CounterState); ok {
			cs.SetInitialCount(count)
			c := count
			rt.lastCount = &c
			rt.lastCountDay = day
		}
		return
	}
}

// AnalyzeRule 调用算子的在线分析能力，返回算子内部建议信息。
// 仅对实现了 Analyzable 的算子有效（如 counter_ac）。返回 (info, true) 表示找到并支持分析。
func (e *Engine) AnalyzeRule(ruleName string) (map[string]interface{}, bool) {
	for _, rt := range e.runtimes {
		if rt.rule.Name != ruleName {
			continue
		}
		if a, ok := rt.state.(Analyzable); ok {
			return a.AnalyzeFull(), true
		}
		return nil, false
	}
	return nil, false
}
