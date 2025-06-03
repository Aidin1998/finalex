//go:build unit && wallet
// +build unit,wallet

package wallet_test

import (
	"context"
	"testing"

	"github.com/Aidin1998/pincex_unified/internal/wallet"
	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/google/uuid"
	"go.uber.org/zap/zaptest"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestWalletService_BasicOperations(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}
	db.AutoMigrate(&models.Wallet{})
	ctx := context.Background()
	logger := zaptest.NewLogger(t)
	km := wallet.NewInMemoryKeyManager()
	cp := wallet.NewDummyCustodyProvider()
	ws := wallet.NewWalletService(km, cp, db, logger)
	userID := uuid.New()

	// Create wallet
	w, err := ws.CreateWallet(ctx, userID, "BTC", "hot", []string{"signer1"}, 1, nil, false)
	if err != nil {
		t.Fatalf("CreateWallet failed: %v", err)
	}
	if w.UserID != userID || w.Asset != "BTC" || w.Type != "hot" {
		t.Errorf("wallet fields mismatch: %+v", w)
	}

	// Simulate deposit by updating dummy custody provider
	cp.Balances()[w.Address] = 10.0
	cp.Balances()[w.ID.String()] = 10.0 // ensure both keys are set for withdrawal

	// Get balance
	bal, err := ws.GetBalance(ctx, w.ID.String())
	if err != nil {
		t.Fatalf("GetBalance failed: %v", err)
	}
	if bal != 10.0 {
		t.Errorf("expected balance 10.0, got %v", bal)
	}

	// Withdraw
	db.AutoMigrate(&models.WithdrawalRequest{})
	wr, err := ws.CreateWithdrawalRequest(ctx, userID, w.ID.String(), "BTC", w.Address, 5.0)
	if err != nil {
		t.Fatalf("CreateWithdrawalRequest failed: %v", err)
	}
	if wr.Amount != 5.0 || wr.Status != "pending" {
		t.Errorf("withdrawal request fields mismatch: %+v", wr)
	}

	// Approve withdrawal (simulate multi-sig)
	db.AutoMigrate(&models.Approval{})
	_ = ws.ApproveWithdrawal(ctx, wr.ID, "signer1")
	_ = ws.ApproveWithdrawal(ctx, wr.ID, "signer2")

	// Broadcast withdrawal
	txid, err := ws.BroadcastWithdrawal(ctx, wr.ID)
	if err != nil {
		t.Fatalf("BroadcastWithdrawal failed: %v", err)
	}
	if txid == "" {
		t.Errorf("expected txid, got empty string")
	}

	// Check balance after withdrawal
	bal2, err := ws.GetBalance(ctx, w.ID.String())
	if err != nil {
		t.Fatalf("GetBalance after withdrawal failed: %v", err)
	}
	if bal2 != 5.0 {
		t.Errorf("expected balance 5.0 after withdrawal, got %v", bal2)
	}
}

func TestEVMAdapter_Integration(t *testing.T) {
	// TODO: Implement EVM adapter integration and error handling tests
}

func TestProvider_Resilience(t *testing.T) {
	// TODO: Test provider failover and error scenarios
}
