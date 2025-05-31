package database

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// SchemaOptimizer handles database schema optimizations including partitioning and constraints
type SchemaOptimizer struct {
	db     *gorm.DB
	logger *zap.Logger
	config *SchemaOptimizerConfig
}

// SchemaOptimizerConfig holds configuration for schema optimization
type SchemaOptimizerConfig struct {
	EnablePartitioning    bool          `yaml:"enable_partitioning" json:"enable_partitioning"`
	PartitionInterval     string        `yaml:"partition_interval" json:"partition_interval"` // daily, weekly, monthly
	RetentionPeriod       time.Duration `yaml:"retention_period" json:"retention_period"`
	EnableConstraints     bool          `yaml:"enable_constraints" json:"enable_constraints"`
	EnableCompression     bool          `yaml:"enable_compression" json:"enable_compression"`
	OptimizeStatistics    bool          `yaml:"optimize_statistics" json:"optimize_statistics"`
	AutoVacuumSettings    *VacuumConfig `yaml:"auto_vacuum_settings" json:"auto_vacuum_settings"`
}

// VacuumConfig holds VACUUM and ANALYZE settings
type VacuumConfig struct {
	Enabled                bool    `yaml:"enabled" json:"enabled"`
	ScaleFactor            float64 `yaml:"scale_factor" json:"scale_factor"`
	Threshold              int64   `yaml:"threshold" json:"threshold"`
	AnalyzeScaleFactor     float64 `yaml:"analyze_scale_factor" json:"analyze_scale_factor"`
	AnalyzeThreshold       int64   `yaml:"analyze_threshold" json:"analyze_threshold"`
	VacuumCostDelay        int     `yaml:"vacuum_cost_delay" json:"vacuum_cost_delay"`
	VacuumCostLimit        int     `yaml:"vacuum_cost_limit" json:"vacuum_cost_limit"`
}

// DefaultSchemaOptimizerConfig returns default configuration
func DefaultSchemaOptimizerConfig() *SchemaOptimizerConfig {
	return &SchemaOptimizerConfig{
		EnablePartitioning: true,
		PartitionInterval:  "daily",
		RetentionPeriod:    30 * 24 * time.Hour, // 30 days
		EnableConstraints:  true,
		EnableCompression:  true,
		OptimizeStatistics: true,
		AutoVacuumSettings: &VacuumConfig{
			Enabled:                true,
			ScaleFactor:            0.1,  // 10%
			Threshold:              50,
			AnalyzeScaleFactor:     0.05, // 5%
			AnalyzeThreshold:       50,
			VacuumCostDelay:        10,
			VacuumCostLimit:        200,
		},
	}
}

// PartitionDefinition represents a table partition
type PartitionDefinition struct {
	TableName      string
	PartitionName  string
	PartitionType  string // RANGE, LIST, HASH
	PartitionKey   string
	PartitionValue string
	Constraints    []string
}

// NewSchemaOptimizer creates a new schema optimizer
func NewSchemaOptimizer(db *gorm.DB, logger *zap.Logger, config *SchemaOptimizerConfig) *SchemaOptimizer {
	if config == nil {
		config = DefaultSchemaOptimizerConfig()
	}
	
	return &SchemaOptimizer{
		db:     db,
		logger: logger,
		config: config,
	}
}

// OptimizeSchema applies all schema optimizations
func (so *SchemaOptimizer) OptimizeSchema(ctx context.Context) error {
	so.logger.Info("Starting schema optimization")
	
	// 1. Create optimized partitions
	if so.config.EnablePartitioning {
		if err := so.createOptimizedPartitions(ctx); err != nil {
			return fmt.Errorf("failed to create partitions: %w", err)
		}
	}
	
	// 2. Apply constraints
	if so.config.EnableConstraints {
		if err := so.applyOptimizedConstraints(ctx); err != nil {
			return fmt.Errorf("failed to apply constraints: %w", err)
		}
	}
	
	// 3. Enable compression
	if so.config.EnableCompression {
		if err := so.enableTableCompression(ctx); err != nil {
			return fmt.Errorf("failed to enable compression: %w", err)
		}
	}
	
	// 4. Optimize statistics
	if so.config.OptimizeStatistics {
		if err := so.optimizeTableStatistics(ctx); err != nil {
			return fmt.Errorf("failed to optimize statistics: %w", err)
		}
	}
	
	// 5. Configure auto-vacuum
	if so.config.AutoVacuumSettings != nil && so.config.AutoVacuumSettings.Enabled {
		if err := so.configureAutoVacuum(ctx); err != nil {
			return fmt.Errorf("failed to configure auto-vacuum: %w", err)
		}
	}
	
	so.logger.Info("Schema optimization completed successfully")
	return nil
}

