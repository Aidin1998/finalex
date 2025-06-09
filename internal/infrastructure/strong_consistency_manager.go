package infrastructure

import (
	"context"
	"sync"
	"time"

	"github.com/Aidin1998/finalex/internal/infrastructure/config"
	"go.uber.org/zap"
)

// ConsensusEngine defines the interface for consensus operations
// (e.g., leader election, commit, quorum, etc.)
type ConsensusEngine interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Propose(ctx context.Context, data interface{}) error
	IsLeader() bool
	GetLeaderID() string
	GetNodeID() string
	GetClusterNodes() []string
}

// DistributedLockManager defines the interface for distributed locks
// (e.g., for critical sections, settlements, etc.)
type DistributedLockManager interface {
	AcquireLock(ctx context.Context, key string, timeout time.Duration) (bool, error)
	ReleaseLock(ctx context.Context, key string) error
	RenewLock(ctx context.Context, key string, renewal time.Duration) error
}

// StrongConsistencyManager coordinates consensus and distributed locks
// and provides a unified interface for strong consistency operations
// across the platform.
type StrongConsistencyManager struct {
	logger        *zap.Logger
	configManager *config.StrongConsistencyConfigManager
	consensus     ConsensusEngine
	dlockManager  DistributedLockManager
	mutex         sync.RWMutex
	started       bool
}

// NewStrongConsistencyManager creates a new manager with config and logger
func NewStrongConsistencyManager(cfgPath string, logger *zap.Logger, consensus ConsensusEngine, dlock DistributedLockManager) *StrongConsistencyManager {
	cfgMgr := config.NewStrongConsistencyConfigManager(cfgPath, logger)
	return &StrongConsistencyManager{
		logger:        logger.Named("strong-consistency-manager"),
		configManager: cfgMgr,
		consensus:     consensus,
		dlockManager:  dlock,
	}
}

// Start launches consensus and lock managers
func (m *StrongConsistencyManager) Start(ctx context.Context) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if m.started {
		return nil
	}
	if err := m.configManager.LoadConfig(); err != nil {
		m.logger.Warn("Failed to load strong consistency config", zap.Error(err))
	}
	if m.consensus != nil {
		if err := m.consensus.Start(ctx); err != nil {
			return err
		}
	}
	m.started = true
	m.logger.Info("Strong consistency manager started")
	return nil
}

// Stop halts consensus and lock managers
func (m *StrongConsistencyManager) Stop(ctx context.Context) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if !m.started {
		return nil
	}
	if m.consensus != nil {
		if err := m.consensus.Stop(ctx); err != nil {
			m.logger.Warn("Failed to stop consensus engine", zap.Error(err))
		}
	}
	m.started = false
	m.logger.Info("Strong consistency manager stopped")
	return nil
}

// Propose submits data to the consensus engine
func (m *StrongConsistencyManager) Propose(ctx context.Context, data interface{}) error {
	if m.consensus == nil {
		return nil
	}
	return m.consensus.Propose(ctx, data)
}

// AcquireLock acquires a distributed lock
func (m *StrongConsistencyManager) AcquireLock(ctx context.Context, key string, timeout time.Duration) (bool, error) {
	if m.dlockManager == nil {
		return false, nil
	}
	return m.dlockManager.AcquireLock(ctx, key, timeout)
}

// ReleaseLock releases a distributed lock
func (m *StrongConsistencyManager) ReleaseLock(ctx context.Context, key string) error {
	if m.dlockManager == nil {
		return nil
	}
	return m.dlockManager.ReleaseLock(ctx, key)
}

// HealthCheck returns the health of the strong consistency subsystem
func (m *StrongConsistencyManager) HealthCheck(ctx context.Context) error {
	if err := m.configManager.LoadConfig(); err != nil {
		return err
	}
	// Optionally check consensus and lock health
	return nil
}

// GetConfig returns the current strong consistency config
func (m *StrongConsistencyManager) GetConfig() map[string]interface{} {
	return m.configManager.GetConfig()
}
