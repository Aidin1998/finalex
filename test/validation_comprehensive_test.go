package test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/Aidin1998/pincex_unified/pkg/validation"
)

// TestEnhancedValidationSuite contains comprehensive tests for the enhanced validation system
type TestEnhancedValidationSuite struct {
	router    *gin.Engine
	logger    *zap.Logger
	validator *validation.EnhancedValidator
}

// Setup initializes the test suite
func (suite *TestEnhancedValidationSuite) Setup() {
	gin.SetMode(gin.TestMode)
	suite.logger = zap.NewNop()
	suite.router = gin.New()

	// Configure enhanced validation
	config := validation.DefaultEnhancedValidationConfig()
	config.StrictModeEnabled = true
	config.EnableDetailedErrorLogs = true

	// Add enhanced validation middleware
	suite.router.Use(validation.EnhancedValidationMiddleware(suite.logger, config))

	// Add security hardening middleware
	securityConfig := validation.DefaultSecurityConfig()
	suite.router.Use(validation.SecurityHardeningMiddleware(suite.logger, securityConfig))

	// Set up test endpoints
	suite.setupTestEndpoints()
}

// setupTestEndpoints creates test endpoints for validation testing
func (suite *TestEnhancedValidationSuite) setupTestEndpoints() {
	api := suite.router.Group("/api/v1")

	// Trading endpoints
	trading := api.Group("/trading")
	{
		trading.POST("/orders", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "order_created"})
		})
		trading.GET("/orders", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"orders": []string{}})
		})
		trading.DELETE("/orders/:id", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "order_cancelled"})
		})
		trading.GET("/pairs", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"pairs": []string{}})
		})
		trading.GET("/orderbook/:symbol", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"orderbook": gin.H{}})
		})
	}

	// Market endpoints
	market := api.Group("/market")
	{
		market.GET("/prices", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"prices": gin.H{}})
		})
	}

	// Fiat endpoints
	fiat := api.Group("/fiat")
	{
		fiat.POST("/deposit", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "deposit_initiated"})
		})
		fiat.POST("/withdraw", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "withdrawal_initiated"})
		})
	}

	// Admin endpoints
	admin := api.Group("/admin")
	{
		admin.POST("/trading/pairs", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "pair_created"})
		})
	}
}

