package messaging

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Aidin1998/finalex/internal/accounts/bookkeeper"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// BookkeeperMessageService handles bookkeeper operations via message queue
type BookkeeperMessageService struct {
	bookkeeper bookkeeper.BookkeeperService
	messageBus *MessageBus
	logger     *zap.Logger
}

// NewBookkeeperMessageService creates a new message-driven bookkeeper service
func NewBookkeeperMessageService(
	bookkeeperSvc bookkeeper.BookkeeperService,
	messageBus *MessageBus,
	logger *zap.Logger,
) *BookkeeperMessageService {
	service := &BookkeeperMessageService{
		bookkeeper: bookkeeperSvc,
		messageBus: messageBus,
		logger:     logger,
	}

	// Register message handlers
	service.registerHandlers()

	return service
}

// registerHandlers registers all message handlers for bookkeeper operations
func (s *BookkeeperMessageService) registerHandlers() {
	s.messageBus.RegisterHandler(MsgFundsLocked, s.handleFundsLocked)
	s.messageBus.RegisterHandler(MsgFundsUnlocked, s.handleFundsUnlocked)
	s.messageBus.RegisterHandler(MsgTradeExecuted, s.handleTradeExecuted)
	s.messageBus.RegisterHandler(MsgBalanceUpdated, s.handleBalanceUpdate)
}

// handleFundsLocked processes fund locking requests with optimized account retrieval
func (s *BookkeeperMessageService) handleFundsLocked(ctx context.Context, msg *ReceivedMessage) error {
	var fundsMsg FundsOperationMessage
	if err := json.Unmarshal(msg.Value, &fundsMsg); err != nil {
		return fmt.Errorf("failed to unmarshal funds operation message: %w", err)
	}

	s.logger.Info("Processing funds lock request",
		zap.String("user_id", fundsMsg.UserID),
		zap.String("currency", fundsMsg.Currency),
		zap.String("amount", fundsMsg.Amount.String()),
		zap.String("order_id", fundsMsg.OrderID))

	// Get current balance for event (single call optimization)
	oldAccount, err := s.bookkeeper.GetAccount(ctx, fundsMsg.UserID, fundsMsg.Currency)
	if err != nil {
		return fmt.Errorf("failed to get account before lock: %w", err)
	}

	// Calculate expected new values for better performance monitoring
	expectedNewAvailable := oldAccount.Available - fundsMsg.Amount.InexactFloat64()
	expectedNewLocked := oldAccount.Locked + fundsMsg.Amount.InexactFloat64()

	// Lock the funds
	err = s.bookkeeper.LockFunds(ctx, fundsMsg.UserID, fundsMsg.Currency, fundsMsg.Amount.InexactFloat64())
	if err != nil {
		s.logger.Error("Failed to lock funds",
			zap.Error(err),
			zap.String("user_id", fundsMsg.UserID),
			zap.String("currency", fundsMsg.Currency))
		return err
	}

	// Publish balance update event with calculated values (eliminates second GetAccount call)
	balanceEvent := &BalanceEventMessage{
		BaseMessage:  NewBaseMessage(MsgBalanceUpdated, "bookkeeper", fundsMsg.MessageID),
		UserID:       fundsMsg.UserID,
		Currency:     fundsMsg.Currency,
		OldBalance:   decimal.NewFromFloat(oldAccount.Balance),
		NewBalance:   decimal.NewFromFloat(oldAccount.Balance), // Balance unchanged in lock operation
		OldAvailable: decimal.NewFromFloat(oldAccount.Available),
		NewAvailable: decimal.NewFromFloat(expectedNewAvailable),
		OldLocked:    decimal.NewFromFloat(oldAccount.Locked),
		NewLocked:    decimal.NewFromFloat(expectedNewLocked),
		Amount:       fundsMsg.Amount,
		Reference:    fundsMsg.OrderID,
		Description:  fmt.Sprintf("Funds locked for order: %s", fundsMsg.Reason),
	}

	return s.messageBus.PublishBalanceEvent(ctx, balanceEvent)
}

