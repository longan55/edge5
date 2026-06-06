package report

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
//  mockSender — 虚拟连接，可手动控制连接/断开和发送结果
// ---------------------------------------------------------------------------

type mockSender struct {
	name        string
	connected   atomic.Bool
	failCount   atomic.Int32 // 连续失败几次后再成功（-1 表示永远失败）
	sendCount   atomic.Int32 // 累计发送成功次数
	callSend    atomic.Int32 // 累计 Send 调用次数
	lastPayload []byte
	mu          sync.Mutex
}

func newMockSender(name string) *mockSender {
	s := &mockSender{name: name}
	s.connected.Store(true)
	return s
}

func (s *mockSender) Name() string { return s.name }

func (s *mockSender) IsConnected() bool { return s.connected.Load() }

// Connect 手动连接
func (s *mockSender) Connect() { s.connected.Store(true) }

// Disconnect 手动断开
func (s *mockSender) Disconnect() { s.connected.Store(false) }

// SetFailCount 设置连续失败次数
//   - 0: 每次 Send 都成功
//   - N: 前 N 次失败，第 N+1 次成功
//   - -1: 永远失败
func (s *mockSender) SetFailCount(n int) { s.failCount.Store(int32(n)) }

// Send 发送数据（mock）
func (s *mockSender) Send(ctx context.Context, topic string, qos byte, data []byte) error {
	s.callSend.Add(1)

	if !s.connected.Load() {
		return ErrNotConnected
	}

	fail := s.failCount.Load()
	if fail != 0 {
		if fail > 0 {
			s.failCount.Add(-1)
		}
		// fail < 0 时永远失败
		return ErrNotConnected
	}

	s.mu.Lock()
	s.lastPayload = append([]byte{}, data...)
	s.sendCount.Add(1)
	s.mu.Unlock()
	return nil
}

// LastPayload 返回最后一次发送的载荷
func (s *mockSender) LastPayload() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastPayload
}

// SendCount 返回成功发送次数
func (s *mockSender) SendCount() int {
	return int(s.sendCount.Load())
}

// CallCount 返回 Send 的总调用次数（含失败）
func (s *mockSender) CallCount() int {
	return int(s.callSend.Load())
}

// ---------------------------------------------------------------------------
//  mockCache — 内存缓存，用于替代 BoltCache
// ---------------------------------------------------------------------------

type mockCache struct {
	mu    sync.Mutex
	items []*CachedItem
}

func newMockCache() *mockCache {
	return &mockCache{items: make([]*CachedItem, 0)}
}

func (c *mockCache) Push(data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = append(c.items, &CachedItem{
		ID:      generateID(),
		Payload: append([]byte{}, data...),
	})
	return nil
}

func (c *mockCache) Pop() (*CachedItem, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.items) == 0 {
		return nil, nil
	}
	item := c.items[0]
	c.items = c.items[1:]
	return item, nil
}

func (c *mockCache) Peek() (*CachedItem, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.items) == 0 {
		return nil, nil
	}
	return c.items[0], nil
}

func (c *mockCache) GetAll() ([]*CachedItem, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]*CachedItem, len(c.items))
	for i, item := range c.items {
		result[i] = &CachedItem{
			ID:          item.ID,
			Payload:     append([]byte{}, item.Payload...),
			RetryCount:  item.RetryCount,
			CreatedAt:   item.CreatedAt,
			NextRetryAt: item.NextRetryAt,
		}
	}
	return result, nil
}

func (c *mockCache) Delete(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, item := range c.items {
		if item.ID == id {
			c.items = append(c.items[:i], c.items[i+1:]...)
			return nil
		}
	}
	return nil
}

func (c *mockCache) Update(item *CachedItem) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, existing := range c.items {
		if existing.ID == item.ID {
			c.items[i] = item
			return nil
		}
	}
	return nil
}

func (c *mockCache) Size() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}

func (c *mockCache) Clear() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = c.items[:0]
	return nil
}

func (c *mockCache) Close() error { return nil }

// ensure mockCache implements Cache
var _ Cache = (*mockCache)(nil)

// ---------------------------------------------------------------------------
//  测试用例
// ---------------------------------------------------------------------------

