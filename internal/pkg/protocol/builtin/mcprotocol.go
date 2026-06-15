package builtin

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"sync"

	"edge5/internal/pkg/protocol"

	"github.com/longan55/go-mcprotocol/mcp"
	"go.uber.org/zap"
)

func init() {
	// 注册内置协议 MelsecMC
	if err := Register(NewMcService()); err != nil {
		panic(fmt.Sprintf("注册 MelsecMC 协议失败: %v", err))
	}
}

// ---------------------------------------------------------------------------
// McService 基于三菱 MC 协议（3E 帧）的 Q 系列 PLC 通信实现
// ---------------------------------------------------------------------------

type McService struct {
	lock       sync.Mutex
	Clients    map[uint]mcp.Client
	connected  bool
	deviceID   uint
	connection protocol.Metadata
}

func NewMcService() *McService {
	return &McService{
		Clients: make(map[uint]mcp.Client),
	}
}

// Info 返回协议元信息
func (m *McService) Info() protocol.Metadata {
	return protocol.Metadata{
		"name":          "MelsecMC",
		"alias":         "MC-3E", // 前端使用的别名
		"version":       "1.0.0",
		"description":   "三菱 PLC MC 协议（3E 帧，Q 系列）",
		"group":         "builtin",
		"source":        "builtin",
		"deviceType":    "PLC",
		"brand":         "Mitsubishi",
		"cName":         "三菱",
		"supportServer": true,
		"supportDebug":  true,
		"models":        []string{"Q系列"},
		"connectionParams": []any{
			protocol.Metadata{"name": "ip", "cName": "IP 地址", "type": "string", "required": true, "default": ""},
			protocol.Metadata{"name": "port", "cName": "端口", "type": "int", "required": true, "default": "6000"},
			protocol.Metadata{"name": "networkNumber", "cName": "网络编号", "type": "string", "required": false, "default": "00"},
			protocol.Metadata{"name": "pcNum", "cName": "PC 编号", "type": "string", "required": false, "default": "FF"},
			protocol.Metadata{"name": "unitIONum", "cName": "单元 I/O 编号", "type": "string", "required": false, "default": "FF03"},
			protocol.Metadata{"name": "unitStationNum", "cName": "单元站号", "type": "string", "required": false, "default": "00"},
		},
		"readParamsSchema": []any{
			protocol.Metadata{"name": "address", "cName": "地址", "type": "string", "required": true, "default": "X0"},
			protocol.Metadata{"name": "offset", "cName": "偏移量", "type": "int", "required": false, "default": "1"},
			protocol.Metadata{"name": "parseType", "cName": "解析类型", "type": "select", "required": true, "default": "bool", "choices": []string{"bool", "short", "ushort", "int", "uint", "long", "ulong", "float", "double", "string"}},
		},
	}
}

// IsSupportServer 返回是否支持服务端模式
func (m *McService) IsSupportServer() bool {
	return true
}

// IsConnected 返回指定句柄的设备是否已连接
func (m *McService) IsConnected(handle protocol.DeviceHandle) bool {
	m.lock.Lock()
	defer m.lock.Unlock()
	client, ok := m.Clients[uint(handle)]
	return ok && client != nil
}

// Connect 根据 Metadata 中的连接参数连接到 PLC，返回设备句柄
// 参数:
//   - ip: string
//   - port: int (默认 6000)
//   - networkNumber: int (默认 0)
//   - pcNum: int (默认 0xFF)
//   - unitIONum: int (默认 0x03FF)
//   - unitStationNum: int (默认 0x00)
func (m *McService) Connect(ctx context.Context, params protocol.Metadata) (protocol.DeviceHandle, error) {
	m.lock.Lock()
	defer m.lock.Unlock()

	if m.Clients == nil {
		m.Clients = make(map[uint]mcp.Client)
	}

	deviceID := uint(getInt(params, "deviceID"))
	if deviceID == 0 {
		return protocol.InvalidHandle, fmt.Errorf("缺少 deviceID 参数")
	}

	if _, ok := m.Clients[deviceID]; ok {
		m.connected = true
		m.deviceID = deviceID
		m.connection = params
		return protocol.DeviceHandle(deviceID), nil
	}

	ip := getString(params, "ip")
	port := getInt(params, "port")
	if port <= 0 {
		port = 6000
	}
	networkNumber := getString(params, "networkNumber")
	pcNum := getString(params, "pcNum")
	unitIONum := getString(params, "unitIONum")
	unitStationNum := getString(params, "unitStationNum")

	if networkNumber == "" {
		networkNumber = "00"
	}
	if pcNum == "" {
		pcNum = "FF"
	}
	if unitIONum == "" {
		unitIONum = "FF03"
	}
	if unitStationNum == "" {
		unitStationNum = "00"
	}

	if ip == "" {
		return protocol.InvalidHandle, fmt.Errorf("缺少 IP 地址参数")
	}

	station := mcp.NewStation(networkNumber, pcNum, unitIONum, unitStationNum)
	client, err := mcp.New3EAliveClient(ip, port, station)
	if err != nil {
		return protocol.InvalidHandle, fmt.Errorf("创建 MC 客户端失败: %w", err)
	}
	if err = client.Connect(); err != nil {
		return protocol.InvalidHandle, fmt.Errorf("连接 MC 客户端失败: %w", err)
	}

	m.Clients[deviceID] = client
	m.connected = true
	m.deviceID = deviceID
	m.connection = params

	if logger != nil {
		logger.Info("设备连接成功",
			zap.Uint("deviceID", deviceID),
			zap.String("ip", ip),
			zap.Int("port", port),
		)
	}
	return protocol.DeviceHandle(deviceID), nil
}

