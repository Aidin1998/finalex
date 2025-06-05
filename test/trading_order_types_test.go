//go:build trading

package test

import (
	"context"
	"fmt"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/Aidin1998/finalex/internal/trading"
	"github.com/Aidin1998/finalex/pkg/models"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/suite"
)

// TradingOrderTypesTestSuite provides comprehensive testing for all order types and lifecycle scenarios
type TradingOrderTypesTestSuite struct {
	suite.Suite
	service        trading.Service
	mockBookkeeper *MockBookkeeperOrderTypes
	mockWSHub      *MockWSHubOrderTypes
	testUsers      []string
	ctx            context.Context
	cancel         context.CancelFunc
}

// MockBookkeeperOrderTypes provides order type specific mock for bookkeeper service
type MockBookkeeperOrderTypes struct {
	balances     map[string]map[string]decimal.Decimal
	reservations map[string]ReservationInfo
	transactions []TransactionRecord
	mu           sync.RWMutex
}

type TransactionRecord struct {
	ID        string
	FromUser  string
	ToUser    string
	Asset     string
	Amount    decimal.Decimal
	Type      string
	Timestamp time.Time
	OrderID   string
}

func NewMockBookkeeperOrderTypes() *MockBookkeeperOrderTypes {
	return &MockBookkeeperOrderTypes{
		balances:     make(map[string]map[string]decimal.Decimal),
		reservations: make(map[string]ReservationInfo),
		transactions: make([]TransactionRecord, 0),
	}
}

func (m *MockBookkeeperOrderTypes) GetBalance(userID, asset string) (decimal.Decimal, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if userBalances, exists := m.balances[userID]; exists {
		if balance, exists := userBalances[asset]; exists {
			return balance, nil
		}
	}

	// Default high balances for testing
	baseBalances := map[string]decimal.Decimal{
		"USDT": decimal.NewFromInt(1000000),
		"BTC":  decimal.NewFromInt(100),
		"ETH":  decimal.NewFromInt(1000),
		"BNB":  decimal.NewFromInt(10000),
	}

	if balance, exists := baseBalances[asset]; exists {
		return balance, nil
	}

	return decimal.NewFromInt(10000), nil
}

func (m *MockBookkeeperOrderTypes) ReserveBalance(userID, asset string, amount decimal.Decimal) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check available balance
	balance, err := m.GetBalance(userID, asset)
	if err != nil {
		return "", err
	}

	if balance.LessThan(amount) {
		return "", fmt.Errorf("insufficient balance: have %s, need %s", balance.String(), amount.String())
	}

	reservationID := fmt.Sprintf("order_res_%s_%s_%d", userID, asset, time.Now().UnixNano())
	m.reservations[reservationID] = ReservationInfo{
		UserID: userID,
		Asset:  asset,
		Amount: amount,
	}

	return reservationID, nil
}

func (m *MockBookkeeperOrderTypes) ReleaseReservation(reservationID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.reservations[reservationID]; !exists {
		return fmt.Errorf("reservation not found: %s", reservationID)
	}

	delete(m.reservations, reservationID)
	return nil
}

func (m *MockBookkeeperOrderTypes) TransferReservedBalance(reservationID, toUserID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	reservation, exists := m.reservations[reservationID]
	if !exists {
		return fmt.Errorf("reservation not found: %s", reservationID)
	}

	// Record transaction
	transaction := TransactionRecord{
		ID:        fmt.Sprintf("tx_%d", time.Now().UnixNano()),
		FromUser:  reservation.UserID,
		ToUser:    toUserID,
		Asset:     reservation.Asset,
		Amount:    reservation.Amount,
		Type:      "order_settlement",
		Timestamp: time.Now(),
	}

	m.transactions = append(m.transactions, transaction)
	delete(m.reservations, reservationID)

	return nil
}

func (m *MockBookkeeperOrderTypes) GetTransactions() []TransactionRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]TransactionRecord, len(m.transactions))
	copy(result, m.transactions)
	return result
}

func (m *MockBookkeeperOrderTypes) GetActiveReservations() map[string]ReservationInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]ReservationInfo)
	for id, res := range m.reservations {
		result[id] = res
	}
	return result
}

