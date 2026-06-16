package handler

import (
	"edge5/config"
	"edge5/global"
	"edge5/internal/model"
	"edge5/internal/repository"
	"edge5/internal/service"
	"edge5/internal/utils/response"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type MQTTHandler struct {
	mqttRepo  *repository.MQTTConfigRepository
	topicRepo *repository.MQTTTopicRepository
}

func NewMQTTHandler(mqttRepo *repository.MQTTConfigRepository) *MQTTHandler {
	return &MQTTHandler{
		mqttRepo:  mqttRepo,
		topicRepo: repository.NewMQTTTopicRepository(global.DB),
	}
}

func (h *MQTTHandler) GetConfig(c *gin.Context) {

	cfg, err := h.mqttRepo.Get()
	if err != nil {
		global.Logger.Error("Handler.GetConfig: 数据库查询失败", zap.Error(err))
		response.Error(c, response.CodeError, "获取MQTT配置失败")
		return
	}

	if cfg == nil {
		global.Logger.Debug("Handler.GetConfig: 数据库无配置，返回默认值")
		cfg = &model.MQTTConfig{
			Broker:              config.CONFIG.MQTT.Broker,
			Protocol:            "mqtt://",
			Host:                "",
			Port:                config.CONFIG.MQTT.Port,
			Username:            config.CONFIG.MQTT.Username,
			Password:            config.CONFIG.MQTT.Password,
			ClientID:            config.CONFIG.MQTT.ClientID,
			KeepAlive:           config.CONFIG.MQTT.KeepAlive,
			QoS:                 int8(config.CONFIG.MQTT.QoS),
			On:                  false,
			GatewaySN:           config.CONFIG.Gateway.SN,
			CreatedAt:           time.Time{},
			UpdatedAt:           time.Time{},
			SSL:                 false,
			SSLVerify:           true,
			ALPNTag:             "",
			CertType:            "",
			CAFile:              "",
			CertFile:            "",
			KeyFile:             "",
			Version:             "5.0",
			ConnectTimeout:      10,
			AutoReconnect:       true,
			ReconnectPeriod:     4000,
			CleanStart:          false,
			SessionExpiry:       7200,
			ReceiveMax:          0,
			MaxPacketSize:       0,
			TopicAliasMax:       0,
			RequestResponseInfo: false,
			RequestProblemInfo:  false,
		}
	}

	if cfg.GatewaySN == "" {
		cfg.GatewaySN = config.CONFIG.Gateway.SN
	}

	type ResponseConfig struct {
		ID                  uint64    `json:"id"`
		Broker              string    `json:"broker"`
		Protocol            string    `json:"protocol"`
		Host                string    `json:"host"`
		Port                int       `json:"port"`
		Username            string    `json:"username"`
		Password            string    `json:"password"`
		ClientID            string    `json:"client_id"`
		KeepAlive           int       `json:"keep_alive"`
		QoS                 int8      `json:"qos"`
		On                  bool      `json:"on"`
		GatewaySN           string    `json:"gateway_sn"`
		CreatedAt           time.Time `json:"created_at"`
		UpdatedAt           time.Time `json:"updated_at"`
		SSL                 bool      `json:"ssl"`
		SSLVerify           bool      `json:"ssl_verify"`
		ALPNTag             string    `json:"alpn_tag"`
		CertType            string    `json:"cert_type"`
		CAFile              string    `json:"ca_file"`
		CertFile            string    `json:"cert_file"`
		KeyFile             string    `json:"key_file"`
		Version             string    `json:"version"`
		ConnectTimeout      int       `json:"connect_timeout"`
		AutoReconnect       bool      `json:"auto_reconnect"`
		ReconnectPeriod     int       `json:"reconnect_period"`
		CleanStart          bool      `json:"clean_start"`
		SessionExpiry       int       `json:"session_expiry"`
		ReceiveMax          *int      `json:"receive_max,omitempty"`
		MaxPacketSize       *int      `json:"max_packet_size,omitempty"`
		TopicAliasMax       *int      `json:"topic_alias_max,omitempty"`
		RequestResponseInfo bool      `json:"request_response_info"`
		RequestProblemInfo  bool      `json:"request_problem_info"`
	}

	resp := ResponseConfig{
		ID:                  cfg.ID,
		Broker:              cfg.Broker,
		Protocol:            cfg.Protocol,
		Host:                cfg.Host,
		Port:                cfg.Port,
		Username:            cfg.Username,
		Password:            cfg.Password,
		ClientID:            cfg.ClientID,
		KeepAlive:           cfg.KeepAlive,
		QoS:                 cfg.QoS,
		On:                  cfg.On,
		GatewaySN:           cfg.GatewaySN,
		CreatedAt:           cfg.CreatedAt,
		UpdatedAt:           cfg.UpdatedAt,
		SSL:                 cfg.SSL,
		SSLVerify:           cfg.SSLVerify,
		ALPNTag:             cfg.ALPNTag,
		CertType:            cfg.CertType,
		CAFile:              cfg.CAFile,
		CertFile:            cfg.CertFile,
		KeyFile:             cfg.KeyFile,
		Version:             cfg.Version,
		ConnectTimeout:      cfg.ConnectTimeout,
		AutoReconnect:       cfg.AutoReconnect,
		ReconnectPeriod:     cfg.ReconnectPeriod,
		CleanStart:          cfg.CleanStart,
		SessionExpiry:       cfg.SessionExpiry,
		RequestResponseInfo: cfg.RequestResponseInfo,
		RequestProblemInfo:  cfg.RequestProblemInfo,
	}

	if cfg.ReceiveMax > 0 {
		resp.ReceiveMax = &cfg.ReceiveMax
	}
	if cfg.MaxPacketSize > 0 {
		resp.MaxPacketSize = &cfg.MaxPacketSize
	}
	if cfg.TopicAliasMax > 0 {
		resp.TopicAliasMax = &cfg.TopicAliasMax
	}

	response.Success(c, resp)
	global.Logger.Debug("Handler.GetConfig: 返回成功",
		zap.Uint64("id", resp.ID),
		zap.String("protocol", resp.Protocol),
		zap.String("host", resp.Host),
		zap.Int("port", resp.Port),
		zap.Bool("ssl", resp.SSL),
		zap.Bool("on", resp.On),
	)
}

func (h *MQTTHandler) UpdateConfig(c *gin.Context) {
	var req model.MQTTConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		global.Logger.Warn("Handler.UpdateConfig: 参数解析失败", zap.Error(err))
		response.Error(c, response.CodeInvalidParam, "参数错误")
		return
	}

	global.Logger.Debug("Handler.UpdateConfig: 收到请求",
		zap.String("protocol", req.Protocol),
		zap.String("host", req.Host),
		zap.Int("port", req.Port),
		zap.Bool("ssl", req.SSL),
		zap.String("version", req.Version),
		zap.Bool("auto_reconnect", req.AutoReconnect),
	)

	// 保存配置时：关闭 on/off（等用户手动“连接”或“重连/测试”再决定是否置为 true）
	req.On = false
	req.GatewaySN = config.CONFIG.Gateway.SN

	h.syncToGlobalConfig(&req)

	if err := h.mqttRepo.Update(&req); err != nil {
		global.Logger.Error("Handler.UpdateConfig: 数据库更新失败", zap.Error(err))
		response.Error(c, response.CodeError, "更新MQTT配置失败")
		return
	}

	global.Logger.Info("Handler.UpdateConfig: 保存成功")
	response.Success(c, nil)
}

