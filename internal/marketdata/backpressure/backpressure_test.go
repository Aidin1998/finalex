// Comprehensive tests for the backpressure management system
package backpressure

import (
	"context"

	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

// TestBackpressureManager tests the core backpressure manager functionality
func TestBackpressureManager(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := getDefaultManagerConfig()

	manager, err := NewBackpressureManager(config, logger)
	require.NoError(t, err)
	require.NotNil(t, manager)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start manager
	err = manager.Start(ctx)
	require.NoError(t, err)

	// Test basic message distribution
	t.Run("BasicMessageDistribution", func(t *testing.T) {
		// Create mock client
		mockConn := &MockWebSocketConn{}
		clientID := "test_client_1"

		err := manager.RegisterClient(clientID, mockConn)
		require.NoError(t, err)

		// Distribute a message
		testData := []byte(`{"test": "message"}`)
		err = manager.DistributeMessage(PriorityHigh, testData, []string{clientID})
		require.NoError(t, err)

		// Wait for processing
		time.Sleep(time.Millisecond * 100)

		// Unregister client
		manager.UnregisterClient(clientID)
	})

	// Test emergency mode
	t.Run("EmergencyMode", func(t *testing.T) {
		// Activate emergency mode
		manager.SetEmergencyMode(true)
		assert.True(t, manager.IsEmergencyMode())

		// Try to send non-critical message (should be dropped)
		testData := []byte(`{"test": "emergency"}`)
		err = manager.DistributeMessage(PriorityLow, testData, nil)
		assert.Error(t, err)
		assert.Equal(t, ErrEmergencyMode, err)

		// Send critical message (should work)
		err = manager.DistributeMessage(PriorityCritical, testData, nil)
		assert.NoError(t, err)

		// Deactivate emergency mode
		manager.SetEmergencyMode(false)
		assert.False(t, manager.IsEmergencyMode())
	})

	// Test rate limiting integration
	t.Run("RateLimiting", func(t *testing.T) {
		mockConn := &MockWebSocketConn{}
		clientID := "test_client_rate_limit"

		err := manager.RegisterClient(clientID, mockConn)
		require.NoError(t, err)
		defer manager.UnregisterClient(clientID)

		// Send messages rapidly to trigger rate limiting
		testData := []byte(`{"test": "rate_limit"}`)
		successCount := 0

		for i := 0; i < 1000; i++ {
			err = manager.DistributeMessage(PriorityMedium, testData, []string{clientID})
			if err == nil {
				successCount++
			}
		}

		// Should have some rate limiting
		assert.Less(t, successCount, 1000, "Rate limiting should prevent all messages from being sent")
	})

	// Stop manager
	err = manager.Stop()
	require.NoError(t, err)
}

// TestClientCapabilityDetector tests client performance measurement
func TestClientCapabilityDetector(t *testing.T) {
	logger := zaptest.NewLogger(t)

	detector, err := NewClientCapabilityDetector(logger)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = detector.Start(ctx)
	require.NoError(t, err)
	defer detector.Stop()

	t.Run("ClientRegistrationAndMeasurement", func(t *testing.T) {
		mockConn := &MockWebSocketConn{}
		clientID := "test_client_detector"

		// Register client
		err := detector.RegisterClient(clientID, mockConn)
		require.NoError(t, err)

		// Record some measurements
		for i := 0; i < 10; i++ {
			err = detector.RecordMessageDelivery(clientID, 1024, time.Millisecond*10)
			require.NoError(t, err)
		}

		// Get capability
		capability, err := detector.GetClientCapability(clientID)
		require.NoError(t, err)
		require.NotNil(t, capability)

		assert.Equal(t, clientID, capability.ClientID)
		assert.Greater(t, capability.AvgBandwidth, int64(0))
		assert.Greater(t, capability.AvgLatency, int64(0))

		// Unregister client
		detector.UnregisterClient(clientID)
	})

	t.Run("ClientClassification", func(t *testing.T) {
		mockConn := &MockWebSocketConn{}
		clientID := "test_client_classification"

		err := detector.RegisterClient(clientID, mockConn)
		require.NoError(t, err)
		defer detector.UnregisterClient(clientID)

		// Simulate HFT client behavior (high bandwidth, low latency)
		for i := 0; i < 50; i++ {
			err = detector.RecordMessageDelivery(clientID, 10*1024, time.Microsecond*500) // High bandwidth, low latency
			require.NoError(t, err)
		}

		// Wait for classification
		time.Sleep(time.Millisecond * 100)

		classification := detector.ClassifyClient(clientID)
		// Should be classified as HFT or at least not retail
		assert.NotEqual(t, RetailClient, classification)
	})
}

// TestAdaptiveRateLimiter tests rate limiting functionality
func TestAdaptiveRateLimiter(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := &RateLimiterConfig{
		GlobalRateLimit:    1000,
		BurstMultiplier:    2.0,
		RefillInterval:     time.Second,
		AdaptationInterval: time.Second * 5,
	}

	rateLimiter, err := NewAdaptiveRateLimiter(config, logger)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = rateLimiter.Start(ctx)
	require.NoError(t, err)
	defer rateLimiter.Stop()

	t.Run("BasicRateLimiting", func(t *testing.T) {
		clientID := "test_client_rate"

		// Create client limit
		limit, err := rateLimiter.CreateClientLimit(clientID, DefaultClientClass)
		require.NoError(t, err)
		require.NotNil(t, limit)

		// Test allowing messages
		allowed := 0
		for i := 0; i < 100; i++ {
			if rateLimiter.Allow(clientID, PriorityMedium) {
				allowed++
			}
		}

		assert.Greater(t, allowed, 0, "Should allow some messages")
		assert.Less(t, allowed, 100, "Should rate limit some messages")

		// Clean up
		rateLimiter.RemoveClientLimit(clientID)
	})

	t.Run("PriorityBasedRateLimiting", func(t *testing.T) {
		clientID := "test_client_priority"

		limit, err := rateLimiter.CreateClientLimit(clientID, DefaultClientClass)
		require.NoError(t, err)
		defer rateLimiter.RemoveClientLimit(clientID)

		// Critical messages should have higher success rate
		criticalAllowed := 0
		lowAllowed := 0

		for i := 0; i < 100; i++ {
			if rateLimiter.Allow(clientID, PriorityCritical) {
				criticalAllowed++
			}
			if rateLimiter.Allow(clientID, PriorityLow) {
				lowAllowed++
			}
		}

		assert.Greater(t, criticalAllowed, lowAllowed, "Critical messages should be allowed more often")
	})
}

// TestPriorityQueue tests the lock-free priority queue
func TestPriorityQueue(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := &PriorityQueueConfig{
		InitialCapacity: 1000,
		MaxCapacity:     10000,
		ShardCount:      4,
	}

	queue, err := NewPriorityQueue(config, logger)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = queue.Start(ctx)
	require.NoError(t, err)
	defer queue.Stop()

	t.Run("BasicEnqueueDequeue", func(t *testing.T) {
		msg := &PriorityMessage{
			Priority:  PriorityHigh,
			Data:      []byte("test message"),
			Timestamp: time.Now(),
			Deadline:  time.Now().Add(time.Second),
		}

		// Enqueue message
		err := queue.Enqueue(msg)
		require.NoError(t, err)

		// Dequeue message
		dequeuedMsg, err := queue.Dequeue()
		require.NoError(t, err)
		require.NotNil(t, dequeuedMsg)

		assert.Equal(t, msg.Priority, dequeuedMsg.Priority)
		assert.Equal(t, string(msg.Data), string(dequeuedMsg.Data))
	})

	t.Run("PriorityOrdering", func(t *testing.T) {
		// Enqueue messages with different priorities
		messages := []*PriorityMessage{
			{Priority: PriorityLow, Data: []byte("low"), Timestamp: time.Now(), Deadline: time.Now().Add(time.Second)},
			{Priority: PriorityCritical, Data: []byte("critical"), Timestamp: time.Now(), Deadline: time.Now().Add(time.Second)},
			{Priority: PriorityMedium, Data: []byte("medium"), Timestamp: time.Now(), Deadline: time.Now().Add(time.Second)},
			{Priority: PriorityHigh, Data: []byte("high"), Timestamp: time.Now(), Deadline: time.Now().Add(time.Second)},
		}

		// Enqueue all messages
		for _, msg := range messages {
			err := queue.Enqueue(msg)
			require.NoError(t, err)
		}

		// Dequeue and check priority order
		expectedOrder := []MessagePriority{PriorityCritical, PriorityHigh, PriorityMedium, PriorityLow}

		for i, expectedPriority := range expectedOrder {
			msg, err := queue.Dequeue()
			require.NoError(t, err, "Failed to dequeue message %d", i)
			assert.Equal(t, expectedPriority, msg.Priority, "Message %d has wrong priority", i)
		}
	})

	t.Run("ConcurrentAccess", func(t *testing.T) {
		const numGoroutines = 10
		const messagesPerGoroutine = 100

		var wg sync.WaitGroup
		var enqueueErrors int64
		var dequeueErrors int64

		// Enqueue goroutines
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(goroutineID int) {
				defer wg.Done()
				for j := 0; j < messagesPerGoroutine; j++ {
					msg := &PriorityMessage{
						Priority:  MessagePriority(j % 5), // Cycle through priorities
						Data:      []byte(fmt.Sprintf("msg_%d_%d", goroutineID, j)),
						Timestamp: time.Now(),
						Deadline:  time.Now().Add(time.Second),
					}
					if err := queue.Enqueue(msg); err != nil {
						atomic.AddInt64(&enqueueErrors, 1)
					}
				}
			}(i)
		}

		// Dequeue goroutines
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < messagesPerGoroutine; j++ {
					if _, err := queue.Dequeue(); err != nil {
						atomic.AddInt64(&dequeueErrors, 1)
					}
				}
			}()
		}

		wg.Wait()

		assert.Equal(t, int64(0), atomic.LoadInt64(&enqueueErrors), "Should have no enqueue errors")
		// Some dequeue errors are expected when queue is empty
	})
}

