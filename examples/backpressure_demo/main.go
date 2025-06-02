// Integration example and testing utilities for backpressure system
package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/auth"
	"github.com/Aidin1998/pincex_unified/pkg/models"

	"github.com/Aidin1998/pincex_unified/internal/marketdata"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// BackpressureExample demonstrates the complete backpressure system integration
type BackpressureExample struct {
	enhancedHub *marketdata.EnhancedHub
	logger      *zap.Logger
	upgrader    websocket.Upgrader

	// Test data generation
	mockTradeGen     *MockTradeGenerator
	mockOrderbookGen *MockOrderbookGenerator

	// Performance tracking
	stats         *PerformanceStats
	statsInterval time.Duration
}

// PerformanceStats tracks system performance during testing
type PerformanceStats struct {
	mu                   sync.RWMutex
	StartTime            time.Time
	TotalMessages        int64
	TotalClients         int64
	EmergencyActivations int64
	AverageLatency       time.Duration
	MessageRate          float64
	DropRate             float64
}

// MockTradeGenerator generates realistic trade data for testing
type MockTradeGenerator struct {
	symbols []string
	prices  map[string]float64
	mu      sync.RWMutex
}

// MockOrderbookGenerator generates realistic orderbook data
type MockOrderbookGenerator struct {
	symbols    []string
	orderbooks map[string]*MockOrderbook
	mu         sync.RWMutex
}

// MockOrderbook represents a simplified orderbook
type MockOrderbook struct {
	Symbol    string      `json:"symbol"`
	Bids      [][]float64 `json:"bids"`
	Asks      [][]float64 `json:"asks"`
	Timestamp time.Time   `json:"timestamp"`
}

// Trade represents a trade message
type Trade struct {
	Symbol    string    `json:"symbol"`
	Price     float64   `json:"price"`
	Quantity  float64   `json:"quantity"`
	Side      string    `json:"side"`
	Timestamp time.Time `json:"timestamp"`
	TradeID   string    `json:"trade_id"`
}

func main() {
	// Initialize logger
	logger, err := zap.NewDevelopment()
	if err != nil {
		log.Fatal("Failed to create logger:", err)
	}
	defer logger.Sync()

	// Create backpressure example
	example, err := NewBackpressureExample(logger)
	if err != nil {
		logger.Fatal("Failed to create backpressure example", zap.Error(err))
	}

	// Start the example
	ctx := context.Background()
	if err := example.Start(ctx); err != nil {
		logger.Fatal("Failed to start backpressure example", zap.Error(err))
	}

	// Setup HTTP server for WebSocket connections
	example.setupHTTPServer()

	logger.Info("Backpressure example started successfully")
	logger.Info("WebSocket endpoint: ws://localhost:8080/ws")
	logger.Info("Stats endpoint: http://localhost:8080/stats")
	logger.Info("Emergency control: POST http://localhost:8080/emergency")

	// Keep running
	select {}
}

// NewBackpressureExample creates a new example instance
func NewBackpressureExample(logger *zap.Logger) (*BackpressureExample, error) {
	// Create mock auth service
	authService := &MockAuthService{}

	// Create enhanced hub with backpressure
	config := &marketdata.EnhancedHubConfig{
		BackpressureConfigPath:      "configs/backpressure.yaml",
		EnablePerformanceMonitoring: true,
		EnableAdaptiveRateLimiting:  true,
		EnableCrossServiceCoord:     true,
		AutoClientClassification:    true,
		ClientTimeoutExtended:       time.Minute * 10,
		MaxClientsPerInstance:       10000,
		MessageBatchSize:            100,
		BroadcastCoalescing:         true,
		CoalescingWindowMs:          10,
	}

	enhancedHub, err := marketdata.NewEnhancedHub(authService, config, logger.Named("enhanced_hub"))
	if err != nil {
		return nil, fmt.Errorf("failed to create enhanced hub: %w", err)
	}

	// Create mock data generators
	mockTradeGen := NewMockTradeGenerator()
	mockOrderbookGen := NewMockOrderbookGenerator()

	example := &BackpressureExample{
		enhancedHub:      enhancedHub,
		logger:           logger,
		upgrader:         websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }},
		mockTradeGen:     mockTradeGen,
		mockOrderbookGen: mockOrderbookGen,
		stats:            &PerformanceStats{StartTime: time.Now()},
		statsInterval:    time.Second * 5,
	}

	return example, nil
}