// Disconnect 断开指定句柄的设备连接
func (m *McService) Disconnect(ctx context.Context, handle protocol.DeviceHandle) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	devID := uint(handle)
	if devID == 0 {
		devID = m.deviceID
	}

	if client, ok := m.Clients[devID]; ok {
		if err := client.Close(); err != nil {
			return fmt.Errorf("关闭 MC 客户端失败: %w", err)
		}
		delete(m.Clients, devID)
	}
	if devID == m.deviceID {
		m.connected = false
		m.deviceID = 0
		m.connection = nil
	}
	return nil
}

// ReadBatch 批量读取 PLC 寄存器数据
func (m *McService) ReadBatch(ctx context.Context, handle protocol.DeviceHandle, req protocol.BatchReadRequest) (*protocol.BatchReadResponse, error) {
	m.lock.Lock()
	devID := uint(handle)
	if devID == 0 {
		devID = m.deviceID
	}
	client, ok := m.Clients[devID]
	m.lock.Unlock()

	if !ok {
		return nil, fmt.Errorf("句柄 %d 未连接", handle)
	}

	results := make([]protocol.BatchReadResult, 0, len(req.Points))

	for _, pt := range req.Points {
		result := m.readPoint(client, pt)
		results = append(results, result)
	}

	return &protocol.BatchReadResponse{
		Results: results,
	}, nil
}

func (m *McService) readPoint(client mcp.Client, pt protocol.Point) protocol.BatchReadResult {
	count := pt.Count
	if count <= 0 {
		count = 1
	}

	var value interface{}
	var err error

	switch pt.DataType {
	case "bool":
		value, err = m.readBoolValue(client, pt.Resource, count)
	case "byte":
		value, err = m.readByteValue(client, pt.Resource, count)
	case "int16", "short":
		value, err = m.readShortValue(client, pt.Resource, count)
	case "uint16", "ushort":
		value, err = m.readUShortValue(client, pt.Resource, count)
	case "int32", "int":
		value, err = m.readIntValue(client, pt.Resource, count)
	case "uint32", "uint":
		value, err = m.readUIntValue(client, pt.Resource, count)
	case "int64", "long":
		value, err = m.readLongValue(client, pt.Resource, count)
	case "uint64", "ulong":
		value, err = m.readULongValue(client, pt.Resource, count)
	case "float32", "float":
		value, err = m.readFloatValue(client, pt.Resource, count)
	case "float64", "double":
		value, err = m.readDoubleValue(client, pt.Resource, count)
	case "string":
		value, err = m.readStringValue(client, pt.Resource, count)
	default:
		err = fmt.Errorf("不支持的数据类型: %s", pt.DataType)
	}

	quality := "good"
	if err != nil {
		quality = "bad"
	}

	return protocol.BatchReadResult{
		PointName: pt.Name,
		Value:     value,
		Quality:   quality,
		Error:     err,
	}
}

// WriteBatch 批量写入 PLC 寄存器数据
func (m *McService) WriteBatch(ctx context.Context, handle protocol.DeviceHandle, req protocol.BatchWriteRequest) error {
	m.lock.Lock()
	devID := uint(handle)
	if devID == 0 {
		devID = m.deviceID
	}
	client, ok := m.Clients[devID]
	m.lock.Unlock()

	if !ok {
		return fmt.Errorf("句柄 %d 未连接", handle)
	}

	for _, item := range req.Items {
		if err := m.writePoint(client, item); err != nil {
			return fmt.Errorf("写入点位 %s 失败: %w", item.Point.Name, err)
		}
	}
	return nil
}

