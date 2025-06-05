// Partition management for horizontal scaling and sharding
package accounts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// PartitionManager handles database partitioning and sharding strategies
type PartitionManager struct {
	db              *gorm.DB
	logger          *zap.Logger
	partitions      map[string]*PartitionInfo
	shardMap        map[string]string // Maps user_id hash to partition
	mutex           sync.RWMutex
	metrics         *PartitionMetrics
	config          *PartitionConfig
	rebalancer      *PartitionRebalancer
}

// PartitionInfo contains metadata about a partition
type PartitionInfo struct {
	Name         string    `json:"name"`
	ShardID      string    `json:"shard_id"`
	HashRange    HashRange `json:"hash_range"`
	Status       string    `json:"status"` // active, readonly, migrating, inactive
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	RecordCount  int64     `json:"record_count"`
	SizeBytes    int64     `json:"size_bytes"`
	LastAccessed time.Time `json:"last_accessed"`
}

// HashRange defines the hash range for a partition
type HashRange struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// PartitionConfig represents partitioning configuration
type PartitionConfig struct {
	Strategy            string   `json:"strategy"`             // hash, range, list
	PartitionCount      int      `json:"partition_count"`
	MaxPartitionSize    int64    `json:"max_partition_size"`   // in bytes
	MaxRecordsPerPartition int64 `json:"max_records_per_partition"`
	RebalanceThreshold  float64  `json:"rebalance_threshold"`  // 0.8 = 80% capacity
	EnableAutoPartition bool     `json:"enable_auto_partition"`
	ShardNodes          []string `json:"shard_nodes"`
	ReplicationFactor   int      `json:"replication_factor"`
}

// PartitionMetrics holds metrics for partition operations
type PartitionMetrics struct {
	PartitionSize     *prometheus.GaugeVec
	PartitionRecords  *prometheus.GaugeVec
	PartitionAccess   *prometheus.CounterVec
	RebalanceOperations *prometheus.CounterVec
	ShardDistribution *prometheus.GaugeVec
	PartitionHealth   *prometheus.GaugeVec
}

// PartitionRebalancer handles automatic partition rebalancing
type PartitionRebalancer struct {
	manager     *PartitionManager
	logger      *zap.Logger
	stopCh      chan struct{}
	interval    time.Duration
	mutex       sync.Mutex
	isRunning   bool
}

// NewPartitionManager creates a new partition manager
func NewPartitionManager(db *gorm.DB, logger *zap.Logger) *PartitionManager {
	config := &PartitionConfig{
		Strategy:               "hash",
		PartitionCount:         16,
		MaxPartitionSize:       10 * 1024 * 1024 * 1024, // 10GB
		MaxRecordsPerPartition: 10000000,                 // 10M records
		RebalanceThreshold:     0.8,
		EnableAutoPartition:    true,
		ReplicationFactor:      3,
	}

	metrics := &PartitionMetrics{
		PartitionSize: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "accounts_partition_size_bytes",
			Help: "Size of each partition in bytes",
		}, []string{"partition", "shard"}),
		PartitionRecords: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "accounts_partition_records",
			Help: "Number of records in each partition",
		}, []string{"partition", "shard"}),
		PartitionAccess: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "accounts_partition_access_total",
			Help: "Total number of partition accesses",
		}, []string{"partition", "operation"}),
		RebalanceOperations: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "accounts_partition_rebalance_total",
			Help: "Total number of partition rebalance operations",
		}, []string{"operation", "status"}),
		ShardDistribution: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "accounts_shard_distribution",
			Help: "Distribution of data across shards",
		}, []string{"shard", "metric"}),
		PartitionHealth: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "accounts_partition_health",
			Help: "Health status of partitions (1=healthy, 0=unhealthy)",
		}, []string{"partition", "shard"}),
	}

	pm := &PartitionManager{
		db:         db,
		logger:     logger,
		partitions: make(map[string]*PartitionInfo),
		shardMap:   make(map[string]string),
		metrics:    metrics,
		config:     config,
	}

	// Initialize partitions
	pm.initializePartitions()

	// Create rebalancer
	pm.rebalancer = &PartitionRebalancer{
		manager:   pm,
		logger:    logger,
		interval:  time.Hour, // Check every hour
		stopCh:    make(chan struct{}),
		isRunning: false,
	}

	return pm
}

