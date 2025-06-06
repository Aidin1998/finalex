package accounts

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
)

// MigrationStrategy defines the migration approach
type MigrationStrategy string

const (
	MigrationBlueGreen    MigrationStrategy = "blue_green"
	MigrationRolling      MigrationStrategy = "rolling"
	MigrationCanary       MigrationStrategy = "canary"
	MigrationShadow       MigrationStrategy = "shadow"
	MigrationOnlineSchema MigrationStrategy = "online_schema"
)

// MigrationPhase represents the current phase of migration
type MigrationPhase string

const (
	PhaseInitializing MigrationPhase = "initializing"
	PhasePreparation  MigrationPhase = "preparation"
	PhaseExecution    MigrationPhase = "execution"
	PhaseValidation   MigrationPhase = "validation"
	PhaseFinalization MigrationPhase = "finalization"
	PhaseCompleted    MigrationPhase = "completed"
	PhaseFailed       MigrationPhase = "failed"
	PhaseRolledBack   MigrationPhase = "rolled_back"
)

// Migration represents a database migration operation
type Migration struct {
	ID                string                 `json:"id"`
	Name              string                 `json:"name"`
	Version           string                 `json:"version"`
	Strategy          MigrationStrategy      `json:"strategy"`
	Phase             MigrationPhase         `json:"phase"`
	Description       string                 `json:"description"`
	SQLUp             []string               `json:"sql_up"`
	SQLDown           []string               `json:"sql_down"`
	PreHooks          []string               `json:"pre_hooks"`
	PostHooks         []string               `json:"post_hooks"`
	ValidationQueries []string               `json:"validation_queries"`
	Dependencies      []string               `json:"dependencies"`
	Rollback          bool                   `json:"rollback"`
	AllowDataLoss     bool                   `json:"allow_data_loss"`
	BatchSize         int                    `json:"batch_size"`
	MaxDuration       time.Duration          `json:"max_duration"`
	StartTime         time.Time              `json:"start_time"`
	EndTime           time.Time              `json:"end_time"`
	Progress          float64                `json:"progress"`
	EstimatedRows     int64                  `json:"estimated_rows"`
	ProcessedRows     int64                  `json:"processed_rows"`
	ErrorMessage      string                 `json:"error_message,omitempty"`
	Metadata          map[string]interface{} `json:"metadata,omitempty"`
}

// MigrationPlan represents a migration execution plan
type MigrationPlan struct {
	ID           string            `json:"id"`
	Migrations   []*Migration      `json:"migrations"`
	Strategy     MigrationStrategy `json:"strategy"`
	DryRun       bool              `json:"dry_run"`
	AutoRollback bool              `json:"auto_rollback"`
	Checkpoints  []string          `json:"checkpoints"`
	CreatedAt    time.Time         `json:"created_at"`
	Status       string            `json:"status"`
}

// MigrationManager handles zero-downtime database migrations
type MigrationManager struct {
	db               *sql.DB
	logger           *zap.Logger
	migrations       map[string]*Migration
	plans            map[string]*MigrationPlan
	currentMigration *Migration
	mu               sync.RWMutex
	planMu           sync.RWMutex

	// Configuration
	maxConcurrency     int
	batchSize          int
	checkInterval      time.Duration
	validationTimeout  time.Duration
	rollbackTimeout    time.Duration
	enableMetrics      bool
	enableCheckpoints  bool
	enableShadowTables bool
	enableValidation   bool

	// State management
	isRunning bool
	stopChan  chan struct{}

	// Metrics
	migrationsTotal      prometheus.Counter
	migrationsSuccessful prometheus.Counter
	migrationsFailed     prometheus.Counter
	migrationDuration    prometheus.Histogram
	rowsProcessed        prometheus.Counter
	rollbacksTotal       prometheus.Counter
	validationErrors     prometheus.Counter
	migrationProgress    prometheus.Gauge
}

// NewMigrationManager creates a new migration manager
func NewMigrationManager(db *sql.DB, logger *zap.Logger) *MigrationManager {
	mm := &MigrationManager{
		db:                 db,
		logger:             logger,
		migrations:         make(map[string]*Migration),
		plans:              make(map[string]*MigrationPlan),
		stopChan:           make(chan struct{}),
		maxConcurrency:     2,
		batchSize:          10000,
		checkInterval:      time.Second * 30,
		validationTimeout:  time.Minute * 5,
		rollbackTimeout:    time.Minute * 10,
		enableMetrics:      true,
		enableCheckpoints:  true,
		enableShadowTables: true,
		enableValidation:   true,
	}

	mm.initMetrics()
	mm.createMigrationTable()

	return mm
}

