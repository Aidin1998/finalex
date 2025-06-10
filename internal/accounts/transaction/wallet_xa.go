package transaction

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Aidin1998/finalex/pkg/models"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// WalletXAResource implements XAResource interface for wallet service operations
type WalletXAResource struct {
	mu              sync.RWMutex
	db              *gorm.DB
	walletService   interface{} // changed from *wallet.WalletService
	logger          *zap.Logger
	xid             string
	state           XAResourceState
	operations      []WalletOperation
	compensationOps []WalletCompensationData
}

// WalletOperationType represents types of wallet operations
type WalletOperationType string

const (
	WalletOpCreateWithdrawal    WalletOperationType = "CREATE_WITHDRAWAL"
	WalletOpApproveWithdrawal   WalletOperationType = "APPROVE_WITHDRAWAL"
	WalletOpBroadcastWithdrawal WalletOperationType = "BROADCAST_WITHDRAWAL"
	WalletOpUpdateBalance       WalletOperationType = "UPDATE_BALANCE"
	WalletOpLockFunds           WalletOperationType = "LOCK_FUNDS"
	WalletOpUnlockFunds         WalletOperationType = "UNLOCK_FUNDS"
	WalletOpTransfer            WalletOperationType = "TRANSFER"
	WalletOpWhitelistAddress    WalletOperationType = "WHITELIST_ADDRESS"
	WalletOpColdStorageTransfer WalletOperationType = "COLD_STORAGE_TRANSFER"
)

// WalletOperation represents a wallet operation within an XA transaction
type WalletOperation struct {
	ID        string              `json:"id"`
	Type      WalletOperationType `json:"type"`
	Timestamp time.Time           `json:"timestamp"`
	Data      interface{}         `json:"data"`
}

// WalletCompensationData holds data needed for rollback operations
type WalletCompensationData struct {
	OperationID    string                 `json:"operation_id"`
	Type           WalletOperationType    `json:"type"`
	OriginalData   map[string]interface{} `json:"original_data"`
	CompensateData map[string]interface{} `json:"compensate_data"`
	Timestamp      time.Time              `json:"timestamp"`
}

// NewWalletXAResource creates a new XA resource for wallet operations
func NewWalletXAResource(db *gorm.DB, walletService interface{}, logger *zap.Logger) *WalletXAResource {
	return &WalletXAResource{
		db:              db,
		walletService:   walletService,
		logger:          logger,
		state:           XAResourceStateActive,
		operations:      make([]WalletOperation, 0),
		compensationOps: make([]WalletCompensationData, 0),
	}
}

// XAResource interface implementation

func (w *WalletXAResource) Start(xid string, flags int) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.state != XAResourceStateActive {
		return &XAException{
			ErrorCode: XAErrorRMFAIL,
			Message:   fmt.Sprintf("Cannot start XA transaction in state %v", w.state),
		}
	}

	w.xid = xid
	w.operations = make([]WalletOperation, 0)
	w.compensationOps = make([]WalletCompensationData, 0)

	w.logger.Info("Started XA transaction for wallet operations",
		zap.String("xid", xid),
		zap.Int("flags", flags))

	return nil
}

func (w *WalletXAResource) End(xid string, flags int) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.xid != xid {
		return &XAException{
			ErrorCode: XAErrorXAER_NOTA,
			Message:   "Transaction not associated with this resource",
		}
	}

	if XAFlag(flags)&XAFlagTMSUCCESS != 0 {
		w.state = XAResourceStateIdle
	} else {
		w.state = XAResourceStateRollbackOnly
	}

	w.logger.Info("Ended XA transaction for wallet operations",
		zap.String("xid", xid),
		zap.Int("flags", flags),
		zap.String("state", string(w.state)))

	return nil
}

