package manipulation

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"pincex/internal/model"
)

// =======================
// PUMP & DUMP DETECTOR
// =======================

// PumpDumpDetector detects pump and dump schemes
type PumpDumpDetector struct {
	config          DetectionConfig
	logger          *zap.SugaredLogger
	threshold       decimal.Decimal
	priceThreshold  decimal.Decimal // Minimum price movement to consider
	volumeThreshold decimal.Decimal // Minimum volume spike to consider
	timeWindow      time.Duration
}

// NewPumpDumpDetector creates a new pump and dump detector
func NewPumpDumpDetector(config DetectionConfig, logger *zap.SugaredLogger) *PumpDumpDetector {
	return &PumpDumpDetector{
		config:          config,
		logger:          logger,
		threshold:       config.PumpDumpThreshold,
		priceThreshold:  decimal.NewFromFloat(10), // 10% price movement
		volumeThreshold: decimal.NewFromFloat(3),  // 3x volume spike
		timeWindow:      time.Duration(config.DetectionWindowMinutes) * time.Minute,
	}
}

// Detect analyzes trading activity for pump and dump patterns
func (pdd *PumpDumpDetector) Detect(activity *TradingActivity) *ManipulationPattern {
	if len(activity.Trades) < 10 {
		return nil // Insufficient data
	}

	indicators := pdd.calculatePumpDumpIndicators(activity)
	confidence := pdd.calculatePumpDumpConfidence(indicators)
	
	if confidence.GreaterThan(pdd.threshold) {
		return &ManipulationPattern{
			Type:        "pump_dump",
			Description: "Detected potential pump and dump scheme",
			Confidence:  confidence,
			Severity:    pdd.determineSeverity(confidence),
			Evidence:    pdd.buildPumpDumpEvidence(indicators),
			DetectedAt:  time.Now(),
			Metadata: map[string]interface{}{
				"price_spike":        indicators.PriceSpike.String(),
				"volume_spike":       indicators.VolumeSpike.String(),
				"price_reversal":     indicators.PriceReversal.String(),
				"coordination_score": indicators.CoordinationScore,
				"phases_detected":    indicators.PhasesDetected,
			},
		}
	}

	return nil
}

// PumpDumpIndicators holds calculated indicators for pump and dump detection
type PumpDumpIndicators struct {
	PriceSpike         decimal.Decimal
	VolumeSpike        decimal.Decimal
	PriceReversal      decimal.Decimal
	CoordinationScore  float64
	PhasesDetected     []string
	SuspiciousTiming   float64
	VolumeProfile      VolumeDistribution
}

// VolumeDistribution represents volume distribution analysis
type VolumeDistribution struct {
	EarlyVolume   decimal.Decimal
	PeakVolume    decimal.Decimal
	LateVolume    decimal.Decimal
	Concentration decimal.Decimal // How concentrated volume is in time
}

// calculatePumpDumpIndicators calculates key indicators for pump and dump detection
func (pdd *PumpDumpDetector) calculatePumpDumpIndicators(activity *TradingActivity) PumpDumpIndicators {
	indicators := PumpDumpIndicators{}
	
	// Sort trades by time
	trades := make([]*model.Trade, len(activity.Trades))
	copy(trades, activity.Trades)
	sort.Slice(trades, func(i, j int) bool {
		return trades[i].CreatedAt.Before(trades[j].CreatedAt)
	})
	
	// 1. Price spike analysis
	indicators.PriceSpike = pdd.calculatePriceSpike(trades)
	
	// 2. Volume spike analysis
	indicators.VolumeSpike = pdd.calculateVolumeSpike(trades)
	
	// 3. Price reversal analysis (dump phase)
	indicators.PriceReversal = pdd.calculatePriceReversal(trades)
	
	// 4. Coordination score (simultaneous activity patterns)
	indicators.CoordinationScore = pdd.calculateCoordinationScore(activity)
	
	// 5. Phase detection (pump, peak, dump)
	indicators.PhasesDetected = pdd.detectPhases(trades)
	
	// 6. Suspicious timing analysis
	indicators.SuspiciousTiming = pdd.analyzeTiming(trades)
	
	// 7. Volume distribution analysis
	indicators.VolumeProfile = pdd.analyzeVolumeDistribution(trades)
	
	return indicators
}

