package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// APITestSuite provides comprehensive API validation testing
type APITestSuite struct {
	config      TestConfig
	environment *TestEnvironment
	server      *httptest.Server
	client      *http.Client
	metrics     PerformanceMetrics
	mu          sync.RWMutex
}

// APIEndpoint represents an API endpoint for testing
type APIEndpoint struct {
	Method       string            `json:"method"`
	Path         string            `json:"path"`
	Headers      map[string]string `json:"headers"`
	Body         interface{}       `json:"body,omitempty"`
	ExpectedCode int               `json:"expected_code"`
	RateLimit    int               `json:"rate_limit"` // requests per second
}

// APIResponse represents a response from an API call
type APIResponse struct {
	StatusCode int                    `json:"status_code"`
	Headers    map[string]string      `json:"headers"`
	Body       map[string]interface{} `json:"body"`
	Latency    time.Duration          `json:"latency"`
	Error      error                  `json:"error,omitempty"`
}

// RateLimitTest represents a rate limiting test configuration
type RateLimitTest struct {
	Endpoint          string        `json:"endpoint"`
	RequestsPerSecond int           `json:"requests_per_second"`
	Duration          time.Duration `json:"duration"`
	ExpectedLimit     int           `json:"expected_limit"`
}

// MockAPIServer creates a mock API server for testing
func NewMockAPIServer() *httptest.Server {
	mux := http.NewServeMux()

	// Authentication endpoints
	mux.HandleFunc("/api/v1/auth/login", handleLogin)
	mux.HandleFunc("/api/v1/auth/logout", handleLogout)
	mux.HandleFunc("/api/v1/auth/refresh", handleRefresh)

	// Trading endpoints
	mux.HandleFunc("/api/v1/orders", handleOrders)
	mux.HandleFunc("/api/v1/orders/", handleOrderByID)
	mux.HandleFunc("/api/v1/trades", handleTrades)
	mux.HandleFunc("/api/v1/orderbook/", handleOrderBook)

	// User endpoints
	mux.HandleFunc("/api/v1/user/profile", handleUserProfile)
	mux.HandleFunc("/api/v1/user/balance", handleUserBalance)
	mux.HandleFunc("/api/v1/user/orders", handleUserOrders)

	// Market data endpoints
	mux.HandleFunc("/api/v1/ticker/", handleTicker)
	mux.HandleFunc("/api/v1/markets", handleMarkets)
	mux.HandleFunc("/api/v1/candles/", handleCandles)

	return httptest.NewServer(mux)
}

func TestAPIEndpointValidation(t *testing.T) {
	config := DefaultTestConfig()
	env := NewTestEnvironment(config)
	server := NewMockAPIServer()
	defer server.Close()

	suite := &APITestSuite{
		config:      config,
		environment: env,
		server:      server,
		client:      &http.Client{Timeout: 30 * time.Second},
	}

	// Define test endpoints
	endpoints := []APIEndpoint{
		{
			Method:       "POST",
			Path:         "/api/v1/auth/login",
			Headers:      map[string]string{"Content-Type": "application/json"},
			Body:         map[string]string{"username": "testuser", "password": "testpass"},
			ExpectedCode: 200,
		},
		{
			Method:       "GET",
			Path:         "/api/v1/user/profile",
			Headers:      map[string]string{"Authorization": "Bearer test-token"},
			ExpectedCode: 200,
		},
		{
			Method:       "POST",
			Path:         "/api/v1/orders",
			Headers:      map[string]string{"Authorization": "Bearer test-token", "Content-Type": "application/json"},
			Body:         createTestOrderRequest(),
			ExpectedCode: 201,
		},
		{
			Method:       "GET",
			Path:         "/api/v1/orderbook/BTC-USD",
			ExpectedCode: 200,
		},
		{
			Method:       "GET",
			Path:         "/api/v1/markets",
			ExpectedCode: 200,
		},
	}

	// Test each endpoint
	for _, endpoint := range endpoints {
		t.Run(fmt.Sprintf("%s_%s", endpoint.Method, endpoint.Path), func(t *testing.T) {
			response := suite.callAPI(endpoint)

			if response.Error != nil {
				t.Fatalf("API call failed: %v", response.Error)
			}

			if response.StatusCode != endpoint.ExpectedCode {
				t.Errorf("Unexpected status code: got %d, expected %d",
					response.StatusCode, endpoint.ExpectedCode)
			}

			// Validate response latency
			if response.Latency > 500*time.Millisecond {
				t.Errorf("API response too slow: %v (expected < 500ms)", response.Latency)
			}

			suite.metrics.AddLatency(response.Latency)
			suite.metrics.AddSuccess()
		})
	}

	suite.metrics.Calculate()

	t.Logf("API Endpoint Validation Results:")
	t.Logf("  Total Endpoints Tested: %d", len(endpoints))
	t.Logf("  Average Response Time: %v", suite.metrics.AverageLatency)
	t.Logf("  P95 Response Time: %v", suite.metrics.P95Latency)
	t.Logf("  Success Rate: %.2f%%", 100.0-suite.metrics.ErrorRate)
}

