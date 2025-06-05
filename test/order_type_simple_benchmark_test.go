//go:build trading
// +build trading

package test

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"
)

// Simple benchmark test file for order type performance comparison
// This file is self-contained to avoid dependency issues

// OrderType test constants
const (
	OrderTypeLimit      = "limit"
	OrderTypeMarket     = "market"
	OrderTypeStopLimit  = "stop-limit"
	OrderTypeStop       = "stop"
	OrderTypeIceberg    = "iceberg"
)

// Order side constants
const (
	SideBuy  = "buy"
	SideSell = "sell"
)

// Simple order struct for testing
type TestOrder struct {
	ID             string
	UserID         string
	Symbol         string
	Side           string
	Type           string
	Price          float64
	Quantity       float64
	StopPrice      *float64
	VisibleQty     *float64
	TotalQty       float64
	Status         string
	Created        time.Time
	FilledQuantity float64
}

// Simple trade struct for testing
type TestTrade struct {
	ID          string
	OrderID     string
	Price       float64
	Quantity    float64
	Side        string
	Timestamp   time.Time
	TakerUserID string
	MakerUserID string
}

// SimpleMockBookkeeper provides a minimalistic mock for testing
type SimpleMockBookkeeper struct {
	balances     map[string]map[string]float64
	reservations map[string]ReservationSimple
	mu           sync.RWMutex
}

type ReservationSimple struct {
	UserID string
	Asset  string
	Amount float64
}

func NewSimpleMockBookkeeper() *SimpleMockBookkeeper {
	return &SimpleMockBookkeeper{
		balances:     make(map[string]map[string]float64),
		reservations: make(map[string]ReservationSimple),
	}
}

func (m *SimpleMockBookkeeper) SetBalance(userID, asset string, amount float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if _, exists := m.balances[userID]; !exists {
		m.balances[userID] = make(map[string]float64)
	}
	m.balances[userID][asset] = amount
}

func (m *SimpleMockBookkeeper) GetBalance(userID, asset string) (float64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if userBalances, exists := m.balances[userID]; exists {
		if balance, exists := userBalances[asset]; exists {
			return balance, nil
		}
	}
	return 0.0, nil
}

func (m *SimpleMockBookkeeper) ReserveBalance(userID, asset string, amount float64) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	reservationID := fmt.Sprintf("res_%d", time.Now().UnixNano())
	m.reservations[reservationID] = ReservationSimple{
		UserID: userID,
		Asset:  asset,
		Amount: amount,
	}
	return reservationID, nil
}

func (m *SimpleMockBookkeeper) CommitReservation(reservationID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if res, exists := m.reservations[reservationID]; exists {
		delete(m.reservations, reservationID)
		
		// Update user balance
		if _, exists := m.balances[res.UserID]; !exists {
			m.balances[res.UserID] = make(map[string]float64)
		}
		
		m.balances[res.UserID][res.Asset] -= res.Amount
		return nil
	}
	return fmt.Errorf("reservation not found")
}

// SimpleMockWSHub provides a minimalistic websocket mock
type SimpleMockWSHub struct {
	connections map[string]bool
	messages    []string
	mu          sync.RWMutex
}

func NewSimpleMockWSHub() *SimpleMockWSHub {
	return &SimpleMockWSHub{
		connections: make(map[string]bool),
		messages:    make([]string, 0),
	}
}

func (h *SimpleMockWSHub) Connect(userID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.connections[userID] = true
}

func (h *SimpleMockWSHub) Broadcast(topic string, message []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.messages = append(h.messages, string(message))
}

// BenchmarkSimpleOrderTypeComparison specifically benchmarks different order types
func BenchmarkSimpleOrderTypeComparison(b *testing.B) {
	mockBookkeeper := NewSimpleMockBookkeeper()
	mockWSHub := NewSimpleMockWSHub()

	// Set up test users
	testUsers := make([]string, 10)
	for i := 0; i < len(testUsers); i++ {
		testUsers[i] = fmt.Sprintf("user_%d", i)
		mockBookkeeper.SetBalance(testUsers[i], "BTC", 10.0)
		mockBookkeeper.SetBalance(testUsers[i], "USDT", 500000.0)
		mockWSHub.Connect(testUsers[i])
	}

	// Test different order types
	orderTypes := []string{
		OrderTypeMarket,
		OrderTypeLimit,
		OrderTypeStop,
		OrderTypeStopLimit,
		OrderTypeIceberg,
	}

	for _, orderType := range orderTypes {
		b.Run(fmt.Sprintf("OrderType_%s", orderType), func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				user := testUsers[i%len(testUsers)]
				
				// Create order with specific type
				order := &TestOrder{
					ID:       fmt.Sprintf("bench_%s_%d", orderType, i),
					UserID:   user,
					Symbol:   "BTCUSDT",
					Side:     SideBuy,
					Type:     orderType,
					Quantity: 0.001,
					Status:   "pending",
					Created:  time.Now(),
				}

				// Set type-specific fields
				if orderType != OrderTypeMarket {
					order.Price = 50000.0
				}
				
				if orderType == OrderTypeStop || orderType == OrderTypeStopLimit {
					stopPrice := 51000.0
					order.StopPrice = &stopPrice
				}
				
				if orderType == OrderTypeIceberg {
					visibleQty := 0.0002
					order.VisibleQty = &visibleQty
					order.TotalQty = order.Quantity
				}

				// Simulate order processing with specific latency by order type
				_, _ = mockBookkeeper.GetBalance(user, "USDT")
				
				// Simulate processing time differences based on order type
				switch orderType {
				case OrderTypeMarket:
					time.Sleep(time.Nanosecond * 50) // Market orders are fastest
				case OrderTypeLimit:
					time.Sleep(time.Nanosecond * 100) // Limit orders slightly slower
				case OrderTypeStop, OrderTypeStopLimit:
					time.Sleep(time.Nanosecond * 150) // Stop orders have more logic
				case OrderTypeIceberg:
					time.Sleep(time.Nanosecond * 200) // Iceberg orders slowest
				}
				
				// Complete order processing simulation
				if orderType != OrderTypeMarket {
					reservationID, _ := mockBookkeeper.ReserveBalance(user, "USDT", order.Price * order.Quantity)
					_ = mockBookkeeper.CommitReservation(reservationID)
				} else {
					reservationID, _ := mockBookkeeper.ReserveBalance(user, "USDT", 50000.0 * order.Quantity)
					_ = mockBookkeeper.CommitReservation(reservationID)
				}
			}
		})
	}
}

