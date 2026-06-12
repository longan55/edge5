package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"edge5/global"
	"edge5/internal/model"
	"edge5/internal/pkg/protocol"
	"edge5/internal/utils/response"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type DeviceDebugHandler struct {
	logger *zap.Logger
}

func NewDeviceDebugHandler(logger *zap.Logger) *DeviceDebugHandler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &DeviceDebugHandler{logger: logger}
}

// DebugReadRequest 调试读取请求
type DebugReadRequest struct {
	Params []DebugReadParam `json:"params" binding:"required"`
}

type DebugReadParam struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	Length  int    `json:"length"`
	Type    string `json:"parseType"`
}

// DebugWriteRequest 调试写入请求
type DebugWriteRequest struct {
	Params []DebugWriteParam `json:"params" binding:"required"`
}

type DebugWriteParam struct {
	Name       string      `json:"name"`
	Address    string      `json:"address"`
	Length     int         `json:"length"`
	Type       string      `json:"type"`
	WriteValue interface{} `json:"writeValue"`
}

// DebugRead 调试读取
func (h *DeviceDebugHandler) DebugRead(c *gin.Context) {
	deviceIDStr := c.Param("id")
	deviceID, err := strconv.ParseUint(deviceIDStr, 10, 64)
	if err != nil {
		response.Error(c, response.CodeInvalidParam, "无效的设备 ID")
		return
	}

	var req DebugReadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, response.CodeInvalidParam, "参数错误: "+err.Error())
		return
	}

	if len(req.Params) == 0 {
		response.Error(c, response.CodeInvalidParam, "请至少添加一个采集参数")
		return
	}

	result, err := h.doRead(deviceID, req.Params)
	if err != nil {
		response.Error(c, response.CodeError, "读取失败: "+err.Error())
		return
	}

	response.Success(c, gin.H{
		"results": result,
	})
}

// DebugWrite 调试写入
func (h *DeviceDebugHandler) DebugWrite(c *gin.Context) {
	deviceIDStr := c.Param("id")
	deviceID, err := strconv.ParseUint(deviceIDStr, 10, 64)
	if err != nil {
		response.Error(c, response.CodeInvalidParam, "无效的设备 ID")
		return
	}

	var req DebugWriteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, response.CodeInvalidParam, "参数错误: "+err.Error())
		return
	}

	if len(req.Params) == 0 {
		response.Error(c, response.CodeInvalidParam, "请至少添加一个写入参数")
		return
	}

	err = h.doWrite(deviceID, req.Params)
	if err != nil {
		response.Error(c, response.CodeError, "写入失败: "+err.Error())
		return
	}

	response.Success(c, gin.H{
		"message": "写入成功",
	})
}

// GetDeviceDebugInfo 获取设备调试信息（是否支持调试、协议采集参数 schema）
func (h *DeviceDebugHandler) GetDeviceDebugInfo(c *gin.Context) {
	deviceIDStr := c.Param("id")
	deviceID, err := strconv.ParseUint(deviceIDStr, 10, 64)
	if err != nil {
		response.Error(c, response.CodeInvalidParam, "无效的设备 ID")
		return
	}

	var device model.Device
	if err := global.DB.First(&device, deviceID).Error; err != nil {
		response.Error(c, response.CodeNotFound, "设备不存在")
		return
	}

	protocolName := device.Protocol
	if protocolName == "" {
		response.Error(c, response.CodeNotFound, "设备未绑定协议")
		return
	}

	h.logger.Info("GetDeviceDebugInfo",
		zap.Uint64("deviceID", deviceID),
		zap.String("protocolName", protocolName),
		zap.String("deviceProtocolField", device.Protocol))

	reg := protocol.DefaultRegistry()
	infos := reg.List()

	h.logger.Info("注册表中的协议数量", zap.Int("count", len(infos)))
	for i, info := range infos {
		infoName := protocol.GetInfoString(info, "name")
		infoAlias := protocol.GetInfoAlias(info)
		infoSupportDebug := protocol.GetInfoBool(info, "supportDebug", false)
		h.logger.Info("协议信息",
			zap.Int("index", i),
			zap.String("name", infoName),
			zap.String("alias", infoAlias),
			zap.Bool("supportDebug", infoSupportDebug),
			zap.Bool("matchByName", infoName == protocolName),
			zap.Bool("matchByAlias", infoAlias == protocolName))
	}

	var supportDebug bool
	var schema []protocol.ReadParamSchema
	var foundProtocolName string

	for _, info := range infos {
		// 使用 MatchProtocolName 支持别名匹配
		if protocol.MatchProtocolName(info, protocolName) {
			foundProtocolName = protocol.GetInfoString(info, "name")
			schema = protocol.ExtractReadParamsSchema(info)
			// 使用协议定义中的 supportDebug 字段
			supportDebug = protocol.GetInfoBool(info, "supportDebug", false)
			h.logger.Info("找到匹配的协议",
				zap.String("foundName", foundProtocolName),
				zap.String("alias", protocol.GetInfoAlias(info)),
				zap.Bool("supportDebug", supportDebug),
				zap.Any("schema", schema))
			break
		}
	}

	if schema == nil {
		schema = []protocol.ReadParamSchema{
			{Name: "address", CName: "地址", Type: "string"},
			{Name: "length", CName: "长度", Type: "int"},
			{Name: "type", CName: "解析类型", Type: "select", Choices: []string{"bool", "short", "ushort", "int", "uint", "long", "ulong", "float", "double", "string"}},
		}
	}

	h.logger.Info("返回调试信息",
		zap.Uint64("deviceID", deviceID),
		zap.String("protocol", protocolName),
		zap.Bool("supportDebug", supportDebug),
		zap.Int("schemaCount", len(schema)))

	response.Success(c, gin.H{
		"deviceId":         deviceID,
		"protocol":         protocolName,
		"supportDebug":     supportDebug,
		"readParamsSchema": schema,
	})
}

