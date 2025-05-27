package logger

import (
	"log/slog"

	"go.uber.org/zap"
	"go.uber.org/zap/exp/zapslog"
	"go.uber.org/zap/zapcore"
)

func New(isProd bool) (*slog.Logger, func() error) {
	var zapLogger *zap.Logger

	if isProd {
		zapLogger = zap.Must(zap.NewProduction())
	} else {
		config := zap.NewDevelopmentConfig()
		config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		zapLogger = zap.Must(config.Build())
	}

	return slog.New(zapslog.NewHandler(zapLogger.Core())), zapLogger.Sync
}
