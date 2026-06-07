package handler

import (
	"edge5/internal/pkg/protocol"
	"edge5/internal/utils/response"

	"github.com/gin-gonic/gin"
)

type DeviceOptionsHandler struct {
	// 从协议注册表动态获取
}

func NewDeviceOptionsHandler() *DeviceOptionsHandler {
	return &DeviceOptionsHandler{}
}

// GetDeviceOptions 从协议注册表动态生成设备添加选项
// 包括设备类型层级结构和每个协议的连接参数
func (h *DeviceOptionsHandler) GetDeviceOptions(c *gin.Context) {
	reg := protocol.DefaultRegistry()
	protocols := reg.List()

	resp := buildOptions(protocols)
	response.Success(c, resp)
}

// optionsResponse 前端设备添加选项响应结构
type optionsResponse struct {
	DeviceTypes     []deviceTypeOption              `json:"deviceTypes"`
	ProtocolOptions map[string]protocolOptionsGroup `json:"protocolOptions"`
}

type deviceTypeOption struct {
	Value  string        `json:"value"`
	Label  string        `json:"label"`
	Brands []brandOption `json:"brands"`
}

type brandOption struct {
	Value     string           `json:"value"`
	Label     string           `json:"label"`
	Protocols []protocolOption `json:"protocols"`
}

type protocolOption struct {
	Value        string   `json:"value"`
	Label        string   `json:"label"`
	ModelRelated bool     `json:"modelRelated"`
	Models       []string `json:"models"`
}

type protocolOptionsGroup struct {
	Options []protocolConnectionOption `json:"options"`
}

type protocolConnectionOption struct {
	Name     string        `json:"name"`
	CName    string        `json:"cName"`
	Type     string        `json:"type"`
	Required bool          `json:"required"`
	Default  interface{}   `json:"default,omitempty"`
	Choices  []interface{} `json:"choices,omitempty"`
}

func buildOptions(protocols []protocol.ProtocolInfo) *optionsResponse {
	resp := &optionsResponse{
		DeviceTypes:     make([]deviceTypeOption, 0),
		ProtocolOptions: make(map[string]protocolOptionsGroup),
	}

	// 按 deviceType → brand → protocol 构建层级
	typeMap := make(map[string]map[string][]protocol.ProtocolInfo)
	for _, p := range protocols {
		if typeMap[p.DeviceType] == nil {
			typeMap[p.DeviceType] = make(map[string][]protocol.ProtocolInfo)
		}
		typeMap[p.DeviceType][p.Brand] = append(typeMap[p.DeviceType][p.Brand], p)
	}

	for deviceType, brands := range typeMap {
		dto := deviceTypeOption{
			Value:  deviceType,
			Label:  deviceType,
			Brands: make([]brandOption, 0),
		}
		for brand, protos := range brands {
			bo := brandOption{
				Value:     brand,
				Label:     brand,
				Protocols: make([]protocolOption, 0),
			}
			for _, p := range protos {
				po := protocolOption{
					Value:        p.Name,
					Label:        p.Name,
					ModelRelated: len(p.Models) > 0,
					Models:       p.Models,
				}
				bo.Protocols = append(bo.Protocols, po)

				// 构建连接参数
				opts := make([]protocolConnectionOption, 0)
				for _, cp := range p.ConnectionParams {
					opt := protocolConnectionOption{
						Name:     cp.Name,
						CName:    cp.CName,
						Type:     cp.Type,
						Required: cp.Required,
					}
					if cp.Default != "" {
						opt.Default = cp.Default
					}
					if len(cp.Choices) > 0 {
						choices := make([]interface{}, len(cp.Choices))
						for i, c := range cp.Choices {
							choices[i] = c
						}
						opt.Choices = choices
					}
					opts = append(opts, opt)
				}
				resp.ProtocolOptions[p.Name] = protocolOptionsGroup{Options: opts}
			}
			dto.Brands = append(dto.Brands, bo)
		}
		resp.DeviceTypes = append(resp.DeviceTypes, dto)
	}

	return resp
}