// Start initializes and starts the example
func (e *BackpressureExample) Start(ctx context.Context) error {
	e.logger.Info("Starting backpressure example")

	// Start enhanced hub
	if err := e.enhancedHub.Start(ctx); err != nil {
		return fmt.Errorf("failed to start enhanced hub: %w", err)
	}

	// Start mock data generation
	go e.generateMockData()

	// Start performance monitoring
	go e.monitorPerformance()

	return nil
}

// setupHTTPServer sets up HTTP server for WebSocket connections and control endpoints
func (e *BackpressureExample) setupHTTPServer() {
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	// WebSocket endpoint
	r.GET("/ws", e.handleWebSocket)

	// Stats endpoint
	r.GET("/stats", e.handleStats)

	// Emergency control endpoint
	r.POST("/emergency", e.handleEmergencyControl)

	// Client simulation endpoint
	r.POST("/simulate-clients", e.handleSimulateClients)

	// Start server
	go func() {
		if err := http.ListenAndServe(":8080", r); err != nil {
			e.logger.Error("HTTP server error", zap.Error(err))
		}
	}()
}

// handleWebSocket handles WebSocket connections
func (e *BackpressureExample) handleWebSocket(c *gin.Context) {
	conn, err := e.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		e.logger.Error("Failed to upgrade WebSocket connection", zap.Error(err))
		return
	}
	defer conn.Close()

	// Generate mock user ID
	userID := fmt.Sprintf("user_%d", rand.Int63())

	// Register client with enhanced hub
	if err := e.enhancedHub.RegisterClientEnhanced(conn, userID); err != nil {
		e.logger.Error("Failed to register client", zap.Error(err))
		return
	}
	defer e.enhancedHub.UnregisterClientEnhanced(conn)

	e.stats.mu.Lock()
	e.stats.TotalClients++
	e.stats.mu.Unlock()

	e.logger.Info("Client connected", zap.String("user_id", userID))

	// Handle client messages (basic ping/pong)
	for {
		messageType, p, err := conn.ReadMessage()
		if err != nil {
			e.logger.Debug("Client disconnected", zap.String("user_id", userID), zap.Error(err))
			break
		}

		if messageType == websocket.PingMessage {
			if err := conn.WriteMessage(websocket.PongMessage, p); err != nil {
				break
			}
		}
	}
}

// handleStats returns current system statistics
func (e *BackpressureExample) handleStats(c *gin.Context) {
	e.stats.mu.RLock()
	statsCopy := &PerformanceStats{
		StartTime:            e.stats.StartTime,
		TotalMessages:        e.stats.TotalMessages,
		TotalClients:         e.stats.TotalClients,
		EmergencyActivations: e.stats.EmergencyActivations,
		AverageLatency:       e.stats.AverageLatency,
		MessageRate:          e.stats.MessageRate,
		DropRate:             e.stats.DropRate,
	}
	e.stats.mu.RUnlock()

	response := map[string]interface{}{
		"system_stats": statsCopy,
		"uptime":       time.Since(statsCopy.StartTime).String(),
	}

	c.JSON(http.StatusOK, response)
}

