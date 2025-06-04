package marketmaking

import (
	"context"
)

// Service defines the consolidated market making and data service interface
type Service interface {
	// Market data operations
	GetMarketData(ctx context.Context, market string, interval string) (interface{}, error)
	SubscribeMarketData(ctx context.Context, market string, callback func(interface{})) error
	UnsubscribeMarketData(ctx context.Context, market string) error

	// Market feeds operations
	AddFeed(ctx context.Context, name, source, market string, config map[string]interface{}) (string, error)
	GetFeed(ctx context.Context, feedID string) (interface{}, error)
	GetFeeds(ctx context.Context) ([]interface{}, error)
	UpdateFeed(ctx context.Context, feedID string, config map[string]interface{}) error
	DeleteFeed(ctx context.Context, feedID string) error
	StartFeed(ctx context.Context, feedID string) error
	StopFeed(ctx context.Context, feedID string) error

	// Analytics operations
	GetMarketMetrics(ctx context.Context, market string) (interface{}, error)
	GetVolumeAnalytics(ctx context.Context, market string, period string) (interface{}, error)
	GetTradingStatistics(ctx context.Context, userID string) (interface{}, error)

	// Service lifecycle
	Start() error
	Stop() error
}

// NewService creates a new consolidated market making service
// func NewService(logger *zap.Logger, db *gorm.DB, pubsub PubSubInterface) (Service, error)
