package a1eprotocol

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"net"
	"strconv"
	"strings"
	"sync"

	"edge5/internal/core/protocol"
	"edge5/internal/core/protocol/builtin"

	"go.uber.org/zap"
)

func init() {
	if err := builtin.Register(NewA1EService()); err != nil {
		panic(fmt.Sprintf("注册 A1E 协议失败: %v", err))
	}
}

type MelsecA1EDataType struct {
	DataCode uint16
	DataType byte
	AsciiCode string
	FromBase int
}

var (
	A1E_X   = &MelsecA1EDataType{DataCode: 22560, DataType: 1, AsciiCode: "X*", FromBase: 16}
	A1E_Y   = &MelsecA1EDataType{DataCode: 22816, DataType: 1, AsciiCode: "Y*", FromBase: 16}
	A1E_M   = &MelsecA1EDataType{DataCode: 19744, DataType: 1, AsciiCode: "M*", FromBase: 10}
	A1E_S   = &MelsecA1EDataType{DataCode: 21280, DataType: 1, AsciiCode: "S*", FromBase: 10}
	A1E_F   = &MelsecA1EDataType{DataCode: 17952, DataType: 1, AsciiCode: "F*", FromBase: 10}
	A1E_B   = &MelsecA1EDataType{DataCode: 16928, DataType: 1, AsciiCode: "B*", FromBase: 16}
	A1E_TS  = &MelsecA1EDataType{DataCode: 21587, DataType: 1, AsciiCode: "TS", FromBase: 10}
	A1E_TC  = &MelsecA1EDataType{DataCode: 21571, DataType: 1, AsciiCode: "TC", FromBase: 10}
	A1E_TN  = &MelsecA1EDataType{DataCode: 21582, DataType: 0, AsciiCode: "TN", FromBase: 10}
	A1E_CS  = &MelsecA1EDataType{DataCode: 17235, DataType: 1, AsciiCode: "CS", FromBase: 10}
	A1E_CC  = &MelsecA1EDataType{DataCode: 17219, DataType: 1, AsciiCode: "CC", FromBase: 10}
	A1E_CN  = &MelsecA1EDataType{DataCode: 17230, DataType: 0, AsciiCode: "CN", FromBase: 10}
	A1E_D   = &MelsecA1EDataType{DataCode: 17440, DataType: 0, AsciiCode: "D*", FromBase: 10}
	A1E_W   = &MelsecA1EDataType{DataCode: 22304, DataType: 0, AsciiCode: "W*", FromBase: 16}
	A1E_R   = &MelsecA1EDataType{DataCode: 21024, DataType: 0, AsciiCode: "R*", FromBase: 10}
)

type A1EService struct {
	lock             sync.Mutex
	connected        bool
	deviceID         uint
	connectionParams map[uint]protocol.Metadata
}

func NewA1EService() *A1EService {
	return &A1EService{
		connectionParams: make(map[uint]protocol.Metadata),
	}
}

func (m *A1EService) Info() protocol.Metadata {
	return protocol.Metadata{
		"name":          "MC-1E",
		"alias":         "MelsecA1E",
		"version":       "1.0.0",
		"description":   "三菱 PLC A1E 兼容协议（FX3U系列）",
		"group":         "builtin",
		"source":        "builtin",
		"deviceType":    "PLC",
		"brand":         "Mitsubishi",
		"cName":         "三菱",
		"supportServer": true,
		"supportDebug":  true,
		"models":        []string{"FX3U"},
		"connectionParams": []any{
			protocol.Metadata{"name": "ip", "cName": "IP 地址", "type": "string", "required": true, "default": ""},
			protocol.Metadata{"name": "port", "cName": "端口", "type": "int", "required": true, "default": "6000"},
			protocol.Metadata{"name": "plcNumber", "cName": "PLC 编号", "type": "string", "required": false, "default": "FF"},
		},
		"readParamsSchema": []any{
			protocol.Metadata{"name": "address", "cName": "地址", "type": "string", "required": true, "default": "M100"},
			protocol.Metadata{"name": "offset", "cName": "偏移量", "type": "int", "required": false, "default": "1"},
			protocol.Metadata{"name": "parseType", "cName": "解析类型", "type": "select", "required": true, "default": "bool", "choices": []string{"bool", "short", "ushort", "int", "uint", "long", "ulong", "float", "double", "string"}},
		},
	}
}

func (m *A1EService) IsSupportServer() bool {
	return true
}

