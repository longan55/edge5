package model

// EdgeCounterDaily 边缘计算计数器日累计记录。
// 每条规则每天一行，count 由引擎在每次 Evaluate 检测到计数变化时增量 upsert。
// (task_id, rule_name, date) 联合唯一，保证 UpsertIncrement 的 ON CONFLICT 能命中。
type EdgeCounterDaily struct {
	BaseModel
	TaskID   uint64 `gorm:"not null;uniqueIndex:idx_edge_counter_daily,priority:1" json:"taskId"`
	RuleName string `gorm:"size:128;not null;uniqueIndex:idx_edge_counter_daily,priority:2" json:"ruleName"`
	DeviceSn string `gorm:"size:64;not null" json:"deviceSn"`
	// Date 本地日期 YYYY-MM-DD，用于跨日清零与新日开始新行
	Date  string `gorm:"size:10;not null;uniqueIndex:idx_edge_counter_daily,priority:3" json:"date"`
	Count int64  `gorm:"not null;default:0" json:"count"`
}

func (EdgeCounterDaily) TableName() string {
	return "edge_counter_daily"
}
