//go:build !admin
// +build !admin

// @title PinCEX Unified API
// @version 1.0.0
// @description Comprehensive Swagger documentation for the PinCEX Unified Exchange API.
// @contact.name API Support
// @contact.url http://example.com/support
// @contact.email support@example.com
// @license.name MIT
// @license.url https://opensource.org/licenses/MIT
// @host localhost:8080
// @BasePath /api/v1
// @schemes http https
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/redis/go-redis/v9"

	// _ "github.com/Aidin1998/finalex/docs" // Swagger docs - TODO: Generate swagger docs

	"github.com/Aidin1998/finalex/internal/accounts/bookkeeper"
	"github.com/Aidin1998/finalex/internal/fiat"
	"github.com/Aidin1998/finalex/internal/infrastructure/config"
	"github.com/Aidin1998/finalex/internal/infrastructure/ws"
	"github.com/Aidin1998/finalex/internal/marketmaking/marketfeeds"
	"github.com/Aidin1998/finalex/internal/trading"
	"github.com/Aidin1998/finalex/internal/trading/dbutil"
	"github.com/Aidin1998/finalex/internal/userauth"
	"github.com/Aidin1998/finalex/pkg/logger"
	"github.com/joho/godotenv"
	"go.uber.org/zap"

	"github.com/Aidin1998/finalex/internal/accounts/transaction"
	"github.com/Aidin1998/finalex/internal/infrastructure/server"
	metricsapi "github.com/Aidin1998/finalex/internal/marketmaking/analytics/metrics"
	"github.com/Aidin1998/finalex/internal/trading/settlement"
)

// --- STUB KYC PROVIDER ---
// Note: This is deprecated as the KYC provider is now defined in userauth/kyc/types.go
// Keeping this temporarily for reference during refactoring
type stubKYCProvider struct{}

// Using stubKYCProvider from userauth/kyc/types.go instead

// --- STUB PUBSUB AND MARKET DATA DISTRIBUTOR ---
type stubPubSub struct{}

func (s *stubPubSub) Publish(ctx context.Context, channel string, message interface{}) error {
	// Stub implementation - in production, this would publish to Redis/Kafka
	return nil
}

func (s *stubPubSub) Subscribe(ctx context.Context, channel string, handler func([]byte)) error {
	// Stub implementation - in production, this would subscribe to Redis/Kafka
	return nil
}

func (s *stubPubSub) Close() error {
	return nil
}

type stubMarketDataDistributor struct{}

func (s *stubMarketDataDistributor) Start() error {
	return nil
}

func (s *stubMarketDataDistributor) Stop() error {
	return nil
}

