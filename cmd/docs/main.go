package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	// Import docs for swagger
	_ "github.com/Aidin1998/finalex/docs/swagger"
)

// @title Finalex Cryptocurrency Exchange API
// @version 1.0.0
// @description Comprehensive API documentation for the Finalex cryptocurrency exchange platform
// @description
// @description This API provides complete access to all platform functionality including:
// @description - User authentication and account management
// @description - Trading operations (spot, cross-pair, advanced orders)
// @description - Wallet and balance management
// @description - Market data and analytics
// @description - Compliance and risk management
// @description - Administrative functions
// @description
// @description ## Authentication
// @description
// @description The API uses multiple authentication methods:
// @description - **Bearer Token**: JWT tokens for user authentication
// @description - **API Key**: For programmatic access
// @description - **HMAC Signature**: For sensitive operations
// @description
// @description ## Rate Limiting
// @description
// @description All endpoints are rate limited based on user tier:
// @description - **Basic**: 100 requests/minute
// @description - **Premium**: 1000 requests/minute
// @description - **VIP**: 10000 requests/minute
// @description
// @description ## Error Handling
// @description
// @description All errors follow RFC 7807 Problem Details standard
//
// @termsOfService https://finalex.io/terms
// @contact.name Finalex API Support
// @contact.url https://finalex.io/support
// @contact.email api-support@finalex.io
// @license.name MIT
// @license.url https://opensource.org/licenses/MIT
// @host localhost:8080
// @BasePath /
// @schemes http https
//
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description JWT token obtained from login endpoint. Format: "Bearer {token}"
//
// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name X-API-Key
// @description API key for programmatic access
//
// @securityDefinitions.apikey HmacAuth
// @in header
// @name X-Signature
// @description HMAC signature for sensitive operations
func main() {
	// Set Gin mode
	gin.SetMode(gin.ReleaseMode)

	// Create Gin router
	router := gin.New()

	// Add middleware
	router.Use(gin.Logger())
	router.Use(gin.Recovery())

	// CORS middleware
	router.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key, X-Signature")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	// Main documentation page
	router.GET("/", func(c *gin.Context) {
		c.File("./docs/api-documentation.html")
	})

	// Swagger documentation routes
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Serve static documentation files
	router.Static("/static", "./docs")

	router.GET("/docs", func(c *gin.Context) {
		c.File("./docs/api-documentation.html")
	})

	// WebSocket documentation endpoints
	router.GET("/websocket", func(c *gin.Context) {
		c.File("./docs/websocket-api.md")
	})

	router.GET("/ws-tester", func(c *gin.Context) {
		c.File("./docs/ws-tester.html")
	})

	// API specification endpoints
	router.GET("/openapi.yaml", func(c *gin.Context) {
		c.Header("Content-Type", "application/x-yaml")
		c.File("./docs/api/openapi.yaml")
	})

	router.GET("/asyncapi.yaml", func(c *gin.Context) {
		c.Header("Content-Type", "application/x-yaml")
		c.File("./docs/api/asyncapi.yaml")
	})

	// Health check with API status
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "healthy",
			"timestamp": time.Now().Unix(),
			"services": gin.H{
				"rest_api":  "online",
				"websocket": "online",
				"database":  "online",
				"redis":     "online",
			},
			"version": "1.0.0",
		})
	})

	// API status endpoint
	router.GET("/api/status", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message":        "Finalex API Documentation Server",
			"version":        "1.0.0",
			"docs_url":       "/docs",
			"swagger_url":    "/swagger/index.html",
			"websocket_docs": "/websocket",
			"ws_tester":      "/ws-tester",
			"openapi_spec":   "/openapi.yaml",
			"asyncapi_spec":  "/asyncapi.yaml",
		})
	})

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: router,
	}

	// Graceful shutdown
	go func() {
		log.Printf("Starting Finalex API Documentation Server on port %s", port)
		log.Printf("Main Documentation: http://localhost:%s/", port)
		log.Printf("Swagger UI: http://localhost:%s/swagger/index.html", port)
		log.Printf("WebSocket Tester: http://localhost:%s/ws-tester", port)
		log.Printf("WebSocket Docs: http://localhost:%s/websocket", port)

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}

	log.Println("Server exited")
}
