// Package transaction provides XA resource implementations for bookkeeper service
package transaction

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/bookkeeper"
	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// BookkeeperXAResource implements XA interface for bookkeeper operations
type BookkeeperXAResource struct {
	bookkeeper bookkeeper.BookkeeperService
	db         *gorm.DB
	logger     *zap.Logger

	// Transaction state tracking
	pendingTransactions map[string]*BookkeeperTransaction
	mu                  sync.RWMutex
}

// BookkeeperTransaction represents a bookkeeper transaction in XA context
type BookkeeperTransaction struct {
	XID              XID
	Operations       []BookkeeperOperation
	CompensationData map[string]interface{}
	DBTransaction    *gorm.DB
	State            string
	CreatedAt        time.Time
}

// BookkeeperOperation represents an operation within a bookkeeper transaction
type BookkeeperOperation struct {
	Type        string // "lock_funds", "unlock_funds", "transfer", "create_transaction"
	UserID      string
	Currency    string
	Amount      float64
	FromUserID  string
	ToUserID    string
	Reference   string
	Description string
	Result      interface{}
	Error       error
}

// NewBookkeeperXAResource creates a new XA resource for bookkeeper operations
func NewBookkeeperXAResource(bookkeeper bookkeeper.BookkeeperService, db *gorm.DB, logger *zap.Logger) *BookkeeperXAResource {
	return &BookkeeperXAResource{
		bookkeeper:          bookkeeper,
		db:                  db,
		logger:              logger,
		pendingTransactions: make(map[string]*BookkeeperTransaction),
	}
}

// GetResourceName returns the name of this resource
func (bxa *BookkeeperXAResource) GetResourceName() string {
	return "bookkeeper"
}

// Start begins a new bookkeeper transaction
func (bxa *BookkeeperXAResource) Start(xid XID) error {
	bxa.mu.Lock()
	defer bxa.mu.Unlock()

	xidStr := xid.String()
	if _, exists := bxa.pendingTransactions[xidStr]; exists {
		return fmt.Errorf("transaction already exists: %s", xidStr)
	}

	// Begin database transaction
	dbTx := bxa.db.Begin()
	if dbTx.Error != nil {
		return fmt.Errorf("failed to begin database transaction: %w", dbTx.Error)
	}

	bxa.pendingTransactions[xidStr] = &BookkeeperTransaction{
		XID:              xid,
		Operations:       make([]BookkeeperOperation, 0),
		CompensationData: make(map[string]interface{}),
		DBTransaction:    dbTx,
		State:            "active",
		CreatedAt:        time.Now(),
	}

	bxa.logger.Info("Started bookkeeper XA transaction",
		zap.String("xid", xidStr))

	return nil
}

// LockFunds locks funds within the XA transaction
func (bxa *BookkeeperXAResource) LockFunds(ctx context.Context, xid XID, userID, currency string, amount float64) error {
	bxa.mu.Lock()
	defer bxa.mu.Unlock()

	xidStr := xid.String()
	txn, exists := bxa.pendingTransactions[xidStr]
	if !exists {
		return fmt.Errorf("transaction not found: %s", xidStr)
	}

	// Get current account state for compensation
	var account models.Account
	if err := txn.DBTransaction.Where("user_id = ? AND currency = ?", userID, currency).First(&account).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("account not found")
		}
		return fmt.Errorf("failed to find account: %w", err)
	}

	// Store compensation data
	compensationKey := fmt.Sprintf("lock_funds_%s_%s", userID, currency)
	txn.CompensationData[compensationKey] = map[string]interface{}{
		"original_available": account.Available,
		"original_locked":    account.Locked,
		"amount":             amount,
	}

	// Check if account has enough available funds
	if account.Available < amount {
		return fmt.Errorf("insufficient funds: available %.8f, required %.8f", account.Available, amount)
	}

	// Update account within transaction
	account.Available -= amount
	account.Locked += amount
	account.UpdatedAt = time.Now()

	if err := txn.DBTransaction.Save(&account).Error; err != nil {
		return fmt.Errorf("failed to lock funds: %w", err)
	}

	// Record operation
	op := BookkeeperOperation{
		Type:        "lock_funds",
		UserID:      userID,
		Currency:    currency,
		Amount:      amount,
		Reference:   fmt.Sprintf("xa_lock_%s", xidStr),
		Description: fmt.Sprintf("XA transaction fund lock for %s", xidStr),
	}
	txn.Operations = append(txn.Operations, op)

	bxa.logger.Debug("Locked funds in XA transaction",
		zap.String("xid", xidStr),
		zap.String("user_id", userID),
		zap.String("currency", currency),
		zap.Float64("amount", amount))

	return nil
}

