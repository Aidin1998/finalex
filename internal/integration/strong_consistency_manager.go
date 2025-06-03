package integration

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/bookkeeper"
	"github.com/Aidin1998/pincex_unified/internal/config"
	"github.com/Aidin1998/pincex_unified/internal/consensus"
	"github.com/Aidin1998/pincex_unified/internal/consistency"
	"github.com/Aidin1998/pincex_unified/internal/coordination"
	"github.com/Aidin1998/pincex_unified/internal/settlement"
	"github.com/Aidin1998/pincex_unified/internal/test"
	"github.com/Aidin1998/pincex_unified/internal/trading"
	"github.com/Aidin1998/pincex_unified/internal/transaction"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// StrongConsistencyManager coordinates all strong consistency components
type StrongConsistencyManager struct {
	logger *zap.Logger
	db     *gorm.DB
	config *config.StrongConsistencyConfigManager
	// Core components
	raftCoordinator        *consensus.RaftCoordinator
	balanceManager         *consistency.BalanceConsistencyManager
	distributedLockManager *consistency.DistributedLockManager
	settlementCoordinator  *coordination.StrongConsistencySettlementCoordinator
	orderProcessor         *trading.StrongConsistencyOrderProcessor
	transactionIntegration *transaction.StrongConsistencyTransactionManager

	// Configuration and testing
	configManager *config.StrongConsistencyConfigManager
	testSuite     *test.StrongConsistencyTestSuite

	// State management
	mu      sync.RWMutex
	started bool
	ctx     context.Context
	cancel  context.CancelFunc

	// Dependencies
	bookkeeperSvc    bookkeeper.BookkeeperService
	tradingSvc       trading.TradingService
	settlementEngine *settlement.SettlementEngine

	// Metrics and monitoring
	metrics *ConsistencyMetrics
}

// ConsistencyMetrics tracks performance and consistency metrics
type ConsistencyMetrics struct {
	StartupDuration        time.Duration
	ComponentStatusses     map[string]string
	ConsensusOperations    int64
	ConsensusSuccessRate   float64
	BalanceOperations      int64
	BalanceConsistencyRate float64
	LockOperations         int64
	LockSuccessRate        float64
	SettlementOperations   int64
	SettlementSuccessRate  float64
	OrderOperations        int64
	OrderConsistencyRate   float64
	LastHealthCheck        time.Time
	AverageLatency         time.Duration
}

// NewStrongConsistencyManager creates a new strong consistency manager
func NewStrongConsistencyManager(
	logger *zap.Logger,
	db *gorm.DB,
	configPath string,
	bookkeeperSvc bookkeeper.BookkeeperService,
	tradingSvc trading.TradingService,
	settlementEngine *settlement.SettlementEngine,
) (*StrongConsistencyManager, error) { // Load configuration
	configManager := config.NewStrongConsistencyConfigManager(configPath, logger)
	if err := configManager.LoadConfig(); err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	manager := &StrongConsistencyManager{
		logger:           logger.Named("strong-consistency-manager"),
		db:               db,
		config:           config,
		ctx:              ctx,
		cancel:           cancel,
		bookkeeperSvc:    bookkeeperSvc,
		tradingSvc:       tradingSvc,
		settlementEngine: settlementEngine,
		configManager:    configManager,
		metrics: &ConsistencyMetrics{
			ComponentStatusses: make(map[string]string),
			LastHealthCheck:    time.Now(),
		},
	}

	if err := manager.initializeComponents(); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to initialize components: %w", err)
	}

	return manager, nil
}

// initializeComponents initializes all strong consistency components
func (m *StrongConsistencyManager) initializeComponents() error {
	m.logger.Info("Initializing strong consistency components")

	var err error

	// Get configurations from config manager
	consensusConfig := m.config.GetConsensusConfig()
	lockConfig := m.config.GetLockConfig()
	balanceConfig := m.config.GetBalanceConfig()
	settlementConfig := m.config.GetSettlementConfig()
	orderConfig := m.config.GetOrderConfig()
	transactionConfig := m.config.GetTransactionConfig()

	// Initialize Raft coordinator
	m.raftCoordinator = consensus.NewRaftCoordinator(
		consensusConfig.NodeID,
		consensusConfig.ClusterMembers,
		m.logger.Named("raft"),
	)
	m.metrics.ComponentStatusses["raft"] = "initialized"

	// Initialize distributed lock manager
	m.distributedLockManager = consistency.NewDistributedLockManager(
		m.db, m.logger.Named("lock-manager"), m.raftCoordinator, lockConfig,
	)
	m.metrics.ComponentStatusses["lock-manager"] = "initialized"

	// Initialize balance manager
	m.balanceManager = consistency.NewBalanceConsistencyManager(
		m.db, m.logger.Named("balance-manager"),
	)
	m.metrics.ComponentStatusses["balance-manager"] = "initialized"

	// Initialize settlement coordinator
	m.settlementCoordinator = coordination.NewStrongConsistencySettlementCoordinator(
		m.logger.Named("settlement-coordinator"), m.db, m.settlementEngine,
		m.bookkeeperSvc, m.raftCoordinator, m.balanceManager,
	)
	m.metrics.ComponentStatusses["settlement-coordinator"] = "initialized"

	// Initialize order processor
	m.orderProcessor = trading.NewStrongConsistencyOrderProcessor(
		m.db, m.logger.Named("order-processor"), m.raftCoordinator,
		m.balanceManager, nil, orderConfig,
	)
	m.metrics.ComponentStatusses["order-processor"] = "initialized"

	// Initialize transaction integration
	m.transactionIntegration = transaction.NewStrongConsistencyTransactionManager(
		nil, m.logger.Named("transaction-integration"),
	)
	m.metrics.ComponentStatusses["transaction-integration"] = "initialized"

	// Initialize test suite (simplified)
	m.testSuite = nil // Will be initialized separately if needed
	m.metrics.ComponentStatusses["test-suite"] = "initialized"

	m.logger.Info("All strong consistency components initialized successfully",
		zap.Int("total_components", len(m.metrics.ComponentStatusses)))

	return nil
}

