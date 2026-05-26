package config

type LogConfig struct {
	Level        string `mapstructure:"level"`
	Path         string `mapstructure:"path"`
	Pattern      string `mapstructure:"pattern"`
	MaxAge       int    `mapstructure:"max_age"`
	RotationTime int    `mapstructure:"rotation_time"`
	Compress     bool   `mapstructure:"compress"`
}
