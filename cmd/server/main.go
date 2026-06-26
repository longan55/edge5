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
	"edge5/internal/service"
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

	initDefaultTopics()

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

	if global.MQTTClient != nil {
		global.MQTTClient.SetOnConnectCallback(func() {
			global.Logger.Info("MQTT重连成功，开始上报缓存数据")
			scheduler := service.GetTaskScheduler()
			if scheduler != nil {
				scheduler.FlushAllCache()
			}
		})
	}

	// 初始化消息构建器（统一的 MQTTGatewayMessage 格式，独立于上报协议）
	service.NewMessageBuilder(global.Logger)

	// 启动 MQTT 业务服务（网关注册、心跳、状态上报、订阅下行主题）
	startMQTTBusiness()

	// 异步测试设备连接并更新在线状态
	service.TestDeviceConnections()

	// 初始化路由（会初始化任务调度器）
	r := router.SetupRouter(config.CONFIG.Server.Mode)

	// 启动所有任务（路由初始化后，任务调度器已就绪）
	service.StartAllTasks()

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
	global.BeforeExit()
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
		&model.MQTTConfig{},
		&model.MQTTTopicTemplate{},
		&model.MQTTTopicConfig{},
	)
}

func initDefaultTopics() {
	var count int64
	global.DB.Model(&model.MQTTTopicTemplate{}).Count(&count)
	if count == 0 {
		defaults := repository.GetDefaultTopics()
		for _, t := range defaults {
			global.DB.Create(t)
		}
		global.Logger.Info("已初始化默认MQTT主题模板")
	}

	var configCount int64
	global.DB.Model(&model.MQTTTopicConfig{}).Where("gateway_sn = ?", config.CONFIG.Gateway.SN).Count(&configCount)
	if configCount == 0 {
		defaultConfig := &model.MQTTTopicConfig{
			Prefix:        "/aixot",
			UpKeyword:     "up",
			DownKeyword:   "down",
			ShowDirection: true,
			GatewaySN:     config.CONFIG.Gateway.SN,
		}
		global.DB.Create(defaultConfig)
		global.Logger.Info("已初始化默认MQTT主题配置")
	}
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
	global.Logger.Info("初始化MQTT：开始规范化配置表")
	if err := normalizeMQTTConfigTable(); err != nil {
		global.Logger.Warn("MQTT 配置表规范化失败，将继续按现有数据尝试连接", zap.Error(err))
	}

	mqttRepo := repository.NewMQTTConfigRepository(global.DB)
	cfg, err := mqttRepo.Get()
	if err != nil {
		global.Logger.Error("MQTT 配置读取失败", zap.Error(err))
		return err
	}

	if cfg == nil {
		global.Logger.Info("数据库中无MQTT配置，使用配置文件(yaml)默认值连接")
		global.MQTTClient = global.NewMqttClient()
		if err := global.MQTTClient.Connect(); err != nil {
			global.Logger.Warn("MQTT 初始连接失败，将依赖自动重连", zap.Error(err))
			return nil
		}

		if waitMQTTConnected(6*time.Second, 500*time.Millisecond) {
			global.Logger.Info("MQTT 初始连接成功，写入数据库")
			_ = mqttRepo.Create(&model.MQTTConfig{
				Broker:              config.CONFIG.MQTT.Broker,
				Protocol:            config.CONFIG.MQTT.Protocol,
				Host:                config.CONFIG.MQTT.Host,
				Port:                config.CONFIG.MQTT.Port,
				Username:            config.CONFIG.MQTT.Username,
				Password:            config.CONFIG.MQTT.Password,
				ClientID:            config.CONFIG.MQTT.ClientID,
				KeepAlive:           config.CONFIG.MQTT.KeepAlive,
				QoS:                 int8(config.CONFIG.MQTT.QoS),
				On:                  true,
				Registered:          false, // 首次创建时注册状态为未注册
				GatewaySN:           config.CONFIG.Gateway.SN,
				SSL:                 config.CONFIG.MQTT.SSL,
				SSLVerify:           config.CONFIG.MQTT.SSLVerify,
				ALPNTag:             config.CONFIG.MQTT.ALPNTag,
				CertType:            config.CONFIG.MQTT.CertType,
				CAFile:              config.CONFIG.MQTT.CAFile,
				CertFile:            config.CONFIG.MQTT.CertFile,
				KeyFile:             config.CONFIG.MQTT.KeyFile,
				Version:             config.CONFIG.MQTT.Version,
				ConnectTimeout:      config.CONFIG.MQTT.ConnectTimeout,
				AutoReconnect:       config.CONFIG.MQTT.AutoReconnect,
				ReconnectPeriod:     config.CONFIG.MQTT.ReconnectPeriod,
				CleanStart:          config.CONFIG.MQTT.CleanStart,
				SessionExpiry:       config.CONFIG.MQTT.SessionExpiry,
				ReceiveMax:          config.CONFIG.MQTT.ReceiveMax,
				MaxPacketSize:       config.CONFIG.MQTT.MaxPacketSize,
				TopicAliasMax:       config.CONFIG.MQTT.TopicAliasMax,
				RequestResponseInfo: config.CONFIG.MQTT.RequestResponse,
				RequestProblemInfo:  config.CONFIG.MQTT.RequestProblem,
				CreatedAt:           time.Time{},
				UpdatedAt:           time.Time{},
			})
		} else {
			global.Logger.Warn("MQTT 初始连接超时（6秒内未连接成功）")
		}
		return nil
	}

	global.Logger.Info("MQTT 配置已从数据库加载",
		zap.Bool("on", cfg.On),
		zap.Bool("registered", cfg.Registered),
		zap.String("protocol", cfg.Protocol),
		zap.String("host", cfg.Host),
		zap.Int("port", cfg.Port),
	)
	syncMQTTToGlobal(cfg)

	if !cfg.On {
		global.Logger.Info("MQTT on=false，不自动连接")
		return nil
	}

	global.Logger.Info("MQTT on=true，开始自动连接")
	global.MQTTClient = global.NewMqttClient()
	if err := global.MQTTClient.Connect(); err != nil {
		global.Logger.Warn("MQTT 初始连接失败，将依赖自动重连", zap.Error(err))
	}

	if waitMQTTConnected(6*time.Second, 500*time.Millisecond) {
		global.Logger.Info("MQTT 自动连接成功")
		cfg.On = true
		_ = mqttRepo.Update(cfg)
	} else {
		global.Logger.Warn("MQTT 自动连接超时（6秒内未连接成功）")
	}

	return nil
}

