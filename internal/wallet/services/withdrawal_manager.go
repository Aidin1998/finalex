// Package services provides withdrawal management functionality
package services

import (
	"context"
	"fmt"
	"time"

	complianceInterfaces "github.com/Aidin1998/finalex/internal/compliance/interfaces"
	"github.com/Aidin1998/finalex/internal/wallet/interfaces"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// WithdrawalManager handles all withdrawal-related operations
type WithdrawalManager struct {
	db                *gorm.DB
	logger            *zap.Logger
	fireblocksClient  interfaces.FireblocksClient
	balanceManager    interfaces.BalanceManager
	fundLockService   interfaces.FundLockService
	addressManager    interfaces.AddressManager
	complianceService complianceInterfaces.ComplianceService
	auditService      complianceInterfaces.AuditService
	eventPublisher    interfaces.EventPublisher
	repository        interfaces.WalletRepository
	cache             interfaces.WalletCache
	config            *WithdrawalConfig
}

// WithdrawalConfig holds withdrawal-specific configuration
type WithdrawalConfig struct {
	MinWithdrawalAmount  map[string]decimal.Decimal // asset -> min amount
	MaxWithdrawalAmount  map[string]decimal.Decimal // asset -> max amount
	DailyWithdrawalLimit map[string]decimal.Decimal // asset -> daily limit
	WithdrawalFee        map[string]decimal.Decimal // asset -> fee
	RequireApproval      map[string]bool            // asset -> approval required
	ComplianceThreshold  decimal.Decimal            // Amount requiring compliance check
	BatchProcessingSize  int
	ProcessingInterval   time.Duration
}

// NewWithdrawalManager creates a new withdrawal manager
func NewWithdrawalManager(
	db *gorm.DB,
	logger *zap.Logger,
	fireblocksClient interfaces.FireblocksClient,
	balanceManager interfaces.BalanceManager,
	fundLockService interfaces.FundLockService,
	addressManager interfaces.AddressManager,
	complianceService complianceInterfaces.ComplianceService,
	auditService complianceInterfaces.AuditService,
	eventPublisher interfaces.EventPublisher,
	repository interfaces.WalletRepository,
	cache interfaces.WalletCache,
	config *WithdrawalConfig,
) *WithdrawalManager {
	return &WithdrawalManager{
		db:                db,
		logger:            logger,
		fireblocksClient:  fireblocksClient,
		balanceManager:    balanceManager,
		fundLockService:   fundLockService,
		addressManager:    addressManager,
		complianceService: complianceService,
		auditService:      auditService,
		eventPublisher:    eventPublisher,
		repository:        repository,
		cache:             cache,
		config:            config,
	}
}

// InitiateWithdrawal starts a new withdrawal process
func (wm *WithdrawalManager) InitiateWithdrawal(ctx context.Context, req *interfaces.WithdrawalRequest) (*interfaces.WithdrawalResponse, error) {
	wm.logger.Info("Initiating withdrawal",
		zap.String("user_id", req.UserID.String()),
		zap.String("asset", req.Asset),
		zap.String("amount", req.Amount.String()),
		zap.String("to_address", req.ToAddress),
	)

	// Validate withdrawal request
	if err := wm.validateWithdrawal(ctx, req); err != nil {
		return nil, fmt.Errorf("withdrawal validation failed: %w", err)
	}

	// Calculate withdrawal fee
	fee := wm.calculateWithdrawalFee(req.Asset, req.Amount)
	totalAmount := req.Amount.Add(fee)
	// Check and lock user balance
	err := wm.fundLockService.LockFunds(ctx, req.UserID, req.Asset, totalAmount, "withdrawal", "")
	if err != nil {
		return nil, fmt.Errorf("failed to lock funds: %w", err)
	}

	// Create withdrawal transaction
	tx := &interfaces.WalletTransaction{
		ID:              uuid.New(),
		UserID:          req.UserID,
		Asset:           req.Asset,
		Amount:          req.Amount,
		Direction:       interfaces.DirectionWithdrawal,
		Status:          interfaces.TxStatusInitiated,
		ToAddress:       req.ToAddress,
		Network:         req.Network,
		ComplianceCheck: "pending", Metadata: interfaces.JSONB{
			"fee":      fee,
			"total":    totalAmount,
			"priority": req.Priority,
			"tag":      req.Tag,
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	// Store transaction
	if err := wm.repository.CreateTransaction(ctx, tx); err != nil {
		// Release the fund lock if transaction creation fails
		wm.fundLockService.ReleaseLock(ctx, req.UserID, req.Asset, totalAmount, "withdrawal", "")
		return nil, fmt.Errorf("failed to create withdrawal transaction: %w", err)
	}
	// Publish withdrawal initiated event
	if err := wm.publishEvent(ctx, &interfaces.WithdrawalInitiatedEvent{
		ID:            uuid.New(),
		TransactionID: tx.ID,
		UserID:        req.UserID,
		Asset:         req.Asset,
		Amount:        req.Amount,
		ToAddress:     req.ToAddress,
		Timestamp:     time.Now(),
	}); err != nil {
		wm.logger.Warn("Failed to publish withdrawal initiated event", zap.Error(err))
	}

	// Start compliance check if required
	if req.Amount.GreaterThan(wm.config.ComplianceThreshold) {
		go wm.runComplianceCheck(context.Background(), tx.ID)
	} else {
		// Auto-approve smaller withdrawals
		go wm.processWithdrawal(context.Background(), tx.ID)
	}

	return &interfaces.WithdrawalResponse{
		TransactionID:    tx.ID,
		Status:           string(tx.Status),
		EstimatedFee:     fee,
		EstimatedTime:    wm.estimateWithdrawalTime(req.Asset, req.Priority),
		RequiresApproval: wm.requiresApproval(req.Asset, req.Amount),
		CreatedAt:        tx.CreatedAt,
	}, nil
}

// ProcessWithdrawal handles the actual withdrawal execution
func (wm *WithdrawalManager) ProcessWithdrawal(ctx context.Context, txID uuid.UUID) error {
	return wm.processWithdrawal(ctx, txID)
}

// ConfirmWithdrawal updates withdrawal status based on Fireblocks confirmation
func (wm *WithdrawalManager) ConfirmWithdrawal(ctx context.Context, txID uuid.UUID, fireblocksStatus string) error {
	wm.logger.Info("Confirming withdrawal",
		zap.String("tx_id", txID.String()),
		zap.String("fireblocks_status", fireblocksStatus),
	)

	// Get transaction
	tx, err := wm.repository.GetTransaction(ctx, txID)
	if err != nil {
		return fmt.Errorf("failed to get transaction: %w", err)
	}

	if tx.Direction != interfaces.DirectionWithdrawal {
		return fmt.Errorf("transaction is not a withdrawal")
	}

	// Map Fireblocks status to our status
	newStatus := wm.mapFireblocksStatus(fireblocksStatus)
	// Update transaction status
	if err := wm.repository.UpdateTransaction(ctx, tx.ID, map[string]interface{}{
		"status":     newStatus,
		"updated_at": time.Now(),
	}); err != nil {
		return fmt.Errorf("failed to update transaction: %w", err)
	}

	// Handle completion or failure
	if newStatus == interfaces.TxStatusCompleted {
		return wm.completeWithdrawal(ctx, tx)
	} else if newStatus == interfaces.TxStatusFailed || newStatus == interfaces.TxStatusRejected {
		return wm.failWithdrawal(ctx, tx)
	}

	return nil
}

// CancelWithdrawal cancels a pending withdrawal
func (wm *WithdrawalManager) CancelWithdrawal(ctx context.Context, txID uuid.UUID, reason string) error {
	wm.logger.Info("Cancelling withdrawal",
		zap.String("tx_id", txID.String()),
		zap.String("reason", reason),
	)

	// Get transaction
	tx, err := wm.repository.GetTransaction(ctx, txID)
	if err != nil {
		return fmt.Errorf("failed to get transaction: %w", err)
	}

	if tx.Direction != interfaces.DirectionWithdrawal {
		return fmt.Errorf("transaction is not a withdrawal")
	}

	// Check if cancellation is allowed
	if !wm.canCancelWithdrawal(tx.Status) {
		return fmt.Errorf("withdrawal cannot be cancelled in status: %s", tx.Status)
	}

	// Update transaction status
	tx.Status = interfaces.TxStatusCancelled
	tx.ErrorMsg = reason
	tx.UpdatedAt = time.Now()

	// Start database transaction
	return wm.db.Transaction(func(dbTx *gorm.DB) error {
		// Update transaction
		if err := wm.repository.UpdateTransactionInTx(ctx, dbTx, tx); err != nil {
			return fmt.Errorf("failed to update transaction: %w", err)
		}

		// Release fund lock
		if lockID, ok := tx.Metadata["lock_id"].(string); ok {
			if err := wm.fundLockService.ReleaseLock(ctx, lockID); err != nil {
				wm.logger.Warn("Failed to release fund lock", zap.String("lock_id", lockID), zap.Error(err))
			}
		}

		// Publish cancellation event
		if err := wm.publishEvent(ctx, &interfaces.WithdrawalCancelledEvent{
			TransactionID: tx.ID,
			UserID:        tx.UserID,
			Asset:         tx.Asset,
			Amount:        tx.Amount,
			Reason:        reason,
			Timestamp:     time.Now(),
		}); err != nil {
			wm.logger.Warn("Failed to publish withdrawal cancelled event", zap.Error(err))
		}

		return nil
	})
}

// GetWithdrawalHistory retrieves withdrawal history for a user
func (wm *WithdrawalManager) GetWithdrawalHistory(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*interfaces.WalletTransaction, error) {
	return wm.repository.GetUserTransactionsByDirection(ctx, userID, interfaces.DirectionWithdrawal, limit, offset)
}

// EstimateWithdrawalFee calculates the withdrawal fee for an asset and amount
func (wm *WithdrawalManager) EstimateWithdrawalFee(ctx context.Context, asset string, amount decimal.Decimal) (decimal.Decimal, error) {
	return wm.calculateWithdrawalFee(asset, amount), nil
}

// ValidateWithdrawalAddress validates a withdrawal address
func (wm *WithdrawalManager) ValidateWithdrawalAddress(ctx context.Context, req *interfaces.AddressValidationRequest) (*interfaces.AddressValidationResult, error) {
	return wm.addressManager.ValidateAddress(ctx, req)
}

// Private helper methods

func (wm *WithdrawalManager) processWithdrawal(ctx context.Context, txID uuid.UUID) error {
	wm.logger.Info("Processing withdrawal", zap.String("tx_id", txID.String()))

	// Get transaction
	tx, err := wm.repository.GetTransaction(ctx, txID)
	if err != nil {
		return fmt.Errorf("failed to get transaction: %w", err)
	}

	// Check compliance status
	if tx.ComplianceCheck == "rejected" {
		return wm.failWithdrawal(ctx, tx)
	}

	if tx.ComplianceCheck == "pending" {
		wm.logger.Info("Withdrawal pending compliance check", zap.String("tx_id", txID.String()))
		return nil // Wait for compliance check to complete
	}
	// Update status to processing
	if err := wm.repository.UpdateTransaction(ctx, tx.ID, map[string]interface{}{
		"status":     interfaces.TxStatusPending,
		"updated_at": time.Now(),
	}); err != nil {
		return fmt.Errorf("failed to update transaction status: %w", err)
	} // Prepare Fireblocks transaction request
	fbReq := &interfaces.TransactionRequest{
		AssetID: tx.Asset,
		Amount:  tx.Amount.String(),
		Source:  interfaces.TransactionSource{Type: "VAULT_ACCOUNT", ID: "0"}, // Main vault
		Destination: interfaces.TransactionDestination{
			Type: "ONE_TIME_ADDRESS",
			OneTimeAddress: &interfaces.OneTimeAddress{
				Address: tx.ToAddress,
			},
		},
		Note: fmt.Sprintf("Withdrawal for user %s", tx.UserID.String()),
	}

	// Add tag if present
	if tag, ok := tx.Metadata["tag"].(string); ok && tag != "" {
		fbReq.Destination.OneTimeAddress.Tag = tag
	}
	// Execute withdrawal via Fireblocks
	fbTx, err := wm.fireblocksClient.CreateTransaction(ctx, fbReq)
	if err != nil {
		wm.logger.Error("Failed to create Fireblocks transaction", zap.Error(err))
		return wm.repository.UpdateTransaction(ctx, tx.ID, map[string]interface{}{
			"status":    interfaces.TxStatusFailed,
			"error_msg": err.Error(),
		})
	}

	// Update transaction with Fireblocks ID
	tx.FireblocksID = fbTx.ID
	tx.Status = interfaces.TxStatusPending
	tx.UpdatedAt = time.Now()

	if err := wm.repository.UpdateTransaction(ctx, tx); err != nil {
		wm.logger.Error("Failed to update transaction with Fireblocks ID", zap.Error(err))
	}

	// Publish processing event
	if err := wm.publishEvent(ctx, &interfaces.WithdrawalProcessingEvent{
		TransactionID: tx.ID,
		UserID:        tx.UserID,
		Asset:         tx.Asset,
		Amount:        tx.Amount,
		FireblocksID:  fbTx.ID,
		Timestamp:     time.Now(),
	}); err != nil {
		wm.logger.Warn("Failed to publish withdrawal processing event", zap.Error(err))
	}

	return nil
}

func (wm *WithdrawalManager) completeWithdrawal(ctx context.Context, tx *interfaces.WalletTransaction) error {
	wm.logger.Info("Completing withdrawal", zap.String("tx_id", tx.ID.String()))

	return wm.db.Transaction(func(dbTx *gorm.DB) error {
		// Deduct balance (already locked, so this removes from total)
		fee, _ := tx.Metadata["fee"].(decimal.Decimal)
		totalAmount := tx.Amount.Add(fee)

		if err := wm.balanceManager.DebitBalance(ctx, dbTx, tx.UserID, tx.Asset, totalAmount, tx.ID.String()); err != nil {
			return fmt.Errorf("failed to debit balance: %w", err)
		}

		// Release fund lock
		if lockID, ok := tx.Metadata["lock_id"].(string); ok {
			if err := wm.fundLockService.ReleaseLock(ctx, lockID); err != nil {
				wm.logger.Warn("Failed to release fund lock", zap.String("lock_id", lockID), zap.Error(err))
			}
		}

		// Log audit event
		auditEvent := &complianceInterfaces.AuditEvent{
			ID:        uuid.New(),
			EventType: "withdrawal_completed",
			UserID:    tx.UserID,
			Details: map[string]interface{}{
				"transaction_id": tx.ID,
				"asset":          tx.Asset,
				"amount":         tx.Amount,
				"fee":            fee,
				"to_address":     tx.ToAddress,
				"tx_hash":        tx.TxHash,
			},
			Timestamp: time.Now(),
		}

		if err := wm.auditService.LogEvent(ctx, auditEvent); err != nil {
			wm.logger.Warn("Failed to log audit event", zap.Error(err))
		}

		// Publish completion event
		if err := wm.publishEvent(ctx, &interfaces.WithdrawalCompletedEvent{
			TransactionID: tx.ID,
			UserID:        tx.UserID,
			Asset:         tx.Asset,
			Amount:        tx.Amount,
			Fee:           fee,
			TxHash:        tx.TxHash,
			ToAddress:     tx.ToAddress,
			Timestamp:     time.Now(),
		}); err != nil {
			wm.logger.Warn("Failed to publish withdrawal completed event", zap.Error(err))
		}

		return nil
	})
}

func (wm *WithdrawalManager) failWithdrawal(ctx context.Context, tx *interfaces.WalletTransaction) error {
	wm.logger.Info("Failing withdrawal", zap.String("tx_id", tx.ID.String()))

	// Release fund lock
	if lockID, ok := tx.Metadata["lock_id"].(string); ok {
		if err := wm.fundLockService.ReleaseLock(ctx, lockID); err != nil {
			wm.logger.Warn("Failed to release fund lock", zap.String("lock_id", lockID), zap.Error(err))
		}
	}

	// Publish failure event
	if err := wm.publishEvent(ctx, &interfaces.WithdrawalFailedEvent{
		TransactionID: tx.ID,
		UserID:        tx.UserID,
		Asset:         tx.Asset,
		Amount:        tx.Amount,
		Reason:        tx.ErrorMsg,
		Timestamp:     time.Now(),
	}); err != nil {
		wm.logger.Warn("Failed to publish withdrawal failed event", zap.Error(err))
	}

	return nil
}

func (wm *WithdrawalManager) validateWithdrawal(ctx context.Context, req *interfaces.WithdrawalRequest) error {
	// Validate amount
	if req.Amount.LessThanOrEqual(decimal.Zero) {
		return fmt.Errorf("withdrawal amount must be positive")
	}

	// Check minimum withdrawal amount
	if minAmount, exists := wm.config.MinWithdrawalAmount[req.Asset]; exists {
		if req.Amount.LessThan(minAmount) {
			return fmt.Errorf("withdrawal amount below minimum: %s", minAmount.String())
		}
	}

	// Check maximum withdrawal amount
	if maxAmount, exists := wm.config.MaxWithdrawalAmount[req.Asset]; exists {
		if req.Amount.GreaterThan(maxAmount) {
			return fmt.Errorf("withdrawal amount exceeds maximum: %s", maxAmount.String())
		}
	}

	// Check daily withdrawal limit
	if err := wm.checkDailyLimit(ctx, req.UserID, req.Asset, req.Amount); err != nil {
		return err
	}

	// Validate address
	addressValidation := &interfaces.AddressValidationRequest{
		Address: req.ToAddress,
		Asset:   req.Asset,
		Network: req.Network,
		Tag:     req.Tag,
	}

	result, err := wm.addressManager.ValidateAddress(ctx, addressValidation)
	if err != nil {
		return fmt.Errorf("address validation failed: %w", err)
	}

	if !result.IsValid {
		return fmt.Errorf("invalid withdrawal address: %s", result.Reason)
	}

	// Check sufficient balance
	balance, err := wm.balanceManager.GetBalance(ctx, req.UserID, req.Asset)
	if err != nil {
		return fmt.Errorf("failed to check balance: %w", err)
	}

	fee := wm.calculateWithdrawalFee(req.Asset, req.Amount)
	totalRequired := req.Amount.Add(fee)

	if balance.Available.LessThan(totalRequired) {
		return fmt.Errorf("insufficient balance: required %s, available %s",
			totalRequired.String(), balance.Available.String())
	}

	return nil
}

func (wm *WithdrawalManager) checkDailyLimit(ctx context.Context, userID uuid.UUID, asset string, amount decimal.Decimal) error {
	dailyLimit, exists := wm.config.DailyWithdrawalLimit[asset]
	if !exists {
		return nil // No limit configured
	}

	// Get today's withdrawals
	today := time.Now().Truncate(24 * time.Hour)
	dailyTotal, err := wm.repository.GetDailyWithdrawalTotal(ctx, userID, asset, today)
	if err != nil {
		return fmt.Errorf("failed to check daily withdrawal total: %w", err)
	}

	if dailyTotal.Add(amount).GreaterThan(dailyLimit) {
		return fmt.Errorf("daily withdrawal limit exceeded: limit %s, current %s, requested %s",
			dailyLimit.String(), dailyTotal.String(), amount.String())
	}

	return nil
}

func (wm *WithdrawalManager) calculateWithdrawalFee(asset string, amount decimal.Decimal) decimal.Decimal {
	if fee, exists := wm.config.WithdrawalFee[asset]; exists {
		return fee
	}
	return decimal.Zero // No fee
}

func (wm *WithdrawalManager) requiresApproval(asset string, amount decimal.Decimal) bool {
	if required, exists := wm.config.RequireApproval[asset]; exists {
		return required
	}
	return amount.GreaterThan(wm.config.ComplianceThreshold)
}

func (wm *WithdrawalManager) canCancelWithdrawal(status interfaces.TxStatus) bool {
	return status == interfaces.TxStatusInitiated ||
		status == interfaces.TxStatusPending
}

func (wm *WithdrawalManager) estimateWithdrawalTime(asset string, priority interfaces.WithdrawalPriority) time.Duration {
	base := map[string]time.Duration{
		"BTC": 60 * time.Minute,
		"ETH": 10 * time.Minute,
	}

	baseTime, exists := base[asset]
	if !exists {
		baseTime = 30 * time.Minute
	}

	// Adjust based on priority
	switch priority {
	case interfaces.PriorityUrgent:
		return baseTime / 4
	case interfaces.PriorityHigh:
		return baseTime / 2
	case interfaces.PriorityMedium:
		return baseTime
	case interfaces.PriorityLow:
		return baseTime * 2
	default:
		return baseTime
	}
}

func (wm *WithdrawalManager) mapFireblocksStatus(status string) interfaces.TxStatus {
	switch status {
	case "SUBMITTED":
		return interfaces.TxStatusPending
	case "PENDING_SIGNATURE", "PENDING_AUTHORIZATION":
		return interfaces.TxStatusPending
	case "BROADCASTING":
		return interfaces.TxStatusPending
	case "CONFIRMING":
		return interfaces.TxStatusConfirming
	case "CONFIRMED", "COMPLETED":
		return interfaces.TxStatusCompleted
	case "FAILED":
		return interfaces.TxStatusFailed
	case "REJECTED":
		return interfaces.TxStatusRejected
	case "CANCELLED":
		return interfaces.TxStatusCancelled
	default:
		return interfaces.TxStatusPending
	}
}

func (wm *WithdrawalManager) runComplianceCheck(ctx context.Context, txID uuid.UUID) {
	// Run compliance check in background
	result, err := wm.complianceService.CheckTransaction(ctx, txID.String())
	if err != nil {
		wm.logger.Error("Compliance check failed", zap.String("tx_id", txID.String()), zap.Error(err))
		return
	}

	// Update transaction with compliance result
	tx, err := wm.repository.GetTransaction(ctx, txID)
	if err != nil {
		wm.logger.Error("Failed to get transaction for compliance update", zap.Error(err))
		return
	}

	tx.ComplianceCheck = result.Status
	if result.Status == "rejected" {
		tx.Status = interfaces.TxStatusRejected
		tx.ErrorMsg = result.Reason
	} else if result.Status == "approved" {
		// Process the withdrawal
		go wm.processWithdrawal(context.Background(), txID)
	}
	tx.UpdatedAt = time.Now()

	if err := wm.repository.UpdateTransaction(ctx, tx); err != nil {
		wm.logger.Error("Failed to update transaction with compliance result", zap.Error(err))
	}
}

func (wm *WithdrawalManager) publishEvent(ctx context.Context, event interface{}) error {
	if wm.eventPublisher != nil {
		return wm.eventPublisher.Publish(ctx, event)
	}
	return nil
}

// GetWithdrawalLimits returns withdrawal limits for a user and asset
func (wm *WithdrawalManager) GetWithdrawalLimits(ctx context.Context, userID uuid.UUID, asset string) (*interfaces.WithdrawalLimits, error) {
	// TODO: Implement proper daily/monthly withdrawal tracking
	// For now, return default limits

	// Get asset-specific limits from config
	minAmount := decimal.NewFromFloat(0.001)
	maxAmount := decimal.NewFromFloat(100000)
	dailyLimit := decimal.NewFromFloat(10000)
	monthlyLimit := decimal.NewFromFloat(100000)
	networkFee := decimal.Zero

	if minAmountConfig, exists := wm.config.MinWithdrawalAmount[asset]; exists {
		minAmount = minAmountConfig
	}
	if maxAmountConfig, exists := wm.config.MaxWithdrawalAmount[asset]; exists {
		maxAmount = maxAmountConfig
	}
	if dailyLimitConfig, exists := wm.config.DailyWithdrawalLimit[asset]; exists {
		dailyLimit = dailyLimitConfig
	}
	if feeConfig, exists := wm.config.WithdrawalFee[asset]; exists {
		networkFee = feeConfig
	}

	// TODO: Get actual daily and monthly used amounts from repository
	dailyUsed := decimal.Zero
	monthlyUsed := decimal.Zero

	// Calculate remaining limits
	dailyRemaining := dailyLimit.Sub(dailyUsed)
	if dailyRemaining.IsNegative() {
		dailyRemaining = decimal.Zero
	}

	monthlyRemaining := monthlyLimit.Sub(monthlyUsed)
	if monthlyRemaining.IsNegative() {
		monthlyRemaining = decimal.Zero
	}

	return &interfaces.WithdrawalLimits{
		Asset:            asset,
		DailyLimit:       dailyLimit,
		DailyUsed:        dailyUsed,
		DailyRemaining:   dailyRemaining,
		MonthlyLimit:     monthlyLimit,
		MonthlyUsed:      monthlyUsed,
		MonthlyRemaining: monthlyRemaining,
		MinWithdrawal:    minAmount,
		MaxWithdrawal:    maxAmount,
		NetworkFee:       networkFee,
		MaintenanceMode:  false, // TODO: Get from config
	}, nil
}
