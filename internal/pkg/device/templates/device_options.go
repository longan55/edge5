package templates

// DeviceOptionsResponse is the payload returned to the frontend when adding a device.
// It contains:
// - deviceTypes: hierarchical options (deviceType -> brand -> protocol -> model)
// - protocolOptions: dynamic connection form fields per protocol
type DeviceOptionsResponse struct {
	DeviceTypes     []DeviceTypeOption              `json:"deviceTypes"`
	ProtocolOptions map[string]ProtocolOptionsGroup `json:"protocolOptions"`
}

type DeviceTypeOption struct {
	Value  string        `json:"value"`
	Label  string        `json:"label"`
	Brands []BrandOption `json:"brands"`
}

type BrandOption struct {
	Value     string           `json:"value"`
	Label     string           `json:"label"`
	Protocols []ProtocolOption `json:"protocols"`
}

type ProtocolOption struct {
	Value        string   `json:"value"`
	Label        string   `json:"label"`
	ModelRelated bool     `json:"modelRelated"`
	Models       []string `json:"models"`
}

type ProtocolOptionsGroup struct {
	Options []ProtocolConnectionOption `json:"options"`
}

type ProtocolConnectionOption struct {
	Name     string        `json:"name"`
	CName    string        `json:"cName"`
	Type     string        `json:"type"`
	Required bool          `json:"required"`
	Default  interface{}   `json:"default,omitempty"`
	Choices  []interface{} `json:"choices,omitempty"` // 非空则前端渲染为下拉选项；为空则渲染输入框
}

func GetDeviceOptions() *DeviceOptionsResponse {
	// Hardcoded per requirement.
	// NOTE: This endpoint is used only for frontend option rendering; actual plugin parsing
	// still depends on device.config.runtime fields and device.plugin Connect param contract.
	return &DeviceOptionsResponse{
		DeviceTypes: []DeviceTypeOption{
			{
				Value: "PLC",
				Label: "PLC",
				Brands: []BrandOption{
					{
						Value: "Mitsubishi",
						Label: "Mitsubishi(三菱)",
						Protocols: []ProtocolOption{
							{
								Value:        "MC-3E",
								Label:        "MC-3E（以太网）",
								ModelRelated: true,
								Models:       []string{"Q03", "Q04"},
							},
							{
								Value:        "FX-Serial",
								Label:        "FX（串口）",
								ModelRelated: false,
								Models:       []string{},
							},
						},
					},
					{
						Value: "Siemens",
						Label: "Siemens(西门子)",
						Protocols: []ProtocolOption{
							{
								Value:        "S7Comm",
								Label:        "S7 通信",
								ModelRelated: true,
								Models:       []string{"s7-200", "s7-300"},
							},
						},
					},
				},
			},
			{
				Value: "CNC",
				Label: "CNC",
				Brands: []BrandOption{
					{
						Value: "Mitsubishi",
						Label: "Mitsubishi(三菱)",
						Protocols: []ProtocolOption{
							{
								Value:        "Melsec-CNC",
								Label:        "Melsec CNC",
								ModelRelated: false,
								Models:       []string{},
							},
						},
					},
					{
						Value: "Fanuc",
						Label: "Fanuc(发那科)",
						Protocols: []ProtocolOption{
							{
								Value:        "Focas",
								Label:        "Focas TCP",
								ModelRelated: false,
								Models:       []string{},
							},
						},
					},
				},
			},
		},
		ProtocolOptions: map[string]ProtocolOptionsGroup{
			"MC-3E": {
				Options: []ProtocolConnectionOption{
					{Name: "ip", CName: "IP地址", Type: "string", Required: true},
					{Name: "port", CName: "端口号", Type: "int", Required: true, Default: 6000},
					{Name: "pcNum", CName: "PC编号", Type: "string", Required: true, Default: "0xFF"},
				},
			},
			"FX-Serial": {
				Options: []ProtocolConnectionOption{
					{Name: "serialPort", CName: "串口号", Type: "string", Required: true},
					{Name: "baudRate", CName: "波特率", Type: "int", Required: true, Default: 9600, Choices: []interface{}{9600, 19200, 38400, 115200}},
					{Name: "dataBit", CName: "数据位", Type: "int", Required: true, Default: 7},
					{Name: "stopBit", CName: "停止位", Type: "float", Required: true, Default: 1},
					{Name: "parity", CName: "校验位", Type: "string", Required: true, Default: "even", Choices: []interface{}{"odd", "even"}},
				},
			},
			"S7Comm": {
				Options: []ProtocolConnectionOption{
					{Name: "ip", CName: "IP地址", Type: "string", Required: true},
					{Name: "rack", CName: "机架号", Type: "int", Required: true, Default: 0},
					{Name: "slot", CName: "槽号", Type: "int", Required: true, Default: 2},
				},
			},
			"Melsec-CNC": {
				Options: []ProtocolConnectionOption{
					{Name: "ip", CName: "IP地址", Type: "string", Required: true},
					{Name: "port", CName: "端口号", Type: "int", Required: true, Default: 683},
				},
			},
			"Focas": {
				Options: []ProtocolConnectionOption{
					{Name: "ip", CName: "IP地址", Type: "string", Required: true},
					{Name: "port", CName: "端口号", Type: "int", Required: true, Default: 8193},
				},
			},
		},
	}
}
