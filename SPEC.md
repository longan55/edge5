# Edge5 边缘网关框架 - 架构设计文档

## 一、项目概述

Edge5 是一个面向边缘计算场景的网关程序框架，旨在为工业设备（PLC、CNC 等）的数据采集、协议转换和云端上报提供统一的基础架构。框架参考 gin-vue-admin 的设计理念，但更注重简洁性和边缘部署的轻量化需求。

### 核心特性

- 轻量级：最小化依赖，适合边缘设备
- 可扩展：通过插件系统支持多种设备和协议
- 可靠：MQTT 离线缓存、设备自动重连
- 安全：基于 JWT 的认证授权体系
- 易用：简洁的 Web 管理界面

## 二、技术栈

### 后端

- **语言**：Go 1.25.3
- **框架**：Gin Web Framework
- **数据库**：
  - 元数据：PostgreSQL 15+ / SQLite3（可选）
  - 缓存：BoltDB（MQTT 离线消息持久化）
- **消息队列**：MQTT（github.com/eclipse/paho.mqtt.golang）
- **日志**：Zap + RotateLogs
- **协程池**：字节跳动 Gopool（已集成）
- **插件**：gRPC（基于 google.golang.org/grpc）
- **配置**：Viper（YAML 配置）

### 前端

- **框架**：Vue 3.4+
- **UI 库**：Element Plus
- **构建工具**：Vite 5+
- **路由**：Vue Router 4
- **状态管理**：Pinia
- **HTTP 客户端**：Axios
- **验证码**：svg-captcha

## 三、系统架构

### 3.1 整体架构图

```
┌─────────────────────────────────────────────────────┐
│                     Web UI (Vue3)                     │
│   ┌──────────┐  ┌──────────┐  ┌──────────────────┐ │
│   │ 登录/权限 │  │ 设备管理  │  │  配置/监控       │ │
│   └──────────┘  └──────────┘  └──────────────────┘ │
└─────────────────────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────┐
│                   API Gateway (Gin)                 │
│   ┌──────────┐  ┌──────────┐  ┌──────────────────┐ │
│   │ JWT 认证  │  │  权限控制  │  │   请求日志       │ │
│   └──────────┘  └──────────┘  └──────────────────┘ │
└─────────────────────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────┐
│                  Service Layer                       │
│  ┌────────┐ ┌────────┐ ┌────────┐ ┌──────────────┐ │
│  │ 用户服务 │ │ 设备服务 │ │MQTT服务 │ │  插件管理器  │ │
│  └────────┘ └────────┘ └────────┘ └──────────────┘ │
└─────────────────────────────────────────────────────┘
                          │
        ┌─────────────────┼─────────────────┐
        ▼                 ▼                 ▼
┌───────────────┐ ┌───────────────┐ ┌───────────────┐
│   PostgreSQL   │ │    BoltDB     │ │  ConnectorPool │
│   (元数据)     │ │   (缓存队列)   │ │  (连接管理)   │
└───────────────┘ └───────────────┘ └───────────────┘
                                               │
                    ┌──────────────────────────┘
                    ▼
        ┌─────────────────────┐
        │   Plugin System     │
        │   (gRPC Plugins)    │
        │ ┌─────┐ ┌─────┐     │
        │ │PLC  │ │CNC  │ ... │
        │ └─────┘ └─────┘     │
        └─────────────────────┘
                    │
                    ▼
        ┌─────────────────────┐
        │   Industrial Devices│
        │ ┌─────┐ ┌─────┐     │
        │ │ PLC │ │ CNC │ ... │
        │ └─────┘ └─────┘     │
        └─────────────────────┘
```

### 3.2 数据流架构

```
设备数据采集流程：

[Device] --协议解析--> [Plugin] --标准化数据--> [CacheQueue]
                                                      │
                                                      ▼
[MQTT Broker] <--发布数据-- [Sender] <--重试机制-- [CacheQueue]
                     (在线时)      (离线时)

配置下发流程：

[Web UI] --API--> [API Server] --设备配置--> [Plugin Manager]
                                            │
                                            ▼
                                    [设备插件] --协议命令--> [Device]
```

