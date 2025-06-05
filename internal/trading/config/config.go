package config

import (
	"fmt"
	"time"

	"github.com/Aidin1998/finalex/internal/trading/engine"
	"github.com/Aidin1998/finalex/internal/trading/eventjournal"
	"github.com/shopspring/decimal"
)

// TradingConfig holds configuration for the trading system
type TradingConfig struct {
	Engine       *engine.Config
	EventJournal eventjournal.Config
}

// DefaultTradingConfig returns a default trading configuration
func DefaultTradingConfig() *TradingConfig {
	return &TradingConfig{
		Engine: &engine.Config{
			Engine: engine.EngineConfig{
				WorkerPoolSize:            4,
				WorkerPoolQueueSize:       10000,
				ResourcePollInterval:      100 * time.Millisecond,
				OrderBookSnapshotInterval: 1 * time.Second,
				WorkerPoolMonitorEnabled:  true,
				EnableBinarySerialization: false,
				EnableGCControl:           true,
				GCPercent:                 100,
				GCInterval:                10 * time.Second,
				EnableProfiling:           false,
				Risk:                      engine.RiskConfig{},
				RateLimiter:               engine.RateLimiterConfig{},
				Recovery: engine.RecoveryConfig{
					Enabled: true,
				},
			},
			Fees: engine.FeeScheduleConfig{},
			AuditLog: struct {
				Enabled      bool
				FilePath     string
				MaxSizeBytes int64
				MaxBackups   int
				MaxAgeDays   int
			}{
				Enabled:      true,
				FilePath:     "/var/log/trading/audit.log",
				MaxSizeBytes: 100 * 1024 * 1024, // 100MB
				MaxBackups:   10,
				MaxAgeDays:   30,
			},
			Kafka: struct {
				Enabled bool
				Topics  struct {
					OrderUpdates     string
					Trades           string
					MarketDataPrefix string
				}
			}{
				Enabled: false,
				Topics: struct {
					OrderUpdates     string
					Trades           string
					MarketDataPrefix string
				}{
					OrderUpdates:     "trading.order.updates",
					Trades:           "trading.trades",
					MarketDataPrefix: "market.data",
				},
			},
			Pairs: []engine.PairConfig{
				{
					Symbol:               "BTC/USDT",
					MinPriceIncrement:    decimal.NewFromFloat(0.01),
					MinQuantityIncrement: decimal.NewFromFloat(0.00001),
				},
				{
					Symbol:               "ETH/USDT",
					MinPriceIncrement:    decimal.NewFromFloat(0.01),
					MinQuantityIncrement: decimal.NewFromFloat(0.0001),
				},
			},
		},
		EventJournal: eventjournal.Config{
			FilePath:     "/var/log/trading/events.log",
			MaxSizeBytes: 100 * 1024 * 1024, // 100MB
			MaxBackups:   10,
			MaxAgeDays:   30,
			Enabled:      true,
			BufferSize:   1000,
		},
	}
}

// ValidateConfig validates the trading configuration
func (c *TradingConfig) ValidateConfig() error {
	if c.Engine.Engine.WorkerPoolSize <= 0 {
		return fmt.Errorf("WorkerPoolSize must be positive")
	}

	if c.Engine.Engine.WorkerPoolQueueSize <= 0 {
		return fmt.Errorf("WorkerPoolQueueSize must be positive")
	}

	// Validate each trading pair configuration
	for _, pair := range c.Engine.Pairs {
		if pair.Symbol == "" {
			return fmt.Errorf("pair symbol cannot be empty")
		}
		if pair.MinPriceIncrement.LessThanOrEqual(decimal.Zero) {
			return fmt.Errorf("MinPriceIncrement must be positive for pair %s", pair.Symbol)
		}
		if pair.MinQuantityIncrement.LessThanOrEqual(decimal.Zero) {
			return fmt.Errorf("MinQuantityIncrement must be positive for pair %s", pair.Symbol)
		}
	}

	return nil
}

// GetPairConfig returns the configuration for a specific trading pair
func (c *TradingConfig) GetPairConfig(symbol string) (*engine.PairConfig, bool) {
	for _, pair := range c.Engine.Pairs {
		if pair.Symbol == symbol {
			return &pair, true
		}
	}
	return nil, false
}

// LoadTradingConfigFromEnv loads trading configuration from environment variables
func LoadTradingConfigFromEnv() *TradingConfig {
	// Start with default config
	config := DefaultTradingConfig()

	// Override with environment variables if needed
	// This would be implemented based on your environment variable naming convention
	// For now, return the default config

	return config
}
