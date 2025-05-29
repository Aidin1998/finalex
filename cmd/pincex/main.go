package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-redis/redis/v8"

	"github.com/Aidin1998/pincex_unified/internal/auth"
	"github.com/Aidin1998/pincex_unified/internal/bookkeeper"
	"github.com/Aidin1998/pincex_unified/internal/config"
	"github.com/Aidin1998/pincex_unified/internal/fiat"
	"github.com/Aidin1998/pincex_unified/internal/identities"
	"github.com/Aidin1998/pincex_unified/internal/kyc"
	"github.com/Aidin1998/pincex_unified/internal/marketdata"
	"github.com/Aidin1998/pincex_unified/internal/marketfeeds"
	"github.com/Aidin1998/pincex_unified/internal/messaging"
	"github.com/Aidin1998/pincex_unified/internal/trading"
	"github.com/Aidin1998/pincex_unified/internal/trading/dbutil"
	"github.com/Aidin1998/pincex_unified/pkg/logger"
	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/joho/godotenv"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/Aidin1998/pincex_unified/internal/server"
)

// --- STUB KYC PROVIDER ---
type stubKYCProvider struct{}

func (s *stubKYCProvider) StartVerification(userID string, data *kyc.KYCData) (string, error) {
	return "stub-session", nil
}
func (s *stubKYCProvider) GetStatus(sessionID string) (kyc.KYCStatus, error) {
	return kyc.KYCStatusPending, nil
}
func (s *stubKYCProvider) WebhookHandler(w http.ResponseWriter, r *http.Request) {}

// ---

// messageTradingWrapper wraps messaging trading service to implement TradingService interface
type messageTradingWrapper struct {
	msgTradingService *messaging.TradingMessageService
	db                *gorm.DB
	logger            *zap.Logger
}

func (w *messageTradingWrapper) Start() error {
	w.logger.Info("Message-driven trading service started")
	return nil
}

func (w *messageTradingWrapper) Stop() error {
	w.logger.Info("Message-driven trading service stopped")
	return nil
}

func (w *messageTradingWrapper) PlaceOrder(ctx context.Context, order *models.Order) (*models.Order, error) {
	// Use async order placement via messaging
	if err := w.msgTradingService.AsyncPlaceOrder(ctx, order); err != nil {
		return nil, err
	}
	// For now, return the order as accepted - in production you'd wait for confirmation
	return order, nil
}

func (w *messageTradingWrapper) CancelOrder(ctx context.Context, orderID string) error {
	// Use async order cancellation via messaging
	return w.msgTradingService.AsyncCancelOrder(ctx, orderID, "", "user_request")
}

// Implement other TradingService methods as stubs or direct DB queries
func (w *messageTradingWrapper) GetOrder(orderID string) (*models.Order, error) {
	return nil, fmt.Errorf("not implemented in messaging mode")
}

func (w *messageTradingWrapper) GetOrders(userID, symbol, status string, limit, offset string) ([]*models.Order, int64, error) {
	return nil, 0, fmt.Errorf("not implemented in messaging mode")
}

func (w *messageTradingWrapper) GetOrderBook(symbol string, depth int) (*models.OrderBookSnapshot, error) {
	return nil, fmt.Errorf("not implemented in messaging mode")
}

func (w *messageTradingWrapper) GetOrderBookBinary(symbol string, depth int) ([]byte, error) {
	return nil, fmt.Errorf("not implemented in messaging mode")
}

func (w *messageTradingWrapper) GetTradingPairs() ([]*models.TradingPair, error) {
	return nil, fmt.Errorf("not implemented in messaging mode")
}

func (w *messageTradingWrapper) GetTradingPair(symbol string) (*models.TradingPair, error) {
	return nil, fmt.Errorf("not implemented in messaging mode")
}

func (w *messageTradingWrapper) CreateTradingPair(pair *models.TradingPair) (*models.TradingPair, error) {
	return nil, fmt.Errorf("not implemented in messaging mode")
}

func (w *messageTradingWrapper) UpdateTradingPair(pair *models.TradingPair) (*models.TradingPair, error) {
	return nil, fmt.Errorf("not implemented in messaging mode")
}

