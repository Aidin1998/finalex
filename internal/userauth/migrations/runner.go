package migrations

import (
	"fmt"
	"sort"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Migration interface for database migrations
type Migration interface {
	Up(db *gorm.DB) error
	Down(db *gorm.DB) error
	Version() string
	Description() string
}

// SchemaMigration represents a migration record in the database
type SchemaMigration struct {
	Version   string `gorm:"primaryKey;column:version"`
	AppliedAt string `gorm:"column:applied_at"`
}

// TableName specifies the table name for SchemaMigration
func (SchemaMigration) TableName() string {
	return "schema_migrations"
}

// MigrationRunner handles database migrations
type MigrationRunner struct {
	db         *gorm.DB
	logger     *zap.Logger
	migrations []Migration
}

// NewMigrationRunner creates a new migration runner
func NewMigrationRunner(db *gorm.DB, logger *zap.Logger) *MigrationRunner {
	return &MigrationRunner{
		db:         db,
		logger:     logger,
		migrations: []Migration{
			// Removed: &Migration001AddHashMetadata{},
		},
	}
}

// RunMigrations runs all pending migrations
func (mr *MigrationRunner) RunMigrations() error {
	// Ensure schema_migrations table exists
	if err := mr.ensureMigrationsTable(); err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	// Get applied migrations
	appliedMigrations, err := mr.getAppliedMigrations()
	if err != nil {
		return fmt.Errorf("failed to get applied migrations: %w", err)
	}

	// Sort migrations by version
	sort.Slice(mr.migrations, func(i, j int) bool {
		return mr.migrations[i].Version() < mr.migrations[j].Version()
	})

	// Run pending migrations
	for _, migration := range mr.migrations {
		if !appliedMigrations[migration.Version()] {
			mr.logger.Info("Running migration",
				zap.String("version", migration.Version()),
				zap.String("description", migration.Description()),
			)

			if err := migration.Up(mr.db); err != nil {
				return fmt.Errorf("failed to run migration %s: %w", migration.Version(), err)
			}

			mr.logger.Info("Migration completed",
				zap.String("version", migration.Version()),
			)
		}
	}

	return nil
}

// RollbackMigration rolls back a specific migration
func (mr *MigrationRunner) RollbackMigration(version string) error {
	for _, migration := range mr.migrations {
		if migration.Version() == version {
			mr.logger.Info("Rolling back migration",
				zap.String("version", version),
				zap.String("description", migration.Description()),
			)

			if err := migration.Down(mr.db); err != nil {
				return fmt.Errorf("failed to rollback migration %s: %w", version, err)
			}

			mr.logger.Info("Migration rolled back",
				zap.String("version", version),
			)
			return nil
		}
	}

	return fmt.Errorf("migration %s not found", version)
}

// GetMigrationStatus returns the status of all migrations
func (mr *MigrationRunner) GetMigrationStatus() (map[string]bool, error) {
	return mr.getAppliedMigrations()
}

// ensureMigrationsTable creates the schema_migrations table if it doesn't exist
func (mr *MigrationRunner) ensureMigrationsTable() error {
	return mr.db.AutoMigrate(&SchemaMigration{})
}

// getAppliedMigrations returns a map of applied migration versions
func (mr *MigrationRunner) getAppliedMigrations() (map[string]bool, error) {
	var migrations []SchemaMigration
	if err := mr.db.Find(&migrations).Error; err != nil {
		return nil, err
	}

	applied := make(map[string]bool)
	for _, migration := range migrations {
		applied[migration.Version] = true
	}

	return applied, nil
}
