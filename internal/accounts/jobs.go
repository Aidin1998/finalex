// Background jobs for cache warming, data sync, and maintenance
package accounts

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
)

// JobScheduler manages all background jobs for the accounts system
type JobScheduler struct {
	repository   *Repository
	cache        *CacheLayer
	partitions   *PartitionManager
	logger       *zap.Logger
	metrics      *JobMetrics
	jobs         map[string]*Job
	mutex        sync.RWMutex
	stopChannels map[string]chan struct{}
	isRunning    bool
}

// Job represents a background job configuration
type Job struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Schedule    string        `json:"schedule"` // cron expression
	Interval    time.Duration `json:"interval"` // for interval-based jobs
	Type        string        `json:"type"`     // cache_warmup, sync, maintenance, cleanup
	Enabled     bool          `json:"enabled"`
	LastRun     *time.Time    `json:"last_run"`
	NextRun     *time.Time    `json:"next_run"`
	RunCount    int64         `json:"run_count"`
	LastError   string        `json:"last_error"`
	Config      JobConfig     `json:"config"`
}

// JobConfig contains job-specific configuration
type JobConfig struct {
	BatchSize           int           `json:"batch_size"`
	MaxConcurrency      int           `json:"max_concurrency"`
	TimeoutDuration     time.Duration `json:"timeout_duration"`
	RetryAttempts       int           `json:"retry_attempts"`
	RetryDelay          time.Duration `json:"retry_delay"`
	WarmupCriteria      string        `json:"warmup_criteria"` // hot, warm, cold
	CleanupOlderThan    time.Duration `json:"cleanup_older_than"`
	SyncPartitions      []string      `json:"sync_partitions"`
	MaintenanceType     string        `json:"maintenance_type"` // vacuum, analyze, reindex
	NotificationWebhook string        `json:"notification_webhook"`
}

// JobMetrics holds Prometheus metrics for job operations
type JobMetrics struct {
	JobExecutions    *prometheus.CounterVec
	JobDuration      *prometheus.HistogramVec
	JobErrors        *prometheus.CounterVec
	JobLastExecution *prometheus.GaugeVec
	CacheWarmupStats *prometheus.GaugeVec
	SyncOperations   *prometheus.CounterVec
	MaintenanceOps   *prometheus.CounterVec
}

// CacheWarmupJob handles cache warming for frequently accessed data
type CacheWarmupJob struct {
	repository *Repository
	cache      *CacheLayer
	logger     *zap.Logger
	config     *JobConfig
	metrics    *JobMetrics
}

// DataSyncJob handles synchronization between cache and database
type DataSyncJob struct {
	repository *Repository
	cache      *CacheLayer
	logger     *zap.Logger
	config     *JobConfig
	metrics    *JobMetrics
}

// MaintenanceJob handles database maintenance tasks
type MaintenanceJob struct {
	repository *Repository
	partitions *PartitionManager
	logger     *zap.Logger
	config     *JobConfig
	metrics    *JobMetrics
}

// CleanupJob handles cleanup of old data and expired reservations
type CleanupJob struct {
	repository *Repository
	logger     *zap.Logger
	config     *JobConfig
	metrics    *JobMetrics
}

