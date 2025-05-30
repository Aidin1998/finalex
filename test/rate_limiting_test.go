package test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/auth"
	"github.com/Aidin1998/pincex_unified/internal/bookkeeper"
	"github.com/Aidin1998/pincex_unified/internal/fiat"
	"github.com/Aidin1998/pincex_unified/internal/identities"
	"github.com/Aidin1998/pincex_unified/internal/kyc"
	"github.com/Aidin1998/pincex_unified/internal/marketdata"
	"github.com/Aidin1998/pincex_unified/internal/marketfeeds"
	"github.com/Aidin1998/pincex_unified/internal/server"
	"github.com/Aidin1998/pincex_unified/internal/settlement"
	"github.com/Aidin1998/pincex_unified/internal/trading"
	"github.com/Aidin1998/pincex_unified/internal/ws"
	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Test UserService adapter for rate limiter
type testUserService struct {
	users map[string]*models.User
}

func newTestUserService() *testUserService {
	return &testUserService{
		users: map[string]*models.User{
			"basic-user": {
				ID:    models.NewUUID(),
				Email: "basic@test.com",
				Tier:  models.TierBasic,
			},
			"premium-user": {
				ID:    models.NewUUID(),
				Email: "premium@test.com",
				Tier:  models.TierPremium,
			},
			"vip-user": {
				ID:    models.NewUUID(),
				Email: "vip@test.com",
				Tier:  models.TierVIP,
			},
		},
	}
}

func (t *testUserService) GetUser(ctx context.Context, userID string) (*models.User, error) {
	for _, user := range t.users {
		if user.ID.String() == userID {
			return user, nil
		}
	}
	return &models.User{
		ID:   models.NewUUID(),
		Tier: models.TierBasic, // Default to basic tier
	}, nil
}

// Stub PubSub for testing
type stubPubSub struct{}

func (s *stubPubSub) Publish(ctx context.Context, channel string, msg interface{}) error {
	return nil
}

func (s *stubPubSub) Subscribe(ctx context.Context, channel string, handler func([]byte)) error {
	return nil
}

func main() {
	// Setup logger
	logger, _ := zap.NewDevelopment()
	gin.SetMode(gin.TestMode)

	// Setup in-memory database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Migrate models
	err = db.AutoMigrate(
		&models.User{},
		&models.Account{},
		&models.Transaction{},
		&models.TradingPair{},
		&models.Order{},
		&models.Trade{},
	)
	if err != nil {
		log.Fatalf("Failed to migrate database: %v", err)
	}
	// Create services with stubs
	authSvc := &auth.AuthService{} // Create mock auth service first
	identitiesSvc, _ := identities.NewService(logger, db, authSvc)
	bookkeeperSvc, _ := bookkeeper.NewService(logger, db)
	kycProvider := &stubKYCProvider{}
	kycService := kyc.NewKYCService(kycProvider)
	fiatSvc, _ := fiat.NewService(logger, db, bookkeeperSvc, kycService)
	pubsub := &stubPubSub{}
	marketfeedsSvc, _ := marketfeeds.NewService(logger, db, pubsub)
	wsHub := ws.NewHub(2, 50)                          // Small test WebSocket hub
	settlementEngine := &settlement.SettlementEngine{} // Mock settlement engine
	tradingSvc, _ := trading.NewService(logger, db, bookkeeperSvc, wsHub, settlementEngine)

	// Create auth service with in-memory rate limiter
	inMemoryRateLimiter := auth.NewInMemoryRateLimiter()
	authSvc := auth.NewAuthService(logger, db, inMemoryRateLimiter)

	// Create market data hub
	marketDataHub := marketdata.NewHub(authSvc)
	go marketDataHub.Run()

	// Create tiered rate limiter with test user service
	testUserSvc := newTestUserService()
	userServiceAdapter := &auth.AuthUserService{
		GetUserFunc: testUserSvc.GetUser,
	}
	tieredRateLimiter := auth.NewTieredRateLimiter(inMemoryRateLimiter, userServiceAdapter)

	// Create server
	srv := server.NewServer(
		logger,
		authSvc,
		identitiesSvc,
		bookkeeperSvc,
		fiatSvc,
		marketfeedsSvc,
		tradingSvc,
		marketDataHub,
		tieredRateLimiter,
	)

	// Create test server
	testServer := httptest.NewServer(srv.Router())
	defer testServer.Close()

	fmt.Printf("🚀 Test server started at %s\n", testServer.URL)

	// Run rate limiting tests
	runRateLimitTests(testServer.URL)

	// Run risk admin API tests
	runRiskAdminApiTests(testServer.URL)
}

