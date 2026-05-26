package global

import (
	"fmt"
	"os"
	"path/filepath"

	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func InitDatabase() error {
	dsn := CONFIG.Database.GetDSN()
	var err error
	var dialector gorm.Dialector

	if CONFIG.Database.Type == "sqlite3" {
		dialector = sqlite.Open(dsn)
	} else {
		dialector = postgres.Open(dsn)
	}

	DB, err = gorm.Open(dialector, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return fmt.Errorf("failed to connect database: %w", err)
	}

	sqlDB, err := DB.DB()
	if err != nil {
		return fmt.Errorf("failed to get database instance: %w", err)
	}

	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(5)

	if CONFIG.Database.Type == "sqlite3" {
		absPath, _ := filepath.Abs(CONFIG.Database.SQLite3.Path)
		dir := filepath.Dir(absPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create database directory: %w", err)
		}
	}

	Logger.Info("数据库连接成功", zap.String("type", CONFIG.Database.Type))

	RegisterQuitTask(func() error {
		sqlDB, err := DB.DB()
		if err != nil {
			return err
		}
		return sqlDB.Close()
	}, "关闭数据库连接", 8)

	return nil
}
