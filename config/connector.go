package config

type ConnectorConfig struct {
	ReconnectInterval int `mapstructure:"reconnect_interval"`
	BaseDelay         int `mapstructure:"base_delay"`
	MaxDelay          int `mapstructure:"max_delay"`
	Factor            int `mapstructure:"factor"`
}