func (h *DeviceDebugHandler) doRead(deviceID uint64, params []DebugReadParam) ([]map[string]interface{}, error) {
	// 获取设备信息
	var device model.Device
	if err := global.DB.First(&device, deviceID).Error; err != nil {
		return nil, fmt.Errorf("设备不存在")
	}

	// 获取协议实例
	reg := protocol.DefaultRegistry()
	proto, ok := reg.Get(device.Protocol)
	if !ok {
		return nil, fmt.Errorf("协议 %s 未注册", device.Protocol)
	}

	// 解析设备配置参数用于连接
	connParams, err := parseDeviceConfigToMetadata(&device)
	if err != nil {
		return nil, fmt.Errorf("解析设备配置失败: %w", err)
	}
	connParams["deviceID"] = float64(device.ID)

	// 连接设备
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	handle, err := proto.Connect(ctx, connParams)
	if err != nil {
		return nil, fmt.Errorf("连接设备失败: %w", err)
	}
	defer func() {
		_ = proto.Disconnect(context.Background(), handle)
	}()

	// 构建读取请求
	points := make([]protocol.Point, 0, len(params))
	for _, p := range params {
		count := p.Length
		if count <= 0 {
			count = 1
		}
		points = append(points, protocol.Point{
			Name:     p.Name,
			Resource: p.Address,
			DataType: p.Type,
			Count:    count,
		})
	}

	readReq := protocol.BatchReadRequest{Points: points}
	resp, err := proto.ReadBatch(ctx, handle, readReq)
	if err != nil {
		return nil, fmt.Errorf("读取失败: %w", err)
	}

	results := make([]map[string]interface{}, 0, len(resp.Results))
	for _, r := range resp.Results {
		item := map[string]interface{}{
			"name":    r.PointName,
			"value":   r.Value,
			"quality": r.Quality,
		}
		if r.Error != nil {
			item["error"] = r.Error.Error()
		}
		results = append(results, item)
	}

	return results, nil
}

func (h *DeviceDebugHandler) doWrite(deviceID uint64, params []DebugWriteParam) error {
	var device model.Device
	if err := global.DB.First(&device, deviceID).Error; err != nil {
		return fmt.Errorf("设备不存在")
	}

	reg := protocol.DefaultRegistry()
	proto, ok := reg.Get(device.Protocol)
	if !ok {
		return fmt.Errorf("协议 %s 未注册", device.Protocol)
	}

	connParams, err := parseDeviceConfigToMetadata(&device)
	if err != nil {
		return fmt.Errorf("解析设备配置失败: %w", err)
	}
	connParams["deviceID"] = float64(device.ID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	handle, err := proto.Connect(ctx, connParams)
	if err != nil {
		return fmt.Errorf("连接设备失败: %w", err)
	}
	defer func() {
		_ = proto.Disconnect(context.Background(), handle)
	}()

	items := make([]protocol.BatchWriteItem, 0, len(params))
	for _, p := range params {
		count := p.Length
		if count <= 0 {
			count = 1
		}
		items = append(items, protocol.BatchWriteItem{
			Point: protocol.Point{
				Name:     p.Name,
				Resource: p.Address,
				DataType: p.Type,
				Count:    count,
			},
			Value: p.WriteValue,
		})
	}

	writeReq := protocol.BatchWriteRequest{Items: items}
	return proto.WriteBatch(ctx, handle, writeReq)
}

// parseDeviceConfigToMetadata 解析设备配置为协议 Metadata
func parseDeviceConfigToMetadata(device *model.Device) (protocol.Metadata, error) {
	result := make(protocol.Metadata)

	if len(device.Config) == 0 {
		return result, nil
	}

	var configMap map[string]interface{}
	if err := json.Unmarshal(device.Config, &configMap); err != nil {
		return nil, err
	}

	for k, v := range configMap {
		// 跳过插件相关字段
		if k == "pluginHost" || k == "pluginPort" || k == "model" {
			continue
		}
		result[k] = v
	}

	return result, nil
}
