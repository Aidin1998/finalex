package wallet

import (
	"context"
	"errors"
	"time"

	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type WalletService struct {
	keyManager      KeyManager
	custodyProvider CustodyProvider
	db              *gorm.DB    // Add DB for persistence
	logger          *zap.Logger // Add logger for audit
}

func NewWalletService(km KeyManager, cp CustodyProvider, db *gorm.DB, logger *zap.Logger) *WalletService {
	return &WalletService{
		keyManager:      km,
		custodyProvider: cp,
		db:              db,
		logger:          logger,
	}
}

// CreateWallet creates a new wallet (hot/warm/cold)
func (s *WalletService) CreateWallet(ctx context.Context, userID uuid.UUID, asset, walletType string) (*models.Wallet, error) {
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
		ID:        uuid.New(),
		UserID:    userID,
		Type:      walletType,
		Asset:     asset,
		Address:   address,
		Balance:   0,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := s.db.WithContext(ctx).Create(wallet).Error; err != nil {
		return nil, err
	}
	s.logger.Info("Created wallet", zap.String("wallet_id", wallet.ID.String()), zap.String("type", walletType))
	return wallet, nil
}

// GetBalance returns the balance for a wallet
func (s *WalletService) GetBalance(ctx context.Context, walletID string) (float64, error) {
	var wallet models.Wallet
	if err := s.db.WithContext(ctx).First(&wallet, "id = ?", walletID).Error; err != nil {
		return 0, err
	}
	// Query blockchain or custody provider for actual balance
	onChain, err := s.custodyProvider.GetBalance(wallet.Address)
	if err != nil {
		return 0, err
	}
	return onChain, nil
}

// CreateWithdrawalRequest creates a new withdrawal request (multi-sig)
func (s *WalletService) CreateWithdrawalRequest(ctx context.Context, userID uuid.UUID, walletID, asset, toAddress string, amount float64) (*models.WithdrawalRequest, error) {
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
	// Call custody provider to create withdrawal
	txid, err := s.custodyProvider.CreateWithdrawal(wr.WalletID, wr.ToAddress, wr.Amount)
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
		if w.Balance != onChain {
			s.logger.Warn("Balance mismatch", zap.String("wallet_id", w.ID.String()), zap.Float64("db", w.Balance), zap.Float64("on_chain", onChain))
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