// TestWebSocketIntegration tests WebSocket integration with backpressure
func TestWebSocketIntegration(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Create backpressure manager
	managerConfig := getDefaultManagerConfig()
	manager, err := NewBackpressureManager(managerConfig, logger.Named("manager"))
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = manager.Start(ctx)
	require.NoError(t, err)
	defer manager.Stop()

	// Create WebSocket integrator
	wsConfig := getDefaultWebSocketConfig()
	integrator := NewWebSocketIntegrator(manager, wsConfig, logger.Named("integrator"))

	err = integrator.Start(ctx)
	require.NoError(t, err)
	defer integrator.Stop()

	t.Run("ClientRegistrationAndMessaging", func(t *testing.T) {
		mockConn := &MockWebSocketConn{}
		clientID := "test_ws_client"

		// Register client
		err := integrator.RegisterClient(clientID, mockConn)
		require.NoError(t, err)

		// Send broadcast message
		testData := map[string]interface{}{
			"symbol": "BTC/USD",
			"price":  45000.0,
		}

		err = integrator.BroadcastMessage("trade", testData)
		require.NoError(t, err)

		// Send targeted message
		err = integrator.SendToClient(clientID, "ticker", testData)
		require.NoError(t, err)

		// Wait for processing
		time.Sleep(time.Millisecond * 100)

		// Check that messages were processed
		assert.Greater(t, len(mockConn.writtenMessages), 0)

		// Unregister client
		integrator.UnregisterClient(clientID)
	})

	t.Run("LoadTesting", func(t *testing.T) {
		const numClients = 50
		const messagesPerClient = 100

		var wg sync.WaitGroup
		clients := make([]*MockWebSocketConn, numClients)
		clientIDs := make([]string, numClients)

		// Register clients
		for i := 0; i < numClients; i++ {
			clients[i] = &MockWebSocketConn{}
			clientIDs[i] = fmt.Sprintf("load_test_client_%d", i)

			err := integrator.RegisterClient(clientIDs[i], clients[i])
			require.NoError(t, err)
		}

		// Send messages concurrently
		start := time.Now()

		for i := 0; i < messagesPerClient; i++ {
			wg.Add(1)
			go func(msgNum int) {
				defer wg.Done()

				testData := map[string]interface{}{
					"message_num": msgNum,
					"timestamp":   time.Now().UnixNano(),
				}

				err := integrator.BroadcastMessage("load_test", testData)
				if err != nil {
					t.Logf("Broadcast error: %v", err)
				}
			}(i)
		}

		wg.Wait()
		duration := time.Since(start)

		t.Logf("Load test completed: %d messages to %d clients in %v", messagesPerClient, numClients, duration)
		t.Logf("Message rate: %.2f msg/sec", float64(messagesPerClient)/duration.Seconds())

		// Clean up clients
		for _, clientID := range clientIDs {
			integrator.UnregisterClient(clientID)
		}
	})
}

