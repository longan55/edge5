package model

import (
	"encoding/json"
	"time"
)

type MQTTConfig struct {
	ID         uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	Broker     string    `gorm:"size:256;not null" json:"broker"`
	Port       int       `gorm:"not null" json:"port"`
	Username   string    `gorm:"size:64" json:"username"`
	Password   string    `gorm:"size:128" json:"password"`
	ClientID   string    `gorm:"size:64;not null" json:"client_id"`
	KeepAlive  int       `gorm:"default:60" json:"keep_alive"`
	QoS        int8      `gorm:"default:1" json:"qos"`
	Status     int8      `gorm:"default:0" json:"status"`
	GatewaySN  string    `gorm:"size:64;uniqueIndex;not null" json:"gateway_sn"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (MQTTConfig) TableName() string {
	return "mqtt_config"
}

type MQTTMessage struct {
	Version    string          `json:"version"`
	GatewaySn  string          `json:"gatewaySn"`
	Timestamp  int64           `json:"timestamp"`
	RequestID  string          `json:"requestId,omitempty"`
	DeviceType string          `json:"deviceType,omitempty"`
	Payload    json.RawMessage `json:"payload"`
}

type MQTTCommandRequest struct {
	GatewaySn  string            `json:"gatewaySn"`
	Timestamp  int64             `json:"timestamp"`
	RequestID  string            `json:"requestId"`
	DeviceType string            `json:"deviceType,omitempty"`
	Payload    CommandPayload    `json:"payload"`
}

type CommandPayload struct {
	Command string                 `json:"command"`
	Params  map[string]interface{} `json:"params,omitempty"`
}

type CommandResponse struct {
	GatewaySn  string            `json:"gatewaySn"`
	Timestamp  int64             `json:"timestamp"`
	RequestID  string            `json:"requestId"`
	Payload    ResponsePayload    `json:"payload"`
}

type ResponsePayload struct {
	Command string                 `json:"command"`
	Result  int                    `json:"result"`
	Message string                 `json:"message"`
	Data    interface{}            `json:"data,omitempty"`
}
