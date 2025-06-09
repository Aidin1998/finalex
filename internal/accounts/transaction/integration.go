package transaction

import (
	"context"
	"fmt"
	"time"

	"github.com/Aidin1998/finalex/internal/accounts/bookkeeper"
	"github.com/Aidin1998/finalex/internal/trading/settlement"
	"github.com/google/uuid"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// TransactionManagerSuite provides a complete transaction management solution
type TransactionManagerSuite struct {
	XAManager     *XATransactionManager
	LockManager   *DistributedLockManager
	ConfigManager *TransactionConfigManager
	Middleware    *TransactionMiddleware

	// XA Resources
	BookkeeperXA *BookkeeperXAResource
	SettlementXA *SettlementXAResource
	TradingXA    *TradingXAResource
	FiatXA       *FiatXAResource

	// New integrated components
	WorkflowOrchestrator *DistributedTransactionOrchestrator
	PerformanceMetrics   *PerformanceMetricsCollector
	MonitoringService    *TransactionMonitor
	TestingFramework     *DistributedTransactionTester

	logger *zap.Logger
	db     *gorm.DB
}

// NewTransactionManagerSuite creates a complete transaction management suite
func NewTransactionManagerSuite(
	db *gorm.DB,
	logger *zap.Logger,
	bookkeeperSvc bookkeeper.BookkeeperService,
	settlementEngine *settlement.SettlementEngine,
	tradingPathManager *TradingPathManager, // changed from MatchingEngine
	fiatSvc interface{}, // TODO: Replace interface{} with the correct FiatService type
	configPath string,
) (*TransactionManagerSuite, error) {
	// Initialize core components
	xaManager := NewXATransactionManager(logger, 5*time.Minute)
	lockManager := NewDistributedLockManager(db, logger)
	configManager := NewTransactionConfigManager(db, configPath)

	// Load configuration
	if err := configManager.LoadConfig(); err != nil {
		logger.Warn("Failed to load config, using defaults", zap.Error(err))
	}
	// config := configManager.GetConfig() // not used

	// Initialize XA Resources
	bookkeeperXAService := bookkeeper.NewBookkeeperXAAdapter(bookkeeperSvc, nil)
	bookkeeperXA := NewBookkeeperXAResource(bookkeeperXAService, logger)
	settlementXA := NewSettlementXAResource(settlementEngine, db, logger)
	tradingXA := NewTradingXAResource(db, tradingPathManager, logger)
	fiatXA := NewFiatXAResource(fiatSvc, db, logger, "")
	// Initialize middleware
	middleware := NewTransactionMiddleware(xaManager, lockManager, logger)

	// Configure middleware with endpoint locking requirements
	configureMiddlewareLocking(middleware)

	// Initialize new integrated components
	workflowOrchestrator := NewDistributedTransactionOrchestrator(
		db, logger, bookkeeperSvc, nil, nil, settlementEngine, fiatSvc,
	)

	performanceMetrics := NewPerformanceMetricsCollector(db)
	monitoringService := NewTransactionMonitor(db)
	testingFramework := NewDistributedTransactionTester(nil, db, logger) // Will set suite reference after creation

	suite := &TransactionManagerSuite{
		XAManager:            xaManager,
		LockManager:          lockManager,
		ConfigManager:        configManager,
		Middleware:           middleware,
		BookkeeperXA:         bookkeeperXA,
		SettlementXA:         settlementXA,
		TradingXA:            tradingXA,
		FiatXA:               fiatXA,
		WorkflowOrchestrator: workflowOrchestrator,
		PerformanceMetrics:   performanceMetrics,
		MonitoringService:    monitoringService,
		TestingFramework:     testingFramework,
		logger:               logger,
		db:                   db,
	}

	// Set the suite reference in the testing framework
	testingFramework.suite = suite

	// Register configuration watchers
	if err := suite.registerConfigWatchers(); err != nil {
		logger.Warn("Failed to register config watchers", zap.Error(err))
	}

	return suite, nil
}

// Start initializes and starts all transaction management components
func (tms *TransactionManagerSuite) Start(ctx context.Context) error {
	tms.logger.Info("Starting Transaction Manager Suite")

	// Start monitoring service
	if err := tms.MonitoringService.Start(ctx); err != nil {
		return fmt.Errorf("failed to start monitoring service: %w", err)
	}

	// Start performance metrics collection
	if err := tms.PerformanceMetrics.Start(ctx); err != nil {
		return fmt.Errorf("failed to start performance metrics: %w", err)
	}
	// }

	// Start recovery manager
	// if err := tms.RecoveryManager.Start(ctx); err != nil {
	// 	return fmt.Errorf("failed to start recovery manager: %w", err)
	// }

	// Enable configuration auto-reload
	tms.ConfigManager.EnableAutoReload(5 * time.Minute)

	// Start workflow orchestrator
	// if err := tms.WorkflowOrchestrator.Start(ctx); err != nil {
	// 	return fmt.Errorf("failed to start workflow orchestrator: %w", err)
	// }

	tms.logger.Info("Transaction Manager Suite started successfully")
	return nil
}

// Stop gracefully shuts down all transaction management components
func (tms *TransactionManagerSuite) Stop(ctx context.Context) error {
	tms.logger.Info("Stopping Transaction Manager Suite")

	// Stop components in reverse order
	// if err := tms.WorkflowOrchestrator.Stop(ctx); err != nil {
	// 	tms.logger.Error("Failed to stop workflow orchestrator", zap.Error(err))
	// }

	tms.ConfigManager.DisableAutoReload()

	// if err := tms.RecoveryManager.Stop(ctx); err != nil {
	// 	tms.logger.Error("Failed to stop recovery manager", zap.Error(err))
	// }
	// Stop performance metrics collection
	tms.PerformanceMetrics.Stop()

	// Stop monitoring service
	tms.MonitoringService.Stop()

	// Stop XA manager
	tms.XAManager.Stop()

	tms.logger.Info("Transaction Manager Suite stopped")
	return nil
}

// GetHealthCheck returns the health status of all transaction components
func (tms *TransactionManagerSuite) GetHealthCheck() map[string]interface{} {
	return map[string]interface{}{
		"xa_manager": map[string]interface{}{
			"active_transactions": len(tms.XAManager.transactions),
			"metrics":             tms.XAManager.GetMetrics(),
		},
		"lock_manager": map[string]interface{}{
			"active_locks": len(tms.LockManager.GetActiveLocks()),
		},
	}
}

// ExecuteDistributedTransaction executes a transaction across multiple services
func (tms *TransactionManagerSuite) ExecuteDistributedTransaction(
	ctx context.Context,
	operations []TransactionOperation,
	timeout time.Duration,
) (*TransactionResult, error) {
	// Start XA transaction
	txn, err := tms.XAManager.Start(ctx, timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to start XA transaction: %w", err)
	}

	// Add transaction to context
	ctx = WithXATransaction(ctx, txn)

	// Enlist resources based on operations
	resources := make(map[string]XAResource)
	for _, op := range operations {
		switch op.Service {
		case "bookkeeper":
			if _, exists := resources["bookkeeper"]; !exists {
				if err := tms.XAManager.Enlist(txn, tms.BookkeeperXA); err != nil {
					return nil, fmt.Errorf("failed to enlist bookkeeper: %w", err)
				}
				resources["bookkeeper"] = tms.BookkeeperXA
			}
		case "settlement":
			if _, exists := resources["settlement"]; !exists {
				if err := tms.XAManager.Enlist(txn, tms.SettlementXA); err != nil {
					return nil, fmt.Errorf("failed to enlist settlement: %w", err)
				}
				resources["settlement"] = tms.SettlementXA
			}
		case "trading":
			if _, exists := resources["trading"]; !exists {
				if err := tms.XAManager.Enlist(txn, tms.TradingXA); err != nil {
					return nil, fmt.Errorf("failed to enlist trading: %w", err)
				}
				resources["trading"] = tms.TradingXA
			}
		case "fiat":
			if _, exists := resources["fiat"]; !exists {
				if err := tms.XAManager.Enlist(txn, tms.FiatXA); err != nil {
					return nil, fmt.Errorf("failed to enlist fiat: %w", err)
				}
				resources["fiat"] = tms.FiatXA
			}
		}
	}

	// Execute operations
	for _, op := range operations {
		if err := tms.executeOperation(ctx, op); err != nil {
			// Abort transaction on operation failure
			if abortErr := tms.XAManager.Abort(ctx, txn); abortErr != nil {
				tms.logger.Error("Failed to abort transaction after operation failure",
					zap.Error(abortErr))
			}
			return nil, fmt.Errorf("operation failed: %w", err)
		}
	}

	// Commit transaction
	if err := tms.XAManager.Commit(ctx, txn); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return &TransactionResult{
		TransactionID: txn.ID,
		Status:        "committed",
		Operations:    operations,
		Timestamp:     time.Now(),
	}, nil
}

// executeOperation executes a single operation within a transaction
func (tms *TransactionManagerSuite) executeOperation(ctx context.Context, op TransactionOperation) error {
	switch op.Service {
	case "bookkeeper":
		return tms.executeBookkeeperOperation(ctx, op)
	case "settlement":
		return tms.executeSettlementOperation(ctx, op)
	case "trading":
		return tms.executeTradingOperation(ctx, op)
	case "fiat":
		return tms.executeFiatOperation(ctx, op)
	default:
		return fmt.Errorf("unknown service: %s", op.Service)
	}
}

// Helper methods for executing operations on specific services
func (tms *TransactionManagerSuite) executeBookkeeperOperation(ctx context.Context, op TransactionOperation) error {
	txn := GetXATransactionFromContext(ctx)
	if txn == nil {
		return fmt.Errorf("no active transaction in context")
	}

	switch op.Operation {
	case "lock_funds":
		userID := op.Parameters["user_id"].(string)
		currency := op.Parameters["currency"].(string)
		amount := op.Parameters["amount"].(float64)
		// Fix: Use LockFundsXA for XA transactions
		return tms.BookkeeperXA.LockFundsXA(ctx, txn.XID, userID, currency, amount)
	case "transfer_funds":
		fromUserID := op.Parameters["from_user_id"].(string)
		toUserID := op.Parameters["to_user_id"].(string)
		currency := op.Parameters["currency"].(string)
		amount := op.Parameters["amount"].(float64)
		description := op.Parameters["description"].(string)
		return tms.BookkeeperXA.TransferFunds(ctx, txn.XID, fromUserID, toUserID, currency, amount, description)
	default:
		return fmt.Errorf("unknown bookkeeper operation: %s", op.Operation)
	}
}

func (tms *TransactionManagerSuite) executeSettlementOperation(ctx context.Context, op TransactionOperation) error {
	txn := GetXATransactionFromContext(ctx)
	if txn == nil {
		return fmt.Errorf("no active transaction in context")
	}

	switch op.Operation {
	case "capture_trade":
		trade := op.Parameters["trade"].(settlement.TradeCapture)
		return tms.SettlementXA.CaptureTrade(ctx, txn.XID, trade)
	case "clear_and_settle":
		return tms.SettlementXA.ClearAndSettle(ctx, txn.XID)
	default:
		return fmt.Errorf("unknown settlement operation: %s", op.Operation)
	}
}

func (tms *TransactionManagerSuite) executeTradingOperation(ctx context.Context, op TransactionOperation) error {
	// Trading operations would be implemented here
	return fmt.Errorf("trading operations not yet implemented in integration")
}

func (tms *TransactionManagerSuite) executeFiatOperation(ctx context.Context, op TransactionOperation) error {
	// Fiat operations would be implemented here
	return fmt.Errorf("fiat operations not yet implemented in integration")
}

// configureMiddlewareLocking sets up locking requirements for different endpoints
func configureMiddlewareLocking(middleware *TransactionMiddleware) {
	// Trading endpoints require order book locks
	middleware.WithLocking("/api/v1/trading/orders", []string{"order_book", "user_balance"})
	middleware.WithLocking("/api/v1/trading/cancel", []string{"order_book"})

	// Withdrawal endpoints require wallet locks
	middleware.WithLocking("/api/v1/wallet/withdraw", []string{"user_balance", "wallet_state"})

	// Transfer endpoints require user balance locks
	middleware.WithLocking("/api/v1/bookkeeper/transfer", []string{"user_balance"})

	// Fiat operations require compliance locks
	middleware.WithLocking("/api/v1/fiat/deposit", []string{"user_balance", "compliance"})
	middleware.WithLocking("/api/v1/fiat/withdraw", []string{"user_balance", "compliance", "bank_details"})
}

// registerConfigWatchers sets up configuration change monitoring
func (tms *TransactionManagerSuite) registerConfigWatchers() error {
	// Register XA Manager config watcher
	xaWatcher := &XAManagerConfigWatcher{xaManager: tms.XAManager}
	tms.ConfigManager.RegisterWatcher(xaWatcher)

	return nil
}

// TransactionOperation represents a single operation within a distributed transaction
type TransactionOperation struct {
	Service    string                 `json:"service"`    // bookkeeper, settlement, trading, fiat, wallet
	Operation  string                 `json:"operation"`  // operation name
	Parameters map[string]interface{} `json:"parameters"` // operation parameters
}

// TransactionResult represents the result of a distributed transaction
type TransactionResult struct {
	TransactionID uuid.UUID              `json:"transaction_id"`
	Status        string                 `json:"status"`
	Operations    []TransactionOperation `json:"operations"`
	Timestamp     time.Time              `json:"timestamp"`
	Error         string                 `json:"error,omitempty"`
}

// GetTransactionManagerSuite returns a configured transaction manager suite instance
// This is the main entry point for integrating distributed transactions
func GetTransactionManagerSuite(
	db *gorm.DB,
	logger *zap.Logger,
	bookkeeperSvc bookkeeper.BookkeeperService,
	settlementEngine *settlement.SettlementEngine,
	configPath string,
) (*TransactionManagerSuite, error) {
	return NewTransactionManagerSuite(
		db,
		logger,
		bookkeeperSvc,
		settlementEngine,
		nil, // trading engine - pass if available
		nil, // fiat service - pass if available
		configPath,
	)
}