## 四、目录结构

```
edge5/
├── cmd/                      # 程序入口
│   └── server/
│       └── main.go
├── config/                   # 配置相关
│   ├── config.go            # 配置加载
│   └── config.yaml          # 配置文件示例
├── internal/                 # 内部包
│   ├── api/                  # API 层
│   │   ├── router/          # 路由注册
│   │   ├── middleware/      # 中间件（认证、日志、权限）
│   │   └── handler/         # 处理器
│   │       ├── sys/        # 系统管理（用户、角色、菜单）
│   │       ├── mqtt/       # MQTT 配置
│   │       └── device/     # 设备管理
│   ├── model/               # 数据模型
│   │   ├── sys/            # 系统模型
│   │   ├── mqtt.go         # MQTT 模型
│   │   └── device.go       # 设备模型
│   ├── repository/          # 数据访问层
│   │   ├── sys/
│   │   ├── mqtt.go
│   │   └── device.go
│   ├── service/             # 业务逻辑层
│   │   ├── sys/
│   │   ├── mqtt.go
│   │   └── device.go
│   ├── pkg/                 # 核心包
│   │   ├── connector/       # 连接管理池
│   │   ├── mqtt/           # MQTT 客户端
│   │   ├── cache/          # 缓存队列
│   │   ├── plugin/         # 插件系统
│   │   └── sender/         # 发送器（发送队列管理）
│   └── utils/               # 工具函数
│       ├── response/       # 统一响应
│       ├── jwt/           # JWT 工具
│       └── captcha/       # 验证码
├── global/                   # 全局对象
│   ├── global.go           # 全局变量声明
│   └── database.go         # 数据库初始化
├── gopool/                   # 协程池（已集成）
├── plugins/                  # 插件实现示例
│   ├── proto/              # gRPC 协议定义
│   ├── plc_mitsubishi/     # 三菱 PLC 插件
│   ├── plc_siemens/        # 西门子 PLC 插件
│   └── cnc_fanuc/          # FANUC CNC 插件
├── web/                      # 前端项目
│   ├── src/
│   │   ├── api/           # API 调用
│   │   ├── router/        # 路由配置
│   │   ├── stores/        # Pinia 状态管理
│   │   ├── views/         # 页面组件
│   │   │   ├── login/     # 登录页
│   │   │   ├── layout/    # 布局组件
│   │   │   ├── system/    # 系统管理
│   │   │   └── device/    # 设备管理
│   │   ├── utils/         # 工具函数
│   │   └── App.vue
│   └── package.json
├── pkg/                      # 公共包（可独立发布）
│   └── edge-sdk/           # SDK（可选）
├── scripts/                  # 脚本
│   ├── init.sql            # 数据库初始化脚本
│   └── build.sh            # 构建脚本
├── go.mod
├── go.sum
└── README.md
```

## 五、数据库设计

### 5.1 元数据库（PostgreSQL/SQLite3）

#### 用户表 (sys_user)

| 字段 | 类型 | 说明 |
|------|------|------|
| id | BIGSERIAL | 主键 |
| username | VARCHAR(64) | 用户名，唯一 |
| password | VARCHAR(128) | 密码（bcrypt 加密） |
| nickname | VARCHAR(64) | 昵称 |
| email | VARCHAR(128) | 邮箱 |
| phone | VARCHAR(32) | 手机号 |
| avatar | VARCHAR(256) | 头像 URL |
| status | TINYINT | 状态：0禁用 1启用 |
| role_id | BIGINT | 关联角色 |
| created_at | TIMESTAMP | 创建时间 |
| updated_at | TIMESTAMP | 更新时间 |

#### 角色表 (sys_role)

| 字段 | 类型 | 说明 |
|------|------|------|
| id | BIGSERIAL | 主键 |
| name | VARCHAR(64) | 角色名称 |
| code | VARCHAR(64) | 角色代码，唯一 |
| status | TINYINT | 状态 |
| sort | INT | 排序 |
| created_at | TIMESTAMP | 创建时间 |