func (h *MQTTHandler) GetStatus(c *gin.Context) {
	connected := false
	if global.MQTTClient != nil {
		connected = global.MQTTClient.IsConnected()
	}
	global.Logger.Debug("Handler.GetStatus: 返回状态", zap.Bool("connected", connected))
	response.Success(c, gin.H{
		"connected": connected,
		"uptime":    service.GetUptime(),
	})
}

func (h *MQTTHandler) syncToGlobalConfig(req *model.MQTTConfig) {
	global.Logger.Debug("syncToGlobalConfig: 同步配置到全局",
		zap.String("protocol", req.Protocol),
		zap.String("host", req.Host),
		zap.Int("port", req.Port),
		zap.Bool("ssl", req.SSL),
		zap.String("version", req.Version),
	)
	config.CONFIG.MQTT.Protocol = req.Protocol
	config.CONFIG.MQTT.Host = req.Host
	config.CONFIG.MQTT.Port = req.Port
	config.CONFIG.MQTT.Username = req.Username
	config.CONFIG.MQTT.Password = req.Password
	config.CONFIG.MQTT.ClientID = req.ClientID
	config.CONFIG.MQTT.KeepAlive = req.KeepAlive
	config.CONFIG.MQTT.QoS = byte(req.QoS)
	config.CONFIG.MQTT.SSL = req.SSL
	config.CONFIG.MQTT.SSLVerify = req.SSLVerify
	config.CONFIG.MQTT.ALPNTag = req.ALPNTag
	config.CONFIG.MQTT.CertType = req.CertType
	config.CONFIG.MQTT.CAFile = req.CAFile
	config.CONFIG.MQTT.CertFile = req.CertFile
	config.CONFIG.MQTT.KeyFile = req.KeyFile
	config.CONFIG.MQTT.Version = req.Version
	config.CONFIG.MQTT.ConnectTimeout = req.ConnectTimeout
	config.CONFIG.MQTT.AutoReconnect = req.AutoReconnect
	config.CONFIG.MQTT.ReconnectPeriod = req.ReconnectPeriod
	config.CONFIG.MQTT.CleanStart = req.CleanStart
	config.CONFIG.MQTT.SessionExpiry = req.SessionExpiry
	config.CONFIG.MQTT.ReceiveMax = req.ReceiveMax
	config.CONFIG.MQTT.MaxPacketSize = req.MaxPacketSize
	config.CONFIG.MQTT.TopicAliasMax = req.TopicAliasMax
	config.CONFIG.MQTT.RequestResponse = req.RequestResponseInfo
	config.CONFIG.MQTT.RequestProblem = req.RequestProblemInfo
}

