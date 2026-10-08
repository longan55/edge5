package edge

import (
	"fmt"
	"sort"
	"time"

	"go.uber.org/zap"
)

// ===========================================================================
// counter_analog：模拟峰值计数（单点位 / 多点位统一）
//
// 面向连续变化的模拟量（如 4-20mA 电流、电压、振动等），统计“峰值”出现次数。
// 与数字信号计数（counter_digital，阈值二值化）的区别：
//   - 滞回比较（施密特触发器）：peakMin 以上视为进入峰值，valleyMax 以下视为回到谷值，
//     两者之间保持上一拍状态，避免模拟量残余底噪（如交流电残余电流 0.x~1.x A）
//     在单一阈值附近抖动反复触发
//   - 自动学习峰/谷值：滑动窗口 P_high / P_(100-high) 百分位法，支持启动预热 + 周期重学
//
// 规则引用 1 个变量时为单点峰值计数；引用多个变量时，每个点位独立做滞回判定，
// 再按 logic（and/or/n_of_m）组合，对组合电平做边沿计数。
// 多点位共用同一套峰谷参数；若各点位幅度不同，请先用点位池的 Scale 倍率归一化。
// 单点位时引用变量上的本点阈值可覆盖 peakMin。
//
// 计数仅在边沿跳变时触发（rising/falling/both），峰值持续期间不重复计数。
// 计数仅存内存，进程重启清零（引擎层 SetInitialCount 从 DB 恢复当日累计），
// 跨自然日 00:00 由 rollDay 清零；学习窗口跨日保留（学习状态本身与日无关）。
//
// 兼容旧类型：counter_ac 在引擎层归一化为本算子，参数完全兼容。
// ===========================================================================

type analogCounterFunc struct{}

func (analogCounterFunc) Meta() FuncMeta {
	return FuncMeta{
		Type:    "counter_analog",
		CName:   "模拟峰值计数",
		Desc:    "统计连续模拟量(电流/电压等)的峰值出现次数：达到峰值最低值记一次，回落到谷值最高值以下完成一次；中间区域保持状态，抗底噪抖动；可自动学习峰谷。继续添加点位即变为多点位组合计数",
		MultiIn: true,
		Params: []ParamMeta{
			{Name: "edge", CName: "计数边沿", Type: "select", Default: "rising",
				Options: []ParamOption{
					{Label: "上升沿(谷→峰)", Value: "rising"},
					{Label: "下降沿(峰→谷)", Value: "falling"},
					{Label: "双向沿", Value: "both"},
				}},
			{Name: "autoLearn", CName: "自动学习峰谷", Type: "bool", Default: false,
				Desc: "开启后按滑动窗口自动学习峰谷值，无需手动填写"},
			// 手动峰谷：仅 autoLearn=false 时前端展示
			{Name: "peakMin", CName: "峰值最低值", Type: "number", Default: 1,
				Desc: "采样值 ≥ 此值视为达到峰值"},
			{Name: "valleyMax", CName: "谷值最高值", Type: "number", Default: 0,
				Desc: "采样值 < 此值视为回到谷值，按信号残余底噪填写"},
			// 学习参数：仅 autoLearn=true 时前端展示
			{Name: "learnWindow", CName: "学习窗口", Type: "number", Default: 200,
				Desc: "滑动窗口采样点数，按百分位计算峰谷"},
			{Name: "warmupSamples", CName: "预热样本数", Type: "number", Default: 50,
				Desc: "启动后采够这么多样本才开始第一次学习"},
			{Name: "learnInterval", CName: "重学间隔(秒)", Type: "number", Default: 300,
				Desc: "周期重学间隔，0 表示只在预热后学习一次"},
			{Name: "learnPercentile", CName: "学习百分位", Type: "number", Default: 95,
				Desc: "如 95 表示 peakMin=P95、valleyMax=P5"},
			// 组合条件：仅多点位（Vars>1）时前端展示
			{Name: "logic", CName: "组合条件", Type: "select", Default: "and",
				Desc: "仅多个点位时生效",
				Options: []ParamOption{
					{Label: "与(全部满足)", Value: "and"},
					{Label: "或(任一满足)", Value: "or"},
					{Label: "满足N个", Value: "n_of_m"},
				}},
			{Name: "requireCount", CName: "满足个数N", Type: "number", Default: 1,
				Desc: "仅组合条件为“满足N个”时生效"},
		},
	}
}