func TestAPIRateLimiting(t *testing.T) {
	config := DefaultTestConfig()
	env := NewTestEnvironment(config)
	server := NewMockAPIServer()
	defer server.Close()

	suite := &APITestSuite{
		config:      config,
		environment: env,
		server:      server,
		client:      &http.Client{Timeout: 10 * time.Second},
	}

	// Test rate limiting scenarios
	rateLimitTests := []RateLimitTest{
		{
			Endpoint:          "/api/v1/user/profile",
			RequestsPerSecond: 10,
			Duration:          10 * time.Second,
			ExpectedLimit:     100, // 10 req/s * 10s
		},
		{
			Endpoint:          "/api/v1/orderbook/BTC-USD",
			RequestsPerSecond: 50,
			Duration:          5 * time.Second,
			ExpectedLimit:     250, // 50 req/s * 5s
		},
		{
			Endpoint:          "/api/v1/orders",
			RequestsPerSecond: 5,
			Duration:          20 * time.Second,
			ExpectedLimit:     100, // 5 req/s * 20s
		},
	}

	for _, test := range rateLimitTests {
		t.Run(fmt.Sprintf("RateLimit_%s", test.Endpoint), func(t *testing.T) {
			var wg sync.WaitGroup
			var successCount, rateLimitCount int64
			var mu sync.Mutex

			startTime := time.Now()
			requestInterval := time.Second / time.Duration(test.RequestsPerSecond)

			// Launch rate limit test
			for time.Since(startTime) < test.Duration {
				wg.Add(1)
				go func() {
					defer wg.Done()

					endpoint := APIEndpoint{
						Method:       "GET",
						Path:         test.Endpoint,
						Headers:      map[string]string{"Authorization": "Bearer test-token"},
						ExpectedCode: 200,
					}

					response := suite.callAPI(endpoint)

					mu.Lock()
					if response.StatusCode == 429 { // Too Many Requests
						rateLimitCount++
					} else if response.StatusCode == 200 {
						successCount++
					}
					mu.Unlock()

					suite.metrics.AddLatency(response.Latency)
				}()

				time.Sleep(requestInterval)
			}

			wg.Wait()

			totalRequests := successCount + rateLimitCount

			// Validate rate limiting behavior
			if totalRequests > int64(test.ExpectedLimit*12/10) { // Allow 20% tolerance
				t.Errorf("Too many requests processed: %d (expected around %d)",
					totalRequests, test.ExpectedLimit)
			}

			if rateLimitCount == 0 && totalRequests > int64(test.ExpectedLimit) {
				t.Errorf("Rate limiting not enforced: processed %d requests without rate limit responses",
					totalRequests)
			}

			t.Logf("Rate Limit Test Results for %s:", test.Endpoint)
			t.Logf("  Total Requests: %d", totalRequests)
			t.Logf("  Successful Requests: %d", successCount)
			t.Logf("  Rate Limited Requests: %d", rateLimitCount)
			t.Logf("  Expected Limit: %d", test.ExpectedLimit)
		})
	}
}

