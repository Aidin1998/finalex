// Package transaction provides comprehensive workflow examples for distributed transactions
package transaction

import (
	"context"
	"fmt"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/bookkeeper"
	"github.com/Aidin1998/pincex_unified/internal/fiat"
	"github.com/Aidin1998/pincex_unified/internal/settlement"
	"github.com/Aidin1998/pincex_unified/internal/trading"
	"github.com/Aidin1998/pincex_unified/internal/wallet"
	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// DistributedTransactionOrchestrator coordinates complex distributed transactions
type DistributedTransactionOrchestrator struct {
	xaManager   *XATransactionManager
	lockManager *DistributedLockManager
	logger      *zap.Logger

	// Service dependencies
	bookkeeperSvc bookkeeper.BookkeeperService
	fiatSvc       fiat.FiatService
	tradingSvc    trading.TradingService
	walletSvc     wallet.WalletService
	settlementSvc settlement.SettlementService

	// XA Resources
	bookkeeperXA *BookkeeperXAResource
	fiatXA       *FiatXAResource
	tradingXA    *TradingXAResource
	walletXA     *WalletXAResource
	settlementXA *SettlementXAResource
}

// NewDistributedTransactionOrchestrator creates a new orchestrator
func NewDistributedTransactionOrchestrator(
	db *gorm.DB,
	logger *zap.Logger,
	bookkeeperSvc bookkeeper.BookkeeperService,
	fiatSvc fiat.FiatService,
	tradingSvc trading.TradingService,
	walletSvc wallet.WalletService,
	settlementSvc settlement.SettlementService,
) *DistributedTransactionOrchestrator {

	xaManager := NewXATransactionManager(logger)
	lockManager := NewDistributedLockManager(db, logger)

	return &DistributedTransactionOrchestrator{
		xaManager:     xaManager,
		lockManager:   lockManager,
		logger:        logger,
		bookkeeperSvc: bookkeeperSvc,
		fiatSvc:       fiatSvc,
		tradingSvc:    tradingSvc,
		walletSvc:     walletSvc,
		settlementSvc: settlementSvc,
	}
}

// ComplexTradeExecutionWorkflow demonstrates a complex trading workflow with distributed transactions
func (dto *DistributedTransactionOrchestrator) ComplexTradeExecutionWorkflow(
	ctx context.Context,
	userID string,
	orderRequest *models.OrderRequest,
) (*models.Trade, error) {

	// Start distributed transaction
	txID := uuid.New()
	xaTx, err := dto.xaManager.BeginTransaction(ctx, txID, 60*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to begin distributed transaction: %w", err)
	}

	// Create XA resources with shared transaction ID
	xidStr := txID.String()
	bookkeeperXA := NewBookkeeperXAResource(dto.bookkeeperSvc, nil, dto.logger)
	tradingXA := NewTradingXAResource(dto.tradingSvc, nil, dto.logger, xidStr)
	settlementXA := NewSettlementXAResource(dto.settlementSvc, nil, dto.logger, xidStr)

	// Enlist resources in transaction
	if err := xaTx.EnlistResource(bookkeeperXA); err != nil {
		dto.xaManager.AbortTransaction(ctx, txID)
		return nil, fmt.Errorf("failed to enlist bookkeeper resource: %w", err)
	}

	if err := xaTx.EnlistResource(tradingXA); err != nil {
		dto.xaManager.AbortTransaction(ctx, txID)
		return nil, fmt.Errorf("failed to enlist trading resource: %w", err)
	}

	if err := xaTx.EnlistResource(settlementXA); err != nil {
		dto.xaManager.AbortTransaction(ctx, txID)
		return nil, fmt.Errorf("failed to enlist settlement resource: %w", err)
	}

	// Acquire distributed locks for critical resources
	userLock, err := dto.lockManager.AcquireLock(ctx, fmt.Sprintf("user:%s", userID), txID.String(), 60*time.Second)
	if err != nil {
		dto.xaManager.AbortTransaction(ctx, txID)
		return nil, fmt.Errorf("failed to acquire user lock: %w", err)
	}
	defer dto.lockManager.ReleaseLock(ctx, userLock.ID)

	marketLock, err := dto.lockManager.AcquireLock(ctx, fmt.Sprintf("market:%s", orderRequest.Symbol), txID.String(), 60*time.Second)
	if err != nil {
		dto.xaManager.AbortTransaction(ctx, txID)
		return nil, fmt.Errorf("failed to acquire market lock: %w", err)
	}
	defer dto.lockManager.ReleaseLock(ctx, marketLock.ID)

	dto.logger.Info("Starting complex trade execution workflow",
		zap.String("transaction_id", txID.String()),
		zap.String("user_id", userID),
		zap.String("symbol", orderRequest.Symbol),
		zap.String("side", orderRequest.Side),
		zap.Float64("quantity", orderRequest.Quantity),
		zap.Float64("price", orderRequest.Price))

	// Step 1: Reserve funds in bookkeeper
	var reserveAmount float64
	if orderRequest.Side == "BUY" {
		reserveAmount = orderRequest.Quantity * orderRequest.Price
		if err := bookkeeperXA.LockFunds(ctx, getXIDFromString(xidStr), userID, orderRequest.QuoteCurrency, reserveAmount); err != nil {
			dto.xaManager.AbortTransaction(ctx, txID)
			return nil, fmt.Errorf("failed to reserve quote currency funds: %w", err)
		}
	} else {
		reserveAmount = orderRequest.Quantity
		if err := bookkeeperXA.LockFunds(ctx, getXIDFromString(xidStr), userID, orderRequest.BaseCurrency, reserveAmount); err != nil {
			dto.xaManager.AbortTransaction(ctx, txID)
			return nil, fmt.Errorf("failed to reserve base currency funds: %w", err)
		}
	}

	// Step 2: Place order in trading engine
	order, err := tradingXA.PlaceOrder(ctx, orderRequest)
	if err != nil {
		dto.xaManager.AbortTransaction(ctx, txID)
		return nil, fmt.Errorf("failed to place order: %w", err)
	}

	// Step 3: Execute trade matching
	trade, err := tradingXA.ExecuteTrade(ctx, order.ID, orderRequest.Quantity, orderRequest.Price)
	if err != nil {
		dto.xaManager.AbortTransaction(ctx, txID)
		return nil, fmt.Errorf("failed to execute trade: %w", err)
	}

	// Step 4: Capture trade for settlement
	if err := settlementXA.CaptureTrade(ctx, trade.ID, trade.BuyerID.String(), trade.SellerID.String(),
		trade.Symbol, trade.Quantity, trade.Price); err != nil {
		dto.xaManager.AbortTransaction(ctx, txID)
		return nil, fmt.Errorf("failed to capture trade for settlement: %w", err)
	}

	// Step 5: Process settlement
	if err := settlementXA.ProcessSettlement(ctx, trade.ID); err != nil {
		dto.xaManager.AbortTransaction(ctx, txID)
		return nil, fmt.Errorf("failed to process settlement: %w", err)
	}

	// Step 6: Update account balances
	if err := bookkeeperXA.TransferFunds(ctx, getXIDFromString(xidStr),
		trade.SellerID.String(), trade.BuyerID.String(),
		orderRequest.BaseCurrency, trade.Quantity); err != nil {
		dto.xaManager.AbortTransaction(ctx, txID)
		return nil, fmt.Errorf("failed to transfer base currency: %w", err)
	}

	if err := bookkeeperXA.TransferFunds(ctx, getXIDFromString(xidStr),
		trade.BuyerID.String(), trade.SellerID.String(),
		orderRequest.QuoteCurrency, trade.Quantity*trade.Price); err != nil {
		dto.xaManager.AbortTransaction(ctx, txID)
		return nil, fmt.Errorf("failed to transfer quote currency: %w", err)
	}

	// Commit the distributed transaction
	if err := dto.xaManager.CommitTransaction(ctx, txID); err != nil {
		dto.logger.Error("Failed to commit trade execution transaction",
			zap.String("transaction_id", txID.String()),
			zap.Error(err))
		return nil, fmt.Errorf("failed to commit distributed transaction: %w", err)
	}

	dto.logger.Info("Complex trade execution workflow completed successfully",
		zap.String("transaction_id", txID.String()),
		zap.String("trade_id", trade.ID.String()),
		zap.Float64("executed_quantity", trade.Quantity),
		zap.Float64("executed_price", trade.Price))

	return trade, nil
}

// FiatDepositWorkflow demonstrates a fiat deposit workflow with distributed transactions
func (dto *DistributedTransactionOrchestrator) FiatDepositWorkflow(
	ctx context.Context,
	userID, currency string,
	amount float64,
	provider string,
) (*models.Transaction, error) {

	// Start distributed transaction
	txID := uuid.New()
	xaTx, err := dto.xaManager.BeginTransaction(ctx, txID, 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to begin distributed transaction: %w", err)
	}

	// Create XA resources
	xidStr := txID.String()
	bookkeeperXA := NewBookkeeperXAResource(dto.bookkeeperSvc, nil, dto.logger)
	fiatXA := NewFiatXAResource(dto.fiatSvc, nil, dto.logger, xidStr)

	// Enlist resources
	if err := xaTx.EnlistResource(bookkeeperXA); err != nil {
		dto.xaManager.AbortTransaction(ctx, txID)
		return nil, fmt.Errorf("failed to enlist bookkeeper resource: %w", err)
	}

	if err := xaTx.EnlistResource(fiatXA); err != nil {
		dto.xaManager.AbortTransaction(ctx, txID)
		return nil, fmt.Errorf("failed to enlist fiat resource: %w", err)
	}

	// Acquire user lock
	userLock, err := dto.lockManager.AcquireLock(ctx, fmt.Sprintf("user:%s", userID), txID.String(), 30*time.Second)
	if err != nil {
		dto.xaManager.AbortTransaction(ctx, txID)
		return nil, fmt.Errorf("failed to acquire user lock: %w", err)
	}
	defer dto.lockManager.ReleaseLock(ctx, userLock.ID)

	dto.logger.Info("Starting fiat deposit workflow",
		zap.String("transaction_id", txID.String()),
		zap.String("user_id", userID),
		zap.String("currency", currency),
		zap.Float64("amount", amount),
		zap.String("provider", provider))

	// Step 1: Initiate deposit with fiat service
	transaction, err := fiatXA.InitiateDeposit(ctx, userID, currency, amount, provider)
	if err != nil {
		dto.xaManager.AbortTransaction(ctx, txID)
		return nil, fmt.Errorf("failed to initiate deposit: %w", err)
	}

	// Step 2: Create transaction record in bookkeeper
	bookkeeperTx, err := bookkeeperXA.CreateTransaction(ctx, getXIDFromString(xidStr),
		userID, "deposit", amount, currency, provider,
		fmt.Sprintf("Fiat deposit via %s", provider))
	if err != nil {
		dto.xaManager.AbortTransaction(ctx, txID)
		return nil, fmt.Errorf("failed to create bookkeeper transaction: %w", err)
	}

	// Step 3: Complete the deposit
	if err := fiatXA.CompleteDeposit(ctx, transaction.ID.String()); err != nil {
		dto.xaManager.AbortTransaction(ctx, txID)
		return nil, fmt.Errorf("failed to complete deposit: %w", err)
	}

	// Step 4: Update account balance
	if err := bookkeeperXA.CompleteTransaction(ctx, getXIDFromString(xidStr), bookkeeperTx.ID.String()); err != nil {
		dto.xaManager.AbortTransaction(ctx, txID)
		return nil, fmt.Errorf("failed to complete bookkeeper transaction: %w", err)
	}

	// Commit the distributed transaction
	if err := dto.xaManager.CommitTransaction(ctx, txID); err != nil {
		dto.logger.Error("Failed to commit fiat deposit transaction",
			zap.String("transaction_id", txID.String()),
			zap.Error(err))
		return nil, fmt.Errorf("failed to commit distributed transaction: %w", err)
	}

	dto.logger.Info("Fiat deposit workflow completed successfully",
		zap.String("transaction_id", txID.String()),
		zap.String("fiat_transaction_id", transaction.ID.String()),
		zap.Float64("amount", amount))

	return transaction, nil
}

// CryptoWithdrawalWorkflow demonstrates a crypto withdrawal workflow using saga pattern
func (dto *DistributedTransactionOrchestrator) CryptoWithdrawalWorkflow(
	ctx context.Context,
	userID string,
	walletID, asset, toAddress string,
	amount float64,
) (*models.WithdrawalRequest, error) {

	// Create saga transaction for complex workflow
	sagaID := uuid.New().String()
	saga := NewTransactionSaga(sagaID, dto.logger)

	var withdrawalRequest *models.WithdrawalRequest
	var bookkeeperTxID string

	// Step 1: Lock user funds
	saga.AddStep("lock_funds",
		func(ctx context.Context) error {
			txID := uuid.New()
			xaTx, err := dto.xaManager.BeginTransaction(ctx, txID, 30*time.Second)
			if err != nil {
				return fmt.Errorf("failed to begin transaction for fund locking: %w", err)
			}

			bookkeeperXA := NewBookkeeperXAResource(dto.bookkeeperSvc, nil, dto.logger)
			if err := xaTx.EnlistResource(bookkeeperXA); err != nil {
				dto.xaManager.AbortTransaction(ctx, txID)
				return fmt.Errorf("failed to enlist bookkeeper resource: %w", err)
			}

			if err := bookkeeperXA.LockFunds(ctx, getXIDFromString(txID.String()), userID, asset, amount); err != nil {
				dto.xaManager.AbortTransaction(ctx, txID)
				return fmt.Errorf("failed to lock funds: %w", err)
			}

			if err := dto.xaManager.CommitTransaction(ctx, txID); err != nil {
				return fmt.Errorf("failed to commit fund locking transaction: %w", err)
			}

			dto.logger.Info("Locked funds for withdrawal",
				zap.String("saga_id", sagaID),
				zap.String("user_id", userID),
				zap.String("asset", asset),
				zap.Float64("amount", amount))

			return nil
		},
		func(ctx context.Context) error {
			// Compensation: unlock funds
			txID := uuid.New()
			xaTx, err := dto.xaManager.BeginTransaction(ctx, txID, 30*time.Second)
			if err != nil {
				return fmt.Errorf("failed to begin compensation transaction: %w", err)
			}

			bookkeeperXA := NewBookkeeperXAResource(dto.bookkeeperSvc, nil, dto.logger)
			if err := xaTx.EnlistResource(bookkeeperXA); err != nil {
				dto.xaManager.AbortTransaction(ctx, txID)
				return fmt.Errorf("failed to enlist bookkeeper resource for compensation: %w", err)
			}

			if err := bookkeeperXA.UnlockFunds(ctx, getXIDFromString(txID.String()), userID, asset, amount); err != nil {
				dto.xaManager.AbortTransaction(ctx, txID)
				return fmt.Errorf("failed to unlock funds in compensation: %w", err)
			}

			return dto.xaManager.CommitTransaction(ctx, txID)
		})

	// Step 2: Create withdrawal request
	saga.AddStep("create_withdrawal_request",
		func(ctx context.Context) error {
			txID := uuid.New()
			xaTx, err := dto.xaManager.BeginTransaction(ctx, txID, 30*time.Second)
			if err != nil {
				return fmt.Errorf("failed to begin transaction for withdrawal request: %w", err)
			}

			walletXA := NewWalletXAResource(dto.walletSvc, nil, dto.logger, txID.String())
			if err := xaTx.EnlistResource(walletXA); err != nil {
				dto.xaManager.AbortTransaction(ctx, txID)
				return fmt.Errorf("failed to enlist wallet resource: %w", err)
			}

			wr, err := walletXA.CreateWithdrawalRequest(ctx, uuid.MustParse(userID), walletID, asset, toAddress, amount)
			if err != nil {
				dto.xaManager.AbortTransaction(ctx, txID)
				return fmt.Errorf("failed to create withdrawal request: %w", err)
			}

			if err := dto.xaManager.CommitTransaction(ctx, txID); err != nil {
				return fmt.Errorf("failed to commit withdrawal request transaction: %w", err)
			}

			withdrawalRequest = wr
			dto.logger.Info("Created withdrawal request",
				zap.String("saga_id", sagaID),
				zap.String("withdrawal_request_id", wr.ID.String()))

			return nil
		},
		func(ctx context.Context) error {
			// Compensation: cancel withdrawal request
			if withdrawalRequest != nil {
				// Mark withdrawal request as cancelled
				dto.logger.Info("Compensating withdrawal request creation",
					zap.String("saga_id", sagaID),
					zap.String("withdrawal_request_id", withdrawalRequest.ID.String()))
			}
			return nil
		})

	// Step 3: Create bookkeeper transaction
	saga.AddStep("create_bookkeeper_transaction",
		func(ctx context.Context) error {
			txID := uuid.New()
			xaTx, err := dto.xaManager.BeginTransaction(ctx, txID, 30*time.Second)
			if err != nil {
				return fmt.Errorf("failed to begin transaction for bookkeeper: %w", err)
			}

			bookkeeperXA := NewBookkeeperXAResource(dto.bookkeeperSvc, nil, dto.logger)
			if err := xaTx.EnlistResource(bookkeeperXA); err != nil {
				dto.xaManager.AbortTransaction(ctx, txID)
				return fmt.Errorf("failed to enlist bookkeeper resource: %w", err)
			}

			tx, err := bookkeeperXA.CreateTransaction(ctx, getXIDFromString(txID.String()),
				userID, "withdrawal", amount, asset, "wallet",
				fmt.Sprintf("Crypto withdrawal to %s", toAddress))
			if err != nil {
				dto.xaManager.AbortTransaction(ctx, txID)
				return fmt.Errorf("failed to create bookkeeper transaction: %w", err)
			}

			if err := dto.xaManager.CommitTransaction(ctx, txID); err != nil {
				return fmt.Errorf("failed to commit bookkeeper transaction: %w", err)
			}

			bookkeeperTxID = tx.ID.String()
			dto.logger.Info("Created bookkeeper transaction for withdrawal",
				zap.String("saga_id", sagaID),
				zap.String("bookkeeper_tx_id", bookkeeperTxID))

			return nil
		},
		func(ctx context.Context) error {
			// Compensation: fail the bookkeeper transaction
			if bookkeeperTxID != "" {
				dto.logger.Info("Compensating bookkeeper transaction creation",
					zap.String("saga_id", sagaID),
					zap.String("bookkeeper_tx_id", bookkeeperTxID))
			}
			return nil
		})

	// Execute the saga
	if err := saga.Execute(ctx); err != nil {
		dto.logger.Error("Crypto withdrawal saga failed",
			zap.String("saga_id", sagaID),
			zap.Error(err))
		return nil, err
	}

	dto.logger.Info("Crypto withdrawal workflow completed successfully",
		zap.String("saga_id", sagaID),
		zap.String("withdrawal_request_id", withdrawalRequest.ID.String()))

	return withdrawalRequest, nil
}

// getXIDFromString creates an XID from a string (utility function)
func getXIDFromString(str string) XID {
	return XID{
		FormatID:     1,
		GlobalTxnID:  []byte(str),
		BranchQualID: []byte("branch"),
	}
}

// CrossServiceTransferWorkflow demonstrates a transfer between different services
func (dto *DistributedTransactionOrchestrator) CrossServiceTransferWorkflow(
	ctx context.Context,
	fromUserID, toUserID, currency string,
	amount float64,
	transferType string, // "internal", "fiat_to_crypto", "crypto_to_fiat"
) error {

	// Start distributed transaction
	txID := uuid.New()
	xaTx, err := dto.xaManager.BeginTransaction(ctx, txID, 45*time.Second)
	if err != nil {
		return fmt.Errorf("failed to begin distributed transaction: %w", err)
	}

	// Create and enlist resources based on transfer type
	bookkeeperXA := NewBookkeeperXAResource(dto.bookkeeperSvc, nil, dto.logger)
	if err := xaTx.EnlistResource(bookkeeperXA); err != nil {
		dto.xaManager.AbortTransaction(ctx, txID)
		return fmt.Errorf("failed to enlist bookkeeper resource: %w", err)
	}

	// Acquire locks for both users
	fromUserLock, err := dto.lockManager.AcquireLock(ctx, fmt.Sprintf("user:%s", fromUserID), txID.String(), 45*time.Second)
	if err != nil {
		dto.xaManager.AbortTransaction(ctx, txID)
		return fmt.Errorf("failed to acquire from user lock: %w", err)
	}
	defer dto.lockManager.ReleaseLock(ctx, fromUserLock.ID)

	toUserLock, err := dto.lockManager.AcquireLock(ctx, fmt.Sprintf("user:%s", toUserID), txID.String(), 45*time.Second)
	if err != nil {
		dto.xaManager.AbortTransaction(ctx, txID)
		return fmt.Errorf("failed to acquire to user lock: %w", err)
	}
	defer dto.lockManager.ReleaseLock(ctx, toUserLock.ID)

	dto.logger.Info("Starting cross-service transfer workflow",
		zap.String("transaction_id", txID.String()),
		zap.String("from_user_id", fromUserID),
		zap.String("to_user_id", toUserID),
		zap.String("currency", currency),
		zap.Float64("amount", amount),
		zap.String("transfer_type", transferType))

	// Execute transfer based on type
	switch transferType {
	case "internal":
		// Simple internal transfer
		if err := bookkeeperXA.TransferFunds(ctx, getXIDFromString(txID.String()),
			fromUserID, toUserID, currency, amount); err != nil {
			dto.xaManager.AbortTransaction(ctx, txID)
			return fmt.Errorf("failed to transfer funds: %w", err)
		}

	case "fiat_to_crypto", "crypto_to_fiat":
		// More complex transfer involving multiple services
		fiatXA := NewFiatXAResource(dto.fiatSvc, nil, dto.logger, txID.String())
		if err := xaTx.EnlistResource(fiatXA); err != nil {
			dto.xaManager.AbortTransaction(ctx, txID)
			return fmt.Errorf("failed to enlist fiat resource: %w", err)
		}

		// Lock funds from source
		if err := bookkeeperXA.LockFunds(ctx, getXIDFromString(txID.String()),
			fromUserID, currency, amount); err != nil {
			dto.xaManager.AbortTransaction(ctx, txID)
			return fmt.Errorf("failed to lock source funds: %w", err)
		}

		// Create appropriate transactions
		if transferType == "fiat_to_crypto" {
			// Debit fiat, credit crypto equivalent
			if err := bookkeeperXA.TransferFunds(ctx, getXIDFromString(txID.String()),
				fromUserID, toUserID, currency, amount); err != nil {
				dto.xaManager.AbortTransaction(ctx, txID)
				return fmt.Errorf("failed to transfer fiat to crypto: %w", err)
			}
		} else {
			// Debit crypto, credit fiat equivalent
			if err := bookkeeperXA.TransferFunds(ctx, getXIDFromString(txID.String()),
				fromUserID, toUserID, currency, amount); err != nil {
				dto.xaManager.AbortTransaction(ctx, txID)
				return fmt.Errorf("failed to transfer crypto to fiat: %w", err)
			}
		}

	default:
		dto.xaManager.AbortTransaction(ctx, txID)
		return fmt.Errorf("unsupported transfer type: %s", transferType)
	}

	// Commit the distributed transaction
	if err := dto.xaManager.CommitTransaction(ctx, txID); err != nil {
		dto.logger.Error("Failed to commit cross-service transfer transaction",
			zap.String("transaction_id", txID.String()),
			zap.Error(err))
		return fmt.Errorf("failed to commit distributed transaction: %w", err)
	}

	dto.logger.Info("Cross-service transfer workflow completed successfully",
		zap.String("transaction_id", txID.String()),
		zap.String("transfer_type", transferType),
		zap.Float64("amount", amount))

	return nil
}

// GetWorkflowMetrics returns metrics for all workflow executions
func (dto *DistributedTransactionOrchestrator) GetWorkflowMetrics() map[string]interface{} {
	return map[string]interface{}{
		"xa_manager_metrics":  dto.xaManager.GetMetrics(),
		"lock_manager_active": len(dto.lockManager.locks),
		"orchestrator_ready":  true,
	}
}

// Stop gracefully stops the orchestrator
func (dto *DistributedTransactionOrchestrator) Stop() {
	dto.lockManager.Stop()
	dto.logger.Info("Distributed transaction orchestrator stopped")
}
