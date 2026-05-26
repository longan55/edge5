package config

type MQTTConfig struct {
	Enabled   bool   `mapstructure:"enabled"`
	Broker    string `mapstructure:"broker"`
	Port      int    `mapstructure:"port"`
	Username  string `mapstructure:"username"`
	Password  string `mapstructure:"password"`
	ClientID  string `mapstructure:"client_id"`
	KeepAlive int    `mapstructure:"keep_alive"`
	QoS       byte   `mapstructure:"qos"`
	GatewaySN string `mapstructure:"gateway_sn"`
}