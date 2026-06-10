package protocol

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// deviceConn 设备连接状态
type deviceConn struct {
	deviceID uint64
	deviceSn string
	protocol string
	proto    DeviceCommProtocol
	handle   DeviceHandle
	cancel   context.CancelFunc
	done     chan struct{}
}

// Manager 协议连接管理器
type Manager struct {
	mu      sync.Mutex
	devices map[uint64]*deviceConn
	logger  *zap.Logger
}

// NewManager 创建协议连接管理器
func NewManager(logger *zap.Logger) *Manager {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Manager{
		devices: make(map[uint64]*deviceConn),
		logger:  logger,
	}
}

// Start 启动设备连接
func (m *Manager) Start(deviceID uint64, deviceSn string, protocolName string, params Metadata) error {
	m.mu.Lock()
	if _, ok := m.devices[deviceID]; ok {
		m.mu.Unlock()
		return nil
	}
	m.mu.Unlock()

	reg := DefaultRegistry()
	proto, ok := reg.Get(protocolName)
	if !ok {
		return fmt.Errorf("protocol %q not found", protocolName)
	}

	ctx, cancel := context.WithCancel(context.Background())

	handle, err := proto.Connect(ctx, params)
	if err != nil {
		cancel()
		return fmt.Errorf("protocol connect failed: %w", err)
	}

	done := make(chan struct{})

	conn := &deviceConn{
		deviceID: deviceID,
		deviceSn: deviceSn,
		protocol: protocolName,
		proto:    proto,
		handle:   handle,
		cancel:   cancel,
		done:     done,
	}

	m.mu.Lock()
	m.devices[deviceID] = conn
	m.mu.Unlock()

	go func() {
		defer close(done)
		defer func() {
			m.mu.Lock()
			delete(m.devices, deviceID)
			m.mu.Unlock()
			_ = proto.Disconnect(context.Background(), handle)
		}()
		// 等待取消信号
		<-ctx.Done()
	}()

	m.logger.Info("设备协议连接已启动",
		zap.Uint64("device_id", deviceID),
		zap.String("device_sn", deviceSn),
		zap.String("protocol", protocolName),
		zap.Uint64("handle", uint64(handle)))

	return nil
}

// Stop 停止设备连接
func (m *Manager) Stop(deviceID uint64) error {
	m.mu.Lock()
	conn, ok := m.devices[deviceID]
	if !ok {
		m.mu.Unlock()
		return nil
	}
	delete(m.devices, deviceID)
	m.mu.Unlock()

	if conn.cancel != nil {
		conn.cancel()
	}

	select {
	case <-conn.done:
	case <-time.After(5 * time.Second):
	}

	if conn.proto != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = conn.proto.Disconnect(ctx, conn.handle)
	}

	m.logger.Info("设备协议连接已停止",
		zap.Uint64("device_id", deviceID),
		zap.String("device_sn", conn.deviceSn),
		zap.String("protocol", conn.protocol))

	return nil
}

// StopAll 停止所有设备连接
func (m *Manager) StopAll() {
	m.mu.Lock()
	devices := make([]*deviceConn, 0, len(m.devices))
	for _, conn := range m.devices {
		devices = append(devices, conn)
	}
	m.mu.Unlock()

	for _, conn := range devices {
		_ = m.Stop(conn.deviceID)
	}
}

// IsConnected 检查设备是否已连接
func (m *Manager) IsConnected(deviceID uint64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.devices[deviceID]
	return ok
}
