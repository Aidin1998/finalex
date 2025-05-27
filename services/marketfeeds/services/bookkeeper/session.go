package bookkeeper

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/litebittech/cex/common/errors"
	"github.com/litebittech/cex/common/pt"
	"github.com/litebittech/cex/services/bookkeeper/cache"
	"github.com/litebittech/cex/services/bookkeeper/txstore"
	"github.com/litebittech/cex/services/bookkeeper/types"
	"github.com/shopspring/decimal"
)

var ErrInsufficientFunds = errors.Invalid.Reason("InsufficientFunds")

type Session struct {
	txStore      txstore.Store
	balanceCache cache.BalanceCache
	usersMutex   *usersMutex

	pendingTransactions []txstore.Transaction
	pendingCommitState  cache.CommitState

	currentCheckpoint *cache.UserCheckpoint
	checkpointIsFresh bool

	unlockUser   func()
	lockedUserID *uuid.UUID
}

func (s *Session) fetchCheckpoint(ctx context.Context, userID uuid.UUID) error {
	if s.lockedUserID != nil && *s.lockedUserID != userID {
		panic("attempting to fetch checkpoint for a different user than the one that is locked")
	}

	s.checkpointIsFresh = s.lockedUserID != nil

	checkpoint, err := s.balanceCache.GetCheckpoint(ctx, userID)
	if err != nil {
		return errors.New("failed to get checkpoint").Wrap(err)
	}
	if checkpoint == nil {
		s.currentCheckpoint = &cache.UserCheckpoint{
			UserID:       userID,
			LastSequence: 0,
			IsFinal:      false,
		}
	} else {
		s.currentCheckpoint = checkpoint
	}
	return nil
}

func (s *Session) dryRunCommit(ctx context.Context, ensurePositiveBalance bool) error {
	userID := s.currentCheckpoint.UserID

	balanceKeys := affectedBalanceKeys(s.pendingTransactions)
	balances, err := s.balanceCache.GetMany(ctx, balanceKeys)
	if err != nil {
		return errors.New("failed to get balances").Wrap(err)
	}

	lastSequence := s.currentCheckpoint.LastSequence
	updatedBalances := balances.Copy()
	for _, tx := range s.pendingTransactions {
		// Ensure correct sequence
		if tx.Sequence != lastSequence+1 {
			return errors.New(fmt.Sprintf("invalid transaction sequence: expected %d, got %d", lastSequence+1, tx.Sequence))
		}
		lastSequence++

		for _, entry := range tx.Entries.Slice() {
			if entry.IsSystemAccount {
				continue
			}

			balanceKey := types.BalanceKey{
				UserID:    tx.UserID.UUID,
				AccountID: entry.AccountID.UUID,
				Currency:  entry.Currency,
			}
			maybeBalance := updatedBalances.Get(balanceKey)
			var balance decimal.Decimal
			if maybeBalance == nil {
				balance = decimal.Zero
			} else {
				balance = *maybeBalance
			}

			delta := entry.Amount
			if entry.Type.SignForLiabilty() < 0 {
				delta = delta.Neg()
			}
			balance = balance.Add(delta)
			if ensurePositiveBalance && balance.IsNegative() {
				return ErrInsufficientFunds.Explain("insufficient balance for account: %s, to process transaction: %s", entry.AccountID, tx.UserID)
			}
			updatedBalances.Set(balanceKey, balance)
		}
	}

	s.pendingCommitState = cache.CommitState{
		UserID:       userID,
		LastSequence: lastSequence,
		Balances:     updatedBalances.GetAll(userID),
	}

	return nil
}

func (s *Session) setTransactionsToCommit(transactions []txstore.Transaction) {
	if s.pendingTransactions != nil {
		panic("transactions already set for commit")
	}
	lastSequence := s.currentCheckpoint.LastSequence

	for i := range transactions {
		lastSequence++
		transactions[i].Sequence = lastSequence
	}

	s.pendingTransactions = transactions
}

func (s *Session) ensureCheckpointIsFresh(ctx context.Context) error {
	if s.lockedUserID != nil && *s.lockedUserID != s.currentCheckpoint.UserID {
		panic("attempting to operate on a different user than the one that is locked")
	}

	if s.checkpointIsFresh {
		return nil
	}

	s.lockUser(s.currentCheckpoint.UserID)

	return s.fetchCheckpoint(ctx, *s.lockedUserID)
}

