// =============================
// Migration Coordinator Implementation (Part 2)
// =============================
// This file continues the coordinator implementation with interface methods
// and utility functions.

package migration

import (
	"context"
	"fmt"
	"sort"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// GetMigrationState retrieves the current state of a migration
func (c *Coordinator) GetMigrationState(migrationID uuid.UUID) (*MigrationState, error) {
	c.migrationsMu.RLock()
	defer c.migrationsMu.RUnlock()

	state, exists := c.migrations[migrationID]
	if !exists {
		return nil, fmt.Errorf("migration %s not found", migrationID)
	}

	// Return a copy to prevent external modifications
	return c.copyMigrationState(state), nil
}

// ListMigrations returns a list of migrations based on filter criteria
func (c *Coordinator) ListMigrations(filter *MigrationFilter) ([]*MigrationState, error) {
	c.migrationsMu.RLock()
	defer c.migrationsMu.RUnlock()

	var results []*MigrationState

	for _, state := range c.migrations {
		if c.matchesFilter(state, filter) {
			results = append(results, c.copyMigrationState(state))
		}
	}

	// Sort by start time (newest first)
	sort.Slice(results, func(i, j int) bool {
		return results[i].StartTime.After(results[j].StartTime)
	})

	// Apply pagination
	if filter != nil {
		if filter.Offset > 0 && filter.Offset < len(results) {
			results = results[filter.Offset:]
		}
		if filter.Limit > 0 && filter.Limit < len(results) {
			results = results[:filter.Limit]
		}
	}

	return results, nil
}

// AbortMigration aborts an active migration
func (c *Coordinator) AbortMigration(ctx context.Context, migrationID uuid.UUID, reason string) error {
	c.migrationsMu.RLock()
	state, exists := c.migrations[migrationID]
	c.migrationsMu.RUnlock()

	if !exists {
		return fmt.Errorf("migration %s not found", migrationID)
	}

	// Check if migration can be aborted
	state.mu.RLock()
	canAbort := state.Phase != PhaseCompleted && state.Phase != PhaseAborted && state.Phase != PhaseFailed
	state.mu.RUnlock()

	if !canAbort {
		return fmt.Errorf("migration %s cannot be aborted in phase %s", migrationID, state.Phase)
	}

	c.logger.Infow("Aborting migration", "migration_id", migrationID, "reason", reason)
	go c.abortMigrationInternal(ctx, state, reason)

	return nil
}

// RetryMigration retries a failed migration
func (c *Coordinator) RetryMigration(ctx context.Context, migrationID uuid.UUID) error {
	c.migrationsMu.RLock()
	state, exists := c.migrations[migrationID]
	c.migrationsMu.RUnlock()

	if !exists {
		return fmt.Errorf("migration %s not found", migrationID)
	}

	// Check if migration can be retried
	state.mu.RLock()
	canRetry := state.Phase == PhaseFailed || state.Phase == PhaseAborted
	retryCount := state.RetryCount
	maxRetries := state.Config.MaxRetries
	state.mu.RUnlock()

	if !canRetry {
		return fmt.Errorf("migration %s cannot be retried in phase %s", migrationID, state.Phase)
	}

	if retryCount >= int64(maxRetries) {
		return fmt.Errorf("migration %s has exceeded maximum retry count %d", migrationID, maxRetries)
	}

	// Reset migration state for retry
	state.mu.Lock()
	state.Phase = PhaseIdle
	state.Status = StatusPending
	state.RetryCount++
	state.StartTime = time.Now()
	state.PrepareTime = nil
	state.CommitTime = nil
	state.EndTime = nil
	state.Progress = 0.0
	state.CanRollback = true
	state.RollbackReason = ""
	state.ErrorCount = 0

	// Reset participant states
	for _, pState := range state.Participants {
		pState.Vote = VotePending
		pState.VoteTime = nil
		pState.IsHealthy = true
		pState.ErrorMessage = ""
		pState.LastHeartbeat = time.Now()
	}

	// Reset votes summary
	state.VotesSummary = VotesSummary{
		TotalParticipants: len(state.Participants),
		PendingVotes:      len(state.Participants),
		Consensus:         VotePending,
	}
	state.mu.Unlock()

	c.logger.Infow("Retrying migration", "migration_id", migrationID, "retry_count", retryCount+1)

	// Start retry execution
	go c.executeMigration(ctx, state, uuid.Nil) // Note: new lock will be acquired

	return nil
}

// RegisterParticipant registers a new migration participant
func (c *Coordinator) RegisterParticipant(participant MigrationParticipant) error {
	if participant == nil {
		return fmt.Errorf("participant cannot be nil")
	}

	participantID := participant.GetID()
	if participantID == "" {
		return fmt.Errorf("participant ID cannot be empty")
	}

	c.participantsMu.Lock()
	defer c.participantsMu.Unlock()

	if _, exists := c.participants[participantID]; exists {
		return fmt.Errorf("participant %s already registered", participantID)
	}

	c.participants[participantID] = participant
	c.logger.Infow("Registered migration participant", "participant_id", participantID, "type", participant.GetType())

	return nil
}

// UnregisterParticipant removes a migration participant
func (c *Coordinator) UnregisterParticipant(participantID string) error {
	c.participantsMu.Lock()
	defer c.participantsMu.Unlock()

	if _, exists := c.participants[participantID]; !exists {
		return fmt.Errorf("participant %s not found", participantID)
	}

	delete(c.participants, participantID)
	c.logger.Infow("Unregistered migration participant", "participant_id", participantID)

	return nil
}

// GetMetrics returns migration metrics
func (c *Coordinator) GetMetrics(migrationID uuid.UUID) (*MigrationMetrics, error) {
	state, err := c.GetMigrationState(migrationID)
	if err != nil {
		return nil, err
	}

	return state.Metrics, nil
}

// GetHealthStatus returns migration health status
func (c *Coordinator) GetHealthStatus(migrationID uuid.UUID) (*HealthStatus, error) {
	state, err := c.GetMigrationState(migrationID)
	if err != nil {
		return nil, err
	}

	return state.HealthStatus, nil
}

// Subscribe creates an event subscription for a migration
func (c *Coordinator) Subscribe(ctx context.Context, migrationID uuid.UUID) (<-chan *MigrationEvent, error) {
	eventChan := make(chan *MigrationEvent, 100)

	c.eventsMu.Lock()
	if c.eventSubscribers[migrationID] == nil {
		c.eventSubscribers[migrationID] = make([]chan *MigrationEvent, 0)
	}
	c.eventSubscribers[migrationID] = append(c.eventSubscribers[migrationID], eventChan)
	c.eventsMu.Unlock()

	// Clean up subscription when context is done
	go func() {
		<-ctx.Done()
		c.unsubscribe(migrationID, eventChan)
	}()

	return eventChan, nil
}

// Shutdown gracefully shuts down the coordinator
func (c *Coordinator) Shutdown(ctx context.Context) error {
	c.logger.Info("Shutting down migration coordinator")

	// Stop background workers
	close(c.stopChan)
	if c.healthTicker != nil {
		c.healthTicker.Stop()
	}
	if c.cleanupTicker != nil {
		c.cleanupTicker.Stop()
	}

	// Wait for active migrations to complete or timeout
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(30 * time.Second)
	}

	for time.Now().Before(deadline) {
		if c.getActiveMigrationsCount() == 0 {
			break
		}
		time.Sleep(1 * time.Second)
	}

	// Close all event subscribers
	c.eventsMu.Lock()
	for _, subscribers := range c.eventSubscribers {
		for _, ch := range subscribers {
			close(ch)
		}
	}
	c.eventSubscribers = make(map[uuid.UUID][]chan *MigrationEvent)
	c.eventsMu.Unlock()

	c.logger.Info("Migration coordinator shutdown complete")
	return nil
}

