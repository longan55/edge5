// Package edge 实现边缘计算引擎：算子注册表 + 规则有状态运行时。
//
// 设计目标：
//   - 计算方式插件化，新增算子只需实现 ComputeFunc 并在 init 中 Register，引擎与调度不变
//   - 引擎不依赖 model/service，输入为 map[地址]float64，便于单元测试
//   - 计数类算子为有状态运行时；计数只保存在内存（进程重启清零），并在跨自然日 00:00 自动清零
package edge

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

// Input 顶层点位池的一个采集点位（引擎内部模型，由 service 从 model.EdgeInput 转换）。
//   - Var: 表达式/条件中引用的变量名
//   - Address: 设备协议点位地址
//   - ParseType: 解析类型
//   - Expr: 采样值计算表达式（x=原始值），service 层在采样后立即求值，
//     算子看到的是计算后的真实物理量；空表达式等价于 x
type Input struct {
	Var       string
	Address   string
	ParseType string
	Expr      string
}

// Params 算子参数
type Params map[string]interface{}

// Float 取数值参数，支持 float64/int/string
func (p Params) Float(key string, def float64) float64 {
	v, ok := p[key]
	if !ok || v == nil {
		return def
	}
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case json.Number:
		f, _ := x.Float64()
		return f
	case string:
		if f, err := strconv.ParseFloat(strings.TrimSpace(x), 64); err == nil {
			return f
		}
	}
	return def
}

// Int 取整型参数
func (p Params) Int(key string, def int) int {
	return int(p.Float(key, float64(def)))
}

// String 取字符串参数
func (p Params) String(key, def string) string {
	v, ok := p[key]
	if !ok || v == nil {
		return def
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return def
}

// VarRef 规则对点位池变量的引用，可附带独立阈值（仅计数类算子有效，覆盖算子统一阈值）
type VarRef struct {
	Var          string
	Threshold    float64
	HasThreshold bool
}

// Rule 一条计算规则（引擎内部模型）
//   - Vars: 引用顶层 Input 的 Var 名（可选携带独立阈值）
//   - Params: 算子参数（统一阈值、边沿、表达式等）
type Rule struct {
	Name     string
	FuncType string
	Vars     []VarRef
	Params   Params
}

// ParamOption 下拉参数选项
type ParamOption struct {
	Label string      `json:"label"`
	Value interface{} `json:"value"`
}

// ParamMeta 算子参数元信息，供前端动态渲染表单
type ParamMeta struct {
	Name    string        `json:"name"`
	CName   string        `json:"cName"`
	Type    string        `json:"type"` // number / select / string
	Default interface{}   `json:"default,omitempty"`
	Options []ParamOption `json:"options,omitempty"`
	Desc    string        `json:"desc,omitempty"`
}

// FuncMeta 算子元信息
type FuncMeta struct {
	Type    string      `json:"type"`
	CName   string      `json:"cName"`
	Desc    string      `json:"desc"`
	MultiIn bool        `json:"multiInput"` // 是否支持多个输入点位
	Params  []ParamMeta `json:"params"`
}

// State 单条规则的有状态运行时
type State interface {
	// Evaluate 输入为本规则变量值（map[var]float64），day 为当前本地日期（2006-01-02，用于跨日清零）。
	// 返回输出值、是否产生输出、错误。
	Evaluate(in map[string]float64, day string) (value interface{}, emit bool, err error)
}

// ComputeFunc 算子：负责描述元信息并为每条规则创建运行时
type ComputeFunc interface {
	Meta() FuncMeta
	NewState(rule Rule) (State, error)
}

// CounterState 计数类算子的有状态运行时额外实现：引擎据此跟踪 delta 并持久化。
// Count 返回当前累计计数；SetInitialCount 用于启动时从 DB 恢复内存计数。
type CounterState interface {
	State
	Count() int64
	SetInitialCount(count int64)
}

// LoggerAware 让算子运行时接收引擎的 logger，便于在内部决策处打 DEBUG 日志。
// 引擎在 NewEngine 创建 state 后调用 SetLogger；不实现该接口的算子忽略日志。
type LoggerAware interface {
	SetLogger(*zap.Logger)
}

// Analyzable 由支持在线分析的算子实现（如 counter_ac 的滑动窗口峰谷学习）。
// 引擎在收到外部手动分析请求时调用，返回算子内部建议值，不修改运行参数。
type Analyzable interface {
	// AnalyzeFull 返回算子内部的完整分析信息（结构由算子自定义）
	AnalyzeFull() map[string]interface{}
}

var registry = map[string]ComputeFunc{}

// Register 注册算子
func Register(f ComputeFunc) {
	meta := f.Meta()
	if meta.Type == "" {
		panic("edge: 算子类型不能为空")
	}
	registry[meta.Type] = f
}

// GetFunc 按类型获取算子
func GetFunc(t string) (ComputeFunc, error) {
	f, ok := registry[t]
	if !ok {
		return nil, fmt.Errorf("不支持的边缘计算算子: %s", t)
	}
	return f, nil
}

// ListFuncs 列出全部算子元信息（顺序固定：注册顺序不保证，这里按类型排序）
func ListFuncs() []FuncMeta {
	types := make([]string, 0, len(registry))
	for t := range registry {
		types = append(types, t)
	}
	// 简单稳定排序
	for i := 0; i < len(types); i++ {
		for j := i + 1; j < len(types); j++ {
			if types[j] < types[i] {
				types[i], types[j] = types[j], types[i]
			}
		}
	}
	metas := make([]FuncMeta, 0, len(types))
	for _, t := range types {
		metas = append(metas, registry[t].Meta())
	}
	return metas
}

// DayKey 返回本地日期键，供跨日清零比较
func DayKey(t time.Time) string {
	return t.Format("2006-01-02")
}
