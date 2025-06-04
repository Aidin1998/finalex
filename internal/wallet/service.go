package wallet

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/wallet/blockchain"
	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// KeyManager interface for key management operations
type KeyManager interface {
	GenerateKey(keyID string) (string, error)
	GetPublicKey(keyID string) ([]byte, error)
	Sign(keyID string, data []byte) ([]byte, error)
	DeleteKey(keyID string) error
}

// CustodyProvider interface for custody operations
type CustodyProvider interface {
	CreateWallet(asset string) (string, error)
	GetBalance(walletID string) (decimal.Decimal, error)
	Transfer(from, to string, amount decimal.Decimal) error
	CreateWithdrawal(from, to string, amount decimal.Decimal) (string, error)
}

// WalletService represents the consolidated wallet service
type WalletService struct {
	keyManager      KeyManager
	custodyProvider CustodyProvider
	blockchainMgr   *blockchain.BlockchainManager
	db              *gorm.DB
	logger          *zap.Logger
	mu              sync.RWMutex
	running         bool
}

// NewWalletService creates a new wallet service
func NewWalletService(km KeyManager, cp CustodyProvider, db *gorm.DB, logger *zap.Logger) (*WalletService, error) {
	// Create blockchain manager
	blockchainMgr, err := blockchain.NewBlockchainManager(logger, db, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create blockchain manager: %w", err)
	}

	service := &WalletService{
		keyManager:      km,
		custodyProvider: cp,
		blockchainMgr:   blockchainMgr,
		db:              db,
		logger:          logger,
	}

	// Auto-migrate wallet-related tables
	if err := db.AutoMigrate(&models.Wallet{}, &models.WalletAudit{}); err != nil {
		return nil, fmt.Errorf("failed to migrate wallet tables: %w", err)
	}

	return service, nil
}

// CreateWallet creates a new wallet (hot/warm/cold)
// Enhanced: supports multi-sig config, address whitelisting, cold storage
func (s *WalletService) CreateWallet(ctx context.Context, userID uuid.UUID, asset, walletType string, signers []string, threshold int, whitelist []string, isColdStorage bool) (*models.Wallet, error) {
	// For hot wallets, generate key using HSM/KMS
	var address string
	var err error
	if walletType == "hot" {
		keyID, err := s.keyManager.GenerateKey(asset + "-" + userID.String())
		if err != nil {
			return nil, err
		}
		pubKey, err := s.keyManager.GetPublicKey(keyID)
		if err != nil {
			return nil, err
		}
		// Convert pubKey to address (blockchain-specific)
		address = string(pubKey) // Replace with real address derivation
	} else {
		// For warm/cold, use custody provider
		address, err = s.custodyProvider.CreateWallet(asset)
		if err != nil {
			return nil, err
		}
	}
	wallet := &models.Wallet{
		ID:               uuid.New(),
		UserID:           userID,
		Type:             walletType,
		Asset:            asset,
		Address:          address,
		Balance:          0,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
		Signers:          signers,
		Threshold:        threshold,
		AddressWhitelist: whitelist,
		IsColdStorage:    isColdStorage,
	}
	if err := s.db.WithContext(ctx).Create(wallet).Error; err != nil {
		return nil, err
	}
	s.logger.Info("Created wallet", zap.String("wallet_id", wallet.ID.String()), zap.String("type", walletType))
	return wallet, nil
}

// GetBalance returns the balance for a wallet
func (s *WalletService) GetBalance(ctx context.Context, walletID string) (decimal.Decimal, error) {
	var wallet models.Wallet
	if err := s.db.WithContext(ctx).First(&wallet, "id = ?", walletID).Error; err != nil {
		return decimal.Zero, err
	}
	// Query blockchain or custody provider for actual balance
	onChain, err := s.custodyProvider.GetBalance(wallet.Address)
	if err != nil {
		return decimal.Zero, err
	}
	return onChain, nil
}