// ---

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

	// --- Settlement Engine Initialization ---
	settlementEngine := settlement.NewSettlementEngine()
	// --- End Settlement Engine Initialization ---

	// --- Distributed Transaction Manager Initialization ---
	configPath := os.Getenv("TRANSACTION_CONFIG_PATH")
	if configPath == "" {
		configPath = "./configs/transaction-manager.yaml"
	}

	// Initialize distributed transaction manager suite
	transactionSuite, err := transaction.GetTransactionManagerSuite(
		db,
		zapLogger,
		nil, // bookkeeperSvc will be initialized later
		settlementEngine,
		configPath,
	)
	if err != nil {
		zapLogger.Fatal("Failed to initialize transaction manager suite", zap.Error(err))
	}
	// --- End Distributed Transaction Manager Initialization ---

	// Simple Redis client for rate limiting
	simpleRedis := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Address,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	// Note: Rate limiting is now handled inside the userauth service

	// Note: Authentication service is now created inside userauth service

	// Note: Identity service is now created inside userauth service

	bookkeeperSvc, err := bookkeeper.NewService(zapLogger, db)
	if err != nil {
		zapLogger.Fatal("Failed to create bookkeeper service", zap.Error(err))
	}

	bookkeeperXAService := bookkeeper.NewBookkeeperXAAdapter(bookkeeperSvc, nil)
	transactionSuite.BookkeeperXA = transaction.NewBookkeeperXAResource(bookkeeperXAService, zapLogger)

	// Create userauth service (includes auth, identities, and KYC)
	userauthSvc, err := userauth.NewSimpleService(zapLogger, db, simpleRedis)
	if err != nil {
		zapLogger.Fatal("Failed to create userauth service", zap.Error(err))
	}

	fiatSvc := fiat.NewFiatService(db, zapLogger, []byte("default-signature-key")) // TODO: Load from config

	// Update transaction suite with fiat service
	transactionSuite.FiatXA = transaction.NewFiatXAResource(fiatSvc, db, zapLogger, "main")

	// Create WebSocket Hub for high-performance real-time data
	wsHub := ws.NewHub(16, 1000) // 16 shards, 1000 message replay buffer
	zapLogger.Info("WebSocket Hub created with 16 shards and 1000 message replay buffer")

	// Initialize trading service (direct mode)
	tradingSvc, err := trading.NewService(zapLogger, db, bookkeeperSvc, wsHub, settlementEngine)
	if err != nil {
		zapLogger.Fatal("Failed to create trading service", zap.Error(err))
	}

	// Create market data distributor with new WebSocket Hub
	// Note: Temporarily using stub pubsub - will be replaced with Redis Streams
	pubsub := &stubPubSub{}
	marketDataDistributor := &stubMarketDataDistributor{}
	_ = marketDataDistributor // Use the variable to avoid unused error
	zapLogger.Info("Market data distributor initialized")

	marketfeedsSvc, err := marketfeeds.NewService(zapLogger, db, pubsub)
	if err != nil {
		zapLogger.Fatal("Failed to create market feeds service", zap.Error(err))
	}

	// --- SETTLEMENT PIPELINE WIRING ---
	// Create confirmation store and channel
	confirmationStore := settlement.NewConfirmationStore()
	confirmationChan := make(chan settlement.SettlementConfirmation, 100)

	// Kafka config (replace with your actual broker addresses and group ID)
	kafkaBrokers := []string{"localhost:9092"} // TODO: load from config
	kafkaGroupID := "settlement-group"         // TODO: load from config
	kafkaTopic := "settlement-requests"        // TODO: load from config

	settlementProcessor, err := settlement.NewSettlementProcessor(kafkaBrokers, kafkaGroupID, kafkaTopic, confirmationChan, zapLogger)
	if err != nil {
		zapLogger.Fatal("Failed to create settlement processor", zap.Error(err))
	}

	// Start the settlement processor in a goroutine
	go func() {
		ctx := context.Background()
		if err := settlementProcessor.StartConsuming(ctx); err != nil {
			zapLogger.Error("Settlement processor stopped", zap.Error(err))
		}
	}()

	// Start a goroutine to save confirmations to the store
	go func() {
		for conf := range confirmationChan {
			confirmationStore.Save(conf)
		}
	}()
	// --- END SETTLEMENT PIPELINE WIRING ---

	// Create API server with modified parameters
	apiServer := server.NewServer(
		zapLogger,
		userauthSvc,
		bookkeeperSvc,
		marketfeedsSvc,
		tradingSvc,
		fiatSvc,
		wsHub,
		db,
	)

	// Initialize transaction API
	transactionAPI := transaction.NewTransactionAPI(transactionSuite, zapLogger)

	// Metrics/alerting/compliance API instantiation (singletons)
	_ = metricsapi.BusinessMetricsInstance
	_ = metricsapi.AlertingServiceInstance
	_ = metricsapi.ComplianceServiceInstance

	// Start services
	if err := userauthSvc.Start(context.Background()); err != nil {
		zapLogger.Fatal("Failed to start userauth service", zap.Error(err))
	}

	// Start distributed transaction manager suite
	if err := transactionSuite.Start(context.Background()); err != nil {
		zapLogger.Fatal("Failed to start transaction manager suite", zap.Error(err))
	}

	if err := bookkeeperSvc.Start(); err != nil {
		zapLogger.Fatal("Failed to start bookkeeper service", zap.Error(err))
	}
	if err := marketfeedsSvc.Start(); err != nil {
		zapLogger.Fatal("Failed to start market feeds service", zap.Error(err))
	}
	if err := tradingSvc.Start(); err != nil {
		zapLogger.Fatal("Failed to start trading service", zap.Error(err))
	}

	// Start HTTP server
	addr := fmt.Sprintf(":%d", cfg.Server.HTTP.Port)
	// Start server in a goroutine
	go func() {
		zapLogger.Info("Starting API server", zap.String("addr", addr))
		router := apiServer.Router()

		// Register transaction management routes
		transactionAPI.RegisterRoutes(router)

		// Register settlement confirmation API routes
		settlement.RegisterConfirmationAPI(router, confirmationStore)

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
	// Stop distributed transaction manager suite
	if err := transactionSuite.Stop(context.Background()); err != nil {
		zapLogger.Error("Failed to stop transaction manager suite", zap.Error(err))
	}
	if err := marketfeedsSvc.Stop(); err != nil {
		zapLogger.Error("Failed to stop market feeds service", zap.Error(err))
	}
	if err := bookkeeperSvc.Stop(); err != nil {
		zapLogger.Error("Failed to stop bookkeeper service", zap.Error(err))
	}
	if err := userauthSvc.Stop(context.Background()); err != nil {
		zapLogger.Error("Failed to stop userauth service", zap.Error(err))
	}

	zapLogger.Info("Server exited properly")
}