func (w *WalletXAResource) Prepare(ctx context.Context, xid XID) (bool, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	xidStr := xid.String()
	if w.xid != xidStr {
		return false, &XAException{
			ErrorCode: XAErrorXAER_NOTA,
			Message:   "Transaction not associated with this resource",
		}
	}

	if w.state == XAResourceStateRollbackOnly {
		return false, &XAException{
			ErrorCode: XAErrorRB_ROLLBACK,
			Message:   "Transaction marked for rollback only",
		}
	}

	// Validate all operations can be committed
	for _, op := range w.operations {
		if err := w.validateOperation(op); err != nil {
			w.logger.Error("Operation validation failed during prepare",
				zap.String("xid", xidStr),
				zap.String("operation_id", op.ID),
				zap.String("type", string(op.Type)),
				zap.Error(err))
			w.state = XAResourceStateRollbackOnly
			return false, &XAException{
				ErrorCode: XAErrorRB_ROLLBACK,
				Message:   fmt.Sprintf("Operation validation failed: %v", err),
			}
		}
	}

	// Check for resource conflicts
	if err := w.checkResourceConflicts(); err != nil {
		w.logger.Error("Resource conflict detected during prepare",
			zap.String("xid", xidStr),
			zap.Error(err))
		w.state = XAResourceStateRollbackOnly
		return false, &XAException{
			ErrorCode: XAErrorRB_ROLLBACK,
			Message:   fmt.Sprintf("Resource conflict: %v", err),
		}
	}

	w.state = XAResourceStatePrepared
	w.logger.Info("Prepared XA transaction for wallet operations",
		zap.String("xid", xidStr),
		zap.Int("operations_count", len(w.operations)))

	return true, nil
}

func (w *WalletXAResource) Commit(ctx context.Context, xid XID, onePhase bool) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	xidStr := xid.String()
	if w.xid != xidStr {
		return &XAException{
			ErrorCode: XAErrorXAER_NOTA,
			Message:   "Transaction not associated with this resource",
		}
	}

	if !onePhase && w.state != XAResourceStatePrepared {
		return &XAException{
			ErrorCode: XAErrorXAER_PROTO,
			Message:   fmt.Sprintf("Invalid state for commit: %v", w.state),
		}
	}

	// Execute all operations within a database transaction
	tx := w.db.Begin()
	if tx.Error != nil {
		return &XAException{
			ErrorCode: XAErrorRMFAIL,
			Message:   fmt.Sprintf("Failed to begin database transaction: %v", tx.Error),
		}
	}

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			w.logger.Error("Panic during wallet XA commit, rolled back",
				zap.String("xid", xidStr),
				zap.Any("panic", r))
		}
	}()

	// Execute operations in order
	for _, op := range w.operations {
		if err := w.executeOperation(tx, op); err != nil {
			tx.Rollback()
			w.logger.Error("Failed to execute wallet operation during commit",
				zap.String("xid", xidStr),
				zap.String("operation_id", op.ID),
				zap.String("type", string(op.Type)),
				zap.Error(err))
			return &XAException{
				ErrorCode: XAErrorRMFAIL,
				Message:   fmt.Sprintf("Failed to execute operation %s: %v", op.ID, err),
			}
		}
	}

	// Commit the database transaction
	if err := tx.Commit().Error; err != nil {
		w.logger.Error("Failed to commit database transaction",
			zap.String("xid", xidStr),
			zap.Error(err))
		return &XAException{
			ErrorCode: XAErrorRMFAIL,
			Message:   fmt.Sprintf("Database commit failed: %v", err),
		}
	}

	w.state = XAResourceStateCommitted
	w.logger.Info("Committed XA transaction for wallet operations",
		zap.String("xid", xidStr),
		zap.Bool("one_phase", onePhase),
		zap.Int("operations_executed", len(w.operations)))

	// Clear operations after successful commit
	w.operations = make([]WalletOperation, 0)
	w.compensationOps = make([]WalletCompensationData, 0)

	return nil
}

