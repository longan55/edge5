package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
	"unsafe"

	"edge5/global"
	"edge5/internal/model"
	"edge5/internal/core/cache"
	"edge5/internal/core/protocol"
	"edge5/internal/repository"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

const maxCacheSize = 10

type TaskDataRecord struct {
	Timestamp time.Time `json:"timestamp"`
	Data      string    `json:"data"`
	UpState   bool      `json:"upState"`
}

type TaskStatus string

const (
	TaskStatusStopped      TaskStatus = "stopped"
	TaskStatusRunning      TaskStatus = "running"
	TaskStatusDisconnected TaskStatus = "disconnected"
)

type taskEntry struct {
	task     *model.Task
	device   *model.Device
	cancel   context.CancelFunc
	done     chan struct{}
	cache    []TaskDataRecord
	cacheMu  sync.Mutex
	status   TaskStatus
	statusMu sync.Mutex
}

type TaskScheduler struct {
	mu               sync.Mutex
	tasks            map[uint64]*taskEntry
	logger           *zap.Logger
	repo             *repository.TaskRepository
	deviceRepo       *repository.DeviceRepository
	deviceStatusRepo *repository.DeviceStatusRepository
}

var taskScheduler *TaskScheduler

func NewTaskScheduler(repo *repository.TaskRepository, deviceRepo *repository.DeviceRepository, logger *zap.Logger) *TaskScheduler {
	if taskScheduler == nil {
		taskScheduler = &TaskScheduler{
			tasks:            make(map[uint64]*taskEntry),
			logger:           logger,
			repo:             repo,
			deviceRepo:       deviceRepo,
			deviceStatusRepo: repository.NewDeviceStatusRepository(global.DB),
		}
		global.RegisterQuitTask(func() error {
			taskScheduler.StopAll()
			return nil
		}, "停止所有采集任务", 5)
	}
	return taskScheduler
}

func GetTaskScheduler() *TaskScheduler {
	return taskScheduler
}

func (s *TaskScheduler) StartTask(taskID uint64) error {
	s.mu.Lock()
	if _, ok := s.tasks[taskID]; ok {
		s.mu.Unlock()
		return fmt.Errorf("任务已在运行")
	}
	s.mu.Unlock()

	task, err := s.repo.GetByID(taskID)
	if err != nil {
		return fmt.Errorf("获取任务失败: %w", err)
	}

	device, err := s.deviceRepo.GetByID(task.DeviceID)
	if err != nil {
		return fmt.Errorf("获取设备失败: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	s.mu.Lock()
	s.tasks[taskID] = &taskEntry{
		task:   task,
		device: device,
		cancel: cancel,
		done:   done,
		cache:  make([]TaskDataRecord, 0, maxCacheSize),
		status: TaskStatusDisconnected,
	}
	s.mu.Unlock()

	go s.runTask(ctx, taskID, task, device, done)

	s.logger.Info("任务已启动", zap.Uint64("taskID", taskID), zap.String("taskName", task.Name))
	return nil
}

func (s *TaskScheduler) StopTask(taskID uint64) error {
	s.mu.Lock()
	entry, ok := s.tasks[taskID]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("任务未运行")
	}
	delete(s.tasks, taskID)
	s.mu.Unlock()

	entry.cancel()

	select {
	case <-entry.done:
	case <-time.After(5 * time.Second):
		s.logger.Warn("任务停止超时，强制退出", zap.Uint64("taskID", taskID))
	}

	s.logger.Info("任务已停止", zap.Uint64("taskID", taskID), zap.String("taskName", entry.task.Name))
	return nil
}

func (s *TaskScheduler) IsRunning(taskID uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.tasks[taskID]
	return ok
}

func (s *TaskScheduler) GetTaskStatus(taskID uint64) TaskStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry, ok := s.tasks[taskID]; ok {
		entry.statusMu.Lock()
		defer entry.statusMu.Unlock()
		return entry.status
	}
	return TaskStatusStopped
}

