//go:build swagger
// +build swagger

// @title Finalex Cryptocurrency Exchange API
// @version 1.0.0
// @description Comprehensive API for the Finalex cryptocurrency exchange platform
// @termsOfService https://finalex.io/terms
// @contact.name Finalex API Support
// @contact.url https://finalex.io/support
// @contact.email api-support@finalex.io
// @license.name MIT
// @license.url https://opensource.org/licenses/MIT
// @host localhost:8080
// @BasePath /
// @schemes http https
package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"go.uber.org/zap"

	// Import docs for swagger
	_ "github.com/Aidin1998/finalex/docs"
)

// @title Finalex Cryptocurrency Exchange API
// @version 1.0.0
// @description Comprehensive API for the Finalex cryptocurrency exchange platform
// @termsOfService https://finalex.io/terms
// @contact.name Finalex API Support
// @contact.url https://finalex.io/support
// @contact.email api-support@finalex.io
// @license.name MIT
// @license.url https://opensource.org/licenses/MIT
// @host localhost:8080
// @BasePath /
// @schemes http https
func main() {
	// Set Gin mode
	gin.SetMode(gin.ReleaseMode)

	// Create Gin router
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(gin.Logger())

	// CORS middleware
	router.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, X-API-Key, X-Signature")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	})

	// Swagger documentation endpoint
	router.GET("/docs/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Redirect root to docs
	router.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/docs/index.html")
	})

	// Setup API routes
	setupAPIRoutes(router, logger)

	// Setup server
	srv := &http.Server{
		Addr:    ":8080",
		Handler: router,
	}

	// Start server in a goroutine
	go func() {
		logger.Info("Starting Finalex API server", zap.String("address", srv.Addr))
		logger.Info("API Documentation available at: http://localhost:8080/docs/")

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Failed to start server", zap.Error(err))
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Fatal("Server forced to shutdown", zap.Error(err))
	}

	logger.Info("Server shutdown complete")
}

func setupAPIRoutes(router *gin.Engine, logger *zap.Logger) {
	// API version group
	api := router.Group("/api")
	v1 := api.Group("/v1")

	// System endpoints
	setupSystemRoutes(v1, logger)

	// Authentication endpoints
	setupAuthRoutes(v1, logger)

	// Trading endpoints
	setupTradingRoutes(v1, logger)

	// Wallet endpoints
	setupWalletRoutes(v1, logger)

	// Compliance endpoints
	setupComplianceRoutes(v1, logger)

	// Admin endpoints
	setupAdminRoutes(v1, logger)
}

func setupSystemRoutes(v1 *gin.RouterGroup, logger *zap.Logger) {
	// Health endpoint
	// @Summary Health check
	// @Description Returns the health status of the API service
	// @Tags System
	// @Accept json
	// @Produce json
	// @Success 200 {object} map[string]interface{} "Service is healthy"
	// @Failure 503 {object} map[string]interface{} "Service is unhealthy"
	// @Router /health [get]
	v1.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "healthy",
			"timestamp": time.Now().UTC(),
			"version":   "1.0.0",
			"uptime":    "unknown", // This would be calculated from start time
			"checks": gin.H{
				"database":      "healthy",
				"redis":         "healthy",
				"external_apis": "healthy",
			},
		})
	})

	// Version endpoint
	// @Summary API version information
	// @Description Returns API version and build information
	// @Tags System
	// @Accept json
	// @Produce json
	// @Success 200 {object} map[string]interface{} "Version information"
	// @Router /version [get]
	v1.GET("/version", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"service":    "Finalex CEX",
			"version":    "1.0.0",
			"build":      "development",
			"commit":     "unknown",
			"time":       time.Now().UTC(),
			"go_version": "go1.21.0",
		})
	})
}

func setupAuthRoutes(v1 *gin.RouterGroup, logger *zap.Logger) {
	auth := v1.Group("/auth")

	// Register endpoint
	// @Summary Register a new user
	// @Description Create a new user account with email and password
	// @Tags Authentication
	// @Accept json
	// @Produce json
	// @Param request body map[string]interface{} true "Registration details"
	// @Success 201 {object} map[string]interface{} "User registered successfully"
	// @Failure 400 {object} map[string]interface{} "Invalid request data"
	// @Failure 409 {object} map[string]interface{} "User already exists"
	// @Router /api/v1/auth/register [post]
	auth.POST("/register", func(c *gin.Context) {
		c.JSON(http.StatusCreated, gin.H{
			"message": "User registration endpoint - Implementation in progress",
			"user_id": "123e4567-e89b-12d3-a456-426614174000",
		})
	})

	// Login endpoint
	// @Summary User login
	// @Description Authenticate user with email/username and password
	// @Tags Authentication
	// @Accept json
	// @Produce json
	// @Param request body map[string]interface{} true "Login credentials"
	// @Success 200 {object} map[string]interface{} "Login successful"
	// @Failure 400 {object} map[string]interface{} "Invalid request data"
	// @Failure 401 {object} map[string]interface{} "Invalid credentials"
	// @Router /api/v1/auth/login [post]
	auth.POST("/login", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"access_token":  "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
			"refresh_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
			"token_type":    "Bearer",
			"expires_in":    3600,
		})
	})
}

