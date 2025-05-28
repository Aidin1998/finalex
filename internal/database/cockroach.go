package database

import (
	"database/sql"
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// NewCockroachDB creates a new CockroachDB connection with optimized settings
func NewCockroachDB(dsn string, maxOpen, maxIdle, connMaxLife int) (*gorm.DB, error) {
	// Use the Postgres driver for CockroachDB
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to CockroachDB: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying DB: %w", err)
	}

	// Connection pool
	sqlDB.SetMaxOpenConns(maxOpen)
	sqlDB.SetMaxIdleConns(maxIdle)
	sqlDB.SetConnMaxLifetime(time.Duration(connMaxLife) * time.Second)

	// CockroachDB requires serializable isolation
	db = db.Set("gorm:tx_options", &sql.TxOptions{Isolation: sql.LevelSerializable})

	return db, nil
}
