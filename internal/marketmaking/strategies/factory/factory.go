// Strategy Factory for MarketMaking module - creates and manages strategies
package factory

import (
	"fmt"
	"sync"

	"github.com/Orbit-CEX/Finalex/internal/marketmaking/strategies/advanced"
	"github.com/Orbit-CEX/Finalex/internal/marketmaking/strategies/basic"
	"github.com/Orbit-CEX/Finalex/internal/marketmaking/strategies/common"
)

// StrategyFactory creates and manages trading strategies
type StrategyFactory struct {
	mu                  sync.RWMutex
	availableStrategies map[string]StrategyCreator
	metadata            map[string]common.StrategyInfo
}

// StrategyCreator is a function that creates a strategy instance
type StrategyCreator func(config common.StrategyConfig) (common.MarketMakingStrategy, error)

// NewStrategyFactory creates a new strategy factory with all built-in strategies
func NewStrategyFactory() *StrategyFactory {
	factory := &StrategyFactory{
		availableStrategies: make(map[string]StrategyCreator),
		metadata:            make(map[string]common.StrategyInfo),
	}

	// Register all built-in strategies
	factory.registerBuiltinStrategies()

	return factory
}

// CreateStrategy creates a new strategy instance by name
func (f *StrategyFactory) CreateStrategy(name string, config common.StrategyConfig) (common.MarketMakingStrategy, error) {
	f.mu.RLock()
	creator, exists := f.availableStrategies[name]
	f.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("strategy '%s' not found", name)
	}

	return creator(config)
}

// GetAvailableStrategies returns a list of all available strategy names
func (f *StrategyFactory) GetAvailableStrategies() []string {
	f.mu.RLock()
	defer f.mu.RUnlock()

	strategies := make([]string, 0, len(f.availableStrategies))
	for name := range f.availableStrategies {
		strategies = append(strategies, name)
	}

	return strategies
}

// GetStrategyInfo returns metadata for a specific strategy
func (f *StrategyFactory) GetStrategyInfo(name string) (common.StrategyInfo, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	info, exists := f.metadata[name]
	if !exists {
		return common.StrategyInfo{}, fmt.Errorf("strategy '%s' not found", name)
	}

	return info, nil
}

// GetAllStrategyInfo returns metadata for all available strategies
func (f *StrategyFactory) GetAllStrategyInfo() map[string]common.StrategyInfo {
	f.mu.RLock()
	defer f.mu.RUnlock()

	result := make(map[string]common.StrategyInfo)
	for name, info := range f.metadata {
		result[name] = info
	}

	return result
}

// RegisterStrategy registers a custom strategy with the factory
func (f *StrategyFactory) RegisterStrategy(name string, creator StrategyCreator, info common.StrategyInfo) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if _, exists := f.availableStrategies[name]; exists {
		return fmt.Errorf("strategy '%s' already exists", name)
	}

	f.availableStrategies[name] = creator
	f.metadata[name] = info

	return nil
}

// UnregisterStrategy removes a strategy from the factory
func (f *StrategyFactory) UnregisterStrategy(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if _, exists := f.availableStrategies[name]; !exists {
		return fmt.Errorf("strategy '%s' not found", name)
	}

	delete(f.availableStrategies, name)
	delete(f.metadata, name)

	return nil
}

// registerBuiltinStrategies registers all built-in strategies
func (f *StrategyFactory) registerBuiltinStrategies() {
	// Basic strategies
	f.registerBasicStrategy()
	f.registerDynamicStrategy()
	f.registerInventorySkewStrategy()

	// Advanced strategies
	f.registerPredictiveStrategy()
	f.registerVolatilitySurfaceStrategy()
	f.registerMicroStructureStrategy()
	f.registerCrossExchangeStrategy()
}

// registerBasicStrategy registers the basic strategy
func (f *StrategyFactory) registerBasicStrategy() {
	f.availableStrategies["basic"] = func(config common.StrategyConfig) (common.MarketMakingStrategy, error) {
		return basic.NewBasicStrategy(config)
	}

	f.metadata["basic"] = common.StrategyInfo{
		Name:        "Basic Strategy",
		Description: "Simple fixed spread market making strategy",
		RiskLevel:   common.RiskLow,
		Complexity:  common.ComplexityLow,
		Version:     "1.0.0",
		Parameters: []common.ParameterDefinition{
			{
				Name:        "spread",
				Type:        "float",
				Default:     0.001,
				MinValue:    0.0001,
				MaxValue:    0.01,
				Description: "Fixed spread percentage",
				Required:    true,
			},
			{
				Name:        "size",
				Type:        "float",
				Default:     100.0,
				MinValue:    1.0,
				MaxValue:    10000.0,
				Description: "Order size",
				Required:    true,
			},
		},
	}
}

