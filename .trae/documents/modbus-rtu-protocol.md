# Modbus RTU 协议实现计划

## Context（背景与目标）

当前 `builtin/` 下已有 mcprotocol、delta、s7、panasonic 等协议，但缺少通用的 Modbus RTU 串口协议。需求要点：

- **站号在连接参数中指定**（不在每个点位重复配置站号），符合"一台设备一个从站"的常见用法。
- **点位配置参数与 PLC 一致**：`address`（地址）+ `parseType`（数据类型）+ `length`（缺省 1），不填站号。
- **字节序通过数据类型名区分**：32/64 位类型提供 ABCD/CDAB/BADC/DCBA 四种变体，适配不同设备的字/字节排列方式。
- **前端无需改动**：设备创建页、调试页、任务页均动态从 `/device/options` 和 `/device/{id}/debug/info` 获取协议元数据（`connectionParams` + `readParamsSchema.choices`），后端协议 `Info()` 返回正确 schema 即可自动渲染。
- 实现完全参考 `delta_rtu_serial.go` + `delta_common.go`（已用 `gomodbus/v2` + `goburrow/serial`，站号在连接参数中）。

## 已确认的设计决策

1. **字节序命名**：ABCD 后缀式。`int32_abcd/_cdab/_badc/_dcba`，`uint32_*`、`float32_*`、`int64_*`、`uint64_*`、`float64_*` 同理。保留兼容别名：`int32`=`int32_cdab`（Modbus 最常见字序）、`float`=`float32_cdab`、`long`=`int64_cdab`、`double`=`float64_cdab` 等，与 mcprotocol 风格一致。
2. **地址格式**：Modbus 标准前缀式，前缀自动识别功能码，地址 1-based：
   - `00001`/`0xxxx` → 线圈（读 FC1 `ReadCoils` / 写 FC5 `WriteSingleCoil` / FC15）
   - `10001`/`1xxxx` → 离散输入（读 FC2 `ReadDiscreteInputs`，只读）
   - `30001`/`3xxxx` → 输入寄存器（读 FC4 `ReadInputRegisters`，只读）
   - `40001`/`4xxxx` → 保持寄存器（读 FC3 `ReadHoldingRegisters` / 写 FC6 `WriteSingleRegister` / FC16 `WriteMultipleRegisters`）
   - 纯数字（无前缀）→ 默认按保持寄存器处理。
3. **校验位 parity**：英文 `N`/`E`/`O`，默认 `N`（无校验）。

## 文件结构

新建目录 `internal/core/protocol/builtin/modbusrtu/`，两个文件（参考 delta 包结构）：

### 1. `modbusrtu/modbus.go` — 主服务（参考 [delta_rtu_serial.go](file:///e:/develop/project/mygo/edge5/internal/core/protocol/builtin/delta/delta_rtu_serial.go)）

```go
package modbusrtu

func init() {
    if err := builtin.Register(NewModbusRTUService()); err != nil {
        panic(...)
    }
}

type ModbusRTUService struct {
    lock             sync.Mutex
    Clients          map[uint]modbus.Client      // deviceID -> client
    Providers        map[uint]modbus.ClientProvider
    SlaveIDs         map[uint]byte               // deviceID -> 站号（连接参数）
    connected        bool
    deviceID         uint
    connection       protocol.Metadata
    portMap          map[uint]string             // deviceID -> 串口（用于 Release）
}
```