func TestReport_Success(t *testing.T) {
	sender := newMockSender("test")
	cache := newMockCache()
	logger := zap.NewNop()

	cfg := DefaultConfig("test/topic")
	cfg.CacheOnFailure = true
	cfg.RetryInterval = 1

	r := New(cfg, sender, cache, logger)
	defer r.Close()

	payload := []byte("hello-world")
	err := r.Report(context.Background(), payload)
	if err != nil {
		t.Fatalf("期望成功发送，但得到错误: %v", err)
	}

	if sender.SendCount() != 1 {
		t.Fatalf("期望发送1次，实际发送 %d 次", sender.SendCount())
	}

	lastPayload := sender.LastPayload()
	if string(lastPayload) != string(payload) {
		t.Fatalf("期望载荷 %q，实际载荷 %q", payload, lastPayload)
	}

	if cache.Size() != 0 {
		t.Fatalf("期望缓存为空，实际缓存 %d 条", cache.Size())
	}
}

func TestReport_Disconnected_CacheEnabled(t *testing.T) {
	sender := newMockSender("test")
	cache := newMockCache()
	logger := zap.NewNop()

	cfg := DefaultConfig("test/topic")
	cfg.CacheOnFailure = true
	cfg.RetryInterval = 1 // 1秒重试一次

	r := New(cfg, sender, cache, logger)
	defer r.Close()

	sender.Disconnect()

	payload := []byte("cached-when-disconnected")
	err := r.Report(context.Background(), payload)
	if err != nil {
		t.Fatalf("连接断开时 Report 不应返回错误（数据已缓存）: %v", err)
	}

	if cache.Size() != 1 {
		t.Fatalf("期望缓存1条，实际缓存 %d 条", cache.Size())
	}

	sender.Connect()

	// 等待重试循环发送缓存
	time.Sleep(1500 * time.Millisecond)

	if cache.Size() != 0 {
		t.Fatalf("重试后期望缓存清空，实际剩余 %d 条", cache.Size())
	}

	if sender.SendCount() != 1 {
		t.Fatalf("期望发送1次（重试），实际发送 %d 次", sender.SendCount())
	}
}

func TestReport_SendFailure_CachesData(t *testing.T) {
	sender := newMockSender("test")
	sender.SetFailCount(-1) // 永远发送失败
	cache := newMockCache()
	logger := zap.NewNop()

	cfg := DefaultConfig("test/topic")
	cfg.CacheOnFailure = true
	cfg.RetryInterval = 1

	r := New(cfg, sender, cache, logger)
	defer r.Close()

	payload := []byte("fail-and-cache")
	err := r.Report(context.Background(), payload)
	if err == nil {
		t.Fatal("期望 Report 返回错误（发送失败）")
	}

	if cache.Size() != 1 {
		t.Fatalf("期望缓存1条，实际缓存 %d 条", cache.Size())
	}
}

func TestReport_SendFailure_CacheDisabled(t *testing.T) {
	sender := newMockSender("test")
	sender.SetFailCount(-1) // 永远失败
	cache := newMockCache()
	logger := zap.NewNop()

	cfg := DefaultConfig("test/topic")
	cfg.CacheOnFailure = false // 关闭缓存

	r := New(cfg, sender, cache, logger)
	defer r.Close()

	payload := []byte("fail-no-cache")
	err := r.Report(context.Background(), payload)
	if err == nil {
		t.Fatal("期望 Report 返回错误（发送失败，缓存关闭）")
	}

	if cache.Size() != 0 {
		t.Fatalf("期望缓存为空（缓存已禁用），实际缓存 %d 条", cache.Size())
	}
}

func TestReport_Disconnected_CacheDisabled(t *testing.T) {
	sender := newMockSender("test")
	cache := newMockCache()
	logger := zap.NewNop()

	cfg := DefaultConfig("test/topic")
	cfg.CacheOnFailure = false // 关闭缓存

	r := New(cfg, sender, cache, logger)
	defer r.Close()

	sender.Disconnect()

	payload := []byte("no-cache")
	err := r.Report(context.Background(), payload)
	if err != ErrNotConnected {
		t.Fatalf("期望错误 %v，实际得到 %v", ErrNotConnected, err)
	}

	if cache.Size() != 0 {
		t.Fatalf("期望缓存为空（缓存已禁用），实际缓存 %d 条", cache.Size())
	}
}

