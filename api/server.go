package api

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/marketdata"
	"github.com/Aidin1998/pincex_unified/internal/trading"
	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/gin-contrib/cors"
	ginzap "github.com/gin-contrib/zap"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/pquerna/otp/totp"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	limiter "github.com/ulule/limiter/v3"
	ginlimiter "github.com/ulule/limiter/v3/drivers/middleware/gin"
	memory "github.com/ulule/limiter/v3/drivers/store/memory"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.uber.org/zap"
)

var validate = validator.New()

// Server represents the API server
type Server struct {
	router        *gin.Engine
	logger        *zap.Logger
	identities    interface{}
	bookkeeper    interface{}
	fiats         interface{}
	marketfeeds   interface{}
	trading       interface{}
	validator     *validator.Validate
	rateLimiter   gin.HandlerFunc
	kycProvider   interface{} // Add KYC provider field
	walletService interface{} // Add wallet service field
}

// NewServer creates a new API server with injected service interfaces
func NewServer(
	logger *zap.Logger,
	identities interface{},
	bookkeeper interface{},
	fiats interface{},
	marketfeeds interface{},
	trading interface{},
	kycProvider interface{},
	walletService interface{}, // Add walletService param
) *Server {
	// Create server
	server := &Server{
		logger:        logger,
		identities:    identities,
		bookkeeper:    bookkeeper,
		fiats:         fiats,
		marketfeeds:   marketfeeds,
		trading:       trading,
		kycProvider:   kycProvider,
		walletService: walletService, // Set walletService
	}

	// Create router
	router := gin.New()

	// Add middleware
	router.Use(ginzap.Ginzap(logger, time.RFC3339, true))
	router.Use(ginzap.RecoveryWithZap(logger, true))
	router.Use(otelgin.Middleware("pincex-api"))

	// Configure CORS
	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// Input validator
	validate := validator.New()
	// Rate limiter (100 req/min per IP as example)
	store := memory.NewStore()
	rate, _ := limiter.NewRateFromFormatted("100-M")
	limiterMiddleware := ginlimiter.NewMiddleware(limiter.New(store, rate))
	server.validator = validate
	server.rateLimiter = limiterMiddleware
	server.router = router
	server.registerRoutes()
	return server
}

// Start starts the API server
func (s *Server) Start(addr string) error {
	s.logger.Info("Starting API server", zap.String("addr", addr))
	return s.router.Run(addr)
}

// Router returns the internal Gin engine for testing purposes
func (s *Server) Router() *gin.Engine {
	return s.router
}