- **Info()** 返回 `protocol.Metadata`：
  - `name`="Modbus-RTU"，`alias`="ModbusRTU"，`deviceType`="PLC"，`brand`="Modbus"，`cName`="Modbus"
  - `supportServer`=false，`supportDebug`=true，`models`=`["通用"]`
  - **connectionParams**（站号在此）：
    - `serial_port` select default "COM1"
    - `baud_rate` select choices [1200,2400,4800,9600,19200,38400,57600,115200] default "9600"
    - `data_bits` select [7,8] default "8"
    - `parity` select [N,E,O] default "N"
    - `stop_bits` select [1,2] default "1"
    - `slave_id` number default "1"   ← 站号
  - **readParamsSchema**（点位参数，与 PLC 一致）：
    - `address` string required default ""
    - `length` int default "1"        ← 偏移/数量，缺省 1
    - `parseType` select choices=[bool, byte, int16, uint16, int32, uint32, int64, uint64, float32, float64, string, int32_abcd, int32_cdab, int32_badc, int32_dcba, uint32_abcd, uint32_cdab, uint32_badc, uint32_dcba, float32_abcd, float32_cdab, float32_badc, float32_dcba, int64_abcd, int64_cdab, int64_badc, int64_dcba, uint64_abcd, uint64_cdab, uint64_badc, uint64_dcba, float64_abcd, float64_cdab, float64_badc, float64_dcba]
- **Connect/Disconnect/IsConnected/IsSupportServer/ReadBatch/WriteBatch**：照搬 delta_rtu_serial 结构。
  - Connect：解析 serial_port/baud_rate/data_bits/parity/stop_bits/slave_id → `builtin.SerialManager.Acquire(port, deviceID)` → `modbus.NewRTUClientProvider` + `modbus.NewClient` → `client.Connect()` → 缓存 client/slaveID/portMap。
  - ReadBatch：取 deviceID 的 client+slaveID，调用 `readBatchMerged`（**同功能码连续地址合并读取**，见下）。
  - WriteBatch：取 deviceID 的 client+slaveID，调用 `writeBatchMerged`（连续保持寄存器合并为一次 `WriteMultipleRegisters` FC16，线圈合并为 FC15）。
  - Disconnect：`client.Close()` + `SerialManager.Release(port)`。
  - 复用 `builtin.GetString`/`builtin.GetInt`/`builtin.SerialManager`/`builtin.Logger`。
  - parity 英文 N/E/O 直接传入 `serial.Config.Parity`（无需中文映射）。

### 2. `modbusrtu/modbus_common.go` — 地址解析 + 字节序读写（参考 [delta_common.go](file:///e:/develop/project/mygo/edge5/internal/core/protocol/builtin/delta/delta_common.go)）

```go
type modbusAddress struct {
    code  byte   // 1=线圈FC1/5, 2=离散输入FC2, 3=输入寄存器FC4, 4=保持寄存器FC3/6/16
    addr  uint16 // 0-based 寄存器/线圈地址
    isBit bool   // 线圈/离散输入为 true
}

// parseAddress 按前缀识别功能码
// "00001"或"0xxx" → code=1,isBit=true,addr=xxx-1
// "10001"或"1xxx" → code=2,isBit=true,addr=xxx-1
// "30001"或"3xxx" → code=3,isBit=false,addr=xxx-1
// "40001"或"4xxx" → code=4,isBit=false,addr=xxx-1
// 纯数字 → code=4(保持寄存器),addr=n-1
func parseAddress(address string) (modbusAddress, error)
```

**字节序实现**（核心，先转字节流再按 4 种方式组装）：

```go
// regsToBytes 把 gomodbus 返回的 []uint16 还原为 Modbus 线路原始字节流
// 每寄存器 2 字节大端 [hi, lo]（gomodbus 解析 uint16 = hi*256+lo）
func regsToBytes(results []uint16) []byte

// assembleUint32 从 2 个寄存器按字节序组装 32 位值
// results = [R0, R1]，对应线路字节 [hi0,lo0, hi1,lo1]
// abcd: [hi0,lo0,hi1,lo1] 大端 = R0<<16 | R1
// cdab: [hi1,lo1,hi0,lo0] 大端 = R1<<16 | R0
// badc: [lo0,hi0,lo1,hi1] 大端 = swap16(R0)<<16 | swap16(R1)
// dcba: [lo1,hi1,lo0,hi0] 大端 = swap16(R1)<<16 | swap16(R0)
func assembleUint32(results []uint16, order string) uint32
// assembleUint64 同理（4 个寄存器），按字节序重排 8 字节后大端读取
```