// Helper methods

func (c *Coordinator) validateMigrationRequest(request *MigrationRequest) error {
	if request.ID == uuid.Nil {
		return fmt.Errorf("migration ID is required")
	}
	if request.Pair == "" {
		return fmt.Errorf("pair is required")
	}
	if request.Config == nil {
		return fmt.Errorf("migration config is required")
	}
	if request.Config.TargetImplementation == "" {
		return fmt.Errorf("target implementation is required")
	}
	if request.Config.MigrationPercentage < 0 || request.Config.MigrationPercentage > 100 {
		return fmt.Errorf("migration percentage must be between 0 and 100")
	}
	return nil
}

func (c *Coordinator) getActiveMigrationsCount() int {
	c.migrationsMu.RLock()
	defer c.migrationsMu.RUnlock()

	count := 0
	for _, state := range c.migrations {
		state.mu.RLock()
		isActive := state.Phase != PhaseCompleted && state.Phase != PhaseAborted && state.Phase != PhaseFailed
		state.mu.RUnlock()
		if isActive {
			count++
		}
	}
	return count
}

func (c *Coordinator) isPairBeingMigrated(pair string) bool {
	c.migrationsMu.RLock()
	defer c.migrationsMu.RUnlock()

	for _, state := range c.migrations {
		state.mu.RLock()
		isActivePair := state.Pair == pair &&
			state.Phase != PhaseCompleted &&
			state.Phase != PhaseAborted &&
			state.Phase != PhaseFailed
		state.mu.RUnlock()
		if isActivePair {
			return true
		}
	}
	return false
}