// registerRoutes registers all API routes
func (s *Server) registerRoutes() {
	// Public routes
	public := s.router.Group("/api/v1")
	{
		// Metrics endpoint
		public.GET("/metrics", gin.WrapH(promhttp.Handler()))

		// Health check
		public.GET("/health", s.healthCheck)

		// Market data
		market := public.Group("/market")
		{
			market.GET("/prices", s.getMarketPrices)
			market.GET("/history/:symbol", s.getMarketHistory)
			market.GET("/price/:symbol", s.NotImplemented)
			market.GET("/summary/:symbol", s.NotImplemented)
		}

		// Authentication
		auth := public.Group("/auth")
		{
			auth.POST("/register", s.NotImplemented)
			auth.POST("/login", s.NotImplemented)
			auth.POST("/refresh", s.NotImplemented)
			auth.POST("/logout", s.NotImplemented)
		}

		// Market Data WebSocket endpoint
		public.GET("/ws/marketdata", func(c *gin.Context) {
			hub := s.getMarketDataHub()
			hub.ServeWS(c.Writer, c.Request)
		})

		// API Documentation (Swagger / ReDoc)
		public.GET("/docs/openapi.yaml", func(c *gin.Context) {
			c.File("docs/openapi.yaml")
		})
		public.GET("/docs", func(c *gin.Context) {
			html := `<!DOCTYPE html>
			<html>
			<head>
			  <title>API Docs</title>
			  <meta charset="utf-8" />
			  <meta name="viewport" content="width=device-width, initial-scale=1">
			  <script src="https://cdn.redoc.ly/redoc/latest/bundles/redoc.standalone.js"></script>
			</head>
			<body>
			  <redoc spec-url='/api/v1/docs/openapi.yaml'></redoc>
			</body>
			</html>`
			c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
		})
	}

	// Protected routes (require authentication)
	protected := s.router.Group("/api/v1")
	protected.Use(s.authMiddleware(), s.inputValidationMiddleware(), s.rateLimiter)
	{
		user := protected.Group("/user")
		{
			user.GET("/profile", s.NotImplemented)
			user.PUT("/profile", s.NotImplemented)
			user.PUT("/password", s.NotImplemented)

			// 2FA
			twoFA := user.Group("/2fa")
			{
				twoFA.POST("/enable", s.enable2FA)
				twoFA.POST("/verify", s.verify2FA)
				twoFA.POST("/disable", s.disable2FA)
			}

			// KYC
			kyc := user.Group("/kyc")
			{
				kyc.POST("/start", s.NotImplemented)
				kyc.GET("/status", s.NotImplemented)
				kyc.POST("/document", s.NotImplemented)
			}
		}

		// Wallet
		wallet := protected.Group("/wallet")
		{
			wallet.POST("/withdraw", s.createWithdrawal)
			wallet.POST("/withdraw/:id/approve", s.approveWithdrawal)
			wallet.GET("/balance", s.getWalletBalance)
			wallet.GET("/transactions", s.listWalletTransactions)
		}
	}

	// Admin routes (require admin authentication)
	admin := s.router.Group("/api/v1/admin")
	admin.Use(s.adminAuthMiddleware())
	{
		admin.GET("/users", s.NotImplemented)
		admin.GET("/user/:id", s.NotImplemented)
		admin.PUT("/user/:id/kyc", s.NotImplemented)
		admin.GET("/pairs", s.NotImplemented)
		admin.POST("/pair", s.NotImplemented)
		admin.PUT("/pair/:symbol", s.NotImplemented)
		admin.GET("/withdrawals", s.NotImplemented)
		admin.PUT("/withdrawal/:id/approve", s.NotImplemented)
		admin.PUT("/withdrawal/:id/reject", s.NotImplemented)
		admin.PUT("/withdrawal/:id/complete", s.NotImplemented)

		// Trading
		// Place Order (supports advanced order types)
		admin.POST("/trade/order", s.placeOrder)
		// Cancel Order
		admin.DELETE("/trade/order/:id", s.cancelOrder)
		// Get Order by ID
		admin.GET("/trade/order/:id", s.getOrder)
		// List Orders (with optional filters for type/status)
		admin.GET("/trade/orders", s.listOrders)
		// OCO group cancel
		admin.DELETE("/trade/oco/:group_id", s.cancelOCOGroup)
		// Triggered/conditional orders
		admin.GET("/trade/triggered", s.listTriggeredOrders)

		// KYC
		admin.GET("/kyc/cases", s.listKYCCases)
		admin.POST("/kyc/case/:id/approve", s.approveKYC)
		admin.POST("/kyc/case/:id/reject", s.rejectKYC)
	}
}

// --- SECURITY MIDDLEWARES ---
// Input validation middleware
func (s *Server) inputValidationMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("validator", s.validator)
		c.Next()
	}
}

// --- AUDIT LOGGING ---
func (s *Server) auditLog(event, userID, ip string, details map[string]interface{}) {
	fields := []zap.Field{
		zap.String("event", event),
		zap.String("userID", userID),
		zap.String("ip", ip),
	}
	for k, v := range details {
		fields = append(fields, zap.Any(k, v))
	}
	s.logger.Info("AUDIT", fields...)
}

// --- 2FA HANDLERS ---
func (s *Server) enable2FA(c *gin.Context) {
	userID := c.GetString("userID")
	// Lookup user in DB (pseudo-code, replace with real DB call)
	user, err := s.getUserByID(userID)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}
	if user.MFAEnabled && user.TOTPSecret != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "2FA already enabled"})
		return
	}
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "OrbitCEX",
		AccountName: user.Email,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate TOTP secret"})
		return
	}
	user.TOTPSecret = key.Secret()
	// Save user.TOTPSecret to DB (pseudo-code)
	_ = s.saveUser(user)
	c.JSON(http.StatusOK, gin.H{"secret": key.Secret(), "url": key.URL()})
}

func (s *Server) verify2FA(c *gin.Context) {
	userID := c.GetString("userID")
	var req struct{ Code string `json:"code"` }
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	user, err := s.getUserByID(userID)
	if err != nil || user.TOTPSecret == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found or 2FA not setup"})
		return
	}
	if !totp.Validate(req.Code, user.TOTPSecret) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid TOTP code"})
		return
	}
	user.MFAEnabled = true
	_ = s.saveUser(user)
	c.JSON(http.StatusOK, gin.H{"message": "2FA enabled"})
}

func (s *Server) disable2FA(c *gin.Context) {
	userID := c.GetString("userID")
	user, err := s.getUserByID(userID)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}
	user.MFAEnabled = false
	user.TOTPSecret = ""
	_ = s.saveUser(user)
	c.JSON(http.StatusOK, gin.H{"message": "2FA disabled"})
}

