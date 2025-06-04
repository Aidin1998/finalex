package coordination

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/accounts/bookkeeper"
	"github.com/Aidin1998/pincex_unified/internal/trading/consensus"
	"github.com/Aidin1998/pincex_unified/internal/trading/consistency"
	"github.com/Aidin1998/pincex_unified/internal/trading/model"
	"github.com/Aidin1998/pincex_unified/internal/trading/settlement"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Trade represents a trade for coordination purposes
type Trade struct {
	ID        uuid.UUID       `json:"id"`
	BuyerID   uuid.UUID       `json:"buyer_id"`
	SellerID  uuid.UUID       `json:"seller_id"`
	Symbol    string          `json:"symbol"`
	Quantity  decimal.Decimal `json:"quantity"`
	Price     decimal.Decimal `json:"price"`
	Timestamp time.Time       `json:"timestamp"`
}

// StrongConsistencySettlementCoordinator provides settlement with strong consistency guarantees
type StrongConsistencySettlementCoordinator struct {
	logger           *zap.Logger
	db               *gorm.DB
	settlementEngine *settlement.SettlementEngine
	bookkeeper       bookkeeper.BookkeeperService
	consensusCoord   *consensus.RaftCoordinator
	balanceManager   *consistency.BalanceConsistencyManager

	// Settlement state tracking with strong consistency
	activeSettlements map[string]*SettlementBatch
	settlementLocks   map[string]*sync.RWMutex
	globalLock        sync.RWMutex

	// Settlement coordination
	settlementQueue chan *TradeSettlementRequest
	batchQueue      chan *SettlementBatch
	completionQueue chan *SettlementResult

	// Configuration
	batchSize          int
	batchTimeout       time.Duration
	maxRetries         int
	consensusThreshold decimal.Decimal

	// Workers
	workers     []SettlementWorker
	workerCount int
	shutdownCh  chan struct{}
	workersWg   sync.WaitGroup

	// Metrics
	metrics *SettlementMetrics
}

// TradeSettlementRequest represents a trade that needs settlement
type TradeSettlementRequest struct {
	TradeID       uuid.UUID       `json:"trade_id"`
	OrderID       uuid.UUID       `json:"order_id"`
	BuyUserID     uuid.UUID       `json:"buy_user_id"`
	SellUserID    uuid.UUID       `json:"sell_user_id"`
	Symbol        string          `json:"symbol"`
	Quantity      decimal.Decimal `json:"quantity"`
	Price         decimal.Decimal `json:"price"`
	TakerSide     string          `json:"taker_side"`
	BaseCurrency  string          `json:"base_currency"`
	QuoteCurrency string          `json:"quote_currency"`
	Timestamp     time.Time       `json:"timestamp"`
	Priority      int             `json:"priority"`
}

// SettlementBatch represents a batch of trades for atomic settlement
type SettlementBatch struct {
	ID                string                    `json:"id"`
	Trades            []*TradeSettlementRequest `json:"trades"`
	NetPositions      map[string]*NetPosition   `json:"net_positions"`
	RequiredTransfers []BalanceTransfer         `json:"required_transfers"`
	CreatedAt         time.Time                 `json:"created_at"`
	Status            string                    `json:"status"`
	ConsensusRequired bool                      `json:"consensus_required"`
	AttemptCount      int                       `json:"attempt_count"`
}

// NetPosition represents a user's net position for settlement
type NetPosition struct {
	UserID        string          `json:"user_id"`
	BaseCurrency  string          `json:"base_currency"`
	QuoteCurrency string          `json:"quote_currency"`
	BaseQuantity  decimal.Decimal `json:"base_quantity"`
	QuoteQuantity decimal.Decimal `json:"quote_quantity"`
}

// BalanceTransfer represents a required balance transfer for settlement
type BalanceTransfer struct {
	FromUserID  string          `json:"from_user_id"`
	ToUserID    string          `json:"to_user_id"`
	Currency    string          `json:"currency"`
	Amount      decimal.Decimal `json:"amount"`
	Reference   string          `json:"reference"`
	Description string          `json:"description"`
}