func (c *Coordinator) getParticipant(participantID string) MigrationParticipant {
	c.participantsMu.RLock()
	defer c.participantsMu.RUnlock()
	return c.participants[participantID]
}

func (c *Coordinator) copyMigrationState(state *MigrationState) *MigrationState {
	state.mu.RLock()
	defer state.mu.RUnlock()

	// Create a deep copy of the migration state
	copy := &MigrationState{
		ID:             state.ID,
		Pair:           state.Pair,
		Phase:          state.Phase,
		Status:         state.Status,
		Config:         state.Config, // Config is immutable, shallow copy is fine
		StartTime:      state.StartTime,
		Progress:       state.Progress,
		OrdersMigrated: state.OrdersMigrated,
		TotalOrders:    state.TotalOrders,
		ErrorCount:     state.ErrorCount,
		RetryCount:     state.RetryCount,
		CanRollback:    state.CanRollback,
		RollbackReason: state.RollbackReason,
		VotesSummary:   state.VotesSummary,
	}

	// Copy pointers if they exist
	if state.PrepareTime != nil {
		prepareTime := *state.PrepareTime
		copy.PrepareTime = &prepareTime
	}
	if state.CommitTime != nil {
		commitTime := *state.CommitTime
		copy.CommitTime = &commitTime
	}
	if state.EndTime != nil {
		endTime := *state.EndTime
		copy.EndTime = &endTime
	}

	// Copy participants map
	copy.Participants = make(map[string]*ParticipantState)
	for id, pState := range state.Participants {
		copyPState := *pState // Shallow copy is sufficient for most fields
		if pState.VoteTime != nil {
			voteTime := *pState.VoteTime
			copyPState.VoteTime = &voteTime
		}
		copy.Participants[id] = &copyPState
	}

	// Copy metrics and health status (shallow copy is sufficient)
	if state.Metrics != nil {
		metricsCopy := *state.Metrics
		copy.Metrics = &metricsCopy
	}
	if state.HealthStatus != nil {
		healthCopy := *state.HealthStatus
		copy.HealthStatus = &healthCopy
	}

	return copy
}

func (c *Coordinator) matchesFilter(state *MigrationState, filter *MigrationFilter) bool {
	if filter == nil {
		return true
	}

	state.mu.RLock()
	defer state.mu.RUnlock()

	if filter.Pair != "" && state.Pair != filter.Pair {
		return false
	}
	if filter.Status != "" && state.Status != filter.Status {
		return false
	}
	if filter.Phase != 0 && state.Phase != filter.Phase {
		return false
	}
	if filter.StartTime != nil && state.StartTime.Before(*filter.StartTime) {
		return false
	}
	if filter.EndTime != nil && state.EndTime != nil && state.EndTime.After(*filter.EndTime) {
		return false
	}

	return true
}

func (c *Coordinator) emitEvent(event *MigrationEvent) {
	c.eventsMu.RLock()
	subscribers := c.eventSubscribers[event.MigrationID]
	c.eventsMu.RUnlock()

	// Send event to all subscribers (non-blocking)
	for _, ch := range subscribers {
		select {
		case ch <- event:
		default:
			// Channel is full, skip this subscriber
			c.logger.Warnw("Event channel full, dropping event",
				"migration_id", event.MigrationID,
				"event_type", event.Type)
		}
	}
}

func (c *Coordinator) unsubscribe(migrationID uuid.UUID, eventChan chan *MigrationEvent) {
	c.eventsMu.Lock()
	defer c.eventsMu.Unlock()

	subscribers := c.eventSubscribers[migrationID]
	for i, ch := range subscribers {
		if ch == eventChan {
			// Remove this subscriber
			c.eventSubscribers[migrationID] = append(subscribers[:i], subscribers[i+1:]...)
			close(eventChan)
			break
		}
	}

	// Clean up empty subscriber list
	if len(c.eventSubscribers[migrationID]) == 0 {
		delete(c.eventSubscribers, migrationID)
	}
}

