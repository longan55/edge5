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
	init              bool
	connected         bool
	client            mqtt.Client
	onConnectCallback func()
}

func NewMqttClient() *myMqttClient {
	return &myMqttClient{}
}

func (my *myMqttClient) Connect() error {
	if my.init {
		if my.connected {
			Logger.Debug("MQTT Connect: 已连接，跳过", zap.Bool("connected", my.connected))
			return nil
		}
		Logger.Warn("MQTT Connect: 已初始化但未连接")
		return errors.New("mqtt client not connected")
	}

	broker := buildBrokerAddress()
	Logger.Info("MQTT Connect: 开始连接",
		zap.String("broker", broker),
		zap.String("client_id", CONFIG.MQTT.ClientID),
		zap.String("username", CONFIG.MQTT.Username),
		zap.String("version", CONFIG.MQTT.Version),
		zap.Bool("ssl", CONFIG.MQTT.SSL),
		zap.Bool("auto_reconnect", CONFIG.MQTT.AutoReconnect),
		zap.Int("keep_alive", CONFIG.MQTT.KeepAlive),
		zap.Int("connect_timeout", CONFIG.MQTT.ConnectTimeout),
	)

	opts := mqtt.NewClientOptions()

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
			Logger.Error("MQTT Connect: TLS配置构建失败", zap.Error(err))
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

		if my.onConnectCallback != nil {
			go my.onConnectCallback()
		}
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
	Logger.Info("MQTT Connect: 初始化完成", zap.Bool("connected", my.connected))
	return token.Error()
}

func buildBrokerAddress() string {
	protocol := CONFIG.MQTT.Protocol
	host := CONFIG.MQTT.Host
	port := CONFIG.MQTT.Port

	if protocol == "" {
		protocol = "mqtt://"
	}
	if host == "" {
		host = CONFIG.MQTT.Broker
	}
	if port <= 0 {
		port = 1883
		if protocol == "mqtts://" {
			port = 8883
		}
	}

	address := fmt.Sprintf("%s%s:%d", protocol, host, port)
	Logger.Debug("MQTT buildBrokerAddress",
		zap.String("raw_protocol", CONFIG.MQTT.Protocol),
		zap.String("raw_host", CONFIG.MQTT.Host),
		zap.String("raw_broker", CONFIG.MQTT.Broker),
		zap.Int("raw_port", CONFIG.MQTT.Port),
		zap.String("resolved", address),
	)
	return address
}

func buildTLSConfig() (*tls.Config, error) {
	Logger.Debug("MQTT buildTLSConfig",
		zap.Bool("ssl_verify", CONFIG.MQTT.SSLVerify),
		zap.String("alpn_tag", CONFIG.MQTT.ALPNTag),
		zap.String("cert_type", CONFIG.MQTT.CertType),
		zap.String("ca_file", CONFIG.MQTT.CAFile),
		zap.String("cert_file", CONFIG.MQTT.CertFile),
		zap.String("key_file", CONFIG.MQTT.KeyFile),
	)

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
	Logger.Debug("MQTT Close",
		zap.Bool("init", my.init),
		zap.Bool("connected", my.connected),
	)
	if !my.init {
		return nil
	}
	if !my.connected {
		my.init = false
		return nil
	}
	my.client.Disconnect(1000)
	my.init = false
	my.connected = false
	Logger.Info("MQTT 连接已断开")
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

func (my *myMqttClient) SetOnConnectCallback(callback func()) {
	my.onConnectCallback = callback
}