func (m *A1EService) IsConnected(handle protocol.DeviceHandle) bool {
	m.lock.Lock()
	defer m.lock.Unlock()
	devID := uint(handle)
	if devID == 0 {
		devID = m.deviceID
	}
	params, ok := m.connectionParams[devID]
	if !ok {
		return false
	}
	ip := builtin.GetString(params, "ip")
	if ip == "" {
		return false
	}
	port := builtin.GetInt(params, "port")
	if port <= 0 {
		port = 6000
	}
	tcpAddr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%v:%v", ip, port))
	if err != nil {
		return false
	}
	conn, err := net.DialTCP("tcp", nil, tcpAddr)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func (m *A1EService) Connect(ctx context.Context, params protocol.Metadata) (protocol.DeviceHandle, error) {
	m.lock.Lock()
	defer m.lock.Unlock()

	deviceID := uint(builtin.GetInt(params, "deviceID"))
	if deviceID == 0 {
		return protocol.InvalidHandle, fmt.Errorf("缺少 deviceID 参数")
	}

	ip := builtin.GetString(params, "ip")
	port := builtin.GetInt(params, "port")
	if port <= 0 {
		port = 6000
	}

	if ip == "" {
		return protocol.InvalidHandle, fmt.Errorf("缺少 IP 地址参数")
	}

	tcpAddr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%v:%v", ip, port))
	if err != nil {
		return protocol.InvalidHandle, err
	}
	conn, err := net.DialTCP("tcp", nil, tcpAddr)
	if err != nil {
		return protocol.InvalidHandle, fmt.Errorf("连接 A1E 客户端失败: %w", err)
	}
	conn.Close()

	m.connectionParams[deviceID] = params
	m.deviceID = deviceID
	m.connected = true

	if builtin.Logger != nil {
		builtin.Logger.Info("A1E协议: 连接测试成功",
			zap.Uint("deviceID", deviceID),
			zap.String("ip", ip),
			zap.Int("port", port),
		)
	}
	return protocol.DeviceHandle(deviceID), nil
}

func (m *A1EService) Disconnect(ctx context.Context, handle protocol.DeviceHandle) error {
	return nil
}

func (m *A1EService) ReadBatch(ctx context.Context, handle protocol.DeviceHandle, req protocol.BatchReadRequest) (*protocol.BatchReadResponse, error) {
	m.lock.Lock()
	devID := uint(handle)
	if devID == 0 {
		devID = m.deviceID
	}
	params := m.connectionParams[devID]
	m.lock.Unlock()

	if params == nil {
		return nil, fmt.Errorf("设备未连接")
	}

	ip := builtin.GetString(params, "ip")
	port := builtin.GetInt(params, "port")
	if port <= 0 {
		port = 6000
	}
	plcNumberStr := builtin.GetString(params, "plcNumber")
	var plcNumber byte = 0xFF
	if plcNumberStr != "" {
		n, err := strconv.ParseUint(plcNumberStr, 16, 8)
		if err == nil {
			plcNumber = byte(n)
		}
	}

	if ip == "" {
		return nil, fmt.Errorf("缺少 IP 地址参数")
	}

	tcpAddr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%v:%v", ip, port))
	if err != nil {
		return nil, fmt.Errorf("解析地址失败: %w", err)
	}
	conn, err := net.DialTCP("tcp", nil, tcpAddr)
	if err != nil {
		return nil, fmt.Errorf("连接 PLC 失败: %w", err)
	}
	defer conn.Close()

	results := make([]protocol.BatchReadResult, 0, len(req.Points))
	for _, pt := range req.Points {
		result := m.readPoint(conn, plcNumber, pt)
		results = append(results, result)
	}

	return &protocol.BatchReadResponse{
		Results: results,
	}, nil
}

func (m *A1EService) readPoint(conn *net.TCPConn, plcNumber byte, pt protocol.Point) protocol.BatchReadResult {
	count := pt.Count
	if count <= 0 {
		count = 1
	}

	var value interface{}
	var err error

	switch pt.DataType {
	case "bool":
		value, err = m.readBoolValue(conn, plcNumber, pt.Resource, count)
	case "byte":
		value, err = m.readByteValue(conn, plcNumber, pt.Resource, count)
	case "int16", "short":
		value, err = m.readShortValue(conn, plcNumber, pt.Resource, count)
	case "uint16", "ushort":
		value, err = m.readUShortValue(conn, plcNumber, pt.Resource, count)
	case "int32", "int":
		value, err = m.readIntValue(conn, plcNumber, pt.Resource, count)
	case "uint32", "uint":
		value, err = m.readUIntValue(conn, plcNumber, pt.Resource, count)
	case "int64", "long":
		value, err = m.readLongValue(conn, plcNumber, pt.Resource, count)
	case "uint64", "ulong":
		value, err = m.readULongValue(conn, plcNumber, pt.Resource, count)
	case "float32", "float":
		value, err = m.readFloatValue(conn, plcNumber, pt.Resource, count)
	case "float64", "double":
		value, err = m.readDoubleValue(conn, plcNumber, pt.Resource, count)
	case "string":
		value, err = m.readStringValue(conn, plcNumber, pt.Resource, count)
	default:
		err = fmt.Errorf("不支持的数据类型: %s", pt.DataType)
	}

	quality := "good"
	if err != nil {
		quality = "bad"
	}

	return protocol.BatchReadResult{
		PointName: pt.Resource,
		Value:     value,
		Quality:   quality,
		Error:     err,
	}
}

func (m *A1EService) WriteBatch(ctx context.Context, handle protocol.DeviceHandle, req protocol.BatchWriteRequest) error {
	m.lock.Lock()
	devID := uint(handle)
	if devID == 0 {
		devID = m.deviceID
	}
	params := m.connectionParams[devID]
	m.lock.Unlock()

	if params == nil {
		return fmt.Errorf("设备未连接")
	}

	ip := builtin.GetString(params, "ip")
	port := builtin.GetInt(params, "port")
	if port <= 0 {
		port = 6000
	}
	plcNumberStr := builtin.GetString(params, "plcNumber")
	var plcNumber byte = 0xFF
	if plcNumberStr != "" {
		n, err := strconv.ParseUint(plcNumberStr, 16, 8)
		if err == nil {
			plcNumber = byte(n)
		}
	}

	if ip == "" {
		return fmt.Errorf("缺少 IP 地址参数")
	}

	tcpAddr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%v:%v", ip, port))
	if err != nil {
		return fmt.Errorf("解析地址失败: %w", err)
	}
	conn, err := net.DialTCP("tcp", nil, tcpAddr)
	if err != nil {
		return fmt.Errorf("连接 PLC 失败: %w", err)
	}
	defer conn.Close()

	for _, item := range req.Items {
		if err := m.writePoint(conn, plcNumber, item); err != nil {
			return fmt.Errorf("写入点位 %s 失败: %w", item.Point.Name, err)
		}
	}
	return nil
}

func (m *A1EService) writePoint(conn *net.TCPConn, plcNumber byte, item protocol.BatchWriteItem) error {
	pt := item.Point
	count := pt.Count
	if count <= 0 {
		count = 1
	}

	switch pt.DataType {
	case "bool":
		return m.writeBoolValue(conn, plcNumber, pt.Resource, pt.Count, item.Value)
	case "byte":
		return m.writeIntValue(conn, plcNumber, pt.Resource, 1, item.Value)
	case "int16", "short":
		return m.writeIntValue(conn, plcNumber, pt.Resource, 2, item.Value)
	case "uint16", "ushort":
		return m.writeIntValue(conn, plcNumber, pt.Resource, 2, item.Value)
	case "int32", "int":
		return m.writeIntValue(conn, plcNumber, pt.Resource, 4, item.Value)
	case "uint32", "uint":
		return m.writeIntValue(conn, plcNumber, pt.Resource, 4, item.Value)
	case "int64", "long":
		return m.writeIntValue(conn, plcNumber, pt.Resource, 8, item.Value)
	case "uint64", "ulong":
		return m.writeIntValue(conn, plcNumber, pt.Resource, 8, item.Value)
	case "float32", "float":
		return m.writeFloatValue(conn, plcNumber, pt.Resource, count, item.Value)
	case "float64", "double":
		return m.writeDoubleValue(conn, plcNumber, pt.Resource, count, item.Value)
	case "string":
		return m.writeStringValue(conn, plcNumber, pt.Resource, count, item.Value)
	default:
		return fmt.Errorf("不支持的数据类型: %s", pt.DataType)
	}
}

func McA1EAnalysisAddress(address string) (*MelsecA1EDataType, int, error) {
	if len(address) == 0 {
		return nil, 0, fmt.Errorf("地址格式无效")
	}

	switch strings.ToUpper(address[0:1]) {
	case "T":
		if len(address) >= 2 {
			switch strings.ToUpper(address[1:2]) {
			case "S":
				offset, err := strconv.ParseInt(address[2:], 10, 64)
				if err != nil {
					return nil, 0, fmt.Errorf("地址解析失败: %v", err)
				}
				return A1E_TS, int(offset), nil
			case "C":
				offset, err := strconv.ParseInt(address[2:], 10, 64)
				if err != nil {
					return nil, 0, fmt.Errorf("地址解析失败: %v", err)
				}
				return A1E_TC, int(offset), nil
			case "N":
				offset, err := strconv.ParseInt(address[2:], 10, 64)
				if err != nil {
					return nil, 0, fmt.Errorf("地址解析失败: %v", err)
				}
				return A1E_TN, int(offset), nil
			}
		}
		return nil, 0, fmt.Errorf("不支持的数据类型: %s", address)
	case "C":
		if len(address) >= 2 {
			switch strings.ToUpper(address[1:2]) {
			case "S":
				offset, err := strconv.ParseInt(address[2:], 10, 64)
				if err != nil {
					return nil, 0, fmt.Errorf("地址解析失败: %v", err)
				}
				return A1E_CS, int(offset), nil
			case "C":
				offset, err := strconv.ParseInt(address[2:], 10, 64)
				if err != nil {
					return nil, 0, fmt.Errorf("地址解析失败: %v", err)
				}
				return A1E_CC, int(offset), nil
			case "N":
				offset, err := strconv.ParseInt(address[2:], 10, 64)
				if err != nil {
					return nil, 0, fmt.Errorf("地址解析失败: %v", err)
				}
				return A1E_CN, int(offset), nil
			}
		}
		return nil, 0, fmt.Errorf("不支持的数据类型: %s", address)
	case "X":
		addr := address[1:]
		var offset int
		var err error
		if strings.HasPrefix(addr, "0") {
			offset, err = strconv.Atoi(addr)
			if err != nil {
				offset64, err2 := strconv.ParseInt(addr, 8, 64)
				if err2 != nil {
					return nil, 0, fmt.Errorf("地址解析失败: %v", err2)
				}
				offset = int(offset64)
			}
		} else {
			offset64, err := strconv.ParseInt(addr, 16, 64)
			if err != nil {
				return nil, 0, fmt.Errorf("地址解析失败: %v", err)
			}
			offset = int(offset64)
		}
		return A1E_X, offset, nil
	case "Y":
		addr := address[1:]
		var offset int
		var err error
		if strings.HasPrefix(addr, "0") {
			offset, err = strconv.Atoi(addr)
			if err != nil {
				offset64, err2 := strconv.ParseInt(addr, 8, 64)
				if err2 != nil {
					return nil, 0, fmt.Errorf("地址解析失败: %v", err2)
				}
				offset = int(offset64)
			}
		} else {
			offset64, err := strconv.ParseInt(addr, 16, 64)
			if err != nil {
				return nil, 0, fmt.Errorf("地址解析失败: %v", err)
			}
			offset = int(offset64)
		}
		return A1E_Y, offset, nil
	case "M":
		offset, err := strconv.ParseInt(address[1:], 10, 64)
		if err != nil {
			return nil, 0, fmt.Errorf("地址解析失败: %v", err)
		}
		return A1E_M, int(offset), nil
	case "S":
		offset, err := strconv.ParseInt(address[1:], 10, 64)
		if err != nil {
			return nil, 0, fmt.Errorf("地址解析失败: %v", err)
		}
		return A1E_S, int(offset), nil
	case "F":
		offset, err := strconv.ParseInt(address[1:], 10, 64)
		if err != nil {
			return nil, 0, fmt.Errorf("地址解析失败: %v", err)
		}
		return A1E_F, int(offset), nil
	case "B":
		offset, err := strconv.ParseInt(address[1:], 16, 64)
		if err != nil {
			return nil, 0, fmt.Errorf("地址解析失败: %v", err)
		}
		return A1E_B, int(offset), nil
	case "D":
		offset, err := strconv.ParseInt(address[1:], 10, 64)
		if err != nil {
			return nil, 0, fmt.Errorf("地址解析失败: %v", err)
		}
		return A1E_D, int(offset), nil
	case "R":
		offset, err := strconv.ParseInt(address[1:], 10, 64)
		if err != nil {
			return nil, 0, fmt.Errorf("地址解析失败: %v", err)
		}
		return A1E_R, int(offset), nil
	case "W":
		offset, err := strconv.ParseInt(address[1:], 16, 64)
		if err != nil {
			return nil, 0, fmt.Errorf("地址解析失败: %v", err)
		}
		return A1E_W, int(offset), nil
	default:
		return nil, 0, fmt.Errorf("不支持的数据类型: %s", address)
	}
}

func BuildReadCommand(address string, length int, isBit bool, plcNumber byte) ([][]byte, error) {
	dataType, offset, err := McA1EAnalysisAddress(address)
	if err != nil {
		return nil, err
	}

	cmdCode := byte(1)
	if isBit {
		cmdCode = byte(0)
	}

	maxLen := 64
	if isBit {
		maxLen = 256
	}

	var commands [][]byte
	currentOffset := offset

	for length > 0 {
		readLen := length
		if readLen > maxLen {
			readLen = maxLen
		}

		cmd := make([]byte, 12)
		cmd[0] = cmdCode
		cmd[1] = plcNumber
		cmd[2] = 10
		cmd[3] = 0

		offsetBytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(offsetBytes, uint32(currentOffset))
		copy(cmd[4:8], offsetBytes)

		dataCodeBytes := make([]byte, 2)
		binary.LittleEndian.PutUint16(dataCodeBytes, dataType.DataCode)
		copy(cmd[8:10], dataCodeBytes)

		if readLen == 256 {
			cmd[10] = 0
			cmd[11] = 0
		} else {
			lenBytes := make([]byte, 2)
			binary.LittleEndian.PutUint16(lenBytes, uint16(readLen))
			copy(cmd[10:12], lenBytes)
		}

		commands = append(commands, cmd)
		currentOffset += readLen
		length -= readLen
	}

	return commands, nil
}

func BuildWriteWordCommand(address string, value []byte, plcNumber byte) ([]byte, error) {
	dataType, offset, err := McA1EAnalysisAddress(address)
	if err != nil {
		return nil, err
	}

	cmd := make([]byte, 12+len(value))
	cmd[0] = 3
	cmd[1] = plcNumber
	cmd[2] = 10
	cmd[3] = 0

	offsetBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(offsetBytes, uint32(offset))
	copy(cmd[4:8], offsetBytes)

	dataCodeBytes := make([]byte, 2)
	binary.LittleEndian.PutUint16(dataCodeBytes, dataType.DataCode)
	copy(cmd[8:10], dataCodeBytes)

	numPoints := len(value) / 2
	lenBytes := make([]byte, 2)
	binary.LittleEndian.PutUint16(lenBytes, uint16(numPoints))
	copy(cmd[10:12], lenBytes)

	copy(cmd[12:], value)
	return cmd, nil
}

func BuildWriteBoolCommand(address string, value []byte, plcNumber byte) ([]byte, error) {
	dataType, offset, err := McA1EAnalysisAddress(address)
	if err != nil {
		return nil, err
	}

	boolArray := make([]bool, len(value))
	for i, v := range value {
		boolArray[i] = v != 0
	}

	compressed := TransBoolArrayToByteData(boolArray)
	cmd := make([]byte, 12+len(compressed))
	cmd[0] = 2
	cmd[1] = plcNumber
	cmd[2] = 10
	cmd[3] = 0

	offsetBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(offsetBytes, uint32(offset))
	copy(cmd[4:8], offsetBytes)

	dataCodeBytes := make([]byte, 2)
	binary.LittleEndian.PutUint16(dataCodeBytes, dataType.DataCode)
	copy(cmd[8:10], dataCodeBytes)

	lenBytes := make([]byte, 2)
	binary.LittleEndian.PutUint16(lenBytes, uint16(len(boolArray)))
	copy(cmd[10:12], lenBytes)

	copy(cmd[12:], compressed)
	return cmd, nil
}

func CheckResponseLegal(response []byte) error {
	if len(response) < 2 {
		return fmt.Errorf("接收数据长度过短")
	}
	if response[1] == 0 {
		return nil
	}
	if response[1] == 91 {
		return fmt.Errorf("PLC 错误: 请参考手册文档 (错误码: %d)", response[2])
	}
	return fmt.Errorf("PLC 错误: 请参考手册文档 (错误码: %d)", response[1])
}

func ExtractActualData(response []byte, isBit bool) ([]byte, error) {
	if isBit {
		result := make([]byte, (len(response)-2)*2)
		for i := 2; i < len(response); i++ {
			if (response[i] & 0x10) == 16 {
				result[(i-2)*2] = 1
			}
			if (response[i] & 1) == 1 {
				result[(i-2)*2+1] = 1
			}
		}
		return result, nil
	}
	return response[2:], nil
}

func TransBoolArrayToByteData(value []bool) []byte {
	num := (len(value) + 1) / 2
	result := make([]byte, num)
	for i := 0; i < num; i++ {
		if value[i*2] {
			result[i] += 16
		}
		if i*2+1 < len(value) && value[i*2+1] {
			result[i]++
		}
	}
	return result
}

func ReadA1E(conn *net.TCPConn, address string, numPoints int, isBit bool, plcNumber byte) ([]byte, error) {
	commands, err := BuildReadCommand(address, numPoints, isBit, plcNumber)
	if err != nil {
		return nil, fmt.Errorf("构建读取命令失败: %w", err)
	}

	var result []byte
	for _, cmd := range commands {
		_, err := conn.Write(cmd)
		if err != nil {
			return nil, fmt.Errorf("发送命令失败: %w", err)
		}

		buf := make([]byte, 1024)
		n, err := conn.Read(buf)
		if err != nil {
			return nil, fmt.Errorf("接收数据失败: %w", err)
		}

		response := buf[:n]
		if err := CheckResponseLegal(response); err != nil {
			return nil, err
		}

		data, err := ExtractActualData(response, isBit)
		if err != nil {
			return nil, err
		}
		result = append(result, data...)
	}

	return result, nil
}

func WriteA1EWord(conn *net.TCPConn, address string, value []byte, plcNumber byte) error {
	cmd, err := BuildWriteWordCommand(address, value, plcNumber)
	if err != nil {
		return fmt.Errorf("构建写入命令失败: %w", err)
	}

	_, err = conn.Write(cmd)
	if err != nil {
		return fmt.Errorf("发送命令失败: %w", err)
	}

	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		return fmt.Errorf("接收响应失败: %w", err)
	}

	response := buf[:n]
	return CheckResponseLegal(response)
}