func (s *Session) finalizeCheckpoint(ctx context.Context) (*cache.CommitState, error) {
	if !s.checkpointIsFresh {
		panic("attempting to finalize checkpoint that is not fresh")
	}

	if s.currentCheckpoint.IsFinal {
		return nil, errors.New("already final")
	}

	userID := s.currentCheckpoint.UserID
	result, err := s.txStore.Query(ctx, txstore.Filter{
		UserID:        userID,
		AfterSequence: s.currentCheckpoint.LastSequence,
	})
	if err != nil {
		return nil, errors.New("failed to query transactions").Wrap(err)
	}

	if s.currentCheckpoint.IsFinal {
		return nil, errors.New("current checkpoint is already marked as final")
	}

	s.setTransactionsToCommit(result.Transactions)
	if err := s.dryRunCommit(ctx, false); err != nil {
		return nil, err
	}
	commitState := s.pendingCommitState

	if err := s.commit(ctx); err != nil {
		return nil, err
	}

	s.currentCheckpoint.LastSequence = result.LastSequence
	s.currentCheckpoint.IsFinal = true

	return &commitState, nil
}

func (s *Session) GetBalance(ctx context.Context, key types.BalanceKey) (*decimal.Decimal, error) {
	balance, err := s.balanceCache.Get(ctx, key)
	if err != nil {
		return nil, errors.New("failed to get balance from cache").Wrap(err)
	}

	s.fetchCheckpoint(ctx, key.UserID)

	if s.currentCheckpoint.IsFinal {
		return &balance.Amount, nil
	}

	s.lockUser(s.currentCheckpoint.UserID)
	defer s.unlockUser()

	if err := s.fetchCheckpoint(ctx, *s.lockedUserID); err != nil {
		return nil, err
	}

	finalCommitState, err := s.finalizeCheckpoint(ctx)
	if err != nil {
		return nil, err
	}

	// TODO: Should be changed on persisted snapshots are implemented
	return pt.Pointer(finalCommitState.Balances[key.AccountID][key.Currency]), nil
}

func (s *Session) lockUser(userID uuid.UUID) {
	if s.lockedUserID != nil {
		panic(fmt.Sprintf("attempting to lock user %s when user %s is already locked", userID, *s.lockedUserID))
	}

	s.unlockUser = s.usersMutex.lockUser(userID)
	s.lockedUserID = &userID
}

func (s *Session) markCheckpointDirty(ctx context.Context) error {
	if err := s.balanceCache.MarkDirty(ctx, []uuid.UUID{s.currentCheckpoint.UserID}); err != nil {
		return err
	}
	s.currentCheckpoint.IsFinal = false
	return nil
}

func (s *Session) commit(ctx context.Context) error {
	if err := s.balanceCache.Commit(ctx, []cache.CommitState{s.pendingCommitState}); err != nil {
		return errors.New("failed to commit new cache").Wrap(err)
	}
	s.pendingTransactions = nil
	return nil
}

func (s *Session) CreateTransaction(ctx context.Context, tx *txstore.Transaction) error {
	s.lockUser(tx.UserID.UUID)
	defer s.unlockUser()

	if err := s.fetchCheckpoint(ctx, tx.UserID.UUID); err != nil {
		return err
	}

	if !s.currentCheckpoint.IsFinal {
		if _, err := s.finalizeCheckpoint(ctx); err != nil {
			return err
		}
	}

	s.setTransactionsToCommit([]txstore.Transaction{*tx})

	if err := s.dryRunCommit(ctx, true); err != nil {
		return err
	}

	if err := s.markCheckpointDirty(ctx); err != nil {
		return err
	}

	if err := s.txStore.Create(ctx, &s.pendingTransactions[0]); err != nil {
		return errors.New("failed to insert transaction in txstore").Wrap(err)
	}

	if err := s.commit(ctx); err != nil {
		return err
	}

	return nil
}

func affectedBalanceKeys(transactions []txstore.Transaction) []types.BalanceKey {
	keysMap := make(map[string]types.BalanceKey)
	for _, tx := range transactions {
		for _, entry := range tx.Entries.Slice() {
			mapKey := fmt.Sprintf("%s:%s:%s", tx.UserID.String(), entry.AccountID.String(), entry.Currency)
			keysMap[mapKey] = types.BalanceKey{
				UserID:    tx.UserID.UUID,
				AccountID: entry.AccountID.UUID,
				Currency:  entry.Currency,
			}
		}
	}

	keys := make([]types.BalanceKey, len(keysMap))
	i := 0
	for _, key := range keysMap {
		keys[i] = key
		i++
	}

	return keys
}