func (s *TaskScheduler) GetTaskData(taskID uint64) []TaskDataRecord {
	s.mu.Lock()
	entry, ok := s.tasks[taskID]
	if !ok {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	entry.cacheMu.Lock()
	defer entry.cacheMu.Unlock()

	result := make([]TaskDataRecord, len(entry.cache))
	copy(result, entry.cache)
	return result
}

func (s *TaskScheduler) StopAll() {
	s.mu.Lock()
	taskIDs := make([]uint64, 0, len(s.tasks))
	for id := range s.tasks {
		taskIDs = append(taskIDs, id)
	}
	s.mu.Unlock()

	for _, id := range taskIDs {
		_ = s.StopTask(id)
	}
	s.logger.Info("所有任务已停止")
}

func (s *TaskScheduler) runTask(ctx context.Context, taskID uint64, task *model.Task, device *model.Device, done chan struct{}) {
	defer close(done)

	s.logger.Info("任务协程启动", zap.Uint64("taskID", taskID))

	proto, ok := protocol.DefaultRegistry().Get(device.Protocol)
	if !ok {
		s.logger.Error("协议未找到", zap.Uint64("taskID", taskID), zap.String("protocol", device.Protocol))
		return
	}

	connectParams := protocol.Metadata{
		"deviceSn": device.DeviceSn,
		"deviceID": device.ID,
	}

	if device.Config != nil {
		var params map[string]interface{}
		if err := json.Unmarshal(device.Config, &params); err == nil {
			for k, v := range params {
				connectParams[k] = v
			}
		}
	}

	points := make([]protocol.Point, 0, len(task.Commands))
	commandMap := make(map[string]model.TaskCommand)
	for _, cmd := range task.Commands {
		points = append(points, protocol.Point{
			Name:     cmd.Name,
			Resource: cmd.Address,
			DataType: cmd.ParseType,
		})
		commandMap[cmd.Address] = cmd
	}

	reconnectTicker := time.NewTicker(30 * time.Second)
	defer reconnectTicker.Stop()

	for {
		// 检查是否已取消
		select {
		case <-ctx.Done():
			s.logger.Info("任务协程退出", zap.Uint64("taskID", taskID))
			return
		default:
		}

		handle, err := proto.Connect(ctx, connectParams)
		if err != nil {
			s.logger.Error("协议连接设备失败，30秒后重试", zap.Uint64("taskID", taskID), zap.String("protocol", device.Protocol), zap.Error(err))
			s.setTaskStatus(taskID, TaskStatusDisconnected)
			s.updateDeviceStatus(device.ID, false, "连接失败: "+err.Error())
			// 等待重连或取消
			select {
			case <-ctx.Done():
				s.logger.Info("任务协程退出（重连等待中取消）", zap.Uint64("taskID", taskID))
				return
			case <-reconnectTicker.C:
				continue
			}
		}

		// 连接成功，更新状态为运行中
		s.setTaskStatus(taskID, TaskStatusRunning)
		s.updateDeviceStatus(device.ID, true, "connected")
		s.logger.Info("设备连接成功，开始采集", zap.Uint64("taskID", taskID))

		func() {
			disconnectCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			defer proto.Disconnect(disconnectCtx, handle)

			ticker := time.NewTicker(time.Duration(task.ReadInterval) * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					s.logger.Info("任务协程退出", zap.Uint64("taskID", taskID))
					return
				case <-ticker.C:
					s.executeRead(ctx, taskID, task, device, proto, handle, points, commandMap)
				}
			}
		}()

		// 连接断开，设置状态为未连接，等待重连
		s.setTaskStatus(taskID, TaskStatusDisconnected)
		s.updateDeviceStatus(device.ID, false, "connection lost")
		s.logger.Warn("设备连接断开，30秒后重试", zap.Uint64("taskID", taskID))

		select {
		case <-ctx.Done():
			s.logger.Info("任务协程退出", zap.Uint64("taskID", taskID))
			return
		case <-reconnectTicker.C:
		}
	}
}

func (s *TaskScheduler) setTaskStatus(taskID uint64, status TaskStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry, ok := s.tasks[taskID]; ok {
		entry.statusMu.Lock()
		entry.status = status
		entry.statusMu.Unlock()
	}
}

func (s *TaskScheduler) updateDeviceStatus(deviceID uint64, online bool, message string) {
	if s.deviceStatusRepo != nil {
		_ = s.deviceStatusRepo.UpsertByDeviceID(deviceID, online, time.Now(), message)
	}
}

