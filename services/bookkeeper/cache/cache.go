package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/litebittech/cex/common/errors"
	"github.com/litebittech/cex/services/bookkeeper/types"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
)

// Checkpoint stores account-level checkpoint
type Checkpoint struct {
	LastSequence int  `json:"lastSequence"`
	IsFinal      bool `json:"isFinal"`
}

type UserCheckpoint struct {
	UserID       uuid.UUID
	LastSequence int
	IsFinal      bool
}

// Balance represents a currency balance
type Balance struct {
	types.BalanceKey
	Amount decimal.Decimal
}

type CommitState struct {
	UserID       uuid.UUID
	Balances     map[uuid.UUID]map[string]decimal.Decimal // Map accountID -> currency -> amount
	LastSequence int
}

type BalanceCache interface {
	// Get a specific currency balance for an account
	Get(ctx context.Context, key types.BalanceKey) (*Balance, error)

	// Get account checkpoint data (LastSequence and dirty flag)
	GetCheckpoint(ctx context.Context, userID uuid.UUID) (*UserCheckpoint, error)

	// Get checkpoint data for multiple accounts
	GetManyCheckpoints(ctx context.Context, userIDs []uuid.UUID) (map[uuid.UUID]*UserCheckpoint, error)

	// Get multiple currency balances
	GetMany(ctx context.Context, keys []types.BalanceKey) (*types.BalanceMap, error)

	// Commit updates balances, checkpoint and removes dirty flag in a single transaction for multiple accounts
	Commit(ctx context.Context, commits []CommitState) error

	// Mark accounts as dirty
	MarkDirty(ctx context.Context, accountIDs []uuid.UUID) error
}

type BalanceCacheImp struct {
	client     *redis.Client
	expiration time.Duration
}

var _ BalanceCache = (*BalanceCacheImp)(nil)

func NewBalanceCache(client *redis.Client, expiration time.Duration) *BalanceCacheImp {
	return &BalanceCacheImp{
		client:     client,
		expiration: expiration,
	}
}

// balancesKey returns the Redis key for an account's currency balances
func (c *BalanceCacheImp) balancesKey(userID uuid.UUID, accountID uuid.UUID) string {
	return fmt.Sprintf("user:%s:%s:balances", userID.String(), accountID.String())
}

// checkpointKey returns the Redis key for an account's checkpoint
func (c *BalanceCacheImp) checkpointKey(userID uuid.UUID) string {
	return fmt.Sprintf("user:%s:checkpoint", userID.String())
}

// Get retrieves a specific currency balance for an account
func (c *BalanceCacheImp) Get(ctx context.Context, key types.BalanceKey) (*Balance, error) {
	balancesKey := c.balancesKey(key.UserID, key.AccountID)

	// Get the currency amount from the hash
	amountStr, err := c.client.HGet(ctx, balancesKey, key.Currency).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil
		}
		return nil, errors.New("failed to get balance from cache").Wrap(err)
	}

	amount, err := decimal.NewFromString(amountStr)
	if err != nil {
		return nil, errors.New("failed to parse balance amount").Wrap(err)
	}

	return &Balance{
		BalanceKey: key,
		Amount:     amount,
	}, nil
}

// GetAccountCheckpoint retrieves user checkpoint data
func (c *BalanceCacheImp) GetCheckpoint(ctx context.Context, userID uuid.UUID) (*UserCheckpoint, error) {
	checkpointKey := c.checkpointKey(userID)

	data, err := c.client.Get(ctx, checkpointKey).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil
		}
		return nil, errors.New("failed to get user checkpoint from cache").Wrap(err)
	}

	var checkpoint Checkpoint
	if err := json.Unmarshal(data, &checkpoint); err != nil {
		return nil, errors.New("failed to unmarshal account checkpoint").Wrap(err)
	}

	return &UserCheckpoint{
		UserID:       userID,
		LastSequence: checkpoint.LastSequence,
		IsFinal:      checkpoint.IsFinal,
	}, nil
}

// GetMany retrieves multiple currency balances
func (c *BalanceCacheImp) GetMany(ctx context.Context, keys []types.BalanceKey) (*types.BalanceMap, error) {
	if len(keys) == 0 {
		return types.NewBalanceMap(nil), nil
	}

	// Group keys by user+account ID to minimize Redis calls
	accountGroups := make(map[string]map[uuid.UUID][]string)
	for _, key := range keys {
		userKey := key.UserID.String()
		if accountGroups[userKey] == nil {
			accountGroups[userKey] = make(map[uuid.UUID][]string)
		}
		accountGroups[userKey][key.AccountID] = append(accountGroups[userKey][key.AccountID], key.Currency)
	}

	// Create result map
	results := types.NewBalanceMap(nil)

	// Process each account
	for userIDStr, userAccounts := range accountGroups {
		userID, _ := uuid.Parse(userIDStr)

		for accountID, currencies := range userAccounts {
			balancesKey := c.balancesKey(userID, accountID)

			amountStrs, err := c.client.HMGet(ctx, balancesKey, currencies...).Result()
			if err != nil {
				return nil, errors.New("failed to get balances from cache").Wrap(err)
			}

			for i, amountStr := range amountStrs {
				if amountStr == nil {
					continue
				}

				amount, err := decimal.NewFromString(amountStr.(string))
				if err != nil {
					return nil, errors.New("failed to parse balance amount").Wrap(err)
				}

				balanceKey := types.BalanceKey{
					UserID:    userID,
					AccountID: accountID,
					Currency:  currencies[i],
				}
				results.Set(balanceKey, amount)
			}
		}
	}

	return results, nil
}

