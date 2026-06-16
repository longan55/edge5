package model

import (
	"encoding/json"
	"time"
)

type MQTTConfig struct {
	ID                  uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	Broker              string    `gorm:"size:256;not null" json:"broker"`
	Protocol            string    `gorm:"size:16;default:'mqtt://'" json:"protocol"`
	Host                string    `gorm:"size:128" json:"host"`
	Port                int       `gorm:"not null" json:"port"`
	Username            string    `gorm:"size:64" json:"username"`
	Password            string    `gorm:"size:128" json:"password"`
	ClientID            string    `gorm:"size:64;not null" json:"client_id"`
	KeepAlive           int       `gorm:"default:60" json:"keep_alive"`
	QoS                 int8      `gorm:"default:1" json:"qos"`
	On                  bool      `gorm:"default:false" json:"on"`
	GatewaySN           string    `gorm:"size:64;uniqueIndex;not null" json:"gateway_sn"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`

	// SSL/TLS settings
	SSL                 bool      `gorm:"default:false" json:"ssl"`
	SSLVerify           bool      `gorm:"default:true" json:"ssl_verify"`
	ALPNTag             string    `gorm:"size:64" json:"alpn_tag"`
	CertType            string    `gorm:"size:32" json:"cert_type"`
	CAFile              string    `gorm:"size:512" json:"ca_file"`
	CertFile            string    `gorm:"size:512" json:"cert_file"`
	KeyFile             string    `gorm:"size:512" json:"key_file"`

	// Advanced settings
	Version             string    `gorm:"size:8;default:'5.0'" json:"version"`
	ConnectTimeout      int       `gorm:"default:10" json:"connect_timeout"`
	AutoReconnect       bool      `gorm:"default:true" json:"auto_reconnect"`
	ReconnectPeriod     int       `gorm:"default:4000" json:"reconnect_period"`
	CleanStart          bool      `gorm:"default:false" json:"clean_start"`
	SessionExpiry       int       `gorm:"default:7200" json:"session_expiry"`
	ReceiveMax          int       `gorm:"default:0" json:"receive_max"`
	MaxPacketSize       int       `gorm:"default:0" json:"max_packet_size"`
	TopicAliasMax       int       `gorm:"default:0" json:"topic_alias_max"`
	RequestResponseInfo bool      `gorm:"default:false" json:"request_response_info"`
	RequestProblemInfo  bool      `gorm:"default:false" json:"request_problem_info"`
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
	GatewaySn  string         `json:"gatewaySn"`
	Timestamp  int64          `json:"timestamp"`
	RequestID  string         `json:"requestId"`
	DeviceType string         `json:"deviceType,omitempty"`
	Payload    CommandPayload `json:"payload"`
}

type CommandPayload struct {
	Command string                 `json:"command"`
	Params  map[string]interface{} `json:"params,omitempty"`
}

type CommandResponse struct {
	GatewaySn string          `json:"gatewaySn"`
	Timestamp int64           `json:"timestamp"`
	RequestID string          `json:"requestId"`
	Payload   ResponsePayload `json:"payload"`
}

type ResponsePayload struct {
	Command string      `json:"command"`
	Result  int         `json:"result"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}
