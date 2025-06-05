package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	//	"github.com/Aidin1998/pincex_unified/common/apiutil"
	"github.com/Aidin1998/pincex_unified/internal/accounts/bookkeeper"
	ws "github.com/Aidin1998/pincex_unified/internal/infrastructure/ws"
	metricsapi "github.com/Aidin1998/pincex_unified/internal/marketmaking/analytics/metrics"
	"github.com/Aidin1998/pincex_unified/internal/marketmaking/marketfeeds"
	"github.com/Aidin1998/pincex_unified/internal/trading"
	"github.com/Aidin1998/pincex_unified/internal/trading/handlers"
	"github.com/Aidin1998/pincex_unified/internal/trading/lifecycle"
	"github.com/Aidin1998/pincex_unified/internal/trading/middleware"
	"github.com/Aidin1998/pincex_unified/internal/userauth"
	"github.com/Aidin1998/pincex_unified/internal/userauth/auth"
	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/Aidin1998/pincex_unified/pkg/validation"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// --- Enhanced error response helpers ---
func jsonError(code, message string, details interface{}) gin.H {
	resp := gin.H{"error": code, "message": message}
	if details != nil {
		resp["details"] = details
	}
	return resp
}

// Server represents the HTTP server
type Server struct {
	logger         *zap.Logger
	userAuthSvc    *userauth.Service
	bookkeeperSvc  bookkeeper.BookkeeperService
	marketfeedsSvc marketfeeds.MarketFeedService
	tradingSvc     trading.TradingService
	wsHub          *ws.Hub
	db             *gorm.DB

	// New components
	tradingHandler    *handlers.TradingHandler
	marketDataHandler *handlers.MarketDataHandler
	rateLimiter       *middleware.RateLimiter
	lifecycleManager  *lifecycle.OrderLifecycleManager
}

// NewServer creates a new HTTP server
func NewServer(
	logger *zap.Logger,
	userAuthSvc *userauth.Service,
	bookkeeperSvc bookkeeper.BookkeeperService,
	marketfeedsSvc marketfeeds.MarketFeedService,
	tradingSvc trading.TradingService,
	wsHub *ws.Hub,
	db *gorm.DB,
) *Server {
	// Initialize event bus for lifecycle manager
	eventBus := lifecycle.NewSimpleEventBus(logger)

	// Initialize order lifecycle manager
	lifecycleManager := lifecycle.NewOrderLifecycleManager(db, logger, eventBus)

	// Initialize rate limiter with default config
	rateLimiterConfig := middleware.DefaultRateLimitConfig()
	rateLimiter := middleware.NewRateLimiter(rateLimiterConfig, logger)

	// Initialize trading handlers
	tradingHandler := handlers.NewTradingHandler(tradingSvc, logger)
	marketDataHandler := handlers.NewMarketDataHandler(tradingSvc, logger)

	return &Server{
		logger:            logger,
		userAuthSvc:       userAuthSvc,
		bookkeeperSvc:     bookkeeperSvc,
		marketfeedsSvc:    marketfeedsSvc,
		tradingSvc:        tradingSvc,
		wsHub:             wsHub,
		db:                db,
		tradingHandler:    tradingHandler,
		marketDataHandler: marketDataHandler,
		rateLimiter:       rateLimiter,
		lifecycleManager:  lifecycleManager,
	}
}

// Router creates a new HTTP router
func (s *Server) Router() *gin.Engine {
	// Create router
	router := gin.New()

	// Add basic middleware
	router.Use(gin.Logger())
	router.Use(gin.Recovery())

	// Add RFC 7807 compliant error handling middleware
	// router.Use(apiutil.RFC7807ErrorMiddleware())

	// Add enhanced input validation middleware with comprehensive security
	enhancedValidationConfig := validation.DefaultEnhancedValidationConfig()
	enhancedValidationConfig.EnablePerformanceLogging = true
	enhancedValidationConfig.EnableDetailedErrorLogs = true
	enhancedValidationConfig.StrictModeEnabled = true
	router.Use(validation.EnhancedValidationMiddleware(s.logger, enhancedValidationConfig))

	// Add advanced security hardening middleware
	securityConfig := validation.DefaultSecurityConfig()
	securityConfig.EnableIPBlacklisting = true
	securityConfig.EnableAdvancedSQLInjection = true
	securityConfig.EnableXSSProtection = true
	securityConfig.EnableCSRFProtection = true
	router.Use(validation.SecurityHardeningMiddleware(s.logger, securityConfig))

	// Add enhanced security middleware for comprehensive JWT validation monitoring
	if s.userAuthSvc.AuthService() != nil {
		securityMiddlewareConfig := auth.DefaultModernSecurityConfig()
		securityMiddlewareConfig.EnableDetailedAuditLogging = true
		securityMiddlewareConfig.LogSuspiciousActivity = true
		securityMiddlewareConfig.BlockHighRiskTokens = true
		router.Use(auth.SecurityMiddleware(s.logger, s.userAuthSvc.AuthService(), securityMiddlewareConfig))

		// Add comprehensive audit logging middleware
		router.Use(auth.AuditMiddleware(s.logger))
	}

	// Keep existing basic validation middleware for legacy compatibility
	router.Use(validation.ValidationMiddleware(s.logger))
	router.Use(validation.RequestValidationMiddleware())

	// Add our trading rate limiting middleware
	router.Use(s.rateLimiter.RequestRateLimitMiddleware())

	// Add health check
	router.GET("/health", s.handleHealth)

	// Add WebSocket route for market data
	router.GET("/ws/marketdata", s.handleWebSocketMarketData)

	metricsAPI := metricsapi.NewMetricsAPI(
		metricsapi.BusinessMetricsInstance,
		metricsapi.AlertingServiceInstance,
		metricsapi.ComplianceServiceInstance,
	)

	router.GET("/metrics/fillrate", gin.WrapH(metricsAPI))
	router.GET("/metrics/slippage", gin.WrapH(metricsAPI))
	router.GET("/metrics/marketimpact", gin.WrapH(metricsAPI))
	router.GET("/metrics/spread", gin.WrapH(metricsAPI))
	router.GET("/metrics/alerts", gin.WrapH(metricsAPI))
	router.GET("/metrics/compliance", gin.WrapH(metricsAPI))

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

				// --- RISK MANAGEMENT ENDPOINTS ---
				riskGroup := admin.Group("/risk")
				{
					// Limit management
					riskGroup.GET("/limits", s.handleGetRiskLimits)
					riskGroup.POST("/limits", s.handleCreateRiskLimit)
					riskGroup.PUT("/limits/:type/:id", s.handleUpdateRiskLimit)
					riskGroup.DELETE("/limits/:type/:id", s.handleDeleteRiskLimit)

					// Exemption management
					riskGroup.GET("/exemptions", s.handleGetRiskExemptions)
					riskGroup.POST("/exemptions", s.handleCreateRiskExemption)
					riskGroup.DELETE("/exemptions/:userID", s.handleDeleteRiskExemption)

					// Risk calculation and monitoring
					riskGroup.GET("/metrics/user/:userID", s.handleGetUserRiskMetrics)
					riskGroup.POST("/calculate/batch", s.handleBatchCalculateRisk)
					riskGroup.GET("/dashboard", s.handleGetRiskDashboard)
					riskGroup.POST("/market-data", s.handleUpdateMarketData)

					// Compliance monitoring
					riskGroup.GET("/compliance/alerts", s.handleGetComplianceAlerts)
					riskGroup.PUT("/compliance/alerts/:alertID", s.handleUpdateComplianceAlert)
					riskGroup.POST("/compliance/rules", s.handleAddComplianceRule)
					riskGroup.GET("/compliance/transactions", s.handleGetComplianceTransactions)

					// Reporting
					riskGroup.POST("/reports/generate", s.handleGenerateRiskReport)
					riskGroup.GET("/reports/:reportID", s.handleGetRiskReport)
					riskGroup.GET("/reports", s.handleListRiskReports)
				}

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

// writeError writes a JSON error response with RFC 7807 compliant format
func (s *Server) writeError(c *gin.Context, err error) {
	instance := c.Request.URL.Path

	// Map specific errors to appropriate RFC 7807 responses
	msg := err.Error()
	switch {
	case strings.Contains(msg, "unauthorized"):
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized", "message": "Authentication required", "instance": instance})
	case strings.Contains(msg, "forbidden"):
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden", "message": "Insufficient permissions", "instance": instance})
	case strings.Contains(msg, "not found"):
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found", "message": "Resource not found", "instance": instance})
	case strings.Contains(msg, "invalid"):
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "Invalid request parameters", "instance": instance})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_server_error", "message": "Internal server error", "instance": instance})
	}
}

