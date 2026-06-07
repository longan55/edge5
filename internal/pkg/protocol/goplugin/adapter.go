package goplugin

import (
	"context"
	"fmt"
	"sync"

	pb "edge5/plugins/proto"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// ConnectionParam 协议连接参数
type ConnectionParam struct {
	Name     string   `json:"name"`
	CName    string   `json:"cName"`
	Type     string   `json:"type"`
	Required bool     `json:"required"`
	Default  string   `json:"default,omitempty"`
	Choices  []string `json:"choices,omitempty"`
}

// Info 协议信息（由 registry 转换为 protocol.ProtocolInfo）
type Info struct {
	Name             string
	Version          string
	DeviceType       string
	Brand            string
	Models           []string
	ConnectionParams []ConnectionParam
	Source           string
	PluginPath       string
}

// DataMessage 订阅数据消息
type DataMessage struct {
	DeviceSn  string
	Values    map[string][]byte
	Timestamp int64
}

// PluginAdapter 将 gRPC DevicePlugin 适配为统一的 Protocol 接口
type PluginAdapter struct {
	pluginPath string
	grpcAddr   string
	client     pb.DevicePluginClient
	conn       *grpc.ClientConn
	info       *Info
	mu         sync.RWMutex
	logger     *zap.Logger
}

// NewPluginAdapter 创建 gRPC 插件适配器
func NewPluginAdapter(pluginPath, grpcAddr string) *PluginAdapter {
	return &PluginAdapter{
		pluginPath: pluginPath,
		grpcAddr:   grpcAddr,
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

	info := &Info{
		Name:       infoResp.GetName(),
		Version:    infoResp.GetVersion(),
		DeviceType: infoResp.GetDeviceType(),
		Brand:      infoResp.GetBrand(),
		Source:     "plugin",
		PluginPath: a.pluginPath,
		Models:     infoResp.GetProtocols(),
	}
	info.ConnectionParams = parseDefaultParams(info.Name)

	a.conn = conn
	a.client = client
	a.info = info

	a.logger.Info("gRPC 插件适配器初始化成功",
		zap.String("name", info.Name),
		zap.String("addr", a.grpcAddr),
		zap.String("version", info.Version))

	return nil
}

// GetInfo 返回协议信息
func (a *PluginAdapter) GetInfo() Info {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.info == nil {
		return Info{}
	}
	return *a.info
}

// Connect 建立设备连接
func (a *PluginAdapter) Connect(ctx context.Context, deviceSn string, params map[string]string) error {
	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()

	if client == nil {
		return fmt.Errorf("plugin not initialized")
	}

	resp, err := client.Connect(ctx, &pb.ConnectRequest{
		DeviceSn: deviceSn,
		Params:   params,
	})
	if err != nil {
		return fmt.Errorf("plugin connect error: %w", err)
	}
	if !resp.GetSuccess() {
		return fmt.Errorf("plugin connect failed: %s", resp.GetMessage())
	}
	return nil
}

// Disconnect 断开设备连接
func (a *PluginAdapter) Disconnect(ctx context.Context, deviceSn string) error {
	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()

	if client == nil {
		return nil
	}

	_, _ = client.Disconnect(ctx, &pb.DisconnectRequest{DeviceSn: deviceSn})
	return nil
}

// ReadData 读取数据
func (a *PluginAdapter) ReadData(ctx context.Context, deviceSn string, addresses []string) (map[string][]byte, error) {
	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()

	if client == nil {
		return nil, fmt.Errorf("plugin not initialized")
	}

	resp, err := client.ReadData(ctx, &pb.ReadRequest{
		DeviceSn:  deviceSn,
		Addresses: addresses,
	})
	if err != nil {
		return nil, fmt.Errorf("plugin read error: %w", err)
	}
	if !resp.GetSuccess() {
		return nil, fmt.Errorf("plugin read failed: %s", resp.GetMessage())
	}
	return resp.GetValues(), nil
}

// WriteData 写入数据
func (a *PluginAdapter) WriteData(ctx context.Context, deviceSn string, values map[string][]byte) error {
	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()

	if client == nil {
		return fmt.Errorf("plugin not initialized")
	}

	resp, err := client.WriteData(ctx, &pb.WriteRequest{
		DeviceSn: deviceSn,
		Values:   values,
	})
	if err != nil {
		return fmt.Errorf("plugin write error: %w", err)
	}
	if !resp.GetSuccess() {
		return fmt.Errorf("plugin write failed: %s", resp.GetMessage())
	}
	return nil
}

// SubscribeData 订阅实时数据
func (a *PluginAdapter) SubscribeData(ctx context.Context, deviceSn string, addresses []string, interval int32) (<-chan DataMessage, error) {
	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()

	if client == nil {
		return nil, fmt.Errorf("plugin not initialized")
	}

	stream, err := client.SubscribeData(ctx, &pb.SubscribeRequest{
		DeviceSn:  deviceSn,
		Addresses: addresses,
		Interval:  interval,
	})
	if err != nil {
		return nil, fmt.Errorf("plugin subscribe error: %w", err)
	}

	ch := make(chan DataMessage, 100)
	go func() {
		defer close(ch)
		for {
			resp, err := stream.Recv()
			if err != nil {
				return
			}
			select {
			case ch <- DataMessage{
				DeviceSn:  resp.GetDeviceSn(),
				Values:    resp.GetValues(),
				Timestamp: resp.GetTimestamp(),
			}:
			case <-ctx.Done():
				return
			}
		}
	}()

	return ch, nil
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
var defaultConnectionParams = map[string][]ConnectionParam{
	"MC-3E": {
		{Name: "ip", CName: "IP地址", Type: "string", Required: true},
		{Name: "port", CName: "端口号", Type: "int", Required: true, Default: "6000"},
		{Name: "pcNum", CName: "PC编号", Type: "string", Required: true, Default: "0xFF"},
	},
	"FX-Serial": {
		{Name: "serialPort", CName: "串口号", Type: "string", Required: true},
		{Name: "baudRate", CName: "波特率", Type: "int", Required: true, Default: "9600", Choices: []string{"9600", "19200", "38400", "115200"}},
		{Name: "dataBit", CName: "数据位", Type: "int", Required: true, Default: "7"},
		{Name: "stopBit", CName: "停止位", Type: "float", Required: true, Default: "1"},
		{Name: "parity", CName: "校验位", Type: "string", Required: true, Default: "even", Choices: []string{"odd", "even"}},
	},
	"S7Comm": {
		{Name: "ip", CName: "IP地址", Type: "string", Required: true},
		{Name: "rack", CName: "机架号", Type: "int", Required: true, Default: "0"},
		{Name: "slot", CName: "槽号", Type: "int", Required: true, Default: "2"},
	},
	"Focas": {
		{Name: "ip", CName: "IP地址", Type: "string", Required: true},
		{Name: "port", CName: "端口号", Type: "int", Required: true, Default: "8193"},
	},
	"Melsec-CNC": {
		{Name: "ip", CName: "IP地址", Type: "string", Required: true},
		{Name: "port", CName: "端口号", Type: "int", Required: true, Default: "683"},
	},
}

func parseDefaultParams(protocolName string) []ConnectionParam {
	if params, ok := defaultConnectionParams[protocolName]; ok {
		return params
	}
	return []ConnectionParam{
		{Name: "ip", CName: "IP地址", Type: "string", Required: true},
		{Name: "port", CName: "端口号", Type: "int", Required: true},
	}
}
