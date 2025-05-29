package config

import "time"

// ShardConfig describes one database shard with its master and replica DSNs
type ShardConfig struct {
	Name       string `json:"name"`
	MasterDSN  string `json:"master_dsn"`
	ReplicaDSN string `json:"replica_dsn"`
}

// PoolConfig holds database connection pooling settings
type PoolConfig struct {
	MaxOpenConns    int           `json:"max_open_conns"`
	MaxIdleConns    int           `json:"max_idle_conns"`
	ConnMaxLifetime time.Duration `json:"conn_max_lifetime"`
}

// DBConfig holds the set of shards and pool configuration
type DBConfig struct {
	Shards []ShardConfig `json:"shards"`
	Pool   PoolConfig    `json:"pool"`
}

// RedisConfig holds Redis cluster connection settings
type RedisConfig struct {
	Addrs    []string `json:"addrs"`
	Password string   `json:"password"`
	PoolSize int      `json:"pool_size"`
}