func TestAPIAuthentication(t *testing.T) {
	config := DefaultTestConfig()
	env := NewTestEnvironment(config)
	server := NewMockAPIServer()
	defer server.Close()

	suite := &APITestSuite{
		config:      config,
		environment: env,
		server:      server,
		client:      &http.Client{Timeout: 10 * time.Second},
	}

	// Test authentication scenarios
	authTests := []struct {
		name         string
		endpoint     APIEndpoint
		expectedCode int
		description  string
	}{
		{
			name: "ValidLogin",
			endpoint: APIEndpoint{
				Method:  "POST",
				Path:    "/api/v1/auth/login",
				Headers: map[string]string{"Content-Type": "application/json"},
				Body:    map[string]string{"username": "testuser", "password": "testpass"},
			},
			expectedCode: 200,
			description:  "Valid login credentials should succeed",
		},
		{
			name: "InvalidLogin",
			endpoint: APIEndpoint{
				Method:  "POST",
				Path:    "/api/v1/auth/login",
				Headers: map[string]string{"Content-Type": "application/json"},
				Body:    map[string]string{"username": "testuser", "password": "wrongpass"},
			},
			expectedCode: 401,
			description:  "Invalid login credentials should fail",
		},
		{
			name: "UnauthorizedAccess",
			endpoint: APIEndpoint{
				Method: "GET",
				Path:   "/api/v1/user/profile",
			},
			expectedCode: 401,
			description:  "Access without token should fail",
		},
		{
			name: "InvalidToken",
			endpoint: APIEndpoint{
				Method:  "GET",
				Path:    "/api/v1/user/profile",
				Headers: map[string]string{"Authorization": "Bearer invalid-token"},
			},
			expectedCode: 401,
			description:  "Access with invalid token should fail",
		},
		{
			name: "ValidTokenAccess",
			endpoint: APIEndpoint{
				Method:  "GET",
				Path:    "/api/v1/user/profile",
				Headers: map[string]string{"Authorization": "Bearer valid-test-token"},
			},
			expectedCode: 200,
			description:  "Access with valid token should succeed",
		},
	}

	for _, test := range authTests {
		t.Run(test.name, func(t *testing.T) {
			response := suite.callAPI(test.endpoint)

			if response.Error != nil && test.expectedCode != 500 {
				t.Fatalf("API call failed: %v", response.Error)
			}

			if response.StatusCode != test.expectedCode {
				t.Errorf("%s: got status %d, expected %d",
					test.description, response.StatusCode, test.expectedCode)
			}

			suite.metrics.AddLatency(response.Latency)
			if response.StatusCode == test.expectedCode {
				suite.metrics.AddSuccess()
			} else {
				suite.metrics.AddFailure()
			}
		})
	}

	suite.metrics.Calculate()

	t.Logf("API Authentication Test Results:")
	t.Logf("  Tests Run: %d", len(authTests))
	t.Logf("  Success Rate: %.2f%%", 100.0-suite.metrics.ErrorRate)
	t.Logf("  Average Response Time: %v", suite.metrics.AverageLatency)
}

func TestAPIInputValidation(t *testing.T) {
	config := DefaultTestConfig()
	env := NewTestEnvironment(config)
	server := NewMockAPIServer()
	defer server.Close()

	suite := &APITestSuite{
		config:      config,
		environment: env,
		server:      server,
		client:      &http.Client{Timeout: 10 * time.Second},
	}

	// Test input validation scenarios
	validationTests := []struct {
		name         string
		endpoint     APIEndpoint
		expectedCode int
		description  string
	}{
		{
			name: "ValidOrderCreation",
			endpoint: APIEndpoint{
				Method:  "POST",
				Path:    "/api/v1/orders",
				Headers: map[string]string{"Authorization": "Bearer test-token", "Content-Type": "application/json"},
				Body:    createValidOrderRequest(),
			},
			expectedCode: 201,
			description:  "Valid order should be created",
		},
		{
			name: "InvalidOrderQuantity",
			endpoint: APIEndpoint{
				Method:  "POST",
				Path:    "/api/v1/orders",
				Headers: map[string]string{"Authorization": "Bearer test-token", "Content-Type": "application/json"},
				Body:    createInvalidQuantityOrderRequest(),
			},
			expectedCode: 400,
			description:  "Order with invalid quantity should be rejected",
		},
		{
			name: "InvalidOrderPrice",
			endpoint: APIEndpoint{
				Method:  "POST",
				Path:    "/api/v1/orders",
				Headers: map[string]string{"Authorization": "Bearer test-token", "Content-Type": "application/json"},
				Body:    createInvalidPriceOrderRequest(),
			},
			expectedCode: 400,
			description:  "Order with invalid price should be rejected",
		},
		{
			name: "MissingRequiredFields",
			endpoint: APIEndpoint{
				Method:  "POST",
				Path:    "/api/v1/orders",
				Headers: map[string]string{"Authorization": "Bearer test-token", "Content-Type": "application/json"},
				Body:    map[string]interface{}{"side": "buy"}, // Missing required fields
			},
			expectedCode: 400,
			description:  "Order with missing fields should be rejected",
		},
		{
			name: "InvalidTradingPair",
			endpoint: APIEndpoint{
				Method: "GET",
				Path:   "/api/v1/orderbook/INVALID-PAIR",
			},
			expectedCode: 404,
			description:  "Request for invalid trading pair should return 404",
		},
	}

	for _, test := range validationTests {
		t.Run(test.name, func(t *testing.T) {
			response := suite.callAPI(test.endpoint)

			if response.Error != nil && test.expectedCode < 500 {
				t.Fatalf("API call failed: %v", response.Error)
			}

			if response.StatusCode != test.expectedCode {
				t.Errorf("%s: got status %d, expected %d",
					test.description, response.StatusCode, test.expectedCode)
			}

			suite.metrics.AddLatency(response.Latency)
			if response.StatusCode == test.expectedCode {
				suite.metrics.AddSuccess()
			} else {
				suite.metrics.AddFailure()
			}
		})
	}

	suite.metrics.Calculate()

	t.Logf("API Input Validation Test Results:")
	t.Logf("  Tests Run: %d", len(validationTests))
	t.Logf("  Success Rate: %.2f%%", 100.0-suite.metrics.ErrorRate)
	t.Logf("  Average Response Time: %v", suite.metrics.AverageLatency)
}

