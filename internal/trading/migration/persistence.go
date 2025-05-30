// =============================
// State Persistence for Migration Recovery
// =============================
// This file implements persistent storage for migration state to enable
// crash recovery and ensure migration consistency across system restarts.

package migration

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// StatePersistence handles persistent storage of migration state
type StatePersistence struct {
	logger   *zap.SugaredLogger
	config   *PersistenceConfig
	db       *gorm.DB
	filePath string
	mu       sync.RWMutex
}

// PersistenceConfig contains configuration for state persistence
type PersistenceConfig struct {
	StorageType        string        `json:"storage_type"` // "file", "database", "both"
	FilePath           string        `json:"file_path"`
	DatabaseEnabled    bool          `json:"database_enabled"`
	FileEnabled        bool          `json:"file_enabled"`
	SyncInterval       time.Duration `json:"sync_interval"`
	BackupEnabled      bool          `json:"backup_enabled"`
	BackupRetention    int           `json:"backup_retention"`
	CompressionEnabled bool          `json:"compression_enabled"`
}

// DefaultPersistenceConfig returns safe defaults
func DefaultPersistenceConfig() *PersistenceConfig {
	return &PersistenceConfig{
		StorageType:        "both",
		FilePath:           "./data/migration_state.json",
		DatabaseEnabled:    true,
		FileEnabled:        true,
		SyncInterval:       10 * time.Second,
		BackupEnabled:      true,
		BackupRetention:    10,
		CompressionEnabled: false,
	}
}

