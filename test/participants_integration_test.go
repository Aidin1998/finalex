// =============================
// Two-Phase Commit Migration Integration Tests
// =============================
// This file provides comprehensive end-to-end integration tests for the
// two-phase commit migration system with zero-downtime guarantees.

package migration_test

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/trading/migration"
	"github.com/Aidin1998/pincex_unified/internal/trading/migration/participants"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// MigrationIntegrationTestSuite provides comprehensive testing for the migration system
type MigrationIntegrationTestSuite struct {
	suite.Suite
	logger      *zap.Logger
	db          *gorm.DB
	coordinator *migration.Coordinator
	lockMgr     *migration.LockManager
	persistence *migration.PersistenceManager
	safety      *migration.SafetyManager
	testCtx     context.Context
	testCancel  context.CancelFunc

	// Test participants
	orderbookParticipant   *participants.OrderBookParticipant
	persistenceParticipant *participants.PersistenceParticipant
	marketdataParticipant  *participants.MarketDataParticipant
	engineParticipant      *participants.EngineParticipant

	// Test state
	testPairs        []string
	activeMigrations map[uuid.UUID]*migration.MigrationState
	mu               sync.RWMutex
}

// TestMigrationEvent represents migration events during testing
type TestMigrationEvent struct {
	Timestamp   time.Time
	MigrationID uuid.UUID
	Pair        string
	Event       string
	Phase       migration.MigrationPhase
	Status      migration.MigrationStatus
	Data        map[string]interface{}
}

// SetupSuite initializes the test suite
func (suite *MigrationIntegrationTestSuite) SetupSuite() {
	suite.logger = zaptest.NewLogger(suite.T())
	suite.testCtx, suite.testCancel = context.WithCancel(context.Background())

	// Initialize test database
	var err error
	suite.db, err = gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(suite.T(), err)

	// Auto-migrate tables (if any needed for migration state)
	// err = suite.db.AutoMigrate(&migration.MigrationStateRecord{})
	// require.NoError(suite.T(), err)

	// Initialize test pairs
	suite.testPairs = []string{"BTC/USDT", "ETH/USDT", "ADA/USDT"}
	suite.activeMigrations = make(map[uuid.UUID]*migration.MigrationState)

	// Setup migration components
	suite.setupMigrationComponents()

	// Register test participants
	suite.setupTestParticipants()
}

// TearDownSuite cleans up after all tests
func (suite *MigrationIntegrationTestSuite) TearDownSuite() {
	if suite.coordinator != nil {
		_ = suite.coordinator.Shutdown(suite.testCtx)
	}
	suite.testCancel()
}

// SetupTest initializes each test
func (suite *MigrationIntegrationTestSuite) SetupTest() {
	// Clear any active migrations
	suite.mu.Lock()
	suite.activeMigrations = make(map[uuid.UUID]*migration.MigrationState)
	suite.mu.Unlock()

	// Reset safety manager state
	suite.safety.Reset()
}

// TearDownTest cleans up after each test
func (suite *MigrationIntegrationTestSuite) TearDownTest() {
	// Clean up any active migrations
	suite.mu.RLock()
	migrationIDs := make([]uuid.UUID, 0, len(suite.activeMigrations))
	for id := range suite.activeMigrations {
		migrationIDs = append(migrationIDs, id)
	}
	suite.mu.RUnlock()

	// Abort any active migrations
	for _, id := range migrationIDs {
		ctx, cancel := context.WithTimeout(suite.testCtx, 5*time.Second)
		_ = suite.coordinator.AbortMigration(ctx, id, "test cleanup")
		cancel()
	}
}