**批量合并读取（核心性能优化）**：Modbus RTU 串口每次通信都有往返延迟（低波特率下尤其明显），同功能码且地址连续/重叠的点位必须合并为一次 Modbus 读取调用，再从结果中切分各点位值。

```go
type pointPlan struct {
    pt       protocol.Point
    addr     modbusAddress
    regCount uint16   // 该点位占用的寄存器/线圈数（int32=2, int64=4, bool=1, string=(length+1)/2 ...）
    index    int      // 原始点位在 req.Points 中的下标（保证结果顺序）
}

type readSegment struct {
    code     byte
    start    uint16   // 段起始地址（0-based）
    count    uint16   // 段总寄存器/线圈数
    points   []pointPlan  // 落在本段的点位
}

// planReadBatch 分组合并算法：
// 1. 解析每个点位为 pointPlan（code+addr+regCount）。
// 2. 按 code 分组（线圈/离散输入/输入寄存器/保持寄存器 4 组）。
// 3. 组内按 addr 升序排序。
// 4. 合并相邻可合并的点位为 readSegment：
//    当前段 [segStart, segEnd] 与下一点位 [pAddr, pAddr+pRegCount-1] 比较，
//    若 pAddr <= segEnd+1（相邻或重叠，允许小空隙）则并入，segEnd 取 max。
//    否则开启新段。
// 5. 对每个 segment 按 code 调用一次 Modbus：
//    code=1 → client.ReadCoils(slaveID, start, count)
//    code=2 → client.ReadDiscreteInputs(slaveID, start, count)
//    code=3 → client.ReadInputRegisters(slaveID, start, count)
//    code=4 → client.ReadHoldingRegisters(slaveID, start, count)
// 6. 每个点位从所属段结果中，按偏移 (pAddr - segStart) 取子集寄存器/线圈，再按数据类型+字节序解析。
func planReadBatch(points []protocol.Point) ([]readSegment, []pointPlan, error)

// 单点解析：从段结果（[]uint16 寄存器 或 []byte 线圈）按偏移取值
func decodePoint(plan pointPlan, segResults []uint16) (interface{}, error)
```

合并约束与边界：
- **功能码上限**：保持寄存器 FC3 单次≤125、输入寄存器 FC4≤125、线圈 FC1≤2000、离散输入 FC2≤2000。段 `count` 超上限时**拆分**为多段（每次不超过上限）。
- **跨功能码不合并**：不同 code 必须独立读取（Modbus 协议限制）。
- **重叠点位**：两点位地址区间重叠时，仍只读一次段，两点位各自从段结果中取值。
- **结果顺序**：按 `pointPlan.index` 还原为输入 `req.Points` 顺序，保证 `BatchReadResponse.Results` 顺序与请求一致。
- **错误隔离**：某段读取失败，仅该段内点位 quality=bad + Error，其他段不受影响（逐段独立调用）。

**字节序与数据类型解析**（从单个点位所属的子寄存器序列解析）：
- 位类型（bool/byte on bit）：线圈 `ReadCoils`、离散输入 `ReadDiscreteInputs`，取线圈结果位。
- 寄存器类型：从段结果 `[]uint16` 取该点位的 `regCount` 个连续寄存器，按 `regsToBytes` + `assembleUint32/Uint64` + 字节序解析。
- 单点解析函数 `decodePoint(plan, segResults)` 按 `pt.DataType` switch，复用 `assembleUint32/Uint64`（带 order，从 DataType 名后缀解析 `_abcd/_cdab/_badc/_dcba`）。
- 兼容别名映射：`int32`→`cdab`、`float`→`float32_cdab`、`int16/short`→单寄存器、`string`→按 length 字节数取寄存器拼接。
- 值类型转换辅助 `toInt16/toUint16/toInt32/.../toFloat64`（照搬 delta_common 的 `toXxx` 函数）。
- 字符串：`length` 作为字节数，`regCount=(length+1)/2`，每个寄存器取 `[hi,lo]` 拼接再截断到 length。
- `BatchReadResult.PointName` 用 `pt.Resource`（与 delta/mcprotocol 一致）。

