package global

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
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

func (my *myMqttClient) Connect() error {
	if my.init {
		if my.connected {
			return nil
		} else {
			return errors.New("mqtt client not connected")
		}
	}
	opts := mqtt.NewClientOptions()

	broker := buildBrokerAddress()
	opts.AddBroker(broker)

	clientID := CONFIG.MQTT.ClientID
	if clientID == "" {
		clientID = CONFIG.Gateway.SN + "-edge5"
	}
	opts.SetClientID(clientID)

	opts.SetUsername(CONFIG.MQTT.Username)
	opts.SetPassword(CONFIG.MQTT.Password)

	if CONFIG.MQTT.Version == "5.0" {
		opts.SetProtocolVersion(5)
	} else {
		opts.SetProtocolVersion(4)
	}

	if CONFIG.MQTT.ConnectTimeout > 0 {
		opts.SetConnectTimeout(time.Duration(CONFIG.MQTT.ConnectTimeout) * time.Second)
	} else {
		opts.SetConnectTimeout(10 * time.Second)
	}

	opts.SetCleanSession(CONFIG.MQTT.CleanStart)

	keepAliveSec := CONFIG.MQTT.KeepAlive
	if keepAliveSec <= 0 {
		keepAliveSec = 60
	}
	opts.SetKeepAlive(time.Duration(keepAliveSec) * time.Second)

	opts.SetPingTimeout(10 * time.Second)

	opts.SetAutoReconnect(CONFIG.MQTT.AutoReconnect)
	opts.SetConnectRetry(CONFIG.MQTT.AutoReconnect)
	if CONFIG.MQTT.ReconnectPeriod > 0 {
		opts.SetConnectRetryInterval(time.Duration(CONFIG.MQTT.ReconnectPeriod) * time.Millisecond)
	} else {
		opts.SetConnectRetryInterval(5 * time.Second)
	}

	if CONFIG.MQTT.SSL {
		tlsConfig, err := buildTLSConfig()
		if err != nil {
			return fmt.Errorf("build TLS config failed: %w", err)
		}
		opts.SetTLSConfig(tlsConfig)
	}

	opts.SetConnectionLostHandler(func(client mqtt.Client, err error) {
		my.connected = false
		Logger.Warn("MQTT连接丢失", zap.Error(err))
	})

	opts.SetOnConnectHandler(func(client mqtt.Client) {
		my.connected = true
		Logger.Info("成功连接到MQTT Broker", zap.String("broker", broker))
	})

	my.client = mqtt.NewClient(opts)

	token := my.client.Connect()
	if ok := token.WaitTimeout(3 * time.Second); !ok {
		Logger.Warn("初始连接超时，HTTP 将继续启动（MQTT 依赖自动重连）")
		my.init = true
		return nil
	}
	if token.Error() != nil {
		Logger.Warn("初始连接失败，SDK会自动重连", zap.Error(token.Error()))
	}

	my.init = true
	return token.Error()
}

func buildBrokerAddress() string {
	protocol := CONFIG.MQTT.Protocol
	if protocol == "" {
		protocol = "mqtt://"
	}
	host := CONFIG.MQTT.Host
	if host == "" {
		host = CONFIG.MQTT.Broker
	}
	port := CONFIG.MQTT.Port
	if port <= 0 {
		port = 1883
		if protocol == "mqtts://" {
			port = 8883
		}
	}
	return fmt.Sprintf("%s%s:%d", protocol, host, port)
}

func buildTLSConfig() (*tls.Config, error) {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: !CONFIG.MQTT.SSLVerify,
	}

	if CONFIG.MQTT.ALPNTag != "" {
		tlsConfig.NextProtos = []string{CONFIG.MQTT.ALPNTag}
	}

	if CONFIG.MQTT.CertType == "self_signed" {
		if CONFIG.MQTT.CAFile != "" {
			caCert, err := os.ReadFile(CONFIG.MQTT.CAFile)
			if err != nil {
				return nil, fmt.Errorf("read CA file failed: %w", err)
			}
			caCertPool := x509.NewCertPool()
			caCertPool.AppendCertsFromPEM(caCert)
			tlsConfig.RootCAs = caCertPool
		}

		if CONFIG.MQTT.CertFile != "" && CONFIG.MQTT.KeyFile != "" {
			cert, err := tls.LoadX509KeyPair(CONFIG.MQTT.CertFile, CONFIG.MQTT.KeyFile)
			if err != nil {
				return nil, fmt.Errorf("load cert/key pair failed: %w", err)
			}
			tlsConfig.Certificates = []tls.Certificate{cert}
		}
	}

	return tlsConfig, nil
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
