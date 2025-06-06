// Package transaction provides XA resource implementations for fiat service
package transaction

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Aidin1998/finalex/pkg/models"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// FiatServiceInterface defines the interface that fiat service should implement
type FiatServiceInterface interface {
	// Add methods here if needed - currently the XA resource doesn't actually use the fiat service
}

// FiatXAResource implements XA interface for fiat service operations
type FiatXAResource struct {
	fiatService FiatServiceInterface
	db          *gorm.DB
	logger      *zap.Logger
	xid         string

	// Transaction state tracking
	operations      []FiatOperation
	compensationOps []FiatCompensationData
	dbTx            *gorm.DB
	prepared        bool
	committed       bool
	aborted         bool
	mu              sync.RWMutex
}

// FiatOperationType represents the type of fiat operation
type FiatOperationType string

const (
	FiatOpInitiateDeposit     FiatOperationType = "initiate_deposit"
	FiatOpCompleteDeposit     FiatOperationType = "complete_deposit"
	FiatOpInitiateWithdrawal  FiatOperationType = "initiate_withdrawal"
	FiatOpCompleteWithdrawal  FiatOperationType = "complete_withdrawal"
	FiatOpFailWithdrawal      FiatOperationType = "fail_withdrawal"
	FiatOpLockFunds           FiatOperationType = "lock_funds"
	FiatOpUnlockFunds         FiatOperationType = "unlock_funds"
	FiatOpValidateBankDetails FiatOperationType = "validate_bank_details"
	FiatOpKYCMonitoring       FiatOperationType = "kyc_monitoring"
)

// FiatOperation represents an operation within a fiat transaction
type FiatOperation struct {
	ID        string
	Type      FiatOperationType
	Timestamp time.Time
	Data      map[string]interface{}
}

// FiatCompensationData holds data needed for compensation/rollback
type FiatCompensationData struct {
	OperationID  string
	Type         FiatOperationType
	OriginalData map[string]interface{}
	Timestamp    time.Time
}

// NewFiatXAResource creates a new XA resource for fiat operations
func NewFiatXAResource(fiatService FiatServiceInterface, db *gorm.DB, logger *zap.Logger, xid string) *FiatXAResource {
	return &FiatXAResource{
		fiatService:     fiatService,
		db:              db,
		logger:          logger,
		xid:             xid,
		operations:      make([]FiatOperation, 0),
		compensationOps: make([]FiatCompensationData, 0),
	}
}

// GetResourceName returns the name of this resource
func (f *FiatXAResource) GetResourceName() string {
	return "fiat"
}

// InitiateDeposit initiates a deposit within XA transaction
func (f *FiatXAResource) InitiateDeposit(ctx context.Context, userID, currency string, amount float64, provider string) (*models.Transaction, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	opID := uuid.New().String()

	// Store compensation data
	compensationData := FiatCompensationData{
		OperationID: opID,
		Type:        FiatOpInitiateDeposit,
		OriginalData: map[string]interface{}{
			"user_id":  userID,
			"currency": currency,
			"amount":   amount,
			"provider": provider,
		},
		Timestamp: time.Now(),
	}

	// Create operation
	operation := FiatOperation{
		ID:        opID,
		Type:      FiatOpInitiateDeposit,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"user_id":  userID,
			"currency": currency,
			"amount":   amount,
			"provider": provider,
		},
	}

	f.operations = append(f.operations, operation)
	f.compensationOps = append(f.compensationOps, compensationData)

	f.logger.Info("Added deposit initiation to XA transaction",
		zap.String("xid", f.xid),
		zap.String("operation_id", opID),
		zap.String("user_id", userID),
		zap.String("currency", currency),
		zap.Float64("amount", amount))

	// Return a placeholder transaction - actual creation will happen during commit
	return &models.Transaction{
		ID:          uuid.New(),
		UserID:      uuid.MustParse(userID),
		Type:        "deposit",
		Amount:      amount,
		Currency:    currency,
		Status:      "pending",
		Reference:   provider,
		Description: fmt.Sprintf("Fiat deposit via %s", provider),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}, nil
}

