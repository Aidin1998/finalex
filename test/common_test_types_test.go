//go:build trading
// +build trading

package test

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Common test types used across multiple test files

// WSMessage represents a WebSocket message for testing
type WSMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// ReservationInfo holds information about balance reservations for testing
type ReservationInfo struct {
	UserID string
	Asset  string
	Amount decimal.Decimal
}

// MockConnection simulates a WebSocket connection for testing
type MockConnection struct {
	ID          string
	UserID      string // For stress test compatibility
	Messages    []WSMessage
	MessagesRaw [][]byte // For stress test compatibility
	LastPing    time.Time
	IsConnected bool
	IsActive    bool // For stress test compatibility
	mu          sync.RWMutex
}

func (m *MockConnection) WriteMessage(messageType int, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Simulate message sending
	m.Messages = append(m.Messages, WSMessage{
		Type: "test_message",
		Data: string(data),
	})
	return nil
}

func (m *MockConnection) ReadMessage() (messageType int, p []byte, err error) {
	// Simulate message reading
	return 1, []byte(`{"type":"ping"}`), nil
}

func (m *MockConnection) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.IsConnected = false
	return nil
}

func (m *MockConnection) GetMessages() []WSMessage {
	m.mu.RLock()
	defer m.mu.RUnlock()

	messages := make([]WSMessage, len(m.Messages))
	copy(messages, m.Messages)
	return messages
}

// AddRawMessage adds a raw byte message (for stress test compatibility)
func (m *MockConnection) AddRawMessage(data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.MessagesRaw = append(m.MessagesRaw, data)
}

// GetRawMessages returns all raw messages
func (m *MockConnection) GetRawMessages() [][]byte {
	m.mu.RLock()
	defer m.mu.RUnlock()

	messages := make([][]byte, len(m.MessagesRaw))
	copy(messages, m.MessagesRaw)
	return messages
}

// --- Begin: Moved from trading_stress_load_test.go ---

type MockBookkeeperStressTest struct {
	balances     sync.Map // userID -> map[asset]decimal.Decimal
	reservations sync.Map // reservationID -> ReservationInfo
	totalOps     int64
	mu           sync.RWMutex
}

func (m *MockBookkeeperStressTest) GetBalance(userID, asset string) (decimal.Decimal, error) {
	atomic.AddInt64(&m.totalOps, 1)
	userBalances, exists := m.balances.Load(userID)
	if !exists {
		return decimal.Zero, nil
	}
	balanceMap := userBalances.(map[string]decimal.Decimal)
	balance, exists := balanceMap[asset]
	if !exists {
		return decimal.Zero, nil
	}
	return balance, nil
}

func (m *MockBookkeeperStressTest) SetBalance(userID, asset string, amount decimal.Decimal) {
	userBalances, _ := m.balances.LoadOrStore(userID, make(map[string]decimal.Decimal))
	balanceMap := userBalances.(map[string]decimal.Decimal)
	balanceMap[asset] = amount
	m.balances.Store(userID, balanceMap)
}

func (m *MockBookkeeperStressTest) ReserveBalance(userID, asset string, amount decimal.Decimal) (string, error) {
	atomic.AddInt64(&m.totalOps, 1)
	reservationID := uuid.New().String()
	reservation := ReservationInfo{
		UserID: userID,
		Asset:  asset,
		Amount: amount,
	}
	m.reservations.Store(reservationID, reservation)
	return reservationID, nil
}

func (m *MockBookkeeperStressTest) CommitReservation(reservationID string) error {
	atomic.AddInt64(&m.totalOps, 1)
	m.reservations.Delete(reservationID)
	return nil
}

func (m *MockBookkeeperStressTest) ReleaseReservation(reservationID string) error {
	atomic.AddInt64(&m.totalOps, 1)
	m.reservations.Delete(reservationID)
	return nil
}

type MockWSHubStressTest struct {
	connections sync.Map // userID -> *MockConnection
	broadcasts  sync.Map // topic -> [][]byte
	totalOps    int64
	mu          sync.RWMutex
}

func (m *MockWSHubStressTest) Connect(userID string) {
	atomic.AddInt64(&m.totalOps, 1)
	conn := &MockConnection{
		UserID:      userID,
		Messages:    make([]WSMessage, 0),
		MessagesRaw: make([][]byte, 0),
		IsActive:    true,
		IsConnected: true,
	}
	m.connections.Store(userID, conn)
}

func (m *MockWSHubStressTest) Disconnect(userID string) {
	atomic.AddInt64(&m.totalOps, 1)
	if conn, exists := m.connections.Load(userID); exists {
		connection := conn.(*MockConnection)
		connection.mu.Lock()
		connection.IsActive = false
		connection.mu.Unlock()
	}
}

func (m *MockWSHubStressTest) Broadcast(topic string, message []byte) {
	atomic.AddInt64(&m.totalOps, 1)
	topicMessages, _ := m.broadcasts.LoadOrStore(topic, make([][]byte, 0))
	messages := topicMessages.([][]byte)
	messages = append(messages, message)
	m.broadcasts.Store(topic, messages)
}

func (m *MockWSHubStressTest) BroadcastToUser(userID string, message []byte) {
	atomic.AddInt64(&m.totalOps, 1)
	if conn, exists := m.connections.Load(userID); exists {
		connection := conn.(*MockConnection)
		connection.mu.Lock()
		if connection.IsActive {
			connection.MessagesRaw = append(connection.MessagesRaw, message)
		}
		connection.mu.Unlock()
	}
}

func (m *MockWSHubStressTest) GetConnection(userID string) *MockConnection {
	if conn, exists := m.connections.Load(userID); exists {
		return conn.(*MockConnection)
	}
	return nil
}

func (m *MockWSHubStressTest) GetBroadcasts(topic string) [][]byte {
	if messages, exists := m.broadcasts.Load(topic); exists {
		return messages.([][]byte)
	}
	return nil
}

// --- End: Moved from trading_stress_load_test.go ---
