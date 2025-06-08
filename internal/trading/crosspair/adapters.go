package crosspair

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/Aidin1998/finalex/internal/trading/orderbook"
)

// OrderbookAdapter adapts existing orderbook implementations to the CrossPair interface
type OrderbookAdapter struct {
	mu               sync.RWMutex
	orderbookManager OrderbookManager
	subscriptions    map[string][]func(orderbook.OrderBookInterface)
}

// OrderbookManager interface for managing orderbooks
type OrderbookManager interface {
	GetOrderbook(pair string) (orderbook.OrderBookInterface, error)
	Subscribe(pair string, callback func(orderbook.OrderBookInterface)) error
	Unsubscribe(pair string, callback func(orderbook.OrderBookInterface)) error
}

// NewOrderbookAdapter creates a new orderbook adapter
func NewOrderbookAdapter(manager OrderbookManager) *OrderbookAdapter {
	return &OrderbookAdapter{
		orderbookManager: manager,
		subscriptions:    make(map[string][]func(orderbook.OrderBookInterface)),
	}
}

// GetOrderbook implements OrderbookProvider interface
func (a *OrderbookAdapter) GetOrderbook(pair string) (orderbook.OrderBookInterface, error) {
	return a.orderbookManager.GetOrderbook(pair)
}

// SubscribeToUpdates implements OrderbookProvider interface
func (a *OrderbookAdapter) SubscribeToUpdates(pair string, callback func(orderbook.OrderBookInterface)) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Add to our subscription list
	if _, exists := a.subscriptions[pair]; !exists {
		a.subscriptions[pair] = make([]func(orderbook.OrderBookInterface), 0)
	}
	a.subscriptions[pair] = append(a.subscriptions[pair], callback)

	// Subscribe to the underlying orderbook manager
	return a.orderbookManager.Subscribe(pair, callback)
}

// UnsubscribeFromUpdates implements OrderbookProvider interface
func (a *OrderbookAdapter) UnsubscribeFromUpdates(pair string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Remove all callbacks for this pair
	callbacks, exists := a.subscriptions[pair]
	if !exists {
		return nil
	}

	// Unsubscribe each callback from the underlying manager
	for _, callback := range callbacks {
		if err := a.orderbookManager.Unsubscribe(pair, callback); err != nil {
			// Log error but continue with other callbacks
			// In a real implementation, you'd use proper logging
			fmt.Printf("failed to unsubscribe callback for pair %s: %v\n", pair, err)
		}
	}

	delete(a.subscriptions, pair)
	return nil
}

// MatchingEngineAdapter adapts existing matching engines to the CrossPair interface
type MatchingEngineAdapter struct {
	engine interface{}
}

// NewMatchingEngineAdapter creates a new matching engine adapter
func NewMatchingEngineAdapter(engine interface{}) *MatchingEngineAdapter {
	return &MatchingEngineAdapter{
		engine: engine,
	}
}

// Mock implementations for development/testing

// MockOrderbookProvider provides a mock implementation for testing
type MockOrderbookProvider struct {
	orderbooks map[string]orderbook.OrderBookInterface
	callbacks  map[string][]func(orderbook.OrderBookInterface)
	mu         sync.RWMutex
}

// NewMockOrderbookProvider creates a new mock orderbook provider
func NewMockOrderbookProvider() *MockOrderbookProvider {
	return &MockOrderbookProvider{
		orderbooks: make(map[string]orderbook.OrderBookInterface),
		callbacks:  make(map[string][]func(orderbook.OrderBookInterface)),
	}
}

// GetOrderbook returns a mock orderbook
func (m *MockOrderbookProvider) GetOrderbook(pair string) (orderbook.OrderBookInterface, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ob, exists := m.orderbooks[pair]
	if !exists {
		return nil, fmt.Errorf("orderbook not found for pair %s", pair)
	}
	return ob, nil
}

// SubscribeToUpdates adds a callback for orderbook updates
func (m *MockOrderbookProvider) SubscribeToUpdates(pair string, callback func(orderbook.OrderBookInterface)) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.callbacks[pair]; !exists {
		m.callbacks[pair] = make([]func(orderbook.OrderBookInterface), 0)
	}
	m.callbacks[pair] = append(m.callbacks[pair], callback)
	return nil
}

// UnsubscribeFromUpdates removes callbacks for a pair
func (m *MockOrderbookProvider) UnsubscribeFromUpdates(pair string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.callbacks, pair)
	return nil
}

