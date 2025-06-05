//go:build trading
// +build trading

package test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Aidin1998/finalex/pkg/models"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// Common test types used across multiple test files

// WSMessage represents a WebSocket message for testing
type WSMessage struct {
	Type   string      `json:"type"`
	Data   interface{} `json:"data"`
	Topic  string
	UserID string
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

// Start implements bookkeeper.BookkeeperService
func (m *MockBookkeeperStressTest) Start() error {
	return nil
}

// Stop implements bookkeeper.BookkeeperService
func (m *MockBookkeeperStressTest) Stop() error {
	return nil
}

// GetAccounts implements bookkeeper.BookkeeperService
func (m *MockBookkeeperStressTest) GetAccounts(ctx context.Context, userIDs []uuid.UUID) ([]*models.Account, error) {
	accounts := make([]*models.Account, 0, len(userIDs))
	for _, userID := range userIDs {
		account := &models.Account{
			ID:     userID,
			UserID: userID,
		}
		accounts = append(accounts, account)
	}
	return accounts, nil
}

// BatchGetAccounts implements bookkeeper.BookkeeperService
func (m *MockBookkeeperStressTest) BatchGetAccounts(ctx context.Context, userIDs []string, currencies []string) (map[string]map[string]*models.Account, error) {
	result := make(map[string]map[string]*models.Account)
	for _, userID := range userIDs {
		userAccounts := make(map[string]*models.Account)
		for _, currency := range currencies {
			userAccounts[currency] = &models.Account{
				ID:     uuid.New(),
				UserID: uuid.MustParse(userID),
			}
		}
		result[userID] = userAccounts
	}
	return result, nil
}

// GetAccount implements bookkeeper.BookkeeperService
func (m *MockBookkeeperStressTest) GetAccount(ctx context.Context, userID uuid.UUID) (*models.Account, error) {
	return &models.Account{
		ID:     userID,
		UserID: userID,
	}, nil
}

// CreateAccount implements bookkeeper.BookkeeperService
func (m *MockBookkeeperStressTest) CreateAccount(ctx context.Context, account *models.Account) (*models.Account, error) {
	return account, nil
}

// UpdateAccount implements bookkeeper.BookkeeperService
func (m *MockBookkeeperStressTest) UpdateAccount(ctx context.Context, account *models.Account) (*models.Account, error) {
	return account, nil
}

// DeleteAccount implements bookkeeper.BookkeeperService
func (m *MockBookkeeperStressTest) DeleteAccount(ctx context.Context, userID uuid.UUID) error {
	return nil
}

// GetAccountBalance implements bookkeeper.BookkeeperService
func (m *MockBookkeeperStressTest) GetAccountBalance(ctx context.Context, userID uuid.UUID, asset string) (decimal.Decimal, error) {
	return m.GetBalance(userID.String(), asset)
}

// GetAccountBalances implements bookkeeper.BookkeeperService
func (m *MockBookkeeperStressTest) GetAccountBalances(ctx context.Context, userID uuid.UUID) (map[string]decimal.Decimal, error) {
	userBalances, exists := m.balances.Load(userID.String())
	if !exists {
		return make(map[string]decimal.Decimal), nil
	}
	return userBalances.(map[string]decimal.Decimal), nil
}

// SetAccountBalance implements bookkeeper.BookkeeperService
func (m *MockBookkeeperStressTest) SetAccountBalance(ctx context.Context, userID uuid.UUID, asset string, balance decimal.Decimal) error {
	m.SetBalance(userID.String(), asset, balance)
	return nil
}

// Transfer implements bookkeeper.BookkeeperService
func (m *MockBookkeeperStressTest) Transfer(ctx context.Context, fromUserID, toUserID uuid.UUID, asset string, amount decimal.Decimal) error {
	return nil
}

// Reserve implements bookkeeper.BookkeeperService
func (m *MockBookkeeperStressTest) Reserve(ctx context.Context, userID uuid.UUID, asset string, amount decimal.Decimal) (string, error) {
	return m.ReserveBalance(userID.String(), asset, amount)
}

// CommitReservation implements bookkeeper.BookkeeperService
func (m *MockBookkeeperStressTest) CommitReservation(ctx context.Context, reservationID string) error {
	atomic.AddInt64(&m.totalOps, 1)
	m.reservations.Delete(reservationID)
	return nil
}

// ReleaseReservation implements bookkeeper.BookkeeperService
func (m *MockBookkeeperStressTest) ReleaseReservation(ctx context.Context, reservationID string) error {
	atomic.AddInt64(&m.totalOps, 1)
	m.reservations.Delete(reservationID)
	return nil
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
		Amount: amount}
	m.reservations.Store(reservationID, reservation)
	return reservationID, nil
}

