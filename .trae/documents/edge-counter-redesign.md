# 边缘计算计数器重构：采样/上报频率分离 + 持久化 + 调试日志

## Context

当前边缘计算计数器存在三个问题，用户在试用后反馈：

1. **频率耦合**：`EdgeTask.ReadInterval` 单字段同时承担"读设备采样"和"MQTT 上报"两个职责。计数依赖边沿检测，必须高频采样（默认 10s 会让 0-峰-0 的脉冲在两次采样之间漏掉）；但每次采样立刻 publish 会刷屏。
2. **计数易丢失**：计数只存内存，进程重启即清零。用户希望"某天某设备计数了多少次"应该持久化（但只需通过 MQTT 上报，不需要单独的查询 API）。
3. **上报始终为 0**：很可能是采样太慢漏脉冲、或阈值默认 1 与实际峰值不匹配、或多点位规则里某输入未读到导致 `ready=false` 整条规则被跳过。需要 DEBUG 日志直接定位。

目标：拆分采样/上报频率；新增计数日表持久化；加 DEBUG 日志；保留 00:00 跨日清零与开机自启恢复机制。

## 关键设计决策

- **频率字段**：新增 `SampleInterval`（默认 1s）与 `ReportInterval`（默认 10s）两个字段；保留 `ReadInterval` 字段但标记废弃（避免破坏性 schema 变更）。代码使用新字段，0 时回退到默认值。
- **持久化时机**：每次计数变化（Evaluate 后 delta != 0）立即增量 upsert 当日记录。当日记录由 `(task_id, rule_name, date)` 唯一索引保证。
- **状态恢复**：启动时从 DB 加载当日计数注入到内存状态；`lastHigh` 不持久化（重启后首次采样仅建立基线，可能漏掉一次边沿但不会重复计数）。
- **跨日清零**：算子内部 `rollDay` 已处理；引擎层通过 `lastCountDay` 跟踪，跨日时不计算 delta（避免昨日 50 → 今日 0 产生 -49 的负 delta）。
- **同 goroutine 双 ticker**：采样与上报在同一个 `select` 中。代价是慢 publish 会拖慢下一拍采样；好处是简单、无需队列。先保留，若现场出现严重延迟再拆分。
- **持久化原子性**：使用 GORM `clause.OnConflict` + `count = count + ?` 表达式，SQLite/MySQL 均原生支持，避免 read-then-write 竞态。
- **日志注入**：新增 `LoggerAware` 接口，`NewEngine` 创建 state 后调用 `SetLogger`。不污染 `ComputeFunc.NewState` 签名。
- **不需要新增 API**：用户明确不需要历史查询接口；持久化纯粹为了"重启不丢、MQTT 持续上报"。

## 实施步骤

### 1. Model 层

**`internal/model/edge_task.go`** 修改 `EdgeTask`：
- 新增 `SampleInterval int gorm:"default:0" json:"sampleInterval"`（注释：采样间隔秒，0 表示用代码默认 1s）
- 新增 `ReportInterval int gorm:"default:0" json:"reportInterval"`（注释：上报间隔秒，0 表示用代码默认 10s）
- 保留 `ReadInterval` 字段，加注释 `// Deprecated: 改用 SampleInterval`

**新增 `internal/model/edge_counter_daily.go`**：
```go
type EdgeCounterDaily struct {
    BaseModel
    TaskID   uint64 `gorm:"not null;uniqueIndex:idx_edge_counter_daily,priority:1" json:"taskId"`
    RuleName string `gorm:"size:128;not null;uniqueIndex:idx_edge_counter_daily,priority:2" json:"ruleName"`
    DeviceSn string `gorm:"size:64;not null" json:"deviceSn"`
    Date     string `gorm:"size:10;not null;uniqueIndex:idx_edge_counter_daily,priority:3" json:"date"` // YYYY-MM-DD
    Count    int64  `gorm:"not null;default:0" json:"count"`
}
func (EdgeCounterDaily) TableName() string { return "edge_counter_daily" }
```

### 2. Repository 层

