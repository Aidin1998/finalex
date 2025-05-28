package engine

import (
	"context"
	"fmt"
	"hash/fnv"
	"runtime"
	"sync"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/trading/orderbook"
	"github.com/Aidin1998/pincex_unified/pkg/metrics"
	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Engine represents the trading engine
type Engine struct {
	logger       *zap.Logger
	db           *gorm.DB
	orderBooks   map[string]*orderbook.OrderBook
	mutex        sync.RWMutex
	isRunning    bool
	workerCount  int
	orderChans   []chan *orderbook.Order
	resultChan   chan *orderResult
	tradingPairs map[string]*models.TradingPair
}

// orderResult represents the result of an order
type orderResult struct {
	Order  *orderbook.Order
	Trades []*models.Trade
	Error  error
}

// NewEngine creates a new trading engine
func NewEngine(logger *zap.Logger, db *gorm.DB) (*Engine, error) {
	wc := runtime.NumCPU()
	return &Engine{
		logger:       logger,
		db:           db,
		orderBooks:   make(map[string]*orderbook.OrderBook),
		isRunning:    false,
		workerCount:  wc,
		orderChans:   make([]chan *orderbook.Order, 0, wc),
		resultChan:   make(chan *orderResult, 1000),
		tradingPairs: make(map[string]*models.TradingPair),
	}, nil
}

// Start starts the trading engine
func (e *Engine) Start() error {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	if e.isRunning {
		return fmt.Errorf("trading engine is already running")
	}

	// Load trading pairs
	if err := e.loadTradingPairs(); err != nil {
		return fmt.Errorf("failed to load trading pairs: %w", err)
	}

	// Create order books
	for symbol := range e.tradingPairs {
		e.orderBooks[symbol] = orderbook.NewOrderBook(symbol)
	}

	// Initialize and start worker pool for order processing
	for i := 0; i < e.workerCount; i++ {
		ch := make(chan *orderbook.Order, 1000)
		e.orderChans = append(e.orderChans, ch)
		go e.processOrders(ch)
	}

	e.isRunning = true
	e.logger.Info("Trading engine started")

	return nil
}

// Stop stops the trading engine
func (e *Engine) Stop() error {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	if !e.isRunning {
		return fmt.Errorf("trading engine is not running")
	}

	e.isRunning = false
	e.logger.Info("Trading engine stopped")

	return nil
}

// loadTradingPairs loads trading pairs from the database
func (e *Engine) loadTradingPairs() error {
	var pairs []*models.TradingPair
	if err := e.db.Find(&pairs).Error; err != nil {
		return fmt.Errorf("failed to load trading pairs: %w", err)
	}

	for _, pair := range pairs {
		e.tradingPairs[pair.Symbol] = pair
	}

	return nil
}

// processOrders processes orders from a given channel
func (e *Engine) processOrders(ch chan *orderbook.Order) {
	for order := range ch {
		start := time.Now()
		e.mutex.RLock()
		if !e.isRunning {
			e.mutex.RUnlock()
			continue
		}

		// Get order book
		ob, exists := e.orderBooks[order.Symbol]
		if !exists {
			e.resultChan <- &orderResult{
				Order: order,
				Error: fmt.Errorf("order book not found for symbol: %s", order.Symbol),
			}
			e.mutex.RUnlock()
			continue
		}

		// Add order to order book
		trades, err := ob.AddOrder(order)
		e.mutex.RUnlock()

		// Send result
		e.resultChan <- &orderResult{
			Order:  order,
			Trades: trades,
			Error:  err,
		}
		// Record metrics
		metrics.OrdersProcessed.WithLabelValues(order.Side).Inc()
		metrics.OrderLatency.Observe(time.Since(start).Seconds())
	}
}

// PlaceOrder places an order in the trading engine
func (e *Engine) PlaceOrder(ctx context.Context, order *models.Order) (*models.Order, error) {
	e.mutex.RLock()
	if !e.isRunning {
		e.mutex.RUnlock()
		return nil, fmt.Errorf("trading engine is not running")
	}
	e.mutex.RUnlock()

	// Validate order
	if err := e.validateOrder(order); err != nil {
		return nil, err
	}

	// Generate order ID if not provided
	if order.ID == uuid.Nil {
		order.ID = uuid.New()
	}

	// Set order status
	order.Status = "new"
	order.CreatedAt = time.Now()
	order.UpdatedAt = time.Now()

	// Save order to database
	if err := e.db.Create(order).Error; err != nil {
		return nil, fmt.Errorf("failed to save order: %w", err)
	}

	// Create order book order
	obOrder := &orderbook.Order{
		ID:        order.ID.String(),
		UserID:    order.UserID.String(),
		Symbol:    order.Symbol,
		Side:      order.Side,
		Price:     order.Price,
		Quantity:  order.Quantity,
		Timestamp: order.CreatedAt,
	}

	// Dispatch order to a worker channel based on symbol hash
	h := fnv.New32a()
	h.Write([]byte(order.Symbol))
	idx := int(h.Sum32()) % e.workerCount
	e.orderChans[idx] <- obOrder

	// Wait for result
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-e.resultChan:
		if result.Error != nil {
			// Update order status
			order.Status = "rejected"
			order.UpdatedAt = time.Now()
			if err := e.db.Save(order).Error; err != nil {
				e.logger.Error("Failed to update order status", zap.Error(err))
			}
			return nil, result.Error
		}

		// Update order status
		if order.Quantity == result.Order.Quantity {
			order.Status = "filled"
		} else if result.Order.Quantity > 0 {
			order.Status = "partially_filled"
			order.Quantity = result.Order.Quantity
		} else {
			order.Status = "filled"
		}
		order.UpdatedAt = time.Now()

		// Save order to database
		if err := e.db.Save(order).Error; err != nil {
			e.logger.Error("Failed to update order", zap.Error(err))
		}

		// Save trades to database
		for _, trade := range result.Trades {
			if err := e.db.Create(trade).Error; err != nil {
				e.logger.Error("Failed to save trade", zap.Error(err))
			}
		}

		return order, nil
	}
}

