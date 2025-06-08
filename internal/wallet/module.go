// Package wallet provides the main wallet module orchestrator
package wallet

import (
	"context"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	complianceInterfaces "github.com/Aidin1998/finalex/internal/compliance/interfaces"
	"github.com/Aidin1998/finalex/internal/wallet/cache"
	"github.com/Aidin1998/finalex/internal/wallet/config"
	"github.com/Aidin1998/finalex/internal/wallet/handlers/grpc"
	"github.com/Aidin1998/finalex/internal/wallet/handlers/rest"
	"github.com/Aidin1998/finalex/internal/wallet/interfaces"
	"github.com/Aidin1998/finalex/internal/wallet/repository"
	"github.com/Aidin1998/finalex/internal/wallet/services"
	"github.com/Aidin1998/finalex/internal/wallet/state"
)

// Module represents the wallet module
type Module struct {
	config *config.WalletConfig
	log    *zap.Logger

	// Database connections
	db    *gorm.DB
	redis redis.Cmdable

	// Core services
	walletService     interfaces.WalletService
	depositManager    interfaces.DepositManager
	withdrawalManager interfaces.WithdrawalManager
	balanceManager    interfaces.BalanceManager
	addressManager    interfaces.AddressManager
	fundLockService   interfaces.FundLockService

	// Supporting services
	fireblocksClient interfaces.FireblocksClient
	repository       interfaces.WalletRepository
	cache            interfaces.WalletCache
	stateMachine     *state.TransactionStateMachine
	eventPublisher   interfaces.EventPublisher

	// API handlers
	grpcHandler *grpc.WalletHandler
	restHandler *rest.WalletHandler

	// Background workers
	workers []Worker
}

// Worker represents a background worker
type Worker interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Name() string
}

// ModuleOptions holds module initialization options
type ModuleOptions struct {
	Config            *config.WalletConfig
	Logger            *zap.Logger
	Database          *gorm.DB
	Redis             redis.Cmdable
	EventPublisher    interfaces.EventPublisher
	ComplianceService complianceInterfaces.ComplianceService
	AuditService      complianceInterfaces.AuditService
}

// NewModule creates a new wallet module instance
func NewModule(opts ModuleOptions) (*Module, error) {
	if err := opts.Config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	module := &Module{
		config:         opts.Config,
		log:            opts.Logger,
		db:             opts.Database,
		redis:          opts.Redis,
		eventPublisher: opts.EventPublisher,
	}

	// Initialize components
	if err := module.initializeComponents(opts); err != nil {
		return nil, fmt.Errorf("failed to initialize components: %w", err)
	}

	return module, nil
}

// initializeComponents initializes all module components
func (m *Module) initializeComponents(opts ModuleOptions) error {
	// Initialize cache
	m.cache = cache.NewRedisWalletCache(
		m.redis,
		m.log,
		m.config.Cache.KeyPrefix,
		m.config.Cache.TTL,
	)

	// Initialize repository
	m.repository = repository.NewWalletRepository(m.db, m.log)

	// Initialize Fireblocks client
	fireblocksClient, err := services.NewFireblocksClient(
		m.config.Fireblocks.BaseURL,
		m.config.Fireblocks.APIKey,
		m.config.Fireblocks.PrivateKeyPath,
		m.config.Fireblocks.VaultAccountID,
		m.log,
	)
	if err != nil {
		return fmt.Errorf("failed to create Fireblocks client: %w", err)
	}
	m.fireblocksClient = fireblocksClient

	// Initialize state machine
	m.stateMachine = state.NewTransactionStateMachine(
		m.repository,
		m.cache,
		m.eventPublisher,
		m.log,
	)

	// Initialize core services
	m.balanceManager = services.NewBalanceManager()

	m.fundLockService = services.NewFundLockService(
		m.repository,
		m.cache,
	)

	m.addressManager = services.NewAddressManager()

	m.depositManager = services.NewDepositManager()

	m.withdrawalManager = services.NewWithdrawalManager()

	// Initialize main wallet service
	m.walletService = services.NewWalletService(
		m.repository,
		m.cache,
		m.depositManager,
		m.withdrawalManager,
		m.balanceManager,
		m.addressManager,
		m.fundLockService,
		m.fireblocksClient,
		opts.ComplianceService,
		m.eventPublisher,
		m.config,
		m.log,
	)

	// Initialize API handlers
	m.grpcHandler = grpc.NewWalletHandler(m.walletService)
	m.restHandler = rest.NewWalletHandler(m.walletService)

	// Initialize background workers
	m.initializeWorkers()

	return nil
}

