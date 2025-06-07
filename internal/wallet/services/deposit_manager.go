// Package services provides deposit management functionality
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

// DepositManager handles all deposit-related operations
type DepositManager struct {
	db                *gorm.DB
	logger            *zap.Logger
	fireblocksClient  interfaces.FireblocksClient
	balanceManager    interfaces.BalanceManager
	fundLockService   interfaces.FundLockService
	complianceService complianceInterfaces.ComplianceService
	auditService      complianceInterfaces.AuditService
	eventPublisher    interfaces.EventPublisher
	repository        interfaces.WalletRepository
	cache             interfaces.WalletCache
	config            *DepositConfig
}

// DepositConfig holds deposit-specific configuration
type DepositConfig struct {
	MinConfirmations     map[string]int // asset -> min confirmations
	MaxPendingDeposits   int
	DepositTimeout       time.Duration
	AutoProcessThreshold decimal.Decimal
	ComplianceRequired   bool
}

// NewDepositManager creates a new deposit manager
func NewDepositManager(
	db *gorm.DB,
	logger *zap.Logger,
	fireblocksClient interfaces.FireblocksClient,
	balanceManager interfaces.BalanceManager,
	fundLockService interfaces.FundLockService,
	complianceService complianceInterfaces.ComplianceService,
	auditService complianceInterfaces.AuditService,
	eventPublisher interfaces.EventPublisher,
	repository interfaces.WalletRepository,
	cache interfaces.WalletCache,
	config *DepositConfig,
) *DepositManager {
	return &DepositManager{
		db:                db,
		logger:            logger,
		fireblocksClient:  fireblocksClient,
		balanceManager:    balanceManager,
		fundLockService:   fundLockService,
		complianceService: complianceService,
		auditService:      auditService,
		eventPublisher:    eventPublisher,
		repository:        repository,
		cache:             cache,
		config:            config,
	}
}

