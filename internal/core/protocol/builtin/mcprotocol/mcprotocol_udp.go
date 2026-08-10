package mcprotocol

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"edge5/internal/core/protocol"
	"edge5/internal/core/protocol/builtin"

	"github.com/longan55/go-mcprotocol/mcp"
	"go.uber.org/zap"
)

func init() {
	if err := builtin.Register(NewMcUDPService()); err != nil {
		panic(fmt.Sprintf("注册 MelsecMC-UDP 协议失败: %v", err))
	}
}

// udpReadTimeout UDP 读取超时时间。
// UDP 无连接，若设备无响应将永久阻塞，必须设置读超时。
// 取 3s 与 3E 帧监控定时器（MONITORING_TIMER）保持一致。
const udpReadTimeout = 3 * time.Second

// udpReadBufferSize UDP 单次读取缓冲区大小。
// MC 3E 帧单包响应通常远小于以太网 MTU，2KB 足够容纳单数据报。
const udpReadBufferSize = 2048

// udpClient 基于 UDP 传输实现 mcp.Client 接口。
// 报文格式（3E 帧）与 TCP 版本完全一致，仅传输层不同：
// 每次 Read/Write/BitRead/HealthCheck 都通过 DialUDP 建立短连接收发一次数据报。
//
// 注意：go-mcprotocol 库的 station 类型未导出，无法作为字段持有，
// 因此每次构建请求时通过 mcp.NewStation 重建实例并调用其导出的 BuildXxxRequest 方法。
// 这些方法只负责拼装 hex 报文，与传输层无关，可安全复用。
type udpClient struct {
	udpAddr        *net.UDPAddr
	networkNumber  string
	pcNum          string
	unitIONum      string
	unitStationNum string
}

// 编译期断言：udpClient 必须实现 mcp.Client 接口
var _ mcp.Client = (*udpClient)(nil)

func newUDPClient(ip string, port int, networkNumber, pcNum, unitIONum, unitStationNum string) (*udpClient, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%v:%v", ip, port))
	if err != nil {
		return nil, err
	}
	return &udpClient{
		udpAddr:        udpAddr,
		networkNumber:  networkNumber,
		pcNum:          pcNum,
		unitIONum:      unitIONum,
		unitStationNum: unitStationNum,
	}, nil
}

// Connect UDP 无连接概念，这里为满足 mcp.Client 接口，空实现。
func (c *udpClient) Connect() error { return nil }

// Close UDP 短连接在 send 内部已关闭，这里为满足 mcp.Client 接口，空实现。
func (c *udpClient) Close() error { return nil }

// send 通过 UDP 发送一帧请求并读取一帧响应（短连接模式）。
// 使用 DialUDP 建立“已连接”UDP socket，限定目标地址，Read 仅接收该对端数据报。
func (c *udpClient) send(payload []byte) ([]byte, error) {
	conn, err := net.DialUDP("udp", nil, c.udpAddr)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if err := conn.SetReadDeadline(time.Now().Add(udpReadTimeout)); err != nil {
		return nil, err
	}

	if _, err := conn.Write(payload); err != nil {
		return nil, err
	}

	buf := make([]byte, udpReadBufferSize)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}

// Read 字读取（word 读），报文与 TCP 版本一致，仅改 UDP 收发。
func (c *udpClient) Read(deviceName string, offset, numPoints int64) ([]byte, error) {
	stn := mcp.NewStation(c.networkNumber, c.pcNum, c.unitIONum, c.unitStationNum)
	requestStr := stn.BuildReadRequest(deviceName, offset, numPoints)

	payload, err := hex.DecodeString(requestStr)
	if err != nil {
		return nil, err
	}
	return c.send(payload)
}

