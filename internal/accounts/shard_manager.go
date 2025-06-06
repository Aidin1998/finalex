// ShardManager provides automatic sharding and rebalancing for horizontal scaling
package accounts

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
)

// ShardManager handles automatic sharding and rebalancing
type ShardManager struct {
	// Core configuration
	shardCount int
	shards     map[string]*ShardInfo
	shardRing  *ConsistentHashRing
	logger     *zap.Logger

	// Rebalancing
	rebalancer      *AutoRebalancer
	rebalanceConfig *RebalanceConfig

	// Monitoring
	metrics *ShardMetrics

	// Synchronization
	mu sync.RWMutex

	// Background tasks
	monitorTicker *time.Ticker
	stopMonitor   chan struct{}
}

// ShardInfo contains metadata about a shard
type ShardInfo struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Status       string        `json:"status"` // active, readonly, migrating, inactive
	HashRanges   []HashRange   `json:"hash_ranges"`
	Weight       int           `json:"weight"`
	LoadFactor   float64       `json:"load_factor"`
	RecordCount  int64         `json:"record_count"`
	SizeBytes    int64         `json:"size_bytes"`
	QPS          float64       `json:"qps"`
	AvgLatency   time.Duration `json:"avg_latency"`
	LastAccessed time.Time     `json:"last_accessed"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
}

// ConsistentHashRing implements consistent hashing for shard distribution
type ConsistentHashRing struct {
	ring         map[uint32]string // hash -> shard_id
	sortedHashes []uint32
	virtualNodes int
	mu           sync.RWMutex
}

// AutoRebalancer handles automatic shard rebalancing
type AutoRebalancer struct {
	manager *ShardManager
	config  *RebalanceConfig
	logger  *zap.Logger

	// State tracking
	isRebalancing bool
	currentPlan   *RebalancePlan
	mu            sync.RWMutex

	// Background processing
	ticker   *time.Ticker
	stopChan chan struct{}
}

// RebalanceConfig represents rebalancing configuration
type RebalanceConfig struct {
	// Thresholds
	LoadImbalanceThreshold float64 `json:"load_imbalance_threshold"` // 0.2 = 20% imbalance triggers rebalance
	MinShardSizeBytes      int64   `json:"min_shard_size_bytes"`
	MaxShardSizeBytes      int64   `json:"max_shard_size_bytes"`

	// Timing
	CheckInterval    time.Duration `json:"check_interval"`
	CooldownPeriod   time.Duration `json:"cooldown_period"`
	MaxRebalanceTime time.Duration `json:"max_rebalance_time"`

	// Safety
	MaxConcurrentMigrations int     `json:"max_concurrent_migrations"`
	MaxDataMovementPercent  float64 `json:"max_data_movement_percent"` // Max % of data to move in one rebalance

	// Performance
	BatchSize          int `json:"batch_size"`
	MigrationRateLimit int `json:"migration_rate_limit"` // Records per second

	// Feature flags
	AutoRebalanceEnabled bool `json:"auto_rebalance_enabled"`
	EmergencyStopEnabled bool `json:"emergency_stop_enabled"`
}

// RebalancePlan represents a rebalancing plan
type RebalancePlan struct {
	ID                uuid.UUID         `json:"id"`
	CreatedAt         time.Time         `json:"created_at"`
	Status            string            `json:"status"` // planned, executing, completed, failed, cancelled
	Migrations        []*ShardMigration `json:"migrations"`
	TotalRecords      int64             `json:"total_records"`
	EstimatedDuration time.Duration     `json:"estimated_duration"`
	ActualDuration    time.Duration     `json:"actual_duration"`
	Progress          float64           `json:"progress"` // 0.0 to 1.0
}

// ShardMigration represents a single shard migration operation
type ShardMigration struct {
	ID          uuid.UUID  `json:"id"`
	FromShard   string     `json:"from_shard"`
	ToShard     string     `json:"to_shard"`
	HashRange   HashRange  `json:"hash_range"`
	RecordCount int64      `json:"record_count"`
	Status      string     `json:"status"` // pending, running, completed, failed
	StartedAt   *time.Time `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at"`
	Progress    float64    `json:"progress"`
	Error       string     `json:"error"`
}

