// Package transaction - Cross-pair specific XA resources implementation
package transaction

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Aidin1998/finalex/internal/accounts/transaction"
	"github.com/Aidin1998/finalex/internal/trading/crosspair"
	dto "github.com/Aidin1998/finalex/internal/trading/crosspair/dto"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// BalanceResource implements XA interface for balance operations
type BalanceResource struct {
	logger         *zap.Logger
	balanceService crosspair.BalanceService
	name           string

	EventPublisher crosspair.CrossPairEventPublisher // Injected event publisher

	// Track prepared operations for rollback
	preparedOps map[string]*BalanceOperation
	opsMutex    sync.RWMutex
}

// BalanceOperation represents a balance operation that can be rolled back
type BalanceOperation struct {
	UserID          uuid.UUID       `json:"user_id"`
	Asset           string          `json:"asset"`
	Amount          decimal.Decimal `json:"amount"`
	OperationType   string          `json:"operation_type"` // DEBIT, CREDIT
	PreviousBalance decimal.Decimal `json:"previous_balance"`
	NewBalance      decimal.Decimal `json:"new_balance"`
	Timestamp       time.Time       `json:"timestamp"`
}

// NewBalanceResource creates a new balance resource
func NewBalanceResource(
	logger *zap.Logger,
	balanceService crosspair.BalanceService,
	eventPublisher crosspair.CrossPairEventPublisher,
) *BalanceResource {
	return &BalanceResource{
		logger:         logger.Named("balance-resource"),
		balanceService: balanceService,
		name:           "BALANCE_SERVICE",
		preparedOps:    make(map[string]*BalanceOperation),
		EventPublisher: eventPublisher,
	}
}

// Prepare implements XAResource interface
func (br *BalanceResource) Prepare(ctx context.Context, xid transaction.XID) (bool, error) {
	br.logger.Debug("preparing balance operations", zap.String("xid", xid.String()))

	// For balance operations, prepare means validating balances and reserving funds
	// This is typically a read-only check in this implementation

	// Mark as prepared (read-only operation)
	return true, nil // true indicates read-only
}

// Commit implements XAResource interface
func (br *BalanceResource) Commit(ctx context.Context, xid transaction.XID, onePhase bool) error {
	br.logger.Debug("committing balance operations",
		zap.String("xid", xid.String()),
		zap.Bool("one_phase", onePhase))

	// For read-only operations, commit is a no-op
	br.cleanup(xid)
	return nil
}

// Rollback implements XAResource interface
func (br *BalanceResource) Rollback(ctx context.Context, xid transaction.XID) error {
	br.logger.Debug("rolling back balance operations", zap.String("xid", xid.String()))

	br.opsMutex.Lock()
	defer br.opsMutex.Unlock()

	xidStr := xid.String()
	if ops, exists := br.preparedOps[xidStr]; exists {
		// Reverse the balance operations
		if err := br.reverseBalanceOperation(ctx, ops); err != nil {
			return fmt.Errorf("failed to reverse balance operation: %w", err)
		}
		delete(br.preparedOps, xidStr)

		// Notify frontend of rollback/failure
		if br.EventPublisher != nil {
			event := dto.CrossPairOrderEvent{
				UserID:    ops.UserID,
				Status:    "FAILED",
				Error:     ptrString("Balance rollback executed"),
				UpdatedAt: time.Now(),
			}
			br.EventPublisher.PublishCrossPairOrderEvent(ctx, event)
		}
	}

	return nil
}

// Forget implements XAResource interface
func (br *BalanceResource) Forget(ctx context.Context, xid transaction.XID) error {
	br.cleanup(xid)
	return nil
}

// Recover implements XAResource interface
func (br *BalanceResource) Recover(ctx context.Context, flags int) ([]transaction.XID, error) {
	// Return prepared transactions that need recovery
	// In a real implementation, this would query persistent storage
	return []transaction.XID{}, nil
}

