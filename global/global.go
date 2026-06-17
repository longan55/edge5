// Package global 全局变量包
package global

import (
	"edge5/config"
	"edge5/internal/pkg/cache"
	"edge5/internal/pkg/connector"
	"sync"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// MQTTBusinessReloader MQTT 业务服务热更新接口
type MQTTBusinessReloader interface {
	ReloadConfig() error
}

var (
	CONFIG            = config.InitConfig("")
	DB                *gorm.DB
	Logger            *zap.Logger
	CacheDB           *cache.BoltCache
	MQTTClient        *myMqttClient
	ConnectorMgr      connector.ConnectorManager
	MyProcess         *Process
	quitTasks         []QuitTask
	quitMux           sync.Mutex

	// MQTTBusinessService MQTT 业务服务实例（用于热更新）
	MQTTBusinessService MQTTBusinessReloader

	// GatewayRegistered 网关向平台的注册状态（true=已注册，false=未注册）
	GatewayRegistered bool
	regMux            sync.RWMutex
)

// SetGatewayRegistered 原子设置网关注册状态
func SetGatewayRegistered(v bool) {
	regMux.Lock()
	GatewayRegistered = v
	regMux.Unlock()
}

// GetGatewayRegistered 原子读取网关注册状态
func GetGatewayRegistered() bool {
	regMux.RLock()
	defer regMux.RUnlock()
	return GatewayRegistered
}

type Process struct {
}

type QuitTask struct {
	F       func() error
	Content string
	Order   int
}

func RegisterQuitTask(f func() error, content string, order int) {
	quitMux.Lock()
	defer quitMux.Unlock()
	quitTasks = append(quitTasks, QuitTask{
		F:       f,
		Content: content,
		Order:   order,
	})
}

func GracefullyExit() {
	quitMux.Lock()
	defer quitMux.Unlock()

	for _, task := range quitTasks {
		if err := task.F(); err != nil {
			Logger.Error("退出任务执行失败: "+task.Content, zap.Error(err))
		}
	}
}
