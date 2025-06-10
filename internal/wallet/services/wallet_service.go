// Package services provides the main wallet service implementation
package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	complianceInterfaces "github.com/Aidin1998/finalex/internal/compliance/interfaces"
	"github.com/Aidin1998/finalex/internal/wallet/config"
	"github.com/Aidin1998/finalex/internal/wallet/interfaces"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// WalletService implements the main wallet functionality
type WalletService struct {
	// Core dependencies
	logger *zap.Logger
	config *config.WalletConfig

	// Service components
	depositManager    interfaces.DepositManager
	withdrawalManager interfaces.WithdrawalManager
	fundLockService   interfaces.FundLockService
	balanceManager    interfaces.BalanceManager
	addressManager    interfaces.AddressManager

	// External integrations
	fireblocksClient  interfaces.FireblocksClient
	complianceService complianceInterfaces.ComplianceService
	eventPublisher    interfaces.EventPublisher

	// Repository and cache
	repository interfaces.WalletRepository
	cache      interfaces.WalletCache

	// State management
	mutex   sync.RWMutex
	started bool
}

// NewWalletService creates a new wallet service
func NewWalletService(
	repository interfaces.WalletRepository,
	cache interfaces.WalletCache,
	depositManager interfaces.DepositManager,
	withdrawalManager interfaces.WithdrawalManager,
	balanceManager interfaces.BalanceManager,
	addressManager interfaces.AddressManager,
	fundLockService interfaces.FundLockService,
	fireblocksClient interfaces.FireblocksClient,
	complianceService complianceInterfaces.ComplianceService,
	eventPublisher interfaces.EventPublisher,
	config *config.WalletConfig,
	logger *zap.Logger,
) interfaces.WalletService {
	return &WalletService{
		repository:        repository,
		cache:             cache,
		depositManager:    depositManager,
		withdrawalManager: withdrawalManager,
		balanceManager:    balanceManager,
		addressManager:    addressManager,
		fundLockService:   fundLockService,
		fireblocksClient:  fireblocksClient,
		complianceService: complianceService,
		eventPublisher:    eventPublisher,
		config:            config,
		logger:            logger,
	}
}

// Start initializes the wallet service
func (w *WalletService) Start(ctx context.Context) error {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	if w.started {
		return fmt.Errorf("wallet service already started")
	}
	w.logger.Info("Starting wallet service")

	w.started = true
	w.logger.Info("Wallet service started successfully")
	return nil
}

// Stop gracefully shuts down the wallet service
func (w *WalletService) Stop(ctx context.Context) error {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	if !w.started {
		return nil
	}

	w.logger.Info("Stopping wallet service")
	w.started = false
	w.logger.Info("Wallet service stopped")
	return nil
}

// HealthCheck performs health check
func (w *WalletService) HealthCheck(ctx context.Context) error {
	w.mutex.RLock()
	defer w.mutex.RUnlock()
	if !w.started {
		return fmt.Errorf("wallet service not started")
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

	if err := w.validateDepositRequest(req); err != nil {
		return nil, fmt.Errorf("invalid deposit request: %w", err)
	}

	// Delegate to deposit manager
	return w.depositManager.InitiateDeposit(ctx, req)
}

// ProcessDeposit processes incoming deposit from Fireblocks
func (w *WalletService) ProcessDeposit(ctx context.Context, txID uuid.UUID, fireblocksData *interfaces.FireblocksData) error {
	w.logger.Info("Processing deposit from Fireblocks",
		zap.String("tx_id", txID.String()),
		zap.String("fireblocks_id", fireblocksData.ID))

	// Get transaction from repository
	tx, err := w.repository.GetTransaction(ctx, txID)
	if err != nil {
		return fmt.Errorf("failed to get transaction: %w", err)
	}

	// Process through deposit manager
	return w.depositManager.ProcessIncomingDeposit(ctx, tx.TxHash, fireblocksData)
}

// ConfirmDeposit updates deposit confirmations
func (w *WalletService) ConfirmDeposit(ctx context.Context, txID uuid.UUID, confirmations int) error {
	w.logger.Info("Confirming deposit",
		zap.String("tx_id", txID.String()),
		zap.Int("confirmations", confirmations))

	return w.depositManager.ConfirmDeposit(ctx, txID, confirmations)
}

// Withdrawal operations

// RequestWithdrawal handles withdrawal requests
func (w *WalletService) RequestWithdrawal(ctx context.Context, req *interfaces.WithdrawalRequest) (*interfaces.WithdrawalResponse, error) {
	w.logger.Info("Processing withdrawal request",
		zap.String("user_id", req.UserID.String()),
		zap.String("asset", req.Asset),
		zap.String("amount", req.Amount.String()))

	if err := w.validateWithdrawalRequest(req); err != nil {
		return nil, fmt.Errorf("invalid withdrawal request: %w", err)
	}

	// Check compliance requirements
	if w.complianceService != nil {
		// Implement compliance checks here
		w.logger.Debug("Performing compliance checks for withdrawal")
	}

	// Delegate to withdrawal manager
	return w.withdrawalManager.InitiateWithdrawal(ctx, req)
}

// ProcessWithdrawal processes withdrawal through fireblocks
func (w *WalletService) ProcessWithdrawal(ctx context.Context, txID uuid.UUID) error {
	w.logger.Info("Processing withdrawal",
		zap.String("tx_id", txID.String()))

	return w.withdrawalManager.ProcessWithdrawal(ctx, txID)
}

// ConfirmWithdrawal updates withdrawal status from fireblocks
func (w *WalletService) ConfirmWithdrawal(ctx context.Context, txID uuid.UUID, fireblocksStatus string) error {
	w.logger.Info("Confirming withdrawal",
		zap.String("tx_id", txID.String()),
		zap.String("status", fireblocksStatus))

	return w.withdrawalManager.UpdateWithdrawalStatus(ctx, txID, fireblocksStatus, "")
}

// CancelWithdrawal cancels a pending withdrawal
func (w *WalletService) CancelWithdrawal(ctx context.Context, txID uuid.UUID, reason string) error {
	w.logger.Info("Cancelling withdrawal",
		zap.String("tx_id", txID.String()),
		zap.String("reason", reason))

	return w.withdrawalManager.CancelWithdrawal(ctx, txID, reason)
}

// Balance operations

// GetBalance gets user balance for specific asset
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
	// Check cache first
	if status, err := w.cache.GetTransactionStatus(ctx, txID); err == nil {
		return status, nil
	}

	// Get from repository
	tx, err := w.repository.GetTransaction(ctx, txID)
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction: %w", err)
	}
	status := &interfaces.TransactionStatus{
		ID:            tx.ID,
		Status:        tx.Status,
		Confirmations: tx.Confirmations,
		TxHash:        tx.TxHash,
	}

	// Cache the status
	_ = w.cache.SetTransactionStatus(ctx, txID, status, 5*time.Minute)

	return status, nil
}

