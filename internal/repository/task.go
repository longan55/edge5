package repository

import (
	"edge5/internal/model"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

type TaskRepository struct {
	db     *gorm.DB
	logger *zap.Logger
}

func NewTaskRepository(db *gorm.DB, logger *zap.Logger) *TaskRepository {
	return &TaskRepository{db: db, logger: logger}
}

func (r *TaskRepository) Create(task *model.Task) error {
	return r.db.Create(task).Error
}

func (r *TaskRepository) Update(task *model.Task) error {
	return r.db.Save(task).Error
}

func (r *TaskRepository) Delete(id uint64) error {
	return r.db.Delete(&model.Task{}, id).Error
}

func (r *TaskRepository) DeleteBatch(ids []uint64) error {
	return r.db.Delete(&model.Task{}, ids).Error
}

func (r *TaskRepository) GetByID(id uint64) (*model.Task, error) {
	var task model.Task
	err := r.db.First(&task, id).Error
	if err != nil {
		return nil, err
	}
	return &task, nil
}

func (r *TaskRepository) List(page, pageSize int, name string) ([]model.Task, int64, error) {
	var tasks []model.Task
	var total int64
	query := r.db.Model(&model.Task{})
	if name != "" {
		query = query.Where("name LIKE ?", "%"+name+"%")
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	if err := query.Order("id DESC").Offset(offset).Limit(pageSize).Find(&tasks).Error; err != nil {
		return nil, 0, err
	}
	return tasks, total, nil
}

func (r *TaskRepository) ListByDeviceID(deviceID uint64) ([]model.Task, error) {
	var tasks []model.Task
	err := r.db.Where("device_id = ?", deviceID).Find(&tasks).Error
	return tasks, err
}

// AutoMigrate 自动迁移表结构
func (r *TaskRepository) AutoMigrate() error {
	return r.db.AutoMigrate(&model.Task{})
}
