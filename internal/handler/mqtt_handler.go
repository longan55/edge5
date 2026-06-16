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
	mqttRepo *repository.MQTTConfigRepository
}

func NewMQTTHandler(mqttRepo *repository.MQTTConfigRepository) *MQTTHandler {
	return &MQTTHandler{mqttRepo: mqttRepo}
}

func (h *MQTTHandler) GetConfig(c *gin.Context) {
	cfg, err := h.mqttRepo.Get()
	if err != nil {
		response.Error(c, response.CodeError, "获取MQTT配置失败")
		return
	}

	if cfg == nil {
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
}

func (h *MQTTHandler) UpdateConfig(c *gin.Context) {
	var req model.MQTTConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, response.CodeInvalidParam, "参数错误")
		return
	}

	// 保存配置时：关闭 on/off（等用户手动“连接”或“重连/测试”再决定是否置为 true）
	req.On = false
	req.GatewaySN = config.CONFIG.Gateway.SN

	h.syncToGlobalConfig(&req)

	if err := h.mqttRepo.Update(&req); err != nil {
		response.Error(c, response.CodeError, "更新MQTT配置失败")
		return
	}

	response.Success(c, nil)
}

func (h *MQTTHandler) GetStatus(c *gin.Context) {
	connected := false
	if global.MQTTClient != nil {
		connected = global.MQTTClient.IsConnected()
	}
	response.Success(c, gin.H{
		"connected": connected,
		"uptime":    service.GetUptime(),
	})
}

func (h *MQTTHandler) syncToGlobalConfig(req *model.MQTTConfig) {
	config.CONFIG.MQTT.Broker = req.Broker
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
		global.Logger.Error("解析MQTT连接参数失败", zap.Error(err))
		response.Error(c, response.CodeInvalidParam, "参数错误")
		return
	}

	req.GatewaySN = config.CONFIG.Gateway.SN

	h.syncToGlobalConfig(&req)
	h.rebuildClient()

	if err := global.MQTTClient.Connect(); err != nil {
		global.Logger.Error("连接MQTT Broker失败", zap.Error(err))
		response.Error(c, response.CodeError, "连接失败")
		return
	}

	// 只有在“真正连接成功”后才写 on=true 到数据库
	if ok := h.waitForConnected(6*time.Second, 500*time.Millisecond); !ok {
		response.Error(c, response.CodeError, "MQTT未在超时时间内连接成功")
		return
	}

	req.On = true
	if err := h.mqttRepo.Update(&req); err != nil {
		response.Error(c, response.CodeError, "更新MQTT连接状态失败")
		return
	}

	response.Success(c, nil)
}

func (h *MQTTHandler) Disconnect(c *gin.Context) {
	var req model.MQTTConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, response.CodeInvalidParam, "参数错误")
		return
	}

	req.GatewaySN = config.CONFIG.Gateway.SN
	h.syncToGlobalConfig(&req)

	// 断开时：置为 false
	req.On = false

	if global.MQTTClient != nil {
		_ = global.MQTTClient.Close()
	}
	global.MQTTClient = global.NewMqttClient()

	if err := h.mqttRepo.Update(&req); err != nil {
		response.Error(c, response.CodeError, "更新MQTT断开状态失败")
		return
	}

	response.Success(c, nil)
}

func (h *MQTTHandler) TestConnection(c *gin.Context) {
	var req model.MQTTConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, response.CodeInvalidParam, "参数错误")
		return
	}

	// 测试不改数据库 on/off 状态
	req.GatewaySN = config.CONFIG.Gateway.SN
	h.syncToGlobalConfig(&req)

	h.rebuildClient()
	_ = global.MQTTClient.Connect()

	connected := h.waitForConnected(6*time.Second, 500*time.Millisecond)
	response.Success(c, gin.H{
		"connected": connected,
	})
}
