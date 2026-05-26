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
	CONFIG.Log.Path, _ = filepath.Abs(CONFIG.Log.Path)
	log.Printf("配置文件: %+v\n", CONFIG)
	return CONFIG
}