// createOptimizedPartitions creates partitioned tables for better performance
func (so *SchemaOptimizer) createOptimizedPartitions(ctx context.Context) error {
	so.logger.Info("Creating optimized partitions")
	
	// Define partition strategies for different tables
	partitionStrategies := []struct {
		table      string
		key        string
		strategy   string
		interval   string
	}{
		{"orders", "created_at", "RANGE", so.config.PartitionInterval},
		{"trades", "created_at", "RANGE", so.config.PartitionInterval},
		{"order_history", "created_at", "RANGE", so.config.PartitionInterval},
	}
	
	for _, strategy := range partitionStrategies {
		if err := so.createTablePartitions(ctx, strategy.table, strategy.key, strategy.strategy, strategy.interval); err != nil {
			so.logger.Error("Failed to create partitions for table",
				zap.String("table", strategy.table),
				zap.Error(err))
			return err
		}
	}
	
	return nil
}

// createTablePartitions creates partitions for a specific table
func (so *SchemaOptimizer) createTablePartitions(ctx context.Context, tableName, partitionKey, strategy, interval string) error {
	// Check if table is already partitioned
	var count int64
	err := so.db.WithContext(ctx).Raw(`
		SELECT COUNT(*) 
		FROM pg_partitioned_table pt
		JOIN pg_class c ON pt.partrelid = c.oid
		WHERE c.relname = ?
	`, tableName).Scan(&count).Error
	
	if err != nil {
		return fmt.Errorf("failed to check partition status: %w", err)
	}
	
	if count > 0 {
		so.logger.Debug("Table is already partitioned, skipping", zap.String("table", tableName))
		return so.createMissingPartitions(ctx, tableName, partitionKey, interval)
	}
	
	// Create partitioned table
	so.logger.Info("Converting table to partitioned table", zap.String("table", tableName))
	
	// Step 1: Rename existing table
	renameSQL := fmt.Sprintf("ALTER TABLE %s RENAME TO %s_old", tableName, tableName)
	if err := so.db.WithContext(ctx).Exec(renameSQL).Error; err != nil {
		return fmt.Errorf("failed to rename table: %w", err)
	}
	
	// Step 2: Create partitioned table
	createPartitionedTableSQL := so.buildPartitionedTableSQL(tableName, partitionKey, strategy)
	if err := so.db.WithContext(ctx).Exec(createPartitionedTableSQL).Error; err != nil {
		return fmt.Errorf("failed to create partitioned table: %w", err)
	}
	
	// Step 3: Create partitions for current and future periods
	if err := so.createTimeBasedPartitions(ctx, tableName, partitionKey, interval, 12); err != nil {
		return fmt.Errorf("failed to create time-based partitions: %w", err)
	}
	
	// Step 4: Migrate data from old table
	if err := so.migrateDataToPartitions(ctx, tableName); err != nil {
		return fmt.Errorf("failed to migrate data: %w", err)
	}
	
	// Step 5: Drop old table
	dropSQL := fmt.Sprintf("DROP TABLE %s_old", tableName)
	if err := so.db.WithContext(ctx).Exec(dropSQL).Error; err != nil {
		so.logger.Warn("Failed to drop old table", zap.String("table", tableName), zap.Error(err))
	}
	
	so.logger.Info("Successfully converted table to partitioned",
		zap.String("table", tableName),
		zap.String("strategy", strategy))
	
	return nil
}

