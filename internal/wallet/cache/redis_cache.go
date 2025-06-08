// Package cache provides caching implementation for the wallet service
package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/Aidin1998/finalex/internal/wallet/interfaces"
)

var (
	// ErrCacheMiss indicates a cache miss
	ErrCacheMiss = errors.New("cache miss")
)

// RedisWalletCache implements WalletCache using Redis
type RedisWalletCache struct {
	client redis.Cmdable
	log    *zap.Logger
	prefix string
	ttl    time.Duration
}

// NewRedisWalletCache creates a new Redis-based wallet cache
func NewRedisWalletCache(client redis.Cmdable, log *zap.Logger, prefix string, ttl time.Duration) *RedisWalletCache {
	return &RedisWalletCache{
		client: client,
		log:    log,
		prefix: prefix,
		ttl:    ttl,
	}
}

// GetBalance retrieves cached balance
func (c *RedisWalletCache) GetBalance(ctx context.Context, userID uuid.UUID, asset string) (*interfaces.AssetBalance, error) {
	key := c.balanceKey(userID, asset)

	data, err := c.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, ErrCacheMiss
		}
		c.log.Error("failed to get balance from cache", zap.Error(err), zap.String("key", key))
		return nil, err
	}

	var balance interfaces.AssetBalance
	if err := json.Unmarshal([]byte(data), &balance); err != nil {
		c.log.Error("failed to unmarshal cached balance", zap.Error(err), zap.String("key", key))
		return nil, err
	}

	return &balance, nil
}

// SetBalance stores balance in cache
func (c *RedisWalletCache) SetBalance(ctx context.Context, userID uuid.UUID, asset string, balance *interfaces.AssetBalance, ttl time.Duration) error {
	key := c.balanceKey(userID, asset)

	data, err := json.Marshal(balance)
	if err != nil {
		c.log.Error("failed to marshal balance for cache", zap.Error(err))
		return err
	}

	if err := c.client.Set(ctx, key, data, ttl).Err(); err != nil {
		c.log.Error("failed to set balance in cache", zap.Error(err), zap.String("key", key))
		return err
	}

	return nil
}

// InvalidateBalance removes balance from cache
func (c *RedisWalletCache) InvalidateBalance(ctx context.Context, userID uuid.UUID, asset string) error {
	key := c.balanceKey(userID, asset)

	if err := c.client.Del(ctx, key).Err(); err != nil {
		c.log.Error("failed to invalidate balance cache", zap.Error(err), zap.String("key", key))
		return err
	}

	return nil
}

// GetTransaction retrieves cached transaction
func (c *RedisWalletCache) GetTransaction(ctx context.Context, txID uuid.UUID) (*interfaces.WalletTransaction, error) {
	key := c.transactionKey(txID)

	data, err := c.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, ErrCacheMiss
		}
		c.log.Error("failed to get transaction from cache", zap.Error(err), zap.String("key", key))
		return nil, err
	}

	var transaction interfaces.WalletTransaction
	if err := json.Unmarshal([]byte(data), &transaction); err != nil {
		c.log.Error("failed to unmarshal cached transaction", zap.Error(err), zap.String("key", key))
		return nil, err
	}

	return &transaction, nil
}

// SetTransaction stores transaction in cache
func (c *RedisWalletCache) SetTransaction(ctx context.Context, transaction *interfaces.WalletTransaction, ttl time.Duration) error {
	key := c.transactionKey(transaction.ID)

	data, err := json.Marshal(transaction)
	if err != nil {
		c.log.Error("failed to marshal transaction for cache", zap.Error(err))
		return err
	}

	if err := c.client.Set(ctx, key, data, ttl).Err(); err != nil {
		c.log.Error("failed to set transaction in cache", zap.Error(err), zap.String("key", key))
		return err
	}

	return nil
}

// InvalidateTransaction removes transaction from cache
func (c *RedisWalletCache) InvalidateTransaction(ctx context.Context, txID uuid.UUID) error {
	key := c.transactionKey(txID)

	if err := c.client.Del(ctx, key).Err(); err != nil {
		c.log.Error("failed to invalidate transaction cache", zap.Error(err), zap.String("key", key))
		return err
	}

	return nil
}