// handleEmergencyControl manually controls emergency mode
func (e *BackpressureExample) handleEmergencyControl(c *gin.Context) {
	var request struct {
		Enable bool `json:"enable"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// TODO: e.enhancedHub.SetEmergencyMode(request.Enable)

	c.JSON(http.StatusOK, gin.H{
		"emergency_mode": request.Enable,
		"timestamp":      time.Now(),
	})
}

// handleSimulateClients creates multiple simulated client connections
func (e *BackpressureExample) handleSimulateClients(c *gin.Context) {
	var request struct {
		Count    int `json:"count"`
		Duration int `json:"duration_seconds"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	go e.simulateClients(request.Count, time.Duration(request.Duration)*time.Second)

	c.JSON(http.StatusOK, gin.H{
		"message": fmt.Sprintf("Simulating %d clients for %d seconds", request.Count, request.Duration),
	})
}

// generateMockData generates realistic market data for testing
func (e *BackpressureExample) generateMockData() {
	tradeTicker := time.NewTicker(time.Millisecond * 100)     // 10 trades per second
	orderbookTicker := time.NewTicker(time.Millisecond * 500) // 2 orderbook updates per second
	tickerTicker := time.NewTicker(time.Second * 5)           // Ticker every 5 seconds

	defer tradeTicker.Stop()
	defer orderbookTicker.Stop()
	defer tickerTicker.Stop()

	for {
		select {
		case <-tradeTicker.C:
			trade := e.mockTradeGen.GenerateTrade()
			if err := e.enhancedHub.BroadcastEnhanced("trade", trade); err != nil {
				e.logger.Debug("Failed to broadcast trade", zap.Error(err))
			} else {
				e.stats.mu.Lock()
				e.stats.TotalMessages++
				e.stats.mu.Unlock()
			}

		case <-orderbookTicker.C:
			orderbook := e.mockOrderbookGen.GenerateOrderbook()
			if err := e.enhancedHub.BroadcastEnhanced("orderbook", orderbook); err != nil {
				e.logger.Debug("Failed to broadcast orderbook", zap.Error(err))
			} else {
				e.stats.mu.Lock()
				e.stats.TotalMessages++
				e.stats.mu.Unlock()
			}

		case <-tickerTicker.C:
			ticker := e.mockTradeGen.GenerateTicker()
			if err := e.enhancedHub.BroadcastEnhanced("ticker", ticker); err != nil {
				e.logger.Debug("Failed to broadcast ticker", zap.Error(err))
			} else {
				e.stats.mu.Lock()
				e.stats.TotalMessages++
				e.stats.mu.Unlock()
			}
		}
	}
}

// monitorPerformance tracks system performance metrics
func (e *BackpressureExample) monitorPerformance() {
	ticker := time.NewTicker(e.statsInterval)
	defer ticker.Stop()

	var lastMessageCount int64

	for range ticker.C {
		e.stats.mu.Lock()
		currentMessages := e.stats.TotalMessages
		messagesThisPeriod := currentMessages - lastMessageCount
		e.stats.MessageRate = float64(messagesThisPeriod) / e.statsInterval.Seconds()
		lastMessageCount = currentMessages

		// TODO: if e.enhancedHub.IsEmergencyMode() {
		// 	e.stats.EmergencyActivations++
		// }
		e.stats.mu.Unlock()

		e.logger.Info("Performance stats",
			zap.Float64("message_rate", e.stats.MessageRate),
			zap.Int64("total_messages", currentMessages),
			zap.Int64("total_clients", e.stats.TotalClients),
			zap.Bool("emergency_mode", false)) // TODO: replace false with actual emergency mode status
	}
}

// simulateClients creates multiple WebSocket connections for load testing
func (e *BackpressureExample) simulateClients(count int, duration time.Duration) {
	e.logger.Info("Starting client simulation",
		zap.Int("count", count),
		zap.Duration("duration", duration))

	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(clientNum int) {
			defer wg.Done()
			e.simulateClient(clientNum, duration)
		}(i)

		// Stagger connection creation
		time.Sleep(time.Millisecond * 10)
	}

	wg.Wait()
	e.logger.Info("Client simulation completed")
}

// simulateClient simulates a single WebSocket client
func (e *BackpressureExample) simulateClient(clientNum int, duration time.Duration) {
	conn, _, err := websocket.DefaultDialer.Dial("ws://localhost:8080/ws", nil)
	if err != nil {
		e.logger.Error("Failed to connect simulated client", zap.Int("client", clientNum), zap.Error(err))
		return
	}
	defer conn.Close()

	// Send periodic pings
	ticker := time.NewTicker(time.Second * 30)
	defer ticker.Stop()

	timeout := time.After(duration)

	for {
		select {
		case <-timeout:
			return
		case <-ticker.C:
			if err := conn.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
				return
			}
		default:
			// Read messages
			conn.SetReadDeadline(time.Now().Add(time.Millisecond * 100))
			_, _, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway) {
					e.logger.Debug("Simulated client read error", zap.Int("client", clientNum), zap.Error(err))
				}
				// Continue reading
			}
		}
	}
}

