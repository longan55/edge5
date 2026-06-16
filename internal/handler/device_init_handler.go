package handler

import (
	"edge5/internal/service"
	"edge5/internal/utils/response"

	"github.com/gin-gonic/gin"
)

type DeviceInitHandler struct{}

func NewDeviceInitHandler() *DeviceInitHandler {
	return &DeviceInitHandler{}
}

// TestConnections 触发设备连接测试
func (h *DeviceInitHandler) TestConnections(c *gin.Context) {
	if err := service.TestDeviceConnections(); err != nil {
		response.Error(c, response.CodeError,err.Error())
		return
	}
	response.Success(c, gin.H{
		"message": "设备连接测试已启动",
	})
}