func (m *McService) writePoint(client mcp.Client, item protocol.BatchWriteItem) error {
	pt := item.Point
	count := pt.Count
	if count <= 0 {
		count = 1
	}

	switch pt.DataType {
	case "bool":
		return m.writeBoolValue(client, pt.Resource, pt.Count, item.Value)
	case "byte":
		return m.writeIntValue(client, pt.Resource, 1, item.Value)
	case "int16", "short":
		return m.writeIntValue(client, pt.Resource, 2, item.Value)
	case "uint16", "ushort":
		return m.writeIntValue(client, pt.Resource, 2, item.Value)
	case "int32", "int":
		return m.writeIntValue(client, pt.Resource, 4, item.Value)
	case "uint32", "uint":
		return m.writeIntValue(client, pt.Resource, 4, item.Value)
	case "int64", "long":
		return m.writeIntValue(client, pt.Resource, 8, item.Value)
	case "uint64", "ulong":
		return m.writeIntValue(client, pt.Resource, 8, item.Value)
	case "float32", "float":
		return m.writeFloatValue(client, pt.Resource, count, item.Value)
	case "float64", "double":
		return m.writeDoubleValue(client, pt.Resource, count, item.Value)
	case "string":
		return m.writeStringValue(client, pt.Resource, count, item.Value)
	default:
		return fmt.Errorf("不支持的数据类型: %s", pt.DataType)
	}
}

// ---------------------------------------------------------------------------
// ReadMelsec 核心读取方法
// ---------------------------------------------------------------------------

// ReadMelsec 读取 PLC 寄存器数据
// address: 设备地址，例如 "D10" → deviceName="D", offset=10
// numPoints: 读取的点数（字数），前端统一用 length 表示 numPoints
func ReadMelsec(client mcp.Client, address string, numPoints int) (string, []byte, error) {
	deviceName, offset, err := ParseAddress(address)
	if err != nil {
		return "", nil, fmt.Errorf("地址解析失败: %v", err)
	}

	log.Printf("ReadMelsec - Device: %s, Offset: %d, NumPoints: %d", deviceName, offset, numPoints)
	read, err := client.Read(deviceName, offset, int64(numPoints))
	if err != nil {
		log.Printf("ReadMelsec ERROR: %v", err)
		return "", nil, fmt.Errorf("读取失败: %w", err)
	}
	registerBinary, err := mcp.NewParser().Do(read)
	if err != nil {
		return "", nil, fmt.Errorf("解析失败: %v", err)
	}
	return address, registerBinary.Payload, nil
}

// ---------------------------------------------------------------------------
// WriteMelsec 核心写入方法
// ---------------------------------------------------------------------------

// WriteMelsec 写入 PLC 寄存器数据
// address: 设备地址，例如 "D10" → deviceName="D", offset=10
// numPoints: 写入的点数（字数），前端统一用 length 表示 numPoints
// value: 要写入的字节数据
func WriteMelsec(client mcp.Client, address string, numPoints int, value []byte) ([]byte, error) {
	deviceName, offset, err := ParseAddress(address)
	if err != nil {
		return nil, fmt.Errorf("无效的地址格式: %v", err)
	}
	resp, err := client.Write(deviceName, offset, int64(numPoints), value)
	if err != nil {
		return nil, fmt.Errorf("写入失败: %v", err)
	}
	return resp, nil
}

// ---------------------------------------------------------------------------
// 辅助读取方法（返回 interface{} 适配新接口）
// ---------------------------------------------------------------------------

func (m *McService) readBoolValue(client mcp.Client, address string, length int) (interface{}, error) {
	_, rawData, err := ReadMelsec(client, address, length)
	if err != nil {
		return nil, err
	}
	i := binary.LittleEndian.Uint16(rawData)
	if length == 1 {
		return i&0x01 == 0x01, nil
	}
	bools := make([]bool, length)
	for j := 0; j < length; j++ {
		bools[j] = (i>>j)&0x01 == 0x01
	}
	return bools, nil
}

func (m *McService) readByteValue(client mcp.Client, address string, length int) (interface{}, error) {
	_, rawData, err := ReadMelsec(client, address, length)
	if err != nil {
		return nil, err
	}
	if len(rawData) == 1 {
		return rawData[0], nil
	}
	return rawData, nil
}

