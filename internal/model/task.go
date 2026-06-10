package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// TaskCommand 采集任务中的单条读取参数
type TaskCommand struct {
	Name      string `json:"name"`
	Address   string `json:"address"`
	Offset    int    `json:"offset"`
	ParseType string `json:"parseType"`
}

// TaskCommands TaskCommand 切片，支持 GORM 自定义序列化
type TaskCommands []TaskCommand

// Scan 实现 sql.Scanner 接口，从数据库读取 JSON 字符串
func (c *TaskCommands) Scan(value any) error {
	if value == nil {
		*c = TaskCommands{}
		return nil
	}
	switch v := value.(type) {
	case []byte:
		return json.Unmarshal(v, c)
	case string:
		return json.Unmarshal([]byte(v), c)
	default:
		return fmt.Errorf("unsupported type for TaskCommands: %T", value)
	}
}

// Value 实现 driver.Valuer 接口，写入数据库时序列化为 JSON 字符串
func (c TaskCommands) Value() (driver.Value, error) {
	if c == nil {
		return "[]", nil
	}
	return json.Marshal(c)
}

// Task 采集任务
type Task struct {
	BaseModel
	Name         string       `gorm:"size:128;not null" json:"name"`
	DeviceID     uint64       `gorm:"not null;index" json:"deviceId"`
	UpTopic      string       `gorm:"size:256" json:"upTopic"`
	ReadInterval int          `gorm:"default:10" json:"readInterval"`
	State        bool         `gorm:"default:false" json:"state"`
	Commands     TaskCommands `gorm:"type:text;serializer:json" json:"commands"`
}

func (Task) TableName() string {
	return "tasks"
}
