package main

import (
	"context"
	"edge5/config"
	"edge5/global"
	"edge5/internal/model"
	"edge5/internal/pkg/cache"
	"edge5/internal/pkg/connector"
	"edge5/internal/router"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"
)

func main() {
	if err := run(); err != nil {
		global.Logger.Fatal("程序启动失败", zap.Error(err))
	}
}

func run() error {
	config.InitConfig("config/config.yaml")

	global.InitLogger()
	global.Logger.Info("Edge5 网关框架启动中...")

	global.MyProcess = &global.Process{}

	if err := global.InitDatabase(); err != nil {
		return fmt.Errorf("初始化数据库失败: %w", err)
	}

	if err := autoMigrate(); err != nil {
		return fmt.Errorf("数据库迁移失败: %w", err)
	}

	if err := initCache(); err != nil {
		global.Logger.Warn("初始化缓存失败，将跳过缓存（联调模式）", zap.Error(err))
		global.CacheDB = nil
	}

	initConnector()
	initPlugin()

	if config.CONFIG.MQTT.Enabled {
		if err := initMQTT(); err != nil {
			global.Logger.Warn("MQTT初始化失败，将稍后重试", zap.Error(err))
		}
	}

	r := router.SetupRouter(config.CONFIG.Server.Mode)

	srv := &http.Server{
		Addr:           fmt.Sprintf("%s:%d", config.CONFIG.Server.Host, config.CONFIG.Server.Port),
		Handler:        r,
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   30 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	go func() {
		global.Logger.Info(fmt.Sprintf("HTTP服务启动: http://%s:%d", config.CONFIG.Server.Host, config.CONFIG.Server.Port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			global.Logger.Error("HTTP服务错误", zap.Error(err))
		}
	}()

	global.RegisterQuitTask(func() error {
		global.Logger.Info("关闭HTTP服务...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(ctx)
	}, "关闭HTTP服务", 1)

	waitForSignal()

	global.Logger.Info("程序退出中...")
	global.GracefullyExit()
	global.Logger.Info("程序已退出")

	return nil
}

func autoMigrate() error {
	return global.DB.AutoMigrate(
		&model.User{},
		&model.Role{},
		&model.Menu{},
		&model.LoginLog{},
		&model.MQTTConfig{},
		&model.Device{},
		&model.DeviceStatus{},
	)
}

func initCache() error {
	var err error
	global.CacheDB, err = cache.NewBoltCache()
	if err != nil {
		return err
	}
	return nil
}

func initConnector() {
	global.ConnectorMgr = connector.NewConnectorManager()
}

func initPlugin() {
	// if config.CONFIG.Plugin.Enabled {
	// 	global.PluginMgr = plugin.NewPluginManager()
	// }
}

func initMQTT() error {
	global.MQTTClient = global.NewMqttClient()
	if err := global.MQTTClient.Connect(); err != nil {
		// 允许启动时连接失败（SDK 会自动重连）；status 仍可正常返回 connected=false
		global.Logger.Warn("MQTT 初始连接失败，将依赖自动重连", zap.Error(err))
	}
	return nil
}

func waitForSignal() {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	sig := <-quit

	global.Logger.Info("接收到退出信号",
		zap.String("signal", sig.String()),
		zap.String("code", fmt.Sprintf("%d", sig)))
}