func (m *McService) readShortValue(client mcp.Client, address string, length int) (interface{}, error) {
	_, rawData, err := ReadMelsec(client, address, length)
	if err != nil {
		return nil, err
	}
	count := len(rawData)
	if count <= 2 {
		return int16(Bin2Int(rawData, binary.LittleEndian)), nil
	}
	vals := make([]int16, 0, count/2)
	for i := 0; i < count; i += 2 {
		vals = append(vals, int16(Bin2Int(rawData[i:i+2], binary.LittleEndian)))
	}
	return vals, nil
}

func (m *McService) readUShortValue(client mcp.Client, address string, length int) (interface{}, error) {
	_, rawData, err := ReadMelsec(client, address, length)
	if err != nil {
		return nil, err
	}
	count := len(rawData)
	if count <= 2 {
		return uint16(Bin2Int(rawData, binary.LittleEndian)), nil
	}
	vals := make([]uint16, 0, count/2)
	for i := 0; i < count; i += 2 {
		vals = append(vals, uint16(Bin2Int(rawData[i:i+2], binary.LittleEndian)))
	}
	return vals, nil
}

func (m *McService) readIntValue(client mcp.Client, address string, length int) (interface{}, error) {
	_, rawData, err := ReadMelsec(client, address, length)
	if err != nil {
		return nil, err
	}
	count := len(rawData)
	if count <= 4 {
		return int32(Bin2Int(rawData, binary.LittleEndian)), nil
	}
	vals := make([]int32, 0, count/4)
	for i := 0; i < count; i += 4 {
		vals = append(vals, int32(Bin2Int(rawData[i:i+4], binary.LittleEndian)))
	}
	return vals, nil
}

func (m *McService) readUIntValue(client mcp.Client, address string, length int) (interface{}, error) {
	_, rawData, err := ReadMelsec(client, address, length)
	if err != nil {
		return nil, err
	}
	count := len(rawData)
	if count <= 4 {
		return uint32(Bin2Int(rawData, binary.LittleEndian)), nil
	}
	vals := make([]uint32, 0, count/4)
	for i := 0; i < count; i += 4 {
		vals = append(vals, uint32(Bin2Int(rawData[i:i+4], binary.LittleEndian)))
	}
	return vals, nil
}

func (m *McService) readLongValue(client mcp.Client, address string, length int) (interface{}, error) {
	_, rawData, err := ReadMelsec(client, address, length)
	if err != nil {
		return nil, err
	}
	count := len(rawData)
	if count <= 8 {
		return int64(Bin2Int(rawData, binary.LittleEndian)), nil
	}
	vals := make([]int64, 0, count/8)
	for i := 0; i < count; i += 8 {
		vals = append(vals, int64(Bin2Int(rawData[i:i+8], binary.LittleEndian)))
	}
	return vals, nil
}

func (m *McService) readULongValue(client mcp.Client, address string, length int) (interface{}, error) {
	_, rawData, err := ReadMelsec(client, address, length)
	if err != nil {
		return nil, err
	}
	count := len(rawData)
	if count <= 8 {
		return uint64(Bin2Int(rawData, binary.LittleEndian)), nil
	}
	vals := make([]uint64, 0, count/8)
	for i := 0; i < count; i += 8 {
		vals = append(vals, uint64(Bin2Int(rawData[i:i+8], binary.LittleEndian)))
	}
	return vals, nil
}

func (m *McService) readFloatValue(client mcp.Client, address string, length int) (interface{}, error) {
	length *= 2
	_, rawData, err := ReadMelsec(client, address, length)
	if err != nil {
		return nil, err
	}
	count := len(rawData)
	if count == 4 {
		return math.Float32frombits(binary.LittleEndian.Uint32(rawData)), nil
	}
	vals := make([]float32, 0, count/4)
	for i := 0; i < count; i += 4 {
		vals = append(vals, math.Float32frombits(binary.LittleEndian.Uint32(rawData[i:i+4])))
	}
	return vals, nil
}

func (m *McService) readDoubleValue(client mcp.Client, address string, length int) (interface{}, error) {
	length *= 4
	_, rawData, err := ReadMelsec(client, address, length)
	if err != nil {
		return nil, err
	}
	count := len(rawData)
	if count == 8 {
		return math.Float64frombits(binary.LittleEndian.Uint64(rawData)), nil
	}
	vals := make([]float64, 0, count/8)
	for i := 0; i < count; i += 8 {
		vals = append(vals, math.Float64frombits(binary.LittleEndian.Uint64(rawData[i:i+8])))
	}
	return vals, nil
}

func (m *McService) readStringValue(client mcp.Client, address string, length int) (interface{}, error) {
	_, rawData, err := ReadMelsec(client, address, length)
	if err != nil {
		return nil, err
	}
	// 去除尾部零字节
	return strings.TrimRight(string(rawData), "\x00"), nil
}

