package crosspair

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	// Note: These packages don't exist yet, commenting out for now
	// "github.com/orbit-cex/finalex/internal/trading/balance"
	// "github.com/orbit-cex/finalex/internal/trading/coordination"
	// "github.com/orbit-cex/finalex/internal/trading/engine"
	// "github.com/orbit-cex/finalex/internal/trading/registry"
)

// Placeholder types for missing dependencies
type BalanceService struct {
	// Placeholder for balance service
}

func (bs *BalanceService) GetBalance(ctx context.Context, userID uuid.UUID, asset string) (decimal.Decimal, error) {
	// Placeholder implementation
	return decimal.NewFromFloat(1000), nil
}

func (bs *BalanceService) Reserve(ctx context.Context, userID uuid.UUID, asset string, amount decimal.Decimal, referenceID string) error {
	// Placeholder implementation
	return nil
}

func (bs *BalanceService) Release(ctx context.Context, userID uuid.UUID, asset string, amount decimal.Decimal, referenceID string) error {
	// Placeholder implementation
	return nil
}

type TradeCoordinator struct {
	// Placeholder for trade coordinator
}

type HighPerformancePairRegistry struct {
	// Placeholder for pair registry
}

func (hpr *HighPerformancePairRegistry) FindCommonBase(asset1, asset2 string) (string, bool) {
	// Placeholder implementation
	return "USDT", true
}

func (hpr *HighPerformancePairRegistry) GetAvailableRoutes(fromAsset, toAsset string) ([]*CrossPairRoute, error) {
	// Placeholder implementation
	return []*CrossPairRoute{}, nil
}

type FeeEngine struct {
	// Placeholder for fee engine
}

func (fe *FeeEngine) CalculateFee(asset, pair string, quantity decimal.Decimal, feeType string) (decimal.Decimal, error) {
	// Placeholder implementation - 0.1% fee
	return quantity.Mul(decimal.NewFromFloat(0.001)), nil
}

// CrossPairEngine is the main orchestrator for cross-pair trading operations
type CrossPairEngine struct {
	mu               sync.RWMutex
	logger           *zap.Logger
	balanceService   *BalanceService
	coordinator      *TradeCoordinator
	pairRegistry     *HighPerformancePairRegistry
	feeEngine        *FeeEngine
	rateCalculator   *SyntheticRateCalculator
	matchingEngines  map[string]MatchingEngine
	orderStore       CrossPairOrderStore
	tradeStore       CrossPairTradeStore
	eventPublisher   EventPublisher
	metricsCollector MetricsCollector
	config           *CrossPairEngineConfig
	activeOrders     map[uuid.UUID]*CrossPairOrder
	executionQueue   chan *CrossPairOrder
	ctx              context.Context
	cancel           context.CancelFunc
	wg               sync.WaitGroup
}

// Dependencies interfaces

// CrossPairOrderStore interface for persisting cross-pair orders
type CrossPairOrderStore interface {
	Create(ctx context.Context, order *CrossPairOrder) error
	Update(ctx context.Context, order *CrossPairOrder) error
	GetByID(ctx context.Context, id uuid.UUID) (*CrossPairOrder, error)
	GetByUserID(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*CrossPairOrder, error)
	GetActiveOrders(ctx context.Context) ([]*CrossPairOrder, error)
}

// CrossPairTradeStore interface for persisting cross-pair trades
type CrossPairTradeStore interface {
	Create(ctx context.Context, trade *CrossPairTrade) error
	GetByOrderID(ctx context.Context, orderID uuid.UUID) ([]*CrossPairTrade, error)
	GetByUserID(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*CrossPairTrade, error)
}

// EventPublisher interface for publishing events
type EventPublisher interface {
	PublishOrderCreated(order *CrossPairOrder) error
	PublishOrderUpdated(order *CrossPairOrder) error
	PublishTradeExecuted(trade *CrossPairTrade) error
	PublishExecutionFailed(orderID uuid.UUID, reason string) error
	// Publish partial fill event for either leg
	PublishPartialFill(order *CrossPairOrder, leg int, fillAmount decimal.Decimal, tradeID uuid.UUID, fees []CrossPairFee) error
}

