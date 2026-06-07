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
	deviceID    uint64
	deviceSn    string
	protocol    string
	protocolObj Protocol
	cancel      context.CancelFunc
	done        chan struct{}
}

// Manager 协议连接管理器
// 负责管理设备与协议之间的连接生命周期
// 取代 internal/service/devicePluginRuntime
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
// deviceID: 设备 ID
// deviceSn: 设备序列号
// protocolName: 协议名称
// params: 连接参数
// subscribeInterval: 订阅间隔（秒）
func (m *Manager) Start(deviceID uint64, deviceSn string, protocolName string, params map[string]string, subscribeInterval int32) error {
	m.mu.Lock()
	if _, ok := m.devices[deviceID]; ok {
		m.mu.Unlock()
		return nil // 已连接
	}
	m.mu.Unlock()

	// 获取协议实现
	reg := DefaultRegistry()
	proto, ok := reg.Get(protocolName)
	if !ok {
		return fmt.Errorf("protocol %q not found", protocolName)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// 连接设备
	if err := proto.Connect(ctx, deviceSn, params); err != nil {
		cancel()
		return fmt.Errorf("protocol connect failed: %w", err)
	}

	done := make(chan struct{})

	conn := &deviceConn{
		deviceID:    deviceID,
		deviceSn:    deviceSn,
		protocol:    protocolName,
		protocolObj: proto,
		cancel:      cancel,
		done:        done,
	}

	m.mu.Lock()
	m.devices[deviceID] = conn
	m.mu.Unlock()

	// 启动订阅循环
	go func() {
		defer close(done)
		defer func() {
			m.mu.Lock()
			delete(m.devices, deviceID)
			m.mu.Unlock()
			_ = proto.Disconnect(context.Background(), deviceSn)
		}()

		ch, err := proto.SubscribeData(ctx, deviceSn, nil, subscribeInterval)
		if err != nil {
			m.logger.Warn("订阅数据失败",
				zap.Uint64("device_id", deviceID),
				zap.String("device_sn", deviceSn),
				zap.Error(err))
			return
		}

		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				m.logger.Debug("收到设备数据",
					zap.Uint64("device_id", deviceID),
					zap.String("device_sn", deviceSn),
					zap.Int("values", len(msg.Values)),
				)
				// 由上层处理 msg
				_ = msg
			}
		}
	}()

	m.logger.Info("设备协议连接已启动",
		zap.Uint64("device_id", deviceID),
		zap.String("device_sn", deviceSn),
		zap.String("protocol", protocolName))

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

	// 等待订阅循环结束
	select {
	case <-conn.done:
	case <-time.After(5 * time.Second):
	}

	// 断开连接
	if conn.protocolObj != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = conn.protocolObj.Disconnect(ctx, conn.deviceSn)
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
