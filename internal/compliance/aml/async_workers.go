package aml

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
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
	redisClient redis.UniversalClient
	pubsub      *redis.PubSub
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

// NewPubSubWorker creates a new pub/sub worker for real-time updates
func NewPubSubWorker(client redis.UniversalClient, handler func(channel, message string) error, logger *zap.SugaredLogger) *PubSubWorker {
	return &PubSubWorker{
		redisClient: client,
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

	result := &AsyncRiskResult{
		RequestID: request.RequestID,
		Timestamp: time.Now(),
		WorkerID:  w.id,
	}

	// Process based on request type
	switch request.Type {
	case RiskRequestCalculateUser:
		result = w.processUserRiskCalculation(request, result)
	default:
		result.Success = false
		result.Error = fmt.Errorf("unsupported request type: %s", request.Type)
	}

	result.ProcessingTime = time.Since(start)
	return result
}

// processUserRiskCalculation calculates risk metrics for a user
func (w *RiskWorker) processUserRiskCalculation(request *AsyncRiskRequest, result *AsyncRiskResult) *AsyncRiskResult {
	ctx, cancel := context.WithTimeout(request.Context, w.service.config.RiskCalculationTimeout)
	defer cancel()

	// Calculate using base service
	riskProfile, err := w.service.baseService.CalculateRisk(ctx, request.UserID)
	if err != nil {
		result.Success = false
		result.Error = fmt.Errorf("risk calculation failed: %w", err)
		return result
	}

	// Convert to RiskMetrics with correct field mapping
	riskMetrics := &RiskMetrics{
		UserID:         riskProfile.UserID,
		TotalExposure:  riskProfile.CurrentExposure,
		ValueAtRisk:    riskProfile.ValueAtRisk,
		RiskScore:      decimal.NewFromFloat(w.calculateRiskScore(riskProfile)),
		LastCalculated: time.Now(),
	}

	result.Success = true
	result.RiskMetrics = riskMetrics

	return result
}

// calculateRiskScore calculates a risk score from risk profile
func (w *RiskWorker) calculateRiskScore(profile *UserRiskProfile) float64 {
	if profile.CurrentExposure.IsZero() {
		return 0.0
	}
	if profile.ValueAtRisk.IsZero() {
		return 0.1
	}
	varRatio := profile.ValueAtRisk.Div(profile.CurrentExposure).InexactFloat64()
	if varRatio > 1.0 {
		varRatio = 1.0
	}
	if varRatio < 0.0 {
		varRatio = 0.0
	}
	return varRatio
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
		}
	}()

	ticker := time.NewTicker(time.Second * 30)
	defer ticker.Stop()

	for {
		select {
		case <-w.stopChan:
			w.logger.Info("Cache worker stopped")
			return
		case <-ticker.C:
			// Perform cache maintenance
			w.logger.Debug("Cache maintenance tick")
		}
	}
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
	w.pubsub = w.redisClient.Subscribe(context.Background(), channels...)
	defer w.pubsub.Close()

	// Process messages
	ch := w.pubsub.Channel()
	for {
		select {
		case <-w.stopChan:
			w.logger.Info("PubSub worker stopped")
			return
		case msg := <-ch:
			if msg == nil {
				continue
			}
			if err := w.handler(msg.Channel, msg.Payload); err != nil {
				w.logger.Errorw("Error handling pub/sub message",
					"channel", msg.Channel,
					"error", err)
			}
		}
	}
}
