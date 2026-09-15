//go:build windows

// Windows 串口实现。
//
// 背景：goburrow/serial 在 Windows 打开串口时 DCB 不设置 RTS/DTR，
// fRtsControl/fDtrControl 默认为 0（DISABLE）。大量 USB↔RS485 转换器
// （RTS 方向控制型、无源 232→485 转换器等）需要 RTS 为高才处于接收态，
// sscom 打开串口时默认置位 RTS/DTR。此处显式使能 RTS/DTR 并关闭所有软硬件
// 流控，与 sscom 默认行为对齐，避免“能发送、收不到响应”。
package modbusrtu

import (
	"fmt"
	"io"
	"math"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// DCB Flags 位掩码（winbase.h）
const (
	dcbFlagBinary        = 0x00000001
	dcbFlagParity        = 0x00000002
	dcbFlagDtrControlPos = 4  // fDtrControl 占 bit4-5，1 = DTR_CONTROL_ENABLE
	dcbFlagRtsControlPos = 12 // fRtsControl 占 bit12-13，1 = RTS_CONTROL_ENABLE
	dcbFlagDtrEnable     = 1 << dcbFlagDtrControlPos
	dcbFlagRtsEnable     = 1 << dcbFlagRtsControlPos
)

type windowsSerial struct {
	handle windows.Handle
}

func openRTUSerial(portName string, baudRate, dataBits, stopBits int, parity string, timeout time.Duration) (io.ReadWriteCloser, error) {
	// \\.\COM10 以上端口必须带 \\.\ 前缀，统一加上对低端口也无害
	path, err := windows.UTF16PtrFromString(`\\.\` + portName)
	if err != nil {
		return nil, fmt.Errorf("串口名无效 %q: %w", portName, err)
	}

	handle, err := windows.CreateFile(
		path,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		0,   // 独占
		nil, // 默认安全属性
		windows.OPEN_EXISTING,
		0, // 非 overlapped，阻塞读写
		0)
	if err != nil {
		return nil, fmt.Errorf("打开串口 %s 失败: %w", portName, err)
	}

	ok := false
	defer func() {
		if !ok {
			windows.CloseHandle(handle)
		}
	}()

	// 先取当前 DCB 保留未用字段，再覆盖关键设置
	var dcb windows.DCB
	dcb.DCBlength = uint32(unsafe.Sizeof(dcb))
	if err := windows.GetCommState(handle, &dcb); err != nil {
		return nil, fmt.Errorf("获取串口状态失败: %w", err)
	}

	dcb.BaudRate = uint32(baudRate)
	dcb.ByteSize = uint8(dataBits)

	switch stopBits {
	case 1:
		dcb.StopBits = windows.ONESTOPBIT
	case 2:
		dcb.StopBits = windows.TWOSTOPBITS
	default:
		return nil, fmt.Errorf("不支持的停止位: %d", stopBits)
	}

	// Flags：仅保留二进制模式/校验位/RTS,DTR 使能，显式不带任何流控位
	flags := uint32(dcbFlagBinary | dcbFlagDtrEnable | dcbFlagRtsEnable)
	switch parity {
	case "N":
		dcb.Parity = windows.NOPARITY
	case "E":
		dcb.Parity = windows.EVENPARITY
		flags |= dcbFlagParity
	case "O":
		dcb.Parity = windows.ODDPARITY
		flags |= dcbFlagParity
	default:
		return nil, fmt.Errorf("不支持的校验位: %s", parity)
	}
	dcb.Flags = flags

	if err := windows.SetCommState(handle, &dcb); err != nil {
		return nil, fmt.Errorf("设置串口参数失败: %w", err)
	}

	// 超时：与 goburrow 行为一致——收到首字节后立即返回缓冲内已有数据，
	// 一个字节都没有时等待 timeout。配合 io.ReadAtLeast 逐字节拼帧。
	timeoutMs := uint32(timeout / time.Millisecond)
	if timeoutMs == 0 {
		timeoutMs = 1000
	}
	commTimeouts := windows.CommTimeouts{
		ReadIntervalTimeout:        math.MaxUint32,
		ReadTotalTimeoutMultiplier: math.MaxUint32,
		ReadTotalTimeoutConstant:   timeoutMs,
		WriteTotalTimeoutConstant:  timeoutMs,
	}
	if err := windows.SetCommTimeouts(handle, &commTimeouts); err != nil {
		return nil, fmt.Errorf("设置串口超时失败: %w", err)
	}

	// 显式拉高 DTR/RTS（部分驱动只认 EscapeCommFunction）
	// 对自动收发方向的转换器无副作用；对 RTS 方向型转换器保证默认处于接收态。
	_ = windows.EscapeCommFunction(handle, windows.SETDTR)
	_ = windows.EscapeCommFunction(handle, windows.SETRTS)

	p := &windowsSerial{handle: handle}
	ok = true
	return p, nil
}

func (p *windowsSerial) Read(b []byte) (int, error) {
	var done uint32
	if err := windows.ReadFile(p.handle, b, &done, nil); err != nil {
		return int(done), err
	}
	if done == 0 {
		return 0, errSerialTimeout
	}
	return int(done), nil
}

func (p *windowsSerial) Write(b []byte) (int, error) {
	var done uint32
	if err := windows.WriteFile(p.handle, b, &done, nil); err != nil {
		return int(done), err
	}
	return int(done), nil
}

func (p *windowsSerial) Close() error {
	if p.handle == 0 || p.handle == windows.InvalidHandle {
		return nil
	}
	_ = windows.EscapeCommFunction(p.handle, windows.CLRDTR)
	_ = windows.EscapeCommFunction(p.handle, windows.CLRRTS)
	err := windows.CloseHandle(p.handle)
	p.handle = windows.InvalidHandle
	return err
}
