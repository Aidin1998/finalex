package coordination

import (
	"context"
	"testing"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/settlement"
	"github.com/Aidin1998/pincex_unified/internal/trading/model"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// MockRepository implements the model.Repository interface for testing
type MockRepository struct {
	mock.Mock
}

func (m *MockRepository) CreateOrder(ctx context.Context, order *model.Order) error {
	args := m.Called(ctx, order)
	return args.Error(0)
}

func (m *MockRepository) GetOrder(ctx context.Context, id uuid.UUID) (*model.Order, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Order), args.Error(1)
}

func (m *MockRepository) GetOrderByID(ctx context.Context, orderID uuid.UUID) (*model.Order, error) {
	args := m.Called(ctx, orderID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Order), args.Error(1)
}

func (m *MockRepository) UpdateOrder(ctx context.Context, order *model.Order) error {
	args := m.Called(ctx, order)
	return args.Error(0)
}

func (m *MockRepository) GetOpenOrdersByPair(ctx context.Context, pair string) ([]*model.Order, error) {
	args := m.Called(ctx, pair)
	return args.Get(0).([]*model.Order), args.Error(1)
}

func (m *MockRepository) GetOpenOrdersByUser(ctx context.Context, userID uuid.UUID) ([]*model.Order, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).([]*model.Order), args.Error(1)
}

func (m *MockRepository) UpdateOrderStatus(ctx context.Context, orderID uuid.UUID, status string) error {
	args := m.Called(ctx, orderID, status)
	return args.Error(0)
}

func (m *MockRepository) UpdateOrderStatusAndFilledQuantity(ctx context.Context, orderID uuid.UUID, status string, filledQty, avgPrice decimal.Decimal) error {
	args := m.Called(ctx, orderID, status, filledQty, avgPrice)
	return args.Error(0)
}

func (m *MockRepository) CancelOrder(ctx context.Context, orderID uuid.UUID) error {
	args := m.Called(ctx, orderID)
	return args.Error(0)
}

func (m *MockRepository) ConvertStopOrderToLimit(ctx context.Context, order *model.Order) error {
	args := m.Called(ctx, order)
	return args.Error(0)
}

func (m *MockRepository) CreateTrade(ctx context.Context, trade *model.Trade) error {
	args := m.Called(ctx, trade)
	return args.Error(0)
}

func (m *MockRepository) CreateTradeTx(ctx context.Context, tx *gorm.DB, trade *model.Trade) error {
	args := m.Called(ctx, tx, trade)
	return args.Error(0)
}

func (m *MockRepository) UpdateOrderStatusTx(ctx context.Context, tx *gorm.DB, orderID uuid.UUID, status string, filledQty, avgPrice decimal.Decimal) error {
	args := m.Called(ctx, tx, orderID, status, filledQty, avgPrice)
	return args.Error(0)
}

func (m *MockRepository) BatchUpdateOrderStatusTx(ctx context.Context, tx *gorm.DB, updates []struct {
	OrderID uuid.UUID
	Status  string
}) error {
	args := m.Called(ctx, tx, updates)
	return args.Error(0)
}

func (m *MockRepository) ExecuteInTransaction(ctx context.Context, txFunc func(*gorm.DB) error) error {
	args := m.Called(ctx, txFunc)
	return args.Error(0)
}

func (m *MockRepository) UpdateOrderHoldID(ctx context.Context, orderID uuid.UUID, holdID string) error {
	args := m.Called(ctx, orderID, holdID)
	return args.Error(0)
}

func (m *MockRepository) GetExpiredGTDOrders(ctx context.Context, pair string, now time.Time) ([]*model.Order, error) {
	args := m.Called(ctx, pair, now)
	return args.Get(0).([]*model.Order), args.Error(1)
}

func (m *MockRepository) GetOCOSiblingOrder(ctx context.Context, ocoGroupID uuid.UUID, excludeOrderID uuid.UUID) (*model.Order, error) {
	args := m.Called(ctx, ocoGroupID, excludeOrderID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Order), args.Error(1)
}

func (m *MockRepository) GetOpenTrailingStopOrders(ctx context.Context, pair string) ([]*model.Order, error) {
	args := m.Called(ctx, pair)
	return args.Get(0).([]*model.Order), args.Error(1)
}

func (m *MockRepository) DeleteOrder(ctx context.Context, id uuid.UUID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockRepository) ListOrders(ctx context.Context, filters map[string]interface{}) ([]*model.Order, error) {
	args := m.Called(ctx, filters)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.Order), args.Error(1)
}

