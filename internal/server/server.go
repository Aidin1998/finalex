package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/Aidin1998/pincex_unified/internal/auth"
	"github.com/Aidin1998/pincex_unified/internal/bookkeeper"
	"github.com/Aidin1998/pincex_unified/internal/fiat"
	"github.com/Aidin1998/pincex_unified/internal/identities"
	"github.com/Aidin1998/pincex_unified/internal/marketdata"
	"github.com/Aidin1998/pincex_unified/internal/marketfeeds"
	"github.com/Aidin1998/pincex_unified/internal/trading"
	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/Aidin1998/pincex_unified/pkg/validation"

	"github.com/gin-contrib/cors"
	ginzap "github.com/gin-contrib/zap"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.uber.org/zap"
)

// Server represents the HTTP server
type Server struct {
	logger            *zap.Logger
	authSvc           auth.AuthService
	identitiesSvc     identities.IdentityService
	bookkeeperSvc     bookkeeper.BookkeeperService
	fiatSvc           fiat.FiatService
	marketfeedsSvc    marketfeeds.MarketFeedService
	tradingSvc        trading.TradingService
	marketDataHub     *marketdata.Hub
	tieredRateLimiter *auth.TieredRateLimiter
}

// NewServer creates a new HTTP server
func NewServer(
	logger *zap.Logger,
	authSvc auth.AuthService,
	identitiesSvc identities.IdentityService,
	bookkeeperSvc bookkeeper.BookkeeperService,
	fiatSvc fiat.FiatService,
	marketfeedsSvc marketfeeds.MarketFeedService,
	tradingSvc trading.TradingService,
	marketDataHub *marketdata.Hub,
	tieredRateLimiter *auth.TieredRateLimiter,
) *Server {
	return &Server{
		logger:            logger,
		authSvc:           authSvc,
		identitiesSvc:     identitiesSvc,
		bookkeeperSvc:     bookkeeperSvc,
		fiatSvc:           fiatSvc,
		marketfeedsSvc:    marketfeedsSvc,
		tradingSvc:        tradingSvc,
		marketDataHub:     marketDataHub,
		tieredRateLimiter: tieredRateLimiter,
	}
}

