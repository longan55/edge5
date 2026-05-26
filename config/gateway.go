package config

type GatewayConfig struct {
	SN   string `mapstructure:"sn"`
	OS   string `mapstructure:"os"`
	Arch string `mapstructure:"arch"`
	SOC  string `mapstructure:"soc"`
}
