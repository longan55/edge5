// Package config 配置文件包
package config

import (
	"log"
	"path/filepath"

	"github.com/spf13/viper"
)

var CONFIG *Config

type Config struct {
	Gateway   GatewayConfig   `mapstructure:"gateway"`
	Server    ServerConfig    `mapstructure:"server"`
	Database  DatabaseConfig  `mapstructure:"database"`
	Cache     CacheConfig     `mapstructure:"cache"`
	MQTT      MQTTConfig      `mapstructure:"mqtt"`
	Log       LogConfig       `mapstructure:"log"`
	Connector ConnectorConfig `mapstructure:"connector"`
	Plugin    PluginConfig    `mapstructure:"plugin"`
	JWT       JWTConfig       `mapstructure:"jwt"`
	Captcha   CaptchaConfig   `mapstructure:"captcha"`
	Serial    SerialConfig    `mapstructure:"serial"`
}

type SerialConfig struct {
	Ports map[string]uint `mapstructure:"ports"`
}

func InitConfig(configPath string) *Config {
	if CONFIG != nil {
		return CONFIG
	}
	if configPath == "" {
		configPath = "config/config.yaml"
	}
	viper.SetConfigFile(configPath)
	viper.SetConfigType("yaml")

	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("failed to read config file: %v", err)
	}

	CONFIG = &Config{}
	if err := viper.Unmarshal(CONFIG); err != nil {
		log.Fatalf("failed to unmarshal config: %v", err)
	}

	setDefaultSerialPorts()

	CONFIG.Log.Path, _ = filepath.Abs(CONFIG.Log.Path)
	log.Printf("配置文件: %+v\n", CONFIG)
	return CONFIG
}

func setDefaultSerialPorts() {
	if CONFIG.Serial.Ports == nil {
		CONFIG.Serial.Ports = map[string]uint{
			"/dev/ttyS0": 0,
			"/dev/ttyS3": 0,
			"/dev/ttyS4": 0,
			"/dev/ttyS5": 0,
			"/dev/ttyS7": 0,
			"/dev/ttyS8": 0,
			"/dev/ttyS9": 0,
		}
	}
}
