package database

import (
	"context"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// QueryRouter handles routing queries between master and replica databases
type QueryRouter struct {
	master        *gorm.DB
	replicas      []*ReplicaDB
	config        *QueryRouterConfig
	logger        *zap.Logger
	healthChecker *ReplicaHealthChecker
	metrics       *QueryRouterMetrics
	mu            sync.RWMutex
}

// ReplicaDB represents a read replica database connection
type ReplicaDB struct {
	db       *gorm.DB
	name     string
	weight   int
	healthy  int32 // atomic boolean
	latency  int64 // atomic int64 (nanoseconds)
	lastUsed int64 // atomic int64 (unix timestamp)
}

// QueryRouterConfig holds configuration for query routing
type QueryRouterConfig struct {
	DefaultReadReplica     bool          `yaml:"default_read_replica" json:"default_read_replica"`
	LoadBalancingStrategy  string        `yaml:"load_balancing_strategy" json:"load_balancing_strategy"` // round_robin, weighted, least_connections, latency_based
	Strategy               string        `yaml:"strategy" json:"strategy"`                               // Alias for LoadBalancingStrategy
	HealthCheckInterval    time.Duration `yaml:"health_check_interval" json:"health_check_interval"`
	HealthCheckTimeout     time.Duration `yaml:"health_check_timeout" json:"health_check_timeout"`
	MaxReplicaLatency      time.Duration `yaml:"max_replica_latency" json:"max_replica_latency"`
	MaxReplicationLag      time.Duration `yaml:"max_replication_lag" json:"max_replication_lag"`
	ReadPreference         string        `yaml:"read_preference" json:"read_preference"` // primary, secondary, secondaryPreferred
	ReplicaFailoverEnabled bool          `yaml:"replica_failover_enabled" json:"replica_failover_enabled"`
	ConsistencyLevel       string        `yaml:"consistency_level" json:"consistency_level"` // eventual, strong, bounded_staleness
	BoundedStalenessMaxAge time.Duration `yaml:"bounded_staleness_max_age" json:"bounded_staleness_max_age"`
	ForceReadFromMaster    []string      `yaml:"force_read_from_master" json:"force_read_from_master"` // Table names that should always read from master
}

// DefaultQueryRouterConfig returns default configuration
func DefaultQueryRouterConfig() *QueryRouterConfig {
	return &QueryRouterConfig{
		DefaultReadReplica:     true,
		LoadBalancingStrategy:  "latency_based",
		HealthCheckInterval:    30 * time.Second,
		HealthCheckTimeout:     5 * time.Second,
		MaxReplicaLatency:      100 * time.Millisecond,
		ReplicaFailoverEnabled: true,
		ConsistencyLevel:       "eventual",
		BoundedStalenessMaxAge: 5 * time.Second,
		ForceReadFromMaster:    []string{"orders", "trades"}, // Critical tables
	}
}

// QueryRouterMetrics holds metrics for query routing
type QueryRouterMetrics struct {
	MasterQueries     int64 `json:"master_queries"`
	ReplicaQueries    int64 `json:"replica_queries"`
	FailoverCount     int64 `json:"failover_count"`
	AvgReplicaLatency int64 `json:"avg_replica_latency_ns"`
}

// NewQueryRouter creates a new query router
func NewQueryRouter(
	master *gorm.DB,
	replicas []*ReplicaDB,
	config *QueryRouterConfig,
	logger *zap.Logger,
) *QueryRouter {
	if config == nil {
		config = DefaultQueryRouterConfig()
	}

	router := &QueryRouter{
		master:   master,
		replicas: replicas,
		config:   config,
		logger:   logger,
		metrics:  &QueryRouterMetrics{},
	}

	// Initialize health checker
	router.healthChecker = NewReplicaHealthChecker(replicas, config, logger)

	// Start health checking
	go router.startHealthChecking()

	return router
}

// AddReplica adds a new read replica
func (qr *QueryRouter) AddReplica(db *gorm.DB, name string, weight int) {
	qr.mu.Lock()
	defer qr.mu.Unlock()

	replica := &ReplicaDB{
		db:     db,
		name:   name,
		weight: weight,
	}
	atomic.StoreInt32(&replica.healthy, 1)

	qr.replicas = append(qr.replicas, replica)
	qr.healthChecker.AddReplica(replica)

	qr.logger.Info("Read replica added",
		zap.String("name", name),
		zap.Int("weight", weight))
}

// RemoveReplica removes a read replica
func (qr *QueryRouter) RemoveReplica(name string) {
	qr.mu.Lock()
	defer qr.mu.Unlock()

	for i, replica := range qr.replicas {
		if replica.name == name {
			qr.replicas = append(qr.replicas[:i], qr.replicas[i+1:]...)
			qr.healthChecker.RemoveReplica(name)
			qr.logger.Info("Read replica removed", zap.String("name", name))
			break
		}
	}
}

// RouteQuery routes a query to the appropriate database (master or replica)
func (qr *QueryRouter) RouteQuery(ctx context.Context, queryType QueryType, table string) *gorm.DB {
	// Always route writes to master
	if queryType == QueryTypeWrite {
		atomic.AddInt64(&qr.metrics.MasterQueries, 1)
		return qr.master
	}

	// Check if table should always read from master
	if qr.shouldReadFromMaster(table) {
		atomic.AddInt64(&qr.metrics.MasterQueries, 1)
		return qr.master
	}

	// Check consistency requirements
	if qr.requiresStrongConsistency(ctx) {
		atomic.AddInt64(&qr.metrics.MasterQueries, 1)
		return qr.master
	}

	// Route to replica if available and healthy
	replica := qr.selectReplica()
	if replica != nil {
		atomic.AddInt64(&qr.metrics.ReplicaQueries, 1)
		atomic.StoreInt64(&replica.lastUsed, time.Now().Unix())
		return replica.db
	}

	// Fallback to master
	atomic.AddInt64(&qr.metrics.MasterQueries, 1)
	atomic.AddInt64(&qr.metrics.FailoverCount, 1)
	qr.logger.Debug("No healthy replicas available, routing to master",
		zap.String("table", table))

	return qr.master
}

// selectReplica selects the best replica based on the load balancing strategy
func (qr *QueryRouter) selectReplica() *ReplicaDB {
	qr.mu.RLock()
	defer qr.mu.RUnlock()

	healthyReplicas := qr.getHealthyReplicas()
	if len(healthyReplicas) == 0 {
		return nil
	}

	switch qr.config.LoadBalancingStrategy {
	case "round_robin":
		return qr.selectRoundRobin(healthyReplicas)
	case "weighted":
		return qr.selectWeighted(healthyReplicas)
	case "least_connections":
		return qr.selectLeastConnections(healthyReplicas)
	case "latency_based":
		return qr.selectLatencyBased(healthyReplicas)
	default:
		return qr.selectLatencyBased(healthyReplicas)
	}
}

// getHealthyReplicas returns all healthy replicas
func (qr *QueryRouter) getHealthyReplicas() []*ReplicaDB {
	var healthy []*ReplicaDB
	for _, replica := range qr.replicas {
		if atomic.LoadInt32(&replica.healthy) == 1 {
			latency := atomic.LoadInt64(&replica.latency)
			if latency == 0 || time.Duration(latency) <= qr.config.MaxReplicaLatency {
				healthy = append(healthy, replica)
			}
		}
	}
	return healthy
}

// selectRoundRobin selects replica using round-robin strategy
func (qr *QueryRouter) selectRoundRobin(replicas []*ReplicaDB) *ReplicaDB {
	if len(replicas) == 0 {
		return nil
	}

	// Simple round-robin based on current time
	index := int(time.Now().UnixNano()) % len(replicas)
	return replicas[index]
}

// selectWeighted selects replica using weighted strategy
func (qr *QueryRouter) selectWeighted(replicas []*ReplicaDB) *ReplicaDB {
	if len(replicas) == 0 {
		return nil
	}

	totalWeight := 0
	for _, replica := range replicas {
		totalWeight += replica.weight
	}

	if totalWeight == 0 {
		return replicas[0]
	}

	target := rand.Intn(totalWeight)
	current := 0

	for _, replica := range replicas {
		current += replica.weight
		if current > target {
			return replica
		}
	}

	return replicas[len(replicas)-1]
}

// selectLeastConnections selects replica with least connections (approximated by last used time)
func (qr *QueryRouter) selectLeastConnections(replicas []*ReplicaDB) *ReplicaDB {
	if len(replicas) == 0 {
		return nil
	}

	var best *ReplicaDB
	oldestUsage := int64(0)

	for _, replica := range replicas {
		lastUsed := atomic.LoadInt64(&replica.lastUsed)
		if best == nil || lastUsed < oldestUsage {
			best = replica
			oldestUsage = lastUsed
		}
	}

	return best
}

// selectLatencyBased selects replica with lowest latency
func (qr *QueryRouter) selectLatencyBased(replicas []*ReplicaDB) *ReplicaDB {
	if len(replicas) == 0 {
		return nil
	}

	var best *ReplicaDB
	lowestLatency := int64(0)

	for _, replica := range replicas {
		latency := atomic.LoadInt64(&replica.latency)
		if best == nil || (latency > 0 && latency < lowestLatency) {
			best = replica
			lowestLatency = latency
		}
	}

	// If no replica has recorded latency, fall back to round-robin
	if best == nil {
		return qr.selectRoundRobin(replicas)
	}

	return best
}

// shouldReadFromMaster checks if a table should always read from master
func (qr *QueryRouter) shouldReadFromMaster(table string) bool {
	for _, masterTable := range qr.config.ForceReadFromMaster {
		if masterTable == table {
			return true
		}
	}
	return false
}

// requiresStrongConsistency checks if the context requires strong consistency
func (qr *QueryRouter) requiresStrongConsistency(ctx context.Context) bool {
	if qr.config.ConsistencyLevel == "strong" {
		return true
	}

	// Check for consistency hint in context
	if hint, ok := ctx.Value("consistency").(string); ok {
		return hint == "strong"
	}

	// Check for bounded staleness
	if qr.config.ConsistencyLevel == "bounded_staleness" {
		if lastWrite, ok := ctx.Value("last_write_time").(time.Time); ok {
			staleness := time.Since(lastWrite)
			return staleness < qr.config.BoundedStalenessMaxAge
		}
	}

	return false
}

// startHealthChecking starts the background health checking routine
func (qr *QueryRouter) startHealthChecking() {
	ticker := time.NewTicker(qr.config.HealthCheckInterval)
	defer ticker.Stop()

	for range ticker.C {
		qr.checkReplicaHealth()
	}
}

// checkReplicaHealth checks the health of all replicas
func (qr *QueryRouter) checkReplicaHealth() {
	qr.mu.RLock()
	replicas := make([]*ReplicaDB, len(qr.replicas))
	copy(replicas, qr.replicas)
	qr.mu.RUnlock()

	var wg sync.WaitGroup
	for _, replica := range replicas {
		wg.Add(1)
		go func(r *ReplicaDB) {
			defer wg.Done()
			qr.checkSingleReplicaHealth(r)
		}(replica)
	}
	wg.Wait()

	// Update average replica latency metric
	qr.updateAverageLatencyMetric()
}

// checkSingleReplicaHealth checks the health of a single replica
func (qr *QueryRouter) checkSingleReplicaHealth(replica *ReplicaDB) {
	ctx, cancel := context.WithTimeout(context.Background(), qr.config.HealthCheckTimeout)
	defer cancel()

	start := time.Now()

	// Simple health check query
	var result int
	err := replica.db.WithContext(ctx).Raw("SELECT 1").Scan(&result).Error

	latency := time.Since(start)
	atomic.StoreInt64(&replica.latency, int64(latency))

	if err != nil {
		// Mark as unhealthy
		if atomic.CompareAndSwapInt32(&replica.healthy, 1, 0) {
			qr.logger.Warn("Replica marked as unhealthy",
				zap.String("replica", replica.name),
				zap.Error(err),
				zap.Duration("latency", latency))
		}
	} else {
		// Mark as healthy
		if atomic.CompareAndSwapInt32(&replica.healthy, 0, 1) {
			qr.logger.Info("Replica marked as healthy",
				zap.String("replica", replica.name),
				zap.Duration("latency", latency))
		}
	}
}

// updateAverageLatencyMetric updates the average latency metric
func (qr *QueryRouter) updateAverageLatencyMetric() {
	qr.mu.RLock()
	defer qr.mu.RUnlock()

	if len(qr.replicas) == 0 {
		return
	}

	var totalLatency int64
	var healthyCount int64

	for _, replica := range qr.replicas {
		if atomic.LoadInt32(&replica.healthy) == 1 {
			totalLatency += atomic.LoadInt64(&replica.latency)
			healthyCount++
		}
	}

	if healthyCount > 0 {
		avgLatency := totalLatency / healthyCount
		atomic.StoreInt64(&qr.metrics.AvgReplicaLatency, avgLatency)
	}
}

// GetMetrics returns current routing metrics
func (qr *QueryRouter) GetMetrics() QueryRouterMetrics {
	return QueryRouterMetrics{
		MasterQueries:     atomic.LoadInt64(&qr.metrics.MasterQueries),
		ReplicaQueries:    atomic.LoadInt64(&qr.metrics.ReplicaQueries),
		FailoverCount:     atomic.LoadInt64(&qr.metrics.FailoverCount),
		AvgReplicaLatency: atomic.LoadInt64(&qr.metrics.AvgReplicaLatency),
	}
}

// GetReplicaStatus returns the status of all replicas
func (qr *QueryRouter) GetReplicaStatus() []ReplicaStatus {
	qr.mu.RLock()
	defer qr.mu.RUnlock()

	status := make([]ReplicaStatus, len(qr.replicas))
	for i, replica := range qr.replicas {
		status[i] = ReplicaStatus{
			Name:     replica.name,
			Healthy:  atomic.LoadInt32(&replica.healthy) == 1,
			Latency:  time.Duration(atomic.LoadInt64(&replica.latency)),
			LastUsed: time.Unix(atomic.LoadInt64(&replica.lastUsed), 0),
			Weight:   replica.weight,
		}
	}

	return status
}

// ForceReadFromMaster adds a context value to force reading from master
func ForceReadFromMaster(ctx context.Context) context.Context {
	return context.WithValue(ctx, "consistency", "strong")
}

// WithBoundedStaleness adds a context value for bounded staleness consistency
func WithBoundedStaleness(ctx context.Context, lastWriteTime time.Time) context.Context {
	return context.WithValue(ctx, "last_write_time", lastWriteTime)
}

// ReplicaHealthChecker manages health checking for replicas
type ReplicaHealthChecker struct {
	replicas []*ReplicaDB
	config   *QueryRouterConfig
	logger   *zap.Logger
	mu       sync.RWMutex
}

// NewReplicaHealthChecker creates a new replica health checker
func NewReplicaHealthChecker(replicas []*ReplicaDB, config *QueryRouterConfig, logger *zap.Logger) *ReplicaHealthChecker {
	return &ReplicaHealthChecker{
		replicas: replicas,
		config:   config,
		logger:   logger,
	}
}

// AddReplica adds a replica to health checking
func (rhc *ReplicaHealthChecker) AddReplica(replica *ReplicaDB) {
	rhc.mu.Lock()
	defer rhc.mu.Unlock()
	rhc.replicas = append(rhc.replicas, replica)
}

// RemoveReplica removes a replica from health checking
func (rhc *ReplicaHealthChecker) RemoveReplica(name string) {
	rhc.mu.Lock()
	defer rhc.mu.Unlock()

	for i, replica := range rhc.replicas {
		if replica.name == name {
			rhc.replicas = append(rhc.replicas[:i], rhc.replicas[i+1:]...)
			break
		}
	}
}

// QueryType represents the type of database query
type QueryType int

const (
	QueryTypeRead QueryType = iota
	QueryTypeWrite
)

// ReplicaStatus represents the status of a replica
type ReplicaStatus struct {
	Name     string        `json:"name"`
	Healthy  bool          `json:"healthy"`
	Latency  time.Duration `json:"latency"`
	LastUsed time.Time     `json:"last_used"`
	Weight   int           `json:"weight"`
}