// SettlementResult represents the result of a settlement operation
type SettlementResult struct {
	BatchID       string          `json:"batch_id"`
	Success       bool            `json:"success"`
	CompletedAt   time.Time       `json:"completed_at"`
	Error         error           `json:"error,omitempty"`
	TradesSettled int             `json:"trades_settled"`
	TotalValue    decimal.Decimal `json:"total_value"`
}

// SettlementWorker handles settlement processing
type SettlementWorker struct {
	ID          int
	coordinator *StrongConsistencySettlementCoordinator
	shutdownCh  chan struct{}
}

// SettlementMetrics tracks settlement performance
type SettlementMetrics struct {
	TotalBatches        int64         `json:"total_batches"`
	SuccessfulBatches   int64         `json:"successful_batches"`
	FailedBatches       int64         `json:"failed_batches"`
	TradesSettled       int64         `json:"trades_settled"`
	AverageLatency      time.Duration `json:"average_latency"`
	AverageBatchSize    float64       `json:"average_batch_size"`
	ConsensusOperations int64         `json:"consensus_operations"`
	BalanceTransfers    int64         `json:"balance_transfers"`
	RecoveryOperations  int64         `json:"recovery_operations"`
	mu                  sync.RWMutex
}

// NewStrongConsistencySettlementCoordinator creates a new settlement coordinator
func NewStrongConsistencySettlementCoordinator(
	logger *zap.Logger,
	db *gorm.DB,
	settlementEngine *settlement.SettlementEngine,
	bookkeeper bookkeeper.BookkeeperService,
	consensusCoord *consensus.RaftCoordinator,
	balanceManager *consistency.BalanceConsistencyManager,
) *StrongConsistencySettlementCoordinator {
	return &StrongConsistencySettlementCoordinator{
		logger:             logger,
		db:                 db,
		settlementEngine:   settlementEngine,
		bookkeeper:         bookkeeper,
		consensusCoord:     consensusCoord,
		balanceManager:     balanceManager,
		activeSettlements:  make(map[string]*SettlementBatch),
		settlementLocks:    make(map[string]*sync.RWMutex),
		settlementQueue:    make(chan *TradeSettlementRequest, 10000),
		batchQueue:         make(chan *SettlementBatch, 1000),
		completionQueue:    make(chan *SettlementResult, 1000),
		batchSize:          100,
		batchTimeout:       5 * time.Second,
		maxRetries:         3,
		consensusThreshold: decimal.NewFromFloat(50000.0), // $50k threshold
		workerCount:        5,
		shutdownCh:         make(chan struct{}),
		metrics:            &SettlementMetrics{},
	}
}

// Start initializes the settlement coordinator
func (sc *StrongConsistencySettlementCoordinator) Start(ctx context.Context) error {
	sc.logger.Info("Starting Strong Consistency Settlement Coordinator",
		zap.Int("worker_count", sc.workerCount),
		zap.Int("batch_size", sc.batchSize),
		zap.Duration("batch_timeout", sc.batchTimeout))

	// Start batch aggregator
	go sc.batchAggregator(ctx)

	// Start settlement workers
	for i := 0; i < sc.workerCount; i++ {
		worker := SettlementWorker{
			ID:          i,
			coordinator: sc,
			shutdownCh:  make(chan struct{}),
		}
		sc.workers = append(sc.workers, worker)
		go worker.start(ctx)
	}

	// Start completion processor
	go sc.completionProcessor(ctx)

	return nil
}

// SubmitTradeForSettlement submits a trade for settlement
func (sc *StrongConsistencySettlementCoordinator) SubmitTradeForSettlement(
	ctx context.Context,
	request *TradeSettlementRequest,
) error {
	select {
	case sc.settlementQueue <- request:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(5 * time.Second):
		return fmt.Errorf("settlement queue timeout")
	}
}