func (c *Coordinator) recoverMigrations() {
	c.logger.Info("Recovering migrations from persistence")

	migrations, err := c.persistence.LoadMigrationStates()
	if err != nil {
		c.logger.Errorw("Failed to recover migrations", "error", err)
		return
	}

	c.migrationsMu.Lock()
	for _, state := range migrations {
		c.migrations[state.ID] = state

		// Update metrics
		switch state.Status {
		case StatusCompleted:
			atomic.AddInt64(&c.successfulMigrations, 1)
		case StatusFailed, StatusAborted:
			atomic.AddInt64(&c.failedMigrations, 1)
		}
		atomic.AddInt64(&c.totalMigrations, 1)
	}
	c.migrationsMu.Unlock()

	c.logger.Infow("Recovered migrations", "count", len(migrations))
}

func (c *Coordinator) startBackgroundWorkers() {
	// Health check worker
	c.healthTicker = time.NewTicker(c.config.HealthCheckInterval)
	go c.healthCheckWorker()

	// Cleanup worker
	c.cleanupTicker = time.NewTicker(c.config.CleanupInterval)
	go c.cleanupWorker()
}

func (c *Coordinator) healthCheckWorker() {
	for {
		select {
		case <-c.healthTicker.C:
			c.performHealthChecks()
		case <-c.stopChan:
			return
		}
	}
}

func (c *Coordinator) cleanupWorker() {
	for {
		select {
		case <-c.cleanupTicker.C:
			c.cleanupOldMigrations()
		case <-c.stopChan:
			return
		}
	}
}

func (c *Coordinator) performHealthChecks() {
	c.migrationsMu.RLock()
	activeMigrations := make([]*MigrationState, 0)
	for _, state := range c.migrations {
		state.mu.RLock()
		isActive := state.Phase != PhaseCompleted && state.Phase != PhaseAborted && state.Phase != PhaseFailed
		state.mu.RUnlock()
		if isActive {
			activeMigrations = append(activeMigrations, state)
		}
	}
	c.migrationsMu.RUnlock()

	for _, state := range activeMigrations {
		c.performMigrationHealthCheck(state)
	}
}

func (c *Coordinator) performMigrationHealthCheck(state *MigrationState) {
	// Check participant health
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	state.mu.Lock()
	for participantID, pState := range state.Participants {
		// Check if participant is stale
		if time.Since(pState.LastHeartbeat) > 1*time.Minute {
			pState.IsHealthy = false
			pState.ErrorMessage = "heartbeat timeout"
		}

		// Perform health check if participant is available
		participant := c.getParticipant(participantID)
		if participant != nil {
			healthCheck, err := participant.HealthCheck(ctx)
			if err != nil {
				pState.IsHealthy = false
				pState.ErrorMessage = err.Error()
			} else {
				state.HealthStatus.HealthChecks[participantID] = healthCheck
			}
		}
	}

	// Update overall health status
	healthyCount := 0
	for _, pState := range state.Participants {
		if pState.IsHealthy {
			healthyCount++
		}
	}

	state.HealthStatus.IsHealthy = healthyCount == len(state.Participants)
	state.HealthStatus.LastCheck = time.Now()
	state.HealthStatus.OverallScore = float64(healthyCount) / float64(len(state.Participants))
	state.mu.Unlock()
}

func (c *Coordinator) cleanupOldMigrations() {
	cutoff := time.Now().Add(-c.config.StateRetentionPeriod)

	c.migrationsMu.Lock()
	defer c.migrationsMu.Unlock()

	var toDelete []uuid.UUID
	for id, state := range c.migrations {
		state.mu.RLock()
		shouldDelete := (state.Phase == PhaseCompleted || state.Phase == PhaseAborted || state.Phase == PhaseFailed) &&
			state.StartTime.Before(cutoff)
		state.mu.RUnlock()

		if shouldDelete {
			toDelete = append(toDelete, id)
		}
	}

	for _, id := range toDelete {
		delete(c.migrations, id)

		// Clean up event subscribers
		c.eventsMu.Lock()
		if subscribers, exists := c.eventSubscribers[id]; exists {
			for _, ch := range subscribers {
				close(ch)
			}
			delete(c.eventSubscribers, id)
		}
		c.eventsMu.Unlock()

		// Remove from persistence
		if c.config.PersistenceEnabled {
			if err := c.persistence.DeleteMigrationState(id); err != nil {
				c.logger.Errorw("Failed to delete migration state from persistence", "error", err, "migration_id", id)
			}
		}
	}

	if len(toDelete) > 0 {
		c.logger.Infow("Cleaned up old migrations", "count", len(toDelete))
	}
}
