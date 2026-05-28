package handler

import (
	"edge5/config"
	"edge5/global"
	"edge5/internal/model"
	"edge5/internal/repository"
	"edge5/internal/utils/response"

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
			Status:    0,
		}
	}

	response.Success(c, cfg)
}

func (h *MQTTHandler) UpdateConfig(c *gin.Context) {
	var req model.MQTTConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, response.CodeInvalidParam, "参数错误")
		return
	}

	// 只保存配置；连接状态由 connect/disconnect/status 维护
	req.Status = 0
	if err := h.mqttRepo.Update(&req); err != nil {
		response.Error(c, response.CodeError, "更新MQTT配置失败")
		return
	}

	// 同步到内存配置，便于后续 connect 使用
	config.CONFIG.MQTT.Broker = req.Broker
	config.CONFIG.MQTT.Port = req.Port
	config.CONFIG.MQTT.Username = req.Username
	config.CONFIG.MQTT.Password = req.Password
	config.CONFIG.MQTT.ClientID = req.ClientID
	config.CONFIG.MQTT.KeepAlive = req.KeepAlive
	config.CONFIG.MQTT.QoS = byte(req.QoS)

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

func (h *MQTTHandler) applyConfigAndReconnect(c *gin.Context, connect bool) {
	var req model.MQTTConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		global.Logger.Error("解析MQTT连接参数失败", zap.Error(err))
		response.Error(c, response.CodeInvalidParam, "参数错误")
		return
	}

	// 同步到内存配置
	config.CONFIG.MQTT.Broker = req.Broker
	config.CONFIG.MQTT.Port = req.Port
	config.CONFIG.MQTT.Username = req.Username
	config.CONFIG.MQTT.Password = req.Password
	config.CONFIG.MQTT.ClientID = req.ClientID
	config.CONFIG.MQTT.KeepAlive = req.KeepAlive
	config.CONFIG.MQTT.QoS = byte(req.QoS)

	// 保存配置
	if connect {
		req.Status = 1
	} else {
		req.Status = 0
	}

	_ = h.mqttRepo.Update(&req)

	// 重建 client，确保能在断开后重新连接
	_ = global.MQTTClient.Close()
	global.MQTTClient = global.NewMqttClient()

	if connect {
		if err := global.MQTTClient.Connect(); err != nil {
			global.Logger.Error("连接MQTT Broker失败", zap.Error(err))
			response.Error(c, response.CodeError, "连接失败")
			return
		}
	}
}

func (h *MQTTHandler) Connect(c *gin.Context) {
	h.applyConfigAndReconnect(c, true)
	if c.IsAborted() {
		return
	}
	response.Success(c, nil)
}

func (h *MQTTHandler) Disconnect(c *gin.Context) {
	// 断开时仍尝试按前端传入参数更新配置
	h.applyConfigAndReconnect(c, false)
	if c.IsAborted() {
		return
	}
	response.Success(c, nil)
}

func (h *MQTTHandler) TestConnection(c *gin.Context) {
	// 简单策略：尝试连接成功即可
	h.applyConfigAndReconnect(c, true)
	if c.IsAborted() {
		return
	}
	response.Success(c, gin.H{
		"connected": global.MQTTClient.IsConnected(),
	})
}