// CancelOrder cancels an order in the trading engine
func (e *Engine) CancelOrder(ctx context.Context, orderID string) error {
	e.mutex.RLock()
	if !e.isRunning {
		e.mutex.RUnlock()
		return fmt.Errorf("trading engine is not running")
	}
	e.mutex.RUnlock()

	// Find order
	var order models.Order
	if err := e.db.Where("id = ?", orderID).First(&order).Error; err != nil {
		return fmt.Errorf("order not found: %w", err)
	}

	// Check if order can be canceled
	if order.Status != "new" && order.Status != "partially_filled" {
		return fmt.Errorf("order cannot be canceled: %s", order.Status)
	}

	// Get order book
	e.mutex.RLock()
	ob, exists := e.orderBooks[order.Symbol]
	if !exists {
		e.mutex.RUnlock()
		return fmt.Errorf("order book not found for symbol: %s", order.Symbol)
	}
	e.mutex.RUnlock()

	// Cancel order in order book
	if err := ob.CancelOrder(orderID); err != nil {
		return fmt.Errorf("failed to cancel order: %w", err)
	}

	// Update order status
	order.Status = "canceled"
	order.UpdatedAt = time.Now()

	// Save order to database
	if err := e.db.Save(&order).Error; err != nil {
		return fmt.Errorf("failed to update order: %w", err)
	}

	return nil
}

// GetOrder gets an order from the trading engine
func (e *Engine) GetOrder(orderID string) (*models.Order, error) {
	var order models.Order
	if err := e.db.Where("id = ?", orderID).First(&order).Error; err != nil {
		return nil, fmt.Errorf("order not found: %w", err)
	}

	return &order, nil
}

// GetOrders gets orders from the trading engine
func (e *Engine) GetOrders(userID, symbol, status string, limit, offset int) ([]*models.Order, int64, error) {
	// Build query
	query := e.db.Model(&models.Order{}).Where("user_id = ?", userID)
	if symbol != "" {
		query = query.Where("symbol = ?", symbol)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}

	// Count total
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count orders: %w", err)
	}

	// Get orders
	var orders []*models.Order
	if err := query.Order("created_at DESC").Limit(limit).Offset(offset).Find(&orders).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to get orders: %w", err)
	}

	return orders, total, nil
}