// ShardMetrics holds Prometheus metrics for sharding
type ShardMetrics struct {
	ShardCount          prometheus.Gauge
	ShardLoadBalance    *prometheus.GaugeVec
	ShardSize           *prometheus.GaugeVec
	ShardQPS            *prometheus.GaugeVec
	ShardLatency        *prometheus.GaugeVec
	RebalanceOperations *prometheus.CounterVec
	MigrationDuration   *prometheus.HistogramVec
	MigrationProgress   *prometheus.GaugeVec
}

// NewShardManager creates a new shard manager
func NewShardManager(shardCount int, logger *zap.Logger) (*ShardManager, error) {
	if shardCount <= 0 {
		return nil, fmt.Errorf("shard count must be positive, got %d", shardCount)
	}

	metrics := &ShardMetrics{
		ShardCount: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "account_shard_count",
				Help: "Total number of shards",
			},
		),
		ShardLoadBalance: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "account_shard_load_balance",
				Help: "Load balance factor for each shard",
			},
			[]string{"shard_id"},
		),
		ShardSize: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "account_shard_size_bytes",
				Help: "Size of each shard in bytes",
			},
			[]string{"shard_id"},
		),
		ShardQPS: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "account_shard_qps",
				Help: "Queries per second for each shard",
			},
			[]string{"shard_id"},
		),
		ShardLatency: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "account_shard_latency_seconds",
				Help: "Average latency for each shard",
			},
			[]string{"shard_id"},
		),
		RebalanceOperations: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "account_rebalance_operations_total",
				Help: "Total number of rebalance operations",
			},
			[]string{"operation", "status"},
		),
		MigrationDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "account_migration_duration_seconds",
				Help:    "Duration of shard migrations",
				Buckets: prometheus.ExponentialBuckets(1, 2, 15), // 1s to ~9h
			},
			[]string{"from_shard", "to_shard"},
		),
		MigrationProgress: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "account_migration_progress",
				Help: "Progress of ongoing migrations (0-1)",
			},
			[]string{"migration_id"},
		),
	}

	// Initialize consistent hash ring
	hashRing := &ConsistentHashRing{
		ring:         make(map[uint32]string),
		virtualNodes: 100, // Number of virtual nodes per shard
	}

	// Initialize shards
	shards := make(map[string]*ShardInfo)
	for i := 0; i < shardCount; i++ {
		shardID := fmt.Sprintf("shard_%03d", i)
		shard := &ShardInfo{
			ID:        shardID,
			Name:      fmt.Sprintf("Account Shard %d", i),
			Status:    "active",
			Weight:    100, // Default weight
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		shards[shardID] = shard

		// Add to hash ring
		hashRing.AddShard(shardID, shard.Weight)
	}

	// Default rebalance configuration
	rebalanceConfig := &RebalanceConfig{
		LoadImbalanceThreshold:  0.2,                     // 20% imbalance
		MinShardSizeBytes:       1024 * 1024 * 100,       // 100MB
		MaxShardSizeBytes:       1024 * 1024 * 1024 * 10, // 10GB
		CheckInterval:           5 * time.Minute,
		CooldownPeriod:          30 * time.Minute,
		MaxRebalanceTime:        2 * time.Hour,
		MaxConcurrentMigrations: 2,
		MaxDataMovementPercent:  0.1, // 10% max movement
		BatchSize:               1000,
		MigrationRateLimit:      10000, // 10k records/sec
		AutoRebalanceEnabled:    false, // Disabled by default
		EmergencyStopEnabled:    true,
	}

	sm := &ShardManager{
		shardCount:      shardCount,
		shards:          shards,
		shardRing:       hashRing,
		logger:          logger,
		rebalanceConfig: rebalanceConfig,
		metrics:         metrics,
		stopMonitor:     make(chan struct{}),
	}

	// Initialize auto rebalancer
	sm.rebalancer = &AutoRebalancer{
		manager:  sm,
		config:   rebalanceConfig,
		logger:   logger,
		stopChan: make(chan struct{}),
	}

	// Update metrics
	sm.metrics.ShardCount.Set(float64(shardCount))

	return sm, nil
}