// GetDepositAddress retrieves cached deposit address
func (c *RedisWalletCache) GetDepositAddress(ctx context.Context, userID uuid.UUID, asset, network string) (*interfaces.DepositAddress, error) {
	key := c.depositAddressKey(userID, asset, network)

	data, err := c.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, ErrCacheMiss
		}
		c.log.Error("failed to get deposit address from cache", zap.Error(err), zap.String("key", key))
		return nil, err
	}

	var address interfaces.DepositAddress
	if err := json.Unmarshal([]byte(data), &address); err != nil {
		c.log.Error("failed to unmarshal cached deposit address", zap.Error(err), zap.String("key", key))
		return nil, err
	}

	return &address, nil
}

// SetDepositAddress stores deposit address in cache
func (c *RedisWalletCache) SetDepositAddress(ctx context.Context, address *interfaces.DepositAddress, ttl time.Duration) error {
	key := c.depositAddressKey(address.UserID, address.Asset, address.Network)

	data, err := json.Marshal(address)
	if err != nil {
		c.log.Error("failed to marshal deposit address for cache", zap.Error(err))
		return err
	}

	if err := c.client.Set(ctx, key, data, ttl).Err(); err != nil {
		c.log.Error("failed to set deposit address in cache", zap.Error(err), zap.String("key", key))
		return err
	}

	return nil
}

// InvalidateDepositAddress removes deposit address from cache
func (c *RedisWalletCache) InvalidateDepositAddress(ctx context.Context, userID uuid.UUID, asset, network string) error {
	key := c.depositAddressKey(userID, asset, network)

	if err := c.client.Del(ctx, key).Err(); err != nil {
		c.log.Error("failed to invalidate deposit address cache", zap.Error(err), zap.String("key", key))
		return err
	}

	return nil
}

// GetLocks retrieves cached fund locks
func (c *RedisWalletCache) GetLocks(ctx context.Context, userID uuid.UUID) ([]*interfaces.FundLock, error) {
	key := c.locksKey(userID)

	data, err := c.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, ErrCacheMiss
		}
		c.log.Error("failed to get locks from cache", zap.Error(err), zap.String("key", key))
		return nil, err
	}

	var locks []*interfaces.FundLock
	if err := json.Unmarshal([]byte(data), &locks); err != nil {
		c.log.Error("failed to unmarshal cached locks", zap.Error(err), zap.String("key", key))
		return nil, err
	}

	return locks, nil
}

// SetLocks stores fund locks in cache
func (c *RedisWalletCache) SetLocks(ctx context.Context, userID uuid.UUID, locks []*interfaces.FundLock, ttl time.Duration) error {
	key := c.locksKey(userID)

	data, err := json.Marshal(locks)
	if err != nil {
		c.log.Error("failed to marshal locks for cache", zap.Error(err))
		return err
	}

	if err := c.client.Set(ctx, key, data, ttl).Err(); err != nil {
		c.log.Error("failed to set locks in cache", zap.Error(err), zap.String("key", key))
		return err
	}

	return nil
}

// InvalidateLocks removes fund locks from cache
func (c *RedisWalletCache) InvalidateLocks(ctx context.Context, userID uuid.UUID) error {
	key := c.locksKey(userID)

	if err := c.client.Del(ctx, key).Err(); err != nil {
		c.log.Error("failed to invalidate locks cache", zap.Error(err), zap.String("key", key))
		return err
	}

	return nil
}

// GetAddresses retrieves cached addresses
func (c *RedisWalletCache) GetAddresses(ctx context.Context, userID uuid.UUID, asset string) ([]*interfaces.DepositAddress, error) {
	key := c.addressesKey(userID, asset)

	data, err := c.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, ErrCacheMiss
		}
		c.log.Error("failed to get addresses from cache", zap.Error(err), zap.String("key", key))
		return nil, err
	}

	var addresses []*interfaces.DepositAddress
	if err := json.Unmarshal([]byte(data), &addresses); err != nil {
		c.log.Error("failed to unmarshal cached addresses", zap.Error(err), zap.String("key", key))
		return nil, err
	}

	return addresses, nil
}

// SetAddresses stores addresses in cache
func (c *RedisWalletCache) SetAddresses(ctx context.Context, userID uuid.UUID, asset string, addresses []*interfaces.DepositAddress, ttl time.Duration) error {
	key := c.addressesKey(userID, asset)

	data, err := json.Marshal(addresses)
	if err != nil {
		c.log.Error("failed to marshal addresses for cache", zap.Error(err))
		return err
	}

	if err := c.client.Set(ctx, key, data, ttl).Err(); err != nil {
		c.log.Error("failed to set addresses in cache", zap.Error(err), zap.String("key", key))
		return err
	}

	return nil
}

