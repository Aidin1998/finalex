package dbutil

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/Aidin1998/finalex/internal/trading/config"
)

// DB and Redis singletons
var (
	shardDBs    []*gorm.DB
	redisClient *redis.ClusterClient
	once        sync.Once
)

// InitializeConnections sets up all DB shard pools and Redis cluster clients
func InitializeConnections(cfg config.DBConfig, rcfg config.RedisConfig) error {
	var initErr error
	once.Do(func() {
		// Initialize each shard's master and replica
		shardDBs = make([]*gorm.DB, len(cfg.Shards))
		for i, s := range cfg.Shards {
			// connect master
			masterDB, err := gorm.Open(postgres.Open(s.MasterDSN), &gorm.Config{Logger: logger.Default.LogMode(logger.Info)})
			if err != nil {
				initErr = fmt.Errorf("failed to open master for shard %s: %w", s.Name, err)
				return
			}
			sqlDB, _ := masterDB.DB()
			sqlDB.SetMaxOpenConns(cfg.Pool.MaxOpenConns)
			sqlDB.SetMaxIdleConns(cfg.Pool.MaxIdleConns)
			sqlDB.SetConnMaxLifetime(cfg.Pool.ConnMaxLifetime)
			// assign master by default
			shardDBs[i] = masterDB
			// TODO: add replica support and health-check routing
		}
		// Initialize Redis cluster
		redisClient = redis.NewClusterClient(&redis.ClusterOptions{
			Addrs:    rcfg.Addrs,
			Password: rcfg.Password,
			PoolSize: rcfg.PoolSize,
		})
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := redisClient.Ping(ctx).Err(); err != nil {
			initErr = fmt.Errorf("failed to ping redis cluster: %w", err)
		}
	})
	return initErr
}

// GetDBForKey returns a GORM DB instance based on a key (e.g., user ID) for sharding routing
func GetDBForKey(key int64) *gorm.DB {
	if len(shardDBs) == 0 {
		panic("shardDBs not initialized")
	}
	idx := int(key) % len(shardDBs)
	return shardDBs[idx]
}

// RedisClient returns the shared Redis cluster client
func RedisClient() *redis.ClusterClient {
	if redisClient == nil {
		panic("redisClient not initialized")
	}
	return redisClient
}

// All redis.ClusterClient usage remains compatible, but ensure context is always passed and check for any API changes in v9.
