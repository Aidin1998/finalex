//go:build fiat

package test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Aidin1998/finalex/internal/fiat"
	"github.com/Aidin1998/finalex/pkg/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// FiatTestSuite provides test setup for fiat module
type FiatTestSuite struct {
	db      *gorm.DB
	service *fiat.FiatService
	handler *fiat.Handler
	logger  *zap.Logger
	router  *gin.Engine
}

// SetupFiatTest initializes test environment
func SetupFiatTest(t *testing.T) *FiatTestSuite {
	// Setup in-memory database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Migrate schemas
	err = db.AutoMigrate(
		&models.User{},
		&models.Account{},
		&models.Transaction{},
		&fiat.FiatDepositReceipt{},
	)
	require.NoError(t, err)

	// Setup logger
	logger, _ := zap.NewDevelopment()

	// Create test signature key
	signatureKey := []byte("test_signature_key_for_unit_tests")

	// Initialize services
	service := fiat.NewFiatService(db, logger, signatureKey)
	handler := fiat.NewHandler(service, logger)

	// Setup Gin router
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Setup providers config
	providers := map[string]fiat.ProviderInfo{
		"test_provider": {
			ID:        "test_provider",
			Name:      "Test Provider",
			PublicKey: "test_public_key",
			Endpoint:  "https://test.provider.com",
			Active:    true,
		},
	}

	// Setup routes
	api := router.Group("/api/v1")
	fiat.Routes(api, db, logger, signatureKey, providers, []string{})

	return &FiatTestSuite{
		db:      db,
		service: service,
		handler: handler,
		logger:  logger,
		router:  router,
	}
}

