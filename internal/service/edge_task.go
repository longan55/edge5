package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"edge5/global"
	"edge5/internal/core/cache"
	"edge5/internal/core/edge"
	"edge5/internal/core/expr"
	"edge5/internal/core/protocol"
	"edge5/internal/model"
	"edge5/internal/repository"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// EdgeTopicKey 边缘计算结果上报的默认主题模板 key
const EdgeTopicKey = "edge_data_up"

// ---------------------------------------------------------------------------
// 调度器
// ---------------------------------------------------------------------------

type edgeTaskEntry struct {
	task   *model.EdgeTask
	device *model.Device
	engine *edge.Engine
	points []protocol.Point
	// inputExprs 点位池各变量已编译的采样值表达式（key=Var，x=原始值）
	inputExprs map[string]*expr.Program
	cancel     context.CancelFunc
	done       chan struct{}
	cache      []TaskDataRecord
	cacheMu    sync.Mutex
	// latestOutputs 最近一次 Evaluate 的全部输出（采样拍更新，上报拍消费）。
	// 仅 runTask 单 goroutine 访问，无需锁
	latestOutputs map[string]interface{}
	status        TaskStatus
	statusMu      sync.Mutex
}

// EdgeTaskScheduler 边缘计算任务调度器：周期采样输入点位 → 引擎计算 → 上报计算结果
type EdgeTaskScheduler struct {
	mu          sync.Mutex
	tasks       map[uint64]*edgeTaskEntry
	logger      *zap.Logger
	repo        *repository.EdgeTaskRepository
	deviceRepo  *repository.DeviceRepository
	statusRepo  *repository.DeviceStatusRepository
	counterRepo *repository.EdgeCounterRepository
}

var edgeScheduler *EdgeTaskScheduler

func NewEdgeTaskScheduler(repo *repository.EdgeTaskRepository, deviceRepo *repository.DeviceRepository, logger *zap.Logger) *EdgeTaskScheduler {
	if edgeScheduler == nil {
		edgeScheduler = &EdgeTaskScheduler{
			tasks:       make(map[uint64]*edgeTaskEntry),
			logger:      logger,
			repo:        repo,
			deviceRepo:  deviceRepo,
			statusRepo:  repository.NewDeviceStatusRepository(global.DB),
			counterRepo: repository.NewEdgeCounterRepository(global.DB, logger),
		}
		global.RegisterQuitTask(func() error {
			edgeScheduler.StopAll()
			return nil
		}, "停止所有边缘计算任务", 6)
	}
	return edgeScheduler
}

func GetEdgeTaskScheduler() *EdgeTaskScheduler {
	return edgeScheduler
}