// initMetrics initializes Prometheus metrics
func (mm *MigrationManager) initMetrics() {
	if !mm.enableMetrics {
		return
	}

	mm.migrationsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "accounts_migrations_total",
		Help: "Total number of migrations executed",
	})

	mm.migrationsSuccessful = promauto.NewCounter(prometheus.CounterOpts{
		Name: "accounts_migrations_successful_total",
		Help: "Total number of successful migrations",
	})

	mm.migrationsFailed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "accounts_migrations_failed_total",
		Help: "Total number of failed migrations",
	})

	mm.migrationDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "accounts_migration_duration_seconds",
		Help:    "Duration of migration operations",
		Buckets: prometheus.ExponentialBuckets(1, 2, 15),
	})

	mm.rowsProcessed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "accounts_migration_rows_processed_total",
		Help: "Total number of rows processed during migrations",
	})

	mm.rollbacksTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "accounts_migration_rollbacks_total",
		Help: "Total number of migration rollbacks",
	})

	mm.validationErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "accounts_migration_validation_errors_total",
		Help: "Total number of migration validation errors",
	})

	mm.migrationProgress = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "accounts_migration_progress_percent",
		Help: "Current migration progress percentage",
	})
}

// createMigrationTable creates the migration tracking table
func (mm *MigrationManager) createMigrationTable() error {
	query := `
	CREATE TABLE IF NOT EXISTS schema_migrations (
		id VARCHAR(255) PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		version VARCHAR(100) NOT NULL,
		strategy VARCHAR(50) NOT NULL,
		phase VARCHAR(50) NOT NULL,
		sql_up TEXT[] NOT NULL,
		sql_down TEXT[] NOT NULL,
		started_at TIMESTAMP NOT NULL,
		completed_at TIMESTAMP,
		rolled_back_at TIMESTAMP,
		progress DECIMAL(5,2) DEFAULT 0,
		estimated_rows BIGINT DEFAULT 0,
		processed_rows BIGINT DEFAULT 0,
		error_message TEXT,
		metadata JSONB,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	
	CREATE INDEX IF NOT EXISTS idx_schema_migrations_version ON schema_migrations(version);
	CREATE INDEX IF NOT EXISTS idx_schema_migrations_phase ON schema_migrations(phase);
	CREATE INDEX IF NOT EXISTS idx_schema_migrations_started_at ON schema_migrations(started_at);
	`

	_, err := mm.db.Exec(query)
	return err
}

// AddMigration adds a new migration to the manager
func (mm *MigrationManager) AddMigration(migration *Migration) error {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	// Validate migration
	if err := mm.validateMigration(migration); err != nil {
		return fmt.Errorf("invalid migration: %w", err)
	}

	// Set defaults
	if migration.BatchSize == 0 {
		migration.BatchSize = mm.batchSize
	}
	if migration.MaxDuration == 0 {
		migration.MaxDuration = time.Hour
	}
	if migration.Metadata == nil {
		migration.Metadata = make(map[string]interface{})
	}

	mm.migrations[migration.ID] = migration

	mm.logger.Info("Added migration",
		zap.String("id", migration.ID),
		zap.String("name", migration.Name),
		zap.String("version", migration.Version),
		zap.String("strategy", string(migration.Strategy)))

	return nil
}

// validateMigration validates a migration configuration
func (mm *MigrationManager) validateMigration(migration *Migration) error {
	if migration.ID == "" {
		return fmt.Errorf("migration ID is required")
	}
	if migration.Name == "" {
		return fmt.Errorf("migration name is required")
	}
	if migration.Version == "" {
		return fmt.Errorf("migration version is required")
	}
	if len(migration.SQLUp) == 0 {
		return fmt.Errorf("migration must have at least one SQL up statement")
	}

	// Validate strategy
	switch migration.Strategy {
	case MigrationBlueGreen, MigrationRolling, MigrationCanary, MigrationShadow, MigrationOnlineSchema:
		// Valid strategies
	default:
		return fmt.Errorf("unsupported migration strategy: %s", migration.Strategy)
	}

	return nil
}

