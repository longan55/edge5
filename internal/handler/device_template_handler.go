package handler

import (
	"edge5/internal/pkg/device/templates"
	"edge5/internal/utils/response"
	"net/http"

	"github.com/gin-gonic/gin"
)

type DeviceTemplateHandler struct {
	// no deps for now
}

func NewDeviceTemplateHandler() *DeviceTemplateHandler {
	return &DeviceTemplateHandler{}
}

// GetTemplate 返回 device_type + protocol 对应的 JSON Schema 以及默认 config
// query: device_type, protocol
func (h *DeviceTemplateHandler) GetTemplate(c *gin.Context) {
	deviceType := c.Query("device_type")
	if deviceType == "" {
		response.Error(c, response.CodeInvalidParam, "device_type is required")
		return
	}
	protocol := c.Query("protocol")

	resp, err := templates.GetDeviceTemplate(deviceType, protocol)
	if err != nil {
		response.ErrorWithCode(c, http.StatusBadRequest, response.CodeInvalidParam, err.Error())
		return
	}

	// resp 可直接被 JSON 序列化给前端
	response.Success(c, resp)
}