// calculatePriceSpike calculates the maximum price spike in the activity window
func (pdd *PumpDumpDetector) calculatePriceSpike(trades []*model.Trade) decimal.Decimal {
	if len(trades) < 2 {
		return decimal.Zero
	}
	
	var maxSpike decimal.Decimal
	basePrice := trades[0].Price
	
	for _, trade := range trades {
		priceChange := trade.Price.Sub(basePrice).Div(basePrice).Mul(decimal.NewFromInt(100))
		if priceChange.GreaterThan(maxSpike) {
			maxSpike = priceChange
		}
	}
	
	return maxSpike
}

// calculateVolumeSpike calculates volume spike compared to baseline
func (pdd *PumpDumpDetector) calculateVolumeSpike(trades []*model.Trade) decimal.Decimal {
	if len(trades) < 10 {
		return decimal.Zero
	}
	
	// Calculate baseline volume (first 30% of trades)
	baselineEnd := len(trades) * 30 / 100
	if baselineEnd < 3 {
		baselineEnd = 3
	}
	
	var baselineVolume decimal.Decimal
	for i := 0; i < baselineEnd; i++ {
		baselineVolume = baselineVolume.Add(trades[i].Quantity)
	}
	baselineAvg := baselineVolume.Div(decimal.NewFromInt(int64(baselineEnd)))
	
	// Find maximum volume spike
	var maxSpike decimal.Decimal
	windowSize := 5 // 5-trade rolling window
	
	for i := windowSize; i <= len(trades)-windowSize; i++ {
		var windowVolume decimal.Decimal
		for j := i - windowSize; j < i + windowSize; j++ {
			windowVolume = windowVolume.Add(trades[j].Quantity)
		}
		windowAvg := windowVolume.Div(decimal.NewFromInt(int64(windowSize * 2)))
		
		if !baselineAvg.IsZero() {
			spike := windowAvg.Div(baselineAvg)
			if spike.GreaterThan(maxSpike) {
				maxSpike = spike
			}
		}
	}
	
	return maxSpike
}

// calculatePriceReversal calculates price reversal (dump phase)
func (pdd *PumpDumpDetector) calculatePriceReversal(trades []*model.Trade) decimal.Decimal {
	if len(trades) < 5 {
		return decimal.Zero
	}
	
	// Find peak price
	var peakPrice decimal.Decimal
	var peakIndex int
	
	for i, trade := range trades {
		if trade.Price.GreaterThan(peakPrice) {
			peakPrice = trade.Price
			peakIndex = i
		}
	}
	
	// Calculate maximum reversal from peak
	var maxReversal decimal.Decimal
	for i := peakIndex; i < len(trades); i++ {
		reversal := peakPrice.Sub(trades[i].Price).Div(peakPrice).Mul(decimal.NewFromInt(100))
		if reversal.GreaterThan(maxReversal) {
			maxReversal = reversal
		}
	}
	
	return maxReversal
}

// calculateCoordinationScore calculates coordination between different participants
func (pdd *PumpDumpDetector) calculateCoordinationScore(activity *TradingActivity) float64 {
	if len(activity.Orders) < 10 {
		return 0.0
	}
	
	// Analyze timing correlation between orders
	var simultaneousOrders int
	var totalPairs int
	
	for i := 0; i < len(activity.Orders); i++ {
		for j := i + 1; j < len(activity.Orders); j++ {
			timeDiff := activity.Orders[j].CreatedAt.Sub(activity.Orders[i].CreatedAt)
			if timeDiff < 0 {
				timeDiff = -timeDiff
			}
			
			totalPairs++
			
			// Orders within 10 seconds are considered coordinated
			if timeDiff <= 10*time.Second {
				simultaneousOrders++
			}
		}
	}
	
	if totalPairs == 0 {
		return 0.0
	}
	
	return float64(simultaneousOrders) / float64(totalPairs)
}

// detectPhases detects pump, peak, and dump phases
func (pdd *PumpDumpDetector) detectPhases(trades []*model.Trade) []string {
	if len(trades) < 15 {
		return nil
	}
	
	var phases []string
	
	// Divide trading period into thirds
	third := len(trades) / 3
	
	// Analyze price trends in each phase
	phase1Trend := pdd.calculateTrend(trades[0:third])
	phase2Trend := pdd.calculateTrend(trades[third : 2*third])
	phase3Trend := pdd.calculateTrend(trades[2*third:])
	
	// Pump phase: strong upward trend
	if phase1Trend > 5.0 { // >5% price increase
		phases = append(phases, "pump")
	}
	
	// Peak phase: consolidation or slight movement
	if math.Abs(phase2Trend) < 2.0 { // <2% movement
		phases = append(phases, "peak")
	}
	
	// Dump phase: strong downward trend
	if phase3Trend < -5.0 { // >5% price decrease
		phases = append(phases, "dump")
	}
	
	return phases
}

