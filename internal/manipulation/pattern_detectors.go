package manipulation

import (
	"math"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"pincex/internal/model"
)

// =======================
// WASH TRADING DETECTOR
// =======================

// WashTradingDetector detects wash trading patterns (self-trading to inflate volume)
type WashTradingDetector struct {
	config     DetectionConfig
	logger     *zap.SugaredLogger
	threshold  decimal.Decimal
	minVolume  decimal.Decimal
	timeWindow time.Duration
}

// NewWashTradingDetector creates a new wash trading detector
func NewWashTradingDetector(config DetectionConfig, logger *zap.SugaredLogger) *WashTradingDetector {
	return &WashTradingDetector{
		config:     config,
		logger:     logger,
		threshold:  config.WashTradingThreshold,
		minVolume:  decimal.NewFromFloat(1000), // Minimum volume to trigger analysis
		timeWindow: time.Duration(config.DetectionWindowMinutes) * time.Minute,
	}
}

// Detect analyzes trading activity for wash trading patterns
func (wtd *WashTradingDetector) Detect(activity *TradingActivity) *ManipulationPattern {
	if len(activity.Orders) < 10 || len(activity.Trades) < 5 {
		return nil // Insufficient data
	}

	// Calculate wash trading indicators
	indicators := wtd.calculateWashTradingIndicators(activity)

	// Check for suspicious patterns
	confidence := wtd.calculateWashTradingConfidence(indicators)

	if confidence.GreaterThan(wtd.threshold) {
		return &ManipulationPattern{
			Type:        "wash_trading",
			Description: "Detected potential wash trading (self-trading to inflate volume)",
			Confidence:  confidence,
			Severity:    wtd.determineSeverity(confidence),
			Evidence:    wtd.buildWashTradingEvidence(indicators),
			DetectedAt:  time.Now(),
			Metadata: map[string]interface{}{
				"volume_inflation_ratio": indicators.VolumeInflationRatio.String(),
				"self_trading_ratio":     indicators.SelfTradingRatio.String(),
				"price_impact":           indicators.PriceImpact.String(),
				"time_clustering":        indicators.TimeClustering,
			},
		}
	}

	return nil
}

// WashTradingIndicators holds calculated indicators for wash trading detection
type WashTradingIndicators struct {
	SelfTradingRatio     decimal.Decimal
	VolumeInflationRatio decimal.Decimal
	PriceImpact          decimal.Decimal
	TimeClustering       float64
	SequentialPatterns   int
	PriceLevelReuse      int
}

// calculateWashTradingIndicators calculates key indicators for wash trading
func (wtd *WashTradingDetector) calculateWashTradingIndicators(activity *TradingActivity) WashTradingIndicators {
	indicators := WashTradingIndicators{}

	// 1. Self-trading ratio - trades where user is both maker and taker
	// Note: In a real implementation, this would require cross-referencing order IDs
	// For now, we simulate based on timing patterns
	indicators.SelfTradingRatio = wtd.estimateSelfTradingRatio(activity)

	// 2. Volume inflation ratio - artificially inflated volume vs organic volume
	indicators.VolumeInflationRatio = wtd.calculateVolumeInflationRatio(activity)

	// 3. Price impact analysis - trades that don't move price significantly
	indicators.PriceImpact = wtd.calculateAveragePriceImpact(activity)

	// 4. Time clustering - orders/trades occurring in suspicious time patterns
	indicators.TimeClustering = wtd.calculateTimeClustering(activity)

	// 5. Sequential patterns - alternating buy/sell orders at same prices
	indicators.SequentialPatterns = wtd.detectSequentialPatterns(activity)

	// 6. Price level reuse - same price levels used repeatedly
	indicators.PriceLevelReuse = wtd.calculatePriceLevelReuse(activity)

	return indicators
}

// estimateSelfTradingRatio estimates the ratio of self-trading based on patterns
func (wtd *WashTradingDetector) estimateSelfTradingRatio(activity *TradingActivity) decimal.Decimal {
	if len(activity.Orders) < 2 {
		return decimal.Zero
	}

	var suspiciousOrders int
	var buyOrders, sellOrders []*model.Order

	// Separate buy and sell orders
	for _, order := range activity.Orders {
		if order.Side == "buy" {
			buyOrders = append(buyOrders, order)
		} else {
			sellOrders = append(sellOrders, order)
		}
	}

	// Look for matching buy/sell patterns at similar prices and times
	for _, buyOrder := range buyOrders {
		for _, sellOrder := range sellOrders {
			// Check if orders are suspiciously similar
			if wtd.areSuspiciouslyMatched(buyOrder, sellOrder) {
				suspiciousOrders++
				break // Count each buy order only once
			}
		}
	}

	if len(activity.Orders) == 0 {
		return decimal.Zero
	}

	return decimal.NewFromInt(int64(suspiciousOrders)).Div(decimal.NewFromInt(int64(len(activity.Orders))))
}