**`internal/repository/edge_task.go`** 修改 `AutoMigrate`：
- 调用 `db.AutoMigrate(&model.EdgeTask{}, &model.EdgeCounterDaily{})`
- 之后执行迁移 SQL：
  ```sql
  UPDATE edge_tasks SET sample_interval = read_interval WHERE sample_interval = 0;
  UPDATE edge_tasks SET report_interval = 10 WHERE report_interval = 0 AND sample_interval > 0;
  ```
  （第二句把"从老库迁移过来、未设上报间隔"的行默认设为 10s，匹配旧行为）

**新增 `internal/repository/edge_counter.go`** `EdgeCounterRepository`：
- `UpsertIncrement(taskID uint64, ruleName, deviceSn, date string, delta int64) error` — 使用 `clause.OnConflict{Columns: task_id/rule_name/date, DoUpdates: clause.Assignments(map{"count": gorm.Expr("count + ?", delta)})}.Create(&EdgeCounterDaily{...})`
- `LoadTodayCounts(taskID uint64, date string) (map[string]int64, error)` — 查询当日所有规则计数

### 3. Engine 层

**`internal/core/edge/edge.go`**：
- 新增接口：
  ```go
  type CounterState interface {
      State
      Count() int64
      SetInitialCount(count int64)
  }
  type LoggerAware interface { SetLogger(*zap.Logger) }
  ```

**`internal/core/edge/engine.go`**：
- `ruleRuntime` 增加字段：`lastCount *int64`、`lastCountDay string`
- `NewEngine`：创建 state 后调用 `if la, ok := st.(LoggerAware); ok && logger != nil { la.SetLogger(logger) }`
- `Evaluate` 返回值改为 `EvalResult struct { Outputs map[string]interface{}; CounterDeltas map[string]int64 }`：
  ```go
  for _, rt := range e.runtimes {
      // ... ready 检查，不 ready 则 continue（不动 lastCount）
      val, emit, err := rt.state.Evaluate(in, day)
      // ... err 处理
      if emit { out[rt.rule.Name] = val }
      // 计数 delta
      if cs, ok := rt.state.(CounterState); ok {
          cur := cs.Count()
          // 跨日：重置基线，本拍不计算 delta
          if rt.lastCountDay != day {
              rt.lastCountDay = day
              rt.lastCount = nil  // 等下文统一设值
          }
          if rt.lastCount != nil {
              delta := cur - *rt.lastCount
              if delta != 0 {
                  counterDeltas[rt.rule.Name] = delta
              }
          }
          c := cur
          rt.lastCount = &c
      }
  }
  ```
- 新增 `SetInitialCount(ruleName string, count int64)`：找到匹配 rule，调用 `CounterState.SetInitialCount`，并设置 `rt.lastCount = &count`、`rt.lastCountDay = today`，使首次 Evaluate 不产生 spurious delta
- `State.SetInitialCount(count)` 内部：`s.count = count; s.lastHigh = nil`（重置基线，避免首拍误判）

**`internal/core/edge/func_counter.go`**：
- `pulseState` 与 `multiCounterState` 增加 `logger *zap.Logger` 字段，实现 `LoggerAware`、`CounterState` 接口
- 在 `Evaluate` 中加入 DEBUG 日志（在 `e.logger.Desugar()` 不存在时跳过）：
  - pulseState：var、value、threshold、isHigh、prevHigh、curHigh、edge(rising/falling/none/baseline-init)、triggered bool、count、day
  - multiCounterState：每个 input 的 var/value/threshold/high、highCount、logic、combo、prevCombo→curCombo、triggered bool、count、day

### 4. Service 层

**`internal/service/edge_task.go`**：
- `EdgeTaskScheduler` 增加 `counterRepo *repository.EdgeCounterRepository` 字段
- `NewEdgeTaskScheduler` 初始化 counterRepo（沿用 `global.DB`）
- `edgeTaskEntry` 增加字段 `latestOutputs map[string]interface{}`（无锁，runTask 单 goroutine 访问）
- `StartTask`：在 `NewEngine` 后调用 `counterRepo.LoadTodayCounts(task.ID, edge.DayKey(time.Now()))`，对每条规则 `eng.SetInitialCount(ruleName, count)`，失败仅 warn 不阻塞启动
- `runTask`：替换单 ticker 为 `sampleTicker`（取 `SampleInterval`，0 时用 1s）+ `reportTicker`（取 `ReportInterval`，0 时用 10s）：
  ```go
  for {
      select {
      case <-ctx.Done(): return
      case <-sampleTicker.C: s.executeSample(...)
      case <-reportTicker.C: s.flushReport(task, device, entry)
      }
  }
  ```
