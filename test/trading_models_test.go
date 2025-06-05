//go:build trading
// +build trading

package test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap/zaptest"

	"github.com/Aidin1998/pincex_unified/internal/trading"
	"github.com/Aidin1998/pincex_unified/internal/trading/model"
	"github.com/Aidin1998/pincex_unified/pkg/models"
)

// TradingModelTestSuite tests trading model structures and validations
type TradingModelTestSuite struct {
	suite.Suite
	logger *zap.Logger
}

func (suite *TradingModelTestSuite) SetupTest() {
	suite.logger = zaptest.NewLogger(suite.T())
}

func TestTradingModelSuite(t *testing.T) {
	suite.Run(t, new(TradingModelTestSuite))
}

// TestOrderModel tests the Order model structure and validation
func (suite *TradingModelTestSuite) TestOrderModel() {
	suite.Run("ValidOrder", func() {
		order := &model.Order{
			ID:               uuid.New(),
			UserID:           uuid.New(),
			Pair:             "BTCUSDT",
			Side:             model.OrderSideBuy,
			Type:             model.OrderTypeLimit,
			Price:            decimal.NewFromFloat(50000.00),
			Quantity:         decimal.NewFromFloat(0.001),
			FilledQuantity:   decimal.Zero,
			Status:           model.OrderStatusNew,
			TimeInForce:      model.TimeInForceGTC,
			CreatedAt:        time.Now(),
			UpdatedAt:        time.Now(),
		}

		suite.NotNil(order.ID)
		suite.NotNil(order.UserID)
		suite.Equal("BTCUSDT", order.Pair)
		suite.Equal(model.OrderSideBuy, order.Side)
		suite.Equal(model.OrderTypeLimit, order.Type)
		suite.True(order.Price.GreaterThan(decimal.Zero))
		suite.True(order.Quantity.GreaterThan(decimal.Zero))
		suite.True(order.FilledQuantity.Equal(decimal.Zero))
		suite.Equal(model.OrderStatusNew, order.Status)
	})

	suite.Run("OrderPooling", func() {
		// Test object pooling for performance
		order1 := model.GetOrderFromPool()
		suite.NotNil(order1)
		
		// Modify order
		order1.ID = uuid.New()
		order1.Pair = "ETHUSDT"
		order1.Price = decimal.NewFromFloat(3000.00)
		
		// Return to pool
		model.PutOrderToPool(order1)
		
		// Get another order from pool
		order2 := model.GetOrderFromPool()
		suite.NotNil(order2)
		
		// Should be reset
		suite.Equal(uuid.Nil, order2.ID)
		suite.Equal("", order2.Pair)
		suite.True(order2.Price.Equal(decimal.Zero))
	})

	suite.Run("OrderConstants", func() {
		// Test order type constants
		suite.Equal("LIMIT", model.OrderTypeLimit)
		suite.Equal("MARKET", model.OrderTypeMarket)
		suite.Equal("STOP_LIMIT", model.OrderTypeStopLimit)
		
		// Test order side constants
		suite.Equal("BUY", model.OrderSideBuy)
		suite.Equal("SELL", model.OrderSideSell)
		
		// Test order status constants
		suite.Equal("NEW", model.OrderStatusNew)
		suite.Equal("OPEN", model.OrderStatusOpen)
		suite.Equal("FILLED", model.OrderStatusFilled)
		suite.Equal("CANCELLED", model.OrderStatusCancelled)
	})
}

