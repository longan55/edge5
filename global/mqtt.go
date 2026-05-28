package global

import (
	"errors"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"go.uber.org/zap"
)

type myMqttClient struct {
	init      bool
	connected bool
	client    mqtt.Client
}

func NewMqttClient() *myMqttClient {
	return &myMqttClient{}
}

// TODO: 新增更多高级连接配置
func (my *myMqttClient) Connect() error {
	if my.init {
		if my.connected {
			return nil
		} else {
			return errors.New("mqtt client not connected")
		}
	}
	opts := mqtt.NewClientOptions()
	opts.AddBroker(CONFIG.MQTT.Broker)
	opts.SetClientID(CONFIG.Gateway.SN + "-edge5")
	opts.SetUsername(CONFIG.MQTT.Username)
	opts.SetPassword(CONFIG.MQTT.Password)
	opts.SetCleanSession(false) // 持久会话，离线消息不丢失
	opts.SetAutoReconnect(true)
	opts.SetConnectRetry(true)
	opts.SetConnectRetryInterval(5 * time.Second)
	opts.SetKeepAlive(60 * time.Second)
	opts.SetPingTimeout(10 * time.Second)

	// 可选：配置连接丢失时的行为
	opts.SetConnectionLostHandler(func(client mqtt.Client, err error) {
		my.connected = false
		// 只记录日志，不做其他操作
		Logger.Warn("MQTT连接丢失，SDK会自动重连", zap.Error(err))
	})

	opts.SetOnConnectHandler(func(client mqtt.Client) {
		my.connected = true
		Logger.Info("成功连接到MQTT Broker", zap.String("broker", CONFIG.MQTT.Broker))
	})

	my.client = mqtt.NewClient(opts)

	// 初始连接（只需要做一次），避免阻塞 HTTP 启动
	token := my.client.Connect()
	if ok := token.WaitTimeout(3 * time.Second); !ok {
		Logger.Warn("初始连接超时，HTTP 将继续启动（MQTT 依赖自动重连）")
		my.init = true
		return nil
	}
	if token.Error() != nil {
		// 即使这里连接失败，AutoReconnect=true也会在后台重试
		Logger.Warn("初始连接失败，SDK会自动重连", zap.Error(token.Error()))
	}

	my.init = true
	return token.Error()
}

func (my *myMqttClient) Close() error {
	if !my.init {
		return nil
	}
	if !my.connected {
		return nil
	}
	my.client.Disconnect(1000)
	my.init = false
	my.connected = false
	return nil
}

func (my *myMqttClient) IsConnected() bool {
	return my.connected
}

func (my *myMqttClient) Publish(topic string, qos byte, payload []byte) error {
	if !my.init {
		return errors.New("mqtt client not connected")
	}
	if !my.connected {
		return errors.New("mqtt client not connected")
	}
	return my.client.Publish(topic, qos, false, payload).Error()
}

func (my *myMqttClient) Subscribe(topic string, qos byte, handler mqtt.MessageHandler) error {
	if !my.init {
		return errors.New("mqtt client not connected")
	}
	if !my.connected {
		return errors.New("mqtt client not connected")
	}
	return my.client.Subscribe(topic, qos, handler).Error()
}

func (my *myMqttClient) Unsubscribe(topic string) error {
	if !my.init {
		return errors.New("mqtt client not connected")
	}
	if !my.connected {
		return errors.New("mqtt client not connected")
	}
	return my.client.Unsubscribe(topic).Error()
}