// CompleteDeposit completes a deposit within XA transaction
func (f *FiatXAResource) CompleteDeposit(ctx context.Context, transactionID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	opID := uuid.New().String()

	// Store compensation data
	compensationData := FiatCompensationData{
		OperationID: opID,
		Type:        FiatOpCompleteDeposit,
		OriginalData: map[string]interface{}{
			"transaction_id": transactionID,
		},
		Timestamp: time.Now(),
	}

	// Create operation
	operation := FiatOperation{
		ID:        opID,
		Type:      FiatOpCompleteDeposit,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"transaction_id": transactionID,
		},
	}

	f.operations = append(f.operations, operation)
	f.compensationOps = append(f.compensationOps, compensationData)

	f.logger.Info("Added deposit completion to XA transaction",
		zap.String("xid", f.xid),
		zap.String("operation_id", opID),
		zap.String("transaction_id", transactionID))

	return nil
}

// InitiateWithdrawal initiates a withdrawal within XA transaction
func (f *FiatXAResource) InitiateWithdrawal(ctx context.Context, userID, currency string, amount float64, bankDetails map[string]interface{}) (*models.Transaction, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	opID := uuid.New().String()

	// Store compensation data
	compensationData := FiatCompensationData{
		OperationID: opID,
		Type:        FiatOpInitiateWithdrawal,
		OriginalData: map[string]interface{}{
			"user_id":      userID,
			"currency":     currency,
			"amount":       amount,
			"bank_details": bankDetails,
		},
		Timestamp: time.Now(),
	}

	// Create operation
	operation := FiatOperation{
		ID:        opID,
		Type:      FiatOpInitiateWithdrawal,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"user_id":      userID,
			"currency":     currency,
			"amount":       amount,
			"bank_details": bankDetails,
		},
	}

	f.operations = append(f.operations, operation)
	f.compensationOps = append(f.compensationOps, compensationData)

	f.logger.Info("Added withdrawal initiation to XA transaction",
		zap.String("xid", f.xid),
		zap.String("operation_id", opID),
		zap.String("user_id", userID),
		zap.String("currency", currency),
		zap.Float64("amount", amount))

	// Return a placeholder transaction - actual creation will happen during commit
	return &models.Transaction{
		ID:          uuid.New(),
		UserID:      uuid.MustParse(userID),
		Type:        "withdrawal",
		Amount:      amount,
		Currency:    currency,
		Status:      "pending",
		Reference:   "bank",
		Description: "Fiat withdrawal to bank account",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}, nil
}

// CompleteWithdrawal completes a withdrawal within XA transaction
func (f *FiatXAResource) CompleteWithdrawal(ctx context.Context, transactionID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	opID := uuid.New().String()

	// Store compensation data
	compensationData := FiatCompensationData{
		OperationID: opID,
		Type:        FiatOpCompleteWithdrawal,
		OriginalData: map[string]interface{}{
			"transaction_id": transactionID,
		},
		Timestamp: time.Now(),
	}

	// Create operation
	operation := FiatOperation{
		ID:        opID,
		Type:      FiatOpCompleteWithdrawal,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"transaction_id": transactionID,
		},
	}

	f.operations = append(f.operations, operation)
	f.compensationOps = append(f.compensationOps, compensationData)

	f.logger.Info("Added withdrawal completion to XA transaction",
		zap.String("xid", f.xid),
		zap.String("operation_id", opID),
		zap.String("transaction_id", transactionID))

	return nil
}

// FailWithdrawal fails a withdrawal within XA transaction
func (f *FiatXAResource) FailWithdrawal(ctx context.Context, transactionID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	opID := uuid.New().String()

	// Store compensation data
	compensationData := FiatCompensationData{
		OperationID: opID,
		Type:        FiatOpFailWithdrawal,
		OriginalData: map[string]interface{}{
			"transaction_id": transactionID,
		},
		Timestamp: time.Now(),
	}

	// Create operation
	operation := FiatOperation{
		ID:        opID,
		Type:      FiatOpFailWithdrawal,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"transaction_id": transactionID,
		},
	}

	f.operations = append(f.operations, operation)
	f.compensationOps = append(f.compensationOps, compensationData)

	f.logger.Info("Added withdrawal failure to XA transaction",
		zap.String("xid", f.xid),
		zap.String("operation_id", opID),
		zap.String("transaction_id", transactionID))

	return nil
}