// BitRead 位读取，报文与 TCP 版本一致，仅改 UDP 收发。
func (c *udpClient) BitRead(deviceName string, offset, numPoints int64) ([]byte, error) {
	stn := mcp.NewStation(c.networkNumber, c.pcNum, c.unitIONum, c.unitStationNum)
	requestStr := stn.BuildBitReadRequest(deviceName, offset, numPoints)

	payload, err := hex.DecodeString(requestStr)
	if err != nil {
		return nil, err
	}
	return c.send(payload)
}

// Write 写入，报文与 TCP 版本一致，仅改 UDP 收发。
func (c *udpClient) Write(deviceName string, offset, numPoints int64, writeData []byte) ([]byte, error) {
	stn := mcp.NewStation(c.networkNumber, c.pcNum, c.unitIONum, c.unitStationNum)
	requestStr := stn.BuildWriteRequest(deviceName, offset, numPoints, writeData)

	payload, err := hex.DecodeString(requestStr)
	if err != nil {
		return nil, err
	}
	return c.send(payload)
}

// HealthCheck 折返测试（自回路测试），用于连接探测。
// 发送固定报文，校验返回的长度、数据数头、折返数据是否与期望一致。
func (c *udpClient) HealthCheck() error {
	stn := mcp.NewStation(c.networkNumber, c.pcNum, c.unitIONum, c.unitStationNum)
	requestStr := stn.BuildHealthCheckRequest()

	payload, err := hex.DecodeString(requestStr)
	if err != nil {
		return err
	}

	resp, err := c.send(payload)
	if err != nil {
		return err
	}

	if len(resp) != 18 {
		return errors.New("plc connect test is fail: return length is [" + fmt.Sprintf("%X", resp) + "]")
	}

	// 折返しデータ数ヘッダ[1byte] => 0500
	if "0500" != fmt.Sprintf("%X", resp[11:13]) {
		return errors.New("plc connect test is fail: return header is [" + fmt.Sprintf("%X", resp[11:13]) + "]")
	}

	// 折返しデータ[5byte]=ABCDE
	if "4142434445" != fmt.Sprintf("%X", resp[13:18]) {
		return errors.New("plc connect test is fail: return body is [" + fmt.Sprintf("%X", resp[13:18]) + "]")
	}

	return nil
}

// probe 通过发送实际的读请求验证设备可达性与连接参数正确性。
//
// 注意：折返测试（自回路测试，命令 1906）在部分三菱型号（如 Q 系列）或
// UDP 模式下会被设备拒绝（返回错误码 C059 等），不能用于连接探测。
// 因此改用读请求：读取 D 寄存器 1 个点，校验响应结束码为 0000（正常完成）。
// 只要结束码正常即证明 IP/端口/网络号/站号等连接参数全部正确，且设备支持读写。
func (c *udpClient) probe() error {
	stn := mcp.NewStation(c.networkNumber, c.pcNum, c.unitIONum, c.unitStationNum)
	requestStr := stn.BuildReadRequest("D", 0, 1)

	payload, err := hex.DecodeString(requestStr)
	if err != nil {
		return err
	}

	resp, err := c.send(payload)
	if err != nil {
		return err
	}

	// 3E 帧响应头至少 11 字节：子头(2)+网络号(1)+PC号(1)+IO号(2)+站号(1)+数据长度(2)+结束码(2)
	if len(resp) < 11 {
		return fmt.Errorf("响应长度不足: %d 字节 [%X]", len(resp), resp)
	}

	endCode := fmt.Sprintf("%X", resp[9:11])
	if endCode != "0000" {
		return fmt.Errorf("设备返回错误码: %s", endCode)
	}
	return nil
}

// ---------------------------------------------------------------------------
// McUDPService — UDP 传输的 MC 协议服务
// 业务逻辑（地址解析、数据类型转换、批量读写）完全复用本包内
// readPoint / writePoint / ReadMelsec / WriteMelsec 等包级函数，
// 与 TCP 版本 (McService) 保持一致，仅传输层换为 UDP。
// ---------------------------------------------------------------------------

