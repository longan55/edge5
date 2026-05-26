package config

import (
	"fmt"
	"os"
	"path/filepath"
)

type DatabaseConfig struct {
	Type       string           `mapstructure:"type"`
	SQLite3    SQLite3Config    `mapstructure:"sqlite3"`
	PostgreSQL PostgreSQLConfig `mapstructure:"postgresql"`
}

func (d *DatabaseConfig) GetDSN() string {
	if d.Type == "sqlite3" {
		absPath, _ := filepath.Abs(d.SQLite3.Path)
		// 检查目录是否存在，不存在则创建
		dir := filepath.Dir(absPath)
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			os.MkdirAll(dir, 0755)
		}
		return absPath
	}
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		d.PostgreSQL.Host,
		d.PostgreSQL.Port,
		d.PostgreSQL.User,
		d.PostgreSQL.Password,
		d.PostgreSQL.DBName,
		d.PostgreSQL.SSLMode,
	)
}

type SQLite3Config struct {
	Path string `mapstructure:"path"`
}

type PostgreSQLConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"dbname"`
	SSLMode  string `mapstructure:"sslmode"`
}

type CacheConfig struct {
	Type   string       `mapstructure:"type"`
	BoltDB BoltDBConfig `mapstructure:"boltdb"`
}

type BoltDBConfig struct {
	Path   string `mapstructure:"path"`
	Bucket string `mapstructure:"bucket"`
}
