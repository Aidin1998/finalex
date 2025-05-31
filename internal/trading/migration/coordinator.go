// =============================
// Migration Coordinator Implementation
// =============================
// This file implements the central coordinator for two-phase commit
// migration protocol with zero-downtime guarantees.

package migration

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Coordinator implements the two-phase commit migration coordinator
type Coordinator struct {
	logger      *zap.SugaredLogger
	lockMgr     *LockManager
	persistence *StatePersistence

	// State management
	migrations     map[uuid.UUID]*MigrationState
	migrationsMu   sync.RWMutex
	participants   map[string]MigrationParticipant
	participantsMu sync.RWMutex

	// Event handling
	eventSubscribers map[uuid.UUID][]chan *MigrationEvent
	eventsMu         sync.RWMutex

	// Control channels
	stopChan      chan struct{}
	healthTicker  *time.Ticker
	cleanupTicker *time.Ticker

	// Metrics
	totalMigrations      int64
	successfulMigrations int64
	failedMigrations     int64

	// Configuration
	config *CoordinatorConfig
}

// CoordinatorConfig contains coordinator configuration
type CoordinatorConfig struct {
	HealthCheckInterval     time.Duration `json:"health_check_interval"`
	CleanupInterval         time.Duration `json:"cleanup_interval"`
	MaxConcurrentMigrations int           `json:"max_concurrent_migrations"`
	DefaultTimeout          time.Duration `json:"default_timeout"`
	StateRetentionPeriod    time.Duration `json:"state_retention_period"`
	PersistenceEnabled      bool          `json:"persistence_enabled"`
	MetricsEnabled          bool          `json:"metrics_enabled"`
}

// DefaultCoordinatorConfig returns safe defaults
func DefaultCoordinatorConfig() *CoordinatorConfig {
	return &CoordinatorConfig{
		HealthCheckInterval:     30 * time.Second,
		CleanupInterval:         5 * time.Minute,
		MaxConcurrentMigrations: 5,
		DefaultTimeout:          10 * time.Minute,
		StateRetentionPeriod:    24 * time.Hour,
		PersistenceEnabled:      true,
		MetricsEnabled:          true,
	}
}

// NewCoordinator creates a new migration coordinator
func NewCoordinator(logger *zap.SugaredLogger, config *CoordinatorConfig, persistence *StatePersistence) *Coordinator {
	if config == nil {
		config = DefaultCoordinatorConfig()
	}

	coordinator := &Coordinator{
		logger:           logger,
		config:           config,
		persistence:      persistence,
		lockMgr:          NewLockManager(logger),
		migrations:       make(map[uuid.UUID]*MigrationState),
		participants:     make(map[string]MigrationParticipant),
		eventSubscribers: make(map[uuid.UUID][]chan *MigrationEvent),
		stopChan:         make(chan struct{}),
	}

	// Start background workers
	coordinator.startBackgroundWorkers()

	// Recover existing migrations if persistence is enabled
	if config.PersistenceEnabled && persistence != nil {
		coordinator.recoverMigrations()
	}

	return coordinator
}