// --- Rate Limiting Tests ---
func runRateLimitTests(baseURL string) {
	fmt.Println("\n📊 Running Rate Limiting Tests...")

	// Test 1: Health check should work without rate limiting issues
	fmt.Println("\n1. Testing health check endpoint...")
	resp, err := http.Get(baseURL + "/health")
	if err != nil {
		log.Printf("❌ Health check failed: %v", err)
		return
	}
	defer resp.Body.Close()
	fmt.Printf("✅ Health check status: %d\n", resp.StatusCode)

	// Test 2: Test rate limiting on API endpoints (without auth for now)
	fmt.Println("\n2. Testing rate limiting on market data endpoint...")
	testRateLimitEndpoint(baseURL+"/api/v1/market/prices", "GET", "", 20)

	// Test 3: Test admin rate limit configuration endpoints (these will need auth)
	fmt.Println("\n3. Testing admin endpoints existence...")
	testEndpointExists(baseURL+"/api/v1/admin/rate-limits/config", "GET")

	// Test 4: Test rate limit headers
	fmt.Println("\n4. Testing rate limit headers...")
	testRateLimitHeaders(baseURL + "/api/v1/market/prices")

	// Test 5: Test emergency mode and configuration
	fmt.Println("\n5. Testing rate limit configuration...")
	testRateLimitConfig(baseURL)

	fmt.Println("\n✅ Rate limiting tests completed!")
}

func testRateLimitEndpoint(url, method, authToken string, requests int) {
	successCount := 0
	rateLimitedCount := 0

	client := &http.Client{Timeout: 5 * time.Second}

	for i := 0; i < requests; i++ {
		req, _ := http.NewRequest(method, url, nil)
		if authToken != "" {
			req.Header.Set("Authorization", "Bearer "+authToken)
		}

		resp, err := client.Do(req)
		if err != nil {
			log.Printf("Request %d failed: %v", i+1, err)
			continue
		}
		resp.Body.Close()

		switch resp.StatusCode {
		case http.StatusOK:
			successCount++
		case http.StatusTooManyRequests:
			rateLimitedCount++
			fmt.Printf("⚠️  Request %d rate limited (429)\n", i+1)
		default:
			fmt.Printf("⚠️  Request %d returned status %d\n", i+1, resp.StatusCode)
		}

		// Small delay between requests
		time.Sleep(50 * time.Millisecond)
	}

	fmt.Printf("📈 Results: %d successful, %d rate limited out of %d requests\n",
		successCount, rateLimitedCount, requests)
}

func testEndpointExists(url, method string) {
	req, _ := http.NewRequest(method, url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("❌ Endpoint %s %s unreachable: %v\n", method, url, err)
		return
	}
	defer resp.Body.Close()

	// We expect 401 or 403 for admin endpoints without auth
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		fmt.Printf("✅ Endpoint %s %s exists (requires auth)\n", method, url)
	} else {
		fmt.Printf("⚠️  Endpoint %s %s returned status %d\n", method, url, resp.StatusCode)
	}
}

func testRateLimitHeaders(url string) {
	resp, err := http.Get(url)
	if err != nil {
		fmt.Printf("❌ Failed to test headers: %v\n", err)
		return
	}
	defer resp.Body.Close()

	headers := []string{"X-RateLimit-Limit", "X-RateLimit-Remaining", "X-RateLimit-Reset"}
	found := 0
	for _, header := range headers {
		if value := resp.Header.Get(header); value != "" {
			fmt.Printf("✅ Header %s: %s\n", header, value)
			found++
		}
	}

	if found == 0 {
		fmt.Printf("⚠️  No rate limit headers found\n")
	} else {
		fmt.Printf("📊 Found %d/3 rate limit headers\n", found)
	}
}