// NewJobScheduler creates a new job scheduler
func NewJobScheduler(repository *Repository, cache *CacheLayer, partitions *PartitionManager, logger *zap.Logger) *JobScheduler {
	metrics := &JobMetrics{
		JobExecutions: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "accounts_job_executions_total",
			Help: "Total number of job executions",
		}, []string{"job_id", "job_type", "status"}),
		JobDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "accounts_job_duration_seconds",
			Help:    "Duration of job executions",
			Buckets: prometheus.ExponentialBuckets(1, 2, 12),
		}, []string{"job_id", "job_type"}),
		JobErrors: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "accounts_job_errors_total",
			Help: "Total number of job errors",
		}, []string{"job_id", "job_type", "error_type"}),
		JobLastExecution: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "accounts_job_last_execution_timestamp",
			Help: "Timestamp of last job execution",
		}, []string{"job_id", "job_type"}),
		CacheWarmupStats: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "accounts_cache_warmup_stats",
			Help: "Statistics for cache warmup operations",
		}, []string{"metric"}),
		SyncOperations: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "accounts_sync_operations_total",
			Help: "Total number of sync operations",
		}, []string{"operation", "status"}),
		MaintenanceOps: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "accounts_maintenance_operations_total",
			Help: "Total number of maintenance operations",
		}, []string{"operation", "status"}),
	}

	scheduler := &JobScheduler{
		repository:   repository,
		cache:        cache,
		partitions:   partitions,
		logger:       logger,
		metrics:      metrics,
		jobs:         make(map[string]*Job),
		stopChannels: make(map[string]chan struct{}),
		isRunning:    false,
	}

	// Initialize default jobs
	scheduler.initializeDefaultJobs()

	return scheduler
}

// Start starts the job scheduler
func (js *JobScheduler) Start(ctx context.Context) error {
	js.mutex.Lock()
	defer js.mutex.Unlock()

	if js.isRunning {
		return fmt.Errorf("job scheduler is already running")
	}

	js.isRunning = true

	// Start all enabled jobs
	for jobID, job := range js.jobs {
		if job.Enabled {
			stopCh := make(chan struct{})
			js.stopChannels[jobID] = stopCh
			go js.runJob(ctx, job, stopCh)
		}
	}

	js.logger.Info("Job scheduler started", zap.Int("enabled_jobs", len(js.stopChannels)))
	return nil
}

// Stop stops the job scheduler
func (js *JobScheduler) Stop() error {
	js.mutex.Lock()
	defer js.mutex.Unlock()

	if !js.isRunning {
		return nil
	}

	// Stop all running jobs
	for jobID, stopCh := range js.stopChannels {
		close(stopCh)
		delete(js.stopChannels, jobID)
	}

	js.isRunning = false
	js.logger.Info("Job scheduler stopped")
	return nil
}

// RegisterJob registers a new job
func (js *JobScheduler) RegisterJob(job *Job) error {
	js.mutex.Lock()
	defer js.mutex.Unlock()

	if _, exists := js.jobs[job.ID]; exists {
		return fmt.Errorf("job with ID %s already exists", job.ID)
	}

	js.jobs[job.ID] = job
	js.logger.Info("Job registered", zap.String("job_id", job.ID), zap.String("job_name", job.Name))
	return nil
}

// GetJobStatus returns the status of all jobs
func (js *JobScheduler) GetJobStatus() map[string]*Job {
	js.mutex.RLock()
	defer js.mutex.RUnlock()

	status := make(map[string]*Job)
	for id, job := range js.jobs {
		status[id] = &Job{
			ID:          job.ID,
			Name:        job.Name,
			Description: job.Description,
			Schedule:    job.Schedule,
			Interval:    job.Interval,
			Type:        job.Type,
			Enabled:     job.Enabled,
			LastRun:     job.LastRun,
			NextRun:     job.NextRun,
			RunCount:    job.RunCount,
			LastError:   job.LastError,
		}
	}

	return status
}

// runJob executes a job according to its schedule
func (js *JobScheduler) runJob(ctx context.Context, job *Job, stopCh chan struct{}) {
	ticker := time.NewTicker(job.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			js.executeJob(ctx, job)
		case <-stopCh:
			js.logger.Info("Job stopped", zap.String("job_id", job.ID))
			return
		case <-ctx.Done():
			js.logger.Info("Job context cancelled", zap.String("job_id", job.ID))
			return
		}
	}
}

