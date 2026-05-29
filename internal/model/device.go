package model

import (
	"encoding/json"
	"time"
)

type Device struct {
	ID         uint64          `gorm:"primaryKey;autoIncrement" json:"id"`
	DeviceSn   string          `gorm:"uniqueIndex;size:64;not null" json:"device_sn"`
	DeviceName string          `gorm:"size:128;not null" json:"device_name"`
	DeviceType string          `gorm:"size:32;not null" json:"device_type"`
	Brand      string          `gorm:"size:32;not null" json:"brand"`
	Protocol   string          `gorm:"size:32;not null" json:"protocol"`
	Status     int8            `gorm:"default:1" json:"status"`
	Config     json.RawMessage `gorm:"type:jsonb" json:"config"`
	PluginName string          `gorm:"size:64" json:"plugin_name"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`

	// runtime joins（用于列表展示，不建表）
	Online        bool      `gorm:"-" json:"online"`
	LastHeartbeat time.Time `gorm:"-" json:"last_heartbeat"`
	Message       string    `gorm:"-" json:"message"`
}

func (Device) TableName() string {
	return "device"
}

type DeviceConfig struct {
	Timeout    int                    `json:"timeout"`
	Retry      int                    `json:"retry"`
	IP         string                 `json:"ip,omitempty"`
	Port       int                    `json:"port,omitempty"`
	SerialPort string                 `json:"serial_port,omitempty"`
	BaudRate   int                    `json:"baud_rate,omitempty"`
	DataBits   int                    `json:"data_bits,omitempty"`
	Parity     string                 `json:"parity,omitempty"`
	StopBits   int                    `json:"stop_bits,omitempty"`
	Extra      map[string]interface{} `json:"extra,omitempty"`
}

type DeviceStatus struct {
	ID            uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	DeviceID      uint64    `gorm:"uniqueIndex;not null" json:"device_id"`
	Online        bool      `gorm:"default:false" json:"online"`
	LastHeartbeat time.Time `json:"last_heartbeat"`
	Message       string    `gorm:"size:512" json:"message"`
}

func (DeviceStatus) TableName() string {
	return "device_status"
}

type DeviceData struct {
	DeviceSn  string                 `json:"device_sn"`
	Type      string                 `json:"device_type"`
	Data      map[string]interface{} `json:"data"`
	Timestamp int64                  `json:"timestamp"`
}
