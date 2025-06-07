// Package services provides the main wallet service implementation
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

// WalletService implements the main wallet functionality
type WalletService struct {
	// Core dependencies
	db     *gorm.DB
	logger *zap.Logger

	// Service components
	depositManager    interfaces.DepositManager
	withdrawalManager interfaces.WithdrawalManager
	fundLockService   interfaces.FundLockService
	balanceManager    interfaces.BalanceManager
	addressManager    interfaces.AddressManager
	stateMachine      interfaces.TransactionStateMachine

	// External integrations
	fireblocksClient  interfaces.FireblocksClient
	complianceService complianceInterfaces.ComplianceService
	auditService      complianceInterfaces.AuditService
	eventPublisher    interfaces.EventPublisher

	// Repository and cache
	repository interfaces.WalletRepository
	cache      interfaces.WalletCache

	// Configuration
	config *WalletConfig

	// State
	started bool
}

// WalletConfig holds wallet service configuration
type WalletConfig struct {
	// Fireblocks configuration
	Fireblocks struct {
		APIKey         string        `yaml:"api_key"`
		PrivateKeyPath string        `yaml:"private_key_path"`
		BaseURL        string        `yaml:"base_url"`
		VaultAccountID string        `yaml:"vault_account_id"`
		WebhookSecret  string        `yaml:"webhook_secret"`
		Timeout        time.Duration `yaml:"timeout"`
	} `yaml:"fireblocks"`

	// Security settings
	Security struct {
		RequireTwoFactor      bool                       `yaml:"require_two_factor"`
		WithdrawalCooldown    time.Duration              `yaml:"withdrawal_cooldown"`
		MaxDailyWithdrawal    map[string]decimal.Decimal `yaml:"max_daily_withdrawal"`
		RequiredConfirmations map[string]int             `yaml:"required_confirmations"`
	} `yaml:"security"`

	// Processing settings
	Processing struct {
		BatchSize          int           `yaml:"batch_size"`
		ProcessingInterval time.Duration `yaml:"processing_interval"`
		RetryAttempts      int           `yaml:"retry_attempts"`
		RetryBackoff       time.Duration `yaml:"retry_backoff"`
	} `yaml:"processing"`

	// Cache settings
	Cache struct {
		BalanceTTL time.Duration `yaml:"balance_ttl"`
		AddressTTL time.Duration `yaml:"address_ttl"`
		LockTTL    time.Duration `yaml:"lock_ttl"`
	} `yaml:"cache"`
}

// NewWalletService creates a new wallet service
func NewWalletService(
	db *gorm.DB,
	logger *zap.Logger,
	config *WalletConfig,
	fireblocksClient interfaces.FireblocksClient,
	complianceService complianceInterfaces.ComplianceService,
	auditService complianceInterfaces.AuditService,
	repository interfaces.WalletRepository,
	cache interfaces.WalletCache,
	eventPublisher interfaces.EventPublisher,
) *WalletService {

	service := &WalletService{
		db:                db,
		logger:            logger,
		config:            config,
		fireblocksClient:  fireblocksClient,
		complianceService: complianceService,
		auditService:      auditService,
		repository:        repository,
		cache:             cache,
		eventPublisher:    eventPublisher,
	}

	// Initialize sub-services
	service.fundLockService = NewFundLockService(repository, cache, logger)
	service.balanceManager = NewBalanceManager(repository, cache, service.fundLockService, logger)
	service.addressManager = NewAddressManager(repository, cache, fireblocksClient, logger)
	service.stateMachine = NewTransactionStateMachine(repository, logger)
	service.depositManager = NewDepositManager(service, repository, fireblocksClient, logger)
	service.withdrawalManager = NewWithdrawalManager(service, repository, fireblocksClient, logger)

	return service
}

