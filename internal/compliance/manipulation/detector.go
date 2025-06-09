package manipulation

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/Aidin1998/finalex/internal/compliance/interfaces"
	"github.com/Aidin1998/finalex/internal/compliance/risk"
	"github.com/Aidin1998/finalex/internal/trading/engine"
	"github.com/Aidin1998/finalex/internal/trading/model"
)

// TradingEngine defines the enforcement interface for trading engine integration
// This allows ManipulationDetector to call enforcement actions without tight coupling
// to a specific engine implementation.
type TradingEngine interface {
	CancelOrder(req *engine.CancelRequest) error
	GetOrderRepository() model.Repository
}

// ManipulationPattern represents a detected manipulation pattern
type ManipulationPattern struct {
	Type        string                 `json:"type"` // "wash_trading", "spoofing", "pump_dump", "layering"
	Description string                 `json:"description"`
	Confidence  decimal.Decimal        `json:"confidence"` // 0-100
	Severity    string                 `json:"severity"`   // "low", "medium", "high", "critical"
	Evidence    []Evidence             `json:"evidence"`
	DetectedAt  time.Time              `json:"detected_at"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// Evidence represents supporting evidence for manipulation detection
type Evidence struct {
	Type        string                 `json:"type"`
	Description string                 `json:"description"`
	Data        map[string]interface{} `json:"data"`
	Timestamp   time.Time              `json:"timestamp"`
}

// ManipulationAlert represents an alert for detected manipulation
// Add PatternIface for interfaces.ManipulationPattern
type ManipulationAlert struct {
	ID              string                         `json:"id"`
	UserID          string                         `json:"user_id"`
	Market          string                         `json:"market"`
	Pattern         ManipulationPattern            `json:"pattern"`       // legacy/local
	PatternIface    interfaces.ManipulationPattern `json:"pattern_iface"` // canonical
	RiskScore       decimal.Decimal                `json:"risk_score"`
	ActionRequired  string                         `json:"action_required"` // "suspend", "investigate", "monitor"
	Status          string                         `json:"status"`          // "open", "investigating", "resolved"
	CreatedAt       time.Time                      `json:"created_at"`
	UpdatedAt       time.Time                      `json:"updated_at"`
	InvestigatorID  string                         `json:"investigator_id"`
	ResolutionNotes string                         `json:"resolution_notes"`
	AutoSuspended   bool                           `json:"auto_suspended"`
	RelatedOrderIDs []string                       `json:"related_order_ids"`
	RelatedTradeIDs []string                       `json:"related_trade_ids"`
}

// TradingActivity represents user trading activity for analysis
type TradingActivity struct {
	UserID     string            `json:"user_id"`
	Market     string            `json:"market"`
	Orders     []*model.Order    `json:"orders"`
	Trades     []*model.Trade    `json:"trades"`
	TimeWindow time.Duration     `json:"time_window"`
	Metrics    ActivityMetrics   `json:"metrics"`
	Patterns   []DetectedPattern `json:"patterns"`
}

// ActivityMetrics contains calculated metrics for trading activity
type ActivityMetrics struct {
	OrderCount             int             `json:"order_count"`
	TradeCount             int             `json:"trade_count"`
	TotalVolume            decimal.Decimal `json:"total_volume"`
	AverageOrderSize       decimal.Decimal `json:"average_order_size"`
	CancellationRate       decimal.Decimal `json:"cancellation_rate"`
	SelfTradingRatio       decimal.Decimal `json:"self_trading_ratio"`
	PriceImpact            decimal.Decimal `json:"price_impact"`
	OrderBookParticipation decimal.Decimal `json:"orderbook_participation"`
	VolumeConcentration    decimal.Decimal `json:"volume_concentration"`
}

// DetectedPattern represents a suspicious pattern in trading activity
type DetectedPattern struct {
	Type       string                 `json:"type"`
	Confidence decimal.Decimal        `json:"confidence"`
	Evidence   []string               `json:"evidence"`
	Metadata   map[string]interface{} `json:"metadata"`
	Timestamp  time.Time              `json:"timestamp"`
}

// ManipulationDetector is the main detection engine
type ManipulationDetector struct {
	mu          sync.RWMutex
	logger      *zap.SugaredLogger
	riskService risk.RiskService

	// Detection configuration
	config DetectionConfig

	// Active monitoring
	userActivities map[string]*TradingActivity
	marketMetrics  map[string]*MarketMetrics
	alerts         []ManipulationAlert

	// Pattern detection engines
	washTradingDetector *WashTradingDetector
	spoofingDetector    *SpoofingDetector
	pumpDumpDetector    *PumpDumpDetector
	layeringDetector    *LayeringDetector

	// Consolidated pattern detection adapter
	consolidatedAdapter *ConsolidatedDetectorAdapter

	// Performance metrics
	totalDetections      int64
	truePositives        int64
	falsePositives       int64
	averageDetectionTime time.Duration

	// Real-time processing
	orderChan    chan *model.Order
	tradeChan    chan *model.Trade
	stopChan     chan struct{}
	processingWG sync.WaitGroup

	// Trading engine integration (for enforcement actions)
	tradingEngine TradingEngine
}

// DetectionConfig contains configuration for manipulation detection
type DetectionConfig struct {
	// General settings
	Enabled                bool `json:"enabled"`
	DetectionWindowMinutes int  `json:"detection_window_minutes"`
	MinOrdersForAnalysis   int  `json:"min_orders_for_analysis"`
	MaxConcurrentAnalyses  int  `json:"max_concurrent_analyses"`

	// Thresholds
	WashTradingThreshold decimal.Decimal `json:"wash_trading_threshold"`
	SpoofingThreshold    decimal.Decimal `json:"spoofing_threshold"`
	PumpDumpThreshold    decimal.Decimal `json:"pump_dump_threshold"`
	LayeringThreshold    decimal.Decimal `json:"layering_threshold"`

	// Auto-suspension settings
	AutoSuspendEnabled   bool            `json:"auto_suspend_enabled"`
	AutoSuspendThreshold decimal.Decimal `json:"auto_suspend_threshold"`
	// Alert settings
	RealTimeAlertsEnabled bool `json:"real_time_alerts_enabled"`
	AlertCooldownMinutes  int  `json:"alert_cooldown_minutes"`

	// Consolidated adapter settings
	UseConsolidatedAdapter bool                      `json:"use_consolidated_adapter"`
	ConsolidatedConfig     ConsolidatedAdapterConfig `json:"consolidated_config"`
}

// ConsolidatedAdapterConfig contains configuration for the consolidated detection adapter
type ConsolidatedAdapterConfig struct {
	Enabled              bool            `json:"enabled"`
	WashTradingThreshold decimal.Decimal `json:"wash_trading_threshold"`
	MinOrderCount        int             `json:"min_order_count"`
	MinTradeCount        int             `json:"min_trade_count"`
	TimeWindowMinutes    int             `json:"time_window_minutes"`
	MinVolume            decimal.Decimal `json:"min_volume"`
}

// MarketMetrics tracks market-level metrics for manipulation detection
type MarketMetrics struct {
	Market               string          `json:"market"`
	LastUpdated          time.Time       `json:"last_updated"`
	VolumeProfile        VolumeProfile   `json:"volume_profile"`
	PriceMovements       []PricePoint    `json:"price_movements"`
	OrderBookDepth       OrderBookDepth  `json:"orderbook_depth"`
	TradingConcentration decimal.Decimal `json:"trading_concentration"`
	VolatilityIndex      decimal.Decimal `json:"volatility_index"`
	SuspiciousActivity   decimal.Decimal `json:"suspicious_activity"`
}

// VolumeProfile represents trading volume patterns
type VolumeProfile struct {
	TotalVolume   decimal.Decimal   `json:"total_volume"`
	AverageVolume decimal.Decimal   `json:"average_volume"`
	VolumeSpikes  []VolumeSpike     `json:"volume_spikes"`
	HourlyVolumes []decimal.Decimal `json:"hourly_volumes"`
}

// VolumeSpike represents abnormal volume activity
type VolumeSpike struct {
	Timestamp time.Time       `json:"timestamp"`
	Volume    decimal.Decimal `json:"volume"`
	Magnitude decimal.Decimal `json:"magnitude"` // Factor above normal
}

// PricePoint represents price movement data
type PricePoint struct {
	Timestamp time.Time       `json:"timestamp"`
	Price     decimal.Decimal `json:"price"`
	Volume    decimal.Decimal `json:"volume"`
	Change    decimal.Decimal `json:"change"` // Percentage change
}

// OrderBookDepth represents order book analysis
type OrderBookDepth struct {
	BidDepth    decimal.Decimal `json:"bid_depth"`
	AskDepth    decimal.Decimal `json:"ask_depth"`
	Spread      decimal.Decimal `json:"spread"`
	Imbalance   decimal.Decimal `json:"imbalance"`
	LargeOrders int             `json:"large_orders"`
}

// NewManipulationDetector creates a new manipulation detection engine
func NewManipulationDetector(logger *zap.SugaredLogger, riskService risk.RiskService, config DetectionConfig, tradingEngine TradingEngine) *ManipulationDetector {
	md := &ManipulationDetector{
		logger:         logger,
		riskService:    riskService,
		config:         config,
		userActivities: make(map[string]*TradingActivity),
		marketMetrics:  make(map[string]*MarketMetrics),
		alerts:         make([]ManipulationAlert, 0),
		orderChan:      make(chan *model.Order, 10000),
		tradeChan:      make(chan *model.Trade, 10000),
		stopChan:       make(chan struct{}),
		tradingEngine:  tradingEngine,
	}
	// Initialize pattern detection engines
	md.washTradingDetector = NewWashTradingDetector(config, logger)
	md.spoofingDetector = NewSpoofingDetector(config, logger)
	md.pumpDumpDetector = NewPumpDumpDetector(config, logger)
	md.layeringDetector = NewLayeringDetector(config, logger)
	// Initialize consolidated detection adapter
	if config.UseConsolidatedAdapter && config.ConsolidatedConfig.Enabled {
		md.consolidatedAdapter = NewConsolidatedDetectorAdapter(logger)
		// Apply configuration to the adapter
		md.consolidatedAdapter.config = AdapterConfig{
			WashTradingThreshold: config.ConsolidatedConfig.WashTradingThreshold,
			MinOrderCount:        config.ConsolidatedConfig.MinOrderCount,
			MinTradeCount:        config.ConsolidatedConfig.MinTradeCount,
			TimeWindow:           time.Duration(config.ConsolidatedConfig.TimeWindowMinutes) * time.Minute,
			MinVolume:            config.ConsolidatedConfig.MinVolume,
		}
		logger.Infow("Consolidated detection adapter initialized",
			"wash_trading_threshold", config.ConsolidatedConfig.WashTradingThreshold.String(),
			"min_order_count", config.ConsolidatedConfig.MinOrderCount,
			"min_trade_count", config.ConsolidatedConfig.MinTradeCount)
	} else {
		md.consolidatedAdapter = nil
		logger.Info("Consolidated detection adapter disabled, using legacy detectors")
	}

	return md
}

// Start begins the real-time detection engine
func (md *ManipulationDetector) Start(ctx context.Context) error {
	if !md.config.Enabled {
		md.logger.Info("Manipulation detection is disabled")
		return nil
	}

	md.logger.Info("Starting manipulation detection engine",
		"detection_window", md.config.DetectionWindowMinutes,
		"auto_suspend", md.config.AutoSuspendEnabled)

	// Start order processing goroutine
	md.processingWG.Add(1)
	go md.processOrders(ctx)

	// Start trade processing goroutine
	md.processingWG.Add(1)
	go md.processTrades(ctx)

	// Start periodic analysis goroutine
	md.processingWG.Add(1)
	go md.periodicAnalysis(ctx)

	// Start market metrics collection
	md.processingWG.Add(1)
	go md.collectMarketMetrics(ctx)

	return nil
}

// Stop stops the detection engine
func (md *ManipulationDetector) Stop() {
	md.logger.Info("Stopping manipulation detection engine")
	close(md.stopChan)
	md.processingWG.Wait()
	close(md.orderChan)
	close(md.tradeChan)
}

// Clock returns the current time (can be mocked for testing)
func (md *ManipulationDetector) Clock() time.Time {
	return time.Now()
}

// ProcessOrder processes a new order for manipulation detection
func (md *ManipulationDetector) ProcessOrder(order *model.Order) {
	if !md.config.Enabled {
		return
	}

	select {
	case md.orderChan <- order:
	default:
		md.logger.Warn("Order channel full, dropping order for manipulation detection",
			"order_id", order.ID)
	}
}

// ProcessTrade processes a new trade for manipulation detection
func (md *ManipulationDetector) ProcessTrade(trade *model.Trade) {
	if !md.config.Enabled {
		return
	}

	select {
	case md.tradeChan <- trade:
	default:
		md.logger.Warn("Trade channel full, dropping trade for manipulation detection",
			"trade_id", trade.ID)
	}
}

// processOrders processes orders from the order channel
func (md *ManipulationDetector) processOrders(ctx context.Context) {
	defer md.processingWG.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case <-md.stopChan:
			return
		case order := <-md.orderChan:
			if order != nil {
				md.analyzeOrder(order)
			}
		}
	}
}

// processTrades processes trades from the trade channel
func (md *ManipulationDetector) processTrades(ctx context.Context) {
	defer md.processingWG.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case <-md.stopChan:
			return
		case trade := <-md.tradeChan:
			if trade != nil {
				md.analyzeTrade(trade)
			}
		}
	}
}

// analyzeOrder analyzes a single order for manipulation patterns
func (md *ManipulationDetector) analyzeOrder(order *model.Order) {
	startTime := time.Now()
	defer func() {
		md.averageDetectionTime = time.Since(startTime)
	}()

	userID := order.UserID.String()
	market := order.Pair

	md.mu.Lock()
	activity, exists := md.userActivities[userID+":"+market]
	if !exists {
		activity = &TradingActivity{
			UserID:     userID,
			Market:     market,
			Orders:     make([]*model.Order, 0),
			Trades:     make([]*model.Trade, 0),
			TimeWindow: time.Duration(md.config.DetectionWindowMinutes) * time.Minute,
		}
		md.userActivities[userID+":"+market] = activity
	}

	// Add order to activity and trim old data
	activity.Orders = append(activity.Orders, order)
	cutoff := time.Now().Add(-activity.TimeWindow)
	var recentOrders []*model.Order
	for _, o := range activity.Orders {
		if o.CreatedAt.After(cutoff) {
			recentOrders = append(recentOrders, o)
		}
	}
	activity.Orders = recentOrders
	md.mu.Unlock()

	// Perform real-time pattern detection
	if len(activity.Orders) >= md.config.MinOrdersForAnalysis {
		md.performPatternDetection(activity)
	}
}

// analyzeTrade analyzes a single trade for manipulation patterns
func (md *ManipulationDetector) analyzeTrade(trade *model.Trade) {
	userID := trade.OrderID.String() // Simplification - would need user mapping
	market := trade.Pair

	md.mu.Lock()
	activity, exists := md.userActivities[userID+":"+market]
	if !exists {
		activity = &TradingActivity{
			UserID:     userID,
			Market:     market,
			Orders:     make([]*model.Order, 0),
			Trades:     make([]*model.Trade, 0),
			TimeWindow: time.Duration(md.config.DetectionWindowMinutes) * time.Minute,
		}
		md.userActivities[userID+":"+market] = activity
	}

	// Add trade to activity and trim old data
	activity.Trades = append(activity.Trades, trade)
	cutoff := time.Now().Add(-activity.TimeWindow)
	var recentTrades []*model.Trade
	for _, t := range activity.Trades {
		if t.CreatedAt.After(cutoff) {
			recentTrades = append(recentTrades, t)
		}
	}
	activity.Trades = recentTrades
	md.mu.Unlock()

	// Update market metrics
	md.updateMarketMetrics(market, trade)
}

// Conversion helpers
func toInterfacesTradingActivity(activity *TradingActivity) *interfaces.TradingActivity {
	userUUID, _ := uuid.Parse(activity.UserID)
	orders := make([]interfaces.Order, len(activity.Orders))
	for i, o := range activity.Orders {
		orders[i] = interfaces.Order{
			ID:        o.ID.String(),
			UserID:    o.UserID,
			Market:    o.Pair,
			Side:      o.Side,
			Type:      o.Type,
			Quantity:  o.Quantity,
			Price:     o.Price,
			Status:    o.Status,
			CreatedAt: o.CreatedAt,
			UpdatedAt: o.UpdatedAt,
			// ExecutedAt and CancelledAt are not present in model.Order, so leave nil
		}
	}
	trades := make([]interfaces.Trade, len(activity.Trades))
	for i, t := range activity.Trades {
		trades[i] = interfaces.Trade{
			ID:        t.ID.String(),
			Market:    t.Pair,
			BuyerID:   t.UserID,        // Approximate: model.Trade does not distinguish buyer/seller
			SellerID:  t.CounterUserID, // Approximate
			Quantity:  t.Quantity,
			Price:     t.Price,
			Timestamp: t.CreatedAt,
			OrderIDs:  []string{t.OrderID.String(), t.CounterOrderID.String()},
		}
	}
	return &interfaces.TradingActivity{
		UserID:     userUUID,
		Market:     activity.Market,
		Orders:     orders,
		Trades:     trades,
		TimeWindow: activity.TimeWindow,
	}
}

func toInterfacesPattern(p ManipulationPattern) interfaces.ManipulationPattern {
	// Map string type to ManipulationAlertType
	var alertType interfaces.ManipulationAlertType
	switch p.Type {
	case "wash_trading":
		alertType = interfaces.ManipulationAlertWashTrading
	case "spoofing":
		alertType = interfaces.ManipulationAlertSpoofing
	case "layering":
		alertType = interfaces.ManipulationAlertLayering
	case "pump_dump":
		alertType = interfaces.ManipulationAlertPumpAndDump
	default:
		alertType = interfaces.ManipulationAlertType(0)
	}
	// Convert Evidence to []PatternEvidence (best effort)
	var evidence []interfaces.PatternEvidence
	for _, e := range p.Evidence {
		evidence = append(evidence, interfaces.PatternEvidence{
			Type:        e.Type,
			Description: e.Description,
			Value:       e.Data,
			Timestamp:   e.Timestamp,
			Metadata:    nil,
		})
	}
	return interfaces.ManipulationPattern{
		Type:        alertType,
		Confidence:  p.Confidence,
		Description: p.Description,
		Evidence:    evidence,
		TimeWindow:  0, // Not tracked in local type
		DetectedAt:  p.DetectedAt,
		Metadata:    p.Metadata,
	}
}

// performPatternDetection runs all pattern detection algorithms
func (md *ManipulationDetector) performPatternDetection(activity *TradingActivity) {
	// Calculate activity metrics
	activity.Metrics = md.calculateActivityMetrics(activity)

	var detectedPatterns []interfaces.ManipulationPattern

	// Use consolidated wash trading detector if available
	if md.consolidatedAdapter != nil {
		convertedActivity := toInterfacesTradingActivity(activity)
		if washPattern := md.consolidatedAdapter.DetectWashTrading(convertedActivity); washPattern != nil {
			detectedPatterns = append(detectedPatterns, *washPattern)
			md.logger.Infow("Consolidated wash trading pattern detected",
				"user_id", activity.UserID,
				"confidence", washPattern.Confidence.String())
		}
	} else {
		// Fallback to legacy wash trading detector
		if washPattern := md.washTradingDetector.Detect(activity); washPattern != nil {
			detectedPatterns = append(detectedPatterns, toInterfacesPattern(*washPattern))
		}
	}

	// Continue using existing detectors for other patterns (for now)
	if spoofPattern := md.spoofingDetector.Detect(activity); spoofPattern != nil {
		detectedPatterns = append(detectedPatterns, toInterfacesPattern(*spoofPattern))
	}

	if pumpDumpPattern := md.pumpDumpDetector.Detect(activity); pumpDumpPattern != nil {
		detectedPatterns = append(detectedPatterns, toInterfacesPattern(*pumpDumpPattern))
	}

	if layeringPattern := md.layeringDetector.Detect(activity); layeringPattern != nil {
		detectedPatterns = append(detectedPatterns, toInterfacesPattern(*layeringPattern))
	}

	// Process detected patterns
	for _, pattern := range detectedPatterns {
		md.handleDetectedPattern(activity, pattern)
	}
}

// calculateActivityMetrics calculates metrics for trading activity
func (md *ManipulationDetector) calculateActivityMetrics(activity *TradingActivity) ActivityMetrics {
	metrics := ActivityMetrics{
		OrderCount: len(activity.Orders),
		TradeCount: len(activity.Trades),
	}

	if len(activity.Orders) == 0 {
		return metrics
	}

	var totalVolume decimal.Decimal
	var cancelledOrders int

	for _, order := range activity.Orders {
		orderVolume := order.Quantity.Mul(order.Price)
		totalVolume = totalVolume.Add(orderVolume)

		if order.Status == model.OrderStatusCancelled {
			cancelledOrders++
		}
	}

	metrics.TotalVolume = totalVolume
	metrics.AverageOrderSize = totalVolume.Div(decimal.NewFromInt(int64(len(activity.Orders))))
	metrics.CancellationRate = decimal.NewFromInt(int64(cancelledOrders)).Div(decimal.NewFromInt(int64(len(activity.Orders))))

	// Calculate additional metrics based on trades
	if len(activity.Trades) > 0 {
		// Self-trading ratio would require cross-referencing user orders
		// Price impact would require market data comparison
		// These would be implemented with additional data sources
	}

	return metrics
}

// Update handleDetectedPattern to accept interfaces.ManipulationPattern
func (md *ManipulationDetector) handleDetectedPattern(activity *TradingActivity, pattern interfaces.ManipulationPattern) {
	alert := ManipulationAlert{
		ID:              fmt.Sprintf("alert_%d", time.Now().UnixNano()),
		UserID:          activity.UserID,
		Market:          activity.Market,
		PatternIface:    pattern,
		RiskScore:       pattern.Confidence,
		Status:          "open",
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
		RelatedOrderIDs: md.extractOrderIDs(activity.Orders),
		RelatedTradeIDs: md.extractTradeIDs(activity.Trades),
	}

	// Determine action required based on risk score and pattern type
	alert.ActionRequired = md.determineAction(pattern)

	// Auto-suspend if enabled and threshold exceeded
	if md.config.AutoSuspendEnabled && pattern.Confidence.GreaterThan(md.config.AutoSuspendThreshold) {
		alert.AutoSuspended = true
		md.suspendUserTrading(activity.UserID, activity.Market, alert.ID)
	}

	md.mu.Lock()
	md.alerts = append(md.alerts, alert)
	md.totalDetections++
	md.mu.Unlock()

	md.logger.Warnw("Manipulation pattern detected",
		"user_id", activity.UserID,
		"market", activity.Market,
		"pattern_type", pattern.Type.String(),
		"confidence", pattern.Confidence,
		"auto_suspended", alert.AutoSuspended)

	// Send real-time alert if enabled
	if md.config.RealTimeAlertsEnabled {
		md.sendRealTimeAlert(alert)
	}
}

// Update determineAction to use interfaces.ManipulationPattern
func (md *ManipulationDetector) determineAction(pattern interfaces.ManipulationPattern) string {
	confidence := pattern.Confidence.InexactFloat64()
	switch pattern.Type {
	case interfaces.ManipulationAlertWashTrading, interfaces.ManipulationAlertPumpAndDump:
		if confidence >= 90 {
			return "suspend"
		}
		return "investigate"
	case interfaces.ManipulationAlertSpoofing, interfaces.ManipulationAlertLayering:
		if confidence >= 80 {
			return "investigate"
		}
		return "monitor"
	default:
		if confidence >= 70 {
			return "monitor"
		}
		return "review"
	}
}

// suspendUserTrading suspends a user's trading activities
func (md *ManipulationDetector) suspendUserTrading(userID, market, alertID string) {
	md.logger.Warnw("Auto-suspending user trading",
		"user_id", userID,
		"market", market,
		"alert_id", alertID)

	// Integration with trading engine: cancel all open orders for the user in the market
	if md.tradingEngine != nil {
		ctx := context.Background()
		// Cancel all open orders for this user and market
		userUUID, err := uuid.Parse(userID)
		if err == nil {
			orders, err := md.tradingEngine.GetOrderRepository().GetOpenOrdersByUser(ctx, userUUID)
			if err == nil {
				for _, order := range orders {
					if order.Pair == market {
						_ = md.tradingEngine.CancelOrder(&engine.CancelRequest{OrderID: order.ID})
					}
				}
			}
		}
		// Optionally: add user to a suspension list (in-memory or persistent)
		// Optionally: block new order submissions (requires integration with trading service)
	}
	// Notify compliance team (already handled by alerting/compliance tooling)
}

// sendRealTimeAlert sends a real-time alert notification
func (md *ManipulationDetector) sendRealTimeAlert(alert ManipulationAlert) {
	// Implementation would integrate with notification system
	// Could use WebSocket, email, Slack, etc.
	md.logger.Infow("Sending real-time manipulation alert",
		"alert_id", alert.ID,
		"user_id", alert.UserID,
		"pattern_type", alert.Pattern.Type)
}

// extractOrderIDs extracts order IDs from a list of orders
func (md *ManipulationDetector) extractOrderIDs(orders []*model.Order) []string {
	ids := make([]string, len(orders))
	for i, order := range orders {
		ids[i] = order.ID.String()
	}
	return ids
}

// extractTradeIDs extracts trade IDs from a list of trades
func (md *ManipulationDetector) extractTradeIDs(trades []*model.Trade) []string {
	ids := make([]string, len(trades))
	for i, trade := range trades {
		ids[i] = trade.ID.String()
	}
	return ids
}

// updateMarketMetrics updates market-level metrics
func (md *ManipulationDetector) updateMarketMetrics(market string, trade *model.Trade) {
	md.mu.Lock()
	defer md.mu.Unlock()

	metrics, exists := md.marketMetrics[market]
	if !exists {
		metrics = &MarketMetrics{
			Market:         market,
			LastUpdated:    time.Now(),
			PriceMovements: make([]PricePoint, 0),
		}
		md.marketMetrics[market] = metrics
	}

	// Add price point
	pricePoint := PricePoint{
		Timestamp: trade.CreatedAt,
		Price:     trade.Price,
		Volume:    trade.Quantity,
	}

	// Calculate price change if we have previous data
	if len(metrics.PriceMovements) > 0 {
		lastPrice := metrics.PriceMovements[len(metrics.PriceMovements)-1].Price
		if !lastPrice.IsZero() {
			change := trade.Price.Sub(lastPrice).Div(lastPrice).Mul(decimal.NewFromInt(100))
			pricePoint.Change = change
		}
	}

	metrics.PriceMovements = append(metrics.PriceMovements, pricePoint)

	// Keep only recent data (last hour)
	cutoff := time.Now().Add(-time.Hour)
	var recentPoints []PricePoint
	for _, point := range metrics.PriceMovements {
		if point.Timestamp.After(cutoff) {
			recentPoints = append(recentPoints, point)
		}
	}
	metrics.PriceMovements = recentPoints
	metrics.LastUpdated = time.Now()
}

// periodicAnalysis performs periodic analysis of accumulated data
func (md *ManipulationDetector) periodicAnalysis(ctx context.Context) {
	defer md.processingWG.Done()

	ticker := time.NewTicker(5 * time.Minute) // Analysis every 5 minutes
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-md.stopChan:
			return
		case <-ticker.C:
			md.performPeriodicAnalysis()
		}
	}
}

// performPeriodicAnalysis performs comprehensive analysis of all user activities
func (md *ManipulationDetector) performPeriodicAnalysis() {
	md.mu.RLock()
	activities := make([]*TradingActivity, 0, len(md.userActivities))
	for _, activity := range md.userActivities {
		activities = append(activities, activity)
	}
	md.mu.RUnlock()

	for _, activity := range activities {
		if len(activity.Orders) >= md.config.MinOrdersForAnalysis {
			md.performPatternDetection(activity)
		}
	}

	md.logger.Debugw("Periodic manipulation analysis completed",
		"activities_analyzed", len(activities),
		"total_detections", md.totalDetections)
}

// collectMarketMetrics collects market-level metrics for analysis
func (md *ManipulationDetector) collectMarketMetrics(ctx context.Context) {
	defer md.processingWG.Done()

	ticker := time.NewTicker(1 * time.Minute) // Collect metrics every minute
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-md.stopChan:
			return
		case <-ticker.C:
			md.updateMarketAnalysis()
		}
	}
}

// updateMarketAnalysis updates market-level analysis metrics
func (md *ManipulationDetector) updateMarketAnalysis() {
	md.mu.Lock()
	defer md.mu.Unlock()

	for market, metrics := range md.marketMetrics {
		if len(metrics.PriceMovements) < 2 {
			continue
		}

		// Calculate volatility index
		var priceChanges []float64
		for _, point := range metrics.PriceMovements {
			if !point.Change.IsZero() {
				priceChanges = append(priceChanges, point.Change.InexactFloat64())
			}
		}

		if len(priceChanges) > 0 {
			volatility := md.calculateVolatility(priceChanges)
			metrics.VolatilityIndex = decimal.NewFromFloat(volatility)
		}

		// Calculate suspicious activity score based on various factors
		suspiciousScore := md.calculateSuspiciousActivityScore(market, metrics)
		metrics.SuspiciousActivity = suspiciousScore

		md.logger.Debugw("Market metrics updated",
			"market", market,
			"price_points", len(metrics.PriceMovements),
			"volatility", metrics.VolatilityIndex,
			"suspicious_activity", metrics.SuspiciousActivity)
	}
}

// calculateVolatility calculates price volatility from price changes
func (md *ManipulationDetector) calculateVolatility(changes []float64) float64 {
	if len(changes) == 0 {
		return 0
	}

	// Calculate mean
	sum := 0.0
	for _, change := range changes {
		sum += change
	}
	mean := sum / float64(len(changes))

	// Calculate variance
	variance := 0.0
	for _, change := range changes {
		variance += math.Pow(change-mean, 2)
	}
	variance /= float64(len(changes))

	// Return standard deviation (volatility)
	return math.Sqrt(variance)
}

// calculateSuspiciousActivityScore calculates a suspicious activity score for a market
func (md *ManipulationDetector) calculateSuspiciousActivityScore(market string, metrics *MarketMetrics) decimal.Decimal {
	score := decimal.Zero

	// Factor in high volatility (weights: 0-30 points)
	if metrics.VolatilityIndex.GreaterThan(decimal.NewFromFloat(10)) { // >10% volatility
		volatilityScore := decimal.Min(metrics.VolatilityIndex.Mul(decimal.NewFromFloat(3)), decimal.NewFromInt(30))
		score = score.Add(volatilityScore)
	}

	// Factor in unusual volume patterns (0-20 points)
	// This would require baseline volume data for comparison

	// Factor in number of recent alerts for this market (0-50 points)
	recentAlerts := md.getRecentAlertsForMarket(market)
	alertScore := decimal.Min(decimal.NewFromInt(int64(recentAlerts*10)), decimal.NewFromInt(50))
	score = score.Add(alertScore)

	// Cap at 100
	return decimal.Min(score, decimal.NewFromInt(100))
}

// getRecentAlertsForMarket gets the number of recent alerts for a market
func (md *ManipulationDetector) getRecentAlertsForMarket(market string) int {
	cutoff := time.Now().Add(-time.Hour)
	count := 0

	for _, alert := range md.alerts {
		if alert.Market == market && alert.CreatedAt.After(cutoff) {
			count++
		}
	}

	return count
}

// GetActiveAlerts returns all active manipulation alerts
func (md *ManipulationDetector) GetActiveAlerts() []ManipulationAlert {
	md.mu.RLock()
	defer md.mu.RUnlock()

	var activeAlerts []ManipulationAlert
	for _, alert := range md.alerts {
		if alert.Status == "open" || alert.Status == "investigating" {
			activeAlerts = append(activeAlerts, alert)
		}
	}

	return activeAlerts
}

// GetDetectionMetrics returns performance metrics for the detection engine
func (md *ManipulationDetector) GetDetectionMetrics() map[string]interface{} {
	md.mu.RLock()
	defer md.mu.RUnlock()

	accuracyRate := float64(0)
	if md.totalDetections > 0 {
		accuracyRate = float64(md.truePositives) / float64(md.totalDetections) * 100
	}

	return map[string]interface{}{
		"total_detections":       md.totalDetections,
		"true_positives":         md.truePositives,
		"false_positives":        md.falsePositives,
		"accuracy_rate":          accuracyRate,
		"average_detection_time": md.averageDetectionTime.String(),
		"active_alerts":          len(md.GetActiveAlerts()),
		"monitored_activities":   len(md.userActivities),
		"monitored_markets":      len(md.marketMetrics),
	}
}

// GetMetrics returns performance metrics for the detection engine
func (md *ManipulationDetector) GetMetrics() map[string]interface{} {
	return md.GetDetectionMetrics()
}

// GetStatus returns the current status of the detection engine
func (md *ManipulationDetector) GetStatus() map[string]interface{} {
	md.mu.RLock()
	defer md.mu.RUnlock()

	return map[string]interface{}{
		"enabled":                md.config.Enabled,
		"total_detections":       md.totalDetections,
		"active_alerts":          len(md.GetActiveAlerts()),
		"monitored_activities":   len(md.userActivities),
		"monitored_markets":      len(md.marketMetrics),
		"average_detection_time": md.averageDetectionTime.String(),
	}
}

// GetAlerts returns all alerts with optional filtering
func (md *ManipulationDetector) GetAlerts() []ManipulationAlert {
	md.mu.RLock()
	defer md.mu.RUnlock()

	return md.alerts
}

// GetAlert returns a specific alert by ID
func (md *ManipulationDetector) GetAlert(alertID string) (*ManipulationAlert, error) {
	md.mu.RLock()
	defer md.mu.RUnlock()

	for _, alert := range md.alerts {
		if alert.ID == alertID {
			return &alert, nil
		}
	}

	return nil, fmt.Errorf("alert not found: %s", alertID)
}

// UpdateConfig updates the detection configuration
func (md *ManipulationDetector) UpdateConfig(config DetectionConfig) {
	md.mu.Lock()
	defer md.mu.Unlock()

	md.config = config
	md.logger.Infow("Detection configuration updated",
		"enabled", config.Enabled,
		"detection_window", config.DetectionWindowMinutes,
		"auto_suspend", config.AutoSuspendEnabled)
}

// UpdateAlertStatus updates the status of a manipulation alert
func (md *ManipulationDetector) UpdateAlertStatus(alertID, status, investigatorID, notes string) error {
	md.mu.Lock()
	defer md.mu.Unlock()

	for i, alert := range md.alerts {
		if alert.ID == alertID {
			md.alerts[i].Status = status
			md.alerts[i].InvestigatorID = investigatorID
			md.alerts[i].ResolutionNotes = notes
			md.alerts[i].UpdatedAt = time.Now()

			// Update performance metrics based on resolution
			if status == "resolved" {
				if notes == "true_positive" {
					md.truePositives++
				} else if notes == "false_positive" {
					md.falsePositives++
				}
			}

			md.logger.Infow("Manipulation alert status updated",
				"alert_id", alertID,
				"status", status,
				"investigator", investigatorID)

			return nil
		}
	}

	return fmt.Errorf("alert not found: %s", alertID)
}