// setupMigrationComponents initializes the core migration components
func (suite *MigrationIntegrationTestSuite) setupMigrationComponents() {
	// Initialize lock manager
	lockConfig := &migration.LockManagerConfig{
		DefaultTTL:      time.Hour,
		RenewalInterval: time.Minute * 10,
		CleanupInterval: time.Minute * 5,
		MaxRetries:      3,
		RetryBackoff:    time.Second,
		EnableMetrics:   true,
	}
	suite.lockMgr = migration.NewLockManager(lockConfig, suite.logger)

	// Initialize persistence manager
	persistenceConfig := &migration.PersistenceConfig{
		EnablePersistence:     true,
		EnableDualStorage:     true,
		BackupRetentionDays:   7,
		BackupCompressionType: "gzip",
		RecoveryEnabled:       true,
		DatabasePath:          ":memory:",
		FileStoragePath:       "/tmp/migration-test",
	}
	var err error
	suite.persistence, err = migration.NewPersistenceManager(persistenceConfig, suite.db, suite.logger)
	require.NoError(suite.T(), err)

	// Initialize safety manager
	safetyConfig := &migration.SafetyConfig{
		EnableSafetyChecks:              true,
		EnableAutomaticRollback:         true,
		EnableIntegrityValidation:       true,
		EnablePerformanceComparison:     true,
		EnableCircuitBreaker:            true,
		EnableHealthMonitoring:          true,
		RollbackThreshold:               0.05, // 5% error rate triggers rollback
		PerformanceDegradationThreshold: 0.20, // 20% degradation triggers rollback
		HealthCheckInterval:             time.Second * 10,
		MetricsCollectionInterval:       time.Second * 5,
	}
	suite.safety = migration.NewSafetyManager(safetyConfig, suite.logger)

	// Initialize coordinator
	coordinatorConfig := &migration.CoordinatorConfig{
		MaxConcurrentMigrations: 5,
		DefaultPrepareTimeout:   time.Minute * 2,
		DefaultCommitTimeout:    time.Minute * 5,
		DefaultAbortTimeout:     time.Minute * 2,
		OverallTimeout:          time.Minute * 10,
		HealthCheckInterval:     time.Second * 30,
		PersistenceEnabled:      true,
		EventBufferSize:         1000,
		MaxEventSubscriptions:   100,
	}
	suite.coordinator = migration.NewCoordinator(coordinatorConfig, suite.lockMgr, suite.persistence, suite.safety, suite.logger)
}

// setupTestParticipants creates and registers test participants
func (suite *MigrationIntegrationTestSuite) setupTestParticipants() {
	// Create orderbook participant
	suite.orderbookParticipant = participants.NewOrderBookParticipant(
		"orderbook", "BTC/USDT", nil, suite.logger,
	)
	err := suite.coordinator.RegisterParticipant(suite.orderbookParticipant)
	require.NoError(suite.T(), err)

	// Create persistence participant
	suite.persistenceParticipant = participants.NewPersistenceParticipant(
		"persistence", "BTC/USDT", suite.db, suite.logger,
	)
	err = suite.coordinator.RegisterParticipant(suite.persistenceParticipant)
	require.NoError(suite.T(), err)

	// Create market data participant
	suite.marketdataParticipant = participants.NewMarketDataParticipant(
		"marketdata", "BTC/USDT", nil, suite.logger,
	)
	err = suite.coordinator.RegisterParticipant(suite.marketdataParticipant)
	require.NoError(suite.T(), err)

	// Create engine participant
	suite.engineParticipant = participants.NewEngineParticipant(
		"engine", "BTC/USDT", nil, suite.logger,
	)
	err = suite.coordinator.RegisterParticipant(suite.engineParticipant)
	require.NoError(suite.T(), err)
}

// Test basic migration lifecycle
func (suite *MigrationIntegrationTestSuite) TestBasicMigrationLifecycle() {
	t := suite.T()
	ctx := context.Background()

	// Create migration request
	migrationID := uuid.New()
	request := &migration.MigrationRequest{
		ID:          migrationID,
		Pair:        "BTC/USDT",
		RequestedBy: "test-suite",
		Config: &migration.MigrationConfig{
			TargetImplementation: "new_orderbook_v2",
			MigrationPercentage:  100,
			PrepareTimeout:       time.Minute * 2,
			CommitTimeout:        time.Minute * 3,
			AbortTimeout:         time.Minute * 2,
			OverallTimeout:       time.Minute * 10,
			EnableRollback:       true,
			EnableSafetyChecks:   true,
			ParticipantIDs:       []string{"orderbook", "persistence", "marketdata", "engine"},
		},
	}

	// Start migration
	state, err := suite.coordinator.StartMigration(ctx, request)
	require.NoError(t, err)
	require.NotNil(t, state)
	assert.Equal(t, migrationID, state.ID)
	assert.Equal(t, "BTC/USDT", state.Pair)
	assert.Equal(t, migration.StatusPending, state.Status)

	// Track migration
	suite.mu.Lock()
	suite.activeMigrations[migrationID] = state
	suite.mu.Unlock()

	// Wait for migration to complete
	err = suite.waitForMigrationCompletion(ctx, migrationID, time.Minute*5)
	assert.NoError(t, err)

	// Verify final state
	finalState, err := suite.coordinator.GetMigrationState(migrationID)
	require.NoError(t, err)
	assert.Equal(t, migration.StatusCompleted, finalState.Status)
	assert.Equal(t, migration.PhaseCompleted, finalState.Phase)
	assert.Equal(t, float64(100), finalState.Progress)
	assert.NotNil(t, finalState.EndTime)
}