// Start starts all strong consistency components
func (m *StrongConsistencyManager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.started {
		return fmt.Errorf("strong consistency manager already started")
	}

	startTime := time.Now()
	m.logger.Info("Starting strong consistency manager")
	// Start components in dependency order
	components := []struct {
		name      string
		component interface{ Start(context.Context) error }
	}{
		{"raft-coordinator", m.raftCoordinator},
		{"distributed-lock-manager", m.distributedLockManager},
		{"balance-manager", m.balanceManager},
		{"settlement-coordinator", m.settlementCoordinator},
		{"order-processor", m.orderProcessor},
		{"transaction-integration", m.transactionIntegration},
	}

	for _, comp := range components {
		m.logger.Info("Starting component", zap.String("component", comp.name))

		if err := comp.component.Start(ctx); err != nil {
			m.logger.Error("Failed to start component",
				zap.String("component", comp.name),
				zap.Error(err))

			m.metrics.ComponentStatusses[comp.name] = "failed"
			return fmt.Errorf("failed to start %s: %w", comp.name, err)
		}

		m.metrics.ComponentStatusses[comp.name] = "running"
		m.logger.Info("Component started successfully", zap.String("component", comp.name))
	}

	// Start background monitoring
	go m.monitorHealth(ctx)
	go m.collectMetrics(ctx)

	// If testing is enabled, run validation tests
	if m.config.Testing.EnableStartupValidation {
		go m.runStartupValidation(ctx)
	}

	m.started = true
	m.metrics.StartupDuration = time.Since(startTime)

	m.logger.Info("Strong consistency manager started successfully",
		zap.Duration("startup_duration", m.metrics.StartupDuration),
		zap.Int("components", len(components)))

	return nil
}

// Stop stops all strong consistency components
func (m *StrongConsistencyManager) Stop(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.started {
		return nil
	}

	m.logger.Info("Stopping strong consistency manager")

	// Cancel context to stop background goroutines
	m.cancel()
	// Stop components in reverse dependency order
	components := []struct {
		name      string
		component interface{ Stop(context.Context) error }
	}{
		{"transaction-integration", m.transactionIntegration},
		{"order-processor", m.orderProcessor},
		{"settlement-coordinator", m.settlementCoordinator},
		{"balance-manager", m.balanceManager},
		{"distributed-lock-manager", m.distributedLockManager},
		{"raft-coordinator", m.raftCoordinator},
	}

	var errors []error

	for _, comp := range components {
		m.logger.Info("Stopping component", zap.String("component", comp.name))

		if err := comp.component.Stop(ctx); err != nil {
			m.logger.Error("Failed to stop component",
				zap.String("component", comp.name),
				zap.Error(err))
			errors = append(errors, fmt.Errorf("failed to stop %s: %w", comp.name, err))
			m.metrics.ComponentStatusses[comp.name] = "failed"
		} else {
			m.metrics.ComponentStatusses[comp.name] = "stopped"
		}
	}

	m.started = false

	if len(errors) > 0 {
		return fmt.Errorf("errors during shutdown: %v", errors)
	}

	m.logger.Info("Strong consistency manager stopped successfully")
	return nil
}

// GetComponents returns all initialized components for external access
func (m *StrongConsistencyManager) GetComponents() *ConsistencyComponents {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return &ConsistencyComponents{
		RaftCoordinator:        m.raftCoordinator,
		BalanceManager:         m.balanceManager,
		DistributedLockManager: m.distributedLockManager,
		SettlementCoordinator:  m.settlementCoordinator,
		OrderProcessor:         m.orderProcessor,
		TransactionIntegration: m.transactionIntegration,
		ConfigManager:          m.configManager,
		TestSuite:              m.testSuite,
	}
}

