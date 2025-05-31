package database

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// IndexManager handles database index creation, optimization, and monitoring
type IndexManager struct {
	db     *gorm.DB
	logger *zap.Logger
	config *IndexManagerConfig
}

// IndexManagerConfig holds configuration for index management
type IndexManagerConfig struct {
	EnableConcurrentIndexing bool          `yaml:"enable_concurrent_indexing" json:"enable_concurrent_indexing"`
	IndexCreationTimeout     time.Duration `yaml:"index_creation_timeout" json:"index_creation_timeout"`
	AnalyzeAfterCreation     bool          `yaml:"analyze_after_creation" json:"analyze_after_creation"`
	MonitoringEnabled        bool          `yaml:"monitoring_enabled" json:"monitoring_enabled"`
}

// DefaultIndexManagerConfig returns default configuration
func DefaultIndexManagerConfig() *IndexManagerConfig {
	return &IndexManagerConfig{
		EnableConcurrentIndexing: true,
		IndexCreationTimeout:     30 * time.Minute,
		AnalyzeAfterCreation:     true,
		MonitoringEnabled:        true,
	}
}

// IndexDefinition represents a database index
type IndexDefinition struct {
	Name       string
	Table      string
	Columns    []string
	Unique     bool
	Concurrent bool
	Where      string // Partial index condition
	Using      string // Index method (btree, hash, gin, etc.)
	Include    []string // INCLUDE columns for covering indexes
}

// NewIndexManager creates a new index manager
func NewIndexManager(db *gorm.DB, logger *zap.Logger, config *IndexManagerConfig) *IndexManager {
	if config == nil {
		config = DefaultIndexManagerConfig()
	}
	
	return &IndexManager{
		db:     db,
		logger: logger,
		config: config,
	}
}

// GetTradingOptimizedIndexes returns indexes optimized for trading operations
func (im *IndexManager) GetTradingOptimizedIndexes() []IndexDefinition {
	return []IndexDefinition{
		// Orders table indexes
		{
			Name:    "orders_user_status_created_idx",
			Table:   "orders",
			Columns: []string{"user_id", "status", "created_at"},
			Where:   "status IN ('open', 'partially_filled', 'pending_trigger')",
			Using:   "btree",
		},
		{
			Name:    "orders_symbol_side_price_idx",
			Table:   "orders",
			Columns: []string{"symbol", "side", "price"},
			Where:   "status IN ('open', 'partially_filled')",
			Using:   "btree",
		},
		{
			Name:    "orders_symbol_status_created_idx",
			Table:   "orders",
			Columns: []string{"symbol", "status", "created_at"},
			Using:   "btree",
		},
		{
			Name:    "orders_status_created_covering_idx",
			Table:   "orders",
			Columns: []string{"status", "created_at"},
			Include: []string{"id", "user_id", "symbol", "side", "price", "quantity"},
			Where:   "status IN ('open', 'partially_filled')",
			Using:   "btree",
		},
		{
			Name:    "orders_user_symbol_side_idx",
			Table:   "orders",
			Columns: []string{"user_id", "symbol", "side"},
			Using:   "btree",
		},
		{
			Name:    "orders_oco_group_idx",
			Table:   "orders",
			Columns: []string{"oco_group_id"},
			Where:   "oco_group_id IS NOT NULL",
			Using:   "btree",
		},
		{
			Name:    "orders_parent_order_idx",
			Table:   "orders",
			Columns: []string{"parent_order_id"},
			Where:   "parent_order_id IS NOT NULL",
			Using:   "btree",
		},
		{
			Name:    "orders_expires_at_idx",
			Table:   "orders",
			Columns: []string{"expires_at"},
			Where:   "expires_at IS NOT NULL AND status IN ('open', 'partially_filled')",
			Using:   "btree",
		},
		{
			Name:    "orders_stop_price_idx",
			Table:   "orders",
			Columns: []string{"symbol", "side", "stop_price"},
			Where:   "stop_price IS NOT NULL AND status = 'pending_trigger'",
			Using:   "btree",
		},
		
		// Trades table indexes
		{
			Name:    "trades_symbol_created_idx",
			Table:   "trades",
			Columns: []string{"symbol", "created_at"},
			Using:   "btree",
		},
		{
			Name:    "trades_order_id_idx",
			Table:   "trades",
			Columns: []string{"order_id"},
			Using:   "btree",
		},
		{
			Name:    "trades_symbol_side_created_idx",
			Table:   "trades",
			Columns: []string{"symbol", "side", "created_at"},
			Using:   "btree",
		},
		{
			Name:    "trades_created_at_brin_idx",
			Table:   "trades",
			Columns: []string{"created_at"},
			Using:   "brin", // BRIN index for time-series data
		},
		{
			Name:    "trades_price_quantity_idx",
			Table:   "trades",
			Columns: []string{"symbol", "price", "quantity"},
			Using:   "btree",
		},
		
		// Order book reconstruction indexes
		{
			Name:    "orders_orderbook_buy_idx",
			Table:   "orders",
			Columns: []string{"symbol", "price", "created_at"},
			Where:   "side = 'buy' AND status IN ('open', 'partially_filled')",
			Using:   "btree",
		},
		{
			Name:    "orders_orderbook_sell_idx",
			Table:   "orders",
			Columns: []string{"symbol", "price", "created_at"},
			Where:   "side = 'sell' AND status IN ('open', 'partially_filled')",
			Using:   "btree",
		},
		
		// Performance monitoring indexes
		{
			Name:    "orders_updated_at_idx",
			Table:   "orders",
			Columns: []string{"updated_at"},
			Using:   "btree",
		},
		{
			Name:    "trades_maker_taker_idx",
			Table:   "trades",
			Columns: []string{"symbol", "is_maker", "created_at"},
			Using:   "btree",
		},
		
		// User activity indexes
		{
			Name:    "orders_user_created_covering_idx",
			Table:   "orders",
			Columns: []string{"user_id", "created_at"},
			Include: []string{"id", "symbol", "side", "type", "status", "price", "quantity"},
			Using:   "btree",
		},
		
		// Risk management indexes
		{
			Name:    "orders_user_symbol_status_idx",
			Table:   "orders",
			Columns: []string{"user_id", "symbol", "status"},
			Using:   "btree",
		},
		{
			Name:    "orders_large_size_idx",
			Table:   "orders",
			Columns: []string{"symbol", "quantity", "created_at"},
			Where:   "quantity > 1000 AND status IN ('open', 'partially_filled')",
			Using:   "btree",
		},
	}
}

