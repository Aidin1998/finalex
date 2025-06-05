//go:build trading

package test

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

// TradingWebSocketTestSuite tests WebSocket integration and real-time messaging
type TradingWebSocketTestSuite struct {
	suite.Suite
	wsHub  *MockWebSocketHub
	logger *zap.Logger
}

// MockWebSocketHub provides comprehensive WebSocket testing capabilities
type MockWebSocketHub struct {
	connections     sync.Map // userID -> *MockWSConnection
	subscriptions   sync.Map // topic -> []userID
	broadcasts      sync.Map // topic -> []message
	messageCount    int64
	connectionCount int64
	mu              sync.RWMutex
}

type MockWSConnection struct {
	UserID        string
	Connected     bool
	Messages      [][]byte
	Subscriptions []string
	LastActivity  time.Time
	mu            sync.RWMutex
}

func NewMockWebSocketHub() *MockWebSocketHub {
	return &MockWebSocketHub{}
}

func (h *MockWebSocketHub) Broadcast(topic string, data []byte) {
	atomic.AddInt64(&h.messageCount, 1)

	// Store broadcast for verification
	if messages, ok := h.broadcasts.Load(topic); ok {
		msgList := messages.([][]byte)
		msgList = append(msgList, data)
		h.broadcasts.Store(topic, msgList)
	} else {
		h.broadcasts.Store(topic, [][]byte{data})
	}

	// Send to subscribed users
	if userIDs, ok := h.subscriptions.Load(topic); ok {
		for _, userID := range userIDs.([]string) {
			if conn, ok := h.connections.Load(userID); ok {
				mockConn := conn.(*MockWSConnection)
				mockConn.mu.Lock()
				if mockConn.Connected {
					mockConn.Messages = append(mockConn.Messages, data)
					mockConn.LastActivity = time.Now()
				}
				mockConn.mu.Unlock()
			}
		}
	}
}

func (h *MockWebSocketHub) BroadcastToUser(userID string, data []byte) {
	atomic.AddInt64(&h.messageCount, 1)

	if conn, ok := h.connections.Load(userID); ok {
		mockConn := conn.(*MockWSConnection)
		mockConn.mu.Lock()
		if mockConn.Connected {
			mockConn.Messages = append(mockConn.Messages, data)
			mockConn.LastActivity = time.Now()
		}
		mockConn.mu.Unlock()
	}
}

func (h *MockWebSocketHub) Subscribe(userID, topic string) error {
	// Ensure user connection exists
	if _, ok := h.connections.Load(userID); !ok {
		conn := &MockWSConnection{
			UserID:        userID,
			Connected:     true,
			Messages:      make([][]byte, 0),
			Subscriptions: make([]string, 0),
			LastActivity:  time.Now(),
		}
		h.connections.Store(userID, conn)
		atomic.AddInt64(&h.connectionCount, 1)
	}

	// Add subscription
	conn, _ := h.connections.Load(userID)
	mockConn := conn.(*MockWSConnection)
	mockConn.mu.Lock()
	mockConn.Subscriptions = append(mockConn.Subscriptions, topic)
	mockConn.mu.Unlock()

	// Update topic subscriptions
	h.mu.Lock()
	defer h.mu.Unlock()

	if userIDs, ok := h.subscriptions.Load(topic); ok {
		userList := userIDs.([]string)
		userList = append(userList, userID)
		h.subscriptions.Store(topic, userList)
	} else {
		h.subscriptions.Store(topic, []string{userID})
	}

	return nil
}

func (h *MockWebSocketHub) Unsubscribe(userID, topic string) error {
	// Remove from topic subscriptions
	h.mu.Lock()
	defer h.mu.Unlock()

	if userIDs, ok := h.subscriptions.Load(topic); ok {
		userList := userIDs.([]string)
		for i, id := range userList {
			if id == userID {
				userList = append(userList[:i], userList[i+1:]...)
				h.subscriptions.Store(topic, userList)
				break
			}
		}
	}

	// Remove from user subscriptions
	if conn, ok := h.connections.Load(userID); ok {
		mockConn := conn.(*MockWSConnection)
		mockConn.mu.Lock()
		for i, sub := range mockConn.Subscriptions {
			if sub == topic {
				mockConn.Subscriptions = append(mockConn.Subscriptions[:i], mockConn.Subscriptions[i+1:]...)
				break
			}
		}
		mockConn.mu.Unlock()
	}

	return nil
}

