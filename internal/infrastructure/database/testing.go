// Test database support and fixtures for the database submodule
package database

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// TestDatabaseConfig configures test database behavior
type TestDatabaseConfig struct {
	// Database type
	Driver string `json:"driver"` // "sqlite", "postgres", "memory"

	// Connection settings
	DSN        string `json:"dsn"`
	TestDBName string `json:"test_db_name"`

	// Isolation settings
	IsolateTests       bool `json:"isolate_tests"`       // Create separate DB per test
	TransactionalTests bool `json:"transactional_tests"` // Wrap tests in transactions

	// Cleanup settings
	CleanupPolicy string `json:"cleanup_policy"` // "always", "on_success", "never"
	KeepSchema    bool   `json:"keep_schema"`    // Keep schema between tests

	// Performance settings
	EnableWAL bool `json:"enable_wal"` // Enable WAL mode for SQLite
	EnableFK  bool `json:"enable_fk"`  // Enable foreign key constraints
	LogSQL    bool `json:"log_sql"`    // Log SQL statements

	// Fixtures
	FixturePath  string   `json:"fixture_path"`
	LoadFixtures []string `json:"load_fixtures"`
}

// TestDatabaseManager manages test databases
type TestDatabaseManager struct {
	config       *TestDatabaseConfig
	logger       *zap.Logger
	databases    map[string]*gorm.DB
	transactions map[string]*gorm.DB
	mu           sync.RWMutex
}

// TestFixture represents test data fixture
type TestFixture struct {
	Name         string               `json:"name"`
	Description  string               `json:"description"`
	Tables       map[string][]TestRow `json:"tables"`
	Order        []string             `json:"order"` // Table load order
	Dependencies []string             `json:"dependencies"`
}

// TestRow represents a row of test data
type TestRow map[string]interface{}

// NewTestDatabaseManager creates a new test database manager
func NewTestDatabaseManager(config *TestDatabaseConfig, logger *zap.Logger) *TestDatabaseManager {
	if config == nil {
		config = &TestDatabaseConfig{
			Driver:             "sqlite",
			TestDBName:         "test_db",
			IsolateTests:       true,
			TransactionalTests: true,
			CleanupPolicy:      "always",
			EnableFK:           true,
			LogSQL:             false,
		}
	}

	if logger == nil {
		logger = zap.NewNop()
	}

	return &TestDatabaseManager{
		config:       config,
		logger:       logger.Named("test-db"),
		databases:    make(map[string]*gorm.DB),
		transactions: make(map[string]*gorm.DB),
	}
}

// CreateTestDatabase creates a test database instance
func (tdm *TestDatabaseManager) CreateTestDatabase(testName string) (*gorm.DB, func(), error) {
	tdm.mu.Lock()
	defer tdm.mu.Unlock()

	var db *gorm.DB
	var err error
	var cleanup func()

	switch tdm.config.Driver {
	case "sqlite", "memory":
		db, cleanup, err = tdm.createSQLiteTestDB(testName)
	case "postgres":
		db, cleanup, err = tdm.createPostgresTestDB(testName)
	default:
		return nil, nil, fmt.Errorf("unsupported test database driver: %s", tdm.config.Driver)
	}

	if err != nil {
		return nil, nil, fmt.Errorf("failed to create test database: %w", err)
	}

	// Store database reference
	tdm.databases[testName] = db

	// Setup transactional test if enabled
	if tdm.config.TransactionalTests {
		tx := db.Begin()
		if tx.Error != nil {
			cleanup()
			return nil, nil, fmt.Errorf("failed to begin test transaction: %w", tx.Error)
		}

		tdm.transactions[testName] = tx

		// Wrap cleanup to rollback transaction
		originalCleanup := cleanup
		cleanup = func() {
			if tx := tdm.transactions[testName]; tx != nil {
				tx.Rollback()
				delete(tdm.transactions, testName)
			}
			originalCleanup()
		}

		db = tx
	}

	// Load fixtures if specified
	if len(tdm.config.LoadFixtures) > 0 {
		if err := tdm.loadFixtures(db, tdm.config.LoadFixtures); err != nil {
			cleanup()
			return nil, nil, fmt.Errorf("failed to load fixtures: %w", err)
		}
	}

	tdm.logger.Debug("created test database",
		zap.String("test_name", testName),
		zap.String("driver", tdm.config.Driver),
		zap.Bool("transactional", tdm.config.TransactionalTests))

	return db, cleanup, nil
}