// registerDynamicStrategy registers the dynamic strategy
func (f *StrategyFactory) registerDynamicStrategy() {
	f.availableStrategies["dynamic"] = func(config common.StrategyConfig) (common.MarketMakingStrategy, error) {
		return basic.NewDynamicStrategy(config)
	}

	f.metadata["dynamic"] = common.StrategyInfo{
		Name:        "Dynamic Strategy",
		Description: "Volatility-adjusted market making strategy",
		RiskLevel:   common.RiskMedium,
		Complexity:  common.ComplexityMedium,
		Version:     "1.0.0",
		Parameters: []common.ParameterDefinition{
			{
				Name:        "base_spread",
				Type:        "float",
				Default:     0.001,
				MinValue:    0.0001,
				MaxValue:    0.01,
				Description: "Base spread percentage",
				Required:    true,
			},
			{
				Name:        "volatility_factor",
				Type:        "float",
				Default:     1.0,
				MinValue:    0.1,
				MaxValue:    10.0,
				Description: "Volatility adjustment factor",
				Required:    true,
			},
			{
				Name:        "size",
				Type:        "float",
				Default:     100.0,
				MinValue:    1.0,
				MaxValue:    10000.0,
				Description: "Order size",
				Required:    true,
			},
		},
	}
}

// registerInventorySkewStrategy registers the inventory skew strategy
func (f *StrategyFactory) registerInventorySkewStrategy() {
	f.availableStrategies["inventory_skew"] = func(config common.StrategyConfig) (common.MarketMakingStrategy, error) {
		return basic.NewInventorySkewStrategy(config)
	}

	f.metadata["inventory_skew"] = common.StrategyInfo{
		Name:        "Inventory Skew Strategy",
		Description: "Inventory-aware market making with position skewing",
		RiskLevel:   common.RiskMedium,
		Complexity:  common.ComplexityMedium,
		Version:     "1.0.0",
		Parameters: []common.ParameterDefinition{
			{
				Name:        "base_spread",
				Type:        "float",
				Default:     0.001,
				MinValue:    0.0001,
				MaxValue:    0.01,
				Description: "Base spread percentage",
				Required:    true,
			},
			{
				Name:        "inventory_factor",
				Type:        "float",
				Default:     0.001,
				MinValue:    0.0001,
				MaxValue:    0.01,
				Description: "Inventory skew factor",
				Required:    true,
			},
			{
				Name:        "size",
				Type:        "float",
				Default:     100.0,
				MinValue:    1.0,
				MaxValue:    10000.0,
				Description: "Order size",
				Required:    true,
			},
			{
				Name:        "max_inventory",
				Type:        "float",
				Default:     1000.0,
				MinValue:    100.0,
				MaxValue:    100000.0,
				Description: "Maximum inventory position",
				Required:    true,
			},
		},
	}
}

// registerPredictiveStrategy registers the predictive strategy
func (f *StrategyFactory) registerPredictiveStrategy() {
	f.availableStrategies["predictive"] = func(config common.StrategyConfig) (common.MarketMakingStrategy, error) {
		return advanced.NewPredictiveStrategy(config)
	}

	f.metadata["predictive"] = common.StrategyInfo{
		Name:        "Predictive Strategy",
		Description: "ML-inspired predictive market making with advanced analytics",
		RiskLevel:   common.RiskHigh,
		Complexity:  common.ComplexityHigh,
		Version:     "1.0.0",
		Parameters: []common.ParameterDefinition{
			{
				Name:        "base_spread",
				Type:        "float",
				Default:     0.001,
				MinValue:    0.0001,
				MaxValue:    0.01,
				Description: "Base spread percentage",
				Required:    true,
			},
			{
				Name:        "prediction_horizon_ms",
				Type:        "int",
				Default:     100,
				MinValue:    10,
				MaxValue:    5000,
				Description: "Prediction horizon in milliseconds",
				Required:    true,
			},
			{
				Name:        "volatility_decay",
				Type:        "float",
				Default:     0.95,
				MinValue:    0.5,
				MaxValue:    0.999,
				Description: "Volatility decay factor",
				Required:    true,
			},
			{
				Name:        "momentum_factor",
				Type:        "float",
				Default:     0.3,
				MinValue:    0.0,
				MaxValue:    1.0,
				Description: "Momentum weight factor",
				Required:    true,
			},
			{
				Name:        "mean_reversion_rate",
				Type:        "float",
				Default:     0.1,
				MinValue:    0.0,
				MaxValue:    1.0,
				Description: "Mean reversion rate",
				Required:    true,
			},
			{
				Name:        "max_position",
				Type:        "float",
				Default:     1000.0,
				MinValue:    100.0,
				MaxValue:    100000.0,
				Description: "Maximum position size",
				Required:    true,
			},
		},
	}
}