// GetShardForKey determines which shard a key belongs to
func (sm *ShardManager) GetShardForKey(key string) string {
	return sm.shardRing.GetShard(key)
}

// GetShardForUser determines which shard a user belongs to
func (sm *ShardManager) GetShardForUser(userID uuid.UUID) string {
	return sm.GetShardForKey(userID.String())
}

// GetShardInfo returns information about a specific shard
func (sm *ShardManager) GetShardInfo(shardID string) (*ShardInfo, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	shard, exists := sm.shards[shardID]
	if !exists {
		return nil, fmt.Errorf("shard %s not found", shardID)
	}

	// Return a copy to prevent external modification
	shardCopy := *shard
	return &shardCopy, nil
}

// GetAllShards returns information about all shards
func (sm *ShardManager) GetAllShards() map[string]*ShardInfo {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	result := make(map[string]*ShardInfo)
	for id, shard := range sm.shards {
		shardCopy := *shard
		result[id] = &shardCopy
	}
	return result
}

// UpdateShardStats updates statistics for a shard
func (sm *ShardManager) UpdateShardStats(shardID string, recordCount int64, sizeBytes int64, qps float64, avgLatency time.Duration) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	shard, exists := sm.shards[shardID]
	if !exists {
		return fmt.Errorf("shard %s not found", shardID)
	}

	shard.RecordCount = recordCount
	shard.SizeBytes = sizeBytes
	shard.QPS = qps
	shard.AvgLatency = avgLatency
	shard.LastAccessed = time.Now()
	shard.UpdatedAt = time.Now()

	// Calculate load factor (combination of size and QPS)
	avgSize := sm.calculateAverageShardSize()
	avgQPS := sm.calculateAverageShardQPS()

	sizeRatio := float64(sizeBytes) / avgSize
	qpsRatio := qps / avgQPS
	shard.LoadFactor = (sizeRatio + qpsRatio) / 2.0

	// Update metrics
	sm.metrics.ShardLoadBalance.WithLabelValues(shardID).Set(shard.LoadFactor)
	sm.metrics.ShardSize.WithLabelValues(shardID).Set(float64(sizeBytes))
	sm.metrics.ShardQPS.WithLabelValues(shardID).Set(qps)
	sm.metrics.ShardLatency.WithLabelValues(shardID).Set(avgLatency.Seconds())

	return nil
}

// StartAutoRebalance starts automatic rebalancing
func (sm *ShardManager) StartAutoRebalance(ctx context.Context) error {
	if !sm.rebalanceConfig.AutoRebalanceEnabled {
		return fmt.Errorf("auto rebalance is not enabled")
	}

	sm.rebalancer.ticker = time.NewTicker(sm.rebalanceConfig.CheckInterval)
	go sm.rebalancer.run(ctx)

	// Start monitoring
	sm.monitorTicker = time.NewTicker(30 * time.Second)
	go sm.monitorShards(ctx)

	sm.logger.Info("Auto rebalancing started",
		zap.Duration("check_interval", sm.rebalanceConfig.CheckInterval))
	return nil
}

// Stop stops the shard manager
func (sm *ShardManager) Stop(ctx context.Context) error {
	// Stop monitoring
	if sm.monitorTicker != nil {
		sm.monitorTicker.Stop()
	}
	close(sm.stopMonitor)

	// Stop rebalancer
	sm.rebalancer.stop()

	sm.logger.Info("Shard manager stopped")
	return nil
}

