package database

import (
	"context"
	"time"
)

// Service defines the consolidated database and caching service interface
type Service interface {
	// Database operations
	ExecuteQuery(ctx context.Context, query string, args ...interface{}) (interface{}, error)
	ExecuteTransaction(ctx context.Context, txFunc func(tx interface{}) error) error
	GetDBStats(ctx context.Context) (map[string]interface{}, error)
	OptimizeQueries(ctx context.Context) (map[string]interface{}, error)

	// Cache operations
	GetFromCache(ctx context.Context, key string) (interface{}, error)
	SetInCache(ctx context.Context, key string, value interface{}, expiration time.Duration) error
	DeleteFromCache(ctx context.Context, key string) error
	FlushCache(ctx context.Context) error

	// Redis operations
	Publish(ctx context.Context, channel string, message interface{}) error
	Subscribe(ctx context.Context, channel string, callback func([]byte)) error

	// Performance operations
	GetPerformanceMetrics(ctx context.Context) (map[string]interface{}, error)
	OptimizeCacheHitRatio(ctx context.Context) error

	// Service lifecycle
	Start() error
	Stop() error
}

// NewService creates a new consolidated database service
// func NewService(logger *zap.Logger, db *gorm.DB, redisClient redis.UniversalClient) (Service, error)
