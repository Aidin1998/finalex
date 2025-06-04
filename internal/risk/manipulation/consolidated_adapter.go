package manipulation

import (
	"github.com/Aidin1998/pincex_unified/internal/compliance/aml/detection/common"
	"go.uber.org/zap"
)

// ConsolidatedDetectorAdapter adapts the consolidated pattern framework to work with existing manipulation detection
type ConsolidatedDetectorAdapter struct {
	registry *common.PatternRegistry
	logger   *zap.SugaredLogger
}

// NewConsolidatedDetectorAdapter creates a new adapter
func NewConsolidatedDetectorAdapter(logger *zap.SugaredLogger) *ConsolidatedDetectorAdapter {
	registry := common.NewPatternRegistry(logger)

	// Register consolidated detectors
	washTradingDetector := common.NewConsolidatedWashTradingDetector(logger, nil)
	registry.RegisterDetector(washTradingDetector)

	return &ConsolidatedDetectorAdapter{
		registry: registry,
		logger:   logger,
	}
}

// ConvertTradingActivity converts manipulation.TradingActivity to common.TradingActivity
func (cda *ConsolidatedDetectorAdapter) ConvertTradingActivity(activity *TradingActivity) *common.TradingActivity {
	// Convert orders
	var orders []common.Order
	for _, order := range activity.Orders {
		orders = append(orders, common.Order{
			ID:        order.ID.String(),
			UserID:    order.UserID.String(),
			Market:    order.Pair,
			Side:      order.Side,
			Size:      order.Quantity,
			Price:     order.Price,
			Timestamp: order.CreatedAt,
			Status:    order.Status,
		})
	}

	// Convert trades
	var trades []common.Trade
	for _, trade := range activity.Trades {
		// For trades, we need to extract user information differently since trades don't directly contain user IDs
		// We'll use the OrderID to get user information (simplified approach)
		buyUserID := trade.OrderID.String()  // Simplified - would need proper user mapping
		sellUserID := trade.OrderID.String() // Simplified - would need proper user mapping

		trades = append(trades, common.Trade{
			ID:          trade.ID.String(),
			Market:      trade.Pair,
			BuyUserID:   buyUserID,
			SellUserID:  sellUserID,
			Size:        trade.Quantity,
			Price:       trade.Price,
			Timestamp:   trade.CreatedAt,
			BuyOrderID:  trade.OrderID.String(),
			SellOrderID: trade.OrderID.String(),
		})
	}

	return &common.TradingActivity{
		UserID:     activity.UserID,
		Market:     activity.Market,
		TimeWindow: activity.TimeWindow,
		Orders:     orders,
		Trades:     trades,
		Metadata:   make(map[string]interface{}),
	}
}

// ConvertDetectionPattern converts common.DetectionPattern to manipulation.ManipulationPattern
func (cda *ConsolidatedDetectorAdapter) ConvertDetectionPattern(pattern *common.DetectionPattern) *ManipulationPattern {
	// Convert evidence
	var evidence []Evidence
	for _, e := range pattern.Evidence {
		evidence = append(evidence, Evidence{
			Type:        e.Type,
			Description: e.Description,
			Data:        e.Data,
			Timestamp:   e.Timestamp,
		})
	}

	return &ManipulationPattern{
		Type:        pattern.Type,
		Description: pattern.Description,
		Confidence:  pattern.Confidence,
		Severity:    pattern.Severity,
		Evidence:    evidence,
		DetectedAt:  pattern.DetectedAt,
		Metadata:    pattern.Metadata,
	}
}

// DetectWashTrading uses the consolidated detector for wash trading detection
func (cda *ConsolidatedDetectorAdapter) DetectWashTrading(activity *TradingActivity) *ManipulationPattern {
	// Convert to common format
	commonActivity := cda.ConvertTradingActivity(activity)

	// Get the consolidated wash trading detector
	detector, exists := cda.registry.GetDetector("consolidated_wash_trading")
	if !exists {
		cda.logger.Warnw("Consolidated wash trading detector not found")
		return nil
	}

	// Run detection
	pattern := detector.Detect(commonActivity)
	if pattern == nil {
		return nil
	}

	// Convert back to manipulation format
	return cda.ConvertDetectionPattern(pattern)
}
