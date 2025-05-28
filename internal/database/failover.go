package database

import (
	"context"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// DBFailoverManager monitors primary and standby databases and switches on failure
type DBFailoverManager struct {
	primary  *gorm.DB
	standby  *gorm.DB
	current  *gorm.DB
	logger   *zap.Logger
	interval time.Duration
}

// NewFailoverManager creates a new failover manager
func NewFailoverManager(primary, standby *gorm.DB, logger *zap.Logger, interval time.Duration) *DBFailoverManager {
	return &DBFailoverManager{
		primary:  primary,
		standby:  standby,
		current:  primary,
		logger:   logger,
		interval: interval,
	}
}

// Start begins the monitoring loop for automatic failover
func (m *DBFailoverManager) Start(ctx context.Context) {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Ping primary
			db, err := m.primary.DB()
			if err != nil {
				m.logger.Warn("Failed to get primary DB connection", zap.Error(err))
				continue
			}
			if err := db.Ping(); err != nil {
				m.logger.Warn("Primary DB down, switching to standby", zap.Error(err))
				m.current = m.standby
			} else {
				if m.current != m.primary {
					m.logger.Info("Primary DB recovered, switching back to primary")
					m.current = m.primary
				}
			}
		case <-ctx.Done():
			m.logger.Info("Stopping DB failover manager")
			return
		}
	}
}

// DB returns the current healthy database connection
func (m *DBFailoverManager) DB() *gorm.DB {
	return m.current
}
