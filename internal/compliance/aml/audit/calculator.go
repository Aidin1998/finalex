package risk

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/shopspring/decimal"
)

// MarketData represents real-time market data for risk calculations
type MarketData struct {
	Symbol     string          `json:"symbol"`
	Price      decimal.Decimal `json:"price"`
	Volatility decimal.Decimal `json:"volatility"`
	Timestamp  time.Time       `json:"timestamp"`
}

// RiskMetrics holds real-time risk calculations
type RiskMetrics struct {
	UserID             string                     `json:"user_id"`
	TotalExposure      decimal.Decimal            `json:"total_exposure"`
	ValueAtRisk        decimal.Decimal            `json:"value_at_risk"`
	MarginRequired     decimal.Decimal            `json:"margin_required"`
	MarkToMarketPnL    decimal.Decimal            `json:"mark_to_market_pnl"`
	PositionExposures  map[string]decimal.Decimal `json:"position_exposures"`
	ConcentrationRisk  decimal.Decimal            `json:"concentration_risk"`
	LiquidityRisk      decimal.Decimal            `json:"liquidity_risk"`
	MaxDrawdown        decimal.Decimal            `json:"max_drawdown"`
	SharpeRatio        decimal.Decimal            `json:"sharpe_ratio"`
	RiskScore          decimal.Decimal            `json:"risk_score"`
	LastUpdated        time.Time                  `json:"last_updated"`
	CalculationLatency time.Duration              `json:"calculation_latency"`
}

// RiskCalculator provides real-time risk calculations
type RiskCalculator struct {
	mu           sync.RWMutex
	marketData   map[string]*MarketData                // symbol -> market data
	correlations map[string]map[string]decimal.Decimal // symbol -> symbol -> correlation
	pm           *PositionManager

	// Configuration
	VaRConfidenceLevel decimal.Decimal // e.g., 0.95 for 95% VaR
	VaRTimeHorizon     time.Duration   // e.g., 1 day
	MarginRate         decimal.Decimal // e.g., 0.2 for 20% margin
}

// NewRiskCalculator creates a new real-time risk calculator
func NewRiskCalculator(pm *PositionManager) *RiskCalculator {
	return &RiskCalculator{
		marketData:         make(map[string]*MarketData),
		correlations:       make(map[string]map[string]decimal.Decimal),
		pm:                 pm,
		VaRConfidenceLevel: decimal.NewFromFloat(0.95),
		VaRTimeHorizon:     24 * time.Hour,
		MarginRate:         decimal.NewFromFloat(0.2),
	}
}

// UpdateMarketData updates real-time market data for risk calculations
func (rc *RiskCalculator) UpdateMarketData(ctx context.Context, symbol string, price, volatility decimal.Decimal) error {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	rc.marketData[symbol] = &MarketData{
		Symbol:     symbol,
		Price:      price,
		Volatility: volatility,
		Timestamp:  time.Now(),
	}

	return nil
}

