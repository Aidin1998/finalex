// Package adapter provides compatibility between old and new strategy interfaces
package adapter

import (
	"context"
	"fmt"
	"time"

	"github.com/Aidin1998/finalex/internal/marketmaking/marketmaker"
	"github.com/Aidin1998/finalex/internal/marketmaking/strategies/common"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// LegacyStrategyAdapter adapts old strategy interface to new unified interface
type LegacyStrategyAdapter struct {
	strategy  marketmaker.Strategy
	name      string
	config    common.StrategyConfig
	status    common.StrategyStatus
	metrics   common.StrategyMetrics
	isRunning bool
}

// NewLegacyStrategyAdapter creates a new adapter for legacy strategies
func NewLegacyStrategyAdapter(strategy marketmaker.Strategy, name string, config common.StrategyConfig) *LegacyStrategyAdapter {
	return &LegacyStrategyAdapter{
		strategy: strategy,
		name:     name,
		config:   config,
		status:   common.StatusStopped,
		metrics: common.StrategyMetrics{
			TotalPnL:          decimal.NewFromInt(0),
			DailyPnL:          decimal.NewFromInt(0),
			SharpeRatio:       decimal.NewFromInt(0),
			MaxDrawdown:       decimal.NewFromInt(0),
			WinRate:           decimal.NewFromInt(0),
			OrdersPlaced:      0,
			OrdersFilled:      0,
			OrdersCancelled:   0,
			SuccessRate:       decimal.NewFromInt(0),
			SpreadCapture:     decimal.NewFromInt(0),
			InventoryTurnover: decimal.NewFromInt(0),
			QuoteUptime:       decimal.NewFromInt(0),
			LastUpdated:       time.Now(),
		},
	}
}

// Quote generates quotes using the legacy strategy
func (a *LegacyStrategyAdapter) Quote(ctx context.Context, input common.QuoteInput) (*common.QuoteOutput, error) {
	if !a.isRunning {
		return nil, fmt.Errorf("strategy is not running")
	}

	// Convert new input format to legacy format
	midPrice, _ := input.MidPrice.Float64()
	volatility, _ := input.Volatility.Float64()
	inventory, _ := input.Inventory.Float64()

	// Call legacy strategy
	bid, ask, size := a.strategy.Quote(midPrice, volatility, inventory)

	// Convert back to new format
	output := &common.QuoteOutput{
		BidPrice:    decimal.NewFromFloat(bid),
		AskPrice:    decimal.NewFromFloat(ask),
		BidSize:     decimal.NewFromFloat(size),
		AskSize:     decimal.NewFromFloat(size),
		Confidence:  decimal.NewFromFloat(0.8), // Default confidence
		TTL:         time.Second * 5,           // Default TTL
		GeneratedAt: time.Now(),
	}

	// Update metrics
	a.metrics.OrdersPlaced++
	a.metrics.LastUpdated = time.Now()

	return output, nil
}

// Initialize prepares the strategy for operation
func (a *LegacyStrategyAdapter) Initialize(ctx context.Context, config common.StrategyConfig) error {
	a.config = config
	a.status = common.StatusStarting
	a.status = common.StatusStopped
	return nil
}

// Start begins strategy operation
func (a *LegacyStrategyAdapter) Start(ctx context.Context) error {
	if a.status != common.StatusStopped {
		return fmt.Errorf("strategy must be stopped before starting")
	}

	a.isRunning = true
	a.status = common.StatusRunning
	return nil
}

// Stop halts strategy operation
func (a *LegacyStrategyAdapter) Stop(ctx context.Context) error {
	a.isRunning = false
	a.status = common.StatusStopped
	return nil
}

// OnMarketData processes incoming market data
func (a *LegacyStrategyAdapter) OnMarketData(ctx context.Context, data *common.MarketData) error {
	// Legacy strategies don't have market data handlers
	return nil
}

// OnOrderFill handles order fill events
func (a *LegacyStrategyAdapter) OnOrderFill(ctx context.Context, fill *common.OrderFill) error {
	a.metrics.OrdersFilled++

	// Calculate PnL (simplified)
	fillValue := fill.Price.Mul(fill.Quantity)
	if fill.Side == "buy" {
		a.metrics.TotalPnL = a.metrics.TotalPnL.Sub(fillValue)
	} else {
		a.metrics.TotalPnL = a.metrics.TotalPnL.Add(fillValue)
	}

	a.metrics.LastTrade = fill.Timestamp
	return nil
}

// OnOrderCancel handles order cancellation events
func (a *LegacyStrategyAdapter) OnOrderCancel(ctx context.Context, orderID uuid.UUID, reason string) error {
	a.metrics.OrdersCancelled++
	return nil
}

// UpdateConfig updates strategy configuration
func (a *LegacyStrategyAdapter) UpdateConfig(ctx context.Context, config common.StrategyConfig) error {
	a.config = config
	return nil
}

// GetConfig returns current configuration
func (a *LegacyStrategyAdapter) GetConfig() common.StrategyConfig {
	return a.config
}

// GetMetrics returns current strategy metrics
func (a *LegacyStrategyAdapter) GetMetrics() *common.StrategyMetrics {
	// Calculate success rate
	if a.metrics.OrdersPlaced > 0 {
		successRate := float64(a.metrics.OrdersFilled) / float64(a.metrics.OrdersPlaced)
		a.metrics.SuccessRate = decimal.NewFromFloat(successRate)
	}

	// Update uptime
	a.metrics.StrategyUptime = time.Since(a.metrics.LastUpdated)

	return &a.metrics
}

// GetStatus returns current strategy status
func (a *LegacyStrategyAdapter) GetStatus() common.StrategyStatus {
	return a.status
}

// Name returns strategy name
func (a *LegacyStrategyAdapter) Name() string {
	return a.name
}

// Version returns strategy version
func (a *LegacyStrategyAdapter) Version() string {
	return "1.0.0-legacy"
}

// Description returns strategy description
func (a *LegacyStrategyAdapter) Description() string {
	return fmt.Sprintf("Legacy strategy adapter for %s", a.name)
}

// RiskLevel returns strategy risk level
func (a *LegacyStrategyAdapter) RiskLevel() common.RiskLevel {
	// Default to medium risk for legacy strategies
	return common.RiskMedium
}

// HealthCheck performs strategy health verification
func (a *LegacyStrategyAdapter) HealthCheck(ctx context.Context) *common.HealthStatus {
	return &common.HealthStatus{
		Healthy:   a.isRunning,
		Status:    string(a.status),
		LastCheck: time.Now(),
		Issues:    []string{},
	}
}

// Reset resets the strategy state
func (a *LegacyStrategyAdapter) Reset(ctx context.Context) error {
	a.metrics = common.StrategyMetrics{
		TotalPnL:          decimal.NewFromInt(0),
		DailyPnL:          decimal.NewFromInt(0),
		SharpeRatio:       decimal.NewFromInt(0),
		MaxDrawdown:       decimal.NewFromInt(0),
		WinRate:           decimal.NewFromInt(0),
		OrdersPlaced:      0,
		OrdersFilled:      0,
		OrdersCancelled:   0,
		SuccessRate:       decimal.NewFromInt(0),
		SpreadCapture:     decimal.NewFromInt(0),
		InventoryTurnover: decimal.NewFromInt(0),
		QuoteUptime:       decimal.NewFromInt(0),
		LastUpdated:       time.Now(),
	}
	return nil
}
