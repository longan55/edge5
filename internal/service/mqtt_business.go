package service

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"edge5/config"
	"edge5/global"
	"edge5/internal/model"
	"edge5/internal/repository"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"go.uber.org/zap"
)

// MQTTBusinessService 网关 MQTT 业务服务
// 负责：网关注册、设备注册、心跳、状态上报、下行指令订阅
type MQTTBusinessService struct {
	logger         *zap.Logger
	gatewaySN      string
	topicCfg       *model.MQTTTopicConfig
	deviceRepo     *repository.DeviceRepository
	deviceStatusRepo *repository.DeviceStatusRepository

	ctx    context.Context
	cancel context.CancelFunc

	mu              sync.Mutex
	registered      bool          // 网关是否已注册
	registerStopCh  chan struct{} // 停止注册轮询的信号
}

// NewMQTTBusinessService 创建 MQTT 业务服务
func NewMQTTBusinessService(
	deviceRepo *repository.DeviceRepository,
	deviceStatusRepo *repository.DeviceStatusRepository,
	logger *zap.Logger,
) *MQTTBusinessService {
	return &MQTTBusinessService{
		logger:           logger,
		gatewaySN:        config.CONFIG.Gateway.SN,
		deviceRepo:       deviceRepo,
		deviceStatusRepo: deviceStatusRepo,
	}
}

// Start 启动 MQTT 业务协程（阻塞，需在 goroutine 中调用）
func (s *MQTTBusinessService) Start() {
	s.ctx, s.cancel = context.WithCancel(context.Background())
	defer s.cancel()

	s.logger.Info("MQTT 业务服务启动中...", zap.String("gateway_sn", s.gatewaySN))

	// 等待 MQTT 连接就绪
	if !s.waitForConnection(30 * time.Second) {
		s.logger.Warn("MQTT 业务服务：等待连接超时，退出")
		return
	}

	// 加载主题配置
	s.loadTopicConfig()

	// 订阅下行主题
	s.subscribeDownlinkTopics()

	// 启动网关注册
	go s.gatewayRegisterLoop()

	// 启动心跳
	go s.heartbeatLoop()

	// 启动状态上报
	go s.propertiesLoop()

	// 阻塞直到上下文取消
	<-s.ctx.Done()
	s.logger.Info("MQTT 业务服务已停止")
}

// Stop 停止 MQTT 业务服务
func (s *MQTTBusinessService) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
}

// ─── 工具方法 ───

func (s *MQTTBusinessService) waitForConnection(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if global.MQTTClient != nil && global.MQTTClient.IsConnected() {
			s.logger.Info("MQTT 业务服务：连接就绪")
			return true
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}

func (s *MQTTBusinessService) loadTopicConfig() {
	topicRepo := repository.NewMQTTTopicRepository(global.DB)
	cfg, err := topicRepo.GetConfig(s.gatewaySN)
	if err != nil || cfg == nil {
		s.logger.Warn("加载主题配置失败，使用默认值", zap.Error(err))
		s.topicCfg = &model.MQTTTopicConfig{
			Prefix:        "/aixot",
			UpKeyword:     "up",
			DownKeyword:   "down",
			ShowDirection: true,
		}
		return
	}
	s.topicCfg = cfg
}

// buildTopic 构建主题路径
// 如: /aixot/up/{gatewaySn}/heartbeat
func (s *MQTTBusinessService) buildTopic(direction string, path string) string {
	prefix := s.topicCfg.Prefix
	if prefix == "" {
		prefix = "/aixot"
	}
	return prefix + "/" + direction + "/" + path
}

// publish 发布消息到指定主题
func (s *MQTTBusinessService) publish(topic string, payload interface{}) error {
	if global.MQTTClient == nil || !global.MQTTClient.IsConnected() {
		return nil // 静默跳过
	}

	data, err := json.Marshal(payload)
	if err != nil {
		s.logger.Error("MQTT 序列化消息失败", zap.Error(err))
		return err
	}

	qos := byte(config.CONFIG.MQTT.QoS)
	if err := global.MQTTClient.Publish(topic, qos, data); err != nil {
		s.logger.Warn("MQTT 发布消息失败", zap.String("topic", topic), zap.Error(err))
		return err
	}

	s.logger.Debug("MQTT 发布消息", zap.String("topic", topic), zap.Int("len", len(data)))
	return nil
}

// buildGatewayMessage 构建网关通用消息体
func (s *MQTTBusinessService) buildGatewayMessage(payload interface{}) *model.MQTTGatewayMessage {
	raw, _ := json.Marshal(payload)
	return &model.MQTTGatewayMessage{
		Version:   "1.0",
		GatewaySn: s.gatewaySN,
		Timestamp: time.Now().UnixMilli(),
		RequestID: model.GenerateRequestID(),
		Payload:   raw,
	}
}

// ─── 网关注册 ───

func (s *MQTTBusinessService) gatewayRegisterLoop() {
	topic := s.buildTopic(s.topicCfg.UpKeyword, "gateway/register")
	s.logger.Info("启动网关注册循环", zap.String("topic", topic))

	// 构建注册负载
	payload := model.GatewayRegisterPayload{
		SN:              s.gatewaySN,
		Model:           "edge5-gateway",
		FirmwareVersion: "1.0.0",
		IP:              "",
		MAC:             "",
	}

	msg := s.buildGatewayMessage(payload)

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	// 立即发送第一帧
	s.publishGatewayRegister(topic, msg)

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.mu.Lock()
			registered := s.registered
			s.mu.Unlock()
			if registered {
				s.logger.Info("网关已注册，停止注册轮询")
				return
			}
			// 刷新时间戳
			msg.Timestamp = time.Now().UnixMilli()
			msg.RequestID = model.GenerateRequestID()
			s.publishGatewayRegister(topic, msg)
		}
	}
}

