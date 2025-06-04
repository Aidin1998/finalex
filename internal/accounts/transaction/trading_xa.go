package transaction

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/trading/model"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// TradingXAResource implements XAResource interface for warm path settlement operations only
// NOTE: Hot path trading (order placement/matching) bypasses XA for performance
type TradingXAResource struct {
	mu                 sync.RWMutex
	db                 *gorm.DB
	tradingPathManager *TradingPathManager // Reference to path manager
	logger             *zap.Logger

	// XA transaction state
	pendingTransactions map[string]*TradingXATransaction
}

// TradingXATransaction represents a trading transaction in XA context
type TradingXATransaction struct {
	XID              XID
	Operations       []TradingOperation
	CompensationData map[string]interface{}
	DBTransaction    *gorm.DB
	State            string
	CreatedAt        time.Time
	OrderSnapshots   map[uuid.UUID]*OrderSnapshot
}

// TradingOperationType represents types of trading operations
type TradingOperationType string

const (
	TradingOpPlaceOrder      TradingOperationType = "PLACE_ORDER"
	TradingOpCancelOrder     TradingOperationType = "CANCEL_ORDER"
	TradingOpMatchOrder      TradingOperationType = "MATCH_ORDER"
	TradingOpExecuteTrade    TradingOperationType = "EXECUTE_TRADE"
	TradingOpUpdateOrder     TradingOperationType = "UPDATE_ORDER"
	TradingOpCreateOrderBook TradingOperationType = "CREATE_ORDER_BOOK"
	TradingOpModifyOrder     TradingOperationType = "MODIFY_ORDER"
)

// TradingOperation represents a trading operation within an XA transaction
type TradingOperation struct {
	ID        string               `json:"id"`
	Type      TradingOperationType `json:"type"`
	Timestamp time.Time            `json:"timestamp"`
	Data      interface{}          `json:"data"`
}

// TradingCompensationData holds data needed for rollback operations
type TradingCompensationData struct {
	OperationID    string                 `json:"operation_id"`
	Type           TradingOperationType   `json:"type"`
	OriginalData   map[string]interface{} `json:"original_data"`
	CompensateData map[string]interface{} `json:"compensate_data"`
	Timestamp      time.Time              `json:"timestamp"`
}

// OrderSnapshot captures the state of an order for rollback purposes
type OrderSnapshot struct {
	Order             *model.Order       `json:"order"`
	PreviousState     string             `json:"previous_state"`
	FilledQuantity    decimal.Decimal    `json:"filled_quantity"`
	RemainingQuantity decimal.Decimal    `json:"remaining_quantity"`
	OrderBookState    *OrderBookSnapshot `json:"order_book_state"`
	Timestamp         time.Time          `json:"timestamp"`
}

// OrderBookSnapshot captures the state of an order book for rollback
type OrderBookSnapshot struct {
	Pair         string                     `json:"pair"`
	BidsSnapshot map[string][]OrderSnapshot `json:"bids_snapshot"`
	AsksSnapshot map[string][]OrderSnapshot `json:"asks_snapshot"`
	OrderCount   int                        `json:"order_count"`
	Timestamp    time.Time                  `json:"timestamp"`
}

// NewTradingXAResource creates a new XA resource for trading operations
func NewTradingXAResource(db *gorm.DB, tradingPathManager *TradingPathManager, logger *zap.Logger) *TradingXAResource {
	return &TradingXAResource{
		db:                  db,
		tradingPathManager:  tradingPathManager,
		logger:              logger,
		pendingTransactions: make(map[string]*TradingXATransaction),
	}
}

// GetResourceName returns the name of this XA resource
func (t *TradingXAResource) GetResourceName() string {
	return "TradingEngine"
}

// XAResource interface implementation

// Start begins a new XA transaction branch
func (t *TradingXAResource) Start(ctx context.Context, xid XID) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	xidStr := xid.String()
	if _, exists := t.pendingTransactions[xidStr]; exists {
		return fmt.Errorf("transaction already started: %s", xidStr)
	}

	// Begin database transaction
	dbTx := t.db.WithContext(ctx).Begin()
	if dbTx.Error != nil {
		return fmt.Errorf("failed to begin database transaction: %w", dbTx.Error)
	}

	txn := &TradingXATransaction{
		XID:              xid,
		Operations:       make([]TradingOperation, 0),
		CompensationData: make(map[string]interface{}),
		DBTransaction:    dbTx,
		State:            "active",
		CreatedAt:        time.Now(),
		OrderSnapshots:   make(map[uuid.UUID]*OrderSnapshot),
	}
	t.pendingTransactions[xidStr] = txn

	t.logger.Info("Trading XA transaction started",
		zap.String("xid", xidStr))

	return nil
}

