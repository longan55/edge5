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

var (
	CONFIG       *config.Config
	DB           *gorm.DB
	Logger       *zap.Logger
	CacheDB      *cache.BoltCache
	MQTTClient   *myMqttClient
	ConnectorMgr connector.ConnectorManager
	MyProcess    *Process
	quitTasks    []QuitTask
	quitMux      sync.Mutex
)

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