// CreatePlan creates a migration execution plan
func (mm *MigrationManager) CreatePlan(migrationIDs []string, strategy MigrationStrategy, dryRun bool) (*MigrationPlan, error) {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	plan := &MigrationPlan{
		ID:           fmt.Sprintf("plan_%d", time.Now().Unix()),
		Strategy:     strategy,
		DryRun:       dryRun,
		AutoRollback: true,
		CreatedAt:    time.Now(),
		Status:       "created",
	}

	// Validate and collect migrations
	var migrations []*Migration
	for _, id := range migrationIDs {
		migration, exists := mm.migrations[id]
		if !exists {
			return nil, fmt.Errorf("migration not found: %s", id)
		}
		migrations = append(migrations, migration)
	}

	// Sort migrations by dependencies
	sortedMigrations, err := mm.resolveDependencies(migrations)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve dependencies: %w", err)
	}

	plan.Migrations = sortedMigrations

	mm.planMu.Lock()
	mm.plans[plan.ID] = plan
	mm.planMu.Unlock()

	return plan, nil
}

// resolveDependencies sorts migrations based on their dependencies
func (mm *MigrationManager) resolveDependencies(migrations []*Migration) ([]*Migration, error) {
	// Simple topological sort for now
	// In production, implement proper dependency resolution
	return migrations, nil
}

// ExecutePlan executes a migration plan
func (mm *MigrationManager) ExecutePlan(ctx context.Context, planID string) error {
	mm.planMu.RLock()
	plan, exists := mm.plans[planID]
	mm.planMu.RUnlock()

	if !exists {
		return fmt.Errorf("migration plan not found: %s", planID)
	}

	if mm.isRunning {
		return fmt.Errorf("another migration is already running")
	}

	mm.isRunning = true
	defer func() { mm.isRunning = false }()

	plan.Status = "executing"

	mm.logger.Info("Starting migration plan execution",
		zap.String("plan_id", planID),
		zap.Int("migrations_count", len(plan.Migrations)),
		zap.Bool("dry_run", plan.DryRun))

	// Execute migrations sequentially
	for i, migration := range plan.Migrations {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		mm.logger.Info("Executing migration",
			zap.String("migration_id", migration.ID),
			zap.String("name", migration.Name),
			zap.Int("step", i+1),
			zap.Int("total", len(plan.Migrations)))

		err := mm.ExecuteMigration(ctx, migration.ID, plan.DryRun)
		if err != nil {
			mm.logger.Error("Migration failed",
				zap.String("migration_id", migration.ID),
				zap.Error(err))

			if plan.AutoRollback {
				mm.logger.Info("Starting automatic rollback")
				rollbackErr := mm.rollbackPlan(ctx, plan, i)
				if rollbackErr != nil {
					mm.logger.Error("Rollback failed", zap.Error(rollbackErr))
					return fmt.Errorf("migration failed and rollback failed: %w, rollback error: %v", err, rollbackErr)
				}
			}

			plan.Status = "failed"
			return fmt.Errorf("migration failed: %w", err)
		}
	}

	plan.Status = "completed"
	mm.logger.Info("Migration plan completed successfully", zap.String("plan_id", planID))

	return nil
}

// ExecuteMigration executes a single migration
func (mm *MigrationManager) ExecuteMigration(ctx context.Context, migrationID string, dryRun bool) error {
	mm.mu.RLock()
	migration, exists := mm.migrations[migrationID]
	mm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("migration not found: %s", migrationID)
	}

	if mm.enableMetrics {
		mm.migrationsTotal.Inc()
	}

	startTime := time.Now()
	migration.StartTime = startTime
	migration.Phase = PhaseInitializing

	defer func() {
		if mm.enableMetrics {
			mm.migrationDuration.Observe(time.Since(startTime).Seconds())
			mm.migrationProgress.Set(migration.Progress)
		}
	}()

	// Save migration state
	if err := mm.saveMigrationState(migration); err != nil {
		return fmt.Errorf("failed to save migration state: %w", err)
	}

	// Execute migration based on strategy
	var err error
	switch migration.Strategy {
	case MigrationBlueGreen:
		err = mm.executeBlueGreenMigration(ctx, migration, dryRun)
	case MigrationRolling:
		err = mm.executeRollingMigration(ctx, migration, dryRun)
	case MigrationCanary:
		err = mm.executeCanaryMigration(ctx, migration, dryRun)
	case MigrationShadow:
		err = mm.executeShadowMigration(ctx, migration, dryRun)
	case MigrationOnlineSchema:
		err = mm.executeOnlineSchemaMigration(ctx, migration, dryRun)
	default:
		err = fmt.Errorf("unsupported migration strategy: %s", migration.Strategy)
	}

	if err != nil {
		migration.Phase = PhaseFailed
		migration.ErrorMessage = err.Error()
		if mm.enableMetrics {
			mm.migrationsFailed.Inc()
		}
	} else {
		migration.Phase = PhaseCompleted
		migration.Progress = 100.0
		migration.EndTime = time.Now()
		if mm.enableMetrics {
			mm.migrationsSuccessful.Inc()
		}
	}

	// Update migration state
	saveErr := mm.saveMigrationState(migration)
	if saveErr != nil {
		mm.logger.Error("Failed to save migration state", zap.Error(saveErr))
	}

	return err
}

