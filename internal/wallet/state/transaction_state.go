// Package state provides transaction state management for the wallet service
package state

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"your-project/internal/wallet/interfaces"
	"your-project/pkg/logger"
)

// TransactionStateMachine manages transaction state transitions
type TransactionStateMachine struct {
	repository interfaces.WalletRepository
	cache      interfaces.WalletCache
	publisher  interfaces.EventPublisher
	log        logger.Logger
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

// StateTransition represents a state transition
type StateTransition struct {
	FromState interfaces.TxStatus
	ToState   interfaces.TxStatus
	Action    string
	Metadata  map[string]interface{}
}

// TransitionResult represents the result of a state transition
type TransitionResult struct {
	Success  bool
	NewState interfaces.TxStatus
	Error    error
	Metadata map[string]interface{}
}

// ValidTransitions defines allowed state transitions
var ValidTransitions = map[interfaces.TxStatus][]interfaces.TxStatus{
	interfaces.TxStatusInitiated: {
		interfaces.TxStatusPending,
		interfaces.TxStatusRejected,
		interfaces.TxStatusFailed,
	},
	interfaces.TxStatusPending: {
		interfaces.TxStatusConfirming,
		interfaces.TxStatusRejected,
		interfaces.TxStatusFailed,
		interfaces.TxStatusCancelled,
	},
	interfaces.TxStatusConfirming: {
		interfaces.TxStatusConfirmed,
		interfaces.TxStatusFailed,
	},
	interfaces.TxStatusConfirmed: {
		interfaces.TxStatusCompleted,
		interfaces.TxStatusFailed,
	},
	// Terminal states - no transitions allowed
	interfaces.TxStatusCompleted: {},
	interfaces.TxStatusRejected:  {},
	interfaces.TxStatusFailed:    {},
	interfaces.TxStatusCancelled: {},
}

// IsValidTransition checks if a state transition is valid
func (sm *TransactionStateMachine) IsValidTransition(from, to interfaces.TxStatus) bool {
	allowedStates, exists := ValidTransitions[from]
	if !exists {
		return false
	}

	for _, allowed := range allowedStates {
		if allowed == to {
			return true
		}
	}

	return false
}

// TransitionTo moves a transaction to a new state
func (sm *TransactionStateMachine) TransitionTo(ctx context.Context, txID uuid.UUID, newState interfaces.TxStatus, metadata map[string]interface{}) (*TransitionResult, error) {
	// Get current transaction
	tx, err := sm.repository.GetTransaction(ctx, txID)
	if err != nil {
		sm.log.Error("failed to get transaction for state transition", "error", err, "tx_id", txID)
		return &TransitionResult{
			Success: false,
			Error:   err,
		}, err
	}

	// Check if transition is valid
	if !sm.IsValidTransition(tx.Status, newState) {
		err := fmt.Errorf("invalid state transition from %s to %s", tx.Status, newState)
		sm.log.Warn("invalid state transition attempted",
			"tx_id", txID,
			"from_state", tx.Status,
			"to_state", newState,
		)
		return &TransitionResult{
			Success: false,
			Error:   err,
		}, err
	}

	// Create state transition record
	transition := StateTransition{
		FromState: tx.Status,
		ToState:   newState,
		Action:    fmt.Sprintf("transition_%s_to_%s", tx.Status, newState),
		Metadata:  metadata,
	}

	// Execute transition
	result, err := sm.executeTransition(ctx, tx, transition)
	if err != nil {
		sm.log.Error("failed to execute state transition",
			"error", err,
			"tx_id", txID,
			"transition", transition,
		)
		return result, err
	}

	sm.log.Info("transaction state transition completed",
		"tx_id", txID,
		"from_state", transition.FromState,
		"to_state", transition.ToState,
		"success", result.Success,
	)

	return result, nil
}

// executeTransition performs the actual state transition
func (sm *TransactionStateMachine) executeTransition(ctx context.Context, tx *interfaces.WalletTransaction, transition StateTransition) (*TransitionResult, error) {
	// Update transaction status
	tx.Status = transition.ToState
	tx.UpdatedAt = time.Now()

	// Merge metadata
	if tx.Metadata == nil {
		tx.Metadata = make(interfaces.JSONB)
	}
	for k, v := range transition.Metadata {
		tx.Metadata[k] = v
	}

	// Handle state-specific logic
	if err := sm.handleStateLogic(ctx, tx, transition); err != nil {
		return &TransitionResult{
			Success: false,
			Error:   err,
		}, err
	}

	// Update in database
	if err := sm.repository.UpdateTransaction(ctx, tx); err != nil {
		return &TransitionResult{
			Success: false,
			Error:   err,
		}, err
	}

	// Update cache
	if sm.cache != nil {
		if err := sm.cache.SetTransaction(ctx, tx); err != nil {
			sm.log.Warn("failed to update transaction cache", "error", err, "tx_id", tx.ID)
		}
	}

	// Publish state change event
	if sm.publisher != nil {
		event := interfaces.WalletEvent{
			Type:      "transaction_state_changed",
			UserID:    tx.UserID,
			TxID:      tx.ID,
			Asset:     tx.Asset,
			Amount:    tx.Amount,
			Direction: tx.Direction,
			Status:    tx.Status,
			Metadata: map[string]interface{}{
				"previous_state":      string(transition.FromState),
				"new_state":           string(transition.ToState),
				"transition_metadata": transition.Metadata,
			},
			Timestamp: time.Now(),
		}

		if err := sm.publisher.PublishWalletEvent(ctx, event); err != nil {
			sm.log.Warn("failed to publish state change event", "error", err, "tx_id", tx.ID)
		}
	}

	return &TransitionResult{
		Success:  true,
		NewState: transition.ToState,
		Metadata: transition.Metadata,
	}, nil
}

// handleStateLogic executes state-specific business logic
func (sm *TransactionStateMachine) handleStateLogic(ctx context.Context, tx *interfaces.WalletTransaction, transition StateTransition) error {
	switch transition.ToState {
	case interfaces.TxStatusPending:
		return sm.handlePendingState(ctx, tx, transition)
	case interfaces.TxStatusConfirming:
		return sm.handleConfirmingState(ctx, tx, transition)
	case interfaces.TxStatusConfirmed:
		return sm.handleConfirmedState(ctx, tx, transition)
	case interfaces.TxStatusCompleted:
		return sm.handleCompletedState(ctx, tx, transition)
	case interfaces.TxStatusFailed:
		return sm.handleFailedState(ctx, tx, transition)
	case interfaces.TxStatusRejected:
		return sm.handleRejectedState(ctx, tx, transition)
	case interfaces.TxStatusCancelled:
		return sm.handleCancelledState(ctx, tx, transition)
	}
	return nil
}

// handlePendingState processes pending state logic
func (sm *TransactionStateMachine) handlePendingState(ctx context.Context, tx *interfaces.WalletTransaction, transition StateTransition) error {
	// For withdrawals, lock funds
	if tx.Direction == interfaces.DirectionWithdrawal {
		// Fund locking should have been done earlier, just verify
		locks, err := sm.repository.GetFundLocks(ctx, tx.UserID, tx.Asset)
		if err != nil {
			return fmt.Errorf("failed to verify fund locks: %w", err)
		}

		locked := false
		for _, lock := range locks {
			if lock.TxRef == tx.ID.String() {
				locked = true
				break
			}
		}

		if !locked {
			return fmt.Errorf("funds not locked for withdrawal transaction %s", tx.ID)
		}
	}

	return nil
}

// handleConfirmingState processes confirming state logic
func (sm *TransactionStateMachine) handleConfirmingState(ctx context.Context, tx *interfaces.WalletTransaction, transition StateTransition) error {
	// Update confirmations if provided in metadata
	if confirmations, ok := transition.Metadata["confirmations"].(int); ok {
		tx.Confirmations = confirmations
	}

	// Update transaction hash if provided
	if txHash, ok := transition.Metadata["tx_hash"].(string); ok {
		tx.TxHash = txHash
	}

	return nil
}

// handleConfirmedState processes confirmed state logic
func (sm *TransactionStateMachine) handleConfirmedState(ctx context.Context, tx *interfaces.WalletTransaction, transition StateTransition) error {
	// Mark transaction as confirmed
	tx.Confirmations = tx.RequiredConf

	// For deposits, credit user balance
	if tx.Direction == interfaces.DirectionDeposit {
		if err := sm.repository.CreditBalance(ctx, tx.UserID, tx.Asset, tx.Amount); err != nil {
			return fmt.Errorf("failed to credit balance: %w", err)
		}

		// Invalidate balance cache
		if sm.cache != nil {
			if err := sm.cache.InvalidateBalance(ctx, tx.UserID, tx.Asset); err != nil {
				sm.log.Warn("failed to invalidate balance cache", "error", err)
			}
		}
	}

	return nil
}

// handleCompletedState processes completed state logic
func (sm *TransactionStateMachine) handleCompletedState(ctx context.Context, tx *interfaces.WalletTransaction, transition StateTransition) error {
	// For withdrawals, release fund locks and debit balance
	if tx.Direction == interfaces.DirectionWithdrawal {
		// Release fund locks
		if err := sm.repository.ReleaseFundLock(ctx, tx.UserID, tx.Asset, tx.ID.String()); err != nil {
			sm.log.Warn("failed to release fund lock", "error", err, "tx_id", tx.ID)
		}

		// Debit balance
		if err := sm.repository.DebitBalance(ctx, tx.UserID, tx.Asset, tx.Amount); err != nil {
			return fmt.Errorf("failed to debit balance: %w", err)
		}

		// Invalidate balance cache
		if sm.cache != nil {
			if err := sm.cache.InvalidateBalance(ctx, tx.UserID, tx.Asset); err != nil {
				sm.log.Warn("failed to invalidate balance cache", "error", err)
			}
		}
	}

	return nil
}

// handleFailedState processes failed state logic
func (sm *TransactionStateMachine) handleFailedState(ctx context.Context, tx *interfaces.WalletTransaction, transition StateTransition) error {
	// Set error message if provided
	if errorMsg, ok := transition.Metadata["error"].(string); ok {
		tx.ErrorMsg = errorMsg
	}

	// For withdrawals, release fund locks
	if tx.Direction == interfaces.DirectionWithdrawal {
		if err := sm.repository.ReleaseFundLock(ctx, tx.UserID, tx.Asset, tx.ID.String()); err != nil {
			sm.log.Warn("failed to release fund lock for failed transaction", "error", err, "tx_id", tx.ID)
		}
	}

	return nil
}

// handleRejectedState processes rejected state logic
func (sm *TransactionStateMachine) handleRejectedState(ctx context.Context, tx *interfaces.WalletTransaction, transition StateTransition) error {
	// Set rejection reason if provided
	if reason, ok := transition.Metadata["reason"].(string); ok {
		tx.ErrorMsg = reason
	}

	// For withdrawals, release fund locks
	if tx.Direction == interfaces.DirectionWithdrawal {
		if err := sm.repository.ReleaseFundLock(ctx, tx.UserID, tx.Asset, tx.ID.String()); err != nil {
			sm.log.Warn("failed to release fund lock for rejected transaction", "error", err, "tx_id", tx.ID)
		}
	}

	return nil
}

// handleCancelledState processes cancelled state logic
func (sm *TransactionStateMachine) handleCancelledState(ctx context.Context, tx *interfaces.WalletTransaction, transition StateTransition) error {
	// For withdrawals, release fund locks
	if tx.Direction == interfaces.DirectionWithdrawal {
		if err := sm.repository.ReleaseFundLock(ctx, tx.UserID, tx.Asset, tx.ID.String()); err != nil {
			sm.log.Warn("failed to release fund lock for cancelled transaction", "error", err, "tx_id", tx.ID)
		}
	}

	return nil
}

// GetTransactionHistory returns state transition history for a transaction
func (sm *TransactionStateMachine) GetTransactionHistory(ctx context.Context, txID uuid.UUID) ([]StateTransition, error) {
	// This would require a separate table to store state transition history
	// For now, return empty slice - this can be implemented later
	return []StateTransition{}, nil
}

// IsTerminalState checks if a state is terminal (no further transitions allowed)
func (sm *TransactionStateMachine) IsTerminalState(state interfaces.TxStatus) bool {
	allowedStates, exists := ValidTransitions[state]
	return exists && len(allowedStates) == 0
}

// GetValidTransitions returns valid transitions from current state
func (sm *TransactionStateMachine) GetValidTransitions(currentState interfaces.TxStatus) []interfaces.TxStatus {
	if allowed, exists := ValidTransitions[currentState]; exists {
		return allowed
	}
	return []interfaces.TxStatus{}
}
