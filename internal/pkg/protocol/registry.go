package protocol

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"edge5/internal/pkg/protocol/goplugin"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// globalRegistry 全局协议注册表
var globalRegistry = &registry{
	protocols: make(map[string]DeviceCommProtocol),
	plugins:   make(map[string]*pluginProcess),
}

// DefaultRegistry 返回全局注册表实例
func DefaultRegistry() ProtocolRegistry {
	return globalRegistry
}

// registry 协议注册表实现
type registry struct {
	mu        sync.RWMutex
	protocols map[string]DeviceCommProtocol
	plugins   map[string]*pluginProcess
	db        *gorm.DB
	logger    *zap.Logger
}

type pluginProcess struct {
	cmd  *exec.Cmd
	info Metadata
}

// SetDB 设置数据库连接（用于 SyncToDB）
func (r *registry) SetDB(db *gorm.DB) {
	r.db = db
}

// SetLogger 设置日志记录器
func (r *registry) SetLogger(logger *zap.Logger) {
	if logger == nil {
		logger = zap.NewNop()
	}
	r.logger = logger
}

// Register 注册一个协议实现
func (r *registry) Register(proto DeviceCommProtocol) error {
	info := proto.Info()
	name := GetInfoString(info, "name")
	if name == "" {
		return fmt.Errorf("protocol name cannot be empty (metadata missing 'name')")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.protocols[name]; exists {
		return fmt.Errorf("protocol %q already registered", name)
	}

	r.protocols[name] = proto
	if r.logger != nil {
		r.logger.Info("协议已注册",
			zap.String("name", name),
			zap.String("device_type", GetInfoString(info, "deviceType")),
			zap.String("brand", GetInfoString(info, "brand")),
			zap.String("source", GetInfoString(info, "source")),
		)
	}
	return nil
}

// Get 根据协议名称获取协议实现
func (r *registry) Get(name string) (DeviceCommProtocol, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	proto, ok := r.protocols[name]
	return proto, ok
}

// List 列出所有已注册的协议元信息
func (r *registry) List() []Metadata {
	r.mu.RLock()
	defer r.mu.RUnlock()

	infos := make([]Metadata, 0, len(r.protocols))
	for _, proto := range r.protocols {
		infos = append(infos, proto.Info())
	}
	return infos
}

// SyncToDB 将注册表中的协议信息同步到数据库
func (r *registry) SyncToDB() error {
	if r.db == nil {
		return fmt.Errorf("database not set")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	for name, proto := range r.protocols {
		info := proto.Info()

		version := GetInfoString(info, "version")
		deviceType := GetInfoString(info, "deviceType")
		brand := GetInfoString(info, "brand")
		source := GetInfoString(info, "source")
		if source == "" {
			source = "builtin"
		}
		pluginPath := GetInfoString(info, "pluginPath")
		models := GetInfoStrings(info, "models")

		cp := ExtractConnectionParams(info)
		paramsJSON := ConnectionParamsToJSON(cp)

		modelsJSON, err := json.Marshal(models)
		if err != nil {
			modelsJSON = []byte("[]")
		}

		var count int64
		r.db.Model(&struct{}{}).Table("protocol_registry").
			Where("name = ?", name).Count(&count)

		if count > 0 {
			err = r.db.Exec(
				`UPDATE protocol_registry SET
				 version = ?, device_type = ?, brand = ?, source = ?,
				 plugin_path = ?, connection_params = ?, models = ?,
				 enabled = 1
				 WHERE name = ?`,
				version, deviceType, brand, source,
				pluginPath, paramsJSON, string(modelsJSON),
				name,
			).Error
		} else {
			err = r.db.Exec(
				`INSERT INTO protocol_registry
				 (name, version, device_type, brand, source, plugin_path, connection_params, models, enabled)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1)`,
				name, version, deviceType, brand, source,
				pluginPath, paramsJSON, string(modelsJSON),
			).Error
		}

		if err != nil {
			if r.logger != nil {
				r.logger.Error("同步协议到数据库失败",
					zap.String("name", name),
					zap.Error(err))
			}
			continue
		}

		if r.logger != nil {
			r.logger.Debug("协议已同步到数据库",
				zap.String("name", name))
		}
	}

	return nil
}

// StartPlugins 启动所有已注册的 gRPC 插件进程
func (r *registry) StartPlugins() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for name, proto := range r.protocols {
		info := proto.Info()
		source := GetInfoString(info, "source")
		if source != "plugin" {
			continue
		}

		pluginPath := GetInfoString(info, "pluginPath")
		if pluginPath == "" {
			if r.logger != nil {
				r.logger.Warn("插件路径为空，跳过启动",
					zap.String("name", name))
			}
			continue
		}

		absPath, err := filepath.Abs(pluginPath)
		if err != nil {
			if r.logger != nil {
				r.logger.Warn("获取插件绝对路径失败",
					zap.String("name", name),
					zap.Error(err))
			}
			continue
		}

		if _, err := os.Stat(absPath); os.IsNotExist(err) {
			if r.logger != nil {
				r.logger.Warn("插件文件不存在，跳过启动",
					zap.String("name", name),
					zap.String("path", absPath))
			}
			continue
		}

		cmd := exec.Command(absPath)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Start(); err != nil {
			if r.logger != nil {
				r.logger.Error("启动插件进程失败",
					zap.String("name", name),
					zap.String("path", absPath),
					zap.Error(err))
			}
			continue
		}

		r.plugins[name] = &pluginProcess{
			cmd:  cmd,
			info: info,
		}

		if r.logger != nil {
			r.logger.Info("插件进程已启动",
				zap.String("name", name),
				zap.String("path", absPath),
				zap.Int("pid", cmd.Process.Pid))
		}
	}

	return nil
}

// StopPlugins 停止所有已启动的插件进程
func (r *registry) StopPlugins() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for name, p := range r.plugins {
		if p.cmd != nil && p.cmd.Process != nil {
			if err := p.cmd.Process.Kill(); err != nil {
				if r.logger != nil {
					r.logger.Warn("停止插件进程失败",
						zap.String("name", name),
						zap.Error(err))
				}
			} else {
				if r.logger != nil {
					r.logger.Info("插件进程已停止",
						zap.String("name", name),
						zap.Int("pid", p.cmd.Process.Pid))
				}
			}
		}
		delete(r.plugins, name)
	}

	return nil
}

