// Package transaction provides comprehensive workflow examples for distributed transactions
package transaction

import (
	"context"
	"fmt"
	"time"

	"github.com/Aidin1998/finalex/internal/accounts/bookkeeper"
	"github.com/Aidin1998/finalex/internal/fiat"
	"github.com/Aidin1998/finalex/internal/trading"
	"github.com/Aidin1998/finalex/internal/trading/settlement"
	"github.com/Aidin1998/finalex/internal/wallet"
	"github.com/Aidin1998/finalex/pkg/models"
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
	bookkeeperSvc    bookkeeper.BookkeeperService
	fiatSvc          fiat.FiatService
	tradingSvc       trading.TradingService
	walletSvc        wallet.WalletService
	settlementEngine *settlement.SettlementEngine

	// XA Resources (removed unused fields)
	// bookkeeperXA *BookkeeperXAResource
	// fiatXA       *FiatXAResource
	// tradingXA    *TradingXAResource
	// walletXA     *WalletXAResource
	// settlementXA *SettlementXAResource
}

// NewDistributedTransactionOrchestrator creates a new orchestrator
func NewDistributedTransactionOrchestrator(
	db *gorm.DB,
	logger *zap.Logger,
	bookkeeperSvc bookkeeper.BookkeeperService,
	fiatSvc fiat.FiatService,
	tradingSvc trading.TradingService,
	walletSvc wallet.WalletService,
	settlementEngine *settlement.SettlementEngine,
) *DistributedTransactionOrchestrator {

	xaManager := NewXATransactionManager(logger, 60*time.Second)
	lockManager := NewDistributedLockManager(db, logger)
	return &DistributedTransactionOrchestrator{
		xaManager:        xaManager,
		lockManager:      lockManager,
		logger:           logger,
		bookkeeperSvc:    bookkeeperSvc,
		fiatSvc:          fiatSvc,
		tradingSvc:       tradingSvc,
		walletSvc:        walletSvc,
		settlementEngine: settlementEngine,
	}
}