func WriteA1EBool(conn *net.TCPConn, address string, value []byte, plcNumber byte) error {
	cmd, err := BuildWriteBoolCommand(address, value, plcNumber)
	if err != nil {
		return fmt.Errorf("构建写入命令失败: %w", err)
	}

	_, err = conn.Write(cmd)
	if err != nil {
		return fmt.Errorf("发送命令失败: %w", err)
	}

	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		return fmt.Errorf("接收响应失败: %w", err)
	}

	response := buf[:n]
	return CheckResponseLegal(response)
}

func (m *A1EService) readBoolValue(conn *net.TCPConn, plcNumber byte, address string, length int) (interface{}, error) {
	rawData, err := ReadA1E(conn, address, length, true, plcNumber)
	if err != nil {
		return nil, err
	}
	if length == 1 {
		return rawData[0]&0x01 == 0x01, nil
	}
	bools := make([]bool, length)
	for j := 0; j < length; j++ {
		if j < len(rawData) {
			bools[j] = rawData[j] != 0
		}
	}
	return bools, nil
}

func (m *A1EService) readByteValue(conn *net.TCPConn, plcNumber byte, address string, length int) (interface{}, error) {
	// A1E 按 word(2字节) 读取，1 byte 实际读 1 个 word，取低字节
	rawData, err := ReadA1E(conn, address, length, false, plcNumber)
	if err != nil {
		return nil, err
	}
	if length == 1 {
		return rawData[0], nil
	}
	result := make([]byte, 0, length)
	for i := 0; i < length; i++ {
		idx := i * 2
		if idx < len(rawData) {
			result = append(result, rawData[idx])
		}
	}
	return result, nil
}