func (s *EdgeTaskScheduler) StartTask(taskID uint64) error {
	s.mu.Lock()
	if _, ok := s.tasks[taskID]; ok {
		s.mu.Unlock()
		return fmt.Errorf("任务已在运行")
	}
	s.mu.Unlock()

	task, err := s.repo.GetByID(taskID)
	if err != nil {
		return fmt.Errorf("获取边缘计算任务失败: %w", err)
	}
	device, err := s.deviceRepo.GetByID(task.DeviceID)
	if err != nil {
		return fmt.Errorf("获取设备失败: %w", err)
	}

	// 启动即构建引擎，提前校验算子类型、表达式与变量引用
	eng, err := edge.NewEngine(toEngineRules(task.Rules), toEngineInputs(task.Inputs), s.logger)
	if err != nil {
		return fmt.Errorf("边缘计算规则无效: %w", err)
	}

	// 从 DB 加载当日已有计数，注入到引擎内存状态，避免重启后从 0 重新计数
	if s.counterRepo != nil {
		today := edge.DayKey(time.Now())
		counts, err := s.counterRepo.LoadTodayCounts(task.ID, today)
		if err != nil {
			s.logger.Warn("加载当日计数失败，从 0 重新计数",
				zap.Uint64("taskID", taskID), zap.Error(err))
		} else {
			for ruleName, count := range counts {
				eng.SetInitialCount(ruleName, count, today)
			}
			if len(counts) > 0 {
				s.logger.Info("已加载当日计数，从断点继续",
					zap.Uint64("taskID", taskID), zap.Any("counts", counts))
			}
		}
	}

	// 编译点位池采样表达式（x=原始值），启动即校验语法；兼容旧 Scale 倍率
	inputExprs := make(map[string]*expr.Program, len(task.Inputs))
	for _, in := range task.Inputs {
		prog, err := expr.CompileProgram(resolveInputExpr(in), map[string]bool{"x": true})
		if err != nil {
			return fmt.Errorf("点位 %s(%s) 计算表达式无效: %w", in.Var, in.Address, err)
		}
		inputExprs[in.Var] = prog
	}

	sps := eng.SamplePoints()
	points := make([]protocol.Point, 0, len(sps))
	for _, sp := range sps {
		points = append(points, protocol.Point{
			Name:     sp.Address,
			Resource: sp.Address,
			DataType: sp.ParseType,
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	s.mu.Lock()
	s.tasks[taskID] = &edgeTaskEntry{
		task:          task,
		device:        device,
		engine:        eng,
		points:        points,
		inputExprs:    inputExprs,
		cancel:        cancel,
		done:          done,
		cache:         make([]TaskDataRecord, 0, maxCacheSize),
		latestOutputs: make(map[string]interface{}),
		status:        TaskStatusDisconnected,
	}
	s.mu.Unlock()

	go s.runTask(ctx, taskID, task, device, eng, points, done)
	s.logger.Info("边缘计算任务已启动", zap.Uint64("taskID", taskID), zap.String("name", task.Name))
	return nil
}

func (s *EdgeTaskScheduler) StopTask(taskID uint64) error {
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
		s.logger.Warn("边缘计算任务停止超时，强制退出", zap.Uint64("taskID", taskID))
	}
	s.logger.Info("边缘计算任务已停止", zap.Uint64("taskID", taskID))
	return nil
}

func (s *EdgeTaskScheduler) IsRunning(taskID uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.tasks[taskID]
	return ok
}

func (s *EdgeTaskScheduler) GetTaskStatus(taskID uint64) TaskStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry, ok := s.tasks[taskID]; ok {
		entry.statusMu.Lock()
		defer entry.statusMu.Unlock()
		return entry.status
	}
	return TaskStatusStopped
}

func (s *EdgeTaskScheduler) GetTaskData(taskID uint64) []TaskDataRecord {
	s.mu.Lock()
	entry, ok := s.tasks[taskID]
	s.mu.Unlock()
	if !ok {
		return nil
	}
	entry.cacheMu.Lock()
	defer entry.cacheMu.Unlock()
	result := make([]TaskDataRecord, len(entry.cache))
	copy(result, entry.cache)
	return result
}

func (s *EdgeTaskScheduler) setStatus(taskID uint64, st TaskStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry, ok := s.tasks[taskID]; ok {
		entry.statusMu.Lock()
		entry.status = st
		entry.statusMu.Unlock()
	}
}

func (s *EdgeTaskScheduler) StopAll() {
	s.mu.Lock()
	ids := make([]uint64, 0, len(s.tasks))
	for id := range s.tasks {
		ids = append(ids, id)
	}
	s.mu.Unlock()
	for _, id := range ids {
		_ = s.StopTask(id)
	}
	s.logger.Info("所有边缘计算任务已停止")
}

func (s *EdgeTaskScheduler) StartAllEnabledTasks() error {
	tasks, err := s.repo.ListAll()
	if err != nil {
		return err
	}
	for _, t := range tasks {
		if t.State {
			if err := s.StartTask(t.ID); err != nil {
				s.logger.Warn("启动边缘计算任务失败", zap.Uint64("taskID", t.ID), zap.Error(err))
			}
		}
	}
	s.logger.Info("边缘计算任务启动完成", zap.Int("total", len(tasks)))
	return nil
}

func (s *EdgeTaskScheduler) runTask(ctx context.Context, taskID uint64, task *model.EdgeTask, device *model.Device,
	eng *edge.Engine, points []protocol.Point, done chan struct{}) {
	defer close(done)

	proto, ok := protocol.DefaultRegistry().Get(device.Protocol)
	if !ok {
		s.logger.Error("协议未找到", zap.Uint64("taskID", taskID), zap.String("protocol", device.Protocol))
		return
	}

	connectParams := protocol.Metadata{
		"deviceSn": device.DeviceSn,
		"deviceID": device.ID,
	}
	if len(device.Config) > 0 {
		var params map[string]interface{}
		if err := json.Unmarshal(device.Config, &params); err == nil {
			for k, v := range params {
				connectParams[k] = v
			}
		}
	}

	reconnectTicker := time.NewTicker(30 * time.Second)
	defer reconnectTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		handle, err := proto.Connect(ctx, connectParams)
		if err != nil {
			s.logger.Error("边缘计算设备连接失败，30秒后重试", zap.Uint64("taskID", taskID), zap.Error(err))
			s.setStatus(taskID, TaskStatusDisconnected)
			if s.statusRepo != nil {
				_ = s.statusRepo.UpsertByDeviceID(device.ID, false, time.Now(), "连接失败: "+err.Error())
			}
			select {
			case <-ctx.Done():
				return
			case <-reconnectTicker.C:
				continue
			}
		}

		s.setStatus(taskID, TaskStatusRunning)
		if s.statusRepo != nil {
			_ = s.statusRepo.UpsertByDeviceID(device.ID, true, time.Now(), "connected")
		}

		// 采样与上报解耦：采样快（高频捕获脉冲），上报慢（避免 MQTT 刷屏）
		sampleInterval := task.SampleInterval
		if sampleInterval <= 0 {
			sampleInterval = 1 // 默认 1s
		}
		reportInterval := task.ReportInterval
		if reportInterval <= 0 {
			reportInterval = 10 // 默认 10s
		}
		// 兜底：上报间隔不能小于采样间隔（否则上报比采样还频繁没意义）
		if reportInterval < sampleInterval {
			reportInterval = sampleInterval
		}

		// 取出 entry 用于 latestOutputs；runTask 单 goroutine 写，无需锁
		s.mu.Lock()
		entry := s.tasks[taskID]
		s.mu.Unlock()
		if entry == nil {
			return
		}

		func() {
			dctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			defer proto.Disconnect(dctx, handle)

			sampleTicker := time.NewTicker(time.Duration(sampleInterval) * time.Second)
			defer sampleTicker.Stop()
			reportTicker := time.NewTicker(time.Duration(reportInterval) * time.Second)
			defer reportTicker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-sampleTicker.C:
					if err := s.executeSample(ctx, task, device, proto, handle, eng, points, entry); err != nil {
						if errors.Is(err, protocol.ErrNotConnected) {
							s.logger.Warn("边缘计算设备连接已失效，跳出采样循环并重连",
								zap.Uint64("taskID", task.ID),
								zap.Uint64("deviceID", device.ID))
							return
						}
					}
				case <-reportTicker.C:
					s.flushReport(task, device, entry)
				}
			}
		}()

		s.setStatus(taskID, TaskStatusDisconnected)
		if s.statusRepo != nil {
			_ = s.statusRepo.UpsertByDeviceID(device.ID, false, time.Now(), "connection lost")
		}
		select {
		case <-ctx.Done():
			return
		case <-reconnectTicker.C:
		}
	}
}

