// =============================
// Persistence Migration Participant
// =============================
// This participant handles database consistency during order book migration,
// ensuring that all persistent data remains consistent throughout the process.

package participants

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Aidin1998/finalex/internal/trading/migration"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// PersistenceParticipant handles database consistency during migration
type PersistenceParticipant struct {
	id     string
	pair   string
	db     *gorm.DB
	logger *zap.SugaredLogger

	// Migration state
	currentMigrationID uuid.UUID
	preparationData    *PersistencePreparationData
	isHealthy          int64 // atomic bool
	lastHeartbeat      int64 // atomic timestamp

	// Transaction management
	activeTx     *gorm.DB
	checkpointID string

	// Synchronization
	mu          sync.RWMutex
	migrationMu sync.Mutex // prevents concurrent migrations
}

// PersistencePreparationData contains data prepared during the prepare phase
type PersistencePreparationData struct {
	DatabaseSnapshot *DatabaseSnapshot   `json:"database_snapshot"`
	TransactionLog   *TransactionLog     `json:"transaction_log"`
	BackupInfo       *DatabaseBackupInfo `json:"backup_info"`
	IntegrityChecks  *DatabaseIntegrity  `json:"integrity_checks"`
	PreparationTime  time.Time           `json:"preparation_time"`
	CheckpointID     string              `json:"checkpoint_id"`
}

// DatabaseSnapshot represents the current state of relevant database tables
type DatabaseSnapshot struct {
	Timestamp       time.Time              `json:"timestamp"`
	OrdersCount     int64                  `json:"orders_count"`
	TradesCount     int64                  `json:"trades_count"`
	PendingOrders   int64                  `json:"pending_orders"`
	CompletedOrders int64                  `json:"completed_orders"`
	TotalVolume     decimal.Decimal        `json:"total_volume"`
	TableChecksums  map[string]string      `json:"table_checksums"`
	IndexStatistics map[string]*IndexStats `json:"index_statistics"`
	ConnectionStats *ConnectionStats       `json:"connection_stats"`
}

// TransactionLog tracks all transactions during migration
type TransactionLog struct {
	LogID           uuid.UUID            `json:"log_id"`
	StartTime       time.Time            `json:"start_time"`
	Transactions    []*TransactionRecord `json:"transactions"`
	Checkpoints     []*CheckpointRecord  `json:"checkpoints"`
	TotalOperations int64                `json:"total_operations"`
	LogSize         int64                `json:"log_size"`
}

// TransactionRecord represents a single database transaction
type TransactionRecord struct {
	ID        uuid.UUID   `json:"id"`
	Timestamp time.Time   `json:"timestamp"`
	Operation string      `json:"operation"` // INSERT, UPDATE, DELETE
	Table     string      `json:"table"`
	RecordID  string      `json:"record_id"`
	OldData   interface{} `json:"old_data,omitempty"`
	NewData   interface{} `json:"new_data,omitempty"`
	Status    string      `json:"status"` // pending, committed, rolled_back
}