// authMiddleware creates a middleware for authentication with enhanced JWT validation
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

		// Extract client IP for enhanced security auditing
		clientIP := c.ClientIP()

		// Set client IP in context for enhanced JWT validator
		ctx := context.WithValue(c.Request.Context(), "client_ip", clientIP)
		c.Request = c.Request.WithContext(ctx)

		var userID string
		var err error

		// Try unified auth service first (enhanced validation)
		if s.userAuthSvc.AuthService() != nil {
			claims, authErr := s.userAuthSvc.AuthService().ValidateToken(ctx, token)
			if authErr == nil {
				userID = claims.UserID.String()
				// Set comprehensive context from enhanced validation claims
				c.Set("userEmail", claims.Email)
				c.Set("userRole", claims.Role)
				c.Set("userPermissions", claims.Permissions)
				c.Set("sessionID", claims.SessionID.String())
				c.Set("tokenType", claims.TokenType)

				// Set security context for monitoring
				c.Set("client_ip", clientIP)
				c.Set("user_id", userID)

				// Log successful authentication for audit trail
				s.logger.Debug("Authentication successful",
					zap.String("user_id", userID),
					zap.String("client_ip", clientIP),
					zap.String("endpoint", c.Request.URL.Path),
					zap.String("method", c.Request.Method))
			} else {
				err = authErr
			}
		} else {
			// Fallback to identities service (legacy validation)
			userIDInterface, err := s.userAuthSvc.IdentityService().ValidateToken(c.Request.Context(), token)
			if err == nil {
				if userIDStr, ok := userIDInterface.(string); ok {
					userID = userIDStr
				} else {
					err = fmt.Errorf("invalid user ID type")
				}
			}
		}

		if err != nil {
			// Enhanced error logging for security monitoring
			s.logger.Warn("Authentication failed",
				zap.String("client_ip", clientIP),
				zap.String("endpoint", c.Request.URL.Path),
				zap.String("method", c.Request.Method),
				zap.Error(err))

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
		isAdmin, err := s.userAuthSvc.IdentityService().IsAdmin(c.Request.Context(), userID.(string))
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

// handleHealth handles health check requests
// @Summary		Health check
// @Description	Returns the health status of the API service
// @Tags			System
// @Accept			json
// @Produce		json
// @Success		200	{object}	map[string]interface{}	"Service is healthy"
// @Failure		500	{object}	map[string]interface{}	"Service is unhealthy"
// @Router			/health [get]
func (s *Server) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "ok",
		"timestamp": time.Now().UTC(),
		"version":   "2.0.0",
		"uptime":    "running",
	})
}

// handleRegister handles user registration
// @Summary		Register a new user
// @Description	Create a new user account with email and password
// @Tags			Authentication
// @Accept			json
// @Produce		json
// @Param			request	body	models.RegisterRequest	true	"Registration details"
// @Success		201		{object}	models.User				"User registered successfully"
// @Failure		400		{object}	map[string]interface{}	"Invalid request data"
// @Failure		409		{object}	map[string]interface{}	"User already exists"
// @Failure		500		{object}	map[string]interface{}	"Internal server error"
// @Router			/identities/register [post]
func (s *Server) handleRegister(c *gin.Context) {
	// Implementation will be added in identities service
	c.JSON(http.StatusOK, gin.H{"message": "registration successful"})
}

// handleLogin handles user login
// @Summary		User login
// @Description	Authenticate user and return JWT token
// @Tags			Authentication
// @Accept			json
// @Produce		json
// @Param			request	body	models.LoginRequest		true	"Login credentials"
// @Success		200		{object}	models.LoginResponse	"Login successful"
// @Failure		400		{object}	map[string]interface{}	"Invalid request data"
// @Failure		401		{object}	map[string]interface{}	"Invalid credentials"
// @Failure		423		{object}	map[string]interface{}	"Account locked"
// @Failure		500		{object}	map[string]interface{}	"Internal server error"
// @Router			/identities/login [post]
func (s *Server) handleLogin(c *gin.Context) {
	// Implementation will be added in identities service
	c.JSON(http.StatusOK, gin.H{"message": "login successful"})
}

// handleLogout handles user logout
// @Summary		User logout
// @Description	Invalidate the current user session
// @Tags			Authentication
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Success		200	{object}	map[string]interface{}	"Logout successful"
// @Failure		401	{object}	map[string]interface{}	"Unauthorized"
// @Failure		500		{object}	map[string]interface{}	"Internal server error"
// @Router			/identities/logout [post]
func (s *Server) handleLogout(c *gin.Context) {
	// Implementation will be added in identities service
	c.JSON(http.StatusOK, gin.H{"message": "logout successful"})
}

// handleRefreshToken handles token refresh
// @Summary		Refresh JWT token
// @Description	Refresh an expired JWT token using refresh token
// @Tags			Authentication
// @Accept			json
// @Produce		json
// @Param			request	body		models.RefreshTokenRequest	true	"Refresh token"
// @Success		200		{object}	models.LoginResponse	"Token refreshed successfully"
// @Failure		400		{object}	map[string]interface{}	"Invalid request data"
// @Failure		401		{object}	map[string]interface{}	"Invalid refresh token"
// @Failure		500		{object}	map[string]interface{}	"Internal server error"
// @Router			/identities/refresh [post]
func (s *Server) handleRefreshToken(c *gin.Context) {
	// Implementation will be added in identities service
	c.JSON(http.StatusOK, gin.H{"message": "token refreshed"})
}

// handle2FAEnable handles 2FA enablement
// @Summary		Enable two-factor authentication
// @Description	Enable 2FA for the authenticated user account
// @Tags			Authentication
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Success		200	{object}	map[string]interface{}	"2FA setup details"
// @Failure		400	{object}	map[string]interface{}	"Invalid request"
// @Failure		401	{object}	map[string]interface{}	"Unauthorized"
// @Failure		409	{object}	map[string]interface{}	"2FA already enabled"
// @Failure		500	{object}	map[string]interface{}	"Internal server error"
// @Router			/identities/2fa/enable [post]
func (s *Server) handle2FAEnable(c *gin.Context) {
	// Implementation will be added in identities service
	c.JSON(http.StatusOK, gin.H{"message": "2FA enabled"})
}

// handle2FAVerify handles 2FA verification
// @Summary		Verify two-factor authentication code
// @Description	Verify 2FA code to complete setup or login
// @Tags			Authentication
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param	request	body	models.TwoFAVerifyRequest	true	"2FA verification code"
// @Success	200	{object}	map[string]interface{}	"2FA verified successfully"
// @Failure	400	{object}	map[string]interface{}	"Invalid request data"
// @Failure	401	{object}	map[string]interface{}	"Unauthorized or invalid code"
// @Failure	500	{object}	map[string]interface{}	"Internal server error"
// @Router			/identities/2fa/verify [post]
func (s *Server) handle2FAVerify(c *gin.Context) {
	// Implementation will be added in identities service
	c.JSON(http.StatusOK, gin.H{"message": "2FA verified"})
}

// handle2FADisable handles 2FA disablement
// @Summary		Disable two-factor authentication
// @Description	Disable 2FA for the authenticated user account
// @Tags			Authentication
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param	request	body	models.TwoFAVerifyRequest	true	"2FA verification code"
// @Success	200	{object}	map[string]interface{}	"2FA disabled successfully"
// @Failure	400	{object}	map[string]interface{}	"Invalid request data"
// @Failure	401	{object}	map[string]interface{}	"Unauthorized or invalid code"
// @Failure	500	{object}	map[string]interface{}	"Internal server error"
// @Router			/identities/2fa/disable [post]
func (s *Server) handle2FADisable(c *gin.Context) {
	// Implementation will be added in identities service
	c.JSON(http.StatusOK, gin.H{"message": "2FA disabled"})
}

// handleGetMe handles getting the current user
// @Summary		Get current user profile
// @Description	Retrieve the authenticated user's profile information
// @Tags			User Profile
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Success		200	{object}	models.User	"User profile"
// @Failure		401	{object}	map[string]interface{}	"Unauthorized"
// @Failure		500	{object}	map[string]interface{}	"Internal server error"
// @Router			/identities/me [get]
func (s *Server) handleGetMe(c *gin.Context) {
	// Implementation will be added in identities service
	c.JSON(http.StatusOK, gin.H{"message": "user retrieved"})
}