// CreateWithdrawalRequest creates a new withdrawal request (multi-sig, whitelist enforced)
func (s *WalletService) CreateWithdrawalRequest(ctx context.Context, userID uuid.UUID, walletID, asset, toAddress string, amount float64) (*models.WithdrawalRequest, error) {
	var wallet models.Wallet
	if err := s.db.WithContext(ctx).First(&wallet, "id = ?", walletID).Error; err != nil {
		return nil, err
	}
	// Enforce address whitelisting
	if len(wallet.AddressWhitelist) > 0 {
		whitelisted := false
		for _, addr := range wallet.AddressWhitelist {
			if addr == toAddress {
				whitelisted = true
				break
			}
		}
		if !whitelisted {
			s.logger.Warn("Withdrawal address not whitelisted", zap.String("wallet_id", walletID), zap.String("to_address", toAddress))
			return nil, errors.New("withdrawal address not whitelisted")
		}
	}

	wr := &models.WithdrawalRequest{
		ID:        uuid.New(),
		UserID:    userID,
		WalletID:  walletID,
		Asset:     asset,
		Amount:    amount,
		ToAddress: toAddress,
		Status:    "pending",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := s.db.WithContext(ctx).Create(wr).Error; err != nil {
		return nil, err
	}
	s.logger.Info("Created withdrawal request", zap.String("request_id", wr.ID.String()))
	return wr, nil
}

// ApproveWithdrawal approves a withdrawal (multi-sig)
func (s *WalletService) ApproveWithdrawal(ctx context.Context, requestID uuid.UUID, approver string) error {
	var wr models.WithdrawalRequest
	if err := s.db.WithContext(ctx).First(&wr, "id = ?", requestID).Error; err != nil {
		return err
	}
	approval := &models.Approval{
		ID:        uuid.New(),
		RequestID: wr.ID,
		Approver:  approver,
		Approved:  true,
		Timestamp: time.Now(),
	}
	if err := s.db.WithContext(ctx).Create(approval).Error; err != nil {
		return err
	}
	// Check if threshold met (e.g., 2 of 3)
	var count int64
	s.db.Model(&models.Approval{}).Where("request_id = ? AND approved = ?", wr.ID, true).Count(&count)
	if count >= 2 { // Example threshold
		wr.Status = "approved"
		s.db.Save(&wr)
		// Optionally auto-broadcast here
	}
	s.logger.Info("Withdrawal approved", zap.String("request_id", wr.ID.String()), zap.String("approver", approver))
	return nil
}

// BroadcastWithdrawal broadcasts the withdrawal to the blockchain
func (s *WalletService) BroadcastWithdrawal(ctx context.Context, requestID uuid.UUID) (string, error) {
	var wr models.WithdrawalRequest
	if err := s.db.WithContext(ctx).First(&wr, "id = ?", requestID).Error; err != nil {
		return "", err
	}
	if wr.Status != "approved" {
		return "", errors.New("not enough approvals")
	}
	// Lookup wallet to get address
	var wallet models.Wallet
	if err := s.db.WithContext(ctx).First(&wallet, "id = ?", wr.WalletID).Error; err != nil {
		return "", err
	}
	// Call custody provider to create withdrawal using wallet address as key
	// Convert float64 to decimal.Decimal for the custody provider
	decimalAmount := decimal.NewFromFloat(wr.Amount)
	txid, err := s.custodyProvider.CreateWithdrawal(wallet.Address, wr.ToAddress, decimalAmount)
	if err != nil {
		return "", err
	}
	wr.Status = "broadcasted"
	wr.UpdatedAt = time.Now()
	s.db.Save(&wr)
	s.logger.Info("Withdrawal broadcasted", zap.String("request_id", wr.ID.String()), zap.String("txid", txid))
	return txid, nil
}

// ReconcileBalances checks on-chain vs. internal balances
func (s *WalletService) ReconcileBalances(ctx context.Context) error {
	var wallets []models.Wallet
	if err := s.db.Find(&wallets).Error; err != nil {
		return err
	}
	for _, w := range wallets {
		onChain, err := s.custodyProvider.GetBalance(w.Address)
		if err != nil {
			s.logger.Error("Failed to get on-chain balance", zap.String("wallet_id", w.ID.String()), zap.Error(err))
			continue
		}
		// Convert decimal to float64 for comparison with the model's float64 field
		onChainFloat, _ := onChain.Float64()
		if w.Balance != onChainFloat {
			s.logger.Warn("Balance mismatch",
				zap.String("wallet_id", w.ID.String()),
				zap.Float64("db", w.Balance),
				zap.Float64("on_chain", onChainFloat))
			// Alert/raise incident here
		}
	}
	return nil
}

// LogAudit logs a wallet operation
func (s *WalletService) LogAudit(walletID, event, actor, details string) error {
	audit := &models.WalletAudit{
		ID:        uuid.New(),
		WalletID:  walletID,
		Event:     event,
		Actor:     actor,
		Details:   details,
		CreatedAt: time.Now(),
	}
	return s.db.Create(audit).Error
}

// ColdStorageTransfer moves funds to/from cold storage (stub)
func (s *WalletService) ColdStorageTransfer(ctx context.Context, fromWalletID, toWalletID string, amount decimal.Decimal, operator string) error {
	amountFloat, _ := amount.Float64() // Convert to float64 for logging	// TODO: Implement secure cold storage transfer logic, approval, and audit
	s.logger.Info("Cold storage transfer initiated",
		zap.String("from", fromWalletID),
		zap.String("to", toWalletID),
		zap.Float64("amount", amountFloat),
		zap.String("operator", operator))
	return nil
}

// AddWhitelistAddress adds an address to the wallet's whitelist
func (s *WalletService) AddWhitelistAddress(ctx context.Context, walletID string, address string) error {
	var wallet models.Wallet
	if err := s.db.WithContext(ctx).First(&wallet, "id = ?", walletID).Error; err != nil {
		return err
	}
	for _, addr := range wallet.AddressWhitelist {
		if addr == address {
			return nil // Already whitelisted
		}
	}
	wallet.AddressWhitelist = append(wallet.AddressWhitelist, address)
	if err := s.db.WithContext(ctx).Save(&wallet).Error; err != nil {
		return err
	}
	s.logger.Info("Added address to whitelist", zap.String("wallet_id", walletID), zap.String("address", address))
	return nil
}

// RemoveWhitelistAddress removes an address from the wallet's whitelist
func (s *WalletService) RemoveWhitelistAddress(ctx context.Context, walletID string, address string) error {
	var wallet models.Wallet
	if err := s.db.WithContext(ctx).First(&wallet, "id = ?", walletID).Error; err != nil {
		return err
	}
	newList := make([]string, 0, len(wallet.AddressWhitelist))
	for _, addr := range wallet.AddressWhitelist {
		if addr != address {
			newList = append(newList, addr)
		}
	}
	wallet.AddressWhitelist = newList
	if err := s.db.WithContext(ctx).Save(&wallet).Error; err != nil {
		return err
	}
	s.logger.Info("Removed address from whitelist", zap.String("wallet_id", walletID), zap.String("address", address))
	return nil
}

// GetBlockchainBalance gets balance from blockchain directly
func (s *WalletService) GetBlockchainBalance(ctx context.Context, network, address string) (decimal.Decimal, error) {
	return s.blockchainMgr.GetBalance(ctx, network, address)
}

// SendBlockchainTransaction sends a transaction on the blockchain
func (s *WalletService) SendBlockchainTransaction(ctx context.Context, network, from, to string, amount decimal.Decimal) (*blockchain.Transaction, error) {
	// Validate addresses
	if !s.blockchainMgr.ValidateAddress(network, from) {
		return nil, fmt.Errorf("invalid from address: %s", from)
	}
	if !s.blockchainMgr.ValidateAddress(network, to) {
		return nil, fmt.Errorf("invalid to address: %s", to)
	}

	// Log audit trail
	if err := s.LogAudit(from, "blockchain_send", "system", fmt.Sprintf("Sending %s %s to %s", amount.String(), network, to)); err != nil {
		s.logger.Error("Failed to log audit", zap.Error(err))
	}

	return s.blockchainMgr.SendTransaction(ctx, network, from, to, amount)
}

// GetTransactionHistory gets transaction history for an address
func (s *WalletService) GetTransactionHistory(ctx context.Context, network, address string, limit int) ([]*blockchain.Transaction, error) {
	return s.blockchainMgr.GetTransactionHistory(ctx, network, address, limit)
}

// GetTransaction gets a specific transaction by hash
func (s *WalletService) GetTransaction(ctx context.Context, network, txHash string) (*blockchain.Transaction, error) {
	return s.blockchainMgr.GetTransaction(ctx, network, txHash)
}

// ValidateAddress validates if an address is valid for the specified network
func (s *WalletService) ValidateAddress(network, address string) bool {
	return s.blockchainMgr.ValidateAddress(network, address)
}

// GetWallet retrieves a wallet by ID
func (s *WalletService) GetWallet(ctx context.Context, walletID string) (*models.Wallet, error) {
	var wallet models.Wallet
	result := s.db.WithContext(ctx).Where("id = ?", walletID).First(&wallet)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to get wallet: %w", result.Error)
	}
	return &wallet, nil
}