// buildPartitionedTableSQL builds the SQL for creating a partitioned table
func (so *SchemaOptimizer) buildPartitionedTableSQL(tableName, partitionKey, strategy string) string {
	switch tableName {
	case "orders":
		return fmt.Sprintf(`
			CREATE TABLE %s (
				id UUID PRIMARY KEY,
				user_id VARCHAR(255) NOT NULL,
				symbol VARCHAR(50) NOT NULL,
				side VARCHAR(10) NOT NULL,
				type VARCHAR(20) NOT NULL,
				price DECIMAL(20,8) NOT NULL,
				quantity DECIMAL(20,8) NOT NULL,
				filled_quantity DECIMAL(20,8) DEFAULT 0,
				status VARCHAR(20) NOT NULL,
				time_in_force VARCHAR(10) DEFAULT 'GTC',
				stop_price DECIMAL(20,8),
				average_price DECIMAL(20,8),
				display_quantity DECIMAL(20,8),
				trailing_offset DECIMAL(20,8),
				expires_at TIMESTAMP,
				oco_group_id UUID,
				parent_order_id UUID,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
			) PARTITION BY RANGE (%s)
		`, tableName, partitionKey)
		
	case "trades":
		return fmt.Sprintf(`
			CREATE TABLE %s (
				id UUID PRIMARY KEY,
				order_id UUID NOT NULL,
				symbol VARCHAR(50) NOT NULL,
				price DECIMAL(20,8) NOT NULL,
				quantity DECIMAL(20,8) NOT NULL,
				side VARCHAR(10) NOT NULL,
				is_maker BOOLEAN NOT NULL DEFAULT false,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
			) PARTITION BY RANGE (%s)
		`, tableName, partitionKey)
		
	case "order_history":
		return fmt.Sprintf(`
			CREATE TABLE %s (
				id UUID PRIMARY KEY,
				order_id UUID NOT NULL,
				status VARCHAR(20) NOT NULL,
				filled_quantity DECIMAL(20,8),
				average_price DECIMAL(20,8),
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
			) PARTITION BY RANGE (%s)
		`, tableName, partitionKey)
		
	default:
		return ""
	}
}

// createTimeBasedPartitions creates time-based partitions
func (so *SchemaOptimizer) createTimeBasedPartitions(ctx context.Context, tableName, partitionKey, interval string, numPartitions int) error {
	now := time.Now()
	
	for i := -6; i < numPartitions-6; i++ { // Create partitions for past 6 periods and future periods
		var start, end time.Time
		var partitionSuffix string
		
		switch interval {
		case "daily":
			start = now.AddDate(0, 0, i).Truncate(24 * time.Hour)
			end = start.AddDate(0, 0, 1)
			partitionSuffix = start.Format("20060102")
			
		case "weekly":
			start = now.AddDate(0, 0, i*7).Truncate(24 * time.Hour)
			// Adjust to start of week (Monday)
			weekday := int(start.Weekday())
			if weekday == 0 {
				weekday = 7
			}
			start = start.AddDate(0, 0, -(weekday-1))
			end = start.AddDate(0, 0, 7)
			partitionSuffix = fmt.Sprintf("w%s", start.Format("200601"))
			
		case "monthly":
			start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).AddDate(0, i, 0)
			end = start.AddDate(0, 1, 0)
			partitionSuffix = start.Format("200601")
			
		default:
			return fmt.Errorf("unsupported partition interval: %s", interval)
		}
		
		partitionName := fmt.Sprintf("%s_%s", tableName, partitionSuffix)
		
		// Check if partition already exists
		var exists int64
		err := so.db.WithContext(ctx).Raw(`
			SELECT COUNT(*) 
			FROM pg_class 
			WHERE relname = ? AND relkind = 'r'
		`, partitionName).Scan(&exists).Error
		
		if err != nil {
			return fmt.Errorf("failed to check partition existence: %w", err)
		}
		
		if exists > 0 {
			continue // Partition already exists
		}
		
		// Create partition
		createPartitionSQL := fmt.Sprintf(`
			CREATE TABLE %s PARTITION OF %s 
			FOR VALUES FROM ('%s') TO ('%s')
		`, partitionName, tableName, start.Format("2006-01-02"), end.Format("2006-01-02"))
		
		if err := so.db.WithContext(ctx).Exec(createPartitionSQL).Error; err != nil {
			so.logger.Error("Failed to create partition",
				zap.String("partition", partitionName),
				zap.Error(err))
			continue
		}
		
		// Add partition-specific indexes
		if err := so.createPartitionIndexes(ctx, partitionName, tableName); err != nil {
			so.logger.Warn("Failed to create partition indexes",
				zap.String("partition", partitionName),
				zap.Error(err))
		}
		
		so.logger.Debug("Created partition",
			zap.String("partition", partitionName),
			zap.String("start", start.Format("2006-01-02")),
			zap.String("end", end.Format("2006-01-02")))
	}
	
	return nil
}