func (h *MockWebSocketHub) Connect(userID string) {
	conn := &MockWSConnection{
		UserID:        userID,
		Connected:     true,
		Messages:      [][]byte{},
		Subscriptions: []string{},
		LastActivity:  time.Now(),
	}
	h.connections.Store(userID, conn)
	atomic.AddInt64(&h.connectionCount, 1)
}

func (h *MockWebSocketHub) Disconnect(userID string) {
	conn := &MockWSConnection{
		UserID:    userID,
		Connected: false,
	}
	h.connections.Store(userID, conn)
	atomic.AddInt64(&h.connectionCount, -1)
}

func (h *MockWebSocketHub) GetSubscribers(topic string) []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	subs, ok := h.subscriptions.Load(topic)
	if !ok {
		return []string{}
	}

	subscribers := subs.([]string)
	for i, userID := range subscribers {
		if conn, ok := h.connections.Load(userID); ok {
			mockConn := conn.(*MockWSConnection)
			if !mockConn.Connected {
				subscribers = append(subscribers[:i], subscribers[i+1:]...)
			}
		}
	}

	return subscribers
}

func (h *MockWebSocketHub) SendToUser(userID string, data []byte) {
	conn, ok := h.connections.Load(userID)
	if !ok {
		return
	}

	mockConn := conn.(*MockWSConnection)
	mockConn.mu.Lock()
	defer mockConn.mu.Unlock()
	mockConn.Messages = append(mockConn.Messages, data)
	mockConn.LastActivity = time.Now()
}

func (h *MockWebSocketHub) GetConnection(userID string) *MockWSConnection {
	if conn, ok := h.connections.Load(userID); ok {
		return conn.(*MockWSConnection)
	}
	return nil
}

func (h *MockWebSocketHub) GetMessageCount() int64 {
	return atomic.LoadInt64(&h.messageCount)
}

func (h *MockWebSocketHub) GetConnectionCount() int64 {
	return atomic.LoadInt64(&h.connectionCount)
}

func (h *MockWebSocketHub) GetBroadcasts(topic string) [][]byte {
	if messages, ok := h.broadcasts.Load(topic); ok {
		return messages.([][]byte)
	}
	return nil
}

func (h *MockWebSocketHub) GetTopicSubscriptionCount(topic string) int {
	if userIDs, ok := h.subscriptions.Load(topic); ok {
		return len(userIDs.([]string))
	}
	return 0
}

func (h *MockWebSocketHub) ClearBroadcasts() {
	h.broadcasts = sync.Map{}
	atomic.StoreInt64(&h.messageCount, 0)
}

func (suite *TradingWebSocketTestSuite) SetupTest() {
	suite.logger = zaptest.NewLogger(suite.T())
	suite.wsHub = NewMockWebSocketHub()
}

func (suite *TradingWebSocketTestSuite) TearDownTest() {
	if suite.wsHub != nil {
		suite.wsHub.ClearBroadcasts()
	}
}

func TestTradingWebSocketTestSuite(t *testing.T) {
	suite.Run(t, new(TradingWebSocketTestSuite))
}

// Test WebSocket Connection Management
func (suite *TradingWebSocketTestSuite) TestConnectionManagement() {
	suite.Run("ConnectUser", func() {
		userID := uuid.New().String()
		suite.wsHub.Connect(userID)

		conn := suite.wsHub.GetConnection(userID)
		suite.NotNil(conn)
		suite.True(conn.Connected)
		suite.Equal(userID, conn.UserID)
		suite.Equal(int64(1), suite.wsHub.GetConnectionCount())
	})

	suite.Run("DisconnectUser", func() {
		userID := uuid.New().String()
		suite.wsHub.Connect(userID)
		suite.wsHub.Disconnect(userID)

		conn := suite.wsHub.GetConnection(userID)
		suite.NotNil(conn)
		suite.False(conn.Connected)
		suite.Equal(int64(0), suite.wsHub.GetConnectionCount())
	})

	suite.Run("MultipleConnections", func() {
		userIDs := make([]string, 100)
		for i := 0; i < 100; i++ {
			userIDs[i] = uuid.New().String()
			suite.wsHub.Connect(userIDs[i])
		}

		suite.Equal(int64(100), suite.wsHub.GetConnectionCount())

		// Verify all connections
		for _, userID := range userIDs {
			conn := suite.wsHub.GetConnection(userID)
			suite.NotNil(conn)
			suite.True(conn.Connected)
		}
	})
}

