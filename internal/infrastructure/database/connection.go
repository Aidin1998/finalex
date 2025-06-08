// Database initialization and connection management
package database

import (
	"context"
	"fmt"
	"hash/crc32"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/plugin/prometheus"
)

// initializeMaster initializes the master database connection
func (dm *DatabaseManager) initializeMaster() error {
	db, err := dm.createConnection(dm.config.MasterDSN, "master")
	if err != nil {
		return fmt.Errorf("failed to create master connection: %w", err)
	}

	dm.mu.Lock()
	dm.master = db
	dm.mu.Unlock()

	dm.logger.Info("master database connection initialized")
	return nil
}

// initializeSlaves initializes slave database connections
func (dm *DatabaseManager) initializeSlaves() error {
	if len(dm.config.SlaveDSNs) == 0 {
		dm.logger.Info("no slave databases configured")
		return nil
	}

	slaves := make([]*gorm.DB, 0, len(dm.config.SlaveDSNs))

	for i, dsn := range dm.config.SlaveDSNs {
		db, err := dm.createConnection(dsn, fmt.Sprintf("slave_%d", i))
		if err != nil {
			dm.logger.Warn("failed to create slave connection",
				zap.Int("slave_index", i),
				zap.Error(err))
			continue
		}
		slaves = append(slaves, db)
	}

	dm.mu.Lock()
	dm.slaves = slaves
	dm.mu.Unlock()

	dm.logger.Info("slave database connections initialized",
		zap.Int("slave_count", len(slaves)))
	return nil
}

// initializeShards initializes database shards
func (dm *DatabaseManager) initializeShards() error {
	if len(dm.config.Shards) == 0 {
		dm.logger.Info("no database shards configured")
		return nil
	}

	for _, shardConfig := range dm.config.Shards {
		shardManager, err := dm.createShardManager(shardConfig)
		if err != nil {
			dm.logger.Warn("failed to create shard",
				zap.String("shard_name", shardConfig.Name),
				zap.Error(err))
			continue
		}

		dm.shards[shardConfig.Name] = shardManager
	}

	dm.logger.Info("database shards initialized",
		zap.Int("shard_count", len(dm.shards)))
	return nil
}

// createConnection creates a new database connection with optimal settings
func (dm *DatabaseManager) createConnection(dsn, role string) (*gorm.DB, error) {
	// Configure GORM logger
	gormLogger := logger.Default
	if dm.logger != nil {
		gormLogger = logger.Default.LogMode(logger.Info)
	}

	// Create connection
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger:                                   gormLogger,
		SkipDefaultTransaction:                   true,
		PrepareStmt:                              true,
		DisableForeignKeyConstraintWhenMigrating: false,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Get underlying SQL DB for connection pool configuration
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get SQL DB: %w", err)
	}

	// Configure connection pool
	sqlDB.SetMaxOpenConns(dm.config.MaxOpenConns)
	sqlDB.SetMaxIdleConns(dm.config.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(dm.config.ConnMaxLifetime)
	sqlDB.SetConnMaxIdleTime(dm.config.ConnMaxIdleTime)

	// Configure timeouts
	if dm.config.ConnectionTimeout > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), dm.config.ConnectionTimeout)
		defer cancel()

		if err := sqlDB.PingContext(ctx); err != nil {
			return nil, fmt.Errorf("initial ping failed: %w", err)
		}
	}

	// Install Prometheus plugin if metrics are enabled
	if dm.config.EnableMetrics {
		if err := db.Use(prometheus.New(prometheus.Config{
			DBName:          role,
			RefreshInterval: 15,
			MetricsCollector: []prometheus.MetricsCollector{
				&prometheus.MySQL{
					VariableNames: []string{"Threads_running"},
				},
			},
		})); err != nil {
			dm.logger.Warn("failed to install prometheus plugin", zap.Error(err))
		}
	}

	// Update connection metrics
	stats := sqlDB.Stats()
	dm.metrics.ConnectionsOpen.WithLabelValues(role, "").Set(float64(stats.OpenConnections))
	dm.metrics.ConnectionsIdle.WithLabelValues(role, "").Set(float64(stats.Idle))
	dm.metrics.ConnectionsInUse.WithLabelValues(role, "").Set(float64(stats.InUse))

	return db, nil
}