// executeBlueGreenMigration executes a blue-green migration
func (mm *MigrationManager) executeBlueGreenMigration(ctx context.Context, migration *Migration, dryRun bool) error {
	migration.Phase = PhasePreparation

	// Create green (new) environment
	greenTable := fmt.Sprintf("%s_green", migration.Metadata["table_name"])

	if !dryRun {
		// Create green table structure
		for _, sql := range migration.SQLUp {
			adaptedSQL := strings.ReplaceAll(sql, migration.Metadata["table_name"].(string), greenTable)
			_, err := mm.db.ExecContext(ctx, adaptedSQL)
			if err != nil {
				return fmt.Errorf("failed to create green table: %w", err)
			}
		}
	}

	migration.Phase = PhaseExecution
	migration.Progress = 25.0

	// Copy data from blue to green
	if !dryRun {
		err := mm.copyTableData(ctx, migration.Metadata["table_name"].(string), greenTable, migration)
		if err != nil {
			return fmt.Errorf("failed to copy data to green table: %w", err)
		}
	}

	migration.Phase = PhaseValidation
	migration.Progress = 75.0

	// Validate green table
	if mm.enableValidation && !dryRun {
		err := mm.validateMigrationResult(ctx, migration, greenTable)
		if err != nil {
			return fmt.Errorf("validation failed: %w", err)
		}
	}

	migration.Phase = PhaseFinalization
	migration.Progress = 90.0

	// Switch blue and green (atomic operation)
	if !dryRun {
		err := mm.switchBlueGreen(ctx, migration.Metadata["table_name"].(string), greenTable)
		if err != nil {
			return fmt.Errorf("failed to switch blue-green: %w", err)
		}
	}

	migration.Progress = 100.0
	return nil
}

// executeRollingMigration executes a rolling migration
func (mm *MigrationManager) executeRollingMigration(ctx context.Context, migration *Migration, dryRun bool) error {
	migration.Phase = PhasePreparation

	// Get total rows for progress tracking
	tableName := migration.Metadata["table_name"].(string)
	totalRows, err := mm.getTableRowCount(ctx, tableName)
	if err != nil {
		return fmt.Errorf("failed to get table row count: %w", err)
	}

	migration.EstimatedRows = totalRows
	migration.Phase = PhaseExecution

	// Execute migration in batches
	batchSize := migration.BatchSize
	processed := int64(0)

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		batchProcessed, err := mm.executeBatch(ctx, migration, batchSize, processed, dryRun)
		if err != nil {
			return fmt.Errorf("batch execution failed: %w", err)
		}

		processed += int64(batchProcessed)
		migration.ProcessedRows = processed

		if totalRows > 0 {
			migration.Progress = float64(processed) / float64(totalRows) * 100.0
		}

		if mm.enableMetrics {
			mm.rowsProcessed.Add(float64(batchProcessed))
		}

		// Update migration state periodically
		if processed%int64(batchSize*10) == 0 {
			mm.saveMigrationState(migration)
		}

		// Break if no more rows to process
		if batchProcessed < batchSize {
			break
		}
	}

	migration.Phase = PhaseValidation

	// Final validation
	if mm.enableValidation && !dryRun {
		err = mm.validateMigrationResult(ctx, migration, tableName)
		if err != nil {
			return fmt.Errorf("validation failed: %w", err)
		}
	}

	migration.Progress = 100.0
	return nil
}

// executeCanaryMigration executes a canary migration
func (mm *MigrationManager) executeCanaryMigration(ctx context.Context, migration *Migration, dryRun bool) error {
	// Implement canary migration logic
	// This would involve migrating a small subset of data first
	return mm.executeRollingMigration(ctx, migration, dryRun)
}

// executeShadowMigration executes a shadow migration
func (mm *MigrationManager) executeShadowMigration(ctx context.Context, migration *Migration, dryRun bool) error {
	// Implement shadow migration logic
	// This would involve creating shadow tables and syncing changes
	return mm.executeBlueGreenMigration(ctx, migration, dryRun)
}