func (m *A1EService) readShortValue(conn *net.TCPConn, plcNumber byte, address string, length int) (interface{}, error) {
	// A1E 按 word(2字节) 为单位读取，length 个 short 对应 length 个 word
	rawData, err := ReadA1E(conn, address, length, false, plcNumber)
	if err != nil {
		return nil, err
	}
	if length == 1 {
		return int16(Bin2Int(rawData[0:2], binary.LittleEndian)), nil
	}
	vals := make([]int16, 0, length)
	for i := 0; i < length; i++ {
		idx := i * 2
		if idx+2 > len(rawData) {
			break
		}
		vals = append(vals, int16(Bin2Int(rawData[idx:idx+2], binary.LittleEndian)))
	}
	return vals, nil
}

func (m *A1EService) readUShortValue(conn *net.TCPConn, plcNumber byte, address string, length int) (interface{}, error) {
	// A1E 按 word(2字节) 为单位读取，length 个 ushort 对应 length 个 word
	rawData, err := ReadA1E(conn, address, length, false, plcNumber)
	if err != nil {
		return nil, err
	}
	if length == 1 {
		return uint16(Bin2Int(rawData[0:2], binary.LittleEndian)), nil
	}
	vals := make([]uint16, 0, length)
	for i := 0; i < length; i++ {
		idx := i * 2
		if idx+2 > len(rawData) {
			break
		}
		vals = append(vals, uint16(Bin2Int(rawData[idx:idx+2], binary.LittleEndian)))
	}
	return vals, nil
}