// createMissingPartitions creates missing partitions for already partitioned tables
func (so *SchemaOptimizer) createMissingPartitions(ctx context.Context, tableName, partitionKey, interval string) error {
	// Get existing partitions
	var existingPartitions []string
	err := so.db.WithContext(ctx).Raw(`
		SELECT schemaname||'.'||tablename as partition_name
		FROM pg_tables 
		WHERE tablename LIKE ? AND schemaname = 'public'
	`, tableName+"_%").Scan(&existingPartitions).Error
	
	if err != nil {
		return fmt.Errorf("failed to get existing partitions: %w", err)
	}
	
	// Create missing partitions (simplified logic)
	return so.createTimeBasedPartitions(ctx, tableName, partitionKey, interval, 6)
}

// migrateDataToPartitions migrates data from old table to partitioned table
func (so *SchemaOptimizer) migrateDataToPartitions(ctx context.Context, tableName string) error {
	so.logger.Info("Migrating data to partitioned table", zap.String("table", tableName))
	
	// Insert data from old table to new partitioned table
	insertSQL := fmt.Sprintf("INSERT INTO %s SELECT * FROM %s_old", tableName, tableName)
	
	start := time.Now()
	result := so.db.WithContext(ctx).Exec(insertSQL)
	duration := time.Since(start)
	
	if result.Error != nil {
		return fmt.Errorf("failed to migrate data: %w", result.Error)
	}
	
	so.logger.Info("Data migration completed",
		zap.String("table", tableName),
		zap.Int64("rows", result.RowsAffected),
		zap.Duration("duration", duration))
	
	return nil
}

// createPartitionIndexes creates indexes for a partition
func (so *SchemaOptimizer) createPartitionIndexes(ctx context.Context, partitionName, baseTableName string) error {
	var indexes []string
	
	switch baseTableName {
	case "orders":
		indexes = []string{
			fmt.Sprintf("CREATE INDEX CONCURRENTLY %s_user_status_idx ON %s (user_id, status)", partitionName, partitionName),
			fmt.Sprintf("CREATE INDEX CONCURRENTLY %s_symbol_side_idx ON %s (symbol, side)", partitionName, partitionName),
			fmt.Sprintf("CREATE INDEX CONCURRENTLY %s_status_created_idx ON %s (status, created_at)", partitionName, partitionName),
		}
	case "trades":
		indexes = []string{
			fmt.Sprintf("CREATE INDEX CONCURRENTLY %s_order_id_idx ON %s (order_id)", partitionName, partitionName),
			fmt.Sprintf("CREATE INDEX CONCURRENTLY %s_symbol_created_idx ON %s (symbol, created_at)", partitionName, partitionName),
		}
	}
	
	for _, indexSQL := range indexes {
		if err := so.db.WithContext(ctx).Exec(indexSQL).Error; err != nil {
			so.logger.Warn("Failed to create partition index", zap.String("sql", indexSQL), zap.Error(err))
		}
	}
	
	return nil
}