// handleFundsUnlocked processes fund unlocking requests with optimized account retrieval
func (s *BookkeeperMessageService) handleFundsUnlocked(ctx context.Context, msg *ReceivedMessage) error {
	var fundsMsg FundsOperationMessage
	if err := json.Unmarshal(msg.Value, &fundsMsg); err != nil {
		return fmt.Errorf("failed to unmarshal funds operation message: %w", err)
	}

	s.logger.Info("Processing funds unlock request",
		zap.String("user_id", fundsMsg.UserID),
		zap.String("currency", fundsMsg.Currency),
		zap.String("amount", fundsMsg.Amount.String()),
		zap.String("order_id", fundsMsg.OrderID))

	// Get current balance for event (single call optimization)
	oldAccount, err := s.bookkeeper.GetAccount(ctx, fundsMsg.UserID, fundsMsg.Currency)
	if err != nil {
		return fmt.Errorf("failed to get account before unlock: %w", err)
	}

	// Calculate expected new values for better performance monitoring
	expectedNewAvailable := oldAccount.Available + fundsMsg.Amount.InexactFloat64()
	expectedNewLocked := oldAccount.Locked - fundsMsg.Amount.InexactFloat64()

	// Unlock the funds
	err = s.bookkeeper.UnlockFunds(ctx, fundsMsg.UserID, fundsMsg.Currency, fundsMsg.Amount.InexactFloat64())
	if err != nil {
		s.logger.Error("Failed to unlock funds",
			zap.Error(err),
			zap.String("user_id", fundsMsg.UserID),
			zap.String("currency", fundsMsg.Currency))
		return err
	}

	// Publish balance update event with calculated values (eliminates second GetAccount call)
	balanceEvent := &BalanceEventMessage{
		BaseMessage:  NewBaseMessage(MsgBalanceUpdated, "bookkeeper", fundsMsg.MessageID),
		UserID:       fundsMsg.UserID,
		Currency:     fundsMsg.Currency,
		OldBalance:   decimal.NewFromFloat(oldAccount.Balance),
		NewBalance:   decimal.NewFromFloat(oldAccount.Balance), // Balance unchanged in unlock operation
		OldAvailable: decimal.NewFromFloat(oldAccount.Available),
		NewAvailable: decimal.NewFromFloat(expectedNewAvailable),
		OldLocked:    decimal.NewFromFloat(oldAccount.Locked),
		NewLocked:    decimal.NewFromFloat(expectedNewLocked),
		Amount:       fundsMsg.Amount,
		Reference:    fundsMsg.OrderID,
		Description:  fmt.Sprintf("Funds unlocked for order: %s", fundsMsg.Reason),
	}

	return s.messageBus.PublishBalanceEvent(ctx, balanceEvent)
}

// handleTradeExecuted processes trade execution and updates balances
func (s *BookkeeperMessageService) handleTradeExecuted(ctx context.Context, msg *ReceivedMessage) error {
	var tradeMsg TradeEventMessage
	if err := json.Unmarshal(msg.Value, &tradeMsg); err != nil {
		return fmt.Errorf("failed to unmarshal trade event message: %w", err)
	}

	s.logger.Info("Processing trade execution for balance updates",
		zap.String("trade_id", tradeMsg.TradeID),
		zap.String("symbol", tradeMsg.Symbol),
		zap.String("buy_user", tradeMsg.BuyUserID),
		zap.String("sell_user", tradeMsg.SellUserID))

	// Parse symbol to get base and quote currencies
	baseCurrency, quoteCurrency, err := parseSymbol(tradeMsg.Symbol)
	if err != nil {
		return fmt.Errorf("failed to parse symbol %s: %w", tradeMsg.Symbol, err)
	}

	// Calculate trade amounts
	quoteAmount := tradeMsg.Price.Mul(tradeMsg.Quantity)

	// Update buyer's balances (gets base currency, pays quote currency + fee)
	if err := s.updateBalanceForTrade(ctx, tradeMsg.BuyUserID, baseCurrency, tradeMsg.Quantity, "trade_buy", tradeMsg.TradeID); err != nil {
		return fmt.Errorf("failed to update buyer base balance: %w", err)
	}

	buyerQuoteAmount := quoteAmount.Add(tradeMsg.BuyFee)
	if err := s.updateBalanceForTrade(ctx, tradeMsg.BuyUserID, quoteCurrency, buyerQuoteAmount.Neg(), "trade_buy", tradeMsg.TradeID); err != nil {
		return fmt.Errorf("failed to update buyer quote balance: %w", err)
	}

	// Update seller's balances (gets quote currency - fee, pays base currency)
	sellerQuoteAmount := quoteAmount.Sub(tradeMsg.SellFee)
	if err := s.updateBalanceForTrade(ctx, tradeMsg.SellUserID, quoteCurrency, sellerQuoteAmount, "trade_sell", tradeMsg.TradeID); err != nil {
		return fmt.Errorf("failed to update seller quote balance: %w", err)
	}

	if err := s.updateBalanceForTrade(ctx, tradeMsg.SellUserID, baseCurrency, tradeMsg.Quantity.Neg(), "trade_sell", tradeMsg.TradeID); err != nil {
		return fmt.Errorf("failed to update seller base balance: %w", err)
	}

	return nil
}