// StartMigration initiates a new migration with two-phase commit protocol
func (c *Coordinator) StartMigration(ctx context.Context, request *MigrationRequest) (*MigrationState, error) {
	// Validate request
	if err := c.validateMigrationRequest(request); err != nil {
		return nil, fmt.Errorf("invalid migration request: %w", err)
	}

	// Check concurrent migration limits
	if c.getActiveMigrationsCount() >= c.config.MaxConcurrentMigrations {
		return nil, fmt.Errorf("maximum concurrent migrations reached: %d", c.config.MaxConcurrentMigrations)
	}

	// Check if pair is already being migrated
	if c.isPairBeingMigrated(request.Pair) {
		return nil, fmt.Errorf("pair %s is already being migrated", request.Pair)
	}

	// Acquire distributed lock for the pair
	lockID, err := c.lockMgr.AcquireLock(ctx, "migration:"+request.Pair, "coordinator", 1*time.Hour)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire migration lock: %w", err)
	}

	// Create migration state
	migrationState := &MigrationState{
		ID:           request.ID,
		Pair:         request.Pair,
		Phase:        PhaseIdle,
		Status:       StatusPending,
		Config:       request.Config,
		StartTime:    time.Now(),
		Participants: make(map[string]*ParticipantState),
		Progress:     0.0,
		CanRollback:  true,
		Metrics:      &MigrationMetrics{},
		HealthStatus: &HealthStatus{
			IsHealthy:    true,
			LastCheck:    time.Now(),
			HealthChecks: make(map[string]*HealthCheck),
		},
	}

	// Store migration state
	c.migrationsMu.Lock()
	c.migrations[request.ID] = migrationState
	c.migrationsMu.Unlock()

	// Persist state if enabled
	if c.config.PersistenceEnabled {
		if err := c.persistence.SaveMigrationState(migrationState); err != nil {
			c.logger.Errorw("Failed to persist migration state", "error", err, "migration_id", request.ID)
		}
	}

	// Emit start event
	c.emitEvent(&MigrationEvent{
		ID:          uuid.New(),
		MigrationID: request.ID,
		Timestamp:   time.Now(),
		Type:        "migration_started",
		Phase:       PhaseIdle,
		Data: map[string]interface{}{
			"pair":         request.Pair,
			"config":       request.Config,
			"lock_id":      lockID,
			"requested_by": request.RequestedBy,
		},
	})

	// Start migration process asynchronously
	go c.executeMigration(ctx, migrationState, lockID)

	atomic.AddInt64(&c.totalMigrations, 1)
	c.logger.Infow("Migration started", "migration_id", request.ID, "pair", request.Pair)

	return migrationState, nil
}

// executeMigration executes the two-phase commit migration protocol
func (c *Coordinator) executeMigration(ctx context.Context, state *MigrationState, lockID uuid.UUID) {
	defer func() {
		// Release lock when migration completes
		if err := c.lockMgr.ReleaseLock(lockID); err != nil {
			c.logger.Errorw("Failed to release migration lock", "error", err, "lock_id", lockID)
		}
	}()

	// Create context with timeout
	migrationCtx, cancel := context.WithTimeout(ctx, state.Config.OverallTimeout)
	defer cancel()

	// Execute migration phases
	success := false

	// Phase 1: Prepare
	if err := c.executePreparePhase(migrationCtx, state); err != nil {
		c.logger.Errorw("Prepare phase failed", "error", err, "migration_id", state.ID)
		c.abortMigrationInternal(migrationCtx, state, fmt.Sprintf("Prepare phase failed: %v", err))
		return
	}

	// Check if all participants voted yes
	if state.VotesSummary.Consensus != VoteYes {
		c.logger.Warnw("Migration aborted due to negative consensus",
			"migration_id", state.ID,
			"consensus", state.VotesSummary.Consensus,
			"yes_votes", state.VotesSummary.YesVotes,
			"no_votes", state.VotesSummary.NoVotes)
		c.abortMigrationInternal(migrationCtx, state, "Negative consensus from participants")
		return
	}

	// Phase 2: Commit
	if err := c.executeCommitPhase(migrationCtx, state); err != nil {
		c.logger.Errorw("Commit phase failed", "error", err, "migration_id", state.ID)
		c.abortMigrationInternal(migrationCtx, state, fmt.Sprintf("Commit phase failed: %v", err))
		return
	}

	success = true
	c.completeMigration(state)

	if success {
		atomic.AddInt64(&c.successfulMigrations, 1)
		c.logger.Infow("Migration completed successfully", "migration_id", state.ID, "pair", state.Pair)
	} else {
		atomic.AddInt64(&c.failedMigrations, 1)
	}
}

