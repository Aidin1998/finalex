package trading

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/consensus"
	"github.com/Aidin1998/pincex_unified/internal/consistency"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// StrongConsistencyTransactionManager interface defines transaction operations needed by trading
type StrongConsistencyTransactionManager interface {
	ExecuteStrongConsistencyTransaction(ctx context.Context, req interface{}) (interface{}, error)
}

// StrongConsistencyOrderProcessor provides enhanced order processing with strong consistency guarantees
type StrongConsistencyOrderProcessor struct {
	// Base components
	db     *gorm.DB
	logger *zap.Logger

	// Consistency components
	raftCoordinator    *consensus.RaftCoordinator
	balanceManager     *consistency.BalanceConsistencyManager
	transactionManager StrongConsistencyTransactionManager

	// Order processing state
	orderLocks      map[string]*sync.Mutex
	orderLocksMutex sync.RWMutex

	// Configuration
	config *OrderProcessingConfig

	// Metrics
	metrics      *OrderProcessingMetrics
	metricsMutex sync.RWMutex

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// OrderProcessingConfig defines configuration for strong consistency order processing
type OrderProcessingConfig struct {
	// Consensus thresholds
	LargeOrderThreshold    float64 `json:"large_order_threshold" yaml:"large_order_threshold"`
	CriticalOrderThreshold float64 `json:"critical_order_threshold" yaml:"critical_order_threshold"`

	// Processing timeouts
	OrderProcessingTimeout time.Duration `json:"order_processing_timeout" yaml:"order_processing_timeout"`
	ConsensusTimeout       time.Duration `json:"consensus_timeout" yaml:"consensus_timeout"`
	BalanceCheckTimeout    time.Duration `json:"balance_check_timeout" yaml:"balance_check_timeout"`

	// Consistency guarantees
	RequireConsensusForLargeOrders bool `json:"require_consensus_for_large_orders" yaml:"require_consensus_for_large_orders"`
	EnableAtomicMatching           bool `json:"enable_atomic_matching" yaml:"enable_atomic_matching"`
	ValidateBalanceBeforeMatching  bool `json:"validate_balance_before_matching" yaml:"validate_balance_before_matching"`

	// Performance tuning
	MaxConcurrentOrders int           `json:"max_concurrent_orders" yaml:"max_concurrent_orders"`
	EnableOrderBatching bool          `json:"enable_order_batching" yaml:"enable_order_batching"`
	BatchSize           int           `json:"batch_size" yaml:"batch_size"`
	BatchTimeout        time.Duration `json:"batch_timeout" yaml:"batch_timeout"`
}

// OrderProcessingMetrics tracks order processing performance and consistency
type OrderProcessingMetrics struct {
	OrdersProcessed           int64 `json:"orders_processed"`
	OrdersRejected            int64 `json:"orders_rejected"`
	ConsensusOrdersProcessed  int64 `json:"consensus_orders_processed"`
	BalanceValidationFailures int64 `json:"balance_validation_failures"`

	AverageProcessingLatency time.Duration `json:"average_processing_latency"`
	ConsensusLatency         time.Duration `json:"consensus_latency"`
	BalanceValidationLatency time.Duration `json:"balance_validation_latency"`

	ActiveOrders           int64 `json:"active_orders"`
	PendingConsensusOrders int64 `json:"pending_consensus_orders"`

	LastUpdate time.Time `json:"last_update"`
}

// Order represents an enhanced order with consistency metadata
type Order struct {
	ID       string  `json:"id" db:"id"`
	UserID   string  `json:"user_id" db:"user_id"`
	Symbol   string  `json:"symbol" db:"symbol"`
	Side     string  `json:"side" db:"side"` // "buy" or "sell"
	Type     string  `json:"type" db:"type"` // "market", "limit", "stop"
	Quantity float64 `json:"quantity" db:"quantity"`
	Price    float64 `json:"price" db:"price"`

	// Consistency metadata
	RequiresConsensus bool   `json:"requires_consensus" db:"requires_consensus"`
	ConsensusStatus   string `json:"consensus_status" db:"consensus_status"`
	BalanceValidated  bool   `json:"balance_validated" db:"balance_validated"`

	// Processing state
	Status      string     `json:"status" db:"status"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
	ProcessedAt *time.Time `json:"processed_at,omitempty" db:"processed_at"`

	// Metadata
	Metadata map[string]interface{} `json:"metadata" db:"metadata"`
}

// OrderProcessingResult represents the result of order processing
type OrderProcessingResult struct {
	OrderID           string                 `json:"order_id"`
	Status            string                 `json:"status"`
	ProcessingLatency time.Duration          `json:"processing_latency"`
	ConsensusRequired bool                   `json:"consensus_required"`
	ConsensusApproved bool                   `json:"consensus_approved"`
	BalanceValidated  bool                   `json:"balance_validated"`
	MatchedTrades     []Trade                `json:"matched_trades,omitempty"`
	Error             string                 `json:"error,omitempty"`
	Metadata          map[string]interface{} `json:"metadata"`
}

// Trade represents a matched trade
type Trade struct {
	ID          string    `json:"id"`
	BuyOrderID  string    `json:"buy_order_id"`
	SellOrderID string    `json:"sell_order_id"`
	BuyerID     string    `json:"buyer_id"`
	SellerID    string    `json:"seller_id"`
	Symbol      string    `json:"symbol"`
	Quantity    float64   `json:"quantity"`
	Price       float64   `json:"price"`
	Timestamp   time.Time `json:"timestamp"`
}

// NewStrongConsistencyOrderProcessor creates a new enhanced order processor
func NewStrongConsistencyOrderProcessor(
	db *gorm.DB,
	logger *zap.Logger,
	raftCoordinator *consensus.RaftCoordinator,
	balanceManager *consistency.BalanceConsistencyManager,
	transactionManager StrongConsistencyTransactionManager,
	config *OrderProcessingConfig,
) *StrongConsistencyOrderProcessor {
	if config == nil {
		config = DefaultOrderProcessingConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &StrongConsistencyOrderProcessor{
		db:                 db,
		logger:             logger,
		raftCoordinator:    raftCoordinator,
		balanceManager:     balanceManager,
		transactionManager: transactionManager,
		orderLocks:         make(map[string]*sync.Mutex),
		config:             config,
		metrics:            &OrderProcessingMetrics{},
		ctx:                ctx,
		cancel:             cancel,
	}
}

// Start initializes the order processor
func (scop *StrongConsistencyOrderProcessor) Start(ctx context.Context) error {
	scop.logger.Info("Starting Strong Consistency Order Processor")

	// Start periodic metrics update
	scop.wg.Add(1)
	go scop.metricsUpdateLoop()

	scop.logger.Info("Strong Consistency Order Processor started successfully")
	return nil
}

// Stop gracefully shuts down the order processor
func (scop *StrongConsistencyOrderProcessor) Stop(ctx context.Context) error {
	scop.logger.Info("Stopping Strong Consistency Order Processor")

	scop.cancel()
	scop.wg.Wait()

	scop.logger.Info("Strong Consistency Order Processor stopped")
	return nil
}

// ProcessOrder processes a single order with strong consistency guarantees
func (scop *StrongConsistencyOrderProcessor) ProcessOrder(ctx context.Context, order *Order) (*OrderProcessingResult, error) {
	startTime := time.Now()
	orderID := order.ID

	scop.logger.Info("Processing order with strong consistency",
		zap.String("order_id", orderID),
		zap.String("user_id", order.UserID),
		zap.String("symbol", order.Symbol),
		zap.Float64("quantity", order.Quantity),
		zap.Float64("price", order.Price))

	// Initialize result
	result := &OrderProcessingResult{
		OrderID:           orderID,
		Status:            "processing",
		ConsensusRequired: scop.requiresConsensus(order),
		Metadata:          make(map[string]interface{}),
	}

	// Acquire order-specific lock to prevent concurrent processing
	orderLock := scop.getOrderLock(orderID)
	orderLock.Lock()
	defer orderLock.Unlock()

	// Step 1: Validate order parameters
	if err := scop.validateOrder(order); err != nil {
		result.Status = "rejected"
		result.Error = fmt.Sprintf("order validation failed: %v", err)
		scop.updateMetrics(func(m *OrderProcessingMetrics) {
			m.OrdersRejected++
		})
		return result, nil
	}

	// Step 2: Validate user balance if required
	if scop.config.ValidateBalanceBeforeMatching {
		if err := scop.validateBalance(ctx, order); err != nil {
			result.Status = "rejected"
			result.Error = fmt.Sprintf("balance validation failed: %v", err)
			result.BalanceValidated = false
			scop.updateMetrics(func(m *OrderProcessingMetrics) {
				m.BalanceValidationFailures++
				m.OrdersRejected++
			})
			return result, nil
		}
		result.BalanceValidated = true
	}

	// Step 3: Require consensus for large orders
	if result.ConsensusRequired {
		approved, err := scop.requestConsensus(ctx, order)
		if err != nil {
			result.Status = "rejected"
			result.Error = fmt.Sprintf("consensus request failed: %v", err)
			return result, err
		}
		if !approved {
			result.Status = "rejected"
			result.Error = "order not approved by consensus"
			result.ConsensusApproved = false
			return result, nil
		}
		result.ConsensusApproved = true

		scop.updateMetrics(func(m *OrderProcessingMetrics) {
			m.ConsensusOrdersProcessed++
		})
	}

	// Step 4: Process order atomically
	trades, err := scop.processOrderAtomically(ctx, order)
	if err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("atomic processing failed: %v", err)
		return result, err
	}

	// Step 5: Update result
	result.Status = "completed"
	result.MatchedTrades = trades
	result.ProcessingLatency = time.Since(startTime)
	result.Metadata["processing_time"] = result.ProcessingLatency.String()
	result.Metadata["trades_count"] = len(trades)

	scop.updateMetrics(func(m *OrderProcessingMetrics) {
		m.OrdersProcessed++
		m.AverageProcessingLatency = (m.AverageProcessingLatency + result.ProcessingLatency) / 2
	})

	scop.logger.Info("Order processed successfully",
		zap.String("order_id", orderID),
		zap.Duration("processing_time", result.ProcessingLatency),
		zap.Int("trades_count", len(trades)))

	return result, nil
}

// ProcessOrderBatch processes multiple orders as a batch with strong consistency
func (scop *StrongConsistencyOrderProcessor) ProcessOrderBatch(ctx context.Context, orders []*Order) ([]*OrderProcessingResult, error) {
	if !scop.config.EnableOrderBatching {
		return scop.processOrdersSequentially(ctx, orders)
	}

	scop.logger.Info("Processing order batch",
		zap.Int("batch_size", len(orders)))

	// Group orders by symbol for better processing efficiency
	ordersBySymbol := scop.groupOrdersBySymbol(orders)

	var allResults []*OrderProcessingResult
	var resultMutex sync.Mutex
	var wg sync.WaitGroup

	// Process each symbol group concurrently
	for symbol, symbolOrders := range ordersBySymbol {
		wg.Add(1)
		go func(sym string, symOrders []*Order) {
			defer wg.Done()

			results, err := scop.processSymbolOrderBatch(ctx, sym, symOrders)
			if err != nil {
				scop.logger.Error("Failed to process symbol order batch",
					zap.String("symbol", sym),
					zap.Error(err))
				return
			}

			resultMutex.Lock()
			allResults = append(allResults, results...)
			resultMutex.Unlock()
		}(symbol, symbolOrders)
	}

	wg.Wait()

	scop.logger.Info("Order batch processed",
		zap.Int("total_orders", len(orders)),
		zap.Int("total_results", len(allResults)))

	return allResults, nil
}

// requiresConsensus determines if an order requires consensus approval
func (scop *StrongConsistencyOrderProcessor) requiresConsensus(order *Order) bool {
	if !scop.config.RequireConsensusForLargeOrders {
		return false
	}

	orderValue := order.Quantity * order.Price
	return orderValue >= scop.config.LargeOrderThreshold
}

// validateOrder performs basic order validation
func (scop *StrongConsistencyOrderProcessor) validateOrder(order *Order) error {
	if order.UserID == "" {
		return fmt.Errorf("user ID is required")
	}
	if order.Symbol == "" {
		return fmt.Errorf("symbol is required")
	}
	if order.Side != "buy" && order.Side != "sell" {
		return fmt.Errorf("invalid side: %s", order.Side)
	}
	if order.Quantity <= 0 {
		return fmt.Errorf("quantity must be positive")
	}
	if order.Price <= 0 {
		return fmt.Errorf("price must be positive")
	}

	return nil
}

// validateBalance validates that the user has sufficient balance for the order
func (scop *StrongConsistencyOrderProcessor) validateBalance(ctx context.Context, order *Order) error {
	balanceCtx, cancel := context.WithTimeout(ctx, scop.config.BalanceCheckTimeout)
	defer cancel()

	var requiredCurrency string
	var requiredAmount float64

	if order.Side == "buy" {
		// For buy orders, check quote currency balance
		requiredCurrency = scop.getQuoteCurrency(order.Symbol)
		requiredAmount = order.Quantity * order.Price
	} else {
		// For sell orders, check base currency balance
		requiredCurrency = scop.getBaseCurrency(order.Symbol)
		requiredAmount = order.Quantity
	}

	balance, err := scop.balanceManager.GetBalance(balanceCtx, order.UserID, requiredCurrency)
	if err != nil {
		return fmt.Errorf("failed to get balance: %w", err)
	}

	if balance < requiredAmount {
		return fmt.Errorf("insufficient balance: required %f, available %f", requiredAmount, balance)
	}

	return nil
}

// requestConsensus requests consensus approval for a large order
func (scop *StrongConsistencyOrderProcessor) requestConsensus(ctx context.Context, order *Order) (bool, error) {
	consensusCtx, cancel := context.WithTimeout(ctx, scop.config.ConsensusTimeout)
	defer cancel()

	operation := consensus.Operation{
		ID:   fmt.Sprintf("order-%s", order.ID),
		Type: consensus.OperationTypeOrderProcessing,
		Data: map[string]interface{}{
			"order_id":    order.ID,
			"user_id":     order.UserID,
			"symbol":      order.Symbol,
			"side":        order.Side,
			"quantity":    order.Quantity,
			"price":       order.Price,
			"order_value": order.Quantity * order.Price,
		},
		Timestamp: time.Now(),
	}

	startTime := time.Now()
	approved, err := scop.raftCoordinator.ProposeGenericOperation(consensusCtx, operation)

	scop.updateMetrics(func(m *OrderProcessingMetrics) {
		m.ConsensusLatency = time.Since(startTime)
	})

	return approved, err
}

// processOrderAtomically processes an order atomically with strong consistency
func (scop *StrongConsistencyOrderProcessor) processOrderAtomically(ctx context.Context, order *Order) ([]Trade, error) {
	// Create transaction request with operations
	requestData := map[string]interface{}{
		"operations": []map[string]interface{}{
			{
				"service":   "bookkeeper",
				"operation": "lock_funds",
				"parameters": map[string]interface{}{
					"user_id":  order.UserID,
					"currency": scop.getRequiredCurrency(order),
					"amount":   scop.getRequiredAmount(order),
				},
			},
			{
				"service":   "trading",
				"operation": "match_order",
				"parameters": map[string]interface{}{
					"order_id": order.ID,
					"symbol":   order.Symbol,
					"side":     order.Side,
					"quantity": order.Quantity,
					"price":    order.Price,
				},
			},
		},
		"timeout": scop.config.OrderProcessingTimeout.Seconds(),
	}

	// Execute with strong consistency transaction manager using simplified interface
	result, err := scop.transactionManager.ExecuteStrongConsistencyTransaction(ctx, requestData)

	if err != nil {
		return nil, fmt.Errorf("transaction failed: %w", err)
	}

	// Extract trades from transaction result
	trades := scop.extractTradesFromResult(result)

	return trades, nil
}

// processOrdersSequentially processes orders one by one
func (scop *StrongConsistencyOrderProcessor) processOrdersSequentially(ctx context.Context, orders []*Order) ([]*OrderProcessingResult, error) {
	results := make([]*OrderProcessingResult, 0, len(orders))

	for _, order := range orders {
		result, err := scop.ProcessOrder(ctx, order)
		if err != nil {
			scop.logger.Error("Failed to process order in batch",
				zap.String("order_id", order.ID),
				zap.Error(err))
			// Continue processing other orders
			continue
		}
		results = append(results, result)
	}

	return results, nil
}

// groupOrdersBySymbol groups orders by trading symbol
func (scop *StrongConsistencyOrderProcessor) groupOrdersBySymbol(orders []*Order) map[string][]*Order {
	grouped := make(map[string][]*Order)

	for _, order := range orders {
		grouped[order.Symbol] = append(grouped[order.Symbol], order)
	}

	return grouped
}

// processSymbolOrderBatch processes a batch of orders for a specific symbol
func (scop *StrongConsistencyOrderProcessor) processSymbolOrderBatch(ctx context.Context, symbol string, orders []*Order) ([]*OrderProcessingResult, error) {
	// For now, process sequentially within symbol group
	// In a real implementation, this could implement more sophisticated matching algorithms
	return scop.processOrdersSequentially(ctx, orders)
}

// Helper methods for currency and amount calculations
func (scop *StrongConsistencyOrderProcessor) getQuoteCurrency(symbol string) string {
	// Simple implementation - in reality this would be more sophisticated
	if len(symbol) >= 6 {
		return symbol[3:] // e.g., "BTCUSD" -> "USD"
	}
	return "USD"
}

func (scop *StrongConsistencyOrderProcessor) getBaseCurrency(symbol string) string {
	// Simple implementation - in reality this would be more sophisticated
	if len(symbol) >= 3 {
		return symbol[:3] // e.g., "BTCUSD" -> "BTC"
	}
	return symbol
}

func (scop *StrongConsistencyOrderProcessor) getRequiredCurrency(order *Order) string {
	if order.Side == "buy" {
		return scop.getQuoteCurrency(order.Symbol)
	}
	return scop.getBaseCurrency(order.Symbol)
}

func (scop *StrongConsistencyOrderProcessor) getRequiredAmount(order *Order) float64 {
	if order.Side == "buy" {
		return order.Quantity * order.Price
	}
	return order.Quantity
}

// extractTradesFromResult extracts trades from transaction result
func (scop *StrongConsistencyOrderProcessor) extractTradesFromResult(result interface{}) []Trade {
	// This is a simplified implementation
	// In reality, trades would be extracted from the matching engine results
	return []Trade{
		{
			ID:        uuid.New().String(),
			Timestamp: time.Now(),
			// Other fields would be populated from actual matching results
		},
	}
}

// getOrderLock gets or creates a mutex for a specific order
func (scop *StrongConsistencyOrderProcessor) getOrderLock(orderID string) *sync.Mutex {
	scop.orderLocksMutex.Lock()
	defer scop.orderLocksMutex.Unlock()

	if lock, exists := scop.orderLocks[orderID]; exists {
		return lock
	}

	lock := &sync.Mutex{}
	scop.orderLocks[orderID] = lock
	return lock
}

// updateMetrics safely updates metrics
func (scop *StrongConsistencyOrderProcessor) updateMetrics(updateFunc func(*OrderProcessingMetrics)) {
	scop.metricsMutex.Lock()
	defer scop.metricsMutex.Unlock()

	updateFunc(scop.metrics)
	scop.metrics.LastUpdate = time.Now()
}

// GetMetrics returns current order processing metrics
func (scop *StrongConsistencyOrderProcessor) GetMetrics() *OrderProcessingMetrics {
	scop.metricsMutex.RLock()
	defer scop.metricsMutex.RUnlock()

	metricsCopy := *scop.metrics
	return &metricsCopy
}

// metricsUpdateLoop periodically updates metrics
func (scop *StrongConsistencyOrderProcessor) metricsUpdateLoop() {
	defer scop.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-scop.ctx.Done():
			return
		case <-ticker.C:
			// Update active orders count and other derived metrics
			scop.updateActiveOrdersCount()
		}
	}
}

// updateActiveOrdersCount updates the count of active orders
func (scop *StrongConsistencyOrderProcessor) updateActiveOrdersCount() {
	// This would query the database for active orders
	// For now, we'll use the lock count as a proxy
	scop.orderLocksMutex.RLock()
	activeCount := int64(len(scop.orderLocks))
	scop.orderLocksMutex.RUnlock()

	scop.updateMetrics(func(m *OrderProcessingMetrics) {
		m.ActiveOrders = activeCount
	})
}

// DefaultOrderProcessingConfig returns default configuration
func DefaultOrderProcessingConfig() *OrderProcessingConfig {
	return &OrderProcessingConfig{
		LargeOrderThreshold:            100000.0,  // $100k
		CriticalOrderThreshold:         1000000.0, // $1M
		OrderProcessingTimeout:         30 * time.Second,
		ConsensusTimeout:               15 * time.Second,
		BalanceCheckTimeout:            5 * time.Second,
		RequireConsensusForLargeOrders: true,
		EnableAtomicMatching:           true,
		ValidateBalanceBeforeMatching:  true,
		MaxConcurrentOrders:            100,
		EnableOrderBatching:            true,
		BatchSize:                      50,
		BatchTimeout:                   5 * time.Second,
	}
}

// IsHealthy checks if the order processor is healthy
func (scop *StrongConsistencyOrderProcessor) IsHealthy() bool {
	// Check if the processor is running
	select {
	case <-scop.ctx.Done():
		return false
	default:
	}

	// Check if Raft coordinator is healthy
	if scop.raftCoordinator != nil && !scop.raftCoordinator.IsHealthy() {
		return false
	}

	// Check if balance manager is healthy
	if scop.balanceManager != nil && !scop.balanceManager.IsHealthy() {
		return false
	}
	// Check metrics for concerning patterns
	metrics := scop.GetMetrics()
	if metrics != nil {
		// If we have too many rejected orders, consider unhealthy
		if metrics.OrdersRejected > 100 &&
			time.Since(metrics.LastUpdate) < 5*time.Minute {
			return false
		}
	}

	return true
}