// GetAccountPartition returns the partition name for a given user ID
func (pm *PartitionManager) GetAccountPartition(userID uuid.UUID) string {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()

	hashKey := pm.hashUserID(userID)
	
	if partition, exists := pm.shardMap[hashKey]; exists {
		pm.metrics.PartitionAccess.WithLabelValues(partition, "get_account").Inc()
		return partition
	}

	// Default to first partition if not found
	partition := pm.getDefaultPartition()
	pm.metrics.PartitionAccess.WithLabelValues(partition, "get_account").Inc()
	return partition
}

// GetTransactionPartition returns the partition for transaction data
func (pm *PartitionManager) GetTransactionPartition(userID uuid.UUID, timestamp time.Time) string {
	// For transactions, we can use time-based partitioning
	// Format: YYYY_MM for monthly partitions
	partition := fmt.Sprintf("transactions_%s", timestamp.Format("2006_01"))
	
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()
	
	pm.metrics.PartitionAccess.WithLabelValues(partition, "get_transaction").Inc()
	return partition
}

// CreatePartition creates a new partition
func (pm *PartitionManager) CreatePartition(ctx context.Context, tableName, partitionName string, hashRange HashRange) error {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	// Create partition table
	partitionTableName := fmt.Sprintf("%s_%s", tableName, partitionName)
	
	switch tableName {
	case "accounts":
		if err := pm.createAccountPartition(ctx, partitionTableName, hashRange); err != nil {
			return fmt.Errorf("failed to create account partition: %w", err)
		}
	case "reservations":
		if err := pm.createReservationPartition(ctx, partitionTableName, hashRange); err != nil {
			return fmt.Errorf("failed to create reservation partition: %w", err)
		}
	case "transaction_journal":
		if err := pm.createTransactionPartition(ctx, partitionTableName); err != nil {
			return fmt.Errorf("failed to create transaction partition: %w", err)
		}
	default:
		return fmt.Errorf("unsupported table for partitioning: %s", tableName)
	}

	// Update partition metadata
	partition := &PartitionInfo{
		Name:         partitionName,
		ShardID:      pm.getShardForPartition(partitionName),
		HashRange:    hashRange,
		Status:       "active",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		RecordCount:  0,
		SizeBytes:    0,
		LastAccessed: time.Now(),
	}

	pm.partitions[partitionName] = partition
	pm.updateShardMap()

	pm.logger.Info("Created new partition",
		zap.String("table", tableName),
		zap.String("partition", partitionName),
		zap.String("shard", partition.ShardID))

	return nil
}

