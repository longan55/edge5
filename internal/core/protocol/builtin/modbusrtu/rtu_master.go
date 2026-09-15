// rtu_master.go 自实现的 Modbus RTU 主站传输层。
//
// 不依赖第三方 modbus 库的串口收发，原因：
//   - 需要完全掌控串口打开（Windows 下 RTS/DTR 使能，见 serial_port_windows.go）
//   - 收发帧直接用于调试抓帧，不经过日志字符串解析
//
// 帧格式：[slaveID][funcCode][data...][CRC16-LO][CRC16-HI]，标准 Modbus CRC16（0xA001）。
package modbusrtu

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"edge5/internal/core/protocol"
	"edge5/internal/core/protocol/builtin"

	"go.uber.org/zap"
)

const (
	rtuADUMinSize = 4 // 站号 + 功能码 + CRC(2)
	rtuADUMaxSize = 256
)

// 功能码
const (
	fcReadCoils              byte = 1
	fcReadDiscreteInputs     byte = 2
	fcReadHoldingRegisters   byte = 3
	fcReadInputRegisters     byte = 4
	fcWriteSingleCoil        byte = 5
	fcWriteSingleRegister    byte = 6
	fcWriteMultipleCoils     byte = 15
	fcWriteMultipleRegisters byte = 16
)

// errSerialTimeout 串口等待首字节超时（与平台实现返回的超时错误语义一致）
var errSerialTimeout = errors.New("serial: timeout")

// modbusError Modbus 异常响应（功能码 | 0x80）
type modbusError struct {
	FuncCode  byte
	Exception byte
}

func (e *modbusError) Error() string {
	return fmt.Sprintf("modbus: 异常响应 功能码=0x%02X 异常码=0x%02X(%s)", e.FuncCode, e.Exception, exceptionText(e.Exception))
}

func exceptionText(code byte) string {
	switch code {
	case 1:
		return "非法功能码"
	case 2:
		return "非法数据地址"
	case 3:
		return "非法数据值"
	case 4:
		return "从站设备故障"
	case 5:
		return "确认"
	case 6:
		return "从站设备忙"
	case 8:
		return "存储奇偶性差错"
	case 10:
		return "网关路径不可用"
	case 11:
		return "网关目标无响应"
	}
	return "未知异常"
}

// rtuMaster 单串口 RTU 主站，串行化所有请求（RTU 半双工总线）
type rtuMaster struct {
	port     io.ReadWriteCloser
	baudRate int
	mu       sync.Mutex

	hookMu sync.Mutex
	hook   *protocol.FrameGroup // 当前交互抓帧组
}

// setFrameHook 绑定本次交互的抓帧组（通信前后设置/清空）
func (m *rtuMaster) setFrameHook(g *protocol.FrameGroup) {
	m.hookMu.Lock()
	m.hook = g
	m.hookMu.Unlock()
}

// Close 关闭底层串口
func (m *rtuMaster) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.port == nil {
		return nil
	}
	err := m.port.Close()
	m.port = nil
	return err
}

func (m *rtuMaster) addFrame(dir string, b []byte) {
	m.hookMu.Lock()
	g := m.hook
	m.hookMu.Unlock()
	if g != nil {
		g.Add(dir, strings.ToUpper(fmt.Sprintf("% x", b)))
	}
}

// crc16 标准 Modbus CRC16（初值 0xFFFF，多项式 0xA001）
func crc16(bs []byte) uint16 {
	crc := uint16(0xFFFF)
	for _, b := range bs {
		crc ^= uint16(b)
		for i := 0; i < 8; i++ {
			if crc&1 != 0 {
				crc = (crc >> 1) ^ 0xA001
			} else {
				crc >>= 1
			}
		}
	}
	return crc
}

// frameDelay 估算帧间隔（3.5 字符 + 请求与响应传输时间），与 Modbus 串行规范一致
func (m *rtuMaster) frameDelay(chars int) time.Duration {
	var characterDelay, frameDelayUS int64 // 微秒
	if m.baudRate <= 0 || m.baudRate > 19200 {
		characterDelay = 750
		frameDelayUS = 1750
	} else {
		characterDelay = 15000000 / int64(m.baudRate)
		frameDelayUS = 35000000 / int64(m.baudRate)
	}
	return time.Duration(characterDelay*int64(chars)+frameDelayUS) * time.Microsecond
}