// Start initializes the wallet service
func (w *WalletService) Start(ctx context.Context) error {
	w.logger.Info("Starting wallet service")

	// Verify Fireblocks connectivity
	if err := w.verifyFireblocksConnection(ctx); err != nil {
		return fmt.Errorf("failed to verify Fireblocks connection: %w", err)
	}

	// Start background processes
	go w.processTransactionUpdates(ctx)
	go w.cleanupExpiredLocks(ctx)

	w.started = true
	w.logger.Info("Wallet service started successfully")

	return nil
}

// Stop gracefully shuts down the wallet service
func (w *WalletService) Stop(ctx context.Context) error {
	w.logger.Info("Stopping wallet service")

	w.started = false

	w.logger.Info("Wallet service stopped")
	return nil
}

// HealthCheck performs health check
func (w *WalletService) HealthCheck(ctx context.Context) error {
	if !w.started {
		return fmt.Errorf("service not started")
	}

	// Check database connectivity
	sqlDB, err := w.db.DB()
	if err != nil {
		return fmt.Errorf("failed to get database connection: %w", err)
	}

	if err := sqlDB.Ping(); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}

	// Check Fireblocks connectivity
	if err := w.verifyFireblocksConnection(ctx); err != nil {
		return fmt.Errorf("Fireblocks connectivity check failed: %w", err)
	}

	return nil
}

// Deposit operations

// RequestDeposit handles deposit requests
func (w *WalletService) RequestDeposit(ctx context.Context, req *interfaces.DepositRequest) (*interfaces.DepositResponse, error) {
	w.logger.Info("Processing deposit request",
		zap.String("user_id", req.UserID.String()),
		zap.String("asset", req.Asset),
		zap.String("network", req.Network))

	// Validate request
	if err := w.validateDepositRequest(req); err != nil {
		return nil, fmt.Errorf("invalid deposit request: %w", err)
	}

	// Check compliance
	complianceReq := &interfaces.ComplianceCheckRequest{
		UserID:    req.UserID,
		Asset:     req.Asset,
		Direction: interfaces.DirectionDeposit,
	}

	complianceResult, err := w.checkCompliance(ctx, complianceReq)
	if err != nil {
		return nil, fmt.Errorf("compliance check failed: %w", err)
	}

	if !complianceResult.Approved {
		return nil, fmt.Errorf("deposit not approved: %s", complianceResult.Reason)
	}

	// Delegate to deposit manager
	return w.depositManager.InitiateDeposit(ctx, req)
}

// ProcessDeposit processes incoming deposit from Fireblocks
func (w *WalletService) ProcessDeposit(ctx context.Context, txID uuid.UUID, fireblocksData *interfaces.FireblocksData) error {
	return w.depositManager.ProcessIncomingDeposit(ctx, fireblocksData)
}

// ConfirmDeposit updates deposit confirmations
func (w *WalletService) ConfirmDeposit(ctx context.Context, txID uuid.UUID, confirmations int) error {
	return w.depositManager.UpdateConfirmations(ctx, txID, confirmations)
}

// Withdrawal operations