// ValidateBankDetails validates bank details within XA transaction
func (f *FiatXAResource) ValidateBankDetails(ctx context.Context, bankDetails map[string]interface{}) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	opID := uuid.New().String()

	// Store compensation data
	compensationData := FiatCompensationData{
		OperationID: opID,
		Type:        FiatOpValidateBankDetails,
		OriginalData: map[string]interface{}{
			"bank_details": bankDetails,
		},
		Timestamp: time.Now(),
	}

	// Create operation
	operation := FiatOperation{
		ID:        opID,
		Type:      FiatOpValidateBankDetails,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"bank_details": bankDetails,
		},
	}

	f.operations = append(f.operations, operation)
	f.compensationOps = append(f.compensationOps, compensationData)

	f.logger.Info("Added bank details validation to XA transaction",
		zap.String("xid", f.xid),
		zap.String("operation_id", opID))

	return nil
}

// Prepare implements the prepare phase of 2PC
func (f *FiatXAResource) Prepare(ctx context.Context, xid XID) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.prepared {
		return true, nil
	}

	f.logger.Info("Preparing fiat XA transaction",
		zap.String("xid", xid.String()),
		zap.Int("operation_count", len(f.operations)))

	// Start database transaction
	f.dbTx = f.db.Begin()
	if f.dbTx.Error != nil {
		return false, fmt.Errorf("failed to begin database transaction: %w", f.dbTx.Error)
	}

	// Validate all operations
	for _, op := range f.operations {
		if err := f.validateOperation(op); err != nil {
			f.dbTx.Rollback()
			return false, fmt.Errorf("operation validation failed: %w", err)
		}
	}

	// Execute all operations within the transaction context
	for _, op := range f.operations {
		if err := f.executeOperation(f.dbTx, ctx, op); err != nil {
			f.dbTx.Rollback()
			return false, fmt.Errorf("operation execution failed: %w", err)
		}
	}

	f.prepared = true
	f.logger.Info("Fiat XA transaction prepared successfully",
		zap.String("xid", xid.String()))

	return true, nil
}

// Commit implements the commit phase of 2PC
func (f *FiatXAResource) Commit(ctx context.Context, xid XID, onePhase bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.committed {
		return nil
	}

	if !f.prepared && !onePhase {
		return fmt.Errorf("transaction not prepared")
	}

	f.logger.Info("Committing fiat XA transaction",
		zap.String("xid", xid.String()),
		zap.Bool("one_phase", onePhase))

	// If one-phase commit, execute operations first
	if onePhase {
		f.dbTx = f.db.Begin()
		if f.dbTx.Error != nil {
			return fmt.Errorf("failed to begin database transaction: %w", f.dbTx.Error)
		}

		for _, op := range f.operations {
			if err := f.executeOperation(f.dbTx, ctx, op); err != nil {
				f.dbTx.Rollback()
				return fmt.Errorf("operation execution failed: %w", err)
			}
		}
	}

	// Commit the database transaction
	if f.dbTx != nil {
		if err := f.dbTx.Commit().Error; err != nil {
			return fmt.Errorf("failed to commit database transaction: %w", err)
		}
	}

	f.committed = true
	f.logger.Info("Fiat XA transaction committed successfully",
		zap.String("xid", xid.String()))

	return nil
}

// Rollback implements the rollback phase of 2PC
func (f *FiatXAResource) Rollback(ctx context.Context, xid XID) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.aborted {
		return nil
	}

	f.logger.Info("Rolling back fiat XA transaction",
		zap.String("xid", xid.String()),
		zap.Int("compensation_count", len(f.compensationOps)))

	// Rollback database transaction if exists
	if f.dbTx != nil {
		f.dbTx.Rollback()
	}

	// Execute compensation logic for each operation in reverse order
	for i := len(f.compensationOps) - 1; i >= 0; i-- {
		comp := f.compensationOps[i]
		if err := f.executeCompensation(comp); err != nil {
			f.logger.Error("Compensation failed",
				zap.String("xid", xid.String()),
				zap.String("operation_id", comp.OperationID),
				zap.String("type", string(comp.Type)),
				zap.Error(err))
			// Continue with other compensations even if one fails
		}
	}

	f.aborted = true
	f.logger.Info("Fiat XA transaction rolled back successfully",
		zap.String("xid", xid.String()))

	return nil
}

