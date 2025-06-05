//go:build trading

package test

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Aidin1998/finalex/internal/trading"
	"github.com/Aidin1998/finalex/pkg/models"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/suite"
)

// TradingMonitoringTestSuite provides comprehensive monitoring and fault tolerance testing
type TradingMonitoringTestSuite struct {
	suite.Suite
	service          trading.Service
	mockBookkeeper   *MockBookkeeperMonitoring
	mockWSHub        *MockWSHubMonitoring
	metricsCollector *MetricsCollector
	healthChecker    *HealthChecker
	alertManager     *AlertManager
	ctx              context.Context
	cancel           context.CancelFunc
}

// MetricsCollector collects and tracks various trading metrics
type MetricsCollector struct {
	orderMetrics       OrderMetrics
	performanceMetrics PerformanceMetrics
	errorMetrics       ErrorMetrics
	systemMetrics      SystemMetrics
	mu                 sync.RWMutex
}

type OrderMetrics struct {
	TotalOrders       int64
	SuccessfulOrders  int64
	FailedOrders      int64
	CancelledOrders   int64
	OrdersByType      map[models.OrderType]int64
	OrdersBySide      map[models.OrderSide]int64
	OrdersByPair      map[string]int64
	AverageOrderValue decimal.Decimal
}

type PerformanceMetrics struct {
	AverageLatency  time.Duration
	MaxLatency      time.Duration
	MinLatency      time.Duration
	ThroughputTPS   float64
	ConcurrentUsers int64
	MemoryUsage     int64
	CPUUsage        float64
}

type ErrorMetrics struct {
	NetworkErrors       int64
	DatabaseErrors      int64
	ValidationErrors    int64
	TimeoutErrors       int64
	AuthErrors          int64
	BusinessLogicErrors int64
	ErrorsByType        map[string]int64
}

type SystemMetrics struct {
	ActiveConnections   int64
	QueueLength         int64
	CircuitBreakerState string
	HealthScore         float64
	Uptime              time.Duration
	LastHealthCheck     time.Time
}

func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		orderMetrics: OrderMetrics{
			OrdersByType: make(map[models.OrderType]int64),
			OrdersBySide: make(map[models.OrderSide]int64),
			OrdersByPair: make(map[string]int64),
		},
		errorMetrics: ErrorMetrics{
			ErrorsByType: make(map[string]int64),
		},
		performanceMetrics: PerformanceMetrics{
			MinLatency: time.Hour, // Initialize to high value
		},
	}
}

func (m *MetricsCollector) RecordOrder(order *models.PlaceOrderRequest, success bool, latency time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	atomic.AddInt64(&m.orderMetrics.TotalOrders, 1)

	if success {
		atomic.AddInt64(&m.orderMetrics.SuccessfulOrders, 1)
	} else {
		atomic.AddInt64(&m.orderMetrics.FailedOrders, 1)
	}

	m.orderMetrics.OrdersByType[order.Type]++
	m.orderMetrics.OrdersBySide[order.Side]++
	m.orderMetrics.OrdersByPair[order.Pair]++

	// Update latency metrics
	if latency > m.performanceMetrics.MaxLatency {
		m.performanceMetrics.MaxLatency = latency
	}
	if latency < m.performanceMetrics.MinLatency {
		m.performanceMetrics.MinLatency = latency
	}
}

func (m *MetricsCollector) RecordError(errorType string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.errorMetrics.ErrorsByType[errorType]++

	switch errorType {
	case "network":
		atomic.AddInt64(&m.errorMetrics.NetworkErrors, 1)
	case "database":
		atomic.AddInt64(&m.errorMetrics.DatabaseErrors, 1)
	case "validation":
		atomic.AddInt64(&m.errorMetrics.ValidationErrors, 1)
	case "timeout":
		atomic.AddInt64(&m.errorMetrics.TimeoutErrors, 1)
	case "auth":
		atomic.AddInt64(&m.errorMetrics.AuthErrors, 1)
	case "business":
		atomic.AddInt64(&m.errorMetrics.BusinessLogicErrors, 1)
	}
}