// calculateTrend calculates price trend for a series of trades
func (pdd *PumpDumpDetector) calculateTrend(trades []*model.Trade) float64 {
	if len(trades) < 2 {
		return 0.0
	}
	
	startPrice := trades[0].Price
	endPrice := trades[len(trades)-1].Price
	
	if startPrice.IsZero() {
		return 0.0
	}
	
	trend := endPrice.Sub(startPrice).Div(startPrice).Mul(decimal.NewFromInt(100))
	return trend.InexactFloat64()
}

// analyzeTiming analyzes suspicious timing patterns
func (pdd *PumpDumpDetector) analyzeTiming(trades []*model.Trade) float64 {
	if len(trades) < 5 {
		return 0.0
	}
	
	// Calculate coefficient of variation for trade intervals
	var intervals []float64
	for i := 1; i < len(trades); i++ {
		interval := trades[i].CreatedAt.Sub(trades[i-1].CreatedAt).Seconds()
		intervals = append(intervals, interval)
	}
	
	if len(intervals) == 0 {
		return 0.0
	}
	
	// Calculate mean and standard deviation
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
	
	// High coordination = low coefficient of variation
	coeffVar := stdDev / mean
	return math.Max(0, 1.0-coeffVar) // Normalize to 0-1
}

// analyzeVolumeDistribution analyzes how volume is distributed over time
func (pdd *PumpDumpDetector) analyzeVolumeDistribution(trades []*model.Trade) VolumeDistribution {
	if len(trades) < 9 {
		return VolumeDistribution{}
	}
	
	third := len(trades) / 3
	
	var earlyVolume, peakVolume, lateVolume decimal.Decimal
	
	// Early phase volume
	for i := 0; i < third; i++ {
		earlyVolume = earlyVolume.Add(trades[i].Quantity)
	}
	
	// Peak phase volume
	for i := third; i < 2*third; i++ {
		peakVolume = peakVolume.Add(trades[i].Quantity)
	}
	
	// Late phase volume
	for i := 2*third; i < len(trades); i++ {
		lateVolume = lateVolume.Add(trades[i].Quantity)
	}
	
	totalVolume := earlyVolume.Add(peakVolume).Add(lateVolume)
	
	// Calculate concentration (how much volume is in peak phase)
	var concentration decimal.Decimal
	if !totalVolume.IsZero() {
		concentration = peakVolume.Div(totalVolume)
	}
	
	return VolumeDistribution{
		EarlyVolume:   earlyVolume,
		PeakVolume:    peakVolume,
		LateVolume:    lateVolume,
		Concentration: concentration,
	}
}

// calculatePumpDumpConfidence calculates overall confidence score
func (pdd *PumpDumpDetector) calculatePumpDumpConfidence(indicators PumpDumpIndicators) decimal.Decimal {
	var score decimal.Decimal
	
	// Weight factors for different indicators
	weights := map[string]decimal.Decimal{
		"price_spike":   decimal.NewFromFloat(25),
		"volume_spike":  decimal.NewFromFloat(20),
		"reversal":      decimal.NewFromFloat(20),
		"phases":        decimal.NewFromFloat(15),
		"coordination":  decimal.NewFromFloat(15),
		"timing":        decimal.NewFromFloat(5),
	}
	
	// Price spike score (0-25 points)
	if indicators.PriceSpike.GreaterThan(pdd.priceThreshold) {
		spikeScore := decimal.Min(indicators.PriceSpike.Div(decimal.NewFromFloat(50)).Mul(weights["price_spike"]), weights["price_spike"])
		score = score.Add(spikeScore)
	}
	
	// Volume spike score (0-20 points)
	if indicators.VolumeSpike.GreaterThan(pdd.volumeThreshold) {
		volumeScore := decimal.Min(indicators.VolumeSpike.Div(decimal.NewFromFloat(10)).Mul(weights["volume_spike"]), weights["volume_spike"])
		score = score.Add(volumeScore)
	}
	
	// Price reversal score (0-20 points)
	if indicators.PriceReversal.GreaterThan(decimal.NewFromFloat(5)) {
		reversalScore := decimal.Min(indicators.PriceReversal.Div(decimal.NewFromFloat(30)).Mul(weights["reversal"]), weights["reversal"])
		score = score.Add(reversalScore)
	}
	
	// Phases score (0-15 points)
	phaseScore := decimal.NewFromInt(int64(len(indicators.PhasesDetected) * 5))
	phaseScore = decimal.Min(phaseScore, weights["phases"])
	score = score.Add(phaseScore)
	
	// Coordination score (0-15 points)
	coordScore := decimal.NewFromFloat(indicators.CoordinationScore).Mul(weights["coordination"])
	score = score.Add(coordScore)
	
	// Timing score (0-5 points)
	timingScore := decimal.NewFromFloat(indicators.SuspiciousTiming).Mul(weights["timing"])
	score = score.Add(timingScore)
	
	// Cap at 100
	return decimal.Min(score, decimal.NewFromInt(100))
}