// MigratePartition migrates data from one partition to another
func (pm *PartitionManager) MigratePartition(ctx context.Context, sourcePartition, targetPartition string) error {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	source, exists := pm.partitions[sourcePartition]
	if !exists {
		return fmt.Errorf("source partition not found: %s", sourcePartition)
	}

	target, exists := pm.partitions[targetPartition]
	if !exists {
		return fmt.Errorf("target partition not found: %s", targetPartition)
	}

	// Set source to readonly mode
	source.Status = "readonly"
	pm.partitions[sourcePartition] = source

	// Set target to migrating mode
	target.Status = "migrating"
	pm.partitions[targetPartition] = target

	pm.logger.Info("Starting partition migration",
		zap.String("source", sourcePartition),
		zap.String("target", targetPartition))

	// Perform migration in batches
	batchSize := 1000
	offset := 0

	for {
		// Migrate accounts
		if err := pm.migrateAccountBatch(ctx, sourcePartition, targetPartition, batchSize, offset); err != nil {
			pm.logger.Error("Failed to migrate account batch", zap.Error(err))
			return err
		}

		// Check if there are more records to migrate
		var count int64
		pm.db.Table(fmt.Sprintf("accounts_%s", sourcePartition)).
			Offset(offset + batchSize).
			Count(&count)

		if count == 0 {
			break
		}

		offset += batchSize
		
		// Add delay to avoid overwhelming the system
		time.Sleep(100 * time.Millisecond)
	}

	// Update partition status
	source.Status = "inactive"
	target.Status = "active"
	pm.partitions[sourcePartition] = source
	pm.partitions[targetPartition] = target

	pm.updateShardMap()

	pm.logger.Info("Completed partition migration",
		zap.String("source", sourcePartition),
		zap.String("target", targetPartition))

	pm.metrics.RebalanceOperations.WithLabelValues("migrate", "success").Inc()
	return nil
}

// StartRebalancer starts the automatic partition rebalancer
func (pm *PartitionManager) StartRebalancer() {
	pm.rebalancer.Start()
}

// StopRebalancer stops the automatic partition rebalancer
func (pm *PartitionManager) StopRebalancer() {
	pm.rebalancer.Stop()
}

// GetPartitionStats returns statistics for all partitions
func (pm *PartitionManager) GetPartitionStats(ctx context.Context) (map[string]*PartitionInfo, error) {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()

	// Update partition statistics
	for name, partition := range pm.partitions {
		if err := pm.updatePartitionStats(ctx, name, partition); err != nil {
			pm.logger.Error("Failed to update partition stats", 
				zap.String("partition", name), zap.Error(err))
		}
	}

	// Create a copy to return
	stats := make(map[string]*PartitionInfo)
	for name, partition := range pm.partitions {
		stats[name] = &PartitionInfo{
			Name:         partition.Name,
			ShardID:      partition.ShardID,
			HashRange:    partition.HashRange,
			Status:       partition.Status,
			CreatedAt:    partition.CreatedAt,
			UpdatedAt:    partition.UpdatedAt,
			RecordCount:  partition.RecordCount,
			SizeBytes:    partition.SizeBytes,
			LastAccessed: partition.LastAccessed,
		}
	}

	return stats, nil
}

// Helper methods

func (pm *PartitionManager) initializePartitions() {
	// Initialize default partitions based on configuration
	partitionCount := pm.config.PartitionCount
	hashSpace := "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	
	for i := 0; i < partitionCount; i++ {
		partitionName := fmt.Sprintf("p%02d", i)
		
		// Calculate hash range for this partition
		startHash := pm.calculatePartitionHash(i, partitionCount)
		endHash := pm.calculatePartitionHash(i+1, partitionCount)
		
		hashRange := HashRange{
			Start: startHash,
			End:   endHash,
		}
		
		partition := &PartitionInfo{
			Name:         partitionName,
			ShardID:      pm.getShardForPartition(partitionName),
			HashRange:    hashRange,
			Status:       "active",
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
			RecordCount:  0,
			SizeBytes:    0,
			LastAccessed: time.Now(),
		}
		
		pm.partitions[partitionName] = partition
	}
	
	pm.updateShardMap()
	
	pm.logger.Info("Initialized partitions", zap.Int("count", partitionCount))
}

func (pm *PartitionManager) hashUserID(userID uuid.UUID) string {
	hash := sha256.Sum256([]byte(userID.String()))
	return hex.EncodeToString(hash[:])
}

func (pm *PartitionManager) getDefaultPartition() string {
	// Return the first available active partition
	for name, partition := range pm.partitions {
		if partition.Status == "active" {
			return name
		}
	}
	return "p00" // Fallback
}

