// Package modbusrtu 实现 Modbus RTU 串口协议（主服务）。
//
// 站号在连接参数 slave_id 中指定；点位仅配 address+length+parseType。
// 同功能码连续地址在 ReadBatch/WriteBatch 底层合并为一次 Modbus 通信。
package modbusrtu

import (
	"context"
	"fmt"
	"sync"

	"edge5/internal/core/protocol"
	"edge5/internal/core/protocol/builtin"

	"github.com/goburrow/serial"
	modbus "github.com/thinkgos/gomodbus/v2"
	"go.uber.org/zap"
)

func init() {
	if err := builtin.Register(NewModbusRTUService()); err != nil {
		panic(fmt.Sprintf("注册 Modbus-RTU 协议失败: %v", err))
	}
}

// ModbusRTUService Modbus RTU 串口协议服务
type ModbusRTUService struct {
	lock      sync.Mutex
	Clients   map[uint]modbus.Client // deviceID -> client
	Providers map[uint]modbus.ClientProvider
	SlaveIDs  map[uint]byte // deviceID -> 站号（连接参数）
	connected bool
	deviceID  uint
	portMap   map[uint]string // deviceID -> 串口（用于 Release）
}

// NewModbusRTUService 创建服务
func NewModbusRTUService() *ModbusRTUService {
	return &ModbusRTUService{
		Clients:   make(map[uint]modbus.Client),
		Providers: make(map[uint]modbus.ClientProvider),
		SlaveIDs:  make(map[uint]byte),
		portMap:   make(map[uint]string),
	}
}

// Info 协议元数据
func (m *ModbusRTUService) Info() protocol.Metadata {
	return protocol.Metadata{
		"name":          "Modbus-RTU",
		"alias":         "ModbusRTU",
		"version":       "1.0.0",
		"description":   "Modbus RTU 串口协议（站号在连接参数，地址自动识别功能码）",
		"group":         "builtin",
		"source":        "builtin",
		"deviceType":    "ANY",
		"brand":         "Modbus",
		"cName":         "Modbus",
		"supportServer": false,
		"supportDebug":  true,
		"models":        []string{"通用"},
		"connectionParams": []any{
			protocol.Metadata{"name": "serial_port", "cName": "串口", "type": "select", "required": true, "default": "COM1"},
			protocol.Metadata{"name": "baud_rate", "cName": "波特率", "type": "select", "required": true, "default": "9600", "choices": []string{"1200", "2400", "4800", "9600", "19200", "38400", "57600", "115200"}},
			protocol.Metadata{"name": "data_bits", "cName": "数据位", "type": "select", "required": true, "default": "8", "choices": []string{"7", "8"}},
			protocol.Metadata{"name": "parity", "cName": "校验位", "type": "select", "required": true, "default": "N", "choices": []string{"N", "E", "O"}},
			protocol.Metadata{"name": "stop_bits", "cName": "停止位", "type": "select", "required": true, "default": "1", "choices": []string{"1", "2"}},
			protocol.Metadata{"name": "slave_id", "cName": "站号", "type": "number", "required": true, "default": "1"},
		},
		"readParamsSchema": []any{
			protocol.Metadata{"name": "address", "cName": "地址", "type": "string", "required": true, "default": ""},
			protocol.Metadata{"name": "length", "cName": "长度", "type": "int", "required": false, "default": "1"},
			protocol.Metadata{"name": "parseType", "cName": "解析类型", "type": "select", "required": true, "default": "uint16", "choices": []string{
				"bool", "byte", "int16", "uint16", "int32", "uint32", "int64", "uint64", "float32", "float64", "string",
				"int32_abcd", "int32_cdab", "int32_badc", "int32_dcba",
				"uint32_abcd", "uint32_cdab", "uint32_badc", "uint32_dcba",
				"float32_abcd", "float32_cdab", "float32_badc", "float32_dcba",
				"int64_abcd", "int64_cdab", "int64_badc", "int64_dcba",
				"uint64_abcd", "uint64_cdab", "uint64_badc", "uint64_dcba",
				"float64_abcd", "float64_cdab", "float64_badc", "float64_dcba",
			}},
		},
	}
}

