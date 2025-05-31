package database

import (
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// NewPostgresDB creates a new PostgreSQL database connection with optimized pooling
func NewPostgresDB(dsn string, maxOpen, maxIdle, connMaxLife int) (*gorm.DB, error) {
	// Create database connection with optimized settings
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
		// Optimize for high-performance trading
		PrepareStmt:                              true,
		DisableForeignKeyConstraintWhenMigrating: false,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Get underlying SQL DB
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}

	// Set optimized connection pool settings for trading engine
	// Default to trading-optimized values if not specified
	if maxOpen == 0 {
		maxOpen = 100 // High concurrency for trading operations
	}
	if maxIdle == 0 {
		maxIdle = 10 // Keep connections ready but not excessive
	}
	if connMaxLife == 0 {
		connMaxLife = 3600 // 1 hour default lifetime
	}

	sqlDB.SetMaxIdleConns(maxIdle)
	sqlDB.SetMaxOpenConns(maxOpen)
	sqlDB.SetConnMaxLifetime(time.Duration(connMaxLife) * time.Second)

	// Set connection max idle time to prevent stale connections
	sqlDB.SetConnMaxIdleTime(15 * time.Minute)

	return db, nil
}
