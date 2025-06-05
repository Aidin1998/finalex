// Ultra-High Concurrency Service Integration Layer
// Bridges the existing accounts service layer with the new ultra-high concurrency database components
package accounts

import (
	"context"
	"fmt"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/accounts/bookkeeper"
	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/go-redis/redis/v8"
	"github.com/go-redsync/redsync/v4"
	"github.com/go-redsync/redsync/v4/redis/goredis/v8"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// UltraHighConcurrencyService integrates all ultra-high concurrency components
type UltraHighConcurrencyService struct {
	// Core components
	repository   *Repository
	cache        *CacheLayer
	database     *DatabaseManager
	partitionMgr *PartitionManager
	metrics      *MetricsCollector

	// Legacy compatibility
	bookkeeper bookkeeper.BookkeeperService

	// Configuration
	config *ServiceConfig
	logger *zap.Logger
}

// ServiceConfig provides configuration for the ultra-high concurrency service
type ServiceConfig struct {
	// Performance settings
	EnableCaching      bool `json:"enable_caching"`
	EnablePartitioning bool `json:"enable_partitioning"`
	EnableMetrics      bool `json:"enable_metrics"`
	BatchSize          int  `json:"batch_size"`
	ConcurrencyLimit   int  `json:"concurrency_limit"`

	// Cache settings
	CacheConfig CacheConfig `json:"cache_config"`

	// Database settings
	DatabaseConfig DatabaseConfig `json:"database_config"`

	// Partition settings
	PartitionConfig PartitionConfig `json:"partition_config"`

	// Timeout settings
	DefaultTimeout time.Duration `json:"default_timeout"`
	LockTimeout    time.Duration `json:"lock_timeout"`
	CacheTimeout   time.Duration `json:"cache_timeout"`
}

// DefaultServiceConfig returns a production-ready configuration
func DefaultServiceConfig() *ServiceConfig {
	return &ServiceConfig{
		EnableCaching:      true,
		EnablePartitioning: true,
		EnableMetrics:      true,
		BatchSize:          100,
		ConcurrencyLimit:   10000,
		DefaultTimeout:     30 * time.Second,
		LockTimeout:        5 * time.Second,
		CacheTimeout:       60 * time.Second,
		CacheConfig:        *DefaultCacheConfig(),
		DatabaseConfig:     *DefaultDatabaseConfig(),
		PartitionConfig:    *DefaultPartitionConfig(),
	}
}

// NewUltraHighConcurrencyService creates a new ultra-high concurrency service
func NewUltraHighConcurrencyService(
	db *gorm.DB,
	redisClient redis.UniversalClient,
	bookkeeperSvc bookkeeper.BookkeeperService,
	config *ServiceConfig,
	logger *zap.Logger,
) (*UltraHighConcurrencyService, error) {
	if config == nil {
		config = DefaultServiceConfig()
	}

	// Initialize database manager
	dbManager, err := NewDatabaseManager(&config.DatabaseConfig, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create database manager: %w", err)
	}

	// Initialize metrics collector
	var metricsCollector *MetricsCollector
	if config.EnableMetrics {
		metricsCollector = NewMetricsCollector()
	}

	// Initialize cache layer
	var cacheLayer *CacheLayer
	if config.EnableCaching {
		// Create redsync for distributed locking
		pool := goredis.NewPool(redisClient)
		rs := redsync.New(pool)

		cacheLayer, err = NewCacheLayer(redisClient, rs, &config.CacheConfig, metricsCollector, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to create cache layer: %w", err)
		}
	}

	// Initialize partition manager
	var partitionMgr *PartitionManager
	if config.EnablePartitioning {
		partitionMgr, err = NewPartitionManager(db, &config.PartitionConfig, metricsCollector, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to create partition manager: %w", err)
		}
	}

	// Initialize repository
	repository, err := NewRepository(dbManager, cacheLayer, metricsCollector, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create repository: %w", err)
	}

	service := &UltraHighConcurrencyService{
		repository:   repository,
		cache:        cacheLayer,
		database:     dbManager,
		partitionMgr: partitionMgr,
		metrics:      metricsCollector,
		bookkeeper:   bookkeeperSvc,
		config:       config,
		logger:       logger,
	}

	logger.Info("Ultra-high concurrency accounts service initialized",
		zap.Bool("caching_enabled", config.EnableCaching),
		zap.Bool("partitioning_enabled", config.EnablePartitioning),
		zap.Bool("metrics_enabled", config.EnableMetrics),
		zap.Int("batch_size", config.BatchSize),
		zap.Int("concurrency_limit", config.ConcurrencyLimit),
	)

	return service, nil
}

// GetAccount retrieves an account with ultra-high concurrency optimizations
func (s *UltraHighConcurrencyService) GetAccount(ctx context.Context, userID, currency string) (*Account, error) {
	startTime := time.Now()
	defer func() {
		if s.metrics != nil {
			s.metrics.RecordDuration("get_account_duration", time.Since(startTime))
		}
	}()

	// Try cache first if enabled
	if s.config.EnableCaching && s.cache != nil {
		account, err := s.cache.GetAccount(ctx, userID, currency)
		if err == nil && account != nil {
			if s.metrics != nil {
				s.metrics.IncrementCounter("cache_hits_total", "operation", "get_account")
			}
			return account, nil
		}
		if s.metrics != nil {
			s.metrics.IncrementCounter("cache_misses_total", "operation", "get_account")
		}
	}

	// Fallback to repository
	account, err := s.repository.GetAccount(ctx, userID, currency)
	if err != nil {
		if s.metrics != nil {
			s.metrics.IncrementCounter("errors_total", "operation", "get_account", "error", "repository_error")
		}
		return nil, fmt.Errorf("failed to get account from repository: %w", err)
	}

	// Update cache if enabled
	if s.config.EnableCaching && s.cache != nil && account != nil {
		go func() {
			cacheCtx, cancel := context.WithTimeout(context.Background(), s.config.CacheTimeout)
			defer cancel()
			if err := s.cache.SetAccount(cacheCtx, account); err != nil {
				s.logger.Warn("Failed to cache account", zap.Error(err))
			}
		}()
	}

	return account, nil
}

// UpdateBalance performs ultra-high concurrency balance updates with optimistic locking
func (s *UltraHighConcurrencyService) UpdateBalance(
	ctx context.Context,
	userID, currency string,
	balanceDelta, availableDelta, lockedDelta decimal.Decimal,
	reason string,
) (*Account, error) {
	startTime := time.Now()
	defer func() {
		if s.metrics != nil {
			s.metrics.RecordDuration("update_balance_duration", time.Since(startTime))
		}
	}()

	// Use repository for atomic update with optimistic locking
	account, err := s.repository.UpdateBalanceAtomic(ctx, userID, currency, balanceDelta, availableDelta, lockedDelta, reason)
	if err != nil {
		if s.metrics != nil {
			s.metrics.IncrementCounter("errors_total", "operation", "update_balance", "error", "repository_error")
		}
		return nil, fmt.Errorf("failed to update balance: %w", err)
	}

	// Invalidate cache if enabled
	if s.config.EnableCaching && s.cache != nil {
		go func() {
			cacheCtx, cancel := context.WithTimeout(context.Background(), s.config.CacheTimeout)
			defer cancel()
			if err := s.cache.InvalidateAccount(cacheCtx, userID, currency); err != nil {
				s.logger.Warn("Failed to invalidate cache", zap.Error(err))
			}
		}()
	}

	if s.metrics != nil {
		s.metrics.IncrementCounter("balance_updates_total", "currency", currency, "reason", reason)
	}

	return account, nil
}

// CreateAccount creates a new account with ultra-high concurrency support
func (s *UltraHighConcurrencyService) CreateAccount(ctx context.Context, userID, currency string) (*Account, error) {
	startTime := time.Now()
	defer func() {
		if s.metrics != nil {
			s.metrics.RecordDuration("create_account_duration", time.Since(startTime))
		}
	}()

	account, err := s.repository.CreateAccount(ctx, userID, currency)
	if err != nil {
		if s.metrics != nil {
			s.metrics.IncrementCounter("errors_total", "operation", "create_account", "error", "repository_error")
		}
		return nil, fmt.Errorf("failed to create account: %w", err)
	}

	// Cache the new account if enabled
	if s.config.EnableCaching && s.cache != nil {
		go func() {
			cacheCtx, cancel := context.WithTimeout(context.Background(), s.config.CacheTimeout)
			defer cancel()
			if err := s.cache.SetAccount(cacheCtx, account); err != nil {
				s.logger.Warn("Failed to cache new account", zap.Error(err))
			}
		}()
	}

	if s.metrics != nil {
		s.metrics.IncrementCounter("accounts_created_total", "currency", currency)
	}

	return account, nil
}

// BatchGetAccounts retrieves multiple accounts efficiently with caching
func (s *UltraHighConcurrencyService) BatchGetAccounts(
	ctx context.Context,
	userIDs []string,
	currencies []string,
) (map[string]map[string]*Account, error) {
	startTime := time.Now()
	defer func() {
		if s.metrics != nil {
			s.metrics.RecordDuration("batch_get_accounts_duration", time.Since(startTime))
		}
	}()

	// Try batch cache lookup if enabled
	var cacheHits map[string]map[string]*Account
	var cacheMisses []AccountKey

	if s.config.EnableCaching && s.cache != nil {
		cacheHits, cacheMisses = s.cache.BatchGetAccounts(ctx, userIDs, currencies)
		if s.metrics != nil {
			s.metrics.RecordGauge("cache_hit_ratio", float64(len(cacheHits))/float64(len(userIDs)*len(currencies)))
		}
	}

	// Fetch cache misses from repository
	var repositoryResults map[string]map[string]*Account
	if len(cacheMisses) > 0 || !s.config.EnableCaching {
		var err error
		repositoryResults, err = s.repository.BatchGetAccounts(ctx, userIDs, currencies)
		if err != nil {
			if s.metrics != nil {
				s.metrics.IncrementCounter("errors_total", "operation", "batch_get_accounts", "error", "repository_error")
			}
			return nil, fmt.Errorf("failed to batch get accounts: %w", err)
		}

		// Update cache with repository results
		if s.config.EnableCaching && s.cache != nil {
			go func() {
				cacheCtx, cancel := context.WithTimeout(context.Background(), s.config.CacheTimeout)
				defer cancel()
				s.cache.BatchSetAccounts(cacheCtx, repositoryResults)
			}()
		}
	}

	// Merge cache hits and repository results
	result := make(map[string]map[string]*Account)

	// Add cache hits
	for userID, userAccounts := range cacheHits {
		if result[userID] == nil {
			result[userID] = make(map[string]*Account)
		}
		for currency, account := range userAccounts {
			result[userID][currency] = account
		}
	}

	// Add repository results
	for userID, userAccounts := range repositoryResults {
		if result[userID] == nil {
			result[userID] = make(map[string]*Account)
		}
		for currency, account := range userAccounts {
			result[userID][currency] = account
		}
	}

	if s.metrics != nil {
		s.metrics.IncrementCounter("batch_operations_total", "operation", "get_accounts", "size", fmt.Sprintf("%d", len(userIDs)))
	}

	return result, nil
}

// TransferFunds performs atomic fund transfers with distributed locking
func (s *UltraHighConcurrencyService) TransferFunds(
	ctx context.Context,
	fromUserID, toUserID, currency string,
	amount decimal.Decimal,
	reason string,
) error {
	startTime := time.Now()
	defer func() {
		if s.metrics != nil {
			s.metrics.RecordDuration("transfer_funds_duration", time.Since(startTime))
		}
	}()

	// Use repository for atomic transfer with distributed locking
	err := s.repository.TransferFunds(ctx, fromUserID, toUserID, currency, amount, reason)
	if err != nil {
		if s.metrics != nil {
			s.metrics.IncrementCounter("errors_total", "operation", "transfer_funds", "error", "repository_error")
		}
		return fmt.Errorf("failed to transfer funds: %w", err)
	}

	// Invalidate cache for both accounts
	if s.config.EnableCaching && s.cache != nil {
		go func() {
			cacheCtx, cancel := context.WithTimeout(context.Background(), s.config.CacheTimeout)
			defer cancel()
			s.cache.InvalidateAccount(cacheCtx, fromUserID, currency)
			s.cache.InvalidateAccount(cacheCtx, toUserID, currency)
		}()
	}

	if s.metrics != nil {
		s.metrics.IncrementCounter("transfers_total", "currency", currency, "reason", reason)
		s.metrics.RecordGauge("transfer_amount", amount.InexactFloat64(), "currency", currency)
	}

	return nil
}

// GetAccountBalance retrieves account balance with caching
func (s *UltraHighConcurrencyService) GetAccountBalance(
	ctx context.Context,
	userID, currency string,
) (balance, available decimal.Decimal, version int64, err error) {
	startTime := time.Now()
	defer func() {
		if s.metrics != nil {
			s.metrics.RecordDuration("get_balance_duration", time.Since(startTime))
		}
	}()

	// Try cache first
	if s.config.EnableCaching && s.cache != nil {
		balance, available, version, err = s.cache.GetAccountBalance(ctx, userID, currency)
		if err == nil {
			if s.metrics != nil {
				s.metrics.IncrementCounter("cache_hits_total", "operation", "get_balance")
			}
			return balance, available, version, nil
		}
		if s.metrics != nil {
			s.metrics.IncrementCounter("cache_misses_total", "operation", "get_balance")
		}
	}

	// Fallback to repository
	balance, available, version, err = s.repository.GetAccountBalance(ctx, userID, currency)
	if err != nil {
		if s.metrics != nil {
			s.metrics.IncrementCounter("errors_total", "operation", "get_balance", "error", "repository_error")
		}
		return decimal.Zero, decimal.Zero, 0, fmt.Errorf("failed to get account balance: %w", err)
	}

	// Update cache
	if s.config.EnableCaching && s.cache != nil {
		go func() {
			cacheCtx, cancel := context.WithTimeout(context.Background(), s.config.CacheTimeout)
			defer cancel()
			s.cache.SetAccountBalance(cacheCtx, userID, currency, balance, available, version)
		}()
	}

	return balance, available, version, nil
}

// HealthCheck provides comprehensive health checking for all components
func (s *UltraHighConcurrencyService) HealthCheck(ctx context.Context) map[string]interface{} {
	health := make(map[string]interface{})

	// Database health
	if s.database != nil {
		health["database"] = s.database.HealthCheck(ctx)
	}

	// Cache health
	if s.cache != nil {
		health["cache"] = s.cache.HealthCheck(ctx)
	}

	// Partition manager health
	if s.partitionMgr != nil {
		health["partitions"] = s.partitionMgr.HealthCheck(ctx)
	}

	// Repository health
	if s.repository != nil {
		health["repository"] = s.repository.HealthCheck(ctx)
	}

	// Metrics health
	if s.metrics != nil {
		health["metrics"] = map[string]interface{}{
			"status":   "healthy",
			"counters": s.metrics.GetCounterValues(),
			"gauges":   s.metrics.GetGaugeValues(),
		}
	}

	// Bookkeeper health (legacy compatibility)
	health["bookkeeper_legacy"] = map[string]interface{}{
		"status": "available",
	}

	return health
}

// GetMetrics returns current metrics for monitoring
func (s *UltraHighConcurrencyService) GetMetrics() map[string]interface{} {
	if s.metrics == nil {
		return nil
	}

	return map[string]interface{}{
		"counters":   s.metrics.GetCounterValues(),
		"gauges":     s.metrics.GetGaugeValues(),
		"histograms": s.metrics.GetHistogramValues(),
	}
}

// Start initializes all components and background jobs
func (s *UltraHighConcurrencyService) Start(ctx context.Context) error {
	s.logger.Info("Starting ultra-high concurrency accounts service...")

	// Start database manager
	if s.database != nil {
		if err := s.database.Start(); err != nil {
			return fmt.Errorf("failed to start database manager: %w", err)
		}
	}

	// Start cache layer
	if s.cache != nil {
		if err := s.cache.Start(ctx); err != nil {
			return fmt.Errorf("failed to start cache layer: %w", err)
		}
	}

	// Start partition manager
	if s.partitionMgr != nil {
		if err := s.partitionMgr.Start(ctx); err != nil {
			return fmt.Errorf("failed to start partition manager: %w", err)
		}
	}

	// Start background jobs
	go s.startBackgroundJobs(ctx)

	s.logger.Info("Ultra-high concurrency accounts service started successfully")
	return nil
}

// Stop gracefully shuts down all components
func (s *UltraHighConcurrencyService) Stop(ctx context.Context) error {
	s.logger.Info("Stopping ultra-high concurrency accounts service...")

	// Stop partition manager
	if s.partitionMgr != nil {
		if err := s.partitionMgr.Stop(ctx); err != nil {
			s.logger.Warn("Error stopping partition manager", zap.Error(err))
		}
	}

	// Stop cache layer
	if s.cache != nil {
		if err := s.cache.Stop(ctx); err != nil {
			s.logger.Warn("Error stopping cache layer", zap.Error(err))
		}
	}

	// Stop database manager
	if s.database != nil {
		if err := s.database.Stop(); err != nil {
			s.logger.Warn("Error stopping database manager", zap.Error(err))
		}
	}

	s.logger.Info("Ultra-high concurrency accounts service stopped")
	return nil
}

// startBackgroundJobs starts maintenance and optimization jobs
func (s *UltraHighConcurrencyService) startBackgroundJobs(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Run cache warming
			if s.cache != nil {
				go func() {
					jobCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel()
					if err := s.cache.WarmCache(jobCtx); err != nil {
						s.logger.Warn("Cache warming failed", zap.Error(err))
					}
				}()
			}

			// Run partition maintenance
			if s.partitionMgr != nil {
				go func() {
					jobCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
					defer cancel()
					if err := s.partitionMgr.MaintenanceJob(jobCtx); err != nil {
						s.logger.Warn("Partition maintenance failed", zap.Error(err))
					}
				}()
			}

			// Update metrics
			if s.metrics != nil {
				s.metrics.RecordGauge("background_jobs_last_run", float64(time.Now().Unix()))
			}
		}
	}
}