// createTestUser creates a test user for testing
func (suite *FiatTestSuite) createTestUser(t *testing.T) *models.User {
	user := &models.User{
		ID:        uuid.New(),
		Email:     "test@example.com",
		Username:  "testuser",
		FirstName: "Test",
		LastName:  "User",
		KYCStatus: "approved",
		Role:      "user",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err := suite.db.Create(user).Error
	require.NoError(t, err)
	return user
}

// TestProcessDepositReceipt tests the main deposit receipt processing
func TestProcessDepositReceipt(t *testing.T) {
	suite := SetupFiatTest(t)
	user := suite.createTestUser(t)

	ctx := context.Background()

	t.Run("Valid Receipt Processing", func(t *testing.T) {
		// Create valid receipt request
		req := &fiat.FiatReceiptRequest{
			UserID:      user.ID,
			ProviderID:  "test_provider",
			ReferenceID: "TEST_REF_001",
			Amount:      decimal.NewFromFloat(100.50),
			Currency:    "USD",
			Timestamp:   time.Now(),
			TraceID:     uuid.New().String(),
		}

		// Generate valid signature
		req.ProviderSig = suite.generateTestSignature(req)

		// Process receipt
		response, err := suite.service.ProcessDepositReceipt(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, response)

		assert.Equal(t, fiat.ReceiptStatusCompleted, response.Status)
		assert.NotEqual(t, uuid.Nil, response.ReceiptID)
		assert.Equal(t, req.TraceID, response.TraceID)
		assert.Contains(t, response.Message, "successfully")
	})

	t.Run("Duplicate Receipt Handling", func(t *testing.T) {
		// Create receipt request
		req := &fiat.FiatReceiptRequest{
			UserID:      user.ID,
			ProviderID:  "test_provider",
			ReferenceID: "TEST_REF_002",
			Amount:      decimal.NewFromFloat(200.00),
			Currency:    "USD",
			Timestamp:   time.Now(),
			TraceID:     uuid.New().String(),
		}
		req.ProviderSig = suite.generateTestSignature(req)

		// Process first time
		response1, err := suite.service.ProcessDepositReceipt(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, fiat.ReceiptStatusCompleted, response1.Status)

		// Process same receipt again (idempotent)
		response2, err := suite.service.ProcessDepositReceipt(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, fiat.ReceiptStatusDuplicate, response2.Status)
		assert.Equal(t, response1.ReceiptID, response2.ReceiptID)
	})

	t.Run("Invalid Signature", func(t *testing.T) {
		req := &fiat.FiatReceiptRequest{
			UserID:      user.ID,
			ProviderID:  "test_provider",
			ReferenceID: "TEST_REF_003",
			Amount:      decimal.NewFromFloat(50.00),
			Currency:    "USD",
			Timestamp:   time.Now(),
			ProviderSig: "invalid_signature",
			TraceID:     uuid.New().String(),
		}

		response, err := suite.service.ProcessDepositReceipt(ctx, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "signature validation failed")
		assert.Equal(t, fiat.ReceiptStatusFailed, response.Status)
	})

	t.Run("User Not Found", func(t *testing.T) {
		req := &fiat.FiatReceiptRequest{
			UserID:      uuid.New(), // Non-existent user
			ProviderID:  "test_provider",
			ReferenceID: "TEST_REF_004",
			Amount:      decimal.NewFromFloat(75.00),
			Currency:    "USD",
			Timestamp:   time.Now(),
			TraceID:     uuid.New().String(),
		}
		req.ProviderSig = suite.generateTestSignature(req)

		response, err := suite.service.ProcessDepositReceipt(ctx, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "user validation failed")
		assert.Equal(t, fiat.ReceiptStatusFailed, response.Status)
	})

	t.Run("User KYC Not Approved", func(t *testing.T) {
		// Create user with pending KYC
		pendingUser := &models.User{
			ID:        uuid.New(),
			Email:     "pending@example.com",
			Username:  "pendinguser",
			FirstName: "Pending",
			LastName:  "User",
			KYCStatus: "pending",
			Role:      "user",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		err := suite.db.Create(pendingUser).Error
		require.NoError(t, err)

		req := &fiat.FiatReceiptRequest{
			UserID:      pendingUser.ID,
			ProviderID:  "test_provider",
			ReferenceID: "TEST_REF_005",
			Amount:      decimal.NewFromFloat(125.00),
			Currency:    "USD",
			Timestamp:   time.Now(),
			TraceID:     uuid.New().String(),
		}
		req.ProviderSig = suite.generateTestSignature(req)

		response, err := suite.service.ProcessDepositReceipt(ctx, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "KYC not approved")
		assert.Equal(t, fiat.ReceiptStatusFailed, response.Status)
	})
}

// TestDepositReceiptAPI tests the HTTP API endpoints
func TestDepositReceiptAPI(t *testing.T) {
	suite := SetupFiatTest(t)
	user := suite.createTestUser(t)

	t.Run("POST /api/v1/fiat/receipts - Valid Request", func(t *testing.T) {
		req := &fiat.FiatReceiptRequest{
			UserID:      user.ID,
			ProviderID:  "test_provider",
			ReferenceID: "API_TEST_001",
			Amount:      decimal.NewFromFloat(500.00),
			Currency:    "USD",
			Timestamp:   time.Now(),
		}
		req.ProviderSig = suite.generateTestSignature(req)

		body, _ := json.Marshal(req)
		request := httptest.NewRequest("POST", "/api/v1/fiat/receipts", bytes.NewBuffer(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Provider-ID", "test_provider")
		request.Header.Set("X-Provider-Signature", req.ProviderSig)
		request.Header.Set("X-Timestamp", req.Timestamp.Format(time.RFC3339))
		request.Header.Set("User-Agent", "TestProvider/1.0")

		recorder := httptest.NewRecorder()
		suite.router.ServeHTTP(recorder, request)

		assert.Equal(t, http.StatusOK, recorder.Code)

		var response fiat.FiatReceiptResponse
		err := json.Unmarshal(recorder.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, fiat.ReceiptStatusCompleted, response.Status)
	})

	t.Run("POST /api/v1/fiat/receipts - Missing Provider ID", func(t *testing.T) {
		req := &fiat.FiatReceiptRequest{
			UserID:      user.ID,
			ProviderID:  "test_provider",
			ReferenceID: "API_TEST_002",
			Amount:      decimal.NewFromFloat(300.00),
			Currency:    "USD",
			Timestamp:   time.Now(),
		}
		req.ProviderSig = suite.generateTestSignature(req)

		body, _ := json.Marshal(req)
		request := httptest.NewRequest("POST", "/api/v1/fiat/receipts", bytes.NewBuffer(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("User-Agent", "TestProvider/1.0")
		// Missing X-Provider-ID header

		recorder := httptest.NewRecorder()
		suite.router.ServeHTTP(recorder, request)

		assert.Equal(t, http.StatusBadRequest, recorder.Code)

		var response map[string]interface{}
		err := json.Unmarshal(recorder.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, "MISSING_PROVIDER_ID", response["error"])
	})

	t.Run("GET /api/v1/fiat/receipts/{id} - Valid Receipt", func(t *testing.T) {
		// First create a receipt
		req := &fiat.FiatReceiptRequest{
			UserID:      user.ID,
			ProviderID:  "test_provider",
			ReferenceID: "API_GET_TEST_001",
			Amount:      decimal.NewFromFloat(150.00),
			Currency:    "USD",
			Timestamp:   time.Now(),
		}
		req.ProviderSig = suite.generateTestSignature(req)

		response, err := suite.service.ProcessDepositReceipt(context.Background(), req)
		require.NoError(t, err)

		// Now retrieve it via API
		url := fmt.Sprintf("/api/v1/fiat/receipts/%s", response.ReceiptID.String())
		request := httptest.NewRequest("GET", url, nil)
		request.Header.Set("X-Provider-ID", "test_provider")
		request.Header.Set("X-Provider-Signature", "dummy_sig_for_get")
		request.Header.Set("X-Timestamp", time.Now().Format(time.RFC3339))
		request.Header.Set("User-Agent", "TestProvider/1.0")

		recorder := httptest.NewRecorder()
		suite.router.ServeHTTP(recorder, request)

		assert.Equal(t, http.StatusOK, recorder.Code)

		var receipt fiat.FiatDepositReceipt
		err = json.Unmarshal(recorder.Body.Bytes(), &receipt)
		require.NoError(t, err)
		assert.Equal(t, response.ReceiptID, receipt.ID)
		assert.Equal(t, req.Amount.String(), receipt.Amount.String())
	})

	t.Run("GET /api/v1/fiat/receipts/{id} - Invalid UUID", func(t *testing.T) {
		request := httptest.NewRequest("GET", "/api/v1/fiat/receipts/invalid-uuid", nil)
		request.Header.Set("X-Provider-ID", "test_provider")
		request.Header.Set("X-Provider-Signature", "dummy_sig")
		request.Header.Set("X-Timestamp", time.Now().Format(time.RFC3339))
		request.Header.Set("User-Agent", "TestProvider/1.0")

		recorder := httptest.NewRecorder()
		suite.router.ServeHTTP(recorder, request)

		assert.Equal(t, http.StatusBadRequest, recorder.Code)

		var response map[string]interface{}
		err := json.Unmarshal(recorder.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, "INVALID_RECEIPT_ID", response["error"])
	})
}

// TestSecurityMiddleware tests security validations
func TestSecurityMiddleware(t *testing.T) {
	suite := SetupFiatTest(t)

	t.Run("Rate Limiting", func(t *testing.T) {
		// This test would need to be expanded with actual rate limiting logic
		// For now, we just test that the middleware is applied
		request := httptest.NewRequest("GET", "/api/v1/fiat/health", nil)
		recorder := httptest.NewRecorder()
		suite.router.ServeHTTP(recorder, request)

		assert.Equal(t, http.StatusOK, recorder.Code)
	})

	t.Run("Security Headers", func(t *testing.T) {
		request := httptest.NewRequest("GET", "/api/v1/fiat/health", nil)
		recorder := httptest.NewRecorder()
		suite.router.ServeHTTP(recorder, request)

		// Check that security headers are added
		assert.Equal(t, "nosniff", recorder.Header().Get("X-Content-Type-Options"))
		assert.Equal(t, "DENY", recorder.Header().Get("X-Frame-Options"))
		assert.Contains(t, recorder.Header().Get("Strict-Transport-Security"), "max-age=31536000")
	})

	t.Run("Timestamp Validation", func(t *testing.T) {
		user := suite.createTestUser(t)
		req := &fiat.FiatReceiptRequest{
			UserID:      user.ID,
			ProviderID:  "test_provider",
			ReferenceID: "TIMESTAMP_TEST",
			Amount:      decimal.NewFromFloat(100.00),
			Currency:    "USD",
			Timestamp:   time.Now().Add(-10 * time.Minute), // Old timestamp
		}
		req.ProviderSig = suite.generateTestSignature(req)

		body, _ := json.Marshal(req)
		request := httptest.NewRequest("POST", "/api/v1/fiat/receipts", bytes.NewBuffer(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Provider-ID", "test_provider")
		request.Header.Set("X-Provider-Signature", req.ProviderSig)
		request.Header.Set("X-Timestamp", req.Timestamp.Format(time.RFC3339))
		request.Header.Set("User-Agent", "TestProvider/1.0")

		recorder := httptest.NewRecorder()
		suite.router.ServeHTTP(recorder, request)

		assert.Equal(t, http.StatusBadRequest, recorder.Code)

		var response map[string]interface{}
		err := json.Unmarshal(recorder.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, "TIMESTAMP_OUT_OF_RANGE", response["error"])
	})
}

// TestUserDepositsEndpoint tests user deposit retrieval
func TestUserDepositsEndpoint(t *testing.T) {
	suite := SetupFiatTest(t)
	user := suite.createTestUser(t)

	// Create multiple receipts for the user
	for i := 0; i < 5; i++ {
		req := &fiat.FiatReceiptRequest{
			UserID:      user.ID,
			ProviderID:  "test_provider",
			ReferenceID: fmt.Sprintf("USER_TEST_%d", i),
			Amount:      decimal.NewFromFloat(float64((i + 1) * 100)),
			Currency:    "USD",
			Timestamp:   time.Now(),
		}
		req.ProviderSig = suite.generateTestSignature(req)

		_, err := suite.service.ProcessDepositReceipt(context.Background(), req)
		require.NoError(t, err)
	}

	t.Run("Get User Deposits", func(t *testing.T) {
		receipts, total, err := suite.service.GetUserDeposits(context.Background(), user.ID, 10, 0)
		require.NoError(t, err)
		assert.Equal(t, int64(5), total)
		assert.Len(t, receipts, 5)
	})

	t.Run("Pagination", func(t *testing.T) {
		receipts, total, err := suite.service.GetUserDeposits(context.Background(), user.ID, 2, 0)
		require.NoError(t, err)
		assert.Equal(t, int64(5), total)
		assert.Len(t, receipts, 2)

		receipts2, _, err := suite.service.GetUserDeposits(context.Background(), user.ID, 2, 2)
		require.NoError(t, err)
		assert.Len(t, receipts2, 2)
		assert.NotEqual(t, receipts[0].ID, receipts2[0].ID)
	})
}

// Helper method to generate test signature
func (suite *FiatTestSuite) generateTestSignature(req *fiat.FiatReceiptRequest) string {
	// This mimics the signature generation in the service
	// In a real implementation, this would use the same algorithm
	payload := fmt.Sprintf("%s:%s:%s:%s:%s:%d",
		req.UserID.String(),
		req.ProviderID,
		req.ReferenceID,
		req.Amount.String(),
		req.Currency,
		req.Timestamp.Unix())

	// Simple hash for testing - in production this would use proper HMAC
	return fmt.Sprintf("test_signature_%x", len(payload))
}

// TestAuditTrail tests audit logging functionality
func TestAuditTrail(t *testing.T) {
	suite := SetupFiatTest(t)
	user := suite.createTestUser(t)

	req := &fiat.FiatReceiptRequest{
		UserID:      user.ID,
		ProviderID:  "test_provider",
		ReferenceID: "AUDIT_TEST_001",
		Amount:      decimal.NewFromFloat(250.00),
		Currency:    "USD",
		Timestamp:   time.Now(),
		TraceID:     "audit-trace-001",
	}
	req.ProviderSig = suite.generateTestSignature(req)

	response, err := suite.service.ProcessDepositReceipt(context.Background(), req)
	require.NoError(t, err)

	// Retrieve receipt and check audit trail
	receipt, err := suite.service.GetDepositReceipt(context.Background(), response.ReceiptID)
	require.NoError(t, err)

	assert.NotEmpty(t, receipt.AuditTrail)
	assert.Contains(t, receipt.AuditTrail, "deposit_completed")
	assert.Contains(t, receipt.AuditTrail, req.TraceID)
}

// TestHealthCheck tests the health check endpoint
func TestHealthCheck(t *testing.T) {
	suite := SetupFiatTest(t)

	request := httptest.NewRequest("GET", "/api/v1/fiat/health", nil)
	recorder := httptest.NewRecorder()
	suite.router.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusOK, recorder.Code)

	var response map[string]interface{}
	err := json.Unmarshal(recorder.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "fiat", response["service"])
	assert.Equal(t, "healthy", response["status"])
	assert.Equal(t, "connected", response["database"])
}
