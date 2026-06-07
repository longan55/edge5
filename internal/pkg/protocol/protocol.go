// Package protocol 提供统一的协议接口框架。
//
// 插件和内置协议共用同一套 Protocol 接口，
// 通过 ProtocolRegistry 统一管理和获取。
package protocol

import (
	"context"
	"encoding/json"
	"fmt"
)

// ConnectionParam 定义协议连接参数
// 前端根据此信息动态生成表单
type ConnectionParam struct {
	Name     string   `json:"name"`              // 参数字段名，如 "ip"
	CName    string   `json:"cName"`             // 中文名，如 "IP地址"
	Type     string   `json:"type"`              // 类型：string/int/float/bool
	Required bool     `json:"required"`          // 是否必填
	Default  string   `json:"default,omitempty"` // 默认值
	Choices  []string `json:"choices,omitempty"` // 枚举值列表（非空时前端渲染为下拉框）
}

// ProtocolInfo 协议信息
type ProtocolInfo struct {
	Name             string            `json:"name"`                 // 协议名称，如 "MC-3E"
	Version          string            `json:"version"`              // 协议版本
	DeviceType       string            `json:"deviceType"`           // 设备类型，如 "PLC"
	Brand            string            `json:"brand"`                // 品牌，如 "Mitsubishi"
	Models           []string          `json:"models"`               // 支持的设备型号
	ConnectionParams []ConnectionParam `json:"connectionParams"`     // 连接参数字段定义
	Source           string            `json:"source"`               // 来源：builtin / plugin
	PluginPath       string            `json:"pluginPath,omitempty"` // 仅 plugin 模式：插件可执行文件路径
}

// DataMessage 订阅数据消息
type DataMessage struct {
	DeviceSn  string
	Values    map[string][]byte
	Timestamp int64
}

// Protocol 统一协议接口
// 插件和内置协议都实现此接口
type Protocol interface {
	// Info 返回协议信息
	Info() ProtocolInfo

	// Connect 建立设备连接
	Connect(ctx context.Context, deviceSn string, params map[string]string) error

	// Disconnect 断开设备连接
	Disconnect(ctx context.Context, deviceSn string) error

	// ReadData 读取数据
	ReadData(ctx context.Context, deviceSn string, addresses []string) (map[string][]byte, error)

	// WriteData 写入数据
	WriteData(ctx context.Context, deviceSn string, values map[string][]byte) error

	// SubscribeData 订阅实时数据，返回只读 channel
	SubscribeData(ctx context.Context, deviceSn string, addresses []string, interval int32) (<-chan DataMessage, error)
}

// ProtocolRegistry 协议注册表接口
type ProtocolRegistry interface {
	// Register 注册一个协议实现
	Register(proto Protocol) error

	// Get 根据协议名称获取协议实现
	Get(name string) (Protocol, bool)

	// List 列出所有已注册的协议信息
	List() []ProtocolInfo

	// SyncToDB 将注册表同步到数据库
	SyncToDB() error

	// StartPlugins 启动所有 gRPC 插件进程
	StartPlugins() error

	// StopPlugins 停止所有插件进程
	StopPlugins() error
}

// ErrProtocolNotFound 协议未找到
var ErrProtocolNotFound = fmt.Errorf("protocol not found")

// ConnectionParamsToJSON 将 ConnectionParam 切片序列化为 JSON 字符串
func ConnectionParamsToJSON(params []ConnectionParam) string {
	data, err := json.Marshal(params)
	if err != nil {
		return "[]"
	}
	return string(data)
}

// ConnectionParamsFromJSON 从 JSON 字符串反序列化 ConnectionParam 切片
func ConnectionParamsFromJSON(data string) ([]ConnectionParam, error) {
	var params []ConnectionParam
	if err := json.Unmarshal([]byte(data), &params); err != nil {
		return nil, fmt.Errorf("unmarshal connection params failed: %w", err)
	}
	return params, nil
}
