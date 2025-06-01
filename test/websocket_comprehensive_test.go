package test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// WebSocketTestSuite provides comprehensive WebSocket functionality testing
type WebSocketTestSuite struct {
	config      TestConfig
	environment *TestEnvironment
	server      *httptest.Server
	upgrader    websocket.Upgrader
	metrics     PerformanceMetrics
	connections map[string]*websocket.Conn
	mu          sync.RWMutex
}

// WebSocketMessage represents a WebSocket message
type WebSocketMessage struct {
	Type      string                 `json:"type"`
	Channel   string                 `json:"channel,omitempty"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
	ID        string                 `json:"id,omitempty"`
}

// WebSocketSubscription represents a subscription request
type WebSocketSubscription struct {
	Type     string   `json:"type"`
	Channels []string `json:"channels"`
}

// LatencyMeasurement tracks message latency
type LatencyMeasurement struct {
	SentAt     time.Time     `json:"sent_at"`
	ReceivedAt time.Time     `json:"received_at"`
	Latency    time.Duration `json:"latency"`
	MessageID  string        `json:"message_id"`
}

func NewWebSocketTestSuite(config TestConfig) *WebSocketTestSuite {
	suite := &WebSocketTestSuite{
		config:      config,
		environment: NewTestEnvironment(config),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
		connections: make(map[string]*websocket.Conn),
	}

	// Create mock WebSocket server
	suite.server = httptest.NewServer(http.HandlerFunc(suite.handleWebSocket))

	return suite
}

func TestWebSocketLatency(t *testing.T) {
	config := DefaultTestConfig()
	config.ConcurrentOps = 10
	config.MaxDuration = 30 * time.Second

	suite := NewWebSocketTestSuite(config)
	defer suite.server.Close()

	var wg sync.WaitGroup
	var measurements []LatencyMeasurement
	var measurementsMu sync.Mutex

	// Test latency with multiple concurrent connections
	for i := 0; i < config.ConcurrentOps; i++ {
		wg.Add(1)
		go func(clientID int) {
			defer wg.Done()

			// Connect to WebSocket
			url := "ws" + strings.TrimPrefix(suite.server.URL, "http") + "/ws"
			conn, _, err := websocket.DefaultDialer.Dial(url, nil)
			if err != nil {
				t.Errorf("Failed to connect WebSocket client %d: %v", clientID, err)
				return
			}
			defer conn.Close()

			// Subscribe to test channels
			subscription := WebSocketSubscription{
				Type:     "subscribe",
				Channels: []string{"orderbook.BTC-USD", "trades.BTC-USD"},
			}

			if err := conn.WriteJSON(subscription); err != nil {
				t.Errorf("Failed to send subscription: %v", err)
				return
			}

			// Start receiving messages and measure latency
			startTime := time.Now()
			messageCount := 0

			for time.Since(startTime) < config.MaxDuration && messageCount < 100 {
				var msg WebSocketMessage
				err := conn.ReadJSON(&msg)
				if err != nil {
					t.Errorf("Failed to read WebSocket message: %v", err)
					break
				}

				receivedAt := time.Now()
				latency := receivedAt.Sub(msg.Timestamp)

				// Validate latency requirement (< 50ms)
				if latency > 50*time.Millisecond {
					t.Errorf("WebSocket latency too high: %v (expected < 50ms)", latency)
				}

				measurement := LatencyMeasurement{
					SentAt:     msg.Timestamp,
					ReceivedAt: receivedAt,
					Latency:    latency,
					MessageID:  msg.ID,
				}

				measurementsMu.Lock()
				measurements = append(measurements, measurement)
				measurementsMu.Unlock()

				suite.metrics.AddLatency(latency)
				suite.metrics.AddSuccess()
				messageCount++
			}
		}(i)
	}

	wg.Wait()
	suite.metrics.Calculate()

	// Validate overall latency performance
	if suite.metrics.P95Latency > 50*time.Millisecond {
		t.Errorf("P95 WebSocket latency too high: %v (expected < 50ms)", suite.metrics.P95Latency)
	}

	if suite.metrics.P99Latency > 100*time.Millisecond {
		t.Errorf("P99 WebSocket latency too high: %v (expected < 100ms)", suite.metrics.P99Latency)
	}

	if suite.metrics.ErrorRate > 1.0 {
		t.Errorf("WebSocket error rate too high: %.2f%% (expected < 1%%)", suite.metrics.ErrorRate)
	}

	t.Logf("WebSocket Latency Test Results:")
	t.Logf("  Total Messages: %d", len(measurements))
	t.Logf("  Average Latency: %v", suite.metrics.AverageLatency)
	t.Logf("  P95 Latency: %v", suite.metrics.P95Latency)
	t.Logf("  P99 Latency: %v", suite.metrics.P99Latency)
	t.Logf("  Error Rate: %.2f%%", suite.metrics.ErrorRate)
}

func TestWebSocketConcurrency(t *testing.T) {
	config := DefaultTestConfig()
	config.ConcurrentOps = 100 // High concurrency test
	config.MaxDuration = 60 * time.Second

	suite := NewWebSocketTestSuite(config)
	defer suite.server.Close()

	var wg sync.WaitGroup
	connectionCount := int64(0)
	messageCount := int64(0)
	var countMu sync.Mutex

	startTime := time.Now()

	// Launch concurrent WebSocket connections
	for i := 0; i < config.ConcurrentOps; i++ {
		wg.Add(1)
		go func(clientID int) {
			defer wg.Done()

			// Connect to WebSocket
			url := "ws" + strings.TrimPrefix(suite.server.URL, "http") + "/ws"
			conn, _, err := websocket.DefaultDialer.Dial(url, nil)
			if err != nil {
				suite.metrics.AddFailure()
				return
			}
			defer conn.Close()

			countMu.Lock()
			connectionCount++
			countMu.Unlock()

			suite.metrics.AddSuccess()

			// Subscribe to multiple channels
			subscription := WebSocketSubscription{
				Type: "subscribe",
				Channels: []string{
					"orderbook.BTC-USD",
					"trades.BTC-USD",
					"ticker.BTC-USD",
					"orderbook.ETH-USD",
					"trades.ETH-USD",
				},
			}

			if err := conn.WriteJSON(subscription); err != nil {
				suite.metrics.AddFailure()
				return
			}

			// Receive messages
			for time.Since(startTime) < config.MaxDuration {
				conn.SetReadDeadline(time.Now().Add(1 * time.Second))
				var msg WebSocketMessage
				err := conn.ReadJSON(&msg)
				if err != nil {
					if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
						suite.metrics.AddFailure()
					}
					break
				}

				countMu.Lock()
				messageCount++
				countMu.Unlock()

				suite.metrics.AddSuccess()
				suite.metrics.AddLatency(time.Since(msg.Timestamp))
			}
		}(i)
	}

	wg.Wait()
	suite.metrics.Calculate()

	duration := time.Since(startTime)
	throughput := float64(messageCount) / duration.Seconds()

	// Validate concurrency performance
	if connectionCount < int64(config.ConcurrentOps*8/10) { // At least 80% connections should succeed
		t.Errorf("Too few successful connections: %d/%d", connectionCount, config.ConcurrentOps)
	}

	if throughput < 1000 { // Expect at least 1000 messages/sec
		t.Errorf("WebSocket throughput too low: %.2f msg/sec (expected > 1000)", throughput)
	}

	if suite.metrics.ErrorRate > 5.0 {
		t.Errorf("WebSocket error rate too high: %.2f%% (expected < 5%%)", suite.metrics.ErrorRate)
	}

	t.Logf("WebSocket Concurrency Test Results:")
	t.Logf("  Successful Connections: %d/%d", connectionCount, config.ConcurrentOps)
	t.Logf("  Total Messages: %d", messageCount)
	t.Logf("  Throughput: %.2f msg/sec", throughput)
	t.Logf("  Average Latency: %v", suite.metrics.AverageLatency)
	t.Logf("  Error Rate: %.2f%%", suite.metrics.ErrorRate)
}

func TestWebSocketSubscriptionManagement(t *testing.T) {
	config := DefaultTestConfig()
	suite := NewWebSocketTestSuite(config)
	defer suite.server.Close()

	// Connect to WebSocket
	url := "ws" + strings.TrimPrefix(suite.server.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket: %v", err)
	}
	defer conn.Close()

	// Test subscription
	subscription := WebSocketSubscription{
		Type:     "subscribe",
		Channels: []string{"orderbook.BTC-USD", "trades.BTC-USD"},
	}

	if err := conn.WriteJSON(subscription); err != nil {
		t.Fatalf("Failed to send subscription: %v", err)
	}

	// Verify subscription confirmation
	var response WebSocketMessage
	if err := conn.ReadJSON(&response); err != nil {
		t.Fatalf("Failed to read subscription confirmation: %v", err)
	}

	if response.Type != "subscription_confirmed" {
		t.Errorf("Expected subscription confirmation, got: %s", response.Type)
	}

	// Receive some messages
	messageCount := 0
	for messageCount < 10 {
		var msg WebSocketMessage
		if err := conn.ReadJSON(&msg); err != nil {
			t.Fatalf("Failed to read message: %v", err)
		}

		// Verify message format
		if msg.Type == "" {
			t.Error("Message missing type field")
		}

		if msg.Channel == "" {
			t.Error("Message missing channel field")
		}

		if msg.Timestamp.IsZero() {
			t.Error("Message missing timestamp")
		}

		messageCount++
	}

	// Test unsubscription
	unsubscription := WebSocketSubscription{
		Type:     "unsubscribe",
		Channels: []string{"trades.BTC-USD"},
	}

	if err := conn.WriteJSON(unsubscription); err != nil {
		t.Fatalf("Failed to send unsubscription: %v", err)
	}

	// Verify unsubscription confirmation
	if err := conn.ReadJSON(&response); err != nil {
		t.Fatalf("Failed to read unsubscription confirmation: %v", err)
	}

	if response.Type != "unsubscription_confirmed" {
		t.Errorf("Expected unsubscription confirmation, got: %s", response.Type)
	}

	t.Logf("WebSocket Subscription Management Test Completed Successfully")
}

func TestWebSocketOrderUpdates(t *testing.T) {
	config := DefaultTestConfig()
	suite := NewWebSocketTestSuite(config)
	defer suite.server.Close()

	// Connect to WebSocket
	url := "ws" + strings.TrimPrefix(suite.server.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket: %v", err)
	}
	defer conn.Close()

	// Subscribe to order updates
	subscription := WebSocketSubscription{
		Type:     "subscribe",
		Channels: []string{"orders"},
	}

	if err := conn.WriteJSON(subscription); err != nil {
		t.Fatalf("Failed to send subscription: %v", err)
	}

	// Skip subscription confirmation
	var response WebSocketMessage
	conn.ReadJSON(&response)

	// Simulate order placement and track updates
	orderUpdates := make(map[string][]WebSocketMessage)
	var updatesMu sync.Mutex

	// Start listening for order updates
	go func() {
		for {
			var msg WebSocketMessage
			err := conn.ReadJSON(&msg)
			if err != nil {
				return
			}

			if msg.Channel == "orders" && msg.Data != nil {
				if orderID, exists := msg.Data["order_id"].(string); exists {
					updatesMu.Lock()
					orderUpdates[orderID] = append(orderUpdates[orderID], msg)
					updatesMu.Unlock()
				}
			}
		}
	}()

	// Simulate order lifecycle
	time.Sleep(5 * time.Second) // Allow time to receive order updates

	updatesMu.Lock()
	totalUpdates := 0
	for _, updates := range orderUpdates {
		totalUpdates += len(updates)
	}
	updatesMu.Unlock()

	// Validate order update functionality
	if totalUpdates == 0 {
		t.Error("No order updates received")
	}

	t.Logf("WebSocket Order Updates Test Results:")
	t.Logf("  Orders Tracked: %d", len(orderUpdates))
	t.Logf("  Total Updates: %d", totalUpdates)
}

func TestWebSocketMarketDataStream(t *testing.T) {
	config := DefaultTestConfig()
	suite := NewWebSocketTestSuite(config)
	defer suite.server.Close()

	// Connect to WebSocket
	url := "ws" + strings.TrimPrefix(suite.server.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket: %v", err)
	}
	defer conn.Close()

	// Subscribe to market data streams
	subscription := WebSocketSubscription{
		Type: "subscribe",
		Channels: []string{
			"orderbook.BTC-USD",
			"trades.BTC-USD",
			"ticker.BTC-USD",
			"candles.BTC-USD.1m",
		},
	}

	if err := conn.WriteJSON(subscription); err != nil {
		t.Fatalf("Failed to send subscription: %v", err)
	}

	// Skip subscription confirmation
	var response WebSocketMessage
	conn.ReadJSON(&response)

	// Collect market data for analysis
	marketData := make(map[string][]WebSocketMessage)
	var dataMu sync.Mutex

	startTime := time.Now()
	for time.Since(startTime) < 30*time.Second {
		var msg WebSocketMessage
		conn.SetReadDeadline(time.Now().Add(1 * time.Second))
		err := conn.ReadJSON(&msg)
		if err != nil {
			if netErr, ok := err.(interface{ Timeout() bool }); ok && netErr.Timeout() {
				continue
			}
			break
		}

		dataMu.Lock()
		marketData[msg.Channel] = append(marketData[msg.Channel], msg)
		dataMu.Unlock()

		// Validate message latency
		latency := time.Since(msg.Timestamp)
		if latency > 50*time.Millisecond {
			t.Errorf("Market data latency too high: %v (expected < 50ms)", latency)
		}

		suite.metrics.AddLatency(latency)
		suite.metrics.AddSuccess()
	}

	suite.metrics.Calculate()

	// Validate market data stream quality
	dataMu.Lock()
	for channel, messages := range marketData {
		if len(messages) == 0 {
			t.Errorf("No messages received for channel: %s", channel)
		} else {
			t.Logf("Channel %s: %d messages", channel, len(messages))
		}
	}
	dataMu.Unlock()

	if suite.metrics.AverageLatency > 25*time.Millisecond {
		t.Errorf("Average market data latency too high: %v (expected < 25ms)", suite.metrics.AverageLatency)
	}

	t.Logf("WebSocket Market Data Stream Test Results:")
	t.Logf("  Channels Tested: %d", len(marketData))
	t.Logf("  Average Latency: %v", suite.metrics.AverageLatency)
	t.Logf("  P95 Latency: %v", suite.metrics.P95Latency)
	t.Logf("  Total Messages: %d", suite.metrics.TotalOperations)
}

// WebSocket server implementation for testing

func (suite *WebSocketTestSuite) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := suite.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	clientID := uuid.New().String()
	suite.mu.Lock()
	suite.connections[clientID] = conn
	suite.mu.Unlock()

	defer func() {
		suite.mu.Lock()
		delete(suite.connections, clientID)
		suite.mu.Unlock()
	}()

	// Handle subscription messages
	subscriptions := make(map[string]bool)

	// Start message broadcasting
	go suite.broadcastMessages(clientID, &subscriptions)

	// Handle incoming messages
	for {
		var msg WebSocketSubscription
		err := conn.ReadJSON(&msg)
		if err != nil {
			break
		}

		if msg.Type == "subscribe" {
			for _, channel := range msg.Channels {
				subscriptions[channel] = true
			}

			// Send confirmation
			response := WebSocketMessage{
				Type:      "subscription_confirmed",
				Data:      map[string]interface{}{"channels": msg.Channels},
				Timestamp: time.Now(),
			}
			conn.WriteJSON(response)

		} else if msg.Type == "unsubscribe" {
			for _, channel := range msg.Channels {
				delete(subscriptions, channel)
			}

			// Send confirmation
			response := WebSocketMessage{
				Type:      "unsubscription_confirmed",
				Data:      map[string]interface{}{"channels": msg.Channels},
				Timestamp: time.Now(),
			}
			conn.WriteJSON(response)
		}
	}
}

func (suite *WebSocketTestSuite) broadcastMessages(clientID string, subscriptions *map[string]bool) {
	ticker := time.NewTicker(100 * time.Millisecond) // Send message every 100ms
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			suite.mu.RLock()
			conn, exists := suite.connections[clientID]
			suite.mu.RUnlock()

			if !exists {
				return
			}

			// Send messages for subscribed channels
			for channel, subscribed := range *subscriptions {
				if !subscribed {
					continue
				}

				var msg WebSocketMessage
				msg.Type = "data"
				msg.Channel = channel
				msg.Timestamp = time.Now()
				msg.ID = uuid.New().String()

				// Generate appropriate data based on channel
				switch {
				case strings.HasPrefix(channel, "orderbook"):
					msg.Data = suite.generateOrderBookData()
				case strings.HasPrefix(channel, "trades"):
					msg.Data = suite.generateTradeData()
				case strings.HasPrefix(channel, "ticker"):
					msg.Data = suite.generateTickerData()
				case strings.HasPrefix(channel, "orders"):
					msg.Data = suite.generateOrderUpdateData()
				case strings.HasPrefix(channel, "candles"):
					msg.Data = suite.generateCandleData()
				default:
					msg.Data = map[string]interface{}{"message": "test data"}
				}

				if err := conn.WriteJSON(msg); err != nil {
					return
				}
			}
		}
	}
}

func (suite *WebSocketTestSuite) generateOrderBookData() map[string]interface{} {
	return map[string]interface{}{
		"bids": [][]string{
			{"49950.00", "1.5"},
			{"49900.00", "2.0"},
		},
		"asks": [][]string{
			{"50050.00", "1.0"},
			{"50100.00", "1.8"},
		},
		"timestamp": time.Now().Unix(),
	}
}

func (suite *WebSocketTestSuite) generateTradeData() map[string]interface{} {
	return map[string]interface{}{
		"id":        uuid.New().String(),
		"price":     fmt.Sprintf("%.2f", 50000+float64((time.Now().Unix()%1000)-500)),
		"quantity":  fmt.Sprintf("%.4f", 0.1+float64(time.Now().Unix()%100)/1000),
		"side":      []string{"buy", "sell"}[time.Now().Unix()%2],
		"timestamp": time.Now().Unix(),
	}
}

func (suite *WebSocketTestSuite) generateTickerData() map[string]interface{} {
	basePrice := 50000.0
	change := float64((time.Now().Unix() % 1000) - 500)
	return map[string]interface{}{
		"price":     fmt.Sprintf("%.2f", basePrice+change),
		"change":    fmt.Sprintf("%.2f", change/basePrice*100),
		"volume":    fmt.Sprintf("%.2f", 1000+float64(time.Now().Unix()%500)),
		"high":      fmt.Sprintf("%.2f", basePrice+500),
		"low":       fmt.Sprintf("%.2f", basePrice-500),
		"timestamp": time.Now().Unix(),
	}
}

func (suite *WebSocketTestSuite) generateOrderUpdateData() map[string]interface{} {
	statuses := []string{"pending", "partial", "filled", "cancelled"}
	return map[string]interface{}{
		"order_id":   uuid.New().String(),
		"status":     statuses[time.Now().Unix()%int64(len(statuses))],
		"filled_qty": fmt.Sprintf("%.4f", float64(time.Now().Unix()%100)/100),
		"remaining":  fmt.Sprintf("%.4f", 1.0-float64(time.Now().Unix()%100)/100),
		"timestamp":  time.Now().Unix(),
	}
}

func (suite *WebSocketTestSuite) generateCandleData() map[string]interface{} {
	basePrice := 50000.0
	return map[string]interface{}{
		"timestamp": time.Now().Unix(),
		"open":      fmt.Sprintf("%.2f", basePrice),
		"high":      fmt.Sprintf("%.2f", basePrice+100),
		"low":       fmt.Sprintf("%.2f", basePrice-100),
		"close":     fmt.Sprintf("%.2f", basePrice+50),
		"volume":    fmt.Sprintf("%.2f", 100.0),
	}
}
