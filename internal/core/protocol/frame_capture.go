package protocol

import "sync"

// 帧方向常量
const (
	FrameDirectionTX = "tx" // 下发（主站→从站）
	FrameDirectionRX = "rx" // 接收（从站→主站）
)

// FrameItem 单帧数据（十六进制字符串）
type FrameItem struct {
	Direction string `json:"dir"` // tx / rx
	Hex       string `json:"hex"` // 空格分隔的十六进制字节，如 "32 03 02 57 00 01 31 a1"
}

// FrameGroup 一次完整交互（一次请求 + 对应响应）。
// 超时时 Frames 中可能只有 tx 没有 rx。
type FrameGroup struct {
	mu     sync.Mutex
	Meta   Metadata    `json:"meta"`   // 该次交互的可读上下文（功能码、地址、数量等）
	Frames []FrameItem `json:"frames"` // 收发帧，按时间顺序
}

// Add 追加一帧
func (g *FrameGroup) Add(direction, hex string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.Frames = append(g.Frames, FrameItem{Direction: direction, Hex: hex})
}

// FrameCapture 调试帧抓取器。由上层（如调试 handler）创建，
// 通过 BatchReadRequest/BatchWriteRequest 的 Options["frameCapture"] 传入，
// 协议实现将底层每次通信的组（含上下文元数据与收发帧）写入其中。
type FrameCapture struct {
	mu     sync.Mutex
	groups []*FrameGroup
}

// NewFrameCapture 创建帧抓取器
func NewFrameCapture() *FrameCapture {
	return &FrameCapture{}
}

// BeginGroup 开始一次交互组，返回该组句柄供协议写入收发帧
func (c *FrameCapture) BeginGroup(meta Metadata) *FrameGroup {
	g := &FrameGroup{Meta: meta}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.groups = append(c.groups, g)
	return g
}

// Groups 返回所有交互组
func (c *FrameCapture) Groups() []*FrameGroup {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]*FrameGroup, len(c.groups))
	copy(out, c.groups)
	return out
}

// FrameCaptureKey Options 中存放帧抓取器的键名
const FrameCaptureKey = "frameCapture"