func (analogCounterFunc) NewState(rule Rule) (State, error) {
	if len(rule.Vars) == 0 {
		return nil, fmt.Errorf("模拟峰值计数至少需要 1 个引用变量")
	}
	peakMin := rule.Params.Float("peakMin", 1)
	valleyMax := rule.Params.Float("valleyMax", 0)
	if valleyMax > peakMin {
		// 容错：谷高于峰，互换
		peakMin, valleyMax = valleyMax, peakMin
	}

	single := len(rule.Vars) == 1
	cfgs := make([]analogInputCfg, 0, len(rule.Vars))
	for _, vr := range rule.Vars {
		pm := peakMin
		// 单点位时本点阈值覆盖 peakMin（兼容旧 counter_ac 行为）；
		// 多点位时统一使用全局峰谷，避免各点判定不对称
		if single && vr.HasThreshold {
			pm = vr.Threshold
		}
		cfgs = append(cfgs, analogInputCfg{varName: vr.Var, peakMin: pm, valleyMax: valleyMax})
	}

	windowSize := rule.Params.Int("learnWindow", 200)
	if windowSize < 10 {
		windowSize = 10
	}
	warmup := rule.Params.Int("warmupSamples", 50)
	if warmup < 5 {
		warmup = 5
	}
	if warmup > windowSize {
		warmup = windowSize
	}
	pct := rule.Params.Float("learnPercentile", 95)
	if pct <= 50 || pct >= 100 {
		pct = 95
	}
	learnInterval := time.Duration(rule.Params.Int("learnInterval", 300)) * time.Second
	if learnInterval < 0 {
		learnInterval = 0 // 0 表示禁用周期重学
	}
	n := rule.Params.Int("requireCount", 1)
	if n < 1 {
		n = 1
	}
	if n > len(cfgs) {
		n = len(cfgs)
	}
	return &analogCounterState{
		cfgs:          cfgs,
		edge:          rule.Params.String("edge", "rising"),
		logic:         rule.Params.String("logic", "and"),
		requireCount:  n,
		autoLearn:     rule.Params.Float("autoLearn", 0) != 0,
		windowSize:    windowSize,
		warmup:        warmup,
		learnInterval: learnInterval,
		pct:           pct,
		window:        make([]float64, 0, windowSize),
	}, nil
}

type analogInputCfg struct {
	varName   string
	peakMin   float64
	valleyMax float64
}

type analogCounterState struct {
	// 用户配置参数
	cfgs          []analogInputCfg
	edge          string
	logic         string
	requireCount  int
	autoLearn     bool
	windowSize    int
	warmup        int
	learnInterval time.Duration
	pct           float64

	// 运行状态
	lastHigh *bool
	count    int64
	day      string
	logger   *zap.Logger

	// 学习状态
	window           []float64 // 环形缓冲区：保留最近 windowSize 个采样值（多点位时混合入窗）
	samples          int       // 总采样点数（区分预热是否完成）
	warmupDone       bool
	learnedPeakMin   float64
	learnedValleyMax float64
	lastLearnTime    time.Time
}

// SetLogger 实现 LoggerAware
func (s *analogCounterState) SetLogger(l *zap.Logger) { s.logger = l }

// Count 实现 CounterState
func (s *analogCounterState) Count() int64 { return s.count }

// SetInitialCount 实现 CounterState：从 DB 恢复当日累计；
// 同时清空 lastHigh，使下次采样仅建立基线（注意：学习状态保留，重启后会重新预热）
func (s *analogCounterState) SetInitialCount(count int64) {
	s.count = count
	s.lastHigh = nil
}

// Analyze 实现 Analyzable：手动触发分析，返回当前窗口的 P_high/P_low 与样本数。
// 不写入运行参数，仅返回建议值供前端预览/确认。
func (s *analogCounterState) Analyze() (peakMin, valleyMax float64, samples int) {
	n := len(s.window)
	if n < 5 {
		// 样本不足，返回用户配置值，samples 告诉前端真实情况
		return s.cfgs[0].peakMin, s.cfgs[0].valleyMax, n
	}
	sorted := make([]float64, n)
	copy(sorted, s.window)
	sort.Float64s(sorted)
	hi := int(s.pct / 100 * float64(n-1))
	lo := int((100 - s.pct) / 100 * float64(n-1))
	if hi >= n {
		hi = n - 1
	}
	if lo < 0 {
		lo = 0
	}
	return sorted[hi], sorted[lo], n
}

// AnalyzeFull 返回完整分析信息（包含已学习的值与状态）
func (s *analogCounterState) AnalyzeFull() map[string]interface{} {
	p, v, n := s.Analyze()
	return map[string]interface{}{
		"suggestedPeakMin":    p,
		"suggestedValleyMax":  v,
		"samples":             n,
		"warmupDone":          s.warmupDone,
		"learnedPeakMin":      s.learnedPeakMin,
		"learnedValleyMax":    s.learnedValleyMax,
		"configuredPeakMin":   s.cfgs[0].peakMin,
		"configuredValleyMax": s.cfgs[0].valleyMax,
		"autoLearn":           s.autoLearn,
	}
}

func (s *analogCounterState) rollDay(day string) {
	if day != "" && s.day != "" && day != s.day {
		s.count = 0
		s.lastHigh = nil // 跨日重新建立基线
		// 注意：学习窗口跨日保留，避免零点重新预热
	}
	s.day = day
}