// batchAggregator aggregates trades into settlement batches
func (sc *StrongConsistencySettlementCoordinator) batchAggregator(ctx context.Context) {
	ticker := time.NewTicker(sc.batchTimeout)
	defer ticker.Stop()

	var currentBatch []*TradeSettlementRequest

	for {
		select {
		case trade := <-sc.settlementQueue:
			currentBatch = append(currentBatch, trade)

			// Create batch when size limit reached
			if len(currentBatch) >= sc.batchSize {
				sc.createAndSubmitBatch(currentBatch)
				currentBatch = nil
			}

		case <-ticker.C:
			// Create batch on timeout if we have trades
			if len(currentBatch) > 0 {
				sc.createAndSubmitBatch(currentBatch)
				currentBatch = nil
			}

		case <-sc.shutdownCh:
			// Process remaining trades
			if len(currentBatch) > 0 {
				sc.createAndSubmitBatch(currentBatch)
			}
			return

		case <-ctx.Done():
			return
		}
	}
}

// createAndSubmitBatch creates a settlement batch and submits it for processing
func (sc *StrongConsistencySettlementCoordinator) createAndSubmitBatch(trades []*TradeSettlementRequest) {
	batchID := fmt.Sprintf("batch_%d", time.Now().UnixNano())

	batch := &SettlementBatch{
		ID:        batchID,
		Trades:    trades,
		CreatedAt: time.Now(),
		Status:    "pending",
	}

	// Calculate net positions
	batch.NetPositions = sc.calculateNetPositions(trades)

	// Calculate required transfers
	batch.RequiredTransfers = sc.calculateRequiredTransfers(batch.NetPositions)

	// Determine if consensus is required
	batch.ConsensusRequired = sc.requiresConsensus(batch)

	// Track active settlement
	sc.globalLock.Lock()
	sc.activeSettlements[batchID] = batch
	if _, exists := sc.settlementLocks[batchID]; !exists {
		sc.settlementLocks[batchID] = &sync.RWMutex{}
	}
	sc.globalLock.Unlock()

	// Submit for processing
	select {
	case sc.batchQueue <- batch:
		sc.logger.Info("Settlement batch created",
			zap.String("batch_id", batchID),
			zap.Int("trade_count", len(trades)),
			zap.Bool("consensus_required", batch.ConsensusRequired))
	default:
		sc.logger.Error("Failed to submit batch - queue full", zap.String("batch_id", batchID))
	}
}

// calculateNetPositions calculates net positions for a batch of trades
func (sc *StrongConsistencySettlementCoordinator) calculateNetPositions(
	trades []*TradeSettlementRequest,
) map[string]*NetPosition {
	positions := make(map[string]*NetPosition)

	for _, trade := range trades {
		// Calculate net positions for both users
		sc.updateNetPosition(positions, trade.BuyUserID.String(), trade.BaseCurrency, trade.QuoteCurrency, trade.Quantity, trade.Price.Neg())
		sc.updateNetPosition(positions, trade.SellUserID.String(), trade.BaseCurrency, trade.QuoteCurrency, trade.Quantity.Neg(), trade.Price)
	}

	return positions
}

// updateNetPosition updates a user's net position
func (sc *StrongConsistencySettlementCoordinator) updateNetPosition(
	positions map[string]*NetPosition,
	userID, baseCurrency, quoteCurrency string,
	baseQty, quoteValue decimal.Decimal,
) {
	key := fmt.Sprintf("%s_%s_%s", userID, baseCurrency, quoteCurrency)

	if position, exists := positions[key]; exists {
		position.BaseQuantity = position.BaseQuantity.Add(baseQty)
		position.QuoteQuantity = position.QuoteQuantity.Add(quoteValue)
	} else {
		positions[key] = &NetPosition{
			UserID:        userID,
			BaseCurrency:  baseCurrency,
			QuoteCurrency: quoteCurrency,
			BaseQuantity:  baseQty,
			QuoteQuantity: quoteValue,
		}
	}
}

