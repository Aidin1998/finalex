package infrastructure

import (
	"github.com/Aidin1998/finalex/internal/infrastructure/config"
)

// StrongConsistencyConfigManager is an alias for the config module's implementation.
type StrongConsistencyConfigManager = config.StrongConsistencyConfigManager

// NewStrongConsistencyConfigManager creates a new config manager (delegates to config module).
var NewStrongConsistencyConfigManager = config.NewStrongConsistencyConfigManager