// pushSample 把样本加入环形缓冲区（多点位时全部点位样本混合入窗，
// 配合点位池 Scale 倍率把各点归一化到相近量纲）
func (s *analogCounterState) pushSample(v float64) {
	if len(s.window) < s.windowSize {
		s.window = append(s.window, v)
	} else {
		// 环形覆盖最旧元素
		s.window[s.samples%s.windowSize] = v
	}
	s.samples++
}

// learn 触发一次学习，更新 learnedPeakMin/learnedValleyMax
func (s *analogCounterState) learn() {
	n := len(s.window)
	if n < 5 {
		return
	}
	sorted := make([]float64, n)
	copy(sorted, s.window)
	sort.Float64s(sorted)
	hi := int(s.pct / 100 * float64(n-1))
	lo := int((100 - s.pct) / 100 * float64(n-1))
	if hi >= n {
		hi = n - 1
	}
	if lo < 0 {
		lo = 0
	}
	s.learnedPeakMin = sorted[hi]
	s.learnedValleyMax = sorted[lo]
	s.warmupDone = true
	s.lastLearnTime = time.Now()
}

// hysteresis 单点施密特滞回判定：peak 及以上为高、valley 及以下为低、中间保持上次状态。
// 注意用 <= 而非 <：当 valleyMax=0 且信号回到 0 时，0<=0 为 true 才能触发"回到谷值"，
// 否则信号达到峰值后永远保持高，没有 rising edge，计数永远为 0。
func hysteresis(val, peak, valley float64, last *bool) bool {
	if val >= peak {
		return true
	}
	if val <= valley {
		return false
	}
	if last != nil {
		return *last
	}
	return false // 首次进入中间区域，视为低
}

func (s *analogCounterState) Evaluate(in map[string]float64, day string) (interface{}, bool, error) {
	s.rollDay(day)

	// 样本入窗口（无论 autoLearn 是否开启都入窗，方便手动分析接口）
	for _, c := range s.cfgs {
		s.pushSample(in[c.varName])
	}

	// 自动学习触发
	if s.autoLearn {
		now := time.Now()
		if !s.warmupDone {
			if s.samples >= s.warmup {
				s.learn()
			}
		} else if s.learnInterval > 0 && now.Sub(s.lastLearnTime) >= s.learnInterval {
			s.learn()
		}
	}

	// 决定当前使用的阈值：autoLearn 启用且学习完成后用学习值，否则用用户配置
	curPeakMin := s.cfgs[0].peakMin
	curValleyMax := s.cfgs[0].valleyMax
	if s.autoLearn && s.warmupDone {
		curPeakMin = s.learnedPeakMin
		curValleyMax = s.learnedValleyMax
	}

	// 每个点位独立滞回判定。注意多点位共用一组峰谷时，各点的"上拍状态"
	// 无法用一个 lastHigh 表达——这里以组合电平的上拍状态作为保持回退，
	// 中间区域的单点保持在多点位场景退化为组合保持，语义仍然稳定。
	highCount := 0
	inputStatus := make([]map[string]interface{}, 0, len(s.cfgs))
	for _, c := range s.cfgs {
		val := in[c.varName]
		peak, valley := curPeakMin, curValleyMax
		isHigh := hysteresis(val, peak, valley, s.lastHigh)
		if isHigh {
			highCount++
		}
		inputStatus = append(inputStatus, map[string]interface{}{
			"var":       c.varName,
			"value":     val,
			"peakMin":   peak,
			"valleyMax": valley,
			"high":      isHigh,
		})
	}
	var combo bool
	switch s.logic {
	case "or":
		combo = highCount >= 1
	case "n_of_m":
		combo = highCount >= s.requireCount
	default: // and
		combo = highCount == len(s.cfgs)
	}

	triggered := false
	reason := "baseline-init"
	if s.lastHigh != nil {
		prev := *s.lastHigh
		rising := !prev && combo
		falling := prev && !combo
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
		s.logger.Debug("analog-counter evaluate",
			zap.Any("inputs", inputStatus),
			zap.Int("highCount", highCount),
			zap.Int("requireCount", s.requireCount),
			zap.String("logic", s.logic),
			zap.Float64("curPeakMin", curPeakMin),
			zap.Float64("curValleyMax", curValleyMax),
			zap.Bool("combo", combo),
			zap.Any("lastHigh", s.lastHigh),
			zap.String("edge", s.edge),
			zap.String("reason", reason),
			zap.Bool("triggered", triggered),
			zap.Int64("count", s.count),
			zap.Int("samples", s.samples),
			zap.Bool("warmupDone", s.warmupDone),
			zap.Float64("learnedPeakMin", s.learnedPeakMin),
			zap.Float64("learnedValleyMax", s.learnedValleyMax),
			zap.String("day", day),
		)
	}

	h := combo
	s.lastHigh = &h
	return s.count, true, nil
}