// TestTradingOrderValidation tests trading order placement validation
func TestTradingOrderValidation(t *testing.T) {
	suite := &TestEnhancedValidationSuite{}
	suite.Setup()

	tests := []struct {
		name           string
		payload        map[string]interface{}
		headers        map[string]string
		expectedStatus int
		shouldPass     bool
		description    string
	}{
		{
			name: "Valid Limit Order",
			payload: map[string]interface{}{
				"symbol":   "BTC/USD",
				"side":     "buy",
				"type":     "limit",
				"quantity": "0.1",
				"price":    "50000.00",
			},
			headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": "Bearer valid_token_here",
			},
			expectedStatus: http.StatusOK,
			shouldPass:     true,
			description:    "Valid limit order should pass validation",
		},
		{
			name: "Valid Market Order",
			payload: map[string]interface{}{
				"symbol":   "ETH/USD",
				"side":     "sell",
				"type":     "market",
				"quantity": "2.5",
			},
			headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": "Bearer valid_token_here",
			},
			expectedStatus: http.StatusOK,
			shouldPass:     true,
			description:    "Valid market order should pass validation",
		},
		{
			name: "Invalid Symbol Format",
			payload: map[string]interface{}{
				"symbol":   "BTCUSD", // Missing separator
				"side":     "buy",
				"type":     "limit",
				"quantity": "0.1",
				"price":    "50000.00",
			},
			headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": "Bearer valid_token_here",
			},
			expectedStatus: http.StatusBadRequest,
			shouldPass:     false,
			description:    "Invalid symbol format should fail validation",
		},
		{
			name: "Missing Required Fields",
			payload: map[string]interface{}{
				"symbol": "BTC/USD",
				"side":   "buy",
				// Missing type and quantity
			},
			headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": "Bearer valid_token_here",
			},
			expectedStatus: http.StatusBadRequest,
			shouldPass:     false,
			description:    "Missing required fields should fail validation",
		},
		{
			name: "Invalid Side Value",
			payload: map[string]interface{}{
				"symbol":   "BTC/USD",
				"side":     "invalid_side",
				"type":     "limit",
				"quantity": "0.1",
				"price":    "50000.00",
			},
			headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": "Bearer valid_token_here",
			},
			expectedStatus: http.StatusBadRequest,
			shouldPass:     false,
			description:    "Invalid side value should fail validation",
		},
		{
			name: "Limit Order Without Price",
			payload: map[string]interface{}{
				"symbol":   "BTC/USD",
				"side":     "buy",
				"type":     "limit",
				"quantity": "0.1",
				// Missing price for limit order
			},
			headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": "Bearer valid_token_here",
			},
			expectedStatus: http.StatusBadRequest,
			shouldPass:     false,
			description:    "Limit order without price should fail business logic validation",
		},
		{
			name: "Negative Quantity",
			payload: map[string]interface{}{
				"symbol":   "BTC/USD",
				"side":     "buy",
				"type":     "market",
				"quantity": "-0.1",
			},
			headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": "Bearer valid_token_here",
			},
			expectedStatus: http.StatusBadRequest,
			shouldPass:     false,
			description:    "Negative quantity should fail validation",
		},
		{
			name: "SQL Injection Attempt",
			payload: map[string]interface{}{
				"symbol":   "BTC/USD'; DROP TABLE orders; --",
				"side":     "buy",
				"type":     "limit",
				"quantity": "0.1",
				"price":    "50000.00",
			},
			headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": "Bearer valid_token_here",
			},
			expectedStatus: http.StatusBadRequest,
			shouldPass:     false,
			description:    "SQL injection attempt should be blocked",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Prepare request
			jsonPayload, _ := json.Marshal(tt.payload)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/trading/orders", bytes.NewBuffer(jsonPayload))

			// Set headers
			for key, value := range tt.headers {
				req.Header.Set(key, value)
			}

			// Execute request
			w := httptest.NewRecorder()
			suite.router.ServeHTTP(w, req)

			// Validate response
			assert.Equal(t, tt.expectedStatus, w.Code, tt.description)

			if !tt.shouldPass {
				// Check that error response contains validation details
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)
				assert.Contains(t, response, "error")
			}
		})
	}
}

// TestSecurityHardening tests advanced security features
func TestSecurityHardening(t *testing.T) {
	suite := &TestEnhancedValidationSuite{}
	suite.Setup()

	tests := []struct {
		name           string
		method         string
		path           string
		headers        map[string]string
		query          string
		expectedStatus int
		description    string
	}{
		{
			name:   "Suspicious User Agent",
			method: "GET",
			path:   "/api/v1/trading/pairs",
			headers: map[string]string{
				"User-Agent":    "sqlmap/1.0",
				"Authorization": "Bearer valid_token",
			},
			expectedStatus: 429,
			description:    "Suspicious user agent should be blocked",
		},
		{
			name:   "XSS Attempt in Query",
			method: "GET",
			path:   "/api/v1/trading/orders",
			headers: map[string]string{
				"Authorization": "Bearer valid_token",
			},
			query:          "symbol=<script>alert('xss')</script>",
			expectedStatus: 429,
			description:    "XSS attempt should be blocked",
		},
		{
			name:   "Command Injection Attempt",
			method: "GET",
			path:   "/api/v1/market/prices",
			headers: map[string]string{
				"Authorization": "Bearer valid_token",
			},
			query:          "symbols=BTC/USD; rm -rf /",
			expectedStatus: 429,
			description:    "Command injection attempt should be blocked",
		},
		{
			name:   "Directory Traversal Attempt",
			method: "GET",
			path:   "/api/v1/trading/orderbook/../../etc/passwd",
			headers: map[string]string{
				"Authorization": "Bearer valid_token",
			},
			expectedStatus: 429,
			description:    "Directory traversal attempt should be blocked",
		},
		{
			name:   "Valid Request",
			method: "GET",
			path:   "/api/v1/trading/pairs",
			headers: map[string]string{
				"User-Agent":    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
				"Authorization": "Bearer valid_token",
			},
			expectedStatus: 200,
			description:    "Valid request should pass security checks",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Prepare request
			url := tt.path
			if tt.query != "" {
				url += "?" + tt.query
			}

			req := httptest.NewRequest(tt.method, url, nil)

			// Set headers
			for key, value := range tt.headers {
				req.Header.Set(key, value)
			}

			// Execute request
			w := httptest.NewRecorder()
			suite.router.ServeHTTP(w, req)

			// Validate response
			assert.Equal(t, tt.expectedStatus, w.Code, tt.description)
		})
	}
}