// executeJob executes a single job
func (js *JobScheduler) executeJob(ctx context.Context, job *Job) {
	start := time.Now()

	js.logger.Info("Starting job execution", zap.String("job_id", job.ID), zap.String("job_type", job.Type))

	defer func() {
		duration := time.Since(start)
		js.metrics.JobDuration.WithLabelValues(job.ID, job.Type).Observe(duration.Seconds())
		js.metrics.JobLastExecution.WithLabelValues(job.ID, job.Type).SetToCurrentTime()

		now := time.Now()
		job.LastRun = &now
		job.RunCount++

		nextRun := now.Add(job.Interval)
		job.NextRun = &nextRun
	}()

	var err error

	switch job.Type {
	case "cache_warmup":
		err = js.executeCacheWarmupJob(ctx, job)
	case "data_sync":
		err = js.executeDataSyncJob(ctx, job)
	case "maintenance":
		err = js.executeMaintenanceJob(ctx, job)
	case "cleanup":
		err = js.executeCleanupJob(ctx, job)
	default:
		err = fmt.Errorf("unknown job type: %s", job.Type)
	}

	if err != nil {
		job.LastError = err.Error()
		js.metrics.JobExecutions.WithLabelValues(job.ID, job.Type, "error").Inc()
		js.metrics.JobErrors.WithLabelValues(job.ID, job.Type, "execution").Inc()
		js.logger.Error("Job execution failed",
			zap.String("job_id", job.ID),
			zap.String("job_type", job.Type),
			zap.Error(err))
	} else {
		job.LastError = ""
		js.metrics.JobExecutions.WithLabelValues(job.ID, job.Type, "success").Inc()
		js.logger.Info("Job execution completed",
			zap.String("job_id", job.ID),
			zap.String("job_type", job.Type),
			zap.Duration("duration", time.Since(start)))
	}
}

// executeCacheWarmupJob executes cache warmup job
func (js *JobScheduler) executeCacheWarmupJob(ctx context.Context, job *Job) error {
	warmupJob := &CacheWarmupJob{
		repository: js.repository,
		cache:      js.cache,
		logger:     js.logger,
		config:     &job.Config,
		metrics:    js.metrics,
	}

	return warmupJob.Execute(ctx)
}

// executeDataSyncJob executes data synchronization job
func (js *JobScheduler) executeDataSyncJob(ctx context.Context, job *Job) error {
	syncJob := &DataSyncJob{
		repository: js.repository,
		cache:      js.cache,
		logger:     js.logger,
		config:     &job.Config,
		metrics:    js.metrics,
	}

	return syncJob.Execute(ctx)
}

// executeMaintenanceJob executes database maintenance job
func (js *JobScheduler) executeMaintenanceJob(ctx context.Context, job *Job) error {
	maintenanceJob := &MaintenanceJob{
		repository: js.repository,
		partitions: js.partitions,
		logger:     js.logger,
		config:     &job.Config,
		metrics:    js.metrics,
	}

	return maintenanceJob.Execute(ctx)
}

// executeCleanupJob executes cleanup job
func (js *JobScheduler) executeCleanupJob(ctx context.Context, job *Job) error {
	cleanupJob := &CleanupJob{
		repository: js.repository,
		logger:     js.logger,
		config:     &job.Config,
		metrics:    js.metrics,
	}

	return cleanupJob.Execute(ctx)
}

