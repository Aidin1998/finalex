package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	tradingconfig "github.com/Aidin1998/pincex_unified/internal/core/trading/config"

	"github.com/spf13/viper"
)

// Config represents the application configuration
type Config struct {
	Server struct {
		Port int
	}
	Database struct {
		DSN             string
		CockroachDSN    string
		MaxOpenConns    int
		MaxIdleConns    int
		ConnMaxLifetime int // seconds
	}
	Redis struct {
		Address  string
		Password string
		DB       int
	}
	JWT struct {
		Secret          string
		ExpirationHours int
		RefreshSecret   string
		RefreshExpHours int
	}
	KYC struct {
		ProviderURL      string
		ProviderAPIKey   string
		DocumentBasePath string
	}
	Kafka struct {
		Brokers             []string
		EnableMessageQueue  bool
		ConsumerGroupPrefix string
	}
	DBSharding   tradingconfig.DBConfig
	RedisCluster tradingconfig.RedisConfig
}

// LoadConfig loads the application configuration
func LoadConfig() (*Config, error) {
	// Set default configuration
	config := &Config{}
	config.Server.Port = 8080
	config.Database.DSN = "postgres://postgres:postgres@localhost:5432/pincex?sslmode=disable"
	config.Redis.Address = "localhost:6379"
	config.Redis.Password = ""
	config.Redis.DB = 0
	config.JWT.Secret = "your-secret-key"
	config.JWT.ExpirationHours = 24
	config.JWT.RefreshSecret = "your-refresh-secret-key"
	config.JWT.RefreshExpHours = 168
	config.KYC.ProviderURL = "https://kyc-provider.example.com"
	config.KYC.ProviderAPIKey = "your-kyc-provider-api-key"
	config.KYC.DocumentBasePath = "/var/lib/pincex/kyc-documents"

	// Kafka defaults
	config.Kafka.Brokers = []string{"localhost:9092"}
	config.Kafka.EnableMessageQueue = true
	config.Kafka.ConsumerGroupPrefix = "pincex"

	// Sharded DB defaults: single shard using primary DSN
	config.DBSharding = tradingconfig.DBConfig{
		Shards: []tradingconfig.ShardConfig{{
			Name:      "shard-0",
			MasterDSN: config.Database.DSN,
		}},
		Pool: tradingconfig.PoolConfig{
			MaxOpenConns:    config.Database.MaxOpenConns,
			MaxIdleConns:    config.Database.MaxIdleConns,
			ConnMaxLifetime: time.Duration(config.Database.ConnMaxLifetime) * time.Second,
		},
	}
	// Redis cluster defaults already set in config.RedisCluster

	// Load configuration from environment variables
	if port, err := strconv.Atoi(os.Getenv("SERVER_PORT")); err == nil {
		config.Server.Port = port
	}

	if dsn := os.Getenv("DATABASE_DSN"); dsn != "" {
		config.Database.DSN = dsn
	}
	if crdsn := os.Getenv("COCKROACH_DSN"); crdsn != "" {
		config.Database.CockroachDSN = crdsn
	}

	if redisAddr := os.Getenv("REDIS_ADDRESS"); redisAddr != "" {
		config.Redis.Address = redisAddr
	}

	if redisPassword := os.Getenv("REDIS_PASSWORD"); redisPassword != "" {
		config.Redis.Password = redisPassword
	}

	if redisDB, err := strconv.Atoi(os.Getenv("REDIS_DB")); err == nil {
		config.Redis.DB = redisDB
	}

	if jwtSecret := os.Getenv("JWT_SECRET"); jwtSecret != "" {
		config.JWT.Secret = jwtSecret
	}

	if jwtExpHours, err := strconv.Atoi(os.Getenv("JWT_EXPIRATION_HOURS")); err == nil {
		config.JWT.ExpirationHours = jwtExpHours
	}

	if jwtRefreshSecret := os.Getenv("JWT_REFRESH_SECRET"); jwtRefreshSecret != "" {
		config.JWT.RefreshSecret = jwtRefreshSecret
	}

	if jwtRefreshExpHours, err := strconv.Atoi(os.Getenv("JWT_REFRESH_EXPIRATION_HOURS")); err == nil {
		config.JWT.RefreshExpHours = jwtRefreshExpHours
	}

	if kycProviderURL := os.Getenv("KYC_PROVIDER_URL"); kycProviderURL != "" {
		config.KYC.ProviderURL = kycProviderURL
	}

	if kycProviderAPIKey := os.Getenv("KYC_PROVIDER_API_KEY"); kycProviderAPIKey != "" {
		config.KYC.ProviderAPIKey = kycProviderAPIKey
	}

	if kycDocumentBasePath := os.Getenv("KYC_DOCUMENT_BASE_PATH"); kycDocumentBasePath != "" {
		config.KYC.DocumentBasePath = kycDocumentBasePath
	}

	// Load Kafka configuration from environment variables
	if kafkaBrokers := os.Getenv("KAFKA_BROKERS"); kafkaBrokers != "" {
		config.Kafka.Brokers = strings.Split(kafkaBrokers, ",")
	}

	if enableMQ := os.Getenv("ENABLE_MESSAGE_QUEUE"); enableMQ != "" {
		config.Kafka.EnableMessageQueue = enableMQ == "true"
	}

	if groupPrefix := os.Getenv("KAFKA_GROUP_PREFIX"); groupPrefix != "" {
		config.Kafka.ConsumerGroupPrefix = groupPrefix
	}

	// Build sharded DB config from environment variables
	masters := strings.Split(os.Getenv("DB_SHARD_MASTERS"), ",")
	if len(masters) > 0 && masters[0] != "" {
		replicas := strings.Split(os.Getenv("DB_SHARD_REPLICAS"), ",")
		shards := make([]tradingconfig.ShardConfig, len(masters))
		for i, master := range masters {
			shard := tradingconfig.ShardConfig{Name: fmt.Sprintf("shard-%d", i), MasterDSN: master}
			if i < len(replicas) && replicas[i] != "" {
				shard.ReplicaDSN = replicas[i]
			}
			shards[i] = shard
		}
		config.DBSharding.Shards = shards
	}

	// Build Redis cluster config
	addrs := strings.Split(os.Getenv("REDIS_CLUSTER_ADDRESSES"), ",")
	if len(addrs) > 0 && addrs[0] != "" {
		config.RedisCluster.Addrs = addrs
	}
	if pwd := os.Getenv("REDIS_CLUSTER_PASSWORD"); pwd != "" {
		config.RedisCluster.Password = pwd
	}
	if ps, err := strconv.Atoi(os.Getenv("REDIS_CLUSTER_POOL_SIZE")); err == nil {
		config.RedisCluster.PoolSize = ps
	}

	// Load configuration from file
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("./config")
	viper.AddConfigPath("/etc/pincex")

	if err := viper.ReadInConfig(); err != nil {
		// Config file not found, use default and environment values
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	} else {
		// Config file found, override default and environment values
		if viper.IsSet("server.port") {
			config.Server.Port = viper.GetInt("server.port")
		}

		if viper.IsSet("database.dsn") {
			config.Database.DSN = viper.GetString("database.dsn")
		}

		if viper.IsSet("redis.address") {
			config.Redis.Address = viper.GetString("redis.address")
		}

		if viper.IsSet("redis.password") {
			config.Redis.Password = viper.GetString("redis.password")
		}

		if viper.IsSet("redis.db") {
			config.Redis.DB = viper.GetInt("redis.db")
		}

		if viper.IsSet("jwt.secret") {
			config.JWT.Secret = viper.GetString("jwt.secret")
		}

		if viper.IsSet("jwt.expiration_hours") {
			config.JWT.ExpirationHours = viper.GetInt("jwt.expiration_hours")
		}

		if viper.IsSet("jwt.refresh_secret") {
			config.JWT.RefreshSecret = viper.GetString("jwt.refresh_secret")
		}

		if viper.IsSet("jwt.refresh_expiration_hours") {
			config.JWT.RefreshExpHours = viper.GetInt("jwt.refresh_expiration_hours")
		}

		if viper.IsSet("kyc.provider_url") {
			config.KYC.ProviderURL = viper.GetString("kyc.provider_url")
		}

		if viper.IsSet("kyc.provider_api_key") {
			config.KYC.ProviderAPIKey = viper.GetString("kyc.provider_api_key")
		}

		if viper.IsSet("kyc.document_base_path") {
			config.KYC.DocumentBasePath = viper.GetString("kyc.document_base_path")
		}

		if viper.IsSet("kafka.brokers") {
			config.Kafka.Brokers = viper.GetStringSlice("kafka.brokers")
		}

		if viper.IsSet("kafka.enable_message_queue") {
			config.Kafka.EnableMessageQueue = viper.GetBool("kafka.enable_message_queue")
		}

		if viper.IsSet("kafka.consumer_group_prefix") {
			config.Kafka.ConsumerGroupPrefix = viper.GetString("kafka.consumer_group_prefix")
		}
	}

	return config, nil
}