// createShardManager creates a shard manager for a specific shard
func (dm *DatabaseManager) createShardManager(shardConfig ShardConfig) (*DatabaseManager, error) {
	config := &DatabaseConfig{
		MasterDSN:           shardConfig.MasterDSN,
		SlaveDSNs:           shardConfig.SlaveDSNs,
		MaxOpenConns:        dm.config.MaxOpenConns,
		MaxIdleConns:        dm.config.MaxIdleConns,
		ConnMaxLifetime:     dm.config.ConnMaxLifetime,
		ConnMaxIdleTime:     dm.config.ConnMaxIdleTime,
		HealthCheckInterval: dm.config.HealthCheckInterval,
		ConnectionTimeout:   dm.config.ConnectionTimeout,
		QueryTimeout:        dm.config.QueryTimeout,
		MaxRetries:          dm.config.MaxRetries,
		RetryDelay:          dm.config.RetryDelay,
		EncryptionKey:       dm.config.EncryptionKey,
		EnableMetrics:       dm.config.EnableMetrics,
		MetricsPrefix:       fmt.Sprintf("%s_shard_%s", dm.config.MetricsPrefix, shardConfig.Name),
	}

	return NewDatabaseManager(config, dm.logger.Named(fmt.Sprintf("shard_%s", shardConfig.Name)))
}

// ShardBy returns a database manager for the specified shard key
func (dm *DatabaseManager) ShardBy(key string) DatabaseInterface {
	if len(dm.shards) == 0 {
		return dm
	}

	// Calculate hash for the key
	hash := crc32.ChecksumIEEE([]byte(key))

	// Find the appropriate shard
	for _, shardConfig := range dm.config.Shards {
		if hash >= shardConfig.HashRangeStart && hash <= shardConfig.HashRangeEnd {
			if shard, exists := dm.shards[shardConfig.Name]; exists {
				return shard
			}
		}
	}

	// Fallback to default shard or master
	if len(dm.shards) > 0 {
		for _, shard := range dm.shards {
			return shard
		}
	}

	return dm
}

// GetShard returns a specific shard by name
func (dm *DatabaseManager) GetShard(name string) DatabaseInterface {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	if shard, exists := dm.shards[name]; exists {
		return shard
	}

	return dm
}

// startHealthMonitoring starts the health monitoring goroutine
func (dm *DatabaseManager) startHealthMonitoring() {
	if dm.config.HealthCheckInterval <= 0 {
		dm.logger.Info("health monitoring disabled")
		return
	}

	dm.healthTicker = time.NewTicker(dm.config.HealthCheckInterval)

	go func() {
		defer dm.healthTicker.Stop()

		for {
			select {
			case <-dm.healthCtx.Done():
				return
			case <-dm.healthTicker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				status := dm.HealthCheck(ctx)
				cancel()

				if !status.Healthy {
					dm.logger.Warn("database health check failed",
						zap.String("error", status.LastError),
						zap.Duration("latency", status.Latency))
				}
			}
		}
	}()

	dm.logger.Info("health monitoring started",
		zap.Duration("interval", dm.config.HealthCheckInterval))
}

// AutoMigrate performs automatic migration for given models
func (dm *DatabaseManager) AutoMigrate(models ...interface{}) error {
	db := dm.Master()
	if db == nil {
		return fmt.Errorf("master database not available")
	}

	start := time.Now()
	defer func() {
		duration := time.Since(start)
		dm.metrics.QueryDuration.WithLabelValues("migrate", "schema", "").Observe(duration.Seconds())
	}()

	if err := db.AutoMigrate(models...); err != nil {
		dm.metrics.QueryErrors.WithLabelValues("migrate", "schema", "", "migration_error").Inc()
		return fmt.Errorf("auto migration failed: %w", err)
	}

	dm.metrics.QueryTotal.WithLabelValues("migrate", "schema", "").Inc()
	dm.logger.Info("auto migration completed",
		zap.Int("model_count", len(models)),
		zap.Duration("duration", time.Since(start)))

	return nil
}

