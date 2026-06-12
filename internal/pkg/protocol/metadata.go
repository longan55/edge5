package protocol

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Metadata 用于统一承载所有协议的参数
// 使用 map[string]any 而非命名类型，确保跨包兼容性（不产生类型不匹配问题）
type Metadata = map[string]any

// ---------------------------------------------------------------------------
// 协议元信息提取工具（从 Metadata 中读取标准字段）
// ---------------------------------------------------------------------------

// GetInfoString 从 Metadata 中安全读取字符串字段
func GetInfoString(m Metadata, key string) string {
	if m == nil {
		return ""
	}
	v, _ := m[key].(string)
	return v
}

// GetInfoStrings 从 Metadata 中安全读取字符串切片字段
func GetInfoStrings(m Metadata, key string) []string {
	if m == nil {
		return nil
	}
	v, _ := m[key].([]string)
	return v
}

// GetInfoSlice 从 Metadata 中安全读取 []any 字段
func GetInfoSlice(m Metadata, key string) []any {
	if m == nil {
		return nil
	}
	v, _ := m[key].([]any)
	return v
}

// GetInfoBool 从 Metadata 中安全读取布尔字段
func GetInfoBool(m Metadata, key string, defaultValue bool) bool {
	if m == nil {
		return defaultValue
	}
	if v, ok := m[key].(bool); ok {
		return v
	}
	return defaultValue
}

// GetInfoAlias 从 Metadata 中获取协议别名
func GetInfoAlias(m Metadata) string {
	if m == nil {
		return ""
	}
	v, _ := m["alias"].(string)
	return v
}

// MatchProtocolName 检查给定的协议名称是否匹配（支持别名）
func MatchProtocolName(info Metadata, protocolName string) bool {
	if info == nil || protocolName == "" {
		return false
	}
	name := GetInfoString(info, "name")
	alias := GetInfoAlias(info)
	// 精确匹配名称或别名
	if name == protocolName || alias == protocolName {
		return true
	}
	// 大小写不敏感匹配
	if strings.EqualFold(name, protocolName) || strings.EqualFold(alias, protocolName) {
		return true
	}
	return false
}

// ConnectionParam 连接参数定义（用于 Informational 目的 — 前端表单生成）
type ConnectionParam struct {
	Name     string   `json:"name"`
	CName    string   `json:"cName"`
	Type     string   `json:"type"`
	Required bool     `json:"required"`
	Default  string   `json:"default,omitempty"`
	Choices  []string `json:"choices,omitempty"`
}

// ExtractConnectionParams 从 Metadata 中抽取 ConnectionParam 列表
// Metadata 中 key="connectionParams" 且值类型为 []any, 每个元素 map[string]any
func ExtractConnectionParams(m Metadata) []ConnectionParam {
	raw := GetInfoSlice(m, "connectionParams")
	if len(raw) == 0 {
		return nil
	}
	result := make([]ConnectionParam, 0, len(raw))
	for _, r := range raw {
		if item, ok := r.(map[string]any); ok {
			cp := ConnectionParam{
				Name:     toString(item["name"]),
				CName:    toString(item["cName"]),
				Type:     toString(item["type"]),
				Required: toBool(item["required"]),
				Default:  toString(item["default"]),
			}
			if choices, ok := item["choices"].([]any); ok {
				for _, c := range choices {
					cp.Choices = append(cp.Choices, toString(c))
				}
			}
			result = append(result, cp)
		}
	}
	return result
}

// ConnectionParamsToJSON 将 ConnectionParam 切片序列化为 JSON
func ConnectionParamsToJSON(params []ConnectionParam) string {
	data, err := json.Marshal(params)
	if err != nil {
		return "[]"
	}
	return string(data)
}

// ConnectionParamsFromJSON 从 JSON 反序列化 ConnectionParam 切片
func ConnectionParamsFromJSON(data string) ([]ConnectionParam, error) {
	var params []ConnectionParam
	if err := json.Unmarshal([]byte(data), &params); err != nil {
		return nil, fmt.Errorf("unmarshal connection params failed: %w", err)
	}
	return params, nil
}

func toString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

func toBool(v any) bool {
	if v == nil {
		return false
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}

// ---------------------------------------------------------------------------
// 采集参数 Schema（readParamsSchema）
// ---------------------------------------------------------------------------

// ReadParamSchema 采集参数定义（协议声明可采集的字段及类型）
type ReadParamSchema struct {
	Name    string   `json:"name"`
	CName   string   `json:"cName"`
	Type    string   `json:"type"`
	Default string   `json:"default,omitempty"`
	Choices []string `json:"choices,omitempty"`
}

// ExtractReadParamsSchema 从 Metadata 中抽取 ReadParamSchema 列表
// Metadata 中 key="readParamsSchema" 且值类型为 []any，每个元素 map[string]any
func ExtractReadParamsSchema(m Metadata) []ReadParamSchema {
	raw := GetInfoSlice(m, "readParamsSchema")
	if len(raw) == 0 {
		return nil
	}
	result := make([]ReadParamSchema, 0, len(raw))
	for _, r := range raw {
		if item, ok := r.(map[string]any); ok {
			schema := ReadParamSchema{
				Name:    toString(item["name"]),
				CName:   toString(item["cName"]),
				Type:    toString(item["type"]),
				Default: toString(item["default"]),
			}
			if choices, ok := item["choices"].([]any); ok {
				for _, c := range choices {
					schema.Choices = append(schema.Choices, toString(c))
				}
			} else if choices, ok := item["choices"].([]string); ok {
				schema.Choices = choices
			}
			result = append(result, schema)
		}
	}
	return result
}
