//go:build trading

package test

import (
	"sync"
	"time"

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
	UserID      string        // For stress test compatibility
	Messages    []WSMessage
	MessagesRaw [][]byte     // For stress test compatibility
	LastPing    time.Time
	IsConnected bool
	IsActive    bool         // For stress test compatibility
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
