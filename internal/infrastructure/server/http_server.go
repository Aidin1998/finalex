package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/Aidin1998/finalex/internal/infrastructure/config"
	"github.com/Aidin1998/finalex/internal/infrastructure/middleware"
	"github.com/Aidin1998/finalex/internal/infrastructure/ratelimit"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// HTTPServer represents an enhanced HTTP server with middleware integration
type HTTPServer struct {
	config         *config.HTTPServerConfig
	logger         *zap.Logger
	router         *gin.Engine
	middleware     *middleware.UnifiedMiddleware
	healthChecker  *HealthChecker
	rateLimiter    *ratelimit.RateLimitManager
	handlerManager *HandlerManager
}

// HTTPServerOptions contains options for creating an HTTPServer
type HTTPServerOptions struct {
	Config         *config.HTTPServerConfig
	Logger         *zap.Logger
	Middleware     *middleware.UnifiedMiddleware
	HealthChecker  *HealthChecker
	RateLimiter    *ratelimit.RateLimitManager
	HandlerManager *HandlerManager
}

// HandlerManager manages HTTP route handlers
type HandlerManager struct {
	logger   *zap.Logger
	handlers map[string]map[string]gin.HandlerFunc
}

// NewHandlerManager creates a new HandlerManager
func NewHandlerManager(logger *zap.Logger) *HandlerManager {
	return &HandlerManager{
		logger:   logger,
		handlers: make(map[string]map[string]gin.HandlerFunc),
	}
}

// RegisterHandler registers a handler for a specific method and path
func (hm *HandlerManager) RegisterHandler(method, path string, handler gin.HandlerFunc) {
	if hm.handlers[method] == nil {
		hm.handlers[method] = make(map[string]gin.HandlerFunc)
	}
	hm.handlers[method][path] = handler
	hm.logger.Debug("Registered HTTP handler",
		zap.String("method", method),
		zap.String("path", path))
}

// GetHandlers returns all registered handlers
func (hm *HandlerManager) GetHandlers() map[string]map[string]gin.HandlerFunc {
	return hm.handlers
}

// NewHTTPServer creates a new enhanced HTTP server
func NewHTTPServer(opts HTTPServerOptions) (*HTTPServer, error) {
	if opts.Config == nil {
		return nil, fmt.Errorf("HTTP server config is required")
	}
	if opts.Logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	// Set Gin mode based on environment
	gin.SetMode(gin.ReleaseMode)

	// Create Gin router
	router := gin.New()

	server := &HTTPServer{
		config:         opts.Config,
		logger:         opts.Logger,
		router:         router,
		middleware:     opts.Middleware,
		healthChecker:  opts.HealthChecker,
		rateLimiter:    opts.RateLimiter,
		handlerManager: opts.HandlerManager,
	}

	if err := server.setupMiddleware(); err != nil {
		return nil, fmt.Errorf("failed to setup middleware: %w", err)
	}

	if err := server.setupRoutes(); err != nil {
		return nil, fmt.Errorf("failed to setup routes: %w", err)
	}

	return server, nil
}

// setupMiddleware configures all middleware for the HTTP server
func (s *HTTPServer) setupMiddleware() error {
	// Recovery middleware
	s.router.Use(gin.CustomRecovery(func(c *gin.Context, recovered interface{}) {
		s.logger.Error("HTTP server panic recovered",
			zap.Any("panic", recovered),
			zap.String("path", c.Request.URL.Path),
			zap.String("method", c.Request.Method))

		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal_server_error",
			"message": "An internal server error occurred",
		})
	}))

	// Request logging middleware
	s.router.Use(gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		s.logger.Info("HTTP request",
			zap.String("method", param.Method),
			zap.String("path", param.Path),
			zap.Int("status", param.StatusCode),
			zap.Duration("latency", param.Latency),
			zap.String("client_ip", param.ClientIP),
			zap.String("user_agent", param.Request.UserAgent()),
		)
		return ""
	}))

	// CORS middleware
	s.router.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	})

	// Security headers middleware
	s.router.Use(func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Next()
	})

	// Rate limiting middleware
	if s.rateLimiter != nil {
		// TODO: Implement rate limiter middleware for UnifiedMiddleware
		// s.router.Use(s.rateLimiter.GinMiddleware())
	}
	// Custom middleware from middleware manager
	// TODO: Implement Gin middleware wrapper for UnifiedMiddleware
	// if s.middleware != nil {
	//     httpMiddleware := s.middleware.GetHTTPMiddleware()
	//     for _, mw := range httpMiddleware {
	//         s.router.Use(mw)
	//     }
	// }

	// Request timeout middleware
	s.router.Use(func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), s.config.ReadTimeout)
		defer cancel()

		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})

	return nil
}