// MetricsCollector interface for collecting metrics
type MetricsCollector interface {
	RecordOrderCreated(fromAsset, toAsset string)
	RecordOrderExecuted(fromAsset, toAsset string, executionTime time.Duration)
	RecordOrderFailed(fromAsset, toAsset string, reason string)
	RecordSlippage(fromAsset, toAsset string, slippage decimal.Decimal)
	RecordVolume(fromAsset, toAsset string, volume decimal.Decimal)
}

// NewCrossPairEngine creates a new cross-pair trading engine
func NewCrossPairEngine(
	logger *zap.Logger,
	balanceService *BalanceService,
	coordinator *TradeCoordinator,
	pairRegistry *HighPerformancePairRegistry,
	feeEngine *FeeEngine,
	rateCalculator *SyntheticRateCalculator,
	matchingEngines map[string]MatchingEngine,
	orderStore CrossPairOrderStore,
	tradeStore CrossPairTradeStore,
	eventPublisher EventPublisher,
	metricsCollector MetricsCollector,
	config *CrossPairEngineConfig,
) *CrossPairEngine {
	ctx, cancel := context.WithCancel(context.Background())

	return &CrossPairEngine{
		logger:           logger.Named("cross-pair-engine"),
		balanceService:   balanceService,
		coordinator:      coordinator,
		pairRegistry:     pairRegistry,
		feeEngine:        feeEngine,
		rateCalculator:   rateCalculator,
		matchingEngines:  matchingEngines,
		orderStore:       orderStore,
		tradeStore:       tradeStore,
		eventPublisher:   eventPublisher,
		metricsCollector: metricsCollector,
		config:           config,
		activeOrders:     make(map[uuid.UUID]*CrossPairOrder),
		executionQueue:   make(chan *CrossPairOrder, config.QueueSize),
		ctx:              ctx,
		cancel:           cancel,
	}
}

// DefaultCrossPairEngineConfig returns default configuration
func DefaultCrossPairEngineConfig() *CrossPairEngineConfig {
	return &CrossPairEngineConfig{
		MaxConcurrentExecutions: 100,
		ExecutionTimeout:        30 * time.Second,
		MaxSlippageTolerance:    decimal.NewFromFloat(0.05),  // 5%
		MinOrderValue:           decimal.NewFromInt(10),      // $10 minimum
		MaxOrderValue:           decimal.NewFromInt(1000000), // $1M maximum
		DefaultExpiration:       5 * time.Minute,
		RetryAttempts:           3,
		RetryDelay:              1 * time.Second,
	}
}

// Start starts the cross-pair engine
func (e *CrossPairEngine) Start() error {
	e.logger.Info("starting cross-pair engine",
		zap.Int("max_concurrent_executions", e.config.MaxConcurrentExecutions))

	// Start worker goroutines for order execution
	for i := 0; i < e.config.MaxConcurrentExecutions; i++ {
		e.wg.Add(1)
		go e.executionWorker(i)
	}

	// Start cleanup goroutine for expired orders
	e.wg.Add(1)
	go e.cleanupWorker()

	// Load active orders from storage
	if err := e.loadActiveOrders(); err != nil {
		e.logger.Error("failed to load active orders", zap.Error(err))
		return fmt.Errorf("failed to load active orders: %w", err)
	}

	return nil
}

// Stop stops the cross-pair engine
func (e *CrossPairEngine) Stop() error {
	e.logger.Info("stopping cross-pair engine")

	e.cancel()
	close(e.executionQueue)
	e.wg.Wait()

	return nil
}