// applyOptimizedConstraints applies optimized constraints to tables
func (so *SchemaOptimizer) applyOptimizedConstraints(ctx context.Context) error {
	so.logger.Info("Applying optimized constraints")
	
	constraints := []string{
		// Orders table constraints
		`ALTER TABLE orders ADD CONSTRAINT orders_price_positive CHECK (price > 0)`,
		`ALTER TABLE orders ADD CONSTRAINT orders_quantity_positive CHECK (quantity > 0)`,
		`ALTER TABLE orders ADD CONSTRAINT orders_filled_quantity_valid CHECK (filled_quantity >= 0 AND filled_quantity <= quantity)`,
		`ALTER TABLE orders ADD CONSTRAINT orders_side_valid CHECK (side IN ('buy', 'sell'))`,
		`ALTER TABLE orders ADD CONSTRAINT orders_status_valid CHECK (status IN ('open', 'partially_filled', 'filled', 'cancelled', 'rejected', 'pending_trigger'))`,
		`ALTER TABLE orders ADD CONSTRAINT orders_type_valid CHECK (type IN ('market', 'limit', 'stop', 'stop_limit', 'trailing_stop'))`,
		
		// Trades table constraints
		`ALTER TABLE trades ADD CONSTRAINT trades_price_positive CHECK (price > 0)`,
		`ALTER TABLE trades ADD CONSTRAINT trades_quantity_positive CHECK (quantity > 0)`,
		`ALTER TABLE trades ADD CONSTRAINT trades_side_valid CHECK (side IN ('buy', 'sell'))`,
		
		// Performance constraints
		`ALTER TABLE orders ADD CONSTRAINT orders_created_at_not_future CHECK (created_at <= CURRENT_TIMESTAMP)`,
		`ALTER TABLE trades ADD CONSTRAINT trades_created_at_not_future CHECK (created_at <= CURRENT_TIMESTAMP)`,
	}
	
	for _, constraintSQL := range constraints {
		if err := so.db.WithContext(ctx).Exec(constraintSQL).Error; err != nil {
			// Log warning but continue - constraint might already exist
			so.logger.Debug("Constraint application failed (may already exist)", zap.String("sql", constraintSQL), zap.Error(err))
		}
	}
	
	return nil
}

// enableTableCompression enables compression for large tables
func (so *SchemaOptimizer) enableTableCompression(ctx context.Context) error {
	so.logger.Info("Enabling table compression")
	
	tables := []string{"orders", "trades", "order_history"}
	
	for _, table := range tables {
		// Enable TOAST compression
		compressSQL := fmt.Sprintf("ALTER TABLE %s SET (toast_tuple_target = 128)", table)
		if err := so.db.WithContext(ctx).Exec(compressSQL).Error; err != nil {
			so.logger.Warn("Failed to enable compression", zap.String("table", table), zap.Error(err))
		}
		
		// Set storage parameters for better compression
		storageSQL := fmt.Sprintf("ALTER TABLE %s SET (fillfactor = 90)", table)
		if err := so.db.WithContext(ctx).Exec(storageSQL).Error; err != nil {
			so.logger.Warn("Failed to set storage parameters", zap.String("table", table), zap.Error(err))
		}
	}
	
	return nil
}

// optimizeTableStatistics optimizes table statistics for better query planning
func (so *SchemaOptimizer) optimizeTableStatistics(ctx context.Context) error {
	so.logger.Info("Optimizing table statistics")
	
	// Set custom statistics targets for important columns
	statisticsSettings := []string{
		"ALTER TABLE orders ALTER COLUMN symbol SET STATISTICS 1000",
		"ALTER TABLE orders ALTER COLUMN user_id SET STATISTICS 1000",
		"ALTER TABLE orders ALTER COLUMN status SET STATISTICS 500",
		"ALTER TABLE orders ALTER COLUMN created_at SET STATISTICS 1000",
		"ALTER TABLE trades ALTER COLUMN symbol SET STATISTICS 1000",
		"ALTER TABLE trades ALTER COLUMN created_at SET STATISTICS 1000",
	}
	
	for _, statsSQL := range statisticsSettings {
		if err := so.db.WithContext(ctx).Exec(statsSQL).Error; err != nil {
			so.logger.Warn("Failed to set statistics", zap.String("sql", statsSQL), zap.Error(err))
		}
	}
	
	// Run ANALYZE to update statistics
	tables := []string{"orders", "trades", "order_history"}
	for _, table := range tables {
		analyzeSQL := fmt.Sprintf("ANALYZE %s", table)
		if err := so.db.WithContext(ctx).Exec(analyzeSQL).Error; err != nil {
			so.logger.Warn("Failed to analyze table", zap.String("table", table), zap.Error(err))
		}
	}
	
	return nil
}

