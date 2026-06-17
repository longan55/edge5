package service

import (
	"context"
	"encoding/json"
	"strings"
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
	logger           *zap.Logger
	gatewaySN        string
	topicCfg         *model.MQTTTopicConfig
	topicTemplates   map[string]*model.MQTTTopicTemplate // 主题模板缓存
	subscribedTopics []string                            // 已订阅的主题列表（用于热更新取消订阅）
	deviceRepo       *repository.DeviceRepository
	deviceStatusRepo *repository.DeviceStatusRepository
	mqttRepo         *repository.MQTTConfigRepository

	ctx    context.Context // 整个服务的生命周期
	cancel context.CancelFunc

	businessCtx    context.Context // 心跳/状态上报/设备注册的生命周期
	businessCancel context.CancelFunc
}

// NewMQTTBusinessService 创建 MQTT 业务服务
func NewMQTTBusinessService(
	deviceRepo *repository.DeviceRepository,
	deviceStatusRepo *repository.DeviceStatusRepository,
	mqttRepo *repository.MQTTConfigRepository,
	logger *zap.Logger,
) *MQTTBusinessService {
	return &MQTTBusinessService{
		logger:           logger,
		gatewaySN:        config.CONFIG.Gateway.SN,
		deviceRepo:       deviceRepo,
		deviceStatusRepo: deviceStatusRepo,
		mqttRepo:         mqttRepo,
		topicTemplates:   make(map[string]*model.MQTTTopicTemplate),
	}
}

// Start 启动 MQTT 业务协程（阻塞，需在 goroutine 中调用）
func (s *MQTTBusinessService) Start() {
	s.ctx, s.cancel = context.WithCancel(context.Background())
	defer s.cancel()

	s.logger.Info("MQTT 业务服务启动中...", zap.String("gateway_sn", s.gatewaySN))

	// 循环等待 MQTT 连接就绪（不限时）
	if !s.waitForConnectionLoop() {
		s.logger.Warn("MQTT 业务服务：上下文已取消，退出")
		return
	}

	// 加载主题配置和模板（缓存到内存）
	s.loadTopicConfigAndTemplates()

	// 订阅下行主题
	s.subscribeDownlinkTopics()

	// 进入注册状态机循环
	s.registerStateMachine()

	s.logger.Info("MQTT 业务服务已停止")
}

// Stop 停止 MQTT 业务服务
func (s *MQTTBusinessService) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
}

// ─── 连接等待 ───

// waitForConnectionLoop 循环等待 MQTT 连接，直到连接成功或上下文取消
func (s *MQTTBusinessService) waitForConnectionLoop() bool {
	for {
		select {
		case <-s.ctx.Done():
			return false
		default:
		}

		if global.MQTTClient != nil && global.MQTTClient.IsConnected() {
			s.logger.Info("MQTT 业务服务：连接就绪")
			return true
		}
		s.logger.Debug("MQTT 未连接，等待重试...")
		time.Sleep(2 * time.Second)
	}
}

// ─── 主题配置与模板加载 ───

func (s *MQTTBusinessService) loadTopicConfigAndTemplates() {
	topicRepo := repository.NewMQTTTopicRepository(global.DB)

	// 加载全局主题配置
	cfg, err := topicRepo.GetConfig(s.gatewaySN)
	if err != nil || cfg == nil {
		s.logger.Warn("加载主题配置失败，使用默认值", zap.Error(err))
		s.topicCfg = &model.MQTTTopicConfig{
			Prefix:        "/aixot",
			UpKeyword:     "up",
			DownKeyword:   "down",
			ShowDirection: true,
		}
	} else {
		s.topicCfg = cfg
	}

	// 加载所有主题模板并缓存
	templates, err := topicRepo.List()
	if err != nil {
		s.logger.Warn("加载主题模板失败，使用内置默认值", zap.Error(err))
		// 使用内置默认模板
		for _, t := range repository.GetDefaultTopics() {
			s.topicTemplates[t.Key] = t
		}
	} else {
		for _, t := range templates {
			s.topicTemplates[t.Key] = t
		}
	}

	s.logger.Info("主题配置加载完成",
		zap.String("prefix", s.topicCfg.Prefix),
		zap.String("up_keyword", s.topicCfg.UpKeyword),
		zap.String("down_keyword", s.topicCfg.DownKeyword),
		zap.Int("templates_count", len(s.topicTemplates)),
	)
}