func (w *messageTradingWrapper) ListOrders(userID string, filter *models.OrderFilter) ([]*models.Order, error) {
	return nil, fmt.Errorf("not implemented in messaging mode")
}

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: .env file not found, using environment variables")
	}

	// Create logger
	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}
	zapLogger, err := logger.NewLogger(logLevel)
	if err != nil {
		log.Fatalf("Failed to create logger: %v", err)
	}
	defer zapLogger.Sync()

	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		zapLogger.Fatal("Failed to load configuration", zap.Error(err))
	}

	// Initialize sharded databases and Redis cluster
	if err := dbutil.InitializeConnections(
		cfg.DBSharding,
		cfg.RedisCluster,
	); err != nil {
		zapLogger.Fatal("Failed to initialize DB shards or Redis cluster", zap.Error(err))
	}

	// Use shard 0 as default DB for other services
	db := dbutil.GetDBForKey(0)

	// Simple Redis client for rate limiting
	simpleRedis := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Address,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	var rateLimiter auth.RateLimiter = auth.NewRedisRateLimiter(simpleRedis)
	// Tiered rate limiter
	userService := auth.NewAuthUserService(db)
	tieredRateLimiter := auth.NewTieredRateLimiter(simpleRedis, zapLogger, userService)

	// Create unified authentication service
	authSvc, err := auth.NewAuthService(
		zapLogger,
		db,
		cfg.JWT.Secret,
		time.Duration(cfg.JWT.ExpirationHours)*time.Hour,
		cfg.JWT.RefreshSecret,
		time.Duration(cfg.JWT.RefreshExpHours)*time.Hour,
		"pincex-exchange",
		rateLimiter,
	)
	if err != nil {
		zapLogger.Fatal("Failed to create auth service", zap.Error(err))
	}

	// Create user service adapter for tiered rate limiter
	userService = auth.NewAuthUserService(db)

	// Create services using failover DB and auth service
	identitiesSvc, err := identities.NewService(zapLogger, db, authSvc)
	if err != nil {
		zapLogger.Fatal("Failed to create identities service", zap.Error(err))
	}

	bookkeeperSvc, err := bookkeeper.NewService(zapLogger, db)
	if err != nil {
		zapLogger.Fatal("Failed to create bookkeeper service", zap.Error(err))
	}

	// Create a stub KYC service (replace with real provider as needed)
	kycProvider := &stubKYCProvider{}
	kycService := kyc.NewKYCService(kycProvider)

	fiatSvc, err := fiat.NewService(zapLogger, db, bookkeeperSvc, kycService)
	if err != nil {
		zapLogger.Fatal("Failed to create fiat service", zap.Error(err))
	}

	// Create a Redis pubsub backend for marketfeeds (replace with config as needed)
	pubsub := marketdata.NewRedisPubSub("localhost:6379")

	// Create marketdata WebSocket Hub with auth service integration
	marketDataHub := marketdata.NewHub(authSvc)
	go marketDataHub.Run()
	zapLogger.Info("Marketdata WebSocket Hub started")

	// Initialize trading service (direct mode)
	tradingSvc, err := trading.NewService(zapLogger, db, bookkeeperSvc)
	if err != nil {
		zapLogger.Fatal("Failed to create trading service", zap.Error(err))
	}

	// Remove old pool metrics ticker for pgDB/crDB

	// Create market data distributor with Redis pubsub
	marketDataDistributor := marketdata.NewMarketDataDistributor(marketDataHub, pubsub)
	go marketDataDistributor.Start()
	zapLogger.Info("Marketdata distributor started")

	marketfeedsSvc, err := marketfeeds.NewService(zapLogger, db, pubsub)
	if err != nil {
		zapLogger.Fatal("Failed to create market feeds service", zap.Error(err))
	}

	// Create API server
	apiServer := server.NewServer(
		zapLogger,
		authSvc,
		identitiesSvc,
		bookkeeperSvc,
		fiatSvc,
		marketfeedsSvc,
		tradingSvc,
		marketDataHub,
		tieredRateLimiter,
	)

	// Start services
	if err := identitiesSvc.Start(); err != nil {
		zapLogger.Fatal("Failed to start identities service", zap.Error(err))
	}
	if err := bookkeeperSvc.Start(); err != nil {
		zapLogger.Fatal("Failed to start bookkeeper service", zap.Error(err))
	}
	if err := fiatSvc.Start(); err != nil {
		zapLogger.Fatal("Failed to start fiat service", zap.Error(err))
	}
	if err := marketfeedsSvc.Start(); err != nil {
		zapLogger.Fatal("Failed to start market feeds service", zap.Error(err))
	}
	if err := tradingSvc.Start(); err != nil {
		zapLogger.Fatal("Failed to start trading service", zap.Error(err))
	}

	// Start HTTP server
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	// Start server in a goroutine
	go func() {
		zapLogger.Info("Starting API server", zap.String("addr", addr))
		router := apiServer.Router()
		if err := http.ListenAndServe(addr, router); err != nil {
			zapLogger.Fatal("Failed to start API server", zap.Error(err))
		}
	}()

	// Wait for interrupt to shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	zapLogger.Info("Shutting down server...")

	// Graceful shutdown is not supported by Gin, so just exit

	// Stop services
	if err := tradingSvc.Stop(); err != nil {
		zapLogger.Error("Failed to stop trading service", zap.Error(err))
	}
	if err := marketfeedsSvc.Stop(); err != nil {
		zapLogger.Error("Failed to stop market feeds service", zap.Error(err))
	}
	if err := fiatSvc.Stop(); err != nil {
		zapLogger.Error("Failed to stop fiat service", zap.Error(err))
	}
	if err := bookkeeperSvc.Stop(); err != nil {
		zapLogger.Error("Failed to stop bookkeeper service", zap.Error(err))
	}
	if err := identitiesSvc.Stop(); err != nil {
		zapLogger.Error("Failed to stop identities service", zap.Error(err))
	}

	zapLogger.Info("Server exited properly")
}