// executePreparePhase executes the prepare phase of two-phase commit
func (c *Coordinator) executePreparePhase(ctx context.Context, state *MigrationState) error {
	c.updateMigrationPhase(state, PhasePrepare, StatusPreparing)

	// Create context with prepare timeout
	prepareCtx, cancel := context.WithTimeout(ctx, state.Config.PrepareTimeout)
	defer cancel()

	// Initialize participants if not done
	if len(state.Participants) == 0 {
		c.initializeParticipants(state)
	}

	// Send prepare requests to all participants
	var wg sync.WaitGroup
	errorsChan := make(chan error, len(state.Participants))

	for participantID := range state.Participants {
		wg.Add(1)
		go func(pid string) {
			defer wg.Done()

			participant := c.getParticipant(pid)
			if participant == nil {
				errorsChan <- fmt.Errorf("participant %s not found", pid)
				return
			}

			// Send prepare request
			participantState, err := participant.Prepare(prepareCtx, state.ID, state.Config)
			if err != nil {
				// Record negative vote
				c.recordParticipantVote(state, pid, VoteNo, err.Error())
				errorsChan <- fmt.Errorf("participant %s prepare failed: %w", pid, err)
				return
			}

			// Record positive vote and state
			c.recordParticipantVote(state, pid, VoteYes, "")
			c.updateParticipantState(state, pid, participantState)

		}(participantID)
	}

	// Wait for all participants or timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All participants responded
	case <-prepareCtx.Done():
		// Timeout occurred - mark pending participants as timeout
		c.markTimeoutParticipants(state)
		return fmt.Errorf("prepare phase timeout")
	}

	// Collect any errors
	close(errorsChan)
	var errors []error
	for err := range errorsChan {
		errors = append(errors, err)
	}

	// Update votes summary
	c.updateVotesSummary(state)

	// Check if we have consensus
	if state.VotesSummary.Consensus == VoteYes {
		now := time.Now()
		state.PrepareTime = &now
		state.mu.Lock()
		state.Status = StatusPrepared
		state.mu.Unlock()

		c.emitEvent(&MigrationEvent{
			ID:          uuid.New(),
			MigrationID: state.ID,
			Timestamp:   time.Now(),
			Type:        "prepare_phase_completed",
			Phase:       PhasePrepare,
			Data: map[string]interface{}{
				"votes_summary": state.VotesSummary,
				"consensus":     "yes",
			},
		})

		return nil
	}

	// No consensus - prepare to abort
	if len(errors) > 0 {
		return fmt.Errorf("prepare phase failed with %d errors: %v", len(errors), errors[0])
	}

	return fmt.Errorf("no consensus reached in prepare phase")
}

// executeCommitPhase executes the commit phase of two-phase commit
func (c *Coordinator) executeCommitPhase(ctx context.Context, state *MigrationState) error {
	c.updateMigrationPhase(state, PhaseCommit, StatusCommitting)

	// Create context with commit timeout
	commitCtx, cancel := context.WithTimeout(ctx, state.Config.CommitTimeout)
	defer cancel()

	// Send commit requests to all participants
	var wg sync.WaitGroup
	errorsChan := make(chan error, len(state.Participants))

	for participantID := range state.Participants {
		wg.Add(1)
		go func(pid string) {
			defer wg.Done()

			participant := c.getParticipant(pid)
			if participant == nil {
				errorsChan <- fmt.Errorf("participant %s not found", pid)
				return
			}

			// Send commit request
			if err := participant.Commit(commitCtx, state.ID); err != nil {
				errorsChan <- fmt.Errorf("participant %s commit failed: %w", pid, err)
				return
			}

			// Update participant status
			state.mu.Lock()
			if pState, exists := state.Participants[pid]; exists {
				pState.IsHealthy = true
				pState.LastHeartbeat = time.Now()
				pState.ErrorMessage = ""
			}
			state.mu.Unlock()

		}(participantID)
	}

	// Wait for all participants or timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All participants committed
	case <-commitCtx.Done():
		// Timeout occurred
		return fmt.Errorf("commit phase timeout")
	}

	// Collect any errors
	close(errorsChan)
	var errors []error
	for err := range errorsChan {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		return fmt.Errorf("commit phase failed with %d errors: %v", len(errors), errors[0])
	}

	// Update state
	now := time.Now()
	state.CommitTime = &now
	state.mu.Lock()
	state.Status = StatusCommitted
	state.Progress = 100.0
	state.mu.Unlock()

	c.emitEvent(&MigrationEvent{
		ID:          uuid.New(),
		MigrationID: state.ID,
		Timestamp:   time.Now(),
		Type:        "commit_phase_completed",
		Phase:       PhaseCommit,
		Data: map[string]interface{}{
			"participants_count": len(state.Participants),
			"errors_count":       len(errors),
		},
	})

	return nil
}

