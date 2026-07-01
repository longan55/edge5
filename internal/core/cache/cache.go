package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"edge5/config"

	"github.com/boltdb/bolt"
	"go.uber.org/zap"
)

// BoltCache 基于 BoltDB 的持久化缓存，每次操作即时打开/关闭数据库，
// 不持有文件锁，方便外部工具随时连接查看。
type BoltCache struct {
	dbPath string
	bucket []byte
	mu     sync.Mutex
	logger *zap.Logger
}

type CacheMessage struct {
	ID          string `json:"id"`
	Topic       string `json:"topic"`
	Payload     []byte `json:"payload"`
	RetryCount  int    `json:"retry_count"`
	CreatedAt   int64  `json:"created_at"`
	NextRetryAt int64  `json:"next_retry_at"`
}

type CacheOption func(*BoltCache)

func WithLogger(logger *zap.Logger) CacheOption {
	return func(bc *BoltCache) {
		bc.logger = logger
	}
}

func NewBoltCache(opts ...CacheOption) (*BoltCache, error) {
	cachePath, _ := filepath.Abs(config.CONFIG.Cache.BoltDB.Path)
	dir := filepath.Dir(cachePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	// 清理可能残留的锁文件
	lockPath := cachePath + ".lock"
	if _, err := os.Stat(lockPath); err == nil {
		_ = os.Remove(lockPath)
	}

	bucket := []byte(config.CONFIG.Cache.BoltDB.Bucket)
	if bucket == nil {
		bucket = []byte("cache_queue")
	}

	bc := &BoltCache{
		dbPath: cachePath,
		bucket: bucket,
		logger: zap.NewNop(),
	}

	for _, opt := range opts {
		opt(bc)
	}

	// 初始化时创建 bucket（用完即关闭，不持有锁）
	if err := bc.openAndUpdate(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(bucket)
		return err
	}); err != nil {
		return nil, fmt.Errorf("failed to create bucket: %w", err)
	}

	bc.logger.Info("BoltDB缓存初始化成功", zap.String("path", cachePath))
	return bc, nil
}

// openDB 打开数据库连接
func (bc *BoltCache) openDB() (*bolt.DB, error) {
	return bolt.Open(bc.dbPath, 0644, &bolt.Options{
		Timeout: time.Second,
	})
}

// openAndUpdate 打开数据库、执行写事务、关闭
func (bc *BoltCache) openAndUpdate(fn func(*bolt.Tx) error) error {
	db, err := bc.openDB()
	if err != nil {
		return fmt.Errorf("failed to open boltdb: %w", err)
	}
	defer db.Close()

	return db.Update(fn)
}

// openAndView 打开数据库、执行读事务、关闭
func (bc *BoltCache) openAndView(fn func(*bolt.Tx) error) error {
	db, err := bc.openDB()
	if err != nil {
		return fmt.Errorf("failed to open boltdb: %w", err)
	}
	defer db.Close()

	return db.View(fn)
}

func (bc *BoltCache) Push(msg *CacheMessage) error {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	return bc.openAndUpdate(func(tx *bolt.Tx) error {
		return tx.Bucket(bc.bucket).Put([]byte(msg.ID), data)
	})
}

func (bc *BoltCache) Pop() (*CacheMessage, error) {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	var msg *CacheMessage
	err := bc.openAndUpdate(func(tx *bolt.Tx) error {
		cursor := tx.Bucket(bc.bucket).Cursor()
		k, v := cursor.First()

		if k == nil {
			return nil
		}

		msg = &CacheMessage{}
		if err := json.Unmarshal(v, msg); err != nil {
			return err
		}

		return cursor.Delete()
	})

	return msg, err
}

func (bc *BoltCache) Peek() (*CacheMessage, error) {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	var msg *CacheMessage
	err := bc.openAndView(func(tx *bolt.Tx) error {
		cursor := tx.Bucket(bc.bucket).Cursor()
		_, v := cursor.First()

		if v == nil {
			return nil
		}

		msg = &CacheMessage{}
		return json.Unmarshal(v, msg)
	})

	return msg, err
}

func (bc *BoltCache) Get(id string) (*CacheMessage, error) {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	var msg *CacheMessage
	err := bc.openAndView(func(tx *bolt.Tx) error {
		v := tx.Bucket(bc.bucket).Get([]byte(id))
		if v == nil {
			return fmt.Errorf("message %s not found", id)
		}

		msg = &CacheMessage{}
		return json.Unmarshal(v, msg)
	})

	return msg, err
}

func (bc *BoltCache) Delete(id string) error {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	return bc.openAndUpdate(func(tx *bolt.Tx) error {
		return tx.Bucket(bc.bucket).Delete([]byte(id))
	})
}

func (bc *BoltCache) Update(msg *CacheMessage) error {
	return bc.Push(msg)
}

func (bc *BoltCache) Size() int {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	var count int
	bc.openAndView(func(tx *bolt.Tx) error {
		cursor := tx.Bucket(bc.bucket).Cursor()
		for k, _ := cursor.First(); k != nil; k, _ = cursor.Next() {
			count++
		}
		return nil
	})

	return count
}

func (bc *BoltCache) GetAll() ([]*CacheMessage, error) {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	var messages []*CacheMessage
	err := bc.openAndView(func(tx *bolt.Tx) error {
		cursor := tx.Bucket(bc.bucket).Cursor()
		for k, v := cursor.First(); k != nil; k, v = cursor.Next() {
			msg := &CacheMessage{}
			if err := json.Unmarshal(v, msg); err != nil {
				continue
			}
			messages = append(messages, msg)
		}
		return nil
	})

	return messages, err
}

func (bc *BoltCache) Clear() error {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	return bc.openAndUpdate(func(tx *bolt.Tx) error {
		return tx.DeleteBucket(bc.bucket)
	})
}

func (bc *BoltCache) Close() error {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	bc.logger.Info("BoltDB缓存关闭", zap.Int("remaining_messages", bc.unsafeSize()))

	// 数据库未持久持有，无需关闭连接
	return nil
}

// unsafeSize 不加锁的 Size 版本，仅用于 Close 时日志输出
func (bc *BoltCache) unsafeSize() int {
	var count int
	bc.openAndView(func(tx *bolt.Tx) error {
		cursor := tx.Bucket(bc.bucket).Cursor()
		for k, _ := cursor.First(); k != nil; k, _ = cursor.Next() {
			count++
		}
		return nil
	})
	return count
}
