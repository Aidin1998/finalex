package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	tradingconfig "github.com/Aidin1998/finalex/internal/trading/config"

	"github.com/spf13/viper"
)

// ServerConfig represents server configuration
type ServerConfig struct {
	HTTP HTTPServerConfig `yaml:"http" json:"http"`
	GRPC GRPCServerConfig `yaml:"grpc" json:"grpc"`
	WS   WSServerConfig   `yaml:"websocket" json:"websocket"`
	TLS  TLSConfig        `yaml:"tls" json:"tls"`
}

// HTTPServerConfig represents HTTP server configuration
type HTTPServerConfig struct {
	Host               string        `yaml:"host" json:"host"`
	Port               int           `yaml:"port" json:"port"`
	ReadTimeout        time.Duration `yaml:"read_timeout" json:"read_timeout"`
	WriteTimeout       time.Duration `yaml:"write_timeout" json:"write_timeout"`
	IdleTimeout        time.Duration `yaml:"idle_timeout" json:"idle_timeout"`
	ReadHeaderTimeout  time.Duration `yaml:"read_header_timeout" json:"read_header_timeout"`
	MaxHeaderBytes     int           `yaml:"max_header_bytes" json:"max_header_bytes"`
	ShutdownTimeout    time.Duration `yaml:"shutdown_timeout" json:"shutdown_timeout"`
	EnableCompression  bool          `yaml:"enable_compression" json:"enable_compression"`
	EnableHTTP2        bool          `yaml:"enable_http2" json:"enable_http2"`
	MaxConcurrentConns int           `yaml:"max_concurrent_conns" json:"max_concurrent_conns"`
	HealthCheckPath    string        `yaml:"health_check_path" json:"health_check_path"`
	ReadinessCheckPath string        `yaml:"readiness_check_path" json:"readiness_check_path"`
	MetricsPath        string        `yaml:"metrics_path" json:"metrics_path"`
}

// GRPCServerConfig represents gRPC server configuration
type GRPCServerConfig struct {
	Host              string          `yaml:"host" json:"host"`
	Port              int             `yaml:"port" json:"port"`
	MaxRecvMsgSize    int             `yaml:"max_recv_msg_size" json:"max_recv_msg_size"`
	MaxSendMsgSize    int             `yaml:"max_send_msg_size" json:"max_send_msg_size"`
	ConnectionTimeout time.Duration   `yaml:"connection_timeout" json:"connection_timeout"`
	KeepAlive         KeepAliveConfig `yaml:"keep_alive" json:"keep_alive"`
	EnableReflection  bool            `yaml:"enable_reflection" json:"enable_reflection"`
	ShutdownTimeout   time.Duration   `yaml:"shutdown_timeout" json:"shutdown_timeout"`
}

// KeepAliveConfig represents gRPC keepalive configuration
type KeepAliveConfig struct {
	Time                  time.Duration `yaml:"time" json:"time"`
	Timeout               time.Duration `yaml:"timeout" json:"timeout"`
	EnforcementPolicy     bool          `yaml:"enforcement_policy" json:"enforcement_policy"`
	MinTime               time.Duration `yaml:"min_time" json:"min_time"`
	PermitWithoutStream   bool          `yaml:"permit_without_stream" json:"permit_without_stream"`
	MaxConnectionIdle     time.Duration `yaml:"max_connection_idle" json:"max_connection_idle"`
	MaxConnectionAge      time.Duration `yaml:"max_connection_age" json:"max_connection_age"`
	MaxConnectionAgeGrace time.Duration `yaml:"max_connection_age_grace" json:"max_connection_age_grace"`
}

// WSServerConfig represents WebSocket server configuration
type WSServerConfig struct {
	Host                string        `yaml:"host" json:"host"`
	Port                int           `yaml:"port" json:"port"`
	ReadBufferSize      int           `yaml:"read_buffer_size" json:"read_buffer_size"`
	WriteBufferSize     int           `yaml:"write_buffer_size" json:"write_buffer_size"`
	EnableCompression   bool          `yaml:"enable_compression" json:"enable_compression"`
	PingInterval        time.Duration `yaml:"ping_interval" json:"ping_interval"`
	PongTimeout         time.Duration `yaml:"pong_timeout" json:"pong_timeout"`
	WriteTimeout        time.Duration `yaml:"write_timeout" json:"write_timeout"`
	ReadTimeout         time.Duration `yaml:"read_timeout" json:"read_timeout"`
	IdleTimeout         time.Duration `yaml:"idle_timeout" json:"idle_timeout"`
	MaxMessageSize      int64         `yaml:"max_message_size" json:"max_message_size"`
	MaxMessageQueueSize int           `yaml:"max_message_queue_size" json:"max_message_queue_size"`
	ShutdownTimeout     time.Duration `yaml:"shutdown_timeout" json:"shutdown_timeout"`
	MaxConnections      int           `yaml:"max_connections" json:"max_connections"`
	AllowedOrigins      []string      `yaml:"allowed_origins" json:"allowed_origins"`
}

// TLSConfig represents TLS configuration
type TLSConfig struct {
	Enabled             bool          `yaml:"enabled" json:"enabled"`
	CertFile            string        `yaml:"cert_file" json:"cert_file"`
	KeyFile             string        `yaml:"key_file" json:"key_file"`
	CAFile              string        `yaml:"ca_file" json:"ca_file"`
	MinVersion          string        `yaml:"min_version" json:"min_version"`
	MaxVersion          string        `yaml:"max_version" json:"max_version"`
	CipherSuites        []string      `yaml:"cipher_suites" json:"cipher_suites"`
	PreferServerCiphers bool          `yaml:"prefer_server_ciphers" json:"prefer_server_ciphers"`
	ClientAuth          string        `yaml:"client_auth" json:"client_auth"`
	HotReload           bool          `yaml:"hot_reload" json:"hot_reload"`
	ReloadInterval      time.Duration `yaml:"reload_interval" json:"reload_interval"`
	ECDHCurves          []string      `yaml:"ecdh_curves" json:"ecdh_curves"`
}