// CreateOrder creates a new cross-pair order
func (e *CrossPairEngine) CreateOrder(ctx context.Context, request *CreateOrderRequest) (*CrossPairOrder, error) {
	// Validate request
	if err := request.Validate(); err != nil {
		return nil, fmt.Errorf("invalid order request: %w", err)
	}

	// Check if route exists
	baseAsset, routeExists := e.pairRegistry.FindCommonBase(request.FromAsset, request.ToAsset)
	if !routeExists {
		return nil, NewCrossPairError("NO_ROUTE_AVAILABLE",
			fmt.Sprintf("no trading route available for %s to %s", request.FromAsset, request.ToAsset))
	}

	// Calculate current rate and validate order
	rateResult, err := e.rateCalculator.CalculateRateWithDetails(request.FromAsset, request.ToAsset, request.Quantity)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate rate: %w", err)
	}

	// Check minimum liquidity and confidence
	if rateResult.Confidence.LessThan(decimal.NewFromFloat(0.5)) {
		return nil, NewCrossPairError("INSUFFICIENT_CONFIDENCE", "insufficient confidence in rate calculation")
	}

	// Create order
	order := &CrossPairOrder{
		ID:               uuid.New(),
		UserID:           request.UserID,
		FromAsset:        request.FromAsset,
		ToAsset:          request.ToAsset,
		Type:             request.Type,
		Side:             request.Side,
		Status:           CrossPairOrderPending,
		Quantity:         request.Quantity,
		Price:            request.Price,
		EstimatedRate:    rateResult.Route.SyntheticRate,
		MaxSlippage:      request.MaxSlippage,
		Route:            rateResult.Route,
		ExecutedQuantity: decimal.Zero,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	// Set expiration time
	if request.TimeInForce == "IOC" {
		// Immediate or Cancel
		expiresAt := time.Now().Add(1 * time.Second)
		order.ExpiresAt = &expiresAt
	} else if request.TimeInForce == "GTT" && request.ExpiresAt != nil {
		// Good Till Time
		order.ExpiresAt = request.ExpiresAt
	} else {
		// Default expiration
		expiresAt := time.Now().Add(e.config.DefaultExpiration)
		order.ExpiresAt = &expiresAt
	}

	// Validate order
	if err := order.Validate(); err != nil {
		return nil, fmt.Errorf("order validation failed: %w", err)
	}

	// Check user balance
	if err := e.validateUserBalance(ctx, order); err != nil {
		return nil, fmt.Errorf("insufficient balance: %w", err)
	}

	// Reserve balance for the order
	if err := e.reserveBalance(ctx, order); err != nil {
		return nil, fmt.Errorf("failed to reserve balance: %w", err)
	}

	// Store order
	if err := e.orderStore.Create(ctx, order); err != nil {
		// Release reserved balance on storage failure
		if releaseErr := e.releaseBalance(ctx, order); releaseErr != nil {
			e.logger.Error("failed to release balance after storage failure",
				zap.String("order_id", order.ID.String()),
				zap.Error(releaseErr))
		}
		return nil, fmt.Errorf("failed to store order: %w", err)
	}

	// Add to active orders
	e.mu.Lock()
	e.activeOrders[order.ID] = order
	e.mu.Unlock()

	// Queue for execution
	select {
	case e.executionQueue <- order:
		// Order queued successfully
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		// Queue is full, reject order
		if err := e.cancelOrder(ctx, order.ID, "execution queue full"); err != nil {
			e.logger.Error("failed to cancel order after queue rejection",
				zap.String("order_id", order.ID.String()),
				zap.Error(err))
		}
		return nil, NewCrossPairError("QUEUE_FULL", "execution queue is full, please try again later")
	}

	// Publish event
	if err := e.eventPublisher.PublishOrderCreated(order); err != nil {
		e.logger.Error("failed to publish order created event",
			zap.String("order_id", order.ID.String()),
			zap.Error(err))
	}

	// Record metrics
	e.metricsCollector.RecordOrderCreated(order.FromAsset, order.ToAsset)

	e.logger.Info("cross-pair order created",
		zap.String("order_id", order.ID.String()),
		zap.String("user_id", order.UserID.String()),
		zap.String("from_asset", order.FromAsset),
		zap.String("to_asset", order.ToAsset),
		zap.String("quantity", order.Quantity.String()),
		zap.String("base_asset", baseAsset))

	return order, nil
}

// CreateOrderRequest represents a request to create a cross-pair order
type CreateOrderRequest struct {
	UserID      uuid.UUID          `json:"user_id"`
	FromAsset   string             `json:"from_asset"`
	ToAsset     string             `json:"to_asset"`
	Type        CrossPairOrderType `json:"type"`
	Side        CrossPairOrderSide `json:"side"`
	Quantity    decimal.Decimal    `json:"quantity"`
	Price       *decimal.Decimal   `json:"price,omitempty"`
	MaxSlippage decimal.Decimal    `json:"max_slippage"`
	TimeInForce string             `json:"time_in_force"` // "GTC", "IOC", "GTT"
	ExpiresAt   *time.Time         `json:"expires_at,omitempty"`
}