// RequestWithdrawal handles withdrawal requests
func (w *WalletService) RequestWithdrawal(ctx context.Context, req *interfaces.WithdrawalRequest) (*interfaces.WithdrawalResponse, error) {
	w.logger.Info("Processing withdrawal request",
		zap.String("user_id", req.UserID.String()),
		zap.String("asset", req.Asset),
		zap.String("amount", req.Amount.String()),
		zap.String("to_address", req.ToAddress))

	// Validate request
	if err := w.validateWithdrawalRequest(req); err != nil {
		return nil, fmt.Errorf("invalid withdrawal request: %w", err)
	}

	// Check available balance
	availableBalance, err := w.fundLockService.GetAvailableBalance(ctx, req.UserID, req.Asset)
	if err != nil {
		return nil, fmt.Errorf("failed to get available balance: %w", err)
	}

	if req.Amount.GreaterThan(availableBalance) {
		return nil, fmt.Errorf("insufficient balance: available %s, requested %s", availableBalance.String(), req.Amount.String())
	}

	// Check compliance
	complianceReq := &interfaces.ComplianceCheckRequest{
		UserID:    req.UserID,
		Asset:     req.Asset,
		Amount:    req.Amount,
		Direction: interfaces.DirectionWithdrawal,
		Address:   req.ToAddress,
	}

	complianceResult, err := w.checkCompliance(ctx, complianceReq)
	if err != nil {
		return nil, fmt.Errorf("compliance check failed: %w", err)
	}

	if !complianceResult.Approved {
		return nil, fmt.Errorf("withdrawal not approved: %s", complianceResult.Reason)
	}

	// Validate destination address
	addressValidation, err := w.addressManager.ValidateAddress(ctx, &interfaces.AddressValidationRequest{
		Address: req.ToAddress,
		Asset:   req.Asset,
		Network: req.Network,
	})
	if err != nil {
		return nil, fmt.Errorf("address validation failed: %w", err)
	}

	if !addressValidation.Valid {
		return nil, fmt.Errorf("invalid destination address: %s", addressValidation.Reason)
	}

	// Delegate to withdrawal manager
	return w.withdrawalManager.InitiateWithdrawal(ctx, req)
}

// ProcessWithdrawal processes approved withdrawal
func (w *WalletService) ProcessWithdrawal(ctx context.Context, txID uuid.UUID) error {
	return w.withdrawalManager.ProcessWithdrawal(ctx, txID)
}

// ConfirmWithdrawal updates withdrawal status from Fireblocks
func (w *WalletService) ConfirmWithdrawal(ctx context.Context, txID uuid.UUID, fireblocksStatus string) error {
	// Map Fireblocks status to internal status
	var status interfaces.TxStatus
	switch fireblocksStatus {
	case interfaces.TxStatusCompleted:
		status = interfaces.TxStatusCompleted
	case interfaces.TxStatusFailed:
		status = interfaces.TxStatusFailed
	case interfaces.TxStatusCancelled:
		status = interfaces.TxStatusCancelled
	case interfaces.TxStatusRejected:
		status = interfaces.TxStatusRejected
	default:
		status = interfaces.TxStatusPending
	}

	return w.withdrawalManager.UpdateStatus(ctx, txID, string(status), "")
}

// CancelWithdrawal cancels a pending withdrawal
func (w *WalletService) CancelWithdrawal(ctx context.Context, txID uuid.UUID, reason string) error {
	return w.withdrawalManager.CancelWithdrawal(ctx, txID, reason)
}

// Balance operations

// GetBalance gets balance for specific asset
func (w *WalletService) GetBalance(ctx context.Context, userID uuid.UUID, asset string) (*interfaces.AssetBalance, error) {
	return w.balanceManager.GetBalance(ctx, userID, asset)
}

// GetBalances gets all balances for user
func (w *WalletService) GetBalances(ctx context.Context, userID uuid.UUID) (*interfaces.BalanceResponse, error) {
	return w.balanceManager.GetBalances(ctx, userID)
}

// UpdateBalance updates user balance
func (w *WalletService) UpdateBalance(ctx context.Context, userID uuid.UUID, asset string, amount decimal.Decimal, txType string, txRef string) error {
	return w.balanceManager.UpdateBalance(ctx, userID, asset, amount, txType, txRef)
}

// Transaction operations

// GetTransaction gets transaction by ID
func (w *WalletService) GetTransaction(ctx context.Context, txID uuid.UUID) (*interfaces.WalletTransaction, error) {
	return w.repository.GetTransaction(ctx, txID)
}

// GetTransactionStatus gets transaction status
func (w *WalletService) GetTransactionStatus(ctx context.Context, txID uuid.UUID) (*interfaces.TransactionStatus, error) {
	tx, err := w.repository.GetTransaction(ctx, txID)
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction: %w", err)
	}

	return &interfaces.TransactionStatus{
		ID:            tx.ID,
		Status:        tx.Status,
		Confirmations: tx.Confirmations,
		Required:      tx.RequiredConf,
		TxHash:        tx.TxHash,
		ErrorMsg:      tx.ErrorMsg,
	}, nil
}

