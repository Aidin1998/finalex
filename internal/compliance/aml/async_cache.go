package aml

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime/debug"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/trading/model"
	"github.com/shopspring/decimal"
)

// CircuitBreaker implements circuit breaker pattern for fallback
type CircuitBreaker struct {
	failureThreshold float64
	resetTimeout     time.Duration
	failureCount     int64
	successCount     int64
	lastFailureTime  time.Time
	state            CircuitBreakerState
}

// CircuitBreakerState represents circuit breaker states
type CircuitBreakerState int

const (
	CircuitBreakerClosed CircuitBreakerState = iota
	CircuitBreakerOpen
	CircuitBreakerHalfOpen
)

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(failureThreshold float64) *CircuitBreaker {
	return &CircuitBreaker{
		failureThreshold: failureThreshold,
		resetTimeout:     time.Minute * 2, // 2 minute reset timeout
		state:            CircuitBreakerClosed,
	}
}

// IsOpen returns true if the circuit breaker is open
func (cb *CircuitBreaker) IsOpen() bool {
	if cb.state == CircuitBreakerOpen {
		// Check if we should transition to half-open
		if time.Since(cb.lastFailureTime) > cb.resetTimeout {
			cb.state = CircuitBreakerHalfOpen
			return false
		}
		return true
	}
	return false
}

// RecordSuccess records a successful operation
func (cb *CircuitBreaker) RecordSuccess() {
	cb.successCount++
	if cb.state == CircuitBreakerHalfOpen {
		cb.state = CircuitBreakerClosed
		cb.failureCount = 0
	}
}

// RecordFailure records a failed operation
func (cb *CircuitBreaker) RecordFailure() {
	cb.failureCount++
	cb.lastFailureTime = time.Now()

	totalRequests := cb.failureCount + cb.successCount
	if totalRequests > 10 { // Minimum sample size
		failureRate := float64(cb.failureCount) / float64(totalRequests)
		if failureRate >= cb.failureThreshold {
			cb.state = CircuitBreakerOpen
		}
	}
}

// getCachedRiskMetrics retrieves risk metrics from Redis cache
func (s *AsyncRiskService) getCachedRiskMetrics(ctx context.Context, userID string) (*RiskMetrics, error) {
	cacheKey := fmt.Sprintf(s.cacheKeys.RiskMetrics, userID)
	// Set timeout for cache operation
	cacheCtx, cancel := context.WithTimeout(ctx, s.config.CacheTimeout)
	defer cancel()
	data, err := s.redisClient.Get(cacheCtx, cacheKey).Result()
	if err != nil {
		if err.Error() == "redis: nil" {
			return nil, nil // Cache miss
		}
		return nil, fmt.Errorf("cache get error: %w", err)
	}

	var metrics RiskMetrics
	if err := json.Unmarshal([]byte(data), &metrics); err != nil {
		return nil, fmt.Errorf("cache unmarshal error: %w", err)
	}
	// Check if cached data is still fresh
	if time.Since(metrics.LastCalculated) > s.cacheConfig.RiskMetricsTTL {
		return nil, nil // Stale data, treat as cache miss
	}

	return &metrics, nil
}

// cacheRiskMetrics stores risk metrics in Redis cache
func (s *AsyncRiskService) cacheRiskMetrics(ctx context.Context, userID string, metrics *RiskMetrics) error {
	cacheKey := fmt.Sprintf(s.cacheKeys.RiskMetrics, userID)

	// Set timeout for cache operation
	cacheCtx, cancel := context.WithTimeout(ctx, s.config.CacheTimeout)
	defer cancel()

	data, err := json.Marshal(metrics)
	if err != nil {
		return fmt.Errorf("cache marshal error: %w", err)
	}
	err = s.redisClient.Set(cacheCtx, cacheKey, data, s.cacheConfig.RiskMetricsTTL).Err()
	if err != nil {
		return fmt.Errorf("cache set error: %w", err)
	}

	// Also cache user profile data
	profileKey := fmt.Sprintf(s.cacheKeys.UserRiskProfile, userID)
	err = s.redisClient.Set(cacheCtx, profileKey, data, s.cacheConfig.UserRiskProfileTTL).Err()
	if err != nil {
		s.logger.Warnw("Failed to cache user profile", "user_id", userID, "error", err)
	}

	return nil
}