func (s *TaskScheduler) executeRead(ctx context.Context, taskID uint64, task *model.Task, device *model.Device, proto protocol.DeviceCommProtocol, handle protocol.DeviceHandle, points []protocol.Point, commandMap map[string]model.TaskCommand) {
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("executeRead 发生 panic", zap.Uint64("taskID", taskID), zap.Any("panic", r))
		}
	}()

	resp, err := proto.ReadBatch(ctx, handle, protocol.BatchReadRequest{
		Points: points,
	})
	if err != nil {
		s.logger.Warn("读取数据失败", zap.Uint64("taskID", taskID), zap.Error(err))
		return
	}
	if len(resp.Results) == 0 {
		return
	}

	dataMap := make(map[string]interface{})
	for _, result := range resp.Results {
		if cmd, ok := commandMap[result.PointName]; ok {
			if result.Quality == "good" || result.Quality == "" {
				dataMap[cmd.Name] = result.Value
			} else {
				s.logger.Warn("数据质量异常", zap.Uint64("taskID", taskID), zap.String("point", result.PointName), zap.String("quality", result.Quality))
			}
		}
	}

	if len(dataMap) == 0 {
		return
	}

	s.reportData(task, device, dataMap)
}

func parseValue(raw []byte, parseType string) interface{} {
	switch parseType {
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
			return float64(mustParseFloat32(bits))
		}
	case "double":
		if len(raw) >= 8 {
			bits := uint64(raw[0]) | uint64(raw[1])<<8 | uint64(raw[2])<<16 | uint64(raw[3])<<24 |
				uint64(raw[4])<<32 | uint64(raw[5])<<40 | uint64(raw[6])<<48 | uint64(raw[7])<<56
			return float64(mustParseFloat64(bits))
		}
	case "string":
		return string(raw)
	default:
		return string(raw)
	}
	return string(raw)
}

func mustParseFloat32(bits uint32) float32 {
	return *(*float32)(unsafe.Pointer(&bits))
}

func mustParseFloat64(bits uint64) float64 {
	return *(*float64)(unsafe.Pointer(&bits))
}