// InitiateDeposit starts a new deposit process
func (dm *DepositManager) InitiateDeposit(ctx context.Context, req *interfaces.DepositRequest) (*interfaces.DepositResponse, error) {
	dm.logger.Info("Initiating deposit",
		zap.String("user_id", req.UserID.String()),
		zap.String("asset", req.Asset),
		zap.String("amount", req.Amount.String()),
	)

	// Create deposit transaction record
	tx := &interfaces.WalletTransaction{
		ID:              uuid.New(),
		UserID:          req.UserID,
		Asset:           req.Asset,
		Amount:          req.Amount,
		Direction:       interfaces.DirectionDeposit,
		Status:          interfaces.TxStatusInitiated,
		ToAddress:       req.ToAddress,
		Network:         req.Network,
		RequiredConf:    dm.getMinConfirmations(req.Asset),
		ComplianceCheck: "pending",
		Metadata:        req.Metadata,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	// Store transaction
	if err := dm.repository.CreateTransaction(ctx, tx); err != nil {
		dm.logger.Error("Failed to create deposit transaction", zap.Error(err))
		return nil, fmt.Errorf("failed to create deposit transaction: %w", err)
	}
	// Publish deposit initiated event
	if err := dm.publishEvent(ctx, &interfaces.DepositInitiatedEvent{
		ID:            uuid.New(),
		TransactionID: tx.ID,
		UserID:        req.UserID,
		Asset:         req.Asset,
		Amount:        req.Amount,
		ToAddress:     req.ToAddress,
		Timestamp:     time.Now(),
	}); err != nil {
		dm.logger.Warn("Failed to publish deposit initiated event", zap.Error(err))
	}

	// Start compliance check if required
	if dm.config.ComplianceRequired {
		go dm.runComplianceCheck(context.Background(), tx.ID)
	}
	return &interfaces.DepositResponse{
		TransactionID: tx.ID,
		Status:        tx.Status,
		Address:       nil, // Will be set if address generation is needed
		Network:       req.Network,
		EstimatedTime: dm.estimateDepositTime(req.Asset),
		CreatedAt:     tx.CreatedAt,
	}, nil
}

// ProcessIncomingDeposit handles deposit notifications from Fireblocks
func (dm *DepositManager) ProcessIncomingDeposit(ctx context.Context, data *interfaces.FireblocksWebhookData) error {
	dm.logger.Info("Processing incoming deposit",
		zap.String("fireblocks_id", data.ID),
		zap.String("tx_hash", data.TxHash),
	)

	// Find or create transaction record
	tx, err := dm.findOrCreateTransaction(ctx, data)
	if err != nil {
		return fmt.Errorf("failed to find/create transaction: %w", err)
	}

	// Update transaction with Fireblocks data	tx.FireblocksID = data.ID
	tx.TxHash = data.TxHash
	// Note: Source address not available in webhook data structure
	tx.FromAddress = "" // Will be populated from blockchain data if needed
	// Note: NumOfConfirmations not available in webhook data structure
	tx.Confirmations = 0 // Will be updated based on network data
	tx.Status = dm.mapFireblocksStatus(data.Status)
	tx.UpdatedAt = time.Now()

	// Save updated transaction
	if err := dm.repository.UpdateTransaction(ctx, tx.ID, map[string]interface{}{
		"fireblocks_id": tx.FireblocksID,
		"tx_hash":       tx.TxHash,
		"from_address":  tx.FromAddress,
		"confirmations": tx.Confirmations,
		"status":        tx.Status,
		"updated_at":    tx.UpdatedAt,
	}); err != nil {
		return fmt.Errorf("failed to update transaction: %w", err)
	}

	// Check if deposit should be confirmed
	if tx.Confirmations >= tx.RequiredConf && tx.Status == interfaces.TxStatusConfirming {
		return dm.confirmDeposit(ctx, tx)
	}

	return nil
}

// ConfirmDeposit finalizes a deposit after sufficient confirmations
func (dm *DepositManager) ConfirmDeposit(ctx context.Context, txID uuid.UUID, confirmations int) error {
	dm.logger.Info("Confirming deposit",
		zap.String("tx_id", txID.String()),
		zap.Int("confirmations", confirmations),
	)

	// Get transaction
	tx, err := dm.repository.GetTransaction(ctx, txID)
	if err != nil {
		return fmt.Errorf("failed to get transaction: %w", err)
	}

	if tx.Direction != interfaces.DirectionDeposit {
		return fmt.Errorf("transaction is not a deposit")
	}

	// Check if already confirmed
	if tx.Status == interfaces.TxStatusCompleted {
		return nil // Already confirmed
	}

	// Update confirmations
	tx.Confirmations = confirmations
	tx.UpdatedAt = time.Now()

	// Check if we have enough confirmations
	if confirmations >= tx.RequiredConf {
		return dm.confirmDeposit(ctx, tx)
	}

	// Update status to confirming
	tx.Status = interfaces.TxStatusConfirming
	return dm.repository.UpdateTransaction(ctx, tx)
}

// ValidateDeposit performs validation checks on a deposit
func (dm *DepositManager) ValidateDeposit(ctx context.Context, req *interfaces.DepositRequest) error {
	// Validate amount
	if req.Amount.LessThanOrEqual(decimal.Zero) {
		return fmt.Errorf("deposit amount must be positive")
	}

	// Validate asset
	if req.Asset == "" {
		return fmt.Errorf("asset is required")
	}

	// Validate network
	if req.Network == "" {
		return fmt.Errorf("network is required")
	}

	// Validate address
	if req.ToAddress == "" {
		return fmt.Errorf("deposit address is required")
	}

	// Check if user has too many pending deposits
	pendingCount, err := dm.repository.CountPendingDeposits(ctx, req.UserID)
	if err != nil {
		return fmt.Errorf("failed to check pending deposits: %w", err)
	}

	if pendingCount >= dm.config.MaxPendingDeposits {
		return fmt.Errorf("too many pending deposits")
	}

	return nil
}

// GetDepositHistory retrieves deposit history for a user
func (dm *DepositManager) GetDepositHistory(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*interfaces.WalletTransaction, error) {
	return dm.repository.GetUserTransactionsByDirection(ctx, userID, interfaces.DirectionDeposit, limit, offset)
}

// Private helper methods

func (dm *DepositManager) confirmDeposit(ctx context.Context, tx *interfaces.WalletTransaction) error {
	// Start database transaction
	return dm.db.Transaction(func(dbTx *gorm.DB) error {
		// Update transaction status
		tx.Status = interfaces.TxStatusCompleted
		tx.UpdatedAt = time.Now()

		if err := dm.repository.UpdateTransactionInTx(ctx, dbTx, tx); err != nil {
			return fmt.Errorf("failed to update transaction: %w", err)
		}

		// Update user balance
		if err := dm.balanceManager.CreditBalance(ctx, dbTx, tx.UserID, tx.Asset, tx.Amount, tx.ID.String()); err != nil {
			return fmt.Errorf("failed to credit balance: %w", err)
		}

		// Log audit event
		auditEvent := &complianceInterfaces.AuditEvent{
			ID:        uuid.New(),
			EventType: "deposit_confirmed",
			UserID:    tx.UserID,
			Details: map[string]interface{}{
				"transaction_id": tx.ID,
				"asset":          tx.Asset,
				"amount":         tx.Amount,
				"confirmations":  tx.Confirmations,
				"tx_hash":        tx.TxHash,
			},
			Timestamp: time.Now(),
		}

		if err := dm.auditService.LogEvent(ctx, auditEvent); err != nil {
			dm.logger.Warn("Failed to log audit event", zap.Error(err))
		}

		// Publish deposit confirmed event
		if err := dm.publishEvent(ctx, &interfaces.DepositConfirmedEvent{
			TransactionID: tx.ID,
			UserID:        tx.UserID,
			Asset:         tx.Asset,
			Amount:        tx.Amount,
			TxHash:        tx.TxHash,
			Confirmations: tx.Confirmations,
			Timestamp:     time.Now(),
		}); err != nil {
			dm.logger.Warn("Failed to publish deposit confirmed event", zap.Error(err))
		}

		return nil
	})
}

func (dm *DepositManager) findOrCreateTransaction(ctx context.Context, data *interfaces.FireblocksWebhookData) (*interfaces.WalletTransaction, error) {
	// Try to find existing transaction by Fireblocks ID
	if tx, err := dm.repository.GetTransactionByFireblocksID(ctx, data.ID); err == nil {
		return tx, nil
	}

	// Try to find by transaction hash
	if data.TxHash != "" {
		if tx, err := dm.repository.GetTransactionByTxHash(ctx, data.TxHash); err == nil {
			return tx, nil
		}
	}

	// Create new transaction record
	tx := &interfaces.WalletTransaction{
		ID:           uuid.New(),
		Asset:        data.AssetID,
		Amount:       data.Amount,
		Direction:    interfaces.DirectionDeposit,
		Status:       interfaces.TxStatusPending,
		FireblocksID: data.ID,
		TxHash:       data.TxHash,
		FromAddress:  data.Source.Address,
		ToAddress:    data.Destination.Address,
		Network:      data.NetworkRecord.Name,
		RequiredConf: dm.getMinConfirmations(data.AssetID),
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := dm.repository.CreateTransaction(ctx, tx); err != nil {
		return nil, err
	}

	return tx, nil
}

func (dm *DepositManager) mapFireblocksStatus(status string) interfaces.TxStatus {
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
		return interfaces.TxStatusConfirmed
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

func (dm *DepositManager) getMinConfirmations(asset string) int {
	if conf, exists := dm.config.MinConfirmations[asset]; exists {
		return conf
	}
	return 1 // Default
}

func (dm *DepositManager) estimateDepositTime(asset string) time.Duration {
	// Asset-specific time estimates
	switch asset {
	case "BTC":
		return 30 * time.Minute
	case "ETH":
		return 5 * time.Minute
	default:
		return 15 * time.Minute
	}
}

func (dm *DepositManager) runComplianceCheck(ctx context.Context, txID uuid.UUID) {
	// Run compliance check in background
	result, err := dm.complianceService.CheckTransaction(ctx, txID.String())
	if err != nil {
		dm.logger.Error("Compliance check failed", zap.String("tx_id", txID.String()), zap.Error(err))
		return
	}

	// Update transaction with compliance result
	tx, err := dm.repository.GetTransaction(ctx, txID)
	if err != nil {
		dm.logger.Error("Failed to get transaction for compliance update", zap.Error(err))
		return
	}

	tx.ComplianceCheck = result.Status
	if result.Status == "rejected" {
		tx.Status = interfaces.TxStatusRejected
		tx.ErrorMsg = result.Reason
	}
	tx.UpdatedAt = time.Now()

	if err := dm.repository.UpdateTransaction(ctx, tx); err != nil {
		dm.logger.Error("Failed to update transaction with compliance result", zap.Error(err))
	}
}

func (dm *DepositManager) publishEvent(ctx context.Context, event interface{}) error {
	if dm.eventPublisher != nil {
		return dm.eventPublisher.Publish(ctx, event)
	}
	return nil
}

// CompleteDeposit completes a deposit transaction and credits the user's account
func (dm *DepositManager) CompleteDeposit(ctx context.Context, txID uuid.UUID) error {
	// Get the transaction
	tx, err := dm.repository.GetTransaction(ctx, txID)
	if err != nil {
		return fmt.Errorf("failed to get transaction: %w", err)
	}

	// Verify transaction is ready to be completed
	if tx.Status != interfaces.TxStatusConfirmed {
		return fmt.Errorf("transaction is not ready for completion, status: %s", tx.Status)
	}

	// Start database transaction
	return dm.db.Transaction(func(dbTx *gorm.DB) error {
		// Update transaction status
		tx.Status = interfaces.TxStatusCompleted
		tx.UpdatedAt = time.Now()

		updates := map[string]interface{}{
			"status":     tx.Status,
			"updated_at": tx.UpdatedAt,
		}

		if err := dm.repository.UpdateTransaction(ctx, tx.ID, updates); err != nil {
			return fmt.Errorf("failed to update transaction status: %w", err)
		}
		// Credit user's balance
		if err := dm.balanceManager.UpdateBalance(ctx, tx.UserID, tx.Asset, tx.Amount, "deposit", tx.ID.String()); err != nil {
			return fmt.Errorf("failed to credit user balance: %w", err)
		}

		// Log the completion
		dm.logger.Info("Deposit completed successfully",
			zap.String("tx_id", tx.ID.String()),
			zap.String("user_id", tx.UserID.String()),
			zap.String("asset", tx.Asset),
			zap.String("amount", tx.Amount.String()),
		)
		// Publish deposit completed event
		event := &interfaces.DepositCompletedEvent{
			ID:            uuid.New(),
			TransactionID: tx.ID,
			UserID:        tx.UserID,
			Asset:         tx.Asset,
			Amount:        tx.Amount,
			TxHash:        tx.TxHash,
			Timestamp:     time.Now(),
		}

		if err := dm.publishEvent(ctx, event); err != nil {
			dm.logger.Error("Failed to publish deposit completed event", zap.Error(err))
			// Don't fail the transaction for event publishing failure
		}

		return nil
	})
}
