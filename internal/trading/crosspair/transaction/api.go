// Package transaction - Unified API for cross-pair transaction lifecycle management
package transaction

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/Aidin1998/finalex/internal/accounts/transaction"
	"github.com/Aidin1998/finalex/internal/trading/crosspair"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// TransactionAPI provides a unified interface for transaction management
type TransactionAPI struct {
	coordinator     *CrossPairTransactionCoordinator
	retryManager    *RetryManager
	deadLetterQueue *DeadLetterQueue
	logger          *zap.Logger
}

// TransactionRequest represents a request to start a transaction
type TransactionRequest struct {
	Type     CrossPairTransactionType  `json:"type"`
	UserID   uuid.UUID                 `json:"user_id"`
	Order    *crosspair.CrossPairOrder `json:"order,omitempty"`
	Trade    *crosspair.CrossPairTrade `json:"trade,omitempty"`
	Route    *crosspair.CrossPairRoute `json:"route,omitempty"`
	Timeout  *time.Duration            `json:"timeout,omitempty"`
	TraceID  string                    `json:"trace_id,omitempty"`
	Metadata map[string]interface{}    `json:"metadata,omitempty"`
}

// TransactionResponse represents the response from a transaction operation
type TransactionResponse struct {
	TransactionID uuid.UUID           `json:"transaction_id"`
	State         string              `json:"state"`
	CreatedAt     time.Time           `json:"created_at"`
	CompletedAt   *time.Time          `json:"completed_at,omitempty"`
	TraceID       string              `json:"trace_id"`
	Result        *TransactionResult  `json:"result,omitempty"`
	Error         string              `json:"error,omitempty"`
	Metrics       *TransactionMetrics `json:"metrics,omitempty"`
}

// TransactionResult represents the result of a completed transaction
type TransactionResult struct {
	Trade         *crosspair.CrossPairTrade `json:"trade,omitempty"`
	ActualRate    *decimal.Decimal          `json:"actual_rate,omitempty"`
	Slippage      *decimal.Decimal          `json:"slippage,omitempty"`
	TotalFees     []crosspair.CrossPairFee  `json:"total_fees,omitempty"`
	GasUsed       decimal.Decimal           `json:"gas_used"`
	ExecutionTime time.Duration             `json:"execution_time"`
}

// TransactionListRequest represents parameters for listing transactions
type TransactionListRequest struct {
	UserID   *uuid.UUID                `json:"user_id,omitempty"`
	Type     *CrossPairTransactionType `json:"type,omitempty"`
	State    *TransactionState         `json:"state,omitempty"`
	FromTime *time.Time                `json:"from_time,omitempty"`
	ToTime   *time.Time                `json:"to_time,omitempty"`
	Limit    int                       `json:"limit,omitempty"`
	Offset   int                       `json:"offset,omitempty"`
}

// NewTransactionAPI creates a new transaction API
func NewTransactionAPI(
	coordinator *CrossPairTransactionCoordinator,
	retryManager *RetryManager,
	deadLetterQueue *DeadLetterQueue,
	logger *zap.Logger,
) *TransactionAPI {
	return &TransactionAPI{
		coordinator:     coordinator,
		retryManager:    retryManager,
		deadLetterQueue: deadLetterQueue,
		logger:          logger.Named("transaction-api"),
	}
}

// BeginTransaction starts a new cross-pair transaction
func (api *TransactionAPI) BeginTransaction(
	ctx context.Context,
	req *TransactionRequest,
) (*TransactionResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("transaction request is required")
	}

	// Validate request
	if err := api.validateTransactionRequest(req); err != nil {
		return nil, fmt.Errorf("invalid transaction request: %w", err)
	}

	// Begin transaction
	txnCtx, err := api.coordinator.BeginTransaction(ctx, req.Type, req.UserID, req.Order)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}

	// Set additional context
	if req.Trade != nil {
		txnCtx.Trade = req.Trade
	}
	if req.Route != nil {
		txnCtx.Route = req.Route
	}
	if req.Timeout != nil {
		txnCtx.TimeoutAt = time.Now().Add(*req.Timeout)
	}
	if req.TraceID != "" {
		txnCtx.TraceID = req.TraceID
	}

	// Register resources based on transaction type
	if err := api.registerResourcesForTransaction(txnCtx); err != nil {
		api.coordinator.AbortTransaction(ctx, txnCtx, fmt.Sprintf("failed to register resources: %v", err))
		return nil, fmt.Errorf("failed to register transaction resources: %w", err)
	}

	// Submit for processing
	if err := api.coordinator.workerPool.Submit(txnCtx); err != nil {
		api.coordinator.AbortTransaction(ctx, txnCtx, fmt.Sprintf("failed to submit for processing: %v", err))
		return nil, fmt.Errorf("failed to submit transaction for processing: %w", err)
	}

	return &TransactionResponse{
		TransactionID: txnCtx.ID,
		State:         TransactionState(atomic.LoadInt64(&txnCtx.state)).String(),
		CreatedAt:     txnCtx.CreatedAt,
		TraceID:       txnCtx.TraceID,
	}, nil
}

