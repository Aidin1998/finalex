package manipulation

import (
	"fmt"
	"math"
	"time"

	"github.com/Aidin1998/finalex/internal/compliance/interfaces"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// ConsolidatedDetectorAdapter adapts a consolidated pattern framework to work with manipulation detection
// All types are referenced from the interfaces package. Detector logic is stubbed.
type ConsolidatedDetectorAdapter struct {
	logger    *zap.SugaredLogger
	threshold decimal.Decimal
	config    AdapterConfig
}

// AdapterConfig holds configuration for the consolidated adapter
type AdapterConfig struct {
	WashTradingThreshold decimal.Decimal
	MinOrderCount        int
	MinTradeCount        int
	TimeWindow           time.Duration
	MinVolume            decimal.Decimal
}

// NewConsolidatedDetectorAdapter creates a new adapter with proper configuration
func NewConsolidatedDetectorAdapter(logger *zap.SugaredLogger) *ConsolidatedDetectorAdapter {
	return &ConsolidatedDetectorAdapter{
		logger:    logger,
		threshold: decimal.NewFromFloat(70.0), // Default threshold for detection
		config: AdapterConfig{
			WashTradingThreshold: decimal.NewFromFloat(70.0),
			MinOrderCount:        5,
			MinTradeCount:        3,
			TimeWindow:           30 * time.Minute,
			MinVolume:            decimal.NewFromFloat(1000.0),
		},
	}
}

// NewConsolidatedDetectorAdapterWithConfig creates a new adapter with custom configuration
func NewConsolidatedDetectorAdapterWithConfig(logger *zap.SugaredLogger, config AdapterConfig) *ConsolidatedDetectorAdapter {
	return &ConsolidatedDetectorAdapter{
		logger:    logger,
		threshold: config.WashTradingThreshold,
		config:    config,
	}
}

// UpdateConfig updates the adapter configuration
func (cda *ConsolidatedDetectorAdapter) UpdateConfig(config AdapterConfig) {
	cda.config = config
	cda.threshold = config.WashTradingThreshold
	cda.logger.Infow("Consolidated adapter configuration updated",
		"wash_trading_threshold", config.WashTradingThreshold.String(),
		"min_order_count", config.MinOrderCount,
		"min_trade_count", config.MinTradeCount,
		"time_window", config.TimeWindow.String(),
		"min_volume", config.MinVolume.String())
}

// GetConfig returns the current adapter configuration
func (cda *ConsolidatedDetectorAdapter) GetConfig() AdapterConfig {
	return cda.config
}

// DefaultAdapterConfig returns a default configuration for the consolidated adapter
func DefaultAdapterConfig() AdapterConfig {
	return AdapterConfig{
		WashTradingThreshold: decimal.NewFromFloat(70.0),
		MinOrderCount:        5,
		MinTradeCount:        3,
		TimeWindow:           30 * time.Minute,
		MinVolume:            decimal.NewFromFloat(1000.0),
	}
}

// ValidateConfig validates the adapter configuration
func (cda *ConsolidatedDetectorAdapter) ValidateConfig() error {
	if cda.config.WashTradingThreshold.LessThan(decimal.Zero) || cda.config.WashTradingThreshold.GreaterThan(decimal.NewFromInt(100)) {
		return fmt.Errorf("wash trading threshold must be between 0 and 100, got %s", cda.config.WashTradingThreshold.String())
	}
	if cda.config.MinOrderCount < 1 {
		return fmt.Errorf("min order count must be at least 1, got %d", cda.config.MinOrderCount)
	}
	if cda.config.MinTradeCount < 1 {
		return fmt.Errorf("min trade count must be at least 1, got %d", cda.config.MinTradeCount)
	}
	if cda.config.TimeWindow <= 0 {
		return fmt.Errorf("time window must be positive, got %s", cda.config.TimeWindow.String())
	}
	if cda.config.MinVolume.LessThan(decimal.Zero) {
		return fmt.Errorf("min volume must be non-negative, got %s", cda.config.MinVolume.String())
	}
	return nil
}

// ConvertTradingActivity converts and validates trading activity data
func (cda *ConsolidatedDetectorAdapter) ConvertTradingActivity(activity *interfaces.TradingActivity) *interfaces.TradingActivity {
	if activity == nil {
		cda.logger.Warn("ConvertTradingActivity: received nil activity")
		return nil
	}

	// Create a copy to avoid modifying the original
	converted := &interfaces.TradingActivity{
		UserID:     activity.UserID,
		Market:     activity.Market,
		TimeWindow: activity.TimeWindow,
		StartTime:  activity.StartTime,
		EndTime:    activity.EndTime,
		Orders:     make([]interfaces.Order, len(activity.Orders)),
		Trades:     make([]interfaces.Trade, len(activity.Trades)),
	}
	// Validate and normalize orders
	validOrders := 0
	for _, order := range activity.Orders {
		// Validate order data
		if order.ID == "" || order.Quantity.LessThanOrEqual(decimal.Zero) || order.Price.LessThanOrEqual(decimal.Zero) {
			cda.logger.Debugf("Skipping invalid order: ID=%s, Qty=%s, Price=%s", order.ID, order.Quantity.String(), order.Price.String())
			continue
		}

		// Normalize order fields
		converted.Orders[validOrders] = interfaces.Order{
			ID:          order.ID,
			UserID:      order.UserID,
			Market:      order.Market,
			Side:        cda.normalizeSide(order.Side),
			Type:        cda.normalizeOrderType(order.Type),
			Quantity:    order.Quantity.Round(8), // Round to 8 decimal places
			Price:       order.Price.Round(8),
			Status:      cda.normalizeOrderStatus(order.Status),
			CreatedAt:   order.CreatedAt,
			UpdatedAt:   order.UpdatedAt,
			ExecutedAt:  order.ExecutedAt,
			CancelledAt: order.CancelledAt,
		}
		validOrders++
	}
	converted.Orders = converted.Orders[:validOrders]
	// Validate and normalize trades
	validTrades := 0
	for _, trade := range activity.Trades {
		// Validate trade data
		if trade.ID == "" || trade.Quantity.LessThanOrEqual(decimal.Zero) || trade.Price.LessThanOrEqual(decimal.Zero) {
			cda.logger.Debugf("Skipping invalid trade: ID=%s, Qty=%s, Price=%s", trade.ID, trade.Quantity.String(), trade.Price.String())
			continue
		}

		// Normalize trade fields
		converted.Trades[validTrades] = interfaces.Trade{
			ID:        trade.ID,
			Market:    trade.Market,
			BuyerID:   trade.BuyerID,
			SellerID:  trade.SellerID,
			Quantity:  trade.Quantity.Round(8),
			Price:     trade.Price.Round(8),
			Timestamp: trade.Timestamp,
			OrderIDs:  trade.OrderIDs,
		}
		validTrades++
	}
	converted.Trades = converted.Trades[:validTrades]

	// Set time window if not provided
	if converted.TimeWindow == 0 && !converted.StartTime.IsZero() && !converted.EndTime.IsZero() {
		converted.TimeWindow = converted.EndTime.Sub(converted.StartTime)
	}

	cda.logger.Debugf("ConvertTradingActivity: converted %d orders and %d trades for user %s in market %s",
		len(converted.Orders), len(converted.Trades), activity.UserID.String(), activity.Market)

	return converted
}

// ConvertDetectionPattern converts different pattern types to ManipulationPattern
func (cda *ConsolidatedDetectorAdapter) ConvertDetectionPattern(pattern interface{}) *interfaces.ManipulationPattern {
	if pattern == nil {
		return nil
	}

	// Try to convert from different pattern types
	switch p := pattern.(type) {
	case *interfaces.ManipulationPattern:
		// Already the correct type
		return p

	case interfaces.ManipulationPattern:
		// Convert value to pointer
		return &p

	case map[string]interface{}:
		// Convert from generic map (common in JSON scenarios)
		return cda.convertFromMap(p)

	case *ManipulationPattern:
		// Convert from local ManipulationPattern type
		return cda.convertFromLocalPattern(p)

	case ManipulationPattern:
		// Convert from local ManipulationPattern type (value)
		return cda.convertFromLocalPattern(&p)

	default:
		cda.logger.Warnf("ConvertDetectionPattern: unsupported pattern type %T", pattern)
		return nil
	}
}

// DetectWashTrading implements comprehensive wash trading detection algorithm
func (cda *ConsolidatedDetectorAdapter) DetectWashTrading(activity *interfaces.TradingActivity) *interfaces.ManipulationPattern {
	if activity == nil {
		return nil
	}

	// Validate minimum data requirements
	if len(activity.Orders) < cda.config.MinOrderCount || len(activity.Trades) < cda.config.MinTradeCount {
		cda.logger.Debugf("DetectWashTrading: insufficient data - orders: %d, trades: %d", len(activity.Orders), len(activity.Trades))
		return nil
	}

	// Calculate total volume to check minimum threshold
	totalVolume := cda.calculateTotalVolume(activity)
	if totalVolume.LessThan(cda.config.MinVolume) {
		cda.logger.Debugf("DetectWashTrading: volume below threshold - total: %s, min: %s", totalVolume.String(), cda.config.MinVolume.String())
		return nil
	}

	cda.logger.Debugf("DetectWashTrading: analyzing activity for user %s in market %s with %d orders and %d trades",
		activity.UserID.String(), activity.Market, len(activity.Orders), len(activity.Trades))

	// Calculate wash trading indicators
	indicators := cda.calculateWashTradingIndicators(activity)

	// Calculate overall confidence score
	confidence := cda.calculateWashTradingConfidence(indicators)

	// Check if confidence exceeds threshold
	if confidence.GreaterThan(cda.config.WashTradingThreshold) {
		pattern := &interfaces.ManipulationPattern{
			Type:        interfaces.ManipulationAlertWashTrading,
			Confidence:  confidence,
			Description: cda.generateWashTradingDescription(indicators, confidence),
			Evidence:    cda.buildWashTradingEvidence(indicators),
			TimeWindow:  activity.TimeWindow,
			DetectedAt:  time.Now(),
			Metadata: map[string]interface{}{
				"user_id":                activity.UserID.String(),
				"market":                 activity.Market,
				"total_orders":           len(activity.Orders),
				"total_trades":           len(activity.Trades),
				"total_volume":           totalVolume.String(),
				"self_trading_ratio":     indicators.SelfTradingRatio.String(),
				"volume_inflation_ratio": indicators.VolumeInflationRatio.String(),
				"price_impact":           indicators.PriceImpact.String(),
				"time_clustering":        indicators.TimeClustering,
				"sequential_patterns":    indicators.SequentialPatterns,
				"price_level_reuse":      indicators.PriceLevelReuse,
				"detection_algorithm":    "consolidated_wash_trading_v1.0",
			},
		}

		cda.logger.Infof("DetectWashTrading: wash trading detected for user %s in market %s with confidence %.2f%%",
			activity.UserID.String(), activity.Market, confidence.InexactFloat64())

		return pattern
	}

	cda.logger.Debugf("DetectWashTrading: no wash trading detected - confidence: %.2f%%, threshold: %.2f%%",
		confidence.InexactFloat64(), cda.config.WashTradingThreshold.InexactFloat64())

	return nil
}

// Helper methods for data normalization and validation

// normalizeSide normalizes order side to standard format
func (cda *ConsolidatedDetectorAdapter) normalizeSide(side string) string {
	switch side {
	case "buy", "BUY", "Buy":
		return "buy"
	case "sell", "SELL", "Sell":
		return "sell"
	default:
		return side
	}
}

// normalizeOrderType normalizes order type to standard format
func (cda *ConsolidatedDetectorAdapter) normalizeOrderType(orderType string) string {
	switch orderType {
	case "market", "MARKET", "Market":
		return "market"
	case "limit", "LIMIT", "Limit":
		return "limit"
	case "stop", "STOP", "Stop":
		return "stop"
	case "stop_limit", "STOP_LIMIT", "Stop_Limit":
		return "stop_limit"
	default:
		return orderType
	}
}

// normalizeOrderStatus normalizes order status to standard format
func (cda *ConsolidatedDetectorAdapter) normalizeOrderStatus(status string) string {
	switch status {
	case "pending", "PENDING", "Pending":
		return "pending"
	case "filled", "FILLED", "Filled":
		return "filled"
	case "cancelled", "CANCELLED", "Cancelled":
		return "cancelled"
	case "partially_filled", "PARTIALLY_FILLED", "Partially_Filled":
		return "partially_filled"
	default:
		return status
	}
}

// Helper methods for pattern conversion

// convertFromMap converts a map to ManipulationPattern
func (cda *ConsolidatedDetectorAdapter) convertFromMap(m map[string]interface{}) *interfaces.ManipulationPattern {
	pattern := &interfaces.ManipulationPattern{
		DetectedAt: time.Now(),
		Metadata:   make(map[string]interface{}),
	}

	if patternType, ok := m["type"].(string); ok {
		pattern.Type = cda.stringToAlertType(patternType)
	}

	if confidence, ok := m["confidence"].(float64); ok {
		pattern.Confidence = decimal.NewFromFloat(confidence)
	} else if confidenceStr, ok := m["confidence"].(string); ok {
		if conf, err := decimal.NewFromString(confidenceStr); err == nil {
			pattern.Confidence = conf
		}
	}

	if description, ok := m["description"].(string); ok {
		pattern.Description = description
	}

	if timeWindow, ok := m["time_window"].(time.Duration); ok {
		pattern.TimeWindow = timeWindow
	}

	if detectedAt, ok := m["detected_at"].(time.Time); ok {
		pattern.DetectedAt = detectedAt
	}

	if metadata, ok := m["metadata"].(map[string]interface{}); ok {
		pattern.Metadata = metadata
	}

	return pattern
}

// convertFromLocalPattern converts from local ManipulationPattern to interfaces.ManipulationPattern
func (cda *ConsolidatedDetectorAdapter) convertFromLocalPattern(p *ManipulationPattern) *interfaces.ManipulationPattern {
	if p == nil {
		return nil
	}

	// Convert evidence
	evidence := make([]interfaces.PatternEvidence, len(p.Evidence))
	for i, e := range p.Evidence {
		evidence[i] = interfaces.PatternEvidence{
			Type:        e.Type,
			Description: e.Description,
			Value:       e.Data,
			Timestamp:   e.Timestamp,
			Metadata:    nil,
		}
	}

	return &interfaces.ManipulationPattern{
		Type:        cda.stringToAlertType(p.Type),
		Confidence:  p.Confidence,
		Description: p.Description,
		Evidence:    evidence,
		TimeWindow:  0, // Not available in local type
		DetectedAt:  p.DetectedAt,
		Metadata:    p.Metadata,
	}
}

// stringToAlertType converts string to ManipulationAlertType
func (cda *ConsolidatedDetectorAdapter) stringToAlertType(s string) interfaces.ManipulationAlertType {
	switch s {
	case "wash_trading":
		return interfaces.ManipulationAlertWashTrading
	case "spoofing":
		return interfaces.ManipulationAlertSpoofing
	case "layering":
		return interfaces.ManipulationAlertLayering
	case "pump_and_dump", "pump_dump":
		return interfaces.ManipulationAlertPumpAndDump
	case "front_running":
		return interfaces.ManipulationAlertFrontRunning
	case "insider_trading":
		return interfaces.ManipulationAlertInsiderTrading
	default:
		return interfaces.ManipulationAlertWashTrading // Default fallback
	}
}

// Helper methods for wash trading detection

// calculateTotalVolume calculates total trading volume
func (cda *ConsolidatedDetectorAdapter) calculateTotalVolume(activity *interfaces.TradingActivity) decimal.Decimal {
	var totalVolume decimal.Decimal
	for _, trade := range activity.Trades {
		volume := trade.Quantity.Mul(trade.Price)
		totalVolume = totalVolume.Add(volume)
	}
	return totalVolume
}

// calculateWashTradingIndicators calculates comprehensive wash trading indicators
func (cda *ConsolidatedDetectorAdapter) calculateWashTradingIndicators(activity *interfaces.TradingActivity) WashTradingIndicators {
	indicators := WashTradingIndicators{}

	// 1. Self-trading ratio estimation
	indicators.SelfTradingRatio = cda.estimateSelfTradingRatio(activity)

	// 2. Volume inflation ratio
	indicators.VolumeInflationRatio = cda.calculateVolumeInflationRatio(activity)

	// 3. Average price impact
	indicators.PriceImpact = cda.calculateAveragePriceImpact(activity)

	// 4. Time clustering analysis
	indicators.TimeClustering = cda.calculateTimeClustering(activity)

	// 5. Sequential pattern detection
	indicators.SequentialPatterns = cda.detectSequentialPatterns(activity)

	// 6. Price level reuse analysis
	indicators.PriceLevelReuse = cda.calculatePriceLevelReuse(activity)

	return indicators
}

// estimateSelfTradingRatio estimates self-trading based on order patterns
func (cda *ConsolidatedDetectorAdapter) estimateSelfTradingRatio(activity *interfaces.TradingActivity) decimal.Decimal {
	if len(activity.Orders) < 2 {
		return decimal.Zero
	}

	var suspiciousOrders int
	var buyOrders, sellOrders []interfaces.Order

	// Separate buy and sell orders
	for _, order := range activity.Orders {
		if order.Side == "buy" {
			buyOrders = append(buyOrders, order)
		} else if order.Side == "sell" {
			sellOrders = append(sellOrders, order)
		}
	}

	// Look for matching buy/sell patterns
	for _, buyOrder := range buyOrders {
		for _, sellOrder := range sellOrders {
			if cda.areSuspiciouslyMatched(buyOrder, sellOrder) {
				suspiciousOrders++
				break
			}
		}
	}

	if len(activity.Orders) == 0 {
		return decimal.Zero
	}

	return decimal.NewFromInt(int64(suspiciousOrders)).Div(decimal.NewFromInt(int64(len(activity.Orders))))
}

// areSuspiciouslyMatched checks if two orders are suspiciously matched
func (cda *ConsolidatedDetectorAdapter) areSuspiciouslyMatched(buyOrder, sellOrder interfaces.Order) bool {
	// Check price similarity (within 0.1%)
	priceDiff := buyOrder.Price.Sub(sellOrder.Price).Abs()
	priceThreshold := buyOrder.Price.Mul(decimal.NewFromFloat(0.001))
	if priceDiff.GreaterThan(priceThreshold) {
		return false
	}

	// Check quantity similarity (within 5%)
	qtyDiff := buyOrder.Quantity.Sub(sellOrder.Quantity).Abs()
	qtyThreshold := buyOrder.Quantity.Mul(decimal.NewFromFloat(0.05))
	if qtyDiff.GreaterThan(qtyThreshold) {
		return false
	}

	// Check timing (within 30 seconds)
	timeDiff := buyOrder.CreatedAt.Sub(sellOrder.CreatedAt)
	if timeDiff < 0 {
		timeDiff = -timeDiff
	}

	return timeDiff <= 30*time.Second
}

// calculateVolumeInflationRatio calculates potentially inflated volume ratio
func (cda *ConsolidatedDetectorAdapter) calculateVolumeInflationRatio(activity *interfaces.TradingActivity) decimal.Decimal {
	if len(activity.Trades) == 0 {
		return decimal.Zero
	}

	var totalVolume, suspiciousVolume decimal.Decimal

	for _, trade := range activity.Trades {
		volume := trade.Quantity.Mul(trade.Price)
		totalVolume = totalVolume.Add(volume)

		// Mark volume as suspicious if trade appears to be self-trading
		if cda.isLikelySelfTrade(trade, activity) {
			suspiciousVolume = suspiciousVolume.Add(volume)
		}
	}

	if totalVolume.IsZero() {
		return decimal.Zero
	}

	return suspiciousVolume.Div(totalVolume)
}

// isLikelySelfTrade checks if a trade is likely self-trading
func (cda *ConsolidatedDetectorAdapter) isLikelySelfTrade(trade interfaces.Trade, activity *interfaces.TradingActivity) bool {
	// Check if buyer and seller are the same (if available)
	if trade.BuyerID == trade.SellerID && trade.BuyerID == activity.UserID {
		return true
	}

	// Check for minimal price impact
	return cda.hasMinimalPriceImpact(trade, activity.Trades)
}

// hasMinimalPriceImpact checks if a trade has minimal price impact
func (cda *ConsolidatedDetectorAdapter) hasMinimalPriceImpact(trade interfaces.Trade, allTrades []interfaces.Trade) bool {
	var beforePrice, afterPrice decimal.Decimal
	found := false

	for i, t := range allTrades {
		if t.ID == trade.ID {
			if i > 0 {
				beforePrice = allTrades[i-1].Price
			}
			if i < len(allTrades)-1 {
				afterPrice = allTrades[i+1].Price
				found = true
			}
			break
		}
	}

	if !found || beforePrice.IsZero() || afterPrice.IsZero() {
		return false
	}

	// Calculate price impact
	priceChange := afterPrice.Sub(beforePrice).Abs()
	priceImpact := priceChange.Div(beforePrice).Mul(decimal.NewFromInt(100))

	// Consider impact minimal if less than 0.01%
	return priceImpact.LessThan(decimal.NewFromFloat(0.01))
}

// calculateAveragePriceImpact calculates average price impact of trades
func (cda *ConsolidatedDetectorAdapter) calculateAveragePriceImpact(activity *interfaces.TradingActivity) decimal.Decimal {
	if len(activity.Trades) <= 1 {
		return decimal.Zero
	}

	var totalImpact decimal.Decimal
	var count int

	for i := 1; i < len(activity.Trades); i++ {
		currentPrice := activity.Trades[i].Price
		previousPrice := activity.Trades[i-1].Price

		if !previousPrice.IsZero() {
			impact := currentPrice.Sub(previousPrice).Abs().Div(previousPrice)
			totalImpact = totalImpact.Add(impact)
			count++
		}
	}

	if count == 0 {
		return decimal.Zero
	}

	return totalImpact.Div(decimal.NewFromInt(int64(count)))
}

// calculateTimeClustering calculates time clustering coefficient
func (cda *ConsolidatedDetectorAdapter) calculateTimeClustering(activity *interfaces.TradingActivity) float64 {
	if len(activity.Orders) <= 2 {
		return 0.0
	}

	// Calculate variance in order timing
	var intervals []float64
	for i := 1; i < len(activity.Orders); i++ {
		interval := activity.Orders[i].CreatedAt.Sub(activity.Orders[i-1].CreatedAt).Seconds()
		intervals = append(intervals, interval)
	}

	if len(intervals) == 0 {
		return 0.0
	}

	// Calculate coefficient of variation
	mean := 0.0
	for _, interval := range intervals {
		mean += interval
	}
	mean /= float64(len(intervals))

	variance := 0.0
	for _, interval := range intervals {
		variance += math.Pow(interval-mean, 2)
	}
	variance /= float64(len(intervals))

	stdDev := math.Sqrt(variance)

	if mean == 0 {
		return 0.0
	}

	// High clustering = low coefficient of variation
	coeffVar := stdDev / mean
	return math.Max(0, 1.0-coeffVar)
}

// detectSequentialPatterns detects alternating buy/sell patterns
func (cda *ConsolidatedDetectorAdapter) detectSequentialPatterns(activity *interfaces.TradingActivity) int {
	if len(activity.Orders) < 4 {
		return 0
	}

	var patterns int
	var lastSide string
	var samePrice int

	for i, order := range activity.Orders {
		if i == 0 {
			lastSide = order.Side
			continue
		}

		// Check for alternating pattern
		if order.Side != lastSide {
			// Check if at similar price level
			if i > 0 {
				priceDiff := order.Price.Sub(activity.Orders[i-1].Price).Abs()
				priceThreshold := order.Price.Mul(decimal.NewFromFloat(0.001))

				if priceDiff.LessThan(priceThreshold) {
					samePrice++
					if samePrice >= 3 {
						patterns++
						samePrice = 0
					}
				} else {
					samePrice = 0
				}
			}
		} else {
			samePrice = 0
		}

		lastSide = order.Side
	}

	return patterns
}

// calculatePriceLevelReuse calculates price level reuse
func (cda *ConsolidatedDetectorAdapter) calculatePriceLevelReuse(activity *interfaces.TradingActivity) int {
	if len(activity.Orders) < 5 {
		return 0
	}

	priceCount := make(map[string]int)

	// Count occurrences of each price level
	for _, order := range activity.Orders {
		priceKey := order.Price.Round(4).String()
		priceCount[priceKey]++
	}

	// Count price levels used multiple times
	var reuseCount int
	for _, count := range priceCount {
		if count >= 3 {
			reuseCount += count - 2
		}
	}

	return reuseCount
}

// calculateWashTradingConfidence calculates overall confidence score
func (cda *ConsolidatedDetectorAdapter) calculateWashTradingConfidence(indicators WashTradingIndicators) decimal.Decimal {
	var score decimal.Decimal

	// Weight factors for different indicators
	weights := map[string]decimal.Decimal{
		"self_trading":     decimal.NewFromFloat(40),
		"volume_inflation": decimal.NewFromFloat(25),
		"time_clustering":  decimal.NewFromFloat(15),
		"sequential":       decimal.NewFromFloat(10),
		"price_reuse":      decimal.NewFromFloat(10),
	}

	// Self-trading ratio (0-40 points)
	selfTradingScore := indicators.SelfTradingRatio.Mul(weights["self_trading"])
	score = score.Add(selfTradingScore)

	// Volume inflation ratio (0-25 points)
	volumeScore := indicators.VolumeInflationRatio.Mul(weights["volume_inflation"])
	score = score.Add(volumeScore)

	// Time clustering (0-15 points)
	clusteringScore := decimal.NewFromFloat(indicators.TimeClustering).Mul(weights["time_clustering"])
	score = score.Add(clusteringScore)

	// Sequential patterns (0-10 points)
	if indicators.SequentialPatterns > 0 {
		sequentialScore := decimal.Min(decimal.NewFromInt(int64(indicators.SequentialPatterns*2)), weights["sequential"])
		score = score.Add(sequentialScore)
	}

	// Price level reuse (0-10 points)
	if indicators.PriceLevelReuse > 0 {
		reuseScore := decimal.Min(decimal.NewFromInt(int64(indicators.PriceLevelReuse)), weights["price_reuse"])
		score = score.Add(reuseScore)
	}

	// Cap at 100
	return decimal.Min(score, decimal.NewFromInt(100))
}

// generateWashTradingDescription generates a descriptive message
func (cda *ConsolidatedDetectorAdapter) generateWashTradingDescription(indicators WashTradingIndicators, confidence decimal.Decimal) string {
	severityLevel := "medium"
	if confidence.GreaterThan(decimal.NewFromFloat(90)) {
		severityLevel = "critical"
	} else if confidence.GreaterThan(decimal.NewFromFloat(75)) {
		severityLevel = "high"
	} else if confidence.GreaterThan(decimal.NewFromFloat(50)) {
		severityLevel = "medium"
	} else {
		severityLevel = "low"
	}

	return fmt.Sprintf("Wash trading pattern detected with %s severity (%.1f%% confidence). "+
		"Key indicators: self-trading ratio %.1f%%, volume inflation %.1f%%, sequential patterns %d, price reuse %d.",
		severityLevel, confidence.InexactFloat64(),
		indicators.SelfTradingRatio.Mul(decimal.NewFromInt(100)).InexactFloat64(),
		indicators.VolumeInflationRatio.Mul(decimal.NewFromInt(100)).InexactFloat64(),
		indicators.SequentialPatterns, indicators.PriceLevelReuse)
}

// buildWashTradingEvidence builds evidence list for wash trading detection
func (cda *ConsolidatedDetectorAdapter) buildWashTradingEvidence(indicators WashTradingIndicators) []interfaces.PatternEvidence {
	var evidence []interfaces.PatternEvidence

	if indicators.SelfTradingRatio.GreaterThan(decimal.NewFromFloat(0.3)) {
		evidence = append(evidence, interfaces.PatternEvidence{
			Type:        "pattern",
			Description: "High self-trading ratio detected",
			Value:       map[string]interface{}{"ratio": indicators.SelfTradingRatio.String()},
			Timestamp:   time.Now(),
		})
	}

	if indicators.VolumeInflationRatio.GreaterThan(decimal.NewFromFloat(0.5)) {
		evidence = append(evidence, interfaces.PatternEvidence{
			Type:        "volume",
			Description: "Artificially inflated volume patterns",
			Value:       map[string]interface{}{"ratio": indicators.VolumeInflationRatio.String()},
			Timestamp:   time.Now(),
		})
	}

	if indicators.TimeClustering > 0.7 {
		evidence = append(evidence, interfaces.PatternEvidence{
			Type:        "timing",
			Description: "Suspicious time clustering in orders",
			Value:       map[string]interface{}{"clustering": indicators.TimeClustering},
			Timestamp:   time.Now(),
		})
	}

	if indicators.SequentialPatterns >= 2 {
		evidence = append(evidence, interfaces.PatternEvidence{
			Type:        "pattern",
			Description: "Alternating buy/sell patterns detected",
			Value:       map[string]interface{}{"count": indicators.SequentialPatterns},
			Timestamp:   time.Now(),
		})
	}

	if indicators.PriceLevelReuse >= 3 {
		evidence = append(evidence, interfaces.PatternEvidence{
			Type:        "price",
			Description: "Repeated use of same price levels",
			Value:       map[string]interface{}{"reuse_count": indicators.PriceLevelReuse},
			Timestamp:   time.Now(),
		})
	}

	return evidence
}