func (w *WalletXAResource) Rollback(ctx context.Context, xid XID) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	xidStr := xid.String()
	if w.xid != xidStr {
		return &XAException{
			ErrorCode: XAErrorXAER_NOTA,
			Message:   "Transaction not associated with this resource",
		}
	}

	// Execute compensation operations in reverse order
	for i := len(w.compensationOps) - 1; i >= 0; i-- {
		comp := w.compensationOps[i]
		if err := w.executeCompensation(comp); err != nil {
			w.logger.Error("Failed to execute compensation operation",
				zap.String("xid", xidStr),
				zap.String("operation_id", comp.OperationID),
				zap.String("type", string(comp.Type)),
				zap.Error(err))
			// Continue with other compensations even if one fails
		}
	}

	w.state = XAResourceStateRolledBack
	w.logger.Info("Rolled back XA transaction for wallet operations",
		zap.String("xid", xidStr),
		zap.Int("compensations_executed", len(w.compensationOps)))

	// Clear operations after rollback
	w.operations = make([]WalletOperation, 0)
	w.compensationOps = make([]WalletCompensationData, 0)

	return nil
}

func (w *WalletXAResource) Forget(ctx context.Context, xid XID) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	xidStr := xid.String()
	if w.xid != xidStr {
		return &XAException{
			ErrorCode: XAErrorXAER_NOTA,
			Message:   "Transaction not associated with this resource",
		}
	}

	// Clean up any heuristic completion state
	w.state = XAResourceStateActive
	w.operations = make([]WalletOperation, 0)
	w.compensationOps = make([]WalletCompensationData, 0)
	w.xid = ""

	w.logger.Info("Forgot XA transaction for wallet operations", zap.String("xid", xidStr))
	return nil
}

func (w *WalletXAResource) Recover(ctx context.Context, flags int) ([]XID, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	// In a real implementation, this would scan persistent storage
	// for in-doubt transactions and return their XIDs
	var xids []XID

	w.logger.Info("Recovered XA transactions for wallet operations",
		zap.Int("flags", flags),
		zap.Int("transactions_count", len(xids)))

	return xids, nil
}

func (w *WalletXAResource) IsSameRM(other XAResource) bool {
	walletXA, ok := other.(*WalletXAResource)
	if !ok {
		return false
	}
	return w.db == walletXA.db && w.walletService == walletXA.walletService
}

// Wallet-specific XA operations

// CreateWithdrawalRequest creates a withdrawal request within XA transaction
func (w *WalletXAResource) CreateWithdrawalRequest(ctx context.Context, userID uuid.UUID, walletID, asset, toAddress string, amount float64) (*models.WithdrawalRequest, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	opID := uuid.New().String()

	// Store compensation data
	compensationData := WalletCompensationData{
		OperationID: opID,
		Type:        WalletOpCreateWithdrawal,
		OriginalData: map[string]interface{}{
			"user_id":    userID,
			"wallet_id":  walletID,
			"asset":      asset,
			"to_address": toAddress,
			"amount":     amount,
		},
		Timestamp: time.Now(),
	}

	// Create operation
	operation := WalletOperation{
		ID:        opID,
		Type:      WalletOpCreateWithdrawal,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"user_id":    userID,
			"wallet_id":  walletID,
			"asset":      asset,
			"to_address": toAddress,
			"amount":     amount,
		},
	}

	w.operations = append(w.operations, operation)
	w.compensationOps = append(w.compensationOps, compensationData)

	w.logger.Info("Added withdrawal request creation to XA transaction",
		zap.String("xid", w.xid),
		zap.String("operation_id", opID),
		zap.String("wallet_id", walletID),
		zap.Float64("amount", amount))
	// Return a placeholder withdrawal request
	// The actual creation will happen during commit
	return &models.WithdrawalRequest{
		ID:        uuid.New(),
		UserID:    userID,
		WalletID:  walletID,
		Asset:     asset,
		Amount:    decimal.NewFromFloat(amount),
		ToAddress: toAddress,
		Status:    "pending",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}, nil
}