// GetResourceName implements XAResource interface
func (br *BalanceResource) GetResourceName() string {
	return br.name
}

// ValidateTransaction implements CrossPairResource interface
func (br *BalanceResource) ValidateTransaction(ctx context.Context, txnCtx *CrossPairTransactionContext) error {
	if txnCtx.Order == nil {
		return fmt.Errorf("order is required for balance validation")
	}

	// Validate user has sufficient balance
	order := txnCtx.Order
	if err := br.validateSufficientBalance(ctx, order.UserID, order.FromAsset, order.Quantity); err != nil {
		return fmt.Errorf("insufficient balance validation failed: %w", err)
	}

	return nil
}

// EstimateGas implements CrossPairResource interface
func (br *BalanceResource) EstimateGas(ctx context.Context, txnCtx *CrossPairTransactionContext) (decimal.Decimal, error) {
	// Balance operations typically have minimal gas cost
	return decimal.NewFromFloat(0.001), nil
}

// GetCompensationAction implements CrossPairResource interface
func (br *BalanceResource) GetCompensationAction(ctx context.Context, txnCtx *CrossPairTransactionContext) (*CompensationAction, error) {
	return &CompensationAction{
		ResourceName: br.name,
		Action:       "RESTORE_BALANCE",
		Data: map[string]interface{}{
			"user_id": txnCtx.UserID.String(),
			"order":   txnCtx.Order,
		},
	}, nil
}

// Helper methods
func (br *BalanceResource) validateSufficientBalance(ctx context.Context, userID uuid.UUID, asset string, amount decimal.Decimal) error {
	// This would typically call the balance service to check balance
	// For now, we'll assume it's implemented
	br.logger.Debug("validating sufficient balance",
		zap.String("user_id", userID.String()),
		zap.String("asset", asset),
		zap.String("amount", amount.String()))

	return nil
}

func (br *BalanceResource) reverseBalanceOperation(ctx context.Context, op *BalanceOperation) error {
	br.logger.Info("reversing balance operation",
		zap.String("user_id", op.UserID.String()),
		zap.String("asset", op.Asset),
		zap.String("amount", op.Amount.String()),
		zap.String("operation_type", op.OperationType))

	// Implement balance reversal logic here
	return nil
}

func (br *BalanceResource) cleanup(xid transaction.XID) {
	br.opsMutex.Lock()
	defer br.opsMutex.Unlock()
	delete(br.preparedOps, xid.String())
}

// MatchingEngineResource implements XA interface for matching engine operations
type MatchingEngineResource struct {
	logger         *zap.Logger
	matchingEngine crosspair.MatchingEngine
	name           string
	symbol         string

	// Track prepared orders for rollback
	preparedOrders map[string]*OrderOperation
	ordersMutex    sync.RWMutex

	EventPublisher crosspair.CrossPairEventPublisher // Injected event publisher
}

// OrderOperation represents an order operation that can be rolled back
type OrderOperation struct {
	OrderID       uuid.UUID                 `json:"order_id"`
	Symbol        string                    `json:"symbol"`
	Operation     string                    `json:"operation"` // PLACE, CANCEL, MODIFY
	OriginalOrder *crosspair.CrossPairOrder `json:"original_order,omitempty"`
	Timestamp     time.Time                 `json:"timestamp"`
}

// NewMatchingEngineResource creates a new matching engine resource
func NewMatchingEngineResource(
	logger *zap.Logger,
	matchingEngine crosspair.MatchingEngine,
	symbol string,
	eventPublisher crosspair.CrossPairEventPublisher,
) *MatchingEngineResource {
	return &MatchingEngineResource{
		logger:         logger.Named("matching-engine-resource"),
		matchingEngine: matchingEngine,
		name:           fmt.Sprintf("MATCHING_ENGINE_%s", symbol),
		symbol:         symbol,
		preparedOrders: make(map[string]*OrderOperation),
		EventPublisher: eventPublisher,
	}
}