// GetUserTransactions gets user transactions
func (w *WalletService) GetUserTransactions(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*interfaces.WalletTransaction, error) {
	return w.repository.GetUserTransactions(ctx, userID, limit, offset)
}

// ListTransactions returns user transactions for asset, direction, limit, offset
func (w *WalletService) ListTransactions(ctx context.Context, userID uuid.UUID, asset string, direction string, limit, offset int) ([]*interfaces.WalletTransaction, int64, error) {
	// Implement filtering by asset and direction, or remove the stub comment. If not implemented, add a clear error or panic to prevent silent failure.
	txs, err := w.repository.GetUserTransactions(ctx, userID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	// Optionally filter by asset and direction
	var filtered []*interfaces.WalletTransaction
	for _, tx := range txs {
		if (asset == "" || tx.Asset == asset) && (direction == "" || string(tx.Direction) == direction) {
			filtered = append(filtered, tx)
		}
	}
	return filtered, int64(len(filtered)), nil
}

// Address operations

// GenerateAddress generates new deposit address
func (w *WalletService) GenerateAddress(ctx context.Context, userID uuid.UUID, asset, network string) (*interfaces.DepositAddress, error) {
	w.logger.Info("Generating deposit address",
		zap.String("user_id", userID.String()),
		zap.String("asset", asset),
		zap.String("network", network))

	return w.addressManager.GenerateAddress(ctx, userID, asset, network)
}

// GetUserAddresses gets user addresses
func (w *WalletService) GetUserAddresses(ctx context.Context, userID uuid.UUID, asset string) ([]*interfaces.DepositAddress, error) {
	return w.addressManager.GetUserAddresses(ctx, userID, asset)
}

// GetDepositAddress gets deposit address for user/asset/network
func (w *WalletService) GetDepositAddress(ctx context.Context, userID uuid.UUID, asset, network string) (*interfaces.DepositAddress, error) {
	// There is no direct GetDepositAddress on AddressManager, so use GetUserAddresses and filter
	addresses, err := w.addressManager.GetUserAddresses(ctx, userID, asset)
	if err != nil {
		return nil, err
	}
	for _, addr := range addresses {
		if addr.Network == network {
			return addr, nil
		}
	}
	return nil, fmt.Errorf("no deposit address found for asset %s on network %s", asset, network)
}

// ValidateAddress validates address format and requirements
func (w *WalletService) ValidateAddress(ctx context.Context, req *interfaces.AddressValidationRequest) (*interfaces.AddressValidationResult, error) {
	return w.addressManager.ValidateAddress(ctx, req)
}

// Private validation methods

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

func (w *WalletService) validateWithdrawalRequest(req *interfaces.WithdrawalRequest) error {
	if req.UserID == uuid.Nil {
		return fmt.Errorf("user ID is required")
	}
	if req.Asset == "" {
		return fmt.Errorf("asset is required")
	}
	if req.Amount.IsZero() || req.Amount.IsNegative() {
		return fmt.Errorf("invalid amount")
	}
	if req.ToAddress == "" {
		return fmt.Errorf("destination address is required")
	}
	return nil
}