// CommitTransaction commits a transaction
func (api *TransactionAPI) CommitTransaction(
	ctx context.Context,
	transactionID uuid.UUID,
) (*TransactionResponse, error) {
	txnCtx, err := api.coordinator.GetTransaction(transactionID)
	if err != nil {
		return nil, fmt.Errorf("transaction not found: %w", err)
	}

	if err := api.coordinator.CommitTransaction(ctx, txnCtx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return api.buildTransactionResponse(txnCtx), nil
}

// AbortTransaction aborts a transaction
func (api *TransactionAPI) AbortTransaction(
	ctx context.Context,
	transactionID uuid.UUID,
	reason string,
) (*TransactionResponse, error) {
	txnCtx, err := api.coordinator.GetTransaction(transactionID)
	if err != nil {
		return nil, fmt.Errorf("transaction not found: %w", err)
	}

	if err := api.coordinator.AbortTransaction(ctx, txnCtx, reason); err != nil {
		return nil, fmt.Errorf("failed to abort transaction: %w", err)
	}

	return api.buildTransactionResponse(txnCtx), nil
}

// GetTransaction retrieves a transaction by ID
func (api *TransactionAPI) GetTransaction(
	ctx context.Context,
	transactionID uuid.UUID,
) (*TransactionResponse, error) {
	txnCtx, err := api.coordinator.GetTransaction(transactionID)
	if err != nil {
		return nil, fmt.Errorf("transaction not found: %w", err)
	}

	return api.buildTransactionResponse(txnCtx), nil
}

// WaitForTransaction waits for a transaction to complete
func (api *TransactionAPI) WaitForTransaction(
	ctx context.Context,
	transactionID uuid.UUID,
	timeout time.Duration,
) (*TransactionResponse, error) {
	txnCtx, err := api.coordinator.GetTransaction(transactionID)
	if err != nil {
		return nil, fmt.Errorf("transaction not found: %w", err)
	}

	// Wait for completion or timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	select {
	case <-txnCtx.completedChan:
		return api.buildTransactionResponse(txnCtx), nil
	case <-timeoutCtx.Done():
		return nil, fmt.Errorf("timeout waiting for transaction completion")
	}
}

// ListTransactions lists transactions based on criteria
func (api *TransactionAPI) ListTransactions(
	ctx context.Context,
	req *TransactionListRequest,
) ([]*TransactionResponse, error) {
	// This would typically query a persistent store
	// For now, return active transactions from memory

	api.coordinator.transactionsMu.RLock()
	defer api.coordinator.transactionsMu.RUnlock()

	var results []*TransactionResponse
	count := 0
	limit := req.Limit
	if limit <= 0 {
		limit = 100 // Default limit
	}

	for _, txnCtx := range api.coordinator.transactions {
		// Apply filters
		if req.UserID != nil && txnCtx.UserID != *req.UserID {
			continue
		}
		if req.Type != nil && txnCtx.Type != *req.Type {
			continue
		}
		if req.State != nil && TransactionState(atomic.LoadInt64(&txnCtx.state)) != *req.State {
			continue
		}
		if req.FromTime != nil && txnCtx.CreatedAt.Before(*req.FromTime) {
			continue
		}
		if req.ToTime != nil && txnCtx.CreatedAt.After(*req.ToTime) {
			continue
		}

		// Apply offset
		if count < req.Offset {
			count++
			continue
		}

		// Add to results
		results = append(results, api.buildTransactionResponse(txnCtx))
		count++

		// Apply limit
		if len(results) >= limit {
			break
		}
	}

	return results, nil
}

// GetMetrics returns coordinator metrics
func (api *TransactionAPI) GetMetrics(ctx context.Context) (*CoordinatorMetrics, error) {
	return api.coordinator.GetMetrics(), nil
}

// RetryTransaction retries a failed transaction
func (api *TransactionAPI) RetryTransaction(
	ctx context.Context,
	transactionID uuid.UUID,
) (*TransactionResponse, error) {
	// Check if transaction is in dead letter queue
	entry, err := api.deadLetterQueue.GetDeadLetterEntry(transactionID)
	if err == nil {
		// Retry from dead letter queue
		if err := api.deadLetterQueue.RetryDeadLetter(transactionID); err != nil {
			return nil, fmt.Errorf("failed to retry dead lettered transaction: %w", err)
		}
		return api.buildTransactionResponse(entry.TransactionContext), nil
	}

	// Check if transaction exists in active transactions
	txnCtx, err := api.coordinator.GetTransaction(transactionID)
	if err != nil {
		return nil, fmt.Errorf("transaction not found: %w", err)
	}

	// Schedule retry
	if err := api.retryManager.ScheduleRetry(ctx, txnCtx, fmt.Errorf("manual retry requested")); err != nil {
		return nil, fmt.Errorf("failed to schedule retry: %w", err)
	}

	return api.buildTransactionResponse(txnCtx), nil
}

