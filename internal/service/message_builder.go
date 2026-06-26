package service

import (
	"encoding/json"
	"strings"
	"time"

	"edge5/config"
	"edge5/global"
	"edge5/internal/model"
	"edge5/internal/repository"

	"go.uber.org/zap"
)

// MessageBuilder 统一的消息构建器，独立于具体的上报协议
// 负责构建 MQTTGatewayMessage 统一消息格式和主题路径
type MessageBuilder struct {
	gatewaySN      string
	topicCfg       *model.MQTTTopicConfig
	topicTemplates map[string]*model.MQTTTopicTemplate
	logger         *zap.Logger
}

var messageBuilder *MessageBuilder

func NewMessageBuilder(logger *zap.Logger) *MessageBuilder {
	if messageBuilder == nil {
		messageBuilder = &MessageBuilder{
			gatewaySN:      config.CONFIG.Gateway.SN,
			topicTemplates: make(map[string]*model.MQTTTopicTemplate),
			logger:         logger,
		}
		messageBuilder.loadTopicConfigAndTemplates()
	}
	return messageBuilder
}

func GetMessageBuilder() *MessageBuilder {
	return messageBuilder
}

func (mb *MessageBuilder) loadTopicConfigAndTemplates() {
	topicRepo := repository.NewMQTTTopicRepository(global.DB)

	cfg, err := topicRepo.GetConfig(mb.gatewaySN)
	if err != nil || cfg == nil {
		mb.topicCfg = &model.MQTTTopicConfig{
			Prefix:        "/aixot",
			UpKeyword:     "up",
			DownKeyword:   "down",
			ShowDirection: true,
		}
	} else {
		mb.topicCfg = cfg
	}

	templates, err := topicRepo.List()
	if err != nil {
		mb.logger.Warn("加载主题模板失败，使用默认值", zap.Error(err))
		for _, t := range repository.GetDefaultTopics() {
			mb.topicTemplates[t.Key] = t
		}
	} else {
		for _, t := range templates {
			mb.topicTemplates[t.Key] = t
		}
	}

	mb.logger.Info("消息构建器：主题配置加载完成",
		zap.String("prefix", mb.topicCfg.Prefix),
		zap.Int("templates_count", len(mb.topicTemplates)),
	)
}

// BuildGatewayMessage 构建统一的网关消息体（独立于具体的上报协议）
func (mb *MessageBuilder) BuildGatewayMessage(payload interface{}) *model.MQTTGatewayMessage {
	raw, _ := json.Marshal(payload)
	return &model.MQTTGatewayMessage{
		Version:   "1.0",
		GatewaySn: mb.gatewaySN,
		Timestamp: time.Now().UnixMilli(),
		RequestID: model.GenerateRequestID(),
		Payload:   raw,
	}
}

// BuildDeviceDataMessage 构建设备数据上报消息体
// payload 直接存放业务数据平铺，不包含 data 嵌套
func (mb *MessageBuilder) BuildDeviceDataMessage(deviceSn string, deviceType string, brand string, deviceModel string, taskID uint64, payload interface{}) *model.MQTTGatewayMessage {
	raw, _ := json.Marshal(payload)
	return &model.MQTTGatewayMessage{
		Version:    "1.0",
		GatewaySn:  mb.gatewaySN,
		DeviceSn:   deviceSn,
		TaskID:     taskID,
		DeviceType: deviceType,
		Brand:      brand,
		Model:      deviceModel,
		Timestamp:  time.Now().UnixMilli(),
		RequestID:  model.GenerateRequestID(),
		Payload:    raw,
	}
}

// BuildTopic 根据模板 key 构建完整主题路径
func (mb *MessageBuilder) BuildTopic(key string, deviceSn string) string {
	template, ok := mb.topicTemplates[key]
	if !ok {
		mb.logger.Warn("主题模板不存在，使用 key 作为路径", zap.String("key", key))
		return mb.buildTopicFallback(key, deviceSn)
	}

	prefix := mb.topicCfg.Prefix
	if prefix == "" {
		prefix = template.Prefix
		if prefix == "" {
			prefix = "/aixot"
		}
	}

	direction := template.Direction
	if direction == "up" && mb.topicCfg.UpKeyword != "" {
		direction = mb.topicCfg.UpKeyword
	} else if direction == "down" && mb.topicCfg.DownKeyword != "" {
		direction = mb.topicCfg.DownKeyword
	}

	path := template.Path
	path = strings.ReplaceAll(path, "{gatewaySn}", mb.gatewaySN)
	if deviceSn != "" {
		path = strings.ReplaceAll(path, "{deviceSn}", deviceSn)
	}

	return prefix + "/" + direction + "/" + path
}

func (mb *MessageBuilder) buildTopicFallback(key string, deviceSn string) string {
	var direction, path string
	if strings.HasSuffix(key, "_up") {
		direction = mb.topicCfg.UpKeyword
		if direction == "" {
			direction = "up"
		}
		path = strings.TrimSuffix(key, "_up")
	} else if strings.HasSuffix(key, "_down") || strings.HasSuffix(key, "_down_ack") {
		direction = mb.topicCfg.DownKeyword
		if direction == "" {
			direction = "down"
		}
		path = strings.TrimSuffix(key, "_down_ack")
		path = strings.TrimSuffix(path, "_down")
	} else {
		direction = mb.topicCfg.UpKeyword
		path = key
	}

	path = strings.ReplaceAll(path, "{gatewaySn}", mb.gatewaySN)
	if deviceSn != "" {
		path = strings.ReplaceAll(path, "{deviceSn}", deviceSn)
	}

	prefix := mb.topicCfg.Prefix
	if prefix == "" {
		prefix = "/aixot"
	}

	return prefix + "/" + direction + "/" + path
}

// ReloadTopicConfig 热更新主题配置
func (mb *MessageBuilder) ReloadTopicConfig() {
	mb.loadTopicConfigAndTemplates()
}