// createSQLiteTestDB creates an SQLite test database
func (tdm *TestDatabaseManager) createSQLiteTestDB(testName string) (*gorm.DB, func(), error) {
	var dsn string
	var cleanup func()

	if tdm.config.Driver == "memory" || tdm.config.DSN == ":memory:" {
		// In-memory database
		dsn = ":memory:"
		cleanup = func() {
			// Memory databases are automatically cleaned up
		}
	} else {
		// File-based database
		if tdm.config.IsolateTests {
			// Create unique database file per test
			dbFile := filepath.Join(os.TempDir(), fmt.Sprintf("%s_%s_%d.db",
				tdm.config.TestDBName, testName, time.Now().UnixNano()))
			dsn = dbFile
			cleanup = func() {
				if tdm.config.CleanupPolicy == "always" ||
					tdm.config.CleanupPolicy == "on_success" {
					os.Remove(dbFile)
				}
			}
		} else {
			// Shared database file
			dbFile := filepath.Join(os.TempDir(), fmt.Sprintf("%s.db", tdm.config.TestDBName))
			dsn = dbFile
			cleanup = func() {
				// Don't remove shared database
			}
		}
	}

	// Configure GORM logger
	var gormLogger logger.Interface = logger.Default.LogMode(logger.Silent)
	if tdm.config.LogSQL {
		gormLogger = logger.Default.LogMode(logger.Info)
	}

	// Open database
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger:                                   gormLogger,
		SkipDefaultTransaction:                   false,
		DisableForeignKeyConstraintWhenMigrating: !tdm.config.EnableFK,
	})

	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("failed to open SQLite database: %w", err)
	}

	// Configure SQLite
	if tdm.config.EnableWAL {
		db.Exec("PRAGMA journal_mode=WAL")
	}

	if tdm.config.EnableFK {
		db.Exec("PRAGMA foreign_keys=ON")
	}

	return db, cleanup, nil
}

// createPostgresTestDB creates a PostgreSQL test database
func (tdm *TestDatabaseManager) createPostgresTestDB(testName string) (*gorm.DB, func(), error) {
	// This would implement PostgreSQL test database creation
	// For now, return an error indicating it needs implementation
	return nil, nil, fmt.Errorf("PostgreSQL test database implementation needed")
}

// LoadFixture loads a specific fixture into the database
func (tdm *TestDatabaseManager) LoadFixture(db *gorm.DB, fixtureName string) error {
	if tdm.config.FixturePath == "" {
		return fmt.Errorf("fixture path not configured")
	}

	fixturePath := filepath.Join(tdm.config.FixturePath, fixtureName+".json")

	// Load fixture file
	fixture, err := tdm.loadFixtureFile(fixturePath)
	if err != nil {
		return fmt.Errorf("failed to load fixture file: %w", err)
	}

	return tdm.applyFixture(db, fixture)
}

// loadFixtures loads multiple fixtures
func (tdm *TestDatabaseManager) loadFixtures(db *gorm.DB, fixtureNames []string) error {
	for _, fixtureName := range fixtureNames {
		if err := tdm.LoadFixture(db, fixtureName); err != nil {
			return fmt.Errorf("failed to load fixture %s: %w", fixtureName, err)
		}
	}
	return nil
}

// loadFixtureFile loads a fixture from file
func (tdm *TestDatabaseManager) loadFixtureFile(path string) (*TestFixture, error) {
	// This would implement fixture file loading
	// For now, return a placeholder
	return &TestFixture{
		Name:        "placeholder",
		Description: "Placeholder fixture",
		Tables:      make(map[string][]TestRow),
		Order:       []string{},
	}, nil
}

// applyFixture applies a fixture to the database
func (tdm *TestDatabaseManager) applyFixture(db *gorm.DB, fixture *TestFixture) error {
	tdm.logger.Debug("applying fixture",
		zap.String("fixture_name", fixture.Name),
		zap.Int("table_count", len(fixture.Tables)))

	// Apply tables in specified order
	order := fixture.Order
	if len(order) == 0 {
		// If no order specified, use table names
		for tableName := range fixture.Tables {
			order = append(order, tableName)
		}
	}

	for _, tableName := range order {
		rows, exists := fixture.Tables[tableName]
		if !exists {
			continue
		}

		if err := tdm.insertFixtureRows(db, tableName, rows); err != nil {
			return fmt.Errorf("failed to insert rows for table %s: %w", tableName, err)
		}

		tdm.logger.Debug("inserted fixture rows",
			zap.String("table", tableName),
			zap.Int("row_count", len(rows)))
	}

	return nil
}

// insertFixtureRows inserts fixture rows into a table
func (tdm *TestDatabaseManager) insertFixtureRows(db *gorm.DB, tableName string, rows []TestRow) error {
	if len(rows) == 0 {
		return nil
	}

	// Convert rows to interface{} slice for GORM
	var records []interface{}
	for _, row := range rows {
		records = append(records, row)
	}

	// Insert in batches for better performance
	batchSize := 100
	for i := 0; i < len(records); i += batchSize {
		end := i + batchSize
		if end > len(records) {
			end = len(records)
		}

		batch := records[i:end]
		if err := db.Table(tableName).Create(batch).Error; err != nil {
			return fmt.Errorf("failed to insert batch: %w", err)
		}
	}

	return nil
}

