// Package config 配置文件包
package config

import (
	"fmt"
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

func InitConfig(configPath string) error {
	viper.SetConfigFile(configPath)
	viper.SetConfigType("yaml")

	if err := viper.ReadInConfig(); err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	CONFIG = &Config{}
	if err := viper.Unmarshal(CONFIG); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	CONFIG.Log.Path, _ = filepath.Abs(CONFIG.Log.Path)

	return nil
}