func (m *A1EService) readIntValue(conn *net.TCPConn, plcNumber byte, address string, length int) (interface{}, error) {
	// int32 占 2 个 word，length 个 int32 对应 length*2 个 word
	rawData, err := ReadA1E(conn, address, length*2, false, plcNumber)
	if err != nil {
		return nil, err
	}
	if length == 1 {
		return int32(Bin2Int(rawData[0:4], binary.LittleEndian)), nil
	}
	vals := make([]int32, 0, length)
	for i := 0; i < length; i++ {
		idx := i * 4
		if idx+4 > len(rawData) {
			break
		}
		vals = append(vals, int32(Bin2Int(rawData[idx:idx+4], binary.LittleEndian)))
	}
	return vals, nil
}

func (m *A1EService) readUIntValue(conn *net.TCPConn, plcNumber byte, address string, length int) (interface{}, error) {
	// uint32 占 2 个 word，length 个 uint32 对应 length*2 个 word
	rawData, err := ReadA1E(conn, address, length*2, false, plcNumber)
	if err != nil {
		return nil, err
	}
	if length == 1 {
		return uint32(Bin2Int(rawData[0:4], binary.LittleEndian)), nil
	}
	vals := make([]uint32, 0, length)
	for i := 0; i < length; i++ {
		idx := i * 4
		if idx+4 > len(rawData) {
			break
		}
		vals = append(vals, uint32(Bin2Int(rawData[idx:idx+4], binary.LittleEndian)))
	}
	return vals, nil
}