// End ends the association of the XA resource with the transaction
func (t *TradingXAResource) End(ctx context.Context, xid XID, flags int) error {
	t.mu.RLock()
	defer t.mu.RUnlock()

	xidStr := xid.String()
	txn, exists := t.pendingTransactions[xidStr]
	if !exists {
		return fmt.Errorf("transaction not found: %s", xidStr)
	}

	txn.State = "ended"

	t.logger.Debug("Trading XA transaction ended",
		zap.String("xid", xidStr),
		zap.Int("flags", flags))
	return nil
}

// Prepare implements XA prepare phase
func (t *TradingXAResource) Prepare(ctx context.Context, xid XID) (bool, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	xidStr := xid.String()
	txn, exists := t.pendingTransactions[xidStr]
	if !exists {
		return false, fmt.Errorf("transaction not found: %s", xidStr)
	}

	// Validate all operations can be committed
	for _, op := range txn.Operations {
		if err := t.validateOperation(txn, op); err != nil {
			t.logger.Error("Operation validation failed during prepare",
				zap.String("xid", xidStr),
				zap.String("operation_id", op.ID),
				zap.String("type", string(op.Type)),
				zap.Error(err))
			txn.State = "rollback_only"
			return false, fmt.Errorf("operation validation failed: %w", err)
		}
	}

	txn.State = "prepared"
	t.logger.Info("Trading XA transaction prepared",
		zap.String("xid", xidStr),
		zap.Int("operations_count", len(txn.Operations)))
	return false, nil // Return false since we're not read-only
}

// Commit implements XA commit phase
func (t *TradingXAResource) Commit(ctx context.Context, xid XID, onePhase bool) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	xidStr := xid.String()
	txn, exists := t.pendingTransactions[xidStr]
	if !exists {
		return fmt.Errorf("transaction not found: %s", xidStr)
	}

	if !onePhase && txn.State != "prepared" {
		return fmt.Errorf("invalid state for commit: %s", txn.State)
	}

	// Execute all operations within the existing database transaction
	for _, op := range txn.Operations {
		if err := t.executeOperation(ctx, txn.DBTransaction, op); err != nil {
			txn.DBTransaction.Rollback()
			t.logger.Error("Failed to execute trading operation during commit",
				zap.String("xid", xidStr),
				zap.String("operation_id", op.ID),
				zap.String("type", string(op.Type)),
				zap.Error(err))
			return fmt.Errorf("failed to execute operation %s: %w", op.ID, err)
		}
	}

	// Commit the database transaction
	if err := txn.DBTransaction.Commit().Error; err != nil {
		t.logger.Error("Failed to commit database transaction",
			zap.String("xid", xidStr),
			zap.Error(err))
		return fmt.Errorf("database commit failed: %w", err)
	}

	txn.State = "committed"
	t.logger.Info("Trading XA transaction committed",
		zap.String("xid", xidStr),
		zap.Bool("one_phase", onePhase),
		zap.Int("operations_executed", len(txn.Operations)))

	// Clean up
	delete(t.pendingTransactions, xidStr)
	return nil
}

// Rollback implements XA rollback phase
func (t *TradingXAResource) Rollback(ctx context.Context, xid XID) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	xidStr := xid.String()
	txn, exists := t.pendingTransactions[xidStr]
	if !exists {
		// Transaction may have already been cleaned up
		return nil
	}

	// Rollback the database transaction
	if err := txn.DBTransaction.Rollback().Error; err != nil {
		t.logger.Error("Failed to rollback database transaction",
			zap.String("xid", xidStr),
			zap.Error(err))
	}

	txn.State = "aborted"

	t.logger.Info("Trading XA transaction rolled back",
		zap.String("xid", xidStr),
		zap.Int("operations", len(txn.Operations)))
	// Clean up
	delete(t.pendingTransactions, xidStr)

	return nil
}

// Forget implements XA forget operation
func (t *TradingXAResource) Forget(ctx context.Context, xid XID) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	xidStr := xid.String()

	// Clean up any heuristic completion state
	delete(t.pendingTransactions, xidStr)

	t.logger.Info("Forgot XA transaction for trading operations",
		zap.String("xid", xidStr))
	return nil
}