// handleUpdateMe handles updating the current user
// @Summary		Update current user profile
// @Description	Update the authenticated user's profile information
// @Tags			User Profile
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param	request	body	models.UpdateUserRequest	true	"Updated user information"
// @Success	200	{object}	models.User	"Updated user profile"
// @Failure	400	{object}	map[string]interface{}	"Invalid request data"
// @Failure	401	{object}	map[string]interface{}	"Unauthorized"
// @Failure	500	{object}	map[string]interface{}	"Internal server error"
// @Router			/identities/me [put]
func (s *Server) handleUpdateMe(c *gin.Context) {
	// Implementation will be added in identities service
	c.JSON(http.StatusOK, gin.H{"message": "user updated"})
}

// handleKYCSubmit handles KYC submission
// @Summary		Submit KYC documents
// @Description	Submit Know Your Customer (KYC) documents for verification
// @Tags			KYC
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			request	body		models.KYCSubmissionRequest	true	"KYC documents and information"
// @Success		200		{object}	map[string]interface{}	"KYC submitted successfully"
// @Failure		400		{object}	map[string]interface{}	"Invalid request data"
// @Failure		401		{object}	map[string]interface{}	"Unauthorized"
// @Failure		409		{object}	map[string]interface{}	"KYC already submitted"
// @Failure		500		{object}	map[string]interface{}	"Internal server error"
// @Router			/identities/kyc/submit [post]
func (s *Server) handleKYCSubmit(c *gin.Context) {
	// Implementation will be added in identities service
	c.JSON(http.StatusOK, gin.H{"message": "KYC submitted"})
}

// handleKYCStatus handles getting KYC status
// @Summary		Get KYC verification status
// @Description	Retrieve the current KYC verification status for the user
// @Tags			KYC
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Success		200	{object}	map[string]interface{}	"KYC status information"
// @Failure		401	{object}	map[string]interface{}	"Unauthorized"
// @Failure		500	{object}	map[string]interface{}	"Internal server error"
// @Router			/identities/kyc/status [get]
func (s *Server) handleKYCStatus(c *gin.Context) {
	// Implementation will be added in identities service
	c.JSON(http.StatusOK, gin.H{"message": "KYC status retrieved"})
}

// handleGetAccounts handles getting accounts
// @Summary		Get user accounts
// @Description	Retrieve all cryptocurrency accounts for the authenticated user
// @Tags			Accounts
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Success		200	{array}	models.Account	"List of user accounts"
// @Failure		401	{object}	map[string]interface{}	"Unauthorized"
// @Failure		500	{object}	map[string]interface{}	"Internal server error"
// @Router			/accounts [get]
func (s *Server) handleGetAccounts(c *gin.Context) {
	// Implementation will be added in bookkeeper service
	c.JSON(http.StatusOK, gin.H{"message": "accounts retrieved"})
}

// handleGetAccount handles getting an account
// @Summary		Get specific account
// @Description	Retrieve account details for a specific cryptocurrency
// @Tags			Accounts
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			currency	path		string				true	"Currency symbol (e.g., BTC, ETH)"
// @Success		200			{object}	models.Account		"Account details"
// @Failure		401			{object}	map[string]interface{}	"Unauthorized"
// @Failure		404			{object}	map[string]interface{}	"Account not found"
// @Failure		500			{object}	map[string]interface{}	"Internal server error"
// @Router			/accounts/{currency} [get]
func (s *Server) handleGetAccount(c *gin.Context) {
	// Implementation will be added in bookkeeper service
	c.JSON(http.StatusOK, gin.H{"message": "account retrieved"})
}

// handleGetAccountTransactions handles getting account transactions
// @Summary		Get account transactions
// @Description	Retrieve transaction history for a specific cryptocurrency account
// @Tags			Accounts
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			currency	path		string					true	"Currency symbol (e.g., BTC, ETH)"
// @Param			page		query		int						false	"Page number for pagination"	default(1)
// @Param			per_page	query		int						false	"Items per page"			default(20)
// @Param			type		query		string					false	"Transaction type filter"	Enums(deposit, withdrawal, trade, fee)
// @Success		200			{object}	map[string]interface{}	"Transaction history"
// @Failure		401			{object}	map[string/interface{}	"Unauthorized"
// @Failure		404			{object}	map[string]interface{}	"Account not found"
// @Failure		500			{object}	map[string]interface{}	"Internal server error"
// @Router			/accounts/{currency}/transactions [get]
func (s *Server) handleGetAccountTransactions(c *gin.Context) {
	// Implementation will be added in bookkeeper service
	c.JSON(http.StatusOK, gin.H{"message": "account transactions retrieved"})
}

// handleGetTradingPairs handles getting trading pairs
// @Summary		Get trading pairs
// @Description	Retrieve all available trading pairs with their configurations
// @Tags			Trading
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			status	query		string					false	"Filter by pair status"	Enums(active, inactive, maintenance)
// @Success		200		{array}		models.TradingPair		"List of trading pairs"
// @Failure		401		{object}	map[string]interface{}		"Unauthorized"
// @Failure		500		{object}	map[string]interface{}		"Internal server error"
// @Router			/trading/pairs [get]
func (s *Server) handleGetTradingPairs(c *gin.Context) {
	// Implementation will be added in trading service
	c.JSON(http.StatusOK, gin.H{"message": "trading pairs retrieved"})
}

// handleGetTradingPair handles getting a trading pair
// @Summary		Get specific trading pair
// @Description	Retrieve details for a specific trading pair
// @Tags			Trading
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			symbol	path		string				true	"Trading pair symbol (e.g., BTCUSD, ETHBTC)"
// @Success		200		{object}	models.TradingPair	"Trading pair details"
// @Failure		401		{object}	map[string]interface{}	"Unauthorized"
// @Failure		404		{object}	map[string]interface{}	"Trading pair not found"
// @Failure		500		{object}	map[string]interface{}	"Internal server error"
// @Router			/trading/pairs/{symbol} [get]
func (s *Server) handleGetTradingPair(c *gin.Context) {
	// Implementation will be added in trading service
	c.JSON(http.StatusOK, gin.H{"message": "trading pair retrieved"})
}

// handlePlaceOrder handles placing an order
// @Summary      Place a new order
// @Description  Place a buy or sell order
// @Tags         Orders
// @Accept       json
// @Produce      json
// @Param         request body     models.OrderPlacementRequest true "Order placement payload"
// @Success      201     {object}  models.Order "Order placed successfully"
// @Failure      400     {object}  map[string]interface{} "Invalid request"
// @Failure      401     {object}  map[string]interface{} "Unauthorized"
// @Failure      429     {object}  map[string]interface{} "Rate limit exceeded"
// @Failure      500     {object}  map[string]interface{} "Internal server error"
// @Router       /trading/orders [post]
// @Security     BearerAuth
func (s *Server) handlePlaceOrder(c *gin.Context) {
	// Generate or extract trace ID
	traceID := c.GetHeader("X-Trace-Id")
	if traceID == "" {
		traceID = uuid.New().String()
	}
	c.Set("trace_id", traceID)

	// Record API gateway receipt checkpoint
	ts := time.Now().UTC()
	s.logger.Info("latency_checkpoint", zap.String("trace_id", traceID), zap.String("stage", "api_gateway_receipt"), zap.Time("timestamp", ts))
	// TODO: Write to time-series DB (Prometheus/Influx/Timescale) here

	// Pass trace ID downstream (e.g., via context, headers, or order struct)
	ctx := context.WithValue(c.Request.Context(), "trace_id", traceID)
	_ = ctx // TODO: Use ctx for downstream calls

	// For demonstration, simulate validation and order book insertion
	validationTime := time.Now().UTC()
	s.logger.Info("latency_checkpoint", zap.String("trace_id", traceID), zap.String("stage", "validation_completion"), zap.Time("timestamp", validationTime))
	orderbookTime := time.Now().UTC()
	s.logger.Info("latency_checkpoint", zap.String("trace_id", traceID), zap.String("stage", "orderbook_insertion"), zap.Time("timestamp", orderbookTime))

	// Respond with trace ID for client-side correlation
	c.JSON(http.StatusOK, gin.H{"message": "order placed", "trace_id": traceID})
}

// handleGetOrders handles getting orders
// @Summary		Get user orders
// @Description	Retrieve all orders for the authenticated user with optional filtering
// @Tags			Trading
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			symbol		query		string					false	"Filter by trading pair symbol"
// @Param			status		query		string					false	"Filter by order status"	Enums(open, filled, cancelled, partial)
// @Param			type		query		string					false	"Filter by order type"		Enums(market, limit, stop, stop_limit)
// @Param			page		query		int						false	"Page number for pagination"		default(1)
// @Param			per_page	query		int						false	"Items per page"				default(20)
// @Success		200			{object}	map[string]interface{}	"List of user orders"
// @Failure		400			{object}	map[string]interface{}	"Invalid query parameters"
// @Failure		401			{object}	map[string]interface{}	"Unauthorized"
// @Failure		500			{object}	map[string]interface{}	"Internal server error"
// @Router			/trading/orders [get]
func (s *Server) handleGetOrders(c *gin.Context) {
	// Implementation will be added in trading service
	c.JSON(http.StatusOK, gin.H{"message": "orders retrieved"})
}

