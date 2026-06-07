package model

import "time"

// ProtocolRegistry 协议注册表数据库模型
// 存储所有已注册的协议信息（包括插件和内置协议）
type ProtocolRegistry struct {
	ID               uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	Name             string    `gorm:"uniqueIndex;size:64;not null" json:"name"`                 // 协议名称
	Version          string    `gorm:"size:32;not null" json:"version"`                          // 协议版本
	DeviceType       string    `gorm:"size:32;not null" json:"device_type"`                      // 设备类型 PLC/CNC/...
	Brand            string    `gorm:"size:32;not null" json:"brand"`                            // 品牌
	Source           string    `gorm:"size:16;not null;default:builtin" json:"source"`           // 来源：builtin / plugin
	PluginPath       string    `gorm:"size:512" json:"plugin_path"`                              // 插件可执行文件路径（仅 plugin 模式）
	ConnectionParams string    `gorm:"type:text;not null;default:'[]'" json:"connection_params"` // JSON: ConnectionParam 数组
	Models           string    `gorm:"type:text;not null;default:'[]'" json:"models"`            // JSON: 支持的设备型号列表
	Enabled          bool      `gorm:"not null;default:true" json:"enabled"`                     // 是否启用
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func (ProtocolRegistry) TableName() string {
	return "protocol_registry"
}