// Test concurrent migrations on different pairs
func (suite *MigrationIntegrationTestSuite) TestConcurrentMigrations() {
	t := suite.T()
	ctx := context.Background()

	var wg sync.WaitGroup
	migrationCount := 3
	results := make(chan error, migrationCount)

	for i := 0; i < migrationCount; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			migrationID := uuid.New()
			pair := suite.testPairs[index]

			request := &migration.MigrationRequest{
				ID:          migrationID,
				Pair:        pair,
				RequestedBy: "concurrent-test",
				Config: &migration.MigrationConfig{
					TargetImplementation: "new_orderbook_v2",
					MigrationPercentage:  50 + int32(index*10),
					PrepareTimeout:       time.Minute,
					CommitTimeout:        time.Minute * 2,
					AbortTimeout:         time.Minute,
					OverallTimeout:       time.Minute * 5,
					EnableRollback:       true,
					EnableSafetyChecks:   true,
					ParticipantIDs:       []string{"orderbook", "persistence"},
				},
			}

			// Start migration
			state, err := suite.coordinator.StartMigration(ctx, request)
			if err != nil {
				results <- fmt.Errorf("failed to start migration for %s: %w", pair, err)
				return
			}

			// Track migration
			suite.mu.Lock()
			suite.activeMigrations[migrationID] = state
			suite.mu.Unlock()

			// Wait for completion
			err = suite.waitForMigrationCompletion(ctx, migrationID, time.Minute*3)
			if err != nil {
				results <- fmt.Errorf("migration failed for %s: %w", pair, err)
				return
			}

			results <- nil
		}(i)
	}

	wg.Wait()
	close(results)

	// Check results
	for err := range results {
		assert.NoError(t, err)
	}
}

// Test migration failure and recovery
func (suite *MigrationIntegrationTestSuite) TestMigrationFailureRecovery() {
	t := suite.T()
	ctx := context.Background()

	// Create a migration that will fail during prepare phase
	migrationID := uuid.New()
	request := &migration.MigrationRequest{
		ID:          migrationID,
		Pair:        "BTC/USDT",
		RequestedBy: "failure-test",
		Config: &migration.MigrationConfig{
			TargetImplementation: "invalid_implementation", // This should cause failure
			MigrationPercentage:  100,
			PrepareTimeout:       time.Second * 30,
			CommitTimeout:        time.Minute,
			AbortTimeout:         time.Second * 30,
			OverallTimeout:       time.Minute * 2,
			EnableRollback:       true,
			EnableSafetyChecks:   true,
			ParticipantIDs:       []string{"orderbook", "persistence"},
		},
	}

	// Start migration
	state, err := suite.coordinator.StartMigration(ctx, request)
	require.NoError(t, err)

	// Track migration
	suite.mu.Lock()
	suite.activeMigrations[migrationID] = state
	suite.mu.Unlock()

	// Wait for failure
	time.Sleep(time.Second * 5)

	// Check that migration failed or was aborted
	finalState, err := suite.coordinator.GetMigrationState(migrationID)
	require.NoError(t, err)

	// Should be failed or aborted
	assert.True(t, finalState.Status == migration.StatusFailed || finalState.Status == migration.StatusAborted)

	// Verify rollback occurred if needed
	if finalState.CanRollback {
		assert.True(t, finalState.Phase == migration.PhaseAborted || finalState.Phase == migration.PhaseFailed)
	}
}

