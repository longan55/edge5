// Package modbusrtu 实现 Modbus RTU 串口协议（主服务）。
//
// 站号在连接参数 slave_id 中指定；点位仅配 address+length+parseType。
// 同功能码连续地址在 ReadBatch/WriteBatch 底层合并为一次 Modbus 通信。
package modbusrtu

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"edge5/internal/core/protocol"
	"edge5/internal/core/protocol/builtin"

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
	Clients   map[uint]*rtuMaster    // deviceID -> RTU 主站
	SlaveIDs  map[uint]byte          // deviceID -> 站号（连接参数）
	Timeouts  map[uint]time.Duration // deviceID -> 读响应超时（连接参数）
	connected bool
	deviceID  uint
	portMap   map[uint]string // deviceID -> 串口（用于 Release）
}

// NewModbusRTUService 创建服务
func NewModbusRTUService() *ModbusRTUService {
	return &ModbusRTUService{
		Clients:  make(map[uint]*rtuMaster),
		SlaveIDs: make(map[uint]byte),
		Timeouts: make(map[uint]time.Duration),
		portMap:  make(map[uint]string),
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
			protocol.Metadata{"name": "response_timeout", "cName": "响应超时(ms)", "type": "number", "required": false, "default": "2000"},
		},
		"readParamsSchema": []any{
			protocol.Metadata{"name": "address", "cName": "地址", "type": "string", "required": true, "default": "", "placeholder": "十进制 600/40600；十六进制 0x3A、3Ah（0-based线地址）"},
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
	slaveID := parseSlaveID(params)

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
	// 读响应超时：连接参数可配（毫秒），默认 2000ms。
	// Windows 下 serial.Config.Timeout=0 会使 ReadFile 永久阻塞，
	// 从站不响应时 context 超时对 syscall 无效，必须设置。
	timeoutMs := builtin.GetInt(params, "response_timeout")
	if timeoutMs < 100 {
		timeoutMs = 2000
	}
	readTimeout := time.Duration(timeoutMs) * time.Millisecond

	// 串口占用管理
	if builtin.SerialManager != nil {
		if err := builtin.SerialManager.Acquire(port, deviceID); err != nil {
			return protocol.InvalidHandle, err
		}
	}

	// 自管串口：Windows 下显式使能 RTS/DTR（部分 RS485 转换器需要 RTS 高电平
	// 才能接收），非 Windows 使用 termios CLOCAL 模式。
	sp, err := openRTUSerial(port, baudRate, dataBits, stopBits, parity, readTimeout)
	if err != nil {
		if builtin.SerialManager != nil {
			builtin.SerialManager.Release(port)
		}
		return protocol.InvalidHandle, fmt.Errorf("打开串口失败: %w", err)
	}
	client := &rtuMaster{port: sp, baudRate: baudRate}

	m.Clients[deviceID] = client
	m.SlaveIDs[deviceID] = slaveID
	m.Timeouts[deviceID] = readTimeout
	m.portMap[deviceID] = port
	m.connected = true
	m.deviceID = deviceID

	if builtin.Logger != nil {
		builtin.Logger.Info("Modbus-RTU 设备连接成功",
			zap.Uint("deviceID", deviceID),
			zap.String("port", port),
			zap.Int("baudRate", baudRate),
			zap.Int("dataBits", dataBits),
			zap.String("parity", parity),
			zap.Int("stopBits", stopBits),
			zap.Uint8("slaveID", slaveID),
			zap.Duration("timeout", readTimeout),
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
		delete(m.SlaveIDs, devID)
		delete(m.Timeouts, devID)
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

	// 调试帧抓取：Options 中带 FrameCapture 时，记录每段底层收发帧
	capture, _ := req.Options[protocol.FrameCaptureKey].(*protocol.FrameCapture)

	results := make([]protocol.BatchReadResult, len(req.Points))
	for _, seg := range segments {
		wireCode := wireFuncCode(seg.code)
		if capture != nil {
			group := capture.BeginGroup(protocol.Metadata{
				"op":       "read",
				"slaveID":  slaveID,
				"code":     wireCode,
				"funcName": funcCodeName(wireCode),
				"start":    seg.start,
				"count":    seg.count,
				"isBit":    seg.isBit,
			})
			client.setFrameHook(group)
		}
		segData, err := readSegmentData(client, slaveID, seg)
		client.setFrameHook(nil)
		if err != nil {
			// 段级错误隔离：仅本段点位 quality=bad
			err = wrapTimeoutErr(err, m.portMap[devID], slaveID, m.Timeouts[devID], wireCode, seg.start, seg.count)
			for _, p := range seg.points {
				results[p.index] = protocol.BatchReadResult{
					PointName: p.pt.Resource,
					Quality:   "bad",
					Error:     err,
				}
			}
			if builtin.Logger != nil {
				builtin.Logger.Warn("Modbus-RTU 段读取失败",
					zap.Uint8("code", wireCode),
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

	// 调试帧抓取
	capture, _ := req.Options[protocol.FrameCaptureKey].(*protocol.FrameCapture)

	for _, seg := range segments {
		var code byte
		var count uint16
		if seg.isBit {
			if len(seg.bits) == 1 {
				code, count = 5, 1
			} else {
				code, count = 15, uint16(len(seg.bits))
			}
		} else {
			if len(seg.regs) == 1 {
				code, count = 6, 1
			} else {
				code, count = 16, uint16(len(seg.regs))
			}
		}

		if capture != nil {
			group := capture.BeginGroup(protocol.Metadata{
				"op":       "write",
				"slaveID":  slaveID,
				"code":     code,
				"funcName": funcCodeName(code),
				"start":    seg.start,
				"count":    count,
				"isBit":    seg.isBit,
			})
			client.setFrameHook(group)
		}

		var werr error
		if seg.isBit {
			if code == 5 {
				werr = client.WriteSingleCoil(slaveID, seg.start, seg.bits[0] != 0)
			} else {
				werr = client.WriteMultipleCoils(slaveID, seg.start, count, seg.bits)
			}
		} else {
			if code == 6 {
				werr = client.WriteSingleRegister(slaveID, seg.start, seg.regs[0])
			} else {
				werr = client.WriteMultipleRegisters(slaveID, seg.start, count, seg.regs)
			}
		}
		client.setFrameHook(nil)
		if werr != nil {
			return fmt.Errorf("写入失败(地址=0x%04X): %w", seg.start,
				wrapTimeoutErr(werr, m.portMap[devID], slaveID, m.Timeouts[devID], code, seg.start, count))
		}
	}
	return nil
}

// wrapTimeoutErr 把串口无响应超时包装为带排查指引的错误，其他错误原样返回。
// errSerialTimeout 表示请求帧已成功写入串口、但在设定时间内未收到任何返回字节。
func wrapTimeoutErr(err error, port string, slaveID byte, timeout time.Duration, code byte, start, count uint16) error {
	if !errors.Is(err, errSerialTimeout) {
		return err
	}
	return fmt.Errorf("设备无响应，等待 %s 未收到任何返回字节（串口=%s 站号=0x%02X(%d) 功能码=0x%02X %s 起始地址=0x%04X 数量=0x%04X(%d)）。"+
		"排查：①波特率/数据位/校验位/停止位是否与设备一致；②站号是否正确；③RS485 A/B是否接反、设备是否上电；"+
		"④完整发送帧HEX见界面“通信报文”区，确认对端是否收到（USB转换器TX灯是否闪烁）；⑤设备响应慢可调大连接参数“响应超时(ms)”(当前 %dms)",
		timeout, port, slaveID, slaveID, code, funcCodeName(code), start, count, count, timeout.Milliseconds())
}

// wireFuncCode 内部分区码转为线路 Modbus 功能码：
// 1=线圈→FC1, 2=离散输入→FC2, 3=输入寄存器→FC4, 4=保持寄存器→FC3
func wireFuncCode(internalCode byte) byte {
	switch internalCode {
	case 3:
		return fcReadInputRegisters
	case 4:
		return fcReadHoldingRegisters
	}
	return internalCode
}

// funcCodeName 功能码中文名（用于错误信息）
func funcCodeName(code byte) string {
	switch code {
	case 1:
		return "读线圈FC1"
	case 2:
		return "读离散输入FC2"
	case 3:
		return "读保持寄存器FC3"
	case 4:
		return "读输入寄存器FC4"
	case 5:
		return "写单线圈FC5"
	case 6:
		return "写单寄存器FC6"
	case 15:
		return "写多线圈FC15"
	case 16:
		return "写多寄存器FC16"
	}
	return "未知"
}