// Config represents the application configuration
type Config struct {
	Server   ServerConfig `yaml:"server" json:"server"`
	Database struct {
		DSN             string `yaml:"dsn" json:"dsn"`
		CockroachDSN    string `yaml:"cockroach_dsn" json:"cockroach_dsn"`
		MaxOpenConns    int    `yaml:"max_open_conns" json:"max_open_conns"`
		MaxIdleConns    int    `yaml:"max_idle_conns" json:"max_idle_conns"`
		ConnMaxLifetime int    `yaml:"conn_max_lifetime" json:"conn_max_lifetime"` // seconds
	} `yaml:"database" json:"database"`
	Redis struct {
		Address  string `yaml:"address" json:"address"`
		Password string `yaml:"password" json:"password"`
		DB       int    `yaml:"db" json:"db"`
	} `yaml:"redis" json:"redis"`
	JWT struct {
		Secret          string `yaml:"secret" json:"secret"`
		ExpirationHours int    `yaml:"expiration_hours" json:"expiration_hours"`
		RefreshSecret   string `yaml:"refresh_secret" json:"refresh_secret"`
		RefreshExpHours int    `yaml:"refresh_exp_hours" json:"refresh_exp_hours"`
	} `yaml:"jwt" json:"jwt"`
	KYC struct {
		ProviderURL      string `yaml:"provider_url" json:"provider_url"`
		ProviderAPIKey   string `yaml:"provider_api_key" json:"provider_api_key"`
		DocumentBasePath string `yaml:"document_base_path" json:"document_base_path"`
	} `yaml:"kyc" json:"kyc"`
	Kafka struct {
		Brokers             []string `yaml:"brokers" json:"brokers"`
		EnableMessageQueue  bool     `yaml:"enable_message_queue" json:"enable_message_queue"`
		ConsumerGroupPrefix string   `yaml:"consumer_group_prefix" json:"consumer_group_prefix"`
	} `yaml:"kafka" json:"kafka"`
	DBSharding   tradingconfig.DBConfig    `yaml:"db_sharding" json:"db_sharding"`
	RedisCluster tradingconfig.RedisConfig `yaml:"redis_cluster" json:"redis_cluster"`
}

// LoadConfig loads the application configuration
func LoadConfig() (*Config, error) {
	// Set default configuration
	config := &Config{}

	// Server defaults
	config.Server = ServerConfig{
		HTTP: HTTPServerConfig{
			Host:               "0.0.0.0",
			Port:               8080,
			ReadTimeout:        30 * time.Second,
			WriteTimeout:       30 * time.Second,
			IdleTimeout:        120 * time.Second,
			ReadHeaderTimeout:  10 * time.Second,
			MaxHeaderBytes:     1 << 20, // 1MB
			ShutdownTimeout:    30 * time.Second,
			EnableCompression:  true,
			EnableHTTP2:        true,
			MaxConcurrentConns: 10000,
			HealthCheckPath:    "/health",
			ReadinessCheckPath: "/ready",
			MetricsPath:        "/metrics",
		},
		GRPC: GRPCServerConfig{
			Host:              "0.0.0.0",
			Port:              9090,
			MaxRecvMsgSize:    4 << 20, // 4MB
			MaxSendMsgSize:    4 << 20, // 4MB
			ConnectionTimeout: 5 * time.Second,
			KeepAlive: KeepAliveConfig{
				Time:                  2 * time.Hour,
				Timeout:               20 * time.Second,
				EnforcementPolicy:     true,
				MinTime:               5 * time.Minute,
				PermitWithoutStream:   false,
				MaxConnectionIdle:     2 * time.Hour,
				MaxConnectionAge:      24 * time.Hour,
				MaxConnectionAgeGrace: 5 * time.Minute,
			},
			EnableReflection: false,
			ShutdownTimeout:  30 * time.Second,
		},
		WS: WSServerConfig{
			Host:                "0.0.0.0",
			Port:                8081,
			ReadBufferSize:      1024,
			WriteBufferSize:     1024,
			EnableCompression:   true,
			PingInterval:        54 * time.Second,
			PongTimeout:         60 * time.Second,
			WriteTimeout:        10 * time.Second,
			ReadTimeout:         30 * time.Second,
			IdleTimeout:         120 * time.Second,
			MaxMessageSize:      512 * 1024, // 512KB
			MaxMessageQueueSize: 1000,
			ShutdownTimeout:     30 * time.Second,
			MaxConnections:      10000,
			AllowedOrigins:      []string{"*"},
		},
		TLS: TLSConfig{
			Enabled:             true,
			MinVersion:          "1.2",
			MaxVersion:          "1.3",
			PreferServerCiphers: true,
			ClientAuth:          "NoClientCert",
			HotReload:           true,
			ReloadInterval:      5 * time.Minute,
			CipherSuites: []string{
				"TLS_AES_256_GCM_SHA384",
				"TLS_CHACHA20_POLY1305_SHA256",
				"TLS_AES_128_GCM_SHA256",
				"TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384",
				"TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305",
				"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
			},
			ECDHCurves: []string{
				"X25519",
				"P-256",
				"P-384",
			},
		},
	}

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
		config.Server.HTTP.Port = port
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
			config.Server.HTTP.Port = viper.GetInt("server.port")
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