func (m *A1EService) readLongValue(conn *net.TCPConn, plcNumber byte, address string, length int) (interface{}, error) {
	// int64 占 4 个 word，length 个 int64 对应 length*4 个 word
	rawData, err := ReadA1E(conn, address, length*4, false, plcNumber)
	if err != nil {
		return nil, err
	}
	if length == 1 {
		return int64(Bin2Int(rawData[0:8], binary.LittleEndian)), nil
	}
	vals := make([]int64, 0, length)
	for i := 0; i < length; i++ {
		idx := i * 8
		if idx+8 > len(rawData) {
			break
		}
		vals = append(vals, int64(Bin2Int(rawData[idx:idx+8], binary.LittleEndian)))
	}
	return vals, nil
}

func (m *A1EService) readULongValue(conn *net.TCPConn, plcNumber byte, address string, length int) (interface{}, error) {
	// uint64 占 4 个 word，length 个 uint64 对应 length*4 个 word
	rawData, err := ReadA1E(conn, address, length*4, false, plcNumber)
	if err != nil {
		return nil, err
	}
	if length == 1 {
		return uint64(Bin2Int(rawData[0:8], binary.LittleEndian)), nil
	}
	vals := make([]uint64, 0, length)
	for i := 0; i < length; i++ {
		idx := i * 8
		if idx+8 > len(rawData) {
			break
		}
		vals = append(vals, uint64(Bin2Int(rawData[idx:idx+8], binary.LittleEndian)))
	}
	return vals, nil
}

func (m *A1EService) readFloatValue(conn *net.TCPConn, plcNumber byte, address string, length int) (interface{}, error) {
	// float32 占 2 个 word，length 个 float32 对应 length*2 个 word
	rawData, err := ReadA1E(conn, address, length*2, false, plcNumber)
	if err != nil {
		return nil, err
	}
	if length == 1 {
		return math.Float32frombits(binary.LittleEndian.Uint32(rawData[0:4])), nil
	}
	vals := make([]float32, 0, length)
	for i := 0; i < length; i++ {
		idx := i * 4
		if idx+4 > len(rawData) {
			break
		}
		vals = append(vals, math.Float32frombits(binary.LittleEndian.Uint32(rawData[idx:idx+4])))
	}
	return vals, nil
}

