//go:build trading
// +build trading

package test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/Aidin1998/finalex/internal/trading/handlers"
	"github.com/Aidin1998/finalex/pkg/models"
)

// Mock Trading Service for Handler Tests
type MockTradingService struct {
	mock.Mock
}

func (m *MockTradingService) Start() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockTradingService) Stop() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockTradingService) PlaceOrder(ctx context.Context, order *models.Order) (*models.Order, error) {
	args := m.Called(ctx, order)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Order), args.Error(1)
}

func (m *MockTradingService) CancelOrder(ctx context.Context, orderID string) error {
	args := m.Called(ctx, orderID)
	return args.Error(0)
}

func (m *MockTradingService) GetOrder(orderID string) (*models.Order, error) {
	args := m.Called(orderID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Order), args.Error(1)
}

func (m *MockTradingService) GetOrders(userID, symbol, status string, limit, offset string) ([]*models.Order, int64, error) {
	args := m.Called(userID, symbol, status, limit, offset)
	return args.Get(0).([]*models.Order), args.Get(1).(int64), args.Error(2)
}

func (m *MockTradingService) GetOrderBook(symbol string, depth int) (*models.OrderBookSnapshot, error) {
	args := m.Called(symbol, depth)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.OrderBookSnapshot), args.Error(1)
}

func (m *MockTradingService) GetOrderBookBinary(symbol string, depth int) ([]byte, error) {
	args := m.Called(symbol, depth)
	return args.Get(0).([]byte), args.Error(1)
}

func (m *MockTradingService) GetTradingPairs() ([]*models.TradingPair, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.TradingPair), args.Error(1)
}

func (m *MockTradingService) GetTradingPair(symbol string) (*models.TradingPair, error) {
	args := m.Called(symbol)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.TradingPair), args.Error(1)
}

func (m *MockTradingService) CreateTradingPair(pair *models.TradingPair) (*models.TradingPair, error) {
	args := m.Called(pair)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.TradingPair), args.Error(1)
}

func (m *MockTradingService) UpdateTradingPair(pair *models.TradingPair) (*models.TradingPair, error) {
	args := m.Called(pair)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.TradingPair), args.Error(1)
}

func (m *MockTradingService) ListOrders(userID string, filter *models.OrderFilter) ([]*models.Order, error) {
	args := m.Called(userID, filter)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.Order), args.Error(1)
}

// TradingHandlerTestSuite tests trading HTTP handlers
type TradingHandlerTestSuite struct {
	suite.Suite
	router        *gin.Engine
	handler       *handlers.TradingHandler
	marketHandler *handlers.MarketDataHandler
	mockService   *MockTradingService
	logger        *zap.Logger
}

func (suite *TradingHandlerTestSuite) SetupTest() {
	suite.logger = zaptest.NewLogger(suite.T())
	suite.mockService = new(MockTradingService)

	// Create handlers
	suite.handler = handlers.NewTradingHandler(suite.mockService, suite.logger)
	suite.marketHandler = handlers.NewMarketDataHandler(suite.mockService, suite.logger)

	// Setup Gin router
	gin.SetMode(gin.TestMode)
	suite.router = gin.New()

	// Add auth middleware mock
	suite.router.Use(func(c *gin.Context) {
		c.Set("user_id", "123e4567-e89b-12d3-a456-426614174000")
		c.Next()
	})

	// Setup routes
	suite.setupRoutes()
}

func (suite *TradingHandlerTestSuite) setupRoutes() {
	v1 := suite.router.Group("/v1")
	trading := v1.Group("/trading")
	{
		trading.POST("/orders", suite.handler.PlaceOrder)
		trading.GET("/orders", suite.handler.GetOrders)
		trading.GET("/orders/:id", suite.handler.GetOrder)
		trading.DELETE("/orders/:id", suite.handler.CancelOrder)
	}

	api := suite.router.Group("/api/v1")
	{
		api.GET("/depth/:symbol", suite.marketHandler.GetOrderBook)
		api.GET("/ticker/24hr/:symbol", suite.marketHandler.GetTicker)
		api.GET("/ticker/24hr", suite.marketHandler.GetAllTickers)
		api.GET("/trades/:symbol", suite.marketHandler.GetTrades)
		api.GET("/exchangeInfo", suite.marketHandler.GetExchangeInfo)
	}
}

func TestTradingHandlerSuite(t *testing.T) {
	suite.Run(t, new(TradingHandlerTestSuite))
}