func setupTradingRoutes(v1 *gin.RouterGroup, logger *zap.Logger) {
	trading := v1.Group("/trading")

	// Get trading pairs
	// @Summary Get trading pairs
	// @Description Retrieve list of available trading pairs
	// @Tags Trading
	// @Accept json
	// @Produce json
	// @Param status query string false "Filter by status" Enums(active, inactive, suspended)
	// @Param limit query int false "Number of items to return" default(50)
	// @Param offset query int false "Number of items to skip" default(0)
	// @Success 200 {object} map[string]interface{} "Trading pairs retrieved successfully"
	// @Router /api/v1/trading/pairs [get]
	trading.GET("/pairs", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"data": []gin.H{
				{
					"symbol":       "BTC/USDT",
					"base_asset":   "BTC",
					"quote_asset":  "USDT",
					"status":       "ACTIVE",
					"min_quantity": "0.00001",
					"price_step":   "0.01",
				},
				{
					"symbol":       "ETH/USDT",
					"base_asset":   "ETH",
					"quote_asset":  "USDT",
					"status":       "ACTIVE",
					"min_quantity": "0.001",
					"price_step":   "0.01",
				},
			},
			"pagination": gin.H{
				"limit":        50,
				"offset":       0,
				"total":        2,
				"has_next":     false,
				"has_previous": false,
			},
		})
	})

	// Place order
	// @Summary Place new order
	// @Description Place a new trading order
	// @Tags Trading
	// @Accept json
	// @Produce json
	// @Security BearerAuth
	// @Param request body map[string]interface{} true "Order details"
	// @Success 201 {object} map[string]interface{} "Order placed successfully"
	// @Failure 400 {object} map[string]interface{} "Invalid request data"
	// @Failure 401 {object} map[string]interface{} "Unauthorized"
	// @Failure 403 {object} map[string]interface{} "Insufficient balance"
	// @Router /api/v1/trading/orders [post]
	trading.POST("/orders", func(c *gin.Context) {
		c.JSON(http.StatusCreated, gin.H{
			"id":                 "123e4567-e89b-12d3-a456-426614174000",
			"symbol":             "BTC/USDT",
			"side":               "BUY",
			"type":               "LIMIT",
			"status":             "NEW",
			"quantity":           "0.001",
			"price":              "50000.00",
			"filled_quantity":    "0",
			"remaining_quantity": "0.001",
			"created_at":         time.Now().UTC(),
		})
	})
}

func setupWalletRoutes(v1 *gin.RouterGroup, logger *zap.Logger) {
	wallet := v1.Group("/wallet")

	// Get balance
	// @Summary Get wallet balance
	// @Description Get current wallet balance for all currencies
	// @Tags Wallet
	// @Accept json
	// @Produce json
	// @Security BearerAuth
	// @Param currency query string false "Filter by specific currency"
	// @Success 200 {object} map[string]interface{} "Wallet balance retrieved successfully"
	// @Failure 401 {object} map[string]interface{} "Unauthorized"
	// @Router /api/v1/wallet/balance [get]
	wallet.GET("/balance", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"balances": []gin.H{
				{
					"currency":  "BTC",
					"available": "1.50000000",
					"locked":    "0.00100000",
					"total":     "1.50100000",
					"usd_value": "75050.00",
				},
				{
					"currency":  "USDT",
					"available": "10000.00",
					"locked":    "500.00",
					"total":     "10500.00",
					"usd_value": "10500.00",
				},
			},
			"total_value_usd": "85550.00",
			"last_updated":    time.Now().UTC(),
		})
	})

	// Request deposit
	// @Summary Request cryptocurrency deposit
	// @Description Request a cryptocurrency deposit to user's wallet
	// @Tags Wallet
	// @Accept json
	// @Produce json
	// @Security BearerAuth
	// @Param request body map[string]interface{} true "Deposit request"
	// @Success 200 {object} map[string]interface{} "Deposit request created successfully"
	// @Failure 400 {object} map[string]interface{} "Invalid request data"
	// @Failure 401 {object} map[string]interface{} "Unauthorized"
	// @Router /api/v1/wallet/deposit [post]
	wallet.POST("/deposit", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"deposit_id":             "123e4567-e89b-12d3-a456-426614174000",
			"asset":                  "BTC",
			"network":                "BTC",
			"address":                "bc1qxy2kgdygjrsqtzq2n0yrf2493p83kkfjhx0wlh",
			"min_deposit":            "0.001",
			"confirmations_required": 3,
			"estimated_arrival_time": "30-60 minutes",
		})
	})
}

