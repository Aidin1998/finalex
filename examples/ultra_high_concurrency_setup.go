// Example: Ultra-High Concurrency Database Layer Setup
// This file demonstrates how to integrate and use the ultra-high concurrency database layer
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/accounts"
	"github.com/go-redis/redis/v8"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	// Initialize logger
	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Sync()

	// Setup database connection
	db, err := setupDatabase()
	if err != nil {
		logger.Fatal("Failed to setup database", zap.Error(err))
	}

	// Setup Redis connection
	redisClient := setupRedis()

	// Create bookkeeper service (existing service for backward compatibility)
	bookkeeperSvc := &mockBookkeeperService{}

	// Configure ultra-high concurrency service
	config := accounts.DefaultServiceConfig()
	config.EnableCaching = true
	config.EnablePartitioning = true
	config.EnableMetrics = true
	config.ConcurrencyLimit = 100000 // Support 100k concurrent operations

	// Create ultra-high concurrency service
	uhcService, err := accounts.NewUltraHighConcurrencyService(
		db,
		redisClient,
		bookkeeperSvc,
		config,
		logger,
	)
	if err != nil {
		logger.Fatal("Failed to create ultra-high concurrency service", zap.Error(err))
	}

	// Start the service
	ctx := context.Background()
	if err := uhcService.Start(ctx); err != nil {
		logger.Fatal("Failed to start service", zap.Error(err))
	}
	defer uhcService.Stop(ctx)

	// Example usage demonstrations
	demonstrateBasicOperations(ctx, uhcService, logger)
	demonstrateHighConcurrencyOperations(ctx, uhcService, logger)
	demonstrateMonitoring(ctx, uhcService, logger)

	logger.Info("Ultra-high concurrency setup completed successfully")
}

func setupDatabase() (*gorm.DB, error) {
	// Database configuration for ultra-high concurrency
	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
		getEnv("DB_HOST", "localhost"),
		getEnv("DB_USER", "postgres"),
		getEnv("DB_PASSWORD", "password"),
		getEnv("DB_NAME", "pincex"),
		getEnv("DB_PORT", "5432"),
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		// Performance optimizations for high concurrency
		DisableForeignKeyConstraintWhenMigrating: true,
		PrepareStmt:                              true,
	})
	if err != nil {
		return nil, err
	}

	// Configure connection pool for ultra-high concurrency
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	// Ultra-high concurrency connection pool settings
	sqlDB.SetMaxOpenConns(1000) // Support 1000+ connections per pod
	sqlDB.SetMaxIdleConns(100)  // Keep 100 idle connections
	sqlDB.SetConnMaxLifetime(time.Hour)
	sqlDB.SetConnMaxIdleTime(30 * time.Minute)

	return db, nil
}

func setupRedis() redis.UniversalClient {
	// Redis cluster configuration for high availability
	redisOptions := &redis.UniversalOptions{
		Addrs: []string{
			getEnv("REDIS_HOST", "localhost:6379"),
		},
		Password:     getEnv("REDIS_PASSWORD", ""),
		DB:           0,
		PoolSize:     100, // Large pool for high concurrency
		MinIdleConns: 20,  // Keep minimum idle connections
		MaxRetries:   3,   // Retry failed operations
		DialTimeout:  10 * time.Second,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		PoolTimeout:  30 * time.Second,
		IdleTimeout:  5 * time.Minute,
	}

	return redis.NewUniversalClient(redisOptions)
}

func demonstrateBasicOperations(ctx context.Context, service *accounts.UltraHighConcurrencyService, logger *zap.Logger) {
	logger.Info("=== Demonstrating Basic Operations ===")

	userID := "user123"
	currency := "USD"

	// Create account
	account, err := service.CreateAccount(ctx, userID, currency)
	if err != nil {
		logger.Error("Failed to create account", zap.Error(err))
		return
	}
	logger.Info("Created account", zap.String("account_id", account.ID))

	// Update balance
	amount := decimal.NewFromFloat(1000.50)
	err = service.UpdateBalance(ctx, userID, currency, amount, "deposit", "initial_deposit")
	if err != nil {
		logger.Error("Failed to update balance", zap.Error(err))
		return
	}
	logger.Info("Updated balance", zap.String("amount", amount.String()))

	// Get account
	retrievedAccount, err := service.GetAccount(ctx, userID, currency)
	if err != nil {
		logger.Error("Failed to get account", zap.Error(err))
		return
	}
	logger.Info("Retrieved account",
		zap.String("balance", retrievedAccount.Balance.String()),
		zap.String("available", retrievedAccount.Available.String()),
	)
}