// responseLength 按请求 PDU 计算期望响应的完整 ADU 长度
func responseLength(adu []byte) int {
	length := rtuADUMinSize
	count := int(binary.BigEndian.Uint16(adu[4:6]))
	switch adu[1] {
	case fcReadDiscreteInputs, fcReadCoils:
		length += 1 + count/8
		if count%8 != 0 {
			length++
		}
	case fcReadInputRegisters, fcReadHoldingRegisters:
		length += 1 + count*2
	case fcWriteSingleCoil, fcWriteSingleRegister,
		fcWriteMultipleCoils, fcWriteMultipleRegisters:
		length += 4
	}
	return length
}

// sendADU 发送完整 RTU 请求帧并读取响应
func (m *rtuMaster) sendADU(adu []byte) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 发送（Write 全部字节成功后才记录 TX）
	n, err := m.port.Write(adu)
	if err != nil {
		return nil, fmt.Errorf("串口写入失败: %w", err)
	}
	if n != len(adu) {
		return nil, fmt.Errorf("串口写入不完整: 写入 %d/%d 字节", n, len(adu))
	}
	m.addFrame(protocol.FrameDirectionTX, adu)
	if builtin.Logger != nil {
		builtin.Logger.Debug("modbus sending", zap.String("hex", strings.ToUpper(fmt.Sprintf("% x", adu))))
	}

	// 帧间隔（等待从站响应窗口）
	wait := m.frameDelay(len(adu) + responseLength(adu))
	time.Sleep(wait)

	expectLen := responseLength(adu)
	function := adu[1]
	functionFail := adu[1] | 0x80

	var buf [rtuADUMaxSize]byte
	// 先读最小帧（站号+功能码+CRC）
	got, err := io.ReadAtLeast(m.port, buf[:], rtuADUMinSize)
	if err != nil {
		return nil, normalizeTimeout(err)
	}

	// 按功能码补齐剩余字节
	switch {
	case buf[1] == function:
		if got < expectLen {
			rest, err2 := io.ReadFull(m.port, buf[got:expectLen])
			got += rest
			if err2 != nil {
				return nil, normalizeTimeout(err2)
			}
		}
	case buf[1] == functionFail:
		// 异常响应固定 5 字节
		if got < 5 {
			rest, err2 := io.ReadFull(m.port, buf[got:5])
			got += rest
			if err2 != nil {
				return nil, normalizeTimeout(err2)
			}
		}
	default:
		resp := buf[:got]
		m.addFrame(protocol.FrameDirectionRX, resp)
		return nil, fmt.Errorf("modbus: 响应功能码不匹配 收到 0x%02X 期望 0x%02X", buf[1], function)
	}

	resp := buf[:got]
	m.addFrame(protocol.FrameDirectionRX, resp)
	if builtin.Logger != nil {
		builtin.Logger.Debug("modbus received", zap.String("hex", strings.ToUpper(fmt.Sprintf("% x", resp))))
	}

	// CRC 校验
	calc := crc16(resp[:got-2])
	want := binary.LittleEndian.Uint16(resp[got-2:])
	if calc != want {
		return nil, fmt.Errorf("modbus: 响应 CRC 错误 计算=%04X 帧内=%04X 帧=[% X]", calc, want, resp)
	}

	// 站号一致性
	if resp[0] != adu[0] {
		return nil, fmt.Errorf("modbus: 响应站号 0x%02X 与请求 0x%02X 不一致", resp[0], adu[0])
	}

	// 异常响应
	if resp[1] == functionFail {
		return nil, &modbusError{FuncCode: function, Exception: resp[2]}
	}
	return resp, nil
}

// normalizeTimeout 把各平台串口库的超时错误统一为 errSerialTimeout
func normalizeTimeout(err error) error {
	if errors.Is(err, errSerialTimeout) || err.Error() == "serial: timeout" {
		return errSerialTimeout
	}
	return err
}

// buildADU 组装 [slave, func, data..., CRC-LO, CRC-HI]
func buildADU(slaveID, funcCode byte, data []byte) []byte {
	adu := make([]byte, 0, len(data)+4)
	adu = append(adu, slaveID, funcCode)
	adu = append(adu, data...)
	c := crc16(adu)
	adu = append(adu, byte(c), byte(c>>8))
	return adu
}

func readAddressBody(address, quantity uint16) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint16(b[0:2], address)
	binary.BigEndian.PutUint16(b[2:4], quantity)
	return b
}

// ---- 读 ----