// Test Subscription Management
func (suite *TradingWebSocketTestSuite) TestSubscriptionManagement() {
	suite.Run("UserSubscription", func() {
		userID := uuid.New().String()
		topic := "orders.btcusdt"

		err := suite.wsHub.Subscribe(userID, topic)
		suite.NoError(err)

		conn := suite.wsHub.GetConnection(userID)
		suite.NotNil(conn)
		suite.Contains(conn.Subscriptions, topic)
		suite.Equal(1, suite.wsHub.GetTopicSubscriptionCount(topic))
	})

	suite.Run("UserUnsubscription", func() {
		userID := uuid.New().String()
		topic := "orders.btcusdt"

		err := suite.wsHub.Subscribe(userID, topic)
		suite.NoError(err)

		err = suite.wsHub.Unsubscribe(userID, topic)
		suite.NoError(err)

		conn := suite.wsHub.GetConnection(userID)
		suite.NotNil(conn)
		suite.NotContains(conn.Subscriptions, topic)
		suite.Equal(0, suite.wsHub.GetTopicSubscriptionCount(topic))
	})

	suite.Run("MultipleSubscriptions", func() {
		userID := uuid.New().String()
		topics := []string{"orders.btcusdt", "trades.btcusdt", "orderbook.btcusdt"}

		for _, topic := range topics {
			err := suite.wsHub.Subscribe(userID, topic)
			suite.NoError(err)
		}

		conn := suite.wsHub.GetConnection(userID)
		suite.NotNil(conn)
		suite.Equal(len(topics), len(conn.Subscriptions))

		for _, topic := range topics {
			suite.Contains(conn.Subscriptions, topic)
			suite.Equal(1, suite.wsHub.GetTopicSubscriptionCount(topic))
		}
	})
}

// Test Broadcasting
func (suite *TradingWebSocketTestSuite) TestBroadcasting() {
	suite.Run("PublicBroadcast", func() {
		topic := "trades.btcusdt"
		message := []byte(`{"symbol":"BTCUSDT","price":"50000","quantity":"0.001"}`)

		// Subscribe multiple users
		userIDs := make([]string, 5)
		for i := 0; i < 5; i++ {
			userIDs[i] = uuid.New().String()
			suite.wsHub.Subscribe(userIDs[i], topic)
		}

		// Broadcast message
		suite.wsHub.Broadcast(topic, message)

		// Verify all subscribed users received message
		for _, userID := range userIDs {
			conn := suite.wsHub.GetConnection(userID)
			suite.NotNil(conn)
			suite.Len(conn.Messages, 1)
			suite.Equal(message, conn.Messages[0])
		}

		// Verify broadcast was stored
		broadcasts := suite.wsHub.GetBroadcasts(topic)
		suite.Len(broadcasts, 1)
		suite.Equal(message, broadcasts[0])
	})

	suite.Run("UserSpecificBroadcast", func() {
		userID := uuid.New().String()
		message := []byte(`{"order_id":"123","status":"filled"}`)

		suite.wsHub.Connect(userID)
		suite.wsHub.BroadcastToUser(userID, message)

		conn := suite.wsHub.GetConnection(userID)
		suite.NotNil(conn)
		suite.Len(conn.Messages, 1)
		suite.Equal(message, conn.Messages[0])
	})

	suite.Run("BroadcastToDisconnectedUser", func() {
		userID := uuid.New().String()
		message := []byte(`{"order_id":"123","status":"filled"}`)

		suite.wsHub.Connect(userID)
		suite.wsHub.Disconnect(userID)
		suite.wsHub.BroadcastToUser(userID, message)

		conn := suite.wsHub.GetConnection(userID)
		suite.NotNil(conn)
		suite.Len(conn.Messages, 0) // Should not receive message when disconnected
	})
}

