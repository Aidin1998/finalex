package events

import (
	"go.uber.org/zap"
)

// EventBusLogger is a helper for logging event bus metrics and failures
func EventBusLogger(logger *zap.Logger, msg string, fields ...zap.Field) {
	logger.Info(msg, fields...)
}