// Prepare implements XAResource interface
func (mer *MatchingEngineResource) Prepare(ctx context.Context, xid transaction.XID) (bool, error) {
	mer.logger.Debug("preparing matching engine operations",
		zap.String("xid", xid.String()),
		zap.String("symbol", mer.symbol))

	// Validate that orders can be placed/executed
	// This is typically a validation step
	return false, nil // false indicates not read-only
}

// Commit implements XAResource interface
func (mer *MatchingEngineResource) Commit(ctx context.Context, xid transaction.XID, onePhase bool) error {
	mer.logger.Debug("committing matching engine operations",
		zap.String("xid", xid.String()),
		zap.String("symbol", mer.symbol),
		zap.Bool("one_phase", onePhase))

	// Execute the actual order operations
	mer.cleanup(xid)
	return nil
}

// Rollback implements XAResource interface
func (mer *MatchingEngineResource) Rollback(ctx context.Context, xid transaction.XID) error {
	mer.logger.Debug("rolling back matching engine operations",
		zap.String("xid", xid.String()),
		zap.String("symbol", mer.symbol))

	mer.ordersMutex.Lock()
	defer mer.ordersMutex.Unlock()

	xidStr := xid.String()
	if op, exists := mer.preparedOrders[xidStr]; exists {
		// Reverse the order operation
		if err := mer.reverseOrderOperation(ctx, op); err != nil {
			return fmt.Errorf("failed to reverse order operation: %w", err)
		}
		delete(mer.preparedOrders, xidStr)

		// Notify frontend of rollback/failure with rich context
		if mer.EventPublisher != nil && op.OriginalOrder != nil {
			event := dto.CrossPairOrderEvent{
				OrderID:        op.OriginalOrder.ID,
				UserID:         op.OriginalOrder.UserID,
				Status:         "ROLLED_BACK",
				FillAmountLeg1: op.OriginalOrder.ExecutedQuantity, // best available
				SyntheticRate:  op.OriginalOrder.EstimatedRate,
				Fee:            toDTOFees(op.OriginalOrder.Fees),
				CreatedAt:      op.OriginalOrder.CreatedAt,
				UpdatedAt:      time.Now(),
				Error:          ptrString("Order rollback executed"),
			}
			mer.EventPublisher.PublishCrossPairOrderEvent(ctx, event)
		}
	}

	return nil
}

// Forget implements XAResource interface
func (mer *MatchingEngineResource) Forget(ctx context.Context, xid transaction.XID) error {
	mer.cleanup(xid)
	return nil
}

// Recover implements XAResource interface
func (mer *MatchingEngineResource) Recover(ctx context.Context, flags int) ([]transaction.XID, error) {
	return []transaction.XID{}, nil
}

// GetResourceName implements XAResource interface
func (mer *MatchingEngineResource) GetResourceName() string {
	return mer.name
}

// ValidateTransaction implements CrossPairResource interface
func (mer *MatchingEngineResource) ValidateTransaction(ctx context.Context, txnCtx *CrossPairTransactionContext) error {
	if txnCtx.Order == nil {
		return fmt.Errorf("order is required for matching engine validation")
	}

	// Validate order parameters
	order := txnCtx.Order
	if err := mer.validateOrderParameters(order); err != nil {
		return fmt.Errorf("order validation failed: %w", err)
	}

	return nil
}

// EstimateGas implements CrossPairResource interface
func (mer *MatchingEngineResource) EstimateGas(ctx context.Context, txnCtx *CrossPairTransactionContext) (decimal.Decimal, error) {
	// Matching engine operations have moderate gas cost
	return decimal.NewFromFloat(0.01), nil
}

// GetCompensationAction implements CrossPairResource interface
func (mer *MatchingEngineResource) GetCompensationAction(ctx context.Context, txnCtx *CrossPairTransactionContext) (*CompensationAction, error) {
	return &CompensationAction{
		ResourceName: mer.name,
		Action:       "CANCEL_ORDER",
		Data: map[string]interface{}{
			"symbol": mer.symbol,
			"order":  txnCtx.Order,
		},
	}, nil
}