// Test Place Order Handler
func (suite *TradingHandlerTestSuite) TestPlaceOrderHandler() {
	suite.Run("ValidLimitOrder", func() {
		// Setup mock response
		expectedOrder := &models.Order{
			ID:          uuid.New(),
			UserID:      uuid.MustParse("123e4567-e89b-12d3-a456-426614174000"),
			Symbol:      "BTCUSDT",
			Side:        "buy",
			Type:        "limit",
			Quantity:    0.001,
			Price:       50000.0,
			Status:      "NEW",
			TimeInForce: "GTC",
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		suite.mockService.On("PlaceOrder", mock.AnythingOfType("*gin.Context"), mock.AnythingOfType("*models.Order")).
			Return(expectedOrder, nil)

		// Create request
		reqBody := map[string]interface{}{
			"symbol":        "BTCUSDT",
			"side":          "buy",
			"type":          "limit",
			"quantity":      "0.001",
			"price":         "50000.00",
			"time_in_force": "GTC",
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/v1/trading/orders", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		suite.router.ServeHTTP(w, req)

		assert.Equal(suite.T(), http.StatusCreated, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(suite.T(), err)
		assert.Equal(suite.T(), "BTCUSDT", response["symbol"])
		assert.Equal(suite.T(), "buy", response["side"])
		assert.Equal(suite.T(), "limit", response["type"])

		suite.mockService.AssertExpectations(suite.T())
	})

	suite.Run("ValidMarketOrder", func() {
		expectedOrder := &models.Order{
			ID:          uuid.New(),
			UserID:      uuid.MustParse("123e4567-e89b-12d3-a456-426614174000"),
			Symbol:      "BTCUSDT",
			Side:        "sell",
			Type:        "market",
			Quantity:    0.001,
			Status:      "NEW",
			TimeInForce: "IOC",
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		suite.mockService.On("PlaceOrder", mock.AnythingOfType("*gin.Context"), mock.AnythingOfType("*models.Order")).
			Return(expectedOrder, nil)

		reqBody := map[string]interface{}{
			"symbol":   "BTCUSDT",
			"side":     "sell",
			"type":     "market",
			"quantity": "0.001",
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/v1/trading/orders", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		suite.router.ServeHTTP(w, req)

		assert.Equal(suite.T(), http.StatusCreated, w.Code)
		suite.mockService.AssertExpectations(suite.T())
	})

	suite.Run("InvalidRequest", func() {
		reqBody := map[string]interface{}{
			"symbol": "BTCUSDT",
			// Missing required fields
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/v1/trading/orders", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		suite.router.ServeHTTP(w, req)

		assert.Equal(suite.T(), http.StatusBadRequest, w.Code)
	})

	suite.Run("ServiceError", func() {
		suite.mockService.On("PlaceOrder", mock.AnythingOfType("*gin.Context"), mock.AnythingOfType("*models.Order")).
			Return(nil, fmt.Errorf("insufficient balance"))

		reqBody := map[string]interface{}{
			"symbol":        "BTCUSDT",
			"side":          "buy",
			"type":          "limit",
			"quantity":      "0.001",
			"price":         "50000.00",
			"time_in_force": "GTC",
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/v1/trading/orders", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		suite.router.ServeHTTP(w, req)

		assert.Equal(suite.T(), http.StatusForbidden, w.Code)
		suite.mockService.AssertExpectations(suite.T())
	})
}

// Test Get Orders Handler
func (suite *TradingHandlerTestSuite) TestGetOrdersHandler() {
	suite.Run("ValidRequest", func() {
		orders := []*models.Order{
			{
				ID:     uuid.New(),
				Symbol: "BTCUSDT",
				Side:   "buy",
				Type:   "limit",
				Status: "open",
			},
		}

		suite.mockService.On("GetOrders", "123e4567-e89b-12d3-a456-426614174000", "", "", "50", "0").
			Return(orders, int64(1), nil)

		req := httptest.NewRequest("GET", "/v1/trading/orders", nil)
		w := httptest.NewRecorder()
		suite.router.ServeHTTP(w, req)

		assert.Equal(suite.T(), http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(suite.T(), err)
		assert.Contains(suite.T(), response, "orders")
		assert.Contains(suite.T(), response, "pagination")

		suite.mockService.AssertExpectations(suite.T())
	})

	suite.Run("WithFilters", func() {
		orders := []*models.Order{}

		suite.mockService.On("GetOrders", "123e4567-e89b-12d3-a456-426614174000", "BTCUSDT", "open", "50", "0").
			Return(orders, int64(0), nil)

		req := httptest.NewRequest("GET", "/v1/trading/orders?symbol=BTCUSDT&status=open", nil)
		w := httptest.NewRecorder()
		suite.router.ServeHTTP(w, req)

		assert.Equal(suite.T(), http.StatusOK, w.Code)
		suite.mockService.AssertExpectations(suite.T())
	})
}

// Test Get Order Handler
func (suite *TradingHandlerTestSuite) TestGetOrderHandler() {
	orderID := uuid.New().String()

	suite.Run("ValidRequest", func() {
		order := &models.Order{
			ID:     uuid.MustParse(orderID),
			Symbol: "BTCUSDT",
			Side:   "buy",
			Type:   "limit",
			Status: "open",
		}

		suite.mockService.On("GetOrder", orderID).Return(order, nil)

		req := httptest.NewRequest("GET", fmt.Sprintf("/v1/trading/orders/%s", orderID), nil)
		w := httptest.NewRecorder()
		suite.router.ServeHTTP(w, req)

		assert.Equal(suite.T(), http.StatusOK, w.Code)
		suite.mockService.AssertExpectations(suite.T())
	})

	suite.Run("OrderNotFound", func() {
		suite.mockService.On("GetOrder", orderID).Return(nil, fmt.Errorf("order not found"))

		req := httptest.NewRequest("GET", fmt.Sprintf("/v1/trading/orders/%s", orderID), nil)
		w := httptest.NewRecorder()
		suite.router.ServeHTTP(w, req)

		assert.Equal(suite.T(), http.StatusNotFound, w.Code)
		suite.mockService.AssertExpectations(suite.T())
	})

	suite.Run("InvalidOrderID", func() {
		req := httptest.NewRequest("GET", "/v1/trading/orders/invalid-uuid", nil)
		w := httptest.NewRecorder()
		suite.router.ServeHTTP(w, req)

		assert.Equal(suite.T(), http.StatusBadRequest, w.Code)
	})
}

// Test Cancel Order Handler
func (suite *TradingHandlerTestSuite) TestCancelOrderHandler() {
	orderID := uuid.New().String()

	suite.Run("ValidRequest", func() {
		suite.mockService.On("CancelOrder", mock.AnythingOfType("*gin.Context"), orderID).Return(nil)

		req := httptest.NewRequest("DELETE", fmt.Sprintf("/v1/trading/orders/%s", orderID), nil)
		w := httptest.NewRecorder()
		suite.router.ServeHTTP(w, req)

		assert.Equal(suite.T(), http.StatusOK, w.Code)
		suite.mockService.AssertExpectations(suite.T())
	})

	suite.Run("OrderNotFound", func() {
		suite.mockService.On("CancelOrder", mock.AnythingOfType("*gin.Context"), orderID).
			Return(fmt.Errorf("order not found"))

		req := httptest.NewRequest("DELETE", fmt.Sprintf("/v1/trading/orders/%s", orderID), nil)
		w := httptest.NewRecorder()
		suite.router.ServeHTTP(w, req)

		assert.Equal(suite.T(), http.StatusNotFound, w.Code)
		suite.mockService.AssertExpectations(suite.T())
	})

	suite.Run("CannotBeCancelled", func() {
		suite.mockService.On("CancelOrder", mock.AnythingOfType("*gin.Context"), orderID).
			Return(fmt.Errorf("order cannot be cancelled"))

		req := httptest.NewRequest("DELETE", fmt.Sprintf("/v1/trading/orders/%s", orderID), nil)
		w := httptest.NewRecorder()
		suite.router.ServeHTTP(w, req)

		assert.Equal(suite.T(), http.StatusBadRequest, w.Code)
		suite.mockService.AssertExpectations(suite.T())
	})
}

// Test Market Data Handlers
func (suite *TradingHandlerTestSuite) TestGetOrderBookHandler() {
	suite.Run("ValidRequest", func() {
		orderBook := &models.OrderBookSnapshot{
			Symbol: "BTCUSDT",
			Bids: []models.OrderBookLevel{
				{Price: 49999.99, Quantity: 0.5},
				{Price: 49999.98, Quantity: 1.0},
			},
			Asks: []models.OrderBookLevel{
				{Price: 50000.01, Quantity: 0.3},
				{Price: 50000.02, Quantity: 0.8},
			},
			LastUpdateID: 123456,
		}

		suite.mockService.On("GetOrderBook", "BTCUSDT", 100).Return(orderBook, nil)

		req := httptest.NewRequest("GET", "/api/v1/depth/BTCUSDT", nil)
		w := httptest.NewRecorder()
		suite.router.ServeHTTP(w, req)

		assert.Equal(suite.T(), http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(suite.T(), err)
		assert.Equal(suite.T(), "BTCUSDT", response["symbol"])
		assert.Contains(suite.T(), response, "bids")
		assert.Contains(suite.T(), response, "asks")

		suite.mockService.AssertExpectations(suite.T())
	})

	suite.Run("WithCustomLimit", func() {
		orderBook := &models.OrderBookSnapshot{
			Symbol: "BTCUSDT",
			Bids:   []models.OrderBookLevel{},
			Asks:   []models.OrderBookLevel{},
		}

		suite.mockService.On("GetOrderBook", "BTCUSDT", 10).Return(orderBook, nil)

		req := httptest.NewRequest("GET", "/api/v1/depth/BTCUSDT?limit=10", nil)
		w := httptest.NewRecorder()
		suite.router.ServeHTTP(w, req)

		assert.Equal(suite.T(), http.StatusOK, w.Code)
		suite.mockService.AssertExpectations(suite.T())
	})

	suite.Run("InvalidLimit", func() {
		req := httptest.NewRequest("GET", "/api/v1/depth/BTCUSDT?limit=9999", nil)
		w := httptest.NewRecorder()
		suite.router.ServeHTTP(w, req)

		assert.Equal(suite.T(), http.StatusBadRequest, w.Code)
	})

	suite.Run("SymbolNotFound", func() {
		suite.mockService.On("GetOrderBook", "INVALID", 100).
			Return(nil, fmt.Errorf("trading pair not found"))

		req := httptest.NewRequest("GET", "/api/v1/depth/INVALID", nil)
		w := httptest.NewRecorder()
		suite.router.ServeHTTP(w, req)

		assert.Equal(suite.T(), http.StatusNotFound, w.Code)
		suite.mockService.AssertExpectations(suite.T())
	})
}

func (suite *TradingHandlerTestSuite) TestGetTradesHandler() {
	suite.Run("ValidRequest", func() {
		req := httptest.NewRequest("GET", "/api/v1/trades/BTCUSDT", nil)
		w := httptest.NewRecorder()
		suite.router.ServeHTTP(w, req)

		// Currently returns empty array, should not error
		assert.Equal(suite.T(), http.StatusOK, w.Code)
	})

	suite.Run("WithLimit", func() {
		req := httptest.NewRequest("GET", "/api/v1/trades/BTCUSDT?limit=100", nil)
		w := httptest.NewRecorder()
		suite.router.ServeHTTP(w, req)

		assert.Equal(suite.T(), http.StatusOK, w.Code)
	})

	suite.Run("InvalidLimit", func() {
		req := httptest.NewRequest("GET", "/api/v1/trades/BTCUSDT?limit=2000", nil)
		w := httptest.NewRecorder()
		suite.router.ServeHTTP(w, req)

		assert.Equal(suite.T(), http.StatusBadRequest, w.Code)
	})
}

// Performance Benchmarks for Handlers
func BenchmarkPlaceOrderHandler(b *testing.B) {
	logger := zaptest.NewLogger(b)
	mockService := new(MockTradingService)
	handler := handlers.NewTradingHandler(mockService, logger)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", "123e4567-e89b-12d3-a456-426614174000")
		c.Next()
	})
	router.POST("/orders", handler.PlaceOrder)

	expectedOrder := &models.Order{
		ID:          uuid.New(),
		Symbol:      "BTCUSDT",
		Side:        "buy",
		Type:        "limit",
		Status:      "NEW",
		TimeInForce: "GTC",
	}

	mockService.On("PlaceOrder", mock.Anything, mock.Anything).Return(expectedOrder, nil)

	reqBody := map[string]interface{}{
		"symbol":        "BTCUSDT",
		"side":          "buy",
		"type":          "limit",
		"quantity":      "0.001",
		"price":         "50000.00",
		"time_in_force": "GTC",
	}
	body, _ := json.Marshal(reqBody)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := httptest.NewRequest("POST", "/orders", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
		}
	})
}

func BenchmarkGetOrderBookHandler(b *testing.B) {
	logger := zaptest.NewLogger(b)
	mockService := new(MockTradingService)
	handler := handlers.NewMarketDataHandler(mockService, logger)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/depth/:symbol", handler.GetOrderBook)

	orderBook := &models.OrderBookSnapshot{
		Symbol: "BTCUSDT",
		Bids:   []models.OrderBookLevel{{Price: 50000, Quantity: 1.0}},
		Asks:   []models.OrderBookLevel{{Price: 50001, Quantity: 1.0}},
	}

	mockService.On("GetOrderBook", "BTCUSDT", 100).Return(orderBook, nil)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := httptest.NewRequest("GET", "/depth/BTCUSDT", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
		}
	})
}