// CreateIndex creates a single index
func (im *IndexManager) CreateIndex(ctx context.Context, index IndexDefinition) error {
	sql := im.buildCreateIndexSQL(index)
	
	start := time.Now()
	err := im.db.WithContext(ctx).Exec(sql).Error
	duration := time.Since(start)
	
	if err != nil {
		im.logger.Error("Failed to create index",
			zap.String("index", index.Name),
			zap.String("table", index.Table),
			zap.Error(err),
			zap.Duration("duration", duration))
		return fmt.Errorf("failed to create index %s: %w", index.Name, err)
	}
	
	im.logger.Info("Index created successfully",
		zap.String("index", index.Name),
		zap.String("table", index.Table),
		zap.Duration("duration", duration))
	
	// Run ANALYZE if configured
	if im.config.AnalyzeAfterCreation {
		analyzeSQL := fmt.Sprintf("ANALYZE %s", index.Table)
		if err := im.db.WithContext(ctx).Exec(analyzeSQL).Error; err != nil {
			im.logger.Warn("Failed to analyze table after index creation",
				zap.String("table", index.Table),
				zap.Error(err))
		}
	}
	
	return nil
}

// CreateAllTradingIndexes creates all trading-optimized indexes
func (im *IndexManager) CreateAllTradingIndexes(ctx context.Context) error {
	indexes := im.GetTradingOptimizedIndexes()
	
	im.logger.Info("Creating trading-optimized indexes", zap.Int("count", len(indexes)))
	
	for _, index := range indexes {
		// Check if index already exists
		exists, err := im.IndexExists(ctx, index.Name)
		if err != nil {
			im.logger.Error("Failed to check index existence",
				zap.String("index", index.Name),
				zap.Error(err))
			continue
		}
		
		if exists {
			im.logger.Debug("Index already exists, skipping",
				zap.String("index", index.Name))
			continue
		}
		
		if err := im.CreateIndex(ctx, index); err != nil {
			return fmt.Errorf("failed to create index %s: %w", index.Name, err)
		}
	}
	
	im.logger.Info("All trading indexes created successfully")
	return nil
}