func (pm *PartitionManager) calculatePartitionHash(index, total int) string {
	// Calculate hash range for uniform distribution
	maxHash := "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	if index == 0 {
		return "0000000000000000000000000000000000000000000000000000000000000000"
	}
	if index >= total {
		return maxHash
	}
	
	// Simple calculation - in production, use more sophisticated distribution
	portion := float64(index) / float64(total)
	hashInt := uint64(portion * float64(^uint64(0)))
	return fmt.Sprintf("%064x", hashInt)
}

func (pm *PartitionManager) getShardForPartition(partitionName string) string {
	// Simple round-robin assignment to shards
	shardCount := len(pm.config.ShardNodes)
	if shardCount == 0 {
		return "shard_01" // Default shard
	}
	
	// Extract partition number and map to shard
	partitionNum := 0
	if len(partitionName) > 1 && partitionName[0] == 'p' {
		if num, err := strconv.Atoi(partitionName[1:]); err == nil {
			partitionNum = num
		}
	}
	
	shardIndex := partitionNum % shardCount
	return fmt.Sprintf("shard_%02d", shardIndex+1)
}

func (pm *PartitionManager) updateShardMap() {
	// Rebuild shard map based on current partitions
	pm.shardMap = make(map[string]string)
	
	for name, partition := range pm.partitions {
		if partition.Status != "active" {
			continue
		}
		
		// Map hash range to partition
		startHash := partition.HashRange.Start
		endHash := partition.HashRange.End
		
		// For simplicity, map the entire range to this partition
		// In production, you'd have a more sophisticated mapping
		pm.shardMap[name] = name
	}
}

func (pm *PartitionManager) createAccountPartition(ctx context.Context, tableName string, hashRange HashRange) error {
	// Create accounts partition table
	createSQL := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			LIKE accounts INCLUDING ALL
		) PARTITION OF accounts FOR VALUES WITH (MODULUS %d, REMAINDER %d);
		
		CREATE INDEX IF NOT EXISTS idx_%s_user_currency ON %s (user_id, currency);
		CREATE INDEX IF NOT EXISTS idx_%s_user_partition ON %s (user_id);
		CREATE INDEX IF NOT EXISTS idx_%s_currency ON %s (currency);
		CREATE INDEX IF NOT EXISTS idx_%s_created ON %s (created_at);
		CREATE INDEX IF NOT EXISTS idx_%s_updated ON %s (updated_at);
	`, tableName, pm.config.PartitionCount, pm.getPartitionRemainder(tableName),
		tableName, tableName,
		tableName, tableName,
		tableName, tableName,
		tableName, tableName,
		tableName, tableName)

	return pm.db.Exec(createSQL).Error
}

func (pm *PartitionManager) createReservationPartition(ctx context.Context, tableName string, hashRange HashRange) error {
	// Create reservations partition table
	createSQL := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			LIKE reservations INCLUDING ALL
		);
		
		CREATE INDEX IF NOT EXISTS idx_%s_user_currency ON %s (user_id, currency);
		CREATE INDEX IF NOT EXISTS idx_%s_reference ON %s (reference_id);
		CREATE INDEX IF NOT EXISTS idx_%s_status ON %s (status);
		CREATE INDEX IF NOT EXISTS idx_%s_expires ON %s (expires_at);
	`, tableName,
		tableName, tableName,
		tableName, tableName,
		tableName, tableName,
		tableName, tableName)

	return pm.db.Exec(createSQL).Error
}

