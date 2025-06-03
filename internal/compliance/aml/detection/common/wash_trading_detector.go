package common

import (
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// ConsolidatedWashTradingDetector provides unified wash trading detection
type ConsolidatedWashTradingDetector struct {
	name               string
	threshold          decimal.Decimal
	minVolume          decimal.Decimal
	timeWindow         time.Duration
	logger             *zap.SugaredLogger
	severityCalculator *SeverityCalculator
	evidenceBuilder    *EvidenceBuilder
	confidenceCalc     *ConfidenceCalculator
}

// TradingActivity represents trading activity for wash trading analysis
type TradingActivity struct {
	UserID     string
	Market     string
	TimeWindow time.Duration
	Orders     []Order
	Trades     []Trade
	Metadata   map[string]interface{}
}

// Order represents a trading order
type Order struct {
	ID        string          `json:"id"`
	UserID    string          `json:"user_id"`
	Market    string          `json:"market"`
	Side      string          `json:"side"` // "buy" or "sell"
	Size      decimal.Decimal `json:"size"`
	Price     decimal.Decimal `json:"price"`
	Timestamp time.Time       `json:"timestamp"`
	Status    string          `json:"status"`
}

// Trade represents an executed trade
type Trade struct {
	ID          string          `json:"id"`
	Market      string          `json:"market"`
	BuyUserID   string          `json:"buy_user_id"`
	SellUserID  string          `json:"sell_user_id"`
	Size        decimal.Decimal `json:"size"`
	Price       decimal.Decimal `json:"price"`
	Timestamp   time.Time       `json:"timestamp"`
	BuyOrderID  string          `json:"buy_order_id"`
	SellOrderID string          `json:"sell_order_id"`
}

// WashTradingIndicators holds calculated indicators for wash trading detection
type WashTradingIndicators struct {
	SelfTradingRatio     decimal.Decimal `json:"self_trading_ratio"`
	VolumeInflationRatio decimal.Decimal `json:"volume_inflation_ratio"`
	OrderSequenceScore   decimal.Decimal `json:"order_sequence_score"`
	TimeClustering       float64         `json:"time_clustering"`
	SequentialPatterns   int             `json:"sequential_patterns"`
	BalanceConsistency   decimal.Decimal `json:"balance_consistency"`
}

// Implement DetectionActivity interface
func (ta *TradingActivity) GetUserID() string {
	return ta.UserID
}

func (ta *TradingActivity) GetMarket() string {
	return ta.Market
}

func (ta *TradingActivity) GetTimeWindow() time.Duration {
	return ta.TimeWindow
}

func (ta *TradingActivity) GetMetadata() map[string]interface{} {
	return ta.Metadata
}

// NewConsolidatedWashTradingDetector creates a new consolidated wash trading detector
func NewConsolidatedWashTradingDetector(logger *zap.SugaredLogger, config map[string]interface{}) *ConsolidatedWashTradingDetector {
	detector := &ConsolidatedWashTradingDetector{
		name:               "consolidated_wash_trading",
		threshold:          decimal.NewFromFloat(75.0), // Default threshold
		minVolume:          decimal.NewFromFloat(1000),
		timeWindow:         30 * time.Minute,
		logger:             logger,
		severityCalculator: NewDefaultSeverityCalculator(),
		evidenceBuilder:    NewEvidenceBuilder(logger),
		confidenceCalc:     NewConfidenceCalculator(logger),
	}

	// Apply configuration if provided
	if config != nil {
		detector.UpdateConfig(config)
	}

	return detector
}

// GetName returns the detector name
func (wtd *ConsolidatedWashTradingDetector) GetName() string {
	return wtd.name
}

// GetType returns the detector type
func (wtd *ConsolidatedWashTradingDetector) GetType() string {
	return "wash_trading"
}

// UpdateConfig updates detector configuration
func (wtd *ConsolidatedWashTradingDetector) UpdateConfig(config map[string]interface{}) error {
	if threshold, ok := config["threshold"].(float64); ok {
		wtd.threshold = decimal.NewFromFloat(threshold)
	}
	if minVolume, ok := config["min_volume"].(float64); ok {
		wtd.minVolume = decimal.NewFromFloat(minVolume)
	}
	if timeWindowMinutes, ok := config["time_window_minutes"].(int); ok {
		wtd.timeWindow = time.Duration(timeWindowMinutes) * time.Minute
	}
	return nil
}

// Detect analyzes trading activity for wash trading patterns
func (wtd *ConsolidatedWashTradingDetector) Detect(activity DetectionActivity) *DetectionPattern {
	tradingActivity, ok := activity.(*TradingActivity)
	if !ok {
		wtd.logger.Warnw("Invalid activity type for wash trading detector", "expected", "TradingActivity")
		return nil
	}

	// Check minimum volume requirement
	totalVolume := wtd.calculateTotalVolume(tradingActivity.Trades)
	if totalVolume.LessThan(wtd.minVolume) {
		return nil
	}

	// Calculate wash trading indicators
	indicators := wtd.calculateWashTradingIndicators(tradingActivity)

	// Calculate base confidence score
	baseConfidence := wtd.calculateBaseConfidence(indicators)

	// Build evidence
	evidence := wtd.buildWashTradingEvidence(indicators)

	// Calculate final confidence with evidence
	finalConfidence := wtd.confidenceCalc.CalculateConfidence(evidence, baseConfidence)

	// Check if threshold is met
	if !finalConfidence.GreaterThan(wtd.threshold) {
		return nil
	}

	// Determine severity
	severity := wtd.severityCalculator.DetermineSeverity(finalConfidence)

	return &DetectionPattern{
		Type:        "wash_trading",
		Description: "Detected potential wash trading activity",
		Confidence:  finalConfidence,
		Severity:    severity,
		Evidence:    evidence,
		DetectedAt:  time.Now(),
		Metadata: map[string]interface{}{
			"self_trading_ratio":     indicators.SelfTradingRatio.String(),
			"volume_inflation_ratio": indicators.VolumeInflationRatio.String(),
			"order_sequence_score":   indicators.OrderSequenceScore.String(),
			"time_clustering":        indicators.TimeClustering,
			"sequential_patterns":    indicators.SequentialPatterns,
			"balance_consistency":    indicators.BalanceConsistency.String(),
			"total_volume":           totalVolume.String(),
			"trade_count":            len(tradingActivity.Trades),
			"order_count":            len(tradingActivity.Orders),
		},
	}
}

// calculateWashTradingIndicators calculates comprehensive wash trading indicators
func (wtd *ConsolidatedWashTradingDetector) calculateWashTradingIndicators(activity *TradingActivity) WashTradingIndicators {
	indicators := WashTradingIndicators{}

	// Calculate self trading ratio
	indicators.SelfTradingRatio = wtd.calculateSelfTradingRatio(activity.Trades)

	// Calculate volume inflation ratio
	indicators.VolumeInflationRatio = wtd.calculateVolumeInflationRatio(activity.Trades)

	// Calculate order sequence score
	indicators.OrderSequenceScore = wtd.calculateOrderSequenceScore(activity.Orders)

	// Calculate time clustering
	indicators.TimeClustering = wtd.calculateTimeClustering(activity.Orders)

	// Count sequential patterns
	indicators.SequentialPatterns = wtd.countSequentialPatterns(activity.Orders)

	// Calculate balance consistency
	indicators.BalanceConsistency = wtd.calculateBalanceConsistency(activity.Trades)

	return indicators
}

// calculateSelfTradingRatio calculates the ratio of self-trading
func (wtd *ConsolidatedWashTradingDetector) calculateSelfTradingRatio(trades []Trade) decimal.Decimal {
	if len(trades) == 0 {
		return decimal.Zero
	}

	selfTrades := 0
	for _, trade := range trades {
		if trade.BuyUserID == trade.SellUserID {
			selfTrades++
		}
	}

	ratio := decimal.NewFromInt(int64(selfTrades)).Div(decimal.NewFromInt(int64(len(trades))))
	return ratio.Mul(decimal.NewFromInt(100)) // Convert to percentage
}

// calculateVolumeInflationRatio calculates volume inflation compared to market baseline
func (wtd *ConsolidatedWashTradingDetector) calculateVolumeInflationRatio(trades []Trade) decimal.Decimal {
	// Simplified calculation - in practice would compare to market baseline
	totalVolume := decimal.Zero
	uniqueTradeVolume := decimal.Zero

	for _, trade := range trades {
		tradeVolume := trade.Size.Mul(trade.Price)
		totalVolume = totalVolume.Add(tradeVolume)

		// If it's not self-trading, count toward unique volume
		if trade.BuyUserID != trade.SellUserID {
			uniqueTradeVolume = uniqueTradeVolume.Add(tradeVolume)
		}
	}

	if uniqueTradeVolume.IsZero() {
		return decimal.NewFromInt(100) // 100% inflation if all trades are self-trades
	}

	inflationRatio := totalVolume.Sub(uniqueTradeVolume).Div(uniqueTradeVolume)
	return inflationRatio.Mul(decimal.NewFromInt(100)) // Convert to percentage
}

// calculateOrderSequenceScore analyzes alternating buy/sell patterns
func (wtd *ConsolidatedWashTradingDetector) calculateOrderSequenceScore(orders []Order) decimal.Decimal {
	if len(orders) < 2 {
		return decimal.Zero
	}

	alternatingCount := 0
	for i := 1; i < len(orders); i++ {
		if orders[i].Side != orders[i-1].Side {
			alternatingCount++
		}
	}

	score := decimal.NewFromInt(int64(alternatingCount)).Div(decimal.NewFromInt(int64(len(orders) - 1)))
	return score.Mul(decimal.NewFromInt(100)) // Convert to percentage
}

// calculateTimeClustering analyzes time clustering patterns
func (wtd *ConsolidatedWashTradingDetector) calculateTimeClustering(orders []Order) float64 {
	if len(orders) < 2 {
		return 0.0
	}

	// Calculate average time between orders
	var totalGap time.Duration
	for i := 1; i < len(orders); i++ {
		gap := orders[i].Timestamp.Sub(orders[i-1].Timestamp)
		totalGap += gap
	}

	avgGap := totalGap / time.Duration(len(orders)-1)

	// Calculate variance from average (simplified clustering detection)
	var variance float64
	for i := 1; i < len(orders); i++ {
		gap := orders[i].Timestamp.Sub(orders[i-1].Timestamp)
		diff := float64(gap - avgGap)
		variance += diff * diff
	}

	// Higher variance means less clustering, lower variance means more clustering
	// Return inverse measure where higher values indicate more clustering
	if variance == 0 {
		return 100.0 // Perfect clustering
	}

	// Simplified scoring - in practice would be more sophisticated
	clusteringScore := 100.0 / (1.0 + variance/1e18) // Normalize variance
	return clusteringScore
}

// countSequentialPatterns counts alternating buy/sell patterns
func (wtd *ConsolidatedWashTradingDetector) countSequentialPatterns(orders []Order) int {
	if len(orders) < 4 { // Need at least 4 orders for a pattern
		return 0
	}

	patterns := 0
	currentPattern := 0

	for i := 1; i < len(orders); i++ {
		if orders[i].Side != orders[i-1].Side {
			currentPattern++
		} else {
			if currentPattern >= 3 { // At least 3 alternations
				patterns++
			}
			currentPattern = 0
		}
	}

	// Check final pattern
	if currentPattern >= 3 {
		patterns++
	}

	return patterns
}

// calculateBalanceConsistency analyzes if trades maintain consistent balance
func (wtd *ConsolidatedWashTradingDetector) calculateBalanceConsistency(trades []Trade) decimal.Decimal {
	if len(trades) == 0 {
		return decimal.Zero
	}

	// Simplified calculation - tracks if buy/sell volumes are balanced
	buyVolume := decimal.Zero
	sellVolume := decimal.Zero

	for _, trade := range trades {
		volume := trade.Size.Mul(trade.Price)
		// For self-trades, it's both buy and sell
		if trade.BuyUserID == trade.SellUserID {
			buyVolume = buyVolume.Add(volume)
			sellVolume = sellVolume.Add(volume)
		}
	}

	if buyVolume.IsZero() && sellVolume.IsZero() {
		return decimal.Zero
	}

	// Calculate balance ratio (closer to 1.0 means more balanced)
	if buyVolume.IsZero() || sellVolume.IsZero() {
		return decimal.Zero
	}

	smaller := buyVolume
	larger := sellVolume
	if sellVolume.LessThan(buyVolume) {
		smaller = sellVolume
		larger = buyVolume
	}

	balance := smaller.Div(larger)
	return balance.Mul(decimal.NewFromInt(100)) // Convert to percentage
}

// calculateTotalVolume calculates total trading volume
func (wtd *ConsolidatedWashTradingDetector) calculateTotalVolume(trades []Trade) decimal.Decimal {
	totalVolume := decimal.Zero
	for _, trade := range trades {
		volume := trade.Size.Mul(trade.Price)
		totalVolume = totalVolume.Add(volume)
	}
	return totalVolume
}

// calculateBaseConfidence calculates base confidence score from indicators
func (wtd *ConsolidatedWashTradingDetector) calculateBaseConfidence(indicators WashTradingIndicators) decimal.Decimal {
	// Weighted combination of indicators
	score := decimal.Zero

	// Self trading ratio (40% weight)
	score = score.Add(indicators.SelfTradingRatio.Mul(decimal.NewFromFloat(0.4)))

	// Volume inflation ratio (30% weight)
	inflationScore := indicators.VolumeInflationRatio
	if inflationScore.GreaterThan(decimal.NewFromInt(100)) {
		inflationScore = decimal.NewFromInt(100) // Cap at 100
	}
	score = score.Add(inflationScore.Mul(decimal.NewFromFloat(0.3)))

	// Order sequence score (20% weight)
	score = score.Add(indicators.OrderSequenceScore.Mul(decimal.NewFromFloat(0.2)))

	// Balance consistency (10% weight)
	score = score.Add(indicators.BalanceConsistency.Mul(decimal.NewFromFloat(0.1)))

	return score
}

// buildWashTradingEvidence builds evidence list for wash trading detection
func (wtd *ConsolidatedWashTradingDetector) buildWashTradingEvidence(indicators WashTradingIndicators) []Evidence {
	var evidence []Evidence

	// Self trading evidence
	if indicators.SelfTradingRatio.GreaterThan(decimal.NewFromInt(50)) {
		evidence = append(evidence, wtd.evidenceBuilder.AddEvidence(
			"self_trading",
			"High self-trading ratio detected",
			map[string]interface{}{
				"ratio":     indicators.SelfTradingRatio.String(),
				"threshold": "50%",
			},
			decimal.NewFromFloat(0.9),
		))
	}

	// Volume inflation evidence
	if indicators.VolumeInflationRatio.GreaterThan(decimal.NewFromInt(25)) {
		evidence = append(evidence, wtd.evidenceBuilder.AddEvidence(
			"volume_inflation",
			"Suspicious volume inflation detected",
			map[string]interface{}{
				"inflation_ratio": indicators.VolumeInflationRatio.String(),
				"threshold":       "25%",
			},
			decimal.NewFromFloat(0.8),
		))
	}

	// Time clustering evidence
	if indicators.TimeClustering >= 75.0 {
		evidence = append(evidence, wtd.evidenceBuilder.BuildTimingEvidence(
			"wash_trading",
			map[string]interface{}{
				"clustering_score": indicators.TimeClustering,
				"threshold":        75.0,
			},
		))
	}

	// Sequential patterns evidence
	if indicators.SequentialPatterns >= 2 {
		evidence = append(evidence, wtd.evidenceBuilder.AddEvidence(
			"pattern",
			"Alternating buy/sell patterns detected",
			map[string]interface{}{
				"pattern_count": indicators.SequentialPatterns,
				"threshold":     2,
			},
			decimal.NewFromFloat(0.7),
		))
	}

	return evidence
}