// GetUserTransactions gets user transactions
func (w *WalletService) GetUserTransactions(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*interfaces.WalletTransaction, error) {
	return w.repository.GetUserTransactions(ctx, userID, limit, offset)
}

// Address operations

// GenerateAddress generates new deposit address
func (w *WalletService) GenerateAddress(ctx context.Context, userID uuid.UUID, asset, network string) (*interfaces.DepositAddress, error) {
	return w.addressManager.GenerateAddress(ctx, userID, asset, network)
}

// GetUserAddresses gets user addresses
func (w *WalletService) GetUserAddresses(ctx context.Context, userID uuid.UUID, asset string) ([]*interfaces.DepositAddress, error) {
	return w.addressManager.GetUserAddresses(ctx, userID, asset)
}

// ValidateAddress validates address format
func (w *WalletService) ValidateAddress(ctx context.Context, req *interfaces.AddressValidationRequest) (*interfaces.AddressValidationResult, error) {
	return w.addressManager.ValidateAddress(ctx, req)
}

// Private methods

// validateDepositRequest validates deposit request
func (w *WalletService) validateDepositRequest(req *interfaces.DepositRequest) error {
	if req.UserID == uuid.Nil {
		return fmt.Errorf("user ID is required")
	}

	if req.Asset == "" {
		return fmt.Errorf("asset is required")
	}

	if req.Network == "" {
		return fmt.Errorf("network is required")
	}

	return nil
}

// validateWithdrawalRequest validates withdrawal request
func (w *WalletService) validateWithdrawalRequest(req *interfaces.WithdrawalRequest) error {
	if req.UserID == uuid.Nil {
		return fmt.Errorf("user ID is required")
	}

	if req.Asset == "" {
		return fmt.Errorf("asset is required")
	}

	if req.Amount.LessThanOrEqual(decimal.Zero) {
		return fmt.Errorf("amount must be positive")
	}

	if req.ToAddress == "" {
		return fmt.Errorf("destination address is required")
	}

	if req.Network == "" {
		return fmt.Errorf("network is required")
	}

	if req.TwoFactorToken == "" {
		return fmt.Errorf("two-factor token is required")
	}

	return nil
}

// checkCompliance performs compliance check
func (w *WalletService) checkCompliance(ctx context.Context, req *interfaces.ComplianceCheckRequest) (*interfaces.ComplianceCheckResult, error) {
	// For now, implement basic compliance check
	// In production, this would integrate with the full compliance module

	// Check if amount is within limits
	if req.Amount.GreaterThan(decimal.NewFromFloat(100000)) { // $100k limit
		return &interfaces.ComplianceCheckResult{
			Approved: false,
			Reason:   "Amount exceeds daily limit",
			Score:    100,
		}, nil
	}

	// Default approval for valid requests
	return &interfaces.ComplianceCheckResult{
		Approved: true,
		Score:    0,
	}, nil
}

// verifyFireblocksConnection verifies Fireblocks connectivity
func (w *WalletService) verifyFireblocksConnection(ctx context.Context) error {
	// Test connection by getting vault accounts
	_, err := w.fireblocksClient.GetVaultAccounts(ctx)
	return err
}

// processTransactionUpdates processes transaction status updates
func (w *WalletService) processTransactionUpdates(ctx context.Context) {
	ticker := time.NewTicker(w.config.Processing.ProcessingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.processPendingTransactions(ctx)
		}
	}
}

// processPendingTransactions processes pending transactions
func (w *WalletService) processPendingTransactions(ctx context.Context) {
	// Get pending transactions
	transactions, err := w.repository.GetTransactionsByStatus(ctx, interfaces.TxStatusPending, w.config.Processing.BatchSize)
	if err != nil {
		w.logger.Error("Failed to get pending transactions", zap.Error(err))
		return
	}

	for _, tx := range transactions {
		if err := w.updateTransactionStatus(ctx, tx); err != nil {
			w.logger.Error("Failed to update transaction status",
				zap.String("tx_id", tx.ID.String()),
				zap.Error(err))
		}
	}
}