// Forget implements the forget phase for heuristic completions
func (f *FiatXAResource) Forget(ctx context.Context, xid XID) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.logger.Info("Forgetting fiat XA transaction",
		zap.String("xid", xid.String()))

	// Clean up any resources associated with this transaction
	f.operations = nil
	f.compensationOps = nil
	f.dbTx = nil

	return nil
}

// Recover implements the recovery phase
func (f *FiatXAResource) Recover(ctx context.Context, flags int) ([]XID, error) {
	// In a production system, this would query the database for
	// transactions that were prepared but not committed
	return []XID{}, nil
}

// validateOperation validates an operation before execution
func (f *FiatXAResource) validateOperation(op FiatOperation) error {
	switch op.Type {
	case FiatOpInitiateDeposit:
		data := op.Data
		currency := data["currency"].(string)
		amount := data["amount"].(float64)
		provider := data["provider"].(string)

		if amount <= 0 {
			return fmt.Errorf("invalid deposit amount: %f", amount)
		}

		if currency == "" {
			return fmt.Errorf("currency is required")
		}

		if provider == "" {
			return fmt.Errorf("provider is required")
		}

	case FiatOpInitiateWithdrawal:
		data := op.Data
		currency := data["currency"].(string)
		amount := data["amount"].(float64)
		bankDetails := data["bank_details"].(map[string]interface{})

		if amount <= 0 {
			return fmt.Errorf("invalid withdrawal amount: %f", amount)
		}

		if currency == "" {
			return fmt.Errorf("currency is required")
		}

		if len(bankDetails) == 0 {
			return fmt.Errorf("bank details are required")
		}

	case FiatOpCompleteDeposit, FiatOpCompleteWithdrawal, FiatOpFailWithdrawal:
		data := op.Data
		transactionID := data["transaction_id"].(string)

		if transactionID == "" {
			return fmt.Errorf("transaction_id is required")
		}

		// Check if transaction exists
		var transaction models.Transaction
		if err := f.db.First(&transaction, "id = ?", transactionID).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return fmt.Errorf("transaction not found: %s", transactionID)
			}
			return fmt.Errorf("failed to find transaction: %w", err)
		}

	case FiatOpValidateBankDetails:
		data := op.Data
		bankDetails := data["bank_details"].(map[string]interface{})

		if len(bankDetails) == 0 {
			return fmt.Errorf("bank details are required")
		}

		// Validate required fields
		requiredFields := []string{"account_number", "routing_number", "account_type", "bank_name"}
		for _, field := range requiredFields {
			if _, exists := bankDetails[field]; !exists {
				return fmt.Errorf("required field '%s' is missing from bank details", field)
			}
		}
	}

	return nil
}

// executeOperation executes an operation within the database transaction
func (f *FiatXAResource) executeOperation(tx *gorm.DB, ctx context.Context, op FiatOperation) error {
	switch op.Type {
	case FiatOpInitiateDeposit:
		return f.executeInitiateDeposit(tx, ctx, op)
	case FiatOpCompleteDeposit:
		return f.executeCompleteDeposit(tx, ctx, op)
	case FiatOpInitiateWithdrawal:
		return f.executeInitiateWithdrawal(tx, ctx, op)
	case FiatOpCompleteWithdrawal:
		return f.executeCompleteWithdrawal(tx, ctx, op)
	case FiatOpFailWithdrawal:
		return f.executeFailWithdrawal(tx, ctx, op)
	case FiatOpValidateBankDetails:
		return f.executeValidateBankDetails(tx, ctx, op)
	default:
		return fmt.Errorf("unknown operation type: %s", op.Type)
	}
}