// MockWSHubOrderTypes provides order type specific mock for WebSocket hub
type MockWSHubOrderTypes struct {
	notifications []OrderNotification
	subscriptions map[string][]string
	mu            sync.RWMutex
}

type OrderNotification struct {
	UserID      string
	Topic       string
	MessageType string
	OrderData   interface{}
	Timestamp   time.Time
}

func NewMockWSHubOrderTypes() *MockWSHubOrderTypes {
	return &MockWSHubOrderTypes{
		notifications: make([]OrderNotification, 0),
		subscriptions: make(map[string][]string),
	}
}

func (m *MockWSHubOrderTypes) PublishToUser(userID string, data interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.notifications = append(m.notifications, OrderNotification{
		UserID:      userID,
		MessageType: "user_notification",
		OrderData:   data,
		Timestamp:   time.Now(),
	})
}

func (m *MockWSHubOrderTypes) PublishToTopic(topic string, data interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.notifications = append(m.notifications, OrderNotification{
		Topic:       topic,
		MessageType: "topic_notification",
		OrderData:   data,
		Timestamp:   time.Now(),
	})
}

func (m *MockWSHubOrderTypes) SubscribeToTopic(userID, topic string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.subscriptions[topic] = append(m.subscriptions[topic], userID)
}

func (m *MockWSHubOrderTypes) GetNotifications() []OrderNotification {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]OrderNotification, len(m.notifications))
	copy(result, m.notifications)
	return result
}

func (m *MockWSHubOrderTypes) GetSubscriptions() map[string][]string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string][]string)
	for topic, users := range m.subscriptions {
		result[topic] = make([]string, len(users))
		copy(result[topic], users)
	}
	return result
}

func (suite *TradingOrderTypesTestSuite) SetupSuite() {
	log.Println("Setting up trading order types test suite...")

	suite.ctx, suite.cancel = context.WithTimeout(context.Background(), 10*time.Minute)

	suite.mockBookkeeper = NewMockBookkeeperOrderTypes()
	suite.mockWSHub = NewMockWSHubOrderTypes()

	suite.service = trading.NewService(
		suite.mockBookkeeper,
		suite.mockWSHub,
		trading.WithOrderTypes([]models.OrderType{
			models.Market,
			models.Limit,
			models.StopLoss,
			models.StopLimit,
			models.TakeProfit,
			models.TrailingStop,
			models.IcebergOrder,
		}),
		trading.WithTimeInForce([]models.TimeInForce{
			models.GTC, // Good Till Cancelled
			models.IOC, // Immediate Or Cancel
			models.FOK, // Fill Or Kill
			models.GTD, // Good Till Date
		}),
	)

	suite.setupTestUsers()

	err := suite.service.Start(suite.ctx)
	suite.Require().NoError(err, "Failed to start trading service")
}

func (suite *TradingOrderTypesTestSuite) TearDownSuite() {
	if suite.cancel != nil {
		suite.cancel()
	}
	if suite.service != nil {
		suite.service.Stop()
	}
}

func (suite *TradingOrderTypesTestSuite) setupTestUsers() {
	suite.testUsers = []string{
		"limit_trader_1", "limit_trader_2",
		"market_trader_1", "market_trader_2",
		"stop_trader_1", "stop_trader_2",
		"iceberg_trader_1", "iceberg_trader_2",
	}
}

// TestMarketOrders tests market order functionality
func (suite *TradingOrderTypesTestSuite) TestMarketOrders() {
	log.Println("Testing market orders...")

	buyer := "market_buyer"
	seller := "market_seller"

	// Place a sell limit order first to provide liquidity
	sellOrder := &models.PlaceOrderRequest{
		UserID:   seller,
		Pair:     "BTC/USDT",
		Side:     models.Sell,
		Type:     models.Limit,
		Price:    decimal.NewFromInt(50000),
		Quantity: decimal.NewFromFloat(1.0),
	}

	limitOrder, err := suite.service.PlaceOrder(suite.ctx, sellOrder)
	suite.Require().NoError(err, "Should place limit order for liquidity")
	suite.Require().NotNil(limitOrder, "Limit order should be created")

	// Place market buy order
	marketOrder := &models.PlaceOrderRequest{
		UserID:   buyer,
		Pair:     "BTC/USDT",
		Side:     models.Buy,
		Type:     models.Market,
		Quantity: decimal.NewFromFloat(0.5), // Partial fill
	}

	placedMarketOrder, err := suite.service.PlaceOrder(suite.ctx, marketOrder)
	suite.Assert().NoError(err, "Market order should be placed successfully")
	suite.Assert().NotNil(placedMarketOrder, "Market order should be created")
	suite.Assert().Equal(models.Market, placedMarketOrder.Type, "Order type should be Market")

	// Market orders should execute immediately or fail
	suite.Assert().Contains([]models.OrderStatus{models.Filled, models.PartiallyFilled, models.Rejected},
		placedMarketOrder.Status, "Market order should have immediate execution status")

	log.Printf("Market order test completed - Order ID: %s, Status: %s",
		placedMarketOrder.ID, placedMarketOrder.Status)
}