// abortMigrationInternal aborts migration and sends abort to participants
func (c *Coordinator) abortMigrationInternal(ctx context.Context, state *MigrationState, reason string) {
	c.updateMigrationPhase(state, PhaseAbort, StatusAborting)

	state.mu.Lock()
	state.RollbackReason = reason
	state.CanRollback = false
	state.mu.Unlock()

	// Send abort to all participants
	abortCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	for participantID := range state.Participants {
		wg.Add(1)
		go func(pid string) {
			defer wg.Done()

			participant := c.getParticipant(pid)
			if participant != nil {
				if err := participant.Abort(abortCtx, state.ID); err != nil {
					c.logger.Errorw("Failed to abort participant", "error", err, "participant", pid)
				}
			}
		}(participantID)
	}

	wg.Wait()

	// Update state
	now := time.Now()
	state.EndTime = &now
	state.mu.Lock()
	state.Phase = PhaseAborted
	state.Status = StatusAborted
	state.mu.Unlock()

	c.emitEvent(&MigrationEvent{
		ID:          uuid.New(),
		MigrationID: state.ID,
		Timestamp:   time.Now(),
		Type:        "migration_aborted",
		Phase:       PhaseAbort,
		Data: map[string]interface{}{
			"reason": reason,
		},
	})
}

// GetAllMigrations returns all migrations managed by the coordinator
func (c *Coordinator) GetAllMigrations() []*MigrationState {
	states, _ := c.ListMigrations(nil)
	return states
}

// GetMigration returns the migration state and a boolean indicating existence
func (c *Coordinator) GetMigration(id uuid.UUID) (*MigrationState, bool) {
	c.migrationsMu.RLock()
	defer c.migrationsMu.RUnlock()
	state, exists := c.migrations[id]
	return state, exists
}

// ResumeMigration resumes a paused or aborted migration by restarting execution
func (c *Coordinator) ResumeMigration(ctx context.Context, migrationID uuid.UUID) error {
	state, err := c.GetMigrationState(migrationID)
	if err != nil {
		return fmt.Errorf("migration not found: %w", err)
	}
	go c.executeMigration(ctx, state, uuid.Nil)
	return nil
}

// GetTotalMigrations returns the total number of migrations started
func (c *Coordinator) GetTotalMigrations() int64 {
	return atomic.LoadInt64(&c.totalMigrations)
}

// GetSuccessfulMigrations returns the number of successfully completed migrations
func (c *Coordinator) GetSuccessfulMigrations() int64 {
	return atomic.LoadInt64(&c.successfulMigrations)
}

// GetFailedMigrations returns the number of failed migrations
func (c *Coordinator) GetFailedMigrations() int64 {
	return atomic.LoadInt64(&c.failedMigrations)
}

// GetActiveMigrations returns migrations not yet completed or failed
func (c *Coordinator) GetActiveMigrations() []*MigrationState {
	c.migrationsMu.RLock()
	defer c.migrationsMu.RUnlock()
	var active []*MigrationState
	for _, state := range c.migrations {
		if state.Phase != PhaseCompleted && state.Phase != PhaseAborted && state.Phase != PhaseFailed {
			active = append(active, state)
		}
	}
	return active
}

// GetAllParticipants returns a copy of all registered participants
func (c *Coordinator) GetAllParticipants() map[string]MigrationParticipant {
	c.participantsMu.RLock()
	defer c.participantsMu.RUnlock()
	copyMap := make(map[string]MigrationParticipant, len(c.participants))
	for id, p := range c.participants {
		copyMap[id] = p
	}
	return copyMap
}

// GetParticipant returns a participant by ID
func (c *Coordinator) GetParticipant(id string) (MigrationParticipant, bool) {
	c.participantsMu.RLock()
	defer c.participantsMu.RUnlock()
	p, ok := c.participants[id]
	return p, ok
}

// Helper methods

