package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"runtime"
	"sync"
	"time"

	"edge5/config"
	"edge5/global"
	"edge5/internal/core/protocol"
	"edge5/internal/model"
	"edge5/internal/repository"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"go.uber.org/zap"
)

var (
	networkIP   string
	networkMAC  string
	networkOnce sync.Once
)

// MQTTBusinessService 网关 MQTT 业务服务
// 负责：网关注册、设备注册、心跳、状态上报、下行指令订阅
type MQTTBusinessService struct {
	logger           *zap.Logger
	gatewaySN        string
	subscribedTopics []string // 已订阅的主题列表（用于热更新取消订阅）
	deviceRepo       *repository.DeviceRepository
	deviceStatusRepo *repository.DeviceStatusRepository
	mqttRepo         *repository.MQTTConfigRepository
	systemMonitor    *SystemMonitor

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
	systemMonitor *SystemMonitor,
	logger *zap.Logger,
) *MQTTBusinessService {
	return &MQTTBusinessService{
		logger:           logger,
		gatewaySN:        config.CONFIG.Gateway.SN,
		deviceRepo:       deviceRepo,
		deviceStatusRepo: deviceStatusRepo,
		mqttRepo:         mqttRepo,
		systemMonitor:    systemMonitor,
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

// buildTopic 委托给 MessageBuilder 构建主题
func (s *MQTTBusinessService) buildTopic(key string, deviceSn string) string {
	return GetMessageBuilder().BuildTopic(key, deviceSn)
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

	s.logger.Debug("MQTT 发布消息："+string(data), zap.String("topic", topic))
	return nil
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
func getNetworkInfo(name string) (ip, mac string) {
	if runtime.GOOS == "linux" {
		iface, err := net.InterfaceByName(name)
		if err != nil {
			return "", ""
		}
		mac = iface.HardwareAddr.String()
		addrs, err := iface.Addrs()
		if err != nil {
			return ip, mac
		}
		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() && ipNet.IP.To4() != nil {
				ip = ipNet.IP.String()
				return ip, mac
			}
		}
		return ip, mac
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return "", ""
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() && ipNet.IP.To4() != nil {
				return ipNet.IP.String(), iface.HardwareAddr.String()
			}
		}
	}
	return "", ""
}

func getCachedNetworkInfo() (ip, mac string) {
	networkOnce.Do(func() {
		networkIP, networkMAC = getNetworkInfo("eth0")
	})
	return networkIP, networkMAC
}

func get2Ips() (ip, mac string) {
	ip1, mac1 := getNetworkInfo("eth0")
	ip2, mac2 := getNetworkInfo("eth1")
	return ip1 + ";" + ip2, mac1 + ";" + mac2
}

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
	topic := s.buildTopic("register_up", "")
	s.logger.Info("启动网关注册发送", zap.String("topic", topic))

	ip, mac := getCachedNetworkInfo()
	if ip == "" || mac == "" {
		s.logger.Warn("获取网络信息失败，IP/MAC 将为空")
	}

	payload := model.GatewayRegisterPayload{
		SN:              s.gatewaySN,
		Model:           "edge5-gateway",
		FirmwareVersion: "1.0.0",
		IP:              ip,
		MAC:             mac,
	}

	msg := GetMessageBuilder().BuildGatewayMessage(payload)

	// 立即发送第一帧
	s.publishGatewayRegister(topic, msg)

	// 指数退避：长时间收不到响应时逐步加长发送间隔，避免对 Broker 造成压力
	const (
		initialInterval = 10 * time.Second
		maxInterval     = 5 * time.Minute
	)
	interval := initialInterval
	timer := time.NewTimer(interval)
	defer timer.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return false
		case <-timer.C:
			if global.GetGatewayRegistered() {
				return true
			}
			msg.Timestamp = time.Now().UnixMilli()
			msg.RequestID = model.GenerateRequestID()
			s.publishGatewayRegister(topic, msg)
			// 未收到响应，下次发送间隔翻倍（封顶 maxInterval）
			interval *= 2
			if interval > maxInterval {
				interval = maxInterval
			}
			s.logger.Info("注册请求未收到响应，加长发送间隔", zap.Duration("nextInterval", interval))
			timer.Reset(interval)
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

	// MQTT重连后上报缓存数据
	go func() {
		time.Sleep(2 * time.Second) // 等待MQTT连接稳定
		scheduler := GetTaskScheduler()
		if scheduler != nil {
			scheduler.FlushAllCache()
		}
	}()
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
	topic := s.buildTopic("device_register_up", "")
	s.logger.Info("注册设备", zap.String("device_sn", device.DeviceSn), zap.String("topic", topic))

	payload := model.DeviceRegisterPayload{
		DeviceSN:   device.DeviceSn,
		Model:      device.DeviceName,
		Brand:      device.Brand,
		DeviceType: device.DeviceType,
		Protocol:   device.Protocol,
	}

	msg := GetMessageBuilder().BuildGatewayMessage(payload)
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
			topic := s.buildTopic("heartbeat_up", "")
			payload := model.HeartbeatPayload{
				Timestamp: time.Now().UnixMilli(),
			}
			msg := GetMessageBuilder().BuildGatewayMessage(payload)
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
			topic := s.buildTopic("gateway_status_up", "")
			payload := s.collectGatewayProperties()
			msg := GetMessageBuilder().BuildGatewayMessage(payload)
			s.publish(topic, msg)
		}
	}
}