// healthCheck handles the health check endpoint
func (s *Server) healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
		"time":   time.Now(),
	})
}

// authMiddleware returns a middleware for authentication
func (s *Server) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get token from Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
			c.Abort()
			return
		}

		// Extract token
		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			_ = authHeader[7:]
		} else {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authorization format"})
			c.Abort()
			return
		}

		// Validate token
		// In a real implementation, you would validate the JWT token
		// For this example, we'll just assume it's valid

		// Set user ID in context
		c.Set("userID", "00000000-0000-0000-0000-000000000000")

		c.Next()
	}
}

// adminAuthMiddleware returns a middleware for admin authentication
func (s *Server) adminAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get token from Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
			c.Abort()
			return
		}

		// Extract token
		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			_ = authHeader[7:]
		} else {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authorization format"})
			c.Abort()
			return
		}

		// Validate token and check admin role
		// In a real implementation, you would validate the JWT token and check the user's role
		// For this example, we'll just assume it's valid and the user is an admin

		// Set user ID in context
		c.Set("userID", "00000000-0000-0000-0000-000000000000")
		c.Set("isAdmin", true)

		c.Next()
	}
}

// --- STUB HANDLERS ---
func (s *Server) NotImplemented(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "Not implemented"})
}

func (s *Server) getMarketPrices(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"prices": []interface{}{}})
}

func (s *Server) getMarketHistory(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"history": []interface{}{}})
}

func (s *Server) getAccounts(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"accounts": []interface{}{}})
}

// --- STUB writeError ---
func (s *Server) writeError(c *gin.Context, err error) {
	s.logger.Error("handler error", zap.Error(err))
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}

// --- TRADE HANDLERS ---
// placeOrder handles advanced order types (limit, market, stop, stop-limit, trailing-stop, OCO)
func (s *Server) placeOrder(c *gin.Context) {
	var req models.Order
	if err := c.ShouldBindJSON(&req); err != nil {
		s.writeError(c, err)
		return
	}
	userID, _ := c.Get("userID")
	if userIDStr, ok := userID.(string); ok {
		req.UserID, _ = uuid.Parse(userIDStr)
	}
	tradingSvc, ok := s.trading.(trading.TradingService)
	if !ok {
		s.writeError(c, fmt.Errorf("trading service unavailable"))
		return
	}
	order, err := tradingSvc.PlaceOrder(c.Request.Context(), &req)
	if err != nil {
		s.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"order_id": order.ID, "status": order.Status, "order": order})
}

// cancelOrder cancels an order by ID
func (s *Server) cancelOrder(c *gin.Context) {
	orderID := c.Param("id")
	tradingSvc, ok := s.trading.(trading.TradingService)
	if !ok {
		s.writeError(c, fmt.Errorf("trading service unavailable"))
		return
	}
	if err := tradingSvc.CancelOrder(c.Request.Context(), orderID); err != nil {
		s.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"order_id": orderID, "status": "canceled"})
}

// getOrder returns order details by ID
func (s *Server) getOrder(c *gin.Context) {
	orderID := c.Param("id")
	tradingSvc, ok := s.trading.(trading.TradingService)
	if !ok {
		s.writeError(c, fmt.Errorf("trading service unavailable"))
		return
	}
	order, err := tradingSvc.GetOrder(orderID)
	if err != nil {
		s.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"order": order})
}

// listOrders lists orders with optional filters for type/status
func (s *Server) listOrders(c *gin.Context) {
	var filter models.OrderFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		s.writeError(c, err)
		return
	}
	userID, _ := c.Get("userID")
	userIDStr, _ := userID.(string)
	tradingSvc, ok := s.trading.(trading.TradingService)
	if !ok {
		s.writeError(c, fmt.Errorf("trading service unavailable"))
		return
	}
	orders, err := tradingSvc.ListOrders(userIDStr, &filter)
	if err != nil {
		s.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"orders": orders})
}

func (s *Server) cancelOCOGroup(c *gin.Context) {
	s.writeError(c, fmt.Errorf("OCO group cancel not implemented in trading service"))
}

func (s *Server) listTriggeredOrders(c *gin.Context) {
	s.writeError(c, fmt.Errorf("Listing triggered/conditional orders not implemented in trading service"))
}

// --- MARKET DATA HUB SINGLETON ---
var (
	marketDataHub     *marketdata.Hub
	marketDataHubOnce sync.Once
)

func (s *Server) getMarketDataHub() *marketdata.Hub {
	marketDataHubOnce.Do(func() {
		marketDataHub = marketdata.NewHub()
		go marketDataHub.Run()
	})
	return marketDataHub
}
