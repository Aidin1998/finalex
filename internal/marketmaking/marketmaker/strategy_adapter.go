// Strategy adapter for adapting market making strategies to the backtest interface
package marketmaker

import (
	"context"
	"fmt"
	"math"

	"github.com/Aidin1998/finalex/pkg/models"
	"github.com/google/uuid"
)

// StrategyAdapter adapts a Strategy to the BacktestStrategy interface
// for use with the BacktestEngine
type StrategyAdapter struct {
	strategy     Strategy // The underlying strategy implementation
	config       map[string]interface{}
	lastPrice    float64
	volatility   float64
	inventory    float64
	metrics      map[string]float64
	strategyName string
}

// Initialize sets up the strategy with provided configuration
func (sa *StrategyAdapter) Initialize(config map[string]interface{}) error {
	sa.config = config
	sa.metrics = make(map[string]float64)
	sa.strategyName = fmt.Sprintf("%T", sa.strategy)

	// Set initial values
	sa.lastPrice = 0
	sa.volatility = 0.01 // Default volatility value
	sa.inventory = 0

	return nil
}

// OnMarketData processes market data and generates orders
func (sa *StrategyAdapter) OnMarketData(ctx context.Context, data *BacktestMarketData) ([]*models.Order, error) {
	orders := make([]*models.Order, 0)

	// Get current market price from data
	// For simplicity, we'll use the latest price from the first available pair
	var currentPrice float64

	// Find the most recent price from any available pair
	for pair, bars := range data.priceHistory {
		if len(bars) > 0 {
			latestBar := bars[len(bars)-1]
			currentPrice = latestBar.Close
			sa.lastPrice = currentPrice

			// Generate quote using the strategy
			bid, ask, size := sa.strategy.Quote(currentPrice, sa.volatility, sa.inventory)

			// Create bid order
			bidOrder := &models.Order{
				ID:        uuid.New(),
				Symbol:    pair,
				Side:      "buy",
				Type:      "limit",
				Price:     bid,
				Quantity:  size,
				CreatedAt: data.currentTime,
				Status:    "new",
			}

			// Create ask order
			askOrder := &models.Order{
				ID:        uuid.New(),
				Symbol:    pair,
				Side:      "sell",
				Type:      "limit",
				Price:     ask,
				Quantity:  size,
				CreatedAt: data.currentTime,
				Status:    "new",
			}

			// Add orders to result
			orders = append(orders, bidOrder, askOrder)

			// Update metrics
			sa.metrics["spread"] = (ask - bid) / currentPrice
			sa.metrics["mid_price"] = currentPrice
			sa.metrics["last_quote_time"] = float64(data.currentTime.Unix())

			break // We only need one pair for now
		}
	}

	return orders, nil
}

// OnOrderFill processes filled orders during backtesting
func (sa *StrategyAdapter) OnOrderFill(ctx context.Context, trade *BacktestTrade) error {
	// Update inventory based on filled trades
	if trade.Side == "buy" {
		sa.inventory += trade.Size
	} else {
		sa.inventory -= trade.Size
	}

	// Update metrics
	sa.metrics["inventory"] = sa.inventory
	sa.metrics["last_trade_price"] = trade.EntryPrice
	sa.metrics["last_trade_time"] = float64(trade.EntryTime.Unix())
	sa.metrics["trade_count"] = sa.metrics["trade_count"] + 1

	// Calculate PnL
	if sa.metrics["total_pnl"] == 0 {
		sa.metrics["total_pnl"] = trade.NetPnL
	} else {
		sa.metrics["total_pnl"] += trade.NetPnL
	}

	return nil
}

// GetMetrics returns strategy performance metrics
func (sa *StrategyAdapter) GetMetrics() map[string]float64 {
	return sa.metrics
}

// Name returns the name of the strategy
func (sa *StrategyAdapter) Name() string {
	return sa.strategyName
}

// updateVolatility calculates volatility from recent price history
func (sa *StrategyAdapter) updateVolatility(prices []float64) {
	if len(prices) < 2 {
		return
	}

	// Simple volatility calculation (standard deviation of returns)
	var sum, sumSquared float64
	returns := make([]float64, len(prices)-1)

	for i := 1; i < len(prices); i++ {
		if prices[i-1] > 0 {
			returns[i-1] = (prices[i] - prices[i-1]) / prices[i-1]
			sum += returns[i-1]
		}
	}

	mean := sum / float64(len(returns))

	for _, ret := range returns {
		diff := ret - mean
		sumSquared += diff * diff
	}

	variance := sumSquared / float64(len(returns))
	sa.volatility = math.Sqrt(variance)
	sa.metrics["volatility"] = sa.volatility
}
