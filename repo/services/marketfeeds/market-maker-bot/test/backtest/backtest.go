package backtest

import (
	"time"
)

// TradeEvent represents a historical trade or order book event.
type TradeEvent struct {
	Timestamp time.Time
	Price     float64
	Volume    float64
	Side      string // "buy" or "sell"
}

// BacktestConfig holds configuration for the backtest run.
type BacktestConfig struct {
	InitialBalance float64
	LatencyMs      int
	SpreadModel    func(float64) float64 // function to model spread given volatility
}

// BacktestResult holds the results of a backtest run.
type BacktestResult struct {
	FinalBalance float64
	Trades       int
	PnL          float64
}

// Backtester simulates trading on historical data.
type Backtester struct {
	Config     BacktestConfig
	Events     []TradeEvent
	Balance    float64
	OpenOrders map[string]TradeEvent
	TradeCount int
}

// NewBacktester creates a new backtesting environment.
func NewBacktester(cfg BacktestConfig, events []TradeEvent) *Backtester {
	return &Backtester{
		Config:     cfg,
		Events:     events,
		Balance:    cfg.InitialBalance,
		OpenOrders: make(map[string]TradeEvent),
	}
}

// Run executes the backtest simulation.
func (b *Backtester) Run() BacktestResult {
	for _, event := range b.Events {
		b.simulateEvent(event)
	}
	return BacktestResult{
		FinalBalance: b.Balance,
		Trades:       b.TradeCount,
		PnL:          b.Balance - b.Config.InitialBalance,
	}
}

// simulateEvent processes a single trade event, applying spread and latency modeling.
func (b *Backtester) simulateEvent(event TradeEvent) {
	// Example: apply latency by skipping events within latency window
	// Example: apply spread model to simulate realistic fills
	// This is a placeholder for integration with strategy and orderbook modules
	b.TradeCount++
	// Update balance as a simple simulation (buy/sell logic can be expanded)
	if event.Side == "buy" {
		b.Balance -= event.Price * event.Volume
	} else {
		b.Balance += event.Price * event.Volume
	}
}
