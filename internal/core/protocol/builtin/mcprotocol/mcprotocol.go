package mcprotocol

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"

	"edge5/internal/core/protocol"
	"edge5/internal/core/protocol/builtin"

	"github.com/longan55/go-mcprotocol/mcp"
	"go.uber.org/zap"
)

func init() {
	if err := builtin.Register(NewMcService()); err != nil {
		panic(fmt.Sprintf("注册 MelsecMC 协议失败: %v", err))
	}
}

type McService struct {
	lock sync.Mutex
	// ioLock 保护网络 I/O 临界区（拨号-收发-断开）串行化。
	// PLC 通常只允许 1 个 TCP 连接，采集任务与调试/MQTT 读写若并发拨号会互踢导致 EOF。
	// ioLock 让并发请求排队执行，批间自然交替使用 PLC 连接，不长期占用。
	ioLock           sync.Mutex
	connected        bool
	deviceID         uint
	connectionParams map[uint]protocol.Metadata
}

func NewMcService() *McService {
	return &McService{
		connectionParams: make(map[uint]protocol.Metadata),
	}
}

func (m *McService) Info() protocol.Metadata {
	return protocol.Metadata{
		"name":          "MC-3E",
		"alias":         "MelsecMC",
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

func (m *McService) IsSupportServer() bool {
	return true
}

// buildClient 从连接参数构建 TCP(3E) MC 客户端，返回客户端与解析后的连接参数。
// 与 UDP 版 (McUDPService.buildClient) 共用 parseConnParams 解析逻辑，
// 仅客户端构造方式不同。
//
// 使用长连接版 New3EAliveClient：Connect/Close 真正拨号/断开，Read/Write/BitRead
// 复用同一条 TCP 连接。配合 ReadBatch/WriteBatch 中的 "client.Connect() +
// defer client.Close()" 模式，实现"每批一连接，批内多点位共用，批结束立即断开"。
// 相比旧版 New3EClient（Connect/Close 为空操作、每次 Read 内部独立拨号），
// 可消除"每点位一连接"造成的 PLC 连接竞争与交替 EOF 问题，同时不长期占用
// PLC 的有限连接数资源。
func (m *McService) buildClient(params protocol.Metadata) (mcp.Client, mcConnParams, error) {
	p, err := parseConnParams(params)
	if err != nil {
		return nil, p, err
	}
	station := mcp.NewStation(p.networkNumber, p.pcNum, p.unitIONum, p.unitStationNum)
	client, err := mcp.New3EAliveClient(p.ip, p.port, station)
	if err != nil {
		return nil, p, fmt.Errorf("创建 MC 客户端失败: %w", err)
	}
	return client, p, nil
}

func (m *McService) IsConnected(handle protocol.DeviceHandle) bool {
	m.lock.Lock()
	devID := uint(handle)
	if devID == 0 {
		devID = m.deviceID
	}
	params, ok := m.connectionParams[devID]
	m.lock.Unlock()

	if !ok {
		return false
	}

	// ioLock 串行化网络 I/O，避免与并发采集/调试/MQTT 请求互踢 PLC 连接
	m.ioLock.Lock()
	defer m.ioLock.Unlock()

	client, _, err := m.buildClient(params)
	if err != nil {
		return false
	}
	return probeRead(client) == nil
}

func (m *McService) Connect(ctx context.Context, params protocol.Metadata) (protocol.DeviceHandle, error) {
	m.lock.Lock()
	deviceID := uint(builtin.GetInt(params, "deviceID"))
	if deviceID == 0 {
		m.lock.Unlock()
		return protocol.InvalidHandle, fmt.Errorf("缺少 deviceID 参数")
	}
	// 检查该设备是否已建立过连接（如采集任务已 Connect 并存入连接参数）。
	// 已连接则跳过 probeRead 拨号验证，避免与并发采集/调试/MQTT 拨号互踢 PLC
	// 连接；设备实际可达性由后续 ReadBatch/WriteBatch 兜底验证。
	_, alreadyConnected := m.connectionParams[deviceID]
	m.lock.Unlock()

	// ioLock 串行化网络 I/O：避免采集任务正在拨号/读写时，调试/MQTT 请求并发拨号互踢
	m.ioLock.Lock()
	defer m.ioLock.Unlock()

	client, p, err := m.buildClient(params)
	if err != nil {
		return protocol.InvalidHandle, err
	}
	// 仅首次连接时用读请求验证设备可达与连接参数正确（与 UDP 版统一）。
	// 已连接（如采集任务进行中）时跳过，避免 probeRead 拨号与紧接的
	// ReadBatch/WriteBatch 拨号撞 PLC 连接释放延迟导致 EOF。
	if !alreadyConnected {
		if err := probeRead(client); err != nil {
			return protocol.InvalidHandle, fmt.Errorf("连接 MC 客户端失败: %w", err)
		}
	}

	m.lock.Lock()
	m.connectionParams[deviceID] = params
	m.deviceID = deviceID
	m.connected = true
	m.lock.Unlock()

	if builtin.Logger != nil {
		builtin.Logger.Info("MC协议: 连接测试成功",
			zap.Uint("deviceID", deviceID),
			zap.String("ip", p.ip),
			zap.Int("port", p.port),
		)
	}
	return protocol.DeviceHandle(deviceID), nil
}

func (m *McService) Disconnect(ctx context.Context, handle protocol.DeviceHandle) error {
	return nil
}

func (m *McService) ReadBatch(ctx context.Context, handle protocol.DeviceHandle, req protocol.BatchReadRequest) (*protocol.BatchReadResponse, error) {
	// ioLock 串行化网络 I/O：采集任务与调试/MQTT 读写共用同一 PLC 连接，并发拨号会互踢。
	// 加锁后并发请求排队执行，采集批结束释放锁，调试/MQTT 即可插入。
	m.ioLock.Lock()
	defer m.ioLock.Unlock()

	m.lock.Lock()
	devID := uint(handle)
	if devID == 0 {
		devID = m.deviceID
	}
	params := m.connectionParams[devID]
	m.lock.Unlock()

	if params == nil {
		return nil, protocol.ErrNotConnected
	}

	client, _, err := m.buildClient(params)
	if err != nil {
		return nil, err
	}
	if err = client.Connect(); err != nil {
		return nil, fmt.Errorf("连接 MC 客户端失败: %w", err)
	}
	defer client.Close()

	results := make([]protocol.BatchReadResult, 0, len(req.Points))
	for _, pt := range req.Points {
		result := readPoint(client, pt)
		results = append(results, result)
	}

	return &protocol.BatchReadResponse{
		Results: results,
	}, nil
}

func readPoint(client mcp.Client, pt protocol.Point) protocol.BatchReadResult {
	count := pt.Count
	if count <= 0 {
		count = 1
	}

	var value interface{}
	var err error

	switch pt.DataType {
	case "bool":
		value, err = readBoolValue(client, pt.Resource, count)
	case "byte":
		value, err = readByteValue(client, pt.Resource, count)
	case "int16", "short":
		value, err = readShortValue(client, pt.Resource, count)
	case "uint16", "ushort":
		value, err = readUShortValue(client, pt.Resource, count)
	case "int32", "int":
		value, err = readIntValue(client, pt.Resource, count)
	case "uint32", "uint":
		value, err = readUIntValue(client, pt.Resource, count)
	case "int64", "long":
		value, err = readLongValue(client, pt.Resource, count)
	case "uint64", "ulong":
		value, err = readULongValue(client, pt.Resource, count)
	case "float32", "float":
		value, err = readFloatValue(client, pt.Resource, count)
	case "float64", "double":
		value, err = readDoubleValue(client, pt.Resource, count)
	case "string":
		value, err = readStringValue(client, pt.Resource, count)
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

func (m *McService) WriteBatch(ctx context.Context, handle protocol.DeviceHandle, req protocol.BatchWriteRequest) error {
	// ioLock 串行化网络 I/O，与 ReadBatch/Connect 共用，避免并发拨号互踢
	m.ioLock.Lock()
	defer m.ioLock.Unlock()

	m.lock.Lock()
	devID := uint(handle)
	if devID == 0 {
		devID = m.deviceID
	}
	params := m.connectionParams[devID]
	m.lock.Unlock()

	if params == nil {
		return protocol.ErrNotConnected
	}

	client, _, err := m.buildClient(params)
	if err != nil {
		return err
	}
	if err = client.Connect(); err != nil {
		return fmt.Errorf("连接 MC 客户端失败: %w", err)
	}
	defer client.Close()

	for _, item := range req.Items {
		if err := writePoint(client, item); err != nil {
			return fmt.Errorf("写入点位 %s 失败: %w", item.Point.Name, err)
		}
	}
	return nil
}

func writePoint(client mcp.Client, item protocol.BatchWriteItem) error {
	pt := item.Point
	count := pt.Count
	if count <= 0 {
		count = 1
	}

	switch pt.DataType {
	case "bool":
		return writeBoolValue(client, pt.Resource, pt.Count, item.Value)
	case "byte":
		return writeIntValue(client, pt.Resource, 1, item.Value)
	case "int16", "short":
		return writeIntValue(client, pt.Resource, 2, item.Value)
	case "uint16", "ushort":
		return writeIntValue(client, pt.Resource, 2, item.Value)
	case "int32", "int":
		return writeIntValue(client, pt.Resource, 4, item.Value)
	case "uint32", "uint":
		return writeIntValue(client, pt.Resource, 4, item.Value)
	case "int64", "long":
		return writeIntValue(client, pt.Resource, 8, item.Value)
	case "uint64", "ulong":
		return writeIntValue(client, pt.Resource, 8, item.Value)
	case "float32", "float":
		return writeFloatValue(client, pt.Resource, count, item.Value)
	case "float64", "double":
		return writeDoubleValue(client, pt.Resource, count, item.Value)
	case "string":
		return writeStringValue(client, pt.Resource, count, item.Value)
	default:
		return fmt.Errorf("不支持的数据类型: %s", pt.DataType)
	}
}

func ReadMelsec(client mcp.Client, address string, numPoints int) (string, []byte, error) {
	deviceName, offset, err := ParseAddress(address)
	if err != nil {
		return "", nil, fmt.Errorf("地址解析失败: %v", err)
	}

	// log.Printf("ReadMelsec - Device: %s, Offset: %d, NumPoints: %d", deviceName, offset, numPoints)
	read, err := client.Read(deviceName, offset, int64(numPoints))
	if err != nil {
		return "", nil, fmt.Errorf("读取失败: %w", err)
	}
	registerBinary, err := mcp.NewParser().Do(read)
	if err != nil {
		return "", nil, fmt.Errorf("解析失败: %v", err)
	}
	return address, registerBinary.Payload, nil
}

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

func readBoolValue(client mcp.Client, address string, length int) (interface{}, error) {
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

func readByteValue(client mcp.Client, address string, length int) (interface{}, error) {
	_, rawData, err := ReadMelsec(client, address, length)
	if err != nil {
		return nil, err
	}
	if len(rawData) == 1 {
		return rawData[0], nil
	}
	return rawData, nil
}

func readShortValue(client mcp.Client, address string, length int) (interface{}, error) {
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

func readUShortValue(client mcp.Client, address string, length int) (interface{}, error) {
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

func readIntValue(client mcp.Client, address string, length int) (interface{}, error) {
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

func readUIntValue(client mcp.Client, address string, length int) (interface{}, error) {
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

func readLongValue(client mcp.Client, address string, length int) (interface{}, error) {
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

func readULongValue(client mcp.Client, address string, length int) (interface{}, error) {
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

func readFloatValue(client mcp.Client, address string, length int) (interface{}, error) {
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

func readDoubleValue(client mcp.Client, address string, length int) (interface{}, error) {
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

func readStringValue(client mcp.Client, address string, length int) (interface{}, error) {
	_, rawData, err := ReadMelsec(client, address, length)
	if err != nil {
		return nil, err
	}
	return strings.TrimRight(string(rawData), "\x00"), nil
}

func writeBoolValue(client mcp.Client, address string, length int, value interface{}) error {
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

func writeIntValue(client mcp.Client, address string, byteLen int, value interface{}) error {
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

func writeFloatValue(client mcp.Client, address string, length int, value interface{}) error {
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

func writeDoubleValue(client mcp.Client, address string, length int, value interface{}) error {
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

func writeStringValue(client mcp.Client, address string, length int, value interface{}) error {
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

func ParseAddress(address string) (deviceName string, offset int64, err error) {
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

func (m *McService) ReadParams(ctx context.Context, params protocol.Metadata) (protocol.Metadata, error) {
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