func (m *MetricsCollector) UpdateSystemMetrics(connections, queueLength int64, healthScore float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.systemMetrics.ActiveConnections = connections
	m.systemMetrics.QueueLength = queueLength
	m.systemMetrics.HealthScore = healthScore
	m.systemMetrics.LastHealthCheck = time.Now()
}

func (m *MetricsCollector) GetMetrics() (OrderMetrics, PerformanceMetrics, ErrorMetrics, SystemMetrics) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.orderMetrics, m.performanceMetrics, m.errorMetrics, m.systemMetrics
}

// HealthChecker monitors system health
type HealthChecker struct {
	checks          map[string]HealthCheck
	lastResults     map[string]HealthResult
	alertThresholds map[string]float64
	mu              sync.RWMutex
}

type HealthCheck func() HealthResult

type HealthResult struct {
	Status    string  // "healthy", "warning", "critical"
	Score     float64 // 0.0 to 1.0
	Message   string
	Timestamp time.Time
	Latency   time.Duration
}

func NewHealthChecker() *HealthChecker {
	return &HealthChecker{
		checks:      make(map[string]HealthCheck),
		lastResults: make(map[string]HealthResult),
		alertThresholds: map[string]float64{
			"response_time": 0.7, // Alert if response time health drops below 70%
			"error_rate":    0.8, // Alert if error rate health drops below 80%
			"throughput":    0.6, // Alert if throughput health drops below 60%
		},
	}
}

func (h *HealthChecker) RegisterCheck(name string, check HealthCheck) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.checks[name] = check
}

func (h *HealthChecker) RunChecks() map[string]HealthResult {
	h.mu.Lock()
	defer h.mu.Unlock()

	results := make(map[string]HealthResult)

	for name, check := range h.checks {
		start := time.Now()
		result := check()
		result.Latency = time.Since(start)
		result.Timestamp = time.Now()

		results[name] = result
		h.lastResults[name] = result
	}

	return results
}

func (h *HealthChecker) GetOverallHealth() HealthResult {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if len(h.lastResults) == 0 {
		return HealthResult{
			Status:  "unknown",
			Score:   0.0,
			Message: "No health checks available",
		}
	}

	totalScore := 0.0
	criticalCount := 0
	warningCount := 0

	for _, result := range h.lastResults {
		totalScore += result.Score

		if result.Status == "critical" {
			criticalCount++
		} else if result.Status == "warning" {
			warningCount++
		}
	}

	averageScore := totalScore / float64(len(h.lastResults))

	status := "healthy"
	if criticalCount > 0 {
		status = "critical"
	} else if warningCount > 0 || averageScore < 0.7 {
		status = "warning"
	}

	return HealthResult{
		Status:    status,
		Score:     averageScore,
		Message:   fmt.Sprintf("Overall health: %d checks, %d critical, %d warning", len(h.lastResults), criticalCount, warningCount),
		Timestamp: time.Now(),
	}
}

// AlertManager handles alerting based on metrics and health checks
type AlertManager struct {
	alerts        []Alert
	alertRules    []AlertRule
	notifications chan AlertNotification
	mu            sync.RWMutex
}

type Alert struct {
	ID          string
	Severity    string
	Title       string
	Description string
	Timestamp   time.Time
	Resolved    bool
	ResolvedAt  time.Time
}

type AlertRule struct {
	Name        string
	Condition   func(*MetricsCollector) bool
	Severity    string
	Description string
}

type AlertNotification struct {
	Alert     Alert
	Action    string // "triggered", "resolved"
	Timestamp time.Time
}

func NewAlertManager() *AlertManager {
	return &AlertManager{
		alerts:        make([]Alert, 0),
		alertRules:    make([]AlertRule, 0),
		notifications: make(chan AlertNotification, 100),
	}
}

