// Comprehensive tests for enhanced MarketMaker observability system
package marketmaker

import (
	"context"
	"testing"
	"time"

	common "github.com/Aidin1998/finalex/internal/marketmaking/strategies/common"
	"github.com/Aidin1998/finalex/pkg/models"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// Mock implementations for testing

type MockTradingAPI struct {
	mock.Mock
}

func (m *MockTradingAPI) PlaceOrder(ctx context.Context, order *models.Order) (*models.Order, error) {
	args := m.Called(ctx, order)
	return args.Get(0).(*models.Order), args.Error(1)
}

func (m *MockTradingAPI) CancelOrder(ctx context.Context, orderID string) error {
	args := m.Called(ctx, orderID)
	return args.Error(0)
}

func (m *MockTradingAPI) GetOrderBook(pair string, depth int) (*models.OrderBookSnapshot, error) {
	args := m.Called(pair, depth)
	return args.Get(0).(*models.OrderBookSnapshot), args.Error(1)
}

func (m *MockTradingAPI) GetInventory(pair string) (decimal.Decimal, error) {
	args := m.Called(pair)
	return args.Get(0).(decimal.Decimal), args.Error(1)
}

func (m *MockTradingAPI) GetAccountBalance() (decimal.Decimal, error) {
	args := m.Called()
	return args.Get(0).(decimal.Decimal), args.Error(1)
}

func (m *MockTradingAPI) GetOpenOrders(pair string) ([]*models.Order, error) {
	args := m.Called(pair)
	return args.Get(0).([]*models.Order), args.Error(1)
}

func (m *MockTradingAPI) GetRecentTrades(pair string, limit int) ([]*models.Trade, error) {
	args := m.Called(pair, limit)
	return args.Get(0).([]*models.Trade), args.Error(1)
}

// Fix MarketData reference in MockTradingAPI
func (m *MockTradingAPI) GetMarketData(pair string) (*common.MarketData, error) {
	args := m.Called(pair)
	return args.Get(0).(*common.MarketData), args.Error(1)
}

func (m *MockTradingAPI) BatchCancelOrders(orderIDs []string) error {
	args := m.Called(orderIDs)
	return args.Error(0)
}

func (m *MockTradingAPI) GetPositionRisk(pair string) (*PositionRisk, error) {
	args := m.Called(pair)
	return args.Get(0).(*PositionRisk), args.Error(1)
}

func (m *MockTradingAPI) HealthCheck(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

type MockStrategy struct {
	mock.Mock
}

func (m *MockStrategy) GenerateSignals(ctx context.Context, marketData *EnhancedMarketData) ([]*models.Order, error) {
	args := m.Called(ctx, marketData)
	return args.Get(0).([]*models.Order), args.Error(1)
}

func (m *MockStrategy) UpdateParameters(params map[string]interface{}) error {
	args := m.Called(params)
	return args.Error(0)
}

func (m *MockStrategy) Description() string {
	args := m.Called()
	return args.String(0)
}
func (m *MockStrategy) Version() string {
	args := m.Called()
	return args.String(0)
}
func (m *MockStrategy) RiskLevel() common.RiskLevel {
	args := m.Called()
	return args.Get(0).(common.RiskLevel)
}
func (m *MockStrategy) GetConfig() common.StrategyConfig {
	args := m.Called()
	return args.Get(0).(common.StrategyConfig)
}
func (m *MockStrategy) GetMetrics() *common.StrategyMetrics {
	args := m.Called()
	return args.Get(0).(*common.StrategyMetrics)
}
func (m *MockStrategy) GetStatus() common.StrategyStatus {
	args := m.Called()
	return args.Get(0).(common.StrategyStatus)
}
func (m *MockStrategy) HealthCheck(ctx context.Context) *common.HealthStatus {
	args := m.Called(ctx)
	return args.Get(0).(*common.HealthStatus)
}
func (m *MockStrategy) Reset(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}
func (m *MockStrategy) Initialize(ctx context.Context, config common.StrategyConfig) error {
	args := m.Called(ctx, config)
	return args.Error(0)
}
func (m *MockStrategy) Start(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}
func (m *MockStrategy) Stop(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}
func (m *MockStrategy) OnMarketData(ctx context.Context, data *common.MarketData) error {
	args := m.Called(ctx, data)
	return args.Error(0)
}
func (m *MockStrategy) OnOrderFill(ctx context.Context, fill *common.OrderFill) error {
	args := m.Called(ctx, fill)
	return args.Error(0)
}
func (m *MockStrategy) OnOrderCancel(ctx context.Context, orderID uuid.UUID, reason string) error {
	args := m.Called(ctx, orderID, reason)
	return args.Error(0)
}
func (m *MockStrategy) UpdateConfig(ctx context.Context, config common.StrategyConfig) error {
	args := m.Called(ctx, config)
	return args.Error(0)
}
func (m *MockStrategy) Name() string {
	args := m.Called()
	return args.String(0)
}
func (m *MockStrategy) Quote(ctx context.Context, input common.QuoteInput) (*common.QuoteOutput, error) {
	args := m.Called(ctx, input)
	return args.Get(0).(*common.QuoteOutput), args.Error(1)
}

type MockFeedManager struct {
	mock.Mock
}

func (m *MockFeedManager) Reconnect(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockFeedManager) GetConnectionStatus() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockFeedManager) Reset() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockFeedManager) HealthCheck(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

// Test Enhanced Service Creation and Initialization

func TestNewEnhancedService(t *testing.T) {
	mockTradingAPI := &MockTradingAPI{}
	mockStrategy := &MockStrategy{}

	cfg := MarketMakerConfig{
		Pairs:        []string{"BTC-USD", "ETH-USD"},
		MinDepth:     100.0,
		TargetSpread: 0.1,
		MaxInventory: 1000.0,
	}

	observabilityConfig := &ObservabilityConfig{
		LogLevel:            "info",
		EnableTracing:       true,
		TraceSampleRate:     1.0,
		MetricsEnabled:      true,
		MetricsPort:         9090,
		HealthCheckInterval: 30 * time.Second,
		HealthTimeout:       10 * time.Second,
		EnableSelfHealing:   true,
		AdminAPIEnabled:     true,
		AdminAPIPort:        8080,
		BacktestEnabled:     true,
		EmergencyConfig: &EmergencyConfig{
			EnableEmergencyKill:     true,
			CircuitBreakerThreshold: 5,
			CircuitBreakerWindow:    5 * time.Minute,
			AutoRecoveryEnabled:     true,
			AutoRecoveryDelay:       30 * time.Second,
			MaxLossThreshold:        10000.0,
			PositionSizeThreshold:   50000.0,
		},
		SelfHealingConfig: &SelfHealingConfig{
			EnableSelfHealing:       true,
			MaxHealingAttempts:      3,
			HealingCooldown:         10 * time.Second,
			FeedReconnectDelay:      5 * time.Second,
			StrategyRestartDelay:    10 * time.Second,
			CircuitBreakerThreshold: 5,
			EmergencyKillEnabled:    true,
			AutoRecoveryEnabled:     true,
		},
	}

	enhancedService, err := NewEnhancedService(cfg, observabilityConfig, mockTradingAPI, mockStrategy)

	require.NoError(t, err)
	require.NotNil(t, enhancedService)

	// Verify components are initialized
	assert.NotNil(t, enhancedService.logger)
	assert.NotNil(t, enhancedService.metrics)
	assert.NotNil(t, enhancedService.healthMonitor)
	assert.NotNil(t, enhancedService.emergencyController)
	assert.NotNil(t, enhancedService.selfHealingManager)
	assert.NotNil(t, enhancedService.adminToolsManager)
	assert.NotNil(t, enhancedService.backtestEngine)

	// Verify configuration
	assert.Equal(t, observabilityConfig, enhancedService.observabilityConfig)
	assert.Equal(t, "initializing", enhancedService.operationalState.Status)
}

// Test Health Monitoring

func TestHealthMonitoring(t *testing.T) {
	mockTradingAPI := &MockTradingAPI{}
	mockStrategy := &MockStrategy{}

	// Set up successful health checks
	mockTradingAPI.On("HealthCheck", mock.Anything).Return(nil)
	mockStrategy.On("HealthCheck", mock.Anything).Return(nil)

	cfg := MarketMakerConfig{
		Pairs: []string{"BTC-USD"},
	}

	observabilityConfig := &ObservabilityConfig{
		LogLevel:            "info",
		EnableTracing:       false,
		MetricsEnabled:      true,
		HealthCheckInterval: 100 * time.Millisecond,
		HealthTimeout:       1 * time.Second,
		EnableSelfHealing:   true,
		EmergencyConfig:     &EmergencyConfig{EnableEmergencyKill: false},
		SelfHealingConfig:   &SelfHealingConfig{EnableSelfHealing: false},
	}

	enhancedService, err := NewEnhancedService(cfg, observabilityConfig, mockTradingAPI, mockStrategy)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start the service
	err = enhancedService.Start(ctx)
	require.NoError(t, err)

	// Wait for a few health checks
	time.Sleep(300 * time.Millisecond)

	// Verify health check was called
	mockTradingAPI.AssertCalled(t, "HealthCheck", mock.Anything)
	mockStrategy.AssertCalled(t, "HealthCheck", mock.Anything)

	// Check operational state
	state := enhancedService.GetOperationalState()
	assert.Equal(t, "running", state.Status)
	assert.NotEmpty(t, state.HealthStatus)

	// Stop the service
	err = enhancedService.Stop(ctx)
	require.NoError(t, err)
}

// Test Emergency Kill Switch

func TestEmergencyKillSwitch(t *testing.T) {
	mockTradingAPI := &MockTradingAPI{}
	mockStrategy := &MockStrategy{}

	cfg := MarketMakerConfig{
		Pairs: []string{"BTC-USD"},
	}

	observabilityConfig := &ObservabilityConfig{
		LogLevel:          "info",
		EnableTracing:     false,
		MetricsEnabled:    true,
		EnableSelfHealing: false,
		EmergencyConfig: &EmergencyConfig{
			EnableEmergencyKill:     true,
			CircuitBreakerThreshold: 5,
			AutoRecoveryEnabled:     false,
			MaxLossThreshold:        1000.0,
		},
		SelfHealingConfig: &SelfHealingConfig{
			EnableSelfHealing: false,
		},
	}

	enhancedService, err := NewEnhancedService(cfg, observabilityConfig, mockTradingAPI, mockStrategy)
	require.NoError(t, err)

	ctx := context.Background()

	// Test manual emergency kill
	assert.False(t, enhancedService.IsInEmergencyState())

	err = enhancedService.TriggerManualEmergencyKill(ctx, "test emergency")
	require.NoError(t, err)

	assert.True(t, enhancedService.IsInEmergencyState())

	// Verify emergency status
	status := enhancedService.emergencyController.GetStatus()
	assert.True(t, status["is_killed"].(bool))

	// Test recovery (should work since auto-recovery is disabled and we can manually recover)
	enhancedService.observabilityConfig.EmergencyConfig.AutoRecoveryEnabled = true
	err = enhancedService.AttemptRecovery(ctx)
	require.NoError(t, err)

	assert.False(t, enhancedService.IsInEmergencyState())
}

// Test Self-Healing

func TestSelfHealing(t *testing.T) {
	mockTradingAPI := &MockTradingAPI{}
	mockStrategy := &MockStrategy{}
	mockFeedManager := &MockFeedManager{}

	// Set up feed manager to fail health check, then succeed on healing
	mockFeedManager.On("HealthCheck", mock.Anything).Return(assert.AnError).Once()
	mockFeedManager.On("Reset").Return(nil)
	mockFeedManager.On("Reconnect", mock.Anything).Return(nil)
	mockFeedManager.On("GetConnectionStatus").Return("connected")

	cfg := MarketMakerConfig{
		Pairs: []string{"BTC-USD"},
	}

	observabilityConfig := &ObservabilityConfig{
		LogLevel:          "info",
		EnableTracing:     false,
		MetricsEnabled:    true,
		EnableSelfHealing: true,
		SelfHealingConfig: &SelfHealingConfig{
			EnableSelfHealing:  true,
			MaxHealingAttempts: 3,
			HealingCooldown:    100 * time.Millisecond,
			FeedReconnectDelay: 50 * time.Millisecond,
		},
		EmergencyConfig: &EmergencyConfig{
			EnableEmergencyKill: false,
		},
	}

	enhancedService, err := NewEnhancedService(cfg, observabilityConfig, mockTradingAPI, mockStrategy)
	require.NoError(t, err)

	// Register feed healer
	feedHealer := NewFeedSelfHealer(mockFeedManager, observabilityConfig.SelfHealingConfig, enhancedService.logger, enhancedService.metrics)
	enhancedService.selfHealingManager.RegisterHealer("feed", feedHealer)

	ctx := context.Background()

	// Trigger self-healing
	err = enhancedService.selfHealingManager.TriggerHealing(ctx, "feed", "connection_lost")
	require.NoError(t, err)

	// Verify healing methods were called
	mockFeedManager.AssertCalled(t, "Reset")
	mockFeedManager.AssertCalled(t, "Reconnect", mock.Anything)

	// Check healing history
	history := enhancedService.selfHealingManager.GetHealingHistory()
	assert.Len(t, history, 1)
	assert.Equal(t, "feed", history[0].Component)
	assert.Equal(t, "connection_lost", history[0].Reason)
	assert.True(t, history[0].Success)
}

// Test Circuit Breaker

func TestCircuitBreaker(t *testing.T) {
	config := CircuitBreakerConfig{
		Threshold: 3,
		Window:    5 * time.Second,
		Timeout:   1 * time.Second,
	}

	cb := NewCircuitBreaker(config)

	// Initially closed
	assert.Equal(t, CircuitBreakerClosed, cb.GetState())
	assert.True(t, cb.CanExecute())

	// Record failures to trip the circuit breaker
	for i := 0; i < 3; i++ {
		cb.RecordError()
	}

	// Should be open now
	assert.Equal(t, CircuitBreakerOpen, cb.GetState())
	assert.False(t, cb.CanExecute())

	// Wait for timeout and try again
	time.Sleep(1100 * time.Millisecond)
	assert.True(t, cb.CanExecute()) // Should allow execution (half-open)

	// Record success to close the circuit
	cb.RecordSuccess()
	assert.Equal(t, CircuitBreakerClosed, cb.GetState())
}

// Test Metrics Collection

func TestMetricsCollection(t *testing.T) {
	metrics := NewMetricsCollector()

	ctx := context.Background()
	err := metrics.Start(ctx)
	require.NoError(t, err)

	// Record some metrics
	metrics.RecordOrderPlacement("BTC-USD", "buy", "limit", 50000.0, 100.0)
	metrics.RecordOrderExecution("BTC-USD", "buy", 50000.0, 100.0, 150*time.Millisecond)
	// The following are not implemented in MetricsCollector, so comment/remove them:
	// metrics.RecordOrderCancelled("BTC-USD", "sell", "user_request")
	// metrics.RecordInventoryChange("BTC-USD", 100.0, 5000000.0)
	metrics.RecordStrategyPnL("basic_mm", 1250.50)
	metrics.RecordRiskEvent("position_limit", 75000.0)
	// metrics.RecordFeedHealth("binance", "healthy", 99.9)

	// Verify metrics were recorded (in a real test, you'd check Prometheus metrics)
	// For now, just verify no panics occurred

	metrics.Stop(ctx)
}

func TestStructuredLogging(t *testing.T) {
	logger, err := NewStructuredLogger("test-service", "test-version")
	require.NoError(t, err)

	ctx := context.Background()
	traceID := NewTraceID()
	ctx = WithTraceID(ctx, traceID)

	// Test various log methods
	logger.LogInfo(ctx, "test info message", map[string]interface{}{
		"key1": "value1",
		"key2": 123,
	})

	logger.LogError(ctx, "test error message", map[string]interface{}{
		"error": "test error",
	})

	// Use LogOrderPlaced instead of LogOrderEvent
	logger.LogOrderPlaced(ctx, "order123", "BTC-USD", "buy", "limit", 50000.0, 1.0)

	// Use LogInventoryChange with correct argument types
	logger.LogInventoryChange(ctx, "BTC-USD", 100.0, 5000000.0, "order_fill")

	logger.LogStrategyEvent(ctx, "basic_mm", "signal_generated", map[string]interface{}{
		"signal_type": "buy",
		"confidence":  0.85,
	})

	// Verify no panics occurred during logging
	assert.NotEmpty(t, traceID)
}

// Test Admin Tools

func TestAdminTools(t *testing.T) {
	// ...existing code...
}