// handleGetOrder handles getting a order
// @Summary		Get specific order
// @Description	Retrieve details for a specific order
// @Tags			Trading
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			id	path		string				true	"Order ID"
// @Success		200	{object}	map[string]interface{}	"Order details"
// @Failure		400	{object}	map[string]interface{}	"Invalid order ID"
// @Failure		401	{object}	map[string]interface{}	"Unauthorized"
// @Failure		404	{object}	map[string]interface{}	"Order not found"
// @Failure		500	{object}	map[string]interface{}	"Internal server error"
// @Router			/trading/orders/{id} [get]
func (s *Server) handleGetOrder(c *gin.Context) {
	// Implementation will be added in trading service
	c.JSON(http.StatusOK, gin.H{"message": "order retrieved"})
}

// handleCancelOrder handles canceling an order
// @Summary		Cancel an order
// @Description	Cancel an existing open order
// @Tags			Trading
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			id	path		string				true	"Order ID to cancel"
// @Success		200	{object}	map[string]interface{}	"Order cancelled successfully"
// @Failure		400	{object}	map[string]interface{}	"Invalid order ID"
// @Failure		401	{object}	map[string]interface{}	"Unauthorized"
// @Failure		404	{object}	map[string]interface{}	"Order not found"
// @Failure		409	{object}	map[string]interface{}	"Order cannot be cancelled"
// @Failure		500	{object}	map[string]interface{}	"Internal server error"
// @Router			/trading/orders/{id} [delete]
func (s *Server) handleCancelOrder(c *gin.Context) {
	// Implementation will be added in trading service
	c.JSON(http.StatusOK, gin.H{"message": "order canceled"})
}

// handleGetOrderBook handles getting the order book
// @Summary		Get order book
// @Description	Retrieve the current order book for a specific trading pair
// @Tags			Trading
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			symbol	path		string				true	"Trading pair symbol (e.g., BTCUSD, ETHBTC)"
// @Param			depth	query		int					false	"Order book depth (number of price levels)"	default(20)
// @Success		200		{object}	map[string]interface{}	"Order book data"
// @Failure		400		{object}	map[string]interface{}	"Invalid symbol or parameters"
// @Failure		401		{object}	map[string]interface{}	"Unauthorized"
// @Failure		404		{object}	map[string]interface{}	"Trading pair not found"
// @Failure		500		{object}	map[string]interface{}	"Internal server error"
// @Router			/trading/orderbook/{symbol} [get]
func (s *Server) handleGetOrderBook(c *gin.Context) {
	// Implementation will be added in trading service
	c.JSON(http.StatusOK, gin.H{"message": "order book retrieved"})
}

// handleGetMarketPrices handles getting market prices
// @Summary		Get market prices
// @Description	Retrieve current market prices for all trading pairs
// @Tags			Market Data
// @Accept			json
// @Produce		json
// @Param			symbols	query		[]string				false	"Filter by specific symbols (comma-separated)"
// @Success		200		{array}		models.MarketPrice		"List of market prices"
// @Failure		400		{object}	map[string]interface{}		"Invalid query parameters"
// @Failure		500		{object}	map[string]interface{}		"Internal server error"
// @Router			/market/prices [get]
func (s *Server) handleGetMarketPrices(c *gin.Context) {
	// Implementation will be added in marketfeeds service
	c.JSON(http.StatusOK, gin.H{"message": "market prices retrieved"})
}

// handleGetMarketPrice handles getting a market price
// @Summary		Get specific market price
// @Description	Retrieve current market price for a specific trading pair
// @Tags			Market Data
// @Accept			json
// @Produce		json
// @Param			symbol	path		string				true	"Trading pair symbol (e.g., BTCUSD, ETHBTC)"
// @Success		200		{object}	models.MarketPrice	"Market price data"
// @Failure		400		{object}	map[string]interface{}	"Invalid symbol"
// @Failure		404		{object}	map[string]interface{}	"Trading pair not found"
// @Failure		500		{object}	map[string]interface{}	"Internal server error"
// @Router			/market/prices/{symbol} [get]
func (s *Server) handleGetMarketPrice(c *gin.Context) {
	// Implementation will be added in marketfeeds service
	c.JSON(http.StatusOK, gin.H{"message": "market price retrieved"})
}

// handleGetCandles handles getting candles
// @Summary		Get candlestick data
// @Description	Retrieve OHLCV candlestick data for a specific trading pair
// @Tags			Market Data
// @Accept			json
// @Produce		json
// @Param			symbol		path		string					true	"Trading pair symbol (e.g., BTCUSD, ETHBTC)"
// @Param			interval	query		string					true	"Candle interval"	Enums(1m, 3m, 5m, 15m, 30m, 1h, 2h, 4h, 6h, 8h, 12h, 1d, 3d, 1w, 1M)
// @Param			start_time	query		string					false	"Start time (RFC3339 format)"
// @Param			end_time	query		string					false	"End time (RFC3339 format)"
// @Param			limit		query		int						false	"Number of candles to return"	default(500)
// @Success		200			{array}		models.Candle				"Candlestick data"
// @Failure 400 {object} models.APIErrorResponse "Invalid parameters"
// @Failure 404 {object} models.APIErrorResponse "Trading pair not found"
// @Failure		500			{object}	map[string]interface{}		"Internal server error"
// @Router			/market/candles/{symbol} [get]
func (s *Server) handleGetCandles(c *gin.Context) {
	// Implementation will be added in marketfeeds service
	c.JSON(http.StatusOK, gin.H{"message": "candles retrieved"})
}

// handleFiatDeposit handles fiat deposit
// @Summary		Initiate fiat deposit
// @Description	Create a new fiat currency deposit request
// @Tags			Fiat Transactions
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			request	body		models.FiatDepositRequest	true	"Deposit details"
// @Success		201		{object}	map[string]interface{}	"Deposit initiated successfully"
// @Failure		400		{object}	map[string]interface{}	"Invalid request data"
// @Failure		401		{object}	map[string]interface{}	"Unauthorized"
// @Failure		403		{object}	map[string]interface{}	"Insufficient permissions or KYC required"
// @Failure		500		{object}	map[string]interface{}	"Internal server error"
// @Router			/fiat/deposit [post]
func (s *Server) handleFiatDeposit(c *gin.Context) {
	// Implementation will be added in fiat service
	c.JSON(http.StatusOK, gin.H{"message": "fiat deposit initiated"})
}

// handleFiatWithdraw handles fiat withdrawal
// @Summary		Initiate fiat withdrawal
// @Description	Create a new fiat currency withdrawal request
// @Tags			Fiat Transactions
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			request	body		map[string]interface{}		true	"Withdrawal details"
// @Success		201		{object}	map[string]interface{}	"Withdrawal initiated successfully"
// @Failure		400		{object}	map[string]interface{}	"Invalid request data"
// @Failure		401		{object}	map[string]interface{}	"Unauthorized"
// @Failure		403		{object}	map[string]interface{}	"Insufficient permissions or balance"
// @Failure		500		{object}	map[string]interface{}	"Internal server error"
// @Router			/fiat/withdraw [post]
func (s *Server) handleFiatWithdraw(c *gin.Context) {
	// Implementation will be added in fiat service
	c.JSON(http.StatusOK, gin.H{"message": "fiat withdrawal initiated"})
}

// handleGetFiatDeposits handles getting fiat deposits
// @Summary		Get fiat deposits
// @Description	Retrieve fiat deposit history for the authenticated user
// @Tags			Fiat Transactions
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			currency	query		string						false	"Filter by currency (e.g., USD, EUR)"
// @Param			status		query		string						false	"Filter by status"	Enums(pending, processing, completed, failed, cancelled)
// @Param			page		query		int							false	"Page number for pagination"	default(1)
// @Param			per_page	query		int							false	"Items per page"				default(20)
// @Success		200			{object}	docs.FiatTransactionHistory	"Fiat deposit history"
// @Failure		401			{object}	map[string]interface{}		"Unauthorized"
// @Failure		500			{object}	map[string]interface{}		"Internal server error"
// @Router			/fiat/deposits [get]
func (s *Server) handleGetFiatDeposits(c *gin.Context) {
	// Implementation will be added in fiat service
	c.JSON(http.StatusOK, gin.H{"message": "fiat deposits retrieved"})
}