// GetDeadLetters returns dead lettered transactions
func (api *TransactionAPI) GetDeadLetters(ctx context.Context) ([]*DeadLetterEntry, error) {
	return api.deadLetterQueue.ListDeadLetterEntries(), nil
}

// Helper methods

func (api *TransactionAPI) validateTransactionRequest(req *TransactionRequest) error {
	if req.UserID == uuid.Nil {
		return fmt.Errorf("user ID is required")
	}

	switch req.Type {
	case TransactionTypeTrade:
		if req.Order == nil {
			return fmt.Errorf("order is required for trade transactions")
		}
	case TransactionTypeWithdrawal:
		if req.Order == nil {
			return fmt.Errorf("order is required for withdrawal transactions")
		}
	case TransactionTypeSettlement:
		if req.Trade == nil {
			return fmt.Errorf("trade is required for settlement transactions")
		}
	case TransactionTypeTransfer:
		if req.Order == nil {
			return fmt.Errorf("order is required for transfer transactions")
		}
	default:
		return fmt.Errorf("unknown transaction type: %s", req.Type)
	}

	return nil
}

func (api *TransactionAPI) registerResourcesForTransaction(txnCtx *CrossPairTransactionContext) error {
	// Register balance service resource
	balanceResource := NewBalanceResource(api.logger, api.coordinator.balanceService)
	txnCtx.Resources = append(txnCtx.Resources, balanceResource)

	// Register matching engine resources based on transaction type
	if txnCtx.Type == TransactionTypeTrade && txnCtx.Order != nil {
		// Register matching engines for both sides of the trade
		if txnCtx.Route != nil {
			for _, leg := range txnCtx.Route.Legs {
				if engine, exists := api.coordinator.matchingEngines[leg.Symbol]; exists {
					engineResource := NewMatchingEngineResource(api.logger, engine, leg.Symbol)
					txnCtx.Resources = append(txnCtx.Resources, engineResource)
				}
			}
		}
	}

	// Register trade store resource
	if txnCtx.Type == TransactionTypeTrade || txnCtx.Type == TransactionTypeSettlement {
		tradeStoreResource := NewTradeStoreResource(api.logger, api.coordinator.tradeStore)
		txnCtx.Resources = append(txnCtx.Resources, tradeStoreResource)
	}

	// Initialize resource states
	for _, resource := range txnCtx.Resources {
		txnCtx.resourceStates[resource.GetResourceName()] = ResourceState{
			Name:  resource.GetResourceName(),
			State: transaction.XAResourceStateIdle,
		}
	}

	return nil
}

func (api *TransactionAPI) buildTransactionResponse(txnCtx *CrossPairTransactionContext) *TransactionResponse {
	state := TransactionState(atomic.LoadInt64(&txnCtx.state))

	resp := &TransactionResponse{
		TransactionID: txnCtx.ID,
		State:         state.String(),
		CreatedAt:     txnCtx.CreatedAt,
		TraceID:       txnCtx.TraceID,
		Metrics:       txnCtx.Metrics,
	}

	// Add completion time if transaction is finished
	if state == StateCommitted || state == StateAborted || state == StateCompensated {
		if txnCtx.Metrics != nil && txnCtx.Metrics.CompletedAt != nil {
			resp.CompletedAt = txnCtx.Metrics.CompletedAt
		}
	}

	// Add result if transaction completed successfully
	if state == StateCommitted && txnCtx.Trade != nil {
		resp.Result = &TransactionResult{
			Trade:         txnCtx.Trade,
			ExecutionTime: txnCtx.Metrics.TotalTime,
		}

		// Add calculated values if available
		if txnCtx.Route != nil {
			resp.Result.ActualRate = &txnCtx.Route.SyntheticRate
		}
	}

	// Add error information for failed transactions
	if state == StateFailed || state == StateAborted || state == StateDeadLettered {
		if txnCtx.DeadLetterInfo != nil {
			resp.Error = txnCtx.DeadLetterInfo.Reason
		}
	}

	return resp
}

// ExecuteTradeTransaction is a convenience method for executing a complete trade transaction
func (api *TransactionAPI) ExecuteTradeTransaction(
	ctx context.Context,
	order *crosspair.CrossPairOrder,
	route *crosspair.CrossPairRoute,
	timeout time.Duration,
) (*TransactionResponse, error) {
	// Begin transaction
	req := &TransactionRequest{
		Type:    TransactionTypeTrade,
		UserID:  order.UserID,
		Order:   order,
		Route:   route,
		Timeout: &timeout,
	}

	resp, err := api.BeginTransaction(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to begin trade transaction: %w", err)
	}

	// Wait for completion
	return api.WaitForTransaction(ctx, resp.TransactionID, timeout)
}
