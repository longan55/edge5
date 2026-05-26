package repository

import (
	"edge5/internal/model"

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

func (r *MQTTConfigRepository) Get() (*model.MQTTConfig, error) {
	var config model.MQTTConfig
	err := r.db.First(&config).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &config, nil
}

func (r *MQTTConfigRepository) Create(config *model.MQTTConfig) error {
	return r.db.Create(config).Error
}

func (r *MQTTConfigRepository) Update(config *model.MQTTConfig) error {
	return r.db.Save(config).Error
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
