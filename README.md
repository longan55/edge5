# Edge5 边缘网关框架

一个面向边缘计算场景的网关程序框架，专注于工业设备（PLC、CNC 等）的数据采集、协议转换和云端上报。

## 核心目标

- **全协议接入**：目标支持所有常见工业协议
- **开箱即用**：内置协议驱动，无需额外配置即可采集数据
- **热插拔协议**：支持动态添加协议插件，无需重启服务
- **自动处理**：自动完成数据采集、协议转换、缓存和上报
- **协议打通**：统一数据格式，屏蔽协议差异

## 已支持协议

| 协议 | 品牌 | 设备类型 | 说明 |
|------|------|----------|------|
| MC Protocol | 三菱 | PLC (Q系列) | 内置支持 |

## 计划支持协议

| 协议 | 品牌 | 设备类型 | 状态 |
|------|------|----------|------|
| Modbus | 通用 | PLC/传感器 | 开发中 |
| CNC Protocol | 三菱 | CNC | 规划中 |

---

## 技术栈

### 后端

- **语言**: Go 1.25.3
- **框架**: Gin Web Framework
- **数据库**: SQLite3 + BoltDB（缓存）
- **消息**: MQTT (paho.mqtt.golang)
- **日志**: Zap + RotateLogs
- **插件**: gRPC
- **配置**: Viper (YAML)

### 前端

- **框架**: Vue 3 + Vite 5
- **UI**: Element Plus
- **状态管理**: Pinia
- **路由**: Vue Router 4
- **HTTP**: Axios

---

## 系统架构

### 整体架构

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
│    SQLite3    │ │    BoltDB     │ │  ConnectorPool │
│   (元数据)     │ │   (缓存队列)   │ │  (连接管理)   │
└───────────────┘ └───────────────┘ └───────────────┘
                                               │
                    ┌──────────────────────────┘
                    ▼
        ┌─────────────────────┐
        │   Protocol System   │
        │   (内置协议驱动)     │
        │ ┌─────┐ ┌─────┐     │
        │ │MC协议│ │Modbus│...│
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

### 数据流架构

```
设备数据采集流程：
[Device] --协议解析--> [Protocol Driver] --标准化数据--> [CacheQueue]
                                                          │
                                                          ▼
[MQTT Broker] <--发布数据-- [Sender] <--重试机制-- [CacheQueue]
                 (在线时)      (离线时)

配置下发流程：
[Web UI] --API--> [API Server] --设备配置--> [Protocol Manager]
                                              │
                                              ▼
                                      [协议驱动] --协议命令--> [Device]
```

---

## 项目结构

```
edge5/
├── cmd/                     # 程序入口
│   └── server/
│       └── main.go
├── config/                  # 配置相关
│   ├── config.go            # 配置加载
│   └── config.yaml          # 配置文件示例
├── internal/                # 内部包
│   ├── api/                 # API 层
│   │   ├── router/          # 路由注册
│   │   ├── middleware/      # 中间件（认证、日志、权限）
│   │   └── handler/         # 处理器
│   │       ├── sys/         # 系统管理（用户、角色、菜单）
│   │       ├── mqtt/        # MQTT 配置
│   │       └── device/      # 设备管理
│   ├── model/               # 数据模型
│   │   ├── sys/             # 系统模型
│   │   ├── mqtt.go          # MQTT 模型
│   │   └── device.go        # 设备模型
│   ├── repository/          # 数据访问层
│   ├── service/             # 业务逻辑层
│   ├── pkg/                 # 核心包
│   │   ├── connector/       # 连接管理池
│   │   ├── mqtt/            # MQTT 客户端
│   │   ├── cache/           # 缓存队列 (BoltDB)
│   │   ├── plugin/          # 插件系统
│   │   └── protocol/        # 协议驱动
│   │       └── builtin/     # 内置协议（MC、Modbus等）
│   └── utils/               # 工具函数
├── global/                  # 全局对象
├── gopool/                  # 协程池
├── web/                     # 前端项目
├── go.mod
├── go.sum
├── SPEC.md                  # 详细架构设计文档
└── README.md                # 项目说明文档
```

---

## 快速开始

### 1. 克隆项目

```bash
git clone <repository-url>
cd edge5
```

### 2. 安装依赖

```bash
# 后端依赖
go mod tidy

# 前端依赖
cd web
npm install
```

### 3. 配置

编辑 `config/config.yaml` 文件：

```yaml
server:
  host: "0.0.0.0"
  port: 8080
  mode: "debug"

database:
  type: "sqlite3"
  sqlite3:
    path: "./data/edge5.db"

mqtt:
  enabled: true
  broker: "tcp://127.0.0.1:1883"
  port: 1883
  gateway_sn: "GWD001"

jwt:
  secret: "your-secret-key-change-in-production"
```

### 4. 启动后端

```bash
go run cmd/server/main.go
```

后端服务将在 http://localhost:8080 启动。

### 5. 启动前端

```bash
cd web
npm run dev
```

前端开发服务器将在 http://localhost:3000 启动。

### 6. 登录

默认管理员账号：
- 用户名: `admin`
- 密码: `admin123`

---

## 核心功能

### 1. 协议驱动系统

- 内置工业协议驱动，无需额外插件
- 支持三菱 MC 协议（Q系列PLC）
- 即将支持 Modbus、三菱 CNC 协议
- 统一数据格式，屏蔽协议差异

### 2. 连接管理池

- 统一的设备连接管理
- 自动重连机制（指数退避）
- 15秒定时检查
- 状态监控

### 3. MQTT 客户端

- 与连接管理池集成
- 支持配置化
- 订阅和发布
- 自动重连

### 4. 消息缓存