// Migrate performs a specific migration version
func (dm *DatabaseManager) Migrate(version string) error {
	// This would typically integrate with a migration tool like golang-migrate
	// For now, we'll implement a simple version tracking

	db := dm.Master()
	if db == nil {
		return fmt.Errorf("master database not available")
	}

	// Ensure migration table exists
	if err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version VARCHAR(255) PRIMARY KEY,
			applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`).Error; err != nil {
		return fmt.Errorf("failed to create migration table: %w", err)
	}

	// Check if migration already applied
	var count int64
	if err := db.Model(&struct{}{}).
		Table("schema_migrations").
		Where("version = ?", version).
		Count(&count).Error; err != nil {
		return fmt.Errorf("failed to check migration status: %w", err)
	}

	if count > 0 {
		dm.logger.Info("migration already applied", zap.String("version", version))
		return nil
	}

	// Load and execute migration file
	migrationPath := fmt.Sprintf("migrations/postgres/%s.up.sql", version)
	migrationSQL, err := loadMigrationFile(migrationPath)
	if err != nil {
		return fmt.Errorf("failed to load migration file: %w", err)
	}

	// Execute migration in transaction
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(migrationSQL).Error; err != nil {
			return fmt.Errorf("migration execution failed: %w", err)
		}

		// Record migration as applied
		if err := tx.Exec("INSERT INTO schema_migrations (version) VALUES (?)", version).Error; err != nil {
			return fmt.Errorf("failed to record migration: %w", err)
		}

		dm.logger.Info("migration applied successfully", zap.String("version", version))
		return nil
	})
}

// loadMigrationFile loads a migration file (placeholder implementation)
func loadMigrationFile(path string) (string, error) {
	// In a real implementation, this would read from the filesystem
	// For now, return a placeholder
	return "-- Migration placeholder", nil
}

// reconnectWithRetry attempts to reconnect to the database with retry logic
func (dm *DatabaseManager) reconnectWithRetry(dsn, role string) (*gorm.DB, error) {
	var lastError error

	for attempt := 0; attempt <= dm.maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(dm.config.RetryDelay)
			dm.logger.Info("retrying database connection",
				zap.String("role", role),
				zap.Int("attempt", attempt))
		}

		db, err := dm.createConnection(dsn, role)
		if err == nil {
			dm.logger.Info("database reconnection successful",
				zap.String("role", role),
				zap.Int("attempts", attempt+1))
			return db, nil
		}

		lastError = err
		dm.logger.Warn("database connection attempt failed",
			zap.String("role", role),
			zap.Int("attempt", attempt),
			zap.Error(err))
	}

	return nil, fmt.Errorf("failed to reconnect after %d attempts: %w", dm.maxRetries, lastError)
}

// validateQuery performs basic SQL injection protection
func (dm *DatabaseManager) validateQuery(query string, args ...interface{}) error {
	// Basic SQL injection checks
	query = strings.ToLower(strings.TrimSpace(query))

	// Check for dangerous patterns
	dangerousPatterns := []string{
		"drop table",
		"drop database",
		"delete from",
		"truncate",
		"alter table",
		"create table",
		"insert into",
		"update ",
		"union all",
		"union select",
		"' or '1'='1",
		"' or 1=1",
		"'; drop",
		"'; delete",
		"'; update",
		"'; insert",
	}

	for _, pattern := range dangerousPatterns {
		if strings.Contains(query, pattern) {
			return fmt.Errorf("potentially dangerous SQL pattern detected: %s", pattern)
		}
	}

	// Ensure parameterized queries
	if len(args) == 0 && strings.Contains(query, "'") {
		return fmt.Errorf("use parameterized queries instead of string literals")
	}

	return nil
}