// ApproveWithdrawal adds withdrawal approval to XA transaction
func (w *WalletXAResource) ApproveWithdrawal(ctx context.Context, requestID uuid.UUID, approver string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	opID := uuid.New().String()

	// Store compensation data
	compensationData := WalletCompensationData{
		OperationID: opID,
		Type:        WalletOpApproveWithdrawal,
		OriginalData: map[string]interface{}{
			"request_id": requestID,
			"approver":   approver,
		},
		Timestamp: time.Now(),
	}

	// Create operation
	operation := WalletOperation{
		ID:        opID,
		Type:      WalletOpApproveWithdrawal,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"request_id": requestID,
			"approver":   approver,
		},
	}

	w.operations = append(w.operations, operation)
	w.compensationOps = append(w.compensationOps, compensationData)

	w.logger.Info("Added withdrawal approval to XA transaction",
		zap.String("xid", w.xid),
		zap.String("operation_id", opID),
		zap.String("request_id", requestID.String()),
		zap.String("approver", approver))

	return nil
}

// BroadcastWithdrawal adds withdrawal broadcasting to XA transaction
func (w *WalletXAResource) BroadcastWithdrawal(ctx context.Context, requestID uuid.UUID) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	opID := uuid.New().String()

	// Store compensation data
	compensationData := WalletCompensationData{
		OperationID: opID,
		Type:        WalletOpBroadcastWithdrawal,
		OriginalData: map[string]interface{}{
			"request_id": requestID,
		},
		Timestamp: time.Now(),
	}

	// Create operation
	operation := WalletOperation{
		ID:        opID,
		Type:      WalletOpBroadcastWithdrawal,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"request_id": requestID,
		},
	}

	w.operations = append(w.operations, operation)
	w.compensationOps = append(w.compensationOps, compensationData)

	w.logger.Info("Added withdrawal broadcasting to XA transaction",
		zap.String("xid", w.xid),
		zap.String("operation_id", opID),
		zap.String("request_id", requestID.String()))

	return nil
}

// UpdateWalletBalance adds balance update to XA transaction
func (w *WalletXAResource) UpdateWalletBalance(ctx context.Context, walletID string, amount float64, opType string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	opID := uuid.New().String()

	// Get current balance for compensation
	var wallet models.Wallet
	if err := w.db.WithContext(ctx).First(&wallet, "id = ?", walletID).Error; err != nil {
		return fmt.Errorf("failed to get wallet for balance update: %w", err)
	}

	// Store compensation data
	compensationData := WalletCompensationData{
		OperationID: opID,
		Type:        WalletOpUpdateBalance,
		OriginalData: map[string]interface{}{
			"wallet_id":        walletID,
			"original_balance": wallet.Balance,
		},
		CompensateData: map[string]interface{}{
			"wallet_id":       walletID,
			"restore_balance": wallet.Balance,
		},
		Timestamp: time.Now(),
	}

	// Create operation
	operation := WalletOperation{
		ID:        opID,
		Type:      WalletOpUpdateBalance,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"wallet_id": walletID,
			"amount":    amount,
			"operation": opType,
		},
	}

	w.operations = append(w.operations, operation)
	w.compensationOps = append(w.compensationOps, compensationData)

	w.logger.Info("Added balance update to XA transaction",
		zap.String("xid", w.xid),
		zap.String("operation_id", opID),
		zap.String("wallet_id", walletID),
		zap.Float64("amount", amount))

	return nil
}

