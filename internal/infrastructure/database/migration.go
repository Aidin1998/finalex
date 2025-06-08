// Package database provides migration management for database schema evolution
package database

import (
	"context"
	"fmt"
	"sort"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Migration represents a database migration
type Migration struct {
	Version     string    `gorm:"primaryKey" json:"version"`
	Description string    `json:"description"`
	SQL         string    `gorm:"type:text" json:"sql"`
	Checksum    string    `json:"checksum"`
	AppliedAt   time.Time `json:"applied_at"`
	AppliedBy   string    `json:"applied_by"`
	Success     bool      `json:"success"`
	ErrorMsg    string    `gorm:"type:text" json:"error_msg,omitempty"`
}

// MigrationManager handles database migrations
type MigrationManager struct {
	db          *gorm.DB
	logger      *zap.Logger
	migrations  []MigrationScript
	tableName   string
	lockTimeout time.Duration
}

// MigrationScript represents a migration script
type MigrationScript struct {
	Version     string
	Description string
	UpSQL       string
	DownSQL     string
	Checksum    string
}

// MigrationOptions configure migration behavior
type MigrationOptions struct {
	TableName      string
	LockTimeout    time.Duration
	DryRun         bool
	SkipValidation bool
}

// NewMigrationManager creates a new migration manager
func NewMigrationManager(db *gorm.DB, logger *zap.Logger, opts *MigrationOptions) *MigrationManager {
	if opts == nil {
		opts = &MigrationOptions{
			TableName:   "schema_migrations",
			LockTimeout: 30 * time.Second,
		}
	}

	return &MigrationManager{
		db:          db,
		logger:      logger,
		migrations:  make([]MigrationScript, 0),
		tableName:   opts.TableName,
		lockTimeout: opts.LockTimeout,
	}
}

// RegisterMigration registers a new migration script
func (mm *MigrationManager) RegisterMigration(script MigrationScript) error {
	if script.Version == "" {
		return fmt.Errorf("migration version cannot be empty")
	}

	if script.UpSQL == "" {
		return fmt.Errorf("migration up SQL cannot be empty")
	}

	// Check for duplicate versions
	for _, existing := range mm.migrations {
		if existing.Version == script.Version {
			return fmt.Errorf("migration version %s already registered", script.Version)
		}
	}

	mm.migrations = append(mm.migrations, script)
	mm.logger.Debug("migration registered",
		zap.String("version", script.Version),
		zap.String("description", script.Description))

	return nil
}

// InitializeSchema initializes the migration schema table
func (mm *MigrationManager) InitializeSchema(ctx context.Context) error {
	// Create migrations table if it doesn't exist
	if err := mm.db.WithContext(ctx).AutoMigrate(&Migration{}); err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	mm.logger.Info("migration schema initialized", zap.String("table", mm.tableName))
	return nil
}

// GetPendingMigrations returns migrations that haven't been applied
func (mm *MigrationManager) GetPendingMigrations(ctx context.Context) ([]MigrationScript, error) {
	// Get applied migrations
	var appliedMigrations []Migration
	if err := mm.db.WithContext(ctx).
		Where("success = ?", true).
		Find(&appliedMigrations).Error; err != nil {
		return nil, fmt.Errorf("failed to query applied migrations: %w", err)
	}

	// Create map of applied versions
	appliedVersions := make(map[string]bool)
	for _, migration := range appliedMigrations {
		appliedVersions[migration.Version] = true
	}

	// Filter pending migrations
	var pending []MigrationScript
	for _, migration := range mm.migrations {
		if !appliedVersions[migration.Version] {
			pending = append(pending, migration)
		}
	}

	// Sort by version
	sort.Slice(pending, func(i, j int) bool {
		return pending[i].Version < pending[j].Version
	})

	return pending, nil
}

// GetAppliedMigrations returns all successfully applied migrations
func (mm *MigrationManager) GetAppliedMigrations(ctx context.Context) ([]Migration, error) {
	var migrations []Migration
	if err := mm.db.WithContext(ctx).
		Where("success = ?", true).
		Order("applied_at ASC").
		Find(&migrations).Error; err != nil {
		return nil, fmt.Errorf("failed to query applied migrations: %w", err)
	}

	return migrations, nil
}

// Migrate applies all pending migrations
func (mm *MigrationManager) Migrate(ctx context.Context) error {
	pending, err := mm.GetPendingMigrations(ctx)
	if err != nil {
		return fmt.Errorf("failed to get pending migrations: %w", err)
	}

	if len(pending) == 0 {
		mm.logger.Info("no pending migrations")
		return nil
	}

	mm.logger.Info("applying migrations", zap.Int("count", len(pending)))

	for _, migration := range pending {
		if err := mm.applyMigration(ctx, migration); err != nil {
			return fmt.Errorf("failed to apply migration %s: %w", migration.Version, err)
		}
	}

	mm.logger.Info("all migrations applied successfully")
	return nil
}

// MigrateTo applies migrations up to a specific version
func (mm *MigrationManager) MigrateTo(ctx context.Context, targetVersion string) error {
	pending, err := mm.GetPendingMigrations(ctx)
	if err != nil {
		return fmt.Errorf("failed to get pending migrations: %w", err)
	}

	var toApply []MigrationScript
	for _, migration := range pending {
		if migration.Version <= targetVersion {
			toApply = append(toApply, migration)
		}
	}

	if len(toApply) == 0 {
		mm.logger.Info("no migrations to apply", zap.String("target_version", targetVersion))
		return nil
	}

	mm.logger.Info("applying migrations to version",
		zap.String("target_version", targetVersion),
		zap.Int("count", len(toApply)))

	for _, migration := range toApply {
		if err := mm.applyMigration(ctx, migration); err != nil {
			return fmt.Errorf("failed to apply migration %s: %w", migration.Version, err)
		}
	}

	return nil
}

// Rollback rolls back the last applied migration
func (mm *MigrationManager) Rollback(ctx context.Context) error {
	// Get the last applied migration
	var lastMigration Migration
	if err := mm.db.WithContext(ctx).
		Where("success = ?", true).
		Order("applied_at DESC").
		First(&lastMigration).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			mm.logger.Info("no migrations to rollback")
			return nil
		}
		return fmt.Errorf("failed to find last migration: %w", err)
	}

	// Find the migration script
	var script *MigrationScript
	for _, migration := range mm.migrations {
		if migration.Version == lastMigration.Version {
			script = &migration
			break
		}
	}

	if script == nil {
		return fmt.Errorf("migration script not found for version %s", lastMigration.Version)
	}

	if script.DownSQL == "" {
		return fmt.Errorf("no rollback SQL defined for migration %s", lastMigration.Version)
	}

	return mm.rollbackMigration(ctx, *script)
}

