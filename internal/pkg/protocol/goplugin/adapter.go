package goplugin

import (
	"context"
	"fmt"
	"sync"
	"unsafe"

	pb "edge5/plugins/proto"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// PluginAdapter 将 gRPC DevicePlugin 适配为统一的协议接口。
// 注意：此包不导入 protocol 包（避免循环导入）。
// registry.go 中的 bridge 类型负责将本适配器转换为 protocol.DeviceCommProtocol。
type PluginAdapter struct {
	pluginPath string
	grpcAddr   string
	client     pb.DevicePluginClient
	conn       *grpc.ClientConn
	info       map[string]any
	states     map[string]*pluginState
	mu         sync.RWMutex
	logger     *zap.Logger
}

type pluginState struct {
	connected bool
	deviceSn  string
}

// NewPluginAdapter 创建 gRPC 插件适配器
func NewPluginAdapter(pluginPath, grpcAddr string) *PluginAdapter {
	return &PluginAdapter{
		pluginPath: pluginPath,
		grpcAddr:   grpcAddr,
		info:       make(map[string]any),
		states:     make(map[string]*pluginState),
		logger:     zap.NewNop(),
	}
}

// SetLogger 设置日志记录器
func (a *PluginAdapter) SetLogger(logger *zap.Logger) {
	if logger != nil {
		a.logger = logger
	}
}

// Init 初始化 gRPC 连接并获取插件信息
func (a *PluginAdapter) Init() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.conn != nil {
		return nil
	}

	conn, err := grpc.Dial(a.grpcAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return fmt.Errorf("dial plugin gRPC %s failed: %w", a.grpcAddr, err)
	}

	client := pb.NewDevicePluginClient(conn)

	infoResp, err := client.GetPluginInfo(context.Background(), &pb.InfoRequest{})
	if err != nil {
		conn.Close()
		return fmt.Errorf("get plugin info failed: %w", err)
	}

	a.info = map[string]any{
		"name":       infoResp.GetName(),
		"version":    infoResp.GetVersion(),
		"deviceType": infoResp.GetDeviceType(),
		"brand":      infoResp.GetBrand(),
		"models":     infoResp.GetProtocols(),
		"source":     "plugin",
		"pluginPath": a.pluginPath,
	}
	if cp := defaultConnectionParams[infoResp.GetName()]; cp != nil {
		a.info["connectionParams"] = cp
	} else {
		a.info["connectionParams"] = []map[string]any{
			{"name": "ip", "cName": "IP地址", "type": "string", "required": true},
			{"name": "port", "cName": "端口号", "type": "int", "required": true},
		}
	}

	a.conn = conn
	a.client = client

	a.logger.Info("gRPC 插件适配器初始化成功",
		zap.String("name", infoResp.GetName()),
		zap.String("addr", a.grpcAddr),
		zap.String("version", infoResp.GetVersion()))

	return nil
}

// GetInfo 返回协议元信息（map 格式）
func (a *PluginAdapter) GetInfo() map[string]any {
	a.mu.RLock()
	defer a.mu.RUnlock()
	result := make(map[string]any, len(a.info))
	for k, v := range a.info {
		result[k] = v
	}
	return result
}

// Connect 建立设备连接
func (a *PluginAdapter) Connect(ctx context.Context, params map[string]any) error {
	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()

	if client == nil {
		return fmt.Errorf("plugin not initialized")
	}

	deviceSn, _ := params["deviceSn"].(string)
	if deviceSn == "" {
		return fmt.Errorf("missing deviceSn in params")
	}

	protoParams := make(map[string]string)
	for k, v := range params {
		if k == "deviceSn" {
			continue
		}
		protoParams[k] = fmt.Sprintf("%v", v)
	}

	resp, err := client.Connect(ctx, &pb.ConnectRequest{
		DeviceSn: deviceSn,
		Params:   protoParams,
	})
	if err != nil {
		return fmt.Errorf("plugin connect error: %w", err)
	}
	if !resp.GetSuccess() {
		return fmt.Errorf("plugin connect failed: %s", resp.GetMessage())
	}

	a.mu.Lock()
	a.states[deviceSn] = &pluginState{connected: true, deviceSn: deviceSn}
	a.mu.Unlock()

	return nil
}

// Disconnect 断开所有设备连接
func (a *PluginAdapter) Disconnect(ctx context.Context) error {
	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()

	if client == nil {
		return nil
	}

	a.mu.Lock()
	devices := make([]string, 0, len(a.states))
	for sn := range a.states {
		devices = append(devices, sn)
	}
	a.mu.Unlock()

	for _, sn := range devices {
		_, _ = client.Disconnect(ctx, &pb.DisconnectRequest{DeviceSn: sn})
		a.mu.Lock()
		delete(a.states, sn)
		a.mu.Unlock()
	}

	return nil
}

// IsConnected 是否有设备在线
func (a *PluginAdapter) IsConnected() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.states) > 0
}

func (a *PluginAdapter) IsSupportServer() bool {
	return false
}