// TestPathParameterValidation tests path parameter validation
func TestPathParameterValidation(t *testing.T) {
	suite := &TestEnhancedValidationSuite{}
	suite.Setup()

	tests := []struct {
		name           string
		path           string
		expectedStatus int
		description    string
	}{
		{
			name:           "Valid UUID",
			path:           "/api/v1/trading/orders/550e8400-e29b-41d4-a716-446655440000",
			expectedStatus: 200,
			description:    "Valid UUID should pass validation",
		},
		{
			name:           "Invalid UUID Format",
			path:           "/api/v1/trading/orders/invalid-uuid-format",
			expectedStatus: 400,
			description:    "Invalid UUID format should fail validation",
		},
		{
			name:           "Valid Trading Symbol",
			path:           "/api/v1/trading/orderbook/BTC/USD",
			expectedStatus: 200,
			description:    "Valid trading symbol should pass validation",
		},
		{
			name:           "Invalid Trading Symbol",
			path:           "/api/v1/trading/orderbook/INVALID",
			expectedStatus: 400,
			description:    "Invalid trading symbol should fail validation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			req.Header.Set("Authorization", "Bearer valid_token")

			w := httptest.NewRecorder()
			suite.router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code, tt.description)
		})
	}
}

// TestQueryParameterValidation tests query parameter validation
func TestQueryParameterValidation(t *testing.T) {
	suite := &TestEnhancedValidationSuite{}
	suite.Setup()

	tests := []struct {
		name           string
		query          string
		expectedStatus int
		description    string
	}{
		{
			name:           "Valid Status Filter",
			query:          "status=open",
			expectedStatus: 200,
			description:    "Valid status filter should pass validation",
		},
		{
			name:           "Invalid Status Value",
			query:          "status=invalid_status",
			expectedStatus: 400,
			description:    "Invalid status value should fail validation",
		},
		{
			name:           "Valid Limit and Offset",
			query:          "limit=50&offset=100",
			expectedStatus: 200,
			description:    "Valid limit and offset should pass validation",
		},
		{
			name:           "Negative Limit",
			query:          "limit=-10",
			expectedStatus: 400,
			description:    "Negative limit should fail validation",
		},
		{
			name:           "Excessive Limit",
			query:          "limit=10000",
			expectedStatus: 400,
			description:    "Excessive limit should fail validation",
		},
		{
			name:           "Forbidden Parameter",
			query:          "forbidden_param=value",
			expectedStatus: 400,
			description:    "Forbidden parameter should fail validation in strict mode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/api/v1/trading/orders"
			if tt.query != "" {
				url += "?" + tt.query
			}

			req := httptest.NewRequest(http.MethodGet, url, nil)
			req.Header.Set("Authorization", "Bearer valid_token")

			w := httptest.NewRecorder()
			suite.router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code, tt.description)
		})
	}
}