func (m *A1EService) readDoubleValue(conn *net.TCPConn, plcNumber byte, address string, length int) (interface{}, error) {
	// float64 占 4 个 word，length 个 float64 对应 length*4 个 word
	rawData, err := ReadA1E(conn, address, length*4, false, plcNumber)
	if err != nil {
		return nil, err
	}
	if length == 1 {
		return math.Float64frombits(binary.LittleEndian.Uint64(rawData[0:8])), nil
	}
	vals := make([]float64, 0, length)
	for i := 0; i < length; i++ {
		idx := i * 8
		if idx+8 > len(rawData) {
			break
		}
		vals = append(vals, math.Float64frombits(binary.LittleEndian.Uint64(rawData[idx:idx+8])))
	}
	return vals, nil
}

func (m *A1EService) readStringValue(conn *net.TCPConn, plcNumber byte, address string, length int) (interface{}, error) {
	// length 为字符串字节数，A1E 按 word 读取，需转换为 word 数(向上取整)
	wordCount := (length + 1) / 2
	rawData, err := ReadA1E(conn, address, wordCount, false, plcNumber)
	if err != nil {
		return nil, err
	}
	if len(rawData) > length {
		rawData = rawData[:length]
	}
	return strings.TrimRight(string(rawData), "\x00"), nil
}

func (m *A1EService) writeBoolValue(conn *net.TCPConn, plcNumber byte, address string, length int, value interface{}) error {
	var boolVal []byte
	switch v := value.(type) {
	case bool:
		if v {
			boolVal = []byte{0x01}
		} else {
			boolVal = []byte{0x00}
		}
	case string:
		v = strings.TrimSpace(v)
		switch strings.ToUpper(v) {
		case "1", "TRUE":
			boolVal = []byte{0x01}
		case "0", "FALSE":
			boolVal = []byte{0x00}
		default:
			return fmt.Errorf("无效的布尔值: %s", v)
		}
	case float64:
		if v != 0 {
			boolVal = []byte{0x01}
		} else {
			boolVal = []byte{0x00}
		}
	default:
		return fmt.Errorf("不支持的布尔值类型: %T", value)
	}
	return WriteA1EBool(conn, address, boolVal, plcNumber)
}

func (m *A1EService) writeIntValue(conn *net.TCPConn, plcNumber byte, address string, byteLen int, value interface{}) error {
	var intVal int64
	switch v := value.(type) {
	case float64:
		intVal = int64(v)
	case int:
		intVal = int64(v)
	case int64:
		intVal = v
	case int32:
		intVal = int64(v)
	case int16:
		intVal = int64(v)
	case uint:
		intVal = int64(v)
	case uint64:
		intVal = int64(v)
	case uint32:
		intVal = int64(v)
	case uint16:
		intVal = int64(v)
	case string:
		v = strings.TrimSpace(v)
		parsed, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return fmt.Errorf("无效的整数值: %v", err)
		}
		intVal = parsed
	default:
		return fmt.Errorf("不支持的整数值类型: %T", value)
	}

	data := Int2Bin(int(intVal), byte(byteLen), binary.LittleEndian)
	return WriteA1EWord(conn, address, data, plcNumber)
}

func (m *A1EService) writeFloatValue(conn *net.TCPConn, plcNumber byte, address string, length int, value interface{}) error {
	var floatVal float64
	switch v := value.(type) {
	case float64:
		floatVal = v
	case float32:
		floatVal = float64(v)
	case string:
		v = strings.TrimSpace(v)
		parsed, err := strconv.ParseFloat(v, 32)
		if err != nil {
			return fmt.Errorf("无效的浮点值: %v", err)
		}
		floatVal = parsed
	case int:
		floatVal = float64(v)
	case int64:
		floatVal = float64(v)
	default:
		return fmt.Errorf("不支持的浮点值类型: %T", value)
	}

	f32 := float32(floatVal)
	bits := math.Float32bits(f32)
	data := make([]byte, 4)
	binary.LittleEndian.PutUint32(data, bits)
	return WriteA1EWord(conn, address, data, plcNumber)
}

func (m *A1EService) writeDoubleValue(conn *net.TCPConn, plcNumber byte, address string, length int, value interface{}) error {
	var doubleVal float64
	switch v := value.(type) {
	case float64:
		doubleVal = v
	case float32:
		doubleVal = float64(v)
	case string:
		v = strings.TrimSpace(v)
		parsed, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return fmt.Errorf("无效的双精度浮点值: %v", err)
		}
		doubleVal = parsed
	case int:
		doubleVal = float64(v)
	case int64:
		doubleVal = float64(v)
	default:
		return fmt.Errorf("不支持的双精度浮点值类型: %T", value)
	}

	bits := math.Float64bits(doubleVal)
	data := make([]byte, 8)
	binary.LittleEndian.PutUint64(data, bits)
	return WriteA1EWord(conn, address, data, plcNumber)
}

func (m *A1EService) writeStringValue(conn *net.TCPConn, plcNumber byte, address string, length int, value interface{}) error {
	var str string
	switch v := value.(type) {
	case string:
		str = v
	case []byte:
		str = string(v)
	default:
		return fmt.Errorf("不支持的字符串值类型: %T", value)
	}
	data := []byte(str)
	if len(data)%2 != 0 {
		data = append(data, 0)
	}
	return WriteA1EWord(conn, address, data, plcNumber)
}