// areSuspiciouslyMatched checks if two orders are suspiciously matched
func (wtd *WashTradingDetector) areSuspiciouslyMatched(buyOrder, sellOrder *model.Order) bool {
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

// calculateVolumeInflationRatio calculates the ratio of potentially inflated volume
func (wtd *WashTradingDetector) calculateVolumeInflationRatio(activity *TradingActivity) decimal.Decimal {
	if len(activity.Trades) == 0 {
		return decimal.Zero
	}

	var totalVolume, suspiciousVolume decimal.Decimal

	for _, trade := range activity.Trades {
		volume := trade.Quantity.Mul(trade.Price)
		totalVolume = totalVolume.Add(volume)

		// Mark volume as suspicious if trade has minimal price impact
		// This is a simplified heuristic
		if trade.Maker && wtd.hasMinimalPriceImpact(trade, activity.Trades) {
			suspiciousVolume = suspiciousVolume.Add(volume)
		}
	}

	if totalVolume.IsZero() {
		return decimal.Zero
	}

	return suspiciousVolume.Div(totalVolume)
}

// hasMinimalPriceImpact checks if a trade has minimal price impact
func (wtd *WashTradingDetector) hasMinimalPriceImpact(trade *model.Trade, allTrades []*model.Trade) bool {
	// Find trades before and after this trade
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

	// Calculate price impact as percentage change
	priceChange := afterPrice.Sub(beforePrice).Abs()
	priceImpact := priceChange.Div(beforePrice).Mul(decimal.NewFromInt(100))

	// Consider impact minimal if less than 0.01%
	return priceImpact.LessThan(decimal.NewFromFloat(0.01))
}

// calculateAveragePriceImpact calculates the average price impact of trades
func (wtd *WashTradingDetector) calculateAveragePriceImpact(activity *TradingActivity) decimal.Decimal {
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
func (wtd *WashTradingDetector) calculateTimeClustering(activity *TradingActivity) float64 {
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

	// Calculate coefficient of variation (std dev / mean)
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
	return math.Max(0, 1.0-coeffVar) // Normalize to 0-1 where 1 = high clustering
}

// detectSequentialPatterns detects alternating buy/sell patterns
func (wtd *WashTradingDetector) detectSequentialPatterns(activity *TradingActivity) int {
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
				priceThreshold := order.Price.Mul(decimal.NewFromFloat(0.001)) // 0.1%

				if priceDiff.LessThan(priceThreshold) {
					samePrice++
					if samePrice >= 3 { // Pattern of 3+ alternating orders at same price
						patterns++
						samePrice = 0 // Reset counter
					}
				} else {
					samePrice = 0
				}
			}
		} else {
			samePrice = 0 // Reset if not alternating
		}

		lastSide = order.Side
	}

	return patterns
}

// calculatePriceLevelReuse calculates how often same price levels are reused
func (wtd *WashTradingDetector) calculatePriceLevelReuse(activity *TradingActivity) int {
	if len(activity.Orders) < 5 {
		return 0
	}

	priceCount := make(map[string]int)

	// Count occurrences of each price level (rounded to avoid floating point issues)
	for _, order := range activity.Orders {
		// Round to 4 decimal places for comparison
		priceKey := order.Price.Round(4).String()
		priceCount[priceKey]++
	}

	// Count price levels used multiple times
	var reuseCount int
	for _, count := range priceCount {
		if count >= 3 { // Price level used 3+ times
			reuseCount += count - 2 // Each additional use beyond 2 is suspicious
		}
	}

	return reuseCount
}

