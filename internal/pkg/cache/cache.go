package cache

import (
	"edge5/config"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/boltdb/bolt"
	"go.uber.org/zap"
)

type BoltCache struct {
	db     *bolt.DB
	bucket []byte
	mu     sync.RWMutex
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

	db, err := bolt.Open(cachePath, 0644, &bolt.Options{
		Timeout: time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open boltdb: %w", err)
	}

	bucket := []byte(config.CONFIG.Cache.BoltDB.Bucket)
	if bucket == nil {
		bucket = []byte("cache_queue")
	}

	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(bucket)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create bucket: %w", err)
	}

	bc := &BoltCache{
		db:     db,
		bucket: bucket,
		logger: zap.NewNop(),
	}

	for _, opt := range opts {
		opt(bc)
	}

	bc.logger.Info("BoltDB缓存初始化成功",
		zap.String("path", cachePath))

	return bc, nil
}

func (bc *BoltCache) Push(msg *CacheMessage) error {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	return bc.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bc.bucket).Put([]byte(msg.ID), data)
	})
}

func (bc *BoltCache) Pop() (*CacheMessage, error) {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	var msg *CacheMessage
	err := bc.db.Update(func(tx *bolt.Tx) error {
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
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	var msg *CacheMessage
	err := bc.db.View(func(tx *bolt.Tx) error {
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
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	var msg *CacheMessage
	err := bc.db.View(func(tx *bolt.Tx) error {
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

	return bc.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bc.bucket).Delete([]byte(id))
	})
}

func (bc *BoltCache) Update(msg *CacheMessage) error {
	return bc.Push(msg)
}

func (bc *BoltCache) Size() int {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	var count int
	bc.db.View(func(tx *bolt.Tx) error {
		cursor := tx.Bucket(bc.bucket).Cursor()
		for k, _ := cursor.First(); k != nil; k, _ = cursor.Next() {
			count++
		}
		return nil
	})

	return count
}

func (bc *BoltCache) GetAll() ([]*CacheMessage, error) {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	var messages []*CacheMessage
	err := bc.db.View(func(tx *bolt.Tx) error {
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

	return bc.db.Update(func(tx *bolt.Tx) error {
		return tx.DeleteBucket(bc.bucket)
	})
}

func (bc *BoltCache) Close() error {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	bc.logger.Info("BoltDB缓存关闭",
		zap.Int("remaining_messages", bc.Size()))

	return bc.db.Close()
}