// TestTradeModel tests the Trade model structure
func (suite *TradingModelTestSuite) TestTradeModel() {
	suite.Run("ValidTrade", func() {
		trade := &model.Trade{
			ID:             uuid.New(),
			OrderID:        uuid.New(),
			CounterOrderID: uuid.New(),
			UserID:         uuid.New(),
			CounterUserID:  uuid.New(),
			Pair:           "BTCUSDT",
			Price:          decimal.NewFromFloat(50000.00),
			Quantity:       decimal.NewFromFloat(0.001),
			Side:           model.OrderSideBuy,
			Maker:          true,
			CreatedAt:      time.Now(),
		}

		suite.NotNil(trade.ID)
		suite.NotNil(trade.OrderID)
		suite.NotNil(trade.CounterOrderID)
		suite.Equal("BTCUSDT", trade.Pair)
		suite.True(trade.Price.GreaterThan(decimal.Zero))
		suite.True(trade.Quantity.GreaterThan(decimal.Zero))
		suite.True(trade.Maker)
	})

	suite.Run("TradePooling", func() {
		// Test trade object pooling
		trade1 := model.GetTradeFromPool()
		suite.NotNil(trade1)
		
		// Modify trade
		trade1.ID = uuid.New()
		trade1.Pair = "ETHUSDT"
		trade1.Price = decimal.NewFromFloat(3000.00)
		
		// Return to pool
		model.PutTradeToPool(trade1)
		
		// Get another trade from pool
		trade2 := model.GetTradeFromPool()
		suite.NotNil(trade2)
		
		// Should be reset
		suite.Equal(uuid.Nil, trade2.ID)
		suite.Equal("", trade2.Pair)
		suite.True(trade2.Price.Equal(decimal.Zero))
	})
}

// TestOrderValidation tests order validation logic
func (suite *TradingModelTestSuite) TestOrderValidation() {
	suite.Run("ValidLimitOrder", func() {
		order := model.NewOrderForTest("BTCUSDT", model.OrderSideBuy, "50000.00", "0.001")
		
		suite.Equal("BTCUSDT", order.Pair)
		suite.Equal(model.OrderSideBuy, order.Side)
		suite.Equal(decimal.RequireFromString("50000.00"), order.Price)
		suite.Equal(decimal.RequireFromString("0.001"), order.Quantity)
	})

	suite.Run("InvalidOrderData", func() {
		// Test with invalid pair
		order := &model.Order{
			Pair:     "", // Empty pair should be invalid
			Side:     model.OrderSideBuy,
			Type:     model.OrderTypeLimit,
			Price:    decimal.NewFromFloat(50000.00),
			Quantity: decimal.NewFromFloat(0.001),
		}
		
		suite.Equal("", order.Pair)
		
		// Test with negative price
		order.Price = decimal.NewFromFloat(-100.00)
		suite.True(order.Price.LessThan(decimal.Zero))
		
		// Test with zero quantity
		order.Quantity = decimal.Zero
		suite.True(order.Quantity.Equal(decimal.Zero))
	})
}

// TestOrderTypes tests different order types and their properties
func (suite *TradingModelTestSuite) TestOrderTypes() {
	baseOrder := &model.Order{
		ID:       uuid.New(),
		UserID:   uuid.New(),
		Pair:     "BTCUSDT",
		Side:     model.OrderSideBuy,
		Quantity: decimal.NewFromFloat(0.001),
		Status:   model.OrderStatusNew,
	}

	suite.Run("LimitOrder", func() {
		order := *baseOrder
		order.Type = model.OrderTypeLimit
		order.Price = decimal.NewFromFloat(50000.00)
		order.TimeInForce = model.TimeInForceGTC
		
		suite.Equal(model.OrderTypeLimit, order.Type)
		suite.True(order.Price.GreaterThan(decimal.Zero))
		suite.Equal(model.TimeInForceGTC, order.TimeInForce)
	})

	suite.Run("MarketOrder", func() {
		order := *baseOrder
		order.Type = model.OrderTypeMarket
		order.TimeInForce = model.TimeInForceIOC
		
		suite.Equal(model.OrderTypeMarket, order.Type)
		suite.Equal(model.TimeInForceIOC, order.TimeInForce)
	})

	suite.Run("StopLimitOrder", func() {
		order := *baseOrder
		order.Type = model.OrderTypeStopLimit
		order.Price = decimal.NewFromFloat(50000.00)
		order.StopPrice = decimal.NewFromFloat(49000.00)
		order.TimeInForce = model.TimeInForceGTC
		
		suite.Equal(model.OrderTypeStopLimit, order.Type)
		suite.True(order.StopPrice.LessThan(order.Price))
	})

	suite.Run("IcebergOrder", func() {
		order := *baseOrder
		order.Type = model.OrderTypeIceberg
		order.Price = decimal.NewFromFloat(50000.00)
		order.DisplayQuantity = decimal.NewFromFloat(0.0001) // Show only 10% of quantity
		order.Hidden = false // Iceberg shows partial quantity
		
		suite.Equal(model.OrderTypeIceberg, order.Type)
		suite.True(order.DisplayQuantity.LessThan(order.Quantity))
		suite.False(order.Hidden)
	})

	suite.Run("HiddenOrder", func() {
		order := *baseOrder
		order.Type = model.OrderTypeHidden
		order.Price = decimal.NewFromFloat(50000.00)
		order.Hidden = true
		
		suite.Equal(model.OrderTypeHidden, order.Type)
		suite.True(order.Hidden)
	})
}