// calculateWashTradingConfidence calculates overall confidence score
func (wtd *WashTradingDetector) calculateWashTradingConfidence(indicators WashTradingIndicators) decimal.Decimal {
	var score decimal.Decimal

	// Weight factors for different indicators
	weights := map[string]decimal.Decimal{
		"self_trading":     decimal.NewFromFloat(40), // Highest weight
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

// buildWashTradingEvidence builds evidence list for wash trading detection
func (wtd *WashTradingDetector) buildWashTradingEvidence(indicators WashTradingIndicators) []Evidence {
	var evidence []Evidence

	if indicators.SelfTradingRatio.GreaterThan(decimal.NewFromFloat(0.3)) {
		evidence = append(evidence, Evidence{
			Type:        "pattern",
			Description: "High self-trading ratio detected",
			Value:       indicators.SelfTradingRatio.String(),
			Timestamp:   time.Now(),
		})
	}

	if indicators.VolumeInflationRatio.GreaterThan(decimal.NewFromFloat(0.5)) {
		evidence = append(evidence, Evidence{
			Type:        "volume",
			Description: "Artificially inflated volume patterns",
			Value:       indicators.VolumeInflationRatio.String(),
			Timestamp:   time.Now(),
		})
	}

	if indicators.TimeClustering > 0.7 {
		evidence = append(evidence, Evidence{
			Type:        "timing",
			Description: "Suspicious time clustering in orders",
			Value:       decimal.NewFromFloat(indicators.TimeClustering).String(),
			Timestamp:   time.Now(),
		})
	}

	if indicators.SequentialPatterns >= 2 {
		evidence = append(evidence, Evidence{
			Type:        "pattern",
			Description: "Alternating buy/sell patterns detected",
			Value:       decimal.NewFromInt(int64(indicators.SequentialPatterns)).String(),
			Timestamp:   time.Now(),
		})
	}

	return evidence
}

// determineSeverity determines severity based on confidence score
func (wtd *WashTradingDetector) determineSeverity(confidence decimal.Decimal) string {
	if confidence.GreaterThan(decimal.NewFromFloat(90)) {
		return "critical"
	} else if confidence.GreaterThan(decimal.NewFromFloat(75)) {
		return "high"
	} else if confidence.GreaterThan(decimal.NewFromFloat(60)) {
		return "medium"
	}
	return "low"
}

// =======================
// SPOOFING DETECTOR
// =======================

// SpoofingDetector detects spoofing patterns (fake orders to manipulate prices)
type SpoofingDetector struct {
	config                    DetectionConfig
	logger                    *zap.SugaredLogger
	threshold                 decimal.Decimal
	minOrderSize              decimal.Decimal
	cancellationWindowSeconds int
}

// NewSpoofingDetector creates a new spoofing detector
func NewSpoofingDetector(config DetectionConfig, logger *zap.SugaredLogger) *SpoofingDetector {
	return &SpoofingDetector{
		config:                    config,
		logger:                    logger,
		threshold:                 config.SpoofingThreshold,
		minOrderSize:              decimal.NewFromFloat(10000), // Minimum order size for analysis
		cancellationWindowSeconds: 300,                         // 5 minutes
	}
}

// Detect analyzes trading activity for spoofing patterns
func (sd *SpoofingDetector) Detect(activity *TradingActivity) *ManipulationPattern {
	if len(activity.Orders) < 5 {
		return nil // Insufficient data
	}

	indicators := sd.calculateSpoofingIndicators(activity)
	confidence := sd.calculateSpoofingConfidence(indicators)

	if confidence.GreaterThan(sd.threshold) {
		return &ManipulationPattern{
			Type:        "spoofing",
			Description: "Detected potential spoofing (fake orders to manipulate prices)",
			Confidence:  confidence,
			Severity:    sd.determineSeverity(confidence),
			Evidence:    sd.buildSpoofingEvidence(indicators),
			DetectedAt:  time.Now(),
			Metadata: map[string]interface{}{
				"cancellation_rate":   indicators.CancellationRate.String(),
				"large_order_ratio":   indicators.LargeOrderRatio.String(),
				"quick_cancellations": indicators.QuickCancellations,
				"book_pressure_ratio": indicators.BookPressureRatio.String(),
			},
		}
	}

	return nil
}

// SpoofingIndicators holds calculated indicators for spoofing detection
type SpoofingIndicators struct {
	CancellationRate   decimal.Decimal
	QuickCancellations int
	LargeOrderRatio    decimal.Decimal
	BookPressureRatio  decimal.Decimal
	PriceImpactRatio   decimal.Decimal
	LayeringPatterns   int
}

// calculateSpoofingIndicators calculates key indicators for spoofing
func (sd *SpoofingDetector) calculateSpoofingIndicators(activity *TradingActivity) SpoofingIndicators {
	indicators := SpoofingIndicators{}

	// 1. Overall cancellation rate
	indicators.CancellationRate = sd.calculateCancellationRate(activity)

	// 2. Quick cancellations (cancelled within short time window)
	indicators.QuickCancellations = sd.countQuickCancellations(activity)

	// 3. Large order ratio (proportion of large orders vs normal orders)
	indicators.LargeOrderRatio = sd.calculateLargeOrderRatio(activity)

	// 4. Order book pressure ratio (orders placed to create pressure)
	indicators.BookPressureRatio = sd.calculateBookPressureRatio(activity)

	// 5. Price impact ratio (orders with minimal actual impact)
	indicators.PriceImpactRatio = sd.calculatePriceImpactRatio(activity)

	// 6. Layering patterns (multiple orders at similar price levels)
	indicators.LayeringPatterns = sd.detectLayeringPatterns(activity)

	return indicators
}

// calculateCancellationRate calculates the overall order cancellation rate
func (sd *SpoofingDetector) calculateCancellationRate(activity *TradingActivity) decimal.Decimal {
	if len(activity.Orders) == 0 {
		return decimal.Zero
	}

	var cancelledCount int
	for _, order := range activity.Orders {
		if order.Status == model.OrderStatusCancelled {
			cancelledCount++
		}
	}

	return decimal.NewFromInt(int64(cancelledCount)).Div(decimal.NewFromInt(int64(len(activity.Orders))))
}

// countQuickCancellations counts orders cancelled within a short time window
func (sd *SpoofingDetector) countQuickCancellations(activity *TradingActivity) int {
	var quickCancellations int

	for _, order := range activity.Orders {
		if order.Status == model.OrderStatusCancelled {
			// In a real implementation, we would have UpdatedAt field for cancellation time
			// For now, simulate quick cancellation detection
			if sd.isQuickCancellation(order) {
				quickCancellations++
			}
		}
	}

	return quickCancellations
}

// isQuickCancellation determines if an order was cancelled quickly
func (sd *SpoofingDetector) isQuickCancellation(order *model.Order) bool {
	// In a real implementation, this would check the time between order placement and cancellation
	// For simulation, we use order size as a proxy (larger orders more likely to be spoofs)
	orderValue := order.Quantity.Mul(order.Price)
	return orderValue.GreaterThan(sd.minOrderSize)
}

// calculateLargeOrderRatio calculates the ratio of large orders to total orders
func (sd *SpoofingDetector) calculateLargeOrderRatio(activity *TradingActivity) decimal.Decimal {
	if len(activity.Orders) == 0 {
		return decimal.Zero
	}

	var largeOrderCount int
	for _, order := range activity.Orders {
		orderValue := order.Quantity.Mul(order.Price)
		if orderValue.GreaterThan(sd.minOrderSize) {
			largeOrderCount++
		}
	}

	return decimal.NewFromInt(int64(largeOrderCount)).Div(decimal.NewFromInt(int64(len(activity.Orders))))
}

// calculateBookPressureRatio calculates order book pressure indicators
func (sd *SpoofingDetector) calculateBookPressureRatio(activity *TradingActivity) decimal.Decimal {
	if len(activity.Orders) == 0 {
		return decimal.Zero
	}

	// Analyze order distribution on bid/ask sides
	var bidOrders, askOrders int
	var bidVolume, askVolume decimal.Decimal

	for _, order := range activity.Orders {
		orderVolume := order.Quantity.Mul(order.Price)
		if order.Side == "buy" {
			bidOrders++
			bidVolume = bidVolume.Add(orderVolume)
		} else {
			askOrders++
			askVolume = askVolume.Add(orderVolume)
		}
	}

	totalVolume := bidVolume.Add(askVolume)
	if totalVolume.IsZero() {
		return decimal.Zero
	}

	// Calculate imbalance ratio
	imbalance := bidVolume.Sub(askVolume).Abs().Div(totalVolume)
	return imbalance
}

// calculatePriceImpactRatio calculates the ratio of orders with minimal price impact
func (sd *SpoofingDetector) calculatePriceImpactRatio(activity *TradingActivity) decimal.Decimal {
	if len(activity.Trades) <= 1 {
		return decimal.Zero
	}

	var minimalImpactTrades int
	for i := 1; i < len(activity.Trades); i++ {
		if sd.hasMinimalPriceImpact(activity.Trades[i], activity.Trades) {
			minimalImpactTrades++
		}
	}

	return decimal.NewFromInt(int64(minimalImpactTrades)).Div(decimal.NewFromInt(int64(len(activity.Trades))))
}

// hasMinimalPriceImpact checks if a trade has minimal price impact (same logic as wash trading)
func (sd *SpoofingDetector) hasMinimalPriceImpact(trade *model.Trade, allTrades []*model.Trade) bool {
	// Find trades before and after this trade
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

	// Calculate price impact as percentage change
	priceChange := afterPrice.Sub(beforePrice).Abs()
	priceImpact := priceChange.Div(beforePrice).Mul(decimal.NewFromInt(100))

	// Consider impact minimal if less than 0.05%
	return priceImpact.LessThan(decimal.NewFromFloat(0.05))
}

// detectLayeringPatterns detects layering patterns in orders
func (sd *SpoofingDetector) detectLayeringPatterns(activity *TradingActivity) int {
	if len(activity.Orders) < 3 {
		return 0
	}

	// Group orders by price levels
	priceLevels := make(map[string][]*model.Order)
	for _, order := range activity.Orders {
		priceKey := order.Price.Round(4).String()
		priceLevels[priceKey] = append(priceLevels[priceKey], order)
	}

	var layeringPatterns int
	for _, orders := range priceLevels {
		if len(orders) >= 3 { // 3+ orders at same price level
			// Check if most orders are cancelled
			var cancelledCount int
			for _, order := range orders {
				if order.Status == model.OrderStatusCancelled {
					cancelledCount++
				}
			}

			// If >70% of orders at this level are cancelled, it's likely layering
			cancellationRate := float64(cancelledCount) / float64(len(orders))
			if cancellationRate > 0.7 {
				layeringPatterns++
			}
		}
	}

	return layeringPatterns
}

// calculateSpoofingConfidence calculates overall confidence score for spoofing
func (sd *SpoofingDetector) calculateSpoofingConfidence(indicators SpoofingIndicators) decimal.Decimal {
	var score decimal.Decimal

	// Weight factors for different indicators
	weights := map[string]decimal.Decimal{
		"cancellation":  decimal.NewFromFloat(30), // High weight
		"quick_cancel":  decimal.NewFromFloat(25),
		"large_orders":  decimal.NewFromFloat(20),
		"book_pressure": decimal.NewFromFloat(15),
		"layering":      decimal.NewFromFloat(10),
	}

	// Cancellation rate (0-30 points)
	cancellationScore := indicators.CancellationRate.Mul(weights["cancellation"])
	score = score.Add(cancellationScore)

	// Quick cancellations (0-25 points)
	if indicators.QuickCancellations > 0 {
		quickCancelScore := decimal.Min(decimal.NewFromInt(int64(indicators.QuickCancellations*5)), weights["quick_cancel"])
		score = score.Add(quickCancelScore)
	}

	// Large order ratio (0-20 points)
	largeOrderScore := indicators.LargeOrderRatio.Mul(weights["large_orders"])
	score = score.Add(largeOrderScore)

	// Book pressure ratio (0-15 points)
	pressureScore := indicators.BookPressureRatio.Mul(weights["book_pressure"])
	score = score.Add(pressureScore)

	// Layering patterns (0-10 points)
	if indicators.LayeringPatterns > 0 {
		layeringScore := decimal.Min(decimal.NewFromInt(int64(indicators.LayeringPatterns*3)), weights["layering"])
		score = score.Add(layeringScore)
	}

	// Cap at 100
	return decimal.Min(score, decimal.NewFromInt(100))
}

// buildSpoofingEvidence builds evidence list for spoofing detection
func (sd *SpoofingDetector) buildSpoofingEvidence(indicators SpoofingIndicators) []Evidence {
	var evidence []Evidence

	if indicators.CancellationRate.GreaterThan(decimal.NewFromFloat(0.6)) {
		evidence = append(evidence, Evidence{
			Type:        "cancellation",
			Description: "High order cancellation rate",
			Value:       indicators.CancellationRate.String(),
			Timestamp:   time.Now(),
		})
	}

	if indicators.QuickCancellations >= 3 {
		evidence = append(evidence, Evidence{
			Type:        "timing",
			Description: "Multiple quick order cancellations",
			Value:       decimal.NewFromInt(int64(indicators.QuickCancellations)).String(),
			Timestamp:   time.Now(),
		})
	}

	if indicators.LargeOrderRatio.GreaterThan(decimal.NewFromFloat(0.4)) {
		evidence = append(evidence, Evidence{
			Type:        "size",
			Description: "High proportion of large orders",
			Value:       indicators.LargeOrderRatio.String(),
			Timestamp:   time.Now(),
		})
	}

	if indicators.LayeringPatterns >= 2 {
		evidence = append(evidence, Evidence{
			Type:        "pattern",
			Description: "Layering patterns detected",
			Value:       decimal.NewFromInt(int64(indicators.LayeringPatterns)).String(),
			Timestamp:   time.Now(),
		})
	}

	return evidence
}

// determineSeverity determines severity based on confidence score for spoofing
func (sd *SpoofingDetector) determineSeverity(confidence decimal.Decimal) string {
	if confidence.GreaterThan(decimal.NewFromFloat(85)) {
		return "critical"
	} else if confidence.GreaterThan(decimal.NewFromFloat(70)) {
		return "high"
	} else if confidence.GreaterThan(decimal.NewFromFloat(55)) {
		return "medium"
	}
	return "low"
}
