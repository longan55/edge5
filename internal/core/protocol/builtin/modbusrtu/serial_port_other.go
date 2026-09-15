//go:build !windows

// 非 Windows 平台串口：沿用 goburrow/serial（Linux termios 使用 CLOCAL 忽略
// 调制解调器控制线，自动收发方向的 RS485 转换器无需 RTS 控制）。
package modbusrtu

import (
	"io"
	"time"

	"github.com/goburrow/serial"
)

func openRTUSerial(portName string, baudRate, dataBits, stopBits int, parity string, timeout time.Duration) (io.ReadWriteCloser, error) {
	p, err := serial.Open(&serial.Config{
		Address:  portName,
		BaudRate: baudRate,
		DataBits: dataBits,
		StopBits: stopBits,
		Parity:   parity,
		Timeout:  timeout,
	})
	if err != nil {
		return nil, err
	}
	return p, nil
}