// executeSample 单次采样：读设备 → 引擎 Evaluate → 持久化计数 delta → 更新 latestOutputs。
// 不直接 publish 到 MQTT，由 flushReport 在上报间隔到来时统一发送。
func (s *EdgeTaskScheduler) executeSample(ctx context.Context, task *model.EdgeTask, device *model.Device,
	proto protocol.DeviceCommProtocol, handle protocol.DeviceHandle, eng *edge.Engine, points []protocol.Point,
	entry *edgeTaskEntry) error {
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("边缘计算 executeSample panic", zap.Uint64("taskID", task.ID), zap.Any("panic", r))
		}
	}()

	if len(points) == 0 {
		return nil
	}

	resp, err := proto.ReadBatch(ctx, handle, protocol.BatchReadRequest{Points: points})
	if err != nil {
		s.logger.Warn("边缘计算采样读取失败", zap.Uint64("taskID", task.ID), zap.Error(err))
		return err
	}
	if resp == nil || len(resp.Results) == 0 {
		return nil
	}

	// 仅采集质量良好的数值点位（key=address）
	rawValues := make(map[string]float64)
	for _, r := range resp.Results {
		if r.Quality != "good" && r.Quality != "" {
			continue
		}
		if f, ok := edgeToFloat(r.Value); ok {
			rawValues[r.PointName] = f
		}
	}
	if len(rawValues) == 0 {
		return nil
	}

	// 应用点位计算表达式（x=原始值），输出 key=Var 名；表达式启动时已编译
	values := make(map[string]float64, len(task.Inputs))
	for _, in := range task.Inputs {
		raw, ok := rawValues[in.Address]
		if !ok {
			continue // 该点位本次未读到（质量异常或不在结果中），保持缺失以触发 ready=false
		}
		prog := entry.inputExprs[in.Var]
		if prog == nil {
			values[in.Var] = raw
			continue
		}
		v, evalErr := prog.Eval(map[string]float64{"x": raw})
		if evalErr != nil {
			// 除零等运行时错误：本拍跳过该点位（规则侧表现为变量缺失），不污染计数基线
			s.logger.Warn("边缘计算点位表达式求值失败，本拍跳过该点位",
				zap.Uint64("taskID", task.ID),
				zap.String("var", in.Var),
				zap.String("expr", resolveInputExpr(in)),
				zap.Float64("x", raw),
				zap.Error(evalErr))
			continue
		}
		s.logger.Debug("边缘计算点位表达式求值",
			zap.Uint64("taskID", task.ID),
			zap.String("var", in.Var),
			zap.Float64("raw", raw),
			zap.String("expr", resolveInputExpr(in)),
			zap.Float64("result", v))
		values[in.Var] = v
	}
	if len(values) == 0 {
		return nil
	}

	now := time.Now()
	result := eng.Evaluate(values, now)

	// 持久化计数 delta（即使本次没产生 outputs，也可能有计数变化）
	if s.counterRepo != nil && len(result.CounterDeltas) > 0 {
		today := edge.DayKey(now)
		for ruleName, delta := range result.CounterDeltas {
			if delta == 0 {
				continue
			}
			if err := s.counterRepo.UpsertIncrement(task.ID, ruleName, device.DeviceSn, today, delta); err != nil {
				s.logger.Error("计数持久化失败",
					zap.Uint64("taskID", task.ID),
					zap.String("ruleName", ruleName),
					zap.Int64("delta", delta),
					zap.Error(err))
			}
		}
	}

	// 更新 latestOutputs（即使是空也不清空，保留上次的计数输出）
	for k, v := range result.Outputs {
		entry.latestOutputs[k] = v
	}
	return nil
}