// CreateRebalancePlan creates a new rebalancing plan
func (sm *ShardManager) CreateRebalancePlan() (*RebalancePlan, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	// Check if rebalancing is needed
	if !sm.needsRebalancing() {
		return nil, fmt.Errorf("rebalancing not needed")
	}

	// Find shards that need rebalancing
	overloadedShards := sm.findOverloadedShards()
	underloadedShards := sm.findUnderloadedShards()

	if len(overloadedShards) == 0 || len(underloadedShards) == 0 {
		return nil, fmt.Errorf("no valid rebalancing candidates found")
	}

	// Create migration plan
	migrations := sm.createMigrations(overloadedShards, underloadedShards)
	if len(migrations) == 0 {
		return nil, fmt.Errorf("no valid migrations could be created")
	}

	plan := &RebalancePlan{
		ID:                uuid.New(),
		CreatedAt:         time.Now(),
		Status:            "planned",
		Migrations:        migrations,
		EstimatedDuration: sm.estimateMigrationDuration(migrations),
	}

	// Calculate total records
	for _, migration := range migrations {
		plan.TotalRecords += migration.RecordCount
	}

	return plan, nil
}

// ExecuteRebalancePlan executes a rebalancing plan
func (sm *ShardManager) ExecuteRebalancePlan(ctx context.Context, plan *RebalancePlan) error {
	sm.rebalancer.mu.Lock()
	defer sm.rebalancer.mu.Unlock()

	if sm.rebalancer.isRebalancing {
		return fmt.Errorf("rebalancing already in progress")
	}

	sm.rebalancer.isRebalancing = true
	sm.rebalancer.currentPlan = plan
	defer func() {
		sm.rebalancer.isRebalancing = false
		sm.rebalancer.currentPlan = nil
	}()

	plan.Status = "executing"
	sm.metrics.RebalanceOperations.WithLabelValues("execute", "started").Inc()

	startTime := time.Now()

	// Execute migrations
	for i, migration := range plan.Migrations {
		select {
		case <-ctx.Done():
			plan.Status = "cancelled"
			return ctx.Err()
		default:
		}

		if err := sm.executeMigration(ctx, migration); err != nil {
			plan.Status = "failed"
			sm.metrics.RebalanceOperations.WithLabelValues("execute", "failed").Inc()
			return fmt.Errorf("migration %s failed: %w", migration.ID, err)
		}

		// Update progress
		plan.Progress = float64(i+1) / float64(len(plan.Migrations))
	}

	plan.Status = "completed"
	plan.ActualDuration = time.Since(startTime)
	plan.Progress = 1.0

	sm.metrics.RebalanceOperations.WithLabelValues("execute", "completed").Inc()
	sm.logger.Info("Rebalance plan completed",
		zap.String("plan_id", plan.ID.String()),
		zap.Duration("duration", plan.ActualDuration),
		zap.Int("migrations", len(plan.Migrations)))

	return nil
}

// AddShard adds a new shard to the hash ring
func (sm *ShardManager) AddShard(shardID string, weight int) {
	sm.shardRing.AddShard(shardID, weight)
}

// RemoveShard removes a shard from the hash ring
func (sm *ShardManager) RemoveShard(shardID string) {
	sm.shardRing.RemoveShard(shardID)
}

// ConsistentHashRing methods

// AddShard adds a shard to the hash ring
func (chr *ConsistentHashRing) AddShard(shardID string, weight int) {
	chr.mu.Lock()
	defer chr.mu.Unlock()

	// Add virtual nodes
	for i := 0; i < chr.virtualNodes*weight; i++ {
		hash := chr.hashKey(fmt.Sprintf("%s:%d", shardID, i))
		chr.ring[hash] = shardID
	}

	chr.updateSortedHashes()
}

// RemoveShard removes a shard from the hash ring
func (chr *ConsistentHashRing) RemoveShard(shardID string) {
	chr.mu.Lock()
	defer chr.mu.Unlock()

	// Remove all virtual nodes for this shard
	for hash, id := range chr.ring {
		if id == shardID {
			delete(chr.ring, hash)
		}
	}

	chr.updateSortedHashes()
}

// GetShard returns the shard for a given key
func (chr *ConsistentHashRing) GetShard(key string) string {
	chr.mu.RLock()
	defer chr.mu.RUnlock()

	if len(chr.ring) == 0 {
		return ""
	}

	hash := chr.hashKey(key)

	// Find the first hash greater than or equal to the key hash
	idx := sort.Search(len(chr.sortedHashes), func(i int) bool {
		return chr.sortedHashes[i] >= hash
	})

	// Wrap around if necessary
	if idx == len(chr.sortedHashes) {
		idx = 0
	}

	return chr.ring[chr.sortedHashes[idx]]
}