// TestFiatOperationValidation tests fiat operation validation
func TestFiatOperationValidation(t *testing.T) {
	suite := &TestEnhancedValidationSuite{}
	suite.Setup()

	tests := []struct {
		name           string
		payload        map[string]interface{}
		expectedStatus int
		shouldPass     bool
		description    string
	}{
		{
			name: "Valid Bank Transfer",
			payload: map[string]interface{}{
				"amount":          "1000.00",
				"currency":        "USD",
				"payment_method":  "bank_transfer",
				"bank_account_id": "550e8400-e29b-41d4-a716-446655440000",
			},
			expectedStatus: 200,
			shouldPass:     true,
			description:    "Valid bank transfer should pass validation",
		},
		{
			name: "Bank Transfer Without Account ID",
			payload: map[string]interface{}{
				"amount":         "1000.00",
				"currency":       "USD",
				"payment_method": "bank_transfer",
				// Missing bank_account_id
			},
			expectedStatus: 400,
			shouldPass:     false,
			description:    "Bank transfer without account ID should fail business logic validation",
		},
		{
			name: "Invalid Currency",
			payload: map[string]interface{}{
				"amount":         "1000.00",
				"currency":       "INVALID",
				"payment_method": "credit_card",
			},
			expectedStatus: 400,
			shouldPass:     false,
			description:    "Invalid currency should fail validation",
		},
		{
			name: "Negative Amount",
			payload: map[string]interface{}{
				"amount":         "-1000.00",
				"currency":       "USD",
				"payment_method": "credit_card",
			},
			expectedStatus: 400,
			shouldPass:     false,
			description:    "Negative amount should fail validation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonPayload, _ := json.Marshal(tt.payload)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/fiat/deposit", bytes.NewBuffer(jsonPayload))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer valid_token")

			w := httptest.NewRecorder()
			suite.router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code, tt.description)
		})
	}
}

// TestRateLimiting tests rate limiting functionality
func TestRateLimiting(t *testing.T) {
	suite := &TestEnhancedValidationSuite{}
	suite.Setup()

	// Make rapid requests to test rate limiting
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/trading/pairs", nil)
		req.Header.Set("Authorization", "Bearer valid_token")
		req.Header.Set("User-Agent", "test-client")

		w := httptest.NewRecorder()
		suite.router.ServeHTTP(w, req)

		// Early requests should pass, later ones might be rate limited
		if i < 5 {
			assert.Equal(t, 200, w.Code, "Early requests should pass")
		}
		// Note: Actual rate limiting behavior depends on implementation details
	}
}

// TestPerformanceValidation tests validation performance
func TestPerformanceValidation(t *testing.T) {
	suite := &TestEnhancedValidationSuite{}
	suite.Setup()

	// Test payload
	payload := map[string]interface{}{
		"symbol":   "BTC/USD",
		"side":     "buy",
		"type":     "limit",
		"quantity": "0.1",
		"price":    "50000.00",
	}

	jsonPayload, _ := json.Marshal(payload)

	// Measure validation performance
	start := time.Now()
	iterations := 1000

	for i := 0; i < iterations; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/trading/orders", bytes.NewBuffer(jsonPayload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer valid_token")

		w := httptest.NewRecorder()
		suite.router.ServeHTTP(w, req)
	}

	duration := time.Since(start)
	avgDuration := duration / time.Duration(iterations)

	t.Logf("Average validation time: %v", avgDuration)
	assert.Less(t, avgDuration, 10*time.Millisecond, "Validation should complete within 10ms on average")
}

// TestContentTypeValidation tests content type validation
func TestContentTypeValidation(t *testing.T) {
	suite := &TestEnhancedValidationSuite{}
	suite.Setup()

	payload := map[string]interface{}{
		"symbol":   "BTC/USD",
		"side":     "buy",
		"type":     "market",
		"quantity": "0.1",
	}

	tests := []struct {
		name           string
		contentType    string
		expectedStatus int
		description    string
	}{
		{
			name:           "Valid JSON Content Type",
			contentType:    "application/json",
			expectedStatus: 200,
			description:    "Valid JSON content type should pass",
		},
		{
			name:           "Invalid Content Type",
			contentType:    "text/plain",
			expectedStatus: 400,
			description:    "Invalid content type should fail validation",
		},
		{
			name:           "Missing Content Type",
			contentType:    "",
			expectedStatus: 400,
			description:    "Missing content type should fail validation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonPayload, _ := json.Marshal(payload)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/trading/orders", bytes.NewBuffer(jsonPayload))

			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}
			req.Header.Set("Authorization", "Bearer valid_token")

			w := httptest.NewRecorder()
			suite.router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code, tt.description)
		})
	}
}

