package templates

import (
	"encoding/json"
	"fmt"
)

type DeviceTemplateResponse struct {
	Schema        map[string]any `json:"schema"`
	DefaultConfig map[string]any `json:"default_config"`
}

func GetDeviceTemplate(deviceType, protocol string) (*DeviceTemplateResponse, error) {
	switch deviceType {
	case "plc":
		return getPlcTemplate(protocol)
	case "cnc":
		return getCncTemplate()
	default:
		return nil, fmt.Errorf("unsupported device_type: %s", deviceType)
	}
}

func getPlcTemplate(protocol string) (*DeviceTemplateResponse, error) {
	// runtime schema depends on protocol (tcp/serial)
	var runtimeSchema map[string]any
	var runtimeDefault map[string]any

	switch protocol {
	case "tcp":
		runtimeSchema = map[string]any{
			"title": "PLC Runtime Config (TCP)",
			"type":  "object",
			"required": []any{
				"ip",
				"port",
			},
			"properties": map[string]any{
				"ip": map[string]any{
					"type":      "string",
					"title":     "IP地址",
					"minLength": 1,
				},
				"port": map[string]any{
					"type":    "integer",
					"title":   "端口",
					"minimum": 1,
					"maximum": 65535,
				},
				"extra": map[string]any{
					"type":    "object",
					"title":   "扩展参数（给插件，含 host/port 等）",
					"default": map[string]any{},
				},
			},
		}
		runtimeDefault = map[string]any{
			"ip":   "",
			"port": 6000,
			"extra": map[string]any{
				"host": "",
				"port": 50051,
			},
		}
	case "serial":
		runtimeSchema = map[string]any{
			"title": "PLC Runtime Config (Serial)",
			"type":  "object",
			"required": []any{
				"serial_port",
				"baud_rate",
			},
			"properties": map[string]any{
				"serial_port": map[string]any{
					"type":  "string",
					"title": "串口",
				},
				"baud_rate": map[string]any{
					"type":    "integer",
					"title":   "波特率",
					"minimum": 300,
					"maximum": 921600,
					"enum":    []any{9600, 19200, 38400, 115200},
				},
				"extra": map[string]any{
					"type":    "object",
					"title":   "扩展参数（给插件，含 host/port 等）",
					"default": map[string]any{},
				},
			},
		}
		runtimeDefault = map[string]any{
			"serial_port": "/dev/ttyS0",
			"baud_rate":   9600,
			"extra": map[string]any{
				"host": "",
				"port": 50051,
			},
		}
	default:
		return nil, fmt.Errorf("unsupported plc protocol: %s", protocol)
	}

	collectionSchema := map[string]any{
		"title": "PLC Collection Config",
		"type":  "object",
		"required": []any{
			"intervalMs",
			"points",
		},
		"properties": map[string]any{
			"intervalMs": map[string]any{
				"type":    "integer",
				"title":   "采集间隔(ms)",
				"minimum": 100,
				"default": 1000,
			},
			"points": map[string]any{
				"type":     "array",
				"title":    "点位列表",
				"minItems": 1,
				"items": map[string]any{
					"type": "object",
					"required": []any{
						"key",
						"zhName",
						"address",
						"type",
					},
					"properties": map[string]any{
						"key": map[string]any{
							"type":    "string",
							"title":   "英文字段Key",
							"pattern": "^[a-zA-Z_][a-zA-Z0-9_]*$",
						},
						"zhName": map[string]any{
							"type":  "string",
							"title": "中文名（点位含义）",
						},
						"address": map[string]any{
							"type":  "string",
							"title": "寄存器/地址",
						},
						"type": map[string]any{
							"type":  "string",
							"title": "数据类型",
							"enum":  []any{"int16", "uint16", "int32", "uint32", "float"},
						},
						"offset": map[string]any{
							"type":    "number",
							"title":   "偏移(可选)",
							"default": 0,
						},
						"scale": map[string]any{
							"type":    "number",
							"title":   "比例(可选)",
							"default": 1,
						},
						"unit": map[string]any{
							"type":  "string",
							"title": "单位(可选)",
						},
					},
				},
			},
		},
	}

	// unified config schema: { runtime: {...}, collection: {...} }
	schema := map[string]any{
		"title": "PLC Device Config",
		"type":  "object",
		"required": []any{
			"runtime",
			"collection",
		},
		"properties": map[string]any{
			"runtime":    runtimeSchema,
			"collection": collectionSchema,
		},
	}

	defaultConfig := map[string]any{
		"runtime": runtimeDefault,
		"collection": map[string]any{
			"intervalMs": 1000,
			"points": []any{
				map[string]any{
					"key":     "temp",
					"zhName":  "温度",
					"address": "D100",
					"type":    "int16",
					"offset":  0,
					"scale":   1,
					"unit":    "℃",
				},
			},
		},
	}

	return &DeviceTemplateResponse{
		Schema:        schema,
		DefaultConfig: defaultConfig,
	}, nil
}

func getCncTemplate() (*DeviceTemplateResponse, error) {
	// CNC typically doesn't need points; we define enabled fields list.
	collectionSchema := map[string]any{
		"title": "CNC Collection Config",
		"type":  "object",
		"required": []any{
			"intervalMs",
			"fields",
		},
		"properties": map[string]any{
			"intervalMs": map[string]any{
				"type":    "integer",
				"title":   "采集间隔(ms)",
				"minimum": 100,
				"default": 1000,
			},
			"fields": map[string]any{
				"type":     "array",
				"title":    "启用字段列表",
				"minItems": 1,
				"items": map[string]any{
					"type": "object",
					"required": []any{
						"key",
						"zhName",
					},
					"properties": map[string]any{
						"key": map[string]any{
							"type":  "string",
							"title": "英文字段Key（含义由 CNC 规范确定）",
							"enum":  []any{"spindleSpeed", "feedRate", "alarmCode"},
						},
						"zhName": map[string]any{
							"type":  "string",
							"title": "中文名（展示含义）",
						},
					},
				},
			},
		},
	}

	// CNC runtime: keep minimal so plugin can implement its own connection rules.
	// Here we reuse protocol notion but keep it flexible.
	runtimeSchema := map[string]any{
		"title":    "CNC Runtime Config",
		"type":     "object",
		"required": []any{},
		"properties": map[string]any{
			// allow plugin-defined extra fields through "Extra" object
			"extra": map[string]any{
				"type":    "object",
				"title":   "扩展参数（透传给插件）",
				"default": map[string]any{},
			},
		},
	}

	schema := map[string]any{
		"title": "CNC Device Config",
		"type":  "object",
		"required": []any{
			"runtime",
			"collection",
		},
		"properties": map[string]any{
			"runtime":    runtimeSchema,
			"collection": collectionSchema,
		},
	}

	defaultConfig := map[string]any{
		"runtime": map[string]any{
			"extra": map[string]any{},
		},
		"collection": map[string]any{
			"intervalMs": 1000,
			"fields": []any{
				map[string]any{
					"key":    "spindleSpeed",
					"zhName": "主轴转速",
				},
			},
		},
	}

	return &DeviceTemplateResponse{
		Schema:        schema,
		DefaultConfig: defaultConfig,
	}, nil
}

// For safety: ensure JSON-serializable (avoid map[any]any leakage)
func mustMarshal(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
