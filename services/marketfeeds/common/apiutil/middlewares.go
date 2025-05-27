package apiutil

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func LoggerMiddleware(log *slog.Logger) echo.MiddlewareFunc {
	return middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogStatus:   true,
		LogURI:      true,
		LogError:    true,
		HandleError: true, // forwards error to the global error handler, so it can decide appropriate status code
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			if v.Error == nil {
				log.LogAttrs(context.Background(), slog.LevelInfo, "request",
					slog.String("uri", v.URI),
					slog.Int("status", v.Status),
				)
			} else {
				var level slog.Level
				if v.Status >= http.StatusInternalServerError {
					level = slog.LevelError
				} else {
					level = slog.LevelInfo
				}
				log.LogAttrs(context.Background(), level, "request error",
					slog.String("uri", v.URI),
					slog.Int("status", v.Status),
					slog.String("error", v.Error.Error()),
				)
			}
			return nil
		},
	})
}