func (pm *PartitionManager) createTransactionPartition(ctx context.Context, tableName string) error {
	// Create transaction journal partition table
	createSQL := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			LIKE transaction_journal INCLUDING ALL
		);
		
		CREATE INDEX IF NOT EXISTS idx_%s_user_id ON %s (user_id);
		CREATE INDEX IF NOT EXISTS idx_%s_currency ON %s (currency);
		CREATE INDEX IF NOT EXISTS idx_%s_reference ON %s (reference_id);
		CREATE INDEX IF NOT EXISTS idx_%s_created ON %s (created_at);
	`, tableName,
		tableName, tableName,
		tableName, tableName,
		tableName, tableName,
		tableName, tableName)

	return pm.db.Exec(createSQL).Error
}

func (pm *PartitionManager) getPartitionRemainder(tableName string) int {
	// Extract partition number from table name for modulus calculation
	parts := strings.Split(tableName, "_")
	if len(parts) > 1 {
		if num, err := strconv.Atoi(strings.TrimPrefix(parts[len(parts)-1], "p")); err == nil {
			return num
		}
	}
	return 0
}

func (pm *PartitionManager) migrateAccountBatch(ctx context.Context, source, target string, batchSize, offset int) error {
	sourceTable := fmt.Sprintf("accounts_%s", source)
	targetTable := fmt.Sprintf("accounts_%s", target)
	
	// Copy data in batch
	copySQL := fmt.Sprintf(`
		INSERT INTO %s 
		SELECT * FROM %s 
		LIMIT %d OFFSET %d
	`, targetTable, sourceTable, batchSize, offset)
	
	return pm.db.Exec(copySQL).Error
}

func (pm *PartitionManager) updatePartitionStats(ctx context.Context, name string, partition *PartitionInfo) error {
	// Update record count
	var recordCount int64
	accountTable := fmt.Sprintf("accounts_%s", name)
	
	if err := pm.db.Table(accountTable).Count(&recordCount).Error; err == nil {
		partition.RecordCount = recordCount
		pm.metrics.PartitionRecords.WithLabelValues(name, partition.ShardID).Set(float64(recordCount))
	}
	
	// Update size (simplified - in production, query actual table size)
	partition.SizeBytes = recordCount * 500 // Estimated 500 bytes per record
	pm.metrics.PartitionSize.WithLabelValues(name, partition.ShardID).Set(float64(partition.SizeBytes))
	
	// Update health status
	healthStatus := 1.0
	if partition.Status != "active" {
		healthStatus = 0.0
	}
	pm.metrics.PartitionHealth.WithLabelValues(name, partition.ShardID).Set(healthStatus)
	
	partition.UpdatedAt = time.Now()
	return nil
}

// Rebalancer methods

func (r *PartitionRebalancer) Start() {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.isRunning {
		return
	}

	r.isRunning = true
	go r.run()
	r.logger.Info("Started partition rebalancer")
}

func (r *PartitionRebalancer) Stop() {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if !r.isRunning {
		return
	}

	r.isRunning = false
	close(r.stopCh)
	r.logger.Info("Stopped partition rebalancer")
}

func (r *PartitionRebalancer) run() {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			r.checkAndRebalance()
		case <-r.stopCh:
			return
		}
	}
}

func (r *PartitionRebalancer) checkAndRebalance() {
	ctx := context.Background()
	stats, err := r.manager.GetPartitionStats(ctx)
	if err != nil {
		r.logger.Error("Failed to get partition stats", zap.Error(err))
		return
	}

	// Check if any partition needs rebalancing
	for name, partition := range stats {
		if r.needsRebalancing(partition) {
			r.logger.Info("Partition needs rebalancing",
				zap.String("partition", name),
				zap.Int64("records", partition.RecordCount),
				zap.Int64("size", partition.SizeBytes))

			// Trigger rebalancing (simplified)
			r.manager.metrics.RebalanceOperations.WithLabelValues("check", "triggered").Inc()
		}
	}
}

func (r *PartitionRebalancer) needsRebalancing(partition *PartitionInfo) bool {
	config := r.manager.config
	
	// Check size threshold
	if partition.SizeBytes > int64(float64(config.MaxPartitionSize)*config.RebalanceThreshold) {
		return true
	}
	
	// Check record count threshold
	if partition.RecordCount > int64(float64(config.MaxRecordsPerPartition)*config.RebalanceThreshold) {
		return true
	}
	
	return false
}
