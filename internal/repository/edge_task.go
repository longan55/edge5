package repository

import (
	"edge5/internal/model"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

type EdgeTaskRepository struct {
	db     *gorm.DB
	logger *zap.Logger
}

func NewEdgeTaskRepository(db *gorm.DB, logger *zap.Logger) *EdgeTaskRepository {
	return &EdgeTaskRepository{db: db, logger: logger}
}

func (r *EdgeTaskRepository) Create(task *model.EdgeTask) error {
	return r.db.Create(task).Error
}

func (r *EdgeTaskRepository) Update(task *model.EdgeTask) error {
	return r.db.Save(task).Error
}

func (r *EdgeTaskRepository) Delete(id uint64) error {
	return r.db.Delete(&model.EdgeTask{}, id).Error
}

func (r *EdgeTaskRepository) DeleteBatch(ids []uint64) error {
	return r.db.Delete(&model.EdgeTask{}, ids).Error
}

func (r *EdgeTaskRepository) GetByID(id uint64) (*model.EdgeTask, error) {
	var task model.EdgeTask
	if err := r.db.First(&task, id).Error; err != nil {
		return nil, err
	}
	return &task, nil
}

func (r *EdgeTaskRepository) List(page, pageSize int, name string) ([]model.EdgeTask, int64, error) {
	var tasks []model.EdgeTask
	var total int64
	query := r.db.Model(&model.EdgeTask{})
	if name != "" {
		query = query.Where("name LIKE ?", "%"+name+"%")
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	if err := query.Order("id ASC").Offset(offset).Limit(pageSize).Find(&tasks).Error; err != nil {
		return nil, 0, err
	}
	return tasks, total, nil
}

func (r *EdgeTaskRepository) ListAll() ([]model.EdgeTask, error) {
	var tasks []model.EdgeTask
	err := r.db.Order("id ASC").Find(&tasks).Error
	return tasks, err
}

func (r *EdgeTaskRepository) ListByDeviceID(deviceID uint64) ([]model.EdgeTask, error) {
	var tasks []model.EdgeTask
	err := r.db.Where("device_id = ?", deviceID).Find(&tasks).Error
	return tasks, err
}

// AutoMigrate 自动迁移表结构
func (r *EdgeTaskRepository) AutoMigrate() error {
	if err := r.db.AutoMigrate(&model.EdgeTask{}, &model.EdgeCounterDaily{}); err != nil {
		return err
	}
	// 老库迁移：将 ReadInterval 复制到 SampleInterval（仅当 SampleInterval=0）；
	// 若老库行未设 ReportInterval，默认给 10s 匹配旧行为
	if err := r.db.Exec(`UPDATE edge_tasks SET sample_interval = read_interval WHERE sample_interval = 0 AND read_interval > 0`).Error; err != nil {
		r.logger.Warn("迁移 sample_interval 失败", zap.Error(err))
	}
	if err := r.db.Exec(`UPDATE edge_tasks SET report_interval = 10 WHERE report_interval = 0 AND sample_interval > 0`).Error; err != nil {
		r.logger.Warn("迁移 report_interval 失败", zap.Error(err))
	}
	return nil
}