func (f *FiatXAResource) executeInitiateDeposit(tx *gorm.DB, ctx context.Context, op FiatOperation) error {
	data := op.Data
	userID := data["user_id"].(string)
	currency := data["currency"].(string)
	amount := data["amount"].(float64)
	provider := data["provider"].(string)

	// Create transaction record
	transaction := &models.Transaction{
		ID:          uuid.New(),
		UserID:      uuid.MustParse(userID),
		Type:        "deposit",
		Amount:      amount,
		Currency:    currency,
		Status:      "pending",
		Reference:   provider,
		Description: fmt.Sprintf("Fiat deposit via %s", provider),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	return tx.Create(transaction).Error
}

func (f *FiatXAResource) executeCompleteDeposit(tx *gorm.DB, ctx context.Context, op FiatOperation) error {
	data := op.Data
	transactionID := data["transaction_id"].(string)

	// Update transaction status to completed
	return tx.Model(&models.Transaction{}).
		Where("id = ? AND type = ? AND status = ?", transactionID, "deposit", "pending").
		Updates(map[string]interface{}{
			"status":     "completed",
			"updated_at": time.Now(),
		}).Error
}

func (f *FiatXAResource) executeInitiateWithdrawal(tx *gorm.DB, ctx context.Context, op FiatOperation) error {
	data := op.Data
	userID := data["user_id"].(string)
	currency := data["currency"].(string)
	amount := data["amount"].(float64)

	// Create transaction record
	transaction := &models.Transaction{
		ID:          uuid.New(),
		UserID:      uuid.MustParse(userID),
		Type:        "withdrawal",
		Amount:      amount,
		Currency:    currency,
		Status:      "pending",
		Reference:   "bank",
		Description: "Fiat withdrawal to bank account",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	return tx.Create(transaction).Error
}

func (f *FiatXAResource) executeCompleteWithdrawal(tx *gorm.DB, ctx context.Context, op FiatOperation) error {
	data := op.Data
	transactionID := data["transaction_id"].(string)

	// Update transaction status to completed
	return tx.Model(&models.Transaction{}).
		Where("id = ? AND type = ? AND status = ?", transactionID, "withdrawal", "pending").
		Updates(map[string]interface{}{
			"status":     "completed",
			"updated_at": time.Now(),
		}).Error
}

func (f *FiatXAResource) executeFailWithdrawal(tx *gorm.DB, ctx context.Context, op FiatOperation) error {
	data := op.Data
	transactionID := data["transaction_id"].(string)

	// Update transaction status to failed
	return tx.Model(&models.Transaction{}).
		Where("id = ? AND type = ? AND status = ?", transactionID, "withdrawal", "pending").
		Updates(map[string]interface{}{
			"status":     "failed",
			"updated_at": time.Now(),
		}).Error
}

func (f *FiatXAResource) executeValidateBankDetails(tx *gorm.DB, ctx context.Context, op FiatOperation) error {
	// Bank details validation doesn't require database operations
	// The validation is already done in validateOperation
	return nil
}

func (f *FiatXAResource) executeCompensation(comp FiatCompensationData) error {
	// Implement compensation logic for each operation type
	switch comp.Type {
	case FiatOpInitiateDeposit:
		// For deposit initiation, we would typically need to cancel the
		// payment request with the provider or mark it as cancelled
		f.logger.Info("Compensating deposit initiation",
			zap.String("operation_id", comp.OperationID))

		// In a real implementation, this might involve:
		// - Cancelling payment requests with payment providers
		// - Notifying external systems of cancellation
		// - Updating transaction status in the database

		data := comp.OriginalData
		userID := data["user_id"].(string)

		// Update any created transaction to cancelled status
		if err := f.db.Model(&models.Transaction{}).
			Where("user_id = ? AND type = ? AND status = ?", userID, "deposit", "pending").
			Updates(map[string]interface{}{
				"status":     "cancelled",
				"updated_at": time.Now(),
			}).Error; err != nil {
			return fmt.Errorf("failed to cancel deposit transaction: %w", err)
		}

	case FiatOpInitiateWithdrawal:
		// For withdrawal initiation, we would cancel the withdrawal
		f.logger.Info("Compensating withdrawal initiation",
			zap.String("operation_id", comp.OperationID))

		data := comp.OriginalData
		userID := data["user_id"].(string)

		// Update any created transaction to cancelled status
		if err := f.db.Model(&models.Transaction{}).
			Where("user_id = ? AND type = ? AND status = ?", userID, "withdrawal", "pending").
			Updates(map[string]interface{}{
				"status":     "cancelled",
				"updated_at": time.Now(),
			}).Error; err != nil {
			return fmt.Errorf("failed to cancel withdrawal transaction: %w", err)
		}

	case FiatOpCompleteDeposit:
		// For completed deposits, we would reverse the completion
		f.logger.Info("Compensating deposit completion",
			zap.String("operation_id", comp.OperationID))

		data := comp.OriginalData
		transactionID := data["transaction_id"].(string)

		// Revert transaction status back to pending
		if err := f.db.Model(&models.Transaction{}).
			Where("id = ? AND type = ? AND status = ?", transactionID, "deposit", "completed").
			Updates(map[string]interface{}{
				"status":     "pending",
				"updated_at": time.Now(),
			}).Error; err != nil {
			return fmt.Errorf("failed to revert deposit completion: %w", err)
		}

	case FiatOpCompleteWithdrawal:
		// For completed withdrawals, we would reverse the completion
		f.logger.Info("Compensating withdrawal completion",
			zap.String("operation_id", comp.OperationID))

		data := comp.OriginalData
		transactionID := data["transaction_id"].(string)

		// Revert transaction status back to pending
		if err := f.db.Model(&models.Transaction{}).
			Where("id = ? AND type = ? AND status = ?", transactionID, "withdrawal", "completed").
			Updates(map[string]interface{}{
				"status":     "pending",
				"updated_at": time.Now(),
			}).Error; err != nil {
			return fmt.Errorf("failed to revert withdrawal completion: %w", err)
		}

	case FiatOpFailWithdrawal:
		// For failed withdrawals, we would reverse the failure
		f.logger.Info("Compensating withdrawal failure",
			zap.String("operation_id", comp.OperationID))

		data := comp.OriginalData
		transactionID := data["transaction_id"].(string)

		// Revert transaction status back to pending
		if err := f.db.Model(&models.Transaction{}).
			Where("id = ? AND type = ? AND status = ?", transactionID, "withdrawal", "failed").
			Updates(map[string]interface{}{
				"status":     "pending",
				"updated_at": time.Now(),
			}).Error; err != nil {
			return fmt.Errorf("failed to revert withdrawal failure: %w", err)
		}

	default:
		f.logger.Warn("No compensation logic for operation type",
			zap.String("type", string(comp.Type)),
			zap.String("operation_id", comp.OperationID))
	}

	return nil
}

// GetOperationCount returns the number of operations in the current transaction
func (f *FiatXAResource) GetOperationCount() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return len(f.operations)
}

// GetTransactionState returns the current state of the XA resource
func (f *FiatXAResource) GetTransactionState() (prepared, committed, aborted bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.prepared, f.committed, f.aborted
}

// IsReadOnly returns whether this resource performs read-only operations
func (f *FiatXAResource) IsReadOnly() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()

	// Check if all operations are read-only
	for _, op := range f.operations {
		switch op.Type {
		case FiatOpValidateBankDetails:
			// Read-only operations
			continue
		default:
			// Write operations
			return false
		}
	}

	return len(f.operations) > 0
}

// GetPendingTransactionCount returns the number of pending transactions
func (f *FiatXAResource) GetPendingTransactionCount() int {
	// This would typically query the database for pending fiat transactions
	// For now, we return the current operation count
	return f.GetOperationCount()
}

// GetTransactionMetrics returns metrics about the fiat transactions
func (f *FiatXAResource) GetTransactionMetrics() map[string]interface{} {
	f.mu.RLock()
	defer f.mu.RUnlock()

	metrics := map[string]interface{}{
		"operation_count":    len(f.operations),
		"compensation_count": len(f.compensationOps),
		"prepared":           f.prepared,
		"committed":          f.committed,
		"aborted":            f.aborted,
		"xid":                f.xid,
	}

	// Count operations by type
	opCounts := make(map[string]int)
	for _, op := range f.operations {
		opCounts[string(op.Type)]++
	}
	metrics["operation_types"] = opCounts

	return metrics
}