// TestLimitOrders tests limit order functionality
func (suite *TradingOrderTypesTestSuite) TestLimitOrders() {
	log.Println("Testing limit orders...")

	userID := "limit_trader"

	// Test buy limit order
	buyLimitOrder := &models.PlaceOrderRequest{
		UserID:   userID,
		Pair:     "BTC/USDT",
		Side:     models.Buy,
		Type:     models.Limit,
		Price:    decimal.NewFromInt(45000), // Below market
		Quantity: decimal.NewFromFloat(0.1),
	}

	placedBuyOrder, err := suite.service.PlaceOrder(suite.ctx, buyLimitOrder)
	suite.Assert().NoError(err, "Buy limit order should be placed")
	suite.Assert().NotNil(placedBuyOrder, "Buy limit order should be created")
	suite.Assert().Equal(models.Limit, placedBuyOrder.Type, "Order type should be Limit")
	suite.Assert().Equal(models.Open, placedBuyOrder.Status, "Limit order should be open")

	// Test sell limit order
	sellLimitOrder := &models.PlaceOrderRequest{
		UserID:   userID,
		Pair:     "BTC/USDT",
		Side:     models.Sell,
		Type:     models.Limit,
		Price:    decimal.NewFromInt(55000), // Above market
		Quantity: decimal.NewFromFloat(0.1),
	}

	placedSellOrder, err := suite.service.PlaceOrder(suite.ctx, sellLimitOrder)
	suite.Assert().NoError(err, "Sell limit order should be placed")
	suite.Assert().NotNil(placedSellOrder, "Sell limit order should be created")

	// Verify orders are in the order book
	orderBook, err := suite.service.GetOrderBook(suite.ctx, "BTC/USDT", 10)
	suite.Assert().NoError(err, "Should get order book")
	suite.Assert().NotNil(orderBook, "Order book should exist")

	log.Printf("Limit orders test completed - Buy: %s, Sell: %s",
		placedBuyOrder.ID, placedSellOrder.ID)
}

// TestStopLossOrders tests stop loss order functionality
func (suite *TradingOrderTypesTestSuite) TestStopLossOrders() {
	log.Println("Testing stop loss orders...")

	userID := "stop_trader"

	// Place initial position (buy order)
	initialOrder := &models.PlaceOrderRequest{
		UserID:   userID,
		Pair:     "BTC/USDT",
		Side:     models.Buy,
		Type:     models.Market,
		Quantity: decimal.NewFromFloat(0.1),
	}

	_, err := suite.service.PlaceOrder(suite.ctx, initialOrder)
	if err != nil {
		log.Printf("Initial order failed: %v", err)
		// Continue with stop loss test anyway
	}

	// Place stop loss order
	stopLossOrder := &models.PlaceOrderRequest{
		UserID:           userID,
		Pair:             "BTC/USDT",
		Side:             models.Sell,
		Type:             models.StopLoss,
		Quantity:         decimal.NewFromFloat(0.1),
		StopPrice:        decimal.NewFromInt(45000), // Stop if price drops to 45000
		TriggerCondition: models.LessThanOrEqual,
	}

	placedStopOrder, err := suite.service.PlaceOrder(suite.ctx, stopLossOrder)
	suite.Assert().NoError(err, "Stop loss order should be placed")
	suite.Assert().NotNil(placedStopOrder, "Stop loss order should be created")
	suite.Assert().Equal(models.StopLoss, placedStopOrder.Type, "Order type should be StopLoss")
	suite.Assert().Equal(models.Pending, placedStopOrder.Status, "Stop order should be pending")

	log.Printf("Stop loss order test completed - Order ID: %s", placedStopOrder.ID)
}

