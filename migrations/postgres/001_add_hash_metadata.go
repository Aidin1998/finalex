package migrations

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Migration001AddHashMetadata adds hash metadata columns to support hybrid hashing
type Migration001AddHashMetadata struct{}

// Up runs the migration
func (m *Migration001AddHashMetadata) Up(db *gorm.DB) error {
	// Add hash_data column to api_keys table
	if err := db.Exec(`
		ALTER TABLE api_keys 
		ADD COLUMN IF NOT EXISTS hash_data TEXT
	`).Error; err != nil {
		return fmt.Errorf("failed to add hash_data column: %w", err)
	}

	// Create index on hash_data for performance
	if err := db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_api_keys_hash_data 
		ON api_keys (hash_data) 
		WHERE hash_data IS NOT NULL
	`).Error; err != nil {
		return fmt.Errorf("failed to create hash_data index: %w", err)
	}

	// Add migration record
	if err := db.Exec(`
		INSERT INTO schema_migrations (version, applied_at) 
		VALUES ('001_add_hash_metadata', ?) 
		ON CONFLICT (version) DO NOTHING
	`, time.Now()).Error; err != nil {
		return fmt.Errorf("failed to record migration: %w", err)
	}

	return nil
}

// Down reverses the migration
func (m *Migration001AddHashMetadata) Down(db *gorm.DB) error {
	// Drop index
	if err := db.Exec(`
		DROP INDEX IF EXISTS idx_api_keys_hash_data
	`).Error; err != nil {
		return fmt.Errorf("failed to drop hash_data index: %w", err)
	}

	// Drop column
	if err := db.Exec(`
		ALTER TABLE api_keys 
		DROP COLUMN IF EXISTS hash_data
	`).Error; err != nil {
		return fmt.Errorf("failed to drop hash_data column: %w", err)
	}

	// Remove migration record
	if err := db.Exec(`
		DELETE FROM schema_migrations 
		WHERE version = '001_add_hash_metadata'
	`).Error; err != nil {
		return fmt.Errorf("failed to remove migration record: %w", err)
	}

	return nil
}

// Version returns the migration version
func (m *Migration001AddHashMetadata) Version() string {
	return "001_add_hash_metadata"
}

// Description returns the migration description
func (m *Migration001AddHashMetadata) Description() string {
	return "Add hash metadata columns to support hybrid hashing algorithms"
}