#### 菜单表 (sys_menu)

| 字段 | 类型 | 说明 |
|------|------|------|
| id | BIGSERIAL | 主键 |
| name | VARCHAR(64) | 菜单名称 |
| path | VARCHAR(128) | 路由路径 |
| component | VARCHAR(256) | 组件路径 |
| icon | VARCHAR(64) | 图标 |
| parent_id | BIGINT | 父菜单 ID |
| sort | INT | 排序 |
| type | TINYINT | 类型：1目录 2菜单 3按钮 |
| status | TINYINT | 状态 |
| perms | VARCHAR(128) | 权限标识 |

#### 角色菜单关联表 (sys_role_menu)

| 字段 | 类型 | 说明 |
|------|------|------|
| role_id | BIGINT | 角色 ID |
| menu_id | BIGINT | 菜单 ID |

#### MQTT 配置表 (mqtt_config)

| 字段 | 类型 | 说明 |
|------|------|------|
| id | BIGSERIAL | 主键 |
| broker | VARCHAR(256) | Broker 地址 |
| port | INT | 端口 |
| username | VARCHAR(64) | 用户名 |
| password | VARCHAR(128) | 密码 |
| client_id | VARCHAR(64) | 客户端 ID |
| keep_alive | INT | 保活时间(秒) |
| qos | TINYINT | QoS 级别 |
| status | TINYINT | 连接状态 |
| gateway_sn | VARCHAR(64) | 网关序列号 |
| created_at | TIMESTAMP | 创建时间 |
| updated_at | TIMESTAMP | 更新时间 |

#### 设备表 (device)

| 字段 | 类型 | 说明 |
|------|------|------|
| id | BIGSERIAL | 主键 |
| device_sn | VARCHAR(64) | 设备序列号，唯一 |
| device_name | VARCHAR(128) | 设备名称 |
| device_type | VARCHAR(32) | 设备类型：plc/cnc |
| brand | VARCHAR(32) | 品牌：mitsubishi/siemens/fanuc |
| protocol | VARCHAR(32) | 协议：tcp/serial |
| status | TINYINT | 状态：0禁用 1启用 |
| config | JSONB | 设备配置（JSON） |
| plugin_name | VARCHAR(64) | 关联插件名称 |
| created_at | TIMESTAMP | 创建时间 |
| updated_at | TIMESTAMP | 更新时间 |

#### 设备状态表 (device_status)

| 字段 | 类型 | 说明 |
|------|------|------|
| id | BIGSERIAL | 主键 |
| device_id | BIGINT | 设备 ID |
| online | BOOLEAN | 在线状态 |
| last_heartbeat | TIMESTAMP | 最后心跳时间 |
| message | TEXT | 状态信息 |

### 5.2 缓存数据库（BoltDB）

BoltDB 用于 MQTT 离线消息的持久化存储，采用以下 bucket 结构：

```
cache_queue          # 缓存队列 bucket
  └── messages       # 消息列表（按时间戳排序）
      └── [uuid] -> Message struct

pending_queue        # 待发送队列
  └── messages
      └── [uuid] -> Message struct

sent_history         # 发送历史（用于去重）
  └── [request_id] -> timestamp
```

#### 缓存消息结构

```go
type CacheMessage struct {
    ID          string    `json:"id"`           // 消息唯一 ID
    Topic       string    `json:"topic"`        // MQTT 主题
    Payload     []byte    `json:"payload"`      // 消息内容
    RetryCount  int       `json:"retry_count"`  // 重试次数
    CreatedAt   int64     `json:"created_at"`   // 创建时间戳
    NextRetryAt int64     `json:"next_retry_at"`// 下次重试时间
}
```

## 六、核心模块设计

### 6.1 连接管理池（Connector Pool）

#### 设计目标

- 统一管理所有设备连接
- 自动重连机制（指数退避）
- 状态监控和通知