// readBits FC1/FC2，返回位打包字节
func (m *rtuMaster) readBits(funcCode, slaveID byte, address, quantity uint16) ([]byte, error) {
	resp, err := m.sendADU(buildADU(slaveID, funcCode, readAddressBody(address, quantity)))
	if err != nil {
		return nil, err
	}
	byteCount := int(resp[2])
	if len(resp) < 3+byteCount+2 {
		return nil, fmt.Errorf("modbus: 位读取响应长度不足")
	}
	return resp[3 : 3+byteCount], nil
}

func (m *rtuMaster) ReadCoils(slaveID byte, address, quantity uint16) ([]byte, error) {
	return m.readBits(fcReadCoils, slaveID, address, quantity)
}

func (m *rtuMaster) ReadDiscreteInputs(slaveID byte, address, quantity uint16) ([]byte, error) {
	return m.readBits(fcReadDiscreteInputs, slaveID, address, quantity)
}

// readRegisters FC3/FC4，返回寄存器数组
func (m *rtuMaster) readRegisters(funcCode, slaveID byte, address, quantity uint16) ([]uint16, error) {
	resp, err := m.sendADU(buildADU(slaveID, funcCode, readAddressBody(address, quantity)))
	if err != nil {
		return nil, err
	}
	byteCount := int(resp[2])
	if byteCount != int(quantity)*2 || len(resp) < 3+byteCount+2 {
		return nil, fmt.Errorf("modbus: 寄存器读取响应长度异常 byteCount=%d quantity=%d", byteCount, quantity)
	}
	regs := make([]uint16, quantity)
	for i := 0; i < int(quantity); i++ {
		regs[i] = binary.BigEndian.Uint16(resp[3+i*2:])
	}
	return regs, nil
}

func (m *rtuMaster) ReadHoldingRegisters(slaveID byte, address, quantity uint16) ([]uint16, error) {
	return m.readRegisters(fcReadHoldingRegisters, slaveID, address, quantity)
}

func (m *rtuMaster) ReadInputRegisters(slaveID byte, address, quantity uint16) ([]uint16, error) {
	return m.readRegisters(fcReadInputRegisters, slaveID, address, quantity)
}

// ---- 写 ----

func (m *rtuMaster) WriteSingleCoil(slaveID byte, address uint16, on bool) error {
	value := uint16(0x0000)
	if on {
		value = 0xFF00
	}
	body := make([]byte, 4)
	binary.BigEndian.PutUint16(body[0:2], address)
	binary.BigEndian.PutUint16(body[2:4], value)
	_, err := m.sendADU(buildADU(slaveID, fcWriteSingleCoil, body))
	return err
}

func (m *rtuMaster) WriteSingleRegister(slaveID byte, address, value uint16) error {
	body := make([]byte, 4)
	binary.BigEndian.PutUint16(body[0:2], address)
	binary.BigEndian.PutUint16(body[2:4], value)
	_, err := m.sendADU(buildADU(slaveID, fcWriteSingleRegister, body))
	return err
}

func (m *rtuMaster) WriteMultipleRegisters(slaveID byte, address, quantity uint16, values []uint16) error {
	body := make([]byte, 0, 5+len(values)*2)
	head := make([]byte, 5)
	binary.BigEndian.PutUint16(head[0:2], address)
	binary.BigEndian.PutUint16(head[2:4], quantity)
	head[4] = byte(len(values) * 2)
	body = append(body, head...)
	for _, v := range values {
		b := make([]byte, 2)
		binary.BigEndian.PutUint16(b, v)
		body = append(body, b...)
	}
	_, err := m.sendADU(buildADU(slaveID, fcWriteMultipleRegisters, body))
	return err
}

// WriteMultipleCoils 连续写多个线圈。bitValues 为未打包序列（每字节 0/1），内部按位打包。
func (m *rtuMaster) WriteMultipleCoils(slaveID byte, address, quantity uint16, bitValues []byte) error {
	if len(bitValues) < int(quantity) {
		return fmt.Errorf("modbus: 写多位数据不足 quantity=%d 实际=%d", quantity, len(bitValues))
	}
	byteCount := (int(quantity) + 7) / 8
	packed := make([]byte, byteCount)
	for i := 0; i < int(quantity); i++ {
		if bitValues[i] != 0 {
			packed[i/8] |= 1 << uint(i%8)
		}
	}
	body := make([]byte, 0, 5+byteCount)
	head := make([]byte, 5)
	binary.BigEndian.PutUint16(head[0:2], address)
	binary.BigEndian.PutUint16(head[2:4], quantity)
	head[4] = byte(byteCount)
	body = append(body, head...)
	body = append(body, packed...)
	_, err := m.sendADU(buildADU(slaveID, fcWriteMultipleCoils, body))
	return err
}
