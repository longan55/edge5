// Package report 提供通用上报框架
//
// 所有设备通过统一的 Reporter 接口上报数据。
// 框架内部处理：
//   - 连接断开/上报失败时自动缓存
//   - 连接恢复后自动重试缓存数据
//   - 默认 MQTT 上报实现

//go:build !testonly

package report

import (
	"edge5/global"
	"edge5/internal/core/cache"

	"go.uber.org/zap"
)

// NewDefaultMQTTReporter 创建默认的 MQTT 上报器
//
// topic: 上报主题
// 使用全局的 MQTT 客户端和 BoltCache
func NewDefaultMQTTReporter(topic string) Reporter {
	logger := global.Logger

	var sender Sender
	if global.MQTTClient != nil {
		sender = NewMQTTSender(
			global.MQTTClient,
			WithMQTTLogger(logger),
			WithMQTTName("mqtt-default"),
		)
	} else {
		logger.Warn("全局 MQTT 客户端未初始化，MQTT 上报器将以无发送器模式创建")
	}

	var c Cache
	if global.CacheDB != nil {
		c = NewBoltCacheAdapter(global.CacheDB)
	}

	cfg := DefaultConfig(topic)

	return New(cfg, sender, c, logger)
}

// NewMQTTReporter 使用指定参数创建 MQTT 上报器
//
// pub: MQTT 发布器（实现 MQTTPublisher 接口）
// boltCache: BoltDB 缓存（用于失败缓存）
// topic: 上报主题
// logger: 日志记录器
func NewMQTTReporter(pub MQTTPublisher, boltCache *cache.BoltCache, topic string, logger *zap.Logger) Reporter {
	if logger == nil {
		logger = zap.NewNop()
	}

	var sender Sender
	if pub != nil {
		sender = NewMQTTSender(pub, WithMQTTLogger(logger), WithMQTTName("mqtt"))
	}

	var c Cache
	if boltCache != nil {
		c = NewBoltCacheAdapter(boltCache)
	}

	cfg := DefaultConfig(topic)

	return New(cfg, sender, c, logger)
}

// NewCustomReporter 完全自定义的 Reporter 创建
//
// cfg: 上报配置
// sender: 发送器实现
// c: 缓存实现（nil 时不缓存）
// logger: 日志记录器
func NewCustomReporter(cfg Config, sender Sender, c Cache, logger *zap.Logger) Reporter {
	return New(cfg, sender, c, logger)
}
