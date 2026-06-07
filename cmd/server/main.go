package main

import (
	"context"
	"edge5/config"
	"edge5/global"
	"edge5/internal/model"
	"edge5/internal/pkg/cache"
	"edge5/internal/pkg/connector"
	"edge5/internal/pkg/protocol"
	_ "edge5/internal/pkg/protocol/builtin"
	"edge5/internal/repository"
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

	if err := initPlugin(); err != nil {
		global.Logger.Warn("协议系统初始化失败", zap.Error(err))
	}

	if err := initMQTT(); err != nil {
		global.Logger.Warn("MQTT初始化失败，将稍后重试", zap.Error(err))
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
	protocol.Shutdown(global.Logger)
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
		&model.ProtocolRegistry{},
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

func initPlugin() error {
	return protocol.Init(global.DB, global.Logger)
}

func initMQTT() error {
	if err := normalizeMQTTConfigTable(); err != nil {
		global.Logger.Warn("MQTT 配置表规范化失败，将继续按现有数据尝试连接", zap.Error(err))
	}

	mqttRepo := repository.NewMQTTConfigRepository(global.DB)
	cfg, err := mqttRepo.Get()
	if err != nil {
		return err
	}

	if cfg == nil {
		global.MQTTClient = global.NewMqttClient()
		if err := global.MQTTClient.Connect(); err != nil {
			global.Logger.Warn("MQTT 初始连接失败，将依赖自动重连", zap.Error(err))
			return nil
		}

		if waitMQTTConnected(6*time.Second, 500*time.Millisecond) {
			_ = mqttRepo.Create(&model.MQTTConfig{
				Broker:    config.CONFIG.MQTT.Broker,
				Port:      config.CONFIG.MQTT.Port,
				Username:  config.CONFIG.MQTT.Username,
				Password:  config.CONFIG.MQTT.Password,
				ClientID:  config.CONFIG.MQTT.ClientID,
				KeepAlive: config.CONFIG.MQTT.KeepAlive,
				QoS:       int8(config.CONFIG.MQTT.QoS),
				On:        true,
				GatewaySN: config.CONFIG.Gateway.SN,
				CreatedAt: time.Time{},
				UpdatedAt: time.Time{},
			})
		}
		return nil
	}

	syncMQTTToGlobal(cfg)

	if !cfg.On {
		global.Logger.Info("MQTT on=false，不自动连接")
		return nil
	}

	global.MQTTClient = global.NewMqttClient()
	if err := global.MQTTClient.Connect(); err != nil {
		global.Logger.Warn("MQTT 初始连接失败，将依赖自动重连", zap.Error(err))
	}

	if waitMQTTConnected(6*time.Second, 500*time.Millisecond) {
		cfg.On = true
		_ = mqttRepo.Update(cfg)
	}

	return nil
}

func syncMQTTToGlobal(cfg *model.MQTTConfig) {
	config.CONFIG.MQTT.Broker = cfg.Broker
	config.CONFIG.MQTT.Port = cfg.Port
	config.CONFIG.MQTT.Username = cfg.Username
	config.CONFIG.MQTT.Password = cfg.Password
	config.CONFIG.MQTT.ClientID = cfg.ClientID
	config.CONFIG.MQTT.KeepAlive = cfg.KeepAlive
	config.CONFIG.MQTT.QoS = byte(cfg.QoS)
}

func waitMQTTConnected(timeout time.Duration, pollInterval time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if global.MQTTClient != nil && global.MQTTClient.IsConnected() {
			return true
		}
		time.Sleep(pollInterval)
	}
	return global.MQTTClient != nil && global.MQTTClient.IsConnected()
}

func normalizeMQTTConfigTable() error {
	if global.DB.Migrator().HasColumn(&model.MQTTConfig{}, "status") {
		_ = global.DB.Exec("UPDATE mqtt_config SET on = CASE WHEN status = 1 THEN 1 ELSE 0 END WHERE status IS NOT NULL").Error
	}

	mqttRepo := repository.NewMQTTConfigRepository(global.DB)
	cfg, err := mqttRepo.Get()
	if err != nil {
		return err
	}
	if cfg == nil {
		return nil
	}
	return mqttRepo.Update(cfg)
}

func waitForSignal() {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	sig := <-quit

	global.Logger.Info("接收到退出信号",
		zap.String("signal", sig.String()),
		zap.String("code", fmt.Sprintf("%d", sig)))
}