// GetWalletsByUser retrieves all wallets for a user
func (s *WalletService) GetWalletsByUser(ctx context.Context, userID uuid.UUID) ([]*models.Wallet, error) {
	var wallets []*models.Wallet
	result := s.db.WithContext(ctx).Where("user_id = ?", userID).Find(&wallets)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to get user wallets: %w", result.Error)
	}
	return wallets, nil
}

// UpdateWalletStatus updates the status of a wallet
func (s *WalletService) UpdateWalletStatus(ctx context.Context, walletID, status string) error {
	result := s.db.WithContext(ctx).Model(&models.Wallet{}).Where("id = ?", walletID).Update("status", status)
	if result.Error != nil {
		return fmt.Errorf("failed to update wallet status: %w", result.Error)
	}

	if err := s.LogAudit(walletID, "status_update", "system", fmt.Sprintf("Status changed to %s", status)); err != nil {
		s.logger.Error("Failed to log audit", zap.Error(err))
	}

	s.logger.Info("Wallet status updated",
		zap.String("wallet_id", walletID),
		zap.String("status", status))

	return nil
}

// FreezeWallet freezes a wallet (prevents all operations)
func (s *WalletService) FreezeWallet(ctx context.Context, walletID, reason string) error {
	return s.UpdateWalletStatus(ctx, walletID, "frozen")
}