// CalculateRealTimeRisk performs comprehensive real-time risk calculations
func (rc *RiskCalculator) CalculateRealTimeRisk(ctx context.Context, userID string) (*RiskMetrics, error) {
	start := time.Now()

	rc.mu.RLock()
	defer rc.mu.RUnlock()

	positions := rc.pm.ListPositions(ctx, userID)
	if len(positions) == 0 {
		return &RiskMetrics{
			UserID:             userID,
			TotalExposure:      decimal.Zero,
			ValueAtRisk:        decimal.Zero,
			MarginRequired:     decimal.Zero,
			MarkToMarketPnL:    decimal.Zero,
			PositionExposures:  make(map[string]decimal.Decimal),
			ConcentrationRisk:  decimal.Zero,
			LiquidityRisk:      decimal.Zero,
			MaxDrawdown:        decimal.Zero,
			SharpeRatio:        decimal.Zero,
			RiskScore:          decimal.Zero,
			LastUpdated:        time.Now(),
			CalculationLatency: time.Since(start),
		}, nil
	}

	// Calculate mark-to-market values and exposures
	totalExposure := decimal.Zero
	markToMarketPnL := decimal.Zero
	positionExposures := make(map[string]decimal.Decimal)

	for market, position := range positions {
		if position.Quantity.IsZero() {
			continue
		}

		marketData, exists := rc.marketData[market]
		if !exists {
			continue // Skip if no market data available
		}

		// Position value at current market price
		currentValue := position.Quantity.Mul(marketData.Price)

		// Position cost (based on average price)
		positionCost := position.Quantity.Mul(position.AvgPrice)

		// Unrealized PnL
		unrealizedPnL := currentValue.Sub(positionCost)
		markToMarketPnL = markToMarketPnL.Add(unrealizedPnL)

		// Exposure (absolute value)
		exposure := currentValue.Abs()
		positionExposures[market] = exposure
		totalExposure = totalExposure.Add(exposure)
	}

	// Calculate Value at Risk (VaR)
	var estimatedVaR decimal.Decimal
	if totalExposure.IsPositive() {
		// Simple parametric VaR calculation
		// VaR = Position * Volatility * Z-score * sqrt(time)
		avgVolatility := rc.calculateAverageVolatility()
		zScore := decimal.NewFromFloat(1.645)               // 95% confidence level
		timeScaling := decimal.NewFromFloat(math.Sqrt(1.0)) // 1-day horizon

		estimatedVaR = totalExposure.Mul(avgVolatility).Mul(zScore).Mul(timeScaling)
	}

	// Calculate margin requirement
	marginRequired := totalExposure.Mul(rc.MarginRate)

	// Calculate concentration risk (Herfindahl index)
	concentrationRisk := rc.calculateConcentrationRisk(positionExposures, totalExposure)

	// Calculate liquidity risk (simplified)
	liquidityRisk := rc.calculateLiquidityRisk(positions)

	// Calculate overall risk score
	riskScore := rc.calculateRiskScore(totalExposure, estimatedVaR, concentrationRisk, liquidityRisk)

	return &RiskMetrics{
		UserID:             userID,
		TotalExposure:      totalExposure,
		ValueAtRisk:        estimatedVaR,
		MarginRequired:     marginRequired,
		MarkToMarketPnL:    markToMarketPnL,
		PositionExposures:  positionExposures,
		ConcentrationRisk:  concentrationRisk,
		LiquidityRisk:      liquidityRisk,
		MaxDrawdown:        decimal.Zero, // Would require historical data
		SharpeRatio:        decimal.Zero, // Would require return history
		RiskScore:          riskScore,
		LastUpdated:        time.Now(),
		CalculationLatency: time.Since(start),
	}, nil
}

// calculateAverageVolatility calculates weighted average volatility across all markets
func (rc *RiskCalculator) calculateAverageVolatility() decimal.Decimal {
	if len(rc.marketData) == 0 {
		return decimal.NewFromFloat(0.02) // Default 2% volatility
	}

	totalVolatility := decimal.Zero
	count := 0

	for _, data := range rc.marketData {
		if data.Volatility.IsPositive() {
			totalVolatility = totalVolatility.Add(data.Volatility)
			count++
		}
	}

	if count == 0 {
		return decimal.NewFromFloat(0.02)
	}

	return totalVolatility.Div(decimal.NewFromInt(int64(count)))
}

// calculateConcentrationRisk calculates portfolio concentration risk using Herfindahl index
func (rc *RiskCalculator) calculateConcentrationRisk(exposures map[string]decimal.Decimal, totalExposure decimal.Decimal) decimal.Decimal {
	if totalExposure.IsZero() {
		return decimal.Zero
	}

	herfindahl := decimal.Zero
	for _, exposure := range exposures {
		weight := exposure.Div(totalExposure)
		herfindahl = herfindahl.Add(weight.Mul(weight))
	}

	return herfindahl
}