#### 接口定义

```go
// Connector 连接器接口
type Connector interface {
    // Uri 返回连接标识
    Uri() string
    // Connect 建立连接
    Connect() error
    // Close 关闭连接
    Close() error
    // IsConnected 检查连接状态
    IsConnected() bool
}

// ConnectorManager 连接管理器
type ConnectorManager interface {
    // Register 注册连接器
    Register(key string, conn Connector) error
    // Unregister 注销连接器
    Unregister(key string) error
    // Get 获取连接器
    Get(key string) (Connector, bool)
    // List 获取所有连接器
    List() map[string]Connector
    // States 获取所有连接状态
    States() map[string]bool
}
```

#### 重连策略

采用指数退避算法，参考 TCP 重传机制：

```
base_delay = 1s
max_delay = 60s
factor = 2

delay = min(base_delay * (factor ^ retry_count), max_delay)
```

- 初始重试延迟：1 秒
- 最大重试延迟：60 秒
- 定时检查：15 秒（参考代码）
- 最大重试次数：无限制（持续重连）

### 6.2 MQTT 客户端

#### 设计目标

- 与连接管理池集成
- 支持配置化（从数据库读取）
- 消息发布和订阅
- 自动重连
- 离线消息缓存

#### 接口定义

```go
// MQTTClient MQTT 客户端接口
type MQTTClient interface {
    // Connect 连接
    Connect() error
    // Disconnect 断开连接
    Disconnect()
    // Publish 发布消息
    Publish(topic string, payload []byte, qos byte) error
    // Subscribe 订阅主题
    Subscribe(topic string, qos byte, handler MessageHandler) error
    // Unsubscribe 取消订阅
    Unsubscribe(topic string) error
    // IsConnected 检查连接状态
    IsConnected() bool
    // SetStatusCallback 设置状态回调
    SetStatusCallback(callback func(bool))
}

// MessageHandler 消息处理函数
type MessageHandler func(topic string, payload []byte)
```

#### MQTT 主题规范（参考 aixot 规范）

```
网关注册：    /aixot/up/gateway/register
网关心跳：    /aixot/up/{gatewaySn}/heartbeat
网关状态：    /aixot/up/{gatewaySn}/properties
设备数据：    /aixot/up/{gatewaySn}/{deviceSn}/data
命令下发：    /aixot/down/{gatewaySn}/command
命令响应：    /aixot/up/{gatewaySn}/command/reply
```

### 6.3 缓存队列（Cache Queue）

#### 设计目标

- MQTT 离线时持久化消息
- 发送层不感知缓存逻辑
- 定时重试发送
- 消息去重

#### 接口定义

```go
// Sender 发送器接口（发送层使用）
type Sender interface {
    // Send 发送消息（内部处理离线缓存）
    Send(topic string, payload []byte) error
    // SendWithRetry 发送消息（指定重试）
    SendWithRetry(topic string, payload []byte, maxRetry int) error
}

// CacheQueue 缓存队列接口
type CacheQueue interface {
    // Push 添加消息到缓存
    Push(msg *CacheMessage) error
    // Pop 取出消息
    Pop() (*CacheMessage, error)
    // Peek 查看消息（不取出）
    Peek() (*CacheMessage, error)
    // Size 队列大小
    Size() int
    // Clear 清空队列
    Clear() error
}
```

#### 发送流程

```
1. Sender.Send(topic, payload)
   │
   ├─> MQTT 在线？
   │   ├─ 是：直接 Publish，失败则缓存
   │   └─ 否：缓存到 BoltDB
   │
2. 后台任务（定时 5s）：
   ├─ 检查 MQTT 连接状态
   ├─ 从 BoltDB 读取缓存消息
   ├─ 按时间排序，依次发送
   └─ 发送成功则删除缓存
```

### 6.4 插件系统（Plugin System）

#### 设计目标

- 基于 gRPC 的插件架构
- 插件独立编译、部署
- 支持多种设备和协议
- 热插拔（运行时加载/卸载）