func (h *MQTTHandler) rebuildClient() {
	if global.MQTTClient != nil {
		_ = global.MQTTClient.Close()
	}
	global.MQTTClient = global.NewMqttClient()
}

func (h *MQTTHandler) waitForConnected(timeout time.Duration, pollInterval time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if global.MQTTClient != nil && global.MQTTClient.IsConnected() {
			return true
		}
		time.Sleep(pollInterval)
	}
	return global.MQTTClient != nil && global.MQTTClient.IsConnected()
}

func (h *MQTTHandler) Connect(c *gin.Context) {
	var req model.MQTTConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		global.Logger.Error("Handler.Connect: 解析参数失败", zap.Error(err))
		response.Error(c, response.CodeInvalidParam, "参数错误")
		return
	}

	global.Logger.Info("Handler.Connect: 收到连接请求",
		zap.String("protocol", req.Protocol),
		zap.String("host", req.Host),
		zap.Int("port", req.Port),
		zap.Bool("ssl", req.SSL),
	)

	req.GatewaySN = config.CONFIG.Gateway.SN

	h.syncToGlobalConfig(&req)
	h.rebuildClient()

	if err := global.MQTTClient.Connect(); err != nil {
		global.Logger.Error("Handler.Connect: 连接失败", zap.Error(err))
		response.Error(c, response.CodeError, "连接失败")
		return
	}

	// 只有在“真正连接成功”后才写 on=true 到数据库
	if ok := h.waitForConnected(6*time.Second, 500*time.Millisecond); !ok {
		global.Logger.Warn("Handler.Connect: 连接超时")
		response.Error(c, response.CodeError, "MQTT未在超时时间内连接成功")
		return
	}

	req.On = true
	if err := h.mqttRepo.Update(&req); err != nil {
		global.Logger.Error("Handler.Connect: 更新连接状态失败", zap.Error(err))
		return
	}

	global.Logger.Info("Handler.Connect: 连接成功")
	response.Success(c, nil)
}

func (h *MQTTHandler) Disconnect(c *gin.Context) {
	var req model.MQTTConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		global.Logger.Warn("Handler.Disconnect: 参数解析失败", zap.Error(err))
		response.Error(c, response.CodeInvalidParam, "参数错误")
		return
	}

	global.Logger.Info("Handler.Disconnect: 收到断开请求")

	req.GatewaySN = config.CONFIG.Gateway.SN
	h.syncToGlobalConfig(&req)

	req.On = false

	if global.MQTTClient != nil {
		_ = global.MQTTClient.Close()
	}
	global.MQTTClient = global.NewMqttClient()

	if err := h.mqttRepo.Update(&req); err != nil {
		global.Logger.Error("Handler.Disconnect: 更新断开状态失败", zap.Error(err))
		response.Error(c, response.CodeError, "更新MQTT断开状态失败")
		return
	}

	global.Logger.Info("Handler.Disconnect: 断开成功")
	response.Success(c, nil)
}

func (h *MQTTHandler) TestConnection(c *gin.Context) {
	var req model.MQTTConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		global.Logger.Warn("Handler.TestConnection: 参数解析失败", zap.Error(err))
		response.Error(c, response.CodeInvalidParam, "参数错误")
		return
	}

	global.Logger.Info("Handler.TestConnection: 收到测试连接请求",
		zap.String("protocol", req.Protocol),
		zap.String("host", req.Host),
		zap.Int("port", req.Port),
		zap.Bool("ssl", req.SSL),
	)

	req.GatewaySN = config.CONFIG.Gateway.SN
	h.syncToGlobalConfig(&req)

	h.rebuildClient()
	_ = global.MQTTClient.Connect()

	connected := h.waitForConnected(6*time.Second, 500*time.Millisecond)
	global.Logger.Info("Handler.TestConnection: 测试结果", zap.Bool("connected", connected))
	response.Success(c, gin.H{
		"connected": connected,
	})
}

// GetTopics 获取主题模板列表
func (h *MQTTHandler) GetTopics(c *gin.Context) {
	global.Logger.Debug("Handler.GetTopics: 收到请求")
	topics, err := h.topicRepo.List()
	if err != nil {
		global.Logger.Error("Handler.GetTopics: 查询失败", zap.Error(err))
		response.Error(c, response.CodeError, "获取主题列表失败")
		return
	}
	global.Logger.Debug("Handler.GetTopics: 返回成功", zap.Int("count", len(topics)))
	response.Success(c, topics)
}