// Router creates a new HTTP router
func (s *Server) Router() *gin.Engine {
	// Create router
	router := gin.New()

	// Add middleware
	router.Use(ginzap.Ginzap(s.logger, "2006-01-02T15:04:05Z07:00", true))
	router.Use(ginzap.RecoveryWithZap(s.logger, true))
	router.Use(otelgin.Middleware("pincex"))
	router.Use(cors.Default())

	// Add comprehensive input validation middleware
	router.Use(validation.ValidationMiddleware(s.logger))
	router.Use(validation.RequestValidationMiddleware())

	// Add tiered rate limiting middleware (if available)
	if s.tieredRateLimiter != nil {
		router.Use(s.tieredRateLimiter.Middleware())
	}

	// Add health check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Add WebSocket route for market data
	router.GET("/ws/marketdata", s.handleWebSocketMarketData)

	// Add API routes
	api := router.Group("/api")
	{
		// Add v1 routes
		v1 := api.Group("/v1")
		{
			// Add identities routes
			identities := v1.Group("/identities")
			{
				identities.POST("/register", s.handleRegister)
				identities.POST("/login", s.handleLogin)
				identities.POST("/logout", s.authMiddleware(), s.handleLogout)
				identities.POST("/refresh", s.handleRefreshToken)
				identities.POST("/2fa/enable", s.authMiddleware(), s.handle2FAEnable)
				identities.POST("/2fa/verify", s.authMiddleware(), s.handle2FAVerify)
				identities.POST("/2fa/disable", s.authMiddleware(), s.handle2FADisable)
				identities.GET("/me", s.authMiddleware(), s.handleGetMe)
				identities.PUT("/me", s.authMiddleware(), s.handleUpdateMe)
				identities.POST("/kyc/submit", s.authMiddleware(), s.handleKYCSubmit)
				identities.GET("/kyc/status", s.authMiddleware(), s.handleKYCStatus)
			}

			// Add accounts routes
			accounts := v1.Group("/accounts", s.authMiddleware())
			{
				accounts.GET("", s.handleGetAccounts)
				accounts.GET("/:currency", s.handleGetAccount)
				accounts.GET("/:currency/transactions", s.handleGetAccountTransactions)
			}

			// Add trading routes
			trading := v1.Group("/trading", s.authMiddleware())
			{
				trading.GET("/pairs", s.handleGetTradingPairs)
				trading.GET("/pairs/:symbol", s.handleGetTradingPair)
				trading.POST("/orders", s.handlePlaceOrder)
				trading.GET("/orders", s.handleGetOrders)
				trading.GET("/orders/:id", s.handleGetOrder)
				trading.DELETE("/orders/:id", s.handleCancelOrder)
				trading.GET("/orderbook/:symbol", s.handleGetOrderBook)
			}

			// Add market routes
			market := v1.Group("/market")
			{
				market.GET("/prices", s.handleGetMarketPrices)
				market.GET("/prices/:symbol", s.handleGetMarketPrice)
				market.GET("/candles/:symbol", s.handleGetCandles)
			}

			// Add fiat routes
			fiat := v1.Group("/fiat", s.authMiddleware())
			{
				fiat.POST("/deposit", s.handleFiatDeposit)
				fiat.POST("/withdraw", s.handleFiatWithdraw)
				fiat.GET("/deposits", s.handleGetFiatDeposits)
				fiat.GET("/withdrawals", s.handleGetFiatWithdrawals)
			}

			// Add admin routes
			admin := v1.Group("/admin", s.authMiddleware(), s.adminMiddleware())
			{
				admin.POST("/trading/pairs", s.handleCreateTradingPair)
				admin.PUT("/trading/pairs/:symbol", s.handleUpdateTradingPair)
				admin.GET("/users", s.handleGetUsers)
				admin.GET("/users/:id", s.handleGetUser)
				admin.PUT("/users/:id/kyc", s.handleUpdateUserKYC)

				// Rate limiting management endpoints
				rateLimit := admin.Group("/rate-limits")
				{
					rateLimit.GET("/config", s.handleGetRateLimitConfig)
					rateLimit.PUT("/config", s.handleUpdateRateLimitConfig)
					rateLimit.POST("/emergency-mode", s.handleSetEmergencyMode)
					rateLimit.PUT("/tiers/:tier", s.handleUpdateTierLimits)
					rateLimit.PUT("/endpoints", s.handleUpdateEndpointConfig)
					rateLimit.GET("/users/:userID/status", s.handleGetUserRateLimitStatus)
					rateLimit.GET("/ips/:ip/status", s.handleGetIPRateLimitStatus)
					rateLimit.DELETE("/users/:userID/:rateType", s.handleResetUserRateLimit)
					rateLimit.DELETE("/ips/:ip/:endpoint", s.handleResetIPRateLimit)
					rateLimit.POST("/cleanup", s.handleCleanupRateLimitData)
				}
			}
		}
	}

	return router
}

// httpError interface for errors with status code
type httpError interface {
	error
	StatusCode() int
}

// errorMapper maps error messages to HTTP status codes
type errorMapper struct{}

func (m *errorMapper) mapError(err error) int {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "unauthorized"):
		return http.StatusUnauthorized
	case strings.Contains(msg, "forbidden"):
		return http.StatusForbidden
	case strings.Contains(msg, "not found"):
		return http.StatusNotFound
	default:
		return http.StatusInternalServerError
	}
}

// writeError writes a JSON error response with mapped status
func (s *Server) writeError(c *gin.Context, err error) {
	status := (&errorMapper{}).mapError(err)
	c.JSON(status, gin.H{"error": err.Error()})
}