// Validate validates a create order request
func (r *CreateOrderRequest) Validate() error {
	if r.UserID == uuid.Nil {
		return NewCrossPairError("INVALID_USER_ID", "user ID is required")
	}
	if r.FromAsset == "" {
		return NewCrossPairError("INVALID_FROM_ASSET", "from asset is required")
	}
	if r.ToAsset == "" {
		return NewCrossPairError("INVALID_TO_ASSET", "to asset is required")
	}
	if r.FromAsset == r.ToAsset {
		return NewCrossPairError("SAME_ASSETS", "from and to assets cannot be the same")
	}
	if r.Quantity.LessThanOrEqual(decimal.Zero) {
		return NewCrossPairError("INVALID_QUANTITY", "quantity must be greater than zero")
	}
	if r.Type == CrossPairLimitOrder && (r.Price == nil || r.Price.LessThanOrEqual(decimal.Zero)) {
		return NewCrossPairError("INVALID_PRICE", "price is required for limit orders")
	}
	if r.MaxSlippage.LessThan(decimal.Zero) || r.MaxSlippage.GreaterThan(decimal.NewFromFloat(1.0)) {
		return NewCrossPairError("INVALID_SLIPPAGE", "max slippage must be between 0 and 1")
	}
	if r.TimeInForce != "" && r.TimeInForce != "GTC" && r.TimeInForce != "IOC" && r.TimeInForce != "GTT" {
		return NewCrossPairError("INVALID_TIME_IN_FORCE", "time in force must be GTC, IOC, or GTT")
	}
	return nil
}

// CancelOrder cancels a cross-pair order
func (e *CrossPairEngine) CancelOrder(ctx context.Context, orderID uuid.UUID, userID uuid.UUID) error {
	e.mu.RLock()
	order, exists := e.activeOrders[orderID]
	e.mu.RUnlock()

	if !exists {
		return NewCrossPairError("ORDER_NOT_FOUND", "order not found")
	}

	if order.UserID != userID {
		return NewCrossPairError("UNAUTHORIZED", "unauthorized to cancel this order")
	}

	if order.Status != CrossPairOrderPending {
		return NewCrossPairError("ORDER_NOT_CANCELABLE", "order cannot be canceled in current status")
	}

	return e.cancelOrder(ctx, orderID, "user requested")
}

// cancelOrder cancels an order (internal method)
func (e *CrossPairEngine) cancelOrder(ctx context.Context, orderID uuid.UUID, reason string) error {
	e.mu.Lock()
	order, exists := e.activeOrders[orderID]
	if !exists {
		e.mu.Unlock()
		return NewCrossPairError("ORDER_NOT_FOUND", "order not found")
	}

	order.Status = CrossPairOrderCanceled
	order.UpdatedAt = time.Now()
	order.ErrorMessage = &reason

	delete(e.activeOrders, orderID)
	e.mu.Unlock()

	// Release reserved balance
	if err := e.releaseBalance(ctx, order); err != nil {
		e.logger.Error("failed to release balance for canceled order",
			zap.String("order_id", orderID.String()),
			zap.Error(err))
	}

	// Update order in storage
	if err := e.orderStore.Update(ctx, order); err != nil {
		e.logger.Error("failed to update canceled order in storage",
			zap.String("order_id", orderID.String()),
			zap.Error(err))
	}

	// Publish event
	if err := e.eventPublisher.PublishOrderUpdated(order); err != nil {
		e.logger.Error("failed to publish order updated event",
			zap.String("order_id", orderID.String()),
			zap.Error(err))
	}

	e.logger.Info("cross-pair order canceled",
		zap.String("order_id", orderID.String()),
		zap.String("reason", reason))

	return nil
}

// GetOrder retrieves a cross-pair order by ID
func (e *CrossPairEngine) GetOrder(ctx context.Context, orderID uuid.UUID, userID uuid.UUID) (*CrossPairOrder, error) {
	order, err := e.orderStore.GetByID(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("failed to get order: %w", err)
	}

	if order.UserID != userID {
		return nil, NewCrossPairError("UNAUTHORIZED", "unauthorized to view this order")
	}

	return order, nil
}

// GetUserOrders retrieves orders for a specific user
func (e *CrossPairEngine) GetUserOrders(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*CrossPairOrder, error) {
	return e.orderStore.GetByUserID(ctx, userID, limit, offset)
}