func (s *MQTTBusinessService) publishGatewayRegister(topic string, msg *model.MQTTGatewayMessage) {
	data, _ := json.Marshal(msg)
	if global.MQTTClient == nil || !global.MQTTClient.IsConnected() {
		return
	}

	qos := byte(config.CONFIG.MQTT.QoS)
	global.MQTTClient.Publish(topic, qos, data)
	s.logger.Info("发送网关注册请求", zap.String("topic", topic), zap.String("requestId", msg.RequestID))
}

// handleGatewayRegisterAck 处理网关注册响应
func (s *MQTTBusinessService) handleGatewayRegisterAck(payload []byte) {
	s.logger.Info("收到网关注册响应", zap.String("payload", string(payload)))

	var ack model.GatewayRegisterAckPayload
	if err := json.Unmarshal(payload, &ack); err != nil {
		s.logger.Warn("解析注册响应失败", zap.Error(err))
		return
	}

	if ack.Result == 0 {
		s.mu.Lock()
		s.registered = true
		s.mu.Unlock()
		s.logger.Info("网关注册成功", zap.String("message", ack.Message))

		// 注册成功后启动设备注册
		go s.deviceRegisterLoop()
	} else {
		s.logger.Warn("网关注册失败", zap.Int("result", ack.Result), zap.String("message", ack.Message))
	}
}

// ─── 设备注册 ───

func (s *MQTTBusinessService) deviceRegisterLoop() {
	s.logger.Info("启动设备注册")

	// 从数据库获取所有已启用设备
	devices, _, err := s.deviceRepo.List(1, 1000, "", "")
	if err != nil {
		s.logger.Warn("获取设备列表失败", zap.Error(err))
		return
	}

	for _, device := range devices {
		if device.Status != 1 {
			continue
		}
		select {
		case <-s.ctx.Done():
			return
		default:
			s.registerDevice(device)
		}
	}
}

func (s *MQTTBusinessService) registerDevice(device *model.Device) {
	topic := s.buildTopic(s.topicCfg.UpKeyword, s.gatewaySN+"/device/register")
	s.logger.Info("注册设备", zap.String("device_sn", device.DeviceSn))

	payload := model.DeviceRegisterPayload{
		DeviceSN:   device.DeviceSn,
		Model:      device.DeviceName,
		Brand:      device.Brand,
		DeviceType: device.DeviceType,
		Protocol:   device.Protocol,
	}

	msg := s.buildGatewayMessage(payload)
	s.publish(topic, msg)
}

// handleDeviceRegisterAck 处理设备注册响应
func (s *MQTTBusinessService) handleDeviceRegisterAck(payload []byte) {
	s.logger.Info("收到设备注册响应", zap.String("payload", string(payload)))

	var ack model.DeviceRegisterAckPayload
	if err := json.Unmarshal(payload, &ack); err != nil {
		s.logger.Warn("解析设备注册响应失败", zap.Error(err))
		return
	}

	if ack.Result == 0 {
		s.logger.Info("设备注册成功", zap.String("device_sn", ack.DeviceSN), zap.String("message", ack.Message))
	} else {
		s.logger.Warn("设备注册失败", zap.String("device_sn", ack.DeviceSN), zap.Int("result", ack.Result), zap.String("message", ack.Message))
	}
}

// ─── 心跳 ───

func (s *MQTTBusinessService) heartbeatLoop() {
	topic := s.buildTopic(s.topicCfg.UpKeyword, s.gatewaySN+"/heartbeat")
	s.logger.Info("启动心跳循环", zap.String("topic", topic))

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			payload := model.HeartbeatPayload{
				Timestamp: time.Now().UnixMilli(),
			}
			msg := s.buildGatewayMessage(payload)
			s.publish(topic, msg)
		}
	}
}

// ─── 网关状态上报 ───

func (s *MQTTBusinessService) propertiesLoop() {
	topic := s.buildTopic(s.topicCfg.UpKeyword, s.gatewaySN+"/properties")
	s.logger.Info("启动网关状态上报循环", zap.String("topic", topic))

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			// 采集系统信息（TODO: 实现实际采集）
			payload := s.collectGatewayProperties()
			msg := s.buildGatewayMessage(payload)
			s.publish(topic, msg)
		}
	}
}