// buildPumpDumpEvidence builds evidence list for pump and dump detection
func (pdd *PumpDumpDetector) buildPumpDumpEvidence(indicators PumpDumpIndicators) []Evidence {
	var evidence []Evidence
	
	if indicators.PriceSpike.GreaterThan(pdd.priceThreshold) {
		evidence = append(evidence, Evidence{
			Type:        "price",
			Description: "Significant price spike detected",
			Value:       indicators.PriceSpike.String() + "%",
			Timestamp:   time.Now(),
		})
	}
	
	if indicators.VolumeSpike.GreaterThan(pdd.volumeThreshold) {
		evidence = append(evidence, Evidence{
			Type:        "volume",
			Description: "Unusual volume spike detected",
			Value:       indicators.VolumeSpike.String() + "x",
			Timestamp:   time.Now(),
		})
	}
	
	if indicators.PriceReversal.GreaterThan(decimal.NewFromFloat(10)) {
		evidence = append(evidence, Evidence{
			Type:        "price",
			Description: "Sharp price reversal (dump phase)",
			Value:       indicators.PriceReversal.String() + "%",
			Timestamp:   time.Now(),
		})
	}
	
	if len(indicators.PhasesDetected) >= 2 {
		evidence = append(evidence, Evidence{
			Type:        "pattern",
			Description: "Pump and dump phases identified",
			Value:       fmt.Sprintf("%v", indicators.PhasesDetected),
			Timestamp:   time.Now(),
		})
	}
	
	if indicators.CoordinationScore > 0.3 {
		evidence = append(evidence, Evidence{
			Type:        "coordination",
			Description: "High coordination between participants",
			Value:       decimal.NewFromFloat(indicators.CoordinationScore).String(),
			Timestamp:   time.Now(),
		})
	}
	
	return evidence
}

// determineSeverity determines severity based on confidence score for pump and dump
func (pdd *PumpDumpDetector) determineSeverity(confidence decimal.Decimal) string {
	if confidence.GreaterThan(decimal.NewFromFloat(80)) {
		return "critical"
	} else if confidence.GreaterThan(decimal.NewFromFloat(65)) {
		return "high"
	} else if confidence.GreaterThan(decimal.NewFromFloat(50)) {
		return "medium"
	}
	return "low"
}

// =======================
// LAYERING DETECTOR
// =======================

// LayeringDetector detects layering patterns (multiple orders to create false market depth)
type LayeringDetector struct {
	config         DetectionConfig
	logger         *zap.SugaredLogger
	threshold      decimal.Decimal
	minLayers      int
	priceTickSize  decimal.Decimal
}

// NewLayeringDetector creates a new layering detector
func NewLayeringDetector(config DetectionConfig, logger *zap.SugaredLogger) *LayeringDetector {
	return &LayeringDetector{
		config:        config,
		logger:        logger,
		threshold:     config.LayeringThreshold,
		minLayers:     3, // Minimum number of price levels to consider layering
		priceTickSize: decimal.NewFromFloat(0.01), // Minimum price increment
	}
}