// UnlockFunds unlocks funds within the XA transaction
func (bxa *BookkeeperXAResource) UnlockFunds(ctx context.Context, xid XID, userID, currency string, amount float64) error {
	bxa.mu.Lock()
	defer bxa.mu.Unlock()

	xidStr := xid.String()
	txn, exists := bxa.pendingTransactions[xidStr]
	if !exists {
		return fmt.Errorf("transaction not found: %s", xidStr)
	}

	// Get current account state
	var account models.Account
	if err := txn.DBTransaction.Where("user_id = ? AND currency = ?", userID, currency).First(&account).Error; err != nil {
		return fmt.Errorf("failed to find account: %w", err)
	}

	// Store compensation data
	compensationKey := fmt.Sprintf("unlock_funds_%s_%s", userID, currency)
	txn.CompensationData[compensationKey] = map[string]interface{}{
		"original_available": account.Available,
		"original_locked":    account.Locked,
		"amount":             amount,
	}

	// Check if account has enough locked funds
	if account.Locked < amount {
		return fmt.Errorf("insufficient locked funds: locked %.8f, required %.8f", account.Locked, amount)
	}

	// Update account within transaction
	account.Available += amount
	account.Locked -= amount
	account.UpdatedAt = time.Now()

	if err := txn.DBTransaction.Save(&account).Error; err != nil {
		return fmt.Errorf("failed to unlock funds: %w", err)
	}

	// Record operation
	op := BookkeeperOperation{
		Type:        "unlock_funds",
		UserID:      userID,
		Currency:    currency,
		Amount:      amount,
		Reference:   fmt.Sprintf("xa_unlock_%s", xidStr),
		Description: fmt.Sprintf("XA transaction fund unlock for %s", xidStr),
	}
	txn.Operations = append(txn.Operations, op)

	bxa.logger.Debug("Unlocked funds in XA transaction",
		zap.String("xid", xidStr),
		zap.String("user_id", userID),
		zap.String("currency", currency),
		zap.Float64("amount", amount))

	return nil
}