// flushReport 上报拍：把 latestOutputs 拷贝后 publish 到 MQTT。
// 不清空 latestOutputs，下次上报拍仍会发送当前最新值（计数不变时反复上报是预期的，
// 让消费端能看到当前计数；如需"仅变化才上报"可在算子层判断 emit）。
func (s *EdgeTaskScheduler) flushReport(task *model.EdgeTask, device *model.Device, entry *edgeTaskEntry) {
	if len(entry.latestOutputs) == 0 {
		return
	}
	outputs := make(map[string]interface{}, len(entry.latestOutputs))
	for k, v := range entry.latestOutputs {
		outputs[k] = v
	}
	s.reportData(task, device, outputs)
}

// edgeToFloat 把协议返回的标量值统一转为 float64（计数与算术均基于数值）
func edgeToFloat(v interface{}) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int8:
		return float64(x), true
	case int16:
		return float64(x), true
	case int32:
		return float64(x), true
	case int64:
		return float64(x), true
	case uint:
		return float64(x), true
	case uint8:
		return float64(x), true
	case uint16:
		return float64(x), true
	case uint32:
		return float64(x), true
	case uint64:
		return float64(x), true
	case bool:
		if x {
			return 1, true
		}
		return 0, true
	}
	return 0, false
}

// resolveInputExpr 归一化点位的采样值表达式：
//   - Expr 非空：去空格后直接使用
//   - Expr 为空且旧 Scale 有效（≠0、≠1）：回退为 x*Scale
//   - 否则：x（原值）
func resolveInputExpr(in model.EdgeInput) string {
	if e := strings.TrimSpace(in.Expr); e != "" {
		return e
	}
	if in.Scale != 0 && in.Scale != 1 {
		return "x*" + strconv.FormatFloat(in.Scale, 'f', -1, 64)
	}
	return "x"
}

