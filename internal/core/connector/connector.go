package connector

import (
	"edge5/config"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

type Connector interface {
	Uri() string
	Connect() error
	Close() error
	IsConnected() bool
}

type ConnectorManager interface {
	Register(key string, conn Connector) error
	Unregister(key string) error
	Get(key string) (Connector, bool)
	List() map[string]Connector
	States() map[string]bool
	UpdateState(key string, state bool)
	CloseAll()
}

type connectorManager struct {
	connectors map[string]Connector
	states     map[string]bool
	mutex      sync.RWMutex
	logger     *zap.Logger
	onClose    func() error
}

type ManagerOption func(*connectorManager)

func WithLogger(logger *zap.Logger) ManagerOption {
	return func(m *connectorManager) {
		m.logger = logger
	}
}

func WithOnClose(fn func() error) ManagerOption {
	return func(m *connectorManager) {
		m.onClose = fn
	}
}

func NewConnectorManager(opts ...ManagerOption) ConnectorManager {
	mgr := &connectorManager{
		connectors: make(map[string]Connector),
		states:     make(map[string]bool),
		logger:     zap.NewNop(),
	}

	for _, opt := range opts {
		opt(mgr)
	}

	if mgr.onClose != nil {
		go func() {
			<-make(chan struct{})
			mgr.onClose()
		}()
	}

	go mgr.reconnectLoop()

	return mgr
}

func (m *connectorManager) Register(key string, conn Connector) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if _, exists := m.connectors[key]; exists {
		return fmt.Errorf("connector %s already registered", key)
	}

	m.connectors[key] = conn
	m.states[key] = false

	m.logger.Info("连接器已注册",
		zap.String("key", key),
		zap.String("uri", conn.Uri()))

	return nil
}

func (m *connectorManager) Unregister(key string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	conn, exists := m.connectors[key]
	if !exists {
		return fmt.Errorf("connector %s not found", key)
	}

	if conn.IsConnected() {
		conn.Close()
	}

	delete(m.connectors, key)
	delete(m.states, key)

	m.logger.Info("连接器已注销",
		zap.String("key", key))

	return nil
}

func (m *connectorManager) Get(key string) (Connector, bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	conn, exists := m.connectors[key]
	return conn, exists
}

func (m *connectorManager) List() map[string]Connector {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	result := make(map[string]Connector, len(m.connectors))
	for k, v := range m.connectors {
		result[k] = v
	}
	return result
}

func (m *connectorManager) States() map[string]bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	result := make(map[string]bool, len(m.states))
	for k, v := range m.states {
		result[k] = v
	}
	return result
}

func (m *connectorManager) UpdateState(key string, state bool) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.states[key] = state
}

func (m *connectorManager) reconnectLoop() {
	interval := time.Duration(config.CONFIG.Connector.ReconnectInterval) * time.Millisecond
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	baseDelay := time.Duration(config.CONFIG.Connector.BaseDelay) * time.Millisecond
	maxDelay := time.Duration(config.CONFIG.Connector.MaxDelay) * time.Millisecond
	factor := config.CONFIG.Connector.Factor

	retryCount := make(map[string]int)

	for range ticker.C {
		m.mutex.RLock()
		for key, conn := range m.connectors {
			state := m.states[key]
			if !state && conn != nil {
				go func(k string, c Connector, retries int) {
					delay := min(time.Duration(factor^retries)*baseDelay, maxDelay)
					if retries > 0 {
						time.Sleep(delay)
					}

					if err := c.Connect(); err != nil {
						m.logger.Error("重连失败",
							zap.String("key", k),
							zap.String("uri", c.Uri()),
							zap.Error(err))
						retryCount[k]++
						return
					}

					m.UpdateState(k, true)
					retryCount[k] = 0
					m.logger.Info("重连成功",
						zap.String("key", k),
						zap.String("uri", c.Uri()))
				}(key, conn, retryCount[key])
			}
		}
		m.mutex.RUnlock()
	}
}

func (m *connectorManager) CloseAll() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	for key, conn := range m.connectors {
		if conn != nil && conn.IsConnected() {
			if err := conn.Close(); err != nil {
				m.logger.Error("关闭连接失败",
					zap.String("key", key),
					zap.Error(err))
			} else {
				m.logger.Info("连接已关闭",
					zap.String("key", key))
			}
		}
		m.states[key] = false
	}
}

func min(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