// Test Real-time Trading Updates
func (suite *TradingWebSocketTestSuite) TestTradingUpdates() {
	suite.Run("OrderUpdates", func() {
		userID := uuid.New().String()
		orderTopic := fmt.Sprintf("orders.%s", userID)

		suite.wsHub.Subscribe(userID, orderTopic)

		// Simulate order placement
		orderPlaced := []byte(`{"type":"order_placed","order_id":"123","status":"pending"}`)
		suite.wsHub.Broadcast(orderTopic, orderPlaced)

		// Simulate order fill
		orderFilled := []byte(`{"type":"order_filled","order_id":"123","status":"filled","filled_qty":"0.001"}`)
		suite.wsHub.Broadcast(orderTopic, orderFilled)

		conn := suite.wsHub.GetConnection(userID)
		suite.NotNil(conn)
		suite.Len(conn.Messages, 2)
		suite.Contains(string(conn.Messages[0]), "order_placed")
		suite.Contains(string(conn.Messages[1]), "order_filled")
	})

	suite.Run("TradeUpdates", func() {
		topic := "trades.btcusdt"
		userIDs := make([]string, 10)

		// Subscribe multiple users to trade feed
		for i := 0; i < 10; i++ {
			userIDs[i] = uuid.New().String()
			suite.wsHub.Subscribe(userIDs[i], topic)
		}

		// Simulate multiple trades
		trades := [][]byte{
			[]byte(`{"symbol":"BTCUSDT","price":"50000","quantity":"0.001","timestamp":"2023-01-01T00:00:00Z"}`),
			[]byte(`{"symbol":"BTCUSDT","price":"50001","quantity":"0.002","timestamp":"2023-01-01T00:00:01Z"}`),
			[]byte(`{"symbol":"BTCUSDT","price":"49999","quantity":"0.0015","timestamp":"2023-01-01T00:00:02Z"}`),
		}

		for _, trade := range trades {
			suite.wsHub.Broadcast(topic, trade)
		}

		// Verify all users received all trades
		for _, userID := range userIDs {
			conn := suite.wsHub.GetConnection(userID)
			suite.NotNil(conn)
			suite.Len(conn.Messages, len(trades))
		}
	})

	suite.Run("OrderBookUpdates", func() {
		topic := "orderbook.btcusdt"

		// Subscribe users to order book updates
		userIDs := make([]string, 20)
		for i := 0; i < 20; i++ {
			userIDs[i] = uuid.New().String()
			suite.wsHub.Subscribe(userIDs[i], topic)
		}

		// Simulate rapid order book updates
		updates := make([][]byte, 100)
		for i := 0; i < 100; i++ {
			price := 50000 + i
			updates[i] = []byte(fmt.Sprintf(`{"symbol":"BTCUSDT","bids":[{"price":"%d","quantity":"0.001"}],"asks":[{"price":"%d","quantity":"0.001"}]}`, price-1, price+1))
		}

		start := time.Now()
		for _, update := range updates {
			suite.wsHub.Broadcast(topic, update)
		}
		elapsed := time.Since(start)

		// Verify performance and delivery
		suite.T().Logf("Order book updates: %d updates in %v", len(updates), elapsed)

		for _, userID := range userIDs {
			conn := suite.wsHub.GetConnection(userID)
			suite.NotNil(conn)
			suite.Len(conn.Messages, len(updates))
		}
	})
}

