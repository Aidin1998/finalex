package aml

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/redis"
	redisClient "github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// RiskWorker handles async risk calculation requests
type RiskWorker struct {
	id       string
	service  *AsyncRiskService
	logger   *zap.SugaredLogger
	stopChan chan struct{}
	wg       *sync.WaitGroup
}

// CacheWorker handles cache operations
type CacheWorker struct {
	id       string
	service  *AsyncRiskService
	logger   *zap.SugaredLogger
	stopChan chan struct{}
	wg       *sync.WaitGroup
}

// PubSubWorker handles Redis pub/sub for real-time updates
type PubSubWorker struct {
	redisClient *redis.Client
	pubsub      *redisClient.PubSub
	handler     func(channel, message string) error
	logger      *zap.SugaredLogger
	stopChan    chan struct{}
	wg          sync.WaitGroup
}

// NewRiskWorker creates a new risk calculation worker
func NewRiskWorker(id string, service *AsyncRiskService, logger *zap.SugaredLogger) *RiskWorker {
	return &RiskWorker{
		id:       id,
		service:  service,
		logger:   logger.With("worker_id", id, "worker_type", "risk"),
		stopChan: make(chan struct{}),
		wg:       &service.workers,
	}
}

// NewCacheWorker creates a new cache worker
func NewCacheWorker(id string, service *AsyncRiskService, logger *zap.SugaredLogger) *CacheWorker {
	return &CacheWorker{
		id:       id,
		service:  service,
		logger:   logger.With("worker_id", id, "worker_type", "cache"),
		stopChan: make(chan struct{}),
		wg:       &service.workers,
	}
}

// NewPubSubWorker creates a new pub/sub worker
func NewPubSubWorker(redisClient *redis.Client, handler func(string, string) error, logger *zap.SugaredLogger) *PubSubWorker {
	return &PubSubWorker{
		redisClient: redisClient,
		handler:     handler,
		logger:      logger.With("worker_type", "pubsub"),
		stopChan:    make(chan struct{}),
	}
}

// Start begins the risk worker processing loop
func (w *RiskWorker) Start() {
	w.wg.Add(1)
	go w.processLoop()
	w.logger.Info("Risk worker started")
}

// Stop gracefully stops the risk worker
func (w *RiskWorker) Stop() {
	close(w.stopChan)
	w.logger.Info("Risk worker stopping...")
}

// processLoop is the main processing loop for risk calculations
func (w *RiskWorker) processLoop() {
	defer w.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			w.logger.Errorw("Risk worker panic recovered", "panic", r, "stack", string(debug.Stack()))
			// Restart the worker after a brief delay
			time.Sleep(time.Second)
			go w.processLoop()
		}
	}()

	for {
		select {
		case <-w.stopChan:
			w.logger.Info("Risk worker stopped")
			return

		case request := <-w.service.requestChan:
			if request == nil {
				continue
			}

			// Process the request
			result := w.processRequest(request)

			// Send result back
			select {
			case request.ResponseChan <- result:
				// Successfully sent result
			case <-time.After(time.Millisecond * 100):
				w.logger.Warnw("Failed to send result within timeout",
					"request_id", request.RequestID,
					"type", request.Type)
			case <-w.stopChan:
				return
			}
		}
	}
}

// processRequest processes a single risk request
func (w *RiskWorker) processRequest(request *AsyncRiskRequest) *AsyncRiskResult {
	start := time.Now()
	queueTime := start.Sub(request.SubmittedAt)

	result := &AsyncRiskResult{
		RequestID: request.RequestID,
		QueueTime: queueTime,
		Timestamp: time.Now(),
		WorkerID:  w.id,
	}

	// Check for timeout
	if time.Now().After(request.TimeoutAt) {
		result.Success = false
		result.Error = fmt.Errorf("request timed out")
		result.ProcessingTime = time.Since(start)
		return result
	}

	// Process based on request type
	calcStart := time.Now()
	switch request.Type {
	case RiskRequestCalculateUser:
		result = w.processUserRiskCalculation(request, result)
	case RiskRequestValidateOrder:
		result = w.processOrderValidation(request, result)
	case RiskRequestProcessTrade:
		result = w.processTradeProcessing(request, result)
	case RiskRequestBatchCalculate:
		result = w.processBatchCalculation(request, result)
	case RiskRequestComplianceCheck:
		result = w.processComplianceCheck(request, result)
	default:
		result.Success = false
		result.Error = fmt.Errorf("unsupported request type: %s", request.Type)
	}

	result.CalculationTime = time.Since(calcStart)
	result.ProcessingTime = time.Since(start)

	return result
}

