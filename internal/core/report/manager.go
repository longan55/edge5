package report

import (
	"fmt"
	"sync"

	"go.uber.org/zap"
)

// ReporterManager 管理多个 Reporter 实例
// 提供统一的注册、获取、关闭功能
type ReporterManager struct {
	mu        sync.RWMutex
	reporters map[string]Reporter
	logger    *zap.Logger
}

// NewReporterManager 创建上报管理器
func NewReporterManager(logger *zap.Logger) *ReporterManager {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &ReporterManager{
		reporters: make(map[string]Reporter),
		logger:    logger,
	}
}

// Register 注册一个上报器
// name: 上报器名称（唯一标识）
// r: 上报器实例
func (m *ReporterManager) Register(name string, r Reporter) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.reporters[name]; exists {
		return fmt.Errorf("reporter %q already registered", name)
	}

	m.reporters[name] = r
	m.logger.Info("上报器已注册", zap.String("name", name))
	return nil
}

// Unregister 注销并关闭一个上报器
func (m *ReporterManager) Unregister(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	r, exists := m.reporters[name]
	if !exists {
		return fmt.Errorf("reporter %q not found", name)
	}

	if closer, ok := r.(interface{ Close() error }); ok {
		if err := closer.Close(); err != nil {
			m.logger.Error("关闭上报器失败",
				zap.String("name", name),
				zap.Error(err))
		}
	}

	delete(m.reporters, name)
	m.logger.Info("上报器已注销", zap.String("name", name))
	return nil
}

// Get 获取上报器
func (m *ReporterManager) Get(name string) (Reporter, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	r, exists := m.reporters[name]
	return r, exists
}

// CloseAll 关闭所有上报器
func (m *ReporterManager) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, r := range m.reporters {
		if closer, ok := r.(interface{ Close() error }); ok {
			if err := closer.Close(); err != nil {
				m.logger.Error("关闭上报器失败",
					zap.String("name", name),
					zap.Error(err))
			}
		}
		delete(m.reporters, name)
	}

	m.logger.Info("所有上报器已关闭")
}

// Names 返回所有已注册的上报器名称
func (m *ReporterManager) Names() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.reporters))
	for name := range m.reporters {
		names = append(names, name)
	}
	return names
}