// cacheBatchRiskMetrics caches multiple risk metrics
func (s *AsyncRiskService) cacheBatchRiskMetrics(ctx context.Context, batchResults map[string]*RiskMetrics) error {
	if len(batchResults) == 0 {
		return nil
	}

	// Set timeout for cache operation
	cacheCtx, cancel := context.WithTimeout(ctx, s.config.CacheTimeout)
	defer cancel()
	// Use pipeline for batch operations
	pipe := s.redisClient.Pipeline()

	for userID, metrics := range batchResults {
		cacheKey := fmt.Sprintf(s.cacheKeys.RiskMetrics, userID)

		data, err := json.Marshal(metrics)
		if err != nil {
			s.logger.Warnw("Failed to marshal metrics for caching", "user_id", userID, "error", err)
			continue
		}

		pipe.Set(cacheCtx, cacheKey, data, s.cacheConfig.RiskMetricsTTL)

		// Also cache user profile
		profileKey := fmt.Sprintf(s.cacheKeys.UserRiskProfile, userID)
		pipe.Set(cacheCtx, profileKey, data, s.cacheConfig.UserRiskProfileTTL)
	}

	_, err := pipe.Exec(cacheCtx)
	if err != nil {
		return fmt.Errorf("batch cache error: %w", err)
	}

	s.logger.Debugw("Cached batch risk metrics", "count", len(batchResults))
	return nil
}

// cacheComplianceResult caches compliance check results
func (s *AsyncRiskService) cacheComplianceResult(ctx context.Context, transactionID string, result *ComplianceResult) error {
	cacheKey := fmt.Sprintf(s.cacheKeys.TransactionFlags, transactionID)

	// Set timeout for cache operation
	cacheCtx, cancel := context.WithTimeout(ctx, s.config.CacheTimeout)
	defer cancel()

	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("compliance result marshal error: %w", err)
	}

	// Cache compliance results for shorter duration
	ttl := time.Hour * 2 // 2 hours for compliance results
	err = s.redisClient.Set(cacheCtx, cacheKey, data, ttl).Err()
	if err != nil {
		return fmt.Errorf("compliance result cache error: %w", err)
	}

	return nil
}

// invalidateUserCache removes cached data for a user
func (s *AsyncRiskService) invalidateUserCache(ctx context.Context, userID string) error {
	// Set timeout for cache operation
	cacheCtx, cancel := context.WithTimeout(ctx, s.config.CacheTimeout)
	defer cancel()

	// Remove risk metrics and user profile
	keys := []string{
		fmt.Sprintf(s.cacheKeys.RiskMetrics, userID),
		fmt.Sprintf(s.cacheKeys.UserRiskProfile, userID),
		fmt.Sprintf(s.cacheKeys.RealtimeRisk, userID),
		fmt.Sprintf(s.cacheKeys.RealtimePositions, userID),
	}
	err := s.redisClient.Del(cacheCtx, keys...).Err()
	if err != nil {
		return fmt.Errorf("cache invalidation error: %w", err)
	}

	s.logger.Debugw("Invalidated user cache", "user_id", userID)
	return nil
}

// publishTradeUpdate publishes real-time trade updates
func (s *AsyncRiskService) publishTradeUpdate(trade *model.Trade) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.config.CacheTimeout)
	defer cancel()
	updateData := map[string]interface{}{
		"type":      "trade_update",
		"trade_id":  trade.ID.String(),
		"user_id":   trade.OrderID.String(),
		"symbol":    trade.Pair,
		"quantity":  trade.Quantity,
		"price":     trade.Price,
		"timestamp": time.Now(),
	}

	data, err := json.Marshal(updateData)
	if err != nil {
		return fmt.Errorf("trade update marshal error: %w", err)
	}

	// Publish to risk updates channel
	err = s.redisClient.Publish(ctx, "risk:updates", data).Err()
	if err != nil {
		return fmt.Errorf("trade update publish error: %w", err)
	}

	return nil
}

// handleRealtimeUpdate processes real-time updates from Redis pub/sub
func (s *AsyncRiskService) handleRealtimeUpdate(channel, message string) error {
	defer func() {
		if r := recover(); r != nil {
			s.logger.Errorw("Panic in real-time update handler", "panic", r, "stack", string(debug.Stack()))
		}
	}()

	var updateData map[string]interface{}
	if err := json.Unmarshal([]byte(message), &updateData); err != nil {
		return fmt.Errorf("failed to unmarshal update: %w", err)
	}

	updateType, _ := updateData["type"].(string)

	switch updateType {
	case "trade_update":
		return s.handleTradeUpdate(updateData)
	case "market_update":
		return s.handleMarketUpdate(updateData)
	case "compliance_alert":
		return s.handleComplianceAlert(updateData)
	default:
		s.logger.Debugw("Unknown update type", "type", updateType, "channel", channel)
	}

	return nil
}

