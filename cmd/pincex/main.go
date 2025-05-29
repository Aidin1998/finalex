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

	"github.com/Aidin1998/pincex_unified/internal/bookkeeper"
	"github.com/Aidin1998/pincex_unified/internal/config"
	"github.com/Aidin1998/pincex_unified/internal/database"
	"github.com/Aidin1998/pincex_unified/internal/fiat"
	"github.com/Aidin1998/pincex_unified/internal/identities"
	"github.com/Aidin1998/pincex_unified/internal/kyc"
	"github.com/Aidin1998/pincex_unified/internal/marketdata"
	"github.com/Aidin1998/pincex_unified/internal/marketfeeds"
	"github.com/Aidin1998/pincex_unified/internal/trading"
	"github.com/Aidin1998/pincex_unified/pkg/logger"
	"github.com/Aidin1998/pincex_unified/pkg/metrics"
	"github.com/joho/godotenv"
	"go.uber.org/zap"

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

	// Connect to PostgreSQL
	pgDB, err := database.NewPostgresDB(cfg.Database.DSN, cfg.Database.MaxOpenConns, cfg.Database.MaxIdleConns, cfg.Database.ConnMaxLifetime)
	if err != nil {
		zapLogger.Fatal("Failed to connect to PostgreSQL", zap.Error(err))
	}

	// Connect to CockroachDB
	crDB, err := database.NewCockroachDB(cfg.Database.CockroachDSN, cfg.Database.MaxOpenConns, cfg.Database.MaxIdleConns, cfg.Database.ConnMaxLifetime)
	if err != nil {
		zapLogger.Fatal("Failed to connect to CockroachDB", zap.Error(err))
	}

	// Schedule data lifecycle: archive orders older than 24h every hour
	ticker := time.NewTicker(1 * time.Hour)
	go func() {
		for range ticker.C {
			ctx := context.Background()
			if err := database.ArchiveOrders(ctx, crDB, pgDB, 24*time.Hour); err != nil {
				zapLogger.Error("Order archive failed", zap.Error(err))
			}
		}
	}()

	// Initialize DB failover manager (PG primary, CR standby)
	failMgr := database.NewFailoverManager(pgDB, crDB, zapLogger, 30*time.Second)
	// Start failover monitoring
	ctx := context.Background()
	go failMgr.Start(ctx)
	// Use failover-managed DB for services
	db := failMgr.DB()

	// Create services using failover DB
	identitiesSvc, err := identities.NewService(zapLogger, db)
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

	marketfeedsSvc, err := marketfeeds.NewService(zapLogger, db, pubsub)
	if err != nil {
		zapLogger.Fatal("Failed to create market feeds service", zap.Error(err))
	}

	// Trading service uses CockroachDB directly
	tradingSvc, err := trading.NewService(zapLogger, crDB, bookkeeperSvc)
	if err != nil {
		zapLogger.Fatal("Failed to create trading service", zap.Error(err))
	}

	// Create API server
	apiServer := server.NewServer(
		zapLogger,
		identitiesSvc,
		bookkeeperSvc,
		fiatSvc,
		marketfeedsSvc,
		tradingSvc,
	)

	// Schedule DB pool metrics collection every 30s
	tickerDB := time.NewTicker(30 * time.Second)
	go func() {
		for range tickerDB.C {
			if sqlDB, err := pgDB.DB(); err == nil {
				stats := sqlDB.Stats()
				metrics.DBOpenConns.WithLabelValues("postgres").Set(float64(stats.OpenConnections))
				metrics.DBIdleConns.WithLabelValues("postgres").Set(float64(stats.Idle))
				metrics.DBInUseConns.WithLabelValues("postgres").Set(float64(stats.InUse))
			}
			if sqlDB, err := crDB.DB(); err == nil {
				stats := sqlDB.Stats()
				metrics.DBOpenConns.WithLabelValues("cockroach").Set(float64(stats.OpenConnections))
				metrics.DBIdleConns.WithLabelValues("cockroach").Set(float64(stats.Idle))
				metrics.DBInUseConns.WithLabelValues("cockroach").Set(float64(stats.InUse))
			}
		}
	}()

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
