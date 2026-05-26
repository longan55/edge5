package plugin

// import (
// 	"context"
// 	"edge5/global"
// 	"fmt"
// 	"sync"

// 	"go.uber.org/zap"
// 	"google.golang.org/grpc"
// 	"google.golang.org/grpc/credentials/insecure"
// )

// type DataHandler func(deviceSn string, data map[string][]byte, timestamp int64)

// type PluginInfo struct {
// 	Name       string   `json:"name"`
// 	Version    string   `json:"version"`
// 	DeviceType string   `json:"device_type"`
// 	Brand      string   `json:"brand"`
// 	Protocols  []string `json:"protocols"`
// }

// type PluginManager interface {
// 	LoadPlugin(name string, addr string) error
// 	UnloadPlugin(name string) error
// 	GetPlugin(name string) (DevicePlugin, bool)
// 	ListPlugins() []PluginInfo
// 	RegisterDataHandler(handler DataHandler)
// }

// type DevicePlugin interface {
// 	GetInfo() (*PluginInfo, error)
// 	Connect(deviceSn string, protocol string, params map[string]string) error
// 	Disconnect(deviceSn string) error
// 	ReadData(deviceSn string, addresses []string) (map[string][]byte, error)
// 	WriteData(deviceSn string, values map[string][]byte) error
// }

// type pluginManager struct {
// 	plugins map[string]*pluginInstance
// 	mu      sync.RWMutex
// 	handler DataHandler
// }

// type pluginInstance struct {
// 	info    *PluginInfo
// 	conn    *grpc.ClientConn
// 	client  DevicePluginClient
// 	handler DataHandler
// }

// func NewPluginManager() PluginManager {
// 	return &pluginManager{
// 		plugins: make(map[string]*pluginInstance),
// 	}
// }

// func (pm *pluginManager) LoadPlugin(name string, addr string) error {
// 	pm.mu.Lock()
// 	defer pm.mu.Unlock()

// 	if _, exists := pm.plugins[name]; exists {
// 		return fmt.Errorf("plugin %s already loaded", name)
// 	}

// 	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
// 	if err != nil {
// 		return fmt.Errorf("failed to connect to plugin %s at %s: %w", name, addr, err)
// 	}

// 	client := NewDevicePluginClient(conn)

// 	info, err := client.GetPluginInfo(context.Background(), &InfoRequest{})
// 	if err != nil {
// 		conn.Close()
// 		return fmt.Errorf("failed to get plugin info: %w", err)
// 	}

// 	pm.plugins[name] = &pluginInstance{
// 		info: &PluginInfo{
// 			Name:       info.Name,
// 			Version:    info.Version,
// 			DeviceType: info.DeviceType,
// 			Brand:      info.Brand,
// 			Protocols:  info.Protocols,
// 		},
// 		conn:   conn,
// 		client: client,
// 	}

// 	global.Logger.Info("插件加载成功",
// 		zap.String("name", name),
// 		zap.String("version", info.Version))

// 	return nil
// }

// func (pm *pluginManager) UnloadPlugin(name string) error {
// 	pm.mu.Lock()
// 	defer pm.mu.Unlock()

// 	instance, exists := pm.plugins[name]
// 	if !exists {
// 		return fmt.Errorf("plugin %s not found", name)
// 	}

// 	if err := instance.conn.Close(); err != nil {
// 		global.Logger.Error("关闭插件连接失败",
// 			zap.String("name", name),
// 			zap.Error(err))
// 	}

// 	delete(pm.plugins, name)

// 	global.Logger.Info("插件卸载成功", zap.String("name", name))
// 	return nil
// }

// func (pm *pluginManager) GetPlugin(name string) (DevicePlugin, bool) {
// 	pm.mu.RLock()
// 	defer pm.mu.RUnlock()

// 	instance, exists := pm.plugins[name]
// 	if !exists {
// 		return nil, false
// 	}

// 	return instance.client, true
// }

// func (pm *pluginManager) ListPlugins() []PluginInfo {
// 	pm.mu.RLock()
// 	defer pm.mu.RUnlock()

// 	plugins := make([]PluginInfo, 0, len(pm.plugins))
// 	for _, instance := range pm.plugins {
// 		plugins = append(plugins, *instance.info)
// 	}

// 	return plugins
// }

// func (pm *pluginManager) RegisterDataHandler(handler DataHandler) {
// 	pm.mu.Lock()
// 	defer pm.mu.Unlock()
// 	pm.handler = handler
// }

// type grpcDevicePlugin struct {
// 	client DevicePluginClient
// }

// func (g *grpcDevicePlugin) GetInfo() (*PluginInfo, error) {
// 	resp, err := g.client.GetPluginInfo(context.Background(), &InfoRequest{})
// 	if err != nil {
// 		return nil, err
// 	}

// 	return &PluginInfo{
// 		Name:       resp.Name,
// 		Version:    resp.Version,
// 		DeviceType: resp.DeviceType,
// 		Brand:      resp.Brand,
// 		Protocols:  resp.Protocols,
// 	}, nil
// }

// func (g *grpcDevicePlugin) Connect(deviceSn string, protocol string, params map[string]string) error {
// 	_, err := g.client.Connect(context.Background(), &ConnectRequest{
// 		DeviceSn: deviceSn,
// 		Protocol: protocol,
// 		Params:   params,
// 	})
// 	return err
// }

// func (g *grpcDevicePlugin) Disconnect(deviceSn string) error {
// 	_, err := g.client.Disconnect(context.Background(), &DisconnectRequest{
// 		DeviceSn: deviceSn,
// 	})
// 	return err
// }

// func (g *grpcDevicePlugin) ReadData(deviceSn string, addresses []string) (map[string][]byte, error) {
// 	resp, err := g.client.ReadData(context.Background(), &ReadRequest{
// 		DeviceSn:  deviceSn,
// 		Addresses: addresses,
// 	})
// 	if err != nil {
// 		return nil, err
// 	}

// 	return resp.Values, nil
// }

// func (g *grpcDevicePlugin) WriteData(deviceSn string, values map[string][]byte) error {
// 	_, err := g.client.WriteData(context.Background(), &WriteRequest{
// 		DeviceSn: deviceSn,
// 		Values:   values,
// 	})
// 	return err
// }

// func (g *grpcDevicePlugin) SubscribeData(deviceSn string, addresses []string, interval int32) (<-chan *DataResponse, error) {
// 	stream, err := g.client.SubscribeData(context.Background(), &SubscribeRequest{
// 		DeviceSn:  deviceSn,
// 		Addresses: addresses,
// 		Interval:  interval,
// 	})
// 	if err != nil {
// 		return nil, err
// 	}

// 	ch := make(chan *DataResponse)
// 	go func() {
// 		defer close(ch)
// 		for {
// 			resp, err := stream.Recv()
// 			if err != nil {
// 				return
// 			}
// 			select {
// 			case ch <- resp:
// 			case <-stream.Context().Done():
// 				return
// 			}
// 		}
// 	}()

// 	return ch, nil
// }