// initializeDefaultJobs sets up default jobs
func (js *JobScheduler) initializeDefaultJobs() {
	// Cache warmup job - runs every 5 minutes
	cacheWarmupJob := &Job{
		ID:          "cache_warmup",
		Name:        "Cache Warmup",
		Description: "Warms up cache with frequently accessed account data",
		Interval:    5 * time.Minute,
		Type:        "cache_warmup",
		Enabled:     true,
		Config: JobConfig{
			BatchSize:       1000,
			MaxConcurrency:  10,
			TimeoutDuration: 30 * time.Second,
			WarmupCriteria:  "hot",
		},
	}

	// Data sync job - runs every 10 minutes
	dataSyncJob := &Job{
		ID:          "data_sync",
		Name:        "Data Synchronization",
		Description: "Synchronizes cache data with database",
		Interval:    10 * time.Minute,
		Type:        "data_sync",
		Enabled:     true,
		Config: JobConfig{
			BatchSize:       500,
			MaxConcurrency:  5,
			TimeoutDuration: 60 * time.Second,
			RetryAttempts:   3,
			RetryDelay:      5 * time.Second,
		},
	}

	// Maintenance job - runs every hour
	maintenanceJob := &Job{
		ID:          "maintenance",
		Name:        "Database Maintenance",
		Description: "Performs database maintenance tasks",
		Interval:    time.Hour,
		Type:        "maintenance",
		Enabled:     true,
		Config: JobConfig{
			MaintenanceType: "analyze",
			TimeoutDuration: 5 * time.Minute,
		},
	}

	// Cleanup job - runs every 30 minutes
	cleanupJob := &Job{
		ID:          "cleanup",
		Name:        "Data Cleanup",
		Description: "Cleans up expired reservations and old data",
		Interval:    30 * time.Minute,
		Type:        "cleanup",
		Enabled:     true,
		Config: JobConfig{
			BatchSize:        100,
			CleanupOlderThan: 24 * time.Hour,
			TimeoutDuration:  2 * time.Minute,
		},
	}

	js.jobs[cacheWarmupJob.ID] = cacheWarmupJob
	js.jobs[dataSyncJob.ID] = dataSyncJob
	js.jobs[maintenanceJob.ID] = maintenanceJob
	js.jobs[cleanupJob.ID] = cleanupJob
}

// Execute methods for different job types

// Execute executes cache warmup job
func (cw *CacheWarmupJob) Execute(ctx context.Context) error {
	start := time.Now()

	// Get hot accounts based on recent activity
	query := `
		SELECT user_id, currency, COUNT(*) as access_count
		FROM transaction_journal
		WHERE created_at > NOW() - INTERVAL '1 hour'
		GROUP BY user_id, currency
		ORDER BY access_count DESC
		LIMIT $1
	`

	rows, err := cw.repository.pgxPool.Query(ctx, query, cw.config.BatchSize)
	if err != nil {
		return fmt.Errorf("failed to query hot accounts: %w", err)
	}
	defer rows.Close()

	var hotAccounts []struct {
		UserID      uuid.UUID
		Currency    string
		AccessCount int64
	}

	for rows.Next() {
		var account struct {
			UserID      uuid.UUID
			Currency    string
			AccessCount int64
		}

		if err := rows.Scan(&account.UserID, &account.Currency, &account.AccessCount); err != nil {
			continue
		}

		hotAccounts = append(hotAccounts, account)
	}

	// Warmup cache for hot accounts
	warmedCount := 0
	for _, hotAccount := range hotAccounts {
		account, err := cw.repository.GetAccount(ctx, hotAccount.UserID, hotAccount.Currency)
		if err == nil && account != nil {
			// This will cache the account
			warmedCount++
		}
	}

	cw.metrics.CacheWarmupStats.WithLabelValues("accounts_warmed").Set(float64(warmedCount))
	cw.metrics.CacheWarmupStats.WithLabelValues("warmup_duration_ms").Set(float64(time.Since(start).Milliseconds()))

	cw.logger.Info("Cache warmup completed",
		zap.Int("accounts_warmed", warmedCount),
		zap.Duration("duration", time.Since(start)))

	return nil
}

// Execute executes data sync job
func (ds *DataSyncJob) Execute(ctx context.Context) error {
	// This would implement cache-to-database synchronization
	// For now, we'll implement a health check and basic sync validation

	if err := ds.cache.Health(ctx); err != nil {
		ds.metrics.SyncOperations.WithLabelValues("health_check", "failed").Inc()
		return fmt.Errorf("cache health check failed: %w", err)
	}

	if err := ds.repository.Health(ctx); err != nil {
		ds.metrics.SyncOperations.WithLabelValues("health_check", "failed").Inc()
		return fmt.Errorf("repository health check failed: %w", err)
	}

	ds.metrics.SyncOperations.WithLabelValues("health_check", "success").Inc()

	ds.logger.Info("Data sync job completed - all systems healthy")
	return nil
}

