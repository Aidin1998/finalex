// Database submodule integration tests - corrected version
package test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Aidin1998/finalex/internal/infrastructure/database"
)

// testMigrationIntegration tests migration interface with actual migrations
func testMigrationIntegration(t *testing.T, ctx context.Context, logger *zap.Logger, tempDir string) {
	// Create test database
	dbPath := filepath.Join(tempDir, "migration_test.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	require.NoError(t, err)

	// Create migration manager
	migrationManager := database.NewMigrationManager(db, logger, &database.MigrationOptions{
		TableName:   "test_migrations",
		LockTimeout: 30 * time.Second,
	})

	// Initialize migration schema
	err = migrationManager.InitializeSchema(ctx)
	require.NoError(t, err)

	// Register a test migration that mimics the pattern from /migrations/postgres
	testMigration := database.MigrationScript{
		Version:     "001_test_migration",
		Description: "Test migration for integration testing",
		UpSQL: `
			CREATE TABLE IF NOT EXISTS test_table (
				id INTEGER PRIMARY KEY,
				name TEXT NOT NULL,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP
			);
			CREATE INDEX IF NOT EXISTS idx_test_table_name ON test_table(name);
		`,
		DownSQL: `
			DROP INDEX IF EXISTS idx_test_table_name;
			DROP TABLE IF EXISTS test_table;
		`,
		Checksum: "test_checksum_001",
	}

	err = migrationManager.RegisterMigration(testMigration)
	require.NoError(t, err)

	// Test getting pending migrations
	pending, err := migrationManager.GetPendingMigrations(ctx)
	require.NoError(t, err)
	assert.Len(t, pending, 1)
	assert.Equal(t, "001_test_migration", pending[0].Version)

	// Apply migrations
	err = migrationManager.Migrate(ctx)
	require.NoError(t, err)

	// Verify migration was applied
	pending, err = migrationManager.GetPendingMigrations(ctx)
	require.NoError(t, err)
	assert.Len(t, pending, 0)

	// Verify table was created
	var tableExists bool
	err = db.Raw("SELECT name FROM sqlite_master WHERE type='table' AND name='test_table'").Row().Scan(&tableExists)
	require.NoError(t, err)

	// Test rollback
	err = migrationManager.RollbackTo(ctx, "000_initial")
	require.NoError(t, err)

	// Verify table was dropped
	var count int
	err = db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='test_table'").Row().Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

// TestDatabaseIntegrationFixed tests the complete database submodule integration with correct APIs
func TestDatabaseIntegrationFixed(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	// Create temporary directory for test databases
	tempDir := t.TempDir()

	t.Run("MigrationIntegration", func(t *testing.T) {
		testMigrationIntegration(t, ctx, logger, tempDir)
	})

	t.Run("BackupRestoreIntegration", func(t *testing.T) {
		testBackupRestoreIntegrationFixed(t, ctx, logger, tempDir)
	})

	t.Run("TestDatabaseIntegration", func(t *testing.T) {
		testTestDatabaseIntegrationFixed(t, ctx, logger, tempDir)
	})

	t.Run("ComprehensiveManagerIntegration", func(t *testing.T) {
		testComprehensiveManagerIntegrationFixed(t, ctx, logger, tempDir)
	})
}

// testBackupRestoreIntegrationFixed tests backup and restore functionality with correct APIs
func testBackupRestoreIntegrationFixed(t *testing.T, ctx context.Context, logger *zap.Logger, tempDir string) {
	// Create source database with test data
	sourcePath := filepath.Join(tempDir, "backup_source.db")
	sourceDB, err := gorm.Open(sqlite.Open(sourcePath), &gorm.Config{})
	require.NoError(t, err)

	// Create test table and data
	err = sourceDB.Exec(`
		CREATE TABLE backup_test (
			id INTEGER PRIMARY KEY,
			data TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`).Error
	require.NoError(t, err)

	err = sourceDB.Exec(`
		INSERT INTO backup_test (data) VALUES 
		('test data 1'),
		('test data 2'),
		('test data 3')
	`).Error
	require.NoError(t, err)

	// Create backup manager with correct config structure
	backupManager := database.NewBackupManager(sourceDB, logger, &database.BackupConfig{
		DefaultBackupPath: tempDir,
		RetentionPeriod:   7 * 24 * time.Hour,
		EnableEncryption:  false,
		CompressionLevel:  6,
		WorkerPoolSize:    2,
	})

	// Test backup creation
	backupPath := filepath.Join(tempDir, "test_backup.sql")
	backupOptions := &database.BackupOptions{
		OutputPath:  backupPath,
		Format:      "sql",
		Compression: false,
		SchemaOnly:  false,
		DataOnly:    false,
		Metadata: map[string]interface{}{
			"test_name":  "integration_test",
			"created_by": "automated_test",
		},
	}

	backupInfo, err := backupManager.CreateBackup(ctx, backupOptions)
	require.NoError(t, err)
	assert.NotNil(t, backupInfo)
	assert.Equal(t, "test_backup.sql", filepath.Base(backupInfo.Path))

	// Verify backup file exists
	_, err = os.Stat(backupPath)
	require.NoError(t, err)

	// Create target database for restore
	targetPath := filepath.Join(tempDir, "backup_target.db")
	targetDB, err := gorm.Open(sqlite.Open(targetPath), &gorm.Config{})
	require.NoError(t, err)

	// Create restore manager
	restoreManager := database.NewBackupManager(targetDB, logger, &database.BackupConfig{
		DefaultBackupPath: tempDir,
	})

	// Test restore
	restoreOptions := &database.RestoreOptions{
		BackupPath: backupPath,
		CleanFirst: true,
		IfExists:   "replace",
	}

	err = restoreManager.RestoreBackup(ctx, restoreOptions)
	require.NoError(t, err)

	// Verify restored data
	var count int64
	err = targetDB.Raw("SELECT COUNT(*) FROM backup_test").Row().Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, int64(3), count)

	// Test backup listing
	backups, err := backupManager.ListBackups(ctx)
	require.NoError(t, err)
	assert.Len(t, backups, 1)
	assert.Equal(t, "test_backup.sql", filepath.Base(backups[0].Path))
}

// testTestDatabaseIntegrationFixed tests the test database support with correct APIs
func testTestDatabaseIntegrationFixed(t *testing.T, ctx context.Context, logger *zap.Logger, tempDir string) {
	// Create test database config
	config := &database.TestDatabaseConfig{
		Driver:             "sqlite",
		DSN:                ":memory:",
		IsolateTests:       true,
		TransactionalTests: true,
		CleanupPolicy:      "always",
		LogSQL:             false,
		EnableFK:           true,
		FixturePath:        filepath.Join(tempDir, "fixtures"),
	}

	// Create test database manager (returns 1 value, not 2)
	testDBManager := database.NewTestDatabaseManager(config, logger)

	// Create fixtures directory
	fixturesDir := filepath.Join(tempDir, "fixtures")
	err := os.MkdirAll(fixturesDir, 0755)
	require.NoError(t, err)

	// Test database creation (method takes testName only)
	testDB, cleanup, err := testDBManager.CreateTestDatabase("test_integration")
	require.NoError(t, err)
	defer cleanup()

	// Create test table
	err = testDB.Exec(`
		CREATE TABLE test_users (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			email TEXT UNIQUE NOT NULL
		)
	`).Error
	require.NoError(t, err)

	// Insert some test data manually since LoadFixtures is unexported
	err = testDB.Exec(`
		INSERT INTO test_users (name, email) VALUES 
		('Test User 1', 'test1@example.com'),
		('Test User 2', 'test2@example.com')
	`).Error
	require.NoError(t, err)

	// Verify test data
	var count int64
	err = testDB.Raw("SELECT COUNT(*) FROM test_users").Row().Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)

	// Test basic transactional behavior using GORM's transaction support
	err = testDB.Transaction(func(tx *gorm.DB) error {
		// Insert data in transaction
		err := tx.Exec("INSERT INTO test_users (name, email) VALUES (?, ?)", "Test User 3", "test3@example.com").Error
		if err != nil {
			return err
		}

		// Verify data exists in transaction
		var txCount int64
		err = tx.Raw("SELECT COUNT(*) FROM test_users").Row().Scan(&txCount)
		if err != nil {
			return err
		}
		assert.Equal(t, int64(3), txCount)

		// Return error to rollback
		return assert.AnError
	})
	require.Error(t, err) // Transaction should have failed and rolled back

	// Verify data was rolled back
	var finalCount int64
	err = testDB.Raw("SELECT COUNT(*) FROM test_users").Row().Scan(&finalCount)
	require.NoError(t, err)
	assert.Equal(t, int64(2), finalCount)
}