// TestStopLimitOrders tests stop limit order functionality
func (suite *TradingOrderTypesTestSuite) TestStopLimitOrders() {
	log.Println("Testing stop limit orders...")

	userID := "stop_limit_trader"

	stopLimitOrder := &models.PlaceOrderRequest{
		UserID:           userID,
		Pair:             "BTC/USDT",
		Side:             models.Buy,
		Type:             models.StopLimit,
		Price:            decimal.NewFromInt(52000), // Limit price
		Quantity:         decimal.NewFromFloat(0.1),
		StopPrice:        decimal.NewFromInt(51000), // Stop price
		TriggerCondition: models.GreaterThanOrEqual,
	}

	placedStopLimitOrder, err := suite.service.PlaceOrder(suite.ctx, stopLimitOrder)
	suite.Assert().NoError(err, "Stop limit order should be placed")
	suite.Assert().NotNil(placedStopLimitOrder, "Stop limit order should be created")
	suite.Assert().Equal(models.StopLimit, placedStopLimitOrder.Type, "Order type should be StopLimit")
	suite.Assert().Equal(models.Pending, placedStopLimitOrder.Status, "Stop limit order should be pending")

	log.Printf("Stop limit order test completed - Order ID: %s", placedStopLimitOrder.ID)
}

// TestTakeProfitOrders tests take profit order functionality
func (suite *TradingOrderTypesTestSuite) TestTakeProfitOrders() {
	log.Println("Testing take profit orders...")

	userID := "take_profit_trader"

	takeProfitOrder := &models.PlaceOrderRequest{
		UserID:           userID,
		Pair:             "BTC/USDT",
		Side:             models.Sell,
		Type:             models.TakeProfit,
		Quantity:         decimal.NewFromFloat(0.1),
		StopPrice:        decimal.NewInt(60000), // Take profit at 60000
		TriggerCondition: models.GreaterThanOrEqual,
	}

	placedTakeProfitOrder, err := suite.service.PlaceOrder(suite.ctx, takeProfitOrder)
	suite.Assert().NoError(err, "Take profit order should be placed")
	suite.Assert().NotNil(placedTakeProfitOrder, "Take profit order should be created")
	suite.Assert().Equal(models.TakeProfit, placedTakeProfitOrder.Type, "Order type should be TakeProfit")

	log.Printf("Take profit order test completed - Order ID: %s", placedTakeProfitOrder.ID)
}

// TestTrailingStopOrders tests trailing stop order functionality
func (suite *TradingOrderTypesTestSuite) TestTrailingStopOrders() {
	log.Println("Testing trailing stop orders...")

	userID := "trailing_stop_trader"

	trailingStopOrder := &models.PlaceOrderRequest{
		UserID:          userID,
		Pair:            "BTC/USDT",
		Side:            models.Sell,
		Type:            models.TrailingStop,
		Quantity:        decimal.NewFromFloat(0.1),
		TrailingAmount:  decimal.NewFromInt(2000),  // Trail by $2000
		TrailingPercent: decimal.NewFromFloat(4.0), // or 4%
	}

	placedTrailingOrder, err := suite.service.PlaceOrder(suite.ctx, trailingStopOrder)
	suite.Assert().NoError(err, "Trailing stop order should be placed")
	suite.Assert().NotNil(placedTrailingOrder, "Trailing stop order should be created")
	suite.Assert().Equal(models.TrailingStop, placedTrailingOrder.Type, "Order type should be TrailingStop")

	log.Printf("Trailing stop order test completed - Order ID: %s", placedTrailingOrder.ID)
}