// TransferFunds transfers funds between accounts within the XA transaction
func (bxa *BookkeeperXAResource) TransferFunds(ctx context.Context, xid XID, fromUserID, toUserID, currency string, amount float64, description string) error {
	bxa.mu.Lock()
	defer bxa.mu.Unlock()

	xidStr := xid.String()
	txn, exists := bxa.pendingTransactions[xidStr]
	if !exists {
		return fmt.Errorf("transaction not found: %s", xidStr)
	}

	// Get both accounts
	var fromAccount, toAccount models.Account

	if err := txn.DBTransaction.Where("user_id = ? AND currency = ?", fromUserID, currency).First(&fromAccount).Error; err != nil {
		return fmt.Errorf("failed to find from account: %w", err)
	}

	if err := txn.DBTransaction.Where("user_id = ? AND currency = ?", toUserID, currency).First(&toAccount).Error; err != nil {
		return fmt.Errorf("failed to find to account: %w", err)
	}

	// Store compensation data
	compensationKey := fmt.Sprintf("transfer_%s_%s_%s", fromUserID, toUserID, currency)
	txn.CompensationData[compensationKey] = map[string]interface{}{
		"from_original_balance":   fromAccount.Balance,
		"from_original_available": fromAccount.Available,
		"to_original_balance":     toAccount.Balance,
		"to_original_available":   toAccount.Available,
		"amount":                  amount,
	}

	// Check if from account has enough available funds
	if fromAccount.Available < amount {
		return fmt.Errorf("insufficient funds in from account: available %.8f, required %.8f", fromAccount.Available, amount)
	}

	// Update accounts
	fromAccount.Balance -= amount
	fromAccount.Available -= amount
	fromAccount.UpdatedAt = time.Now()

	toAccount.Balance += amount
	toAccount.Available += amount
	toAccount.UpdatedAt = time.Now()

	// Save accounts within transaction
	if err := txn.DBTransaction.Save(&fromAccount).Error; err != nil {
		return fmt.Errorf("failed to update from account: %w", err)
	}

	if err := txn.DBTransaction.Save(&toAccount).Error; err != nil {
		return fmt.Errorf("failed to update to account: %w", err)
	}

	// Create transaction records
	reference := fmt.Sprintf("xa_transfer_%s", xidStr)
	now := time.Now()

	fromTxn := &models.Transaction{
		ID:          uuid.New(),
		UserID:      uuid.MustParse(fromUserID),
		Type:        "transfer_out",
		Amount:      -amount,
		Currency:    currency,
		Status:      "pending",
		Reference:   reference,
		Description: description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	toTxn := &models.Transaction{
		ID:          uuid.New(),
		UserID:      uuid.MustParse(toUserID),
		Type:        "transfer_in",
		Amount:      amount,
		Currency:    currency,
		Status:      "pending",
		Reference:   reference,
		Description: description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := txn.DBTransaction.Create(fromTxn).Error; err != nil {
		return fmt.Errorf("failed to create from transaction: %w", err)
	}

	if err := txn.DBTransaction.Create(toTxn).Error; err != nil {
		return fmt.Errorf("failed to create to transaction: %w", err)
	}

	// Record operation
	op := BookkeeperOperation{
		Type:        "transfer",
		UserID:      fromUserID,
		FromUserID:  fromUserID,
		ToUserID:    toUserID,
		Currency:    currency,
		Amount:      amount,
		Reference:   reference,
		Description: description,
	}
	txn.Operations = append(txn.Operations, op)

	bxa.logger.Debug("Transferred funds in XA transaction",
		zap.String("xid", xidStr),
		zap.String("from_user", fromUserID),
		zap.String("to_user", toUserID),
		zap.String("currency", currency),
		zap.Float64("amount", amount))

	return nil
}

// Prepare implements XA prepare phase
func (bxa *BookkeeperXAResource) Prepare(ctx context.Context, xid XID) (bool, error) {
	bxa.mu.Lock()
	defer bxa.mu.Unlock()

	xidStr := xid.String()
	txn, exists := bxa.pendingTransactions[xidStr]
	if !exists {
		return false, fmt.Errorf("transaction not found: %s", xidStr)
	}

	// Check if transaction is in valid state
	if txn.State != "active" {
		return false, fmt.Errorf("transaction not in active state: %s", txn.State)
	}

	// Validate all operations can be committed
	// This is where we would check constraints, locks, etc.
	for _, op := range txn.Operations {
		if err := bxa.validateOperation(txn, op); err != nil {
			bxa.logger.Error("Operation validation failed during prepare",
				zap.String("xid", xidStr),
				zap.String("operation", op.Type),
				zap.Error(err))
			return false, err
		}
	}

	// Mark as prepared
	txn.State = "prepared"

	bxa.logger.Info("Bookkeeper XA transaction prepared",
		zap.String("xid", xidStr),
		zap.Int("operations", len(txn.Operations)))

	return true, nil
}

// Commit implements XA commit phase
func (bxa *BookkeeperXAResource) Commit(ctx context.Context, xid XID, onePhase bool) error {
	bxa.mu.Lock()
	defer bxa.mu.Unlock()

	xidStr := xid.String()
	txn, exists := bxa.pendingTransactions[xidStr]
	if !exists {
		return fmt.Errorf("transaction not found: %s", xidStr)
	}

	// For one-phase commit, prepare if not already done
	if onePhase && txn.State == "active" {
		if prepared, err := bxa.Prepare(ctx, xid); !prepared || err != nil {
			if err := bxa.rollback(ctx, xid); err != nil {
				bxa.logger.Error("Failed to rollback after failed one-phase prepare",
					zap.String("xid", xidStr),
					zap.Error(err))
			}
			return fmt.Errorf("one-phase prepare failed: %w", err)
		}
	}

	// Commit the database transaction
	if err := txn.DBTransaction.Commit().Error; err != nil {
		bxa.logger.Error("Failed to commit database transaction",
			zap.String("xid", xidStr),
			zap.Error(err))
		return fmt.Errorf("database commit failed: %w", err)
	}

	// Update transaction records to completed
	for _, op := range txn.Operations {
		if op.Type == "transfer" {
			// Update transaction records to completed
			bxa.db.Model(&models.Transaction{}).
				Where("reference = ? AND status = ?", op.Reference, "pending").
				Update("status", "completed")
		}
	}

	txn.State = "committed"

	bxa.logger.Info("Bookkeeper XA transaction committed",
		zap.String("xid", xidStr),
		zap.Int("operations", len(txn.Operations)))

	// Clean up
	delete(bxa.pendingTransactions, xidStr)

	return nil
}

// Rollback implements XA rollback phase
func (bxa *BookkeeperXAResource) Rollback(ctx context.Context, xid XID) error {
	bxa.mu.Lock()
	defer bxa.mu.Unlock()

	return bxa.rollback(ctx, xid)
}

// rollback implements the actual rollback logic
func (bxa *BookkeeperXAResource) rollback(ctx context.Context, xid XID) error {
	xidStr := xid.String()
	txn, exists := bxa.pendingTransactions[xidStr]
	if !exists {
		// Transaction may have already been cleaned up
		return nil
	}

	// Rollback the database transaction
	if err := txn.DBTransaction.Rollback().Error; err != nil {
		bxa.logger.Error("Failed to rollback database transaction",
			zap.String("xid", xidStr),
			zap.Error(err))
	}

	txn.State = "aborted"

	bxa.logger.Info("Bookkeeper XA transaction rolled back",
		zap.String("xid", xidStr),
		zap.Int("operations", len(txn.Operations)))

	// Clean up
	delete(bxa.pendingTransactions, xidStr)

	return nil
}

// Forget implements XA forget phase (for heuristic completions)
func (bxa *BookkeeperXAResource) Forget(ctx context.Context, xid XID) error {
	bxa.mu.Lock()
	defer bxa.mu.Unlock()

	xidStr := xid.String()
	delete(bxa.pendingTransactions, xidStr)

	bxa.logger.Info("Bookkeeper XA transaction forgotten",
		zap.String("xid", xidStr))

	return nil
}

// Recover implements XA recovery phase
func (bxa *BookkeeperXAResource) Recover(ctx context.Context, flags int) ([]XID, error) {
	bxa.mu.RLock()
	defer bxa.mu.RUnlock()

	var xids []XID
	for _, txn := range bxa.pendingTransactions {
		if txn.State == "prepared" {
			xids = append(xids, txn.XID)
		}
	}

	bxa.logger.Info("Bookkeeper XA recovery found prepared transactions",
		zap.Int("count", len(xids)))

	return xids, nil
}

// validateOperation validates that an operation can be committed
func (bxa *BookkeeperXAResource) validateOperation(txn *BookkeeperTransaction, op BookkeeperOperation) error {
	// Add validation logic here
	// For example, check that accounts still exist, amounts are valid, etc.
	return nil
}

// GetPendingTransactionCount returns the number of pending transactions
func (bxa *BookkeeperXAResource) GetPendingTransactionCount() int {
	bxa.mu.RLock()
	defer bxa.mu.RUnlock()

	return len(bxa.pendingTransactions)
}

// GetTransactionState returns the state of a specific transaction
func (bxa *BookkeeperXAResource) GetTransactionState(xid XID) (string, bool) {
	bxa.mu.RLock()
	defer bxa.mu.RUnlock()

	txn, exists := bxa.pendingTransactions[xid.String()]
	if !exists {
		return "", false
	}

	return txn.State, true
}