// SetOrderbook sets a mock orderbook for testing
func (m *MockOrderbookProvider) SetOrderbook(pair string, ob orderbook.OrderBookInterface) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.orderbooks[pair] = ob
}

// TriggerUpdate triggers callbacks for testing
func (m *MockOrderbookProvider) TriggerUpdate(pair string) {
	m.mu.RLock()
	callbacks := m.callbacks[pair]
	ob := m.orderbooks[pair]
	m.mu.RUnlock()

	for _, callback := range callbacks {
		if ob != nil {
			callback(ob)
		}
	}
}

// MockEventPublisher provides a mock implementation for testing
type MockEventPublisher struct {
	events []interface{}
	mu     sync.Mutex
}

// NewMockEventPublisher creates a new mock event publisher
func NewMockEventPublisher() *MockEventPublisher {
	return &MockEventPublisher{
		events: make([]interface{}, 0),
	}
}

// PublishOrderCreated implements EventPublisher interface
func (m *MockEventPublisher) PublishOrderCreated(order *CrossPairOrder) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, map[string]interface{}{
		"type":  "order_created",
		"order": order,
	})
	return nil
}

// PublishOrderUpdated implements EventPublisher interface
func (m *MockEventPublisher) PublishOrderUpdated(order *CrossPairOrder) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, map[string]interface{}{
		"type":  "order_updated",
		"order": order,
	})
	return nil
}

// PublishTradeExecuted implements EventPublisher interface
func (m *MockEventPublisher) PublishTradeExecuted(trade *CrossPairTrade) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, map[string]interface{}{
		"type":  "trade_executed",
		"trade": trade,
	})
	return nil
}

// PublishExecutionFailed implements EventPublisher interface
func (m *MockEventPublisher) PublishExecutionFailed(orderID uuid.UUID, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, map[string]interface{}{
		"type":     "execution_failed",
		"order_id": orderID,
		"reason":   reason,
	})
	return nil
}

// GetEvents returns all published events for testing
func (m *MockEventPublisher) GetEvents() []interface{} {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make([]interface{}, len(m.events))
	copy(result, m.events)
	return result
}

// MockMetricsCollector provides a mock implementation for testing
type MockMetricsCollector struct {
	metrics map[string]interface{}
	mu      sync.Mutex
}

// NewMockMetricsCollector creates a new mock metrics collector
func NewMockMetricsCollector() *MockMetricsCollector {
	return &MockMetricsCollector{
		metrics: make(map[string]interface{}),
	}
}

// RecordOrderCreated implements MetricsCollector interface
func (m *MockMetricsCollector) RecordOrderCreated(fromAsset, toAsset string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("orders_created_%s_%s", fromAsset, toAsset)
	if count, exists := m.metrics[key]; exists {
		m.metrics[key] = count.(int) + 1
	} else {
		m.metrics[key] = 1
	}
}

// RecordOrderExecuted implements MetricsCollector interface
func (m *MockMetricsCollector) RecordOrderExecuted(fromAsset, toAsset string, executionTime time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("orders_executed_%s_%s", fromAsset, toAsset)
	if count, exists := m.metrics[key]; exists {
		m.metrics[key] = count.(int) + 1
	} else {
		m.metrics[key] = 1
	}

	timeKey := fmt.Sprintf("execution_time_%s_%s", fromAsset, toAsset)
	m.metrics[timeKey] = executionTime
}

// RecordOrderFailed implements MetricsCollector interface
func (m *MockMetricsCollector) RecordOrderFailed(fromAsset, toAsset string, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("orders_failed_%s_%s", fromAsset, toAsset)
	if count, exists := m.metrics[key]; exists {
		m.metrics[key] = count.(int) + 1
	} else {
		m.metrics[key] = 1
	}
}

// RecordSlippage implements MetricsCollector interface
func (m *MockMetricsCollector) RecordSlippage(fromAsset, toAsset string, slippage decimal.Decimal) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("slippage_%s_%s", fromAsset, toAsset)
	m.metrics[key] = slippage
}

// RecordVolume implements MetricsCollector interface
func (m *MockMetricsCollector) RecordVolume(fromAsset, toAsset string, volume decimal.Decimal) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("volume_%s_%s", fromAsset, toAsset)
	if existing, exists := m.metrics[key]; exists {
		m.metrics[key] = existing.(decimal.Decimal).Add(volume)
	} else {
		m.metrics[key] = volume
	}
}

// GetMetrics returns all metrics for testing
func (m *MockMetricsCollector) GetMetrics() map[string]interface{} {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make(map[string]interface{})
	for k, v := range m.metrics {
		result[k] = v
	}
	return result
}