#### 协议定义（Protocol Buffers）

```protobuf
syntax = "proto3";

package edge.plugin;

service DevicePlugin {
    // 获取插件信息
    rpc GetPluginInfo(InfoRequest) returns (InfoResponse);
    
    // 连接设备
    rpc Connect(ConnectRequest) returns (ConnectResponse);
    
    // 断开设备
    rpc Disconnect(DisconnectRequest) returns (DisconnectResponse);
    
    // 读取数据
    rpc ReadData(ReadRequest) returns (ReadResponse);
    
    // 写入数据
    rpc WriteData(WriteRequest) returns (WriteResponse);
    
    // 订阅数据
    rpc SubscribeData(SubscribeRequest) returns (stream DataResponse);
}

// 插件信息
message PluginInfo {
    string name = 1;           // 插件名称
    string version = 2;         // 版本号
    string device_type = 3;    // 设备类型
    string brand = 4;           // 品牌
    repeated string protocols = 5; // 支持的协议
}

// 设备配置
message DeviceConfig {
    string device_sn = 1;
    string protocol = 2;       // tcp/serial
    map<string, string> params = 3; // 协议参数
}

// 连接请求
message ConnectRequest {
    DeviceConfig config = 1;
}

// 连接响应
message ConnectResponse {
    bool success = 1;
    string message = 2;
    bool is_connected = 3;
}

// 读取数据请求
message ReadRequest {
    string device_sn = 1;
    repeated string addresses = 2; // 读取地址列表
}

// 读取数据响应
message ReadResponse {
    bool success = 1;
    string message = 2;
    map<string, bytes> values = 3; // 地址 -> 值
}

// 写入数据请求
message WriteRequest {
    string device_sn = 1;
    map<string, bytes> values = 2; // 地址 -> 值
}

// 写入数据响应
message WriteResponse {
    bool success = 1;
    string message = 2;
}

// 数据响应（流）
message DataResponse {
    string device_sn = 1;
    map<string, bytes> values = 2;
    int64 timestamp = 3;
}
```

#### 插件管理器接口

```go
// PluginManager 插件管理器
type PluginManager interface {
    // LoadPlugin 加载插件
    LoadPlugin(name string, addr string) error
    // UnloadPlugin 卸载插件
    UnloadPlugin(name string) error
    // GetPlugin 获取插件
    GetPlugin(name string) (DevicePlugin, bool)
    // ListPlugins 列出所有插件
    ListPlugins() []PluginInfo
    // RegisterHandler 注册数据回调
    RegisterHandler(handler DataHandler)
}

// DevicePlugin 设备插件接口
type DevicePlugin interface {
    // GetInfo 获取插件信息
    GetInfo() (*PluginInfo, error)
    // Connect 连接设备
    Connect(config *DeviceConfig) error
    // Disconnect 断开设备
    Disconnect() error
    // Read 读取数据
    Read(addresses []string) (map[string][]byte, error)
    // Write 写入数据
    Write(values map[string][]byte) error
    // Subscribe 订阅数据（返回 channel）
    Subscribe() (<-chan *DeviceData, error)
}

// DataHandler 数据处理回调
type DataHandler func(deviceSn string, data map[string]interface{})
```

### 6.5 设备模型（Device Model）

#### 通用设备模型

```go
type Device struct {
    ID          uint64     `json:"id"`
    DeviceSn    string     `json:"device_sn"`
    DeviceName  string     `json:"device_name"`
    DeviceType  string     `json:"device_type"`   // plc/cnc
    Brand       string     `json:"brand"`         // mitsubishi/siemens/fanuc
    Protocol    string     `json:"protocol"`      // tcp/serial
    Config      DeviceConfig `json:"config"`
    Status      int        `json:"status"`
    PluginName  string     `json:"plugin_name"`
    CreatedAt   time.Time  `json:"created_at"`
    UpdatedAt   time.Time  `json:"updated_at"`
}

type DeviceConfig struct {
    // 通用配置
    Timeout   int `json:"timeout"`   // 连接超时(ms)
    Retry     int `json:"retry"`     // 重试次数
    
    // TCP 配置
    IP        string `json:"ip,omitempty"`
    Port      int    `json:"port,omitempty"`
    
    // 串口配置
    SerialPort string `json:"serial_port,omitempty"`
    BaudRate  int    `json:"baud_rate,omitempty"`
    DataBits  int    `json:"data_bits,omitempty"`
    Parity    string `json:"parity,omitempty"`
    StopBits  int    `json:"stop_bits,omitempty"`
    
    // 品牌特定配置（JSONB 存储）
    Extra     map[string]interface{} `json:"extra,omitempty"`
}
```

