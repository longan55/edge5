package service

import (
	"math"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
	"go.uber.org/zap"
)

type SystemResource struct {
	CPUUsedPercent  float64 `json:"cpuUsedPercent"`
	MemTotal        uint64  `json:"memTotal"`
	MemUsed         uint64  `json:"memUsed"`
	MemUsedPercent  float64 `json:"memUsedPercent"`
	DiskTotal       uint64  `json:"diskTotal"`
	DiskUsed        uint64  `json:"diskUsed"`
	DiskUsedPercent float64 `json:"diskUsedPercent"`
}

type SystemMonitor struct {
	mu        sync.RWMutex
	resources SystemResource
	logger    *zap.Logger
	ticker    *time.Ticker
	stopChan  chan struct{}
}

var systemMonitor *SystemMonitor

func NewSystemMonitor(logger *zap.Logger) *SystemMonitor {
	if systemMonitor == nil {
		systemMonitor = &SystemMonitor{
			logger:   logger,
			ticker:   time.NewTicker(30 * time.Second),
			stopChan: make(chan struct{}),
		}
		systemMonitor.collect()
		go systemMonitor.run()
	}
	return systemMonitor
}

func (s *SystemMonitor) run() {
	for {
		select {
		case <-s.ticker.C:
			s.collect()
		case <-s.stopChan:
			s.ticker.Stop()
			return
		}
	}
}

func (s *SystemMonitor) collect() {
	resources := SystemResource{}

	cpuPercent, err := cpu.Percent(0, false)
	if err != nil {
		s.logger.Warn("采集CPU使用率失败", zap.Error(err))
	} else if len(cpuPercent) > 0 {
		resources.CPUUsedPercent = math.Round(cpuPercent[0]*100) / 100
	}

	memInfo, err := mem.VirtualMemory()
	if err != nil {
		s.logger.Warn("采集内存信息失败", zap.Error(err))
	} else {
		resources.MemTotal = memInfo.Total
		resources.MemUsed = memInfo.Used
		resources.MemUsedPercent = math.Round(memInfo.UsedPercent*100) / 100
	}

	diskInfo, err := disk.Usage("/")
	if err != nil {
		s.logger.Warn("采集磁盘信息失败", zap.Error(err))
	} else {
		resources.DiskTotal = diskInfo.Total
		resources.DiskUsed = diskInfo.Used
		resources.DiskUsedPercent = math.Round(diskInfo.UsedPercent*100) / 100
	}

	s.mu.Lock()
	s.resources = resources
	s.mu.Unlock()
}

func (s *SystemMonitor) GetResources() SystemResource {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.resources
}

func (s *SystemMonitor) Stop() {
	close(s.stopChan)
}

func GetSystemMonitor() *SystemMonitor {
	return systemMonitor
}