// calculateRequiredTransfers calculates the balance transfers needed for settlement
func (sc *StrongConsistencySettlementCoordinator) calculateRequiredTransfers(
	positions map[string]*NetPosition,
) []BalanceTransfer {
	var transfers []BalanceTransfer

	// Group positions by currency
	baseCurrencyPositions := make(map[string]map[string]decimal.Decimal)
	quoteCurrencyPositions := make(map[string]map[string]decimal.Decimal)

	for _, position := range positions {
		// Base currency positions
		if _, exists := baseCurrencyPositions[position.BaseCurrency]; !exists {
			baseCurrencyPositions[position.BaseCurrency] = make(map[string]decimal.Decimal)
		}
		baseCurrencyPositions[position.BaseCurrency][position.UserID] = position.BaseQuantity

		// Quote currency positions
		if _, exists := quoteCurrencyPositions[position.QuoteCurrency]; !exists {
			quoteCurrencyPositions[position.QuoteCurrency] = make(map[string]decimal.Decimal)
		}
		quoteCurrencyPositions[position.QuoteCurrency][position.UserID] = position.QuoteQuantity
	}

	// Calculate transfers for each currency
	transfers = append(transfers, sc.calculateCurrencyTransfers(baseCurrencyPositions)...)
	transfers = append(transfers, sc.calculateCurrencyTransfers(quoteCurrencyPositions)...)

	return transfers
}

// calculateCurrencyTransfers calculates transfers for a specific currency
func (sc *StrongConsistencySettlementCoordinator) calculateCurrencyTransfers(
	currencyPositions map[string]map[string]decimal.Decimal,
) []BalanceTransfer {
	var transfers []BalanceTransfer

	for currency, positions := range currencyPositions {
		// Find users with positive and negative balances
		var creditors []struct {
			userID string
			amount decimal.Decimal
		}
		var debtors []struct {
			userID string
			amount decimal.Decimal
		}

		for userID, amount := range positions {
			if amount.IsPositive() {
				creditors = append(creditors, struct {
					userID string
					amount decimal.Decimal
				}{userID, amount})
			} else if amount.IsNegative() {
				debtors = append(debtors, struct {
					userID string
					amount decimal.Decimal
				}{userID, amount.Abs()})
			}
		}

		// Match creditors with debtors
		for _, debtor := range debtors {
			remaining := debtor.amount

			for i := 0; i < len(creditors) && remaining.IsPositive(); i++ {
				if creditors[i].amount.IsPositive() {
					transferAmount := decimal.Min(remaining, creditors[i].amount)

					transfers = append(transfers, BalanceTransfer{
						FromUserID:  debtor.userID,
						ToUserID:    creditors[i].userID,
						Currency:    currency,
						Amount:      transferAmount,
						Reference:   fmt.Sprintf("settlement_%s", currency),
						Description: fmt.Sprintf("Settlement transfer for %s", currency),
					})

					remaining = remaining.Sub(transferAmount)
					creditors[i].amount = creditors[i].amount.Sub(transferAmount)
				}
			}
		}
	}

	return transfers
}

// requiresConsensus determines if a batch requires consensus
func (sc *StrongConsistencySettlementCoordinator) requiresConsensus(batch *SettlementBatch) bool {
	// Calculate total value of the batch
	totalValue := decimal.Zero
	for _, trade := range batch.Trades {
		tradeValue := trade.Quantity.Mul(trade.Price)
		totalValue = totalValue.Add(tradeValue)
	}

	// Require consensus for high-value batches
	return totalValue.GreaterThan(sc.consensusThreshold)
}