// In-memory storage implementations for development/testing

// InMemoryOrderStore provides an in-memory implementation of CrossPairOrderStore
type InMemoryOrderStore struct {
	orders map[uuid.UUID]*CrossPairOrder
	mu     sync.RWMutex
}

// NewInMemoryOrderStore creates a new in-memory order store
func NewInMemoryOrderStore() *InMemoryOrderStore {
	return &InMemoryOrderStore{
		orders: make(map[uuid.UUID]*CrossPairOrder),
	}
}

// Create implements CrossPairOrderStore interface
func (s *InMemoryOrderStore) Create(ctx context.Context, order *CrossPairOrder) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Create a copy to avoid external mutations
	orderCopy := *order
	s.orders[order.ID] = &orderCopy
	return nil
}

// Update implements CrossPairOrderStore interface
func (s *InMemoryOrderStore) Update(ctx context.Context, order *CrossPairOrder) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.orders[order.ID]; !exists {
		return fmt.Errorf("order not found: %s", order.ID.String())
	}

	// Create a copy to avoid external mutations
	orderCopy := *order
	s.orders[order.ID] = &orderCopy
	return nil
}

// GetByID implements CrossPairOrderStore interface
func (s *InMemoryOrderStore) GetByID(ctx context.Context, id uuid.UUID) (*CrossPairOrder, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	order, exists := s.orders[id]
	if !exists {
		return nil, fmt.Errorf("order not found: %s", id.String())
	}

	// Return a copy to avoid external mutations
	orderCopy := *order
	return &orderCopy, nil
}

// GetByUserID implements CrossPairOrderStore interface
func (s *InMemoryOrderStore) GetByUserID(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*CrossPairOrder, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var userOrders []*CrossPairOrder
	for _, order := range s.orders {
		if order.UserID == userID {
			orderCopy := *order
			userOrders = append(userOrders, &orderCopy)
		}
	}

	// Apply pagination
	start := offset
	end := offset + limit

	if start >= len(userOrders) {
		return []*CrossPairOrder{}, nil
	}

	if end > len(userOrders) {
		end = len(userOrders)
	}

	return userOrders[start:end], nil
}

// GetActiveOrders implements CrossPairOrderStore interface
func (s *InMemoryOrderStore) GetActiveOrders(ctx context.Context) ([]*CrossPairOrder, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var activeOrders []*CrossPairOrder
	for _, order := range s.orders {
		if order.Status == CrossPairOrderPending || order.Status == CrossPairOrderExecuting {
			orderCopy := *order
			activeOrders = append(activeOrders, &orderCopy)
		}
	}

	return activeOrders, nil
}

// InMemoryTradeStore provides an in-memory implementation of CrossPairTradeStore
type InMemoryTradeStore struct {
	trades map[uuid.UUID]*CrossPairTrade
	mu     sync.RWMutex
}

// NewInMemoryTradeStore creates a new in-memory trade store
func NewInMemoryTradeStore() *InMemoryTradeStore {
	return &InMemoryTradeStore{
		trades: make(map[uuid.UUID]*CrossPairTrade),
	}
}

// Create implements CrossPairTradeStore interface
func (s *InMemoryTradeStore) Create(ctx context.Context, trade *CrossPairTrade) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Create a copy to avoid external mutations
	tradeCopy := *trade
	s.trades[trade.ID] = &tradeCopy
	return nil
}

// GetByOrderID implements CrossPairTradeStore interface
func (s *InMemoryTradeStore) GetByOrderID(ctx context.Context, orderID uuid.UUID) ([]*CrossPairTrade, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var orderTrades []*CrossPairTrade
	for _, trade := range s.trades {
		if trade.OrderID == orderID {
			tradeCopy := *trade
			orderTrades = append(orderTrades, &tradeCopy)
		}
	}

	return orderTrades, nil
}

// GetByUserID implements CrossPairTradeStore interface
func (s *InMemoryTradeStore) GetByUserID(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*CrossPairTrade, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var userTrades []*CrossPairTrade
	for _, trade := range s.trades {
		if trade.UserID == userID {
			tradeCopy := *trade
			userTrades = append(userTrades, &tradeCopy)
		}
	}

	// Apply pagination
	start := offset
	end := offset + limit

	if start >= len(userTrades) {
		return []*CrossPairTrade{}, nil
	}

	if end > len(userTrades) {
		end = len(userTrades)
	}

	return userTrades[start:end], nil
}
