// Type aliases to help resolve duplication issues
package marketmaker

import (
	"time"
)

// Define some type aliases to avoid redeclaration errors
// This allows our code to compile without modifying the original declarations

// StrategyPerformanceExt is an alias for the conflict resolution
type StrategyPerformanceExt struct {
	TotalPnL        float64       `json:"total_pnl"`
	DailyPnL        float64       `json:"daily_pnl"`
	SharpeRatio     float64       `json:"sharpe_ratio"`
	MaxDrawdown     float64       `json:"max_drawdown"`
	OrdersPlaced    int64         `json:"orders_placed"`
	OrdersFilled    int64         `json:"orders_filled"`
	OrdersCancelled int64         `json:"orders_cancelled"`
	SuccessRate     float64       `json:"success_rate"`
	AvgFillTime     time.Duration `json:"avg_fill_time"`
	LastTrade       time.Time     `json:"last_trade"`
}

// Convert between different StrategyPerformance definitions
func ConvertFromAdminToolsStrategyPerformance(sp *StrategyPerformance) *StrategyPerformanceExt {
	return &StrategyPerformanceExt{
		TotalPnL:        sp.TotalPnL,
		DailyPnL:        sp.DailyPnL,
		OrdersPlaced:    sp.OrdersPlaced,
		OrdersFilled:    sp.OrdersFilled,
		OrdersCancelled: sp.OrdersCancelled,
		SuccessRate:     sp.SuccessRate,
		AvgFillTime:     sp.AvgFillTime,
		LastTrade:       sp.LastTrade,
	}
}
