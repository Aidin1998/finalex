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
func ConvertFromServiceStrategyPerformance(sp *StrategyPerformance) *StrategyPerformanceExt {
	return &StrategyPerformanceExt{
		TotalPnL:        sp.PnL,         // Map PnL to TotalPnL
		DailyPnL:        sp.TotalReturn, // Map TotalReturn to DailyPnL as approximation
		SharpeRatio:     sp.SharpeRatio,
		MaxDrawdown:     sp.MaxDrawdown,
		OrdersPlaced:    int64(sp.TradeCount), // Map TradeCount to OrdersPlaced
		OrdersFilled:    int64(sp.TradeCount), // Assume all trades were filled
		OrdersCancelled: 0,                    // Not available in service StrategyPerformance
		SuccessRate:     sp.SuccessRate,
		AvgFillTime:     sp.LatencyP95,  // Use P95 latency as avg fill time
		LastTrade:       sp.LastUpdated, // Use LastUpdated as LastTrade
	}
}

// Convert from AdminTools StrategyInstancePerformance to StrategyPerformanceExt
func ConvertFromAdminToolsStrategyPerformance(sip *StrategyInstancePerformance) *StrategyPerformanceExt {
	return &StrategyPerformanceExt{
		TotalPnL:        sip.TotalPnL,
		DailyPnL:        sip.DailyPnL,
		SharpeRatio:     0.0, // Not available in StrategyInstancePerformance
		MaxDrawdown:     0.0, // Not available in StrategyInstancePerformance
		OrdersPlaced:    sip.OrdersPlaced,
		OrdersFilled:    sip.OrdersFilled,
		OrdersCancelled: sip.OrdersCancelled,
		SuccessRate:     sip.SuccessRate,
		AvgFillTime:     sip.AvgFillTime,
		LastTrade:       sip.LastTrade,
	}
}