// InvalidateAddresses removes addresses from cache
func (c *RedisWalletCache) InvalidateAddresses(ctx context.Context, userID uuid.UUID, asset string) error {
	key := c.addressesKey(userID, asset)

	if err := c.client.Del(ctx, key).Err(); err != nil {
		c.log.Error("failed to invalidate addresses cache", zap.Error(err), zap.String("key", key))
		return err
	}

	return nil
}

// SetTransactionRate implements rate limiting for transactions
func (c *RedisWalletCache) SetTransactionRate(ctx context.Context, userID uuid.UUID, window time.Duration) error {
	key := c.rateKey(userID)

	if err := c.client.Set(ctx, key, 1, window).Err(); err != nil {
		c.log.Error("failed to set transaction rate in cache", zap.Error(err), zap.String("key", key))
		return err
	}

	return nil
}

// CheckTransactionRate checks if user has exceeded transaction rate limit
func (c *RedisWalletCache) CheckTransactionRate(ctx context.Context, userID uuid.UUID) (bool, error) {
	key := c.rateKey(userID)

	count, err := c.client.Get(ctx, key).Int()
	if err != nil {
		if err == redis.Nil {
			return false, nil
		}
		c.log.Error("failed to get transaction rate from cache", zap.Error(err), zap.String("key", key))
		return false, err
	}

	return count > 0, nil
}

// IncrementTransactionRate increments the transaction count for rate limiting
func (c *RedisWalletCache) IncrementTransactionRate(ctx context.Context, userID uuid.UUID) error {
	key := c.rateKey(userID)

	if err := c.client.Incr(ctx, key).Err(); err != nil {
		c.log.Error("failed to increment transaction rate", zap.Error(err), zap.String("key", key))
		return err
	}

	return nil
}

// AcquireLock acquires a distributed lock
func (c *RedisWalletCache) AcquireLock(ctx context.Context, lockKey string, ttl time.Duration) (bool, error) {
	key := c.lockKey(lockKey)

	acquired, err := c.client.SetNX(ctx, key, "locked", ttl).Result()
	if err != nil {
		c.log.Error("failed to acquire fund lock", zap.Error(err), zap.String("key", key))
		return false, err
	}

	return acquired, nil
}

// ReleaseLock releases a distributed lock
func (c *RedisWalletCache) ReleaseLock(ctx context.Context, lockKey string) error {
	key := c.lockKey(lockKey)

	if err := c.client.Del(ctx, key).Err(); err != nil {
		c.log.Error("failed to release fund lock", zap.Error(err), zap.String("key", key))
		return err
	}

	return nil
}

// CacheAddressValidation caches address validation result
func (c *RedisWalletCache) CacheAddressValidation(ctx context.Context, address string, result *interfaces.AddressValidationResult, ttl time.Duration) error {
	key := c.addressValidationKey(address)

	data, err := json.Marshal(result)
	if err != nil {
		c.log.Error("failed to marshal address validation result", zap.Error(err))
		return err
	}

	if err := c.client.Set(ctx, key, data, ttl).Err(); err != nil {
		c.log.Error("failed to cache address validation", zap.Error(err), zap.String("key", key))
		return err
	}

	return nil
}

// GetCachedAddressValidation retrieves cached address validation result
func (c *RedisWalletCache) GetCachedAddressValidation(ctx context.Context, address string) (*interfaces.AddressValidationResult, error) {
	key := c.addressValidationKey(address)

	data, err := c.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, ErrCacheMiss
		}
		c.log.Error("failed to get address validation from cache", zap.Error(err), zap.String("key", key))
		return nil, err
	}

	var result interfaces.AddressValidationResult
	if err := json.Unmarshal([]byte(data), &result); err != nil {
		c.log.Error("failed to unmarshal cached address validation", zap.Error(err), zap.String("key", key))
		return nil, err
	}

	return &result, nil
}

// FlushUserCache removes all cached data for a user
func (c *RedisWalletCache) FlushUserCache(ctx context.Context, userID uuid.UUID) error {
	pattern := fmt.Sprintf("%s:user:%s:*", c.prefix, userID.String())

	keys, err := c.client.Keys(ctx, pattern).Result()
	if err != nil {
		c.log.Error("failed to get user cache keys", zap.Error(err), zap.String("pattern", pattern))
		return err
	}

	if len(keys) > 0 {
		if err := c.client.Del(ctx, keys...).Err(); err != nil {
			c.log.Error("failed to flush user cache", zap.Error(err), zap.String("user_id", userID.String()))
			return err
		}
	}

	return nil
}