// Recover implements XA recovery operation
func (t *TradingXAResource) Recover(ctx context.Context, flags int) ([]XID, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	// In a real implementation, this would scan persistent storage
	// for in-doubt transactions and return their XIDs
	var xids []XID

	t.logger.Info("Recovered XA transactions for trading operations",
		zap.Int("flags", flags),
		zap.Int("transactions_count", len(xids)))

	return xids, nil
}

// IsSameRM checks if this resource manager is the same as another
func (t *TradingXAResource) IsSameRM(other XAResource) bool {
	tradingXA, ok := other.(*TradingXAResource)
	if !ok {
		return false
	}
	return t.db == tradingXA.db && t.tradingPathManager == tradingXA.tradingPathManager
}

// Trading operations in the new Hot/Warm/Cold path architecture
//
// Trading operations are now handled through the TradingPathManager:
// - Hot Path: Direct matching engine operations (microsecond latency) - bypass XA
// - Warm Path: Async settlement with XA transactions (millisecond latency) <- Used by this XA resource
// - Cold Path: Reconciliation and error handling
//
// This XA resource specifically handles warm path operations that require
// distributed transaction coordination for settlement and consistency.

// Helper methods

func (t *TradingXAResource) validateOperation(txn *TradingXATransaction, op TradingOperation) error {
	switch op.Type {
	case TradingOpPlaceOrder:
		data := op.Data.(map[string]interface{})
		order := data["order"].(*model.Order)

		// Validate order parameters
		if order.Quantity.LessThanOrEqual(decimal.Zero) {
			return fmt.Errorf("invalid order quantity: %s", order.Quantity.String())
		}

		if order.Price.LessThan(decimal.Zero) {
			return fmt.Errorf("invalid order price: %s", order.Price.String())
		}

		if order.Pair == "" {
			return fmt.Errorf("trading pair is required")
		}

		if order.Side != model.OrderSideBuy && order.Side != model.OrderSideSell {
			return fmt.Errorf("invalid order side: %s", order.Side)
		}

	case TradingOpCancelOrder:
		data := op.Data.(map[string]interface{})
		orderID := data["order_id"].(uuid.UUID)

		if orderID == uuid.Nil {
			return fmt.Errorf("invalid order ID for cancellation")
		}

	case TradingOpExecuteTrade:
		data := op.Data.(map[string]interface{})
		trade := data["trade"].(*model.Trade)

		if trade.Quantity.LessThanOrEqual(decimal.Zero) {
			return fmt.Errorf("invalid trade quantity: %s", trade.Quantity.String())
		}

		if trade.Price.LessThanOrEqual(decimal.Zero) {
			return fmt.Errorf("invalid trade price: %s", trade.Price.String())
		}

	case TradingOpUpdateOrder:
		data := op.Data.(map[string]interface{})
		orderID := data["order_id"].(uuid.UUID)

		if orderID == uuid.Nil {
			return fmt.Errorf("invalid order ID for update")
		}
	}
	return nil
}

// Helper methods for operation execution

func (t *TradingXAResource) executeOperation(ctx context.Context, tx *gorm.DB, op TradingOperation) error {
	switch op.Type {
	case TradingOpPlaceOrder:
		return t.executePlaceOrder(ctx, tx, op)
	case TradingOpCancelOrder:
		return t.executeCancelOrder(ctx, tx, op)
	case TradingOpExecuteTrade:
		return t.executeExecuteTrade(ctx, tx, op)
	case TradingOpUpdateOrder:
		return t.executeUpdateOrder(ctx, tx, op)
	default:
		return fmt.Errorf("unknown operation type: %s", op.Type)
	}
}