// PersistedMigrationState represents the database model for migration state
type PersistedMigrationState struct {
	ID          string     `gorm:"primaryKey;type:uuid" json:"id"`
	Pair        string     `gorm:"index" json:"pair"`
	StateData   string     `gorm:"type:text" json:"state_data"`
	Phase       string     `gorm:"index" json:"phase"`
	Status      string     `gorm:"index" json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// TableName returns the table name for GORM
func (PersistedMigrationState) TableName() string {
	return "migration_states"
}

// NewStatePersistence creates a new state persistence manager
func NewStatePersistence(logger *zap.SugaredLogger, config *PersistenceConfig, db *gorm.DB) (*StatePersistence, error) {
	if config == nil {
		config = DefaultPersistenceConfig()
	}

	sp := &StatePersistence{
		logger:   logger,
		config:   config,
		db:       db,
		filePath: config.FilePath,
	}

	// Initialize storage
	if err := sp.initialize(); err != nil {
		return nil, fmt.Errorf("failed to initialize state persistence: %w", err)
	}

	return sp, nil
}

// initialize sets up the persistence storage
func (sp *StatePersistence) initialize() error {
	// Create file directory if file storage is enabled
	if sp.config.FileEnabled {
		dir := filepath.Dir(sp.filePath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create state directory: %w", err)
		}
	}

	// Initialize database table if database storage is enabled
	if sp.config.DatabaseEnabled && sp.db != nil {
		if err := sp.db.AutoMigrate(&PersistedMigrationState{}); err != nil {
			return fmt.Errorf("failed to migrate migration state table: %w", err)
		}
	}

	sp.logger.Infow("State persistence initialized",
		"storage_type", sp.config.StorageType,
		"file_enabled", sp.config.FileEnabled,
		"database_enabled", sp.config.DatabaseEnabled,
		"file_path", sp.filePath)

	return nil
}

// SaveMigrationState persists a migration state
func (sp *StatePersistence) SaveMigrationState(state *MigrationState) error {
	sp.mu.Lock()
	defer sp.mu.Unlock()

	// Serialize state
	stateData, err := sp.serializeState(state)
	if err != nil {
		return fmt.Errorf("failed to serialize migration state: %w", err)
	}

	// Save to database if enabled
	if sp.config.DatabaseEnabled && sp.db != nil {
		if err := sp.saveToDB(state, stateData); err != nil {
			sp.logger.Errorw("Failed to save state to database", "error", err, "migration_id", state.ID)
			// Don't fail if only database save fails
		}
	}

	// Save to file if enabled
	if sp.config.FileEnabled {
		if err := sp.saveToFile(state, stateData); err != nil {
			sp.logger.Errorw("Failed to save state to file", "error", err, "migration_id", state.ID)
			// Don't fail if only file save fails
		}
	}

	return nil
}

// LoadMigrationStates loads all migration states from persistence
func (sp *StatePersistence) LoadMigrationStates() ([]*MigrationState, error) {
	sp.mu.RLock()
	defer sp.mu.RUnlock()

	var states []*MigrationState
	var err error

	// Load from database if enabled
	if sp.config.DatabaseEnabled && sp.db != nil {
		dbStates, dbErr := sp.loadFromDB()
		if dbErr != nil {
			sp.logger.Errorw("Failed to load states from database", "error", dbErr)
		} else {
			states = append(states, dbStates...)
		}
	}

	// Load from file if enabled
	if sp.config.FileEnabled {
		fileStates, fileErr := sp.loadFromFile()
		if fileErr != nil {
			sp.logger.Errorw("Failed to load states from file", "error", fileErr)
		} else {
			// Merge file states with database states (database takes precedence)
			states = sp.mergeStates(states, fileStates)
		}
	}

	sp.logger.Infow("Loaded migration states", "count", len(states))
	return states, err
}

// LoadMigrationState loads a specific migration state
func (sp *StatePersistence) LoadMigrationState(migrationID uuid.UUID) (*MigrationState, error) {
	sp.mu.RLock()
	defer sp.mu.RUnlock()

	// Try database first
	if sp.config.DatabaseEnabled && sp.db != nil {
		if state, err := sp.loadFromDBByID(migrationID); err == nil {
			return state, nil
		}
	}

	// Fallback to file
	if sp.config.FileEnabled {
		states, err := sp.loadFromFile()
		if err != nil {
			return nil, err
		}

		for _, state := range states {
			if state.ID == migrationID {
				return state, nil
			}
		}
	}

	return nil, fmt.Errorf("migration state %s not found", migrationID)
}

// DeleteMigrationState removes a migration state from persistence
func (sp *StatePersistence) DeleteMigrationState(migrationID uuid.UUID) error {
	sp.mu.Lock()
	defer sp.mu.Unlock()

	// Delete from database if enabled
	if sp.config.DatabaseEnabled && sp.db != nil {
		if err := sp.deleteFromDB(migrationID); err != nil {
			sp.logger.Errorw("Failed to delete state from database", "error", err, "migration_id", migrationID)
		}
	}

	// Update file if enabled
	if sp.config.FileEnabled {
		if err := sp.deleteFromFile(migrationID); err != nil {
			sp.logger.Errorw("Failed to delete state from file", "error", err, "migration_id", migrationID)
		}
	}

	return nil
}

// CreateBackup creates a backup of the current state
func (sp *StatePersistence) CreateBackup() error {
	if !sp.config.BackupEnabled {
		return nil
	}

	sp.mu.RLock()
	defer sp.mu.RUnlock()

	// Create backup filename with timestamp
	timestamp := time.Now().Format("20060102_150405")
	backupPath := fmt.Sprintf("%s.backup_%s", sp.filePath, timestamp)

	// Copy current state file to backup
	if _, err := os.Stat(sp.filePath); err == nil {
		input, err := ioutil.ReadFile(sp.filePath)
		if err != nil {
			return fmt.Errorf("failed to read state file for backup: %w", err)
		}

		if err := ioutil.WriteFile(backupPath, input, 0644); err != nil {
			return fmt.Errorf("failed to create backup file: %w", err)
		}

		sp.logger.Infow("Created state backup", "backup_path", backupPath)
	}

	// Clean up old backups
	sp.cleanupOldBackups()

	return nil
}

// Database operations

func (sp *StatePersistence) saveToDB(state *MigrationState, stateData []byte) error {
	persistedState := &PersistedMigrationState{
		ID:        state.ID.String(),
		Pair:      state.Pair,
		StateData: string(stateData),
		Phase:     state.Phase.String(),
		Status:    string(state.Status),
		UpdatedAt: time.Now(),
	}

	if state.EndTime != nil {
		persistedState.CompletedAt = state.EndTime
	}

	// Use UPSERT operation
	result := sp.db.Save(persistedState)
	if result.Error != nil {
		return fmt.Errorf("failed to save migration state to database: %w", result.Error)
	}

	return nil
}

func (sp *StatePersistence) loadFromDB() ([]*MigrationState, error) {
	var persistedStates []PersistedMigrationState
	result := sp.db.Find(&persistedStates)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to load migration states from database: %w", result.Error)
	}

	var states []*MigrationState
	for _, persistedState := range persistedStates {
		state, err := sp.deserializeState([]byte(persistedState.StateData))
		if err != nil {
			sp.logger.Errorw("Failed to deserialize state from database",
				"error", err,
				"migration_id", persistedState.ID)
			continue
		}
		states = append(states, state)
	}

	return states, nil
}

func (sp *StatePersistence) loadFromDBByID(migrationID uuid.UUID) (*MigrationState, error) {
	var persistedState PersistedMigrationState
	result := sp.db.Where("id = ?", migrationID.String()).First(&persistedState)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to load migration state from database: %w", result.Error)
	}

	state, err := sp.deserializeState([]byte(persistedState.StateData))
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize state: %w", err)
	}

	return state, nil
}

func (sp *StatePersistence) deleteFromDB(migrationID uuid.UUID) error {
	result := sp.db.Where("id = ?", migrationID.String()).Delete(&PersistedMigrationState{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete migration state from database: %w", result.Error)
	}

	return nil
}

// File operations

func (sp *StatePersistence) saveToFile(state *MigrationState, stateData []byte) error {
	// Load existing states
	states, err := sp.loadFromFile()
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to load existing states: %w", err)
	}

	// Update or add the current state
	found := false
	for i, existingState := range states {
		if existingState.ID == state.ID {
			states[i] = state
			found = true
			break
		}
	}
	if !found {
		states = append(states, state)
	}

	// Serialize all states
	allStatesData, err := json.MarshalIndent(states, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal states: %w", err)
	}

	// Write to temporary file first
	tempPath := sp.filePath + ".tmp"
	if err := ioutil.WriteFile(tempPath, allStatesData, 0644); err != nil {
		return fmt.Errorf("failed to write temporary state file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tempPath, sp.filePath); err != nil {
		return fmt.Errorf("failed to rename temporary state file: %w", err)
	}

	return nil
}

func (sp *StatePersistence) loadFromFile() ([]*MigrationState, error) {
	if _, err := os.Stat(sp.filePath); os.IsNotExist(err) {
		return []*MigrationState{}, nil
	}

	data, err := ioutil.ReadFile(sp.filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read state file: %w", err)
	}

	if len(data) == 0 {
		return []*MigrationState{}, nil
	}

	var states []*MigrationState
	if err := json.Unmarshal(data, &states); err != nil {
		return nil, fmt.Errorf("failed to unmarshal state file: %w", err)
	}

	return states, nil
}

func (sp *StatePersistence) deleteFromFile(migrationID uuid.UUID) error {
	states, err := sp.loadFromFile()
	if err != nil {
		return err
	}

	// Filter out the state to delete
	var filteredStates []*MigrationState
	for _, state := range states {
		if state.ID != migrationID {
			filteredStates = append(filteredStates, state)
		}
	}

	// Write back the filtered states
	data, err := json.MarshalIndent(filteredStates, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal filtered states: %w", err)
	}

	if err := ioutil.WriteFile(sp.filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write filtered state file: %w", err)
	}

	return nil
}

// Helper methods

func (sp *StatePersistence) serializeState(state *MigrationState) ([]byte, error) {
	// Create a copy without the mutex
	state.mu.RLock()
	stateCopy := &MigrationState{
		ID:             state.ID,
		Pair:           state.Pair,
		Phase:          state.Phase,
		Status:         state.Status,
		Config:         state.Config,
		StartTime:      state.StartTime,
		Progress:       state.Progress,
		OrdersMigrated: state.OrdersMigrated,
		TotalOrders:    state.TotalOrders,
		ErrorCount:     state.ErrorCount,
		RetryCount:     state.RetryCount,
		Participants:   make(map[string]*ParticipantState),
		VotesSummary:   state.VotesSummary,
		Metrics:        state.Metrics,
		HealthStatus:   state.HealthStatus,
		CanRollback:    state.CanRollback,
		RollbackReason: state.RollbackReason,
	}

	// Copy time pointers
	if state.PrepareTime != nil {
		prepareTime := *state.PrepareTime
		stateCopy.PrepareTime = &prepareTime
	}
	if state.CommitTime != nil {
		commitTime := *state.CommitTime
		stateCopy.CommitTime = &commitTime
	}
	if state.EndTime != nil {
		endTime := *state.EndTime
		stateCopy.EndTime = &endTime
	}

	// Copy participants
	for id, pState := range state.Participants {
		pStateCopy := *pState
		if pState.VoteTime != nil {
			voteTime := *pState.VoteTime
			pStateCopy.VoteTime = &voteTime
		}
		stateCopy.Participants[id] = &pStateCopy
	}
	state.mu.RUnlock()

	return json.Marshal(stateCopy)
}

func (sp *StatePersistence) deserializeState(data []byte) (*MigrationState, error) {
	var state MigrationState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}

	// Initialize the mutex
	state.mu = sync.RWMutex{}

	return &state, nil
}

func (sp *StatePersistence) mergeStates(dbStates, fileStates []*MigrationState) []*MigrationState {
	// Create a map of database states for quick lookup
	dbStateMap := make(map[uuid.UUID]*MigrationState)
	for _, state := range dbStates {
		dbStateMap[state.ID] = state
	}

	// Add file states that don't exist in database
	var mergedStates []*MigrationState
	mergedStates = append(mergedStates, dbStates...)

	for _, fileState := range fileStates {
		if _, exists := dbStateMap[fileState.ID]; !exists {
			mergedStates = append(mergedStates, fileState)
		}
	}

	return mergedStates
}

func (sp *StatePersistence) cleanupOldBackups() {
	if sp.config.BackupRetention <= 0 {
		return
	}

	dir := filepath.Dir(sp.filePath)
	baseName := filepath.Base(sp.filePath)

	files, err := ioutil.ReadDir(dir)
	if err != nil {
		sp.logger.Errorw("Failed to read backup directory", "error", err)
		return
	}

	// Find backup files
	var backups []os.FileInfo
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		if len(file.Name()) > len(baseName)+8 &&
			file.Name()[:len(baseName)] == baseName &&
			file.Name()[len(baseName):len(baseName)+8] == ".backup_" {
			backups = append(backups, file)
		}
	}

	// Sort by modification time (newest first)
	// This is a simple approach; more sophisticated sorting could be implemented
	if len(backups) > sp.config.BackupRetention {
		for i := sp.config.BackupRetention; i < len(backups); i++ {
			backupPath := filepath.Join(dir, backups[i].Name())
			if err := os.Remove(backupPath); err != nil {
				sp.logger.Errorw("Failed to remove old backup", "error", err, "backup_path", backupPath)
			} else {
				sp.logger.Debugw("Removed old backup", "backup_path", backupPath)
			}
		}
	}
}

// GetPersistenceMetrics returns metrics about the persistence layer
func (sp *StatePersistence) GetPersistenceMetrics() map[string]interface{} {
	sp.mu.RLock()
	defer sp.mu.RUnlock()

	metrics := map[string]interface{}{
		"storage_type":     sp.config.StorageType,
		"file_enabled":     sp.config.FileEnabled,
		"database_enabled": sp.config.DatabaseEnabled,
		"backup_enabled":   sp.config.BackupEnabled,
	}

	// File metrics
	if sp.config.FileEnabled {
		if stat, err := os.Stat(sp.filePath); err == nil {
			metrics["file_size_bytes"] = stat.Size()
			metrics["file_modified"] = stat.ModTime()
		}
	}

	// Database metrics
	if sp.config.DatabaseEnabled && sp.db != nil {
		var count int64
		sp.db.Model(&PersistedMigrationState{}).Count(&count)
		metrics["database_records"] = count
	}

	return metrics
}