// toEngineInputs model 点位池转为引擎点位池（表达式归一化，兼容旧 Scale）
func toEngineInputs(inputs model.EdgeInputs) []edge.Input {
	out := make([]edge.Input, 0, len(inputs))
	for _, in := range inputs {
		out = append(out, edge.Input{
			Var:       in.Var,
			Address:   in.Address,
			ParseType: in.ParseType,
			Expr:      resolveInputExpr(in),
		})
	}
	return out
}

// toEngineRules model 规则转为引擎规则（引用顶层点位池的 Var 名）
func toEngineRules(rules model.EdgeRules) []edge.Rule {
	out := make([]edge.Rule, 0, len(rules))
	for _, r := range rules {
		er := edge.Rule{
			Name:     r.Name,
			FuncType: r.FuncType,
			Params:   edge.Params(r.Params),
		}
		if er.Params == nil {
			er.Params = edge.Params{}
		}
		for _, vr := range r.Vars {
			evr := edge.VarRef{
				Var:          vr.Var,
				HasThreshold: vr.Threshold != nil,
			}
			if vr.Threshold != nil {
				evr.Threshold = *vr.Threshold
			}
			er.Vars = append(er.Vars, evr)
		}
		out = append(out, er)
	}
	return out
}

func (s *EdgeTaskScheduler) cacheData(taskID uint64, rec TaskDataRecord) {
	s.mu.Lock()
	entry, ok := s.tasks[taskID]
	s.mu.Unlock()
	if !ok {
		return
	}
	entry.cacheMu.Lock()
	defer entry.cacheMu.Unlock()
	entry.cache = append([]TaskDataRecord{rec}, entry.cache...)
	if len(entry.cache) > maxCacheSize {
		entry.cache = entry.cache[:maxCacheSize]
	}
}

func (s *EdgeTaskScheduler) reportData(task *model.EdgeTask, device *model.Device, data map[string]interface{}) {
	ts := time.Now()
	msg := GetMessageBuilder().BuildDeviceDataMessage(device.DeviceSn, device.DeviceType, device.Brand, device.DeviceName, task.ID, data)
	jsonData, err := json.Marshal(msg)
	if err != nil {
		s.logger.Error("边缘计算结果序列化失败", zap.Uint64("taskID", task.ID), zap.Error(err))
		return
	}

	topic := task.UpTopic
	if topic == "" {
		topic = GetMessageBuilder().BuildTopic(EdgeTopicKey, device.DeviceSn)
	}

	rec := TaskDataRecord{Timestamp: ts, Data: string(jsonData), UpState: false}

	if global.MQTTClient == nil || !global.MQTTClient.IsConnected() {
		s.persistOrDrop(topic, jsonData, ts, task.ID)
		s.cacheData(task.ID, rec)
		return
	}

	if err := global.MQTTClient.Publish(topic, byte(global.CONFIG.MQTT.QoS), jsonData); err != nil {
		s.logger.Error("边缘计算结果 MQTT 发布失败", zap.String("topic", topic), zap.Error(err))
		s.persistOrDrop(topic, jsonData, ts, task.ID)
		s.cacheData(task.ID, rec)
		return
	}
	rec.UpState = true
	s.cacheData(task.ID, rec)
}