// Mock implementations for testing

type MockAuthService struct{}

func (m *MockAuthService) AuthenticateUser(ctx context.Context, email, password string) (*auth.TokenPair, *models.User, error) {
	return nil, &models.User{
		ID:        uuid.New(),
		Email:     email,
		Username:  "mockuser",
		FirstName: "Mock",
		LastName:  "User",
		KYCStatus: "approved",
		Role:      "user",
		Tier:      "basic",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}, nil
}
func (m *MockAuthService) CheckMFA(ctx context.Context, userID uuid.UUID) (bool, error) {
	return false, nil
}
func (m *MockAuthService) ValidateToken(ctx context.Context, token string) (*auth.TokenClaims, error) {
	return &auth.TokenClaims{UserID: uuid.New()}, nil
}
func (m *MockAuthService) RefreshToken(ctx context.Context, refreshToken string) (*auth.TokenPair, error) {
	return nil, nil
}
func (m *MockAuthService) RevokeToken(ctx context.Context, tokenString string) error   { return nil }
func (m *MockAuthService) RevokeAllTokens(ctx context.Context, userID uuid.UUID) error { return nil }
func (m *MockAuthService) CreateAPIKey(ctx context.Context, userID uuid.UUID, name string, permissions []string, expiresAt *time.Time) (*auth.APIKey, error) {
	return nil, nil
}
func (m *MockAuthService) ValidateAPIKey(ctx context.Context, apiKey string) (*auth.APIKeyClaims, error) {
	return nil, nil
}
func (m *MockAuthService) RevokeAPIKey(ctx context.Context, keyID uuid.UUID) error { return nil }
func (m *MockAuthService) ListAPIKeys(ctx context.Context, userID uuid.UUID) ([]*auth.APIKey, error) {
	return nil, nil
}
func (m *MockAuthService) GenerateTOTPSecret(ctx context.Context, userID uuid.UUID) (*auth.TOTPSetup, error) {
	return nil, nil
}
func (m *MockAuthService) VerifyTOTPSetup(ctx context.Context, userID uuid.UUID, secret, token string) error {
	return nil
}
func (m *MockAuthService) VerifyTOTPToken(ctx context.Context, userID uuid.UUID, token string) error {
	return nil
}
func (m *MockAuthService) DisableTOTP(ctx context.Context, userID uuid.UUID, currentPassword string) error {
	return nil
}
func (m *MockAuthService) CreateSession(ctx context.Context, userID uuid.UUID, deviceFingerprint string) (*auth.Session, error) {
	return nil, nil
}
func (m *MockAuthService) ValidateSession(ctx context.Context, sessionID uuid.UUID) (*auth.Session, error) {
	return nil, nil
}
func (m *MockAuthService) InvalidateSession(ctx context.Context, sessionID uuid.UUID) error {
	return nil
}
func (m *MockAuthService) InvalidateAllSessions(ctx context.Context, userID uuid.UUID) error {
	return nil
}
func (m *MockAuthService) InitiateOAuthFlow(ctx context.Context, provider, redirectURI string) (*auth.OAuthState, error) {
	return nil, nil
}
func (m *MockAuthService) HandleOAuthCallback(ctx context.Context, state, code string) (*auth.TokenPair, error) {
	return nil, nil
}
func (m *MockAuthService) LinkOAuthAccount(ctx context.Context, userID uuid.UUID, provider, oauthUserID string) error {
	return nil
}
func (m *MockAuthService) ValidatePermission(ctx context.Context, userID uuid.UUID, resource, action string) error {
	return nil
}
func (m *MockAuthService) AssignRole(ctx context.Context, userID uuid.UUID, role string) error {
	return nil
}
func (m *MockAuthService) RevokeRole(ctx context.Context, userID uuid.UUID, role string) error {
	return nil
}
func (m *MockAuthService) GetUserPermissions(ctx context.Context, userID uuid.UUID) ([]auth.Permission, error) {
	return nil, nil
}
func (m *MockAuthService) RegisterTrustedDevice(ctx context.Context, userID uuid.UUID, deviceFingerprint string) error {
	return nil
}
func (m *MockAuthService) IsTrustedDevice(ctx context.Context, userID uuid.UUID, deviceFingerprint string) (bool, error) {
	return false, nil
}
func (m *MockAuthService) RevokeTrustedDevice(ctx context.Context, userID uuid.UUID, deviceFingerprint string) error {
	return nil
}