func (m *MockRepository) GetTrade(ctx context.Context, id uuid.UUID) (*model.Trade, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Trade), args.Error(1)
}

func (m *MockRepository) ListTrades(ctx context.Context, filters map[string]interface{}) ([]*model.Trade, error) {
	args := m.Called(ctx, filters)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.Trade), args.Error(1)
}

func TestNewTradeSettlementCoordinator(t *testing.T) {
	logger := zap.NewNop()
	settlementEngine := settlement.NewSettlementEngine()
	mockRepo := &MockRepository{}

	kafkaConfig := &CoordinatorKafkaConfig{
		Brokers:         []string{"localhost:9092"},
		TradeTopicIn:    "trade-topic-in",
		TradeTopicOut:   "trade-topic-out",
		SettlementTopic: "settlement-topic",
		ConsumerGroup:   "test-group",
	}

	coordinator, err := NewTradeSettlementCoordinator(
		logger,
		settlementEngine,
		mockRepo,
		kafkaConfig,
	)

	assert.NoError(t, err)
	assert.NotNil(t, coordinator)
	assert.Equal(t, mockRepo, coordinator.repository)
	assert.Equal(t, settlementEngine, coordinator.settlementEngine)
	assert.Equal(t, logger, coordinator.logger)
}

func TestValidateTradeConsistency_ValidTrade(t *testing.T) {
	logger := zap.NewNop()
	settlementEngine := settlement.NewSettlementEngine()
	mockRepo := &MockRepository{}

	kafkaConfig := &CoordinatorKafkaConfig{
		Brokers:         []string{"localhost:9092"},
		TradeTopicIn:    "trade-topic-in",
		TradeTopicOut:   "trade-topic-out",
		SettlementTopic: "settlement-topic",
		ConsumerGroup:   "test-group",
	}

	coordinator, err := NewTradeSettlementCoordinator(
		logger,
		settlementEngine,
		mockRepo,
		kafkaConfig,
	)
	assert.NoError(t, err)

	// Create test data
	orderID := uuid.New()
	userID := uuid.New()
	tradeID := uuid.New()
	testOrder := &model.Order{
		ID:        orderID,
		UserID:    userID,
		Pair:      "BTCUSDT",
		Side:      "buy",
		Type:      "limit",
		Quantity:  decimal.NewFromFloat(1.0),
		Price:     decimal.NewFromFloat(50000.0),
		Status:    "filled",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	testTrade := &model.Trade{
		ID:        tradeID,
		OrderID:   orderID,
		Pair:      "BTCUSDT",
		Side:      "buy",
		Quantity:  decimal.NewFromFloat(1.0),
		Price:     decimal.NewFromFloat(50000.0),
		Maker:     true,
		CreatedAt: time.Now(),
	}

	// Mock the repository call
	mockRepo.On("GetOrder", mock.Anything, orderID).Return(testOrder, nil)

	// Test validation
	ctx := context.Background()
	err = coordinator.ValidateTradeConsistency(ctx, testTrade)

	assert.NoError(t, err)
	mockRepo.AssertExpectations(t)
}

func TestValidateTradeConsistency_InvalidTradeID(t *testing.T) {
	logger := zap.NewNop()
	settlementEngine := settlement.NewSettlementEngine()
	mockRepo := &MockRepository{}

	kafkaConfig := &CoordinatorKafkaConfig{
		Brokers:         []string{"localhost:9092"},
		TradeTopicIn:    "trade-topic-in",
		TradeTopicOut:   "trade-topic-out",
		SettlementTopic: "settlement-topic",
		ConsumerGroup:   "test-group",
	}

	coordinator, err := NewTradeSettlementCoordinator(
		logger,
		settlementEngine,
		mockRepo,
		kafkaConfig,
	)
	assert.NoError(t, err)

	// Test with invalid trade ID
	testTrade := &model.Trade{
		ID:        uuid.Nil, // Invalid ID
		OrderID:   uuid.New(),
		Pair:      "BTCUSDT",
		Side:      "buy",
		Quantity:  decimal.NewFromFloat(1.0),
		Price:     decimal.NewFromFloat(50000.0),
		Maker:     true,
		CreatedAt: time.Now(),
	}

	ctx := context.Background()
	err = coordinator.ValidateTradeConsistency(ctx, testTrade)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid trade ID")
}

func TestValidateTradeConsistency_InvalidQuantity(t *testing.T) {
	logger := zap.NewNop()
	settlementEngine := settlement.NewSettlementEngine()
	mockRepo := &MockRepository{}

	kafkaConfig := &CoordinatorKafkaConfig{
		Brokers:         []string{"localhost:9092"},
		TradeTopicIn:    "trade-topic-in",
		TradeTopicOut:   "trade-topic-out",
		SettlementTopic: "settlement-topic",
		ConsumerGroup:   "test-group",
	}

	coordinator, err := NewTradeSettlementCoordinator(
		logger,
		settlementEngine,
		mockRepo,
		kafkaConfig,
	)
	assert.NoError(t, err)

	// Test with invalid quantity
	testTrade := &model.Trade{
		ID:        uuid.New(),
		OrderID:   uuid.New(),
		Pair:      "BTCUSDT",
		Side:      "buy",
		Quantity:  decimal.NewFromFloat(-1.0), // Invalid negative quantity
		Price:     decimal.NewFromFloat(50000.0),
		Maker:     true,
		CreatedAt: time.Now(),
	}

	ctx := context.Background()
	err = coordinator.ValidateTradeConsistency(ctx, testTrade)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid trade quantity")
}

func TestValidateTradeConsistency_PairMismatch(t *testing.T) {
	logger := zap.NewNop()
	settlementEngine := settlement.NewSettlementEngine()
	mockRepo := &MockRepository{}

	kafkaConfig := &CoordinatorKafkaConfig{
		Brokers:         []string{"localhost:9092"},
		TradeTopicIn:    "trade-topic-in",
		TradeTopicOut:   "trade-topic-out",
		SettlementTopic: "settlement-topic",
		ConsumerGroup:   "test-group",
	}

	coordinator, err := NewTradeSettlementCoordinator(
		logger,
		settlementEngine,
		mockRepo,
		kafkaConfig,
	)
	assert.NoError(t, err)

	// Create test data with mismatched pairs
	orderID := uuid.New()
	userID := uuid.New()
	tradeID := uuid.New()
	testOrder := &model.Order{
		ID:        orderID,
		UserID:    userID,
		Pair:      "ETHUSDT", // Different pair than trade
		Side:      "buy",
		Type:      "limit",
		Quantity:  decimal.NewFromFloat(1.0),
		Price:     decimal.NewFromFloat(3000.0),
		Status:    "filled",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	testTrade := &model.Trade{
		ID:        tradeID,
		OrderID:   orderID,
		Pair:      "BTCUSDT", // Different pair than order
		Side:      "buy",
		Quantity:  decimal.NewFromFloat(1.0),
		Price:     decimal.NewFromFloat(50000.0),
		Maker:     true,
		CreatedAt: time.Now(),
	}

	// Mock the repository call
	mockRepo.On("GetOrder", mock.Anything, orderID).Return(testOrder, nil)

	// Test validation
	ctx := context.Background()
	err = coordinator.ValidateTradeConsistency(ctx, testTrade)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "trade pair BTCUSDT does not match order pair ETHUSDT")
	mockRepo.AssertExpectations(t)
}

func TestValidateTradeConsistency_OrderNotFound(t *testing.T) {
	logger := zap.NewNop()
	settlementEngine := settlement.NewSettlementEngine()
	mockRepo := &MockRepository{}

	kafkaConfig := &CoordinatorKafkaConfig{
		Brokers:         []string{"localhost:9092"},
		TradeTopicIn:    "trade-topic-in",
		TradeTopicOut:   "trade-topic-out",
		SettlementTopic: "settlement-topic",
		ConsumerGroup:   "test-group",
	}

	coordinator, err := NewTradeSettlementCoordinator(
		logger,
		settlementEngine,
		mockRepo,
		kafkaConfig,
	)
	assert.NoError(t, err)

	// Create test trade
	orderID := uuid.New()
	tradeID := uuid.New()

	testTrade := &model.Trade{
		ID:        tradeID,
		OrderID:   orderID,
		Pair:      "BTCUSDT",
		Side:      "buy",
		Quantity:  decimal.NewFromFloat(1.0),
		Price:     decimal.NewFromFloat(50000.0),
		Maker:     true,
		CreatedAt: time.Now(),
	}

	// Mock the repository to return an error (order not found)
	mockRepo.On("GetOrder", mock.Anything, orderID).Return(nil, assert.AnError)

	// Test validation - should not fail even if order is not found
	ctx := context.Background()
	err = coordinator.ValidateTradeConsistency(ctx, testTrade)

	// Should not error because we handle missing orders gracefully
	assert.NoError(t, err)
	mockRepo.AssertExpectations(t)
}