// processUserRiskCalculation calculates risk metrics for a user
func (w *RiskWorker) processUserRiskCalculation(request *AsyncRiskRequest, result *AsyncRiskResult) *AsyncRiskResult {
	ctx, cancel := context.WithTimeout(request.Context, w.service.config.RiskCalculationTimeout)
	defer cancel()

	// Try cache first
	cacheStart := time.Now()
	if w.service.config.EnableCaching {
		if cached, err := w.service.getCachedRiskMetrics(ctx, request.UserID); err == nil && cached != nil {
			result.Success = true
			result.RiskMetrics = cached
			result.CacheHit = true
			result.CacheTime = time.Since(cacheStart)
			return result
		}
	}
	result.CacheTime = time.Since(cacheStart)

	// Calculate using base service
	riskProfile, err := w.service.baseService.CalculateRisk(ctx, request.UserID)
	if err != nil {
		result.Success = false
		result.Error = fmt.Errorf("risk calculation failed: %w", err)
		return result
	}

	// Convert to RiskMetrics
	riskMetrics := &RiskMetrics{
		UserID:          riskProfile.UserID,
		CurrentExposure: riskProfile.CurrentExposure,
		ValueAtRisk:     riskProfile.ValueAtRisk,
		MarginRequired:  riskProfile.MarginRequired,
		RiskScore:       w.calculateRiskScore(riskProfile),
		LastUpdated:     time.Now(),
	}

	result.Success = true
	result.RiskMetrics = riskMetrics
	result.CacheHit = false

	// Cache the result asynchronously
	if w.service.config.EnableCaching {
		go w.service.cacheRiskMetrics(ctx, request.UserID, riskMetrics)
	}

	return result
}

// processOrderValidation validates an order against risk limits
func (w *RiskWorker) processOrderValidation(request *AsyncRiskRequest, result *AsyncRiskResult) *AsyncRiskResult {
	if request.Order == nil {
		result.Success = false
		result.Error = fmt.Errorf("order is required for validation")
		return result
	}

	ctx, cancel := context.WithTimeout(request.Context, w.service.config.RiskCalculationTimeout)
	defer cancel()

	order := request.Order

	// Calculate order value
	orderValue := order.Price.Mul(order.Quantity)

	// Check position limits
	approved, err := w.service.baseService.CheckPositionLimit(ctx, order.UserID, order.Symbol, order.Quantity, order.Price)
	if err != nil {
		result.Success = false
		result.Error = fmt.Errorf("position limit check failed: %w", err)
		return result
	}

	// Get current risk metrics
	riskMetrics, err := w.service.getCachedRiskMetrics(ctx, order.UserID)
	if err != nil || riskMetrics == nil {
		// Calculate fresh if not cached
		riskProfile, calcErr := w.service.baseService.CalculateRisk(ctx, order.UserID)
		if calcErr != nil {
			result.Success = false
			result.Error = fmt.Errorf("risk calculation for validation failed: %w", calcErr)
			return result
		}

		riskMetrics = &RiskMetrics{
			UserID:          riskProfile.UserID,
			CurrentExposure: riskProfile.CurrentExposure,
			ValueAtRisk:     riskProfile.ValueAtRisk,
			MarginRequired:  riskProfile.MarginRequired,
			RiskScore:       w.calculateRiskScore(riskProfile),
			LastUpdated:     time.Now(),
		}
	}

	result.Success = true
	result.RiskMetrics = riskMetrics
	result.Parameters = map[string]interface{}{
		"approved":    approved,
		"order_value": orderValue,
	}

	return result
}

// processTradeProcessing processes completed trades
func (w *RiskWorker) processTradeProcessing(request *AsyncRiskRequest, result *AsyncRiskResult) *AsyncRiskResult {
	if len(request.Trades) == 0 {
		result.Success = true // No trades to process
		return result
	}

	ctx, cancel := context.WithTimeout(request.Context, w.service.config.RiskCalculationTimeout)
	defer cancel()

	// Process each trade
	for _, trade := range request.Trades {
		err := w.service.baseService.ProcessTrade(ctx, trade.ID, trade.UserID, trade.Symbol, trade.Quantity, trade.Price)
		if err != nil {
			result.Success = false
			result.Error = fmt.Errorf("trade processing failed for trade %s: %w", trade.ID, err)
			return result
		}

		// Invalidate cache for the user
		if w.service.config.EnableCaching {
			go w.service.invalidateUserCache(ctx, trade.UserID)
		}

		// Publish real-time update
		if w.service.config.EnableRealtimeUpdates {
			go w.service.publishTradeUpdate(trade)
		}
	}

	result.Success = true
	return result
}