// Detect analyzes trading activity for layering patterns
func (ld *LayeringDetector) Detect(activity *TradingActivity) *ManipulationPattern {
	if len(activity.Orders) < 6 {
		return nil // Insufficient data for layering detection
	}

	indicators := ld.calculateLayeringIndicators(activity)
	confidence := ld.calculateLayeringConfidence(indicators)
	
	if confidence.GreaterThan(ld.threshold) {
		return &ManipulationPattern{
			Type:        "layering",
			Description: "Detected potential layering (multiple orders to create false market depth)",
			Confidence:  confidence,
			Severity:    ld.determineSeverity(confidence),
			Evidence:    ld.buildLayeringEvidence(indicators),
			DetectedAt:  time.Now(),
			Metadata: map[string]interface{}{
				"layer_count":        indicators.LayerCount,
				"depth_distortion":   indicators.DepthDistortion.String(),
				"cancellation_rate":  indicators.CancellationRate.String(),
				"sequential_layers":  indicators.SequentialLayers,
				"size_progression":   indicators.SizeProgression.String(),
			},
		}
	}

	return nil
}

// LayeringIndicators holds calculated indicators for layering detection
type LayeringIndicators struct {
	LayerCount         int
	DepthDistortion    decimal.Decimal
	CancellationRate   decimal.Decimal
	SequentialLayers   int
	SizeProgression    decimal.Decimal
	TimeClusterScore   float64
	PriceLevelDensity  float64
}

// calculateLayeringIndicators calculates key indicators for layering detection
func (ld *LayeringDetector) calculateLayeringIndicators(activity *TradingActivity) LayeringIndicators {
	indicators := LayeringIndicators{}
	
	// 1. Count layers (price levels with multiple orders)
	indicators.LayerCount = ld.countLayers(activity.Orders)
	
	// 2. Analyze depth distortion
	indicators.DepthDistortion = ld.calculateDepthDistortion(activity.Orders)
	
	// 3. Calculate cancellation rate for layered orders
	indicators.CancellationRate = ld.calculateLayeredCancellationRate(activity.Orders)
	
	// 4. Detect sequential layering patterns
	indicators.SequentialLayers = ld.detectSequentialLayers(activity.Orders)
	
	// 5. Analyze order size progression across layers
	indicators.SizeProgression = ld.analyzeSizeProgression(activity.Orders)
	
	// 6. Calculate time clustering score
	indicators.TimeClusterScore = ld.calculateTimeClusterScore(activity.Orders)
	
	// 7. Calculate price level density
	indicators.PriceLevelDensity = ld.calculatePriceLevelDensity(activity.Orders)
	
	return indicators
}

// countLayers counts the number of price levels with multiple orders
func (ld *LayeringDetector) countLayers(orders []*model.Order) int {
	priceLevels := make(map[string]int)
	
	for _, order := range orders {
		priceKey := order.Price.Round(4).String()
		priceLevels[priceKey]++
	}
	
	var layers int
	for _, count := range priceLevels {
		if count >= 2 { // 2+ orders at same price level
			layers++
		}
	}
	
	return layers
}

// calculateDepthDistortion calculates how much the order book depth is artificially inflated
func (ld *LayeringDetector) calculateDepthDistortion(orders []*model.Order) decimal.Decimal {
	if len(orders) == 0 {
		return decimal.Zero
	}
	
	// Group orders by side
	var bidOrders, askOrders []*model.Order
	for _, order := range orders {
		if order.Side == "buy" {
			bidOrders = append(bidOrders, order)
		} else {
			askOrders = append(askOrders, order)
		}
	}
	
	// Calculate depth for each side
	bidDepth := ld.calculateSideDepthDistortion(bidOrders)
	askDepth := ld.calculateSideDepthDistortion(askOrders)
	
	// Return maximum distortion
	return decimal.Max(bidDepth, askDepth)
}

// calculateSideDepthDistortion calculates depth distortion for one side (bid or ask)
func (ld *LayeringDetector) calculateSideDepthDistortion(orders []*model.Order) decimal.Decimal {
	if len(orders) < 3 {
		return decimal.Zero
	}
	
	// Group by price level
	priceLevels := make(map[string][]*model.Order)
	for _, order := range orders {
		priceKey := order.Price.Round(4).String()
		priceLevels[priceKey] = append(priceLevels[priceKey], order)
	}
	
	// Calculate distortion based on cancellation patterns
	var totalQuantity, suspiciousQuantity decimal.Decimal
	
	for _, levelOrders := range priceLevels {
		if len(levelOrders) >= 2 { // Multiple orders at same level
			var cancelledCount int
			var levelQuantity decimal.Decimal
			
			for _, order := range levelOrders {
				levelQuantity = levelQuantity.Add(order.Quantity)
				if order.Status == model.OrderStatusCancelled {
					cancelledCount++
				}
			}
			
			totalQuantity = totalQuantity.Add(levelQuantity)
			
			// If >50% of orders at this level are cancelled, mark as suspicious
			if float64(cancelledCount)/float64(len(levelOrders)) > 0.5 {
				suspiciousQuantity = suspiciousQuantity.Add(levelQuantity)
			}
		}
	}
	
	if totalQuantity.IsZero() {
		return decimal.Zero
	}
	
	return suspiciousQuantity.Div(totalQuantity)
}