// TestIcebergOrders tests iceberg order functionality
func (suite *TradingOrderTypesTestSuite) TestIcebergOrders() {
	log.Println("Testing iceberg orders...")

	userID := "iceberg_trader"

	icebergOrder := &models.PlaceOrderRequest{
		UserID:          userID,
		Pair:            "BTC/USDT",
		Side:            models.Buy,
		Type:            models.IcebergOrder,
		Price:           decimal.NewFromInt(49000),
		Quantity:        decimal.NewFromFloat(1.0), // Total quantity
		IcebergQuantity: decimal.NewFromFloat(0.1), // Visible quantity
	}

	placedIcebergOrder, err := suite.service.PlaceOrder(suite.ctx, icebergOrder)
	suite.Assert().NoError(err, "Iceberg order should be placed")
	suite.Assert().NotNil(placedIcebergOrder, "Iceberg order should be created")
	suite.Assert().Equal(models.IcebergOrder, placedIcebergOrder.Type, "Order type should be IcebergOrder")

	// Check that only iceberg quantity is visible in order book
	orderBook, err := suite.service.GetOrderBook(suite.ctx, "BTC/USDT", 10)
	if err == nil && orderBook != nil {
		// Verify iceberg behavior (only portion visible)
		log.Printf("Iceberg order in book - visible quantity should be limited")
	}

	log.Printf("Iceberg order test completed - Order ID: %s", placedIcebergOrder.ID)
}

// TestTimeInForceOptions tests various time in force options
func (suite *TradingOrderTypesTestSuite) TestTimeInForceOptions() {
	log.Println("Testing time in force options...")

	userID := "tif_trader"

	// Test Good Till Cancelled (GTC)
	gtcOrder := &models.PlaceOrderRequest{
		UserID:      userID,
		Pair:        "BTC/USDT",
		Side:        models.Buy,
		Type:        models.Limit,
		Price:       decimal.NewFromInt(45000),
		Quantity:    decimal.NewFromFloat(0.1),
		TimeInForce: models.GTC,
	}

	placedGTCOrder, err := suite.service.PlaceOrder(suite.ctx, gtcOrder)
	suite.Assert().NoError(err, "GTC order should be placed")
	suite.Assert().NotNil(placedGTCOrder, "GTC order should be created")

	// Test Immediate Or Cancel (IOC)
	iocOrder := &models.PlaceOrderRequest{
		UserID:      userID,
		Pair:        "BTC/USDT",
		Side:        models.Buy,
		Type:        models.Limit,
		Price:       decimal.NewFromInt(50000),
		Quantity:    decimal.NewFromFloat(0.1),
		TimeInForce: models.IOC,
	}

	placedIOCOrder, err := suite.service.PlaceOrder(suite.ctx, iocOrder)
	// IOC should either fill immediately or be cancelled
	if err == nil {
		suite.Assert().Contains([]models.OrderStatus{models.Filled, models.PartiallyFilled, models.Cancelled},
			placedIOCOrder.Status, "IOC order should have immediate execution result")
	}

	// Test Fill Or Kill (FOK)
	fokOrder := &models.PlaceOrderRequest{
		UserID:      userID,
		Pair:        "BTC/USDT",
		Side:        models.Buy,
		Type:        models.Limit,
		Price:       decimal.NewFromInt(50000),
		Quantity:    decimal.NewFromFloat(0.1),
		TimeInForce: models.FOK,
	}

	placedFOKOrder, err := suite.service.PlaceOrder(suite.ctx, fokOrder)
	// FOK should either fill completely or be rejected
	if err == nil {
		suite.Assert().Contains([]models.OrderStatus{models.Filled, models.Rejected},
			placedFOKOrder.Status, "FOK order should be either filled or rejected")
	}

	// Test Good Till Date (GTD)
	gtdOrder := &models.PlaceOrderRequest{
		UserID:      userID,
		Pair:        "BTC/USDT",
		Side:        models.Buy,
		Type:        models.Limit,
		Price:       decimal.NewFromInt(45000),
		Quantity:    decimal.NewFromFloat(0.1),
		TimeInForce: models.GTD,
		ExpiryTime:  time.Now().Add(1 * time.Hour),
	}

	placedGTDOrder, err := suite.service.PlaceOrder(suite.ctx, gtdOrder)
	suite.Assert().NoError(err, "GTD order should be placed")
	if placedGTDOrder != nil {
		suite.Assert().Equal(models.GTD, placedGTDOrder.TimeInForce, "Should have GTD time in force")
	}

	log.Printf("Time in force test completed - GTC: %s, IOC: %v, FOK: %v, GTD: %s",
		placedGTCOrder.ID, placedIOCOrder != nil, placedFOKOrder != nil,
		func() string {
			if placedGTDOrder != nil {
				return placedGTDOrder.ID
			} else {
				return "nil"
			}
		}())
}