// AddWhitelistAddress adds address whitelisting to XA transaction
func (w *WalletXAResource) AddWhitelistAddress(ctx context.Context, walletID string, address string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	opID := uuid.New().String()

	// Get current whitelist for compensation
	var wallet models.Wallet
	if err := w.db.WithContext(ctx).First(&wallet, "id = ?", walletID).Error; err != nil {
		return fmt.Errorf("failed to get wallet for whitelist update: %w", err)
	}

	// Store compensation data
	compensationData := WalletCompensationData{
		OperationID: opID,
		Type:        WalletOpWhitelistAddress,
		OriginalData: map[string]interface{}{
			"wallet_id":          walletID,
			"original_whitelist": wallet.AddressWhitelist,
		},
		CompensateData: map[string]interface{}{
			"wallet_id":      walletID,
			"remove_address": address,
		},
		Timestamp: time.Now(),
	}

	// Create operation
	operation := WalletOperation{
		ID:        opID,
		Type:      WalletOpWhitelistAddress,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"wallet_id": walletID,
			"address":   address,
			"action":    "add",
		},
	}

	w.operations = append(w.operations, operation)
	w.compensationOps = append(w.compensationOps, compensationData)

	w.logger.Info("Added whitelist address to XA transaction",
		zap.String("xid", w.xid),
		zap.String("operation_id", opID),
		zap.String("wallet_id", walletID),
		zap.String("address", address))

	return nil
}

// Helper methods

func (w *WalletXAResource) validateOperation(op WalletOperation) error {
	switch op.Type {
	case WalletOpCreateWithdrawal:
		data := op.Data.(map[string]interface{})
		walletID := data["wallet_id"].(string)
		amount := data["amount"].(float64)

		if amount <= 0 {
			return fmt.Errorf("invalid withdrawal amount: %f", amount)
		}

		// Check wallet exists and has sufficient balance
		var wallet models.Wallet
		if err := w.db.First(&wallet, "id = ?", walletID).Error; err != nil {
			return fmt.Errorf("wallet not found: %w", err)
		}

		// In a real implementation, you might want to check actual available balance
		// considering pending withdrawals and locked funds

	case WalletOpApproveWithdrawal:
		data := op.Data.(map[string]interface{})
		requestID := data["request_id"].(uuid.UUID)

		// Check withdrawal request exists and is in pending state
		var wr models.WithdrawalRequest
		if err := w.db.First(&wr, "id = ?", requestID).Error; err != nil {
			return fmt.Errorf("withdrawal request not found: %w", err)
		}

		if wr.Status != "pending" {
			return fmt.Errorf("withdrawal request not in pending state: %s", wr.Status)
		}

	case WalletOpBroadcastWithdrawal:
		data := op.Data.(map[string]interface{})
		requestID := data["request_id"].(uuid.UUID)

		// Check withdrawal request exists and is approved
		var wr models.WithdrawalRequest
		if err := w.db.First(&wr, "id = ?", requestID).Error; err != nil {
			return fmt.Errorf("withdrawal request not found: %w", err)
		}

		if wr.Status != "approved" {
			return fmt.Errorf("withdrawal request not approved: %s", wr.Status)
		}

	case WalletOpUpdateBalance:
		data := op.Data.(map[string]interface{})
		walletID := data["wallet_id"].(string)

		// Check wallet exists
		var wallet models.Wallet
		if err := w.db.First(&wallet, "id = ?", walletID).Error; err != nil {
			return fmt.Errorf("wallet not found: %w", err)
		}

	case WalletOpWhitelistAddress:
		data := op.Data.(map[string]interface{})
		walletID := data["wallet_id"].(string)

		// Check wallet exists
		var wallet models.Wallet
		if err := w.db.First(&wallet, "id = ?", walletID).Error; err != nil {
			return fmt.Errorf("wallet not found: %w", err)
		}
	}

	return nil
}

