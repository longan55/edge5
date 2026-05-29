package handler

import (
	"edge5/config"
	"edge5/global"
	"edge5/internal/model"
	"edge5/internal/repository"
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
			Broker:    config.CONFIG.MQTT.Broker,
			Port:      config.CONFIG.MQTT.Port,
			Username:  config.CONFIG.MQTT.Username,
			Password:  config.CONFIG.MQTT.Password,
			ClientID:  config.CONFIG.MQTT.ClientID,
			KeepAlive: config.CONFIG.MQTT.KeepAlive,
			QoS:       int8(config.CONFIG.MQTT.QoS),
			On:        false,
			GatewaySN: config.CONFIG.Gateway.SN,
			CreatedAt: time.Time{},
			UpdatedAt: time.Time{},
		}
	}

	// 确保网关序列号总是有值（避免数据库 not null / uniqueIndex 冲突）
	if cfg.GatewaySN == "" {
		cfg.GatewaySN = config.CONFIG.Gateway.SN
	}

	response.Success(c, cfg)
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
	})
}

func (h *MQTTHandler) syncToGlobalConfig(req *model.MQTTConfig) {
	config.CONFIG.MQTT.Broker = req.Broker
	config.CONFIG.MQTT.Port = req.Port
	config.CONFIG.MQTT.Username = req.Username
	config.CONFIG.MQTT.Password = req.Password
	config.CONFIG.MQTT.ClientID = req.ClientID
	config.CONFIG.MQTT.KeepAlive = req.KeepAlive
	config.CONFIG.MQTT.QoS = byte(req.QoS)
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