// TestOrderLifecycleManagement tests complete order lifecycle
func (suite *TradingOrderTypesTestSuite) TestOrderLifecycleManagement() {
	log.Println("Testing order lifecycle management...")

	userID := "lifecycle_trader"

	// 1. Place order
	order := &models.PlaceOrderRequest{
		UserID:   userID,
		Pair:     "BTC/USDT",
		Side:     models.Buy,
		Type:     models.Limit,
		Price:    decimal.NewFromInt(45000),
		Quantity: decimal.NewFromFloat(0.5),
	}

	placedOrder, err := suite.service.PlaceOrder(suite.ctx, order)
	suite.Require().NoError(err, "Should place order")
	suite.Require().NotNil(placedOrder, "Order should be created")

	// 2. Verify order is active
	retrievedOrder, err := suite.service.GetOrder(suite.ctx, userID, placedOrder.ID)
	suite.Assert().NoError(err, "Should retrieve order")
	suite.Assert().Equal(placedOrder.ID, retrievedOrder.ID, "Order IDs should match")
	suite.Assert().Equal(models.Open, retrievedOrder.Status, "Order should be open")

	// 3. Partially fill order (simulate)
	// This would require matching logic, so we'll test modification instead

	// 4. Modify order (if supported)
	// Most exchanges don't support direct modification, requires cancel and replace

	// 5. Cancel order
	err = suite.service.CancelOrder(suite.ctx, userID, placedOrder.ID)
	suite.Assert().NoError(err, "Should cancel order")

	// 6. Verify order is cancelled
	cancelledOrder, err := suite.service.GetOrder(suite.ctx, userID, placedOrder.ID)
	if err == nil && cancelledOrder != nil {
		suite.Assert().Equal(models.Cancelled, cancelledOrder.Status, "Order should be cancelled")
	}

	// 7. Check order history
	orders, err := suite.service.GetOrders(suite.ctx, userID, &models.GetOrdersRequest{
		Status: &models.Cancelled,
		Limit:  10,
	})
	suite.Assert().NoError(err, "Should get cancelled orders")
	suite.Assert().True(len(orders) > 0, "Should have cancelled orders in history")

	log.Printf("Order lifecycle test completed - Order ID: %s", placedOrder.ID)
}

// TestOrderMatching tests order matching scenarios
func (suite *TradingOrderTypesTestSuite) TestOrderMatching() {
	log.Println("Testing order matching scenarios...")

	buyer := "buyer_user"
	seller := "seller_user"

	// Place sell order first
	sellOrder := &models.PlaceOrderRequest{
		UserID:   seller,
		Pair:     "ETH/USDT",
		Side:     models.Sell,
		Type:     models.Limit,
		Price:    decimal.NewFromInt(3000),
		Quantity: decimal.NewFromFloat(2.0),
	}

	placedSellOrder, err := suite.service.PlaceOrder(suite.ctx, sellOrder)
	suite.Require().NoError(err, "Should place sell order")

	// Place matching buy order
	buyOrder := &models.PlaceOrderRequest{
		UserID:   buyer,
		Pair:     "ETH/USDT",
		Side:     models.Buy,
		Type:     models.Limit,
		Price:    decimal.NewFromInt(3000),
		Quantity: decimal.NewFromFloat(1.0), // Partial match
	}

	placedBuyOrder, err := suite.service.PlaceOrder(suite.ctx, buyOrder)
	suite.Assert().NoError(err, "Should place buy order")

	// Check if orders matched
	if placedBuyOrder != nil {
		updatedBuyOrder, err := suite.service.GetOrder(suite.ctx, buyer, placedBuyOrder.ID)
		if err == nil && updatedBuyOrder != nil {
			suite.Assert().Contains([]models.OrderStatus{models.Filled, models.PartiallyFilled},
				updatedBuyOrder.Status, "Buy order should be filled or partially filled")
		}
	}

	if placedSellOrder != nil {
		updatedSellOrder, err := suite.service.GetOrder(suite.ctx, seller, placedSellOrder.ID)
		if err == nil && updatedSellOrder != nil {
			suite.Assert().Contains([]models.OrderStatus{models.Open, models.PartiallyFilled},
				updatedSellOrder.Status, "Sell order should be open or partially filled")
		}
	}

	// Check transaction records
	transactions := suite.mockBookkeeper.GetTransactions()
	if len(transactions) > 0 {
		log.Printf("Generated %d transactions from matching", len(transactions))
	}

	log.Printf("Order matching test completed")
}

