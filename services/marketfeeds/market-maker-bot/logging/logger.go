package logging

import (
	"go.uber.org/zap"
)

var Logger *zap.Logger

func InitLogger() error {
	var err error
	Logger, err = zap.NewProduction()
	return err
}

func Info(msg string, fields ...zap.Field) {
	Logger.Info(msg, fields...)
}

func Warn(msg string, fields ...zap.Field) {
	Logger.Warn(msg, fields...)
}

func Error(msg string, fields ...zap.Field) {
	Logger.Error(msg, fields...)
}