// Commit updates balances, checkpoint and removes dirty flag in a single transaction for multiple accounts
func (c *BalanceCacheImp) Commit(ctx context.Context, commits []CommitState) error {
	if len(commits) == 0 {
		return nil
	}

	_, err := c.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		for _, commit := range commits {

			checkpointKey := c.checkpointKey(commit.UserID)

			// Update balances for each account
			for accountID, currencies := range commit.Balances {
				balancesKey := c.balancesKey(commit.UserID, accountID)

				if len(currencies) > 0 {
					fields := make(map[string]interface{})
					for currency, amount := range currencies {
						fields[currency] = amount.String()
					}
					pipe.HSet(ctx, balancesKey, fields)
					pipe.Expire(ctx, balancesKey, c.expiration)
				}

			}

			checkpoint := &Checkpoint{
				LastSequence: commit.LastSequence,
				IsFinal:      true,
			}

			checkpointData, err := json.Marshal(checkpoint)
			if err != nil {
				return errors.New("failed to marshal account checkpoint").Wrap(err)
			}

			pipe.Set(ctx, checkpointKey, checkpointData, c.expiration)
		}

		return nil
	})
	if err != nil {
		return errors.New("failed to commit account balances").Wrap(err)
	}

	return nil
}

// GetManyCheckpoints retrieves checkpoint data for multiple accounts
func (c *BalanceCacheImp) GetManyCheckpoints(ctx context.Context, userIDs []uuid.UUID) (map[uuid.UUID]*UserCheckpoint, error) {
	if len(userIDs) == 0 {
		return make(map[uuid.UUID]*UserCheckpoint), nil
	}

	// Create a pipeline to batch Redis commands
	pipe := c.client.Pipeline()
	cmds := make(map[uuid.UUID]*redis.StringCmd)

	// Queue up all the GET commands
	for _, userID := range userIDs {
		checkpointKey := c.checkpointKey(userID)
		cmds[userID] = pipe.Get(ctx, checkpointKey)
	}

	// Execute the pipeline
	_, err := pipe.Exec(ctx)
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, errors.New("failed to execute pipeline for checkpoint retrieval").Wrap(err)
	}

	// Process the results
	results := make(map[uuid.UUID]*UserCheckpoint)
	for userID, cmd := range cmds {
		data, err := cmd.Bytes()
		if err != nil {
			if errors.Is(err, redis.Nil) {
				// No checkpoint exists for this account
				continue
			}
			return nil, errors.New("failed to get checkpoint for account").Wrap(err)
		}

		var checkpoint Checkpoint
		if err := json.Unmarshal(data, &checkpoint); err != nil {
			return nil, errors.New("failed to unmarshal account checkpoint").Wrap(err)
		}

		results[userID] = &UserCheckpoint{
			UserID:       userID,
			LastSequence: checkpoint.LastSequence,
			IsFinal:      checkpoint.IsFinal,
		}
	}

	return results, nil
}

// MarkDirty marks accounts as dirty
func (c *BalanceCacheImp) MarkDirty(ctx context.Context, userIDs []uuid.UUID) error {
	if len(userIDs) == 0 {
		return nil
	}

	pipe := c.client.Pipeline()

	for _, userID := range userIDs {
		checkpointKey := c.checkpointKey(userID)

		cmd := pipe.Get(ctx, checkpointKey)

		pipe.Exec(ctx)

		data, err := cmd.Bytes()
		var checkpoint Checkpoint

		if err != nil {
			if errors.Is(err, redis.Nil) {
				checkpoint = Checkpoint{
					LastSequence: 0,
					IsFinal:      false,
				}
			} else {
				return errors.New("failed to get account checkpoint").Wrap(err)
			}
		} else {
			if err := json.Unmarshal(data, &checkpoint); err != nil {
				return errors.New("failed to unmarshal account checkpoint").Wrap(err)
			}

			checkpoint.IsFinal = false
		}

		updatedData, err := json.Marshal(checkpoint)
		if err != nil {
			return errors.New("failed to marshal account checkpoint").Wrap(err)
		}

		pipe.Set(ctx, checkpointKey, updatedData, c.expiration)
	}

	_, err := pipe.Exec(ctx)
	if err != nil {
		return errors.New("failed to mark accounts as dirty").Wrap(err)
	}

	return nil
}
