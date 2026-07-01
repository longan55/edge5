// Package report 提供通用上报框架。
//
// 所有设备通过统一的 Reporter 接口上报数据。
// 框架内部处理：
//   - 连接断开/上报失败时自动缓存
//   - 连接恢复后自动重试缓存数据
//   - 默认 MQTT 上报实现
//
// 测试说明
//
// 测试使用 mock sender 模拟 MQTT 连接的连接/断开状态，
// 使用 mock cache 替代 BoltDB 缓存。
//
// 运行测试（factory.go 依赖全局配置，用 testonly tag 排除）：
//
//	go test -tags testonly ./internal/pkg/report/
package report
