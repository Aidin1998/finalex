package messaging

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Aidin1998/pincex_unified/internal/bookkeeper"
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

// handleFundsLocked processes fund locking requests
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

	// Get current balance for event
	oldAccount, err := s.bookkeeper.GetAccount(ctx, fundsMsg.UserID, fundsMsg.Currency)
	if err != nil {
		return fmt.Errorf("failed to get account before lock: %w", err)
	}

	// Lock the funds
	err = s.bookkeeper.LockFunds(ctx, fundsMsg.UserID, fundsMsg.Currency, fundsMsg.Amount.InexactFloat64())
	if err != nil {
		s.logger.Error("Failed to lock funds",
			zap.Error(err),
			zap.String("user_id", fundsMsg.UserID),
			zap.String("currency", fundsMsg.Currency))
		return err
	}

	// Get updated balance for event
	newAccount, err := s.bookkeeper.GetAccount(ctx, fundsMsg.UserID, fundsMsg.Currency)
	if err != nil {
		s.logger.Error("Failed to get account after lock", zap.Error(err))
		return err
	}

	// Publish balance update event
	balanceEvent := &BalanceEventMessage{
		BaseMessage:  NewBaseMessage(MsgBalanceUpdated, "bookkeeper", fundsMsg.MessageID),
		UserID:       fundsMsg.UserID,
		Currency:     fundsMsg.Currency,
		OldBalance:   decimal.NewFromFloat(oldAccount.Balance),
		NewBalance:   decimal.NewFromFloat(newAccount.Balance),
		OldAvailable: decimal.NewFromFloat(oldAccount.Available),
		NewAvailable: decimal.NewFromFloat(newAccount.Available),
		OldLocked:    decimal.NewFromFloat(oldAccount.Locked),
		NewLocked:    decimal.NewFromFloat(newAccount.Locked),
		Amount:       fundsMsg.Amount,
		Reference:    fundsMsg.OrderID,
		Description:  fmt.Sprintf("Funds locked for order: %s", fundsMsg.Reason),
	}

	return s.messageBus.PublishBalanceEvent(ctx, balanceEvent)
}

// handleFundsUnlocked processes fund unlocking requests
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

	// Get current balance for event
	oldAccount, err := s.bookkeeper.GetAccount(ctx, fundsMsg.UserID, fundsMsg.Currency)
	if err != nil {
		return fmt.Errorf("failed to get account before unlock: %w", err)
	}

	// Unlock the funds
	err = s.bookkeeper.UnlockFunds(ctx, fundsMsg.UserID, fundsMsg.Currency, fundsMsg.Amount.InexactFloat64())
	if err != nil {
		s.logger.Error("Failed to unlock funds",
			zap.Error(err),
			zap.String("user_id", fundsMsg.UserID),
			zap.String("currency", fundsMsg.Currency))
		return err
	}

	// Get updated balance for event
	newAccount, err := s.bookkeeper.GetAccount(ctx, fundsMsg.UserID, fundsMsg.Currency)
	if err != nil {
		s.logger.Error("Failed to get account after unlock", zap.Error(err))
		return err
	}

	// Publish balance update event
	balanceEvent := &BalanceEventMessage{
		BaseMessage:  NewBaseMessage(MsgBalanceUpdated, "bookkeeper", fundsMsg.MessageID),
		UserID:       fundsMsg.UserID,
		Currency:     fundsMsg.Currency,
		OldBalance:   decimal.NewFromFloat(oldAccount.Balance),
		NewBalance:   decimal.NewFromFloat(newAccount.Balance),
		OldAvailable: decimal.NewFromFloat(oldAccount.Available),
		NewAvailable: decimal.NewFromFloat(newAccount.Available),
		OldLocked:    decimal.NewFromFloat(oldAccount.Locked),
		NewLocked:    decimal.NewFromFloat(newAccount.Locked),
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