- MQTT 离线时持久化消息到 BoltDB
- 自动重试发送
- 发送层不感知缓存逻辑

### 5. 设备管理

- 支持 PLC 和 CNC 设备
- TCP 和串口协议
- 通用配置 + 品牌特定配置
- 设备状态实时监控

### 6. 采集任务

- 程序启动后自动检查并启动任务
- 每个任务独立协程运行
- 采集数据通过 MQTT 上报
- MQTT 未连接时自动缓存数据
- 支持手动启停任务

---

## API 文档

### 认证

```
POST /api/v1/login          # 登录
GET  /api/v1/captcha        # 获取验证码
GET  /api/v1/user/info      # 获取用户信息
```

### 系统管理

```
GET    /api/v1/user/list     # 用户列表
POST   /api/v1/user          # 创建用户
PUT    /api/v1/user/:id      # 更新用户
DELETE /api/v1/user/:id      # 删除用户

GET    /api/v1/menu/tree     # 菜单树
GET    /api/v1/menu/list     # 菜单列表
```

### MQTT

```
GET  /api/v1/mqtt/config     # 获取配置
PUT  /api/v1/mqtt/config     # 更新配置
POST /api/v1/mqtt/connect    # 连接
POST /api/v1/mqtt/disconnect # 断开
GET  /api/v1/mqtt/status     # 状态
POST /api/v1/mqtt/test       # 测试连接
```

### 设备

```
GET    /api/v1/device/list    # 设备列表
GET    /api/v1/device/:id     # 设备详情
POST   /api/v1/device         # 创建设备
PUT    /api/v1/device/:id     # 更新设备
DELETE /api/v1/device/:id     # 删除设备
POST   /api/v1/device/:id/start # 启动设备
POST   /api/v1/device/:id/stop  # 停止设备
GET    /api/v1/device/:id/status # 设备状态
```

### 采集任务

```
GET    /api/v1/task/list      # 任务列表
GET    /api/v1/task/:id       # 任务详情
POST   /api/v1/task           # 创建任务
PUT    /api/v1/task/:id       # 更新任务
DELETE /api/v1/task/:id       # 删除任务
POST   /api/v1/task/:id/start # 启动任务
POST   /api/v1/task/:id/stop  # 停止任务
GET    /api/v1/task/:id/cache # 任务缓存数据
```

---

## MQTT 主题规范

遵循 aixot 平台规范：

```
网关注册:    /aixot/up/gateway/register
网关心跳:    /aixot/up/{gatewaySn}/heartbeat
网关状态:    /aixot/up/{gatewaySn}/properties
设备数据:    /aixot/up/{gatewaySn}/{deviceSn}/data
命令下发:    /aixot/down/{gatewaySn}/command
命令响应:    /aixot/up/{gatewaySn}/command/reply
```

---

## 配置说明

### 日志配置

```yaml
log:
  level: "info"        # debug/info/warn/error
  path: "./logs"
  pattern: "%Y%m%d.log"
  max_age: 7          # 文件最大保存天数
  rotation_time: 24   # 切割时间间隔(小时)
  compress: true      # 是否压缩
```

### 连接池配置

```yaml
connector:
  reconnect_interval: 15000  # 重连检查间隔(ms)
  base_delay: 1000          # 初始延迟(ms)
  max_delay: 60000          # 最大延迟(ms)
  factor: 2                 # 退避因子
```

---

## 数据库

### SQLite3

默认使用 SQLite3，数据库文件位于 `./data/edge5.db`。

### 缓存数据库 (BoltDB)

BoltDB 用于 MQTT 离线消息的持久化存储，数据库文件位于 `./data/cache.db`。

---

## 优雅退出

程序支持优雅退出，按顺序执行清理任务：

1. 关闭 HTTP 服务
2. 断开 MQTT 连接
3. 停止设备采集
4. 关闭设备连接
5. 关闭插件连接
6. 保存缓存队列
7. 关闭 BoltDB
8. 关闭 SQL 数据库
9. 刷新日志缓冲区
10. 关闭日志

---

## 开发指南

### 代码规范

- 使用 Go 语言官方代码规范
- 使用 `gofmt` 格式化代码
- 遵循 Clean Architecture 原则

### 测试

```bash
go test ./...
```

### 构建

```bash
# Linux
GOOS=linux GOARCH=amd64 go build -o edge5-server cmd/server/main.go

# Windows
go build -o edge5-server.exe cmd/server/main.go
```

### 添加新协议驱动

1. 在 `internal/pkg/protocol/builtin/` 下创建新协议实现
2. 实现协议接口（Connect、Read、Write 等）
3. 在协议注册表中注册新协议
4. 在前端设备管理中添加协议选项

---

## 版本规划

### v1.0.0 (当前)

- 基础框架搭建
- 用户认证和权限
- MQTT 配置和管理
- 设备管理（CRUD）
- 连接管理池
- 消息缓存队列
- 内置三菱 MC 协议

### v1.1.0

- 内置 Modbus 协议支持
- 设备数据可视化
- 采集任务管理优化

### v1.2.0

- 内置三菱 CNC 协议支持
- 远程配置下发
- 数据采集历史查询

### v2.0.0

- 插件市场
- 多协议并行采集
- 边缘计算能力

---

## 参考文档

- [Gin Web Framework](https://gin-gonic.com/)
- [Vue 3 Documentation](https://vuejs.org/)
- [Element Plus](https://element-plus.org/)
- [BoltDB](https://github.com/etcd-io/bbolt)
- [paho.mqtt.golang](https://github.com/eclipse/paho.mqtt.golang)
- [gRPC](https://grpc.io/)
- [Zap Logger](https://github.com/uber-go/zap)

---

## 许可证

MIT License
