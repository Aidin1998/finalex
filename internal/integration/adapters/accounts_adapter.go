// Package adapters provides adapter implementations for service contracts
package adapters

import (
	"context"
	"fmt"

	"github.com/Aidin1998/finalex/internal/integration/contracts"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// AccountsServiceAdapter implements contracts.AccountsServiceContract
// by adapting the accounts module's native interface
type AccountsServiceAdapter struct {
	service contracts.AccountsServiceContract
	logger  *zap.Logger
}

// NewAccountsServiceAdapter creates a new accounts service adapter
func NewAccountsServiceAdapter(service contracts.AccountsServiceContract, logger *zap.Logger) *AccountsServiceAdapter {
	return &AccountsServiceAdapter{
		service: service,
		logger:  logger,
	}
}

// GetBalance retrieves account balance for user and currency
func (a *AccountsServiceAdapter) GetBalance(ctx context.Context, userID uuid.UUID, currency string) (*contracts.Balance, error) {
	a.logger.Debug("Getting balance",
		zap.String("user_id", userID.String()),
		zap.String("currency", currency))

	// Call accounts service to get balance
	balance, err := a.service.GetBalance(ctx, userID, currency)
	if err != nil {
		a.logger.Error("Failed to get balance",
			zap.String("user_id", userID.String()),
			zap.String("currency", currency),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get balance: %w", err)
	}

	// Convert accounts balance to contract balance
	return &contracts.Balance{
		UserID:    balance.UserID,
		Currency:  balance.Currency,
		Available: balance.Available,
		Reserved:  balance.Reserved,
		Total:     balance.Total,
		UpdatedAt: balance.UpdatedAt,
	}, nil
}

// GetAllBalances retrieves all balances for a user
func (a *AccountsServiceAdapter) GetAllBalances(ctx context.Context, userID uuid.UUID) ([]*contracts.Balance, error) {
	a.logger.Debug("Getting all balances", zap.String("user_id", userID.String()))

	// Call accounts service to get all balances
	balances, err := a.service.GetAllBalances(ctx, userID)
	if err != nil {
		a.logger.Error("Failed to get all balances",
			zap.String("user_id", userID.String()),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get all balances: %w", err)
	}

	// Convert accounts balances to contract balances
	contractBalances := make([]*contracts.Balance, len(balances))
	for i, balance := range balances {
		contractBalances[i] = &contracts.Balance{
			UserID:    balance.UserID,
			Currency:  balance.Currency,
			Available: balance.Available,
			Reserved:  balance.Reserved,
			Total:     balance.Total,
			UpdatedAt: balance.UpdatedAt,
		}
	}

	return contractBalances, nil
}

// ReserveBalance reserves balance for a transaction
func (a *AccountsServiceAdapter) ReserveBalance(ctx context.Context, req *contracts.ReserveBalanceRequest) (*contracts.ReservationResponse, error) {
	a.logger.Debug("Reserving balance",
		zap.String("user_id", req.UserID.String()),
		zap.String("currency", req.Currency),
		zap.String("amount", req.Amount.String()))

	// Convert contract request to accounts request
	accountsReq := &contracts.ReserveBalanceRequest{
		UserID:          req.UserID,
		Currency:        req.Currency,
		Amount:          req.Amount,
		ReservationType: req.ReservationType,
		ReferenceID:     req.ReferenceID,
		ExpiresAt:       req.ExpiresAt,
		Metadata:        req.Metadata,
	}

	// Call accounts service to reserve balance
	response, err := a.service.ReserveBalance(ctx, accountsReq)
	if err != nil {
		a.logger.Error("Failed to reserve balance",
			zap.String("user_id", req.UserID.String()),
			zap.String("currency", req.Currency),
			zap.String("amount", req.Amount.String()),
			zap.Error(err))
		return nil, fmt.Errorf("failed to reserve balance: %w", err)
	}

	// Convert accounts response to contract response
	return &contracts.ReservationResponse{
		ReservationID: response.ReservationID,
		UserID:        response.UserID,
		Currency:      response.Currency,
		Amount:        response.Amount,
		ExpiresAt:     response.ExpiresAt,
		CreatedAt:     response.CreatedAt,
	}, nil
}

// ReleaseReservation releases a balance reservation
func (a *AccountsServiceAdapter) ReleaseReservation(ctx context.Context, reservationID uuid.UUID) error {
	a.logger.Debug("Releasing reservation", zap.String("reservation_id", reservationID.String()))

	// Call accounts service to release reservation
	if err := a.service.ReleaseReservation(ctx, reservationID); err != nil {
		a.logger.Error("Failed to release reservation",
			zap.String("reservation_id", reservationID.String()),
			zap.Error(err))
		return fmt.Errorf("failed to release reservation: %w", err)
	}

	return nil
}

// TransferBalance transfers balance between users
func (a *AccountsServiceAdapter) TransferBalance(ctx context.Context, req *contracts.TransferRequest) (*contracts.TransferResponse, error) {
	a.logger.Debug("Transferring balance",
		zap.String("from_user_id", req.FromUserID.String()),
		zap.String("to_user_id", req.ToUserID.String()),
		zap.String("currency", req.Currency),
		zap.String("amount", req.Amount.String()))

	// Convert contract request to accounts request
	accountsReq := &contracts.TransferRequest{
		FromUserID:  req.FromUserID,
		ToUserID:    req.ToUserID,
		Currency:    req.Currency,
		Amount:      req.Amount,
		Type:        req.Type,
		ReferenceID: req.ReferenceID,
		Description: req.Description,
		Metadata:    req.Metadata,
	}

	// Call accounts service to transfer balance
	response, err := a.service.TransferBalance(ctx, accountsReq)
	if err != nil {
		a.logger.Error("Failed to transfer balance",
			zap.String("from_user_id", req.FromUserID.String()),
			zap.String("to_user_id", req.ToUserID.String()),
			zap.String("currency", req.Currency),
			zap.String("amount", req.Amount.String()),
			zap.Error(err))
		return nil, fmt.Errorf("failed to transfer balance: %w", err)
	}

	// Convert accounts response to contract response
	return &contracts.TransferResponse{
		TransactionID: response.TransactionID,
		FromUserID:    response.FromUserID,
		ToUserID:      response.ToUserID,
		Currency:      response.Currency,
		Amount:        response.Amount,
		Status:        response.Status,
		CreatedAt:     response.CreatedAt,
	}, nil
}

// BeginXATransaction begins a distributed transaction
func (a *AccountsServiceAdapter) BeginXATransaction(ctx context.Context, req *contracts.XATransactionRequest) (*contracts.XATransaction, error) {
	a.logger.Debug("Beginning XA transaction", zap.String("xid", req.XID))

	// Convert contract operations to accounts operations
	accountsOps := make([]contracts.XAOperation, len(req.Operations))
	for i, op := range req.Operations {
		accountsOps[i] = contracts.XAOperation{
			Type:       op.Type,
			UserID:     op.UserID,
			Currency:   op.Currency,
			Amount:     op.Amount,
			Parameters: op.Parameters,
		}
	}

	// Convert contract request to accounts request
	accountsReq := &contracts.XATransactionRequest{
		XID:        req.XID,
		Resources:  req.Resources,
		Operations: accountsOps,
		Timeout:    req.Timeout,
		Metadata:   req.Metadata,
	}

	// Call accounts service to begin XA transaction
	xa, err := a.service.BeginXATransaction(ctx, accountsReq)
	if err != nil {
		a.logger.Error("Failed to begin XA transaction",
			zap.String("xid", req.XID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to begin XA transaction: %w", err)
	}

	// Convert accounts XA transaction to contract XA transaction
	return &contracts.XATransaction{
		XID:       xa.XID,
		Status:    xa.Status,
		Resources: xa.Resources,
		CreatedAt: xa.CreatedAt,
		ExpiresAt: xa.ExpiresAt,
	}, nil
}

// CommitXATransaction commits a distributed transaction
func (a *AccountsServiceAdapter) CommitXATransaction(ctx context.Context, xaID uuid.UUID) error {
	a.logger.Debug("Committing XA transaction", zap.String("xa_id", xaID.String()))

	// Call accounts service to commit XA transaction
	if err := a.service.CommitXATransaction(ctx, xaID); err != nil {
		a.logger.Error("Failed to commit XA transaction",
			zap.String("xa_id", xaID.String()),
			zap.Error(err))
		return fmt.Errorf("failed to commit XA transaction: %w", err)
	}

	return nil
}

// RollbackXATransaction rolls back a distributed transaction
func (a *AccountsServiceAdapter) RollbackXATransaction(ctx context.Context, xaID uuid.UUID) error {
	a.logger.Debug("Rolling back XA transaction", zap.String("xa_id", xaID.String()))

	// Call accounts service to rollback XA transaction
	if err := a.service.RollbackXATransaction(ctx, xaID); err != nil {
		a.logger.Error("Failed to rollback XA transaction",
			zap.String("xa_id", xaID.String()),
			zap.Error(err))
		return fmt.Errorf("failed to rollback XA transaction: %w", err)
	}

	return nil
}

// GetTransactionHistory retrieves transaction history
func (a *AccountsServiceAdapter) GetTransactionHistory(ctx context.Context, userID uuid.UUID, currency string, limit, offset int) ([]*contracts.Transaction, int64, error) {
	a.logger.Debug("Getting transaction history",
		zap.String("user_id", userID.String()),
		zap.String("currency", currency),
		zap.Int("limit", limit),
		zap.Int("offset", offset))

	// Call accounts service to get transaction history
	transactions, total, err := a.service.GetTransactionHistory(ctx, userID, currency, limit, offset)
	if err != nil {
		a.logger.Error("Failed to get transaction history",
			zap.String("user_id", userID.String()),
			zap.String("currency", currency),
			zap.Error(err))
		return nil, 0, fmt.Errorf("failed to get transaction history: %w", err)
	}

	// Convert accounts transactions to contract transactions
	contractTxs := make([]*contracts.Transaction, len(transactions))
	for i, tx := range transactions {
		contractTxs[i] = &contracts.Transaction{
			ID:          tx.ID,
			UserID:      tx.UserID,
			Type:        tx.Type,
			Currency:    tx.Currency,
			Amount:      tx.Amount,
			Balance:     tx.Balance,
			Status:      tx.Status,
			ReferenceID: tx.ReferenceID,
			Description: tx.Description,
			Metadata:    tx.Metadata,
			CreatedAt:   tx.CreatedAt,
			UpdatedAt:   tx.UpdatedAt,
			CompletedAt: tx.CompletedAt,
		}
	}

	return contractTxs, total, nil
}

// GetTransaction retrieves a specific transaction
func (a *AccountsServiceAdapter) GetTransaction(ctx context.Context, transactionID uuid.UUID) (*contracts.Transaction, error) {
	a.logger.Debug("Getting transaction", zap.String("transaction_id", transactionID.String()))

	// Call accounts service to get transaction
	tx, err := a.service.GetTransaction(ctx, transactionID)
	if err != nil {
		a.logger.Error("Failed to get transaction",
			zap.String("transaction_id", transactionID.String()),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get transaction: %w", err)
	}

	// Convert accounts transaction to contract transaction
	return &contracts.Transaction{
		ID:          tx.ID,
		UserID:      tx.UserID,
		Type:        tx.Type,
		Currency:    tx.Currency,
		Amount:      tx.Amount,
		Balance:     tx.Balance,
		Status:      tx.Status,
		ReferenceID: tx.ReferenceID,
		Description: tx.Description,
		Metadata:    tx.Metadata,
		CreatedAt:   tx.CreatedAt,
		UpdatedAt:   tx.UpdatedAt,
		CompletedAt: tx.CompletedAt,
	}, nil
}

// CreateAccount creates a new account
func (a *AccountsServiceAdapter) CreateAccount(ctx context.Context, userID uuid.UUID, currency string) (*contracts.Account, error) {
	a.logger.Debug("Creating account",
		zap.String("user_id", userID.String()),
		zap.String("currency", currency))

	// Call accounts service to create account
	account, err := a.service.CreateAccount(ctx, userID, currency)
	if err != nil {
		a.logger.Error("Failed to create account",
			zap.String("user_id", userID.String()),
			zap.String("currency", currency),
			zap.Error(err))
		return nil, fmt.Errorf("failed to create account: %w", err)
	}

	// Convert accounts account to contract account
	return &contracts.Account{
		ID:        account.ID,
		UserID:    account.UserID,
		Currency:  account.Currency,
		Balance:   account.Balance,
		Available: account.Available,
		Reserved:  account.Reserved,
		Status:    account.Status,
		CreatedAt: account.CreatedAt,
		UpdatedAt: account.UpdatedAt,
	}, nil
}

// GetAccount retrieves account information
func (a *AccountsServiceAdapter) GetAccount(ctx context.Context, userID uuid.UUID, currency string) (*contracts.Account, error) {
	a.logger.Debug("Getting account",
		zap.String("user_id", userID.String()),
		zap.String("currency", currency))

	// Call accounts service to get account
	account, err := a.service.GetAccount(ctx, userID, currency)
	if err != nil {
		a.logger.Error("Failed to get account",
			zap.String("user_id", userID.String()),
			zap.String("currency", currency),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get account: %w", err)
	}

	// Convert accounts account to contract account
	return &contracts.Account{
		ID:        account.ID,
		UserID:    account.UserID,
		Currency:  account.Currency,
		Balance:   account.Balance,
		Available: account.Available,
		Reserved:  account.Reserved,
		Status:    account.Status,
		CreatedAt: account.CreatedAt,
		UpdatedAt: account.UpdatedAt,
	}, nil
}

// LockAccount locks an account
func (a *AccountsServiceAdapter) LockAccount(ctx context.Context, userID uuid.UUID, currency string, reason string) error {
	a.logger.Debug("Locking account",
		zap.String("user_id", userID.String()),
		zap.String("currency", currency),
		zap.String("reason", reason))

	// Call accounts service to lock account
	if err := a.service.LockAccount(ctx, userID, currency, reason); err != nil {
		a.logger.Error("Failed to lock account",
			zap.String("user_id", userID.String()),
			zap.String("currency", currency),
			zap.String("reason", reason),
			zap.Error(err))
		return fmt.Errorf("failed to lock account: %w", err)
	}

	return nil
}

// UnlockAccount unlocks an account
func (a *AccountsServiceAdapter) UnlockAccount(ctx context.Context, userID uuid.UUID, currency string) error {
	a.logger.Debug("Unlocking account",
		zap.String("user_id", userID.String()),
		zap.String("currency", currency))

	// Call accounts service to unlock account
	if err := a.service.UnlockAccount(ctx, userID, currency); err != nil {
		a.logger.Error("Failed to unlock account",
			zap.String("user_id", userID.String()),
			zap.String("currency", currency),
			zap.Error(err))
		return fmt.Errorf("failed to unlock account: %w", err)
	}

	return nil
}

// ReconcileBalances performs balance reconciliation
func (a *AccountsServiceAdapter) ReconcileBalances(ctx context.Context, req *contracts.ReconciliationRequest) (*contracts.ReconciliationResult, error) {
	a.logger.Debug("Reconciling balances")

	// Convert contract request to accounts request
	accountsReq := &contracts.ReconciliationRequest{
		UserIDs:    req.UserIDs,
		Currencies: req.Currencies,
		StartTime:  req.StartTime,
		EndTime:    req.EndTime,
		Force:      req.Force,
	}

	// Call accounts service to reconcile balances
	result, err := a.service.ReconcileBalances(ctx, accountsReq)
	if err != nil {
		a.logger.Error("Failed to reconcile balances", zap.Error(err))
		return nil, fmt.Errorf("failed to reconcile balances: %w", err)
	}

	// Convert accounts discrepancies to contract discrepancies
	contractDiscrepancies := make([]contracts.BalanceDiscrepancy, len(result.Discrepancies))
	for i, discrepancy := range result.Discrepancies {
		contractDiscrepancies[i] = contracts.BalanceDiscrepancy{
			UserID:          discrepancy.UserID,
			Currency:        discrepancy.Currency,
			ExpectedBalance: discrepancy.ExpectedBalance,
			ActualBalance:   discrepancy.ActualBalance,
			Difference:      discrepancy.Difference,
			Severity:        discrepancy.Severity,
		}
	}

	// Convert accounts result to contract result
	return &contracts.ReconciliationResult{
		TotalAccounts:    result.TotalAccounts,
		ReconciledCount:  result.ReconciledCount,
		DiscrepancyCount: result.DiscrepancyCount,
		Discrepancies:    contractDiscrepancies,
		Summary:          result.Summary,
		ProcessedAt:      result.ProcessedAt,
	}, nil
}

// ValidateBalanceIntegrity validates balance integrity
func (a *AccountsServiceAdapter) ValidateBalanceIntegrity(ctx context.Context, userID uuid.UUID) (*contracts.IntegrityReport, error) {
	a.logger.Debug("Validating balance integrity", zap.String("user_id", userID.String()))

	// Call accounts service to validate balance integrity
	report, err := a.service.ValidateBalanceIntegrity(ctx, userID)
	if err != nil {
		a.logger.Error("Failed to validate balance integrity",
			zap.String("user_id", userID.String()),
			zap.Error(err))
		return nil, fmt.Errorf("failed to validate balance integrity: %w", err)
	}

	// Convert accounts issues to contract issues
	contractIssues := make([]contracts.IntegrityIssue, len(report.Issues))
	for i, issue := range report.Issues {
		contractIssues[i] = contracts.IntegrityIssue{
			Type:        issue.Type,
			Currency:    issue.Currency,
			Description: issue.Description,
			Impact:      issue.Impact,
			Severity:    issue.Severity,
		}
	}

	// Convert accounts report to contract report
	return &contracts.IntegrityReport{
		UserID:    report.UserID,
		IsValid:   report.IsValid,
		Issues:    contractIssues,
		Metrics:   report.Metrics,
		CheckedAt: report.CheckedAt,
	}, nil
}

// HealthCheck performs health check
func (a *AccountsServiceAdapter) HealthCheck(ctx context.Context) (*contracts.HealthStatus, error) {
	a.logger.Debug("Performing health check")

	// Call accounts service health check
	health, err := a.service.HealthCheck(ctx)
	if err != nil {
		a.logger.Error("Health check failed", zap.Error(err))
		return nil, fmt.Errorf("health check failed: %w", err)
	}

	// Convert accounts health status to contract health status
	return &contracts.HealthStatus{
		Status:       health.Status,
		Timestamp:    health.Timestamp,
		Version:      health.Version,
		Uptime:       health.Uptime,
		Metrics:      health.Metrics,
		Dependencies: health.Dependencies,
	}, nil
}