**写入合并**：
- 连续保持寄存器点位合并为一次 `WriteMultipleRegisters`（FC16）；连续线圈合并为 `WriteMultipleCoils`（FC15）。
- 单点或不连续则降级为 `WriteSingleRegister`（FC6）/`WriteSingleCoil`（FC5）。
- 先把每个点位值按其数据类型+字节序编码为寄存器序列，再按地址合并到段。

## 要修改的文件

### 新建（2 个）
- `internal/core/protocol/builtin/modbusrtu/modbus.go`
- `internal/core/protocol/builtin/modbusrtu/modbus_common.go`

### 修改（1 个，1 行）
- [main.go](file:///e:/develop/project/mygo/edge5/main.go#L12-L20)：在 import 块加入 `_ "edge5/internal/core/protocol/builtin/modbusrtu"`，使 `init()` 注册生效。与现有 `_ "...a1eprotocol"`、`_ "...mcprotocol"` 等空导入并列。

### 前端：无需改动
设备创建页 [List.vue](file:///e:/develop/project/mygo/edge5/web/src/views/device/List.vue)、调试 schema（`debugSchemaColumns` 用 `param.choices` 覆盖）、任务页 [Task.vue](file:///e:/develop/project/mygo/edge5/internal/core/protocol/builtin/../web/src/views/task/Task.vue) 均动态读取后端 schema。`/device/options`（[device_options_handler.go](file:///e:/develop/project/mygo/edge5/internal/handler/device_options_handler.go)）和 `/device/{id}/debug/info`（[device_debug_handler.go](file:///e:/develop/project/mygo/edge5/internal/handler/device_debug_handler.go)）通过 `ExtractConnectionParams`/`ExtractReadParamsSchema` 自动返回新协议参数。

## 复用的现有设施

- [builtin.Register](file:///e:/develop/project/mygo/edge5/internal/core/protocol/builtin/registry.go) — 协议注册
- [builtin.SerialManager](file:///e:/develop/project/mygo/edge5/internal/core/protocol/builtin/serial_manager.go) — 串口占用管理（Acquire/Release）
- [builtin.GetString/GetInt](file:///e:/develop/project/mygo/edge5/internal/core/protocol/builtin/util.go)、`builtin.Logger`
- [protocol.Metadata/Point/BatchReadRequest/...](file:///e:/develop/project/mygo/edge5/internal/core/protocol/protocol.go) — 协议接口与数据结构
- [protocol.ExtractConnectionParams/ExtractReadParamsSchema](file:///e:/develop/project/mygo/edge5/internal/core/protocol/metadata.go) — 前端 schema 抽取
- 依赖：`github.com/thinkgos/gomodbus/v2`、`github.com/goburrow/serial`（delta 已引入，go.mod 无需新增）

## 验证方法（端到端）

1. **编译**：`go build ./...` 确认无语法/类型错误。
2. **启动**：运行程序，日志应出现 `注册内置协议 name=Modbus-RTU`。
3. **前端设备创建**：访问设备列表 → 新增设备 → 设备类型选 PLC → 品牌选 Modbus → 协议选 Modbus-RTU → 确认连接参数表单自动出现 serial_port/baud_rate/data_bits/parity/stop_bits/slave_id（含下拉选项）。
4. **调试页**：进入设备调试 → 确认采集参数表头出现 address/length/parseType，parseType 下拉含全部字节序变体。添加点位如 `40001` + `int32_cdab`，点读取，验证能读到保持寄存器值。
5. **任务/数采页**：任务页选 Modbus-RTU 设备，确认 readParamsSchema 加载、点位可配置字段名+地址+类型+长度。
6. **字节序**：对同一 32 位寄存器用 4 种类型读取，验证值差异符合 ABCD/CDAB/BADC/DCBA 定义。
7. **地址格式**：分别用 `00001`（线圈）、`10001`（离散输入）、`30001`（输入寄存器）、`40001`（保持寄存器）验证功能码自动识别正确。