func (a *AlertManager) AddRule(rule AlertRule) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.alertRules = append(a.alertRules, rule)
}

func (a *AlertManager) CheckRules(metrics *MetricsCollector) {
	a.mu.Lock()
	defer a.mu.Unlock()

	for _, rule := range a.alertRules {
		if rule.Condition(metrics) {
			alert := Alert{
				ID:          fmt.Sprintf("%s_%d", rule.Name, time.Now().UnixNano()),
				Severity:    rule.Severity,
				Title:       rule.Name,
				Description: rule.Description,
				Timestamp:   time.Now(),
			}

			a.alerts = append(a.alerts, alert)

			select {
			case a.notifications <- AlertNotification{
				Alert:     alert,
				Action:    "triggered",
				Timestamp: time.Now(),
			}:
			default:
				// Channel full, skip notification
			}
		}
	}
}

func (a *AlertManager) GetActiveAlerts() []Alert {
	a.mu.RLock()
	defer a.mu.RUnlock()

	active := make([]Alert, 0)
	for _, alert := range a.alerts {
		if !alert.Resolved {
			active = append(active, alert)
		}
	}
	return active
}

// MockBookkeeperMonitoring provides monitoring-focused mock
type MockBookkeeperMonitoring struct {
	operations       int64
	errors           int64
	lastResponseTime time.Duration
	circuitOpen      bool
	degradedMode     bool
	mu               sync.RWMutex
}

func NewMockBookkeeperMonitoring() *MockBookkeeperMonitoring {
	return &MockBookkeeperMonitoring{}
}

func (m *MockBookkeeperMonitoring) GetBalance(userID, asset string) (decimal.Decimal, error) {
	start := time.Now()
	defer func() {
		m.lastResponseTime = time.Since(start)
		atomic.AddInt64(&m.operations, 1)
	}()

	if m.circuitOpen {
		atomic.AddInt64(&m.errors, 1)
		return decimal.Zero, fmt.Errorf("circuit breaker open")
	}

	if m.degradedMode {
		time.Sleep(100 * time.Millisecond) // Simulate slow response
	}

	return decimal.NewFromInt(10000), nil
}

func (m *MockBookkeeperMonitoring) ReserveBalance(userID, asset string, amount decimal.Decimal) (string, error) {
	start := time.Now()
	defer func() {
		m.lastResponseTime = time.Since(start)
		atomic.AddInt64(&m.operations, 1)
	}()

	if m.circuitOpen {
		atomic.AddInt64(&m.errors, 1)
		return "", fmt.Errorf("circuit breaker open")
	}

	return fmt.Sprintf("monitoring_res_%s_%d", userID, time.Now().UnixNano()), nil
}

func (m *MockBookkeeperMonitoring) ReleaseReservation(reservationID string) error {
	atomic.AddInt64(&m.operations, 1)
	return nil
}

func (m *MockBookkeeperMonitoring) TransferReservedBalance(reservationID, toUserID string) error {
	atomic.AddInt64(&m.operations, 1)
	return nil
}

func (m *MockBookkeeperMonitoring) SetCircuitOpen(open bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.circuitOpen = open
}

func (m *MockBookkeeperMonitoring) SetDegradedMode(degraded bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.degradedMode = degraded
}

func (m *MockBookkeeperMonitoring) GetOperations() int64 {
	return atomic.LoadInt64(&m.operations)
}

func (m *MockBookkeeperMonitoring) GetErrors() int64 {
	return atomic.LoadInt64(&m.errors)
}

func (m *MockBookkeeperMonitoring) GetLastResponseTime() time.Duration {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastResponseTime
}

// MockWSHubMonitoring provides monitoring-focused WebSocket mock
type MockWSHubMonitoring struct {
	messagesSent     int64
	messagesDropped  int64
	connections      int64
	connectionErrors int64
}