// ---------------------------------------------------------------------------
// gopluginBridge — 将 PluginAdapter 适配为 DeviceCommProtocol
// ---------------------------------------------------------------------------

type gopluginBridge struct {
	adapter *goplugin.PluginAdapter
}

func (b *gopluginBridge) Info() Metadata {
	info := b.adapter.GetInfo()
	return Metadata(info)
}

func (b *gopluginBridge) Connect(ctx context.Context, params Metadata) error {
	return b.adapter.Connect(ctx, params)
}

func (b *gopluginBridge) Disconnect(ctx context.Context) error {
	return b.adapter.Disconnect(ctx)
}

func (b *gopluginBridge) IsConnected() bool {
	return b.adapter.IsConnected()
}

func (b *gopluginBridge) IsSupportServer() bool {
	return b.adapter.IsSupportServer()
}

func (b *gopluginBridge) ReadBatch(ctx context.Context, req BatchReadRequest) (*BatchReadResponse, error) {
	result, err := b.adapter.ReadBatch(ctx, req)
	if err != nil {
		return nil, err
	}
	resp, ok := result.(*BatchReadResponse)
	if !ok {
		return nil, fmt.Errorf("unexpected ReadBatch result type: %T", result)
	}
	return resp, nil
}

func (b *gopluginBridge) WriteBatch(ctx context.Context, req BatchWriteRequest) error {
	return b.adapter.WriteBatch(ctx, req)
}

// LoadPluginsFromDir 扫描插件目录，加载 gRPC 插件
func (r *registry) LoadPluginsFromDir(dir string) error {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolve plugin dir failed: %w", err)
	}

	entries, err := os.ReadDir(absDir)
	if err != nil {
		if os.IsNotExist(err) {
			if r.logger != nil {
				r.logger.Warn("插件目录不存在，跳过加载",
					zap.String("dir", absDir))
			}
			return nil
		}
		return fmt.Errorf("read plugin dir failed: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		finfo, err := entry.Info()
		if err != nil {
			continue
		}
		if finfo.Mode()&0111 == 0 {
			continue
		}

		pluginPath := filepath.Join(absDir, entry.Name())

		// 解析文件名获取 gRPC 地址  name@host:port
		fileName := entry.Name()
		if strings.HasSuffix(strings.ToLower(fileName), ".exe") {
			fileName = fileName[:len(fileName)-4]
		}

		atIdx := strings.LastIndex(fileName, "@")
		var grpcAddr string
		if atIdx > 0 && atIdx < len(fileName)-1 {
			grpcAddr = fileName[atIdx+1:]
		} else {
			grpcAddr = "127.0.0.1:50052"
		}

		if !strings.Contains(grpcAddr, ":") {
			grpcAddr = grpcAddr + ":50052"
		}

		adapter := goplugin.NewPluginAdapter(pluginPath, grpcAddr)
		adapter.SetLogger(r.logger)

		if err := adapter.Init(); err != nil {
			if r.logger != nil {
				r.logger.Warn("初始化插件适配器失败",
					zap.String("path", pluginPath),
					zap.String("addr", grpcAddr),
					zap.Error(err))
			}
			continue
		}

		if r.logger != nil {
			info := adapter.GetInfo()
			r.logger.Info("gRPC 插件加载成功",
				zap.String("name", GetInfoString(info, "name")),
				zap.String("addr", grpcAddr))
		}

		bridge := &gopluginBridge{adapter: adapter}
		if err := r.Register(bridge); err != nil {
			if r.logger != nil {
				r.logger.Warn("注册插件协议失败",
					zap.String("path", pluginPath),
					zap.Error(err))
			}
			continue
		}
	}

	return nil
}