func (w *WalletXAResource) checkResourceConflicts() error {
	// Check for conflicts between operations in this transaction
	walletIDs := make(map[string]bool)

	for _, op := range w.operations {
		switch op.Type {
		case WalletOpCreateWithdrawal, WalletOpUpdateBalance, WalletOpWhitelistAddress:
			data := op.Data.(map[string]interface{})
			walletID := data["wallet_id"].(string)

			if walletIDs[walletID] {
				// Multiple operations on same wallet - could be valid
				// but let's log it for monitoring
				w.logger.Warn("Multiple operations on same wallet in transaction",
					zap.String("xid", w.xid),
					zap.String("wallet_id", walletID))
			}
			walletIDs[walletID] = true
		}
	}

	// In a real implementation, you might want to check for:
	// - Concurrent transactions affecting the same wallets
	// - Resource locks
	// - Business rule conflicts

	return nil
}

func (w *WalletXAResource) executeOperation(tx *gorm.DB, op WalletOperation) error {
	ctx := context.Background()

	switch op.Type {
	case WalletOpCreateWithdrawal:
		return w.executeCreateWithdrawal(tx, ctx, op)
	case WalletOpApproveWithdrawal:
		return w.executeApproveWithdrawal(tx, ctx, op)
	case WalletOpBroadcastWithdrawal:
		return w.executeBroadcastWithdrawal(tx, ctx, op)
	case WalletOpUpdateBalance:
		return w.executeUpdateBalance(tx, ctx, op)
	case WalletOpWhitelistAddress:
		return w.executeWhitelistAddress(tx, ctx, op)
	default:
		return fmt.Errorf("unknown operation type: %s", op.Type)
	}
}

