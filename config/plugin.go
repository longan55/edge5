package config

type PluginConfig struct {
	Enabled    bool   `mapstructure:"enabled"`
	GRPCPort   int    `mapstructure:"grpc_port"`
	PluginsDir string `mapstructure:"plugins_dir"`
}