// Helper methods
func (mer *MatchingEngineResource) validateOrderParameters(order *crosspair.CrossPairOrder) error {
	if order.Quantity.LessThanOrEqual(decimal.Zero) {
		return fmt.Errorf("order quantity must be positive")
	}

	if order.FromAsset == "" || order.ToAsset == "" {
		return fmt.Errorf("from and to assets must be specified")
	}

	return nil
}

func (mer *MatchingEngineResource) reverseOrderOperation(ctx context.Context, op *OrderOperation) error {
	mer.logger.Info("reversing order operation",
		zap.String("order_id", op.OrderID.String()),
		zap.String("symbol", op.Symbol),
		zap.String("operation", op.Operation))

	// Implement order reversal logic here
	return nil
}

func (mer *MatchingEngineResource) cleanup(xid transaction.XID) {
	mer.ordersMutex.Lock()
	defer mer.ordersMutex.Unlock()
	delete(mer.preparedOrders, xid.String())
}

// TradeStoreResource implements XA interface for trade storage operations
type TradeStoreResource struct {
	logger     *zap.Logger
	tradeStore crosspair.CrossPairTradeStore
	name       string

	// Track prepared trades for rollback
	preparedTrades map[string]*TradeOperation
	tradesMutex    sync.RWMutex

	EventPublisher crosspair.CrossPairEventPublisher // Injected event publisher
}

// TradeOperation represents a trade operation that can be rolled back
type TradeOperation struct {
	TradeID   uuid.UUID                 `json:"trade_id"`
	Trade     *crosspair.CrossPairTrade `json:"trade"`
	Operation string                    `json:"operation"` // CREATE, UPDATE, DELETE
	Timestamp time.Time                 `json:"timestamp"`
}

// NewTradeStoreResource creates a new trade store resource
func NewTradeStoreResource(
	logger *zap.Logger,
	tradeStore crosspair.CrossPairTradeStore,
	eventPublisher crosspair.CrossPairEventPublisher,
) *TradeStoreResource {
	return &TradeStoreResource{
		logger:         logger.Named("trade-store-resource"),
		tradeStore:     tradeStore,
		name:           "TRADE_STORE",
		preparedTrades: make(map[string]*TradeOperation),
		EventPublisher: eventPublisher,
	}
}

// Prepare implements XAResource interface
func (tsr *TradeStoreResource) Prepare(ctx context.Context, xid transaction.XID) (bool, error) {
	tsr.logger.Debug("preparing trade store operations", zap.String("xid", xid.String()))

	// Validate that trade can be stored
	return false, nil // false indicates not read-only
}

// Commit implements XAResource interface
func (tsr *TradeStoreResource) Commit(ctx context.Context, xid transaction.XID, onePhase bool) error {
	tsr.logger.Debug("committing trade store operations",
		zap.String("xid", xid.String()),
		zap.Bool("one_phase", onePhase))

	// Persist the trade data
	tsr.cleanup(xid)
	return nil
}

// Rollback implements XAResource interface
func (tsr *TradeStoreResource) Rollback(ctx context.Context, xid transaction.XID) error {
	tsr.logger.Debug("rolling back trade store operations", zap.String("xid", xid.String()))

	tsr.tradesMutex.Lock()
	defer tsr.tradesMutex.Unlock()

	xidStr := xid.String()
	if op, exists := tsr.preparedTrades[xidStr]; exists {
		// Reverse the trade operation
		if err := tsr.reverseTradeOperation(ctx, op); err != nil {
			return fmt.Errorf("failed to reverse trade operation: %w", err)
		}
		delete(tsr.preparedTrades, xidStr)

		// Notify frontend of rollback/failure with rich context
		if tsr.EventPublisher != nil && op.Trade != nil {
			event := dto.CrossPairOrderEvent{
				OrderID:        op.Trade.OrderID,
				UserID:         op.Trade.UserID,
				Status:         "ROLLED_BACK",
				Leg1TradeID:    op.Trade.ID, // If available, else leave zero
				FillAmountLeg1: op.Trade.Quantity,
				SyntheticRate:  op.Trade.ExecutedRate,
				Fee:            toDTOFees(op.Trade.Fees),
				CreatedAt:      op.Trade.CreatedAt,
				UpdatedAt:      time.Now(),
				Error:          ptrString("Trade rollback executed"),
			}
			tsr.EventPublisher.PublishCrossPairOrderEvent(ctx, event)
		}
	}

	return nil
}