// Delete removes a key from the cache (required by WalletCache interface)
func (c *RedisWalletCache) Delete(ctx context.Context, key string) error {
	if err := c.client.Del(ctx, key).Err(); err != nil {
		c.log.Error("failed to delete key from cache", zap.Error(err), zap.String("key", key))
		return err
	}
	return nil
}

// Clear is a stub method to satisfy the WalletCache interface
func (c *RedisWalletCache) Clear(ctx context.Context) error {
	// No-op stub
	return nil
}

// Key generation helpers
func (c *RedisWalletCache) balanceKey(userID uuid.UUID, asset string) string {
	return fmt.Sprintf("%s:balance:%s:%s", c.prefix, userID.String(), asset)
}

func (c *RedisWalletCache) transactionKey(txID uuid.UUID) string {
	return fmt.Sprintf("%s:transaction:%s", c.prefix, txID.String())
}

func (c *RedisWalletCache) depositAddressKey(userID uuid.UUID, asset, network string) string {
	return fmt.Sprintf("%s:address:%s:%s:%s", c.prefix, userID.String(), asset, network)
}

func (c *RedisWalletCache) addressesKey(userID uuid.UUID, asset string) string {
	return fmt.Sprintf("%s:addresses:%s:%s", c.prefix, userID.String(), asset)
}

func (c *RedisWalletCache) locksKey(userID uuid.UUID) string {
	return fmt.Sprintf("%s:locks:%s", c.prefix, userID.String())
}

func (c *RedisWalletCache) rateKey(userID uuid.UUID) string {
	return fmt.Sprintf("%s:rate:%s", c.prefix, userID.String())
}

func (c *RedisWalletCache) lockKey(lockKey string) string {
	return fmt.Sprintf("%s:lock:%s", c.prefix, lockKey)
}

func (c *RedisWalletCache) addressValidationKey(address string) string {
	return fmt.Sprintf("%s:validation:%s", c.prefix, address)
}

// Get implements a generic get for the WalletCache interface
func (c *RedisWalletCache) Get(ctx context.Context, key string) (interface{}, error) {
	data, err := c.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, ErrCacheMiss
		}
		c.log.Error("failed to get key from cache", zap.Error(err), zap.String("key", key))
		return nil, err
	}
	return data, nil
}

// GetTransactionStatus is a stub to satisfy the WalletCache interface
func (c *RedisWalletCache) GetTransactionStatus(ctx context.Context, txID uuid.UUID) (*interfaces.TransactionStatus, error) {
	// Not implemented: return nil for now
	return nil, nil
}

// GetUserAddresses is a stub to satisfy the WalletCache interface
func (c *RedisWalletCache) GetUserAddresses(ctx context.Context, userID uuid.UUID, asset string) ([]*interfaces.DepositAddress, error) {
	// Not implemented: return nil for now
	return nil, nil
}

// InvalidateTransactionStatus is a stub to satisfy the WalletCache interface
func (c *RedisWalletCache) InvalidateTransactionStatus(ctx context.Context, txID uuid.UUID) error {
	// Not implemented: return nil for now
	return nil
}

// InvalidateUserAddresses is a stub to satisfy the WalletCache interface
func (c *RedisWalletCache) InvalidateUserAddresses(ctx context.Context, userID uuid.UUID, asset string) error {
	// Not implemented: return nil for now
	return nil
}

// InvalidateUserBalances is a stub to satisfy the WalletCache interface
func (c *RedisWalletCache) InvalidateUserBalances(ctx context.Context, userID uuid.UUID) error {
	// Not implemented: return nil for now
	return nil
}

// Set is a stub to satisfy the WalletCache interface
func (c *RedisWalletCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	// Not implemented: return nil for now
	return nil
}

// SetTransactionStatus is a stub to satisfy the WalletCache interface
func (c *RedisWalletCache) SetTransactionStatus(ctx context.Context, txID uuid.UUID, status *interfaces.TransactionStatus, ttl time.Duration) error {
	// Not implemented: return nil for now
	return nil
}

// SetUserAddresses is a stub to satisfy the WalletCache interface
func (c *RedisWalletCache) SetUserAddresses(ctx context.Context, userID uuid.UUID, asset string, addresses []*interfaces.DepositAddress, ttl time.Duration) error {
	// Not implemented: return nil for now
	return nil
}