// GetOrderBook gets the order book for a symbol
func (e *Engine) GetOrderBook(symbol string, depth int) (*models.OrderBookSnapshot, error) {
	e.mutex.RLock()
	defer e.mutex.RUnlock()

	// Get order book
	ob, exists := e.orderBooks[symbol]
	if !exists {
		return nil, fmt.Errorf("order book not found for symbol: %s", symbol)
	}

	// Get order book snapshot
	return ob.GetOrderBook(depth)
}

// GetTradingPairs gets all trading pairs
func (e *Engine) GetTradingPairs() ([]*models.TradingPair, error) {
	e.mutex.RLock()
	defer e.mutex.RUnlock()

	// Get trading pairs
	pairs := make([]*models.TradingPair, 0, len(e.tradingPairs))
	for _, pair := range e.tradingPairs {
		pairs = append(pairs, pair)
	}

	return pairs, nil
}

// GetTradingPair gets a trading pair
func (e *Engine) GetTradingPair(symbol string) (*models.TradingPair, error) {
	e.mutex.RLock()
	defer e.mutex.RUnlock()

	// Get trading pair
	pair, exists := e.tradingPairs[symbol]
	if !exists {
		return nil, fmt.Errorf("trading pair not found: %s", symbol)
	}

	return pair, nil
}

// CreateTradingPair creates a trading pair
func (e *Engine) CreateTradingPair(pair *models.TradingPair) (*models.TradingPair, error) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	// Check if trading pair already exists
	if _, exists := e.tradingPairs[pair.Symbol]; exists {
		return nil, fmt.Errorf("trading pair already exists: %s", pair.Symbol)
	}

	// Set trading pair ID and timestamps
	pair.ID = uuid.New()
	pair.CreatedAt = time.Now()
	pair.UpdatedAt = time.Now()

	// Save trading pair to database
	if err := e.db.Create(pair).Error; err != nil {
		return nil, fmt.Errorf("failed to save trading pair: %w", err)
	}

	// Add trading pair to map
	e.tradingPairs[pair.Symbol] = pair

	// Create order book
	e.orderBooks[pair.Symbol] = orderbook.NewOrderBook(pair.Symbol)

	return pair, nil
}

// UpdateTradingPair updates a trading pair
func (e *Engine) UpdateTradingPair(pair *models.TradingPair) (*models.TradingPair, error) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	// Check if trading pair exists
	existingPair, exists := e.tradingPairs[pair.Symbol]
	if !exists {
		return nil, fmt.Errorf("trading pair not found: %s", pair.Symbol)
	}

	// Update trading pair
	existingPair.BaseCurrency = pair.BaseCurrency
	existingPair.QuoteCurrency = pair.QuoteCurrency
	existingPair.PriceDecimals = pair.PriceDecimals
	existingPair.QuantityDecimals = pair.QuantityDecimals
	existingPair.MinQuantity = pair.MinQuantity
	existingPair.MaxQuantity = pair.MaxQuantity
	existingPair.MinPrice = pair.MinPrice
	existingPair.MaxPrice = pair.MaxPrice
	existingPair.Status = pair.Status
	existingPair.UpdatedAt = time.Now()

	// Save trading pair to database
	if err := e.db.Save(existingPair).Error; err != nil {
		return nil, fmt.Errorf("failed to update trading pair: %w", err)
	}

	return existingPair, nil
}

// BatchPlaceOrders processes a batch of orders in a single transaction (stub)
func (e *Engine) BatchPlaceOrders(ctx context.Context, orders []*models.Order) ([]*models.Order, error) {
	tx := e.db.Begin()
	processed := make([]*models.Order, 0, len(orders))
	for _, o := range orders {
		res, err := e.PlaceOrder(ctx, o)
		if err != nil {
			tx.Rollback()
			return nil, err
		}
		processed = append(processed, res)
	}
	if err := tx.Commit().Error; err != nil {
		return nil, err
	}
	return processed, nil
}

// validateOrder ensures order fields are valid
func (e *Engine) validateOrder(order *models.Order) error {
	if order.TimeInForce == "" {
		order.TimeInForce = "GTC"
	}
	// TODO: Add more validation (e.g., price, quantity > 0)
	return nil
}