// IsSupportServer 是否支持对外提供 MQTT 接口（Modbus RTU 不对外）
func (m *ModbusRTUService) IsSupportServer() bool {
	return false
}

// Connect 建立串口连接并缓存 client/站号/串口
func (m *ModbusRTUService) Connect(ctx context.Context, params protocol.Metadata) (protocol.DeviceHandle, error) {
	m.lock.Lock()
	defer m.lock.Unlock()

	deviceID := uint(builtin.GetInt(params, "deviceID"))
	if deviceID == 0 {
		return protocol.InvalidHandle, fmt.Errorf("缺少 deviceID 参数")
	}

	// 已连接则复用
	if client, ok := m.Clients[deviceID]; ok && client != nil {
		m.connected = true
		m.deviceID = deviceID
		return protocol.DeviceHandle(deviceID), nil
	}

	port := builtin.GetString(params, "serial_port")
	baudRate := builtin.GetInt(params, "baud_rate")
	dataBits := builtin.GetInt(params, "data_bits")
	parity := builtin.GetString(params, "parity")
	stopBits := builtin.GetInt(params, "stop_bits")
	slaveID := byte(builtin.GetInt(params, "slave_id"))

	if port == "" {
		return protocol.InvalidHandle, fmt.Errorf("缺少串口参数")
	}
	if baudRate <= 0 {
		baudRate = 9600
	}
	if dataBits <= 0 {
		dataBits = 8
	}
	if stopBits <= 0 {
		stopBits = 1
	}
	if parity == "" {
		parity = "N"
	}
	if slaveID == 0 {
		slaveID = 1
	}

	// 串口占用管理
	if builtin.SerialManager != nil {
		if err := builtin.SerialManager.Acquire(port, deviceID); err != nil {
			return protocol.InvalidHandle, err
		}
	}

	provider := modbus.NewRTUClientProvider(
		modbus.WithSerialConfig(serial.Config{
			Address:  port,
			BaudRate: baudRate,
			DataBits: dataBits,
			StopBits: stopBits,
			Parity:   parity,
		}),
	)
	client := modbus.NewClient(provider)

	if err := client.Connect(); err != nil {
		if builtin.SerialManager != nil {
			builtin.SerialManager.Release(port)
		}
		return protocol.InvalidHandle, fmt.Errorf("连接失败: %w", err)
	}

	m.Clients[deviceID] = client
	m.Providers[deviceID] = provider
	m.SlaveIDs[deviceID] = slaveID
	m.portMap[deviceID] = port
	m.connected = true
	m.deviceID = deviceID

	if builtin.Logger != nil {
		builtin.Logger.Info("Modbus-RTU 设备连接成功",
			zap.Uint("deviceID", deviceID),
			zap.String("port", port),
			zap.Int("baudRate", baudRate),
			zap.Uint8("slaveID", slaveID),
		)
	}
	return protocol.DeviceHandle(deviceID), nil
}

// Disconnect 断开连接并释放串口
func (m *ModbusRTUService) Disconnect(ctx context.Context, handle protocol.DeviceHandle) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	devID := uint(handle)
	if devID == 0 {
		devID = m.deviceID
	}

	if client, ok := m.Clients[devID]; ok {
		client.Close()
		delete(m.Clients, devID)
		delete(m.Providers, devID)
		delete(m.SlaveIDs, devID)
	}

	if port, ok := m.portMap[devID]; ok {
		if builtin.SerialManager != nil {
			builtin.SerialManager.Release(port)
		}
		delete(m.portMap, devID)
	}

	m.connected = false

	if builtin.Logger != nil {
		builtin.Logger.Info("Modbus-RTU 设备断开连接", zap.Uint("deviceID", devID))
	}
	return nil
}

