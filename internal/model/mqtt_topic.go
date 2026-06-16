package model

type MQTTTopicTemplate struct {
	ID          uint64 `gorm:"primaryKey;autoIncrement" json:"id"`
	Key         string `gorm:"size:64;uniqueIndex;not null" json:"key"`
	DisplayName string `gorm:"size:128;not null" json:"display_name"`
	Prefix      string `gorm:"size:64;default:'/aixot'" json:"prefix"`
	Direction   string `gorm:"size:16;not null" json:"direction"`
	Path        string `gorm:"size:512;not null" json:"path"`
	CustomPart  string `gorm:"size:256" json:"custom_part"`
	IsDefault   bool   `gorm:"default:false" json:"is_default"`
	Sort        int    `gorm:"default:0" json:"sort"`
}

func (MQTTTopicTemplate) TableName() string {
	return "mqtt_topic_template"
}

type MQTTTopicConfig struct {
	ID          uint64 `gorm:"primaryKey;autoIncrement" json:"id"`
	Prefix      string `gorm:"size:64;default:'/aixot'" json:"prefix"`
	UpKeyword   string `gorm:"size:32;default:'up'" json:"up_keyword"`
	DownKeyword string `gorm:"size:32;default:'down'" json:"down_keyword"`
	ShowDirection bool `gorm:"default:true" json:"show_direction"`
	GatewaySN   string `gorm:"size:64;uniqueIndex;not null" json:"gateway_sn"`
	CreatedAt   int64  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   int64  `gorm:"autoUpdateTime" json:"updated_at"`
}

func (MQTTTopicConfig) TableName() string {
	return "mqtt_topic_config"
}