func Int2Bin(n int, bytesLength byte, order binary.ByteOrder) []byte {
	switch bytesLength {
	case 1:
		tmp := int8(n)
		bytesBuffer := bytes.NewBuffer([]byte{})
		binary.Write(bytesBuffer, order, &tmp)
		return bytesBuffer.Bytes()
	case 2:
		tmp := int16(n)
		bytesBuffer := bytes.NewBuffer([]byte{})
		binary.Write(bytesBuffer, order, &tmp)
		return bytesBuffer.Bytes()
	case 3:
		tmp := int32(n)
		bytesBuffer := bytes.NewBuffer([]byte{})
		binary.Write(bytesBuffer, order, &tmp)
		return bytesBuffer.Bytes()[0:3]
	case 4:
		tmp := int32(n)
		bytesBuffer := bytes.NewBuffer([]byte{})
		binary.Write(bytesBuffer, order, &tmp)
		return bytesBuffer.Bytes()
	case 5:
		tmp := int64(n)
		bytesBuffer := bytes.NewBuffer([]byte{})
		binary.Write(bytesBuffer, order, &tmp)
		return bytesBuffer.Bytes()[0:5]
	case 6:
		tmp := int64(n)
		bytesBuffer := bytes.NewBuffer([]byte{})
		binary.Write(bytesBuffer, order, &tmp)
		return bytesBuffer.Bytes()[0:6]
	case 7:
		tmp := int64(n)
		bytesBuffer := bytes.NewBuffer([]byte{})
		binary.Write(bytesBuffer, order, &tmp)
		return bytesBuffer.Bytes()[0:7]
	case 8:
		tmp := int64(n)
		bytesBuffer := bytes.NewBuffer([]byte{})
		binary.Write(bytesBuffer, order, &tmp)
		return bytesBuffer.Bytes()
	}
	return nil
}

func Bin2Int(b []byte, orders ...binary.ByteOrder) int {
	var order binary.ByteOrder = binary.BigEndian
	if len(orders) > 0 {
		order = orders[0]
	}
	if len(b) == 3 {
		b = append([]byte{0}, b...)
	}
	bytesBuffer := bytes.NewBuffer(b)
	switch len(b) {
	case 1:
		var tmp int8
		err := binary.Read(bytesBuffer, order, &tmp)
		if err != nil {
			return 0
		}
		return int(tmp)
	case 2:
		var tmp int16
		err := binary.Read(bytesBuffer, order, &tmp)
		if err != nil {
			return 0
		}
		return int(tmp)
	case 4:
		var tmp int32
		err := binary.Read(bytesBuffer, order, &tmp)
		if err != nil {
			return 0
		}
		return int(tmp)
	case 8:
		var tmp int64
		err := binary.Read(bytesBuffer, order, &tmp)
		if err != nil {
			return 0
		}
		return int(tmp)
	default:
		signed := len(b) > 0 && b[0]&0x80 != 0
		if signed {
			complement := make([]byte, len(b))
			for i := range complement {
				complement[i] = ^b[i]
			}
			for i := len(complement) - 1; i >= 0; i-- {
				complement[i]++
				if complement[i] != 0 {
					break
				}
			}
			val := 0
			for i := 0; i < len(complement); i++ {
				if order == binary.BigEndian {
					val = val<<8 | int(complement[i])
				} else {
					val = val | int(complement[i])<<(8*i)
				}
			}
			return -val
		}
		val := 0
		for i := 0; i < len(b); i++ {
			if order == binary.BigEndian {
				val = val<<8 | int(b[i])
			} else {
				val = val | int(b[i])<<(8*i)
			}
		}
		return val
	}
}

func (m *A1EService) ReadParams(ctx context.Context, params protocol.Metadata) (protocol.Metadata, error) {
	pointsRaw := getInfoSlice(params, "points")
	points := make([]protocol.Point, 0, len(pointsRaw))
	for _, p := range pointsRaw {
		if pt, ok := p.(map[string]any); ok {
			points = append(points, protocol.Point{
				Name:     toString(pt["name"]),
				Resource: toString(pt["address"]),
				DataType: toString(pt["type"]),
				Count:    builtin.GetIntValue(pt, "length", 1),
			})
		}
	}

	req := protocol.BatchReadRequest{Points: points}
	resp, err := m.ReadBatch(ctx, protocol.DeviceHandle(m.deviceID), req)
	if err != nil {
		return nil, err
	}

	result := make(protocol.Metadata)
	vals := make([]protocol.Metadata, 0, len(resp.Results))
	for _, r := range resp.Results {
		vals = append(vals, protocol.Metadata{
			"name":    r.PointName,
			"value":   r.Value,
			"quality": r.Quality,
		})
	}
	result["results"] = vals
	return result, nil
}

func getInfoSlice(m protocol.Metadata, key string) []any {
	if m == nil {
		return nil
	}
	v, _ := m[key].([]any)
	return v
}

func toString(v any) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	default:
		return fmt.Sprintf("%v", val)
	}
}