func (s *EdgeTaskScheduler) persistOrDrop(topic string, payload []byte, ts time.Time, taskID uint64) {
	if global.CacheDB == nil {
		s.logger.Warn("MQTT 未连接且无持久化缓存，边缘计算结果丢弃", zap.Uint64("taskID", taskID))
		return
	}
	msg := &cache.CacheMessage{
		ID:        fmt.Sprintf("edge_task_%d_%d", taskID, ts.UnixMilli()),
		Topic:     topic,
		Payload:   payload,
		CreatedAt: ts.Unix(),
	}
	if err := global.CacheDB.Push(msg); err != nil {
		s.logger.Error("边缘计算结果持久化缓存失败", zap.Uint64("taskID", taskID), zap.Error(err))
	}
}

// FlushAllCache MQTT 重连后补报缓存（与普通任务共用 BoltDB 缓存，TaskScheduler 已统一刷新；
// 此处仅刷新边缘任务的内存缓存）
func (s *EdgeTaskScheduler) FlushAllCache() {
	if global.MQTTClient == nil || !global.MQTTClient.IsConnected() {
		return
	}
	s.mu.Lock()
	entries := make([]*edgeTaskEntry, 0, len(s.tasks))
	for _, e := range s.tasks {
		entries = append(entries, e)
	}
	s.mu.Unlock()
	for _, e := range entries {
		e.cacheMu.Lock()
		if len(e.cache) == 0 {
			e.cacheMu.Unlock()
			continue
		}
		records := make([]TaskDataRecord, len(e.cache))
		copy(records, e.cache)
		e.cache = e.cache[:0]
		e.cacheMu.Unlock()

		topic := e.task.UpTopic
		if topic == "" {
			topic = GetMessageBuilder().BuildTopic(EdgeTopicKey, e.device.DeviceSn)
		}
		for i := len(records) - 1; i >= 0; i-- {
			if global.MQTTClient == nil || !global.MQTTClient.IsConnected() {
				e.cacheMu.Lock()
				e.cache = append(e.cache, records[:i+1]...)
				e.cacheMu.Unlock()
				return
			}
			if err := global.MQTTClient.Publish(topic, byte(global.CONFIG.MQTT.QoS), []byte(records[i].Data)); err != nil {
				s.logger.Error("边缘计算缓存补报失败", zap.Uint64("taskID", e.task.ID), zap.Error(err))
				return
			}
		}
	}
}

// StartAllEdgeTasks 开机自启入口
func StartAllEdgeTasks() {
	sc := GetEdgeTaskScheduler()
	if sc == nil {
		global.Logger.Info("边缘计算调度器未初始化，跳过启动")
		return
	}
	if err := sc.StartAllEnabledTasks(); err != nil {
		global.Logger.Error("启动边缘计算任务失败", zap.Error(err))
	}
}

// ---------------------------------------------------------------------------
// CRUD 服务
// ---------------------------------------------------------------------------

type EdgeTaskService struct {
	repo       *repository.EdgeTaskRepository
	deviceRepo *repository.DeviceRepository
	logger     *zap.Logger
}

func NewEdgeTaskService(repo *repository.EdgeTaskRepository, deviceRepo *repository.DeviceRepository, logger *zap.Logger) *EdgeTaskService {
	return &EdgeTaskService{repo: repo, deviceRepo: deviceRepo, logger: logger}
}