// GetUserTrades retrieves trades for a specific user
func (e *CrossPairEngine) GetUserTrades(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*CrossPairTrade, error) {
	return e.tradeStore.GetByUserID(ctx, userID, limit, offset)
}

// GetAvailableRoutes returns available trading routes
func (e *CrossPairEngine) GetAvailableRoutes(ctx context.Context) (map[string][]string, error) {
	// This would return all possible cross-pair routes
	// For now, return a simplified structure
	routes := make(map[string][]string)
	// This is a placeholder implementation
	return routes, nil
}

// GetRateQuote returns a rate quote for a cross-pair trade
func (e *CrossPairEngine) GetRateQuote(ctx context.Context, fromAsset, toAsset string, quantity decimal.Decimal) (*RateQuote, error) {
	rateResult, err := e.rateCalculator.CalculateRateWithDetails(fromAsset, toAsset, quantity)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate rate: %w", err)
	}

	// Calculate estimated fees
	estimatedFees, err := e.calculateEstimatedFees(fromAsset, toAsset, quantity, rateResult.Route)
	if err != nil {
		e.logger.Warn("failed to calculate estimated fees", zap.Error(err))
		estimatedFees = []CrossPairFee{} // Return empty fees if calculation fails
	}

	return &RateQuote{
		FromAsset:         fromAsset,
		ToAsset:           toAsset,
		Quantity:          quantity,
		EstimatedRate:     rateResult.Route.SyntheticRate,
		EstimatedOutput:   quantity.Mul(rateResult.Route.SyntheticRate),
		EstimatedSlippage: rateResult.EstimatedSlippage,
		Confidence:        rateResult.Confidence,
		LiquidityUSD:      rateResult.LiquidityUSD,
		Route:             rateResult.Route,
		EstimatedFees:     estimatedFees,
		ValidUntil:        time.Now().Add(30 * time.Second), // Quote valid for 30 seconds
	}, nil
}

// RateQuote represents a rate quote for cross-pair trading
type RateQuote struct {
	FromAsset         string          `json:"from_asset"`
	ToAsset           string          `json:"to_asset"`
	Quantity          decimal.Decimal `json:"quantity"`
	EstimatedRate     decimal.Decimal `json:"estimated_rate"`
	EstimatedOutput   decimal.Decimal `json:"estimated_output"`
	EstimatedSlippage decimal.Decimal `json:"estimated_slippage"`
	Confidence        decimal.Decimal `json:"confidence"`
	LiquidityUSD      decimal.Decimal `json:"liquidity_usd"`
	Route             *CrossPairRoute `json:"route"`
	EstimatedFees     []CrossPairFee  `json:"estimated_fees"`
	ValidUntil        time.Time       `json:"valid_until"`
}

// validateUserBalance validates that the user has sufficient balance
func (e *CrossPairEngine) validateUserBalance(ctx context.Context, order *CrossPairOrder) error {
	balance, err := e.balanceService.GetBalance(ctx, order.UserID, order.FromAsset)
	if err != nil {
		return fmt.Errorf("failed to get user balance: %w", err)
	}
	if balance.LessThan(order.Quantity) {
		return NewCrossPairError("INSUFFICIENT_BALANCE",
			fmt.Sprintf("insufficient %s balance: required %s, available %s",
				order.FromAsset, order.Quantity.String(), balance.String()))
	}

	return nil
}

// reserveBalance reserves balance for an order
func (e *CrossPairEngine) reserveBalance(ctx context.Context, order *CrossPairOrder) error {
	return e.balanceService.Reserve(ctx, order.UserID, order.FromAsset, order.Quantity, order.ID.String())
}

// releaseBalance releases reserved balance
func (e *CrossPairEngine) releaseBalance(ctx context.Context, order *CrossPairOrder) error {
	return e.balanceService.Release(ctx, order.UserID, order.FromAsset, order.Quantity, order.ID.String())
}