// UnfreezeWallet unfreezes a wallet
func (s *WalletService) UnfreezeWallet(ctx context.Context, walletID string) error {
	return s.UpdateWalletStatus(ctx, walletID, "active")
}

// GetWalletAuditLog retrieves audit log for a wallet
func (s *WalletService) GetWalletAuditLog(ctx context.Context, walletID string, limit int) ([]*models.WalletAudit, error) {
	var audits []*models.WalletAudit
	result := s.db.WithContext(ctx).Where("wallet_id = ?", walletID).
		Order("created_at DESC").
		Limit(limit).
		Find(&audits)

	if result.Error != nil {
		return nil, fmt.Errorf("failed to get wallet audit log: %w", result.Error)
	}

	return audits, nil
}

// Start starts the wallet service
func (s *WalletService) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("wallet service is already running")
	}

	s.logger.Info("Starting wallet service")

	// Start blockchain manager
	if err := s.blockchainMgr.Start(ctx); err != nil {
		return fmt.Errorf("failed to start blockchain manager: %w", err)
	}

	s.running = true
	s.logger.Info("Wallet service started successfully")

	return nil
}

// Stop stops the wallet service
func (s *WalletService) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return fmt.Errorf("wallet service is not running")
	}

	s.logger.Info("Stopping wallet service")

	// Stop blockchain manager
	if err := s.blockchainMgr.Stop(); err != nil {
		s.logger.Error("Failed to stop blockchain manager", zap.Error(err))
	}

	s.running = false
	s.logger.Info("Wallet service stopped")

	return nil
}