// start starts a settlement worker
func (worker *SettlementWorker) start(ctx context.Context) {
	worker.coordinator.workersWg.Add(1)
	defer worker.coordinator.workersWg.Done()

	worker.coordinator.logger.Info("Starting settlement worker", zap.Int("worker_id", worker.ID))

	for {
		select {
		case batch := <-worker.coordinator.batchQueue:
			result := worker.processBatch(ctx, batch)

			select {
			case worker.coordinator.completionQueue <- result:
				// Successfully submitted result
			case <-ctx.Done():
				return
			}

		case <-worker.shutdownCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

// processBatch processes a settlement batch
func (worker *SettlementWorker) processBatch(ctx context.Context, batch *SettlementBatch) *SettlementResult {
	startTime := time.Now()
	coordinator := worker.coordinator

	coordinator.logger.Info("Processing settlement batch",
		zap.String("batch_id", batch.ID),
		zap.Int("worker_id", worker.ID),
		zap.Int("trade_count", len(batch.Trades)))

	// Get batch lock
	coordinator.globalLock.RLock()
	batchLock := coordinator.settlementLocks[batch.ID]
	coordinator.globalLock.RUnlock()

	batchLock.Lock()
	defer batchLock.Unlock()

	result := &SettlementResult{
		BatchID:     batch.ID,
		CompletedAt: time.Now(),
	}

	// Increment attempt count
	batch.AttemptCount++

	// Get consensus if required
	if batch.ConsensusRequired && coordinator.consensusCoord != nil {
		if err := worker.getConsensusApproval(ctx, batch); err != nil {
			result.Success = false
			result.Error = fmt.Errorf("consensus approval failed: %w", err)
			return result
		}

		coordinator.metrics.mu.Lock()
		coordinator.metrics.ConsensusOperations++
		coordinator.metrics.mu.Unlock()
	}

	// Execute settlement atomically
	if err := worker.executeSettlement(ctx, batch); err != nil {
		result.Success = false
		result.Error = err

		// Retry if under max attempts
		if batch.AttemptCount < coordinator.maxRetries {
			coordinator.logger.Warn("Settlement failed, will retry",
				zap.String("batch_id", batch.ID),
				zap.Int("attempt", batch.AttemptCount),
				zap.Error(err))

			// Re-queue for retry
			go func() {
				time.Sleep(time.Duration(batch.AttemptCount) * time.Second)
				select {
				case coordinator.batchQueue <- batch:
				case <-ctx.Done():
				}
			}()
		}

		return result
	}

	// Settlement successful
	result.Success = true
	result.TradesSettled = len(batch.Trades)

	// Calculate total value
	totalValue := decimal.Zero
	for _, trade := range batch.Trades {
		totalValue = totalValue.Add(trade.Quantity.Mul(trade.Price))
	}
	result.TotalValue = totalValue

	// Update metrics
	coordinator.metrics.mu.Lock()
	coordinator.metrics.TradesSettled += int64(len(batch.Trades))
	coordinator.metrics.AverageLatency = (coordinator.metrics.AverageLatency + time.Since(startTime)) / 2
	coordinator.metrics.BalanceTransfers += int64(len(batch.RequiredTransfers))
	coordinator.metrics.mu.Unlock()

	coordinator.logger.Info("Settlement batch completed successfully",
		zap.String("batch_id", batch.ID),
		zap.Int("trades_settled", len(batch.Trades)),
		zap.String("total_value", totalValue.String()),
		zap.Duration("duration", time.Since(startTime)))

	return result
}

// getConsensusApproval gets consensus approval for a settlement batch
func (worker *SettlementWorker) getConsensusApproval(ctx context.Context, batch *SettlementBatch) error {
	coordinator := worker.coordinator

	// Calculate total value for consensus operation
	totalValue := decimal.Zero
	for _, trade := range batch.Trades {
		totalValue = totalValue.Add(trade.Quantity.Mul(trade.Price))
	}

	consensusOp := &consensus.CriticalOperation{
		ID:       fmt.Sprintf("settlement_%s", batch.ID),
		Type:     "settlement_batch",
		Amount:   totalValue,
		Currency: "USD", // Use base currency
		Priority: 1,     // Settlement operations are critical
		Metadata: map[string]interface{}{
			"batch_id":           batch.ID,
			"trade_count":        len(batch.Trades),
			"transfer_count":     len(batch.RequiredTransfers),
			"consensus_required": true,
		},
		Timestamp: time.Now(),
	}

	result, err := coordinator.consensusCoord.ProposeOperation(ctx, consensusOp)
	if err != nil {
		return err
	}

	if !result.Approved {
		return fmt.Errorf("settlement batch rejected by consensus: %v", result.Error)
	}

	return nil
}

// executeSettlement executes the settlement atomically
func (worker *SettlementWorker) executeSettlement(ctx context.Context, batch *SettlementBatch) error {
	coordinator := worker.coordinator

	// Start database transaction
	tx := coordinator.db.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer tx.Rollback()

	// Execute all balance transfers atomically
	for _, transfer := range batch.RequiredTransfers {
		err := coordinator.balanceManager.AtomicTransfer(
			ctx,
			transfer.FromUserID,
			transfer.ToUserID,
			transfer.Currency,
			transfer.Amount,
			transfer.Reference,
			transfer.Description,
		)
		if err != nil {
			return fmt.Errorf("balance transfer failed: %w", err)
		}
	}

	// Update trade statuses
	for _, trade := range batch.Trades {
		err := tx.Model(&model.Trade{}).
			Where("id = ?", trade.TradeID).
			Update("settlement_status", "settled").Error
		if err != nil {
			return fmt.Errorf("failed to update trade status: %w", err)
		}
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit settlement transaction: %w", err)
	}

	batch.Status = "completed"
	return nil
}

// completionProcessor processes settlement completion results
func (sc *StrongConsistencySettlementCoordinator) completionProcessor(ctx context.Context) {
	for {
		select {
		case result := <-sc.completionQueue:
			sc.handleSettlementResult(result)
		case <-sc.shutdownCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

// handleSettlementResult handles a settlement result
func (sc *StrongConsistencySettlementCoordinator) handleSettlementResult(result *SettlementResult) {
	// Update metrics
	sc.metrics.mu.Lock()
	sc.metrics.TotalBatches++
	if result.Success {
		sc.metrics.SuccessfulBatches++
	} else {
		sc.metrics.FailedBatches++
	}
	sc.metrics.mu.Unlock()

	// Clean up active settlement
	sc.globalLock.Lock()
	delete(sc.activeSettlements, result.BatchID)
	delete(sc.settlementLocks, result.BatchID)
	sc.globalLock.Unlock()

	if result.Success {
		sc.logger.Info("Settlement completed successfully",
			zap.String("batch_id", result.BatchID),
			zap.Int("trades_settled", result.TradesSettled))
	} else {
		sc.logger.Error("Settlement failed",
			zap.String("batch_id", result.BatchID),
			zap.Error(result.Error))
	}
}

// GetMetrics returns current settlement metrics
func (sc *StrongConsistencySettlementCoordinator) GetMetrics() *SettlementMetrics {
	sc.metrics.mu.RLock()
	defer sc.metrics.mu.RUnlock()

	metrics := &SettlementMetrics{
		TotalBatches:        sc.metrics.TotalBatches,
		SuccessfulBatches:   sc.metrics.SuccessfulBatches,
		FailedBatches:       sc.metrics.FailedBatches,
		TradesSettled:       sc.metrics.TradesSettled,
		AverageLatency:      sc.metrics.AverageLatency,
		ConsensusOperations: sc.metrics.ConsensusOperations,
		BalanceTransfers:    sc.metrics.BalanceTransfers,
		RecoveryOperations:  sc.metrics.RecoveryOperations,
	}

	if sc.metrics.TotalBatches > 0 {
		metrics.AverageBatchSize = float64(sc.metrics.TradesSettled) / float64(sc.metrics.TotalBatches)
	}

	return metrics
}

// Stop gracefully shuts down the settlement coordinator
func (sc *StrongConsistencySettlementCoordinator) Stop(ctx context.Context) error {
	sc.logger.Info("Stopping Strong Consistency Settlement Coordinator")

	close(sc.shutdownCh)

	// Stop workers
	for _, worker := range sc.workers {
		close(worker.shutdownCh)
	}

	// Wait for workers to complete
	sc.workersWg.Wait()

	return nil
}

// IsHealthy checks if the settlement coordinator is healthy
func (sc *StrongConsistencySettlementCoordinator) IsHealthy() bool {
	// Check if workers are running
	if sc.shutdownCh == nil {
		return false
	}

	select {
	case <-sc.shutdownCh:
		return false
	default:
	}

	// Check if consensus coordinator is healthy
	if sc.consensusCoord != nil && !sc.consensusCoord.IsHealthy() {
		return false
	}

	// Check if balance manager is healthy
	if sc.balanceManager != nil && !sc.balanceManager.IsHealthy() {
		return false
	}

	// Check metrics for concerning patterns
	sc.metrics.mu.RLock()
	failureRate := float64(sc.metrics.FailedBatches) / float64(sc.metrics.TotalBatches+1)
	sc.metrics.mu.RUnlock()

	// If failure rate is too high, consider unhealthy
	if failureRate > 0.1 { // 10% failure rate threshold
		return false
	}

	return true
}

// ProcessTradeBatch processes a batch of trades for settlement
func (sc *StrongConsistencySettlementCoordinator) ProcessTradeBatch(ctx context.Context, trades []Trade) error {
	if len(trades) == 0 {
		return nil
	}

	sc.logger.Info("Processing trade batch for settlement", zap.Int("count", len(trades)))

	// Create settlement batch
	batchID := uuid.New().String()
	batch := &SettlementBatch{
		ID:        batchID,
		Trades:    convertTradesToSettlementRequests(trades),
		Status:    "pending",
		CreatedAt: time.Now(),
	}

	// Add to active settlements
	sc.globalLock.Lock()
	sc.activeSettlements[batchID] = batch
	sc.settlementLocks[batchID] = &sync.RWMutex{}
	sc.globalLock.Unlock()
	// Process the batch using existing settlement logic
	sc.createAndSubmitBatch(batch.Trades)

	sc.logger.Info("Trade batch processed successfully",
		zap.String("batch_id", batchID),
		zap.Int("trades_count", len(trades)))

	return nil
}

// GetPendingBatchCount returns the number of pending settlement batches
func (sc *StrongConsistencySettlementCoordinator) GetPendingBatchCount() int {
	sc.globalLock.RLock()
	defer sc.globalLock.RUnlock()

	pendingCount := 0
	for _, batch := range sc.activeSettlements {
		if batch.Status == "pending" || batch.Status == "processing" {
			pendingCount++
		}
	}

	return pendingCount
}

// Helper function to convert coordination.Trade to settlement requests
func convertTradesToSettlementRequests(trades []Trade) []*TradeSettlementRequest {
	requests := make([]*TradeSettlementRequest, len(trades))
	for i, trade := range trades {
		requests[i] = &TradeSettlementRequest{
			TradeID:       trade.ID,
			OrderID:       uuid.New(), // Generate order ID if not available
			BuyUserID:     trade.BuyerID,
			SellUserID:    trade.SellerID,
			Symbol:        trade.Symbol,
			Quantity:      trade.Quantity,
			Price:         trade.Price,
			TakerSide:     "buy",            // Default to buy side
			BaseCurrency:  trade.Symbol[:3], // Extract base currency from symbol
			QuoteCurrency: trade.Symbol[3:], // Extract quote currency from symbol
			Timestamp:     trade.Timestamp,
			Priority:      1, // Default priority
		}
	}
	return requests
}
