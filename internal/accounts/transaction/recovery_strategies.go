package transaction

import (
	"context"
	"log"
	"time"
)

// PreparingRecoveryStrategy handles recovery for transactions in preparing state
type PreparingRecoveryStrategy struct {
	manager *TransactionRecoveryManager
}

func (s *PreparingRecoveryStrategy) Recover(ctx context.Context, txn *XATransaction, session *RecoverySession) error {
	log.Printf("Recovering transaction %s from PREPARING state", txn.ID)

	// For transactions stuck in preparing state, safest to abort
	return s.manager.xaManager.Abort(ctx, txn)
}

func (s *PreparingRecoveryStrategy) CanRecover(txn *XATransaction) bool {
	// Can recover if transaction is in preparing state and not too old
	maxAge := 10 * time.Minute
	return txn.State == XAStatePreparing && time.Since(txn.UpdatedAt) < maxAge
}

func (s *PreparingRecoveryStrategy) GetPriority() int {
	return 3 // Medium priority
}

func (s *PreparingRecoveryStrategy) GetDescription() string {
	return "PreparingRecovery"
}

// PreparedRecoveryStrategy handles recovery for transactions in prepared state
type PreparedRecoveryStrategy struct {
	manager *TransactionRecoveryManager
}

func (s *PreparedRecoveryStrategy) Recover(ctx context.Context, txn *XATransaction, session *RecoverySession) error {
	log.Printf("Recovering transaction %s from PREPARED state", txn.ID)

	// For transactions in prepared state, safest to commit
	return s.manager.xaManager.Commit(ctx, txn)
}

func (s *PreparedRecoveryStrategy) CanRecover(txn *XATransaction) bool {
	return txn.State == XAStatePrepared
}

func (s *PreparedRecoveryStrategy) GetPriority() int {
	return 2 // High priority - prepared transactions should be resolved quickly
}

func (s *PreparedRecoveryStrategy) GetDescription() string {
	return "PreparedRecovery"
}

// CommittingRecoveryStrategy handles recovery for transactions in committing state
type CommittingRecoveryStrategy struct {
	manager *TransactionRecoveryManager
}

func (s *CommittingRecoveryStrategy) Recover(ctx context.Context, txn *XATransaction, session *RecoverySession) error {
	log.Printf("Recovering transaction %s from COMMITTING state", txn.ID)
	return s.manager.xaManager.Commit(ctx, txn)
}

func (s *CommittingRecoveryStrategy) CanRecover(txn *XATransaction) bool {
	return txn.State == XAStateCommitting
}

func (s *CommittingRecoveryStrategy) GetPriority() int {
	return 1 // Highest priority - committing transactions should complete quickly
}

func (s *CommittingRecoveryStrategy) GetDescription() string {
	return "CommittingRecovery"
}

// AbortingRecoveryStrategy handles recovery for transactions in aborting state
type AbortingRecoveryStrategy struct {
	manager *TransactionRecoveryManager
}

func (s *AbortingRecoveryStrategy) Recover(ctx context.Context, txn *XATransaction, session *RecoverySession) error {
	log.Printf("Recovering transaction %s from ABORTING state", txn.ID)
	return s.manager.xaManager.Abort(ctx, txn)
}

func (s *AbortingRecoveryStrategy) CanRecover(txn *XATransaction) bool {
	return txn.State == XAStateAborting
}

func (s *AbortingRecoveryStrategy) GetPriority() int {
	return 1 // Highest priority - aborting transactions should complete quickly
}

func (s *AbortingRecoveryStrategy) GetDescription() string {
	return "AbortingRecovery"
}

// UnknownStateRecoveryStrategy handles recovery for transactions in unknown state
type UnknownStateRecoveryStrategy struct {
	manager *TransactionRecoveryManager
}

func (s *UnknownStateRecoveryStrategy) Recover(ctx context.Context, txn *XATransaction, session *RecoverySession) error {
	log.Printf("Recovering transaction %s from UNKNOWN state", txn.ID)
	// For unknown state, safest to abort
	return s.manager.xaManager.Abort(ctx, txn)
}

func (s *UnknownStateRecoveryStrategy) CanRecover(txn *XATransaction) bool {
	return txn.State == XAStateUnknown
}

func (s *UnknownStateRecoveryStrategy) GetPriority() int {
	return 4 // Lower priority - requires investigation
}

func (s *UnknownStateRecoveryStrategy) GetDescription() string {
	return "UnknownStateRecovery"
}