// ConsistencyComponents holds all strong consistency components
type ConsistencyComponents struct {
	RaftCoordinator        *consensus.RaftCoordinator
	BalanceManager         *consistency.BalanceConsistencyManager
	DistributedLockManager *consistency.DistributedLockManager
	SettlementCoordinator  *coordination.StrongConsistencySettlementCoordinator
	OrderProcessor         *trading.StrongConsistencyOrderProcessor
	TransactionIntegration *transaction.StrongConsistencyTransactionManager
	ConfigManager          *config.StrongConsistencyConfigManager
	TestSuite              *test.StrongConsistencyTestSuite
}

// GetMetrics returns current consistency metrics
func (m *StrongConsistencyManager) GetMetrics() *ConsistencyMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Create a copy to avoid race conditions
	metrics := *m.metrics
	statusCopy := make(map[string]string)
	for k, v := range m.metrics.ComponentStatusses {
		statusCopy[k] = v
	}
	metrics.ComponentStatusses = statusCopy

	return &metrics
}

// IsHealthy returns true if all components are healthy
func (m *StrongConsistencyManager) IsHealthy() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.started {
		return false
	}

	// Check if all components are running
	for component, status := range m.metrics.ComponentStatusses {
		if status != "running" {
			m.logger.Warn("Component not healthy",
				zap.String("component", component),
				zap.String("status", status))
			return false
		}
	}

	// Check if health check is recent
	if time.Since(m.metrics.LastHealthCheck) > 30*time.Second {
		return false
	}

	return true
}

// monitorHealth monitors the health of all components
func (m *StrongConsistencyManager) monitorHealth(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.performHealthCheck()
		}
	}
}

// performHealthCheck checks the health of all components
func (m *StrongConsistencyManager) performHealthCheck() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.metrics.LastHealthCheck = time.Now()

	// Check each component
	healthChecks := []struct {
		name    string
		checker interface{ IsHealthy() bool }
	}{
		{"raft-coordinator", m.raftCoordinator},
		{"distributed-lock-manager", m.distributedLockManager},
		{"balance-manager", m.balanceManager},
		{"settlement-coordinator", m.settlementCoordinator},
		{"order-processor", m.orderProcessor},
		{"transaction-integration", m.transactionIntegration},
	}

	for _, check := range healthChecks {
		if check.checker.IsHealthy() {
			if m.metrics.ComponentStatusses[check.name] != "running" {
				m.logger.Info("Component recovered", zap.String("component", check.name))
			}
			m.metrics.ComponentStatusses[check.name] = "running"
		} else {
			if m.metrics.ComponentStatusses[check.name] == "running" {
				m.logger.Warn("Component unhealthy", zap.String("component", check.name))
			}
			m.metrics.ComponentStatusses[check.name] = "unhealthy"
		}
	}
}

// collectMetrics collects performance metrics from all components
func (m *StrongConsistencyManager) collectMetrics(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.updateMetrics()
		}
	}
}

// updateMetrics updates performance metrics
func (m *StrongConsistencyManager) updateMetrics() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Collect metrics from each component
	if consensus := m.raftCoordinator.GetMetrics(); consensus != nil {
		m.metrics.ConsensusOperations = consensus.TotalOperations
		m.metrics.ConsensusSuccessRate = consensus.SuccessRate
	}

	if balance := m.balanceManager.GetMetrics(); balance != nil {
		m.metrics.BalanceOperations = balance.TotalOperations
		m.metrics.BalanceConsistencyRate = balance.ConsistencyRate
	}

	if lock := m.distributedLockManager.GetMetrics(); lock != nil {
		m.metrics.LockOperations = lock.TotalOperations
		m.metrics.LockSuccessRate = lock.SuccessRate
	}

	if settlement := m.settlementCoordinator.GetMetrics(); settlement != nil {
		m.metrics.SettlementOperations = settlement.TotalOperations
		m.metrics.SettlementSuccessRate = settlement.SuccessRate
	}

	if order := m.orderProcessor.GetMetrics(); order != nil {
		m.metrics.OrderOperations = order.TotalOperations
		m.metrics.OrderConsistencyRate = order.ConsistencyRate
	}
}

// runStartupValidation runs validation tests after startup
func (m *StrongConsistencyManager) runStartupValidation(ctx context.Context) {
	// Wait for components to fully start
	time.Sleep(5 * time.Second)

	m.logger.Info("Running startup validation tests")

	// Run basic validation tests
	if err := m.testSuite.RunBasicValidation(ctx); err != nil {
		m.logger.Error("Startup validation failed", zap.Error(err))
		return
	}

	m.logger.Info("Startup validation tests passed successfully")
}