// Helper methods for APITestSuite

func (suite *APITestSuite) callAPI(endpoint APIEndpoint) APIResponse {
	start := time.Now()

	var body io.Reader
	if endpoint.Body != nil {
		jsonBody, _ := json.Marshal(endpoint.Body)
		body = bytes.NewBuffer(jsonBody)
	}

	req, err := http.NewRequest(endpoint.Method, suite.server.URL+endpoint.Path, body)
	if err != nil {
		return APIResponse{Error: err, Latency: time.Since(start)}
	}

	// Set headers
	for key, value := range endpoint.Headers {
		req.Header.Set(key, value)
	}

	resp, err := suite.client.Do(req)
	if err != nil {
		return APIResponse{Error: err, Latency: time.Since(start)}
	}
	defer resp.Body.Close()

	latency := time.Since(start)

	// Read response body
	responseBody, _ := io.ReadAll(resp.Body)
	var parsedBody map[string]interface{}
	json.Unmarshal(responseBody, &parsedBody)

	// Convert headers to map
	headers := make(map[string]string)
	for key, values := range resp.Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}

	return APIResponse{
		StatusCode: resp.StatusCode,
		Headers:    headers,
		Body:       parsedBody,
		Latency:    latency,
	}
}

// Mock API handlers

func handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var loginReq map[string]string
	json.NewDecoder(r.Body).Decode(&loginReq)

	if loginReq["username"] == "testuser" && loginReq["password"] == "testpass" {
		response := map[string]interface{}{
			"token":      "valid-test-token",
			"expires_in": 3600,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	} else {
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
	}
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "Logged out successfully"})
}