// ComplexTradeExecutionWorkflow demonstrates a complex trading workflow with distributed transactions
func (dto *DistributedTransactionOrchestrator) ComplexTradeExecutionWorkflow(
	ctx context.Context,
	userID string,
	orderRequest *models.OrderRequest,
) (*models.Trade, error) {
	// Start distributed transaction
	xaTx, err := dto.xaManager.Start(ctx, 60*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to begin distributed transaction: %w", err)
	}

	// Create XA resources with correct constructors
	bookkeeperXA := NewBookkeeperXAResource(dto.bookkeeperSvc, nil, dto.logger)
	settlementXA := NewSettlementXAResource(dto.settlementEngine, nil, dto.logger)

	// Enlist resources in transaction
	if err := dto.xaManager.Enlist(xaTx, bookkeeperXA); err != nil {
		dto.xaManager.Abort(ctx, xaTx)
		return nil, fmt.Errorf("failed to enlist bookkeeper resource: %w", err)
	}
	if err := dto.xaManager.Enlist(xaTx, settlementXA); err != nil {
		dto.xaManager.Abort(ctx, xaTx)
		return nil, fmt.Errorf("failed to enlist settlement resource: %w", err)
	}

	// Acquire distributed locks for critical resources
	userUUID, err := uuid.Parse(userID)
	if err != nil {
		dto.xaManager.Abort(ctx, xaTx)
		return nil, fmt.Errorf("invalid userID: %w", err)
	}
	userLock, err := dto.lockManager.AcquireLock(ctx, fmt.Sprintf("user:%s", userID), xaTx.ID.String(), 60*time.Second)
	if err != nil {
		dto.xaManager.Abort(ctx, xaTx)
		return nil, fmt.Errorf("failed to acquire user lock: %w", err)
	}
	defer dto.lockManager.ReleaseLock(ctx, userLock.ID)

	marketLock, err := dto.lockManager.AcquireLock(ctx, fmt.Sprintf("market:%s", orderRequest.Symbol), xaTx.ID.String(), 60*time.Second)
	if err != nil {
		dto.xaManager.Abort(ctx, xaTx)
		return nil, fmt.Errorf("failed to acquire market lock: %w", err)
	}
	defer dto.lockManager.ReleaseLock(ctx, marketLock.ID)

	dto.logger.Info("Starting complex trade execution workflow",
		zap.String("transaction_id", xaTx.ID.String()),
		zap.String("user_id", userID),
		zap.String("symbol", orderRequest.Symbol),
		zap.String("side", orderRequest.Side),
		zap.Float64("quantity", orderRequest.Quantity),
		zap.Float64("price", orderRequest.Price))

	// Step 1: Reserve funds in bookkeeper (assume currency is extracted from symbol)
	currency := orderRequest.Symbol // TODO: parse base/quote if needed
	var reserveAmount float64
	if orderRequest.Side == "BUY" {
		reserveAmount = orderRequest.Quantity * orderRequest.Price
		// Fix: Use LockFundsXA for XA transactions
		if err := bookkeeperXA.LockFundsXA(ctx, xaTx.XID, userID, currency, reserveAmount); err != nil {
			dto.xaManager.Abort(ctx, xaTx)
			return nil, fmt.Errorf("failed to reserve funds: %w", err)
		}
	} else {
		reserveAmount = orderRequest.Quantity
		if err := bookkeeperXA.LockFundsXA(ctx, xaTx.XID, userID, currency, reserveAmount); err != nil {
			dto.xaManager.Abort(ctx, xaTx)
			return nil, fmt.Errorf("failed to reserve funds: %w", err)
		}
	}

	// Step 2: Place order in trading engine (convert to *models.Order)
	order := &models.Order{
		UserID:   userUUID,
		Symbol:   orderRequest.Symbol,
		Side:     orderRequest.Side,
		Quantity: orderRequest.Quantity,
		Price:    orderRequest.Price,
	}
	placedOrder, err := dto.tradingSvc.PlaceOrder(ctx, order)
	if err != nil {
		dto.xaManager.Abort(ctx, xaTx)
		return nil, fmt.Errorf("failed to place order: %w", err)
	}

	// Step 3: Simulate trade (in real system, trade would be returned by matching engine)
	trade := &models.Trade{
		ID:        uuid.New(),
		OrderID:   placedOrder.ID,
		UserID:    userUUID,
		Symbol:    orderRequest.Symbol,
		Side:      orderRequest.Side,
		Price:     orderRequest.Price,
		Quantity:  orderRequest.Quantity,
		CreatedAt: time.Now(),
	}

	// Step 4: Capture trade for settlement (construct TradeCapture)
	tradeCapture := settlement.TradeCapture{
		TradeID:   trade.ID.String(),
		UserID:    trade.UserID.String(),
		Symbol:    trade.Symbol,
		Side:      trade.Side,
		Quantity:  trade.Quantity,
		Price:     trade.Price,
		AssetType: "crypto",
		MatchedAt: trade.CreatedAt,
	}
	if err := settlementXA.CaptureTrade(ctx, xaTx.XID, tradeCapture); err != nil {
		dto.xaManager.Abort(ctx, xaTx)
		return nil, fmt.Errorf("failed to capture trade for settlement: %w", err)
	}

	// Step 5: Clear and settle
	if err := settlementXA.ClearAndSettle(ctx, xaTx.XID); err != nil {
		dto.xaManager.Abort(ctx, xaTx)
		return nil, fmt.Errorf("failed to process settlement: %w", err)
	}

	// Step 6: Update account balances (simulate transfer)
	description := "Trade settlement"
	if err := bookkeeperXA.TransferFunds(ctx, xaTx.XID, userID, userID, currency, trade.Quantity, description); err != nil {
		dto.xaManager.Abort(ctx, xaTx)
		return nil, fmt.Errorf("failed to transfer base currency: %w", err)
	}
	if err := bookkeeperXA.TransferFunds(ctx, xaTx.XID, userID, userID, currency, trade.Quantity*trade.Price, description); err != nil {
		dto.xaManager.Abort(ctx, xaTx)
		return nil, fmt.Errorf("failed to transfer quote currency: %w", err)
	}

	// Commit the distributed transaction
	if err := dto.xaManager.Commit(ctx, xaTx); err != nil {
		dto.logger.Error("Failed to commit trade execution transaction",
			zap.String("transaction_id", xaTx.ID.String()),
			zap.Error(err))
		return nil, fmt.Errorf("failed to commit distributed transaction: %w", err)
	}

	dto.logger.Info("Complex trade execution workflow completed successfully",
		zap.String("transaction_id", xaTx.ID.String()),
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
	xaTx, err := dto.xaManager.Start(ctx, 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to begin distributed transaction: %w", err)
	}

	// Create XA resources
	bookkeeperXA := NewBookkeeperXAResource(dto.bookkeeperSvc, nil, dto.logger)
	fiatXA := NewFiatXAResource(dto.fiatSvc, nil, dto.logger, xaTx.ID.String())

	// Enlist resources
	if err := dto.xaManager.Enlist(xaTx, bookkeeperXA); err != nil {
		dto.xaManager.Abort(ctx, xaTx)
		return nil, fmt.Errorf("failed to enlist bookkeeper resource: %w", err)
	}

	if err := dto.xaManager.Enlist(xaTx, fiatXA); err != nil {
		dto.xaManager.Abort(ctx, xaTx)
		return nil, fmt.Errorf("failed to enlist fiat resource: %w", err)
	}

	// Acquire user lock
	userLock, err := dto.lockManager.AcquireLock(ctx, fmt.Sprintf("user:%s", userID), xaTx.ID.String(), 30*time.Second)
	if err != nil {
		dto.xaManager.Abort(ctx, xaTx)
		return nil, fmt.Errorf("failed to acquire user lock: %w", err)
	}
	defer dto.lockManager.ReleaseLock(ctx, userLock.ID)

	dto.logger.Info("Starting fiat deposit workflow",
		zap.String("transaction_id", xaTx.ID.String()),
		zap.String("user_id", userID),
		zap.String("currency", currency),
		zap.Float64("amount", amount),
		zap.String("provider", provider))

	// Step 1: Initiate deposit with fiat service
	transaction, err := fiatXA.InitiateDeposit(ctx, userID, currency, amount, provider)
	if err != nil {
		dto.xaManager.Abort(ctx, xaTx)
		return nil, fmt.Errorf("failed to initiate deposit: %w", err)
	}

	// Step 2: Complete the deposit
	if err := fiatXA.CompleteDeposit(ctx, transaction.ID.String()); err != nil {
		dto.xaManager.Abort(ctx, xaTx)
		return nil, fmt.Errorf("failed to complete deposit: %w", err)
	}

	// Step 3: Update account balance (simulate transfer from provider to user)
	description := fmt.Sprintf("Fiat deposit via %s", provider)
	if err := bookkeeperXA.TransferFunds(ctx, xaTx.XID, provider, userID, currency, amount, description); err != nil {
		dto.xaManager.Abort(ctx, xaTx)
		return nil, fmt.Errorf("failed to transfer deposit funds: %w", err)
	}

	// Commit the distributed transaction
	if err := dto.xaManager.Commit(ctx, xaTx); err != nil {
		dto.logger.Error("Failed to commit fiat deposit transaction",
			zap.String("transaction_id", xaTx.ID.String()),
			zap.Error(err))
		return nil, fmt.Errorf("failed to commit distributed transaction: %w", err)
	}

	dto.logger.Info("Fiat deposit workflow completed successfully",
		zap.String("transaction_id", xaTx.ID.String()),
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

	// Step 1: Lock user funds
	saga.AddStep("lock_funds",
		func(ctx context.Context) error {
			xaTx, err := dto.xaManager.Start(ctx, 30*time.Second)
			if err != nil {
				return fmt.Errorf("failed to begin transaction for fund locking: %w", err)
			}
			bookkeeperXA := NewBookkeeperXAResource(dto.bookkeeperSvc, nil, dto.logger)
			if err := dto.xaManager.Enlist(xaTx, bookkeeperXA); err != nil {
				dto.xaManager.Abort(ctx, xaTx)
				return fmt.Errorf("failed to enlist bookkeeper resource: %w", err)
			}
			if err := bookkeeperXA.LockFundsXA(ctx, xaTx.XID, userID, asset, amount); err != nil {
				dto.xaManager.Abort(ctx, xaTx)
				return fmt.Errorf("failed to lock funds: %w", err)
			}
			if err := dto.xaManager.Commit(ctx, xaTx); err != nil {
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
			xaTx, err := dto.xaManager.Start(ctx, 30*time.Second)
			if err != nil {
				return fmt.Errorf("failed to begin compensation transaction: %w", err)
			}
			bookkeeperXA := NewBookkeeperXAResource(dto.bookkeeperSvc, nil, dto.logger)
			if err := dto.xaManager.Enlist(xaTx, bookkeeperXA); err != nil {
				dto.xaManager.Abort(ctx, xaTx)
				return fmt.Errorf("failed to enlist bookkeeper resource for compensation: %w", err)
			}
			if err := bookkeeperXA.UnlockFundsXA(ctx, xaTx.XID, userID, asset, amount); err != nil {
				dto.xaManager.Abort(ctx, xaTx)
				return fmt.Errorf("failed to unlock funds in compensation: %w", err)
			}
			return dto.xaManager.Commit(ctx, xaTx)
		})

	// Step 2: Create withdrawal request
	saga.AddStep("create_withdrawal_request",
		func(ctx context.Context) error {
			xaTx, err := dto.xaManager.Start(ctx, 30*time.Second)
			if err != nil {
				return fmt.Errorf("failed to begin transaction for withdrawal request: %w", err)
			}
			walletXA := NewWalletXAResource(nil, &dto.walletSvc, dto.logger)
			if err := dto.xaManager.Enlist(xaTx, walletXA); err != nil {
				dto.xaManager.Abort(ctx, xaTx)
				return fmt.Errorf("failed to enlist wallet resource: %w", err)
			}
			wr, err := walletXA.CreateWithdrawalRequest(ctx, uuid.MustParse(userID), walletID, asset, toAddress, amount)
			if err != nil {
				dto.xaManager.Abort(ctx, xaTx)
				return fmt.Errorf("failed to create withdrawal request: %w", err)
			}
			if err := dto.xaManager.Commit(ctx, xaTx); err != nil {
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
				dto.logger.Info("Compensating withdrawal request creation",
					zap.String("saga_id", sagaID),
					zap.String("withdrawal_request_id", withdrawalRequest.ID.String()))
			}
			return nil
		})

	// Step 3: Simulate bookkeeper transaction (optional, can be extended as needed)
	// ...

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

// CrossServiceTransferWorkflow demonstrates a transfer between different services
func (dto *DistributedTransactionOrchestrator) CrossServiceTransferWorkflow(
	ctx context.Context,
	fromUserID, toUserID, currency string,
	amount float64,
	transferType string, // "internal", "fiat_to_crypto", "crypto_to_fiat"
) error {
	// Start distributed transaction
	xaTx, err := dto.xaManager.Start(ctx, 45*time.Second)
	if err != nil {
		return fmt.Errorf("failed to begin distributed transaction: %w", err)
	}

	// Create and enlist resources based on transfer type
	bookkeeperXA := NewBookkeeperXAResource(dto.bookkeeperSvc, nil, dto.logger)
	if err := dto.xaManager.Enlist(xaTx, bookkeeperXA); err != nil {
		dto.xaManager.Abort(ctx, xaTx)
		return fmt.Errorf("failed to enlist bookkeeper resource: %w", err)
	}

	// Acquire locks for both users
	fromUserLock, err := dto.lockManager.AcquireLock(ctx, fmt.Sprintf("user:%s", fromUserID), xaTx.ID.String(), 45*time.Second)
	if err != nil {
		dto.xaManager.Abort(ctx, xaTx)
		return fmt.Errorf("failed to acquire from user lock: %w", err)
	}
	defer dto.lockManager.ReleaseLock(ctx, fromUserLock.ID)

	toUserLock, err := dto.lockManager.AcquireLock(ctx, fmt.Sprintf("user:%s", toUserID), xaTx.ID.String(), 45*time.Second)
	if err != nil {
		dto.xaManager.Abort(ctx, xaTx)
		return fmt.Errorf("failed to acquire to user lock: %w", err)
	}
	defer dto.lockManager.ReleaseLock(ctx, toUserLock.ID)

	dto.logger.Info("Starting cross-service transfer workflow",
		zap.String("transaction_id", xaTx.ID.String()),
		zap.String("from_user_id", fromUserID),
		zap.String("to_user_id", toUserID),
		zap.String("currency", currency),
		zap.Float64("amount", amount),
		zap.String("transfer_type", transferType))

	// Execute transfer based on type
	switch transferType {
	case "internal":
		description := "Internal transfer"
		if err := bookkeeperXA.TransferFunds(ctx, xaTx.XID, fromUserID, toUserID, currency, amount, description); err != nil {
			dto.xaManager.Abort(ctx, xaTx)
			return fmt.Errorf("failed to transfer funds: %w", err)
		}
	case "fiat_to_crypto", "crypto_to_fiat":
		fiatXA := NewFiatXAResource(dto.fiatSvc, nil, dto.logger, xaTx.ID.String())
		if err := dto.xaManager.Enlist(xaTx, fiatXA); err != nil {
			dto.xaManager.Abort(ctx, xaTx)
			return fmt.Errorf("failed to enlist fiat resource: %w", err)
		}
		if err := bookkeeperXA.LockFundsXA(ctx, xaTx.XID, fromUserID, currency, amount); err != nil {
			dto.xaManager.Abort(ctx, xaTx)
			return fmt.Errorf("failed to lock source funds: %w", err)
		}
		description := "Cross-service transfer"
		if err := bookkeeperXA.TransferFunds(ctx, xaTx.XID, fromUserID, toUserID, currency, amount, description); err != nil {
			dto.xaManager.Abort(ctx, xaTx)
			return fmt.Errorf("failed to transfer funds: %w", err)
		}
	default:
		dto.xaManager.Abort(ctx, xaTx)
		return fmt.Errorf("unsupported transfer type: %s", transferType)
	}

	// Commit the distributed transaction
	if err := dto.xaManager.Commit(ctx, xaTx); err != nil {
		dto.logger.Error("Failed to commit cross-service transfer transaction",
			zap.String("transaction_id", xaTx.ID.String()),
			zap.Error(err))
		return fmt.Errorf("failed to commit distributed transaction: %w", err)
	}

	dto.logger.Info("Cross-service transfer workflow completed successfully",
		zap.String("transaction_id", xaTx.ID.String()),
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
