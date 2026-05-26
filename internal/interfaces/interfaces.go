package interfaces

import (
	"edge5/internal/model"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

type Logger interface {
	Info(msg string, fields ...zap.Field)
	Error(msg string, fields ...zap.Field)
	Warn(msg string, fields ...zap.Field)
}

type QuitTask interface {
	RegisterQuitTask(f func() error, content string, order int)
}

type DB interface {
	Create(value interface{}) error
	Save(value interface{}) error
	Delete(value interface{}, conditions ...interface{}) error
	First(dest interface{}, conditions ...interface{}) error
	Where(query interface{}, args ...interface{}) *gorm.DB
	Preload(query string, args ...interface{}) *gorm.DB
	Offset(offset int) *gorm.DB
	Limit(limit int) *gorm.DB
	Order(clause string) *gorm.DB
	Count(count *int64) error
	Model(value interface{}) *gorm.DB
	Joins(query string, args ...interface{}) *gorm.DB
	Transaction(fc func(tx *gorm.DB) error) error
}

type UserRepository interface {
	Create(user *model.User) error
	Update(user *model.User) error
	Delete(id uint64) error
	GetByID(id uint64) (*model.User, error)
	GetByUsername(username string) (*model.User, error)
	List(page, pageSize int) ([]*model.User, int64, error)
}

type MQTTConfigRepository interface {
	Get() (*model.MQTTConfig, error)
	Create(config *model.MQTTConfig) error
	Update(config *model.MQTTConfig) error
}

type DeviceRepository interface {
	Create(device *model.Device) error
	Update(device *model.Device) error
	Delete(id uint64) error
	GetByID(id uint64) (*model.Device, error)
	GetBySn(sn string) (*model.Device, error)
	List(page, pageSize int, deviceType, brand string) ([]*model.Device, int64, error)
}
