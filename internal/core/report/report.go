package report

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Config 上报配置信息
// 实现方可根据需要扩展字段
type Config struct {
	// Topic 上报主题/路由标识
	Topic string `json:"topic" yaml:"topic"`
	// Qos 消息质量等级（MQTT 等协议使用）
	Qos byte `json:"qos" yaml:"qos"`
	// CacheOnFailure 上报失败时是否自动缓存（默认 true）
	CacheOnFailure bool `json:"cache_on_failure" yaml:"cache_on_failure"`
	// RetryInterval 缓存重试间隔（秒），<=0 时使用默认值 30
	RetryInterval int `json:"retry_interval" yaml:"retry_interval"`
	// MaxRetries 每条缓存消息的最大重试次数，<=0 为无限重试
	MaxRetries int `json:"max_retries" yaml:"max_retries"`
}

// DefaultConfig 返回默认上报配置
func DefaultConfig(topic string) Config {
	return Config{
		Topic:          topic,
		Qos:            1,
		CacheOnFailure: true,
		RetryInterval:  30,
		MaxRetries:     0,
	}
}

// Sender 底层发送接口
// 由具体实现（MQTT、HTTP 等）提供
type Sender interface {
	// Name 返回发送器名称，用于日志标识
	Name() string
	// Send 同步发送数据，返回 error 表示发送失败
	Send(ctx context.Context, topic string, qos byte, data []byte) error
	// IsConnected 返回当前连接状态
	IsConnected() bool
}

// Reporter 通用上报接口
// 所有设备通过此接口上报数据
type Reporter interface {
	// Config 返回当前上报配置
	Config() Config

	// Report 上报数据
	// data: 上报的二进制数据
	// 返回值 error 为 nil 表示上报成功
	// 若内部开启了缓存，即使返回 error 数据也已写入缓存
	Report(ctx context.Context, data []byte) error

	// Close 关闭上报器，释放资源
	Close() error
}

// reporter 是 Reporter 的标准实现
type reporter struct {
	cfg       Config
	sender    Sender
	cache     Cache
	logger    *zap.Logger
	closeCh   chan struct{}
	closeOnce sync.Once
	wg        sync.WaitGroup
}

// New 创建 Reporter 实例
func New(cfg Config, sender Sender, cache Cache, logger *zap.Logger) Reporter {
	if logger == nil {
		logger = zap.NewNop()
	}

	r := &reporter{
		cfg:     cfg,
		sender:  sender,
		cache:   cache,
		logger:  logger,
		closeCh: make(chan struct{}),
	}

	if cfg.CacheOnFailure && cache != nil {
		r.startRetryLoop()
	}

	return r
}

// Config 返回上报配置
func (r *reporter) Config() Config {
	return r.cfg
}

// Report 上报数据
func (r *reporter) Report(ctx context.Context, data []byte) error {
	if r.sender == nil {
		return ErrSenderNotSet
	}

	if r.sender.IsConnected() {
		err := r.sender.Send(ctx, r.cfg.Topic, r.cfg.Qos, data)
		if err == nil {
			return nil
		}

		r.logger.Warn("上报发送失败，将尝试缓存",
			zap.String("sender", r.sender.Name()),
			zap.String("topic", r.cfg.Topic),
			zap.Error(err))

		if !r.cfg.CacheOnFailure || r.cache == nil {
			return err
		}

		if cacheErr := r.cache.Push(data); cacheErr != nil {
			r.logger.Error("写入缓存失败", zap.Error(cacheErr))
			return cacheErr
		}

		r.logger.Info("数据已缓存（发送失败）", zap.Int("body_size", len(data)))
		return err
	}

	r.logger.Warn("上报连接断开，数据将被缓存",
		zap.String("sender", r.sender.Name()),
		zap.String("topic", r.cfg.Topic))

	if !r.cfg.CacheOnFailure || r.cache == nil {
		return ErrNotConnected
	}

	if cacheErr := r.cache.Push(data); cacheErr != nil {
		r.logger.Error("写入缓存失败", zap.Error(cacheErr))
		return cacheErr
	}

	r.logger.Info("数据已缓存（连接断开）", zap.Int("body_size", len(data)))
	return nil
}

// startRetryLoop 启动后台缓存重试循环
func (r *reporter) startRetryLoop() {
	interval := r.cfg.RetryInterval
	if interval <= 0 {
		interval = 30
	}

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()

		ticker := time.NewTicker(time.Duration(interval) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-r.closeCh:
				return
			case <-ticker.C:
				r.retryCache()
			}
		}
	}()

	r.logger.Info("缓存重试后台任务已启动", zap.Int("interval_sec", interval))
}

// retryCache 尝试发送所有缓存数据
func (r *reporter) retryCache() {
	if r.sender == nil || !r.sender.IsConnected() {
		return
	}

	if r.cache == nil {
		return
	}

	cachedItems, err := r.cache.GetAll()
	if err != nil {
		r.logger.Error("读取缓存列表失败", zap.Error(err))
		return
	}

	if len(cachedItems) == 0 {
		return
	}

	r.logger.Info("开始重试缓存上报", zap.Int("count", len(cachedItems)))

	for _, item := range cachedItems {
		select {
		case <-r.closeCh:
			return
		default:
		}

		if !r.sender.IsConnected() {
			r.logger.Warn("重试过程中连接断开，剩余缓存将在下次重试")
			return
		}

		if r.cfg.MaxRetries > 0 && item.RetryCount >= r.cfg.MaxRetries {
			r.logger.Warn("缓存消息已达最大重试次数，丢弃",
				zap.String("id", item.ID),
				zap.Int("retries", item.RetryCount))
			r.cache.Delete(item.ID)
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err := r.sender.Send(ctx, r.cfg.Topic, r.cfg.Qos, item.Payload)
		cancel()

		if err != nil {
			item.RetryCount++
			item.NextRetryAt = time.Now().Add(time.Duration(r.cfg.RetryInterval) * time.Second).Unix()
			r.cache.Update(item)
			r.logger.Warn("缓存重试发送失败",
				zap.String("id", item.ID),
				zap.Int("retries", item.RetryCount),
				zap.Error(err))
			return
		}

		r.cache.Delete(item.ID)
		r.logger.Info("缓存重试发送成功", zap.String("id", item.ID))
	}
}

// Close 关闭上报器
func (r *reporter) Close() error {
	r.closeOnce.Do(func() {
		close(r.closeCh)
	})
	r.wg.Wait()
	name := ""
	if r.sender != nil {
		name = r.sender.Name()
	}
	r.logger.Info("上报器已关闭",
		zap.String("sender", name),
		zap.String("topic", r.cfg.Topic))
	return nil
}

var _ Reporter = (*reporter)(nil)