// RollbackTo rolls back migrations to a specific version
func (mm *MigrationManager) RollbackTo(ctx context.Context, targetVersion string) error {
	applied, err := mm.GetAppliedMigrations(ctx)
	if err != nil {
		return fmt.Errorf("failed to get applied migrations: %w", err)
	}

	// Find migrations to rollback (in reverse order)
	var toRollback []Migration
	for i := len(applied) - 1; i >= 0; i-- {
		migration := applied[i]
		if migration.Version > targetVersion {
			toRollback = append(toRollback, migration)
		}
	}

	if len(toRollback) == 0 {
		mm.logger.Info("no migrations to rollback", zap.String("target_version", targetVersion))
		return nil
	}

	mm.logger.Info("rolling back migrations to version",
		zap.String("target_version", targetVersion),
		zap.Int("count", len(toRollback)))

	// Find the corresponding migration scripts
	for _, migration := range toRollback {
		var script *MigrationScript
		for _, s := range mm.migrations {
			if s.Version == migration.Version {
				script = &s
				break
			}
		}

		if script == nil {
			return fmt.Errorf("migration script not found for version %s", migration.Version)
		}

		if script.DownSQL == "" {
			return fmt.Errorf("no rollback SQL available for migration %s", migration.Version)
		}

		if err := mm.rollbackMigration(ctx, *script); err != nil {
			return fmt.Errorf("failed to rollback migration %s: %w", migration.Version, err)
		}
	}

	mm.logger.Info("rollback completed successfully", zap.String("target_version", targetVersion))
	return nil
}

