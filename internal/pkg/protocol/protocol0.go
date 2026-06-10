package protocol

import "context"

// Metadata 用于统一承载所有协议的参数
// 使用 map[string]any 而非命名类型，确保跨包兼容性（不产生类型不匹配问题）
type Metadata = map[string]any

type DeviceCommProtocol interface {
	Info() Metadata

	Connect(ctx context.Context, params Metadata) error
	Disconnect(ctx context.Context) error
	IsConnected() bool
	//是否支持服务端？如果支持会自动启动对外提供mqtt接口，支持ReadBatch和WriteBatch方法
	IsSupportServer() bool

	// ✅ 批量读
	ReadBatch(ctx context.Context, req BatchReadRequest) (*BatchReadResponse, error)

	// ✅ 批量写
	WriteBatch(ctx context.Context, req BatchWriteRequest) error
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

//Write Request
type BatchWriteItem struct {
	Point    Point
	Value    interface{}
	Priority int // 可选（控制顺序 / 覆盖策略）
}

type BatchWriteRequest struct {
	Items   []BatchWriteItem
	Options Metadata
}

//Write Response
type BatchWriteResult struct {
	PointName string
	Success   bool
	Error     error
}

type BatchWriteResponse struct {
	Results []BatchWriteResult
}