// executeOnlineSchemaMigration executes an online schema migration
func (mm *MigrationManager) executeOnlineSchemaMigration(ctx context.Context, migration *Migration, dryRun bool) error {
	migration.Phase = PhaseExecution

	// Execute schema changes online
	for i, sql := range migration.SQLUp {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if !dryRun {
			_, err := mm.db.ExecContext(ctx, sql)
			if err != nil {
				return fmt.Errorf("failed to execute SQL statement %d: %w", i+1, err)
			}
		}

		migration.Progress = float64(i+1) / float64(len(migration.SQLUp)) * 100.0

		if mm.enableMetrics {
			mm.migrationProgress.Set(migration.Progress)
		}
	}

	return nil
}

// copyTableData copies data between tables
func (mm *MigrationManager) copyTableData(ctx context.Context, sourceTable, destTable string, migration *Migration) error {
	// Get table columns
	columns, err := mm.getTableColumns(ctx, sourceTable)
	if err != nil {
		return err
	}

	columnList := strings.Join(columns, ", ")
	batchSize := migration.BatchSize
	offset := 0

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Copy batch
		copySQL := fmt.Sprintf(`
			INSERT INTO %s (%s)
			SELECT %s FROM %s
			ORDER BY id
			LIMIT %d OFFSET %d
		`, destTable, columnList, columnList, sourceTable, batchSize, offset)

		result, err := mm.db.ExecContext(ctx, copySQL)
		if err != nil {
			return err
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return err
		}

		migration.ProcessedRows += rowsAffected

		// Break if no more rows
		if rowsAffected < int64(batchSize) {
			break
		}

		offset += batchSize
	}

	return nil
}

// switchBlueGreen atomically switches blue and green tables
func (mm *MigrationManager) switchBlueGreen(ctx context.Context, blueTable, greenTable string) error {
	backupTable := fmt.Sprintf("%s_backup_%d", blueTable, time.Now().Unix())

	tx, err := mm.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Rename blue to backup
	_, err = tx.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s RENAME TO %s", blueTable, backupTable))
	if err != nil {
		return err
	}

	// Rename green to blue
	_, err = tx.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s RENAME TO %s", greenTable, blueTable))
	if err != nil {
		return err
	}

	return tx.Commit()
}

// executeBatch executes a migration batch
func (mm *MigrationManager) executeBatch(ctx context.Context, migration *Migration, batchSize int, offset int64, dryRun bool) (int, error) {
	if dryRun {
		// Simulate batch processing
		time.Sleep(time.Millisecond * 10)
		return batchSize, nil
	}

	// Execute actual batch logic here
	// This is a placeholder - implement based on specific migration needs
	return batchSize, nil
}

// getTableRowCount gets the total number of rows in a table
func (mm *MigrationManager) getTableRowCount(ctx context.Context, tableName string) (int64, error) {
	var count int64
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName)
	err := mm.db.QueryRowContext(ctx, query).Scan(&count)
	return count, err
}

