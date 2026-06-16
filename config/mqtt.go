package config

type MQTTConfig struct {
	Enabled          bool   `mapstructure:"enabled"`
	Broker           string `mapstructure:"broker"`
	Protocol         string `mapstructure:"protocol"`
	Host             string `mapstructure:"host"`
	Port             int    `mapstructure:"port"`
	Username         string `mapstructure:"username"`
	Password         string `mapstructure:"password"`
	ClientID         string `mapstructure:"client_id"`
	KeepAlive        int    `mapstructure:"keep_alive"`
	QoS              byte   `mapstructure:"qos"`
	SSL              bool   `mapstructure:"ssl"`
	SSLVerify        bool   `mapstructure:"ssl_verify"`
	ALPNTag          string `mapstructure:"alpn_tag"`
	CertType         string `mapstructure:"cert_type"`
	CAFile           string `mapstructure:"ca_file"`
	CertFile         string `mapstructure:"cert_file"`
	KeyFile          string `mapstructure:"key_file"`
	Version          string `mapstructure:"version"`
	ConnectTimeout   int    `mapstructure:"connect_timeout"`
	AutoReconnect    bool   `mapstructure:"auto_reconnect"`
	ReconnectPeriod  int    `mapstructure:"reconnect_period"`
	CleanStart       bool   `mapstructure:"clean_start"`
	SessionExpiry    int    `mapstructure:"session_expiry"`
	ReceiveMax       int    `mapstructure:"receive_max"`
	MaxPacketSize    int    `mapstructure:"max_packet_size"`
	TopicAliasMax    int    `mapstructure:"topic_alias_max"`
	RequestResponse  bool   `mapstructure:"request_response"`
	RequestProblem   bool   `mapstructure:"request_problem"`
}