// updateBalanceForTrade updates a user's balance for a trade
func (s *BookkeeperMessageService) updateBalanceForTrade(ctx context.Context, userID, currency string, amount decimal.Decimal, transactionType, reference string) error {
	// Get current balance
	oldAccount, err := s.bookkeeper.GetAccount(ctx, userID, currency)
	if err != nil {
		return fmt.Errorf("failed to get account: %w", err)
	}

	// Create transaction
	description := fmt.Sprintf("Trade execution - %s", reference)
	_, err = s.bookkeeper.CreateTransaction(ctx, userID, transactionType, amount.InexactFloat64(), currency, reference, description)
	if err != nil {
		return fmt.Errorf("failed to create transaction: %w", err)
	}

	// Get updated balance
	newAccount, err := s.bookkeeper.GetAccount(ctx, userID, currency)
	if err != nil {
		return fmt.Errorf("failed to get updated account: %w", err)
	}

	// Publish balance update event
	balanceEvent := &BalanceEventMessage{
		BaseMessage:  NewBaseMessage(MsgBalanceUpdated, "bookkeeper", ""),
		UserID:       userID,
		Currency:     currency,
		OldBalance:   decimal.NewFromFloat(oldAccount.Balance),
		NewBalance:   decimal.NewFromFloat(newAccount.Balance),
		OldAvailable: decimal.NewFromFloat(oldAccount.Available),
		NewAvailable: decimal.NewFromFloat(newAccount.Available),
		OldLocked:    decimal.NewFromFloat(oldAccount.Locked),
		NewLocked:    decimal.NewFromFloat(newAccount.Locked),
		Amount:       amount,
		Reference:    reference,
		Description:  description,
	}

	return s.messageBus.PublishBalanceEvent(ctx, balanceEvent)
}

// handleBalanceUpdate handles balance update notifications (for logging/audit)
func (s *BookkeeperMessageService) handleBalanceUpdate(ctx context.Context, msg *ReceivedMessage) error {
	var balanceMsg BalanceEventMessage
	if err := json.Unmarshal(msg.Value, &balanceMsg); err != nil {
		return fmt.Errorf("failed to unmarshal balance event message: %w", err)
	}

	s.logger.Info("Balance updated",
		zap.String("user_id", balanceMsg.UserID),
		zap.String("currency", balanceMsg.Currency),
		zap.String("old_balance", balanceMsg.OldBalance.String()),
		zap.String("new_balance", balanceMsg.NewBalance.String()),
		zap.String("amount", balanceMsg.Amount.String()),
		zap.String("reference", balanceMsg.Reference))

	// Here you could add additional logic like:
	// - Sending notifications to users
	// - Updating analytics/reporting
	// - Triggering risk checks
	// - Updating external systems

	return nil
}

// RequestFundsLock publishes a fund locking request
func (s *BookkeeperMessageService) RequestFundsLock(ctx context.Context, userID, currency string, amount decimal.Decimal, orderID, reason string) error {
	fundsMsg := &FundsOperationMessage{
		BaseMessage: NewBaseMessage(MsgFundsLocked, "trading", ""),
		UserID:      userID,
		Currency:    currency,
		Amount:      amount,
		OrderID:     orderID,
		Reason:      reason,
	}

	return s.messageBus.PublishFundsOperationEvent(ctx, fundsMsg)
}

