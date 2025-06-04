package server

import (
	"context"
	"net/http"
	"strings"
	"time"
	"github.com/Aidin1998/pincex_unified/internal/accounts"
	"github.com/Aidin1998/pincex_unified/internal/fiat"
	"github.com/Aidin1998/pincex_unified/internal/marketmaking"
	"github.com/Aidin1998/pincex_unified/internal/risk"
	"github.com/Aidin1998/pincex_unified/internal/trading"
	"github.com/Aidin1998/pincex_unified/internal/userauth"
	"github.com/Aidin1998/pincex_unified/internal/wallet"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Service defines the API server functionality
type Service struct {
	logger          *zap.Logger
	router          *gin.Engine
	httpServer      *http.Server
	userAuthSvc     userauth.Service
	accountsSvc     accounts.Service
	tradingSvc      trading.Service
	fiatSvc         fiat.Service
	walletSvc       wallet.Service
	marketMakingSvc marketmaking.Service
	riskSvc         risk.Service
	handlers        map[string]map[string]http.HandlerFunc
}

// Config holds the server configuration
type Config struct {
	Port         string
	TLSEnabled   bool
	CertFile     string
	KeyFile      string
	CorsEnabled  bool
	AllowOrigins []string
}

// NewService creates a new server service
func NewService(
	logger *zap.Logger,
	config *Config,
	userAuthSvc userauth.Service,
	accountsSvc accounts.Service,
	tradingSvc trading.Service,
	fiatSvc fiat.Service,
	walletSvc wallet.Service,
	marketMakingSvc marketmaking.Service,
	riskSvc risk.Service,
) (*Service, error) {
	router := gin.Default()

	svc := &Service{
		logger:          logger,
		router:          router,
		userAuthSvc:     userAuthSvc,
		accountsSvc:     accountsSvc,
		tradingSvc:      tradingSvc,
		fiatSvc:         fiatSvc,
		walletSvc:       walletSvc,
		marketMakingSvc: marketMakingSvc,
		riskSvc:         riskSvc,
		handlers:        make(map[string]map[string]http.HandlerFunc),
	}

	// Initialize handlers maps for HTTP methods
	svc.handlers["GET"] = make(map[string]http.HandlerFunc)
	svc.handlers["POST"] = make(map[string]http.HandlerFunc)
	svc.handlers["PUT"] = make(map[string]http.HandlerFunc)
	svc.handlers["DELETE"] = make(map[string]http.HandlerFunc)
	svc.handlers["PATCH"] = make(map[string]http.HandlerFunc)

	// Configure HTTP server
	svc.httpServer = &http.Server{
		Addr:    ":" + config.Port,
		Handler: router,
	}

	// Setup routes
	svc.setupRoutes()

	return svc, nil
}

// Start starts the server
func (s *Service) Start() error {
	s.logger.Info("Starting API server")

	// Start the HTTP server
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("Failed to start server", zap.Error(err))
		}
	}()

	return nil
}

// Stop stops the server
func (s *Service) Stop(ctx context.Context) error {
	s.logger.Info("Stopping API server")
	return s.httpServer.Shutdown(ctx)
}

// setupRoutes configures the API routes
func (s *Service) setupRoutes() {
	// Apply global middleware
	s.router.Use(
		s.corsMiddleware(),
		s.loggingMiddleware(),
		s.userAuthSvc.RateLimitMiddleware(),
	)

	// Set up API version group
	v1 := s.router.Group("/api/v1")

	// Register handlers from the handlers map
	for method, routes := range s.handlers {
		for path, handler := range routes {
			s.registerRouteHandler(v1, method, path, handler)
		}
	}

	// Add basic health check route
	v1.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
}

// registerRouteHandler registers a handler for a specific path and method
func (s *Service) registerRouteHandler(group *gin.RouterGroup, method string, path string, handler http.HandlerFunc) {
	// Convert handler to gin.HandlerFunc
	ginHandler := func(c *gin.Context) {
		handler(c.Writer, c.Request)
	}

	// Register handler based on method
	switch method {
	case "GET":
		group.GET(path, ginHandler)
	case "POST":
		group.POST(path, ginHandler)
	case "PUT":
		group.PUT(path, ginHandler)
	case "DELETE":
		group.DELETE(path, ginHandler)
	case "PATCH":
		group.PATCH(path, ginHandler)
	}
}

// RegisterHandler registers a new HTTP handler
func (s *Service) RegisterHandler(path string, method string, handler http.HandlerFunc) error {
	// Initialize handlers map if needed
	if s.handlers == nil {
		s.handlers = make(map[string]map[string]http.HandlerFunc)
	}

	// Initialize method map if needed
	if _, ok := s.handlers[method]; !ok {
		s.handlers[method] = make(map[string]http.HandlerFunc)
	}

	// Register handler
	s.handlers[method][path] = handler

	// Update routes if router is already initialized
	if s.router != nil {
		s.setupRoutes()
	}

	return nil
}

// Middleware functions
func (s *Service) corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

func (s *Service) loggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		s.logger.Info("API Request",
			zap.String("path", path),
			zap.Int("status", status),
			zap.Duration("latency", latency),
			zap.String("client_ip", c.ClientIP()),
			zap.String("method", c.Request.Method),
		)
	}
}

func (s *Service) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
			c.Abort()
			return
		}

		// Extract token from header
		tokenParts := strings.Split(authHeader, " ")
		if len(tokenParts) != 2 || tokenParts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token format"})
			c.Abort()
			return
		}

		token := tokenParts[1]

		// Validate token
		userID, err := s.userAuthSvc.ValidateToken(c.Request.Context(), token)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			c.Abort()
			return
		}

		// Set user ID in context
		c.Set("userID", userID)
		c.Next()
	}
}

func (s *Service) adminMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, exists := c.Get("userID")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			c.Abort()
			return
		}

		isAdmin, err := s.userAuthSvc.IsAdmin(c.Request.Context(), userID.(string))
		if err != nil || !isAdmin {
			c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
			c.Abort()
			return
		}

		c.Next()
	}
}

// Handler placeholders - to be implemented
func (s *Service) handleHealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (s *Service) handleGetMarkets(c *gin.Context) {
	// Implementation coming soon
	c.JSON(http.StatusOK, gin.H{"message": "Get markets handler"})
}

func (s *Service) handleGetOrderbook(c *gin.Context) {
	// Implementation coming soon
	c.JSON(http.StatusOK, gin.H{"message": "Get orderbook handler"})
}

// Additional handler placeholders would be implemented here...

// This is just a skeleton to be expanded with actual handler implementations
