package report

// CachedItem 缓存的一条数据
type CachedItem struct {
	// ID 缓存消息唯一标识
	ID string `json:"id"`
	// Payload 原始上报数据
	Payload []byte `json:"payload"`
	// RetryCount 已重试次数
	RetryCount int `json:"retry_count"`
	// CreatedAt 创建时间戳（秒）
	CreatedAt int64 `json:"created_at"`
	// NextRetryAt 下次重试时间戳（秒）
	NextRetryAt int64 `json:"next_retry_at"`
}

// Cache 缓存接口
// 上报框架使用此接口存取缓存数据
type Cache interface {
	// Push 写入一条缓存
	Push(data []byte) error
	// Pop 取出并删除最早的一条缓存
	Pop() (*CachedItem, error)
	// Peek 查看最早的一条缓存（不删除）
	Peek() (*CachedItem, error)
	// GetAll 获取所有缓存
	GetAll() ([]*CachedItem, error)
	// Delete 删除指定缓存
	Delete(id string) error
	// Update 更新缓存（如更新重试次数）
	Update(item *CachedItem) error
	// Size 返回缓存数量
	Size() int
	// Clear 清空所有缓存
	Clear() error
	// Close 关闭缓存
	Close() error
}