func NewMockWSHubMonitoring() *MockWSHubMonitoring {
	return &MockWSHubMonitoring{}
}

func (m *MockWSHubMonitoring) PublishToUser(userID string, data interface{}) {
	atomic.AddInt64(&m.messagesSent, 1)
}

func (m *MockWSHubMonitoring) PublishToTopic(topic string, data interface{}) {
	atomic.AddInt64(&m.messagesSent, 1)
}

func (m *MockWSHubMonitoring) SubscribeToTopic(userID, topic string) {
	atomic.AddInt64(&m.connections, 1)
}

func (m *MockWSHubMonitoring) GetMessagesSent() int64 {
	return atomic.LoadInt64(&m.messagesSent)
}

func (m *MockWSHubMonitoring) GetConnections() int64 {
	return atomic.LoadInt64(&m.connections)
}

func (suite *TradingMonitoringTestSuite) SetupSuite() {
	log.Println("Setting up trading monitoring test suite...")

	suite.ctx, suite.cancel = context.WithTimeout(context.Background(), 10*time.Minute)

	suite.mockBookkeeper = NewMockBookkeeperMonitoring()
	suite.mockWSHub = NewMockWSHubMonitoring()
	suite.metricsCollector = NewMetricsCollector()
	suite.healthChecker = NewHealthChecker()
	suite.alertManager = NewAlertManager()

	suite.service = trading.NewService(
		suite.mockBookkeeper,
		suite.mockWSHub,
		trading.WithMonitoring(true),
		trading.WithMetricsCollection(true),
		trading.WithHealthChecks(true),
	)

	suite.setupHealthChecks()
	suite.setupAlertRules()

	err := suite.service.Start(suite.ctx)
	suite.Require().NoError(err, "Failed to start trading service")
}

func (suite *TradingMonitoringTestSuite) TearDownSuite() {
	if suite.cancel != nil {
		suite.cancel()
	}
	if suite.service != nil {
		suite.service.Stop()
	}
}

func (suite *TradingMonitoringTestSuite) setupHealthChecks() {
	// Response time health check
	suite.healthChecker.RegisterCheck("response_time", func() HealthResult {
		responseTime := suite.mockBookkeeper.GetLastResponseTime()

		score := 1.0
		status := "healthy"

		if responseTime > 100*time.Millisecond {
			score = 0.5
			status = "warning"
		}
		if responseTime > 500*time.Millisecond {
			score = 0.2
			status = "critical"
		}

		return HealthResult{
			Status:  status,
			Score:   score,
			Message: fmt.Sprintf("Response time: %v", responseTime),
		}
	})

	// Error rate health check
	suite.healthChecker.RegisterCheck("error_rate", func() HealthResult {
		operations := suite.mockBookkeeper.GetOperations()
		errors := suite.mockBookkeeper.GetErrors()

		if operations == 0 {
			return HealthResult{
				Status:  "healthy",
				Score:   1.0,
				Message: "No operations yet",
			}
		}

		errorRate := float64(errors) / float64(operations)
		score := 1.0 - errorRate

		status := "healthy"
		if errorRate > 0.05 {
			status = "warning"
		}
		if errorRate > 0.15 {
			status = "critical"
		}

		return HealthResult{
			Status:  status,
			Score:   score,
			Message: fmt.Sprintf("Error rate: %.2f%% (%d/%d)", errorRate*100, errors, operations),
		}
	})

	// Throughput health check
	suite.healthChecker.RegisterCheck("throughput", func() HealthResult {
		operations := suite.mockBookkeeper.GetOperations()

		// Simple throughput check - could be more sophisticated
		score := 1.0
		status := "healthy"
		message := fmt.Sprintf("Operations: %d", operations)

		if operations < 100 {
			score = 0.8
			status = "warning"
		}

		return HealthResult{
			Status:  status,
			Score:   score,
			Message: message,
		}
	})
}