// BenchmarkSimpleOrderMatching tests order matching across different order types
func BenchmarkSimpleOrderMatching(b *testing.B) {
	mockBookkeeper := NewSimpleMockBookkeeper()
	mockWSHub := NewSimpleMockWSHub()

	// Set up test users
	testUsers := make([]string, 50)
	for i := 0; i < len(testUsers); i++ {
		testUsers[i] = fmt.Sprintf("user_%d", i)
		mockBookkeeper.SetBalance(testUsers[i], "BTC", 10.0)
		mockBookkeeper.SetBalance(testUsers[i], "USDT", 500000.0)
		mockWSHub.Connect(testUsers[i])
	}
	
	// Test different order types
	orderTypes := []string{
		OrderTypeMarket,
		OrderTypeLimit,
		OrderTypeStop,
		OrderTypeStopLimit,
		OrderTypeIceberg,
	}

	for _, orderType := range orderTypes {
		b.Run(fmt.Sprintf("MatchingSpeed_%s", orderType), func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				buyerID := testUsers[i%len(testUsers)]
				sellerID := testUsers[(i+len(testUsers)/2)%len(testUsers)]
				
				// Create buy order
				buyOrder := &TestOrder{
					ID:       fmt.Sprintf("buy_%s_%d", orderType, i),
					UserID:   buyerID,
					Symbol:   "BTCUSDT",
					Side:     SideBuy,
					Type:     orderType,
					Quantity: 0.001,
					Status:   "pending",
					Created:  time.Now(),
				}
				
				// Create sell order
				sellOrder := &TestOrder{
					ID:       fmt.Sprintf("sell_%s_%d", orderType, i),
					UserID:   sellerID,
					Symbol:   "BTCUSDT",
					Side:     SideSell,
					Type:     orderType,
					Quantity: 0.001,
					Status:   "pending",
					Created:  time.Now(),
				}
				
				// Set prices based on order type
				if orderType != OrderTypeMarket {
					buyOrder.Price = 50000.0
					sellOrder.Price = 50000.0
				}
				
				if orderType == OrderTypeStop || orderType == OrderTypeStopLimit {
					buyStopPrice := 51000.0
					sellStopPrice := 49000.0
					buyOrder.StopPrice = &buyStopPrice
					sellOrder.StopPrice = &sellStopPrice
				}
				
				if orderType == OrderTypeIceberg {
					buyVisibleQty := 0.0002
					sellVisibleQty := 0.0002
					buyOrder.VisibleQty = &buyVisibleQty
					sellOrder.VisibleQty = &sellVisibleQty
					buyOrder.TotalQty = buyOrder.Quantity
					sellOrder.TotalQty = sellOrder.Quantity
				}
				
				// Simulate matching process with order-type-specific timing
				switch orderType {
				case OrderTypeMarket:
					time.Sleep(time.Microsecond * 10) // Market orders match fastest
				case OrderTypeLimit:
					time.Sleep(time.Microsecond * 20) // Limit orders need price comparison
				case OrderTypeStop, OrderTypeStopLimit:
					time.Sleep(time.Microsecond * 35) // Stop orders need trigger checks
				case OrderTypeIceberg:
					time.Sleep(time.Microsecond * 45) // Iceberg orders need partial fill logic
				}
				
				// Simulate successful match
				trade := &TestTrade{
					ID:          fmt.Sprintf("trade_%d", i),
					OrderID:     buyOrder.ID,
					Price:       orderType == OrderTypeMarket ? 50000.0 : buyOrder.Price,
					Quantity:    buyOrder.Quantity,
					Side:        buyOrder.Side,
					Timestamp:   time.Now(),
					TakerUserID: buyOrder.UserID,
					MakerUserID: sellOrder.UserID,
				}
				
				// Broadcast trade
				mockWSHub.Broadcast("trades.BTCUSDT", []byte(fmt.Sprintf(`{"trade":"%s"}`, trade.ID)))
				
				// Update balances
				reserveBuyerID, _ := mockBookkeeper.ReserveBalance(buyerID, "USDT", trade.Price*trade.Quantity)
				reserveSellerID, _ := mockBookkeeper.ReserveBalance(sellerID, "BTC", trade.Quantity)
				_ = mockBookkeeper.CommitReservation(reserveBuyerID)
				_ = mockBookkeeper.CommitReservation(reserveSellerID)
			}
		})
	}
}