// authMiddleware creates a middleware for authentication
func (s *Server) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get token from header
		token := c.GetHeader("Authorization")
		if token == "" {
			s.writeError(c, fmt.Errorf("unauthorized: missing authorization header"))
			c.Abort()
			return
		}

		// Remove "Bearer " prefix if present
		if strings.HasPrefix(token, "Bearer ") {
			token = token[7:]
		}

		var userID string
		var err error

		// Try unified auth service first
		if s.authSvc != nil {
			claims, authErr := s.authSvc.ValidateToken(c.Request.Context(), token)
			if authErr == nil {
				userID = claims.UserID.String()
				// Set additional context from claims
				c.Set("userEmail", claims.Email)
				c.Set("userRole", claims.Role)
				c.Set("userPermissions", claims.Permissions)
				c.Set("sessionID", claims.SessionID.String())
			} else {
				err = authErr
			}
		} else {
			// Fallback to identities service
			userID, err = s.identitiesSvc.ValidateToken(token)
		}

		if err != nil {
			s.writeError(c, fmt.Errorf("unauthorized: %w", err))
			c.Abort()
			return
		}

		// Set user ID in context
		c.Set("userID", userID)
		c.Next()
	}
}

// adminMiddleware creates a middleware for admin authentication
func (s *Server) adminMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get user ID from context
		userID, exists := c.Get("userID")
		if !exists {
			s.writeError(c, fmt.Errorf("unauthorized: missing user ID"))
			c.Abort()
			return
		}

		// Check if user is admin
		isAdmin, err := s.identitiesSvc.IsAdmin(userID.(string))
		if err != nil {
			s.writeError(c, fmt.Errorf("internal error: %w", err))
			c.Abort()
			return
		}

		if !isAdmin {
			s.writeError(c, fmt.Errorf("forbidden: admin access required"))
			c.Abort()
			return
		}

		c.Next()
	}
}

// handleRegister handles user registration
func (s *Server) handleRegister(c *gin.Context) {
	// Implementation will be added in identities service
	c.JSON(http.StatusOK, gin.H{"message": "registration successful"})
}

// handleLogin handles user login
func (s *Server) handleLogin(c *gin.Context) {
	// Implementation will be added in identities service
	c.JSON(http.StatusOK, gin.H{"message": "login successful"})
}

// handleLogout handles user logout
func (s *Server) handleLogout(c *gin.Context) {
	// Implementation will be added in identities service
	c.JSON(http.StatusOK, gin.H{"message": "logout successful"})
}

// handleRefreshToken handles token refresh
func (s *Server) handleRefreshToken(c *gin.Context) {
	// Implementation will be added in identities service
	c.JSON(http.StatusOK, gin.H{"message": "token refreshed"})
}

// handle2FAEnable handles 2FA enablement
func (s *Server) handle2FAEnable(c *gin.Context) {
	// Implementation will be added in identities service
	c.JSON(http.StatusOK, gin.H{"message": "2FA enabled"})
}

// handle2FAVerify handles 2FA verification
func (s *Server) handle2FAVerify(c *gin.Context) {
	// Implementation will be added in identities service
	c.JSON(http.StatusOK, gin.H{"message": "2FA verified"})
}

// handle2FADisable handles 2FA disablement
func (s *Server) handle2FADisable(c *gin.Context) {
	// Implementation will be added in identities service
	c.JSON(http.StatusOK, gin.H{"message": "2FA disabled"})
}

// handleGetMe handles getting the current user
func (s *Server) handleGetMe(c *gin.Context) {
	// Implementation will be added in identities service
	c.JSON(http.StatusOK, gin.H{"message": "user retrieved"})
}

// handleUpdateMe handles updating the current user
func (s *Server) handleUpdateMe(c *gin.Context) {
	// Implementation will be added in identities service
	c.JSON(http.StatusOK, gin.H{"message": "user updated"})
}