func (suite *TradingMonitoringTestSuite) setupAlertRules() {
	// High error rate alert
	suite.alertManager.AddRule(AlertRule{
		Name:     "HighErrorRate",
		Severity: "critical",
		Condition: func(m *MetricsCollector) bool {
			_, _, errorMetrics, _ := m.GetMetrics()
			totalErrors := errorMetrics.NetworkErrors + errorMetrics.DatabaseErrors + errorMetrics.ValidationErrors
			return totalErrors > 10
		},
		Description: "Error rate is too high",
	})

	// Low throughput alert
	suite.alertManager.AddRule(AlertRule{
		Name:     "LowThroughput",
		Severity: "warning",
		Condition: func(m *MetricsCollector) bool {
			_, perfMetrics, _, _ := m.GetMetrics()
			return perfMetrics.ThroughputTPS < 100
		},
		Description: "Throughput is below expected threshold",
	})

	// High latency alert
	suite.alertManager.AddRule(AlertRule{
		Name:     "HighLatency",
		Severity: "warning",
		Condition: func(m *MetricsCollector) bool {
			_, perfMetrics, _, _ := m.GetMetrics()
			return perfMetrics.AverageLatency > 100*time.Millisecond
		},
		Description: "Average latency is too high",
	})
}

// TestMetricsCollection tests comprehensive metrics collection
func (suite *TradingMonitoringTestSuite) TestMetricsCollection() {
	log.Println("Testing metrics collection...")

	const operationCount = 100
	userID := uuid.New()

	// Perform various operations to generate metrics
	for i := 0; i < operationCount; i++ {
		start := time.Now()

		order := &models.PlaceOrderRequest{
			UserID:   userID,
			Pair:     "BTC/USDT",
			Side:     []models.OrderSide{models.Buy, models.Sell}[i%2],
			Type:     []models.OrderType{models.Market, models.Limit}[i%2],
			Price:    decimal.NewFromInt(50000 + int64(i)),
			Quantity: decimal.NewFromFloat(0.01 + float64(i)*0.001),
		}

		_, err := suite.service.PlaceOrder(suite.ctx, order)
		latency := time.Since(start)

		// Record metrics
		suite.metricsCollector.RecordOrder(order, err == nil, latency)

		if err != nil {
			suite.metricsCollector.RecordError("business")
		}
	}

	// Verify metrics collection
	orderMetrics, perfMetrics, errorMetrics, _ := suite.metricsCollector.GetMetrics()

	suite.Assert().Equal(int64(operationCount), orderMetrics.TotalOrders, "Should record all orders")
	suite.Assert().True(orderMetrics.SuccessfulOrders > 0, "Should have some successful orders")
	suite.Assert().True(perfMetrics.MaxLatency > 0, "Should record latency metrics")

	// Check order type distribution
	totalByType := int64(0)
	for _, count := range orderMetrics.OrdersByType {
		totalByType += count
	}
	suite.Assert().Equal(int64(operationCount), totalByType, "Order type distribution should match total")

	log.Printf("Metrics collection results:")
	log.Printf("Total orders: %d", orderMetrics.TotalOrders)
	log.Printf("Successful: %d", orderMetrics.SuccessfulOrders)
	log.Printf("Failed: %d", orderMetrics.FailedOrders)
	log.Printf("Max latency: %v", perfMetrics.MaxLatency)
	log.Printf("Total errors: %d", errorMetrics.BusinessLogicErrors)
}

