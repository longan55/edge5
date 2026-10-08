package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// EdgeInput 边缘计算任务的顶层采集点位（点位池）。
// 多条规则可引用同一变量，引擎按 Address 去重，只采一次。
//   - Var: 表达式/条件中引用的变量名（如 a/b/c/电流）
//   - Address: 设备协议点位地址（如 0x3A / 40059）
//   - ParseType: 解析类型
//   - Expr: 采样值计算表达式，x 表示设备读到的原始值，仅支持 + - * / ( ) 与数字，
//     如 x*0.1、(x-4)*2.5+20；为空视为 x（原值）。所有算子看到的是计算后的值
//   - Scale Deprecated: 旧版倍率字段，仅用于兼容老数据；Expr 为空且 Scale≠1 时按 x*Scale 处理
type EdgeInput struct {
	Var       string  `json:"var"`
	Address   string  `json:"address"`
	ParseType string  `json:"parseType"`
	Expr      string  `json:"expr,omitempty"`
	Scale     float64 `json:"scale,omitempty"`
}

// EdgeVarRef 规则对点位池变量的引用，可附带独立阈值（仅计数类算子有效，覆盖算子统一阈值）
type EdgeVarRef struct {
	Var       string   `json:"var"`
	Threshold *float64 `json:"threshold,omitempty"`
}

// EdgeRule 一条边缘计算规则，对应一个计算后的输出点位（上报字段名为 Name）。
//   - FuncType: 算子类型（counter_digital 数字信号计数 / counter_analog 模拟峰值计数 /
//     arithmetic 四则运算；旧类型 counter_pulse/counter_multi/counter_ac 由引擎自动归一化）
//   - Vars: 引用顶层 EdgeInput 的 Var 名（可选携带独立阈值）；计数类规则引用多个变量时为组合计数
//   - Params: 算子参数（阈值、边沿、与或逻辑、满足个数、峰谷学习、表达式等）
type EdgeRule struct {
	Name     string                 `json:"name"`
	FuncType string                 `json:"funcType"`
	Vars     []EdgeVarRef           `json:"vars"`
	Params   map[string]interface{} `json:"params"`
}

// EdgeRules 规则切片，GORM 自定义 JSON 序列化
type EdgeRules []EdgeRule

func (r *EdgeRules) Scan(value any) error {
	if value == nil {
		*r = EdgeRules{}
		return nil
	}
	switch v := value.(type) {
	case []byte:
		return json.Unmarshal(v, r)
	case string:
		return json.Unmarshal([]byte(v), r)
	default:
		return fmt.Errorf("unsupported type for EdgeRules: %T", value)
	}
}

func (r EdgeRules) Value() (driver.Value, error) {
	if r == nil {
		return "[]", nil
	}
	return json.Marshal(r)
}

// EdgeInputs 点位池切片，GORM 自定义 JSON 序列化
type EdgeInputs []EdgeInput

func (i *EdgeInputs) Scan(value any) error {
	if value == nil {
		*i = EdgeInputs{}
		return nil
	}
	switch v := value.(type) {
	case []byte:
		return json.Unmarshal(v, i)
	case string:
		return json.Unmarshal([]byte(v), i)
	default:
		return fmt.Errorf("unsupported type for EdgeInputs: %T", value)
	}
}

func (i EdgeInputs) Value() (driver.Value, error) {
	if i == nil {
		return "[]", nil
	}
	return json.Marshal(i)
}

// EdgeTask 边缘计算任务：独立于普通采集任务，仅采集点位池声明的点位，
// 周期采样后经边缘计算引擎计算，上报计算结果到独立 MQTT 主题。
type EdgeTask struct {
	BaseModel
	Name string `gorm:"size:128;not null" json:"name"`
	// DeviceID 关联设备
	DeviceID uint64 `gorm:"not null;index" json:"deviceId"`
	// UpTopic 结果上报主题，空则使用默认主题模板
	UpTopic string `gorm:"size:256" json:"upTopic"`
	// ReadInterval Deprecated: 改用 SampleInterval；仅为兼容老库保留，新代码请用 SampleInterval/ReportInterval
	ReadInterval int `gorm:"default:0" json:"readInterval,omitempty"`
	// SampleInterval 采样间隔秒，0 时代码回退到 1s。计数依赖边沿检测，建议 1-2s
	SampleInterval int `gorm:"default:0" json:"sampleInterval"`
	// ReportInterval 上报间隔秒，0 时代码回退到 10s
	ReportInterval int  `gorm:"default:0" json:"reportInterval"`
	State          bool `gorm:"default:false" json:"state"`
	// Inputs 顶层点位池：声明全部要采集的点位（变量名/地址/解析/倍率），规则按 Var 名引用
	Inputs EdgeInputs `gorm:"type:text;serializer:json" json:"inputs"`
	// Rules 计算规则列表，每条规则产生一个上报字段
	Rules EdgeRules `gorm:"type:text;serializer:json" json:"rules"`
}

func (EdgeTask) TableName() string {
	return "edge_tasks"
}
