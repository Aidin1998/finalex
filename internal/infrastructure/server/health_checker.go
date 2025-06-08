package server

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// HealthStatus represents the health status of a component
type HealthStatus string

const (
	HealthStatusUp      HealthStatus = "UP"
	HealthStatusDown    HealthStatus = "DOWN"
	HealthStatusWarning HealthStatus = "WARNING"
)

// ComponentHealth represents the health of a single component
type ComponentHealth struct {
	Status    HealthStatus           `json:"status"`
	Timestamp time.Time              `json:"timestamp"`
	Duration  time.Duration          `json:"duration"`
	Error     string                 `json:"error,omitempty"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

// HealthReport represents the overall health report
type HealthReport struct {
	Status     HealthStatus                `json:"status"`
	Timestamp  time.Time                   `json:"timestamp"`
	Duration   time.Duration               `json:"duration"`
	Components map[string]*ComponentHealth `json:"components"`
}

// ReadinessReport represents the readiness report
type ReadinessReport struct {
	Ready      bool                        `json:"ready"`
	Timestamp  time.Time                   `json:"timestamp"`
	Duration   time.Duration               `json:"duration"`
	Components map[string]*ComponentHealth `json:"components"`
}

// HealthChecker manages health and readiness checks
type HealthChecker struct {
	logger *zap.Logger
	db     *gorm.DB
	redis  *redis.Client

	// Health check configurations
	checks          map[string]HealthCheckFunc
	readinessChecks map[string]ReadinessCheckFunc
	mu              sync.RWMutex

	// Cache for health status
	lastHealthCheck *HealthReport
	lastReadyCheck  *ReadinessReport
	cacheDuration   time.Duration
}

// HealthCheckFunc defines the signature for health check functions
type HealthCheckFunc func(ctx context.Context) *ComponentHealth

// ReadinessCheckFunc defines the signature for readiness check functions
type ReadinessCheckFunc func(ctx context.Context) *ComponentHealth

// NewHealthChecker creates a new health checker
func NewHealthChecker(logger *zap.Logger, db *gorm.DB, redisClient *redis.Client) *HealthChecker {
	hc := &HealthChecker{
		logger:          logger,
		db:              db,
		redis:           redisClient,
		checks:          make(map[string]HealthCheckFunc),
		readinessChecks: make(map[string]ReadinessCheckFunc),
		cacheDuration:   30 * time.Second,
	}

	// Register default health checks
	hc.RegisterHealthCheck("database", hc.checkDatabase)
	hc.RegisterHealthCheck("redis", hc.checkRedis)
	hc.RegisterHealthCheck("memory", hc.checkMemory)
	hc.RegisterHealthCheck("disk", hc.checkDisk)

	// Register default readiness checks
	hc.RegisterReadinessCheck("database", hc.checkDatabaseReadiness)
	hc.RegisterReadinessCheck("redis", hc.checkRedisReadiness)

	return hc
}

// RegisterHealthCheck registers a custom health check
func (hc *HealthChecker) RegisterHealthCheck(name string, check HealthCheckFunc) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	hc.checks[name] = check
}

// RegisterReadinessCheck registers a custom readiness check
func (hc *HealthChecker) RegisterReadinessCheck(name string, check ReadinessCheckFunc) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	hc.readinessChecks[name] = check
}

// HealthHandler returns an HTTP handler for health checks
func (hc *HealthChecker) HealthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		report := hc.CheckHealth(r.Context())

		w.Header().Set("Content-Type", "application/json")

		// Set HTTP status based on health status
		switch report.Status {
		case HealthStatusUp:
			w.WriteHeader(http.StatusOK)
		case HealthStatusWarning:
			w.WriteHeader(http.StatusOK) // 200 for warnings but still functional
		case HealthStatusDown:
			w.WriteHeader(http.StatusServiceUnavailable)
		}

		json.NewEncoder(w).Encode(report)
	}
}

// ReadinessHandler returns an HTTP handler for readiness checks
func (hc *HealthChecker) ReadinessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		report := hc.CheckReadiness(r.Context())

		w.Header().Set("Content-Type", "application/json")

		if report.Ready {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}

		json.NewEncoder(w).Encode(report)
	}
}

// CheckHealth performs all health checks
func (hc *HealthChecker) CheckHealth(ctx context.Context) *HealthReport {
	start := time.Now()

	// Check cache first
	hc.mu.RLock()
	if hc.lastHealthCheck != nil && time.Since(hc.lastHealthCheck.Timestamp) < hc.cacheDuration {
		defer hc.mu.RUnlock()
		return hc.lastHealthCheck
	}
	hc.mu.RUnlock()

	components := make(map[string]*ComponentHealth)
	overallStatus := HealthStatusUp

	hc.mu.RLock()
	checks := make(map[string]HealthCheckFunc)
	for name, check := range hc.checks {
		checks[name] = check
	}
	hc.mu.RUnlock()

	// Execute all health checks concurrently
	var wg sync.WaitGroup
	var mu sync.Mutex

	for name, check := range checks {
		wg.Add(1)
		go func(name string, check HealthCheckFunc) {
			defer wg.Done()

			checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()

			health := check(checkCtx)

			mu.Lock()
			components[name] = health
			if health.Status == HealthStatusDown {
				overallStatus = HealthStatusDown
			} else if health.Status == HealthStatusWarning && overallStatus == HealthStatusUp {
				overallStatus = HealthStatusWarning
			}
			mu.Unlock()
		}(name, check)
	}

	wg.Wait()

	report := &HealthReport{
		Status:     overallStatus,
		Timestamp:  time.Now(),
		Duration:   time.Since(start),
		Components: components,
	}

	// Cache the result
	hc.mu.Lock()
	hc.lastHealthCheck = report
	hc.mu.Unlock()

	return report
}

// CheckReadiness performs all readiness checks
func (hc *HealthChecker) CheckReadiness(ctx context.Context) *ReadinessReport {
	start := time.Now()

	// Check cache first
	hc.mu.RLock()
	if hc.lastReadyCheck != nil && time.Since(hc.lastReadyCheck.Timestamp) < hc.cacheDuration {
		defer hc.mu.RUnlock()
		return hc.lastReadyCheck
	}
	hc.mu.RUnlock()

	components := make(map[string]*ComponentHealth)
	ready := true

	hc.mu.RLock()
	checks := make(map[string]ReadinessCheckFunc)
	for name, check := range hc.readinessChecks {
		checks[name] = check
	}
	hc.mu.RUnlock()

	// Execute all readiness checks concurrently
	var wg sync.WaitGroup
	var mu sync.Mutex

	for name, check := range checks {
		wg.Add(1)
		go func(name string, check ReadinessCheckFunc) {
			defer wg.Done()

			checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			health := check(checkCtx)

			mu.Lock()
			components[name] = health
			if health.Status != HealthStatusUp {
				ready = false
			}
			mu.Unlock()
		}(name, check)
	}
	wg.Wait()

	report := &ReadinessReport{
		Ready:      ready,
		Timestamp:  time.Now(),
		Duration:   time.Since(start),
		Components: components,
	}

	// Cache the result
	hc.mu.Lock()
	hc.lastReadyCheck = report
	hc.mu.Unlock()

	return report
}

// Database health checks
func (hc *HealthChecker) checkDatabase(ctx context.Context) *ComponentHealth {
	start := time.Now()
	health := &ComponentHealth{
		Timestamp: start,
		Details:   make(map[string]interface{}),
	}

	if hc.db == nil {
		health.Status = HealthStatusDown
		health.Error = "database not configured"
		health.Duration = time.Since(start)
		return health
	}

	sqlDB, err := hc.db.DB()
	if err != nil {
		health.Status = HealthStatusDown
		health.Error = "failed to get sql.DB: " + err.Error()
		health.Duration = time.Since(start)
		return health
	}

	// Check database connection
	if err := sqlDB.PingContext(ctx); err != nil {
		health.Status = HealthStatusDown
		health.Error = "database ping failed: " + err.Error()
		health.Duration = time.Since(start)
		return health
	}

	// Get database stats
	stats := sqlDB.Stats()
	health.Details["open_connections"] = stats.OpenConnections
	health.Details["in_use"] = stats.InUse
	health.Details["idle"] = stats.Idle
	health.Details["wait_count"] = stats.WaitCount
	health.Details["wait_duration"] = stats.WaitDuration.String()

	// Check for potential issues
	if stats.OpenConnections > 80 { // Assuming max 100 connections
		health.Status = HealthStatusWarning
		health.Error = "high connection usage"
	} else {
		health.Status = HealthStatusUp
	}

	health.Duration = time.Since(start)
	return health
}

func (hc *HealthChecker) checkDatabaseReadiness(ctx context.Context) *ComponentHealth {
	// For readiness, we just need to check basic connectivity
	return hc.checkDatabase(ctx)
}

// Redis health checks
func (hc *HealthChecker) checkRedis(ctx context.Context) *ComponentHealth {
	start := time.Now()
	health := &ComponentHealth{
		Timestamp: start,
		Details:   make(map[string]interface{}),
	}

	if hc.redis == nil {
		health.Status = HealthStatusDown
		health.Error = "redis not configured"
		health.Duration = time.Since(start)
		return health
	}

	// Check Redis connection
	if err := hc.redis.Ping(ctx).Err(); err != nil {
		health.Status = HealthStatusDown
		health.Error = "redis ping failed: " + err.Error()
		health.Duration = time.Since(start)
		return health
	} // Get Redis info
	_, err := hc.redis.Info(ctx).Result()
	if err != nil {
		health.Status = HealthStatusWarning
		health.Error = "failed to get redis info: " + err.Error()
	} else {
		health.Status = HealthStatusUp
		health.Details["info"] = "available"
	}

	health.Duration = time.Since(start)
	return health
}

func (hc *HealthChecker) checkRedisReadiness(ctx context.Context) *ComponentHealth {
	// For readiness, we just need to check basic connectivity
	return hc.checkRedis(ctx)
}

// Memory health check (placeholder - implement based on requirements)
func (hc *HealthChecker) checkMemory(ctx context.Context) *ComponentHealth {
	start := time.Now()
	health := &ComponentHealth{
		Timestamp: start,
		Status:    HealthStatusUp,
		Details:   make(map[string]interface{}),
		Duration:  time.Since(start),
	}

	// TODO: Implement actual memory checks using runtime.MemStats
	health.Details["status"] = "placeholder - implement memory monitoring"

	return health
}

// Disk health check (placeholder - implement based on requirements)
func (hc *HealthChecker) checkDisk(ctx context.Context) *ComponentHealth {
	start := time.Now()
	health := &ComponentHealth{
		Timestamp: start,
		Status:    HealthStatusUp,
		Details:   make(map[string]interface{}),
		Duration:  time.Since(start),
	}

	// TODO: Implement actual disk space checks
	health.Details["status"] = "placeholder - implement disk monitoring"

	return health
}