// handleGetFiatWithdrawals handles getting fiat withdrawals
// @Summary		Get fiat withdrawals
// @Description	Retrieve fiat withdrawal history for the authenticated user
// @Tags			Fiat Transactions
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			currency	query		string						false	"Filter by currency (e.g., USD, EUR)"
// @Param			status		query		string						false	"Filter by status"	Enums(pending, processing, completed, failed, cancelled)
// @Param			page		query		int							false	"Page number for pagination"	default(1)
// @Param			per_page	query		int							false	"Items per page"				default(20)
// @Success		200			{object}	docs.FiatTransactionHistory	"Fiat withdrawal history"
// @Failure		401			{object}	map[string]interface{}		"Unauthorized"
// @Failure		500			{object}	map[string]interface{}		"Internal server error"
// @Router			/fiat/withdrawals [get]
func (s *Server) handleGetFiatWithdrawals(c *gin.Context) {
	// Implementation will be added in fiat service
	c.JSON(http.StatusOK, gin.H{"message": "fiat withdrawals retrieved"})
}

// handleCreateTradingPair handles creating a trading pair
// @Summary		Create trading pair (Admin)
// @Description	Create a new trading pair for the exchange
// @Tags			Admin - Trading
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			request	body		docs.CreateTradingPairRequest	true	"Trading pair configuration"
// @Success		201		{object}	models.TradingPair				"Trading pair created successfully"
// @Failure		400		{object}	map[string]interface{}	"Invalid request data"
// @Failure		401		{object}	map[string]interface{}	"Unauthorized"
// @Failure		403		{object}	map[string]interface{}	"Admin access required"
// @Failure		409		{object}	map[string]interface{}	"Trading pair already exists"
// @Failure		500		{object}	map[string]interface{}	"Internal server error"
// @Router			/admin/trading/pairs [post]
func (s *Server) handleCreateTradingPair(c *gin.Context) {
	// Implementation will be added in trading service
	c.JSON(http.StatusOK, gin.H{"message": "trading pair created"})
}

// handleUpdateTradingPair handles updating a trading pair
// @Summary		Update trading pair (Admin)
// @Description	Update configuration of an existing trading pair
// @Tags			Admin - Trading
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			symbol	path		string							true	"Trading pair symbol"
// @Param			request	body		docs.UpdateTradingPairRequest	true	"Updated trading pair configuration"
// @Success		200		{object}	models.TradingPair				"Trading pair updated successfully"
// @Failure		400		{object}	map[string]interface{}	"Invalid request data"
// @Failure		401		{object}	map[string]interface{}	"Unauthorized"
// @Failure		403		{object}	map[string]interface{}	"Admin access required"
// @Failure		404		{object}	map[string]interface{}	"Trading pair not found"
// @Failure		500		{object}	map[string]interface{}	"Internal server error"
// @Router			/admin/trading/pairs/{symbol} [put]
func (s *Server) handleUpdateTradingPair(c *gin.Context) {
	// Implementation will be added in trading service
	c.JSON(http.StatusOK, gin.H{"message": "trading pair updated"})
}

// handleGetUsers handles getting users
// @Summary		Get all users (Admin)
// @Description	Retrieve a list of all users with pagination and filtering
// @Tags			Admin - Users
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			page		query		int						false	"Page number for pagination"		default(1)
// @Param			per_page	query		int						false	"Items per page"					default(20)
// @Param			email		query		string					false	"Filter by email"
// @Param			status		query		string					false	"Filter by user status"				Enums(active, inactive, suspended)
// @Param			kyc_status	query		string					false	"Filter by KYC status"				Enums(pending, verified, rejected)
// @Success		200			{object}	docs.UserListResponse	"List of users"
// @Failure		400			{object}	map[string]interface{}	"Invalid query parameters"
// @Failure		401			{object}	map[string]interface{}	"Unauthorized"
// @Failure		403			{object}	map[string]interface{}	"Admin access required"
// @Failure		500			{object}	map[string]interface{}	"Internal server error"
// @Router			/admin/users [get]
func (s *Server) handleGetUsers(c *gin.Context) {
	// Implementation will be added in identities service
	c.JSON(http.StatusOK, gin.H{"message": "users retrieved"})
}

// handleGetUser handles getting a user
// @Summary		Get specific user (Admin)
// @Description	Retrieve detailed information for a specific user
// @Tags			Admin - Users
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			id	path		string				true	"User ID"
// @Success		200	{object}	docs.User			"User details"
// @Failure		401	{object}	map[string]interface{}	"Unauthorized"
// @Failure		403	{object}	map[string]interface{}	"Admin access required"
// @Failure		404	{object}	map[string]interface{}	"User not found"
// @Failure		500	{object}	map[string]interface{}	"Internal server error"
// @Router			/admin/users/{id} [get]
func (s *Server) handleGetUser(c *gin.Context) {
	// Implementation will be added in identities service
	c.JSON(http.StatusOK, gin.H{"message": "user retrieved"})
}

// handleUpdateUserKYC handles updating user KYC status
// @Summary		Update user KYC status (Admin)
// @Description	Update the KYC verification status for a specific user
// @Tags			Admin - KYC
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			id		path		string						true	"User ID"
// @Param			request	body		docs.UpdateKYCStatusRequest	true	"KYC status update"
// @Success		200		{object}	docs.KYCStatusResponse		"KYC status updated successfully"
// @Failure		400		{object}	map[string]interface{}	"Invalid request data"
// @Failure		401		{object}	map[string]interface{}	"Unauthorized"
// @Failure		403		{object}	map[string]interface{}	"Admin access required"
// @Failure		404		{object}	map[string]interface{}	"User not found"
// @Failure		500		{object}	map[string]interface{}	"Internal server error"
// @Router			/admin/users/{id}/kyc [put]
func (s *Server) handleUpdateUserKYC(c *gin.Context) {
	// Implementation will be added in identities service
	c.JSON(http.StatusOK, gin.H{"message": "user KYC updated"})
}

// handleWebSocketMarketData handles WebSocket connections for market data
func (s *Server) handleWebSocketMarketData(c *gin.Context) {
	if s.wsHub == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "WebSocket service unavailable"})
		return
	}
	clientID := c.ClientIP() // or some unique identifier
	s.wsHub.ServeWS(c.Writer, c.Request, clientID)
}

// Rate limiting admin handlers

// @Summary		Get Rate Limit Configuration
// @Description	Retrieve the current rate limiting configuration
// @Tags			Rate Limiting
// @Produce		json
// @Security		BearerAuth
// @Success		200	{object}	docs.RateLimitConfigResponse	"Rate limit configuration retrieved successfully"
// @Failure		401	{object}	map[string]interface{}				"Unauthorized"
// @Failure		403	{object}	map[string]interface{}				"Admin access required"
// @Failure		503	{object}	map[string]interface{}				"Rate limiting service unavailable"
// @Router			/admin/rate-limits/config [get]
func (s *Server) handleGetRateLimitConfig(c *gin.Context) {
	config := s.userAuthSvc.GetRateLimitConfig()
	if config == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Rate limiting not available"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"config": config})
}

// @Summary		Update Rate Limit Configuration
// @Description	Update the global rate limiting configuration
// @Tags			Rate Limiting
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			config	body		docs.RateLimitConfigRequest		true	"Rate limit configuration"
// @Success		200		{object}	docs.StandardResponse			"Configuration updated successfully"
// @Failure		400		{object}	map[string]interface{}				"Invalid configuration format"
// @Failure		401		{object}	map[string]interface{}				"Unauthorized"
// @Failure		403		{object}	map[string]interface{}				"Admin access required"
// @Failure		503		{object}	map[string]interface{}				"Rate limiting service unavailable"
// @Router			/admin/rate-limits/config [put]
func (s *Server) handleUpdateRateLimitConfig(c *gin.Context) {
	var config auth.RateLimitConfig
	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid config format", "details": err.Error()})
		return
	}

	s.userAuthSvc.UpdateRateLimitConfig(&config)
	c.JSON(http.StatusOK, gin.H{"message": "Configuration updated successfully"})
}