// TestSystemIntegration tests the complete integrated system
func TestSystemIntegration(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Load configuration
	config, err := LoadBackpressureConfig("../../configs/backpressure.yaml")
	if err != nil {
		// Use default config if file doesn't exist
		config = GetDefaultBackpressureConfig()
	}

	// Create and start complete system
	manager, err := NewBackpressureManager(&config.Manager, logger.Named("manager"))
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	err = manager.Start(ctx)
	require.NoError(t, err)
	defer manager.Stop()

	integrator := NewWebSocketIntegrator(manager, &config.WebSocket, logger.Named("integrator"))
	err = integrator.Start(ctx)
	require.NoError(t, err)
	defer integrator.Stop()

	t.Run("EndToEndScenario", func(t *testing.T) {
		// Simulate realistic trading scenario
		const numClients = 20
		const testDuration = time.Second * 5

		var wg sync.WaitGroup
		clients := make([]*MockWebSocketConn, numClients)
		clientIDs := make([]string, numClients)

		// Register diverse client types
		for i := 0; i < numClients; i++ {
			clients[i] = &MockWebSocketConn{}
			clientIDs[i] = fmt.Sprintf("scenario_client_%d", i)

			err := integrator.RegisterClient(clientIDs[i], clients[i])
			require.NoError(t, err)
		}

		// Generate market data
		wg.Add(1)
		go func() {
			defer wg.Done()

			ticker := time.NewTicker(time.Millisecond * 100)
			defer ticker.Stop()

			timeout := time.After(testDuration)
			messageCount := 0

			for {
				select {
				case <-timeout:
					t.Logf("Generated %d messages during test", messageCount)
					return
				case <-ticker.C:
					// Generate trade
					trade := map[string]interface{}{
						"symbol":    "BTC/USD",
						"price":     45000 + (messageCount%1000)*0.1,
						"quantity":  1.5,
						"timestamp": time.Now().UnixNano(),
					}

					if err := integrator.BroadcastMessage("trade", trade); err != nil {
						t.Logf("Trade broadcast error: %v", err)
					}
					messageCount++

					// Occasionally generate orderbook update
					if messageCount%10 == 0 {
						orderbook := map[string]interface{}{
							"symbol": "BTC/USD",
							"bids":   [][]float64{{45000, 1.0}, {44999, 2.0}},
							"asks":   [][]float64{{45001, 1.0}, {45002, 2.0}},
						}

						if err := integrator.BroadcastMessage("orderbook", orderbook); err != nil {
							t.Logf("Orderbook broadcast error: %v", err)
						}
					}
				}
			}
		}()

		// Simulate client load
		for i := 0; i < numClients/2; i++ {
			wg.Add(1)
			go func(clientIndex int) {
				defer wg.Done()

				// Simulate client sending occasional messages
				ticker := time.NewTicker(time.Second)
				defer ticker.Stop()

				timeout := time.After(testDuration)

				for {
					select {
					case <-timeout:
						return
					case <-ticker.C:
						// Send targeted message to simulate client interaction
						response := map[string]interface{}{
							"type":      "heartbeat",
							"client":    clientIndex,
							"timestamp": time.Now().UnixNano(),
						}

						if err := integrator.SendToClient(clientIDs[clientIndex], "heartbeat", response); err != nil {
							t.Logf("Client message error: %v", err)
						}
					}
				}
			}(i)
		}

		// Monitor system health
		wg.Add(1)
		go func() {
			defer wg.Done()

			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()

			timeout := time.After(testDuration)

			for {
				select {
				case <-timeout:
					return
				case <-ticker.C:
					stats := integrator.GetStats()
					t.Logf("System stats: %+v", stats)

					if manager.IsEmergencyMode() {
						t.Log("System in emergency mode")
					}
				}
			}
		}()

		wg.Wait()

		// Verify system state
		assert.False(t, manager.IsEmergencyMode(), "System should not be in emergency mode after normal load")

		// Clean up
		for _, clientID := range clientIDs {
			integrator.UnregisterClient(clientID)
		}
	})
}

