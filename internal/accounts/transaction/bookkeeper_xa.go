// Package transaction provides XA resource implementations for bookkeeper service
package transaction

import (
	"context"
	"time"

	"github.com/Aidin1998/finalex/internal/accounts/bookkeeper"
	"github.com/Aidin1998/finalex/pkg/models"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// BookkeeperXAResource implements XA interface for bookkeeper operations
// Acts as a thin adapter that delegates to the BookkeeperXAService
type BookkeeperXAResource struct {
	bookkeeper bookkeeper.BookkeeperXAService
	logger     *zap.Logger
}

// BookkeeperTransaction represents a bookkeeper transaction in XA context
type BookkeeperTransaction struct {
	XID              XID
	Operations       []BookkeeperOperation
	CompensationData map[string]interface{}
	DBTransaction    *gorm.DB
	State            string
	CreatedAt        time.Time
}

// BookkeeperOperation represents an operation within a bookkeeper transaction
type BookkeeperOperation struct {
	Type        string // "lock_funds", "unlock_funds", "transfer", "create_transaction"
	UserID      string
	Currency    string
	Amount      float64
	FromUserID  string
	ToUserID    string
	Reference   string
	Description string
	Result      interface{}
	Error       error
}

// NewBookkeeperXAResource creates a new XA resource for bookkeeper operations
// It now expects a BookkeeperXAService rather than just BookkeeperService to delegate XA operations
func NewBookkeeperXAResource(bookkeeper bookkeeper.BookkeeperXAService, logger *zap.Logger) *BookkeeperXAResource {
	return &BookkeeperXAResource{
		bookkeeper: bookkeeper,
		logger:     logger,
	}
}

// GetResourceName returns the name of this resource
func (bxa *BookkeeperXAResource) GetResourceName() string {
	return bxa.bookkeeper.GetResourceName()
}

// Start begins a new XA transaction branch
// This adapts the XAResource Start method to BookkeeperXAService StartXA method
func (bxa *BookkeeperXAResource) Start(ctx context.Context, xid XID) error {
	xaXID := convertToBookkeeperXID(xid)
	err := bxa.bookkeeper.StartXA(xaXID)
	if err != nil {
		bxa.logger.Error("Failed to start XA transaction",
			zap.String("xid", xaXID.String()),
			zap.Error(err))
	} else {
		bxa.logger.Debug("Started XA transaction",
			zap.String("xid", xaXID.String()))
	}
	return err
}

// End ends the association of the XA resource with the transaction
func (bxa *BookkeeperXAResource) End(ctx context.Context, xid XID, flags int) error {
	xaXID := convertToBookkeeperXID(xid)
	err := bxa.bookkeeper.EndXA(xaXID, flags)
	if err != nil {
		bxa.logger.Error("Failed to end XA transaction",
			zap.String("xid", xaXID.String()),
			zap.Error(err))
	}
	return err
}

// Prepare implements XA prepare phase
func (bxa *BookkeeperXAResource) Prepare(ctx context.Context, xid XID) (bool, error) {
	xaXID := convertToBookkeeperXID(xid)
	err := bxa.bookkeeper.PrepareXA(xaXID)
	if err != nil {
		bxa.logger.Error("Failed to prepare XA transaction",
			zap.String("xid", xaXID.String()),
			zap.Error(err))
		return false, err
	}
	return true, nil
}

// Commit implements XA commit phase
func (bxa *BookkeeperXAResource) Commit(ctx context.Context, xid XID, onePhase bool) error {
	xaXID := convertToBookkeeperXID(xid)
	err := bxa.bookkeeper.CommitXA(xaXID, onePhase)
	if err != nil {
		bxa.logger.Error("Failed to commit XA transaction",
			zap.String("xid", xaXID.String()),
			zap.Error(err))
	}
	return err
}

// Rollback implements XA rollback phase
func (bxa *BookkeeperXAResource) Rollback(ctx context.Context, xid XID) error {
	xaXID := convertToBookkeeperXID(xid)
	err := bxa.bookkeeper.RollbackXA(xaXID)
	if err != nil {
		bxa.logger.Error("Failed to rollback XA transaction",
			zap.String("xid", xaXID.String()),
			zap.Error(err))
	}
	return err
}

// Forget implements XA forget phase (for heuristic completions)
func (bxa *BookkeeperXAResource) Forget(ctx context.Context, xid XID) error {
	// Most implementations don't need special handling for Forget
	// We just log it and return success
	bxa.logger.Info("Forget called for XA transaction",
		zap.String("xid", xid.String()))
	return nil
}

// Recover implements XA recovery phase
func (bxa *BookkeeperXAResource) Recover(ctx context.Context, flags int) ([]XID, error) {
	bookeeperXIDs, err := bxa.bookkeeper.RecoverXA()
	if err != nil {
		bxa.logger.Error("Failed to recover XA transactions",
			zap.Error(err))
		return nil, err
	}

	// Convert BookkeeperXIDs to XIDs
	var xids []XID
	for _, bxid := range bookeeperXIDs {
		xids = append(xids, convertFromBookkeeperXID(bxid))
	}

	return xids, nil
}

// XA-specific operations that delegate to the BookkeeperXAService

// LockFundsXA locks funds within the XA transaction
func (bxa *BookkeeperXAResource) LockFundsXA(ctx context.Context, xid XID, userID, currency string, amount float64) error {
	xaXID := convertToBookkeeperXID(xid)
	return bxa.bookkeeper.LockFundsXA(ctx, xaXID, userID, currency, decimal.NewFromFloat(amount))
}

// UnlockFundsXA unlocks funds within the XA transaction
func (bxa *BookkeeperXAResource) UnlockFundsXA(ctx context.Context, xid XID, userID, currency string, amount float64) error {
	xaXID := convertToBookkeeperXID(xid)
	return bxa.bookkeeper.UnlockFundsXA(ctx, xaXID, userID, currency, decimal.NewFromFloat(amount))
}

// TransferFunds transfers funds between accounts within the XA transaction
func (bxa *BookkeeperXAResource) TransferFunds(ctx context.Context, xid XID, fromUserID, toUserID, currency string, amount float64, description string) error {
	xaXID := convertToBookkeeperXID(xid)
	return bxa.bookkeeper.TransferFundsXA(ctx, xaXID, fromUserID, toUserID, currency, decimal.NewFromFloat(amount), description)
}

// Helper functions for XID conversion between transaction.XID and bookkeeper.XID

func convertToBookkeeperXID(xid XID) bookkeeper.XID {
	return bookkeeper.XID{
		FormatID:    int32(xid.FormatID),
		BranchID:    xid.BranchQualID,
		GlobalTxnID: xid.GlobalTxnID,
	}
}

func convertFromBookkeeperXID(bxid bookkeeper.XID) XID {
	return XID{
		FormatID:     int32(bxid.FormatID),
		GlobalTxnID:  bxid.GlobalTxnID,
		BranchQualID: bxid.BranchID,
	}
}

// Implement BookkeeperService interface by delegating to the underlying service

// StartService implements the BookkeeperService interface's Start method
func (bxa *BookkeeperXAResource) StartService() error {
	return bxa.bookkeeper.Start()
}

// StopService implements the BookkeeperService interface's Stop method
func (bxa *BookkeeperXAResource) StopService() error {
	return bxa.bookkeeper.Stop()
}

// GetAccounts implements the BookkeeperService interface
func (bxa *BookkeeperXAResource) GetAccounts(ctx context.Context, userID string) ([]*models.Account, error) {
	return bxa.bookkeeper.GetAccounts(ctx, userID)
}

// GetAccount implements the BookkeeperService interface
func (bxa *BookkeeperXAResource) GetAccount(ctx context.Context, userID, currency string) (*models.Account, error) {
	return bxa.bookkeeper.GetAccount(ctx, userID, currency)
}

// CreateAccount implements the BookkeeperService interface
func (bxa *BookkeeperXAResource) CreateAccount(ctx context.Context, userID, currency string) (*models.Account, error) {
	return bxa.bookkeeper.CreateAccount(ctx, userID, currency)
}

// GetAccountTransactions implements the BookkeeperService interface
func (bxa *BookkeeperXAResource) GetAccountTransactions(ctx context.Context, userID, currency string, limit, offset int) ([]*models.Transaction, int64, error) {
	return bxa.bookkeeper.GetAccountTransactions(ctx, userID, currency, limit, offset)
}

// CreateTransaction implements the BookkeeperService interface
func (bxa *BookkeeperXAResource) CreateTransaction(ctx context.Context, userID, transactionType string, amount float64, currency, reference, description string) (*models.Transaction, error) {
	return bxa.bookkeeper.CreateTransaction(ctx, userID, transactionType, decimal.NewFromFloat(amount), currency, reference, description)
}

// CompleteTransaction implements the BookkeeperService interface
func (bxa *BookkeeperXAResource) CompleteTransaction(ctx context.Context, transactionID string) error {
	return bxa.bookkeeper.CompleteTransaction(ctx, transactionID)
}

// FailTransaction implements the BookkeeperService interface
func (bxa *BookkeeperXAResource) FailTransaction(ctx context.Context, transactionID string) error {
	return bxa.bookkeeper.FailTransaction(ctx, transactionID)
}

// LockFunds implements the BookkeeperService interface (non-XA version)
func (bxa *BookkeeperXAResource) LockFunds(ctx context.Context, userID, currency string, amount float64) error {
	return bxa.bookkeeper.LockFunds(ctx, userID, currency, decimal.NewFromFloat(amount))
}

// UnlockFunds implements the BookkeeperService interface (non-XA version)
func (bxa *BookkeeperXAResource) UnlockFunds(ctx context.Context, userID, currency string, amount float64) error {
	return bxa.bookkeeper.UnlockFunds(ctx, userID, currency, decimal.NewFromFloat(amount))
}

// BatchGetAccounts implements the BookkeeperService interface
func (bxa *BookkeeperXAResource) BatchGetAccounts(ctx context.Context, userIDs []string, currencies []string) (map[string]map[string]*models.Account, error) {
	return bxa.bookkeeper.BatchGetAccounts(ctx, userIDs, currencies)
}

// BatchLockFunds implements the BookkeeperService interface
func (bxa *BookkeeperXAResource) BatchLockFunds(ctx context.Context, operations []bookkeeper.FundsOperation) (*bookkeeper.BatchOperationResult, error) {
	return bxa.bookkeeper.BatchLockFunds(ctx, operations)
}

// BatchUnlockFunds implements the BookkeeperService interface
func (bxa *BookkeeperXAResource) BatchUnlockFunds(ctx context.Context, operations []bookkeeper.FundsOperation) (*bookkeeper.BatchOperationResult, error) {
	return bxa.bookkeeper.BatchUnlockFunds(ctx, operations)
}

// Note: BookkeeperXAResource doesn't directly implement BookkeeperService
// due to method signature conflict with Start()
// It implements XAResource interface
var _ XAResource = (*BookkeeperXAResource)(nil)