// @Summary		Set Emergency Mode
// @Description	Enable or disable emergency rate limiting mode
// @Tags			Rate Limiting
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			mode	body		docs.EmergencyModeRequest		true	"Emergency mode status"
// @Success		200		{object}	docs.StandardResponse			"Emergency mode updated successfully"
// @Failure		400		{object}	map[string]interface{}				"Invalid request format"
// @Failure		401		{object}	map[string]interface{}				"Unauthorized"
// @Failure		403		{object}	map[string]interface{}				"Admin access required"
// @Failure		503		{object}	map[string]interface{}				"Rate limiting service unavailable"
// @Router			/admin/rate-limits/emergency-mode [post]
func (s *Server) handleSetEmergencyMode(c *gin.Context) {
	var request struct {
		Enabled bool `json:"enabled"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request", "details": err.Error()})
		return
	}

	s.userAuthSvc.SetEmergencyMode(request.Enabled)
	c.JSON(http.StatusOK, gin.H{"message": "Emergency mode updated", "enabled": request.Enabled})
}

// @Summary		Update Tier Rate Limits
// @Description	Update rate limits for a specific user tier (basic, premium, vip)
// @Tags			Rate Limiting
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			tier	path		string						true	"User tier"	Enums(basic, premium, vip)
// @Param			limits	body		docs.TierLimitsRequest		true	"Tier limits configuration"
// @Success		200		{object}	docs.StandardResponse		"Tier limits updated successfully"
// @Failure		400		{object}	map[string]interface{}			"Invalid tier or limits format"
// @Failure		401		{object}	map[string]interface{}			"Unauthorized"
// @Failure		403		{object}	map[string]interface{}			"Admin access required"
// @Failure		503		{object}	map[string]interface{}			"Rate limiting service unavailable"
// @Router			/admin/rate-limits/tiers/{tier} [put]
func (s *Server) handleUpdateTierLimits(c *gin.Context) {
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

	s.userAuthSvc.UpdateTierLimits(tier, limits)
	c.JSON(http.StatusOK, gin.H{"message": "Tier limits updated", "tier": tierParam})
}

// @Summary		Update Endpoint Configuration
// @Description	Update rate limiting configuration for a specific API endpoint
// @Tags			Rate Limiting
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			config	body		docs.EndpointConfigRequest		true	"Endpoint configuration"
// @Success		200		{object}	docs.StandardResponse			"Endpoint configuration updated successfully"
// @Failure		400		{object}	map[string]interface{}				"Invalid endpoint or configuration format"
// @Failure		401		{object}	map[string]interface{}				"Unauthorized"
// @Failure		403		{object}	map[string]interface{}				"Admin access required"
// @Failure		503		{object}	map[string]interface{}				"Rate limiting service unavailable"
// @Router			/admin/rate-limits/endpoints [put]
func (s *Server) handleUpdateEndpointConfig(c *gin.Context) {
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

	s.userAuthSvc.UpdateEndpointConfig(request.Endpoint, request.Config)
	c.JSON(http.StatusOK, gin.H{"message": "Endpoint configuration updated", "endpoint": request.Endpoint})
}

// @Summary		Get User Rate Limit Status
// @Description	Retrieve current rate limit status for a specific user across all rate types
// @Tags			Rate Limiting
// @Produce		json
// @Security		BearerAuth
// @Param			userID	path		string						true	"User ID"
// @Success		200		{object}	docs.UserRateLimitResponse	"User rate limit status retrieved successfully"
// @Failure		400		{object}	map[string]interface{}			"Invalid user ID"
// @Failure		401		{object}	map[string]interface{}			"Unauthorized"
// @Failure		403		{object}	map[string]interface{}			"Admin access required"
// @Failure		500		{object}	map[string]interface{}			"Failed to retrieve user status"
// @Failure		503		{object}	map[string]interface{}			"Rate limiting service unavailable"
// @Router			/admin/rate-limits/users/{userID}/status [get]
func (s *Server) handleGetUserRateLimitStatus(c *gin.Context) {
	userID := c.Param("userID")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "User ID is required"})
		return
	}

	status, err := s.userAuthSvc.GetUserRateLimitStatus(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get user status", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"user_id": userID, "status": status})
}

// @Summary		Get IP Rate Limit Status
// @Description	Retrieve current rate limit status for a specific IP address across all endpoints
// @Tags			Rate Limiting
// @Produce		json
// @Security		BearerAuth
// @Param			ip	path		string					true	"IP address"
// @Success		200	{object}	docs.IPRateLimitResponse	"IP rate limit status retrieved successfully"
// @Failure		400	{object}	map[string]interface{}		"Invalid IP address"
// @Failure		401	{object}	map[string]interface{}		"Unauthorized"
// @Failure		403	{object}	map[string]interface{}		"Admin access required"
// @Failure		500	{object}	map[string]interface{}		"Failed to retrieve IP status"
// @Failure		503	{object}	map[string]interface{}		"Rate limiting service unavailable"
// @Router			/admin/rate-limits/ips/{ip}/status [get]
func (s *Server) handleGetIPRateLimitStatus(c *gin.Context) {
	ip := c.Param("ip")
	if ip == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "IP address is required"})
		return
	}

	status, err := s.userAuthSvc.GetIPRateLimitStatus(c.Request.Context(), ip)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get IP status", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ip": ip, "status": status})
}

// @Summary		Reset User Rate Limit
// @Description	Reset rate limits for a specific user and rate type
// @Tags			Rate Limiting
// @Produce		json
// @Security		BearerAuth
// @Param			userID		path		string					true	"User ID"
// @Param			rateType	path		string					true	"Rate type"	Enums(api_calls, orders, trades, withdrawals, login_attempts)
// @Success		200			{object}	docs.StandardResponse	"User rate limit reset successfully"
// @Failure		400			{object}	map[string]interface{}		"Invalid user ID or rate type"
// @Failure		401			{object}	map[string]interface{}		"Unauthorized"
// @Failure		403			{object}	map[string]interface{}		"Admin access required"
// @Failure		500			{object}	map[string]interface{}		"Failed to reset user rate limit"
// @Failure		503			{object}	map[string]interface{}		"Rate limiting service unavailable"
// @Router			/admin/rate-limits/users/{userID}/reset/{rateType} [post]
func (s *Server) handleResetUserRateLimit(c *gin.Context) {
	userID := c.Param("userID")
	rateType := c.Param("rateType")

	if userID == "" || rateType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "User ID and rate type are required"})
		return
	}

	err := s.userAuthSvc.ResetUserRateLimit(c.Request.Context(), userID, rateType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reset user rate limit", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "User rate limit reset", "user_id": userID, "rate_type": rateType})
}

// @Summary		Reset IP Rate Limit
// @Description	Reset rate limits for a specific IP address and endpoint
// @Tags			Rate Limiting
// @Produce		json
// @Security		BearerAuth
// @Param			ip			path		string					true	"IP address"
// @Param			endpoint	path		string					true	"Endpoint name"
// @Success		200			{object}	docs.StandardResponse	"IP rate limit reset successfully"
// @Failure		400			{object}	map[string]interface{}		"Invalid IP address or endpoint"
// @Failure		401			{object}	map[string]interface{}		"Unauthorized"
// @Failure		403			{object}	map[string]interface{}		"Admin access required"
// @Failure		500			{object}	map[string]interface{}		"Failed to reset IP rate limit"
// @Failure		503			{object}	map[string]interface{}		"Rate limiting service unavailable"
// @Router			/admin/rate-limits/ips/{ip}/reset/{endpoint} [post]
func (s *Server) handleResetIPRateLimit(c *gin.Context) {
	ip := c.Param("ip")
	endpoint := c.Param("endpoint")

	if ip == "" || endpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "IP address and endpoint are required"})
		return
	}

	err := s.userAuthSvc.ResetIPRateLimit(c.Request.Context(), ip, endpoint)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reset IP rate limit", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "IP rate limit reset", "ip": ip, "endpoint": endpoint})
}

// @Summary		Cleanup Rate Limit Data
// @Description	Perform cleanup of expired rate limit data from Redis
// @Tags			Rate Limiting
// @Produce		json
// @Security		BearerAuth
// @Success		200	{object}	docs.StandardResponse	"Rate limit data cleanup completed successfully"
// @Failure		401	{object}	map[string]interface{}		"Unauthorized"
// @Failure		403	{object}	map[string]interface{}		"Admin access required"
// @Failure		500	{object}	map[string]interface{}		"Failed to cleanup rate limit data"
// @Failure		503	{object}	map[string]interface{}		"Rate limiting service unavailable"
// @Router			/admin/rate-limits/cleanup [post]
func (s *Server) handleCleanupRateLimitData(c *gin.Context) {
	err := s.userAuthSvc.CleanupExpiredData(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to cleanup rate limit data", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Rate limit data cleanup completed"})
}

// --- RISK MANAGEMENT ADMIN HANDLERS ---

// @Summary		Get Risk Limits Configuration
// @Description	Retrieve all configured risk limits including user, market, and global limits
// @Tags			Risk Management
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Success		200	{object}	docs.RiskLimitsResponse	"Risk limits configuration"
// @Failure		401	{object}	map[string]interface{}		"Unauthorized"
// @Failure		403	{object}	map[string]interface{}		"Admin access required"
// @Failure		500	{object}	map[string]interface{}		"Internal server error"
// @Router			/admin/risk/limits [get]
func (s *Server) handleGetRiskLimits(c *gin.Context) {
	limits, err := s.riskSvc.GetLimits(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, limits)
}

// @Summary		Create Risk Limit
// @Description	Create a new risk limit for user, market, or global level
// @Tags			Risk Management
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			request	body		docs.CreateRiskLimitRequest	true	"Risk limit configuration"
// @Success		201		{object}	docs.SuccessResponse		"Risk limit created successfully"
// @Failure		400		{object}	map[string]interface{}	"Invalid request data"
// @Failure		401		{object}	map[string]interface{}	"Unauthorized"
// @Failure		403		{object}	map[string]interface{}	"Admin access required"
// @Failure		500		{object}	map[string]interface{}	"Internal server error"
// @Router			/admin/risk/limits [post]
func (s *Server) handleCreateRiskLimit(c *gin.Context) {
	var req struct {
		Type  string `json:"type" binding:"required"`
		ID    string `json:"id" binding:"required"`
		Limit string `json:"limit" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	limVal, err := decimal.NewFromString(req.Limit)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid limit value"})
		return
	}
	if err := s.riskSvc.CreateRiskLimit(c.Request.Context(), aml.LimitType(req.Type), req.ID, limVal); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusCreated)
}