func demonstrateHighConcurrencyOperations(ctx context.Context, service *accounts.UltraHighConcurrencyService, logger *zap.Logger) {
	logger.Info("=== Demonstrating High Concurrency Operations ===")

	// Batch operations for multiple users
	userIDs := []string{"user001", "user002", "user003", "user004", "user005"}
	currencies := []string{"USD", "EUR", "BTC", "ETH"}

	// Batch create accounts
	var accountIDs []string
	for _, userID := range userIDs {
		for _, currency := range currencies {
			account, err := service.CreateAccount(ctx, userID, currency)
			if err != nil {
				logger.Error("Failed to create account in batch",
					zap.Error(err),
					zap.String("user_id", userID),
					zap.String("currency", currency),
				)
				continue
			}
			accountIDs = append(accountIDs, account.ID)
		}
	}

	logger.Info("Created accounts in batch", zap.Int("count", len(accountIDs)))

	// Batch get accounts
	batchUserIDs := []accounts.BatchAccountRequest{}
	for _, userID := range userIDs {
		for _, currency := range currencies {
			batchUserIDs = append(batchUserIDs, accounts.BatchAccountRequest{
				UserID:   userID,
				Currency: currency,
			})
		}
	}

	batchAccounts, err := service.BatchGetAccounts(ctx, batchUserIDs)
	if err != nil {
		logger.Error("Failed to batch get accounts", zap.Error(err))
		return
	}
	logger.Info("Retrieved accounts in batch", zap.Int("count", len(batchAccounts)))

	// Demonstrate concurrent balance updates
	logger.Info("Performing concurrent balance updates...")
	updateCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	for i := 0; i < 100; i++ {
		go func(i int) {
			userID := userIDs[i%len(userIDs)]
			currency := currencies[i%len(currencies)]
			amount := decimal.NewFromFloat(float64(i + 1))

			err := service.UpdateBalance(updateCtx, userID, currency, amount, "test_deposit", fmt.Sprintf("concurrent_test_%d", i))
			if err != nil {
				logger.Error("Concurrent update failed", zap.Error(err), zap.Int("iteration", i))
			}
		}(i)
	}

	// Wait a moment for concurrent operations to complete
	time.Sleep(5 * time.Second)
	logger.Info("Concurrent operations completed")
}

func demonstrateMonitoring(ctx context.Context, service *accounts.UltraHighConcurrencyService, logger *zap.Logger) {
	logger.Info("=== Demonstrating Monitoring and Health Checks ===")

	// Health check
	healthStatus := service.HealthCheck(ctx)
	logger.Info("Health check results", zap.Any("status", healthStatus))

	// Metrics
	metrics := service.GetMetrics()
	logger.Info("Service metrics", zap.Any("metrics", metrics))

	// Performance statistics
	logger.Info("=== Performance Statistics ===")
	if metricsMap, ok := metrics["performance"].(map[string]interface{}); ok {
		for key, value := range metricsMap {
			logger.Info("Performance metric", zap.String("metric", key), zap.Any("value", value))
		}
	}
}

// Mock bookkeeper service for demonstration
type mockBookkeeperService struct{}

func (m *mockBookkeeperService) CreateEntry(ctx context.Context, userID, currency string, amount decimal.Decimal, entryType, reference, description string) error {
	// Mock implementation
	return nil
}

func (m *mockBookkeeperService) GetBalance(ctx context.Context, userID, currency string) (decimal.Decimal, error) {
	// Mock implementation
	return decimal.Zero, nil
}

func (m *mockBookkeeperService) GetEntries(ctx context.Context, userID, currency string, limit, offset int) ([]interface{}, int64, error) {
	// Mock implementation
	return []interface{}{}, 0, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