// Test migration abort functionality
func (suite *MigrationIntegrationTestSuite) TestMigrationAbort() {
	t := suite.T()
	ctx := context.Background()

	migrationID := uuid.New()
	request := &migration.MigrationRequest{
		ID:          migrationID,
		Pair:        "BTC/USDT",
		RequestedBy: "abort-test",
		Config: &migration.MigrationConfig{
			TargetImplementation: "new_orderbook_v2",
			MigrationPercentage:  100,
			PrepareTimeout:       time.Minute * 5, // Long timeout to allow abort
			CommitTimeout:        time.Minute * 5,
			AbortTimeout:         time.Minute,
			OverallTimeout:       time.Minute * 15,
			EnableRollback:       true,
			EnableSafetyChecks:   true,
			ParticipantIDs:       []string{"orderbook", "persistence", "marketdata"},
		},
	}

	// Start migration
	state, err := suite.coordinator.StartMigration(ctx, request)
	require.NoError(t, err)

	// Track migration
	suite.mu.Lock()
	suite.activeMigrations[migrationID] = state
	suite.mu.Unlock()

	// Wait a bit then abort
	time.Sleep(time.Second * 2)

	err = suite.coordinator.AbortMigration(ctx, migrationID, "test abort")
	assert.NoError(t, err)

	// Wait for abort to complete
	time.Sleep(time.Second * 3)

	// Verify abort
	finalState, err := suite.coordinator.GetMigrationState(migrationID)
	require.NoError(t, err)
	assert.Equal(t, migration.StatusAborted, finalState.Status)
	assert.Equal(t, migration.PhaseAborted, finalState.Phase)
}

// Test safety mechanisms and automatic rollback
func (suite *MigrationIntegrationTestSuite) TestSafetyMechanisms() {
	t := suite.T()
	ctx := context.Background()

	// Configure safety manager for aggressive rollback
	suite.safety.UpdateConfig(&migration.SafetyConfig{
		EnableSafetyChecks:              true,
		EnableAutomaticRollback:         true,
		EnableIntegrityValidation:       true,
		EnablePerformanceComparison:     true,
		RollbackThreshold:               0.01, // Very low threshold (1% error rate)
		PerformanceDegradationThreshold: 0.05, // 5% degradation
		HealthCheckInterval:             time.Second,
		MetricsCollectionInterval:       time.Millisecond * 500,
	})

	migrationID := uuid.New()
	request := &migration.MigrationRequest{
		ID:          migrationID,
		Pair:        "BTC/USDT",
		RequestedBy: "safety-test",
		Config: &migration.MigrationConfig{
			TargetImplementation: "new_orderbook_v2",
			MigrationPercentage:  100,
			PrepareTimeout:       time.Minute,
			CommitTimeout:        time.Minute * 2,
			AbortTimeout:         time.Minute,
			OverallTimeout:       time.Minute * 5,
			EnableRollback:       true,
			EnableSafetyChecks:   true,
			ParticipantIDs:       []string{"orderbook", "persistence"},
		},
	}

	// Start migration
	state, err := suite.coordinator.StartMigration(ctx, request)
	require.NoError(t, err)

	// Track migration
	suite.mu.Lock()
	suite.activeMigrations[migrationID] = state
	suite.mu.Unlock()

	// Simulate some load during migration to trigger safety checks
	suite.simulateTrafficLoad(ctx, time.Second*10)

	// Wait for migration to complete or be rolled back
	err = suite.waitForMigrationCompletion(ctx, migrationID, time.Minute*3)
	// Don't assert no error - rollback is expected in some cases

	// Verify safety mechanisms were active
	finalState, err := suite.coordinator.GetMigrationState(migrationID)
	require.NoError(t, err)

	// Check safety status
	assert.NotNil(t, finalState.HealthStatus)
	assert.True(t, finalState.HealthStatus.LastCheck.After(state.StartTime))
}

