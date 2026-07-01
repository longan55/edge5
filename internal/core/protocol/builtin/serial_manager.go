package builtin

import (
	"fmt"
	"sync"
)

var SerialManager *serialManager

type serialManager struct {
	lock    sync.Mutex
	serials map[string]uint
}

func InitSerialManager() {
	SerialManager = &serialManager{
		serials: make(map[string]uint),
	}
}

func InitSerialManagerWithPorts(ports map[string]uint) {
	SerialManager = &serialManager{
		serials: make(map[string]uint),
	}
	for port, deviceID := range ports {
		SerialManager.serials[port] = deviceID
	}
}

func (sm *serialManager) Acquire(port string, deviceID uint) error {
	sm.lock.Lock()
	defer sm.lock.Unlock()

	if existingID, ok := sm.serials[port]; ok {
		if existingID != 0 && existingID != deviceID {
			return fmt.Errorf("串口 %s 已被设备 %d 使用", port, existingID)
		}
		sm.serials[port] = deviceID
		return nil
	}

	sm.serials[port] = deviceID
	return nil
}

func (sm *serialManager) Release(port string) {
	sm.lock.Lock()
	defer sm.lock.Unlock()
	sm.serials[port] = 0
}

func (sm *serialManager) IsAvailable(port string) bool {
	sm.lock.Lock()
	defer sm.lock.Unlock()
	deviceID, ok := sm.serials[port]
	return ok && deviceID == 0
}

func (sm *serialManager) GetDeviceID(port string) uint {
	sm.lock.Lock()
	defer sm.lock.Unlock()
	return sm.serials[port]
}

func (sm *serialManager) ListPorts() []string {
	sm.lock.Lock()
	defer sm.lock.Unlock()
	ports := make([]string, 0, len(sm.serials))
	for port := range sm.serials {
		ports = append(ports, port)
	}
	return ports
}