#### 设备注册到连接池

```go
// 设备管理器
type DeviceManager interface {
    // LoadDevices 从数据库加载设备
    LoadDevices() error
    // AddDevice 添加设备
    AddDevice(device *Device) error
    // UpdateDevice 更新设备
    UpdateDevice(device *Device) error
    // DeleteDevice 删除设备
    DeleteDevice(id uint64) error
    // GetDevice 获取设备
    GetDevice(id uint64) (*Device, error)
    // StartDevice 启动设备连接
    StartDevice(id uint64) error
    // StopDevice 停止设备连接
    StopDevice(id uint64) error
    // GetDeviceStatus 获取设备状态
    GetDeviceStatus(id uint64) (bool, error)
}
```

## 七、API 设计

### 7.1 系统管理

#### 登录

```
POST /api/v1/login
Content-Type: application/json

Request:
{
    "username": "admin",
    "password": "admin123",
    "captcha_id": "xxx",
    "captcha": "1234"
}

Response:
{
    "code": 0,
    "message": "success",
    "data": {
        "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
        "user": {
            "id": 1,
            "username": "admin",
            "nickname": "管理员",
            "avatar": "",
            "role": {
                "id": 1,
                "name": "超级管理员",
                "code": "admin"
            }
        }
    }
}
```

#### 获取用户信息

```
GET /api/v1/user/info
Authorization: Bearer {token}

Response:
{
    "code": 0,
    "message": "success",
    "data": {
        "id": 1,
        "username": "admin",
        "nickname": "管理员",
        "avatar": "",
        "role": {...},
        "permissions": ["user:list", "user:add", ...]
    }
}
```

#### 获取菜单列表

```
GET /api/v1/menu/list
Authorization: Bearer {token}

Response:
{
    "code": 0,
    "message": "success",
    "data": [
        {
            "id": 1,
            "name": "系统管理",
            "path": "/system",
            "component": "Layout",
            "children": [
                {
                    "id": 2,
                    "name": "用户管理",
                    "path": "user",
                    "component": "/system/user/index",
                    "perms": "user:list"
                }
            ]
        }
    ]
}
```

### 7.2 MQTT 配置

#### 获取 MQTT 配置

```
GET /api/v1/mqtt/config
Authorization: Bearer {token}

Response:
{
    "code": 0,
    "data": {
        "id": 1,
        "broker": "mqtt.example.com",
        "port": 1883,
        "username": "gateway",
        "client_id": "gateway_001",
        "keep_alive": 60,
        "qos": 1,
        "status": 1,
        "gateway_sn": "GWD001"
    }
}
```

#### 更新 MQTT 配置

```
PUT /api/v1/mqtt/config
Authorization: Bearer {token}
Content-Type: application/json

{
    "broker": "mqtt.example.com",
    "port": 1883,
    "username": "gateway",
    "password": "password",
    "client_id": "gateway_001",
    "keep_alive": 60,
    "qos": 1,
    "gateway_sn": "GWD001"
}
```

#### 测试 MQTT 连接

```
POST /api/v1/mqtt/test
Authorization: Bearer {token}

Response:
{
    "code": 0,
    "message": "连接成功"
}
```

### 7.3 设备管理

#### 获取设备列表