// RequestFundsUnlock publishes a fund unlocking request
func (s *BookkeeperMessageService) RequestFundsUnlock(ctx context.Context, userID, currency string, amount decimal.Decimal, orderID, reason string) error {
	fundsMsg := &FundsOperationMessage{
		BaseMessage: NewBaseMessage(MsgFundsUnlocked, "trading", ""),
		UserID:      userID,
		Currency:    currency,
		Amount:      amount,
		OrderID:     orderID,
		Reason:      reason,
	}

	return s.messageBus.PublishFundsOperationEvent(ctx, fundsMsg)
}

// BatchFundsOperation represents a batch funds operation request
type BatchFundsOperation struct {
	UserID   string          `json:"user_id"`
	Currency string          `json:"currency"`
	Amount   decimal.Decimal `json:"amount"`
	OrderID  string          `json:"order_id"`
	Reason   string          `json:"reason"`
	OpType   string          `json:"op_type"` // "lock" or "unlock"
}

// BatchFundsOperationMessage represents a batch of funds operations
type BatchFundsOperationMessage struct {
	BaseMessage
	Operations []BatchFundsOperation `json:"operations"`
	BatchID    string                `json:"batch_id"`
}

// handleBatchFundsOperations processes multiple fund operations efficiently
func (s *BookkeeperMessageService) handleBatchFundsOperations(ctx context.Context, msg *ReceivedMessage) error {
	var batchMsg BatchFundsOperationMessage
	if err := json.Unmarshal(msg.Value, &batchMsg); err != nil {
		return fmt.Errorf("failed to unmarshal batch funds operation message: %w", err)
	}

	s.logger.Info("Processing batch funds operations",
		zap.String("batch_id", batchMsg.BatchID),
		zap.Int("operation_count", len(batchMsg.Operations)))

	// Group operations by user and currency for efficient account retrieval
	userCurrencyMap := make(map[string]map[string][]BatchFundsOperation)
	for _, op := range batchMsg.Operations {
		if userCurrencyMap[op.UserID] == nil {
			userCurrencyMap[op.UserID] = make(map[string][]BatchFundsOperation)
		}
		userCurrencyMap[op.UserID][op.Currency] = append(userCurrencyMap[op.UserID][op.Currency], op)
	}

	// Collect all unique user IDs and currencies for batch account retrieval
	var userIDs []string
	var currencies []string
	userSet := make(map[string]bool)
	currencySet := make(map[string]bool)

	for userID, currencyMap := range userCurrencyMap {
		if !userSet[userID] {
			userIDs = append(userIDs, userID)
			userSet[userID] = true
		}
		for currency := range currencyMap {
			if !currencySet[currency] {
				currencies = append(currencies, currency)
				currencySet[currency] = true
			}
		}
	}

	// Batch get all required accounts (single query instead of N queries)
	accounts, err := s.bookkeeper.BatchGetAccounts(ctx, userIDs, currencies)
	if err != nil {
		return fmt.Errorf("failed to batch get accounts: %w", err)
	}

	// Prepare batch operations for bookkeeper service
	var lockOps []bookkeeper.FundsOperation
	var unlockOps []bookkeeper.FundsOperation

	for _, op := range batchMsg.Operations {
		fundsOp := bookkeeper.FundsOperation{
			UserID:   op.UserID,
			Currency: op.Currency,
			Amount:   op.Amount.InexactFloat64(),
			OrderID:  op.OrderID,
			Reason:   op.Reason,
		}

		if op.OpType == "lock" {
			lockOps = append(lockOps, fundsOp)
		} else if op.OpType == "unlock" {
			unlockOps = append(unlockOps, fundsOp)
		}
	}

	// Execute batch operations
	var lockResult, unlockResult *bookkeeper.BatchOperationResult

	if len(lockOps) > 0 {
		lockResult, err = s.bookkeeper.BatchLockFunds(ctx, lockOps)
		if err != nil {
			s.logger.Error("Failed to execute batch lock operations", zap.Error(err))
		}
	}

	if len(unlockOps) > 0 {
		unlockResult, err = s.bookkeeper.BatchUnlockFunds(ctx, unlockOps)
		if err != nil {
			s.logger.Error("Failed to execute batch unlock operations", zap.Error(err))
		}
	}

	// Publish balance events for successful operations
	for _, op := range batchMsg.Operations {
		// Find the account from batch results
		account, exists := accounts[op.UserID][op.Currency]
		if !exists {
			s.logger.Warn("Account not found in batch results",
				zap.String("user_id", op.UserID),
				zap.String("currency", op.Currency))
			continue
		}

		// Calculate expected new values
		var expectedNewAvailable, expectedNewLocked float64
		if op.OpType == "lock" {
			expectedNewAvailable = account.Available - op.Amount.InexactFloat64()
			expectedNewLocked = account.Locked + op.Amount.InexactFloat64()
		} else {
			expectedNewAvailable = account.Available + op.Amount.InexactFloat64()
			expectedNewLocked = account.Locked - op.Amount.InexactFloat64()
		}

		// Create and publish balance event
		balanceEvent := &BalanceEventMessage{
			BaseMessage:  NewBaseMessage(MsgBalanceUpdated, "bookkeeper", batchMsg.MessageID),
			UserID:       op.UserID,
			Currency:     op.Currency,
			OldBalance:   decimal.NewFromFloat(account.Balance),
			NewBalance:   decimal.NewFromFloat(account.Balance), // Balance unchanged in lock/unlock operations
			OldAvailable: decimal.NewFromFloat(account.Available),
			NewAvailable: decimal.NewFromFloat(expectedNewAvailable),
			OldLocked:    decimal.NewFromFloat(account.Locked),
			NewLocked:    decimal.NewFromFloat(expectedNewLocked),
			Amount:       op.Amount,
			Reference:    op.OrderID,
			Description:  fmt.Sprintf("Funds %sed for order: %s", op.OpType, op.Reason),
		}

		if err := s.messageBus.PublishBalanceEvent(ctx, balanceEvent); err != nil {
			s.logger.Error("Failed to publish balance event",
				zap.Error(err),
				zap.String("user_id", op.UserID),
				zap.String("currency", op.Currency))
		}
	}

	// Log batch operation results
	s.logger.Info("Batch funds operations completed",
		zap.String("batch_id", batchMsg.BatchID),
		zap.Int("total_operations", len(batchMsg.Operations)),
		zap.Int("lock_success", safeGetSuccessCount(lockResult)),
		zap.Int("unlock_success", safeGetSuccessCount(unlockResult)))

	return nil
}