// TestHealthChecks tests health check functionality
func (suite *TradingMonitoringTestSuite) TestHealthChecks() {
	log.Println("Testing health checks...")

	// Run initial health checks
	results := suite.healthChecker.RunChecks()
	suite.Assert().True(len(results) > 0, "Should have health check results")

	// Verify all checks ran
	expectedChecks := []string{"response_time", "error_rate", "throughput"}
	for _, checkName := range expectedChecks {
		result, exists := results[checkName]
		suite.Assert().True(exists, "Should have result for check: %s", checkName)
		suite.Assert().True(result.Score >= 0 && result.Score <= 1, "Score should be between 0 and 1")
		suite.Assert().NotEmpty(result.Status, "Should have status")
		suite.Assert().NotEmpty(result.Message, "Should have message")
	}

	// Test overall health
	overallHealth := suite.healthChecker.GetOverallHealth()
	suite.Assert().NotEmpty(overallHealth.Status, "Should have overall health status")
	suite.Assert().True(overallHealth.Score >= 0 && overallHealth.Score <= 1, "Overall score should be valid")

	log.Printf("Health check results:")
	for name, result := range results {
		log.Printf("%s: %s (%.2f) - %s", name, result.Status, result.Score, result.Message)
	}
	log.Printf("Overall health: %s (%.2f) - %s", overallHealth.Status, overallHealth.Score, overallHealth.Message)
}

// TestAlertSystem tests alerting functionality
func (suite *TradingMonitoringTestSuite) TestAlertSystem() {
	log.Println("Testing alert system...")

	userID := uuid.New()

	// Generate some errors to trigger alerts
	for i := 0; i < 15; i++ {
		suite.metricsCollector.RecordError("network")
	}

	// Check alert rules
	suite.alertManager.CheckRules(suite.metricsCollector)

	// Verify alerts were generated
	activeAlerts := suite.alertManager.GetActiveAlerts()
	suite.Assert().True(len(activeAlerts) > 0, "Should have generated alerts")

	// Check for high error rate alert
	foundErrorAlert := false
	for _, alert := range activeAlerts {
		if alert.Title == "HighErrorRate" {
			foundErrorAlert = true
			suite.Assert().Equal("critical", alert.Severity, "Error rate alert should be critical")
			break
		}
	}
	suite.Assert().True(foundErrorAlert, "Should have generated high error rate alert")

	log.Printf("Generated %d alerts", len(activeAlerts))
	for _, alert := range activeAlerts {
		log.Printf("Alert: %s [%s] - %s", alert.Title, alert.Severity, alert.Description)
	}
}

// TestCircuitBreakerMonitoring tests circuit breaker monitoring
func (suite *TradingMonitoringTestSuite) TestCircuitBreakerMonitoring() {
	log.Println("Testing circuit breaker monitoring...")

	userID := uuid.New()

	// Open circuit breaker
	suite.mockBookkeeper.SetCircuitOpen(true)

	order := &models.PlaceOrderRequest{
		UserID:   userID,
		Pair:     "BTC/USDT",
		Side:     models.Buy,
		Type:     models.Limit,
		Price:    decimal.NewFromInt(50000),
		Quantity: decimal.NewFromFloat(0.1),
	}

	// Attempt operations with circuit breaker open
	for i := 0; i < 5; i++ {
		_, err := suite.service.PlaceOrder(suite.ctx, order)
		suite.Assert().Error(err, "Should fail with circuit breaker open")
		suite.metricsCollector.RecordError("network")
	}

	// Run health checks to detect circuit breaker issues
	results := suite.healthChecker.RunChecks()
	errorRateResult := results["error_rate"]

	suite.Assert().True(errorRateResult.Score < 0.8, "Error rate health should be degraded")

	// Close circuit breaker
	suite.mockBookkeeper.SetCircuitOpen(false)

	// Verify recovery
	_, err := suite.service.PlaceOrder(suite.ctx, order)
	if err == nil {
		log.Println("Circuit breaker recovery successful")
	}

	log.Printf("Circuit breaker monitoring completed - Error rate health: %.2f", errorRateResult.Score)
}