// calculateEstimatedFees calculates estimated fees for a cross-pair trade
func (e *CrossPairEngine) calculateEstimatedFees(fromAsset, toAsset string, quantity decimal.Decimal, route *CrossPairRoute) ([]CrossPairFee, error) {
	// This would integrate with the existing FeeEngine to calculate fees
	// Implementation depends on the FeeEngine interface
	fees := []CrossPairFee{}
	// Calculate trading fees for each leg
	firstPairFee, err := e.feeEngine.CalculateFee(fromAsset, route.FirstPair, quantity, "TAKER")
	if err != nil {
		return nil, fmt.Errorf("failed to calculate first pair fee: %w", err)
	}

	secondPairFee, err := e.feeEngine.CalculateFee(toAsset, route.SecondPair, quantity, "TAKER")
	if err != nil {
		return nil, fmt.Errorf("failed to calculate second pair fee: %w", err)
	}

	fees = append(fees, CrossPairFee{
		Asset:   fromAsset,
		Amount:  firstPairFee,
		FeeType: "TRADING",
		Pair:    route.FirstPair,
	})

	fees = append(fees, CrossPairFee{
		Asset:   toAsset,
		Amount:  secondPairFee,
		FeeType: "TRADING",
		Pair:    route.SecondPair,
	})

	return fees, nil
}

// loadActiveOrders loads active orders from storage on startup
func (e *CrossPairEngine) loadActiveOrders() error {
	orders, err := e.orderStore.GetActiveOrders(e.ctx)
	if err != nil {
		return fmt.Errorf("failed to load active orders: %w", err)
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	for _, order := range orders {
		if order.CanExecute() {
			e.activeOrders[order.ID] = order
			// Re-queue for execution
			select {
			case e.executionQueue <- order:
				// Successfully queued
			default:
				// Queue is full, will be picked up by cleanup worker
				e.logger.Warn("failed to queue active order on startup",
					zap.String("order_id", order.ID.String()))
			}
		}
	}

	e.logger.Info("loaded active orders", zap.Int("count", len(e.activeOrders)))
	return nil
}

// executionWorker processes orders from the execution queue
func (e *CrossPairEngine) executionWorker(workerID int) {
	defer e.wg.Done()

	e.logger.Info("starting execution worker", zap.Int("worker_id", workerID))

	for {
		select {
		case order := <-e.executionQueue:
			if order == nil {
				return // Channel closed
			}
			e.executeOrder(order)

		case <-e.ctx.Done():
			return
		}
	}
}

// cleanupWorker periodically cleans up expired orders
func (e *CrossPairEngine) cleanupWorker() {
	defer e.wg.Done()

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			e.cleanupExpiredOrders()

		case <-e.ctx.Done():
			return
		}
	}
}

// cleanupExpiredOrders cancels expired orders
func (e *CrossPairEngine) cleanupExpiredOrders() {
	e.mu.RLock()
	expiredOrders := make([]*CrossPairOrder, 0)
	for _, order := range e.activeOrders {
		if order.IsExpired() {
			expiredOrders = append(expiredOrders, order)
		}
	}
	e.mu.RUnlock()

	for _, order := range expiredOrders {
		if err := e.cancelOrder(e.ctx, order.ID, "expired"); err != nil {
			e.logger.Error("failed to cancel expired order",
				zap.String("order_id", order.ID.String()),
				zap.Error(err))
		}
	}

	if len(expiredOrders) > 0 {
		e.logger.Info("cleaned up expired orders", zap.Int("count", len(expiredOrders)))
	}
}

