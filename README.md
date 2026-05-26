# Edge5 边缘网关框架

一个面向边缘计算场景的网关程序框架，支持工业设备（PLC、CNC 等）的数据采集、协议转换和云端上报。

## 技术栈

- **后端**: Go 1.25.3 + Gin + GORM + BoltDB
- **前端**: Vue 3 + Element Plus + Pinia
- **消息**: MQTT (paho.mqtt.golang)
- **日志**: Zap + RotateLogs
- **数据库**: SQLite3 / PostgreSQL (可选)
- **插件**: gRPC

## 项目结构

```
edge5/
├── cmd/server/          # 程序入口
├── config/              # 配置文件
├── internal/            # 内部包
│   ├── api/             # API层 (路由、处理器、中间件)
│   ├── model/           # 数据模型
│   ├── repository/      # 数据访问层
│   ├── service/         # 业务逻辑层
│   ├── pkg/             # 核心包 (connector, mqtt, cache, plugin)
│   └── utils/           # 工具函数
├── global/              # 全局对象
├── gopool/              # 协程池
├── plugins/             # 插件实现
├── web/                 # 前端项目
└── scripts/             # 脚本
```

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
# 在项目根目录
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

## 核心功能

### 1. 连接管理池

- 统一的设备连接管理
- 自动重连机制（指数退避）
- 15秒定时检查
- 状态监控

### 2. MQTT 客户端

- 与连接管理池集成
- 支持配置化
- 订阅和发布
- 自动重连

### 3. 消息缓存

- MQTT 离线时持久化消息
- BoltDB 存储
- 自动重试发送
- 发送层不感知缓存逻辑

### 4. 插件系统

- 基于 gRPC 的插件架构
- 支持多种设备和协议
- 热插拔（运行时加载/卸载）

### 5. 设备管理

- 支持 PLC 和 CNC 设备
- TCP 和串口协议
- 通用配置 + 品牌特定配置

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
POST   /api/v1/menu          # 创建菜单
PUT    /api/v1/menu/:id      # 更新菜单
DELETE /api/v1/menu/:id      # 删除菜单
```

### MQTT

```
GET  /api/v1/mqtt/config     # 获取配置
PUT  /api/v1/mqtt/config     # 更新配置
POST /api/v1/mqtt/connect    # 连接
POST /api/v1/mqtt/disconnect # 断开
GET  /api/v1/mqtt/status     # 状态
POST /api/v1/mqtt/test      # 测试连接
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

## 数据库

### SQLite3

默认使用 SQLite3，数据库文件位于 `./data/edge5.db`

### PostgreSQL

切换到 PostgreSQL：

```yaml
database:
  type: "postgresql"
  postgresql:
    host: "localhost"
    port: 5432
    user: "edge5"
    password: "password"
    dbname: "edge5"
    sslmode: "disable"
```

## 插件开发

### 1. 定义 proto 文件

在 `plugins/proto/plugin.proto` 中定义设备插件接口。

### 2. 实现插件服务

实现 `DevicePlugin` gRPC 服务。

### 3. 加载插件

通过 Web 界面或 API 加载插件。

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

## 许可证

MIT License