// @Summary		Update Risk Limit
// @Description	Update an existing risk limit by type and identifier
// @Tags			Risk Management
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			type	path		string						true	"Limit type"		Enums(user, market, global)
// @Param			id		path		string						true	"Limit identifier"
// @Param			request	body		docs.UpdateRiskLimitRequest	true	"Updated limit value"
// @Success		200		{object}	docs.SuccessResponse		"Risk limit updated successfully"
// @Failure		400		{object}	map[string]interface{}	"Invalid request data"
// @Failure		401		{object}	map[string]interface{}	"Unauthorized"
// @Failure		403		{object}	map[string]interface{}	"Admin access required"
// @Failure		404		{object}	map[string]interface{}	"Risk limit not found"
// @Failure		500		{object}	map[string]interface{}	"Internal server error"
// @Router			/admin/risk/limits/{type}/{id} [put]
// handleUpdateRiskLimit updates an existing risk limit
func (s *Server) handleUpdateRiskLimit(c *gin.Context) {
	limitType := c.Param("type")
	id := c.Param("id")
	var req struct {
		Limit string `json:"limit" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	limVal, err := decimal.NewFromString(req.Limit)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid limit value"})
		return
	}
	if err := s.riskSvc.UpdateRiskLimit(c.Request.Context(), aml.LimitType(limitType), id, limVal); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusOK)
}

// @Summary		Delete Risk Limit
// @Description	Delete an existing risk limit by type and identifier
// @Tags			Risk Management
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			type	path		string					true	"Limit type"		Enums(user, market, global)
// @Param			id		path		string					true	"Limit identifier"
// @Success		200		{object}	docs.SuccessResponse	"Risk limit deleted successfully"
// @Failure		401		{object}	map[string]interface{}		"Unauthorized"
// @Failure		403		{object}	map[string]interface{}		"Admin access required"
// @Failure		404		{object}	map[string]interface{}		"Risk limit not found"
// @Failure		500		{object}	map[string]interface{}		"Internal server error"
// @Router			/admin/risk/limits/{type}/{id} [delete]
func (s *Server) handleDeleteRiskLimit(c *gin.Context) {
	limitType := c.Param("type")
	id := c.Param("id")
	if err := s.riskSvc.DeleteRiskLimit(c.Request.Context(), aml.LimitType(limitType), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusOK)
}

// @Summary		Get Risk Exemptions
// @Description	Retrieve list of users with risk limit exemptions
// @Tags			Risk Management
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Success		200	{object}	docs.RiskExemptionsResponse	"List of risk exemptions"
// @Failure		401	{object}	map[string]interface{}			"Unauthorized"
// @Failure		403	{object}	map[string]interface{}			"Admin access required"
// @Failure		500	{object}	map[string]interface{}			"Internal server error"
// @Router			/admin/risk/exemptions [get]
func (s *Server) handleGetRiskExemptions(c *gin.Context) {
	ex, err := s.riskSvc.GetExemptions(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, ex)
}

// @Summary		Create Risk Exemption
// @Description	Create a risk exemption for a specific user
// @Tags			Risk Management
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			request	body									docs.CreateRiskExemptionRequest	true	"Risk exemption configuration"
// @Success		201		{object}	docs.SuccessResponse			"Risk exemption created successfully"
// @Failure		400		{object}	map[string]interface{}	"Invalid request data"
// @Failure		401		{object}	map[string]interface{}	"Unauthorized"
// @Failure		403		{object}	map[string]interface{}	"Admin access required"
// @Failure		500		{object}	map[string]interface{}	"Internal server error"
// @Router			/admin/risk/exemptions [post]
func (s *Server) handleCreateRiskExemption(c *gin.Context) {
	var req struct {
		UserID string `json:"userID" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := s.riskSvc.CreateExemption(c.Request.Context(), req.UserID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusCreated)
}

// @Summary		Delete Risk Exemption
// @Description	Remove risk exemption for a specific user
// @Tags			Risk Management
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			userID	path		string					true	"User ID"
// @Success		200		{object}	docs.SuccessResponse	"Risk exemption deleted successfully"
// @Failure		401		{object}	map[string]interface{}		"Unauthorized"
// @Failure		403		{object}	map[string]interface{}		"Admin access required"
// @Failure		404		{object}	map[string]interface{}		"Risk exemption not found"
// @Failure		500		{object}	map[string]interface{}		"Internal server error"
// @Router			/admin/risk/exemptions/{userID} [delete]
func (s *Server) handleDeleteRiskExemption(c *gin.Context) {
	userID := c.Param("userID")
	if err := s.riskSvc.DeleteExemption(c.Request.Context(), userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusOK)
}

// Risk Management API Handlers

// @Summary		Get User Risk Metrics
// @Description	Get real-time risk metrics for a specific user
// @Tags			Risk Management
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			userID	path		string					true	"User ID"
// @Success		200		{object}	docs.RiskMetrics		"User risk metrics"
// @Failure		401		{object}	map[string]interface{}		"Unauthorized"
// @Failure		403		{object}	map[string]interface{}		"Admin access required"
// @Failure		404		{object}	map[string]interface{}		"User not found"
// @Failure		500		{object}	map[string]interface{}		"Internal server error"
// @Router			/admin/risk/users/{userID}/metrics [get]
func (s *Server) handleGetUserRiskMetrics(c *gin.Context) {
	userID := c.Param("userID")

	metrics, err := s.riskSvc.CalculateRealTimeRisk(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, metrics)
}

// @Summary		Batch Calculate Risk Metrics
// @Description	Calculate risk metrics for multiple users in batch
// @Tags			Risk Management
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			request	body		docs.BatchCalculateRiskRequest	true	"User IDs for batch calculation"
// @Success		200		{object}	docs.BatchRiskMetricsResponse	"Batch risk metrics"
// @Failure		400		{object}	map[string]interface{}				"Invalid request data"
// @Failure		401		{object}	map[string]interface{}				"Unauthorized"
// @Failure		403		{object}	map[string]interface{}				"Admin access required"
// @Failure		500		{object}	map[string]interface{}				"Internal server error"
// @Router			/admin/risk/calculate/batch [post]
func (s *Server) handleBatchCalculateRisk(c *gin.Context) {
	var req struct {
		UserIDs []string `json:"userIds" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, jsonError("invalid_request", "Invalid request: 'userIds' must be a non-empty array of user IDs.", err.Error()))
		return
	}

	metrics, err := s.riskSvc.BatchCalculateRisk(c.Request.Context(), req.UserIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, jsonError("internal_error", "Failed to calculate batch risk metrics.", err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{"metrics": metrics})
}

// @Summary		Get Risk Dashboard
// @Description	Get comprehensive risk dashboard metrics and analytics
// @Tags			Risk Management
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Success		200		{object}	docs.DashboardMetrics	"Risk dashboard metrics"
// @Failure		401		{object}	map[string]interface{}		"Unauthorized"
// @Failure		403		{object}	map[string]interface{}		"Admin access required"
// @Failure		500		{object}	map[string]interface{}		"Internal server error"
// @Router			/admin/risk/dashboard [get]
func (s *Server) handleGetRiskDashboard(c *gin.Context) {
	metrics, err := s.riskSvc.GetDashboardMetrics(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, metrics)
}

// @Summary		Update Market Data
// @Description	Update market data for risk calculations (price and volatility)
// @Tags			Risk Management
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			request	body		docs.UpdateMarketDataRequest	true	"Market data update"
// @Success		200		{object}	docs.SuccessResponse			"Market data updated successfully"
// @Failure		400		{object}	map[string]interface{}	"Invalid request data"
// @Failure		401		{object}	map[string]interface{}	"Unauthorized"
// @Failure		403		{object}	map[string]interface{}	"Admin access required"
// @Failure		500		{object}	map[string]interface{}	"Internal server error"
// @Router			/admin/risk/market-data [put]
func (s *Server) handleUpdateMarketData(c *gin.Context) {
	var req struct {
		Symbol     string  `json:"symbol" binding:"required"`
		Price      float64 `json:"price" binding:"required"`
		Volatility float64 `json:"volatility" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	price := decimal.NewFromFloat(req.Price)
	volatility := decimal.NewFromFloat(req.Volatility)

	err := s.riskSvc.UpdateMarketData(c.Request.Context(), req.Symbol, price, volatility)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "market data updated"})
}