// hashKey creates a hash for a key
func (chr *ConsistentHashRing) hashKey(key string) uint32 {
	hash := sha256.Sum256([]byte(key))
	return uint32(hash[0])<<24 | uint32(hash[1])<<16 | uint32(hash[2])<<8 | uint32(hash[3])
}

// updateSortedHashes updates the sorted hash list
func (chr *ConsistentHashRing) updateSortedHashes() {
	chr.sortedHashes = make([]uint32, 0, len(chr.ring))
	for hash := range chr.ring {
		chr.sortedHashes = append(chr.sortedHashes, hash)
	}
	sort.Slice(chr.sortedHashes, func(i, j int) bool {
		return chr.sortedHashes[i] < chr.sortedHashes[j]
	})
}

// Helper methods for rebalancing logic

func (sm *ShardManager) needsRebalancing() bool {
	loadFactors := make([]float64, 0, len(sm.shards))
	for _, shard := range sm.shards {
		if shard.Status == "active" {
			loadFactors = append(loadFactors, shard.LoadFactor)
		}
	}

	if len(loadFactors) < 2 {
		return false
	}

	// Calculate standard deviation
	var sum, mean float64
	for _, factor := range loadFactors {
		sum += factor
	}
	mean = sum / float64(len(loadFactors))

	var variance float64
	for _, factor := range loadFactors {
		variance += (factor - mean) * (factor - mean)
	}
	stdDev := variance / float64(len(loadFactors))

	return stdDev > sm.rebalanceConfig.LoadImbalanceThreshold
}

func (sm *ShardManager) findOverloadedShards() []*ShardInfo {
	var overloaded []*ShardInfo
	avgLoad := sm.calculateAverageLoadFactor()

	for _, shard := range sm.shards {
		if shard.Status == "active" && shard.LoadFactor > avgLoad*(1+sm.rebalanceConfig.LoadImbalanceThreshold) {
			overloaded = append(overloaded, shard)
		}
	}

	return overloaded
}

func (sm *ShardManager) findUnderloadedShards() []*ShardInfo {
	var underloaded []*ShardInfo
	avgLoad := sm.calculateAverageLoadFactor()

	for _, shard := range sm.shards {
		if shard.Status == "active" && shard.LoadFactor < avgLoad*(1-sm.rebalanceConfig.LoadImbalanceThreshold) {
			underloaded = append(underloaded, shard)
		}
	}

	return underloaded
}

func (sm *ShardManager) createMigrations(overloaded, underloaded []*ShardInfo) []*ShardMigration {
	var migrations []*ShardMigration

	// Simple algorithm: move data from most overloaded to least loaded
	for _, fromShard := range overloaded {
		for _, toShard := range underloaded {
			// Calculate how much data to move
			excessLoad := fromShard.LoadFactor - sm.calculateAverageLoadFactor()
			if excessLoad <= 0 {
				continue
			}

			// Estimate records to move (simplified)
			recordsToMove := int64(float64(fromShard.RecordCount) * excessLoad * 0.1) // Move 10% of excess
			if recordsToMove > 0 {
				migration := &ShardMigration{
					ID:          uuid.New(),
					FromShard:   fromShard.ID,
					ToShard:     toShard.ID,
					RecordCount: recordsToMove,
					Status:      "pending",
				}
				migrations = append(migrations, migration)

				// Update shard load estimates
				fromShard.LoadFactor -= excessLoad * 0.1
				toShard.LoadFactor += excessLoad * 0.1

				break // Move to next overloaded shard
			}
		}
	}

	return migrations
}

func (sm *ShardManager) estimateMigrationDuration(migrations []*ShardMigration) time.Duration {
	var totalRecords int64
	for _, migration := range migrations {
		totalRecords += migration.RecordCount
	}

	// Estimate based on migration rate limit
	seconds := float64(totalRecords) / float64(sm.rebalanceConfig.MigrationRateLimit)
	return time.Duration(seconds) * time.Second
}

