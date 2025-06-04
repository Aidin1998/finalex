package infrastructure

import (
	"context"
	"net/http"
)

// Service defines the consolidated infrastructure service interface
type Service interface {
	// Server operations
	StartServer(ctx context.Context, port string) error
	StopServer(ctx context.Context) error
	RegisterHandler(path string, method string, handler http.HandlerFunc) error
	RegisterMiddleware(middleware func(http.Handler) http.Handler) error

	// WebSocket operations
	StartWSServer(ctx context.Context, port string) error
	StopWSServer(ctx context.Context) error
	BroadcastMessage(channel string, message interface{}) error
	RegisterWSHandler(path string, handler func(conn interface{})) error

	// Messaging operations
	PublishEvent(ctx context.Context, topic string, event interface{}) error
	SubscribeToEvents(ctx context.Context, topic string, handler func(event interface{})) error

	// Configuration operations
	LoadConfig(ctx context.Context, configPath string) (map[string]interface{}, error)
	GetConfigValue(key string) (interface{}, error)
	SetConfigValue(key string, value interface{}) error

	// Transaction operations
	BeginTransaction(ctx context.Context) (context.Context, error)
	CommitTransaction(ctx context.Context) error
	RollbackTransaction(ctx context.Context) error

	// Service lifecycle
	Start() error
	Stop() error
}

// NewService creates a new consolidated infrastructure service
// func NewService(logger *zap.Logger, db *gorm.DB, redisClient redis.UniversalClient) (Service, error)
