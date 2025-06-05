package lifecycle

import (
	"context"
	"fmt"
	"strings"

	"github.com/Aidin1998/finalex/internal/trading/model"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// BasicOrderValidator performs basic order validation
type BasicOrderValidator struct {
	logger *zap.Logger
}

// NewBasicOrderValidator creates a new basic order validator
func NewBasicOrderValidator(logger *zap.Logger) *BasicOrderValidator {
	return &BasicOrderValidator{
		logger: logger,
	}
}

// ValidateOrder validates basic order requirements
func (v *BasicOrderValidator) ValidateOrder(ctx context.Context, order *model.Order) error {
	// Validate order ID
	if order.ID == uuid.Nil {
		return fmt.Errorf("order ID is required")
	}

	// Validate user ID
	if order.UserID == uuid.Nil {
		return fmt.Errorf("user ID is required")
	}

	// Validate trading pair
	if order.Pair == "" {
		return fmt.Errorf("trading pair is required")
	}

	// Validate side
	side := strings.ToUpper(order.Side)
	if side != "BUY" && side != "SELL" {
		return fmt.Errorf("invalid order side: %s (must be BUY or SELL)", order.Side)
	}

	// Validate order type
	orderType := strings.ToUpper(order.Type)
	validTypes := []string{"MARKET", "LIMIT", "STOP_LOSS", "STOP_LOSS_LIMIT", "TAKE_PROFIT", "TAKE_PROFIT_LIMIT"}
	isValidType := false
	for _, validType := range validTypes {
		if orderType == validType {
			isValidType = true
			break
		}
	}
	if !isValidType {
		return fmt.Errorf("invalid order type: %s", order.Type)
	}

	// Validate quantity
	if order.Quantity.LessThanOrEqual(decimal.Zero) {
		return fmt.Errorf("order quantity must be positive")
	}

	// Validate price for limit orders
	if orderType == "LIMIT" || orderType == "STOP_LOSS_LIMIT" || orderType == "TAKE_PROFIT_LIMIT" {
		if order.Price.LessThanOrEqual(decimal.Zero) {
			return fmt.Errorf("order price must be positive for limit orders")
		}
	}

	// Validate stop price for stop orders
	if orderType == "STOP_LOSS" || orderType == "STOP_LOSS_LIMIT" || orderType == "TAKE_PROFIT" || orderType == "TAKE_PROFIT_LIMIT" {
		if order.StopPrice.LessThanOrEqual(decimal.Zero) {
			return fmt.Errorf("stop price must be positive for stop orders")
		}
	}

	// Validate time in force
	timeInForce := strings.ToUpper(order.TimeInForce)
	validTIF := []string{"GTC", "IOC", "FOK", "GTD"}
	isValidTIF := false
	for _, validTIF := range validTIF {
		if timeInForce == validTIF {
			isValidTIF = true
			break
		}
	}
	if !isValidTIF {
		return fmt.Errorf("invalid time in force: %s", order.TimeInForce)
	}

	return nil
}

// Name returns the validator name
func (v *BasicOrderValidator) Name() string {
	return "BasicOrderValidator"
}

// MarketValidator validates market-specific constraints
type MarketValidator struct {
	db     *gorm.DB
	logger *zap.Logger
}

// TradingPair represents a trading pair configuration
type TradingPair struct {
	ID             string          `gorm:"primaryKey;type:uuid"`
	Symbol         string          `gorm:"unique;not null"`
	BaseAsset      string          `gorm:"not null"`
	QuoteAsset     string          `gorm:"not null"`
	Status         string          `gorm:"not null;default:'ACTIVE'"`
	MinQuantity    decimal.Decimal `gorm:"type:decimal(20,8);not null"`
	MaxQuantity    decimal.Decimal `gorm:"type:decimal(20,8);not null"`
	QuantityStep   decimal.Decimal `gorm:"type:decimal(20,8);not null"`
	MinPrice       decimal.Decimal `gorm:"type:decimal(20,8);not null"`
	MaxPrice       decimal.Decimal `gorm:"type:decimal(20,8);not null"`
	PriceStep      decimal.Decimal `gorm:"type:decimal(20,8);not null"`
	MinNotional    decimal.Decimal `gorm:"type:decimal(20,8);not null"`
	MaxNotional    decimal.Decimal `gorm:"type:decimal(20,8);not null"`
	BasePrecision  int             `gorm:"not null"`
	QuotePrecision int             `gorm:"not null"`
}

// NewMarketValidator creates a new market validator
func NewMarketValidator(db *gorm.DB, logger *zap.Logger) *MarketValidator {
	return &MarketValidator{
		db:     db,
		logger: logger,
	}
}

// ValidateOrder validates market-specific order requirements
func (v *MarketValidator) ValidateOrder(ctx context.Context, order *model.Order) error {
	// Get trading pair configuration
	var tradingPair TradingPair
	if err := v.db.Where("symbol = ?", order.Pair).First(&tradingPair).Error; err != nil {
		return fmt.Errorf("trading pair not found: %s", order.Pair)
	}

	// Check if trading is enabled for the pair
	if tradingPair.Status != "ACTIVE" {
		return fmt.Errorf("trading is not active for pair %s (status: %s)", order.Pair, tradingPair.Status)
	}

	// Validate quantity constraints
	if order.Quantity.LessThan(tradingPair.MinQuantity) {
		return fmt.Errorf("order quantity %s is below minimum %s", order.Quantity, tradingPair.MinQuantity)
	}

	if order.Quantity.GreaterThan(tradingPair.MaxQuantity) {
		return fmt.Errorf("order quantity %s exceeds maximum %s", order.Quantity, tradingPair.MaxQuantity)
	}

	// Validate quantity step size
	remainder := order.Quantity.Mod(tradingPair.QuantityStep)
	if !remainder.IsZero() {
		return fmt.Errorf("order quantity %s does not conform to step size %s", order.Quantity, tradingPair.QuantityStep)
	}

	// Validate price constraints for limit orders
	orderType := strings.ToUpper(order.Type)
	if orderType == "LIMIT" || orderType == "STOP_LOSS_LIMIT" || orderType == "TAKE_PROFIT_LIMIT" {
		if order.Price.LessThan(tradingPair.MinPrice) {
			return fmt.Errorf("order price %s is below minimum %s", order.Price, tradingPair.MinPrice)
		}

		if order.Price.GreaterThan(tradingPair.MaxPrice) {
			return fmt.Errorf("order price %s exceeds maximum %s", order.Price, tradingPair.MaxPrice)
		}

		// Validate price step size
		priceRemainder := order.Price.Mod(tradingPair.PriceStep)
		if !priceRemainder.IsZero() {
			return fmt.Errorf("order price %s does not conform to step size %s", order.Price, tradingPair.PriceStep)
		}

		// Validate notional value
		notional := order.Price.Mul(order.Quantity)
		if notional.LessThan(tradingPair.MinNotional) {
			return fmt.Errorf("order notional value %s is below minimum %s", notional, tradingPair.MinNotional)
		}

		if notional.GreaterThan(tradingPair.MaxNotional) {
			return fmt.Errorf("order notional value %s exceeds maximum %s", notional, tradingPair.MaxNotional)
		}
	}

	return nil
}

// Name returns the validator name
func (v *MarketValidator) Name() string {
	return "MarketValidator"
}

// BalanceValidator validates user balance requirements
type BalanceValidator struct {
	db     *gorm.DB
	logger *zap.Logger
}

// UserBalance represents a user's asset balance
type UserBalance struct {
	ID        string          `gorm:"primaryKey;type:uuid"`
	UserID    string          `gorm:"type:uuid;not null;index"`
	Asset     string          `gorm:"not null"`
	Available decimal.Decimal `gorm:"type:decimal(20,8);not null;default:0"`
	Locked    decimal.Decimal `gorm:"type:decimal(20,8);not null;default:0"`
	Total     decimal.Decimal `gorm:"type:decimal(20,8);not null;default:0"`
}

// NewBalanceValidator creates a new balance validator
func NewBalanceValidator(db *gorm.DB, logger *zap.Logger) *BalanceValidator {
	return &BalanceValidator{
		db:     db,
		logger: logger,
	}
}

// ValidateOrder validates user balance requirements
func (v *BalanceValidator) ValidateOrder(ctx context.Context, order *model.Order) error {
	// Extract base and quote assets from pair
	baseAsset, quoteAsset, err := v.extractAssets(order.Pair)
	if err != nil {
		return fmt.Errorf("failed to extract assets from pair %s: %w", order.Pair, err)
	}

	side := strings.ToUpper(order.Side)
	orderType := strings.ToUpper(order.Type)

	var requiredAsset string
	var requiredAmount decimal.Decimal

	if side == "BUY" {
		// For buy orders, we need quote asset
		requiredAsset = quoteAsset

		if orderType == "MARKET" {
			// For market buy orders, we can't determine exact amount needed
			// This should be handled by the matching engine
			return nil
		} else {
			// For limit buy orders, we need price * quantity of quote asset
			requiredAmount = order.Price.Mul(order.Quantity)
		}
	} else {
		// For sell orders, we need base asset
		requiredAsset = baseAsset
		requiredAmount = order.Quantity
	}

	// Get user balance
	var balance UserBalance
	if err := v.db.Where("user_id = ? AND asset = ?", order.UserID, requiredAsset).First(&balance).Error; err != nil {
		return fmt.Errorf("insufficient balance: no %s balance found", requiredAsset)
	}

	// Check if user has sufficient available balance
	if balance.Available.LessThan(requiredAmount) {
		return fmt.Errorf("insufficient %s balance: required %s, available %s",
			requiredAsset, requiredAmount, balance.Available)
	}

	return nil
}

// extractAssets extracts base and quote assets from a trading pair symbol
func (v *BalanceValidator) extractAssets(pair string) (string, string, error) {
	// Get trading pair from database
	var tradingPair TradingPair
	if err := v.db.Where("symbol = ?", pair).First(&tradingPair).Error; err != nil {
		return "", "", fmt.Errorf("trading pair not found: %s", pair)
	}

	return tradingPair.BaseAsset, tradingPair.QuoteAsset, nil
}

// Name returns the validator name
func (v *BalanceValidator) Name() string {
	return "BalanceValidator"
}

// RiskValidator validates risk management constraints
type RiskValidator struct {
	db     *gorm.DB
	logger *zap.Logger
}

// UserRiskLimits represents user-specific risk limits
type UserRiskLimits struct {
	ID                 string          `gorm:"primaryKey;type:uuid"`
	UserID             string          `gorm:"type:uuid;not null;unique"`
	MaxOrderValue      decimal.Decimal `gorm:"type:decimal(20,8);not null"`
	MaxDailyOrderCount int             `gorm:"not null"`
	MaxDailyOrderValue decimal.Decimal `gorm:"type:decimal(20,8);not null"`
	MaxOpenOrders      int             `gorm:"not null"`
	MaxPositionSize    decimal.Decimal `gorm:"type:decimal(20,8);not null"`
	AllowedOrderTypes  string          `gorm:"type:text;not null"`
	TradingEnabled     bool            `gorm:"not null;default:true"`
}

// NewRiskValidator creates a new risk validator
func NewRiskValidator(db *gorm.DB, logger *zap.Logger) *RiskValidator {
	return &RiskValidator{
		db:     db,
		logger: logger,
	}
}

// ValidateOrder validates risk management constraints
func (v *RiskValidator) ValidateOrder(ctx context.Context, order *model.Order) error {
	// Get user risk limits
	var riskLimits UserRiskLimits
	if err := v.db.Where("user_id = ?", order.UserID).First(&riskLimits).Error; err != nil {
		// If no specific limits found, use default validation
		return v.validateDefaultRiskLimits(ctx, order)
	}

	// Check if trading is enabled for user
	if !riskLimits.TradingEnabled {
		return fmt.Errorf("trading is disabled for user %s", order.UserID)
	}

	// Validate order type is allowed
	orderType := strings.ToUpper(order.Type)
	allowedTypes := strings.Split(riskLimits.AllowedOrderTypes, ",")
	isAllowed := false
	for _, allowedType := range allowedTypes {
		if strings.TrimSpace(strings.ToUpper(allowedType)) == orderType {
			isAllowed = true
			break
		}
	}
	if !isAllowed {
		return fmt.Errorf("order type %s is not allowed for user %s", orderType, order.UserID)
	}

	// Calculate order value
	var orderValue decimal.Decimal
	if orderType == "MARKET" {
		// For market orders, we can't determine exact value
		// This should be checked by the matching engine
		orderValue = decimal.Zero
	} else {
		orderValue = order.Price.Mul(order.Quantity)
	}

	// Validate max order value
	if !orderValue.IsZero() && orderValue.GreaterThan(riskLimits.MaxOrderValue) {
		return fmt.Errorf("order value %s exceeds maximum allowed %s", orderValue, riskLimits.MaxOrderValue)
	}

	// Check daily order count
	dailyOrderCount, err := v.getDailyOrderCount(order.UserID)
	if err != nil {
		v.logger.Warn("Failed to get daily order count", zap.Error(err))
	} else if dailyOrderCount >= riskLimits.MaxDailyOrderCount {
		return fmt.Errorf("daily order count limit exceeded: %d/%d", dailyOrderCount, riskLimits.MaxDailyOrderCount)
	}

	// Check daily order value
	dailyOrderValue, err := v.getDailyOrderValue(order.UserID)
	if err != nil {
		v.logger.Warn("Failed to get daily order value", zap.Error(err))
	} else if dailyOrderValue.Add(orderValue).GreaterThan(riskLimits.MaxDailyOrderValue) {
		return fmt.Errorf("daily order value limit would be exceeded: current %s + new %s > limit %s",
			dailyOrderValue, orderValue, riskLimits.MaxDailyOrderValue)
	}

	// Check open orders count
	openOrdersCount, err := v.getOpenOrdersCount(order.UserID)
	if err != nil {
		v.logger.Warn("Failed to get open orders count", zap.Error(err))
	} else if openOrdersCount >= riskLimits.MaxOpenOrders {
		return fmt.Errorf("maximum open orders limit exceeded: %d/%d", openOrdersCount, riskLimits.MaxOpenOrders)
	}

	return nil
}

// validateDefaultRiskLimits applies default risk validation when no user-specific limits exist
func (v *RiskValidator) validateDefaultRiskLimits(ctx context.Context, order *model.Order) error {
	// Default maximum order value (example: $100,000 USD equivalent)
	maxOrderValue := decimal.NewFromFloat(100000)

	orderType := strings.ToUpper(order.Type)
	if orderType != "MARKET" {
		orderValue := order.Price.Mul(order.Quantity)
		if orderValue.GreaterThan(maxOrderValue) {
			return fmt.Errorf("order value %s exceeds default maximum %s", orderValue, maxOrderValue)
		}
	}

	// Default maximum open orders
	maxOpenOrders := 100
	openOrdersCount, err := v.getOpenOrdersCount(order.UserID)
	if err != nil {
		v.logger.Warn("Failed to get open orders count", zap.Error(err))
	} else if openOrdersCount >= maxOpenOrders {
		return fmt.Errorf("maximum open orders limit exceeded: %d/%d", openOrdersCount, maxOpenOrders)
	}

	return nil
}

// getDailyOrderCount gets the number of orders placed by user today
func (v *RiskValidator) getDailyOrderCount(userID uuid.UUID) (int, error) {
	var count int64
	err := v.db.Model(&model.Order{}).
		Where("user_id = ? AND DATE(created_at) = CURRENT_DATE", userID.String()).
		Count(&count).Error

	return int(count), err
}

// getDailyOrderValue gets the total value of orders placed by user today
func (v *RiskValidator) getDailyOrderValue(userID uuid.UUID) (decimal.Decimal, error) {
	type Result struct {
		Total decimal.Decimal
	}

	var result Result
	err := v.db.Model(&model.Order{}).
		Select("COALESCE(SUM(price * quantity), 0) as total").
		Where("user_id = ? AND DATE(created_at) = CURRENT_DATE AND type != 'MARKET'", userID.String()).
		Scan(&result).Error

	return result.Total, err
}

// getOpenOrdersCount gets the number of open orders for a user
func (v *RiskValidator) getOpenOrdersCount(userID uuid.UUID) (int, error) {
	var count int64
	err := v.db.Model(&model.Order{}).
		Where("user_id = ? AND status IN ('PENDING', 'ACCEPTED', 'PARTIALLY_FILLED')", userID.String()).
		Count(&count).Error

	return int(count), err
}

// Name returns the validator name
func (v *RiskValidator) Name() string {
	return "RiskValidator"
}