// buildTopicFromTemplate 根据模板 key 构建完整主题路径
// 支持 path 中的变量替换：{gatewaySn} -> s.gatewaySN, {deviceSn} -> deviceSn
func (s *MQTTBusinessService) buildTopicFromTemplate(key string, deviceSn string) string {
	template, ok := s.topicTemplates[key]
	if !ok {
		s.logger.Warn("主题模板不存在，使用 key 作为路径", zap.String("key", key))
		// fallback: 使用 key 作为路径
		return s.buildTopicFallback(key, deviceSn)
	}

	// 使用全局配置的 prefix（优先级高于模板中的 prefix）
	prefix := s.topicCfg.Prefix
	if prefix == "" {
		prefix = template.Prefix
		if prefix == "" {
			prefix = "/aixot"
		}
	}

	// 使用全局配置的 direction 关键词
	direction := template.Direction
	if direction == "up" && s.topicCfg.UpKeyword != "" {
		direction = s.topicCfg.UpKeyword
	} else if direction == "down" && s.topicCfg.DownKeyword != "" {
		direction = s.topicCfg.DownKeyword
	}

	// 替换 path 中的变量
	path := template.Path
	path = strings.ReplaceAll(path, "{gatewaySn}", s.gatewaySN)
	if deviceSn != "" {
		path = strings.ReplaceAll(path, "{deviceSn}", deviceSn)
	}

	// 构建完整主题：prefix/direction/path
	// 注意：prefix 通常以 / 开头，如 "/aixot"
	topic := prefix + "/" + direction + "/" + path
	return topic
}

// buildTopicFallback 当模板不存在时的 fallback 构建
func (s *MQTTBusinessService) buildTopicFallback(key string, deviceSn string) string {
	// 根据 key 推断方向和路径
	var direction, path string
	if strings.HasSuffix(key, "_up") {
		direction = s.topicCfg.UpKeyword
		if direction == "" {
			direction = "up"
		}
		path = strings.TrimSuffix(key, "_up")
	} else if strings.HasSuffix(key, "_down") || strings.HasSuffix(key, "_down_ack") {
		direction = s.topicCfg.DownKeyword
		if direction == "" {
			direction = "down"
		}
		path = strings.TrimSuffix(key, "_down_ack")
		path = strings.TrimSuffix(path, "_down")
	} else {
		direction = s.topicCfg.UpKeyword
		path = key
	}

	// 替换变量
	path = strings.ReplaceAll(path, "{gatewaySn}", s.gatewaySN)
	if deviceSn != "" {
		path = strings.ReplaceAll(path, "{deviceSn}", deviceSn)
	}

	prefix := s.topicCfg.Prefix
	if prefix == "" {
		prefix = "/aixot"
	}

	return prefix + "/" + direction + "/" + path
}

// ─── 发布工具 ───

