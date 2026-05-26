package global

import (
	"compress/gzip"
	"edge5/config"
	"io"
	"os"

	rotate "github.com/lestrrat-go/file-rotatelogs"
	"go.uber.org/zap"
)

func CompressLog(event rotate.Event) {
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