// BatchSaveTopics 批量保存主题模板
func (h *MQTTHandler) BatchSaveTopics(c *gin.Context) {
	var req []*model.MQTTTopicTemplate
	if err := c.ShouldBindJSON(&req); err != nil {
		global.Logger.Warn("Handler.BatchSaveTopics: 参数解析失败", zap.Error(err))
		response.Error(c, response.CodeInvalidParam, "参数错误")
		return
	}

	global.Logger.Info("Handler.BatchSaveTopics: 收到请求", zap.Int("count", len(req)))

	if err := h.topicRepo.BatchSave(req); err != nil {
		global.Logger.Error("Handler.BatchSaveTopics: 保存失败", zap.Error(err))
		response.Error(c, response.CodeError, "保存主题失败")
		return
	}

	global.Logger.Info("Handler.BatchSaveTopics: 保存成功", zap.Int("count", len(req)))
	response.Success(c, nil)
}

// ResetTopics 恢复主题模板为默认值
func (h *MQTTHandler) ResetTopics(c *gin.Context) {
	global.Logger.Info("Handler.ResetTopics: 收到请求")
	if err := h.topicRepo.ResetToDefaults(); err != nil {
		global.Logger.Error("Handler.ResetTopics: 恢复失败", zap.Error(err))
		response.Error(c, response.CodeError, "恢复默认主题失败")
		return
	}

	defaults := repository.GetDefaultTopics()
	global.Logger.Info("Handler.ResetTopics: 恢复成功", zap.Int("count", len(defaults)))
	response.Success(c, defaults)
}

// GetTopicConfig 获取全局主题配置
func (h *MQTTHandler) GetTopicConfig(c *gin.Context) {
	global.Logger.Debug("Handler.GetTopicConfig: 收到请求", zap.String("gateway_sn", config.CONFIG.Gateway.SN))
	cfg, err := h.topicRepo.GetConfig(config.CONFIG.Gateway.SN)
	if err != nil {
		global.Logger.Error("Handler.GetTopicConfig: 查询失败", zap.Error(err))
		response.Error(c, response.CodeError, "获取主题配置失败")
		return
	}
	global.Logger.Debug("Handler.GetTopicConfig: 返回成功",
		zap.String("prefix", cfg.Prefix),
		zap.String("up_keyword", cfg.UpKeyword),
		zap.String("down_keyword", cfg.DownKeyword),
		zap.Bool("show_direction", cfg.ShowDirection),
	)
	response.Success(c, cfg)
}

// SaveTopicConfig 保存全局主题配置
func (h *MQTTHandler) SaveTopicConfig(c *gin.Context) {
	var req model.MQTTTopicConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		global.Logger.Warn("Handler.SaveTopicConfig: 参数解析失败", zap.Error(err))
		response.Error(c, response.CodeInvalidParam, "参数错误")
		return
	}
	req.GatewaySN = config.CONFIG.Gateway.SN

	global.Logger.Info("Handler.SaveTopicConfig: 收到请求",
		zap.String("prefix", req.Prefix),
		zap.String("up_keyword", req.UpKeyword),
		zap.String("down_keyword", req.DownKeyword),
		zap.Bool("show_direction", req.ShowDirection),
	)

	if err := h.topicRepo.SaveConfig(&req); err != nil {
		global.Logger.Error("Handler.SaveTopicConfig: 保存失败", zap.Error(err))
		response.Error(c, response.CodeError, "保存主题配置失败")
		return
	}

	global.Logger.Info("Handler.SaveTopicConfig: 保存成功")
	response.Success(c, nil)
}

// ResetTopicConfig 恢复全局主题配置为默认值
func (h *MQTTHandler) ResetTopicConfig(c *gin.Context) {
	global.Logger.Info("Handler.ResetTopicConfig: 收到请求", zap.String("gateway_sn", config.CONFIG.Gateway.SN))
	if err := h.topicRepo.ResetConfig(config.CONFIG.Gateway.SN); err != nil {
		global.Logger.Error("Handler.ResetTopicConfig: 恢复失败", zap.Error(err))
		response.Error(c, response.CodeError, "恢复默认主题配置失败")
		return
	}

	defaultCfg := &model.MQTTTopicConfig{
		Prefix:        "/aixot",
		UpKeyword:     "up",
		DownKeyword:   "down",
		ShowDirection: true,
		GatewaySN:     config.CONFIG.Gateway.SN,
	}
	global.Logger.Info("Handler.ResetTopicConfig: 恢复成功")
	response.Success(c, defaultCfg)
}