func testRateLimitConfig(baseURL string) {
	// Test getting rate limit configuration
	resp, err := http.Get(baseURL + "/api/v1/admin/rate-limits/config")
	if err != nil {
		fmt.Printf("❌ Failed to get config: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		fmt.Printf("✅ Rate limit config endpoint properly protected\n")
	} else {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("📋 Config endpoint response (%d): %s\n", resp.StatusCode, string(body))
	}

	// Test emergency mode endpoint
	emergencyData := map[string]bool{"enabled": true}
	jsonData, _ := json.Marshal(emergencyData)
	resp, err = http.Post(baseURL+"/api/v1/admin/rate-limits/emergency-mode",
		"application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Printf("❌ Failed to test emergency mode: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		fmt.Printf("✅ Emergency mode endpoint properly protected\n")
	} else {
		fmt.Printf("📋 Emergency mode response: %d\n", resp.StatusCode)
	}
}

// --- Risk Admin API Tests ---
func runRiskAdminApiTests(baseURL string) {
	fmt.Println("\n🔒 Running Risk Admin API Endpoint Tests...")

	// 1. Check endpoints exist and are protected
	testEndpointExists(baseURL+"/api/v1/admin/risk/limits", "GET")
	testEndpointExists(baseURL+"/api/v1/admin/risk/limits", "POST")
	testEndpointExists(baseURL+"/api/v1/admin/risk/limits/BTCUSDT", "PUT")
	testEndpointExists(baseURL+"/api/v1/admin/risk/limits/BTCUSDT", "DELETE")

	testEndpointExists(baseURL+"/api/v1/admin/risk/exemptions", "GET")
	testEndpointExists(baseURL+"/api/v1/admin/risk/exemptions", "POST")
	testEndpointExists(baseURL+"/api/v1/admin/risk/exemptions/user-123", "DELETE")

	// 2. Try CRUD operations (should get 401/403 if unauthenticated)
	testRiskLimitCRUD(baseURL)
	testRiskExemptionCRUD(baseURL)

	fmt.Println("\n✅ Risk admin API endpoint tests completed!")
}

func testRiskLimitCRUD(baseURL string) {
	fmt.Println("\n- Testing risk limit CRUD endpoints...")
	// POST create limit
	limit := map[string]interface{}{"symbol": "BTCUSDT", "limit": 42.0}
	jsonData, _ := json.Marshal(limit)
	resp, err := http.Post(baseURL+"/api/v1/admin/risk/limits", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Printf("❌ POST risk limit failed: %v\n", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		fmt.Printf("✅ POST /risk/limits properly protected\n")
	} else {
		fmt.Printf("⚠️  POST /risk/limits returned status %d\n", resp.StatusCode)
	}

	// PUT update limit
	req, _ := http.NewRequest("PUT", baseURL+"/api/v1/admin/risk/limits/BTCUSDT", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("❌ PUT risk limit failed: %v\n", err)
		return
	}
	defer resp2.Body.Close()
	if resp2.StatusCode == http.StatusUnauthorized || resp2.StatusCode == http.StatusForbidden {
		fmt.Printf("✅ PUT /risk/limits properly protected\n")
	} else {
		fmt.Printf("⚠️  PUT /risk/limits returned status %d\n", resp2.StatusCode)
	}

	// DELETE limit
	req, _ = http.NewRequest("DELETE", baseURL+"/api/v1/admin/risk/limits/BTCUSDT", nil)
	resp3, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("❌ DELETE risk limit failed: %v\n", err)
		return
	}
	defer resp3.Body.Close()
	if resp3.StatusCode == http.StatusUnauthorized || resp3.StatusCode == http.StatusForbidden {
		fmt.Printf("✅ DELETE /risk/limits properly protected\n")
	} else {
		fmt.Printf("⚠️  DELETE /risk/limits returned status %d\n", resp3.StatusCode)
	}
}

func testRiskExemptionCRUD(baseURL string) {
	fmt.Println("\n- Testing risk exemption CRUD endpoints...")
	// POST add exemption
	ex := map[string]interface{}{"account_id": "user-123"}
	jsonData, _ := json.Marshal(ex)
	resp, err := http.Post(baseURL+"/api/v1/admin/risk/exemptions", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Printf("❌ POST risk exemption failed: %v\n", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		fmt.Printf("✅ POST /risk/exemptions properly protected\n")
	} else {
		fmt.Printf("⚠️  POST /risk/exemptions returned status %d\n", resp.StatusCode)
	}

	// DELETE exemption
	req, _ := http.NewRequest("DELETE", baseURL+"/api/v1/admin/risk/exemptions/user-123", nil)
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("❌ DELETE risk exemption failed: %v\n", err)
		return
	}
	defer resp2.Body.Close()
	if resp2.StatusCode == http.StatusUnauthorized || resp2.StatusCode == http.StatusForbidden {
		fmt.Printf("✅ DELETE /risk/exemptions properly protected\n")
	} else {
		fmt.Printf("⚠️  DELETE /risk/exemptions returned status %d\n", resp2.StatusCode)
	}
}

// Stub KYC provider for testing
type stubKYCProvider struct{}

func (s *stubKYCProvider) StartVerification(userID string, data *kyc.KYCData) (string, error) {
	return "session-" + userID, nil
}

func (s *stubKYCProvider) GetStatus(sessionID string) (kyc.KYCStatus, error) {
	return kyc.KYCStatus{Status: "pending", Message: "Verification in progress"}, nil
}

func (s *stubKYCProvider) WebhookHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