// CleanTestDatabase cleans up a test database
func (tdm *TestDatabaseManager) CleanTestDatabase(testName string) error {
	tdm.mu.Lock()
	defer tdm.mu.Unlock()

	// Rollback transaction if exists
	if tx, exists := tdm.transactions[testName]; exists {
		tx.Rollback()
		delete(tdm.transactions, testName)
	}

	// Remove database reference
	if db, exists := tdm.databases[testName]; exists {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
		delete(tdm.databases, testName)
	}

	tdm.logger.Debug("cleaned test database", zap.String("test_name", testName))
	return nil
}

// Cleanup cleans up all test databases
func (tdm *TestDatabaseManager) Cleanup() error {
	tdm.mu.Lock()
	defer tdm.mu.Unlock()

	var errors []error

	// Rollback all transactions
	for testName, tx := range tdm.transactions {
		tx.Rollback()
		delete(tdm.transactions, testName)
	}

	// Close all databases
	for testName, db := range tdm.databases {
		if sqlDB, err := db.DB(); err == nil {
			if err := sqlDB.Close(); err != nil {
				errors = append(errors, fmt.Errorf("failed to close test database %s: %w", testName, err))
			}
		}
		delete(tdm.databases, testName)
	}

	if len(errors) > 0 {
		return fmt.Errorf("cleanup errors: %v", errors)
	}

	tdm.logger.Debug("cleaned up all test databases")
	return nil
}

// Helper function to create a test database manager with default settings
func NewTestDatabaseManagerWithDefaults(logger *zap.Logger) *TestDatabaseManager {
	config := &TestDatabaseConfig{
		Driver:             "memory",
		IsolateTests:       true,
		TransactionalTests: true,
		CleanupPolicy:      "always",
		EnableFK:           true,
		LogSQL:             false,
	}

	return NewTestDatabaseManager(config, logger)
}

// TestDatabaseOption is a function that modifies TestDatabaseConfig
type TestDatabaseOption func(*TestDatabaseConfig)

// WithDriver sets the database driver
func WithDriver(driver string) TestDatabaseOption {
	return func(config *TestDatabaseConfig) {
		config.Driver = driver
	}
}

// WithFixtures sets the fixtures to load
func WithFixtures(fixtures []string, fixturePath string) TestDatabaseOption {
	return func(config *TestDatabaseConfig) {
		config.LoadFixtures = fixtures
		config.FixturePath = fixturePath
	}
}

// WithSQLLogging enables SQL logging
func WithSQLLogging(enabled bool) TestDatabaseOption {
	return func(config *TestDatabaseConfig) {
		config.LogSQL = enabled
	}
}

// WithTransactionalTests enables/disables transactional tests
func WithTransactionalTests(enabled bool) TestDatabaseOption {
	return func(config *TestDatabaseConfig) {
		config.TransactionalTests = enabled
	}
}

// NewTestDatabaseManagerWithOptions creates a test database manager with options
func NewTestDatabaseManagerWithOptions(logger *zap.Logger, options ...TestDatabaseOption) *TestDatabaseManager {
	config := &TestDatabaseConfig{
		Driver:             "memory",
		IsolateTests:       true,
		TransactionalTests: true,
		CleanupPolicy:      "always",
		EnableFK:           true,
		LogSQL:             false,
	}

	for _, option := range options {
		option(config)
	}

	return NewTestDatabaseManager(config, logger)
}

// DatabaseManager test support methods

// CreateTestInstance creates a test instance of the database manager
func (dm *DatabaseManager) CreateTestInstance(testName string, options ...TestDatabaseOption) (*DatabaseManager, func(), error) {
	testDBManager := NewTestDatabaseManagerWithOptions(dm.logger, options...)

	testDB, cleanup, err := testDBManager.CreateTestDatabase(testName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create test database: %w", err)
	}

	// Create a test instance of DatabaseManager with the test database
	testConfig := *dm.config
	testManager := &DatabaseManager{
		config: &testConfig,
		logger: dm.logger.Named("test"),
		master: testDB,
		slaves: []*gorm.DB{}, // No slaves in test mode
		shards: make(map[string]*DatabaseManager),
		cipher: dm.cipher,
	}

	// Wrap cleanup to also clean up the test manager
	wrappedCleanup := func() {
		testDBManager.CleanTestDatabase(testName)
		cleanup()
	}

	return testManager, wrappedCleanup, nil
}