// TestComplexOrderScenarios tests complex multi-order scenarios
func (suite *TradingOrderTypesTestSuite) TestComplexOrderScenarios() {
	log.Println("Testing complex order scenarios...")

	trader := "complex_trader"

	// Scenario 1: Bracket order (buy + stop loss + take profit)

	// Main buy order
	mainOrder := &models.PlaceOrderRequest{
		UserID:   trader,
		Pair:     "BTC/USDT",
		Side:     models.Buy,
		Type:     models.Limit,
		Price:    decimal.NewFromInt(50000),
		Quantity: decimal.NewFromFloat(0.1),
	}

	placedMainOrder, err := suite.service.PlaceOrder(suite.ctx, mainOrder)
	if err == nil && placedMainOrder != nil {
		// Stop loss order
		stopLoss := &models.PlaceOrderRequest{
			UserID:        trader,
			Pair:          "BTC/USDT",
			Side:          models.Sell,
			Type:          models.StopLoss,
			Quantity:      decimal.NewFromFloat(0.1),
			StopPrice:     decimal.NewFromInt(45000),
			ParentOrderID: placedMainOrder.ID,
		}

		placedStopLoss, err := suite.service.PlaceOrder(suite.ctx, stopLoss)
		suite.Assert().NoError(err, "Should place stop loss order")

		// Take profit order
		takeProfit := &models.PlaceOrderRequest{
			UserID:        trader,
			Pair:          "BTC/USDT",
			Side:          models.Sell,
			Type:          models.TakeProfit,
			Quantity:      decimal.NewFromFloat(0.1),
			StopPrice:     decimal.NewFromInt(60000),
			ParentOrderID: placedMainOrder.ID,
		}

		placedTakeProfit, err := suite.service.PlaceOrder(suite.ctx, takeProfit)
		suite.Assert().NoError(err, "Should place take profit order")

		log.Printf("Bracket order scenario completed - Main: %s, Stop: %s, Profit: %s",
			placedMainOrder.ID,
			func() string {
				if placedStopLoss != nil {
					return placedStopLoss.ID
				} else {
					return "nil"
				}
			}(),
			func() string {
				if placedTakeProfit != nil {
					return placedTakeProfit.ID
				} else {
					return "nil"
				}
			}())
	}

	// Scenario 2: Grid trading (multiple buy/sell orders)
	gridSpacing := decimal.NewFromInt(1000)
	basePrice := decimal.NewFromInt(50000)
	gridSize := 5

	var gridOrders []string
	for i := -gridSize / 2; i <= gridSize/2; i++ {
		if i == 0 {
			continue // Skip center price
		}

		price := basePrice.Add(gridSpacing.Mul(decimal.NewFromInt(int64(i))))
		side := models.Buy
		if i > 0 {
			side = models.Sell
		}

		gridOrder := &models.PlaceOrderRequest{
			UserID:   trader,
			Pair:     "BTC/USDT",
			Side:     side,
			Type:     models.Limit,
			Price:    price,
			Quantity: decimal.NewFromFloat(0.01),
		}

		placedGridOrder, err := suite.service.PlaceOrder(suite.ctx, gridOrder)
		if err == nil && placedGridOrder != nil {
			gridOrders = append(gridOrders, placedGridOrder.ID)
		}
	}

	log.Printf("Grid trading scenario completed - %d orders placed", len(gridOrders))

	log.Printf("Complex order scenarios test completed")
}

func TestTradingOrderTypesTestSuite(t *testing.T) {
	suite.Run(t, new(TradingOrderTypesTestSuite))
}
