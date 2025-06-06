package accounts

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
)

// DataLifecyclePolicy defines data retention and archival policies
type DataLifecyclePolicy struct {
	TableName           string        `json:"table_name"`
	RetentionPeriod     time.Duration `json:"retention_period"`
	ArchivePeriod       time.Duration `json:"archive_period"`
	PurgePeriod         time.Duration `json:"purge_period"`
	ArchiveThreshold    int64         `json:"archive_threshold"` // Records count threshold
	CompressionEnabled  bool          `json:"compression_enabled"`
	PartitioningEnabled bool          `json:"partitioning_enabled"`
	AutoCleanupEnabled  bool          `json:"auto_cleanup_enabled"`
	BackupBeforeArchive bool          `json:"backup_before_archive"`
	ComplianceRetention time.Duration `json:"compliance_retention"` // Regulatory compliance
	EncryptionRequired  bool          `json:"encryption_required"`
}

// ArchivalJob represents a scheduled archival operation
type ArchivalJob struct {
	ID               string                 `json:"id"`
	Policy           *DataLifecyclePolicy   `json:"policy"`
	Status           string                 `json:"status"` // pending, running, completed, failed
	StartTime        time.Time              `json:"start_time"`
	EndTime          time.Time              `json:"end_time"`
	RecordsProcessed int64                  `json:"records_processed"`
	RecordsArchived  int64                  `json:"records_archived"`
	RecordsPurged    int64                  `json:"records_purged"`
	ErrorMessage     string                 `json:"error_message,omitempty"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
}

// DataLifecycleManager handles automated data archiving, purging, and lifecycle management
type DataLifecycleManager struct {
	db        *sql.DB
	logger    *zap.Logger
	policies  map[string]*DataLifecyclePolicy
	jobs      map[string]*ArchivalJob
	scheduler *time.Ticker
	stopChan  chan struct{}
	mu        sync.RWMutex
	jobMu     sync.RWMutex

	// Configuration
	batchSize         int
	maxConcurrency    int
	checkInterval     time.Duration
	enableMetrics     bool
	enableCompression bool
	maxRetries        int
	retryDelay        time.Duration

	// Metrics
	jobsTotal        prometheus.Counter
	jobsSuccessful   prometheus.Counter
	jobsFailed       prometheus.Counter
	recordsArchived  prometheus.Counter
	recordsPurged    prometheus.Counter
	archivalDuration prometheus.Histogram
	storageReclaimed prometheus.Gauge
	policyViolations prometheus.Counter
	compressionRatio prometheus.Gauge
}

// NewDataLifecycleManager creates a new data lifecycle manager
func NewDataLifecycleManager(db *sql.DB, logger *zap.Logger) *DataLifecycleManager {
	dlm := &DataLifecycleManager{
		db:                db,
		logger:            logger,
		policies:          make(map[string]*DataLifecyclePolicy),
		jobs:              make(map[string]*ArchivalJob),
		stopChan:          make(chan struct{}),
		batchSize:         10000,
		maxConcurrency:    4,
		checkInterval:     time.Hour,
		enableMetrics:     true,
		enableCompression: true,
		maxRetries:        3,
		retryDelay:        time.Minute * 5,
	}

	dlm.initMetrics()
	dlm.loadDefaultPolicies()

	return dlm
}

// initMetrics initializes Prometheus metrics
func (dlm *DataLifecycleManager) initMetrics() {
	if !dlm.enableMetrics {
		return
	}

	dlm.jobsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "accounts_lifecycle_jobs_total",
		Help: "Total number of lifecycle jobs executed",
	})

	dlm.jobsSuccessful = promauto.NewCounter(prometheus.CounterOpts{
		Name: "accounts_lifecycle_jobs_successful_total",
		Help: "Total number of successful lifecycle jobs",
	})

	dlm.jobsFailed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "accounts_lifecycle_jobs_failed_total",
		Help: "Total number of failed lifecycle jobs",
	})

	dlm.recordsArchived = promauto.NewCounter(prometheus.CounterOpts{
		Name: "accounts_records_archived_total",
		Help: "Total number of records archived",
	})

	dlm.recordsPurged = promauto.NewCounter(prometheus.CounterOpts{
		Name: "accounts_records_purged_total",
		Help: "Total number of records purged",
	})

	dlm.archivalDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "accounts_archival_duration_seconds",
		Help:    "Duration of archival operations",
		Buckets: prometheus.ExponentialBuckets(1, 2, 12),
	})

	dlm.storageReclaimed = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "accounts_storage_reclaimed_bytes",
		Help: "Total storage reclaimed through lifecycle management",
	})

	dlm.policyViolations = promauto.NewCounter(prometheus.CounterOpts{
		Name: "accounts_policy_violations_total",
		Help: "Total number of data policy violations detected",
	})

	dlm.compressionRatio = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "accounts_compression_ratio",
		Help: "Current compression ratio for archived data",
	})
}

// loadDefaultPolicies loads default data lifecycle policies
func (dlm *DataLifecycleManager) loadDefaultPolicies() {
	defaultPolicies := []*DataLifecyclePolicy{
		{
			TableName:           "accounts",
			RetentionPeriod:     time.Hour * 24 * 365 * 7,  // 7 years
			ArchivePeriod:       time.Hour * 24 * 365,      // 1 year
			PurgePeriod:         time.Hour * 24 * 365 * 10, // 10 years
			ArchiveThreshold:    1000000,                   // 1M records
			CompressionEnabled:  true,
			PartitioningEnabled: true,
			AutoCleanupEnabled:  true,
			BackupBeforeArchive: true,
			ComplianceRetention: time.Hour * 24 * 365 * 7,
			EncryptionRequired:  true,
		},
		{
			TableName:           "account_balances",
			RetentionPeriod:     time.Hour * 24 * 365 * 5, // 5 years
			ArchivePeriod:       time.Hour * 24 * 180,     // 6 months
			PurgePeriod:         time.Hour * 24 * 365 * 7, // 7 years
			ArchiveThreshold:    5000000,                  // 5M records
			CompressionEnabled:  true,
			PartitioningEnabled: true,
			AutoCleanupEnabled:  true,
			BackupBeforeArchive: true,
			ComplianceRetention: time.Hour * 24 * 365 * 5,
			EncryptionRequired:  true,
		},
		{
			TableName:           "account_transactions",
			RetentionPeriod:     time.Hour * 24 * 365 * 7,  // 7 years
			ArchivePeriod:       time.Hour * 24 * 90,       // 3 months
			PurgePeriod:         time.Hour * 24 * 365 * 10, // 10 years
			ArchiveThreshold:    10000000,                  // 10M records
			CompressionEnabled:  true,
			PartitioningEnabled: true,
			AutoCleanupEnabled:  true,
			BackupBeforeArchive: true,
			ComplianceRetention: time.Hour * 24 * 365 * 7,
			EncryptionRequired:  true,
		},
		{
			TableName:           "account_events",
			RetentionPeriod:     time.Hour * 24 * 365,     // 1 year
			ArchivePeriod:       time.Hour * 24 * 30,      // 1 month
			PurgePeriod:         time.Hour * 24 * 365 * 3, // 3 years
			ArchiveThreshold:    50000000,                 // 50M records
			CompressionEnabled:  true,
			PartitioningEnabled: true,
			AutoCleanupEnabled:  true,
			BackupBeforeArchive: false, // Events can be regenerated
			ComplianceRetention: time.Hour * 24 * 365,
			EncryptionRequired:  false,
		},
	}

	for _, policy := range defaultPolicies {
		dlm.policies[policy.TableName] = policy
	}
}

// Start begins the lifecycle management process
func (dlm *DataLifecycleManager) Start(ctx context.Context) error {
	dlm.logger.Info("Starting data lifecycle manager",
		zap.Duration("check_interval", dlm.checkInterval),
		zap.Int("max_concurrency", dlm.maxConcurrency))

	dlm.scheduler = time.NewTicker(dlm.checkInterval)

	go dlm.runScheduler(ctx)

	return nil
}

// Stop gracefully stops the lifecycle manager
func (dlm *DataLifecycleManager) Stop() error {
	dlm.logger.Info("Stopping data lifecycle manager")

	close(dlm.stopChan)

	if dlm.scheduler != nil {
		dlm.scheduler.Stop()
	}

	// Wait for running jobs to complete
	dlm.waitForJobs()

	return nil
}

// runScheduler runs the background scheduler
func (dlm *DataLifecycleManager) runScheduler(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-dlm.stopChan:
			return
		case <-dlm.scheduler.C:
			dlm.executeLifecyclePolicies(ctx)
		}
	}
}

// executeLifecyclePolicies executes all configured lifecycle policies
func (dlm *DataLifecycleManager) executeLifecyclePolicies(ctx context.Context) {
	dlm.mu.RLock()
	policies := make([]*DataLifecyclePolicy, 0, len(dlm.policies))
	for _, policy := range dlm.policies {
		policies = append(policies, policy)
	}
	dlm.mu.RUnlock()

	semaphore := make(chan struct{}, dlm.maxConcurrency)
	var wg sync.WaitGroup

	for _, policy := range policies {
		wg.Add(1)
		go func(p *DataLifecyclePolicy) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			if err := dlm.executePolicyForTable(ctx, p); err != nil {
				dlm.logger.Error("Failed to execute lifecycle policy",
					zap.String("table", p.TableName),
					zap.Error(err))
				if dlm.enableMetrics {
					dlm.jobsFailed.Inc()
				}
			}
		}(policy)
	}

	wg.Wait()
}

// executePolicyForTable executes a lifecycle policy for a specific table
func (dlm *DataLifecycleManager) executePolicyForTable(ctx context.Context, policy *DataLifecyclePolicy) error {
	startTime := time.Now()

	if dlm.enableMetrics {
		dlm.jobsTotal.Inc()
		defer func() {
			dlm.archivalDuration.Observe(time.Since(startTime).Seconds())
		}()
	}

	// Check if archival is needed
	needsArchival, err := dlm.checkArchivalNeeded(ctx, policy)
	if err != nil {
		return fmt.Errorf("failed to check archival needs: %w", err)
	}

	if needsArchival {
		job := &ArchivalJob{
			ID:        fmt.Sprintf("%s_%d", policy.TableName, time.Now().Unix()),
			Policy:    policy,
			Status:    "running",
			StartTime: startTime,
			Metadata:  make(map[string]interface{}),
		}

		dlm.jobMu.Lock()
		dlm.jobs[job.ID] = job
		dlm.jobMu.Unlock()

		err = dlm.performArchival(ctx, job)
		if err != nil {
			job.Status = "failed"
			job.ErrorMessage = err.Error()
			return err
		}

		job.Status = "completed"
		job.EndTime = time.Now()

		if dlm.enableMetrics {
			dlm.jobsSuccessful.Inc()
			dlm.recordsArchived.Add(float64(job.RecordsArchived))
			dlm.recordsPurged.Add(float64(job.RecordsPurged))
		}
	}

	// Check if purging is needed
	needsPurging, err := dlm.checkPurgingNeeded(ctx, policy)
	if err != nil {
		return fmt.Errorf("failed to check purging needs: %w", err)
	}

	if needsPurging {
		err = dlm.performPurging(ctx, policy)
		if err != nil {
			return fmt.Errorf("failed to perform purging: %w", err)
		}
	}

	return nil
}

// checkArchivalNeeded determines if archival is needed for a table
func (dlm *DataLifecycleManager) checkArchivalNeeded(ctx context.Context, policy *DataLifecyclePolicy) (bool, error) {
	// Check record count threshold
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE created_at < $1", policy.TableName)
	archiveThreshold := time.Now().Add(-policy.ArchivePeriod)

	var count int64
	err := dlm.db.QueryRowContext(ctx, countQuery, archiveThreshold).Scan(&count)
	if err != nil {
		return false, err
	}

	return count >= policy.ArchiveThreshold, nil
}

// checkPurgingNeeded determines if purging is needed for a table
func (dlm *DataLifecycleManager) checkPurgingNeeded(ctx context.Context, policy *DataLifecyclePolicy) (bool, error) {
	// Check if there are records older than purge period
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM %s_archive WHERE archived_at < $1", policy.TableName)
	purgeThreshold := time.Now().Add(-policy.PurgePeriod)

	var count int64
	err := dlm.db.QueryRowContext(ctx, countQuery, purgeThreshold).Scan(&count)
	if err != nil {
		// Archive table might not exist
		return false, nil
	}

	return count > 0, nil
}

// performArchival performs the actual archival operation
func (dlm *DataLifecycleManager) performArchival(ctx context.Context, job *ArchivalJob) error {
	policy := job.Policy

	// Create archive table if it doesn't exist
	err := dlm.createArchiveTable(ctx, policy.TableName)
	if err != nil {
		return fmt.Errorf("failed to create archive table: %w", err)
	}

	// Perform backup if required
	if policy.BackupBeforeArchive {
		err = dlm.createBackup(ctx, policy.TableName)
		if err != nil {
			return fmt.Errorf("failed to create backup: %w", err)
		}
	}

	// Archive old records in batches
	archiveThreshold := time.Now().Add(-policy.ArchivePeriod)

	for {
		// Get batch of records to archive
		selectQuery := fmt.Sprintf(`
			SELECT * FROM %s 
			WHERE created_at < $1 
			ORDER BY created_at 
			LIMIT %d
		`, policy.TableName, dlm.batchSize)

		rows, err := dlm.db.QueryContext(ctx, selectQuery, archiveThreshold)
		if err != nil {
			return fmt.Errorf("failed to select records for archival: %w", err)
		}

		// Process batch
		batchProcessed, err := dlm.processBatch(ctx, rows, policy.TableName, policy.CompressionEnabled)
		rows.Close()

		if err != nil {
			return fmt.Errorf("failed to process archival batch: %w", err)
		}

		job.RecordsProcessed += int64(batchProcessed)
		job.RecordsArchived += int64(batchProcessed)

		// Break if no more records
		if batchProcessed < dlm.batchSize {
			break
		}

		// Check context cancellation
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}

	return nil
}

// performPurging performs the actual purging operation
func (dlm *DataLifecycleManager) performPurging(ctx context.Context, policy *DataLifecyclePolicy) error {
	purgeThreshold := time.Now().Add(-policy.PurgePeriod)

	// Delete old archived records in batches
	for {
		deleteQuery := fmt.Sprintf(`
			DELETE FROM %s_archive 
			WHERE archived_at < $1 
			LIMIT %d
		`, policy.TableName, dlm.batchSize)

		result, err := dlm.db.ExecContext(ctx, deleteQuery, purgeThreshold)
		if err != nil {
			return fmt.Errorf("failed to purge records: %w", err)
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to get purged rows count: %w", err)
		}

		if dlm.enableMetrics {
			dlm.recordsPurged.Add(float64(rowsAffected))
		}

		// Break if no more records
		if rowsAffected < int64(dlm.batchSize) {
			break
		}

		// Check context cancellation
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}

	return nil
}

// createArchiveTable creates an archive table for the given source table
func (dlm *DataLifecycleManager) createArchiveTable(ctx context.Context, tableName string) error {
	archiveTableName := tableName + "_archive"

	// Check if archive table already exists
	checkQuery := `
		SELECT COUNT(*) FROM information_schema.tables 
		WHERE table_name = $1
	`
	var count int
	err := dlm.db.QueryRowContext(ctx, checkQuery, archiveTableName).Scan(&count)
	if err != nil {
		return err
	}

	if count > 0 {
		return nil // Archive table already exists
	}

	// Create archive table with same structure as source table
	createQuery := fmt.Sprintf(`
		CREATE TABLE %s (LIKE %s INCLUDING ALL);
		ALTER TABLE %s ADD COLUMN archived_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP;
		CREATE INDEX idx_%s_archived_at ON %s (archived_at);
	`, archiveTableName, tableName, archiveTableName, archiveTableName, archiveTableName)

	_, err = dlm.db.ExecContext(ctx, createQuery)
	return err
}

// createBackup creates a backup of the table before archival
func (dlm *DataLifecycleManager) createBackup(ctx context.Context, tableName string) error {
	backupTableName := fmt.Sprintf("%s_backup_%d", tableName, time.Now().Unix())

	backupQuery := fmt.Sprintf(`
		CREATE TABLE %s AS SELECT * FROM %s
	`, backupTableName, tableName)

	_, err := dlm.db.ExecContext(ctx, backupQuery)
	return err
}

// processBatch processes a batch of records for archival
func (dlm *DataLifecycleManager) processBatch(ctx context.Context, rows *sql.Rows, tableName string, compression bool) (int, error) {
	archiveTableName := tableName + "_archive"
	processed := 0

	// Get column names
	columns, err := rows.Columns()
	if err != nil {
		return 0, err
	}

	// Prepare values slice
	values := make([]interface{}, len(columns))
	valuePtrs := make([]interface{}, len(columns))
	for i := range values {
		valuePtrs[i] = &values[i]
	}

	// Start transaction
	tx, err := dlm.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	// Prepare insert and delete statements
	insertPlaceholders := make([]string, len(columns))
	for i := range insertPlaceholders {
		insertPlaceholders[i] = fmt.Sprintf("$%d", i+1)
	}

	insertQuery := fmt.Sprintf(`
		INSERT INTO %s (%s) VALUES (%s)
	`, archiveTableName,
		fmt.Sprintf("%s", strings.Join(columns, ", ")),
		strings.Join(insertPlaceholders, ", "))

	insertStmt, err := tx.PrepareContext(ctx, insertQuery)
	if err != nil {
		return 0, err
	}
	defer insertStmt.Close()

	// Process each row
	var recordsToDelete []interface{}
	for rows.Next() {
		err = rows.Scan(valuePtrs...)
		if err != nil {
			return 0, err
		}

		// Insert into archive table
		_, err = insertStmt.ExecContext(ctx, values...)
		if err != nil {
			return 0, err
		}

		// Store primary key for deletion
		recordsToDelete = append(recordsToDelete, values[0]) // Assuming first column is PK
		processed++
	}

	// Delete archived records from source table
	if len(recordsToDelete) > 0 {
		deleteQuery := fmt.Sprintf(`
			DELETE FROM %s WHERE id = ANY($1)
		`, tableName)

		_, err = tx.ExecContext(ctx, deleteQuery, recordsToDelete)
		if err != nil {
			return 0, err
		}
	}

	// Commit transaction
	err = tx.Commit()
	if err != nil {
		return 0, err
	}

	return processed, nil
}

// waitForJobs waits for all running jobs to complete
func (dlm *DataLifecycleManager) waitForJobs() {
	timeout := time.After(time.Minute * 5)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			dlm.logger.Warn("Timeout waiting for lifecycle jobs to complete")
			return
		case <-ticker.C:
			dlm.jobMu.RLock()
			running := 0
			for _, job := range dlm.jobs {
				if job.Status == "running" {
					running++
				}
			}
			dlm.jobMu.RUnlock()

			if running == 0 {
				return
			}
		}
	}
}

// AddPolicy adds a new lifecycle policy
func (dlm *DataLifecycleManager) AddPolicy(policy *DataLifecyclePolicy) error {
	dlm.mu.Lock()
	defer dlm.mu.Unlock()

	dlm.policies[policy.TableName] = policy
	dlm.logger.Info("Added lifecycle policy",
		zap.String("table", policy.TableName),
		zap.Duration("retention", policy.RetentionPeriod))

	return nil
}

// GetPolicy retrieves a lifecycle policy
func (dlm *DataLifecycleManager) GetPolicy(tableName string) (*DataLifecyclePolicy, bool) {
	dlm.mu.RLock()
	defer dlm.mu.RUnlock()

	policy, exists := dlm.policies[tableName]
	return policy, exists
}

// GetJobStatus returns the status of a lifecycle job
func (dlm *DataLifecycleManager) GetJobStatus(jobID string) (*ArchivalJob, bool) {
	dlm.jobMu.RLock()
	defer dlm.jobMu.RUnlock()

	job, exists := dlm.jobs[jobID]
	return job, exists
}

// GetActiveJobs returns all active lifecycle jobs
func (dlm *DataLifecycleManager) GetActiveJobs() []*ArchivalJob {
	dlm.jobMu.RLock()
	defer dlm.jobMu.RUnlock()

	var activeJobs []*ArchivalJob
	for _, job := range dlm.jobs {
		if job.Status == "running" || job.Status == "pending" {
			activeJobs = append(activeJobs, job)
		}
	}

	return activeJobs
}

// ForceArchival manually triggers archival for a specific table
func (dlm *DataLifecycleManager) ForceArchival(ctx context.Context, tableName string) error {
	dlm.mu.RLock()
	policy, exists := dlm.policies[tableName]
	dlm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("no lifecycle policy found for table: %s", tableName)
	}

	return dlm.executePolicyForTable(ctx, policy)
}

// GetMetrics returns current lifecycle metrics
func (dlm *DataLifecycleManager) GetMetrics() map[string]interface{} {
	dlm.jobMu.RLock()
	defer dlm.jobMu.RUnlock()

	completed := 0
	failed := 0
	running := 0

	for _, job := range dlm.jobs {
		switch job.Status {
		case "completed":
			completed++
		case "failed":
			failed++
		case "running":
			running++
		}
	}

	return map[string]interface{}{
		"policies_count":  len(dlm.policies),
		"jobs_completed":  completed,
		"jobs_failed":     failed,
		"jobs_running":    running,
		"total_jobs":      len(dlm.jobs),
		"batch_size":      dlm.batchSize,
		"max_concurrency": dlm.maxConcurrency,
		"check_interval":  dlm.checkInterval,
	}
}