func (t *TradingXAResource) executePlaceOrder(ctx context.Context, tx *gorm.DB, op TradingOperation) error {
	data := op.Data.(map[string]interface{})
	order := data["order"].(*model.Order)

	// In the hot/warm/cold path architecture, the XA resource handles warm path settlement,
	// not direct order placement. Orders are placed via hot path, and we handle settlement here.
	//
	// This operation represents settling an order that was already placed and potentially matched
	// in the hot path. We persist the final order state and any associated trades.

	// Persist order state to database within the transaction
	if err := t.persistOrderState(tx, order); err != nil {
		return fmt.Errorf("failed to persist order state: %w", err)
	}

	// If there are associated trades in the operation data, persist them
	if tradesData, exists := data["trades"]; exists {
		trades := tradesData.([]*model.Trade)
		for _, trade := range trades {
			if err := t.persistTrade(tx, trade); err != nil {
				return fmt.Errorf("failed to persist trade: %w", err)
			}
		}

		// Log execution
		t.logger.Info("Executed place order settlement operation",
			zap.String("order_id", order.ID.String()),
			zap.String("status", order.Status),
			zap.Int("trades_count", len(trades)))
	} else {
		// Log execution for order-only operation
		t.logger.Info("Executed place order settlement operation",
			zap.String("order_id", order.ID.String()),
			zap.String("status", order.Status),
			zap.Int("trades_count", 0))
	}

	return nil
}

func (t *TradingXAResource) executeCancelOrder(ctx context.Context, tx *gorm.DB, op TradingOperation) error {
	data := op.Data.(map[string]interface{})
	orderID := data["order_id"].(uuid.UUID)

	// Cancel the order through the warm path for consistency
	if err := t.updateOrderStatus(tx, orderID, model.OrderStatusCancelled); err != nil {
		return fmt.Errorf("failed to cancel order: %w", err)
	}

	t.logger.Info("Executed cancel order operation",
		zap.String("order_id", orderID.String()))

	return nil
}

func (t *TradingXAResource) executeExecuteTrade(ctx context.Context, tx *gorm.DB, op TradingOperation) error {
	data := op.Data.(map[string]interface{})
	trade := data["trade"].(*model.Trade)

	// Persist the trade
	if err := t.persistTrade(tx, trade); err != nil {
		return fmt.Errorf("failed to persist trade: %w", err)
	}

	t.logger.Info("Executed trade execution operation",
		zap.String("trade_id", trade.ID.String()),
		zap.String("pair", trade.Pair),
		zap.String("price", trade.Price.String()),
		zap.String("quantity", trade.Quantity.String()))

	return nil
}

func (t *TradingXAResource) executeUpdateOrder(ctx context.Context, tx *gorm.DB, op TradingOperation) error {
	data := op.Data.(map[string]interface{})
	orderID := data["order_id"].(uuid.UUID)
	updates := data["updates"].(map[string]interface{})

	// Update the order with the specified changes
	for field, value := range updates {
		if err := t.updateOrderField(tx, orderID, field, value); err != nil {
			return fmt.Errorf("failed to update order field %s: %w", field, err)
		}
	}

	t.logger.Info("Executed update order operation",
		zap.String("order_id", orderID.String()),
		zap.Int("fields_updated", len(updates)))

	return nil
}

// Database persistence helpers

func (t *TradingXAResource) persistOrderState(tx *gorm.DB, order *model.Order) error {
	// Convert to database model and save
	// This is a simplified implementation - you'd use your actual order repository
	return tx.Save(order).Error
}

func (t *TradingXAResource) persistTrade(tx *gorm.DB, trade *model.Trade) error {
	// Convert to database model and save
	// This is a simplified implementation - you'd use your actual trade repository
	return tx.Create(trade).Error
}

func (t *TradingXAResource) updateOrderStatus(tx *gorm.DB, orderID uuid.UUID, status string) error {
	// Update order status in database
	return tx.Model(&model.Order{}).Where("id = ?", orderID).Update("status", status).Error
}

func (t *TradingXAResource) updateOrderField(tx *gorm.DB, orderID uuid.UUID, field string, value interface{}) error {
	// Update specific order field in database
	return tx.Model(&model.Order{}).Where("id = ?", orderID).Update(field, value).Error
}

// Monitoring and diagnostics for new transaction-based architecture

// GetActiveTransactionCount returns the number of active transactions
func (t *TradingXAResource) GetActiveTransactionCount() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.pendingTransactions)
}

// GetTransactionInfo returns information about a specific transaction
func (t *TradingXAResource) GetTransactionInfo(xid XID) (*TradingXATransaction, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	xidStr := xid.String()
	txn, exists := t.pendingTransactions[xidStr]
	return txn, exists
}

// GetAllActiveTransactions returns information about all active transactions
func (t *TradingXAResource) GetAllActiveTransactions() map[string]*TradingXATransaction {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make(map[string]*TradingXATransaction)
	for xid, txn := range t.pendingTransactions {
		result[xid] = txn
	}
	return result
}