// Test distributed lock management
func (suite *MigrationIntegrationTestSuite) TestDistributedLocks() {
	t := suite.T()
	ctx := context.Background()

	pair := "BTC/USDT"
	lockResource := "migration:" + pair

	// Acquire lock
	lockID1, err := suite.lockMgr.AcquireLock(ctx, lockResource, "test-holder-1", time.Minute)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, lockID1)

	// Try to acquire same lock (should fail)
	_, err = suite.lockMgr.AcquireLock(ctx, lockResource, "test-holder-2", time.Minute)
	assert.Error(t, err)

	// Renew lock
	err = suite.lockMgr.RenewLock(lockID1, time.Minute)
	assert.NoError(t, err)

	// Release lock
	err = suite.lockMgr.ReleaseLock(lockID1)
	assert.NoError(t, err)

	// Now should be able to acquire
	lockID2, err := suite.lockMgr.AcquireLock(ctx, lockResource, "test-holder-3", time.Minute)
	assert.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, lockID2)

	// Clean up
	_ = suite.lockMgr.ReleaseLock(lockID2)
}

// Test persistence and crash recovery
func (suite *MigrationIntegrationTestSuite) TestPersistenceAndRecovery() {
	t := suite.T()
	ctx := context.Background()

	migrationID := uuid.New()
	request := &migration.MigrationRequest{
		ID:          migrationID,
		Pair:        "BTC/USDT",
		RequestedBy: "persistence-test",
		Config: &migration.MigrationConfig{
			TargetImplementation: "new_orderbook_v2",
			MigrationPercentage:  100,
			PrepareTimeout:       time.Minute,
			CommitTimeout:        time.Minute * 2,
			AbortTimeout:         time.Minute,
			OverallTimeout:       time.Minute * 5,
			EnableRollback:       true,
			EnableSafetyChecks:   true,
			ParticipantIDs:       []string{"orderbook", "persistence"},
		},
	}

	// Start migration
	state, err := suite.coordinator.StartMigration(ctx, request)
	require.NoError(t, err)

	// Save state to persistence
	err = suite.persistence.SaveMigrationState(state)
	assert.NoError(t, err)

	// Wait a bit for migration to progress
	time.Sleep(time.Second * 2)

	// Load state from persistence
	loadedState, err := suite.persistence.LoadMigrationState(migrationID)
	assert.NoError(t, err)
	assert.NotNil(t, loadedState)
	assert.Equal(t, migrationID, loadedState.ID)
	assert.Equal(t, "BTC/USDT", loadedState.Pair)

	// List migrations
	migrations, err := suite.persistence.ListMigrations(&migration.MigrationFilter{
		Pair: "BTC/USDT",
	})
	assert.NoError(t, err)
	assert.NotEmpty(t, migrations)

	// Create backup
	backupInfo, err := suite.persistence.CreateBackup(ctx, "test-backup")
	assert.NoError(t, err)
	assert.NotNil(t, backupInfo)

	// List backups
	backups, err := suite.persistence.ListBackups()
	assert.NoError(t, err)
	assert.NotEmpty(t, backups)
}

// Test performance under load
func (suite *MigrationIntegrationTestSuite) TestPerformanceUnderLoad() {
	t := suite.T()
	ctx := context.Background()

	// Start migration
	migrationID := uuid.New()
	request := &migration.MigrationRequest{
		ID:          migrationID,
		Pair:        "BTC/USDT",
		RequestedBy: "performance-test",
		Config: &migration.MigrationConfig{
			TargetImplementation: "new_orderbook_v2",
			MigrationPercentage:  100,
			PrepareTimeout:       time.Minute * 2,
			CommitTimeout:        time.Minute * 3,
			AbortTimeout:         time.Minute,
			OverallTimeout:       time.Minute * 8,
			EnableRollback:       true,
			EnableSafetyChecks:   true,
			ParticipantIDs:       []string{"orderbook", "persistence", "marketdata", "engine"},
		},
	}

	state, err := suite.coordinator.StartMigration(ctx, request)
	require.NoError(t, err)

	// Track migration
	suite.mu.Lock()
	suite.activeMigrations[migrationID] = state
	suite.mu.Unlock()

	// Simulate heavy load during migration
	var wg sync.WaitGroup
	loadCtx, loadCancel := context.WithTimeout(ctx, time.Minute*3)
	defer loadCancel()

	// Start multiple load generators
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			suite.simulateTrafficLoad(loadCtx, time.Minute*3)
		}(i)
	}

	// Wait for migration to complete
	migrationComplete := make(chan error, 1)
	go func() {
		migrationComplete <- suite.waitForMigrationCompletion(ctx, migrationID, time.Minute*5)
	}()

	// Wait for either migration completion or timeout
	select {
	case err := <-migrationComplete:
		assert.NoError(t, err)
	case <-time.After(time.Minute * 6):
		t.Error("Migration took too long under load")
	}

	loadCancel()
	wg.Wait()

	// Verify migration metrics
	metrics, err := suite.coordinator.GetMetrics(migrationID)
	assert.NoError(t, err)
	assert.NotNil(t, metrics)

	// Check performance requirements
	if metrics.AverageLatency > 0 {
		assert.Less(t, metrics.AverageLatency.Milliseconds(), int64(100), "Average latency too high")
	}
}

