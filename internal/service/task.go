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
	"edge5/internal/pkg/protocol"
	"edge5/internal/repository"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

const maxCacheSize = 10

type TaskDataRecord struct {
	Timestamp time.Time              `json:"timestamp"`
	Data      map[string]interface{} `json:"data"`
}

type taskEntry struct {
	task    *model.Task
	device  *model.Device
	cancel  context.CancelFunc
	done    chan struct{}
	cache   []TaskDataRecord
	cacheMu sync.Mutex
}

type TaskScheduler struct {
	mu         sync.Mutex
	tasks      map[uint64]*taskEntry
	logger     *zap.Logger
	repo       *repository.TaskRepository
	deviceRepo *repository.DeviceRepository
}

var taskScheduler *TaskScheduler

func NewTaskScheduler(repo *repository.TaskRepository, deviceRepo *repository.DeviceRepository, logger *zap.Logger) *TaskScheduler {
	if taskScheduler == nil {
		taskScheduler = &TaskScheduler{
			tasks:      make(map[uint64]*taskEntry),
			logger:     logger,
			repo:       repo,
			deviceRepo: deviceRepo,
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

	if !task.State {
		return fmt.Errorf("任务状态为停止，请先启用任务")
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
	<-entry.done

	s.logger.Info("任务已停止", zap.Uint64("taskID", taskID), zap.String("taskName", entry.task.Name))
	return nil
}

func (s *TaskScheduler) IsRunning(taskID uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.tasks[taskID]
	return ok
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

	handle, err := proto.Connect(ctx, connectParams)
	if err != nil {
		s.logger.Error("协议连接设备失败", zap.Uint64("taskID", taskID), zap.String("protocol", device.Protocol), zap.Error(err))
		return
	}

	defer func() {
		disconnectCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = proto.Disconnect(disconnectCtx, handle)
	}()

	ticker := time.NewTicker(time.Duration(task.ReadInterval) * time.Second)
	defer ticker.Stop()

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

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("任务协程退出", zap.Uint64("taskID", taskID))
			return
		case <-ticker.C:
			s.executeRead(ctx, taskID, task, device, proto, handle, points, commandMap)
		}
	}
}

func (s *TaskScheduler) executeRead(ctx context.Context, taskID uint64, task *model.Task, device *model.Device, proto protocol.DeviceCommProtocol, handle protocol.DeviceHandle, points []protocol.Point, commandMap map[string]model.TaskCommand) {
	resp, err := proto.ReadBatch(ctx, handle, protocol.BatchReadRequest{
		Points: points,
	})
	if err != nil {
		s.logger.Warn("读取数据失败", zap.Uint64("taskID", taskID), zap.Error(err))
		return
	}
	fmt.Printf("commandMap: %v\n", commandMap)
	fmt.Printf("resp: %v\n", resp)
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

	record := TaskDataRecord{
		Timestamp: time.Now(),
		Data:      dataMap,
	}

	s.cacheData(taskID, record)
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

func (s *TaskScheduler) reportData(task *model.Task, device *model.Device, data map[string]interface{}) {
	if global.MQTTClient == nil || !global.MQTTClient.IsConnected() {
		s.logger.Warn("MQTT未连接，数据已缓存", zap.Uint64("taskID", task.ID))
		return
	}

	payload := map[string]interface{}{
		"device_sn": device.DeviceSn,
		"task_id":   task.ID,
		"task_name": task.Name,
		"timestamp": time.Now().UnixMilli(),
		"data":      data,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		s.logger.Error("序列化上报数据失败", zap.Uint64("taskID", task.ID), zap.Error(err))
		return
	}
	s.logger.Debug("准备上报数据", zap.Any("data", jsonData))
	topic := task.UpTopic
	if topic == "" {
		topic = fmt.Sprintf("%s/data", device.DeviceSn)
	}

	err = global.MQTTClient.Publish(topic, byte(global.CONFIG.MQTT.QoS), jsonData)
	if err != nil {
		s.logger.Error("MQTT发布失败", zap.Uint64("taskID", task.ID), zap.String("topic", topic), zap.Error(err))
	}
	s.logger.Debug("MQTT发布成功", zap.Uint64("taskID", task.ID), zap.String("topic", topic))
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
	repo   *repository.TaskRepository
	logger *zap.Logger
}

func NewTaskService(repo *repository.TaskRepository, logger *zap.Logger) *TaskService {
	return &TaskService{repo: repo, logger: logger}
}

func (s *TaskService) Create(task *model.Task) error {
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
			tasks[i].State = scheduler.IsRunning(tasks[i].ID)
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