// MockWebSocketConn implements websocket.Conn interface for testing
type MockWebSocketConn struct {
	writtenMessages [][]byte
	closed          bool
	mu              sync.Mutex
}

func (m *MockWebSocketConn) WriteMessage(messageType int, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return fmt.Errorf("connection closed")
	}

	// Copy data to avoid race conditions
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)
	m.writtenMessages = append(m.writtenMessages, dataCopy)

	return nil
}

func (m *MockWebSocketConn) ReadMessage() (messageType int, p []byte, err error) {
	return websocket.TextMessage, []byte(`{"type":"ping"}`), nil
}

func (m *MockWebSocketConn) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func (m *MockWebSocketConn) SetWriteDeadline(t time.Time) error {
	return nil
}

func (m *MockWebSocketConn) SetReadDeadline(t time.Time) error {
	return nil
}

// BenchmarkBackpressureSystem benchmarks the complete backpressure system
func BenchmarkBackpressureSystem(b *testing.B) {
	logger := zaptest.NewLogger(b)
	config := getDefaultManagerConfig()

	manager, err := NewBackpressureManager(config, logger)
	require.NoError(b, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = manager.Start(ctx)
	require.NoError(b, err)
	defer manager.Stop()

	// Register test clients
	const numClients = 100
	clientIDs := make([]string, numClients)
	for i := 0; i < numClients; i++ {
		clientIDs[i] = fmt.Sprintf("bench_client_%d", i)
		mockConn := &MockWebSocketConn{}
		err := manager.RegisterClient(clientIDs[i], mockConn)
		require.NoError(b, err)
	}

	testData := []byte(`{"symbol":"BTC/USD","price":45000,"quantity":1.5}`)

	b.ResetTimer()

	b.Run("BroadcastMessages", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			err := manager.DistributeMessage(PriorityHigh, testData, nil)
			if err != nil && err != ErrQueueFull {
				b.Error(err)
			}
		}
	})

	b.Run("TargetedMessages", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			clientID := clientIDs[i%numClients]
			err := manager.DistributeMessage(PriorityMedium, testData, []string{clientID})
			if err != nil && err != ErrQueueFull {
				b.Error(err)
			}
		}
	})

	// Clean up
	for _, clientID := range clientIDs {
		manager.UnregisterClient(clientID)
	}
}