// Test Concurrent Operations
func (suite *TradingWebSocketTestSuite) TestConcurrentOperations() {
	suite.Run("ConcurrentSubscriptions", func() {
		var wg sync.WaitGroup
		userCount := 100
		topicCount := 10

		topics := make([]string, topicCount)
		for i := 0; i < topicCount; i++ {
			topics[i] = fmt.Sprintf("topic_%d", i)
		}

		// Concurrent subscriptions
		for i := 0; i < userCount; i++ {
			wg.Add(1)
			go func(userIndex int) {
				defer wg.Done()
				userID := fmt.Sprintf("user_%d", userIndex)

				for _, topic := range topics {
					suite.wsHub.Subscribe(userID, topic)
				}
			}(i)
		}

		wg.Wait()

		// Verify subscriptions
		for _, topic := range topics {
			suite.Equal(userCount, suite.wsHub.GetTopicSubscriptionCount(topic))
		}
	})

	suite.Run("ConcurrentBroadcasts", func() {
		topic := "high_frequency"
		userCount := 50
		messageCount := 1000

		// Subscribe users
		userIDs := make([]string, userCount)
		for i := 0; i < userCount; i++ {
			userIDs[i] = fmt.Sprintf("user_%d", i)
			suite.wsHub.Subscribe(userIDs[i], topic)
		}

		var wg sync.WaitGroup
		start := time.Now()

		// Concurrent broadcasts
		for i := 0; i < messageCount; i++ {
			wg.Add(1)
			go func(msgIndex int) {
				defer wg.Done()
				message := []byte(fmt.Sprintf(`{"id":%d,"timestamp":"%d"}`, msgIndex, time.Now().UnixNano()))
				suite.wsHub.Broadcast(topic, message)
			}(i)
		}

		wg.Wait()
		elapsed := time.Since(start)

		messagesPerSecond := float64(messageCount) / elapsed.Seconds()
		suite.T().Logf("Broadcast performance: %d messages in %v (%.2f msg/sec)",
			messageCount, elapsed, messagesPerSecond)

		// Should handle at least 10,000 messages per second
		suite.Greater(messagesPerSecond, 10000.0)

		// Verify message delivery
		broadcasts := suite.wsHub.GetBroadcasts(topic)
		suite.Equal(messageCount, len(broadcasts))
	})

	suite.Run("ConcurrentConnectionManagement", func() {
		var wg sync.WaitGroup
		operationCount := 1000

		for i := 0; i < operationCount; i++ {
			wg.Add(1)
			go func(opIndex int) {
				defer wg.Done()
				userID := fmt.Sprintf("user_%d", opIndex)

				// Connect
				suite.wsHub.Connect(userID)

				// Subscribe to topics
				topics := []string{
					fmt.Sprintf("orders.%s", userID),
					"trades.btcusdt",
					"orderbook.btcusdt",
				}

				for _, topic := range topics {
					suite.wsHub.Subscribe(userID, topic)
				}

				// Send some messages
				for j := 0; j < 5; j++ {
					message := []byte(fmt.Sprintf(`{"user":"%s","message":%d}`, userID, j))
					suite.wsHub.BroadcastToUser(userID, message)
				}

				// Disconnect
				if opIndex%2 == 0 {
					suite.wsHub.Disconnect(userID)
				}
			}(i)
		}

		wg.Wait()

		// Verify final state
		messageCount := suite.wsHub.GetMessageCount()
		suite.Greater(messageCount, int64(operationCount*5)) // At least 5 messages per user
	})
}

// Test Performance Under Load
func (suite *TradingWebSocketTestSuite) TestPerformanceUnderLoad() {
	if testing.Short() {
		suite.T().Skip("Skipping performance tests in short mode")
	}

	suite.Run("HighFrequencyTrading", func() {
		// Simulate high-frequency trading scenario
		userCount := 1000
		messageCount := 100000
		topicCount := 50

		// Create topics for different trading pairs
		topics := make([]string, topicCount)
		for i := 0; i < topicCount; i++ {
			topics[i] = fmt.Sprintf("trades.pair_%d", i)
		}

		// Subscribe users to topics
		userIDs := make([]string, userCount)
		for i := 0; i < userCount; i++ {
			userIDs[i] = fmt.Sprintf("trader_%d", i)
			suite.wsHub.Connect(userIDs[i])

			// Subscribe to 5 random topics
			for j := 0; j < 5; j++ {
				topic := topics[(i*5+j)%topicCount]
				suite.wsHub.Subscribe(userIDs[i], topic)
			}
		}

		var wg sync.WaitGroup
		start := time.Now()

		// Generate high-frequency messages
		for i := 0; i < messageCount; i++ {
			wg.Add(1)
			go func(msgIndex int) {
				defer wg.Done()

				topic := topics[msgIndex%topicCount]
				price := 50000 + (msgIndex % 1000)
				quantity := decimal.NewFromFloat(0.001 + float64(msgIndex%100)*0.0001)

				message := []byte(fmt.Sprintf(`{
					"symbol":"%s",
					"price":"%d",
					"quantity":"%s",
					"timestamp":"%d",
					"trade_id":"%d"
				}`, topic, price, quantity.String(), time.Now().UnixNano(), msgIndex))

				suite.wsHub.Broadcast(topic, message)
			}(i)
		}

		wg.Wait()
		elapsed := time.Since(start)

		messagesPerSecond := float64(messageCount) / elapsed.Seconds()
		suite.T().Logf("High-frequency trading simulation: %d messages to %d users in %v (%.2f msg/sec)",
			messageCount, userCount, elapsed, messagesPerSecond)

		// Should handle at least 50,000 messages per second
		suite.Greater(messagesPerSecond, 50000.0)

		// Verify message counts
		totalMessages := suite.wsHub.GetMessageCount()
		suite.Equal(int64(messageCount), totalMessages)
	})
}