func (s *MQTTBusinessService) collectGatewayProperties() *model.GatewayPropertiesPayload {
	payload := &model.GatewayPropertiesPayload{
		CPUUsage:    0,
		MemoryUsage: 0,
		DiskUsage:   0,
		Uptime:      GetUptimeSeconds(),
		Temperature: 0,
	}

	if s.systemMonitor != nil {
		resources := s.systemMonitor.GetResources()
		payload.CPUUsage = resources.CPUUsedPercent
		payload.MemTotal = resources.MemTotal
		payload.MemUsed = resources.MemUsed
		payload.MemoryUsage = resources.MemUsedPercent
		payload.DiskTotal = resources.DiskTotal
		payload.DiskUsed = resources.DiskUsed
		payload.DiskUsage = resources.DiskUsedPercent
	}

	ip, _ := getCachedNetworkInfo()
	payload.IP = ip

	return payload
}

// ─── 订阅下行主题 ───

func (s *MQTTBusinessService) subscribeDownlinkTopics() {
	qos := byte(config.CONFIG.MQTT.QoS)

	// 清空已订阅列表（热更新时重新记录）
	s.subscribedTopics = []string{}

	// 1. 网关注册响应
	topic1 := s.buildTopic("register_down_ack", "")
	s.subscribe(topic1, qos, s.handleGatewayRegisterAckMessage)

	// 2. 网关指令下发（含 init 注销指令）
	topic2 := s.buildTopic("gateway_cmd_down", "")
	s.subscribe(topic2, qos, s.handleGatewayCommand)

	// 3. 设备注册响应
	topic3 := s.buildTopic("device_register_down_ack", "")
	s.subscribe(topic3, qos, s.handleDeviceRegisterAckMessage)

	// 4. 设备指令下发（通配订阅所有设备）
	// 注意：模板 path 为 "{gatewaySn}/{deviceSn}/command"，订阅时需要用 MQTT 通配符
	topic4 := s.buildTopic("device_cmd_down", "+") // deviceSn 用 + 通配
	s.subscribe(topic4, qos, s.handleDeviceCommand)

	s.logger.Info("下行主题订阅完成",
		zap.String("register_ack", topic1),
		zap.String("gateway_cmd", topic2),
		zap.String("device_register_ack", topic3),
		zap.String("device_cmd", topic4),
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
	GetMessageBuilder().ReloadTopicConfig()

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
	s.logger.Info("收到网关指令:"+regexp.MustCompile(`\s+`).ReplaceAllString(string(msg.Payload()), ""), zap.String("topic", msg.Topic()))

	var wrapper model.MQTTGatewayMessage
	if err := json.Unmarshal(msg.Payload(), &wrapper); err != nil {
		s.logger.Warn("解析网关指令消息体失败", zap.Error(err))
		return
	}

	s.logger.Info("网关指令", zap.String("command", wrapper.Command), zap.String("deviceSn", wrapper.DeviceSn))

	if wrapper.Command == "init" {
		s.replyGatewayCommand(wrapper.RequestID, wrapper.Command, 0, "网关已注销")
		s.handleGatewayInit()
		return
	}

	if wrapper.Command == "write" {
		var writeParams model.DeviceWriteParams
		if len(wrapper.Payload) > 0 {
			if err := json.Unmarshal(wrapper.Payload, &writeParams); err != nil {
				s.logger.Warn("解析设备写指令参数失败", zap.Error(err))
				s.replyGatewayCommand(wrapper.RequestID, "write", 1, "参数解析失败")
				return
			}
		}
		s.handleDeviceWriteCommand(wrapper.RequestID, wrapper.DeviceSn, writeParams)
		return
	}

	s.replyGatewayCommand(wrapper.RequestID, wrapper.Command, 0, "ok")
}

func (s *MQTTBusinessService) replyGatewayCommand(requestID, command string, result int, message string) {
	topic := s.buildTopic("cmd_reply_up", "")
	resp := model.CommandResponse{
		GatewaySn: s.gatewaySN,
		Timestamp: time.Now().UnixMilli(),
		RequestID: requestID,
		Command:   command,
		Payload: model.ResponsePayload{
			Code:    result,
			Message: message,
		},
	}
	s.publish(topic, resp)
}

func (s *MQTTBusinessService) handleDeviceCommand(_ mqtt.Client, msg mqtt.Message) {
	s.logger.Info("收到设备指令", zap.String("topic", msg.Topic()), zap.String("payload", string(msg.Payload())))

	var wrapper model.MQTTGatewayMessage
	if err := json.Unmarshal(msg.Payload(), &wrapper); err != nil {
		s.logger.Warn("解析设备指令消息体失败", zap.Error(err))
		return
	}

	s.logger.Info("设备指令", zap.String("command", wrapper.Command), zap.String("deviceSn", wrapper.DeviceSn))

	switch wrapper.Command {
	case "write":
		var writeParams model.DeviceWriteParams
		if err := json.Unmarshal(wrapper.Payload, &writeParams); err != nil {
			s.logger.Warn("解析设备写指令参数失败", zap.Error(err))
			s.replyDeviceCommand(wrapper.RequestID, wrapper.DeviceSn, "write", 1, "参数解析失败")
			return
		}
		s.handleDeviceWriteCommand(wrapper.RequestID, wrapper.DeviceSn, writeParams)
		return
	case "read":
		var readParams model.DeviceWriteParams
		if err := json.Unmarshal(wrapper.Payload, &readParams); err != nil {
			s.logger.Warn("解析设备读指令参数失败", zap.Error(err))
			s.replyDeviceReadCommand(wrapper.RequestID, wrapper.DeviceSn, &model.DeviceReadReply{
				Code:    1,
				Message: "参数解析失败",
				Details: nil,
			})
			return
		}
		s.handleDeviceReadCommand(wrapper.RequestID, wrapper.DeviceSn, readParams)
		return
	}

	s.replyDeviceCommand(wrapper.RequestID, wrapper.DeviceSn, wrapper.Command, 1, "未知指令")
}

func (s *MQTTBusinessService) handleDeviceWriteCommand(requestID, fallbackDeviceSn string, writeParams model.DeviceWriteParams) {

	deviceSn := fallbackDeviceSn
	if deviceSn == "" {
		s.replyDeviceCommand(requestID, "", "write", 1, "设备序列号不能为空")
		return
	}

	if len(writeParams.Points) == 0 {
		s.replyDeviceCommand(requestID, deviceSn, "write", 1, "请至少添加一个写入项")
		return
	}

	reply, err := s.executeDeviceWrite(deviceSn, writeParams)
	if err != nil {
		s.logger.Error("设备写操作失败", zap.String("deviceSn", deviceSn), zap.Error(err))
		s.replyDeviceCommand(requestID, deviceSn, "write", 1, "写入失败: "+err.Error())
		return
	}

	s.replyDeviceWriteCommand(requestID, deviceSn, reply)
}

func (s *MQTTBusinessService) executeDeviceWrite(deviceSn string, writeParams model.DeviceWriteParams) (*model.DeviceWriteReply, error) {
	device, err := s.deviceRepo.GetBySn(deviceSn)
	if err != nil {
		return nil, fmt.Errorf("设备不存在: %w", err)
	}

	// if device.Status != 1 {
	// 	return nil, fmt.Errorf("设备未启用")
	// }

	reg := protocol.DefaultRegistry()
	proto, ok := reg.Get(device.Protocol)
	if !ok {
		return nil, fmt.Errorf("协议 %s 未注册", device.Protocol)
	}

	connParams, err := parseDeviceConfigToMetadata(device)
	if err != nil {
		return nil, fmt.Errorf("解析设备配置失败: %w", err)
	}
	connParams["deviceID"] = float64(device.ID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 已连接（如采集任务持有持久长连接）时直接写，复用连接且操作后不断开；
	// 未连接时临时建立连接，操作完断开。避免对 1E 等持久连接协议触发
	// "关旧建新"重拨与采集互踢 PLC 连接（1 连接限制下新连接会被立即关闭报 EOF）。
	handle := protocol.DeviceHandle(device.ID)
	if !proto.IsConnected(handle) {
		var connErr error
		handle, connErr = proto.Connect(ctx, connParams)
		if connErr != nil {
			errMsg := connErr.Error()
			details := make([]model.DeviceWriteDetail, 0, len(writeParams.Points))
			for _, point := range writeParams.Points {
				details = append(details, model.DeviceWriteDetail{
					Resource: point.Resource,
					Value:    nil,
					Error:    errMsg,
				})
			}
			return &model.DeviceWriteReply{
				Code:    3,
				Message: errMsg,
				Details: details,
			}, nil
		}
		defer func() {
			_ = proto.Disconnect(context.Background(), handle)
		}()
	}

	items := make([]protocol.BatchWriteItem, 0, len(writeParams.Points))
	points := make([]protocol.Point, 0, len(writeParams.Points))
	for _, point := range writeParams.Points {
		count := point.Count
		if count <= 0 {
			count = 1
		}
		pointName := point.Name
		if pointName == "" {
			pointName = point.Resource
		}
		items = append(items, protocol.BatchWriteItem{
			Point: protocol.Point{
				Name:     pointName,
				Resource: point.Resource,
				DataType: point.DataType,
				Count:    count,
			},
			Value:    point.Value,
			Priority: point.Priority,
		})
		points = append(points, protocol.Point{
			Name:     pointName,
			Resource: point.Resource,
			DataType: point.DataType,
			Count:    count,
		})
	}

	writeReq := protocol.BatchWriteRequest{
		Items:   items,
		Options: writeParams.Options,
	}

	if err := proto.WriteBatch(ctx, handle, writeReq); err != nil {
		errMsg := err.Error()
		details := make([]model.DeviceWriteDetail, 0, len(writeParams.Points))
		for _, point := range writeParams.Points {
			details = append(details, model.DeviceWriteDetail{
				Resource: point.Resource,
				Value:    nil,
				Error:    errMsg,
			})
		}
		return &model.DeviceWriteReply{
			Code:    2,
			Message: errMsg,
			Details: details,
		}, nil
	}

	readReq := protocol.BatchReadRequest{
		Points:  points,
		Options: writeParams.Options,
	}

	// WriteBatch 刚断开 TCP 连接，PLC 释放连接表项存在延迟（几十到几百毫秒），
	// 立即拨号读回容易被拒或读到 EOF（此时写入本身已成功生效）。
	// 写完后先等 100ms 待 PLC 释放连接再读回；若仍失败，再等 150ms 重试一次。
	select {
	case <-ctx.Done():
	case <-time.After(100 * time.Millisecond):
	}

	readResp, err := proto.ReadBatch(ctx, handle, readReq)
	if err != nil {
		select {
		case <-ctx.Done():
		case <-time.After(150 * time.Millisecond):
			readResp, err = proto.ReadBatch(ctx, handle, readReq)
		}
	}
	if err != nil {
		errMsg := err.Error()
		details := make([]model.DeviceWriteDetail, 0, len(writeParams.Points))
		for _, point := range writeParams.Points {
			details = append(details, model.DeviceWriteDetail{
				Resource: point.Resource,
				Value:    nil,
				Error:    errMsg,
			})
		}
		return &model.DeviceWriteReply{
			Code:    2,
			Message: errMsg,
			Details: details,
		}, nil
	}

	details := make([]model.DeviceWriteDetail, 0, len(readResp.Results))
	successCount := 0
	for _, result := range readResp.Results {
		var errMsg string
		if result.Error != nil {
			errMsg = result.Error.Error()
		} else {
			successCount++
		}
		details = append(details, model.DeviceWriteDetail{
			Resource: result.PointName,
			Value:    result.Value,
			Error:    errMsg,
		})
	}

	var code int
	var message string
	if successCount == len(details) {
		code = 0
		message = "ok"
	} else if successCount > 0 {
		code = 1
		message = "部分成功"
	} else {
		code = 2
		message = "全部失败(参数原因)"
	}

	return &model.DeviceWriteReply{
		Code:    code,
		Message: message,
		Details: details,
	}, nil
}

func (s *MQTTBusinessService) handleDeviceReadCommand(requestID, fallbackDeviceSn string, readParams model.DeviceWriteParams) {
	deviceSn := fallbackDeviceSn
	if deviceSn == "" {
		s.replyDeviceReadCommand(requestID, "", &model.DeviceReadReply{
			Code:    1,
			Message: "设备序列号不能为空",
			Details: nil,
		})
		return
	}

	if len(readParams.Points) == 0 {
		s.replyDeviceReadCommand(requestID, deviceSn, &model.DeviceReadReply{
			Code:    1,
			Message: "请至少添加一个读取项",
			Details: nil,
		})
		return
	}

	reply, err := s.executeDeviceRead(deviceSn, readParams)
	if err != nil {
		s.logger.Error("设备读操作失败", zap.String("deviceSn", deviceSn), zap.Error(err))
		s.replyDeviceReadCommand(requestID, deviceSn, &model.DeviceReadReply{
			Code:    1,
			Message: err.Error(),
			Details: nil,
		})
		return
	}

	s.replyDeviceReadCommand(requestID, deviceSn, reply)
}

func (s *MQTTBusinessService) executeDeviceRead(deviceSn string, readParams model.DeviceWriteParams) (*model.DeviceReadReply, error) {
	device, err := s.deviceRepo.GetBySn(deviceSn)
	if err != nil {
		return nil, fmt.Errorf("设备不存在: %w", err)
	}

	// if device.Status != 1 {
	// 	return nil, fmt.Errorf("设备未启用")
	// }

	reg := protocol.DefaultRegistry()
	proto, ok := reg.Get(device.Protocol)
	if !ok {
		return nil, fmt.Errorf("协议 %s 未注册", device.Protocol)
	}

	connParams, err := parseDeviceConfigToMetadata(device)
	if err != nil {
		return nil, fmt.Errorf("解析设备配置失败: %w", err)
	}
	connParams["deviceID"] = float64(device.ID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 已连接（如采集任务持有持久长连接）时直接读，复用连接且操作后不断开；
	// 未连接时临时建立连接，操作完断开。避免对 1E 等持久连接协议触发
	// "关旧建新"重拨与采集互踢 PLC 连接。
	handle := protocol.DeviceHandle(device.ID)
	if !proto.IsConnected(handle) {
		var connErr error
		handle, connErr = proto.Connect(ctx, connParams)
		if connErr != nil {
			errMsg := connErr.Error()
			details := make([]model.DeviceReadDetail, 0, len(readParams.Points))
			for _, point := range readParams.Points {
				details = append(details, model.DeviceReadDetail{
					Resource: point.Resource,
					DataType: point.DataType,
					Value:    nil,
					Error:    errMsg,
				})
			}
			return &model.DeviceReadReply{
				Code:    3,
				Message: errMsg,
				Details: details,
			}, nil
		}
		defer func() {
			_ = proto.Disconnect(context.Background(), handle)
		}()
	}

	points := make([]protocol.Point, 0, len(readParams.Points))
	for _, point := range readParams.Points {
		count := point.Count
		if count <= 0 {
			count = 1
		}
		pointName := point.Name
		if pointName == "" {
			pointName = point.Resource
		}
		points = append(points, protocol.Point{
			Name:     pointName,
			Resource: point.Resource,
			DataType: point.DataType,
			Count:    count,
		})
	}

	readReq := protocol.BatchReadRequest{
		Points:  points,
		Options: readParams.Options,
	}

	readResp, err := proto.ReadBatch(ctx, handle, readReq)
	if err != nil {
		errMsg := err.Error()
		details := make([]model.DeviceReadDetail, 0, len(readParams.Points))
		for _, point := range readParams.Points {
			details = append(details, model.DeviceReadDetail{
				Resource: point.Resource,
				DataType: point.DataType,
				Value:    nil,
				Error:    errMsg,
			})
		}
		return &model.DeviceReadReply{
			Code:    2,
			Message: errMsg,
			Details: details,
		}, nil
	}

	details := make([]model.DeviceReadDetail, 0, len(readResp.Results))
	successCount := 0
	for i, result := range readResp.Results {
		var errMsg string
		if result.Error != nil {
			errMsg = result.Error.Error()
		} else {
			successCount++
		}
		details = append(details, model.DeviceReadDetail{
			Resource: readParams.Points[i].Resource,
			DataType: readParams.Points[i].DataType,
			Value:    result.Value,
			Error:    errMsg,
		})
	}

	var code int
	var message string
	if successCount == len(details) {
		code = 0
		message = "ok"
	} else if successCount > 0 {
		code = 1
		message = "部分成功"
	} else {
		code = 2
		message = "全部失败(参数原因)"
	}

	return &model.DeviceReadReply{
		Code:    code,
		Message: message,
		Details: details,
	}, nil
}

func (s *MQTTBusinessService) replyDeviceCommand(requestID, deviceSn, command string, result int, message string) {
	topic := s.buildTopic("device_cmd_reply_up", deviceSn)
	resp := model.CommandResponse{
		GatewaySn: s.gatewaySN,
		Timestamp: time.Now().UnixMilli(),
		RequestID: requestID,
		Command:   command,
		Payload: model.ResponsePayload{
			Code:    result,
			Message: message,
		},
	}
	s.publish(topic, resp)
}

func (s *MQTTBusinessService) replyDeviceWriteCommand(requestID, deviceSn string, reply *model.DeviceWriteReply) {
	topic := s.buildTopic("device_cmd_reply_up", deviceSn)
	details := make([]model.ResponseDetail, len(reply.Details))
	for i, d := range reply.Details {
		details[i] = model.ResponseDetail{
			Resource: d.Resource,
			Value:    d.Value,
			Error:    d.Error,
		}
	}
	resp := model.CommandResponse{
		GatewaySn: s.gatewaySN,
		Timestamp: time.Now().UnixMilli(),
		RequestID: requestID,
		Command:   "write",
		Payload: model.ResponsePayload{
			Code:    reply.Code,
			Message: reply.Message,
			Details: details,
		},
	}
	s.logger.Debug("响应写入命令", zap.String("topic", topic), zap.Any("resp", resp))
	s.publish(topic, resp)
}

func (s *MQTTBusinessService) replyDeviceReadCommand(requestID, deviceSn string, reply *model.DeviceReadReply) {
	topic := s.buildTopic("device_cmd_reply_up", deviceSn)
	details := make([]model.ResponseDetail, len(reply.Details))
	for i, d := range reply.Details {
		details[i] = model.ResponseDetail{
			Resource: d.Resource,
			Value:    d.Value,
			Error:    d.Error,
		}
	}
	resp := model.CommandResponse{
		GatewaySn: s.gatewaySN,
		Timestamp: time.Now().UnixMilli(),
		RequestID: requestID,
		Command:   "read",
		Payload: model.ResponsePayload{
			Code:    reply.Code,
			Message: reply.Message,
			Details: details,
		},
	}
	s.logger.Debug("响应读取命令", zap.String("topic", topic), zap.Any("resp", resp))
	s.publish(topic, resp)
}

// ─── 设备数据上报（供外部调用） ───

// PublishDeviceData 发布设备数据到平台
func (s *MQTTBusinessService) PublishDeviceData(deviceSn string, data interface{}) error {
	// 使用模板构建主题
	topic := s.buildTopic("device_data_up", deviceSn)

	msg := GetMessageBuilder().BuildGatewayMessage(data)
	return s.publish(topic, msg)
}
