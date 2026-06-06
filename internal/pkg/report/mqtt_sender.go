package report

import (
	"context"
	"fmt"

	"go.uber.org/zap"
)

// MQTTPublisher MQTT 发布接口
// 用于解耦具体 MQTT 客户端实现
type MQTTPublisher interface {
	// IsConnected 返回连接状态
	IsConnected() bool
	// Publish 发布消息到指定主题
	Publish(topic string, qos byte, payload []byte) error
}

// mqttSender 基于 MQTT 的 Sender 实现
type mqttSender struct {
	name   string
	pub    MQTTPublisher
	logger *zap.Logger
}

// MQTTSenderOption 配置选项
type MQTTSenderOption func(*mqttSender)

// WithMQTTLogger 设置日志记录器
func WithMQTTLogger(logger *zap.Logger) MQTTSenderOption {
	return func(s *mqttSender) {
		s.logger = logger
	}
}

// WithMQTTName 设置发送器名称
func WithMQTTName(name string) MQTTSenderOption {
	return func(s *mqttSender) {
		s.name = name
	}
}

// NewMQTTSender 创建 MQTT 发送器
//
// pub: MQTT 发布器（全局 MQTT 客户端或自定义实现）
func NewMQTTSender(pub MQTTPublisher, opts ...MQTTSenderOption) Sender {
	s := &mqttSender{
		name:   "mqtt",
		pub:    pub,
		logger: zap.NewNop(),
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

func (s *mqttSender) Name() string {
	return s.name
}

func (s *mqttSender) IsConnected() bool {
	if s.pub == nil {
		return false
	}
	return s.pub.IsConnected()
}

// Send 发送 MQTT 消息
// ctx: 用于超时控制
// topic: 发布主题
// qos: 服务质量
// data: 消息负载
func (s *mqttSender) Send(ctx context.Context, topic string, qos byte, data []byte) error {
	if s.pub == nil {
		return fmt.Errorf("mqtt publisher is nil")
	}

	if !s.pub.IsConnected() {
		return ErrNotConnected
	}

	done := make(chan error, 1)
	go func() {
		done <- s.pub.Publish(topic, qos, data)
	}()

	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("mqtt publish failed: %w", err)
		}
		return nil
	case <-ctx.Done():
		return fmt.Errorf("mqtt publish timeout: %w", ctx.Err())
	}
}

// 确保 *mqttSender 实现了 Sender
var _ Sender = (*mqttSender)(nil)