type MockWSHubStressTest struct {
	connections sync.Map // userID -> *MockConnection
	broadcasts  sync.Map // topic -> [][]byte
	totalOps    int64
	mu          sync.RWMutex
}

// Start implements ws.Hub
func (m *MockWSHubStressTest) Start() error {
	return nil
}

// Stop implements ws.Hub
func (m *MockWSHubStressTest) Stop() error {
	return nil
}

// Subscribe implements ws.Hub
func (m *MockWSHubStressTest) Subscribe(userID string, topic string) error {
	return nil
}

// Unsubscribe implements ws.Hub
func (m *MockWSHubStressTest) Unsubscribe(userID string, topic string) error {
	return nil
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

// --- Begin: Base Mock Types ---

// MockBookkeeper provides base mock implementation for bookkeeper service
type MockBookkeeper struct {
	balances     sync.Map // userID -> map[asset]decimal.Decimal
	reservations sync.Map // reservationID -> ReservationInfo
	mu           sync.RWMutex
}

func NewMockBookkeeper() *MockBookkeeper {
	return &MockBookkeeper{}
}

// Start implements bookkeeper.BookkeeperService
func (m *MockBookkeeper) Start() error {
	return nil
}

// Stop implements bookkeeper.BookkeeperService
func (m *MockBookkeeper) Stop() error {
	return nil
}

// GetAccounts implements bookkeeper.BookkeeperService
func (m *MockBookkeeper) GetAccounts(ctx context.Context, userIDs []uuid.UUID) ([]*models.Account, error) {
	accounts := make([]*models.Account, 0, len(userIDs))
	for _, userID := range userIDs {
		account := &models.Account{
			ID:     userID,
			UserID: userID,
		}
		accounts = append(accounts, account)
	}
	return accounts, nil
}

// BatchGetAccounts implements bookkeeper.BookkeeperService
func (m *MockBookkeeper) BatchGetAccounts(ctx context.Context, userIDs []string, currencies []string) (map[string]map[string]*models.Account, error) {
	result := make(map[string]map[string]*models.Account)
	for _, userID := range userIDs {
		userAccounts := make(map[string]*models.Account)
		for _, currency := range currencies {
			userAccounts[currency] = &models.Account{
				ID:     uuid.New(),
				UserID: uuid.MustParse(userID),
			}
		}
		result[userID] = userAccounts
	}
	return result, nil
}

// GetAccount implements bookkeeper.BookkeeperService
func (m *MockBookkeeper) GetAccount(ctx context.Context, userID uuid.UUID) (*models.Account, error) {
	return &models.Account{
		ID:     userID,
		UserID: userID,
	}, nil
}

// CreateAccount implements bookkeeper.BookkeeperService
func (m *MockBookkeeper) CreateAccount(ctx context.Context, account *models.Account) (*models.Account, error) {
	return account, nil
}

// UpdateAccount implements bookkeeper.BookkeeperService
func (m *MockBookkeeper) UpdateAccount(ctx context.Context, account *models.Account) (*models.Account, error) {
	return account, nil
}

// DeleteAccount implements bookkeeper.BookkeeperService
func (m *MockBookkeeper) DeleteAccount(ctx context.Context, userID uuid.UUID) error {
	return nil
}

// GetAccountBalance implements bookkeeper.BookkeeperService
func (m *MockBookkeeper) GetAccountBalance(ctx context.Context, userID uuid.UUID, asset string) (decimal.Decimal, error) {
	return m.GetBalance(userID.String(), asset)
}

// GetAccountBalances implements bookkeeper.BookkeeperService
func (m *MockBookkeeper) GetAccountBalances(ctx context.Context, userID uuid.UUID) (map[string]decimal.Decimal, error) {
	userBalances, exists := m.balances.Load(userID.String())
	if !exists {
		return make(map[string]decimal.Decimal), nil
	}
	return userBalances.(map[string]decimal.Decimal), nil
}

// SetAccountBalance implements bookkeeper.BookkeeperService
func (m *MockBookkeeper) SetAccountBalance(ctx context.Context, userID uuid.UUID, asset string, balance decimal.Decimal) error {
	m.SetBalance(userID.String(), asset, balance)
	return nil
}

// Transfer implements bookkeeper.BookkeeperService
func (m *MockBookkeeper) Transfer(ctx context.Context, fromUserID, toUserID uuid.UUID, asset string, amount decimal.Decimal) error {
	return nil
}

// Reserve implements bookkeeper.BookkeeperService
func (m *MockBookkeeper) Reserve(ctx context.Context, userID uuid.UUID, asset string, amount decimal.Decimal) (string, error) {
	reservationID := uuid.New().String()
	reservation := ReservationInfo{
		UserID: userID.String(),
		Asset:  asset,
		Amount: amount,
	}
	m.reservations.Store(reservationID, reservation)
	return reservationID, nil
}

// CommitReservation implements bookkeeper.BookkeeperService
func (m *MockBookkeeper) CommitReservation(ctx context.Context, reservationID string) error {
	m.reservations.Delete(reservationID)
	return nil
}

// ReleaseReservation implements bookkeeper.BookkeeperService
func (m *MockBookkeeper) ReleaseReservation(ctx context.Context, reservationID string) error {
	m.reservations.Delete(reservationID)
	return nil
}

// Helper methods for testing
func (m *MockBookkeeper) GetBalance(userID, asset string) (decimal.Decimal, error) {
	userBalances, exists := m.balances.Load(userID)
	if !exists {
		return decimal.NewFromInt(10000), nil // Default test balance
	}
	balanceMap := userBalances.(map[string]decimal.Decimal)
	balance, exists := balanceMap[asset]
	if !exists {
		return decimal.NewFromInt(10000), nil // Default test balance
	}
	return balance, nil
}

func (m *MockBookkeeper) SetBalance(userID, asset string, amount decimal.Decimal) {
	userBalances, _ := m.balances.LoadOrStore(userID, make(map[string]decimal.Decimal))
	balanceMap := userBalances.(map[string]decimal.Decimal)
	balanceMap[asset] = amount
	m.balances.Store(userID, balanceMap)
}

// MockWSHub provides base mock implementation for WebSocket hub
type MockWSHub struct {
	messages sync.Map // topic -> [][]byte
	users    sync.Map // userID -> [][]byte
	mu       sync.RWMutex
}

func NewMockWSHub() *MockWSHub {
	return &MockWSHub{}
}

// Start implements ws.Hub
func (m *MockWSHub) Start() error {
	return nil
}

// Stop implements ws.Hub
func (m *MockWSHub) Stop() error {
	return nil
}

// Subscribe implements ws.Hub
func (m *MockWSHub) Subscribe(userID string, topic string) error {
	return nil
}

// Unsubscribe implements ws.Hub
func (m *MockWSHub) Unsubscribe(userID string, topic string) error {
	return nil
}

// Broadcast implements ws.Hub
func (m *MockWSHub) Broadcast(topic string, data []byte) {
	topicMessages, _ := m.messages.LoadOrStore(topic, make([][]byte, 0))
	messages := topicMessages.([][]byte)
	messages = append(messages, data)
	m.messages.Store(topic, messages)
}

// BroadcastToUser implements ws.Hub
func (m *MockWSHub) BroadcastToUser(userID string, data []byte) {
	userMessages, _ := m.users.LoadOrStore(userID, make([][]byte, 0))
	messages := userMessages.([][]byte)
	messages = append(messages, data)
	m.users.Store(userID, messages)
}

// Helper methods for testing
func (m *MockWSHub) GetMessageQueue() []WSMessage {
	var result []WSMessage
	m.messages.Range(func(key, value interface{}) bool {
		topic := key.(string)
		messages := value.([][]byte)
		for _, msg := range messages {
			result = append(result, WSMessage{
				Type:  "broadcast",
				Topic: topic,
				Data:  string(msg),
			})
		}
		return true
	})
	return result
}

// --- End: Base Mock Types ---

// createInMemoryDB creates an in-memory database for testing
func createInMemoryDB(t testing.TB) *gorm.DB {
	// For testing purposes, return nil to use mock services
	return nil
}
