package apiutil

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

func LoggerMiddleware(log *slog.Logger) gin.HandlerFunc {
	return gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		var level slog.Level
		if param.StatusCode >= http.StatusInternalServerError {
			level = slog.LevelError
		} else {
			level = slog.LevelInfo
		}

		if param.ErrorMessage != "" {
			log.LogAttrs(context.Background(), level, "request error",
				slog.String("uri", param.Path),
				slog.Int("status", param.StatusCode),
				slog.String("method", param.Method),
				slog.String("error", param.ErrorMessage),
				slog.Duration("latency", param.Latency),
			)
		} else {
			log.LogAttrs(context.Background(), slog.LevelInfo, "request",
				slog.String("uri", param.Path),
				slog.Int("status", param.StatusCode),
				slog.String("method", param.Method),
				slog.Duration("latency", param.Latency),
			)
		}
		return ""
	})
}