// calculateLayeredCancellationRate calculates cancellation rate for orders in layers
func (ld *LayeringDetector) calculateLayeredCancellationRate(orders []*model.Order) decimal.Decimal {
	// Find orders that are part of layers
	priceLevels := make(map[string][]*model.Order)
	for _, order := range orders {
		priceKey := order.Price.Round(4).String()
		priceLevels[priceKey] = append(priceLevels[priceKey], order)
	}
	
	var layeredOrders []*model.Order
	for _, levelOrders := range priceLevels {
		if len(levelOrders) >= 2 { // Part of a layer
			layeredOrders = append(layeredOrders, levelOrders...)
		}
	}
	
	if len(layeredOrders) == 0 {
		return decimal.Zero
	}
	
	var cancelledCount int
	for _, order := range layeredOrders {
		if order.Status == model.OrderStatusCancelled {
			cancelledCount++
		}
	}
	
	return decimal.NewFromInt(int64(cancelledCount)).Div(decimal.NewFromInt(int64(len(layeredOrders))))
}

// detectSequentialLayers detects sequential layering patterns
func (ld *LayeringDetector) detectSequentialLayers(orders []*model.Order) int {
	if len(orders) < 6 {
		return 0
	}
	
	// Sort orders by time
	sortedOrders := make([]*model.Order, len(orders))
	copy(sortedOrders, orders)
	sort.Slice(sortedOrders, func(i, j int) bool {
		return sortedOrders[i].CreatedAt.Before(sortedOrders[j].CreatedAt)
	})
	
	var sequentialLayers int
	var currentSequence []*model.Order
	
	for _, order := range sortedOrders {
		// Check if this order continues the sequence
		if ld.isPartOfSequence(order, currentSequence) {
			currentSequence = append(currentSequence, order)
		} else {
			// Check if we have a valid sequence
			if len(currentSequence) >= 3 && ld.isValidLayeringSequence(currentSequence) {
				sequentialLayers++
			}
			// Start new sequence
			currentSequence = []*model.Order{order}
		}
	}
	
	// Check final sequence
	if len(currentSequence) >= 3 && ld.isValidLayeringSequence(currentSequence) {
		sequentialLayers++
	}
	
	return sequentialLayers
}

// isPartOfSequence checks if an order is part of a sequential layering pattern
func (ld *LayeringDetector) isPartOfSequence(order *model.Order, sequence []*model.Order) bool {
	if len(sequence) == 0 {
		return true
	}
	
	lastOrder := sequence[len(sequence)-1]
	
	// Check timing (within 60 seconds)
	timeDiff := order.CreatedAt.Sub(lastOrder.CreatedAt)
	if timeDiff > 60*time.Second {
		return false
	}
	
	// Check if same side
	if order.Side != lastOrder.Side {
		return false
	}
	
	// Check if prices are progressing away from market
	// (for layering, orders typically get further from current market price)
	priceDiff := order.Price.Sub(lastOrder.Price)
	if order.Side == "buy" {
		return priceDiff.LessThan(decimal.Zero) // Buy orders getting lower
	} else {
		return priceDiff.GreaterThan(decimal.Zero) // Sell orders getting higher
	}
}

// isValidLayeringSequence checks if a sequence represents valid layering
func (ld *LayeringDetector) isValidLayeringSequence(sequence []*model.Order) bool {
	if len(sequence) < 3 {
		return false
	}
	
	// Check for price progression
	var priceProgression bool = true
	for i := 1; i < len(sequence); i++ {
		priceDiff := sequence[i].Price.Sub(sequence[i-1].Price)
		
		if sequence[i].Side == "buy" {
			// Buy orders should be decreasing in price
			if priceDiff.GreaterThanOrEqual(decimal.Zero) {
				priceProgression = false
				break
			}
		} else {
			// Sell orders should be increasing in price
			if priceDiff.LessThanOrEqual(decimal.Zero) {
				priceProgression = false
				break
			}
		}
	}
	
	// Check cancellation rate (layering orders are often cancelled)
	var cancelledCount int
	for _, order := range sequence {
		if order.Status == model.OrderStatusCancelled {
			cancelledCount++
		}
	}
	
	cancellationRate := float64(cancelledCount) / float64(len(sequence))
	
	return priceProgression && cancellationRate > 0.4 // >40% cancelled
}

