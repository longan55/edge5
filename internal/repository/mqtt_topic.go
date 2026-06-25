package repository

import (
	"edge5/internal/model"

	"gorm.io/gorm"
)

type MQTTTopicRepository struct {
	db *gorm.DB
}

func NewMQTTTopicRepository(db *gorm.DB) *MQTTTopicRepository {
	return &MQTTTopicRepository{db: db}
}

func (r *MQTTTopicRepository) List() ([]*model.MQTTTopicTemplate, error) {
	var topics []*model.MQTTTopicTemplate
	err := r.db.Order("sort asc").Find(&topics).Error
	return topics, err
}

func (r *MQTTTopicRepository) GetByKey(key string) (*model.MQTTTopicTemplate, error) {
	var topic model.MQTTTopicTemplate
	err := r.db.Where("key = ?", key).First(&topic).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &topic, err
}

func (r *MQTTTopicRepository) BatchSave(topics []*model.MQTTTopicTemplate) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		for _, topic := range topics {
			var existing model.MQTTTopicTemplate
			err := tx.Where("key = ?", topic.Key).First(&existing).Error
			if err == gorm.ErrRecordNotFound {
				if err := tx.Create(topic).Error; err != nil {
					return err
				}
			} else if err != nil {
				return err
			} else {
				topic.ID = existing.ID
				if err := tx.Save(topic).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (r *MQTTTopicRepository) ResetToDefaults() error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&model.MQTTTopicTemplate{}).Error; err != nil {
			return err
		}
		defaults := GetDefaultTopics()
		for _, t := range defaults {
			if err := tx.Create(t).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *MQTTTopicRepository) GetConfig(gatewaySN string) (*model.MQTTTopicConfig, error) {
	var cfg model.MQTTTopicConfig
	err := r.db.Where("gateway_sn = ?", gatewaySN).First(&cfg).Error
	if err == gorm.ErrRecordNotFound {
		return &model.MQTTTopicConfig{
			Prefix:        "/aixot",
			UpKeyword:     "up",
			DownKeyword:   "down",
			ShowDirection: true,
			GatewaySN:     gatewaySN,
		}, nil
	}
	return &cfg, err
}

func (r *MQTTTopicRepository) SaveConfig(cfg *model.MQTTTopicConfig) error {
	var existing model.MQTTTopicConfig
	err := r.db.Where("gateway_sn = ?", cfg.GatewaySN).First(&existing).Error
	if err == gorm.ErrRecordNotFound {
		return r.db.Create(cfg).Error
	} else if err != nil {
		return err
	}
	cfg.ID = existing.ID
	return r.db.Save(cfg).Error
}

func (r *MQTTTopicRepository) ResetConfig(gatewaySN string) error {
	defaultCfg := &model.MQTTTopicConfig{
		Prefix:        "/aixot",
		UpKeyword:     "up",
		DownKeyword:   "down",
		ShowDirection: true,
		GatewaySN:     gatewaySN,
	}
	return r.SaveConfig(defaultCfg)
}

func GetDefaultTopics() []*model.MQTTTopicTemplate {
	return []*model.MQTTTopicTemplate{
		{Key: "register_up", DisplayName: "注册", Direction: "up", Path: "gateway/register", IsDefault: true, Sort: 1},
		{Key: "register_down_ack", DisplayName: "注册响应", Direction: "down", Path: "gateway/register/ack", IsDefault: true, Sort: 2},
		{Key: "heartbeat_up", DisplayName: "心跳", Direction: "up", Path: "{gatewaySn}/heartbeat", IsDefault: true, Sort: 3},
		{Key: "gateway_status_up", DisplayName: "网关状态", Direction: "up", Path: "{gatewaySn}/properties", IsDefault: true, Sort: 4},
		{Key: "gateway_cmd_down", DisplayName: "网关指令", Direction: "down", Path: "{gatewaySn}/command", IsDefault: true, Sort: 5},
		{Key: "cmd_reply_up", DisplayName: "指令响应", Direction: "up", Path: "{gatewaySn}/command/reply", IsDefault: true, Sort: 6},
		{Key: "device_register_up", DisplayName: "设备注册", Direction: "up", Path: "{gatewaySn}/device/register", IsDefault: true, Sort: 7},
		{Key: "device_register_down_ack", DisplayName: "设备注册响应", Direction: "down", Path: "{gatewaySn}/device/register/ack", IsDefault: true, Sort: 8},
		{Key: "device_data_up", DisplayName: "设备数据上报", Direction: "up", Path: "{gatewaySn}/{deviceSn}/data", IsDefault: true, Sort: 9},
		{Key: "device_cmd_down", DisplayName: "设备指令", Direction: "down", Path: "{gatewaySn}/{deviceSn}/command", IsDefault: true, Sort: 10},
		{Key: "device_cmd_reply_up", DisplayName: "设备指令响应", Direction: "up", Path: "{gatewaySn}/{deviceSn}/command/reply", IsDefault: true, Sort: 11},
	}
}
