package compliance

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Aidin1998/finalex/internal/compliance/interfaces"
)

func TestComplianceModuleIntegration(t *testing.T) {
	// Setup test database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Create compliance module with test configuration
	config := &Config{
		Database: DatabaseConfig{
			Host:            "localhost",
			Port:            5432,
			Database:        "test",
			Username:        "test",
			Password:        "test",
			SSLMode:         "disable",
			MaxConnections:  10,
			ConnMaxLifetime: 1 * time.Hour,
			ConnMaxIdleTime: 15 * time.Minute,
		},
		Audit: AuditConfig{
			Workers:           2,
			BatchSize:         10,
			FlushInterval:     1 * time.Second,
			EnableEncryption:  false,
			EnableCompression: false,
			RetentionDays:     30,
			VerifyChain:       true,
		},
		Compliance: ComplianceConfig{
			EnableKYC:        true,
			EnableAML:        true,
			EnableSanctions:  true,
			KYCRequiredLevel: "basic",
			AMLRiskThreshold: 0.7,
			CacheSize:        100,
			CacheTTL:         5 * time.Minute,
		},
		Manipulation: ManipulationConfig{
			EnableDetection:   true,
			DetectionInterval: 30 * time.Second,
			RiskThreshold:     0.6,
			AlertThreshold:    0.8,
			LookbackPeriod:    1 * time.Hour,
			MaxPatterns:       100,
			EnableML:          false,
		},
		Monitoring: MonitoringConfig{
			Workers:         2,
			AlertQueueSize:  100,
			MetricsInterval: 10 * time.Second,
		},
		Performance: PerformanceConfig{
			MaxConcurrentRequests: 100,
			RequestTimeout:        10 * time.Second,
			EnableRateLimiting:    false,
			EnableCaching:         true,
			CacheSize:             100,
			CacheTTL:              5 * time.Minute,
		},
	}

	factory := NewServiceFactory(config, db)
	module, err := factory.CreateComplianceModule(context.Background())
	require.NoError(t, err)

	// Start the module
	ctx := context.Background()
	err = module.Start(ctx)
	require.NoError(t, err)

	// Test compliance request processing
	t.Run("ProcessComplianceRequest", func(t *testing.T) {
		request := interfaces.ComplianceRequest{
			RequestID:   "test-request-001",
			UserID:      "test-user-001",
			RequestType: "kyc_verification",
			KYCData: &interfaces.KYCData{
				FirstName:      "John",
				LastName:       "Doe",
				DateOfBirth:    time.Date(1990, 1, 1, 0, 0, 0, 0, time.UTC),
				Nationality:    "US",
				DocumentType:   "passport",
				DocumentNumber: "P123456789",
			},
			Timestamp: time.Now(),
		}

		result, err := module.ProcessComplianceRequest(ctx, request)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, request.RequestID, result.RequestID)
		assert.Equal(t, request.UserID, result.UserID)
	})

	// Test system health
	t.Run("GetSystemHealth", func(t *testing.T) {
		health, err := module.GetSystemHealth(ctx)
		require.NoError(t, err)
		assert.NotNil(t, health)
		assert.NotEmpty(t, health.Services)
	})

	// Test metrics
	t.Run("GetMetrics", func(t *testing.T) {
		metrics, err := module.GetMetrics(ctx)
		require.NoError(t, err)
		assert.NotNil(t, metrics)
	})

	// Stop the module
	err = module.Stop()
	assert.NoError(t, err)
}

func TestAuditServiceIntegration(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	config := DefaultConfig()
	factory := NewServiceFactory(config, db)
	module, err := factory.CreateComplianceModule(context.Background())
	require.NoError(t, err)

	ctx := context.Background()
	err = module.Start(ctx)
	require.NoError(t, err)
	defer module.Stop()

	auditSvc := module.GetAuditService()

	// Test audit event logging
	event := interfaces.AuditEvent{
		EventType:  "test_event",
		UserID:     "test-user",
		Action:     "test_action",
		EntityType: "test_entity",
		EntityID:   "test-entity-001",
		NewValue:   "test_value",
		Metadata:   map[string]interface{}{"test": "data"},
		Timestamp:  time.Now(),
		Severity:   "info",
	}

	err = auditSvc.LogEvent(ctx, event)
	assert.NoError(t, err)

	// Test audit event retrieval
	filter := interfaces.AuditFilter{
		UserID:    "test-user",
		StartTime: time.Now().Add(-1 * time.Hour),
		EndTime:   time.Now().Add(1 * time.Hour),
		Limit:     10,
	}

	events, err := auditSvc.GetEvents(ctx, filter)
	assert.NoError(t, err)
	assert.Len(t, events, 1)
	assert.Equal(t, event.EventType, events[0].EventType)
}