// buildCreateIndexSQL builds the CREATE INDEX SQL statement
func (im *IndexManager) buildCreateIndexSQL(index IndexDefinition) string {
	var parts []string
	
	// Base CREATE INDEX
	createPart := "CREATE"
	if index.Unique {
		createPart += " UNIQUE"
	}
	createPart += " INDEX"
	
	if index.Concurrent && im.config.EnableConcurrentIndexing {
		createPart += " CONCURRENTLY"
	}
	
	parts = append(parts, createPart)
	parts = append(parts, fmt.Sprintf("%s ON %s", index.Name, index.Table))
	
	// Index method
	if index.Using != "" {
		parts = append(parts, fmt.Sprintf("USING %s", index.Using))
	}
	
	// Columns
	columnsPart := fmt.Sprintf("(%s)", strings.Join(index.Columns, ", "))
	parts = append(parts, columnsPart)
	
	// INCLUDE columns (for covering indexes)
	if len(index.Include) > 0 {
		includePart := fmt.Sprintf("INCLUDE (%s)", strings.Join(index.Include, ", "))
		parts = append(parts, includePart)
	}
	
	// WHERE clause (for partial indexes)
	if index.Where != "" {
		parts = append(parts, fmt.Sprintf("WHERE %s", index.Where))
	}
	
	return strings.Join(parts, " ")
}

// IndexExists checks if an index exists
func (im *IndexManager) IndexExists(ctx context.Context, indexName string) (bool, error) {
	var count int64
	query := `
		SELECT COUNT(*) 
		FROM pg_indexes 
		WHERE indexname = ?
	`
	
	err := im.db.WithContext(ctx).Raw(query, indexName).Count(&count).Error
	if err != nil {
		return false, err
	}
	
	return count > 0, nil
}

// DropIndex drops an index
func (im *IndexManager) DropIndex(ctx context.Context, indexName string) error {
	sql := fmt.Sprintf("DROP INDEX IF EXISTS %s", indexName)
	
	err := im.db.WithContext(ctx).Exec(sql).Error
	if err != nil {
		im.logger.Error("Failed to drop index",
			zap.String("index", indexName),
			zap.Error(err))
		return fmt.Errorf("failed to drop index %s: %w", indexName, err)
	}
	
	im.logger.Info("Index dropped successfully", zap.String("index", indexName))
	return nil
}

// GetIndexUsageStats returns index usage statistics
func (im *IndexManager) GetIndexUsageStats(ctx context.Context) ([]IndexUsageStat, error) {
	query := `
		SELECT 
			schemaname,
			tablename,
			indexname,
			idx_tup_read,
			idx_tup_fetch,
			idx_scan,
			CASE 
				WHEN idx_scan = 0 THEN 0 
				ELSE idx_tup_read::float / idx_scan 
			END as avg_tuples_per_scan
		FROM pg_stat_user_indexes 
		WHERE schemaname = 'public'
		  AND (tablename = 'orders' OR tablename = 'trades')
		ORDER BY idx_scan DESC
	`
	
	var stats []IndexUsageStat
	err := im.db.WithContext(ctx).Raw(query).Scan(&stats).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get index usage stats: %w", err)
	}
	
	return stats, nil
}

// GetUnusedIndexes returns indexes that are never used
func (im *IndexManager) GetUnusedIndexes(ctx context.Context) ([]string, error) {
	query := `
		SELECT indexname
		FROM pg_stat_user_indexes 
		WHERE schemaname = 'public'
		  AND (tablename = 'orders' OR tablename = 'trades')
		  AND idx_scan = 0
		ORDER BY indexname
	`
	
	var indexes []string
	err := im.db.WithContext(ctx).Raw(query).Scan(&indexes).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get unused indexes: %w", err)
	}
	
	return indexes, nil
}

// GetIndexSizes returns the size of all indexes
func (im *IndexManager) GetIndexSizes(ctx context.Context) ([]IndexSize, error) {
	query := `
		SELECT 
			schemaname,
			tablename,
			indexname,
			pg_size_pretty(pg_relation_size(indexname::regclass)) as size,
			pg_relation_size(indexname::regclass) as size_bytes
		FROM pg_stat_user_indexes 
		WHERE schemaname = 'public'
		  AND (tablename = 'orders' OR tablename = 'trades')
		ORDER BY pg_relation_size(indexname::regclass) DESC
	`
	
	var sizes []IndexSize
	err := im.db.WithContext(ctx).Raw(query).Scan(&sizes).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get index sizes: %w", err)
	}
	
	return sizes, nil
}