func TestReport_MaxRetries_Discard(t *testing.T) {
	sender := newMockSender("test")
	sender.SetFailCount(-1) // 永远失败
	cache := newMockCache()
	logger := zap.NewNop()

	cfg := DefaultConfig("test/topic")
	cfg.CacheOnFailure = true
	cfg.RetryInterval = 1
	cfg.MaxRetries = 2 // 最多重试2次

	r := New(cfg, sender, cache, logger)
	defer r.Close()

	// 先写入一条缓存
	payload := []byte("max-retry-test")
	err := r.Report(context.Background(), payload)
	if err == nil {
		t.Fatal("期望 Report 返回错误")
	}

	if cache.Size() != 1 {
		t.Fatalf("期望缓存1条，实际缓存 %d 条", cache.Size())
	}

	// 重试循环会不断尝试发送，但 sender 永远失败
	// 每次重试 RetryCount 递增，达到 MaxRetries 后丢弃
	time.Sleep(3500 * time.Millisecond)

	// 此时消息应已被丢弃（重试次数达到上限）
	if cache.Size() != 0 {
		t.Fatalf("期望缓存为空（消息已被丢弃），实际缓存 %d 条", cache.Size())
	}
}

func TestReport_SenderNotSet(t *testing.T) {
	cache := newMockCache()
	logger := zap.NewNop()

	cfg := DefaultConfig("test/topic")
	r := New(cfg, nil, cache, logger)
	defer r.Close()

	err := r.Report(context.Background(), []byte("no-sender"))
	if err != ErrSenderNotSet {
		t.Fatalf("期望错误 %v，实际得到 %v", ErrSenderNotSet, err)
	}
}

func TestReport_RetryAfterReconnect(t *testing.T) {
	sender := newMockSender("test")
	cache := newMockCache()
	logger := zap.NewNop()

	cfg := DefaultConfig("test/topic")
	cfg.CacheOnFailure = true
	cfg.RetryInterval = 1 // 1秒重试一次

	r := New(cfg, sender, cache, logger)
	defer r.Close()

	// 断开连接并上报 -> 数据进入缓存
	sender.Disconnect()

	data1 := []byte("data-1")
	data2 := []byte("data-2")

	err1 := r.Report(context.Background(), data1)
	if err1 != nil {
		t.Fatalf("断开连接时上报不应返回错误: %v", err1)
	}

	err2 := r.Report(context.Background(), data2)
	if err2 != nil {
		t.Fatalf("断开连接时上报不应返回错误: %v", err2)
	}

	if cache.Size() != 2 {
		t.Fatalf("期望缓存2条，实际缓存 %d 条", cache.Size())
	}

	// 重新连接
	sender.Connect()

	// 等待重试
	time.Sleep(2000 * time.Millisecond)

	if cache.Size() != 0 {
		t.Fatalf("重试后期望缓存清空，实际剩余 %d 条", cache.Size())
	}

	if sender.SendCount() != 2 {
		t.Fatalf("期望发送2次（2条缓存各重试一次），实际发送 %d 次", sender.SendCount())
	}
}

func TestReport_CloseStopsRetry(t *testing.T) {
	sender := newMockSender("test")
	sender.SetFailCount(-1) // 永远失败
	cache := newMockCache()
	logger := zap.NewNop()

	cfg := DefaultConfig("test/topic")
	cfg.CacheOnFailure = true
	cfg.RetryInterval = 1

	r := New(cfg, sender, cache, logger)

	// 写入一条缓存
	_ = r.Report(context.Background(), []byte("close-test"))
	if cache.Size() != 1 {
		t.Fatalf("期望缓存1条，实际缓存 %d 条", cache.Size())
	}

	// 关闭上报器 -> 重试循环停止
	r.Close()

	// 等待一会儿，确认重试不再进行
	time.Sleep(2000 * time.Millisecond)

	// 缓存应还在（关闭后不重试
	if cache.Size() != 1 {
		t.Fatalf("关闭上报器后缓存不应被消费，实际缓存 %d 条", cache.Size())
	}
}