func syncMQTTToGlobal(cfg *model.MQTTConfig) {
	config.CONFIG.MQTT.Protocol = cfg.Protocol
	config.CONFIG.MQTT.Host = cfg.Host
	config.CONFIG.MQTT.Broker = cfg.Broker
	config.CONFIG.MQTT.Port = cfg.Port
	config.CONFIG.MQTT.Username = cfg.Username
	config.CONFIG.MQTT.Password = cfg.Password
	config.CONFIG.MQTT.ClientID = cfg.ClientID
	config.CONFIG.MQTT.KeepAlive = cfg.KeepAlive
	config.CONFIG.MQTT.QoS = byte(cfg.QoS)
	config.CONFIG.MQTT.SSL = cfg.SSL
	config.CONFIG.MQTT.SSLVerify = cfg.SSLVerify
	config.CONFIG.MQTT.ALPNTag = cfg.ALPNTag
	config.CONFIG.MQTT.CertType = cfg.CertType
	config.CONFIG.MQTT.CAFile = cfg.CAFile
	config.CONFIG.MQTT.CertFile = cfg.CertFile
	config.CONFIG.MQTT.KeyFile = cfg.KeyFile
	config.CONFIG.MQTT.Version = cfg.Version
	config.CONFIG.MQTT.ConnectTimeout = cfg.ConnectTimeout
	config.CONFIG.MQTT.AutoReconnect = cfg.AutoReconnect
	config.CONFIG.MQTT.ReconnectPeriod = cfg.ReconnectPeriod
	config.CONFIG.MQTT.CleanStart = cfg.CleanStart
	config.CONFIG.MQTT.SessionExpiry = cfg.SessionExpiry
	config.CONFIG.MQTT.ReceiveMax = cfg.ReceiveMax
	config.CONFIG.MQTT.MaxPacketSize = cfg.MaxPacketSize
	config.CONFIG.MQTT.TopicAliasMax = cfg.TopicAliasMax
	config.CONFIG.MQTT.RequestResponse = cfg.RequestResponseInfo
	config.CONFIG.MQTT.RequestProblem = cfg.RequestProblemInfo

	global.Logger.Info("MQTT配置已从数据库同步到全局",
		zap.String("protocol", cfg.Protocol),
		zap.String("host", cfg.Host),
		zap.Int("port", cfg.Port),
		zap.Bool("ssl", cfg.SSL),
		zap.String("version", cfg.Version),
		zap.Bool("auto_reconnect", cfg.AutoReconnect),
	)
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

func startMQTTBusiness() {
	// 从数据库加载之前的注册状态
	mqttRepo := repository.NewMQTTConfigRepository(global.DB)
	cfg, err := mqttRepo.Get()
	if err != nil {
		global.Logger.Warn("加载网关注册状态失败", zap.Error(err))
		global.SetGatewayRegistered(false)
	} else if cfg != nil {
		global.SetGatewayRegistered(cfg.Registered)
		global.Logger.Info("加载网关注册状态", zap.Bool("registered", cfg.Registered))
	} else {
		global.SetGatewayRegistered(false)
		global.Logger.Info("数据库中无MQTT配置，注册状态初始化为未注册")
	}

	deviceRepo := repository.NewDeviceRepository(global.DB)
	deviceStatusRepo := repository.NewDeviceStatusRepository(global.DB)
	systemMonitor := service.NewSystemMonitor(global.Logger)
	mqttBusiness := service.NewMQTTBusinessService(deviceRepo, deviceStatusRepo, mqttRepo, systemMonitor, global.Logger)

	// 保存到全局变量（用于热更新）
	global.MQTTBusinessService = mqttBusiness

	go mqttBusiness.Start()

	global.RegisterQuitTask(func() error {
		global.Logger.Info("关闭MQTT业务服务...")
		mqttBusiness.Stop()
		return nil
	}, "关闭MQTT业务服务", 2)
}