type McUDPService struct {
	lock             sync.Mutex
	connected        bool
	deviceID         uint
	connectionParams map[uint]protocol.Metadata
}

func NewMcUDPService() *McUDPService {
	return &McUDPService{
		connectionParams: make(map[uint]protocol.Metadata),
	}
}

func (m *McUDPService) Info() protocol.Metadata {
	return protocol.Metadata{
		"name":          "MC-3E-UDP",
		"alias":         "MelsecMC-UDP",
		"version":       "1.0.0",
		"description":   "三菱 PLC MC 协议（3E 帧，Q 系列，UDP 传输）",
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

func (m *McUDPService) IsSupportServer() bool {
	return true
}

// buildClient 从连接参数构建 UDP 客户端，处理默认值。
func (m *McUDPService) buildClient(params protocol.Metadata) (*udpClient, error) {
	ip := builtin.GetString(params, "ip")
	port := builtin.GetInt(params, "port")
	if port <= 0 {
		port = 6000
	}
	if ip == "" {
		return nil, fmt.Errorf("缺少 IP 地址参数")
	}

	networkNumber := builtin.GetString(params, "networkNumber")
	pcNum := builtin.GetString(params, "pcNum")
	unitIONum := builtin.GetString(params, "unitIONum")
	unitStationNum := builtin.GetString(params, "unitStationNum")

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

	return newUDPClient(ip, port, networkNumber, pcNum, unitIONum, unitStationNum)
}

// IsConnected 通过 UDP 折返测试探测设备在线状态。
// 相比 TCP 的 Dial 探测，UDP 必须收到正确响应才算在线，避免误判。
func (m *McUDPService) IsConnected(handle protocol.DeviceHandle) bool {
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

	client, err := m.buildClient(params)
	if err != nil {
		return false
	}
	return client.probe() == nil
}

func (m *McUDPService) Connect(ctx context.Context, params protocol.Metadata) (protocol.DeviceHandle, error) {
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

	client, err := m.buildClient(params)
	if err != nil {
		return protocol.InvalidHandle, err
	}
	// UDP 无连接概念，用读请求验证设备可达与连接参数正确
	// （折返测试命令在 Q 系列/UDP 模式下不被支持，改用读请求探测）
	if err := client.probe(); err != nil {
		return protocol.InvalidHandle, fmt.Errorf("连接 MC(UDP) 客户端失败: %w", err)
	}

	m.connectionParams[deviceID] = params
	m.deviceID = deviceID
	m.connected = true

	if builtin.Logger != nil {
		builtin.Logger.Info("MC(UDP)协议: 连接测试成功",
			zap.Uint("deviceID", deviceID),
			zap.String("ip", ip),
			zap.Int("port", port),
		)
	}
	return protocol.DeviceHandle(deviceID), nil
}

func (m *McUDPService) Disconnect(ctx context.Context, handle protocol.DeviceHandle) error {
	return nil
}

func (m *McUDPService) ReadBatch(ctx context.Context, handle protocol.DeviceHandle, req protocol.BatchReadRequest) (*protocol.BatchReadResponse, error) {
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

	client, err := m.buildClient(params)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	results := make([]protocol.BatchReadResult, 0, len(req.Points))
	for _, pt := range req.Points {
		results = append(results, readPoint(client, pt))
	}

	return &protocol.BatchReadResponse{
		Results: results,
	}, nil
}

func (m *McUDPService) WriteBatch(ctx context.Context, handle protocol.DeviceHandle, req protocol.BatchWriteRequest) error {
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

	client, err := m.buildClient(params)
	if err != nil {
		return err
	}
	defer client.Close()

	for _, item := range req.Items {
		if err := writePoint(client, item); err != nil {
			return fmt.Errorf("写入点位 %s 失败: %w", item.Point.Name, err)
		}
	}
	return nil
}

func (m *McUDPService) ReadParams(ctx context.Context, params protocol.Metadata) (protocol.Metadata, error) {
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
