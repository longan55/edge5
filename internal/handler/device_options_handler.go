package handler

import (
	"edge5/internal/pkg/protocol"
	"edge5/internal/utils/response"

	"github.com/gin-gonic/gin"
)

type DeviceOptionsHandler struct{}

func NewDeviceOptionsHandler() *DeviceOptionsHandler {
	return &DeviceOptionsHandler{}
}

// GetDeviceOptions 从协议注册表动态生成设备添加选项
func (h *DeviceOptionsHandler) GetDeviceOptions(c *gin.Context) {
	reg := protocol.DefaultRegistry()
	protocols := reg.List()
	resp := buildOptions(protocols)
	response.Success(c, resp)
}

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

func buildOptions(protocols []protocol.Metadata) *optionsResponse {
	resp := &optionsResponse{
		DeviceTypes:     make([]deviceTypeOption, 0),
		ProtocolOptions: make(map[string]protocolOptionsGroup),
	}

	typeMap := make(map[string]map[string][]protocol.Metadata)
	for _, p := range protocols {
		deviceType := protocol.GetInfoString(p, "deviceType")
		brand := protocol.GetInfoString(p, "brand")
		if deviceType == "" || brand == "" {
			continue
		}
		if typeMap[deviceType] == nil {
			typeMap[deviceType] = make(map[string][]protocol.Metadata)
		}
		typeMap[deviceType][brand] = append(typeMap[deviceType][brand], p)
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
				name := protocol.GetInfoString(p, "name")
				models := protocol.GetInfoStrings(p, "models")

				po := protocolOption{
					Value:        name,
					Label:        name,
					ModelRelated: len(models) > 0,
					Models:       models,
				}
				bo.Protocols = append(bo.Protocols, po)

				cp := protocol.ExtractConnectionParams(p)
				opts := make([]protocolConnectionOption, 0)
				for _, param := range cp {
					opt := protocolConnectionOption{
						Name:     param.Name,
						CName:    param.CName,
						Type:     param.Type,
						Required: param.Required,
					}
					if param.Default != "" {
						opt.Default = param.Default
					}
					if len(param.Choices) > 0 {
						choices := make([]interface{}, len(param.Choices))
						for i, c := range param.Choices {
							choices[i] = c
						}
						opt.Choices = choices
					}
					opts = append(opts, opt)
				}
				resp.ProtocolOptions[name] = protocolOptionsGroup{Options: opts}
			}
			dto.Brands = append(dto.Brands, bo)
		}
		resp.DeviceTypes = append(resp.DeviceTypes, dto)
	}

	return resp
}