func setupComplianceRoutes(v1 *gin.RouterGroup, logger *zap.Logger) {
	compliance := v1.Group("/compliance")

	// Perform compliance check
	// @Summary Perform compliance check
	// @Description Perform a comprehensive compliance check for a transaction or user
	// @Tags Compliance
	// @Accept json
	// @Produce json
	// @Security BearerAuth
	// @Param request body map[string]interface{} true "Compliance check request"
	// @Success 200 {object} map[string]interface{} "Compliance check completed"
	// @Failure 400 {object} map[string]interface{} "Invalid request data"
	// @Failure 401 {object} map[string]interface{} "Unauthorized"
	// @Router /api/v1/compliance/check [post]
	compliance.POST("/check", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"check_id":           "123e4567-e89b-12d3-a456-426614174000",
			"status":             "APPROVED",
			"overall_risk_score": 25,
			"risk_level":         "LOW",
			"processing_time":    150,
			"created_at":         time.Now().UTC(),
		})
	})

	// Get compliance alerts
	// @Summary Get compliance alerts
	// @Description Retrieve compliance monitoring alerts
	// @Tags Compliance
	// @Accept json
	// @Produce json
	// @Security BearerAuth
	// @Param severity query string false "Filter by severity" Enums(LOW, MEDIUM, HIGH, CRITICAL)
	// @Param status query string false "Filter by status" Enums(OPEN, INVESTIGATING, RESOLVED)
	// @Success 200 {object} map[string]interface{} "Compliance alerts retrieved successfully"
	// @Failure 401 {object} map[string]interface{} "Unauthorized"
	// @Router /api/v1/compliance/alerts [get]
	compliance.GET("/alerts", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"data": []gin.H{
				{
					"id":          "123e4567-e89b-12d3-a456-426614174000",
					"type":        "SUSPICIOUS_ACTIVITY",
					"severity":    "HIGH",
					"status":      "OPEN",
					"title":       "Large withdrawal pattern detected",
					"description": "User has made multiple large withdrawals in short timeframe",
					"created_at":  time.Now().Add(-2 * time.Hour).UTC(),
				},
			},
			"pagination": gin.H{
				"limit":        50,
				"offset":       0,
				"total":        1,
				"has_next":     false,
				"has_previous": false,
			},
		})
	})
}

func setupAdminRoutes(v1 *gin.RouterGroup, logger *zap.Logger) {
	admin := v1.Group("/admin")

	// Get system stats
	// @Summary Get system statistics
	// @Description Get comprehensive system statistics and metrics
	// @Tags Admin
	// @Accept json
	// @Produce json
	// @Security BearerAuth
	// @Success 200 {object} map[string]interface{} "System statistics retrieved successfully"
	// @Failure 401 {object} map[string]interface{} "Unauthorized"
	// @Failure 403 {object} map[string]interface{} "Admin access required"
	// @Router /api/v1/admin/system/stats [get]
	admin.GET("/system/stats", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"users": gin.H{
				"total":        10000,
				"active_24h":   1500,
				"kyc_approved": 8500,
			},
			"trading": gin.H{
				"volume_24h":   "50000000.00",
				"trades_24h":   25000,
				"active_pairs": 50,
			},
			"system": gin.H{
				"uptime":       "72h30m15s",
				"memory_usage": "2.5GB",
				"cpu_usage":    "35%",
				"disk_usage":   "60%",
			},
			"compliance": gin.H{
				"open_alerts":   15,
				"resolved_24h":  45,
				"high_priority": 3,
			},
		})
	})

	// Get users
	// @Summary Get users
	// @Description Retrieve list of users (admin only)
	// @Tags Admin
	// @Accept json
	// @Produce json
	// @Security BearerAuth
	// @Param status query string false "Filter by status" Enums(active, suspended, pending)
	// @Param limit query int false "Number of items to return" default(50)
	// @Param offset query int false "Number of items to skip" default(0)
	// @Success 200 {object} map[string]interface{} "Users retrieved successfully"
	// @Failure 401 {object} map[string]interface{} "Unauthorized"
	// @Failure 403 {object} map[string]interface{} "Admin access required"
	// @Router /api/v1/admin/users [get]
	admin.GET("/users", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"data": []gin.H{
				{
					"id":         "123e4567-e89b-12d3-a456-426614174000",
					"email":      "user@example.com",
					"username":   "johndoe",
					"kyc_status": "approved",
					"tier":       "premium",
					"created_at": time.Now().Add(-30 * 24 * time.Hour).UTC(),
					"last_login": time.Now().Add(-2 * time.Hour).UTC(),
				},
			},
			"pagination": gin.H{
				"limit":        50,
				"offset":       0,
				"total":        1,
				"has_next":     false,
				"has_previous": false,
			},
		})
	})
}