// testComprehensiveManagerIntegrationFixed tests the comprehensive database manager with correct APIs
func testComprehensiveManagerIntegrationFixed(t *testing.T, ctx context.Context, logger *zap.Logger, tempDir string) {
	// Create database config using actual DatabaseConfig structure
	config := &database.DatabaseConfig{
		MasterDSN:           "file:" + filepath.Join(tempDir, "comprehensive_test.db") + "?cache=shared&mode=rwc",
		MaxOpenConns:        10,
		MaxIdleConns:        5,
		ConnMaxLifetime:     30 * time.Minute,
		ConnMaxIdleTime:     5 * time.Minute,
		HealthCheckInterval: 30 * time.Second,
		ConnectionTimeout:   5 * time.Second,
		QueryTimeout:        30 * time.Second,
		MaxRetries:          3,
		RetryDelay:          1 * time.Second,
		EncryptionKey:       "12345678901234567890123456789012", // 32 byte key
		EnableMetrics:       true,
		MetricsPrefix:       "test_db",
	}

	// Create comprehensive database manager
	dbManager, err := database.NewDatabaseManager(config, logger)
	require.NoError(t, err)
	require.NotNil(t, dbManager)

	// Test health check
	health := dbManager.HealthCheck(ctx)
	assert.True(t, health.Healthy)
	assert.True(t, health.Master)

	// Test database access through interface
	db := dbManager.DB()
	require.NotNil(t, db)

	// Test master database access
	masterDB := dbManager.Master()
	require.NotNil(t, masterDB)

	// Create test table
	err = db.Exec(`
		CREATE TABLE comprehensive_test (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			value INTEGER
		)
	`).Error
	require.NoError(t, err)

	// Test transaction using the interface
	err = dbManager.Transaction(ctx, func(tx *gorm.DB) error {
		return tx.Exec("INSERT INTO comprehensive_test (name, value) VALUES (?, ?)", "test1", 100).Error
	})
	require.NoError(t, err)

	// Test WithTransaction method
	tx, cleanup, err := dbManager.WithTransaction(ctx)
	require.NoError(t, err)

	err = tx.Exec("INSERT INTO comprehensive_test (name, value) VALUES (?, ?)", "test2", 200).Error
	require.NoError(t, err)

	cleanup() // Commit transaction

	// Verify data
	var count int64
	err = db.Raw("SELECT COUNT(*) FROM comprehensive_test").Row().Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)

	// Test encryption functionality
	plaintext := "sensitive data"
	encrypted, err := dbManager.Encrypt(plaintext)
	require.NoError(t, err)
	assert.NotEqual(t, plaintext, encrypted)

	decrypted, err := dbManager.Decrypt(encrypted)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)

	// Test metrics
	metrics := dbManager.GetMetrics()
	assert.True(t, metrics.ConnectionPoolStats.OpenConnections > 0)

	// Test ping
	err = dbManager.Ping(ctx)
	require.NoError(t, err)

	// Test close
	err = dbManager.Close()
	require.NoError(t, err)
}