// ---------------------------------------------------------------------------
// 辅助写入方法（适配新接口）
// ---------------------------------------------------------------------------

func (m *McService) writeBoolValue(client mcp.Client, address string, length int, value interface{}) error {
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
	_, err := WriteMelsec(client, address, length, boolVal)
	return err
}

func (m *McService) writeIntValue(client mcp.Client, address string, byteLen int, value interface{}) error {
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
	_, err := WriteMelsec(client, address, byteLen, data)
	return err
}

func (m *McService) writeFloatValue(client mcp.Client, address string, length int, value interface{}) error {
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
	_, err := WriteMelsec(client, address, length*2, data)
	return err
}

func (m *McService) writeDoubleValue(client mcp.Client, address string, length int, value interface{}) error {
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
	_, err := WriteMelsec(client, address, length*4, data)
	return err
}

func (m *McService) writeStringValue(client mcp.Client, address string, length int, value interface{}) error {
	var str string
	switch v := value.(type) {
	case string:
		str = v
	case []byte:
		str = string(v)
	default:
		return fmt.Errorf("不支持的字符串值类型: %T", value)
	}
	_, err := WriteMelsec(client, address, length, []byte(str))
	return err
}

// ---------------------------------------------------------------------------
// 地址解析工具
// ---------------------------------------------------------------------------

func ParseAddress(address string) (deviceName string, offset int64, err error) {
	// 将 address 由前面的若干字母紧接着数字组成，将字母部分赋值给 deviceName，将数字部分赋值给 offset
	digitIdx := strings.IndexFunc(address, func(r rune) bool { return r >= '0' && r <= '9' })
	if digitIdx < 0 {
		return "", 0, fmt.Errorf("地址格式无效（缺少数字部分）: %s", address)
	}
	deviceName = strings.ToUpper(address[:digitIdx])
	offset, err = strconv.ParseInt(address[digitIdx:], 10, 64)
	if err != nil {
		return "", 0, fmt.Errorf("地址解析失败: %v", err)
	}
	return deviceName, offset, nil
}

// ---------------------------------------------------------------------------
// 数据类型转换工具
// ---------------------------------------------------------------------------

func ParseBoolValue(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	value = strings.ToUpper(value)
	switch value {
	case "1", "TRUE":
		return []byte{0x01}, nil
	case "0", "FALSE":
		return []byte{0x00}, nil
	default:
		return nil, fmt.Errorf("无效的布尔值: %s", value)
	}
}

func ParseIntValue(length int, value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	intValue, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("无效的整数: %v", err)
	}
	return Int2Bin(int(intValue), byte(length), binary.LittleEndian), nil
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
		b = append([]byte{0}, b...) // 3字节特殊处理，扩展为4字节
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
		// 对于不支持的字节长度，尝试作为有符号整数处理
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

// ---------------------------------------------------------------------------
// Metadata 辅助读取
// ---------------------------------------------------------------------------

func getString(m protocol.Metadata, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	default:
		return fmt.Sprintf("%v", val)
	}
}

func getInt(m protocol.Metadata, key string) int {
	if m == nil {
		return 0
	}
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	case string:
		n, _ := strconv.Atoi(val)
		return n
	default:
		return 0
	}
}

// ---------------------------------------------------------------------------
// 保留旧版 ReadBatch / WriteBatch 方法签名以兼容外部调用（内部委托给新方法）
// ---------------------------------------------------------------------------

// ReadParams 兼容旧版 ParamRequest 调用
// TODO: 后续迁移完成后删除
func (m *McService) ReadParams(ctx context.Context, params protocol.Metadata) (protocol.Metadata, error) {
	// 将 Metadata 转为 BatchReadRequest
	pointsRaw := getInfoSlice(params, "points")
	points := make([]protocol.Point, 0, len(pointsRaw))
	for _, p := range pointsRaw {
		if pt, ok := p.(map[string]any); ok {
			points = append(points, protocol.Point{
				Name:     toString(pt["name"]),
				Resource: toString(pt["address"]),
				DataType: toString(pt["type"]),
				Count:    getIntValue(pt, "length", 1),
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

func getIntValue(m map[string]any, key string, defaultVal int) int {
	if m == nil {
		return defaultVal
	}
	v, ok := m[key]
	if !ok {
		return defaultVal
	}
	switch val := v.(type) {
	case int:
		return val
	case float64:
		return int(val)
	case string:
		n, _ := strconv.Atoi(val)
		return n
	default:
		return defaultVal
	}
}

// toString 将任意类型的值转换为字符串
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
