// Package state provides transaction state management for the wallet service
package state

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/Aidin1998/finalex/internal/wallet/interfaces"
	"github.com/Aidin1998/finalex/pkg/logger"
)

// TransactionStateMachine manages transaction state transitions
type TransactionStateMachine struct {
	repository interfaces.WalletRepository
	cache      interfaces.WalletCache
	publisher  interfaces.EventPublisher
	log        logger.Logger
}

// StateTransition represents a state transition record
type StateTransition struct {
	TxID      uuid.UUID              `json:"tx_id"`
	FromState interfaces.TxStatus    `json:"from_state"`
	ToState   interfaces.TxStatus    `json:"to_state"`
	Timestamp time.Time              `json:"timestamp"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// TransitionResult represents the result of a state transition
type TransitionResult struct {
	Success   bool                `json:"success"`
	FromState interfaces.TxStatus `json:"from_state"`
	ToState   interfaces.TxStatus `json:"to_state"`
	Error     error               `json:"error,omitempty"`
	Timestamp time.Time           `json:"timestamp"`
}

// NewTransactionStateMachine creates a new transaction state machine
func NewTransactionStateMachine(
	repository interfaces.WalletRepository,
	cache interfaces.WalletCache,
	publisher interfaces.EventPublisher,
	log logger.Logger,
) *TransactionStateMachine {
	return &TransactionStateMachine{
		repository: repository,
		cache:      cache,
		publisher:  publisher,
		log:        log,
	}
}

// TransitionTo transitions a transaction to a new state
func (sm *TransactionStateMachine) TransitionTo(
	ctx context.Context,
	txID uuid.UUID,
	newState interfaces.TxStatus,
	metadata map[string]interface{},
) (*TransitionResult, error) {
	// Get current transaction
	tx, err := sm.repository.GetTransaction(ctx, txID)
	if err != nil {
		sm.log.Error("failed to get transaction for state transition",
			zap.Error(err),
			zap.String("tx_id", txID.String()))
		return &TransitionResult{
			Success: false,
			Error:   err,
		}, err
	}

	// Check if transition is valid
	if !sm.IsValidTransition(tx.Status, newState) {
		err := fmt.Errorf("invalid state transition from %s to %s", tx.Status, newState)
		sm.log.Warn("invalid state transition attempted",
			zap.String("tx_id", txID.String()),
			zap.String("from_state", string(tx.Status)),
			zap.String("to_state", string(newState)),
		)
		return &TransitionResult{
			Success: false,
			Error:   err,
		}, err
	}

	// Create state transition record
	transition := StateTransition{
		TxID:      txID,
		FromState: tx.Status,
		ToState:   newState,
		Timestamp: time.Now(),
		Metadata:  metadata,
	}

	// Execute the transition
	result, err := sm.executeTransition(ctx, tx, newState, transition)
	if err != nil {
		sm.log.Error("failed to execute state transition",
			zap.Error(err),
			zap.String("tx_id", txID.String()),
			zap.String("from_state", string(transition.FromState)),
			zap.String("to_state", string(transition.ToState)),
		)
		return result, err
	}

	sm.log.Info("transaction state transition completed",
		zap.String("tx_id", txID.String()),
		zap.String("from_state", string(transition.FromState)),
		zap.String("to_state", string(transition.ToState)),
		zap.Bool("success", result.Success),
	)

	return result, nil
}

// executeTransition performs the actual state transition
func (sm *TransactionStateMachine) executeTransition(
	ctx context.Context,
	tx *interfaces.WalletTransaction,
	newState interfaces.TxStatus,
	transition StateTransition,
) (*TransitionResult, error) {
	// Update transaction status
	updates := map[string]interface{}{
		"status":     newState,
		"updated_at": time.Now(),
	}

	if err := sm.repository.UpdateTransaction(ctx, tx.ID, updates); err != nil {
		return &TransitionResult{
			Success:   false,
			FromState: transition.FromState,
			ToState:   newState,
			Error:     err,
			Timestamp: time.Now(),
		}, err
	}

	// Update in-memory transaction
	tx.Status = newState
	tx.UpdatedAt = time.Now()

	// Update cache with TTL
	if sm.cache != nil {
		if err := sm.cache.SetTransaction(ctx, tx, 24*time.Hour); err != nil {
			sm.log.Warn("failed to update transaction cache",
				zap.Error(err),
				zap.String("tx_id", tx.ID.String()))
		}
	}
	// Publish state change event
	if sm.publisher != nil {
		txIDPtr := tx.ID
		amountPtr := tx.Amount

		event := interfaces.WalletEvent{
			ID:        uuid.New(),
			Type:      "transaction_state_changed",
			EventType: "transaction_state_changed",
			UserID:    tx.UserID,
			Asset:     tx.Asset,
			Amount:    &amountPtr,
			Direction: tx.Direction,
			TxID:      &txIDPtr,
			Status:    string(newState),
			Message:   fmt.Sprintf("Transaction transitioned from %s to %s", transition.FromState, newState),
			Metadata: map[string]interface{}{
				"from_state":      string(transition.FromState),
				"to_state":        string(newState),
				"transition_time": transition.Timestamp,
			},
			Timestamp: time.Now(),
		}

		if err := sm.publisher.PublishWalletEvent(ctx, event); err != nil {
			sm.log.Warn("failed to publish state change event",
				zap.Error(err),
				zap.String("tx_id", tx.ID.String()))
		}
	}

	// Handle state-specific logic
	switch newState {
	case interfaces.StatusCompleted:
		sm.handleCompletedState(ctx, tx)
	case interfaces.StatusFailed:
		sm.handleFailedState(ctx, tx)
	case interfaces.StatusRejected:
		sm.handleRejectedState(ctx, tx)
	case interfaces.StatusCancelled:
		sm.handleCancelledState(ctx, tx)
	}

	return &TransitionResult{
		Success:   true,
		FromState: transition.FromState,
		ToState:   newState,
		Error:     nil,
		Timestamp: time.Now(),
	}, nil
}

// handleCompletedState handles logic for completed transactions
func (sm *TransactionStateMachine) handleCompletedState(ctx context.Context, tx *interfaces.WalletTransaction) {
	// For deposits, no fund locks to release
	// For withdrawals, fund locks should already be consumed
	sm.log.Info("transaction completed successfully",
		zap.String("tx_id", tx.ID.String()),
		zap.String("direction", string(tx.Direction)))
}

// handleFailedState handles logic for failed transactions
func (sm *TransactionStateMachine) handleFailedState(ctx context.Context, tx *interfaces.WalletTransaction) {
	// For withdrawals, release fund locks
	if tx.Direction == interfaces.DirectionWithdrawal {
		if err := sm.repository.ReleaseFundLock(ctx, tx.ID); err != nil {
			sm.log.Warn("failed to release fund lock for failed transaction",
				zap.Error(err),
				zap.String("tx_id", tx.ID.String()))
		}
	}
}

// handleRejectedState handles logic for rejected transactions
func (sm *TransactionStateMachine) handleRejectedState(ctx context.Context, tx *interfaces.WalletTransaction) {
	// For withdrawals, release fund locks
	if tx.Direction == interfaces.DirectionWithdrawal {
		if err := sm.repository.ReleaseFundLock(ctx, tx.ID); err != nil {
			sm.log.Warn("failed to release fund lock for rejected transaction",
				zap.Error(err),
				zap.String("tx_id", tx.ID.String()))
		}
	}
}

// handleCancelledState handles logic for cancelled transactions
func (sm *TransactionStateMachine) handleCancelledState(ctx context.Context, tx *interfaces.WalletTransaction) {
	// For withdrawals, release fund locks
	if tx.Direction == interfaces.DirectionWithdrawal {
		if err := sm.repository.ReleaseFundLock(ctx, tx.ID); err != nil {
			sm.log.Warn("failed to release fund lock for cancelled transaction",
				zap.Error(err),
				zap.String("tx_id", tx.ID.String()))
		}
	}
}

// IsValidTransition checks if a state transition is valid
func (sm *TransactionStateMachine) IsValidTransition(from, to interfaces.TxStatus) bool {
	validTransitions := map[interfaces.TxStatus][]interfaces.TxStatus{
		interfaces.StatusPending: {
			interfaces.StatusProcessing,
			interfaces.StatusFailed,
			interfaces.StatusCancelled,
			interfaces.StatusRejected,
		},
		interfaces.StatusProcessing: {
			interfaces.StatusCompleted,
			interfaces.StatusFailed,
			interfaces.StatusCancelled,
		},
		interfaces.StatusCompleted: {}, // Terminal state
		interfaces.StatusFailed:    {}, // Terminal state
		interfaces.StatusCancelled: {}, // Terminal state
		interfaces.StatusRejected:  {}, // Terminal state
	}

	allowed, exists := validTransitions[from]
	if !exists {
		return false
	}

	for _, allowedState := range allowed {
		if allowedState == to {
			return true
		}
	}

	return false
}

// GetValidTransitions returns valid transitions from current state
func (sm *TransactionStateMachine) GetValidTransitions(from interfaces.TxStatus) []interfaces.TxStatus {
	validTransitions := map[interfaces.TxStatus][]interfaces.TxStatus{
		interfaces.StatusPending: {
			interfaces.StatusProcessing,
			interfaces.StatusFailed,
			interfaces.StatusCancelled,
			interfaces.StatusRejected,
		},
		interfaces.StatusProcessing: {
			interfaces.StatusCompleted,
			interfaces.StatusFailed,
			interfaces.StatusCancelled,
		},
		interfaces.StatusCompleted: {},
		interfaces.StatusFailed:    {},
		interfaces.StatusCancelled: {},
		interfaces.StatusRejected:  {},
	}

	return validTransitions[from]
}

// GetCurrentState returns the current state of a transaction
func (sm *TransactionStateMachine) GetCurrentState(ctx context.Context, txID uuid.UUID) (interfaces.TxStatus, error) {
	tx, err := sm.repository.GetTransaction(ctx, txID)
	if err != nil {
		return "", err
	}
	return tx.Status, nil
}

// CanTransitionTo checks if a transaction can transition to a new state
func (sm *TransactionStateMachine) CanTransitionTo(ctx context.Context, txID uuid.UUID, newState interfaces.TxStatus) (bool, error) {
	tx, err := sm.repository.GetTransaction(ctx, txID)
	if err != nil {
		return false, err
	}
	return sm.IsValidTransition(tx.Status, newState), nil
}

// GetAllowedTransitions returns allowed transitions from the current state
func (sm *TransactionStateMachine) GetAllowedTransitions(ctx context.Context, txID uuid.UUID) ([]interfaces.TxStatus, error) {
	tx, err := sm.repository.GetTransaction(ctx, txID)
	if err != nil {
		return nil, err
	}
	return sm.GetValidTransitions(tx.Status), nil
}

// Remove duplicate type/function declarations below this line if present