func (s *MQTTBusinessService) publish(topic string, payload interface{}) error {
	if global.MQTTClient == nil || !global.MQTTClient.IsConnected() {
		return nil
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

// ─── 注册状态机 ───

// registerStateMachine 管理注册/注销状态转换
func (s *MQTTBusinessService) registerStateMachine() {
	// 启动时从数据库再次校验注册状态，确保全局变量与数据库一致
	if s.mqttRepo != nil {
		dbCfg, err := s.mqttRepo.Get()
		if err == nil && dbCfg != nil {
			global.SetGatewayRegistered(dbCfg.Registered)
			s.logger.Info("从数据库同步注册状态", zap.Bool("registered", dbCfg.Registered))
		}
	}

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		registered := global.GetGatewayRegistered()
		if registered {
			// 已注册：直接启动业务协程，不发送注册消息
			s.logger.Info("网关已注册，直接启动业务协程")
			s.startBusinessGoroutines()
			<-s.businessCtx.Done()
			select {
			case <-s.ctx.Done():
				return
			default:
				s.logger.Info("网关被注销，停止业务协程，重新开始注册")
			}
		} else {
			// 未注册：发送注册消息直到收到 ack
			s.logger.Info("网关未注册，开始注册流程")
			if s.doRegister() {
				// 注册成功后直接启动业务协程，不再循环检查注册状态
				continue
			}
			return
		}
	}
}

// doRegister 发送网关注册消息，阻塞直到收到 ack 或上下文取消
func (s *MQTTBusinessService) doRegister() bool {
	// 发送前再次检查注册状态，避免数据库状态已变更但全局变量尚未同步
	if s.mqttRepo != nil {
		dbCfg, err := s.mqttRepo.Get()
		if err == nil && dbCfg != nil && dbCfg.Registered {
			s.logger.Info("检测到数据库注册状态为已注册，跳过注册流程")
			global.SetGatewayRegistered(true)
			return true
		}
	}
	// 使用模板构建主题
	topic := s.buildTopicFromTemplate("register_up", "")
	s.logger.Info("启动网关注册发送", zap.String("topic", topic))

	payload := model.GatewayRegisterPayload{
		SN:              s.gatewaySN,
		Model:           "edge5-gateway",
		FirmwareVersion: "1.0.0",
		IP:              "",
		MAC:             "",
	}

	msg := s.buildGatewayMessage(payload)

	// 立即发送第一帧
	s.publishGatewayRegister(topic, msg)

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return false
		case <-ticker.C:
			if global.GetGatewayRegistered() {
				return true
			}
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

// setRegistered 原子设置注册状态并持久化到数据库
func (s *MQTTBusinessService) setRegistered(v bool) {
	global.SetGatewayRegistered(v)
	if s.mqttRepo != nil {
		if err := s.mqttRepo.UpdateRegistered(v); err != nil {
			s.logger.Warn("更新数据库注册状态失败", zap.Bool("registered", v), zap.Error(err))
		} else {
			s.logger.Info("数据库注册状态已更新", zap.Bool("registered", v))
		}
	}
}

// startBusinessGoroutines 启动心跳、状态上报、设备注册协程
func (s *MQTTBusinessService) startBusinessGoroutines() {
	s.businessCtx, s.businessCancel = context.WithCancel(s.ctx)

	go s.heartbeatLoop()
	go s.propertiesLoop()
	go s.deviceRegisterLoop()
}

// stopBusinessGoroutines 停止业务协程
func (s *MQTTBusinessService) stopBusinessGoroutines() {
	if s.businessCancel != nil {
		s.businessCancel()
	}
}

// ─── 网关注册响应处理 ───

func (s *MQTTBusinessService) handleGatewayRegisterAck(payload []byte) {
	s.logger.Info("收到网关注册响应", zap.String("payload", string(payload)))

	var ack model.GatewayRegisterAckPayload
	if err := json.Unmarshal(payload, &ack); err != nil {
		s.logger.Warn("解析注册响应失败", zap.Error(err))
		return
	}

	if ack.Result == 0 {
		s.setRegistered(true)
		s.logger.Info("网关注册成功", zap.String("message", ack.Message))
	} else {
		s.setRegistered(false)
		s.stopBusinessGoroutines()
		s.logger.Warn("网关注册失败，转为未注册状态", zap.Int("result", ack.Result), zap.String("message", ack.Message))
	}
}

// handleGatewayInit 收到平台下发的 init（注销）消息
func (s *MQTTBusinessService) handleGatewayInit() {
	s.logger.Info("收到平台注销指令（init），注销网关")
	s.setRegistered(false)
	s.stopBusinessGoroutines()
}

// ─── 设备注册 ───

func (s *MQTTBusinessService) deviceRegisterLoop() {
	s.logger.Info("启动设备注册")

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
		case <-s.businessCtx.Done():
			return
		default:
			s.registerDevice(device)
		}
	}
}