// handleKYCSubmit handles KYC submission
func (s *Server) handleKYCSubmit(c *gin.Context) {
	// Implementation will be added in identities service
	c.JSON(http.StatusOK, gin.H{"message": "KYC submitted"})
}

// handleKYCStatus handles getting KYC status
func (s *Server) handleKYCStatus(c *gin.Context) {
	// Implementation will be added in identities service
	c.JSON(http.StatusOK, gin.H{"message": "KYC status retrieved"})
}

// handleGetAccounts handles getting accounts
func (s *Server) handleGetAccounts(c *gin.Context) {
	// Implementation will be added in bookkeeper service
	c.JSON(http.StatusOK, gin.H{"message": "accounts retrieved"})
}

// handleGetAccount handles getting an account
func (s *Server) handleGetAccount(c *gin.Context) {
	// Implementation will be added in bookkeeper service
	c.JSON(http.StatusOK, gin.H{"message": "account retrieved"})
}

// handleGetAccountTransactions handles getting account transactions
func (s *Server) handleGetAccountTransactions(c *gin.Context) {
	// Implementation will be added in bookkeeper service
	c.JSON(http.StatusOK, gin.H{"message": "account transactions retrieved"})
}

// handleGetTradingPairs handles getting trading pairs
func (s *Server) handleGetTradingPairs(c *gin.Context) {
	// Implementation will be added in trading service
	c.JSON(http.StatusOK, gin.H{"message": "trading pairs retrieved"})
}

// handleGetTradingPair handles getting a trading pair
func (s *Server) handleGetTradingPair(c *gin.Context) {
	// Implementation will be added in trading service
	c.JSON(http.StatusOK, gin.H{"message": "trading pair retrieved"})
}

// handlePlaceOrder handles placing an order
func (s *Server) handlePlaceOrder(c *gin.Context) {
	// Implementation will be added in trading service
	c.JSON(http.StatusOK, gin.H{"message": "order placed"})
}

// handleGetOrders handles getting orders
func (s *Server) handleGetOrders(c *gin.Context) {
	// Implementation will be added in trading service
	c.JSON(http.StatusOK, gin.H{"message": "orders retrieved"})
}

// handleGetOrder handles getting an order
func (s *Server) handleGetOrder(c *gin.Context) {
	// Implementation will be added in trading service
	c.JSON(http.StatusOK, gin.H{"message": "order retrieved"})
}

// handleCancelOrder handles canceling an order
func (s *Server) handleCancelOrder(c *gin.Context) {
	// Implementation will be added in trading service
	c.JSON(http.StatusOK, gin.H{"message": "order canceled"})
}

// handleGetOrderBook handles getting the order book
func (s *Server) handleGetOrderBook(c *gin.Context) {
	// Implementation will be added in trading service
	c.JSON(http.StatusOK, gin.H{"message": "order book retrieved"})
}

// handleGetMarketPrices handles getting market prices
func (s *Server) handleGetMarketPrices(c *gin.Context) {
	// Implementation will be added in marketfeeds service
	c.JSON(http.StatusOK, gin.H{"message": "market prices retrieved"})
}

// handleGetMarketPrice handles getting a market price
func (s *Server) handleGetMarketPrice(c *gin.Context) {
	// Implementation will be added in marketfeeds service
	c.JSON(http.StatusOK, gin.H{"message": "market price retrieved"})
}

// handleGetCandles handles getting candles
func (s *Server) handleGetCandles(c *gin.Context) {
	// Implementation will be added in marketfeeds service
	c.JSON(http.StatusOK, gin.H{"message": "candles retrieved"})
}

// handleFiatDeposit handles fiat deposit
func (s *Server) handleFiatDeposit(c *gin.Context) {
	// Implementation will be added in fiat service
	c.JSON(http.StatusOK, gin.H{"message": "fiat deposit initiated"})
}