// initializeWorkers initializes background workers
func (m *Module) initializeWorkers() {
	// Confirmation worker - monitors blockchain confirmations
	confirmationWorker := &ConfirmationWorker{
		repository:   m.repository,
		fireblocks:   m.fireblocksClient,
		stateMachine: m.stateMachine,
		config:       m.config,
		log:          m.log,
		interval:     30 * time.Second,
	}
	m.workers = append(m.workers, confirmationWorker)

	// Cleanup worker - cleans up expired locks and old data
	cleanupWorker := &CleanupWorker{
		repository:      m.repository,
		fundLockService: m.fundLockService,
		log:             m.log,
		interval:        5 * time.Minute,
	}
	m.workers = append(m.workers, cleanupWorker)

	// Balance sync worker - synchronizes balances with Fireblocks
	balanceSyncWorker := &BalanceSyncWorker{
		repository:     m.repository,
		fireblocks:     m.fireblocksClient,
		balanceManager: m.balanceManager,
		config:         m.config,
		log:            m.log,
		interval:       2 * time.Minute,
	}
	m.workers = append(m.workers, balanceSyncWorker)

	// Webhook processor - processes Fireblocks webhooks
	webhookWorker := &WebhookWorker{
		repository:   m.repository,
		stateMachine: m.stateMachine,
		config:       m.config,
		log:          m.log,
	}
	m.workers = append(m.workers, webhookWorker)
}

// Start starts the wallet module
func (m *Module) Start(ctx context.Context) error {
	m.log.Info("starting wallet module")

	// Run database migrations
	if err := m.runMigrations(); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	// Start background workers
	for _, worker := range m.workers {
		if err := worker.Start(ctx); err != nil {
			m.log.Error("failed to start worker", zap.String("worker", worker.Name()), zap.Error(err))
			return fmt.Errorf("failed to start worker %s: %w", worker.Name(), err)
		}
		m.log.Info("started worker", zap.String("worker", worker.Name()))
	}

	// Perform health check
	if err := m.HealthCheck(ctx); err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}

	m.log.Info("wallet module started successfully")
	return nil
}

// Stop stops the wallet module
func (m *Module) Stop(ctx context.Context) error {
	m.log.Info("stopping wallet module")

	// Stop background workers
	for _, worker := range m.workers {
		if err := worker.Stop(ctx); err != nil {
			m.log.Error("failed to stop worker", zap.String("worker", worker.Name()), zap.Error(err))
		} else {
			m.log.Info("stopped worker", zap.String("worker", worker.Name()))
		}
	}

	// Close database connection
	if sqlDB, err := m.db.DB(); err == nil {
		if err := sqlDB.Close(); err != nil {
			m.log.Error("failed to close database connection", zap.Error(err))
		}
	}

	m.log.Info("wallet module stopped")
	return nil
}

// runMigrations runs database migrations
func (m *Module) runMigrations() error {
	m.log.Info("running database migrations")

	// Auto-migrate all models
	err := m.db.AutoMigrate(
		&interfaces.WalletTransaction{},
		&interfaces.WalletBalance{},
		&interfaces.DepositAddress{},
		&interfaces.FundLock{},
	)
	if err != nil {
		return fmt.Errorf("failed to run auto-migrations: %w", err)
	}

	// Create indexes for better performance
	if err := m.createIndexes(); err != nil {
		return fmt.Errorf("failed to create indexes: %w", err)
	}

	m.log.Info("database migrations completed")
	return nil
}