func handleRefresh(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"token":      "refreshed-test-token",
		"expires_in": 3600,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func handleOrders(w http.ResponseWriter, r *http.Request) {
	// Check authentication
	if !isAuthenticated(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if r.Method == "POST" {
		var orderReq map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&orderReq); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		// Validate order request
		if !isValidOrderRequest(orderReq) {
			http.Error(w, "Invalid order request", http.StatusBadRequest)
			return
		}

		response := map[string]interface{}{
			"id":         uuid.New().String(),
			"status":     "pending",
			"created_at": time.Now().Format(time.RFC3339),
		}
		w.WriteHeader(http.StatusCreated)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	} else {
		// GET - return user's orders
		orders := []map[string]interface{}{
			{
				"id":       uuid.New().String(),
				"side":     "buy",
				"quantity": "1.0",
				"price":    "50000.00",
				"status":   "open",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(orders)
	}
}

func handleOrderByID(w http.ResponseWriter, r *http.Request) {
	if !isAuthenticated(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	order := map[string]interface{}{
		"id":       "test-order-id",
		"side":     "buy",
		"quantity": "1.0",
		"price":    "50000.00",
		"status":   "filled",
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(order)
}

func handleTrades(w http.ResponseWriter, r *http.Request) {
	trades := []map[string]interface{}{
		{
			"id":        uuid.New().String(),
			"price":     "50000.00",
			"quantity":  "0.5",
			"timestamp": time.Now().Format(time.RFC3339),
		},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(trades)
}

func handleOrderBook(w http.ResponseWriter, r *http.Request) {
	orderbook := map[string]interface{}{
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
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(orderbook)
}

func handleUserProfile(w http.ResponseWriter, r *http.Request) {
	if !isAuthenticated(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	profile := map[string]interface{}{
		"id":       uuid.New().String(),
		"username": "testuser",
		"email":    "test@example.com",
		"verified": true,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(profile)
}

func handleUserBalance(w http.ResponseWriter, r *http.Request) {
	if !isAuthenticated(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	balance := map[string]interface{}{
		"BTC": "0.5",
		"USD": "25000.00",
		"ETH": "10.0",
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(balance)
}

func handleUserOrders(w http.ResponseWriter, r *http.Request) {
	if !isAuthenticated(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	orders := []map[string]interface{}{
		{
			"id":       uuid.New().String(),
			"side":     "buy",
			"quantity": "1.0",
			"price":    "50000.00",
			"status":   "open",
		},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(orders)
}

func handleTicker(w http.ResponseWriter, r *http.Request) {
	ticker := map[string]interface{}{
		"price":  "50000.00",
		"volume": "1000.5",
		"change": "2.5",
		"high":   "50500.00",
		"low":    "49500.00",
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ticker)
}

func handleMarkets(w http.ResponseWriter, r *http.Request) {
	markets := []map[string]interface{}{
		{
			"symbol": "BTC-USD",
			"base":   "BTC",
			"quote":  "USD",
			"active": true,
		},
		{
			"symbol": "ETH-USD",
			"base":   "ETH",
			"quote":  "USD",
			"active": true,
		},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(markets)
}

func handleCandles(w http.ResponseWriter, r *http.Request) {
	candles := [][]interface{}{
		{time.Now().Unix(), "50000.00", "50100.00", "49900.00", "50050.00", "100.5"},
		{time.Now().Unix() - 3600, "49900.00", "50000.00", "49800.00", "50000.00", "95.2"},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(candles)
}

// Helper functions

func isAuthenticated(r *http.Request) bool {
	auth := r.Header.Get("Authorization")
	return auth == "Bearer test-token" || auth == "Bearer valid-test-token"
}

func isValidOrderRequest(req map[string]interface{}) bool {
	// Check required fields
	required := []string{"side", "quantity", "trading_pair_id"}
	for _, field := range required {
		if _, exists := req[field]; !exists {
			return false
		}
	}

	// Validate quantity
	if qty, ok := req["quantity"].(string); ok {
		if qtyDecimal, err := decimal.NewFromString(qty); err != nil || qtyDecimal.LessThanOrEqual(decimal.Zero) {
			return false
		}
	} else {
		return false
	}

	// Validate price if provided
	if price, exists := req["price"]; exists {
		if priceStr, ok := price.(string); ok {
			if priceDecimal, err := decimal.NewFromString(priceStr); err != nil || priceDecimal.LessThanOrEqual(decimal.Zero) {
				return false
			}
		} else {
			return false
		}
	}

	return true
}

func createTestOrderRequest() map[string]interface{} {
	return map[string]interface{}{
		"side":            "buy",
		"quantity":        "1.0",
		"price":           "50000.00",
		"trading_pair_id": "BTC-USD",
		"type":            "limit",
	}
}

func createValidOrderRequest() map[string]interface{} {
	return map[string]interface{}{
		"side":            "buy",
		"quantity":        "1.0",
		"price":           "50000.00",
		"trading_pair_id": "BTC-USD",
		"type":            "limit",
	}
}

func createInvalidQuantityOrderRequest() map[string]interface{} {
	return map[string]interface{}{
		"side":            "buy",
		"quantity":        "-1.0", // Invalid negative quantity
		"price":           "50000.00",
		"trading_pair_id": "BTC-USD",
		"type":            "limit",
	}
}

func createInvalidPriceOrderRequest() map[string]interface{} {
	return map[string]interface{}{
		"side":            "buy",
		"quantity":        "1.0",
		"price":           "0.00", // Invalid zero price
		"trading_pair_id": "BTC-USD",
		"type":            "limit",
	}
}