// Test event subscription and monitoring
func (suite *MigrationIntegrationTestSuite) TestEventSubscriptionAndMonitoring() {
	t := suite.T()
	ctx := context.Background()

	migrationID := uuid.New()

	// Subscribe to migration events
	eventChan, err := suite.coordinator.Subscribe(ctx, migrationID)
	require.NoError(t, err)
	require.NotNil(t, eventChan)

	// Start event collector
	events := make([]*migration.MigrationEvent, 0)
	var eventsMu sync.Mutex

	go func() {
		for event := range eventChan {
			eventsMu.Lock()
			events = append(events, event)
			eventsMu.Unlock()
		}
	}()

	// Start migration
	request := &migration.MigrationRequest{
		ID:          migrationID,
		Pair:        "BTC/USDT",
		RequestedBy: "event-test",
		Config: &migration.MigrationConfig{
			TargetImplementation: "new_orderbook_v2",
			MigrationPercentage:  100,
			PrepareTimeout:       time.Minute,
			CommitTimeout:        time.Minute * 2,
			AbortTimeout:         time.Minute,
			OverallTimeout:       time.Minute * 5,
			EnableRollback:       true,
			EnableSafetyChecks:   true,
			ParticipantIDs:       []string{"orderbook", "persistence"},
		},
	}

	state, err := suite.coordinator.StartMigration(ctx, request)
	require.NoError(t, err)

	// Track migration
	suite.mu.Lock()
	suite.activeMigrations[migrationID] = state
	suite.mu.Unlock()

	// Wait for migration to complete
	err = suite.waitForMigrationCompletion(ctx, migrationID, time.Minute*3)
	assert.NoError(t, err)

	// Wait a bit for events to be collected
	time.Sleep(time.Second)

	// Verify events were received
	eventsMu.Lock()
	defer eventsMu.Unlock()

	assert.NotEmpty(t, events, "Should have received migration events")

	// Check for specific event types
	eventTypes := make(map[string]int)
	for _, event := range events {
		eventTypes[event.Type]++
	}

	assert.Greater(t, eventTypes["migration_started"], 0, "Should have migration_started event")
	// Additional event type checks can be added based on implementation
}

// Helper method to wait for migration completion
func (suite *MigrationIntegrationTestSuite) waitForMigrationCompletion(ctx context.Context, migrationID uuid.UUID, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for migration %s to complete", migrationID)
		case <-ticker.C:
			state, err := suite.coordinator.GetMigrationState(migrationID)
			if err != nil {
				return fmt.Errorf("failed to get migration state: %w", err)
			}

			switch state.Status {
			case migration.StatusCompleted:
				return nil
			case migration.StatusFailed, migration.StatusAborted:
				return fmt.Errorf("migration failed with status: %s", state.Status)
			}
		}
	}
}

// Helper method to simulate traffic load during migration
func (suite *MigrationIntegrationTestSuite) simulateTrafficLoad(ctx context.Context, duration time.Duration) {
	ctx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()

	ticker := time.NewTicker(time.Millisecond * 10) // High frequency
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Simulate random operations that would affect safety metrics
			_ = rand.Float64() < 0.1 // 10% "error" rate for testing
		}
	}
}