func (s *EdgeTaskService) Create(t *model.EdgeTask) error {
	if t.UpTopic == "" {
		device, err := s.deviceRepo.GetByID(t.DeviceID)
		if err != nil {
			return fmt.Errorf("获取设备信息失败: %w", err)
		}
		t.UpTopic = GetMessageBuilder().BuildTopic(EdgeTopicKey, device.DeviceSn)
	}
	return s.repo.Create(t)
}

func (s *EdgeTaskService) Update(t *model.EdgeTask) error {
	old, err := s.repo.GetByID(t.ID)
	if err != nil && err != gorm.ErrRecordNotFound {
		return err
	}
	if err := s.repo.Update(t); err != nil {
		return err
	}
	if old != nil && old.State {
		needRestart := !t.State ||
			old.SampleInterval != t.SampleInterval ||
			old.ReportInterval != t.ReportInterval ||
			old.DeviceID != t.DeviceID || !reflect.DeepEqual(old.Rules, t.Rules)
		if sc := GetEdgeTaskScheduler(); sc != nil && needRestart {
			_ = sc.StopTask(t.ID)
			if t.State {
				_ = sc.StartTask(t.ID)
			}
		}
	}
	return nil
}

func (s *EdgeTaskService) Delete(id uint64) error {
	if sc := GetEdgeTaskScheduler(); sc != nil {
		_ = sc.StopTask(id)
	}
	return s.repo.Delete(id)
}

func (s *EdgeTaskService) DeleteBatch(ids []uint64) error {
	if sc := GetEdgeTaskScheduler(); sc != nil {
		for _, id := range ids {
			_ = sc.StopTask(id)
		}
	}
	return s.repo.DeleteBatch(ids)
}

func (s *EdgeTaskService) GetByID(id uint64) (*model.EdgeTask, error) {
	return s.repo.GetByID(id)
}

func (s *EdgeTaskService) List(page, pageSize int, name string) ([]model.EdgeTask, int64, error) {
	return s.repo.List(page, pageSize, name)
}

func (s *EdgeTaskService) StartTask(id uint64) error {
	t, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	t.State = true
	if err := s.repo.Update(t); err != nil {
		return err
	}
	if sc := GetEdgeTaskScheduler(); sc != nil {
		return sc.StartTask(id)
	}
	return nil
}

func (s *EdgeTaskService) StopTask(id uint64) error {
	t, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	t.State = false
	if err := s.repo.Update(t); err != nil {
		return err
	}
	if sc := GetEdgeTaskScheduler(); sc != nil {
		return sc.StopTask(id)
	}
	return nil
}

func (s *EdgeTaskService) GetTaskData(id uint64) ([]TaskDataRecord, error) {
	if _, err := s.repo.GetByID(id); err != nil {
		return nil, err
	}
	sc := GetEdgeTaskScheduler()
	if sc == nil {
		return nil, fmt.Errorf("边缘计算调度器未初始化")
	}
	return sc.GetTaskData(id), nil
}

// ListFunctions 返回算子元信息，供前端动态生成参数表单
func (s *EdgeTaskService) ListFunctions() []edge.FuncMeta {
	return edge.ListFuncs()
}

// AnalyzeRule 触发某任务下指定规则的在线分析（仅对实现了 Analyzable 的算子有效，
// 如 counter_ac 的滑动窗口峰谷学习）。任务必须在运行，因为窗口在算子内存中。
func (s *EdgeTaskService) AnalyzeRule(taskID uint64, ruleName string) (map[string]interface{}, error) {
	sc := GetEdgeTaskScheduler()
	if sc == nil {
		return nil, fmt.Errorf("边缘计算调度器未初始化")
	}
	sc.mu.Lock()
	entry, ok := sc.tasks[taskID]
	sc.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("任务未运行，无法分析（窗口在算子内存中，需先启动任务采集若干样本）")
	}
	info, ok := entry.engine.AnalyzeRule(ruleName)
	if !ok {
		return nil, fmt.Errorf("规则 %s 不存在或不支持在线分析", ruleName)
	}
	return info, nil
}