func (sm *ShardManager) executeMigration(ctx context.Context, migration *ShardMigration) error {
	migration.Status = "running"
	migration.StartedAt = &[]time.Time{time.Now()}[0]

	// Update metrics
	sm.metrics.MigrationProgress.WithLabelValues(migration.ID.String()).Set(0)

	// Simulate migration progress (in real implementation, this would move actual data)
	totalSteps := 100
	for step := 0; step < totalSteps; step++ {
		select {
		case <-ctx.Done():
			migration.Status = "cancelled"
			return ctx.Err()
		default:
		}

		// Simulate work
		time.Sleep(10 * time.Millisecond)

		migration.Progress = float64(step+1) / float64(totalSteps)
		sm.metrics.MigrationProgress.WithLabelValues(migration.ID.String()).Set(migration.Progress)
	}

	migration.Status = "completed"
	migration.CompletedAt = &[]time.Time{time.Now()}[0]
	migration.Progress = 1.0

	// Record migration duration
	duration := migration.CompletedAt.Sub(*migration.StartedAt)
	sm.metrics.MigrationDuration.WithLabelValues(migration.FromShard, migration.ToShard).Observe(duration.Seconds())

	return nil
}

func (sm *ShardManager) calculateAverageShardSize() float64 {
	var total int64
	var count int

	for _, shard := range sm.shards {
		if shard.Status == "active" {
			total += shard.SizeBytes
			count++
		}
	}

	if count == 0 {
		return 0
	}
	return float64(total) / float64(count)
}

func (sm *ShardManager) calculateAverageShardQPS() float64 {
	var total float64
	var count int

	for _, shard := range sm.shards {
		if shard.Status == "active" {
			total += shard.QPS
			count++
		}
	}

	if count == 0 {
		return 0
	}
	return total / float64(count)
}

func (sm *ShardManager) calculateAverageLoadFactor() float64 {
	var total float64
	var count int

	for _, shard := range sm.shards {
		if shard.Status == "active" {
			total += shard.LoadFactor
			count++
		}
	}

	if count == 0 {
		return 1.0
	}
	return total / float64(count)
}

func (sm *ShardManager) monitorShards(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-sm.stopMonitor:
			return
		case <-sm.monitorTicker.C:
			sm.updateShardMetrics()
		}
	}
}

func (sm *ShardManager) updateShardMetrics() {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	for _, shard := range sm.shards {
		sm.metrics.ShardLoadBalance.WithLabelValues(shard.ID).Set(shard.LoadFactor)
		sm.metrics.ShardSize.WithLabelValues(shard.ID).Set(float64(shard.SizeBytes))
		sm.metrics.ShardQPS.WithLabelValues(shard.ID).Set(shard.QPS)
		sm.metrics.ShardLatency.WithLabelValues(shard.ID).Set(shard.AvgLatency.Seconds())
	}
}

// AutoRebalancer methods

func (ar *AutoRebalancer) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-ar.stopChan:
			return
		case <-ar.ticker.C:
			ar.checkAndRebalance(ctx)
		}
	}
}

func (ar *AutoRebalancer) stop() {
	if ar.ticker != nil {
		ar.ticker.Stop()
	}
	close(ar.stopChan)
}

func (ar *AutoRebalancer) checkAndRebalance(ctx context.Context) {
	ar.mu.RLock()
	if ar.isRebalancing {
		ar.mu.RUnlock()
		return
	}
	ar.mu.RUnlock()

	plan, err := ar.manager.CreateRebalancePlan()
	if err != nil {
		// No rebalancing needed or error
		return
	}

	ar.logger.Info("Auto rebalancing triggered",
		zap.String("plan_id", plan.ID.String()),
		zap.Int("migrations", len(plan.Migrations)))

	if err := ar.manager.ExecuteRebalancePlan(ctx, plan); err != nil {
		ar.logger.Error("Auto rebalancing failed",
			zap.String("plan_id", plan.ID.String()),
			zap.Error(err))
	}
}