// processBatchCalculation calculates risk for multiple users
func (w *RiskWorker) processBatchCalculation(request *AsyncRiskRequest, result *AsyncRiskResult) *AsyncRiskResult {
	userIDs, ok := request.Parameters["user_ids"].([]string)
	if !ok {
		result.Success = false
		result.Error = fmt.Errorf("invalid user_ids parameter for batch calculation")
		return result
	}

	if len(userIDs) == 0 {
		result.Success = true
		result.Parameters = map[string]interface{}{"batch_results": make(map[string]*RiskMetrics)}
		return result
	}

	ctx, cancel := context.WithTimeout(request.Context, w.service.config.RiskCalculationTimeout)
	defer cancel()

	// Use base service batch calculation
	batchResults, err := w.service.baseService.BatchCalculateRisk(ctx, userIDs)
	if err != nil {
		result.Success = false
		result.Error = fmt.Errorf("batch risk calculation failed: %w", err)
		return result
	}

	// Convert to RiskMetrics format
	riskMetricsResults := make(map[string]*RiskMetrics)
	for userID, riskProfile := range batchResults {
		riskMetricsResults[userID] = &RiskMetrics{
			UserID:          riskProfile.UserID,
			CurrentExposure: riskProfile.CurrentExposure,
			ValueAtRisk:     riskProfile.ValueAtRisk,
			MarginRequired:  riskProfile.MarginRequired,
			RiskScore:       w.calculateRiskScore(riskProfile),
			LastUpdated:     time.Now(),
		}
	}

	result.Success = true
	result.Parameters = map[string]interface{}{"batch_results": riskMetricsResults}

	// Cache results asynchronously
	if w.service.config.EnableCaching {
		go w.service.cacheBatchRiskMetrics(ctx, riskMetricsResults)
	}

	return result
}

// processComplianceCheck performs compliance checks
func (w *RiskWorker) processComplianceCheck(request *AsyncRiskRequest, result *AsyncRiskResult) *AsyncRiskResult {
	// Extract parameters
	transactionID, _ := request.Parameters["transaction_id"].(string)
	amount, _ := request.Parameters["amount"].(decimal.Decimal)
	attrs, _ := request.Parameters["attributes"].(map[string]interface{})

	if transactionID == "" || request.UserID == "" {
		result.Success = false
		result.Error = fmt.Errorf("transaction_id and user_id are required for compliance check")
		return result
	}

	ctx, cancel := context.WithTimeout(request.Context, w.service.config.RiskCalculationTimeout)
	defer cancel()

	// Perform compliance check
	complianceResult, err := w.service.baseService.ComplianceCheck(ctx, transactionID, request.UserID, amount, attrs)
	if err != nil {
		result.Success = false
		result.Error = fmt.Errorf("compliance check failed: %w", err)
		return result
	}

	result.Success = true
	result.ComplianceResult = complianceResult

	// Cache compliance result
	if w.service.config.EnableCaching {
		go w.service.cacheComplianceResult(ctx, transactionID, complianceResult)
	}

	return result
}

// calculateRiskScore calculates a risk score from risk profile
func (w *RiskWorker) calculateRiskScore(profile *UserRiskProfile) float64 {
	// Simple risk score calculation - can be enhanced
	if profile.CurrentExposure.IsZero() {
		return 0.0
	}

	// Risk score based on VaR percentage of exposure
	if profile.ValueAtRisk.IsZero() {
		return 0.1 // Low risk if no VaR
	}

	varRatio := profile.ValueAtRisk.Div(profile.CurrentExposure).InexactFloat64()

	// Normalize to 0-1 scale
	riskScore := varRatio
	if riskScore > 1.0 {
		riskScore = 1.0
	}
	if riskScore < 0.0 {
		riskScore = 0.0
	}

	return riskScore
}

// Start begins the cache worker processing loop
func (w *CacheWorker) Start() {
	w.wg.Add(1)
	go w.processLoop()
	w.logger.Info("Cache worker started")
}

// Stop gracefully stops the cache worker
func (w *CacheWorker) Stop() {
	close(w.stopChan)
	w.logger.Info("Cache worker stopping...")
}