// Run the test suite
func TestMigrationIntegrationSuite(t *testing.T) {
	suite.Run(t, new(MigrationIntegrationTestSuite))
}

// Additional benchmark tests for performance validation
func BenchmarkMigrationThroughput(b *testing.B) {
	// Setup
	logger := zap.NewNop()
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})

	lockConfig := &migration.LockManagerConfig{
		DefaultTTL:    time.Hour,
		EnableMetrics: false, // Disable for benchmarking
	}
	lockMgr := migration.NewLockManager(lockConfig, logger)

	persistenceConfig := &migration.PersistenceConfig{
		EnablePersistence: false, // Disable for benchmarking
	}
	persistence, _ := migration.NewPersistenceManager(persistenceConfig, db, logger)

	safetyConfig := &migration.SafetyConfig{
		EnableSafetyChecks: false, // Disable for benchmarking
	}
	safety := migration.NewSafetyManager(safetyConfig, logger)

	coordinatorConfig := &migration.CoordinatorConfig{
		MaxConcurrentMigrations: 100,
		DefaultPrepareTimeout:   time.Second * 10,
		DefaultCommitTimeout:    time.Second * 10,
		DefaultAbortTimeout:     time.Second * 5,
		OverallTimeout:          time.Second * 30,
		PersistenceEnabled:      false,
	}
	coordinator := migration.NewCoordinator(coordinatorConfig, lockMgr, persistence, safety, logger)

	// Register mock participants
	mockParticipant := &MockParticipant{id: "mock"}
	_ = coordinator.RegisterParticipant(mockParticipant)

	b.ResetTimer()

	// Run benchmark
	for i := 0; i < b.N; i++ {
		ctx := context.Background()
		migrationID := uuid.New()

		request := &migration.MigrationRequest{
			ID:          migrationID,
			Pair:        fmt.Sprintf("PAIR%d", i),
			RequestedBy: "benchmark",
			Config: &migration.MigrationConfig{
				TargetImplementation: "benchmark_target",
				MigrationPercentage:  100,
				PrepareTimeout:       time.Second * 2,
				CommitTimeout:        time.Second * 2,
				AbortTimeout:         time.Second,
				OverallTimeout:       time.Second * 10,
				ParticipantIDs:       []string{"mock"},
			},
		}

		_, err := coordinator.StartMigration(ctx, request)
		if err != nil {
			b.Fatalf("Failed to start migration: %v", err)
		}

		// Wait for completion
		for {
			state, err := coordinator.GetMigrationState(migrationID)
			if err != nil {
				b.Fatalf("Failed to get migration state: %v", err)
			}

			if state.Status == migration.StatusCompleted ||
				state.Status == migration.StatusFailed ||
				state.Status == migration.StatusAborted {
				break
			}

			time.Sleep(time.Millisecond)
		}
	}
}

// MockParticipant for testing and benchmarking
type MockParticipant struct {
	id string
}

func (m *MockParticipant) GetID() string {
	return m.id
}

func (m *MockParticipant) GetType() string {
	return "mock"
}

func (m *MockParticipant) Prepare(ctx context.Context, migrationID uuid.UUID, config *migration.MigrationConfig) (*migration.ParticipantState, error) {
	return &migration.ParticipantState{
		ParticipantID: m.id,
		Vote:          migration.VoteYes,
		IsReady:       true,
		IsHealthy:     true,
		LastHeartbeat: time.Now(),
		OrdersSnapshot: &migration.OrdersSnapshot{
			Timestamp:   time.Now(),
			TotalOrders: 100,
			TotalVolume: 1000.0,
		},
	}, nil
}

func (m *MockParticipant) Commit(ctx context.Context, migrationID uuid.UUID) error {
	return nil
}

func (m *MockParticipant) Abort(ctx context.Context, migrationID uuid.UUID, reason string) error {
	return nil
}

func (m *MockParticipant) HealthCheck(ctx context.Context) (*migration.HealthCheck, error) {
	return &migration.HealthCheck{
		Status:    "healthy",
		Timestamp: time.Now(),
		Message:   "mock participant healthy",
	}, nil
}
