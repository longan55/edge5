package global

import (
	"compress/gzip"
	"edge5/config"
	"fmt"
	"io"
	"os"
	"path"
	"time"

	rotate "github.com/lestrrat-go/file-rotatelogs"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func InitLogger() error {
	if err := os.MkdirAll(config.CONFIG.Log.Path, os.ModeDir|os.ModePerm); err != nil {
		return fmt.Errorf("create log dir [%s] error: %w", config.CONFIG.Log.Path, err)
	}

	writer, err := rotate.New(
		path.Join(config.CONFIG.Log.Path, config.CONFIG.Log.Pattern),
		rotate.WithLinkName(path.Join(config.CONFIG.Log.Path, "latest.log")),
		rotate.WithMaxAge(time.Duration(config.CONFIG.Log.MaxAge)*24*time.Hour),
		rotate.WithRotationTime(time.Duration(config.CONFIG.Log.RotationTime)*time.Hour),
		rotate.WithHandler(rotate.HandlerFunc(compressLog)),
	)
	if err != nil {
		return fmt.Errorf("rotate.New error: %w", err)
	}

	writerErr, err := rotate.New(
		path.Join(config.CONFIG.Log.Path, "error.%Y%m%d.log"),
		rotate.WithMaxAge(time.Duration(config.CONFIG.Log.MaxAge)*24*time.Hour),          //文件最大保存时间
		rotate.WithRotationTime(time.Duration(config.CONFIG.Log.RotationTime)*time.Hour), //日志切割时间间隔
		rotate.WithHandler(rotate.HandlerFunc(compressLog)),                              //注册 日志切割时回调函数-压缩日志
	)
	if err != nil {
		return fmt.Errorf("rotate.New error: %w", err)
	}

	// 创建一个WriteSyncer，可以是os.Stdout、os.Stderr等等
	var ws zapcore.WriteSyncer

	switch config.CONFIG.Log.Level {
	case "debug":
		ws = zapcore.AddSync(io.MultiWriter(writer, os.Stdout))
	default:
		ws = zapcore.AddSync(writer)
	}

	// 配置日志级别
	levelConf := zap.NewAtomicLevel()
	level, err := zapcore.ParseLevel(config.CONFIG.Log.Level)
	if err != nil {
		levelConf.SetLevel(zapcore.InfoLevel)
	} else {
		levelConf.SetLevel(level)
	}

	// 编码器配置
	var encoderConfig zapcore.EncoderConfig
	if config.CONFIG.Server.Mode == "release" {
		encoderConfig = zap.NewProductionEncoderConfig()
	} else {
		encoderConfig = zap.NewDevelopmentEncoderConfig()
	}

	// 设置时间格式
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	// 创建Encoder
	encoder := zapcore.NewConsoleEncoder(encoderConfig)
	// 创建core
	c1 := zapcore.NewCore(encoder, ws, levelConf)
	c2 := zapcore.NewCore(encoder, zapcore.AddSync(writerErr), zap.ErrorLevel)
	core := zapcore.NewTee(c1, c2)

	logger := zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))
	Logger = logger

	Logger.Info("日志记录器创建成功")
	Logger.Info("配置文件", zap.Any("Content", viper.AllSettings()))
	return nil
}

func compressLog(event rotate.Event) {
	if !config.CONFIG.Log.Compress {
		return
	}

	if event.Type() != rotate.FileRotatedEventType {
		return
	}

	fileevent := event.(*rotate.FileRotatedEvent)
	prePath := fileevent.PreviousFile()
	outputFile := prePath + ".gz"

	if prePath == "" {
		return
	}

	inFile, err := os.Open(prePath)
	if err != nil {
		Logger.Error("compress log error: open log file fail", zap.String("FilePath", prePath), zap.Error(err))
		return
	}
	defer inFile.Close()

	outFile, err := os.Create(outputFile)
	if err != nil {
		Logger.Error("compress log error: create compress file fail", zap.String("FilePath", prePath), zap.Error(err))
		return
	}
	defer outFile.Close()

	gzipWriter := gzip.NewWriter(outFile)
	defer gzipWriter.Close()

	buf := make([]byte, 1024*1024)
	for {
		n, err := inFile.Read(buf)
		if err != nil && err != io.EOF {
			return
		}
		if n == 0 {
			break
		}
		gzipWriter.Write(buf[:n])
	}

	os.Remove(prePath)
}