// configureAutoVacuum configures auto-vacuum settings for optimal performance
func (so *SchemaOptimizer) configureAutoVacuum(ctx context.Context) error {
	so.logger.Info("Configuring auto-vacuum settings")
	
	vacuum := so.config.AutoVacuumSettings
	
	tables := []string{"orders", "trades", "order_history"}
	
	for _, table := range tables {
		settings := []string{
			fmt.Sprintf("autovacuum_vacuum_scale_factor = %f", vacuum.ScaleFactor),
			fmt.Sprintf("autovacuum_vacuum_threshold = %d", vacuum.Threshold),
			fmt.Sprintf("autovacuum_analyze_scale_factor = %f", vacuum.AnalyzeScaleFactor),
			fmt.Sprintf("autovacuum_analyze_threshold = %d", vacuum.AnalyzeThreshold),
			fmt.Sprintf("autovacuum_vacuum_cost_delay = %d", vacuum.VacuumCostDelay),
			fmt.Sprintf("autovacuum_vacuum_cost_limit = %d", vacuum.VacuumCostLimit),
		}
		
		settingsStr := strings.Join(settings, ", ")
		vacuumSQL := fmt.Sprintf("ALTER TABLE %s SET (%s)", table, settingsStr)
		
		if err := so.db.WithContext(ctx).Exec(vacuumSQL).Error; err != nil {
			so.logger.Warn("Failed to configure auto-vacuum", zap.String("table", table), zap.Error(err))
		} else {
			so.logger.Debug("Auto-vacuum configured", zap.String("table", table))
		}
	}
	
	return nil
}

// CleanupOldPartitions removes old partitions based on retention policy
func (so *SchemaOptimizer) CleanupOldPartitions(ctx context.Context) error {
	so.logger.Info("Cleaning up old partitions")
	
	cutoffTime := time.Now().Add(-so.config.RetentionPeriod)
	
	// Get all partition tables
	var partitions []struct {
		TableName string `json:"tablename"`
		SchemaName string `json:"schemaname"`
	}
	
	err := so.db.WithContext(ctx).Raw(`
		SELECT tablename, schemaname
		FROM pg_tables 
		WHERE schemaname = 'public' 
		  AND (tablename LIKE 'orders_%' OR tablename LIKE 'trades_%' OR tablename LIKE 'order_history_%')
	`).Scan(&partitions).Error
	
	if err != nil {
		return fmt.Errorf("failed to get partitions: %w", err)
	}
	
	for _, partition := range partitions {
		// Extract date from partition name and check if it's old enough
		if so.shouldDropPartition(partition.TableName, cutoffTime) {
			dropSQL := fmt.Sprintf("DROP TABLE IF EXISTS %s", partition.TableName)
			if err := so.db.WithContext(ctx).Exec(dropSQL).Error; err != nil {
				so.logger.Error("Failed to drop old partition",
					zap.String("partition", partition.TableName),
					zap.Error(err))
			} else {
				so.logger.Info("Dropped old partition", zap.String("partition", partition.TableName))
			}
		}
	}
	
	return nil
}

// shouldDropPartition determines if a partition should be dropped based on retention policy
func (so *SchemaOptimizer) shouldDropPartition(partitionName string, cutoffTime time.Time) bool {
	// Extract date from partition name (simplified logic)
	parts := strings.Split(partitionName, "_")
	if len(parts) < 2 {
		return false
	}
	
	dateStr := parts[len(parts)-1]
	
	// Try to parse different date formats
	formats := []string{"20060102", "200601", "2006w01"}
	
	for _, format := range formats {
		if partitionTime, err := time.Parse(format, dateStr); err == nil {
			return partitionTime.Before(cutoffTime)
		}
	}
	
	return false
}

// GetSchemaOptimizationReport generates a report of current schema optimizations
func (so *SchemaOptimizer) GetSchemaOptimizationReport(ctx context.Context) (*SchemaOptimizationReport, error) {
	report := &SchemaOptimizationReport{
		Timestamp: time.Now(),
	}
	
	// Get partition information
	partitionInfo, err := so.getPartitionInfo(ctx)
	if err != nil {
		so.logger.Error("Failed to get partition info", zap.Error(err))
	} else {
		report.PartitionInfo = partitionInfo
	}
	
	// Get constraint information
	constraintInfo, err := so.getConstraintInfo(ctx)
	if err != nil {
		so.logger.Error("Failed to get constraint info", zap.Error(err))
	} else {
		report.ConstraintInfo = constraintInfo
	}
	
	// Get table sizes
	tableSizes, err := so.getTableSizes(ctx)
	if err != nil {
		so.logger.Error("Failed to get table sizes", zap.Error(err))
	} else {
		report.TableSizes = tableSizes
	}
	
	return report, nil
}