// analyzeSizeProgression analyzes order size progression across layers
func (ld *LayeringDetector) analyzeSizeProgression(orders []*model.Order) decimal.Decimal {
	if len(orders) < 4 {
		return decimal.Zero
	}
	
	// Group by side and sort by price
	bidOrders := make([]*model.Order, 0)
	askOrders := make([]*model.Order, 0)
	
	for _, order := range orders {
		if order.Side == "buy" {
			bidOrders = append(bidOrders, order)
		} else {
			askOrders = append(askOrders, order)
		}
	}
	
	// Analyze size progression for each side
	bidProgression := ld.calculateSideProgression(bidOrders, false) // false = descending price
	askProgression := ld.calculateSideProgression(askOrders, true)  // true = ascending price
	
	return decimal.Max(bidProgression, askProgression)
}

// calculateSideProgression calculates size progression for one side
func (ld *LayeringDetector) calculateSideProgression(orders []*model.Order, ascending bool) decimal.Decimal {
	if len(orders) < 3 {
		return decimal.Zero
	}
	
	// Sort by price
	sort.Slice(orders, func(i, j int) bool {
		if ascending {
			return orders[i].Price.LessThan(orders[j].Price)
		}
		return orders[i].Price.GreaterThan(orders[j].Price)
	})
	
	// Check for decreasing size pattern (typical in layering)
	var progressionScore decimal.Decimal
	var validProgressions int
	
	for i := 1; i < len(orders); i++ {
		sizeDiff := orders[i-1].Quantity.Sub(orders[i].Quantity)
		if sizeDiff.GreaterThan(decimal.Zero) { // Size decreasing
			progressionScore = progressionScore.Add(sizeDiff.Div(orders[i-1].Quantity))
			validProgressions++
		}
	}
	
	if validProgressions == 0 {
		return decimal.Zero
	}
	
	return progressionScore.Div(decimal.NewFromInt(int64(validProgressions)))
}

