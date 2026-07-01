// Package builtin 提供内置协议注册框架。
//
// 内置协议实现只需在 init() 中调用 builtin.Register()，
// 即可自动注册到全局协议注册表。
package builtin

import (
	"edge5/internal/core/protocol"

	"go.uber.org/zap"
)

var Logger *zap.Logger

// SetLogger 设置日志记录器
func SetLogger(l *zap.Logger) {
	Logger = l
}

// Register 注册一个内置协议到全局注册表
func Register(p protocol.DeviceCommProtocol) error {
	reg := protocol.DefaultRegistry()
	if Logger != nil {
		Logger.Info("注册内置协议",
			zap.String("name", protocol.GetInfoString(p.Info(), "name")),
		)
	}
	return reg.Register(p)
}
