package config

import (
	"os"
	"path/filepath"
)

type DatabaseConfig struct {
	SQLite3 SQLite3Config `mapstructure:"sqlite3"`
}

type SQLite3Config struct {
	Path string `mapstructure:"path"`
}

type CacheConfig struct {
	Type   string       `mapstructure:"type"`
	BoltDB BoltDBConfig `mapstructure:"boltdb"`
}

type BoltDBConfig struct {
	Path   string `mapstructure:"path"`
	Bucket string `mapstructure:"bucket"`
}

// 确保数据库目录存在
func (d *DatabaseConfig) EnsureDir() error {
	absPath, _ := filepath.Abs(d.SQLite3.Path)
	dir := filepath.Dir(absPath)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return os.MkdirAll(dir, 0755)
	}
	return nil
}