func TestReport_ConcurrentReports(t *testing.T) {
	sender := newMockSender("test")
	cache := newMockCache()
	logger := zap.NewNop()

	cfg := DefaultConfig("test/topic")
	cfg.CacheOnFailure = true

	r := New(cfg, sender, cache, logger)
	defer r.Close()

	// 并发上报 10 条数据
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			payload := []byte{byte(n)}
			_ = r.Report(context.Background(), payload)
		}(i)
	}
	wg.Wait()

	// 全部应成功发送
	if sender.SendCount() != 10 {
		t.Fatalf("期望发送10次，实际发送 %d 次", sender.SendCount())
	}
}

func TestReport_DisconnectedConcurrent(t *testing.T) {
	sender := newMockSender("test")
	cache := newMockCache()
	logger := zap.NewNop()

	cfg := DefaultConfig("test/topic")
	cfg.CacheOnFailure = true
	cfg.RetryInterval = 1

	r := New(cfg, sender, cache, logger)
	defer r.Close()

	sender.Disconnect()

	// 并发上报 5 条（都在断开连接的情况下）
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			payload := []byte{byte(n)}
			_ = r.Report(context.Background(), payload)
		}(i)
	}
	wg.Wait()

	if cache.Size() != 5 {
		t.Fatalf("期望缓存5条，实际缓存 %d 条", cache.Size())
	}

	// 重连并等待重试
	sender.Connect()
	time.Sleep(2000 * time.Millisecond)

	if cache.Size() != 0 {
		t.Fatalf("重试后期望缓存清空，实际剩余 %d 条", cache.Size())
	}
	if sender.SendCount() != 5 {
		t.Fatalf("期望发送5次（重试），实际发送 %d 次", sender.SendCount())
	}
}

func TestReport_RetryCountIncrements(t *testing.T) {
	sender := newMockSender("test")
	sender.SetFailCount(-1) // 永远失败
	cache := newMockCache()
	logger := zap.NewNop()

	cfg := DefaultConfig("test/topic")
	cfg.CacheOnFailure = true
	cfg.RetryInterval = 1
	cfg.MaxRetries = 3

	r := New(cfg, sender, cache, logger)
	defer r.Close()

	// 上报一条数据 -> 进入缓存
	_ = r.Report(context.Background(), []byte("retry-count"))

	// 等待重试循环至少运行两次
	time.Sleep(2500 * time.Millisecond)

	items, _ := cache.GetAll()
	if len(items) == 0 {
		t.Fatal("期望缓存中仍有数据")
	}

	if items[0].RetryCount == 0 {
		t.Fatal("期望 RetryCount 已递增，实际仍为 0")
	}

	t.Logf("缓存消息 RetryCount = %d (MaxRetries=%d)", items[0].RetryCount, cfg.MaxRetries)
}

func TestReport_NilCache(t *testing.T) {
	sender := newMockSender("test")
	logger := zap.NewNop()

	cfg := DefaultConfig("test/topic")
	cfg.CacheOnFailure = true

	r := New(cfg, sender, nil, logger)
	defer r.Close()

	err := r.Report(context.Background(), []byte("nil-cache"))
	if err != nil {
		t.Fatalf("期望发送成功: %v", err)
	}

	if sender.SendCount() != 1 {
		t.Fatalf("期望发送1次，实际 %d 次", sender.SendCount())
	}
}

func TestReport_NilCacheDisconnected(t *testing.T) {
	sender := newMockSender("test")
	logger := zap.NewNop()

	cfg := DefaultConfig("test/topic")
	cfg.CacheOnFailure = true

	r := New(cfg, sender, nil, logger)
	defer r.Close()

	sender.Disconnect()

	err := r.Report(context.Background(), []byte("nil-cache-disconnected"))
	if err != ErrNotConnected {
		t.Fatalf("期望错误 %v，实际 %v", ErrNotConnected, err)
	}
}

func TestReport_NilSenderWithCache(t *testing.T) {
	cache := newMockCache()
	logger := zap.NewNop()

	cfg := DefaultConfig("test/topic")
	cfg.CacheOnFailure = true

	r := New(cfg, nil, cache, logger)
	defer r.Close()

	err := r.Report(context.Background(), []byte("no-sender"))
	if err != ErrSenderNotSet {
		t.Fatalf("期望错误 %v，实际 %v", ErrSenderNotSet, err)
	}
}