// OptimizeIndexes analyzes and suggests index optimizations
func (im *IndexManager) OptimizeIndexes(ctx context.Context) (*IndexOptimizationReport, error) {
	report := &IndexOptimizationReport{
		Timestamp: time.Now(),
	}
	
	// Get usage stats
	usageStats, err := im.GetIndexUsageStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get usage stats: %w", err)
	}
	report.UsageStats = usageStats
	
	// Get unused indexes
	unusedIndexes, err := im.GetUnusedIndexes(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get unused indexes: %w", err)
	}
	report.UnusedIndexes = unusedIndexes
	
	// Get index sizes
	indexSizes, err := im.GetIndexSizes(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get index sizes: %w", err)
	}
	report.IndexSizes = indexSizes
	
	// Generate recommendations
	report.Recommendations = im.generateRecommendations(usageStats, unusedIndexes, indexSizes)
	
	return report, nil
}

// generateRecommendations generates optimization recommendations
func (im *IndexManager) generateRecommendations(usage []IndexUsageStat, unused []string, sizes []IndexSize) []string {
	var recommendations []string
	
	// Unused indexes recommendation
	if len(unused) > 0 {
		recommendations = append(recommendations, 
			fmt.Sprintf("Consider dropping %d unused indexes: %s", 
				len(unused), strings.Join(unused, ", ")))
	}
	
	// Low usage indexes
	for _, stat := range usage {
		if stat.IdxScan < 10 && stat.IdxScan > 0 {
			recommendations = append(recommendations,
				fmt.Sprintf("Index %s has low usage (%d scans), consider reviewing its necessity", 
					stat.IndexName, stat.IdxScan))
		}
	}
	
	// Large indexes with low efficiency
	sizeMap := make(map[string]int64)
	for _, size := range sizes {
		sizeMap[size.IndexName] = size.SizeBytes
	}
	
	for _, stat := range usage {
		if size, exists := sizeMap[stat.IndexName]; exists && size > 100*1024*1024 { // > 100MB
			if stat.AvgTuplesPerScan < 1.0 {
				recommendations = append(recommendations,
					fmt.Sprintf("Large index %s (>100MB) has low selectivity (%.2f tuples/scan)", 
						stat.IndexName, stat.AvgTuplesPerScan))
			}
		}
	}
	
	return recommendations
}

// ReindexTable rebuilds all indexes for a table
func (im *IndexManager) ReindexTable(ctx context.Context, tableName string) error {
	sql := fmt.Sprintf("REINDEX TABLE %s", tableName)
	
	start := time.Now()
	err := im.db.WithContext(ctx).Exec(sql).Error
	duration := time.Since(start)
	
	if err != nil {
		im.logger.Error("Failed to reindex table",
			zap.String("table", tableName),
			zap.Error(err),
			zap.Duration("duration", duration))
		return fmt.Errorf("failed to reindex table %s: %w", tableName, err)
	}
	
	im.logger.Info("Table reindexed successfully",
		zap.String("table", tableName),
		zap.Duration("duration", duration))
	
	return nil
}

// IndexUsageStat represents index usage statistics
type IndexUsageStat struct {
	SchemaName         string  `json:"schema_name"`
	TableName          string  `json:"table_name"`
	IndexName          string  `json:"index_name"`
	IdxTupRead         int64   `json:"idx_tup_read"`
	IdxTupFetch        int64   `json:"idx_tup_fetch"`
	IdxScan            int64   `json:"idx_scan"`
	AvgTuplesPerScan   float64 `json:"avg_tuples_per_scan"`
}

// IndexSize represents index size information
type IndexSize struct {
	SchemaName string `json:"schema_name"`
	TableName  string `json:"table_name"`
	IndexName  string `json:"index_name"`
	Size       string `json:"size"`
	SizeBytes  int64  `json:"size_bytes"`
}

// IndexOptimizationReport contains index optimization analysis
type IndexOptimizationReport struct {
	Timestamp       time.Time        `json:"timestamp"`
	UsageStats      []IndexUsageStat `json:"usage_stats"`
	UnusedIndexes   []string         `json:"unused_indexes"`
	IndexSizes      []IndexSize      `json:"index_sizes"`
	Recommendations []string         `json:"recommendations"`
}
