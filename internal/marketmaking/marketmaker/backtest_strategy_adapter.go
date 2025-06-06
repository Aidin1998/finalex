package marketmaker

import (
	"context"

	"github.com/google/uuid"

	"github.com/Aidin1998/finalex/internal/marketmaking/strategies/common"
	"github.com/Aidin1998/finalex/pkg/models"
	"github.com/shopspring/decimal"
)

// BacktestStrategyAdapter adapts a new MarketMakingStrategy to the legacy BacktestStrategy interface
// Used for backtesting with the new strategies/common interface

type BacktestStrategyAdapter struct {
	strategy common.MarketMakingStrategy
}

func NewBacktestStrategyAdapter(strategy common.MarketMakingStrategy) *BacktestStrategyAdapter {
	return &BacktestStrategyAdapter{strategy: strategy}
}

func (a *BacktestStrategyAdapter) Initialize(ctx context.Context, config common.StrategyConfig) error {
	return a.strategy.Initialize(ctx, config)
}

// Remove legacy OnMarketData, implement only the new interface
func (a *BacktestStrategyAdapter) OnMarketData(ctx context.Context, data *common.MarketData) error {
	return a.strategy.OnMarketData(ctx, data)
}

// Remove legacy OnOrderFill, implement only the new interface
func (a *BacktestStrategyAdapter) OnOrderFill(ctx context.Context, fill *common.OrderFill) error {
	return a.strategy.OnOrderFill(ctx, fill)
}

func (a *BacktestStrategyAdapter) OnOrderCancel(ctx context.Context, orderID uuid.UUID, reason string) error {
	return a.strategy.OnOrderCancel(ctx, orderID, reason)
}

// GetMetrics returns the underlying strategy's metrics (for new interface)
func (a *BacktestStrategyAdapter) GetMetrics() *common.StrategyMetrics {
	return a.strategy.GetMetrics()
}

// LegacyGetMetrics returns legacy map[string]float64 for old code
func (a *BacktestStrategyAdapter) LegacyGetMetrics() map[string]float64 {
	metrics := a.strategy.GetMetrics()
	return convertToLegacyMetrics(metrics)
}

func (a *BacktestStrategyAdapter) Name() string {
	return a.strategy.Name()
}

func (a *BacktestStrategyAdapter) Description() string {
	return a.strategy.Description()
}

func (a *BacktestStrategyAdapter) GetConfig() common.StrategyConfig {
	return a.strategy.GetConfig()
}

func (a *BacktestStrategyAdapter) GetStatus() common.StrategyStatus {
	return a.strategy.GetStatus()
}

func (a *BacktestStrategyAdapter) HealthCheck(ctx context.Context) *common.HealthStatus {
	return a.strategy.HealthCheck(ctx)
}

func (a *BacktestStrategyAdapter) Quote(ctx context.Context, input common.QuoteInput) (*common.QuoteOutput, error) {
	return a.strategy.Quote(ctx, input)
}

func (a *BacktestStrategyAdapter) Reset(ctx context.Context) error {
	return a.strategy.Reset(ctx)
}

func (a *BacktestStrategyAdapter) RiskLevel() common.RiskLevel {
	return a.strategy.RiskLevel()
}

func (a *BacktestStrategyAdapter) Start(ctx context.Context) error {
	return a.strategy.Start(ctx)
}

func (a *BacktestStrategyAdapter) Stop(ctx context.Context) error {
	return a.strategy.Stop(ctx)
}

func (a *BacktestStrategyAdapter) UpdateConfig(ctx context.Context, config common.StrategyConfig) error {
	return a.strategy.UpdateConfig(ctx, config)
}

func (a *BacktestStrategyAdapter) Version() string {
	return a.strategy.Version()
}

// Helper conversion functions (implement as needed)
func convertToCommonMarketData(data *BacktestMarketData) *common.MarketData {
	if data == nil {
		return &common.MarketData{}
	}
	// Example: Map the first order book snapshot if available
	var ob *models.OrderBookSnapshot
	for _, v := range data.orderBooks {
		ob = v
		break
	}
	return &common.MarketData{
		Symbol:    data.dataSource,
		Timestamp: data.currentTime,
		// Map more fields as needed
		OrderBookDepth: convertOrderBookToPriceLevels(ob),
	}
}

func convertOrderBookToPriceLevels(ob *models.OrderBookSnapshot) []common.PriceLevel {
	if ob == nil {
		return nil
	}
	levels := []common.PriceLevel{}
	for _, lvl := range ob.Bids {
		levels = append(levels, common.PriceLevel{
			Price:  decimal.NewFromFloat(lvl.Price),
			Volume: decimal.NewFromFloat(lvl.Volume),
			Count:  0, // Not available in OrderBookLevel
		})
	}
	for _, lvl := range ob.Asks {
		levels = append(levels, common.PriceLevel{
			Price:  decimal.NewFromFloat(lvl.Price),
			Volume: decimal.NewFromFloat(lvl.Volume),
			Count:  0, // Not available in OrderBookLevel
		})
	}
	return levels
}

func convertToCommonOrderFill(trade *BacktestTrade) *common.OrderFill {
	if trade == nil {
		return &common.OrderFill{}
	}
	return &common.OrderFill{
		OrderID:   [16]byte{}, // Not available in BacktestTrade
		Symbol:    trade.Pair,
		Side:      trade.Side,
		Price:     decimal.NewFromFloat(trade.ExitPrice),
		Quantity:  decimal.NewFromFloat(trade.Size),
		Fee:       decimal.NewFromFloat(trade.Commission),
		Timestamp: trade.ExitTime,
		TradeID:   trade.ID,
	}
}

func convertToLegacyMetrics(metrics *common.StrategyMetrics) map[string]float64 {
	if metrics == nil {
		return map[string]float64{}
	}
	return map[string]float64{
		"total_pnl":          metrics.TotalPnL.InexactFloat64(),
		"daily_pnl":          metrics.DailyPnL.InexactFloat64(),
		"sharpe_ratio":       metrics.SharpeRatio.InexactFloat64(),
		"max_drawdown":       metrics.MaxDrawdown.InexactFloat64(),
		"win_rate":           metrics.WinRate.InexactFloat64(),
		"orders_placed":      float64(metrics.OrdersPlaced),
		"orders_filled":      float64(metrics.OrdersFilled),
		"orders_cancelled":   float64(metrics.OrdersCancelled),
		"success_rate":       metrics.SuccessRate.InexactFloat64(),
		"avg_fill_time":      metrics.AvgFillTime.Seconds(),
		"spread_capture":     metrics.SpreadCapture.InexactFloat64(),
		"inventory_turnover": metrics.InventoryTurnover.InexactFloat64(),
		"quote_uptime":       metrics.QuoteUptime.InexactFloat64(),
	}
}
