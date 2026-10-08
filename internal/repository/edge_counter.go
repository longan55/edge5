package repository

import (
	"edge5/internal/model"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// EdgeCounterRepository 边缘计算计数器日累计记录的持久化访问层。
type EdgeCounterRepository struct {
	db     *gorm.DB
	logger *zap.Logger
}

func NewEdgeCounterRepository(db *gorm.DB, logger *zap.Logger) *EdgeCounterRepository {
	return &EdgeCounterRepository{db: db, logger: logger}
}

// UpsertIncrement 增量更新当日计数：行存在则 count += delta，不存在则插入。
// 使用 ON CONFLICT 原子 upsert，避免 read-then-write 竞态。
func (r *EdgeCounterRepository) UpsertIncrement(taskID uint64, ruleName, deviceSn, date string, delta int64) error {
	row := &model.EdgeCounterDaily{
		TaskID:   taskID,
		RuleName: ruleName,
		DeviceSn: deviceSn,
		Date:     date,
		Count:    delta,
	}
	return r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "task_id"},
			{Name: "rule_name"},
			{Name: "date"},
		},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"count":      gorm.Expr("count + ?", delta),
			"device_sn":  deviceSn,
			"updated_at": gorm.Expr("CURRENT_TIMESTAMP"),
		}),
	}).Create(row).Error
}

// LoadTodayCounts 加载某任务当日全部规则的计数，用于启动时恢复内存状态。
func (r *EdgeCounterRepository) LoadTodayCounts(taskID uint64, date string) (map[string]int64, error) {
	var rows []model.EdgeCounterDaily
	if err := r.db.Where("task_id = ? AND date = ?", taskID, date).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(rows))
	for _, row := range rows {
		out[row.RuleName] = row.Count
	}
	return out, nil
}