// @Summary		Get Compliance Alerts
// @Description	Get active compliance alerts for monitoring and investigation
// @Tags			Compliance
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Success		200		{array}		docs.ComplianceAlert	"List of active compliance alerts"
// @Failure		401		{object}	map[string]interface{}		"Unauthorized"
// @Failure		403		{object}	map[string]interface{}		"Admin access required"
// @Failure		500		{object}	map[string]interface{}		"Internal server error"
// @Router			/admin/risk/compliance/alerts [get]
func (s *Server) handleGetComplianceAlerts(c *gin.Context) {
	alerts, err := s.riskSvc.GetActiveComplianceAlerts(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, alerts)
}

// @Summary		Update Compliance Alert
// @Description	Update the status of a compliance alert (assign, resolve, add notes)
// @Tags			Compliance
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			alertID	path		string							true	"Alert ID"
// @Param			request	body		docs.UpdateComplianceAlertRequest	true	"Alert update data"
// @Success		200		{object}	docs.SuccessResponse			"Alert updated successfully"
// @Failure		400		{object}	map[string]interface{}	"Invalid request data"
// @Failure		401		{object}	map[string]interface{}	"Unauthorized"
// @Failure		403		{object}	map[string]interface{}	"Admin access required"
// @Failure		404		{object}	map[string]interface{}	"Alert not found"
// @Failure		500		{object}	map[string]interface{}	"Internal server error"
// @Router			/admin/risk/compliance/alerts/{alertID} [put]
func (s *Server) handleUpdateComplianceAlert(c *gin.Context) {
	alertID := c.Param("alertID")

	var req struct {
		Status     string `json:"status" binding:"required"`
		AssignedTo string `json:"assignedTo"`
		Notes      string `json:"notes"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err := s.riskSvc.UpdateComplianceAlertStatus(c.Request.Context(), alertID, req.Status, req.AssignedTo, req.Notes)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "alert updated"})
}

// @Summary		Add Compliance Rule
// @Description	Add a new compliance rule for AML/KYT monitoring
// @Tags			Compliance
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			rule	body		docs.ComplianceRule	true	"Compliance rule to add"
// @Success		201		{object}	docs.SuccessResponse	"Rule added successfully"
// @Failure		400		{object}	map[string]interface{}	"Invalid rule data"
// @Failure		401		{object}	map[string]interface{}	"Unauthorized"
// @Failure		403		{object}	map[string]interface{}	"Admin access required"
// @Failure		500		{object}	map[string]interface{}	"Internal server error"
// @Router			/admin/risk/compliance/rules [post]
func (s *Server) handleAddComplianceRule(c *gin.Context) {
	var rule aml.ComplianceRule
	if err := c.ShouldBindJSON(&rule); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err := s.riskSvc.AddComplianceRule(c.Request.Context(), &rule)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "compliance rule added"})
}

// @Summary		Get Compliance Transactions
// @Description	Retrieve compliance transaction records with filtering options
// @Tags			Compliance
// @Produce		json
// @Security		BearerAuth
// @Param			userID	query		string	false	"Filter by user ID"
// @Param			limit	query		int		false	"Maximum number of transactions to return"	default(100)
// @Param			offset	query		int		false	"Number of transactions to skip"			default(0)
// @Success		200		{object}	docs.ComplianceTransactionsResponse	"Compliance transactions retrieved successfully"
// @Failure		400		{object}	map[string]interface{}	"Invalid query parameters"
// @Failure		401		{object}	map[string]interface{}					"Unauthorized"
// @Failure		403		{object}	map[string]interface{}					"Admin access required"
// @Failure		500		{object}	map[string]interface{}					"Internal server error"
// @Router			/admin/risk/compliance/transactions [get]
func (s *Server) handleGetComplianceTransactions(c *gin.Context) {
	// Parse query parameters for filtering
	userID := c.Query("userID")
	limit := c.DefaultQuery("limit", "100")
	offset := c.DefaultQuery("offset", "0")

	// This would be implemented with actual transaction filtering logic
	// For now, return a placeholder response
	c.JSON(http.StatusOK, gin.H{
		"transactions": []interface{}{},
		"userID":       userID,
		"limit":        limit,
		"offset":       offset,
	})
}

// @Summary		Generate Risk Report
// @Description	Generate a new risk assessment or compliance report
// @Tags			Reporting
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			request	body		docs.GenerateReportRequest	true	"Report generation parameters"
// @Success		200		{object}	docs.GenerateReportResponse	"Report generated successfully"
// @Failure		400		{object}	map[string]interface{}	"Invalid request parameters"
// @Failure		401		{object}	map[string]interface{}			"Unauthorized"
// @Failure		403		{object}	map[string]interface{}			"Admin access required"
// @Failure		500		{object}	map[string]interface{}			"Internal server error"
// @Router			/admin/risk/reports/generate [post]
func (s *Server) handleGenerateRiskReport(c *gin.Context) {
	var req struct {
		ReportType string `json:"reportType" binding:"required"`
		StartTime  int64  `json:"startTime" binding:"required"`
		EndTime    int64  `json:"endTime" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	reportData, err := s.riskSvc.GenerateReport(c.Request.Context(), req.ReportType, req.StartTime, req.EndTime)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"reportId":    fmt.Sprintf("report_%d", time.Now().Unix()),
		"data":        reportData,
		"generatedAt": time.Now().Unix(),
	})
}

// @Summary		Get Risk Report
// @Description	Retrieve a specific risk report by ID
// @Tags			Reporting
// @Produce		json
// @Security		BearerAuth
// @Param			reportID	path		string					true	"Report ID"
// @Success		200			{object}	docs.RiskReportResponse	"Report retrieved successfully"
// @Failure		401			{object}	map[string]interface{}		"Unauthorized"
// @Failure		403			{object}	map[string]interface{}		"Admin access required"
// @Failure		404			{object}	map[string]interface{}		"Report not found"
// @Failure		500			{object}	map[string]interface{}		"Internal server error"
// @Router			/admin/risk/reports/{reportID} [get]
func (s *Server) handleGetRiskReport(c *gin.Context) {
	reportID := c.Param("reportID")

	// This would retrieve the report from storage
	// For now, return a placeholder response
	c.JSON(http.StatusOK, gin.H{
		"reportId": reportID,
		"status":   "completed",
		"data":     "Report data would be here",
	})
}

// @Summary		List Risk Reports
// @Description	List all available risk reports with pagination
// @Tags			Reporting
// @Produce		json
// @Security		BearerAuth
// @Param			type	query		string	false	"Filter by report type"		Enums(daily_risk, compliance, aml, regulatory)
// @Param			status	query		string	false	"Filter by report status"	Enums(pending, completed, failed)
// @Param			limit	query		int		false	"Maximum number of reports to return"	default(50)
// @Param			offset	query		int		false	"Number of reports to skip"				default(0)
// @Success		200		{object}	docs.RiskReportsListResponse	"Reports list retrieved successfully"
// @Failure		400		{object}	map[string]interface{}	"Invalid query parameters"
// @Failure		401		{object}	map[string]interface{}				"Unauthorized"
// @Failure		403		{object}	map[string]interface{}				"Admin access required"
// @Failure		500		{object}	map[string]interface{}				"Internal server error"
// @Router			/admin/risk/reports [get]
func (s *Server) handleListRiskReports(c *gin.Context) {
	// This would list reports from storage with pagination
	// For now, return a placeholder response
	c.JSON(http.StatusOK, gin.H{
		"reports": []gin.H{
			{
				"id":        "report_001",
				"type":      "daily_risk",
				"status":    "completed",
				"createdAt": time.Now().AddDate(0, 0, -1).Unix(),
			},
		},
		"total": 1,
	})
}
