// Package protocol 提供统一的协议接口框架。
//
// DeviceCommProtocol 是核心接口，插件和内置协议共用。
// 通过 ProtocolRegistry 统一管理和获取。
package protocol

import (
	"context"
)

// DeviceHandle 设备句柄，Connect 返回，用于后续所有操作标识具体设备
type DeviceHandle uint64

// InvalidHandle 表示无效的设备句柄
const InvalidHandle DeviceHandle = 0

type DeviceCommProtocol interface {
	Info() Metadata

	// Connect 连接设备，返回设备句柄供后续操作使用
	Connect(ctx context.Context, params Metadata) (DeviceHandle, error)
	// Disconnect 断开指定句柄的设备连接
	Disconnect(ctx context.Context, handle DeviceHandle) error
	// IsConnected 检查指定句柄的设备是否已连接
	IsConnected(handle DeviceHandle) bool
	// IsSupportServer 是否支持服务端？如果支持会自动启动对外提供mqtt接口，支持ReadBatch和WriteBatch方法
	IsSupportServer() bool

	// ReadBatch 批量读（按句柄区分设备）
	ReadBatch(ctx context.Context, handle DeviceHandle, req BatchReadRequest) (*BatchReadResponse, error)

	// WriteBatch 批量写（按句柄区分设备）
	WriteBatch(ctx context.Context, handle DeviceHandle, req BatchWriteRequest) error
}

type Point struct {
	Name     string // 点位名（业务层用）
	Resource string // 协议地址（40001 / DB1.DBW0）
	DataType string // bool / int16 / float32 / string
	Count    int    // 长度（字符串/数组用）
}

type BatchReadRequest struct {
	Points  []Point
	Options Metadata // 协议私有参数（超时、缓存等）
}

type BatchReadResult struct {
	PointName string
	Value     interface{}
	Quality   string // good / bad / uncertain
	Error     error
}

type BatchReadResponse struct {
	Results []BatchReadResult
	Raw     []byte // 原始报文（可选）
}

// Write Request
type BatchWriteItem struct {
	Point    Point
	Value    interface{}
	Priority int // 可选（控制顺序 / 覆盖策略）
}

type BatchWriteRequest struct {
	Items   []BatchWriteItem
	Options Metadata
}

// Write Response
type BatchWriteResult struct {
	PointName string
	Success   bool
	Error     error
}

type BatchWriteResponse struct {
	Results []BatchWriteResult
}