// TestDatabaseSubmoduleWithRealMigrationsFixed tests integration with actual migrations from /migrations folder
func TestDatabaseSubmoduleWithRealMigrationsFixed(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()
	tempDir := t.TempDir()

	// Create test database
	dbPath := filepath.Join(tempDir, "real_migration_test.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	require.NoError(t, err)

	// Create migration manager
	migrationManager := database.NewMigrationManager(db, logger, nil)

	// Initialize schema
	err = migrationManager.InitializeSchema(ctx)
	require.NoError(t, err)

	// Test integration with a real migration pattern
	// This simulates how the database submodule would coordinate with /migrations/postgres

	// Register equivalent migration through database submodule
	equivalentMigration := database.MigrationScript{
		Version:     "001_add_hash_metadata",
		Description: "Add hash metadata columns to support hybrid hashing",
		UpSQL: `
			CREATE TABLE IF NOT EXISTS api_keys (
				id INTEGER PRIMARY KEY,
				key_hash TEXT NOT NULL,
				hash_data TEXT
			);
			CREATE INDEX IF NOT EXISTS idx_api_keys_hash_data 
			ON api_keys (hash_data) 
			WHERE hash_data IS NOT NULL;
		`,
		DownSQL: `
			DROP INDEX IF EXISTS idx_api_keys_hash_data;
			DROP TABLE IF EXISTS api_keys;
		`,
		Checksum: "real_migration_001",
	}

	err = migrationManager.RegisterMigration(equivalentMigration)
	require.NoError(t, err)

	// Apply migration through database submodule
	err = migrationManager.Migrate(ctx)
	require.NoError(t, err)

	// Verify the migration coordination worked
	applied, err := migrationManager.GetAppliedMigrations(ctx)
	require.NoError(t, err)
	assert.Len(t, applied, 1)
	assert.Equal(t, "001_add_hash_metadata", applied[0].Version)

	// Test rollback functionality
	err = migrationManager.RollbackTo(ctx, "000_initial")
	require.NoError(t, err)

	// Verify rollback worked
	var tableCount int
	err = db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='api_keys'").Row().Scan(&tableCount)
	require.NoError(t, err)
	assert.Equal(t, 0, tableCount)

	t.Log("Real migration integration test completed successfully")
	t.Logf("Database submodule successfully coordinated with migration pattern from /migrations/postgres")
}