// Test Error Handling
func (suite *TradingWebSocketTestSuite) TestErrorHandling() {
	suite.Run("InvalidSubscriptions", func() {
		userID := uuid.New().String()

		// Test subscription to empty topic
		err := suite.wsHub.Subscribe(userID, "")
		suite.NoError(err) // Should not error, but should handle gracefully

		// Test unsubscription from non-existent topic
		err = suite.wsHub.Unsubscribe(userID, "non_existent_topic")
		suite.NoError(err) // Should not error
	})

	suite.Run("BroadcastToNonExistentTopic", func() {
		message := []byte(`{"test":"message"}`)

		// Should not panic or error
		suite.NotPanics(func() {
			suite.wsHub.Broadcast("non_existent_topic", message)
		})
	})

	suite.Run("DisconnectedUserOperations", func() {
		userID := uuid.New().String()

		suite.wsHub.Connect(userID)
		suite.wsHub.Subscribe(userID, "test_topic")
		suite.wsHub.Disconnect(userID)

		// Operations on disconnected user should not panic
		suite.NotPanics(func() {
			suite.wsHub.BroadcastToUser(userID, []byte(`{"test":"message"}`))
			suite.wsHub.Subscribe(userID, "another_topic")
			suite.wsHub.Unsubscribe(userID, "test_topic")
		})
	})
}

// Test Memory Management
func (suite *TradingWebSocketTestSuite) TestMemoryManagement() {
	suite.Run("MemoryLeakPrevention", func() {
		// Create many connections and subscriptions
		userCount := 10000
		messageCount := 100

		userIDs := make([]string, userCount)
		for i := 0; i < userCount; i++ {
			userIDs[i] = fmt.Sprintf("user_%d", i)
			suite.wsHub.Connect(userIDs[i])
			suite.wsHub.Subscribe(userIDs[i], "memory_test")
		}

		// Send messages
		for i := 0; i < messageCount; i++ {
			message := []byte(fmt.Sprintf(`{"message_id":%d}`, i))
			suite.wsHub.Broadcast("memory_test", message)
		}

		// Disconnect all users
		for _, userID := range userIDs {
			suite.wsHub.Disconnect(userID)
		}

		// Verify cleanup
		suite.Equal(int64(0), suite.wsHub.GetConnectionCount())

		// Clear broadcasts to free memory
		suite.wsHub.ClearBroadcasts()
		suite.Equal(int64(0), suite.wsHub.GetMessageCount())
	})
}

// Benchmark WebSocket operations
func (suite *TradingWebSocketTestSuite) TestWebSocketBenchmarks() {
	if testing.Short() {
		suite.T().Skip("Skipping benchmark tests in short mode")
	}

	suite.Run("BenchmarkBroadcastOperations", func() {
		// Setup
		userCount := 1000
		messageCount := 100000
		topic := "benchmark_topic"

		// Subscribe users
		for i := 0; i < userCount; i++ {
			userID := fmt.Sprintf("user_%d", i)
			suite.wsHub.Subscribe(userID, topic)
		}

		// Benchmark broadcasts
		start := time.Now()

		var wg sync.WaitGroup
		for i := 0; i < messageCount; i++ {
			wg.Add(1)
			go func(msgID int) {
				defer wg.Done()
				message := []byte(fmt.Sprintf(`{"id":%d,"data":"test"}`, msgID))
				suite.wsHub.Broadcast(topic, message)
			}(i)
		}

		wg.Wait()
		elapsed := time.Since(start)

		messagesPerSecond := float64(messageCount) / elapsed.Seconds()
		suite.T().Logf("Broadcast benchmark: %d messages to %d users in %v (%.2f msg/sec)",
			messageCount, userCount, elapsed, messagesPerSecond)

		// Should handle at least 100,000 messages per second
		suite.Greater(messagesPerSecond, 100000.0)
	})
}