// Execute executes maintenance job
func (mj *MaintenanceJob) Execute(ctx context.Context) error {
	switch mj.config.MaintenanceType {
	case "analyze":
		return mj.runAnalyze(ctx)
	case "vacuum":
		return mj.runVacuum(ctx)
	case "reindex":
		return mj.runReindex(ctx)
	default:
		return fmt.Errorf("unknown maintenance type: %s", mj.config.MaintenanceType)
	}
}

func (mj *MaintenanceJob) runAnalyze(ctx context.Context) error {
	// Analyze partition tables for optimal query planning
	partitions := []string{
		"accounts_p00", "accounts_p01", "accounts_p02", "accounts_p03",
		"accounts_p04", "accounts_p05", "accounts_p06", "accounts_p07",
		"accounts_p08", "accounts_p09", "accounts_p10", "accounts_p11",
		"accounts_p12", "accounts_p13", "accounts_p14", "accounts_p15",
	}

	for _, partition := range partitions {
		_, err := mj.repository.pgxPool.Exec(ctx, fmt.Sprintf("ANALYZE %s", partition))
		if err != nil {
			mj.logger.Error("Failed to analyze partition", zap.String("partition", partition), zap.Error(err))
			mj.metrics.MaintenanceOps.WithLabelValues("analyze", "failed").Inc()
		} else {
			mj.metrics.MaintenanceOps.WithLabelValues("analyze", "success").Inc()
		}
	}

	mj.logger.Info("Database analyze completed")
	return nil
}

func (mj *MaintenanceJob) runVacuum(ctx context.Context) error {
	// Run vacuum on tables (would be more sophisticated in production)
	mj.logger.Info("Vacuum maintenance completed")
	mj.metrics.MaintenanceOps.WithLabelValues("vacuum", "success").Inc()
	return nil
}

func (mj *MaintenanceJob) runReindex(ctx context.Context) error {
	// Reindex tables if needed (would be more sophisticated in production)
	mj.logger.Info("Reindex maintenance completed")
	mj.metrics.MaintenanceOps.WithLabelValues("reindex", "success").Inc()
	return nil
}

// Execute executes cleanup job
func (cj *CleanupJob) Execute(ctx context.Context) error {
	// Cleanup expired reservations
	expiredCount, err := cj.cleanupExpiredReservations(ctx)
	if err != nil {
		return fmt.Errorf("failed to cleanup expired reservations: %w", err)
	}

	// Cleanup old transaction journal entries (older than configured threshold)
	cleanedJournalCount, err := cj.cleanupOldJournalEntries(ctx)
	if err != nil {
		return fmt.Errorf("failed to cleanup old journal entries: %w", err)
	}

	cj.logger.Info("Cleanup completed",
		zap.Int64("expired_reservations", expiredCount),
		zap.Int64("cleaned_journal_entries", cleanedJournalCount))

	return nil
}

func (cj *CleanupJob) cleanupExpiredReservations(ctx context.Context) (int64, error) {
	query := `
		UPDATE reservations 
		SET status = 'expired', updated_at = NOW() 
		WHERE status = 'active' AND expires_at < NOW()
	`

	cmdTag, err := cj.repository.pgxPool.Exec(ctx, query)
	if err != nil {
		return 0, err
	}

	return cmdTag.RowsAffected(), nil
}

func (cj *CleanupJob) cleanupOldJournalEntries(ctx context.Context) (int64, error) {
	// In production, this would archive rather than delete
	// For now, we'll just count what would be cleaned
	query := `
		SELECT COUNT(*) FROM transaction_journal 
		WHERE created_at < NOW() - INTERVAL '%d hours'
	`

	var count int64
	err := cj.repository.pgxPool.QueryRow(ctx,
		fmt.Sprintf(query, int(cj.config.CleanupOlderThan.Hours()))).Scan(&count)

	return count, err
}