// setupRoutes configures all routes for the HTTP server
func (s *HTTPServer) setupRoutes() error {
	// Health check routes
	if s.healthChecker != nil {
		s.router.GET(s.config.HealthCheckPath, gin.WrapF(s.healthChecker.HealthHandler()))
		s.router.GET(s.config.ReadinessCheckPath, gin.WrapF(s.healthChecker.ReadinessHandler()))
	}

	// Metrics route
	if s.config.MetricsPath != "" {
		s.router.GET(s.config.MetricsPath, s.metricsHandler)
	}

	// API version info
	s.router.GET("/version", s.versionHandler)

	// Register custom handlers from handler manager
	if s.handlerManager != nil {
		handlers := s.handlerManager.GetHandlers()
		for method, pathHandlers := range handlers {
			for path, handler := range pathHandlers {
				switch method {
				case "GET":
					s.router.GET(path, handler)
				case "POST":
					s.router.POST(path, handler)
				case "PUT":
					s.router.PUT(path, handler)
				case "DELETE":
					s.router.DELETE(path, handler)
				case "PATCH":
					s.router.PATCH(path, handler)
				default:
					s.logger.Warn("Unsupported HTTP method",
						zap.String("method", method),
						zap.String("path", path))
				}
			}
		}
	}

	// 404 handler
	s.router.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "not_found",
			"message": "The requested resource was not found",
			"path":    c.Request.URL.Path,
		})
	})

	// 405 handler
	s.router.NoMethod(func(c *gin.Context) {
		c.JSON(http.StatusMethodNotAllowed, gin.H{
			"error":   "method_not_allowed",
			"message": "The requested method is not allowed for this resource",
			"method":  c.Request.Method,
			"path":    c.Request.URL.Path,
		})
	})

	return nil
}

// metricsHandler provides Prometheus metrics endpoint
func (s *HTTPServer) metricsHandler(c *gin.Context) {
	// TODO: Integrate with metrics package
	c.JSON(http.StatusOK, gin.H{
		"message": "Metrics endpoint - TODO: Integrate with Prometheus",
	})
}

// versionHandler provides API version information
func (s *HTTPServer) versionHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"service": "Finalex CEX",
		"version": "1.0.0",
		"build":   "development",
		"time":    time.Now().UTC().Format(time.RFC3339),
	})
}

// GetRouter returns the Gin router instance
func (s *HTTPServer) GetRouter() *gin.Engine {
	return s.router
}

// RegisterRoute registers a new route with the HTTP server
func (s *HTTPServer) RegisterRoute(method, path string, handler gin.HandlerFunc) {
	if s.handlerManager != nil {
		s.handlerManager.RegisterHandler(method, path, handler)
	}

	switch method {
	case "GET":
		s.router.GET(path, handler)
	case "POST":
		s.router.POST(path, handler)
	case "PUT":
		s.router.PUT(path, handler)
	case "DELETE":
		s.router.DELETE(path, handler)
	case "PATCH":
		s.router.PATCH(path, handler)
	default:
		s.logger.Warn("Unsupported HTTP method for route registration",
			zap.String("method", method),
			zap.String("path", path))
	}
}

// RegisterGroup registers a route group with middleware
func (s *HTTPServer) RegisterGroup(basePath string, middleware ...gin.HandlerFunc) *gin.RouterGroup {
	group := s.router.Group(basePath)
	if len(middleware) > 0 {
		group.Use(middleware...)
	}
	return group
}

// EnableCompression enables gzip compression if configured
func (s *HTTPServer) EnableCompression() {
	if s.config.EnableCompression {
		// TODO: Add gzip middleware
		s.logger.Info("HTTP compression enabled")
	}
}

// GetStats returns HTTP server statistics
func (s *HTTPServer) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"config": map[string]interface{}{
			"host":                 s.config.Host,
			"port":                 s.config.Port,
			"read_timeout":         s.config.ReadTimeout.String(),
			"write_timeout":        s.config.WriteTimeout.String(),
			"idle_timeout":         s.config.IdleTimeout.String(),
			"max_header_bytes":     s.config.MaxHeaderBytes,
			"enable_compression":   s.config.EnableCompression,
			"enable_http2":         s.config.EnableHTTP2,
			"max_concurrent_conns": s.config.MaxConcurrentConns,
		},
		"routes": s.getRouteStats(),
	}
}

// getRouteStats returns statistics about registered routes
func (s *HTTPServer) getRouteStats() map[string]interface{} {
	if s.handlerManager == nil {
		return map[string]interface{}{"total": 0}
	}

	handlers := s.handlerManager.GetHandlers()
	total := 0
	methodCounts := make(map[string]int)

	for method, pathHandlers := range handlers {
		count := len(pathHandlers)
		methodCounts[method] = count
		total += count
	}

	return map[string]interface{}{
		"total":   total,
		"methods": methodCounts,
	}
}