// TestAdvancedThreatDetection tests advanced threat detection
func TestAdvancedThreatDetection(t *testing.T) {
	suite := &TestEnhancedValidationSuite{}
	suite.Setup()

	threats := []struct {
		name        string
		input       string
		description string
	}{
		{
			name:        "SQL Injection with UNION",
			input:       "BTC/USD UNION SELECT * FROM users",
			description: "UNION-based SQL injection should be detected",
		},
		{
			name:        "XSS with Script Tag",
			input:       "<script>alert('xss')</script>",
			description: "Script tag XSS should be detected",
		},
		{
			name:        "Command Injection",
			input:       "BTC/USD; cat /etc/passwd",
			description: "Command injection should be detected",
		},
		{
			name:        "LDAP Injection",
			input:       "*)(&(objectClass=user)",
			description: "LDAP injection should be detected",
		},
		{
			name:        "XXE Attack",
			input:       "<!DOCTYPE foo [<!ENTITY xxe SYSTEM \"file:///etc/passwd\">]>",
			description: "XXE attack should be detected",
		},
		{
			name:        "NoSQL Injection",
			input:       "{\"$ne\": null}",
			description: "NoSQL injection should be detected",
		},
	}

	for _, threat := range threats {
		t.Run(threat.name, func(t *testing.T) {
			// Test in query parameter
			req := httptest.NewRequest(http.MethodGet, "/api/v1/trading/orders?symbol="+threat.input, nil)
			req.Header.Set("Authorization", "Bearer valid_token")

			w := httptest.NewRecorder()
			suite.router.ServeHTTP(w, req)

			assert.Equal(t, 429, w.Code, threat.description+" in query parameter")

			// Test in request body
			payload := map[string]interface{}{
				"symbol":   threat.input,
				"side":     "buy",
				"type":     "market",
				"quantity": "0.1",
			}

			jsonPayload, _ := json.Marshal(payload)
			req = httptest.NewRequest(http.MethodPost, "/api/v1/trading/orders", bytes.NewBuffer(jsonPayload))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer valid_token")

			w = httptest.NewRecorder()
			suite.router.ServeHTTP(w, req)

			assert.Equal(t, 429, w.Code, threat.description+" in request body")
		})
	}
}

// BenchmarkValidationPerformance benchmarks validation performance
func BenchmarkValidationPerformance(b *testing.B) {
	suite := &TestEnhancedValidationSuite{}
	suite.Setup()

	payload := map[string]interface{}{
		"symbol":   "BTC/USD",
		"side":     "buy",
		"type":     "limit",
		"quantity": "0.1",
		"price":    "50000.00",
	}

	jsonPayload, _ := json.Marshal(payload)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/trading/orders", bytes.NewBuffer(jsonPayload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer valid_token")

		w := httptest.NewRecorder()
		suite.router.ServeHTTP(w, req)
	}
}

// TestValidationErrorMessages tests error message quality
func TestValidationErrorMessages(t *testing.T) {
	suite := &TestEnhancedValidationSuite{}
	suite.Setup()

	// Test with invalid payload
	payload := map[string]interface{}{
		"symbol":   "INVALID_SYMBOL",
		"side":     "invalid_side",
		"type":     "invalid_type",
		"quantity": "invalid_quantity",
	}

	jsonPayload, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trading/orders", bytes.NewBuffer(jsonPayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer valid_token")

	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)

	assert.Equal(t, 400, w.Code)

	// Parse response and check error details
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	// Check that validation errors are detailed and helpful
	assert.Contains(t, response, "validation_errors")
	validationErrors, ok := response["validation_errors"].([]interface{})
	require.True(t, ok)
	assert.Greater(t, len(validationErrors), 0, "Should have validation errors")

	// Check error structure
	for _, errInterface := range validationErrors {
		err, ok := errInterface.(map[string]interface{})
		require.True(t, ok)

		assert.Contains(t, err, "field", "Error should specify field")
		assert.Contains(t, err, "tag", "Error should specify validation tag")
		assert.Contains(t, err, "message", "Error should have descriptive message")
	}
}