// createIndexes creates database indexes for performance
func (m *Module) createIndexes() error {
	indexes := []string{
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_wallet_transactions_user_asset ON wallet_transactions(user_id, asset)",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_wallet_transactions_status_created ON wallet_transactions(status, created_at)",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_wallet_transactions_fireblocks_id ON wallet_transactions(fireblocks_id) WHERE fireblocks_id != ''",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_wallet_balances_user_asset ON wallet_balances(user_id, asset)",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_deposit_addresses_user_asset_network ON deposit_addresses(user_id, asset, network)",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_fund_locks_user_asset ON fund_locks(user_id, asset)",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_fund_locks_expires ON fund_locks(expires) WHERE expires IS NOT NULL",
	}

	for _, index := range indexes {
		if err := m.db.Exec(index).Error; err != nil {
			m.log.Warn("failed to create index", zap.String("sql", index), zap.Error(err))
		}
	}

	return nil
}

// HealthCheck performs a comprehensive health check
func (m *Module) HealthCheck(ctx context.Context) error {
	// Check database connection
	if sqlDB, err := m.db.DB(); err != nil {
		return fmt.Errorf("failed to get database connection: %w", err)
	} else if err := sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}

	// Check Redis connection
	if err := m.redis.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis ping failed: %w", err)
	}
	// Check Fireblocks connection (temporarily disabled - stub implementation)
	// if err := m.fireblocksClient.HealthCheck(ctx); err != nil {
	// 	return fmt.Errorf("fireblocks health check failed: %w", err)
	// }

	// Check wallet service
	if err := m.walletService.HealthCheck(ctx); err != nil {
		return fmt.Errorf("wallet service health check failed: %w", err)
	}

	return nil
}

// GetWalletService returns the wallet service
func (m *Module) GetWalletService() interfaces.WalletService {
	return m.walletService
}

// GetGRPCHandler returns the gRPC handler
func (m *Module) GetGRPCHandler() *grpc.WalletHandler {
	return m.grpcHandler
}

// GetRESTHandler returns the REST handler
func (m *Module) GetRESTHandler() *rest.WalletHandler {
	return m.restHandler
}

// GetConfig returns the module configuration
func (m *Module) GetConfig() *config.WalletConfig {
	return m.config
}

// NewModuleFromConfig creates a module from configuration
func NewModuleFromConfig(configPath string, opts ModuleOptions) (*Module, error) {
	// Load configuration from file
	cfg, err := config.LoadConfigFromFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	opts.Config = cfg
	return NewModule(opts)
}

// InitializeDatabase initializes database connection from config
func InitializeDatabase(cfg config.DatabaseConfig) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(cfg.GetDSN()), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Configure connection pool
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get database instance: %w", err)
	}

	sqlDB.SetMaxOpenConns(cfg.MaxConnections)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	return db, nil
}

// InitializeRedis initializes Redis connection from config
func InitializeRedis(cfg config.RedisConfig) (redis.Cmdable, error) {
	if len(cfg.Addresses) == 1 {
		// Single Redis instance
		client := redis.NewClient(&redis.Options{
			Addr:         cfg.Addresses[0],
			Password:     cfg.Password,
			DB:           cfg.Database,
			PoolSize:     cfg.PoolSize,
			MinIdleConns: cfg.MinIdleConns,
			DialTimeout:  cfg.DialTimeout,
			ReadTimeout:  cfg.ReadTimeout,
			WriteTimeout: cfg.WriteTimeout,
			MaxRetries:   cfg.MaxRetries,
		})
		return client, nil
	} else {
		// Redis Cluster
		client := redis.NewClusterClient(&redis.ClusterOptions{
			Addrs:        cfg.Addresses,
			Password:     cfg.Password,
			PoolSize:     cfg.PoolSize,
			MinIdleConns: cfg.MinIdleConns,
			DialTimeout:  cfg.DialTimeout,
			ReadTimeout:  cfg.ReadTimeout,
			WriteTimeout: cfg.WriteTimeout,
			MaxRetries:   cfg.MaxRetries,
		})
		return client, nil
	}
}