// processLoop is the main processing loop for cache operations
func (w *CacheWorker) processLoop() {
	defer w.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			w.logger.Errorw("Cache worker panic recovered", "panic", r)
			// Restart the worker after a brief delay
			time.Sleep(time.Second)
			go w.processLoop()
		}
	}()

	ticker := time.NewTicker(time.Second * 30) // Cache maintenance every 30 seconds
	defer ticker.Stop()

	for {
		select {
		case <-w.stopChan:
			w.logger.Info("Cache worker stopped")
			return

		case <-ticker.C:
			// Perform cache maintenance
			w.performCacheMaintenance()
		}
	}
}

// performCacheMaintenance performs periodic cache maintenance
func (w *CacheWorker) performCacheMaintenance() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Clean expired entries
	w.cleanExpiredEntries(ctx)

	// Preload hot data if configured
	if w.service.config.PreloadUserProfiles {
		w.preloadHotData(ctx)
	}

	// Update cache statistics
	w.updateCacheStats(ctx)
}

// cleanExpiredEntries removes expired cache entries
func (w *CacheWorker) cleanExpiredEntries(ctx context.Context) {
	// Implementation would scan for expired keys and remove them
	// This is a simplified version
	w.logger.Debug("Performing cache cleanup...")
}

// preloadHotData preloads frequently accessed data
func (w *CacheWorker) preloadHotData(ctx context.Context) {
	// Implementation would identify and preload hot user profiles
	w.logger.Debug("Preloading hot user data...")
}

// updateCacheStats updates cache performance statistics
func (w *CacheWorker) updateCacheStats(ctx context.Context) {
	// Update Redis stats
	stats := w.service.redisClient.GetStats()
	w.logger.Debugw("Cache statistics updated",
		"hits", stats.Hits,
		"misses", stats.Misses,
		"total_conns", stats.TotalConns,
		"idle_conns", stats.IdleConns)
}

// Start begins the pub/sub worker
func (w *PubSubWorker) Start(channels []string) {
	w.wg.Add(1)
	go w.processLoop(channels)
	w.logger.Infow("PubSub worker started", "channels", channels)
}

// Stop gracefully stops the pub/sub worker
func (w *PubSubWorker) Stop() {
	close(w.stopChan)
	if w.pubsub != nil {
		w.pubsub.Close()
	}
	w.wg.Wait()
	w.logger.Info("PubSub worker stopped")
}

// processLoop is the main processing loop for pub/sub messages
func (w *PubSubWorker) processLoop(channels []string) {
	defer w.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			w.logger.Errorw("PubSub worker panic recovered", "panic", r)
		}
	}()

	// Subscribe to channels
	w.pubsub = w.redisClient.GetClient().Subscribe(context.Background(), channels...)
	defer w.pubsub.Close()

	// Process messages
	for {
		select {
		case <-w.stopChan:
			w.logger.Info("PubSub worker stopped")
			return

		default:
			msg, err := w.pubsub.ReceiveMessage(context.Background())
			if err != nil {
				w.logger.Errorw("Error receiving pub/sub message", "error", err)
				time.Sleep(time.Second) // Brief delay before retry
				continue
			}

			// Handle the message
			if err := w.handler(msg.Channel, msg.Payload); err != nil {
				w.logger.Errorw("Error handling pub/sub message",
					"channel", msg.Channel,
					"error", err)
			}
		}
	}
}

// startWorkers initializes and starts all worker pools
func (s *AsyncRiskService) startWorkers() error {
	// Start risk calculation workers
	s.riskWorkers = make([]*RiskWorker, s.config.RiskWorkerCount)
	for i := 0; i < s.config.RiskWorkerCount; i++ {
		workerID := fmt.Sprintf("risk-worker-%d", i)
		worker := NewRiskWorker(workerID, s, s.logger)
		s.riskWorkers[i] = worker
		worker.Start()
	}

	// Start cache workers
	s.cacheWorkers = make([]*CacheWorker, s.config.CacheWorkerCount)
	for i := 0; i < s.config.CacheWorkerCount; i++ {
		workerID := fmt.Sprintf("cache-worker-%d", i)
		worker := NewCacheWorker(workerID, s, s.logger)
		s.cacheWorkers[i] = worker
		worker.Start()
	}

	s.logger.Infow("Workers started",
		"risk_workers", len(s.riskWorkers),
		"cache_workers", len(s.cacheWorkers))

	return nil
}

// Add missing import for debug
