package config

type CaptchaConfig struct {
	CaptchaLength int `mapstructure:"captcha_length"`
	ExpireTime    int `mapstructure:"expire_time"`
}
