package report

import (
	"crypto/rand"
	"edge5/internal/pkg/cache"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// boltCacheAdapter 将 cache.BoltCache 适配为 report.Cache 接口
type boltCacheAdapter struct {
	bolt *cache.BoltCache
}

// NewBoltCacheAdapter 创建 BoltCache 适配器
func NewBoltCacheAdapter(boltCache *cache.BoltCache) Cache {
	return &boltCacheAdapter{bolt: boltCache}
}

// Push 插入一条缓存
func (a *boltCacheAdapter) Push(data []byte) error {
	if a.bolt == nil {
		return ErrCacheNotSet
	}

	msg := &cache.CacheMessage{
		ID:          generateID(),
		Payload:     data,
		RetryCount:  0,
		CreatedAt:   time.Now().Unix(),
		NextRetryAt: 0,
	}

	return a.bolt.Push(msg)
}

// Pop 取出并删除最早的一条
func (a *boltCacheAdapter) Pop() (*CachedItem, error) {
	if a.bolt == nil {
		return nil, ErrCacheNotSet
	}

	msg, err := a.bolt.Pop()
	if err != nil {
		return nil, err
	}
	if msg == nil {
		return nil, nil
	}

	return &CachedItem{
		ID:          msg.ID,
		Payload:     msg.Payload,
		RetryCount:  msg.RetryCount,
		CreatedAt:   msg.CreatedAt,
		NextRetryAt: msg.NextRetryAt,
	}, nil
}

// Peek 查看最早的一条（不删除）
func (a *boltCacheAdapter) Peek() (*CachedItem, error) {
	if a.bolt == nil {
		return nil, ErrCacheNotSet
	}

	msg, err := a.bolt.Peek()
	if err != nil {
		return nil, err
	}
	if msg == nil {
		return nil, nil
	}

	return &CachedItem{
		ID:          msg.ID,
		Payload:     msg.Payload,
		RetryCount:  msg.RetryCount,
		CreatedAt:   msg.CreatedAt,
		NextRetryAt: msg.NextRetryAt,
	}, nil
}

// GetAll 获取所有缓存
func (a *boltCacheAdapter) GetAll() ([]*CachedItem, error) {
	if a.bolt == nil {
		return nil, ErrCacheNotSet
	}

	messages, err := a.bolt.GetAll()
	if err != nil {
		return nil, err
	}

	items := make([]*CachedItem, 0, len(messages))
	for _, msg := range messages {
		items = append(items, &CachedItem{
			ID:          msg.ID,
			Payload:     msg.Payload,
			RetryCount:  msg.RetryCount,
			CreatedAt:   msg.CreatedAt,
			NextRetryAt: msg.NextRetryAt,
		})
	}

	return items, nil
}

// Delete 删除指定缓存
func (a *boltCacheAdapter) Delete(id string) error {
	if a.bolt == nil {
		return ErrCacheNotSet
	}

	return a.bolt.Delete(id)
}

// Update 更新缓存
func (a *boltCacheAdapter) Update(item *CachedItem) error {
	if a.bolt == nil {
		return ErrCacheNotSet
	}

	msg := &cache.CacheMessage{
		ID:          item.ID,
		Payload:     item.Payload,
		RetryCount:  item.RetryCount,
		CreatedAt:   item.CreatedAt,
		NextRetryAt: item.NextRetryAt,
	}

	return a.bolt.Update(msg)
}

// Size 返回缓存数量
func (a *boltCacheAdapter) Size() int {
	if a.bolt == nil {
		return 0
	}

	return a.bolt.Size()
}

// Clear 清空缓存
func (a *boltCacheAdapter) Clear() error {
	if a.bolt == nil {
		return ErrCacheNotSet
	}

	return a.bolt.Clear()
}

// Close 关闭缓存
func (a *boltCacheAdapter) Close() error {
	if a.bolt == nil {
		return ErrCacheNotSet
	}

	return a.bolt.Close()
}

// 确保类型安全
var _ Cache = (*boltCacheAdapter)(nil)

// generateID 生成全局唯一 ID
func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	// 设置 version 4 UUID 标记位
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return hex.EncodeToString(b[:4]) + "-" +
		hex.EncodeToString(b[4:6]) + "-" +
		hex.EncodeToString(b[6:8]) + "-" +
		hex.EncodeToString(b[8:10]) + "-" +
		hex.EncodeToString(b[10:])
}

// 反序列化辅助 — 确保 JSON 兼容
func marshalItem(item *CachedItem) ([]byte, error) {
	return json.Marshal(item)
}

func unmarshalItem(data []byte) (*CachedItem, error) {
	var item CachedItem
	if err := json.Unmarshal(data, &item); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cached item: %w", err)
	}
	return &item, nil
}