// calculateLiquidityRisk calculates liquidity risk based on position sizes
func (rc *RiskCalculator) calculateLiquidityRisk(positions map[string]*Position) decimal.Decimal {
	// Simplified liquidity risk calculation
	// In practice, this would consider market depth, trading volumes, etc.

	liquidityRisk := decimal.Zero
	for market, position := range positions {
		positionSize := position.Quantity.Abs()

		// Assume higher risk for larger positions
		if positionSize.GreaterThan(decimal.NewFromInt(1000)) {
			liquidityRisk = liquidityRisk.Add(decimal.NewFromFloat(0.1))
		} else if positionSize.GreaterThan(decimal.NewFromInt(100)) {
			liquidityRisk = liquidityRisk.Add(decimal.NewFromFloat(0.05))
		}

		_ = market
	}

	return liquidityRisk
}

// calculateRiskScore calculates an overall risk score (0-100)
func (rc *RiskCalculator) calculateRiskScore(exposure, var_, concentration, liquidity decimal.Decimal) decimal.Decimal {
	if exposure.IsZero() {
		return decimal.Zero
	}

	// Weighted risk score calculation
	varWeight := decimal.NewFromFloat(0.4)
	concentrationWeight := decimal.NewFromFloat(0.3)
	liquidityWeight := decimal.NewFromFloat(0.3)

	// Normalize VaR as percentage of exposure
	varRatio := var_.Div(exposure)

	// Combine weighted components
	score := varRatio.Mul(varWeight).
		Add(concentration.Mul(concentrationWeight)).
		Add(liquidity.Mul(liquidityWeight))

	// Scale to 0-100
	finalScore := score.Mul(decimal.NewFromInt(100))

	// Cap at 100
	if finalScore.GreaterThan(decimal.NewFromInt(100)) {
		return decimal.NewFromInt(100)
	}

	return finalScore
}

// BatchCalculateRisk calculates risk for multiple users efficiently
func (rc *RiskCalculator) BatchCalculateRisk(ctx context.Context, userIDs []string) (map[string]*RiskMetrics, error) {
	results := make(map[string]*RiskMetrics)
	var wg sync.WaitGroup
	var mu sync.Mutex

	// Process users in parallel with goroutine pool
	semaphore := make(chan struct{}, 10) // Limit concurrent calculations

	for _, userID := range userIDs {
		wg.Add(1)
		go func(uid string) {
			defer wg.Done()

			semaphore <- struct{}{}        // Acquire
			defer func() { <-semaphore }() // Release

			metrics, err := rc.CalculateRealTimeRisk(ctx, uid)
			if err == nil {
				mu.Lock()
				results[uid] = metrics
				mu.Unlock()
			}
		}(userID)
	}

	wg.Wait()
	return results, nil
}

// GetMarketData returns current market data for a symbol
func (rc *RiskCalculator) GetMarketData(symbol string) (*MarketData, bool) {
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	data, exists := rc.marketData[symbol]
	return data, exists
}

// SetCorrelation sets correlation between two symbols
func (rc *RiskCalculator) SetCorrelation(symbol1, symbol2 string, correlation decimal.Decimal) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	if rc.correlations[symbol1] == nil {
		rc.correlations[symbol1] = make(map[string]decimal.Decimal)
	}
	if rc.correlations[symbol2] == nil {
		rc.correlations[symbol2] = make(map[string]decimal.Decimal)
	}

	rc.correlations[symbol1][symbol2] = correlation
	rc.correlations[symbol2][symbol1] = correlation
}

// ValidateCalculationPerformance checks if calculations meet sub-second requirements
func (rc *RiskCalculator) ValidateCalculationPerformance(ctx context.Context, userID string) error {
	start := time.Now()

	_, err := rc.CalculateRealTimeRisk(ctx, userID)
	if err != nil {
		return fmt.Errorf("risk calculation failed: %w", err)
	}

	elapsed := time.Since(start)
	if elapsed > time.Second {
		return fmt.Errorf("risk calculation too slow: %v > 1s", elapsed)
	}

	return nil
}