// getPartitionInfo gets information about table partitions
func (so *SchemaOptimizer) getPartitionInfo(ctx context.Context) ([]PartitionInfo, error) {
	var partitions []PartitionInfo
	
	err := so.db.WithContext(ctx).Raw(`
		SELECT 
			pt.schemaname as schema_name,
			pt.tablename as table_name,
			pg_get_partkeydef(pt.partrelid) as partition_key,
			COUNT(pc.relname) as partition_count
		FROM pg_partitioned_table pt
		JOIN pg_class c ON pt.partrelid = c.oid
		JOIN pg_namespace n ON c.relnamespace = n.oid
		LEFT JOIN pg_inherits i ON pt.partrelid = i.inhparent
		LEFT JOIN pg_class pc ON i.inhrelid = pc.oid
		WHERE n.nspname = 'public'
		GROUP BY pt.schemaname, pt.tablename, pt.partrelid
	`).Scan(&partitions).Error
	
	return partitions, err
}

// getConstraintInfo gets information about table constraints
func (so *SchemaOptimizer) getConstraintInfo(ctx context.Context) ([]ConstraintInfo, error) {
	var constraints []ConstraintInfo
	
	err := so.db.WithContext(ctx).Raw(`
		SELECT 
			tc.table_name,
			tc.constraint_name,
			tc.constraint_type,
			pg_get_constraintdef(c.oid) as constraint_definition
		FROM information_schema.table_constraints tc
		JOIN pg_constraint c ON c.conname = tc.constraint_name
		WHERE tc.table_schema = 'public'
		  AND tc.table_name IN ('orders', 'trades', 'order_history')
		ORDER BY tc.table_name, tc.constraint_type
	`).Scan(&constraints).Error
	
	return constraints, err
}

// getTableSizes gets table and index sizes
func (so *SchemaOptimizer) getTableSizes(ctx context.Context) ([]TableSizeInfo, error) {
	var sizes []TableSizeInfo
	
	err := so.db.WithContext(ctx).Raw(`
		SELECT 
			tablename as table_name,
			pg_size_pretty(pg_total_relation_size(schemaname||'.'||tablename)) as total_size,
			pg_size_pretty(pg_relation_size(schemaname||'.'||tablename)) as table_size,
			pg_size_pretty(pg_total_relation_size(schemaname||'.'||tablename) - pg_relation_size(schemaname||'.'||tablename)) as index_size,
			pg_total_relation_size(schemaname||'.'||tablename) as total_size_bytes,
			pg_relation_size(schemaname||'.'||tablename) as table_size_bytes
		FROM pg_tables 
		WHERE schemaname = 'public'
		  AND tablename IN ('orders', 'trades', 'order_history')
		ORDER BY pg_total_relation_size(schemaname||'.'||tablename) DESC
	`).Scan(&sizes).Error
	
	return sizes, err
}

// Supporting types for schema optimization reporting
type SchemaOptimizationReport struct {
	Timestamp      time.Time         `json:"timestamp"`
	PartitionInfo  []PartitionInfo   `json:"partition_info"`
	ConstraintInfo []ConstraintInfo  `json:"constraint_info"`
	TableSizes     []TableSizeInfo   `json:"table_sizes"`
}

type PartitionInfo struct {
	SchemaName     string `json:"schema_name"`
	TableName      string `json:"table_name"`
	PartitionKey   string `json:"partition_key"`
	PartitionCount int    `json:"partition_count"`
}

type ConstraintInfo struct {
	TableName            string `json:"table_name"`
	ConstraintName       string `json:"constraint_name"`
	ConstraintType       string `json:"constraint_type"`
	ConstraintDefinition string `json:"constraint_definition"`
}

type TableSizeInfo struct {
	TableName      string `json:"table_name"`
	TotalSize      string `json:"total_size"`
	TableSize      string `json:"table_size"`
	IndexSize      string `json:"index_size"`
	TotalSizeBytes int64  `json:"total_size_bytes"`
	TableSizeBytes int64  `json:"table_size_bytes"`
}
