package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"edge5/internal/model"
	"edge5/plugins/proto"

	grpc "google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"edge5/internal/repository"
)

type devicePluginEntry struct {
	deviceID uint64
	deviceSn string
	conn     *grpc.ClientConn
	client   proto.DevicePluginClient

	cancel context.CancelFunc
	done   chan struct{}
}

type devicePluginRuntime struct {
	mu      sync.Mutex
	entries map[uint64]*devicePluginEntry
}

var deviceRuntime = &devicePluginRuntime{
	entries: make(map[uint64]*devicePluginEntry),
}

func (r *devicePluginRuntime) Start(device *model.Device, deviceStatusRepo *repository.DeviceStatusRepository) error {
	if device == nil {
		return errors.New("device nil")
	}

	r.mu.Lock()
	if _, ok := r.entries[device.ID]; ok {
		r.mu.Unlock()
		return nil
	}
	r.mu.Unlock()

	host, port, runtimeParams, intervalMs, err := parseDevicePluginRuntime(device)
	if err != nil {
		return err
	}

	addr := fmt.Sprintf("%s:%d", host, port)

	ctx, cancel := context.WithCancel(context.Background())
	conn, err := grpc.DialContext(ctx, addr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		cancel()
		return fmt.Errorf("dial plugin %s failed: %w", addr, err)
	}

	client := proto.NewDevicePluginClient(conn)

	// Connect
	connectResp, err := client.Connect(ctx, &proto.ConnectRequest{
		DeviceSn: device.DeviceSn,
		Protocol: device.Protocol,
		Params:   runtimeParams,
	})
	if err != nil || !connectResp.GetSuccess() {
		cancel()
		_ = conn.Close()
		if err != nil {
			return fmt.Errorf("plugin connect error: %w", err)
		}
		return fmt.Errorf("plugin connect failed: %s", connectResp.GetMessage())
	}

	done := make(chan struct{})
	entry := &devicePluginEntry{
		deviceID: device.ID,
		deviceSn: device.DeviceSn,
		conn:     conn,
		client:   client,
		cancel:   cancel,
		done:     done,
	}

	r.mu.Lock()
	r.entries[device.ID] = entry
	r.mu.Unlock()

	// SubscribeData loop: update device_status.online/last_heartbeat/message
	go func() {
		defer close(done)

		defer func() {
			// ensure offline on exit
			_ = deviceStatusRepo.UpsertByDeviceID(device.ID, false, time.Now(), "offline")
			r.mu.Lock()
			delete(r.entries, device.ID)
			r.mu.Unlock()
		}()

		subCtx, subCancel := context.WithCancel(ctx)
		defer subCancel()

		stream, err := client.SubscribeData(subCtx, &proto.SubscribeRequest{
			DeviceSn:  device.DeviceSn,
			Addresses: []string{},
			Interval:  int32(intervalMs / 1000),
		})
		if err != nil {
			_ = deviceStatusRepo.UpsertByDeviceID(device.ID, false, time.Now(), fmt.Sprintf("subscribe error: %v", err))
			return
		}

		for {
			resp, err := stream.Recv()
			if err != nil {
				// stream closed
				return
			}

			heartbeat := time.Now()
			if resp.GetTimestamp() > 0 {
				heartbeat = time.UnixMilli(resp.GetTimestamp())
			}

			msg := ""
			if v := resp.GetValues(); v != nil {
				msg = fmt.Sprintf("values=%d", len(v))
			}

			_ = deviceStatusRepo.UpsertByDeviceID(device.ID, true, heartbeat, msg)
		}
	}()

	_ = deviceStatusRepo.UpsertByDeviceID(device.ID, true, time.Now(), "connecting")
	return nil
}

func (r *devicePluginRuntime) Stop(deviceID uint64) error {
	r.mu.Lock()
	entry, ok := r.entries[deviceID]
	if !ok {
		r.mu.Unlock()
		return nil
	}
	delete(r.entries, deviceID)
	r.mu.Unlock()

	if entry.cancel != nil {
		entry.cancel()
	}

	// best-effort Disconnect
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, _ = entry.client.Disconnect(ctx, &proto.DisconnectRequest{DeviceSn: entry.deviceSn})

	if entry.conn != nil {
		_ = entry.conn.Close()
	}
	<-entry.done
	return nil
}

// 解析 device.Config 获取插件拨号所需 host/port、params、采集 interval
func parseDevicePluginRuntime(device *model.Device) (host string, port int, params map[string]string, intervalMs int, err error) {
	// defaults
	params = make(map[string]string)
	intervalMs = 1000

	var raw any
	if len(device.Config) == 0 {
		return "", 0, nil, 0, errors.New("device.config empty")
	}

	if err := json.Unmarshal(device.Config, &raw); err != nil {
		return "", 0, nil, 0, fmt.Errorf("unmarshal device.config failed: %w", err)
	}

	root, ok := raw.(map[string]any)
	if !ok {
		return "", 0, nil, 0, errors.New("device.config not object")
	}

	runtimeObj, _ := root["runtime"].(map[string]any)
	collectionObj, _ := root["collection"].(map[string]any)

	// intervalMs
	if v, ok := collectionObj["intervalMs"]; ok {
		switch x := v.(type) {
		case float64:
			intervalMs = int(x)
		case int:
			intervalMs = x
		case string:
			if n, e := strconv.Atoi(x); e == nil {
				intervalMs = n
			}
		}
	}

	// runtime.extra.host/port
	extraObj, _ := runtimeObj["extra"].(map[string]any)
	host, _ = extraObj["host"].(string)
	switch p := extraObj["port"].(type) {
	case float64:
		port = int(p)
	case int:
		port = p
	case string:
		if n, e := strconv.Atoi(p); e == nil {
			port = n
		}
	}

	if host == "" || port == 0 {
		return "", 0, nil, 0, errors.New("runtime.extra.host/port missing in device.config")
	}

	// runtime params: pass ip/port/serial_port/baud_rate to plugin
	for k, v := range runtimeObj {
		if k == "extra" {
			continue
		}
		switch vv := v.(type) {
		case string:
			params[k] = vv
		case float64:
			// avoid "1.0"
			if vv == float64(int64(vv)) {
				params[k] = strconv.FormatInt(int64(vv), 10)
			} else {
				params[k] = strconv.FormatFloat(vv, 'f', -1, 64)
			}
		case int:
			params[k] = strconv.Itoa(vv)
		case bool:
			params[k] = strconv.FormatBool(vv)
		default:
			// ignore complex objects
		}
	}

	if len(params) == 0 {
		// allow empty params, but still okay
		// params = map[string]string{}
	}

	return host, port, params, intervalMs, nil
}