func (s *MQTTBusinessService) collectGatewayProperties() *model.GatewayPropertiesPayload {
	// TODO: 实现实际系统信息采集（CPU/内存/磁盘/温度等）
	return &model.GatewayPropertiesPayload{
		CPUUsage:    0,
		MemoryUsage: 0,
		DiskUsage:   0,
		Uptime:      time.Now().Unix(),
		Temperature: 0,
	}
}

// ─── 订阅下行主题 ───

func (s *MQTTBusinessService) subscribeDownlinkTopics() {
	qos := byte(config.CONFIG.MQTT.QoS)
	down := s.topicCfg.DownKeyword

	// 1. 网关注册响应
	s.subscribe(s.buildTopic(down, "gateway/register/ack"), qos, s.handleGatewayRegisterAckMessage)

	// 2. 网关指令下发
	s.subscribe(s.buildTopic(down, s.gatewaySN+"/command"), qos, s.handleGatewayCommand)

	// 3. 设备注册响应
	s.subscribe(s.buildTopic(down, s.gatewaySN+"/device/register/ack"), qos, s.handleDeviceRegisterAckMessage)

	// 4. 设备指令下发（通配订阅所有设备）
	s.subscribe(s.buildTopic(down, s.gatewaySN+"/+/command"), qos, s.handleDeviceCommand)

	// 5. 设备指令响应（通配订阅所有设备）
	s.subscribe(s.buildTopic(down, s.gatewaySN+"/+/command/reply"), qos, s.handleDeviceCommandReply)
}

func (s *MQTTBusinessService) subscribe(topic string, qos byte, handler mqtt.MessageHandler) {
	if global.MQTTClient == nil || !global.MQTTClient.IsConnected() {
		s.logger.Warn("MQTT 未连接，跳过订阅", zap.String("topic", topic))
		return
	}

	if err := global.MQTTClient.Subscribe(topic, qos, handler); err != nil {
		s.logger.Error("订阅主题失败", zap.String("topic", topic), zap.Error(err))
		return
	}
	s.logger.Info("订阅主题成功", zap.String("topic", topic))
}

// ─── 下行消息处理器 ───

func (s *MQTTBusinessService) handleGatewayRegisterAckMessage(_ mqtt.Client, msg mqtt.Message) {
	var wrapper model.MQTTGatewayMessage
	if err := json.Unmarshal(msg.Payload(), &wrapper); err != nil {
		s.logger.Warn("解析注册响应消息体失败", zap.Error(err))
		return
	}
	s.handleGatewayRegisterAck(wrapper.Payload)
}

func (s *MQTTBusinessService) handleDeviceRegisterAckMessage(_ mqtt.Client, msg mqtt.Message) {
	var wrapper model.MQTTGatewayMessage
	if err := json.Unmarshal(msg.Payload(), &wrapper); err != nil {
		s.logger.Warn("解析设备注册响应消息体失败", zap.Error(err))
		return
	}
	s.handleDeviceRegisterAck(wrapper.Payload)
}

func (s *MQTTBusinessService) handleGatewayCommand(_ mqtt.Client, msg mqtt.Message) {
	s.logger.Info("收到网关指令", zap.String("topic", msg.Topic()), zap.String("payload", string(msg.Payload())))

	var wrapper model.MQTTGatewayMessage
	if err := json.Unmarshal(msg.Payload(), &wrapper); err != nil {
		s.logger.Warn("解析网关指令消息体失败", zap.Error(err))
		return
	}

	var cmdReq model.CommandPayload
	if err := json.Unmarshal(wrapper.Payload, &cmdReq); err != nil {
		s.logger.Warn("解析网关指令负载失败", zap.Error(err))
		return
	}

	s.logger.Info("网关指令", zap.String("command", cmdReq.Command), zap.Any("params", cmdReq.Params))

	// TODO: 根据指令类型执行具体操作（重启、同步时间、修改上报间隔等）
	// 回复指令执行结果
	s.replyGatewayCommand(wrapper.RequestID, cmdReq.Command, 0, "ok")
}

func (s *MQTTBusinessService) replyGatewayCommand(requestID, command string, result int, message string) {
	topic := s.buildTopic(s.topicCfg.UpKeyword, s.gatewaySN+"/command/reply")
	resp := model.CommandResponse{
		GatewaySn: s.gatewaySN,
		Timestamp: time.Now().UnixMilli(),
		RequestID: requestID,
		Payload: model.ResponsePayload{
			Command: command,
			Result:  result,
			Message: message,
		},
	}
	s.publish(topic, resp)
}

func (s *MQTTBusinessService) handleDeviceCommand(_ mqtt.Client, msg mqtt.Message) {
	s.logger.Info("收到设备指令", zap.String("topic", msg.Topic()), zap.String("payload", string(msg.Payload())))

	// TODO: 解析 topic 中的 deviceSn，执行对应设备指令
}

func (s *MQTTBusinessService) handleDeviceCommandReply(_ mqtt.Client, msg mqtt.Message) {
	s.logger.Info("收到设备指令响应", zap.String("topic", msg.Topic()), zap.String("payload", string(msg.Payload())))

	// TODO: 处理设备指令响应
}