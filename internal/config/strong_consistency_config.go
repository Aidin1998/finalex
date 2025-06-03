package config

import (
	"go.uber.org/zap"
)

// Type alias for the main interface - references the simple implementation
type StrongConsistencyConfigManager = SimpleStrongConsistencyConfigManager

// NewStrongConsistencyConfigManager creates a new config manager (alias for the simple version)
func NewStrongConsistencyConfigManager(configPath string, logger *zap.Logger) *StrongConsistencyConfigManager {
	return NewSimpleStrongConsistencyConfigManager(configPath, logger)
}