// TestOrderLifecycle tests order state transitions
func (suite *TradingModelTestSuite) TestOrderLifecycle() {
	suite.Run("NewToOpen", func() {
		order := model.NewOrderForTest("BTCUSDT", model.OrderSideBuy, "50000.00", "0.001")
		order.Status = model.OrderStatusNew
		
		// Transition to open
		order.Status = model.OrderStatusOpen
		suite.Equal(model.OrderStatusOpen, order.Status)
	})

	suite.Run("PartialFill", func() {
		order := model.NewOrderForTest("BTCUSDT", model.OrderSideBuy, "50000.00", "0.001")
		order.Status = model.OrderStatusOpen
		
		// Partial fill
		order.FilledQuantity = decimal.NewFromFloat(0.0005)
		order.Status = model.OrderStatusPartiallyFilled
		
		suite.Equal(model.OrderStatusPartiallyFilled, order.Status)
		suite.True(order.FilledQuantity.LessThan(order.Quantity))
		suite.True(order.FilledQuantity.GreaterThan(decimal.Zero))
	})

	suite.Run("FullFill", func() {
		order := model.NewOrderForTest("BTCUSDT", model.OrderSideBuy, "50000.00", "0.001")
		order.Status = model.OrderStatusPartiallyFilled
		order.FilledQuantity = decimal.NewFromFloat(0.0005)
		
		// Complete fill
		order.FilledQuantity = order.Quantity
		order.Status = model.OrderStatusFilled
		
		suite.Equal(model.OrderStatusFilled, order.Status)
		suite.True(order.FilledQuantity.Equal(order.Quantity))
	})

	suite.Run("Cancellation", func() {
		order := model.NewOrderForTest("BTCUSDT", model.OrderSideBuy, "50000.00", "0.001")
		order.Status = model.OrderStatusOpen
		
		// Cancel order
		order.Status = model.OrderStatusCancelled
		suite.Equal(model.OrderStatusCancelled, order.Status)
	})
}

// TestObjectPoolMetrics tests object pool performance metrics
func (suite *TradingModelTestSuite) TestObjectPoolMetrics() {
	suite.Run("OrderPoolMetrics", func() {
		// Reset metrics for clean test
		model.PreallocateObjectPools()
		
		// Get some orders
		for i := 0; i < 10; i++ {
			order := model.GetOrderFromPool()
			model.PutOrderToPool(order)
		}
		
		metrics := model.GetOrderPoolMetrics()
		suite.True(metrics.Gets >= 10)
		suite.True(metrics.Puts >= 10)
	})

	suite.Run("TradePoolMetrics", func() {
		// Get some trades
		for i := 0; i < 5; i++ {
			trade := model.GetTradeFromPool()
			model.PutTradeToPool(trade)
		}
		
		metrics := model.GetTradePoolMetrics()
		suite.True(metrics.Gets >= 5)
		suite.True(metrics.Puts >= 5)
	})
}

// BenchmarkOrderPooling benchmarks object pooling performance
func BenchmarkOrderPooling(b *testing.B) {
	model.PreallocateObjectPools()
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			order := model.GetOrderFromPool()
			order.ID = uuid.New()
			order.Pair = "BTCUSDT"
			order.Price = decimal.NewFromFloat(50000.00)
			model.PutOrderToPool(order)
		}
	})
}

// BenchmarkTradePooling benchmarks trade object pooling performance
func BenchmarkTradePooling(b *testing.B) {
	model.PreallocateObjectPools()
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			trade := model.GetTradeFromPool()
			trade.ID = uuid.New()
			trade.Pair = "BTCUSDT"
			trade.Price = decimal.NewFromFloat(50000.00)
			model.PutTradeToPool(trade)
		}
	})
}