// calculateTimeClusterScore calculates how clustered orders are in time
func (ld *LayeringDetector) calculateTimeClusterScore(orders []*model.Order) float64 {
	if len(orders) < 4 {
		return 0.0
	}
	
	// Sort by time
	sortedOrders := make([]*model.Order, len(orders))
	copy(sortedOrders, orders)
	sort.Slice(sortedOrders, func(i, j int) bool {
		return sortedOrders[i].CreatedAt.Before(sortedOrders[j].CreatedAt)
	})
	
	// Calculate intervals between orders
	var intervals []float64
	for i := 1; i < len(sortedOrders); i++ {
		interval := sortedOrders[i].CreatedAt.Sub(sortedOrders[i-1].CreatedAt).Seconds()
		intervals = append(intervals, interval)
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
	
	// Low coefficient of variation = high clustering
	coeffVar := stdDev / mean
	return math.Max(0, 1.0-coeffVar)
}

// calculatePriceLevelDensity calculates density of orders across price levels
func (ld *LayeringDetector) calculatePriceLevelDensity(orders []*model.Order) float64 {
	if len(orders) < 3 {
		return 0.0
	}
	
	// Find price range
	var minPrice, maxPrice decimal.Decimal
	for i, order := range orders {
		if i == 0 {
			minPrice = order.Price
			maxPrice = order.Price
		} else {
			if order.Price.LessThan(minPrice) {
				minPrice = order.Price
			}
			if order.Price.GreaterThan(maxPrice) {
				maxPrice = order.Price
			}
		}
	}
	
	priceRange := maxPrice.Sub(minPrice)
	if priceRange.IsZero() {
		return 0.0
	}
	
	// Count unique price levels
	priceLevels := make(map[string]bool)
	for _, order := range orders {
		priceKey := order.Price.Round(4).String()
		priceLevels[priceKey] = true
	}
	
	// Calculate density (price levels per unit price range)
	density := float64(len(priceLevels)) / priceRange.InexactFloat64()
	
	// Normalize to 0-1 scale (high density = more suspicious)
	return math.Min(1.0, density/100.0) // Assuming 100 levels per unit as maximum
}

// calculateLayeringConfidence calculates overall confidence score for layering
func (ld *LayeringDetector) calculateLayeringConfidence(indicators LayeringIndicators) decimal.Decimal {
	var score decimal.Decimal
	
	// Weight factors for different indicators
	weights := map[string]decimal.Decimal{
		"layer_count":    decimal.NewFromFloat(25),
		"depth_distort":  decimal.NewFromFloat(20),
		"cancellation":   decimal.NewFromFloat(20),
		"sequential":     decimal.NewFromFloat(15),
		"size_progress":  decimal.NewFromFloat(10),
		"time_cluster":   decimal.NewFromFloat(10),
	}
	
	// Layer count score (0-25 points)
	if indicators.LayerCount >= ld.minLayers {
		layerScore := decimal.Min(decimal.NewFromInt(int64(indicators.LayerCount*5)), weights["layer_count"])
		score = score.Add(layerScore)
	}
	
	// Depth distortion score (0-20 points)
	distortionScore := indicators.DepthDistortion.Mul(weights["depth_distort"])
	score = score.Add(distortionScore)
	
	// Cancellation rate score (0-20 points)
	cancellationScore := indicators.CancellationRate.Mul(weights["cancellation"])
	score = score.Add(cancellationScore)
	
	// Sequential layers score (0-15 points)
	if indicators.SequentialLayers > 0 {
		sequentialScore := decimal.Min(decimal.NewFromInt(int64(indicators.SequentialLayers*7)), weights["sequential"])
		score = score.Add(sequentialScore)
	}
	
	// Size progression score (0-10 points)
	progressionScore := indicators.SizeProgression.Mul(weights["size_progress"])
	score = score.Add(progressionScore)
	
	// Time clustering score (0-10 points)
	clusterScore := decimal.NewFromFloat(indicators.TimeClusterScore).Mul(weights["time_cluster"])
	score = score.Add(clusterScore)
	
	// Cap at 100
	return decimal.Min(score, decimal.NewFromInt(100))
}

// buildLayeringEvidence builds evidence list for layering detection
func (ld *LayeringDetector) buildLayeringEvidence(indicators LayeringIndicators) []Evidence {
	var evidence []Evidence
	
	if indicators.LayerCount >= ld.minLayers {
		evidence = append(evidence, Evidence{
			Type:        "structure",
			Description: "Multiple price levels with layered orders",
			Value:       decimal.NewFromInt(int64(indicators.LayerCount)).String(),
			Timestamp:   time.Now(),
		})
	}
	
	if indicators.DepthDistortion.GreaterThan(decimal.NewFromFloat(0.3)) {
		evidence = append(evidence, Evidence{
			Type:        "depth",
			Description: "Artificial market depth inflation",
			Value:       indicators.DepthDistortion.String(),
			Timestamp:   time.Now(),
		})
	}
	
	if indicators.CancellationRate.GreaterThan(decimal.NewFromFloat(0.5)) {
		evidence = append(evidence, Evidence{
			Type:        "cancellation",
			Description: "High cancellation rate in layered orders",
			Value:       indicators.CancellationRate.String(),
			Timestamp:   time.Now(),
		})
	}
	
	if indicators.SequentialLayers >= 2 {
		evidence = append(evidence, Evidence{
			Type:        "pattern",
			Description: "Sequential layering patterns detected",
			Value:       decimal.NewFromInt(int64(indicators.SequentialLayers)).String(),
			Timestamp:   time.Now(),
		})
	}
	
	if indicators.TimeClusterScore > 0.6 {
		evidence = append(evidence, Evidence{
			Type:        "timing",
			Description: "Orders clustered in suspicious time patterns",
			Value:       decimal.NewFromFloat(indicators.TimeClusterScore).String(),
			Timestamp:   time.Now(),
		})
	}
	
	return evidence
}

// determineSeverity determines severity based on confidence score for layering
func (ld *LayeringDetector) determineSeverity(confidence decimal.Decimal) string {
	if confidence.GreaterThan(decimal.NewFromFloat(75)) {
		return "critical"
	} else if confidence.GreaterThan(decimal.NewFromFloat(60)) {
		return "high"
	} else if confidence.GreaterThan(decimal.NewFromFloat(45)) {
		return "medium"
	}
	return "low"
}