// handleTradeUpdate processes trade updates
func (s *AsyncRiskService) handleTradeUpdate(data map[string]interface{}) error {
	userID, _ := data["user_id"].(string)
	if userID == "" {
		return fmt.Errorf("missing user_id in trade update")
	}

	// Invalidate cache for the user
	ctx, cancel := context.WithTimeout(context.Background(), s.config.CacheTimeout)
	defer cancel()

	return s.invalidateUserCache(ctx, userID)
}

// handleMarketUpdate processes market data updates
func (s *AsyncRiskService) handleMarketUpdate(data map[string]interface{}) error {
	symbol, _ := data["symbol"].(string)
	if symbol == "" {
		return fmt.Errorf("missing symbol in market update")
	}

	// Cache market data
	ctx, cancel := context.WithTimeout(context.Background(), s.config.CacheTimeout)
	defer cancel()

	cacheKey := fmt.Sprintf(s.cacheKeys.MarketData, symbol)
	marketData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("market data marshal error: %w", err)
	}

	err = s.redisClient.Set(ctx, cacheKey, marketData, time.Minute*5).Err()
	if err != nil {
		return fmt.Errorf("market data cache error: %w", err)
	}

	return nil
}

// handleComplianceAlert processes compliance alerts
func (s *AsyncRiskService) handleComplianceAlert(data map[string]interface{}) error {
	userID, _ := data["user_id"].(string)
	alertType, _ := data["alert_type"].(string)

	s.logger.Infow("Compliance alert received",
		"user_id", userID,
		"alert_type", alertType,
		"data", data)

	// Cache the alert
	ctx, cancel := context.WithTimeout(context.Background(), s.config.CacheTimeout)
	defer cancel()

	cacheKey := fmt.Sprintf(s.cacheKeys.ComplianceAlerts, userID)
	alertData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("compliance alert marshal error: %w", err)
	}

	// Add to user's alert list
	err = s.redisClient.LPush(ctx, cacheKey, alertData).Err()
	if err != nil {
		return fmt.Errorf("compliance alert cache error: %w", err)
	}

	// Trim list to keep only recent alerts
	err = s.redisClient.LTrim(ctx, cacheKey, 0, 99).Err() // Keep last 100 alerts
	if err != nil {
		s.logger.Warnw("Failed to trim alert list", "error", err)
	}

	// Set expiration on the list
	err = s.redisClient.Expire(ctx, cacheKey, time.Hour*24).Err()
	if err != nil {
		s.logger.Warnw("Failed to set alert list expiration", "error", err)
	}

	return nil
}

// Fallback methods for when async processing fails or circuit breaker is open

// fallbackCalculateRisk uses the base service directly
func (s *AsyncRiskService) fallbackCalculateRisk(ctx context.Context, userID string) (*RiskMetrics, error) {
	s.logger.Warnw("Using fallback risk calculation", "user_id", userID)
	riskProfile, err := s.baseService.CalculateRisk(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &RiskMetrics{
		UserID:            riskProfile.UserID,
		TotalExposure:     riskProfile.CurrentExposure,
		ValueAtRisk:       riskProfile.ValueAtRisk,
		LeverageRatio:     decimal.Zero,
		MarginUtilization: riskProfile.MarginRequired,
		PortfolioValue:    riskProfile.CurrentExposure,
		RiskScore:         decimal.Zero,
		LastCalculated:    time.Now(),
	}, nil
}

// fallbackBatchCalculateRisk uses the base service for batch calculation
func (s *AsyncRiskService) fallbackBatchCalculateRisk(ctx context.Context, userIDs []string) (map[string]*RiskMetrics, error) {
	s.logger.Warnw("Using fallback batch risk calculation", "user_count", len(userIDs))

	batchResults, err := s.baseService.BatchCalculateRisk(ctx, userIDs)
	if err != nil {
		return nil, err
	} // Convert to RiskMetrics (batchResults is already map[string]*RiskMetrics)
	// No conversion needed, just return the results directly
	return batchResults, nil
}