// handleFiatWithdraw handles fiat withdrawal
func (s *Server) handleFiatWithdraw(c *gin.Context) {
	// Implementation will be added in fiat service
	c.JSON(http.StatusOK, gin.H{"message": "fiat withdrawal initiated"})
}

// handleGetFiatDeposits handles getting fiat deposits
func (s *Server) handleGetFiatDeposits(c *gin.Context) {
	// Implementation will be added in fiat service
	c.JSON(http.StatusOK, gin.H{"message": "fiat deposits retrieved"})
}

// handleGetFiatWithdrawals handles getting fiat withdrawals
func (s *Server) handleGetFiatWithdrawals(c *gin.Context) {
	// Implementation will be added in fiat service
	c.JSON(http.StatusOK, gin.H{"message": "fiat withdrawals retrieved"})
}

// handleCreateTradingPair handles creating a trading pair
func (s *Server) handleCreateTradingPair(c *gin.Context) {
	// Implementation will be added in trading service
	c.JSON(http.StatusOK, gin.H{"message": "trading pair created"})
}

// handleUpdateTradingPair handles updating a trading pair
func (s *Server) handleUpdateTradingPair(c *gin.Context) {
	// Implementation will be added in trading service
	c.JSON(http.StatusOK, gin.H{"message": "trading pair updated"})
}

// handleGetUsers handles getting users
func (s *Server) handleGetUsers(c *gin.Context) {
	// Implementation will be added in identities service
	c.JSON(http.StatusOK, gin.H{"message": "users retrieved"})
}

// handleGetUser handles getting a user
func (s *Server) handleGetUser(c *gin.Context) {
	// Implementation will be added in identities service
	c.JSON(http.StatusOK, gin.H{"message": "user retrieved"})
}

// handleUpdateUserKYC handles updating user KYC status
func (s *Server) handleUpdateUserKYC(c *gin.Context) {
	// Implementation will be added in identities service
	c.JSON(http.StatusOK, gin.H{"message": "user KYC updated"})
}

// handleWebSocketMarketData handles WebSocket connections for market data
func (s *Server) handleWebSocketMarketData(c *gin.Context) {
	if s.marketDataHub == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "market data service unavailable"})
		return
	}

	// Upgrade to WebSocket using the marketdata Hub's ServeWS method
	s.marketDataHub.ServeWS(c.Writer, c.Request)
}

// Rate limiting admin handlers

// handleGetRateLimitConfig returns the current rate limiting configuration
func (s *Server) handleGetRateLimitConfig(c *gin.Context) {
	if s.tieredRateLimiter == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Rate limiting not available"})
		return
	}

	config := s.tieredRateLimiter.GetConfig()
	c.JSON(http.StatusOK, config)
}

// handleUpdateRateLimitConfig updates the rate limiting configuration
func (s *Server) handleUpdateRateLimitConfig(c *gin.Context) {
	if s.tieredRateLimiter == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Rate limiting not available"})
		return
	}

	var config auth.RateLimitConfig
	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid configuration", "details": err.Error()})
		return
	}

	s.tieredRateLimiter.UpdateConfig(&config)
	c.JSON(http.StatusOK, gin.H{"message": "Rate limit configuration updated"})
}