- `executeSample` = 旧 `executeRead` 去掉 `s.reportData`，改为：
  1. ReadBatch
  2. 构造 values
  3. `result := eng.Evaluate(values, time.Now())`
  4. 对 `result.CounterDeltas` 逐条 `counterRepo.UpsertIncrement(task.ID, ruleName, device.DeviceSn, edge.DayKey(time.Now()), delta)`，失败仅 warn
  5. `for k, v := range result.Outputs { entry.latestOutputs[k] = v }`
- `flushReport`：`if len(entry.latestOutputs) == 0 { return }; outputs := 拷贝; s.reportData(task, device, outputs)`
- `reportData` 保持原逻辑（MQTT publish + cache 失败兜底）
- `EdgeTaskService.Update` 的 `needRestart` 判断：把 `old.ReadInterval != t.ReadInterval` 改为 `old.SampleInterval != t.SampleInterval || old.ReportInterval != t.ReportInterval`
- 默认主题生成中 `EdgeTopicKey` 不变

### 5. Handler 层

`internal/handler/edge_task_handler.go`：Create/Update 请求体的 `ReadInterval` 字段被 `SampleInterval` + `ReportInterval` 替代。Bind 时新字段自动接收；老字段保留兼容（不报错）。无新增接口。

### 6. 前端

`web/src/views/edge/EdgeTask.vue`：
- 表格列：将 `采样间隔: {{ row.readInterval }}s` 改为两列 `采样: {{ row.sampleInterval || 1 }}s` 与 `上报: {{ row.reportInterval || 10 }}s`
- 弹窗表单：把 `采样间隔(秒)` 单个 `el-input-number` 替换为两个：
  - 采样间隔(秒)：`v-model="formData.sampleInterval"` `:min="1" :max="60"` 默认 1
  - 上报间隔(秒)：`v-model="formData.reportInterval"` `:min="1" :max="3600"` 默认 10
- `emptyForm()`：去掉 `readInterval: 10`，新增 `sampleInterval: 1, reportInterval: 10`
- `handleEdit` 数据映射：去掉 `readInterval: t.readInterval`，新增 `sampleInterval: t.sampleInterval || t.readInterval || 1, reportInterval: t.reportInterval || 10`（兼容老数据回填）

## 验证方案

### 单元/集成
1. 编译：`go build ./...`
2. 前端构建：`cd web && npm run build`
3. 算子语义自测：手写一个 pulseState，模拟 0→1→0→1→0 序列，验证 `Count()` 返回 0→0→1→1→2

### 端到端现场测试
1. 启动程序，确认 `edge_tasks` 表新增 `sample_interval`/`report_interval` 列；老任务 `read_interval` 值已复制到 `sample_interval`
2. 确认 `edge_counter_daily` 表已创建
3. 创建一个 EdgeTask：规则 = counter_pulse，阈值 = 1，采样间隔 = 1s，上报间隔 = 5s
4. 用 Modbus 模拟器让某点位 0-1-0-1-0 变化（每 2s 切一次），观察：
   - DEBUG 日志能看到每拍的 `value/threshold/high/edge/triggered/count`
   - `edge_counter_daily` 表当日行 `count` 递增
   - MQTT 每 5s 收到一次当前计数（即使中间多次脉冲）
5. 重启程序，确认 StartTask 加载到当日已有计数，从该值继续累加；首拍不重复计数
6. 跨日测试（可改系统时间）：00:00 后内存清零，新日开始新行；老日记录保留

## 关键文件

- e:\develop\project\mygo\edge5\internal\model\edge_task.go
- e:\develop\project\mygo\edge5\internal\model\edge_counter_daily.go（新增）
- e:\develop\project\mygo\edge5\internal\repository\edge_task.go
- e:\develop\project\mygo\edge5\internal\repository\edge_counter.go（新增）
- e:\develop\project\mygo\edge5\internal\core\edge\edge.go
- e:\develop\project\mygo\edge5\internal\core\edge\engine.go
- e:\develop\project\mygo\edge5\internal\core\edge\func_counter.go
- e:\develop\project\mygo\edge5\internal\service\edge_task.go
- e:\develop\project\mygo\edge5\web\src\views\edge\EdgeTask.vue
