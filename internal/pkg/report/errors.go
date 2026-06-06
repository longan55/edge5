package report

import "errors"

var (
	// ErrNotConnected 发送器未连接
	ErrNotConnected = errors.New("sender not connected")
	// ErrSenderNotSet 发送器未设置
	ErrSenderNotSet = errors.New("sender not set")
	// ErrCacheNotSet 缓存未设置
	ErrCacheNotSet = errors.New("cache not set")
)