// Legacy compatibility methods - delegate to bookkeeper service
// These methods maintain backward compatibility while using the new ultra-high concurrency layer

func (s *UltraHighConcurrencyService) GetAccounts(ctx context.Context, userID string) ([]*models.Account, error) {
	return s.bookkeeper.GetAccounts(ctx, userID)
}

func (s *UltraHighConcurrencyService) GetAccountTransactions(ctx context.Context, userID, currency string, limit, offset int) ([]*models.Transaction, int64, error) {
	return s.bookkeeper.GetAccountTransactions(ctx, userID, currency, limit, offset)
}

func (s *UltraHighConcurrencyService) CreateTransaction(ctx context.Context, userID, transactionType string, amount float64, currency, reference, description string) (*models.Transaction, error) {
	return s.bookkeeper.CreateTransaction(ctx, userID, transactionType, amount, currency, reference, description)
}

func (s *UltraHighConcurrencyService) CompleteTransaction(ctx context.Context, transactionID string) error {
	return s.bookkeeper.CompleteTransaction(ctx, transactionID)
}

func (s *UltraHighConcurrencyService) FailTransaction(ctx context.Context, transactionID string) error {
	return s.bookkeeper.FailTransaction(ctx, transactionID)
}

func (s *UltraHighConcurrencyService) LockFunds(ctx context.Context, userID, currency string, amount float64) error {
	return s.bookkeeper.LockFunds(ctx, userID, currency, amount)
}

func (s *UltraHighConcurrencyService) UnlockFunds(ctx context.Context, userID, currency string, amount float64) error {
	return s.bookkeeper.UnlockFunds(ctx, userID, currency, amount)
}