// updateTransactionStatus updates transaction status from Fireblocks
func (w *WalletService) updateTransactionStatus(ctx context.Context, tx *interfaces.WalletTransaction) error {
	if tx.FireblocksID == "" {
		return nil // Skip transactions not submitted to Fireblocks
	}

	// Get status from Fireblocks
	fireblocksStatus, err := w.fireblocksClient.GetTransaction(ctx, tx.FireblocksID)
	if err != nil {
		return fmt.Errorf("failed to get Fireblocks transaction: %w", err)
	}

	// Update transaction status
	updates := map[string]interface{}{
		"confirmations": fireblocksStatus.NetworkRecord.NumOfConfirmations,
		"updated_at":    time.Now(),
	}

	if fireblocksStatus.TxHash != "" && tx.TxHash != fireblocksStatus.TxHash {
		updates["tx_hash"] = fireblocksStatus.TxHash
	}

	// Map Fireblocks status to internal status
	var newStatus interfaces.TxStatus
	switch fireblocksStatus.Status {
	case interfaces.TxStatusCompleted:
		newStatus = interfaces.TxStatusCompleted
	case interfaces.TxStatusFailed:
		newStatus = interfaces.TxStatusFailed
	case interfaces.TxStatusCancelled:
		newStatus = interfaces.TxStatusCancelled
	case interfaces.TxStatusRejected:
		newStatus = interfaces.TxStatusRejected
	case interfaces.TxStatusConfirming:
		newStatus = interfaces.TxStatusConfirming
	default:
		newStatus = interfaces.TxStatusPending
	}

	if newStatus != tx.Status {
		updates["status"] = string(newStatus)

		// Handle status-specific logic
		switch newStatus {
		case interfaces.TxStatusCompleted:
			if err := w.handleTransactionCompletion(ctx, tx); err != nil {
				w.logger.Error("Failed to handle transaction completion",
					zap.String("tx_id", tx.ID.String()),
					zap.Error(err))
			}
		case interfaces.TxStatusFailed, interfaces.TxStatusCancelled, interfaces.TxStatusRejected:
			if err := w.handleTransactionFailure(ctx, tx); err != nil {
				w.logger.Error("Failed to handle transaction failure",
					zap.String("tx_id", tx.ID.String()),
					zap.Error(err))
			}
		}
	}

	return w.repository.UpdateTransaction(ctx, tx.ID, updates)
}

// handleTransactionCompletion handles completed transactions
func (w *WalletService) handleTransactionCompletion(ctx context.Context, tx *interfaces.WalletTransaction) error {
	switch tx.Direction {
	case interfaces.DirectionDeposit:
		return w.depositManager.CompleteDeposit(ctx, tx.ID)
	case interfaces.DirectionWithdrawal:
		// Release fund lock
		return w.fundLockService.ReleaseLock(ctx, tx.UserID, tx.Asset, tx.Amount, "withdrawal", tx.ID.String())
	}
	return nil
}

// handleTransactionFailure handles failed transactions
func (w *WalletService) handleTransactionFailure(ctx context.Context, tx *interfaces.WalletTransaction) error {
	if tx.Direction == interfaces.DirectionWithdrawal {
		// Release fund lock for failed withdrawals
		return w.fundLockService.ReleaseLock(ctx, tx.UserID, tx.Asset, tx.Amount, "withdrawal", tx.ID.String())
	}
	return nil
}

// cleanupExpiredLocks cleans up expired fund locks
func (w *WalletService) cleanupExpiredLocks(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute) // Run every 5 minutes
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.fundLockService.CleanExpiredLocks(ctx); err != nil {
				w.logger.Error("Failed to clean expired locks", zap.Error(err))
			}
		}
	}
}