func (c *Coordinator) updateMigrationPhase(state *MigrationState, phase MigrationPhase, status MigrationStatus) {
	state.mu.Lock()
	oldPhase := state.Phase
	state.Phase = phase
	state.Status = status
	state.mu.Unlock()

	if oldPhase != phase {
		c.emitEvent(&MigrationEvent{
			ID:          uuid.New(),
			MigrationID: state.ID,
			Timestamp:   time.Now(),
			Type:        "phase_changed",
			Phase:       phase,
			Data: map[string]interface{}{
				"old_phase": oldPhase.String(),
				"new_phase": phase.String(),
				"status":    status,
			},
		})
	}
}

func (c *Coordinator) initializeParticipants(state *MigrationState) {
	c.participantsMu.RLock()
	defer c.participantsMu.RUnlock()

	state.mu.Lock()
	defer state.mu.Unlock()

	for id, participant := range c.participants {
		state.Participants[id] = &ParticipantState{
			ID:            id,
			Type:          participant.GetType(),
			Vote:          VotePending,
			LastHeartbeat: time.Now(),
			IsHealthy:     true,
		}
	}

	// Initialize votes summary
	state.VotesSummary = VotesSummary{
		TotalParticipants: len(state.Participants),
		PendingVotes:      len(state.Participants),
		Consensus:         VotePending,
	}
}

func (c *Coordinator) recordParticipantVote(state *MigrationState, participantID string, vote ParticipantVote, errorMsg string) {
	state.mu.Lock()
	defer state.mu.Unlock()

	if pState, exists := state.Participants[participantID]; exists {
		pState.Vote = vote
		now := time.Now()
		pState.VoteTime = &now
		pState.LastHeartbeat = now
		pState.ErrorMessage = errorMsg
		pState.IsHealthy = vote == VoteYes
	}
}

func (c *Coordinator) updateVotesSummary(state *MigrationState) {
	state.mu.Lock()
	defer state.mu.Unlock()

	summary := &state.VotesSummary
	summary.YesVotes = 0
	summary.NoVotes = 0
	summary.PendingVotes = 0
	summary.TimeoutVotes = 0

	for _, pState := range state.Participants {
		switch pState.Vote {
		case VoteYes:
			summary.YesVotes++
		case VoteNo:
			summary.NoVotes++
		case VotePending:
			summary.PendingVotes++
		case VoteTimeout:
			summary.TimeoutVotes++
		}
	}

	summary.AllVotesIn = summary.PendingVotes == 0

	// Determine consensus
	if summary.NoVotes > 0 || summary.TimeoutVotes > 0 {
		summary.Consensus = VoteNo
	} else if summary.YesVotes == summary.TotalParticipants {
		summary.Consensus = VoteYes
	} else {
		summary.Consensus = VotePending
	}
}

func (c *Coordinator) markTimeoutParticipants(state *MigrationState) {
	state.mu.Lock()
	defer state.mu.Unlock()

	for _, pState := range state.Participants {
		if pState.Vote == VotePending {
			pState.Vote = VoteTimeout
			pState.IsHealthy = false
			pState.ErrorMessage = "prepare timeout"
		}
	}
}

func (c *Coordinator) updateParticipantState(state *MigrationState, participantID string, newState *ParticipantState) {
	state.mu.Lock()
	defer state.mu.Unlock()

	if pState, exists := state.Participants[participantID]; exists {
		// Update relevant fields
		pState.LastHeartbeat = time.Now()
		pState.IsHealthy = true
		pState.OrdersSnapshot = newState.OrdersSnapshot
		pState.PreparationData = newState.PreparationData
	}
}

func (c *Coordinator) completeMigration(state *MigrationState) {
	now := time.Now()
	state.mu.Lock()
	state.Phase = PhaseCompleted
	state.Status = StatusCompleted
	state.EndTime = &now
	state.Progress = 100.0
	state.mu.Unlock()

	c.emitEvent(&MigrationEvent{
		ID:          uuid.New(),
		MigrationID: state.ID,
		Timestamp:   time.Now(),
		Type:        "migration_completed",
		Phase:       PhaseCompleted,
		Data: map[string]interface{}{
			"duration_ms":     now.Sub(state.StartTime).Milliseconds(),
			"orders_migrated": state.OrdersMigrated,
		},
	})
}

// Additional interface methods continue in next part...