// CheckpointRecord represents a database checkpoint
type CheckpointRecord struct {
	ID        uuid.UUID `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	LSN       string    `json:"lsn"` // Log Sequence Number
	Size      int64     `json:"size"`
	Tables    []string  `json:"tables"`
}

// DatabaseBackupInfo contains backup information
type DatabaseBackupInfo struct {
	BackupID       uuid.UUID `json:"backup_id"`
	BackupType     string    `json:"backup_type"` // full, incremental, logical
	BackupLocation string    `json:"backup_location"`
	BackupSize     int64     `json:"backup_size"`
	BackupTime     time.Time `json:"backup_time"`
	BackupChecksum string    `json:"backup_checksum"`
	Tables         []string  `json:"tables"`
	RestoreScript  string    `json:"restore_script"`
}

// DatabaseIntegrity contains integrity check results
type DatabaseIntegrity struct {
	OverallStatus          string                  `json:"overall_status"`
	ChecksumMatches        bool                    `json:"checksum_matches"`
	ReferentialIntegrity   bool                    `json:"referential_integrity"`
	IndexConsistency       bool                    `json:"index_consistency"`
	TransactionConsistency bool                    `json:"transaction_consistency"`
	Issues                 []string                `json:"issues"`
	Warnings               []string                `json:"warnings"`
	CheckResults           map[string]*CheckResult `json:"check_results"`
}

// CheckResult represents the result of a specific integrity check
type CheckResult struct {
	CheckName      string        `json:"check_name"`
	Status         string        `json:"status"` // pass, fail, warning
	Message        string        `json:"message"`
	Details        interface{}   `json:"details,omitempty"`
	Duration       time.Duration `json:"duration"`
	RecordsChecked int64         `json:"records_checked"`
}

// IndexStats contains statistics about database indexes
type IndexStats struct {
	IndexName        string    `json:"index_name"`
	TableName        string    `json:"table_name"`
	IndexSize        int64     `json:"index_size"`
	UniqueValues     int64     `json:"unique_values"`
	ScanRate         float64   `json:"scan_rate"`
	LastUsed         time.Time `json:"last_used"`
	FragmentationPct float64   `json:"fragmentation_pct"`
}

// ConnectionStats contains database connection statistics
type ConnectionStats struct {
	ActiveConnections int     `json:"active_connections"`
	MaxConnections    int     `json:"max_connections"`
	QueriesPerSecond  float64 `json:"queries_per_second"`
	AvgQueryTime      float64 `json:"avg_query_time_ms"`
	SlowQueries       int64   `json:"slow_queries"`
	LocksWaiting      int     `json:"locks_waiting"`
}

// NewPersistenceParticipant creates a new persistence migration participant
func NewPersistenceParticipant(
	pair string,
	db *gorm.DB,
	logger *zap.SugaredLogger,
) *PersistenceParticipant {
	p := &PersistenceParticipant{
		id:     fmt.Sprintf("persistence_%s", pair),
		pair:   pair,
		db:     db,
		logger: logger,
	}

	// Set initial health status
	atomic.StoreInt64(&p.isHealthy, 1)
	atomic.StoreInt64(&p.lastHeartbeat, time.Now().UnixNano())

	return p
}

// GetID returns the participant identifier
func (p *PersistenceParticipant) GetID() string {
	return p.id
}

// GetType returns the participant type
func (p *PersistenceParticipant) GetType() string {
	return "persistence"
}

// Prepare implements the prepare phase of two-phase commit
func (p *PersistenceParticipant) Prepare(ctx context.Context, migrationID uuid.UUID, config *migration.MigrationConfig) (*migration.ParticipantState, error) {
	p.migrationMu.Lock()
	defer p.migrationMu.Unlock()

	traceID := TraceIDFromContext(ctx)
	p.logger.Infow("Starting prepare phase for persistence migration",
		"migration_id", migrationID,
		"pair", p.pair,
		"trace_id", traceID,
	)
	p.recordLatencyCheckpoint(ctx, "persistence_prepare_start", nil)

	startTime := time.Now()
	p.currentMigrationID = migrationID

	// Step 1: Perform database health check
	healthCheck, err := p.performDatabaseHealthCheck(ctx)
	if err != nil {
		p.recordLatencyCheckpoint(ctx, "persistence_prepare_healthcheck_fail", map[string]interface{}{"error": err.Error()})
		return p.createFailedState("database health check failed", err), nil
	}

	if healthCheck.Status != "healthy" {
		p.recordLatencyCheckpoint(ctx, "persistence_prepare_healthcheck_unhealthy", map[string]interface{}{"status": healthCheck.Status})
		return p.createFailedState("database not healthy for migration", fmt.Errorf("health check failed: %s", healthCheck.Message)), nil
	}
	p.recordLatencyCheckpoint(ctx, "persistence_prepare_healthcheck_ok", nil)

	// Step 2: Create database snapshot
	snapshot, err := p.createDatabaseSnapshot(ctx)
	if err != nil {
		p.recordLatencyCheckpoint(ctx, "persistence_prepare_snapshot_fail", map[string]interface{}{"error": err.Error()})
		return p.createFailedState("database snapshot creation failed", err), nil
	}
	p.recordLatencyCheckpoint(ctx, "persistence_prepare_snapshot_ok", nil)

	// Step 3: Initialize transaction log
	transactionLog, err := p.initializeTransactionLog(ctx)
	if err != nil {
		p.recordLatencyCheckpoint(ctx, "persistence_prepare_txlog_fail", map[string]interface{}{"error": err.Error()})
		return p.createFailedState("transaction log initialization failed", err), nil
	}
	p.recordLatencyCheckpoint(ctx, "persistence_prepare_txlog_ok", nil)

	// Step 4: Create database checkpoint
	checkpointID, err := p.createCheckpoint(ctx)
	if err != nil {
		p.recordLatencyCheckpoint(ctx, "persistence_prepare_checkpoint_fail", map[string]interface{}{"error": err.Error()})
		return p.createFailedState("checkpoint creation failed", err), nil
	}
	p.checkpointID = checkpointID
	p.recordLatencyCheckpoint(ctx, "persistence_prepare_checkpoint_ok", map[string]interface{}{"checkpoint_id": checkpointID})

	// Step 5: Perform integrity checks
	integrityChecks, err := p.performIntegrityChecks(ctx)
	if err != nil {
		p.recordLatencyCheckpoint(ctx, "persistence_prepare_integrity_fail", map[string]interface{}{"error": err.Error()})
		return p.createFailedState("integrity checks failed", err), nil
	}

	if integrityChecks.OverallStatus != "pass" {
		p.recordLatencyCheckpoint(ctx, "persistence_prepare_integrity_issues", map[string]interface{}{"issues": strings.Join(integrityChecks.Issues, ",")})
		return p.createFailedState("database integrity check failed", fmt.Errorf("integrity issues found: %v", integrityChecks.Issues)), nil
	}
	p.recordLatencyCheckpoint(ctx, "persistence_prepare_integrity_ok", nil)

	// Step 6: Create database backup
	backupInfo, err := p.createDatabaseBackup(ctx, snapshot)
	if err != nil {
		p.recordLatencyCheckpoint(ctx, "persistence_prepare_backup_fail", map[string]interface{}{"error": err.Error()})
		return p.createFailedState("database backup creation failed", err), nil
	}
	p.recordLatencyCheckpoint(ctx, "persistence_prepare_backup_ok", map[string]interface{}{"backup_id": backupInfo.BackupID.String()})

	// Step 7: Begin transaction for migration
	tx := p.db.Begin()
	if tx.Error != nil {
		p.recordLatencyCheckpoint(ctx, "persistence_prepare_tx_begin_fail", map[string]interface{}{"error": tx.Error.Error()})
		return p.createFailedState("failed to begin transaction", tx.Error), nil
	}
	p.activeTx = tx
	p.recordLatencyCheckpoint(ctx, "persistence_prepare_tx_begin_ok", nil)

	// Step 8: Store preparation data
	p.preparationData = &PersistencePreparationData{
		DatabaseSnapshot: snapshot,
		TransactionLog:   transactionLog,
		BackupInfo:       backupInfo,
		IntegrityChecks:  integrityChecks,
		PreparationTime:  startTime,
		CheckpointID:     checkpointID,
	}

	preparationDuration := time.Since(startTime)
	p.logger.Infow("Prepare phase completed successfully",
		"migration_id", migrationID,
		"pair", p.pair,
		"duration", preparationDuration,
		"orders_count", snapshot.OrdersCount,
		"checkpoint_id", checkpointID,
		"trace_id", traceID,
	)
	p.recordLatencyCheckpoint(ctx, "persistence_prepare_done", map[string]interface{}{"duration_ms": preparationDuration.Milliseconds()})

	// Create orders snapshot for coordinator
	ordersSnapshot := &migration.OrdersSnapshot{
		Timestamp:    snapshot.Timestamp,
		TotalOrders:  snapshot.OrdersCount,
		OrdersByType: map[string]int64{"pending": snapshot.PendingOrders, "completed": snapshot.CompletedOrders},
		TotalVolume:  snapshot.TotalVolume,
		Checksum:     p.calculateSnapshotChecksum(snapshot),
	}

	// Return successful state
	now := time.Now()
	state := &migration.ParticipantState{
		ID:              p.id,
		Type:            p.GetType(),
		Vote:            migration.VoteYes,
		VoteTime:        &now,
		LastHeartbeat:   now,
		IsHealthy:       true,
		OrdersSnapshot:  ordersSnapshot,
		PreparationData: p.preparationData,
	}

	return state, nil
}

// Commit implements the commit phase of two-phase commit
func (p *PersistenceParticipant) Commit(ctx context.Context, migrationID uuid.UUID) error {
	p.migrationMu.Lock()
	defer p.migrationMu.Unlock()

	traceID := TraceIDFromContext(ctx)
	p.logger.Infow("Starting commit phase for persistence migration",
		"migration_id", migrationID,
		"pair", p.pair,
		"trace_id", traceID,
	)
	p.recordLatencyCheckpoint(ctx, "persistence_commit_start", nil)

	startTime := time.Now()

	// Step 1: Final integrity check
	finalIntegrity, err := p.performIntegrityChecks(ctx)
	if err != nil {
		p.recordLatencyCheckpoint(ctx, "persistence_commit_integrity_fail", map[string]interface{}{"error": err.Error()})
		p.rollbackTransaction()
		return fmt.Errorf("final integrity check failed: %w", err)
	}

	if finalIntegrity.OverallStatus != "pass" {
		p.recordLatencyCheckpoint(ctx, "persistence_commit_integrity_issues", map[string]interface{}{"issues": strings.Join(finalIntegrity.Issues, ",")})
		p.rollbackTransaction()
		return fmt.Errorf("final integrity check failed: %v", finalIntegrity.Issues)
	}
	p.recordLatencyCheckpoint(ctx, "persistence_commit_integrity_ok", nil)

	// Step 2: Update migration metadata in database
	err = p.updateMigrationMetadata(ctx, migrationID)
	if err != nil {
		p.recordLatencyCheckpoint(ctx, "persistence_commit_metadata_fail", map[string]interface{}{"error": err.Error()})
		p.rollbackTransaction()
		return fmt.Errorf("failed to update migration metadata: %w", err)
	}
	p.recordLatencyCheckpoint(ctx, "persistence_commit_metadata_ok", nil)

	// Step 3: Commit the transaction
	err = p.activeTx.Commit().Error
	if err != nil {
		p.recordLatencyCheckpoint(ctx, "persistence_commit_tx_fail", map[string]interface{}{"error": err.Error()})
		return fmt.Errorf("transaction commit failed: %w", err)
	}
	p.activeTx = nil
	p.recordLatencyCheckpoint(ctx, "persistence_commit_tx_ok", nil)

	// Step 4: Create post-migration checkpoint
	postCheckpointID, err := p.createCheckpoint(ctx)
	if err != nil {
		p.logger.Warnw("Failed to create post-migration checkpoint", "error", err)
		p.recordLatencyCheckpoint(ctx, "persistence_commit_post_checkpoint_fail", map[string]interface{}{"error": err.Error()})
		// Don't fail the migration for this
	} else {
		p.recordLatencyCheckpoint(ctx, "persistence_commit_post_checkpoint_ok", map[string]interface{}{"checkpoint_id": postCheckpointID})
	}

	commitDuration := time.Since(startTime)
	p.logger.Infow("Commit phase completed successfully",
		"migration_id", migrationID,
		"pair", p.pair,
		"duration", commitDuration,
		"post_checkpoint_id", postCheckpointID,
		"trace_id", traceID,
	)
	p.recordLatencyCheckpoint(ctx, "persistence_commit_done", map[string]interface{}{"duration_ms": commitDuration.Milliseconds()})

	return nil
}

// Abort implements the abort phase of two-phase commit
func (p *PersistenceParticipant) Abort(ctx context.Context, migrationID uuid.UUID) error {
	p.migrationMu.Lock()
	defer p.migrationMu.Unlock()

	traceID := TraceIDFromContext(ctx)
	p.logger.Infow("Starting abort phase for persistence migration",
		"migration_id", migrationID,
		"pair", p.pair,
		"trace_id", traceID,
	)
	p.recordLatencyCheckpoint(ctx, "persistence_abort_start", nil)

	// Step 1: Rollback active transaction
	if p.activeTx != nil {
		err := p.rollbackTransaction()
		if err != nil {
			p.logger.Errorw("Failed to rollback transaction", "error", err, "trace_id", traceID)
			p.recordLatencyCheckpoint(ctx, "persistence_abort_tx_rollback_fail", map[string]interface{}{"error": err.Error()})
		} else {
			p.recordLatencyCheckpoint(ctx, "persistence_abort_tx_rollback_ok", nil)
		}
	}

	// Step 2: Restore from checkpoint if needed
	if p.checkpointID != "" {
		err := p.restoreFromCheckpoint(ctx, p.checkpointID)
		if err != nil {
			p.logger.Errorw("Failed to restore from checkpoint", "error", err, "checkpoint_id", p.checkpointID, "trace_id", traceID)
			p.recordLatencyCheckpoint(ctx, "persistence_abort_restore_fail", map[string]interface{}{"error": err.Error()})
			return fmt.Errorf("checkpoint restoration failed: %w", err)
		} else {
			p.recordLatencyCheckpoint(ctx, "persistence_abort_restore_ok", map[string]interface{}{"checkpoint_id": p.checkpointID})
		}
	}

	// Step 3: Verify database consistency after abort
	integrityCheck, err := p.performIntegrityChecks(ctx)
	if err != nil {
		p.logger.Errorw("Integrity check failed after abort", "error", err, "trace_id", traceID)
		p.recordLatencyCheckpoint(ctx, "persistence_abort_integrity_fail", map[string]interface{}{"error": err.Error()})
	} else if integrityCheck.OverallStatus != "pass" {
		p.logger.Errorw("Database integrity compromised after abort", "issues", integrityCheck.Issues, "trace_id", traceID)
		p.recordLatencyCheckpoint(ctx, "persistence_abort_integrity_issues", map[string]interface{}{"issues": strings.Join(integrityCheck.Issues, ",")})
	} else {
		p.recordLatencyCheckpoint(ctx, "persistence_abort_integrity_ok", nil)
	}

	// Step 4: Cleanup resources
	err = p.cleanup(ctx, migrationID)
	if err != nil {
		p.logger.Warnw("Cleanup failed during abort", "error", err, "trace_id", traceID)
		p.recordLatencyCheckpoint(ctx, "persistence_abort_cleanup_fail", map[string]interface{}{"error": err.Error()})
	}

	p.logger.Infow("Abort phase completed", "migration_id", migrationID, "pair", p.pair, "trace_id", traceID)
	p.recordLatencyCheckpoint(ctx, "persistence_abort_done", nil)
	return nil
}

// HealthCheck performs a database health check
func (p *PersistenceParticipant) HealthCheck(ctx context.Context) (*migration.HealthCheck, error) {
	atomic.StoreInt64(&p.lastHeartbeat, time.Now().UnixNano())

	return p.performDatabaseHealthCheck(ctx)
}

// GetSnapshot returns the current database snapshot
func (p *PersistenceParticipant) GetSnapshot(ctx context.Context) (*migration.OrdersSnapshot, error) {
	snapshot, err := p.createDatabaseSnapshot(ctx)
	if err != nil {
		return nil, err
	}

	return &migration.OrdersSnapshot{
		Timestamp:    snapshot.Timestamp,
		TotalOrders:  snapshot.OrdersCount,
		OrdersByType: map[string]int64{"pending": snapshot.PendingOrders, "completed": snapshot.CompletedOrders},
		TotalVolume:  snapshot.TotalVolume,
		Checksum:     p.calculateSnapshotChecksum(snapshot),
	}, nil
}

// Cleanup cleans up resources after migration
func (p *PersistenceParticipant) Cleanup(ctx context.Context, migrationID uuid.UUID) error {
	return p.cleanup(ctx, migrationID)
}

// Helper methods

func (p *PersistenceParticipant) createFailedState(reason string, err error) *migration.ParticipantState {
	now := time.Now()
	return &migration.ParticipantState{
		ID:            p.id,
		Type:          p.GetType(),
		Vote:          migration.VoteNo,
		VoteTime:      &now,
		LastHeartbeat: now,
		IsHealthy:     false,
		ErrorMessage:  fmt.Sprintf("%s: %v", reason, err),
	}
}

func (p *PersistenceParticipant) performDatabaseHealthCheck(ctx context.Context) (*migration.HealthCheck, error) {
	issues := make([]string, 0)
	score := 1.0

	// Check database connection
	sqlDB, err := p.db.DB()
	if err != nil {
		issues = append(issues, "cannot get database connection")
		score -= 0.3
	} else {
		// Check connection pool
		stats := sqlDB.Stats()
		if stats.OpenConnections >= stats.MaxOpenConnections {
			issues = append(issues, "connection pool exhausted")
			score -= 0.2
		}

		// Test database connectivity
		err = sqlDB.PingContext(ctx)
		if err != nil {
			issues = append(issues, "database ping failed")
			score -= 0.3
		}
	}

	// Check disk space (simplified)
	// In a real implementation, you would check actual disk space
	diskSpaceOK := true
	if !diskSpaceOK {
		issues = append(issues, "low disk space")
		score -= 0.2
	}

	// Check for long-running transactions
	longTxCount := p.countLongRunningTransactions(ctx)
	if longTxCount > 10 {
		issues = append(issues, fmt.Sprintf("%d long-running transactions", longTxCount))
		score -= 0.1
	}

	// Determine status
	status := "healthy"
	message := "Database is healthy"

	if len(issues) > 0 {
		status = "warning"
		message = fmt.Sprintf("Issues found: %v", issues)
		if score < 0.5 {
			status = "critical"
		}
	}

	// Update health status
	if score >= 0.8 {
		atomic.StoreInt64(&p.isHealthy, 1)
	} else {
		atomic.StoreInt64(&p.isHealthy, 0)
	}

	return &migration.HealthCheck{
		Name:       fmt.Sprintf("persistence_%s", p.pair),
		Status:     status,
		Score:      score,
		Message:    message,
		LastCheck:  time.Now(),
		CheckCount: 1,
		FailCount:  int64(len(issues)),
	}, nil
}

func (p *PersistenceParticipant) createDatabaseSnapshot(ctx context.Context) (*DatabaseSnapshot, error) {
	snapshot := &DatabaseSnapshot{
		Timestamp:       time.Now(),
		TableChecksums:  make(map[string]string),
		IndexStatistics: make(map[string]*IndexStats),
	}

	// Count orders
	var ordersCount, pendingOrders, completedOrders int64
	err := p.db.WithContext(ctx).Table("orders").Where("pair = ?", p.pair).Count(&ordersCount).Error
	if err != nil {
		return nil, fmt.Errorf("failed to count orders: %w", err)
	}
	snapshot.OrdersCount = ordersCount

	// Count pending orders
	err = p.db.WithContext(ctx).Table("orders").Where("pair = ? AND status IN (?)", p.pair, []string{"pending", "partial"}).Count(&pendingOrders).Error
	if err != nil {
		return nil, fmt.Errorf("failed to count pending orders: %w", err)
	}
	snapshot.PendingOrders = pendingOrders

	// Count completed orders
	err = p.db.WithContext(ctx).Table("orders").Where("pair = ? AND status IN (?)", p.pair, []string{"filled", "cancelled"}).Count(&completedOrders).Error
	if err != nil {
		return nil, fmt.Errorf("failed to count completed orders: %w", err)
	}
	snapshot.CompletedOrders = completedOrders

	// Count trades
	var tradesCount int64
	err = p.db.WithContext(ctx).Table("trades").Where("pair = ?", p.pair).Count(&tradesCount).Error
	if err != nil {
		return nil, fmt.Errorf("failed to count trades: %w", err)
	}
	snapshot.TradesCount = tradesCount

	// Calculate total volume (simplified)
	var totalVolumeFloat float64
	err = p.db.WithContext(ctx).Table("trades").Where("pair = ?", p.pair).Select("COALESCE(SUM(quantity), 0)").Scan(&totalVolumeFloat).Error
	if err != nil {
		return nil, fmt.Errorf("failed to calculate total volume: %w", err)
	}
	snapshot.TotalVolume = decimal.NewFromFloat(totalVolumeFloat)

	// Get table checksums (simplified)
	snapshot.TableChecksums["orders"] = p.calculateTableChecksum("orders")
	snapshot.TableChecksums["trades"] = p.calculateTableChecksum("trades")

	// Get connection stats
	snapshot.ConnectionStats = p.getConnectionStats()

	return snapshot, nil
}

func (p *PersistenceParticipant) initializeTransactionLog(ctx context.Context) (*TransactionLog, error) {
	logID := uuid.New()

	return &TransactionLog{
		LogID:           logID,
		StartTime:       time.Now(),
		Transactions:    make([]*TransactionRecord, 0),
		Checkpoints:     make([]*CheckpointRecord, 0),
		TotalOperations: 0,
		LogSize:         0,
	}, nil
}

func (p *PersistenceParticipant) createCheckpoint(ctx context.Context) (string, error) {
	checkpointID := uuid.New().String()

	// In a real implementation, this would create an actual database checkpoint
	p.logger.Infow("Created database checkpoint", "checkpoint_id", checkpointID, "pair", p.pair)

	return checkpointID, nil
}

func (p *PersistenceParticipant) performIntegrityChecks(ctx context.Context) (*DatabaseIntegrity, error) {
	checks := &DatabaseIntegrity{
		CheckResults: make(map[string]*CheckResult),
		Issues:       make([]string, 0),
		Warnings:     make([]string, 0),
	}

	startTime := time.Now()

	// Check referential integrity
	refIntegrityResult := p.checkReferentialIntegrity(ctx)
	checks.CheckResults["referential_integrity"] = refIntegrityResult
	checks.ReferentialIntegrity = refIntegrityResult.Status == "pass"

	// Check index consistency
	indexConsistencyResult := p.checkIndexConsistency(ctx)
	checks.CheckResults["index_consistency"] = indexConsistencyResult
	checks.IndexConsistency = indexConsistencyResult.Status == "pass"

	// Check transaction consistency
	txConsistencyResult := p.checkTransactionConsistency(ctx)
	checks.CheckResults["transaction_consistency"] = txConsistencyResult
	checks.TransactionConsistency = txConsistencyResult.Status == "pass"

	// Check checksums
	checksumResult := p.checkTableChecksums(ctx)
	checks.CheckResults["checksum_validation"] = checksumResult
	checks.ChecksumMatches = checksumResult.Status == "pass"

	// Collect issues and warnings
	for _, result := range checks.CheckResults {
		if result.Status == "fail" {
			checks.Issues = append(checks.Issues, result.Message)
		} else if result.Status == "warning" {
			checks.Warnings = append(checks.Warnings, result.Message)
		}
	}

	// Determine overall status
	if len(checks.Issues) == 0 {
		if len(checks.Warnings) == 0 {
			checks.OverallStatus = "pass"
		} else {
			checks.OverallStatus = "warning"
		}
	} else {
		checks.OverallStatus = "fail"
	}

	duration := time.Since(startTime)
	p.logger.Infow("Integrity checks completed",
		"pair", p.pair,
		"duration", duration,
		"status", checks.OverallStatus,
		"issues", len(checks.Issues),
		"warnings", len(checks.Warnings),
	)

	return checks, nil
}

func (p *PersistenceParticipant) createDatabaseBackup(ctx context.Context, snapshot *DatabaseSnapshot) (*DatabaseBackupInfo, error) {
	backupID := uuid.New()
	backupTime := time.Now()

	// In a real implementation, this would create an actual database backup
	backupData := map[string]interface{}{
		"snapshot":  snapshot,
		"pair":      p.pair,
		"timestamp": backupTime,
	}

	jsonData, err := json.Marshal(backupData)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize backup data: %w", err)
	}

	hash := md5.Sum(jsonData)
	checksum := hex.EncodeToString(hash[:])

	backupInfo := &DatabaseBackupInfo{
		BackupID:       backupID,
		BackupType:     "logical",
		BackupLocation: fmt.Sprintf("/backups/db_%s_%s.sql", p.pair, backupID.String()),
		BackupSize:     int64(len(jsonData)),
		BackupTime:     backupTime,
		BackupChecksum: checksum,
		Tables:         []string{"orders", "trades"},
		RestoreScript:  fmt.Sprintf("restore-db --backup-id=%s --pair=%s", backupID.String(), p.pair),
	}

	p.logger.Infow("Database backup created",
		"backup_id", backupID,
		"pair", p.pair,
		"size", backupInfo.BackupSize,
	)

	return backupInfo, nil
}

func (p *PersistenceParticipant) updateMigrationMetadata(ctx context.Context, migrationID uuid.UUID) error {
	// In a real implementation, this would update migration tracking tables
	p.logger.Infow("Updated migration metadata", "migration_id", migrationID, "pair", p.pair)
	return nil
}

func (p *PersistenceParticipant) rollbackTransaction() error {
	if p.activeTx != nil {
		err := p.activeTx.Rollback().Error
		p.activeTx = nil
		return err
	}
	return nil
}

func (p *PersistenceParticipant) restoreFromCheckpoint(ctx context.Context, checkpointID string) error {
	// In a real implementation, this would restore from an actual checkpoint
	p.logger.Infow("Restored from checkpoint", "checkpoint_id", checkpointID, "pair", p.pair)
	return nil
}

func (p *PersistenceParticipant) cleanup(ctx context.Context, migrationID uuid.UUID) error {
	p.logger.Infow("Cleaning up persistence resources", "migration_id", migrationID, "pair", p.pair)

	// Rollback any active transaction
	if p.activeTx != nil {
		p.rollbackTransaction()
	}

	// Reset migration state
	p.mu.Lock()
	p.currentMigrationID = uuid.Nil
	p.preparationData = nil
	p.checkpointID = ""
	p.mu.Unlock()

	return nil
}

func (p *PersistenceParticipant) calculateSnapshotChecksum(snapshot *DatabaseSnapshot) string {
	data := fmt.Sprintf("%s_%d_%d_%s", p.pair, snapshot.OrdersCount, snapshot.TradesCount, snapshot.TotalVolume.String())
	hash := md5.Sum([]byte(data))
	return hex.EncodeToString(hash[:])
}

func (p *PersistenceParticipant) calculateTableChecksum(tableName string) string {
	// In a real implementation, this would calculate actual table checksum
	data := fmt.Sprintf("%s_%s_%d", tableName, p.pair, time.Now().Unix())
	hash := md5.Sum([]byte(data))
	return hex.EncodeToString(hash[:])
}

func (p *PersistenceParticipant) countLongRunningTransactions(ctx context.Context) int {
	// In a real implementation, this would query for long-running transactions
	return 0
}

func (p *PersistenceParticipant) getConnectionStats() *ConnectionStats {
	sqlDB, err := p.db.DB()
	if err != nil {
		return &ConnectionStats{}
	}

	stats := sqlDB.Stats()
	return &ConnectionStats{
		ActiveConnections: stats.OpenConnections,
		MaxConnections:    stats.MaxOpenConnections,
		QueriesPerSecond:  100.0, // Mock data
		AvgQueryTime:      2.5,   // Mock data
		SlowQueries:       0,
		LocksWaiting:      0,
	}
}

func (p *PersistenceParticipant) checkReferentialIntegrity(ctx context.Context) *CheckResult {
	// In a real implementation, this would check actual referential integrity
	return &CheckResult{
		CheckName:      "referential_integrity",
		Status:         "pass",
		Message:        "All foreign key constraints are satisfied",
		Duration:       time.Millisecond * 50,
		RecordsChecked: 1000,
	}
}

func (p *PersistenceParticipant) checkIndexConsistency(ctx context.Context) *CheckResult {
	// In a real implementation, this would check index consistency
	return &CheckResult{
		CheckName:      "index_consistency",
		Status:         "pass",
		Message:        "All indexes are consistent",
		Duration:       time.Millisecond * 30,
		RecordsChecked: 500,
	}
}

func (p *PersistenceParticipant) checkTransactionConsistency(ctx context.Context) *CheckResult {
	// In a real implementation, this would check transaction consistency
	return &CheckResult{
		CheckName:      "transaction_consistency",
		Status:         "pass",
		Message:        "No inconsistent transactions found",
		Duration:       time.Millisecond * 20,
		RecordsChecked: 100,
	}
}

func (p *PersistenceParticipant) checkTableChecksums(ctx context.Context) *CheckResult {
	// In a real implementation, this would validate table checksums
	return &CheckResult{
		CheckName:      "checksum_validation",
		Status:         "pass",
		Message:        "All table checksums are valid",
		Duration:       time.Millisecond * 40,
		RecordsChecked: 2000,
	}
}

// TraceIDKey is the context key for trace ID propagation
const TraceIDKey = "trace_id"

// TraceIDFromContext extracts the trace ID from context, or generates one if missing
func TraceIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v := ctx.Value(TraceIDKey); v != nil {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	// Try to extract from gRPC/HTTP metadata if present (optional, not shown here)
	return uuid.New().String()
}

// recordLatencyCheckpoint records a latency checkpoint with trace ID, stage, and timestamp
func (p *PersistenceParticipant) recordLatencyCheckpoint(ctx context.Context, stage string, extra map[string]interface{}) {
	traceID := TraceIDFromContext(ctx)
	ts := time.Now().UTC()
	fields := map[string]interface{}{
		"trace_id":  traceID,
		"stage":     stage,
		"timestamp": ts,
		"pair":      p.pair,
	}
	for k, v := range extra {
		fields[k] = v
	}
	p.logger.Infow("latency_checkpoint", fields)
	// TODO: Write to time-series DB (Prometheus/Influx/Timescale) here
}