```
GET /api/v1/device/list?page=1&page_size=10&device_type=plc
Authorization: Bearer {token}

Response:
{
    "code": 0,
    "data": {
        "list": [
            {
                "id": 1,
                "device_sn": "DEV001",
                "device_name": "三菱 PLC 1号",
                "device_type": "plc",
                "brand": "mitsubishi",
                "protocol": "tcp",
                "status": 1,
                "online": true,
                "last_heartbeat": "2024-01-01T10:00:00Z"
            }
        ],
        "total": 100
    }
}
```

#### 创建设备

```
POST /api/v1/device
Authorization: Bearer {token}
Content-Type: application/json

{
    "device_sn": "DEV001",
    "device_name": "三菱 PLC 1号",
    "device_type": "plc",
    "brand": "mitsubishi",
    "protocol": "tcp",
    "config": {
        "ip": "192.168.1.100",
        "port": 6000,
        "timeout": 5000
    },
    "plugin_name": "plc_mitsubishi"
}
```

#### 更新设备

```
PUT /api/v1/device/:id
Authorization: Bearer {token}
Content-Type: application/json

{
    "device_name": "三菱 PLC 1号（已修改）",
    "status": 1
}
```

#### 删除设备

```
DELETE /api/v1/device/:id
Authorization: Bearer {token}
```

#### 启动设备

```
POST /api/v1/device/:id/start
Authorization: Bearer {token}
```

#### 停止设备

```
POST /api/v1/device/:id/stop
Authorization: Bearer {token}
```

#### 获取设备状态

```
GET /api/v1/device/:id/status
Authorization: Bearer {token}

Response:
{
    "code": 0,
    "data": {
        "online": true,
        "last_heartbeat": "2024-01-01T10:00:00Z",
        "message": ""
    }
}
```

### 7.4 插件管理

#### 获取插件列表

```
GET /api/v1/plugin/list
Authorization: Bearer {token}

Response:
{
    "code": 0,
    "data": [
        {
            "name": "plc_mitsubishi",
            "version": "1.0.0",
            "device_type": "plc",
            "brand": "mitsubishi",
            "status": "loaded"
        }
    ]
}
```

#### 加载插件

```
POST /api/v1/plugin/load
Authorization: Bearer {token}
Content-Type: application/json

{
    "name": "plc_mitsubishi",
    "addr": "127.0.0.1:50051"
}
```

#### 卸载插件

```
POST /api/v1/plugin/unload
Authorization: Bearer {token}
Content-Type: application/json

{
    "name": "plc_mitsubishi"
}
```

## 八、前端设计

### 8.1 页面结构

```
/                           # 登录页
├── /layout                 # 管理后台布局
│   ├── /dashboard         # 仪表盘（网关状态）
│   ├── /system            # 系统管理
│   │   ├── /user          # 用户管理
│   │   ├── /role          # 角色管理
│   │   └── /menu          # 菜单管理
│   ├── /mqtt              # MQTT 配置
│   ├── /device           # 设备管理
│   │   ├── /list         # 设备列表
│   │   └── /config       # 设备配置
│   └── /plugin           # 插件管理
```

### 8.2 路由守卫

- 白名单：/login, /404
- 权限验证：获取用户信息，验证 token 有效性
- 菜单权限：根据用户角色过滤可访问菜单

### 8.3 状态管理

使用 Pinia 管理以下状态：

- `useUserStore`：用户信息、token、权限
- `useDeviceStore`：设备列表、设备状态
- `useMqttStore`：MQTT 连接状态

## 九、配置设计

### config.yaml