// safeGetSuccessCount safely extracts success count from batch result
func safeGetSuccessCount(result *bookkeeper.BatchOperationResult) int {
	if result == nil {
		return 0
	}
	return result.SuccessCount
}

// RequestBatchFundsOperations publishes a batch of fund operations
func (s *BookkeeperMessageService) RequestBatchFundsOperations(ctx context.Context, operations []BatchFundsOperation, batchID string) error {
	batchData := map[string]interface{}{
		"operations": operations,
		"batch_id":   batchID,
	}

	batchJson, err := json.Marshal(batchData)
	if err != nil {
		return fmt.Errorf("failed to marshal batch operations: %w", err)
	}

	// Create a dedicated message for batch operations
	batchMsg := &FundsOperationMessage{
		BaseMessage: NewBaseMessage("batch_funds_operations", "trading", ""),
		UserID:      batchID,      // Use UserID field for batch ID
		Currency:    "BATCH",      // Indicates this is a batch operation
		Amount:      decimal.Zero, // Not applicable for batch
		OrderID:     "",
		Reason:      string(batchJson), // Store batch data in reason field
	}

	return s.messageBus.PublishFundsOperationEvent(ctx, batchMsg)
}

// Helper function to parse trading symbol into base and quote currencies
func parseSymbol(symbol string) (base, quote string, err error) {
	// Simple implementation - extend based on your symbol format
	if len(symbol) == 6 {
		return symbol[:3], symbol[3:], nil
	}
	if len(symbol) >= 7 && symbol[3] == '/' {
		return symbol[:3], symbol[4:], nil
	}
	if len(symbol) >= 7 && symbol[4] == '/' {
		return symbol[:4], symbol[5:], nil
	}

	// Default fallback
	return "BTC", "USDT", fmt.Errorf("unsupported symbol format: %s", symbol)
}