func NewMockTradeGenerator() *MockTradeGenerator {
	symbols := []string{"BTC/USD", "ETH/USD", "ADA/USD", "SOL/USD", "MATIC/USD"}
	prices := map[string]float64{
		"BTC/USD":   45000.0,
		"ETH/USD":   3200.0,
		"ADA/USD":   1.2,
		"SOL/USD":   95.0,
		"MATIC/USD": 0.85,
	}

	return &MockTradeGenerator{
		symbols: symbols,
		prices:  prices,
	}
}

func (m *MockTradeGenerator) GenerateTrade() *Trade {
	m.mu.Lock()
	defer m.mu.Unlock()

	symbol := m.symbols[rand.Intn(len(m.symbols))]
	basePrice := m.prices[symbol]

	// Add some random price movement
	priceChange := (rand.Float64() - 0.5) * basePrice * 0.001 // 0.1% max change
	newPrice := basePrice + priceChange
	m.prices[symbol] = newPrice

	return &Trade{
		Symbol:    symbol,
		Price:     newPrice,
		Quantity:  rand.Float64() * 10,
		Side:      []string{"buy", "sell"}[rand.Intn(2)],
		Timestamp: time.Now(),
		TradeID:   fmt.Sprintf("trade_%d", rand.Int63()),
	}
}

func (m *MockTradeGenerator) GenerateTicker() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	symbol := m.symbols[rand.Intn(len(m.symbols))]
	price := m.prices[symbol]

	return map[string]interface{}{
		"symbol":     symbol,
		"price":      price,
		"change_24h": (rand.Float64() - 0.5) * 0.1, // ±10%
		"volume_24h": rand.Float64() * 1000000,
		"timestamp":  time.Now(),
	}
}

func NewMockOrderbookGenerator() *MockOrderbookGenerator {
	symbols := []string{"BTC/USD", "ETH/USD", "ADA/USD", "SOL/USD", "MATIC/USD"}
	orderbooks := make(map[string]*MockOrderbook)

	for _, symbol := range symbols {
		orderbooks[symbol] = &MockOrderbook{
			Symbol: symbol,
			Bids:   generateRandomLevels(10, false),
			Asks:   generateRandomLevels(10, true),
		}
	}

	return &MockOrderbookGenerator{
		symbols:    symbols,
		orderbooks: orderbooks,
	}
}

func (m *MockOrderbookGenerator) GenerateOrderbook() *MockOrderbook {
	m.mu.Lock()
	defer m.mu.Unlock()

	symbol := m.symbols[rand.Intn(len(m.symbols))]
	orderbook := m.orderbooks[symbol]

	// Update timestamp
	orderbook.Timestamp = time.Now()

	// Occasionally update levels
	if rand.Float64() < 0.3 { // 30% chance to update
		orderbook.Bids = generateRandomLevels(10, false)
		orderbook.Asks = generateRandomLevels(10, true)
	}

	return orderbook
}

func generateRandomLevels(count int, isAsk bool) [][]float64 {
	levels := make([][]float64, count)
	basePrice := 45000.0 + rand.Float64()*1000

	for i := 0; i < count; i++ {
		var price float64
		if isAsk {
			price = basePrice + float64(i)*10
		} else {
			price = basePrice - float64(i)*10
		}
		quantity := rand.Float64() * 5
		levels[i] = []float64{price, quantity}
	}

	return levels
}