// handleSetEmergencyMode enables or disables emergency mode
func (s *Server) handleSetEmergencyMode(c *gin.Context) {
	if s.tieredRateLimiter == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Rate limiting not available"})
		return
	}

	var request struct {
		Enabled bool `json:"enabled"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request", "details": err.Error()})
		return
	}

	s.tieredRateLimiter.SetEmergencyMode(request.Enabled)
	c.JSON(http.StatusOK, gin.H{"message": "Emergency mode updated", "enabled": request.Enabled})
}

// handleUpdateTierLimits updates rate limits for a specific tier
func (s *Server) handleUpdateTierLimits(c *gin.Context) {
	if s.tieredRateLimiter == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Rate limiting not available"})
		return
	}

	tierParam := c.Param("tier")
	var limits auth.TierConfig

	if err := c.ShouldBindJSON(&limits); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid limits", "details": err.Error()})
		return
	}

	// Validate tier
	var tier models.UserTier
	switch tierParam {
	case "basic":
		tier = models.TierBasic
	case "premium":
		tier = models.TierPremium
	case "vip":
		tier = models.TierVIP
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid tier", "valid_tiers": []string{"basic", "premium", "vip"}})
		return
	}

	s.tieredRateLimiter.UpdateTierLimits(tier, limits)
	c.JSON(http.StatusOK, gin.H{"message": "Tier limits updated", "tier": tierParam})
}

// handleUpdateEndpointConfig updates configuration for a specific endpoint
func (s *Server) handleUpdateEndpointConfig(c *gin.Context) {
	if s.tieredRateLimiter == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Rate limiting not available"})
		return
	}

	var request struct {
		Endpoint string              `json:"endpoint"`
		Config   auth.EndpointConfig `json:"config"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request", "details": err.Error()})
		return
	}

	if request.Endpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Endpoint is required"})
		return
	}

	s.tieredRateLimiter.UpdateEndpointConfig(request.Endpoint, request.Config)
	c.JSON(http.StatusOK, gin.H{"message": "Endpoint configuration updated", "endpoint": request.Endpoint})
}

// handleGetUserRateLimitStatus returns rate limit status for a specific user
func (s *Server) handleGetUserRateLimitStatus(c *gin.Context) {
	if s.tieredRateLimiter == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Rate limiting not available"})
		return
	}

	userID := c.Param("userID")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "User ID is required"})
		return
	}

	status, err := s.tieredRateLimiter.GetUserRateLimitStatus(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get user status", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"user_id": userID, "status": status})
}

// handleGetIPRateLimitStatus returns rate limit status for a specific IP
func (s *Server) handleGetIPRateLimitStatus(c *gin.Context) {
	if s.tieredRateLimiter == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Rate limiting not available"})
		return
	}

	ip := c.Param("ip")
	if ip == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "IP address is required"})
		return
	}

	status, err := s.tieredRateLimiter.GetIPRateLimitStatus(c.Request.Context(), ip)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get IP status", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ip": ip, "status": status})
}

// handleResetUserRateLimit resets rate limits for a specific user and rate type
func (s *Server) handleResetUserRateLimit(c *gin.Context) {
	if s.tieredRateLimiter == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Rate limiting not available"})
		return
	}

	userID := c.Param("userID")
	rateType := c.Param("rateType")

	if userID == "" || rateType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "User ID and rate type are required"})
		return
	}

	err := s.tieredRateLimiter.ResetUserRateLimit(c.Request.Context(), userID, rateType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reset user rate limit", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "User rate limit reset", "user_id": userID, "rate_type": rateType})
}

// handleResetIPRateLimit resets rate limits for a specific IP and endpoint
func (s *Server) handleResetIPRateLimit(c *gin.Context) {
	if s.tieredRateLimiter == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Rate limiting not available"})
		return
	}

	ip := c.Param("ip")
	endpoint := c.Param("endpoint")

	if ip == "" || endpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "IP address and endpoint are required"})
		return
	}

	err := s.tieredRateLimiter.ResetIPRateLimit(c.Request.Context(), ip, endpoint)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reset IP rate limit", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "IP rate limit reset", "ip": ip, "endpoint": endpoint})
}

// handleCleanupRateLimitData performs cleanup of expired rate limit data
func (s *Server) handleCleanupRateLimitData(c *gin.Context) {
	if s.tieredRateLimiter == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Rate limiting not available"})
		return
	}

	err := s.tieredRateLimiter.CleanupExpiredData(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to cleanup rate limit data", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Rate limit data cleanup completed"})
}