func (s *TaskScheduler) cacheData(taskID uint64, record TaskDataRecord) {
	s.mu.Lock()
	entry, ok := s.tasks[taskID]
	if !ok {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	entry.cacheMu.Lock()
	defer entry.cacheMu.Unlock()

	entry.cache = append([]TaskDataRecord{record}, entry.cache...)
	if len(entry.cache) > maxCacheSize {
		entry.cache = entry.cache[:maxCacheSize]
	}
}

func (s *TaskScheduler) reportData(task *model.Task, device *model.Device, data map[string]interface{}) error {
	ts := time.Now()

	// 使用 MessageBuilder 构建设备数据上报消息（payload 直接平铺业务数据）
	gatewayMsg := GetMessageBuilder().BuildDeviceDataMessage(device.DeviceSn, device.DeviceType, device.Brand, device.DeviceName, task.ID, data)
	jsonData, err := json.Marshal(gatewayMsg)
	if err != nil {
		s.logger.Error("序列化上报数据失败", zap.Uint64("taskID", task.ID), zap.Error(err))
		return err
	}

	topic := task.UpTopic
	if topic == "" {
		topic = GetMessageBuilder().BuildTopic("device_data_up", device.DeviceSn)
	}

	// 构建完整的记录用于缓存
	record := TaskDataRecord{
		Timestamp: ts,
		Data:      string(jsonData),
		UpState:   false,
	}

	if global.MQTTClient == nil || !global.MQTTClient.IsConnected() {
		if global.CacheDB != nil {
			msg := &cache.CacheMessage{
				ID:        fmt.Sprintf("task_%d_%d", task.ID, ts.UnixMilli()),
				Topic:     topic,
				Payload:   jsonData,
				CreatedAt: ts.Unix(),
			}
			if err := global.CacheDB.Push(msg); err != nil {
				s.logger.Error("缓存数据到BoltDB失败", zap.Uint64("taskID", task.ID), zap.Error(err))
				s.cacheData(task.ID, record)
				return err
			}
			s.logger.Info("MQTT未连接，数据已持久化缓存", zap.Uint64("taskID", task.ID))
		} else {
			s.logger.Warn("MQTT未连接且CacheDB未初始化，数据丢弃", zap.Uint64("taskID", task.ID))
			s.cacheData(task.ID, record)
			return fmt.Errorf("MQTT未连接且CacheDB未初始化，数据已内存缓存")
		}
		s.cacheData(task.ID, record)
		return fmt.Errorf("MQTT未连接，数据已缓存")
	}

	err = global.MQTTClient.Publish(topic, byte(global.CONFIG.MQTT.QoS), jsonData)
	if err != nil {
		s.logger.Error("MQTT发布失败", zap.Uint64("taskID", task.ID), zap.String("topic", topic), zap.Error(err))
		if global.CacheDB != nil {
			msg := &cache.CacheMessage{
				ID:        fmt.Sprintf("task_%d_%d", task.ID, ts.UnixMilli()),
				Topic:     topic,
				Payload:   jsonData,
				CreatedAt: ts.Unix(),
			}
			if err := global.CacheDB.Push(msg); err != nil {
				s.logger.Error("缓存数据到BoltDB失败", zap.Uint64("taskID", task.ID), zap.Error(err))
			}
		}
		s.cacheData(task.ID, record)
		return err
	}
	s.logger.Debug("MQTT发布成功", zap.Uint64("taskID", task.ID), zap.String("topic", topic))

	record.UpState = true
	s.cacheData(task.ID, record)
	return nil
}

// FlushAllCache MQTT重连后上报所有缓存数据（内存缓存 + BoltDB 持久化缓存）
func (s *TaskScheduler) FlushAllCache() {
	if global.MQTTClient == nil || !global.MQTTClient.IsConnected() {
		return
	}

	// 1. 刷新内存缓存
	s.mu.Lock()
	entries := make([]*taskEntry, 0, len(s.tasks))
	for _, entry := range s.tasks {
		entries = append(entries, entry)
	}
	s.mu.Unlock()

	for _, entry := range entries {
		s.flushTaskCache(entry)
	}

	// 2. 刷新 BoltDB 持久化缓存
	if global.CacheDB != nil {
		s.flushBoltCache()
	}
}

func (s *TaskScheduler) flushBoltCache() {
	messages, err := global.CacheDB.GetAll()
	if err != nil {
		s.logger.Error("读取BoltDB缓存失败", zap.Error(err))
		return
	}

	if len(messages) == 0 {
		return
	}

	s.logger.Info("开始上报BoltDB缓存数据", zap.Int("count", len(messages)))
	successCount := 0
	for _, msg := range messages {
		if global.MQTTClient == nil || !global.MQTTClient.IsConnected() {
			s.logger.Warn("MQTT连接中断，BoltDB缓存上报暂停", zap.Int("remaining", len(messages)-successCount))
			return
		}

		err := global.MQTTClient.Publish(msg.Topic, byte(global.CONFIG.MQTT.QoS), msg.Payload)
		if err != nil {
			s.logger.Error("BoltDB缓存消息发布失败", zap.String("id", msg.ID), zap.Error(err))
			continue
		}

		if err := global.CacheDB.Delete(msg.ID); err != nil {
			s.logger.Error("删除BoltDB缓存消息失败", zap.String("id", msg.ID), zap.Error(err))
		}
		successCount++
	}

	s.logger.Info("BoltDB缓存数据上报完成", zap.Int("count", successCount))
}

func (s *TaskScheduler) flushTaskCache(entry *taskEntry) {
	entry.cacheMu.Lock()
	if len(entry.cache) == 0 {
		entry.cacheMu.Unlock()
		return
	}

	// 按时间从旧到新上报
	records := make([]TaskDataRecord, len(entry.cache))
	copy(records, entry.cache)
	entry.cache = entry.cache[:0]
	entry.cacheMu.Unlock()

	for i := len(records) - 1; i >= 0; i-- {
		if global.MQTTClient == nil || !global.MQTTClient.IsConnected() {
			// MQTT又断开，将未上报的数据放回缓存
			entry.cacheMu.Lock()
			entry.cache = append(entry.cache, records[:i+1]...)
			entry.cacheMu.Unlock()
			s.logger.Warn("MQTT连接中断，缓存数据上报暂停", zap.Uint64("taskID", entry.task.ID))
			return
		}
		// 直接发布缓存的 JSON 字符串
		topic := entry.task.UpTopic
		if topic == "" {
			topic = GetMessageBuilder().BuildTopic("device_data_up", entry.device.DeviceSn)
		}
		err := global.MQTTClient.Publish(topic, byte(global.CONFIG.MQTT.QoS), []byte(records[i].Data))
		if err != nil {
			s.logger.Error("缓存数据发布失败", zap.Uint64("taskID", entry.task.ID), zap.Error(err))
			// 发布失败，将未上报的数据放回缓存
			entry.cacheMu.Lock()
			entry.cache = append(entry.cache, records[:i+1]...)
			entry.cacheMu.Unlock()
			return
		}
	}

	s.logger.Info("缓存数据上报完成", zap.Uint64("taskID", entry.task.ID), zap.Int("count", len(records)))
}

func (s *TaskScheduler) StartAllEnabledTasks() error {
	tasks, _, err := s.repo.List(1, 1000, "")
	if err != nil {
		return fmt.Errorf("获取任务列表失败: %w", err)
	}

	for _, task := range tasks {
		if task.State {
			if err := s.StartTask(task.ID); err != nil {
				s.logger.Warn("启动任务失败", zap.Uint64("taskID", task.ID), zap.Error(err))
			}
		}
	}

	s.logger.Info("任务启动完成", zap.Int("total", len(tasks)))
	return nil
}

type TaskService struct {
	repo       *repository.TaskRepository
	deviceRepo *repository.DeviceRepository
	logger     *zap.Logger
}

func NewTaskService(repo *repository.TaskRepository, deviceRepo *repository.DeviceRepository, logger *zap.Logger) *TaskService {
	return &TaskService{repo: repo, deviceRepo: deviceRepo, logger: logger}
}

func (s *TaskService) Create(task *model.Task) error {
	if task.UpTopic == "" {
		device, err := s.deviceRepo.GetByID(task.DeviceID)
		if err != nil {
			return fmt.Errorf("获取设备信息失败: %w", err)
		}
		task.UpTopic = GetMessageBuilder().BuildTopic("device_data_up", device.DeviceSn)
	}
	return s.repo.Create(task)
}

func (s *TaskService) Update(task *model.Task) error {
	oldTask, err := s.repo.GetByID(task.ID)
	if err != nil && err != gorm.ErrRecordNotFound {
		return err
	}

	err = s.repo.Update(task)
	if err != nil {
		return err
	}

	if oldTask != nil && oldTask.State && (!task.State || oldTask.ReadInterval != task.ReadInterval || oldTask.Commands != nil) {
		if scheduler := GetTaskScheduler(); scheduler != nil {
			_ = scheduler.StopTask(task.ID)
			if task.State {
				_ = scheduler.StartTask(task.ID)
			}
		}
	}

	return nil
}

func (s *TaskService) Delete(id uint64) error {
	if scheduler := GetTaskScheduler(); scheduler != nil {
		_ = scheduler.StopTask(id)
	}
	return s.repo.Delete(id)
}

func (s *TaskService) DeleteBatch(ids []uint64) error {
	if scheduler := GetTaskScheduler(); scheduler != nil {
		for _, id := range ids {
			_ = scheduler.StopTask(id)
		}
	}
	return s.repo.DeleteBatch(ids)
}

func (s *TaskService) GetByID(id uint64) (*model.Task, error) {
	return s.repo.GetByID(id)
}

func (s *TaskService) List(page, pageSize int, name string) ([]model.Task, int64, error) {
	tasks, total, err := s.repo.List(page, pageSize, name)
	if err != nil {
		return nil, 0, err
	}

	scheduler := GetTaskScheduler()
	if scheduler != nil {
		for i := range tasks {
			status := scheduler.GetTaskStatus(tasks[i].ID)
			if status == TaskStatusRunning {
				tasks[i].State = true
			} else if status == TaskStatusDisconnected {
				tasks[i].State = true
			} else {
				tasks[i].State = false
			}
		}
	}

	return tasks, total, nil
}

func (s *TaskService) StartTask(id uint64) error {
	task, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}

	task.State = true
	if err := s.repo.Update(task); err != nil {
		return err
	}

	if scheduler := GetTaskScheduler(); scheduler != nil {
		return scheduler.StartTask(id)
	}
	return nil
}

func (s *TaskService) StopTask(id uint64) error {
	task, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}

	task.State = false
	if err := s.repo.Update(task); err != nil {
		return err
	}

	if scheduler := GetTaskScheduler(); scheduler != nil {
		return scheduler.StopTask(id)
	}
	return nil
}

func (s *TaskService) GetTaskData(id uint64) ([]TaskDataRecord, error) {
	_, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}

	scheduler := GetTaskScheduler()
	if scheduler == nil {
		return nil, fmt.Errorf("任务调度器未初始化")
	}

	return scheduler.GetTaskData(id), nil
}