func (w *WalletXAResource) executeCreateWithdrawal(tx *gorm.DB, ctx context.Context, op WalletOperation) error {
	data := op.Data.(map[string]interface{})
	userID := data["user_id"].(uuid.UUID)
	walletID := data["wallet_id"].(string)
	asset := data["asset"].(string)
	toAddress := data["to_address"].(string)
	amount := data["amount"].(float64)

	wr := &models.WithdrawalRequest{
		ID:        uuid.New(),
		UserID:    userID,
		WalletID:  walletID,
		Asset:     asset,
		ToAddress: toAddress,
		Amount:    decimal.NewFromFloat(amount),
		Status:    "pending",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	return tx.Create(wr).Error
}

func (w *WalletXAResource) executeApproveWithdrawal(tx *gorm.DB, ctx context.Context, op WalletOperation) error {
	data := op.Data.(map[string]interface{})
	requestID := data["request_id"].(uuid.UUID)
	approver := data["approver"].(string)

	approval := &models.Approval{
		ID:        uuid.New(),
		RequestID: requestID,
		Approver:  approver,
		Approved:  true,
		Timestamp: time.Now(),
	}

	if err := tx.Create(approval).Error; err != nil {
		return err
	}

	// Check if threshold met and update status
	var count int64
	tx.Model(&models.Approval{}).Where("request_id = ? AND approved = ?", requestID, true).Count(&count)

	if count >= 2 { // Example threshold
		return tx.Model(&models.WithdrawalRequest{}).
			Where("id = ?", requestID).
			Updates(map[string]interface{}{
				"status":     "approved",
				"updated_at": time.Now(),
			}).Error
	}

	return nil
}

func (w *WalletXAResource) executeBroadcastWithdrawal(tx *gorm.DB, ctx context.Context, op WalletOperation) error {
	data := op.Data.(map[string]interface{})
	requestID := data["request_id"].(uuid.UUID)

	// Update withdrawal request status to broadcasted
	return tx.Model(&models.WithdrawalRequest{}).
		Where("id = ?", requestID).
		Updates(map[string]interface{}{
			"status":     "broadcasted",
			"updated_at": time.Now(),
		}).Error
}

func (w *WalletXAResource) executeUpdateBalance(tx *gorm.DB, ctx context.Context, op WalletOperation) error {
	data := op.Data.(map[string]interface{})
	walletID := data["wallet_id"].(string)
	amount := data["amount"].(float64)

	return tx.Model(&models.Wallet{}).
		Where("id = ?", walletID).
		Updates(map[string]interface{}{
			"balance":    gorm.Expr("balance + ?", amount),
			"updated_at": time.Now(),
		}).Error
}

func (w *WalletXAResource) executeWhitelistAddress(tx *gorm.DB, ctx context.Context, op WalletOperation) error {
	data := op.Data.(map[string]interface{})
	walletID := data["wallet_id"].(string)
	address := data["address"].(string)
	action := data["action"].(string)

	var wallet models.Wallet
	if err := tx.First(&wallet, "id = ?", walletID).Error; err != nil {
		return err
	}

	if action == "add" {
		// Check if address already exists
		for _, addr := range wallet.AddressWhitelist {
			if addr == address {
				return nil // Already whitelisted
			}
		}
		wallet.AddressWhitelist = append(wallet.AddressWhitelist, address)
	} else if action == "remove" {
		newList := make([]string, 0, len(wallet.AddressWhitelist))
		for _, addr := range wallet.AddressWhitelist {
			if addr != address {
				newList = append(newList, addr)
			}
		}
		wallet.AddressWhitelist = newList
	}

	wallet.UpdatedAt = time.Now()
	return tx.Save(&wallet).Error
}

func (w *WalletXAResource) executeCompensation(comp WalletCompensationData) error {
	ctx := context.Background()

	switch comp.Type {
	case WalletOpCreateWithdrawal:
		// For create withdrawal, we need to delete the created request
		// This is handled by cascading rollback of the database transaction
		return nil

	case WalletOpApproveWithdrawal:
		// Remove the approval that was added
		requestID := comp.OriginalData["request_id"].(uuid.UUID)
		approver := comp.OriginalData["approver"].(string)

		return w.db.WithContext(ctx).
			Where("request_id = ? AND approver = ?", requestID, approver).
			Delete(&models.Approval{}).Error

	case WalletOpUpdateBalance:
		// Restore original balance
		walletID := comp.CompensateData["wallet_id"].(string)
		restoreBalance := comp.CompensateData["restore_balance"].(float64)

		return w.db.WithContext(ctx).Model(&models.Wallet{}).
			Where("id = ?", walletID).
			Updates(map[string]interface{}{
				"balance":    restoreBalance,
				"updated_at": time.Now(),
			}).Error

	case WalletOpWhitelistAddress:
		// Remove the address that was added
		walletID := comp.CompensateData["wallet_id"].(string)
		removeAddress := comp.CompensateData["remove_address"].(string)

		var wallet models.Wallet
		if err := w.db.WithContext(ctx).First(&wallet, "id = ?", walletID).Error; err != nil {
			return err
		}

		newList := make([]string, 0, len(wallet.AddressWhitelist))
		for _, addr := range wallet.AddressWhitelist {
			if addr != removeAddress {
				newList = append(newList, addr)
			}
		}

		wallet.AddressWhitelist = newList
		wallet.UpdatedAt = time.Now()
		return w.db.WithContext(ctx).Save(&wallet).Error

	default:
		w.logger.Warn("Unknown compensation type",
			zap.String("type", string(comp.Type)),
			zap.String("operation_id", comp.OperationID))
		return nil
	}
}

// GetOperationCount returns the number of operations in the current transaction
func (w *WalletXAResource) GetOperationCount() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return len(w.operations)
}

// GetTransactionState returns the current state of the XA resource
func (w *WalletXAResource) GetTransactionState() XAResourceState {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.state
}

// GetOperations returns a copy of current operations (for monitoring/debugging)
func (w *WalletXAResource) GetOperations() []WalletOperation {
	w.mu.RLock()
	defer w.mu.RUnlock()

	operations := make([]WalletOperation, len(w.operations))
	copy(operations, w.operations)
	return operations
}

func (w *WalletXAResource) GetResourceName() string {
	return "wallet"
}