// TestPerformanceDegradationDetection tests detection of performance issues
func (suite *TradingMonitoringTestSuite) TestPerformanceDegradationDetection() {
	log.Println("Testing performance degradation detection...")

	userID := uuid.New()

	// Normal operation baseline
	normalLatencies := make([]time.Duration, 0)
	for i := 0; i < 10; i++ {
		start := time.Now()
		order := &models.PlaceOrderRequest{
			UserID:   userID,
			Pair:     "BTC/USDT",
			Side:     models.Buy,
			Type:     models.Limit,
			Price:    decimal.NewFromInt(50000),
			Quantity: decimal.NewFromFloat(0.01),
		}
		suite.service.PlaceOrder(suite.ctx, order)
		normalLatencies = append(normalLatencies, time.Since(start))
	}

	// Enable degraded mode
	suite.mockBookkeeper.SetDegradedMode(true)

	// Test degraded performance
	degradedLatencies := make([]time.Duration, 0)
	for i := 0; i < 5; i++ {
		start := time.Now()
		order := &models.PlaceOrderRequest{
			UserID:   userID,
			Pair:     "BTC/USDT",
			Side:     models.Buy,
			Type:     models.Limit,
			Price:    decimal.NewFromInt(50000),
			Quantity: decimal.NewFromFloat(0.01),
		}
		suite.service.PlaceOrder(suite.ctx, order)
		degradedLatencies = append(degradedLatencies, time.Since(start))
	}

	// Run health checks
	results := suite.healthChecker.RunChecks()
	responseTimeResult := results["response_time"]

	// Should detect performance degradation
	suite.Assert().True(responseTimeResult.Score < 1.0, "Should detect performance degradation")

	// Disable degraded mode
	suite.mockBookkeeper.SetDegradedMode(false)

	log.Printf("Performance degradation test completed")
	log.Printf("Response time health score: %.2f", responseTimeResult.Score)
}

// TestContinuousMonitoring tests continuous monitoring over time
func (suite *TradingMonitoringTestSuite) TestContinuousMonitoring() {
	log.Println("Testing continuous monitoring...")

	const monitoringDuration = 10 * time.Second
	const checkInterval = 1 * time.Second
	ctx, cancel := context.WithTimeout(suite.ctx, monitoringDuration)
	defer cancel()

	userID := uuid.New()

	// Start continuous operations
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				order := &models.PlaceOrderRequest{
					UserID:   userID,
					Pair:     "BTC/USDT",
					Side:     models.Buy,
					Type:     models.Limit,
					Price:    decimal.NewFromInt(50000),
					Quantity: decimal.NewFromFloat(0.01),
				}

				start := time.Now()
				_, err := suite.service.PlaceOrder(ctx, order)
				latency := time.Since(start)

				suite.metricsCollector.RecordOrder(order, err == nil, latency)
				if err != nil {
					suite.metricsCollector.RecordError("business")
				}
			}
		}
	}()

	// Start continuous monitoring
	healthResults := make([]HealthResult, 0)
	alertCounts := make([]int, 0)

	monitorTicker := time.NewTicker(checkInterval)
	defer monitorTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			wg.Wait()
			log.Printf("Continuous monitoring completed")
			log.Printf("Health checks performed: %d", len(healthResults))
			log.Printf("Alert checks performed: %d", len(alertCounts))

			// Verify continuous monitoring worked
			suite.Assert().True(len(healthResults) > 0, "Should have performed health checks")
			suite.Assert().True(len(alertCounts) > 0, "Should have performed alert checks")
			return

		case <-monitorTicker.C:
			// Run health checks
			overallHealth := suite.healthChecker.GetOverallHealth()
			healthResults = append(healthResults, overallHealth)

			// Check alerts
			suite.alertManager.CheckRules(suite.metricsCollector)
			activeAlerts := suite.alertManager.GetActiveAlerts()
			alertCounts = append(alertCounts, len(activeAlerts))

			// Update system metrics
			suite.metricsCollector.UpdateSystemMetrics(
				suite.mockWSHub.GetConnections(),
				0, // queue length (mock)
				overallHealth.Score,
			)
		}
	}
}

func TestTradingMonitoringTestSuite(t *testing.T) {
	suite.Run(t, new(TradingMonitoringTestSuite))
}