// IsConnected 检查设备是否已连接
func (m *ModbusRTUService) IsConnected(handle protocol.DeviceHandle) bool {
	m.lock.Lock()
	defer m.lock.Unlock()

	devID := uint(handle)
	if devID == 0 {
		devID = m.deviceID
	}

	client, ok := m.Clients[devID]
	return ok && client != nil && m.connected
}

// ReadBatch 批量读取：同功能码连续地址合并为一次 Modbus 通信
func (m *ModbusRTUService) ReadBatch(ctx context.Context, handle protocol.DeviceHandle, req protocol.BatchReadRequest) (*protocol.BatchReadResponse, error) {
	m.lock.Lock()
	defer m.lock.Unlock()

	devID := uint(handle)
	if devID == 0 {
		devID = m.deviceID
	}

	client, ok := m.Clients[devID]
	if !ok || client == nil {
		return nil, fmt.Errorf("设备未连接")
	}
	slaveID := m.SlaveIDs[devID]

	segments, err := planReadBatch(req.Points)
	if err != nil {
		return nil, err
	}

	results := make([]protocol.BatchReadResult, len(req.Points))
	for _, seg := range segments {
		segData, err := readSegmentData(client, slaveID, seg)
		if err != nil {
			// 段级错误隔离：仅本段点位 quality=bad
			for _, p := range seg.points {
				results[p.index] = protocol.BatchReadResult{
					PointName: p.pt.Resource,
					Quality:   "bad",
					Error:     err,
				}
			}
			if builtin.Logger != nil {
				builtin.Logger.Warn("Modbus-RTU 段读取失败",
					zap.Uint8("code", seg.code),
					zap.Uint16("start", seg.start),
					zap.Uint16("count", seg.count),
					zap.Error(err))
			}
			continue
		}
		for _, p := range seg.points {
			val, derr := decodePoint(p, segData, seg.start, seg.isBit)
			quality := "good"
			if derr != nil {
				quality = "bad"
			}
			results[p.index] = protocol.BatchReadResult{
				PointName: p.pt.Resource,
				Value:     val,
				Quality:   quality,
				Error:     derr,
			}
		}
	}

	return &protocol.BatchReadResponse{Results: results}, nil
}

// WriteBatch 批量写入：连续保持寄存器合并为 FC16，连续线圈合并为 FC15，否则降级 FC6/FC5
func (m *ModbusRTUService) WriteBatch(ctx context.Context, handle protocol.DeviceHandle, req protocol.BatchWriteRequest) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	devID := uint(handle)
	if devID == 0 {
		devID = m.deviceID
	}

	client, ok := m.Clients[devID]
	if !ok || client == nil {
		return fmt.Errorf("设备未连接")
	}
	slaveID := m.SlaveIDs[devID]

	segments, err := planWriteBatch(req.Items)
	if err != nil {
		return err
	}

	for _, seg := range segments {
		if seg.isBit {
			if len(seg.bits) == 1 {
				if err := client.WriteSingleCoil(slaveID, seg.start, seg.bits[0] != 0); err != nil {
					return fmt.Errorf("写入线圈 %d 失败: %w", seg.start, err)
				}
			} else {
				if err := client.WriteMultipleCoils(slaveID, seg.start, uint16(len(seg.bits)), seg.bits); err != nil {
					return fmt.Errorf("批量写入线圈 %d 失败: %w", seg.start, err)
				}
			}
		} else {
			if len(seg.regs) == 1 {
				if err := client.WriteSingleRegister(slaveID, seg.start, seg.regs[0]); err != nil {
					return fmt.Errorf("写入寄存器 %d 失败: %w", seg.start, err)
				}
			} else {
				if err := client.WriteMultipleRegisters(slaveID, seg.start, uint16(len(seg.regs)), seg.regs); err != nil {
					return fmt.Errorf("批量写入寄存器 %d 失败: %w", seg.start, err)
				}
			}
		}
	}
	return nil
}