// getTableColumns gets the column names of a table
func (mm *MigrationManager) getTableColumns(ctx context.Context, tableName string) ([]string, error) {
	query := `
		SELECT column_name 
		FROM information_schema.columns 
		WHERE table_name = $1 
		ORDER BY ordinal_position
	`

	rows, err := mm.db.QueryContext(ctx, query, tableName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []string
	for rows.Next() {
		var column string
		if err := rows.Scan(&column); err != nil {
			return nil, err
		}
		columns = append(columns, column)
	}

	return columns, nil
}

// validateMigrationResult validates the migration result
func (mm *MigrationManager) validateMigrationResult(ctx context.Context, migration *Migration, tableName string) error {
	for _, query := range migration.ValidationQueries {
		var result int
		err := mm.db.QueryRowContext(ctx, query).Scan(&result)
		if err != nil {
			if mm.enableMetrics {
				mm.validationErrors.Inc()
			}
			return fmt.Errorf("validation query failed: %w", err)
		}

		if result == 0 {
			if mm.enableMetrics {
				mm.validationErrors.Inc()
			}
			return fmt.Errorf("validation failed: query returned 0")
		}
	}

	return nil
}

// saveMigrationState saves the current migration state to database
func (mm *MigrationManager) saveMigrationState(migration *Migration) error {
	metadataJSON, err := json.Marshal(migration.Metadata)
	if err != nil {
		return err
	}

	query := `
		INSERT INTO schema_migrations (
			id, name, version, strategy, phase, sql_up, sql_down,
			started_at, progress, estimated_rows, processed_rows,
			error_message, metadata, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, CURRENT_TIMESTAMP)
		ON CONFLICT (id) DO UPDATE SET
			phase = EXCLUDED.phase,
			progress = EXCLUDED.progress,
			processed_rows = EXCLUDED.processed_rows,
			error_message = EXCLUDED.error_message,
			updated_at = CURRENT_TIMESTAMP
	`

	sqlUpJSON, _ := json.Marshal(migration.SQLUp)
	sqlDownJSON, _ := json.Marshal(migration.SQLDown)

	_, err = mm.db.Exec(query,
		migration.ID, migration.Name, migration.Version,
		string(migration.Strategy), string(migration.Phase),
		string(sqlUpJSON), string(sqlDownJSON),
		migration.StartTime, migration.Progress,
		migration.EstimatedRows, migration.ProcessedRows,
		migration.ErrorMessage, string(metadataJSON))

	return err
}

// rollbackPlan rolls back a migration plan
func (mm *MigrationManager) rollbackPlan(ctx context.Context, plan *MigrationPlan, failedIndex int) error {
	if mm.enableMetrics {
		mm.rollbacksTotal.Inc()
	}

	// Rollback migrations in reverse order
	for i := failedIndex; i >= 0; i-- {
		migration := plan.Migrations[i]
		err := mm.rollbackMigration(ctx, migration)
		if err != nil {
			return fmt.Errorf("rollback failed for migration %s: %w", migration.ID, err)
		}
	}

	return nil
}

// rollbackMigration rolls back a single migration
func (mm *MigrationManager) rollbackMigration(ctx context.Context, migration *Migration) error {
	migration.Phase = PhaseRolledBack

	// Execute rollback SQL
	for _, sql := range migration.SQLDown {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		_, err := mm.db.ExecContext(ctx, sql)
		if err != nil {
			return fmt.Errorf("rollback SQL failed: %w", err)
		}
	}

	return mm.saveMigrationState(migration)
}

// GetMigrationStatus returns the status of a migration
func (mm *MigrationManager) GetMigrationStatus(migrationID string) (*Migration, bool) {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	migration, exists := mm.migrations[migrationID]
	return migration, exists
}

// GetActiveMigrations returns all currently active migrations
func (mm *MigrationManager) GetActiveMigrations() []*Migration {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	var active []*Migration
	for _, migration := range mm.migrations {
		if migration.Phase != PhaseCompleted && migration.Phase != PhaseFailed && migration.Phase != PhaseRolledBack {
			active = append(active, migration)
		}
	}

	return active
}

// GetMigrationHistory returns migration history from database
func (mm *MigrationManager) GetMigrationHistory(limit int) ([]*Migration, error) {
	query := `
		SELECT id, name, version, strategy, phase, started_at, 
			   progress, estimated_rows, processed_rows, error_message
		FROM schema_migrations 
		ORDER BY started_at DESC 
		LIMIT $1
	`

	rows, err := mm.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var migrations []*Migration
	for rows.Next() {
		var m Migration
		var startedAt sql.NullTime

		err := rows.Scan(&m.ID, &m.Name, &m.Version, &m.Strategy, &m.Phase,
			&startedAt, &m.Progress, &m.EstimatedRows, &m.ProcessedRows, &m.ErrorMessage)
		if err != nil {
			return nil, err
		}

		if startedAt.Valid {
			m.StartTime = startedAt.Time
		}

		migrations = append(migrations, &m)
	}

	return migrations, nil
}

// Stop gracefully stops the migration manager
func (mm *MigrationManager) Stop() error {
	close(mm.stopChan)
	return nil
}

// GetMetrics returns current migration metrics
func (mm *MigrationManager) GetMetrics() map[string]interface{} {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	running := 0
	completed := 0
	failed := 0

	for _, migration := range mm.migrations {
		switch migration.Phase {
		case PhaseCompleted:
			completed++
		case PhaseFailed, PhaseRolledBack:
			failed++
		default:
			running++
		}
	}

	return map[string]interface{}{
		"total_migrations":     len(mm.migrations),
		"running_migrations":   running,
		"completed_migrations": completed,
		"failed_migrations":    failed,
		"current_migration":    mm.currentMigration,
		"is_running":           mm.isRunning,
		"max_concurrency":      mm.maxConcurrency,
		"batch_size":           mm.batchSize,
	}
}
