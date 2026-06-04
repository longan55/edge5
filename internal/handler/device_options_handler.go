package handler

import (
	"edge5/internal/pkg/device/templates"
	"edge5/internal/utils/response"

	"github.com/gin-gonic/gin"
)

type DeviceOptionsHandler struct {
	// no deps
}

func NewDeviceOptionsHandler() *DeviceOptionsHandler {
	return &DeviceOptionsHandler{}
}

// GetDeviceOptions returns hardcoded hierarchical options for device add flow.
func (h *DeviceOptionsHandler) GetDeviceOptions(c *gin.Context) {
	response.Success(c, templates.GetDeviceOptions())
}