func TestComplianceServiceIntegration(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	config := DefaultConfig()
	factory := NewServiceFactory(config, db)
	module, err := factory.CreateComplianceModule(context.Background())
	require.NoError(t, err)

	ctx := context.Background()
	err = module.Start(ctx)
	require.NoError(t, err)
	defer module.Stop()

	complianceSvc := module.GetComplianceService()

	// Test KYC verification
	kycData := interfaces.KYCData{
		FirstName:      "Jane",
		LastName:       "Smith",
		DateOfBirth:    time.Date(1985, 5, 15, 0, 0, 0, 0, time.UTC),
		Nationality:    "GB",
		DocumentType:   "passport",
		DocumentNumber: "GB987654321",
	}

	result, err := complianceSvc.VerifyKYC(ctx, "test-user-002", kycData)
	assert.NoError(t, err)
	assert.NotNil(t, result)

	// Test AML check
	transactionData := interfaces.TransactionData{
		Amount:      1000.0,
		Currency:    "USD",
		FromAccount: "account1",
		ToAccount:   "account2",
		Description: "Test transaction",
	}

	amlResult, err := complianceSvc.CheckAML(ctx, "test-user-002", transactionData)
	assert.NoError(t, err)
	assert.NotNil(t, amlResult)
}

func TestManipulationServiceIntegration(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	config := DefaultConfig()
	factory := NewServiceFactory(config, db)
	module, err := factory.CreateComplianceModule(context.Background())
	require.NoError(t, err)

	ctx := context.Background()
	err = module.Start(ctx)
	require.NoError(t, err)
	defer module.Stop()

	manipulationSvc := module.GetManipulationService()

	// Test transaction analysis
	transactionData := interfaces.TransactionData{
		Amount:      10000.0,
		Currency:    "BTC",
		FromAccount: "account1",
		ToAccount:   "account2",
		Description: "Large BTC transfer",
		Timestamp:   time.Now(),
	}

	result, err := manipulationSvc.AnalyzeTransaction(ctx, transactionData)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.GreaterOrEqual(t, result.RiskScore, 0.0)
	assert.LessOrEqual(t, result.RiskScore, 1.0)
}

func TestMonitoringServiceIntegration(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	config := DefaultConfig()
	factory := NewServiceFactory(config, db)
	module, err := factory.CreateComplianceModule(context.Background())
	require.NoError(t, err)

	ctx := context.Background()
	err = module.Start(ctx)
	require.NoError(t, err)
	defer module.Stop()

	monitoringSvc := module.GetMonitoringService()

	// Test alert generation
	alert := interfaces.MonitoringAlert{
		UserID:    "test-user",
		AlertType: "test_alert",
		Severity:  "medium",
		Message:   "Test alert message",
		Data:      map[string]interface{}{"test": "data"},
	}

	err = monitoringSvc.GenerateAlert(ctx, alert)
	assert.NoError(t, err)

	// Wait a bit for alert processing
	time.Sleep(100 * time.Millisecond)

	// Test alert retrieval
	filter := interfaces.AlertFilter{
		UserID:    "test-user",
		AlertType: "test_alert",
		Limit:     10,
	}

	alerts, err := monitoringSvc.GetAlerts(ctx, filter)
	assert.NoError(t, err)
	assert.Len(t, alerts, 1)
	assert.Equal(t, alert.AlertType, alerts[0].AlertType)
}

func TestEndToEndComplianceFlow(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	config := DefaultConfig()
	factory := NewServiceFactory(config, db)
	module, err := factory.CreateComplianceModule(context.Background())
	require.NoError(t, err)

	ctx := context.Background()
	err = module.Start(ctx)
	require.NoError(t, err)
	defer module.Stop()

	// Create a comprehensive compliance request
	request := interfaces.ComplianceRequest{
		RequestID:   "e2e-test-001",
		UserID:      "e2e-user-001",
		RequestType: "transaction",
		KYCData: &interfaces.KYCData{
			FirstName:      "Alice",
			LastName:       "Johnson",
			DateOfBirth:    time.Date(1992, 3, 20, 0, 0, 0, 0, time.UTC),
			Nationality:    "CA",
			DocumentType:   "drivers_license",
			DocumentNumber: "CA123456789",
		},
		TransactionData: &interfaces.TransactionData{
			Amount:      5000.0,
			Currency:    "USD",
			FromAccount: "alice-account",
			ToAccount:   "external-account",
			Description: "International wire transfer",
			Timestamp:   time.Now(),
		},
		Timestamp: time.Now(),
	}

	// Process the request
	result, err := module.ProcessComplianceRequest(ctx, request)
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Verify the result contains expected fields
	assert.Equal(t, request.RequestID, result.RequestID)
	assert.Equal(t, request.UserID, result.UserID)
	assert.NotEmpty(t, result.Status)
	assert.NotEmpty(t, result.RiskLevel)
	assert.NotZero(t, result.ProcessedAt)

	// Verify audit trail was created
	auditFilter := interfaces.AuditFilter{
		UserID:    request.UserID,
		StartTime: time.Now().Add(-1 * time.Minute),
		EndTime:   time.Now().Add(1 * time.Minute),
		Limit:     100,
	}

	auditEvents, err := module.GetAuditService().GetEvents(ctx, auditFilter)
	assert.NoError(t, err)
	assert.NotEmpty(t, auditEvents)

	// Check system health after processing
	health, err := module.GetSystemHealth(ctx)
	assert.NoError(t, err)
	assert.Equal(t, "healthy", health.OverallStatus)

	// Get final metrics
	metrics, err := module.GetMetrics(ctx)
	assert.NoError(t, err)
	assert.Greater(t, metrics.ComplianceChecks, int64(0))
}