func (s *MQTTBusinessService) registerDevice(device *model.Device) {
	// 使用模板构建主题
	topic := s.buildTopicFromTemplate("device_register_up", "")
	s.logger.Info("注册设备", zap.String("device_sn", device.DeviceSn), zap.String("topic", topic))

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
	s.logger.Info("启动心跳循环")

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.businessCtx.Done():
			s.logger.Info("心跳循环停止")
			return
		case <-ticker.C:
			// 每次发送时动态构建主题（支持热更新）
			topic := s.buildTopicFromTemplate("heartbeat_up", "")
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
	s.logger.Info("启动网关状态上报循环")

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.businessCtx.Done():
			s.logger.Info("状态上报循环停止")
			return
		case <-ticker.C:
			// 每次发送时动态构建主题（支持热更新）
			topic := s.buildTopicFromTemplate("gateway_status_up", "")
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

	// 清空已订阅列表（热更新时重新记录）
	s.subscribedTopics = []string{}

	// 1. 网关注册响应
	topic1 := s.buildTopicFromTemplate("register_down_ack", "")
	s.subscribe(topic1, qos, s.handleGatewayRegisterAckMessage)

	// 2. 网关指令下发（含 init 注销指令）
	topic2 := s.buildTopicFromTemplate("gateway_cmd_down", "")
	s.subscribe(topic2, qos, s.handleGatewayCommand)

	// 3. 设备注册响应
	topic3 := s.buildTopicFromTemplate("device_register_down_ack", "")
	s.subscribe(topic3, qos, s.handleDeviceRegisterAckMessage)

	// 4. 设备指令下发（通配订阅所有设备）
	// 注意：模板 path 为 "{gatewaySn}/{deviceSn}/command"，订阅时需要用 MQTT 通配符
	topic4 := s.buildTopicFromTemplate("device_cmd_down", "+") // deviceSn 用 + 通配
	s.subscribe(topic4, qos, s.handleDeviceCommand)

	// 5. 设备指令响应（通配订阅所有设备）
	topic5 := s.buildTopicFromTemplate("device_cmd_reply_up", "+") // deviceSn 用 + 通配
	s.subscribe(topic5, qos, s.handleDeviceCommandReply)

	s.logger.Info("下行主题订阅完成",
		zap.String("register_ack", topic1),
		zap.String("gateway_cmd", topic2),
		zap.String("device_register_ack", topic3),
		zap.String("device_cmd", topic4),
		zap.String("device_cmd_reply", topic5),
	)
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
	// 记录已订阅的主题（用于热更新时取消订阅）
	s.subscribedTopics = append(s.subscribedTopics, topic)
	s.logger.Info("订阅主题成功", zap.String("topic", topic))
}

// ─── 热更新 ───

// ReloadConfig 热更新主题配置
// 1. 取消原有订阅
// 2. 重新加载配置和模板
// 3. 重新订阅下行主题
// 上行主题（心跳/状态上报）在每次发送时动态构建，无需额外处理
func (s *MQTTBusinessService) ReloadConfig() error {
	s.logger.Info("开始热更新主题配置...")

	// 1. 取消原有订阅
	s.unsubscribeAll()

	// 2. 重新加载配置和模板
	s.loadTopicConfigAndTemplates()

	// 3. 重新订阅下行主题
	s.subscribeDownlinkTopics()

	s.logger.Info("主题配置热更新完成")
	return nil
}

// unsubscribeAll 取消所有订阅
func (s *MQTTBusinessService) unsubscribeAll() {
	if global.MQTTClient == nil || !global.MQTTClient.IsConnected() {
		s.logger.Warn("MQTT 未连接，跳过取消订阅")
		s.subscribedTopics = []string{}
		return
	}

	for _, topic := range s.subscribedTopics {
		if err := global.MQTTClient.Unsubscribe(topic); err != nil {
			s.logger.Warn("取消订阅失败", zap.String("topic", topic), zap.Error(err))
		} else {
			s.logger.Info("取消订阅成功", zap.String("topic", topic))
		}
	}
	s.subscribedTopics = []string{}
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

	// 处理 init 注销指令
	if cmdReq.Command == "init" {
		s.replyGatewayCommand(wrapper.RequestID, cmdReq.Command, 0, "网关已注销")
		s.handleGatewayInit()
		return
	}

	// TODO: 根据指令类型执行具体操作（重启、同步时间、修改上报间隔等）
	s.replyGatewayCommand(wrapper.RequestID, cmdReq.Command, 0, "ok")
}

func (s *MQTTBusinessService) replyGatewayCommand(requestID, command string, result int, message string) {
	// 使用模板构建主题
	topic := s.buildTopicFromTemplate("cmd_reply_up", "")
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

// ─── 设备数据上报（供外部调用） ───

// PublishDeviceData 发布设备数据到平台
func (s *MQTTBusinessService) PublishDeviceData(deviceSn string, data interface{}) error {
	// 使用模板构建主题
	topic := s.buildTopicFromTemplate("device_data_up", deviceSn)

	msg := s.buildGatewayMessage(data)
	return s.publish(topic, msg)
}
