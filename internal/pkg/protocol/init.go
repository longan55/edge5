package protocol

import (
	"fmt"

	"edge5/config"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Init 初始化协议系统
//
// 注意：内置协议需要在外部调用时被 import（触发 init 注册），
// 例如在 main.go 中：import _ "edge5/internal/pkg/protocol/builtin"
func Init(db *gorm.DB, logger *zap.Logger) error {
	if logger == nil {
		logger = zap.NewNop()
	}

	reg, ok := DefaultRegistry().(*registry)
	if !ok {
		return fmt.Errorf("DefaultRegistry is not a *registry")
	}
	reg.SetDB(db)
	reg.SetLogger(logger)

	// 扫描加载 gRPC 插件
	if config.CONFIG.Plugin.Enabled {
		if err := reg.LoadPluginsFromDir(config.CONFIG.Plugin.PluginsDir); err != nil {
			logger.Warn("加载 gRPC 插件失败", zap.Error(err))
		}
	}

	// 同步到数据库
	if err := reg.SyncToDB(); err != nil {
		return err
	}

	// 启动插件进程
	if err := reg.StartPlugins(); err != nil {
		return err
	}

	return nil
}

// Shutdown 关闭协议系统
func Shutdown(logger *zap.Logger) {
	if logger == nil {
		logger = zap.NewNop()
	}

	reg, ok := DefaultRegistry().(*registry)
	if !ok {
		return
	}
	if err := reg.StopPlugins(); err != nil {
		logger.Error("停止插件进程失败", zap.Error(err))
	}
}