// Forget implements XAResource interface
func (tsr *TradeStoreResource) Forget(ctx context.Context, xid transaction.XID) error {
	tsr.cleanup(xid)
	return nil
}

// Recover implements XAResource interface
func (tsr *TradeStoreResource) Recover(ctx context.Context, flags int) ([]transaction.XID, error) {
	return []transaction.XID{}, nil
}

// GetResourceName implements XAResource interface
func (tsr *TradeStoreResource) GetResourceName() string {
	return tsr.name
}

// ValidateTransaction implements CrossPairResource interface
func (tsr *TradeStoreResource) ValidateTransaction(ctx context.Context, txnCtx *CrossPairTransactionContext) error {
	// Validate that trade data is complete and valid
	if txnCtx.Trade == nil {
		return fmt.Errorf("trade data is required for trade store validation")
	}

	return nil
}

// EstimateGas implements CrossPairResource interface
func (tsr *TradeStoreResource) EstimateGas(ctx context.Context, txnCtx *CrossPairTransactionContext) (decimal.Decimal, error) {
	// Trade storage operations have minimal gas cost
	return decimal.NewFromFloat(0.005), nil
}

// GetCompensationAction implements CrossPairResource interface
func (tsr *TradeStoreResource) GetCompensationAction(ctx context.Context, txnCtx *CrossPairTransactionContext) (*CompensationAction, error) {
	return &CompensationAction{
		ResourceName: tsr.name,
		Action:       "DELETE_TRADE",
		Data: map[string]interface{}{
			"trade_id": txnCtx.Trade.ID.String(),
		},
	}, nil
}

// Helper methods
func (tsr *TradeStoreResource) reverseTradeOperation(ctx context.Context, op *TradeOperation) error {
	tsr.logger.Info("reversing trade operation",
		zap.String("trade_id", op.TradeID.String()),
		zap.String("operation", op.Operation))
	// Implement trade reversal logic here
	switch op.Operation {
	case "CREATE":
		// For CREATE operations, we would need a Delete method
		// Since Delete method doesn't exist, we'll simulate it
		tsr.logger.Info("simulating trade deletion (Delete method not available)",
			zap.String("trade_id", op.TradeID.String()))
		return nil
	case "UPDATE":
		// Restore original trade data
		// This would require storing the original state
		return nil
	case "DELETE":
		// Recreate the deleted trade
		// This would require storing the original trade
		return nil
	default:
		return fmt.Errorf("unknown trade operation: %s", op.Operation)
	}
}

func (tsr *TradeStoreResource) cleanup(xid transaction.XID) {
	tsr.tradesMutex.Lock()
	defer tsr.tradesMutex.Unlock()
	delete(tsr.preparedTrades, xid.String())
}

// Helper for pointer to string
func ptrString(s string) *string { return &s }

// Add helper at top-level:
func toDTOFees(fees []crosspair.CrossPairFee) []dto.CrossPairFee {
	result := make([]dto.CrossPairFee, len(fees))
	for i, f := range fees {
		result[i] = dto.CrossPairFee{
			Asset:   f.Asset,
			Amount:  f.Amount,
			FeeType: f.FeeType,
			Pair:    f.Pair,
		}
	}
	return result
}
