package handler

import (
	"edge5/global"
	"edge5/internal/service"
	"edge5/internal/utils/response"

	"github.com/gin-gonic/gin"
)

type SystemHandler struct{}

func NewSystemHandler() *SystemHandler {
	return &SystemHandler{}
}

// GetStatus 获取系统状态信息（合并系统状态和MQTT状态）
func (h *SystemHandler) GetStatus(c *gin.Context) {
	status := service.GetSystemStatus()

	// 添加 MQTT 连接状态
	mqttConnected := false
	if global.MQTTClient != nil {
		mqttConnected = global.MQTTClient.IsConnected()
	}
	status["mqttConnected"] = mqttConnected

	// 添加系统资源信息
	systemMonitor := service.GetSystemMonitor()
	if systemMonitor != nil {
		resources := systemMonitor.GetResources()
		status["resources"] = resources
	}

	response.Success(c, status)
}
