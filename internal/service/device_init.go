package service

import (
	"context"
	"edge5/global"
	"edge5/internal/core/protocol"
	"edge5/internal/model"
	"encoding/json"
	"fmt"

	"os/exec"
	"runtime"
	"sync"
	"time"

	"go.uber.org/zap"
)

var (
	lastTestTime time.Time
	testMutex    sync.Mutex
	testCooldown = 30 * time.Second // 30秒冷却时间
)

// TestDeviceConnections 异步测试所有设备连接
func TestDeviceConnections() error {
	testMutex.Lock()
	defer testMutex.Unlock()

	// 检查冷却时间
	if time.Since(lastTestTime) < testCooldown {
		return fmt.Errorf("操作过于频繁，请等待 %d 秒后再试", int(testCooldown.Seconds()-time.Since(lastTestTime).Seconds()))
	}

	lastTestTime = time.Now()

	go func() {
		global.Logger.Info("开始异步测试设备连接...")

		var devices []model.Device
		if err := global.DB.Find(&devices).Error; err != nil {
			global.Logger.Error("获取设备列表失败", zap.Error(err))
			return
		}

		if len(devices) == 0 {
			global.Logger.Info("无设备需要测试连接")
			return
		}

		var wg sync.WaitGroup
		timeout := 3 * time.Second

		for _, device := range devices {
			wg.Add(1)
			go func(dev model.Device) {
				defer wg.Done()

				ctx, cancel := context.WithTimeout(context.Background(), timeout)
				defer cancel()

				online := testSingleDeviceConnection(ctx, &dev)

				// 更新设备在线状态
				now := time.Now()
				_ = global.DB.Exec(
					"INSERT INTO device_status (device_id, online, last_heartbeat, message) VALUES (?, ?, ?, ?) ON CONFLICT(device_id) DO UPDATE SET online = excluded.online, last_heartbeat = excluded.last_heartbeat, message = excluded.message",
					dev.ID, online, now, mapStatusMessage(online),
				).Error

				global.Logger.Info("设备连接测试完成",
					zap.Uint64("deviceID", dev.ID),
					zap.String("deviceSN", dev.DeviceSn),
					zap.Bool("online", online),
				)
			}(device)
		}

		wg.Wait()
		global.Logger.Info("所有设备连接测试完成")
	}()
	return nil
}

// testSingleDeviceConnection 测试单个设备连接
func testSingleDeviceConnection(ctx context.Context, device *model.Device) bool {
	global.Logger.Debug("开始测试设备连接", zap.Uint64("deviceID", device.ID))
	// 获取协议实例
	reg := protocol.DefaultRegistry()
	proto, ok := reg.Get(device.Protocol)
	if !ok {
		global.Logger.Warn("协议未注册", zap.String("protocol", device.Protocol))
		return false
	}

	// 解析设备配置参数
	connParams, err := parseDeviceConfigToMetadata(device)
	if err != nil {
		global.Logger.Warn("解析设备配置失败", zap.Uint64("deviceID", device.ID), zap.Error(err))
		return false
	}
	connParams["deviceID"] = float64(device.ID)
	global.Logger.Debug("设备连接参数", zap.Any("params", connParams))

	// 已存在连接（如采集/边缘计算任务持有的持久连接）时直接判定在线：
	// 协议 Connect 会复用该连接，若此处再 Connect+Disconnect 会把任务正在
	// 使用的连接（Modbus-RTU 的串口、A1E/MC 的持久连接）从协议单例中删除，
	// 导致任务后续读取永久报"设备未连接"。
	handle := protocol.DeviceHandle(device.ID)
	if proto.IsConnected(handle) {
		global.Logger.Debug("设备已连接（复用中），跳过连接测试", zap.Uint64("deviceID", device.ID))
		return true
	}

	// 未连接才建立临时连接做测试，测试完立即断开
	handle, err = proto.Connect(ctx, connParams)
	if err != nil {
		global.Logger.Debug("设备连接失败", zap.Uint64("deviceID", device.ID), zap.Error(err))
		return false
	}
	global.Logger.Debug("设备连接测试成功", zap.Uint64("deviceID", device.ID))

	// 立即关闭连接
	defer func() {
		_ = proto.Disconnect(context.Background(), handle)
	}()

	return true
}

// parseDeviceConfigToMetadata 解析设备配置为协议 Metadata
func parseDeviceConfigToMetadata(device *model.Device) (protocol.Metadata, error) {
	result := make(protocol.Metadata)

	if len(device.Config) == 0 {
		return result, nil
	}

	var configMap map[string]interface{}
	if err := json.Unmarshal(device.Config, &configMap); err != nil {
		return nil, err
	}

	for k, v := range configMap {
		if k == "pluginHost" || k == "pluginPort" {
			continue
		}
		result[k] = v
	}

	return result, nil
}

func mapStatusMessage(online bool) string {
	if online {
		return "connected"
	}
	return "disconnected"
}

// StartAllTasks 启动所有任务
func StartAllTasks() {
	scheduler := GetTaskScheduler()
	if scheduler == nil {
		global.Logger.Info("任务调度器未初始化，跳过任务启动")
		return
	}
	if err := scheduler.StartAllEnabledTasks(); err != nil {
		global.Logger.Error("启动任务失败", zap.Error(err))
	}
}

// GetUptime 获取系统运行时间（字符串格式）
func GetUptime() string {
	if runtime.GOOS == "linux" {
		return getLinuxUptime()
	}
	// Windows 系统返回默认值
	return "up 0 days, 8 hours, 1 minute"
}

// GetUptimeSeconds 获取系统运行时间（秒数）
// Windows 无法获取时返回固定值 29400（8小时10分）
func GetUptimeSeconds() int64 {
	if runtime.GOOS == "linux" {
		return getLinuxUptimeSeconds()
	}
	return 29400
}

func getLinuxUptimeSeconds() int64 {
	cmd := exec.Command("cat", "/proc/uptime")
	output, err := cmd.Output()
	if err != nil {
		global.Logger.Warn("读取 /proc/uptime 失败", zap.Error(err))
		return 29400
	}
	var uptimeSeconds float64
	_, err = fmt.Sscanf(string(output), "%f", &uptimeSeconds)
	if err != nil {
		global.Logger.Warn("解析 /proc/uptime 失败", zap.Error(err))
		return 29400
	}
	return int64(uptimeSeconds)
}

// getLinuxUptime 执行 uptime -p 获取运行时间
func getLinuxUptime() string {
	cmd := exec.Command("uptime", "-p")
	output, err := cmd.Output()
	if err != nil {
		global.Logger.Warn("执行 uptime 命令失败", zap.Error(err))
		return "up 0 days, 8 hours, 1 minute"
	}
	return fmt.Sprintf("up %s", string(output))
}

// GetSystemStatus 获取系统状态信息
func GetSystemStatus() map[string]interface{} {
	ip, mac := get2Ips()
	return map[string]interface{}{
		"uptime": GetUptime(),
		"os":     runtime.GOOS,
		"arch":   runtime.GOARCH,
		"sn":     global.CONFIG.Gateway.SN,
		"ip":     ip,
		"mac":    mac,
	}
}