```yaml
server:
  host: "0.0.0.0"
  port: 8080
  mode: "debug"  # debug/release

database:
  type: "sqlite3"  # sqlite3/postgresql
  sqlite3:
    path: "./data/edge5.db"
  postgresql:
    host: "localhost"
    port: 5432
    user: "edge5"
    password: "password"
    dbname: "edge5"
    sslmode: "disable"

cache:
  type: "boltdb"
  boltdb:
    path: "./data/cache.db"
    bucket: "cache_queue"

mqtt:
  enabled: true
  broker: "tcp://mqtt.example.com:1883"
  port: 1883
  username: ""
  password: ""
  client_id: "edge5-gateway"
  keep_alive: 60
  qos: 1
  gateway_sn: "GWD001"

log:
  level: "info"  # debug/info/warn/error
  path: "./logs"
  pattern: "%Y%m%d.log"
  max_age: 7    # 天
  rotation_time: 24  # 小时
  compress: true

connector:
  reconnect_interval: 15000  # ms
  base_delay: 1000  # ms
  max_delay: 60000  # ms
  factor: 2

plugin:
  enabled: true
  grpc_port: 50051
  plugins_dir: "./plugins"

jwt:
  secret: "your-secret-key"
  expire: 24  # 小时
  refresh_expire: 168  # 小时
```

## 十、优雅退出机制

### 退出流程

```
1. 接收信号（SIGINT/SIGTERM）
2. 停止接受新请求
3. 取消注册定时任务
4. 关闭 MQTT 连接（等待消息发送完成）
5. 关闭设备连接（停止采集）
6. 关闭插件连接
7. 保存缓存队列
8. 关闭数据库连接
9. 关闭日志
10. 退出进程
```

### 实现代码

```go
// global/process.go
var quitTasks []struct {
    f       func() error
    content string
    order   int
}

func RegisterQuitTask(f func() error, content string, order int) {
    quitTasks = append(quitTasks, struct {
        f       func() error
        content string
        order   int
    }{f, content, order})
}

func GracefullyExit() {
    // 按 order 排序
    sort.Slice(quitTasks, func(i, j int) bool {
        return quitTasks[i].order < quitTasks[j].order
    })
    
    for _, task := range quitTasks {
        Logger.Info("执行退出任务: " + task.content)
        if err := task.f(); err != nil {
            Logger.Error(task.content, zap.Error(err))
        }
    }
}
```

### 退出任务顺序

```
order = 1:  关闭 HTTP 服务器（停止接受新请求）
order = 2:  关闭 MQTT 连接
order = 3:  停止设备采集
order = 4:  关闭设备连接
order = 5:  关闭插件连接
order = 6:  保存缓存队列到 BoltDB
order = 7:  关闭 BoltDB
order = 8:  关闭 SQL 数据库
order = 9:  刷新日志缓冲区
order = 10: 关闭日志
```

## 十一、开发指南

### 11.1 快速开始

1. 克隆项目
2. 修改配置文件 `config/config.yaml`
3. 初始化数据库
4. 启动后端服务
5. 启动前端开发服务器

### 11.2 创建新插件

1. 在 `plugins/` 下创建新目录
2. 定义 .proto 文件
3. 实现 gRPC 服务
4. 注册到插件管理器
5. 在 Web 界面加载插件

### 11.3 添加新 API

1. 在 `internal/model/` 定义请求/响应结构
2. 在 `internal/repository/` 实现数据访问
3. 在 `internal/service/` 实现业务逻辑
4. 在 `internal/api/handler/` 实现处理器
5. 在 `internal/api/router/` 注册路由
6. 在前端添加 API 调用

## 十二、版本规划

### v1.0.0 (MVP)

- 基础框架搭建
- 用户认证和权限
- MQTT 配置和管理
- 设备管理（CRUD）
- 连接管理池
- 消息缓存队列
- 基础插件系统

### 后续版本

- v1.1: 完善 PLC/CNC 插件
- v1.2: 设备数据可视化
- v1.3: 远程配置下发
- v2.0: 插件市场

## 十三、参考文档

- [Gin Web Framework](https://gin-gonic.com/)
- [Vue 3 Documentation](https://vuejs.org/)
- [Element Plus](https://element-plus.org/)
- [BoltDB](https://github.com/etcd-io/bbolt)
- [paho.mqtt.golang](https://github.com/eclipse/paho.mqtt.golang)
- [gRPC](https://grpc.io/)
- [Zap Logger](https://github.com/uber-go/zap)