// applyMigration applies a single migration
func (mm *MigrationManager) applyMigration(ctx context.Context, script MigrationScript) error {
	migration := Migration{
		Version:     script.Version,
		Description: script.Description,
		SQL:         script.UpSQL,
		Checksum:    script.Checksum,
		AppliedAt:   time.Now(),
		AppliedBy:   "system", // TODO: Get from context
		Success:     false,
	}

	// Start transaction
	tx := mm.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return fmt.Errorf("failed to start transaction: %w", tx.Error)
	}

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			mm.logger.Error("migration panicked",
				zap.String("version", script.Version),
				zap.Any("panic", r))
		}
	}()

	// Apply migration SQL
	if err := tx.Exec(script.UpSQL).Error; err != nil {
		migration.ErrorMsg = err.Error()

		// Save failed migration record
		if saveErr := tx.Create(&migration).Error; saveErr != nil {
			mm.logger.Error("failed to save failed migration record",
				zap.String("version", script.Version),
				zap.Error(saveErr))
		}

		tx.Rollback()
		return fmt.Errorf("failed to execute migration SQL: %w", err)
	}

	// Mark as successful
	migration.Success = true

	// Save migration record
	if err := tx.Create(&migration).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to save migration record: %w", err)
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit migration transaction: %w", err)
	}

	mm.logger.Info("migration applied successfully",
		zap.String("version", script.Version),
		zap.String("description", script.Description))

	return nil
}

// rollbackMigration rolls back a single migration
func (mm *MigrationManager) rollbackMigration(ctx context.Context, script MigrationScript) error {
	mm.logger.Info("rolling back migration",
		zap.String("version", script.Version),
		zap.String("description", script.Description))

	// Start transaction
	tx := mm.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return fmt.Errorf("failed to start transaction: %w", tx.Error)
	}

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			mm.logger.Error("rollback panicked",
				zap.String("version", script.Version),
				zap.Any("panic", r))
		}
	}()

	// Execute rollback SQL
	if err := tx.Exec(script.DownSQL).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to execute rollback SQL: %w", err)
	}

	// Remove migration record
	if err := tx.Where("version = ?", script.Version).Delete(&Migration{}).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to remove migration record: %w", err)
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit rollback transaction: %w", err)
	}

	mm.logger.Info("migration rolled back successfully",
		zap.String("version", script.Version))

	return nil
}

// ValidateMigrations validates all registered migrations
func (mm *MigrationManager) ValidateMigrations(ctx context.Context) error {
	// Get applied migrations
	applied, err := mm.GetAppliedMigrations(ctx)
	if err != nil {
		return fmt.Errorf("failed to get applied migrations: %w", err)
	}

	// Check for checksum mismatches
	appliedMap := make(map[string]Migration)
	for _, migration := range applied {
		appliedMap[migration.Version] = migration
	}

	for _, script := range mm.migrations {
		if appliedMigration, exists := appliedMap[script.Version]; exists {
			if appliedMigration.Checksum != script.Checksum {
				return fmt.Errorf("checksum mismatch for migration %s: expected %s, got %s",
					script.Version, script.Checksum, appliedMigration.Checksum)
			}
		}
	}

	mm.logger.Info("migration validation successful")
	return nil
}

// GetMigrationStatus returns the current migration status
func (mm *MigrationManager) GetMigrationStatus(ctx context.Context) (*MigrationStatus, error) {
	applied, err := mm.GetAppliedMigrations(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get applied migrations: %w", err)
	}

	pending, err := mm.GetPendingMigrations(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get pending migrations: %w", err)
	}

	var currentVersion string
	if len(applied) > 0 {
		currentVersion = applied[len(applied)-1].Version
	}

	return &MigrationStatus{
		CurrentVersion:       currentVersion,
		AppliedCount:         len(applied),
		PendingCount:         len(pending),
		TotalCount:           len(mm.migrations),
		LastAppliedAt:        getLastAppliedTime(applied),
		HasPendingMigrations: len(pending) > 0,
	}, nil
}

// MigrationStatus represents the current migration status
type MigrationStatus struct {
	CurrentVersion       string    `json:"current_version"`
	AppliedCount         int       `json:"applied_count"`
	PendingCount         int       `json:"pending_count"`
	TotalCount           int       `json:"total_count"`
	LastAppliedAt        time.Time `json:"last_applied_at"`
	HasPendingMigrations bool      `json:"has_pending_migrations"`
}

// getLastAppliedTime returns the last applied migration time
func getLastAppliedTime(applied []Migration) time.Time {
	if len(applied) == 0 {
		return time.Time{}
	}
	return applied[len(applied)-1].AppliedAt
}