// executeOrder executes a cross-pair order
func (e *CrossPairEngine) executeOrder(order *CrossPairOrder) {
	startTime := time.Now()
	orderLogger := e.logger.With(zap.String("order_id", order.ID.String()))

	orderLogger.Info("executing cross-pair order")

	// Check if order is still valid
	if !order.CanExecute() {
		orderLogger.Warn("order cannot be executed", zap.String("status", string(order.Status)))
		return
	}

	// Update order status to executing
	e.mu.Lock()
	order.Status = CrossPairOrderExecuting
	order.UpdatedAt = time.Now()
	e.mu.Unlock()

	// Execute the order with timeout
	ctx, cancel := context.WithTimeout(e.ctx, e.config.ExecutionTimeout)
	defer cancel()

	err := e.executeOrderWithRetry(ctx, order)
	executionTime := time.Since(startTime)

	// Update final status
	e.mu.Lock()
	if err != nil {
		order.Status = CrossPairOrderFailed
		errorMsg := err.Error()
		order.ErrorMessage = &errorMsg
		e.metricsCollector.RecordOrderFailed(order.FromAsset, order.ToAsset, errorMsg)
	} else {
		order.Status = CrossPairOrderCompleted
		e.metricsCollector.RecordOrderExecuted(order.FromAsset, order.ToAsset, executionTime)
	}
	order.UpdatedAt = time.Now()

	// Remove from active orders
	delete(e.activeOrders, order.ID)
	e.mu.Unlock()

	// Update order in storage
	if updateErr := e.orderStore.Update(ctx, order); updateErr != nil {
		orderLogger.Error("failed to update order in storage", zap.Error(updateErr))
	}

	// Publish event
	if publishErr := e.eventPublisher.PublishOrderUpdated(order); publishErr != nil {
		orderLogger.Error("failed to publish order updated event", zap.Error(publishErr))
	}

	if err != nil {
		orderLogger.Error("order execution failed", zap.Error(err), zap.Duration("execution_time", executionTime))

		// Publish execution failed event
		if publishErr := e.eventPublisher.PublishExecutionFailed(order.ID, err.Error()); publishErr != nil {
			orderLogger.Error("failed to publish execution failed event", zap.Error(publishErr))
		}
	} else {
		orderLogger.Info("order execution completed", zap.Duration("execution_time", executionTime))
	}
}

// executeOrderWithRetry executes an order with retry logic
func (e *CrossPairEngine) executeOrderWithRetry(ctx context.Context, order *CrossPairOrder) error {
	var lastErr error

	for attempt := 0; attempt < e.config.RetryAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(e.config.RetryDelay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		if err := e.executeOrderAttempt(ctx, order); err != nil {
			lastErr = err
			e.logger.Warn("order execution attempt failed",
				zap.String("order_id", order.ID.String()),
				zap.Int("attempt", attempt+1),
				zap.Error(err))
			continue
		}

		return nil // Success
	}

	return fmt.Errorf("order execution failed after %d attempts: %w", e.config.RetryAttempts, lastErr)
}

// executeOrderAttempt performs a single execution attempt
func (e *CrossPairEngine) executeOrderAttempt(ctx context.Context, order *CrossPairOrder) error {
	// Create atomic executor for this execution
	executor := NewAtomicExecutor(
		e.logger,
		e.balanceService,
		e.coordinator,
		e.rateCalculator,
		e.matchingEngines,
		e.tradeStore,
		e.eventPublisher, // Pass eventPublisher for partial fill event publishing
	)

	// Validate execution conditions
	if err := executor.ValidateExecutionConditions(ctx, order); err != nil {
		return fmt.Errorf("execution validation failed: %w", err)
	}

	// Execute the order atomically
	result, err := executor.ExecuteOrder(ctx, order)
	if err != nil {
		return fmt.Errorf("atomic execution failed: %w", err)
	}

	// Update order with execution results
	order.ExecutedQuantity = order.Quantity
	order.ExecutedRate = &result.ActualRate

	// Record metrics
	e.metricsCollector.RecordSlippage(order.FromAsset, order.ToAsset, result.Slippage)
	e.metricsCollector.RecordVolume(order.FromAsset, order.ToAsset, order.Quantity)

	// Publish trade executed event
	if err := e.eventPublisher.PublishTradeExecuted(result.Trade); err != nil {
		e.logger.Error("failed to publish trade executed event",
			zap.String("trade_id", result.Trade.ID.String()),
			zap.Error(err))
	}

	e.logger.Info("cross-pair order executed successfully",
		zap.String("order_id", order.ID.String()),
		zap.String("trade_id", result.Trade.ID.String()),
		zap.String("actual_rate", result.ActualRate.String()),
		zap.String("slippage", result.Slippage.String()),
		zap.Duration("execution_time", result.ExecutionTime))

	return nil
}

// GetEngineStatus returns the current status of the engine
func (e *CrossPairEngine) GetEngineStatus() map[string]interface{} {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return map[string]interface{}{
		"active_orders":     len(e.activeOrders),
		"queue_length":      len(e.executionQueue),
		"queue_capacity":    cap(e.executionQueue),
		"max_concurrent":    e.config.MaxConcurrentExecutions,
		"execution_timeout": e.config.ExecutionTimeout.String(),
	}
}