// ReadBatch 批量读取（通过 gRPC 调用插件的 ReadData 接口）
// 返回 map[string]any 避免循环导入，bridge 会做类型转换
func (a *PluginAdapter) ReadBatch(ctx context.Context, req interface{}) (interface{}, error) {
	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()

	if client == nil {
		return nil, fmt.Errorf("plugin not initialized")
	}

	reqMap, ok := req.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid request type: %T", req)
	}

	pointsRaw, _ := reqMap["Points"].([]interface{})
	addresses := make([]string, 0, len(pointsRaw))
	pointMap := make(map[string]map[string]any)
	for _, p := range pointsRaw {
		if pointMapItem, ok := p.(map[string]any); ok {
			name, _ := pointMapItem["Name"].(string)
			resource, _ := pointMapItem["Resource"].(string)
			dataType, _ := pointMapItem["DataType"].(string)
			addresses = append(addresses, resource)
			pointMap[resource] = map[string]any{
				"Name":     name,
				"DataType": dataType,
			}
		}
	}

	resp, err := client.ReadData(ctx, &pb.ReadRequest{
		Addresses: addresses,
	})
	if err != nil {
		return nil, fmt.Errorf("plugin ReadData error: %w", err)
	}

	if !resp.GetSuccess() {
		return nil, fmt.Errorf("plugin ReadData failed: %s", resp.GetMessage())
	}

	results := make([]interface{}, 0, len(resp.GetValues()))
	for addr, rawValue := range resp.GetValues() {
		if p, ok := pointMap[addr]; ok {
			value := parseRawValue(rawValue, p["DataType"].(string))
			results = append(results, map[string]any{
				"PointName": p["Name"],
				"Value":     value,
				"Quality":   "good",
			})
		}
	}

	return map[string]any{
		"Results": results,
		"Raw":     nil,
	}, nil
}

func parseRawValue(raw []byte, dataType string) interface{} {
	switch dataType {
	case "bool":
		return len(raw) > 0 && raw[0] != 0
	case "short":
		if len(raw) >= 2 {
			return int16(raw[0]) | int16(raw[1])<<8
		}
	case "ushort":
		if len(raw) >= 2 {
			return uint16(raw[0]) | uint16(raw[1])<<8
		}
	case "int":
		if len(raw) >= 4 {
			return int32(raw[0]) | int32(raw[1])<<8 | int32(raw[2])<<16 | int32(raw[3])<<24
		}
	case "uint":
		if len(raw) >= 4 {
			return uint32(raw[0]) | uint32(raw[1])<<8 | uint32(raw[2])<<16 | uint32(raw[3])<<24
		}
	case "long":
		if len(raw) >= 8 {
			return int64(raw[0]) | int64(raw[1])<<8 | int64(raw[2])<<16 | int64(raw[3])<<24 |
				int64(raw[4])<<32 | int64(raw[5])<<40 | int64(raw[6])<<48 | int64(raw[7])<<56
		}
	case "ulong":
		if len(raw) >= 8 {
			return uint64(raw[0]) | uint64(raw[1])<<8 | uint64(raw[2])<<16 | uint64(raw[3])<<24 |
				uint64(raw[4])<<32 | uint64(raw[5])<<40 | uint64(raw[6])<<48 | uint64(raw[7])<<56
		}
	case "float":
		if len(raw) >= 4 {
			bits := uint32(raw[0]) | uint32(raw[1])<<8 | uint32(raw[2])<<16 | uint32(raw[3])<<24
			return *(*float32)(unsafe.Pointer(&bits))
		}
	case "double":
		if len(raw) >= 8 {
			bits := uint64(raw[0]) | uint64(raw[1])<<8 | uint64(raw[2])<<16 | uint64(raw[3])<<24 |
				uint64(raw[4])<<32 | uint64(raw[5])<<40 | uint64(raw[6])<<48 | uint64(raw[7])<<56
			return *(*float64)(unsafe.Pointer(&bits))
		}
	case "string":
		return string(raw)
	default:
		return string(raw)
	}
	return string(raw)
}

// WriteBatch 批量写入（直接使用 interface{} 避免循环导入）
func (a *PluginAdapter) WriteBatch(ctx context.Context, req interface{}) error {
	return fmt.Errorf("WriteBatch not implemented for gRPC plugin adapter")
}

// Close 关闭 gRPC 连接
func (a *PluginAdapter) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.conn != nil {
		return a.conn.Close()
	}
	return nil
}

// 默认连接参数配置
var defaultConnectionParams = map[string][]map[string]any{
	"MC-3E": {
		{"name": "ip", "cName": "IP地址", "type": "string", "required": true},
		{"name": "port", "cName": "端口号", "type": "int", "required": true, "default": "6000"},
		{"name": "pcNum", "cName": "PC编号", "type": "string", "required": true, "default": "0xFF"},
	},
	"FX-Serial": {
		{"name": "serialPort", "cName": "串口号", "type": "string", "required": true},
		{"name": "baudRate", "cName": "波特率", "type": "int", "required": true, "default": "9600", "choices": []string{"9600", "19200", "38400", "115200"}},
		{"name": "dataBit", "cName": "数据位", "type": "int", "required": true, "default": "7"},
		{"name": "stopBit", "cName": "停止位", "type": "float", "required": true, "default": "1"},
		{"name": "parity", "cName": "校验位", "type": "string", "required": true, "default": "even", "choices": []string{"odd", "even"}},
	},
	"S7Comm": {
		{"name": "ip", "cName": "IP地址", "type": "string", "required": true},
		{"name": "rack", "cName": "机架号", "type": "int", "required": true, "default": "0"},
		{"name": "slot", "cName": "槽号", "type": "int", "required": true, "default": "2"},
	},
	"Focas": {
		{"name": "ip", "cName": "IP地址", "type": "string", "required": true},
		{"name": "port", "cName": "端口号", "type": "int", "required": true, "default": "8193"},
	},
	"Melsec-CNC": {
		{"name": "ip", "cName": "IP地址", "type": "string", "required": true},
		{"name": "port", "cName": "端口号", "type": "int", "required": true, "default": "683"},
	},
}
