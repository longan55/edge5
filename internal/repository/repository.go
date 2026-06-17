package repository

import (
	"edge5/internal/model"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

type UserRepository struct {
	db     *gorm.DB
	logger *zap.Logger
}

func NewUserRepository(db *gorm.DB, logger *zap.Logger) *UserRepository {
	return &UserRepository{db: db, logger: logger}
}

func (r *UserRepository) Create(user *model.User) error {
	return r.db.Create(user).Error
}

func (r *UserRepository) Update(user *model.User) error {
	return r.db.Save(user).Error
}

func (r *UserRepository) Delete(id uint64) error {
	return r.db.Delete(&model.User{}, id).Error
}

func (r *UserRepository) GetByID(id uint64) (*model.User, error) {
	var user model.User
	err := r.db.Preload("Role").First(&user, id).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *UserRepository) GetByUsername(username string) (*model.User, error) {
	var user model.User
	err := r.db.Preload("Role").Where("username = ?", username).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *UserRepository) List(page, pageSize int) ([]*model.User, int64, error) {
	var users []*model.User
	var total int64

	offset := (page - 1) * pageSize
	err := r.db.Model(&model.User{}).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = r.db.Preload("Role").Offset(offset).Limit(pageSize).Find(&users).Error
	return users, total, err
}

type MQTTConfigRepository struct {
	db *gorm.DB
}

func NewMQTTConfigRepository(db *gorm.DB) *MQTTConfigRepository {
	return &MQTTConfigRepository{db: db}
}

// Get 只返回 mqtt_config 表的第一条（按 id 升序）
func (r *MQTTConfigRepository) Get() (*model.MQTTConfig, error) {
	var cfg model.MQTTConfig
	err := r.db.Order("id asc").First(&cfg).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &cfg, nil
}

// Create 确保表内最终只保留一条记录
func (r *MQTTConfigRepository) Create(cfg *model.MQTTConfig) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// 先清空，避免产生多行
		if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Where("1 = 1").Delete(&model.MQTTConfig{}).Error; err != nil {
			return err
		}
		return tx.Create(cfg).Error
	})
}

// Update 确保 mqtt_config 只保留第一条记录：覆盖第一条并删除其余行
// 注意：保留原有记录的 registered 字段值，避免更新配置时丢失注册状态
func (r *MQTTConfigRepository) Update(cfg *model.MQTTConfig) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var first model.MQTTConfig
		err := tx.Order("id asc").First(&first).Error
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Where("1 = 1").Delete(&model.MQTTConfig{}).Error; err != nil {
					return err
				}
				return tx.Create(cfg).Error
			}
			return err
		}

		// 保留原有记录的注册状态
		cfg.Registered = first.Registered

		// 覆盖第一条
		cfg.ID = first.ID
		if err := tx.Save(cfg).Error; err != nil {
			return err
		}

		// 删除其它行
		if err := tx.Where("id <> ?", first.ID).Delete(&model.MQTTConfig{}).Error; err != nil {
			return err
		}
		return nil
	})
}

// UpdateRegistered 单独更新 registered 字段（使用原生 SQL 确保兼容 SQLite）
func (r *MQTTConfigRepository) UpdateRegistered(registered bool) error {
	val := 0
	if registered {
		val = 1
	}
	return r.db.Exec("UPDATE mqtt_config SET registered = ? WHERE id = (SELECT id FROM mqtt_config ORDER BY id ASC LIMIT 1)", val).Error
}

type DeviceRepository struct {
	db *gorm.DB
}

func NewDeviceRepository(db *gorm.DB) *DeviceRepository {
	return &DeviceRepository{db: db}
}

func (r *DeviceRepository) Create(device *model.Device) error {
	return r.db.Create(device).Error
}

func (r *DeviceRepository) Update(device *model.Device) error {
	return r.db.Save(device).Error
}

func (r *DeviceRepository) Delete(id uint64) error {
	return r.db.Delete(&model.Device{}, id).Error
}

func (r *DeviceRepository) GetByID(id uint64) (*model.Device, error) {
	var device model.Device
	err := r.db.First(&device, id).Error
	if err != nil {
		return nil, err
	}
	return &device, nil
}

func (r *DeviceRepository) GetBySn(sn string) (*model.Device, error) {
	var device model.Device
	err := r.db.Where("device_sn = ?", sn).First(&device).Error
	if err != nil {
		return nil, err
	}
	return &device, nil
}

func (r *DeviceRepository) List(page, pageSize int, deviceType, brand string) ([]*model.Device, int64, error) {
	var devices []*model.Device
	var total int64

	query := r.db.Model(&model.Device{})

	if deviceType != "" {
		query = query.Where("device_type = ?", deviceType)
	}
	if brand != "" {
		query = query.Where("brand = ?", brand)
	}

	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	err = query.Offset(offset).Limit(pageSize).Find(&devices).Error
	return devices, total, err
}

// DeviceStatusRepository provides access to device online status.
type DeviceStatusRepository struct {
	db *gorm.DB
}

func NewDeviceStatusRepository(db *gorm.DB) *DeviceStatusRepository {
	return &DeviceStatusRepository{db: db}
}

func (r *DeviceStatusRepository) GetByDeviceID(deviceID uint64) (*model.DeviceStatus, error) {
	var status model.DeviceStatus
	err := r.db.Where("device_id = ?", deviceID).First(&status).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &status, nil
}

// UpsertByDeviceID 如果存在则更新，否则创建一条
func (r *DeviceStatusRepository) UpsertByDeviceID(deviceID uint64, online bool, lastHeartbeat time.Time, message string) error {
	// sqlite/postgres 都可用：先查，再 Save/Create
	status, err := r.GetByDeviceID(deviceID)
	if err != nil {
		return err
	}

	if status == nil {
		status = &model.DeviceStatus{
			DeviceID:      deviceID,
			Online:        online,
			LastHeartbeat: lastHeartbeat,
			Message:       message,
		}
		return r.db.Create(status).Error
	}

	status.Online = online
	status.LastHeartbeat = lastHeartbeat
	status.Message = message
	return r.db.Save(status).Error
}
