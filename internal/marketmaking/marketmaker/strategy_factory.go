// Strategy Factory for MarketMaker - provides strategy creation and management
package marketmaker

import (
	"fmt"
	"math"
	"time"
)

// StrategyFactory creates and manages trading strategies for backtesting and live trading
type StrategyFactory struct {
	availableStrategies map[string]StrategyCreator
	defaultParams       map[string]map[string]interface{}
}

// StrategyCreator is a function that creates a strategy instance
type StrategyCreator func(params map[string]interface{}) (Strategy, error)

// StrategyMetadata contains information about a strategy
type StrategyMetadata struct {
	Name        string                `json:"name"`
	Description string                `json:"description"`
	Parameters  []ParameterDefinition `json:"parameters"`
	RiskLevel   string                `json:"risk_level"`
	Complexity  string                `json:"complexity"`
	Version     string                `json:"version"`
}

// FactoryParameterDefinition defines a strategy parameter
// Renamed to avoid conflict with admin_tools.go
type FactoryParameterDefinition struct {
	Name        string      `json:"name"`
	Type        string      `json:"type"`
	Default     interface{} `json:"default"`
	MinValue    interface{} `json:"min_value,omitempty"`
	MaxValue    interface{} `json:"max_value,omitempty"`
	Description string      `json:"description"`
	Required    bool        `json:"required"`
}

// NewStrategyFactory creates a new strategy factory
func NewStrategyFactory() *StrategyFactory {
	factory := &StrategyFactory{
		availableStrategies: make(map[string]StrategyCreator),
		defaultParams:       make(map[string]map[string]interface{}),
	}

	// Register built-in strategies
	factory.registerBuiltinStrategies()

	return factory
}

// registerBuiltinStrategies registers all built-in trading strategies
func (sf *StrategyFactory) registerBuiltinStrategies() {
	// Basic Strategy
	sf.RegisterStrategy("basic", sf.createBasicStrategy, map[string]interface{}{
		"spread":    0.001,
		"size":      100.0,
		"tick_size": 0.01,
	})

	// Dynamic Strategy
	sf.RegisterStrategy("dynamic", sf.createDynamicStrategy, map[string]interface{}{
		"base_spread":     0.001,
		"vol_factor":      0.5,
		"size":            100.0,
		"lookback_period": 20,
	})

	// Inventory Skew Strategy
	sf.RegisterStrategy("inventory-skew", sf.createInventorySkewStrategy, map[string]interface{}{
		"base_spread": 0.001,
		"inv_factor":  0.001,
		"size":        100.0,
		"max_skew":    0.5,
	})

	// Predictive Strategy
	sf.RegisterStrategy("predictive", sf.createPredictiveStrategy, map[string]interface{}{
		"base_spread":      0.001,
		"max_inventory":    1000.0,
		"prediction_alpha": 0.1,
		"lookback_window":  50,
	})

	// Volatility Surface Strategy
	sf.RegisterStrategy("vol-surface", sf.createVolSurfaceStrategy, map[string]interface{}{
		"base_spread":    0.001,
		"max_inventory":  1000.0,
		"vol_scaling":    2.0,
		"surface_window": 100,
	})

	// Microstructure Strategy
	sf.RegisterStrategy("micro-structure", sf.createMicroStructureStrategy, map[string]interface{}{
		"base_spread":      0.001,
		"max_inventory":    1000.0,
		"tick_size":        0.01,
		"order_flow_alpha": 0.2,
	})

	// Cross-Exchange Arbitrage Strategy
	sf.RegisterStrategy("cross-exchange", sf.createCrossExchangeStrategy, map[string]interface{}{
		"base_spread":       0.001,
		"max_inventory":     1000.0,
		"arb_threshold":     0.001,
		"execution_timeout": 1000,
	})

	// Mean Reversion Strategy
	sf.RegisterStrategy("mean-reversion", sf.createMeanReversionStrategy, map[string]interface{}{
		"base_spread":     0.001,
		"reversion_speed": 0.1,
		"lookback_period": 30,
		"size":            100.0,
	})

	// Momentum Strategy
	sf.RegisterStrategy("momentum", sf.createMomentumStrategy, map[string]interface{}{
		"base_spread":     0.001,
		"momentum_factor": 0.2,
		"trend_window":    20,
		"size":            100.0,
	})
}

// RegisterStrategy registers a new strategy with the factory
func (sf *StrategyFactory) RegisterStrategy(name string, creator StrategyCreator, defaultParams map[string]interface{}) {
	sf.availableStrategies[name] = creator
	sf.defaultParams[name] = defaultParams
}

// CreateStrategy creates a strategy instance by name with optional parameters
func (sf *StrategyFactory) CreateStrategy(name string, params map[string]interface{}) (Strategy, error) {
	creator, exists := sf.availableStrategies[name]
	if !exists {
		return nil, fmt.Errorf("strategy '%s' not found", name)
	}

	// Merge with default parameters
	mergedParams := sf.mergeParams(name, params)

	return creator(mergedParams)
}

// GetAvailableStrategies returns a list of available strategies
func (sf *StrategyFactory) GetAvailableStrategies() []string {
	strategies := make([]string, 0, len(sf.availableStrategies))
	for name := range sf.availableStrategies {
		strategies = append(strategies, name)
	}
	return strategies
}

// GetStrategyMetadata returns metadata for a strategy
func (sf *StrategyFactory) GetStrategyMetadata(name string) (*StrategyMetadata, error) {
	if _, exists := sf.availableStrategies[name]; !exists {
		return nil, fmt.Errorf("strategy '%s' not found", name)
	}

	// Return metadata based on strategy type
	switch name {
	case "basic":
		return &StrategyMetadata{
			Name:        "Basic Market Making",
			Description: "Simple market making with fixed spreads",
			RiskLevel:   "LOW",
			Complexity:  "SIMPLE",
			Version:     "1.0",
			Parameters: []ParameterDefinition{
				{Name: "spread", Type: "float", Default: 0.001, MinValue: 0.0001, MaxValue: 0.01, Description: "Fixed spread for quotes", Required: true},
				{Name: "size", Type: "float", Default: 100.0, MinValue: 1.0, MaxValue: 10000.0, Description: "Order size", Required: true},
				{Name: "tick_size", Type: "float", Default: 0.01, MinValue: 0.001, MaxValue: 1.0, Description: "Minimum price increment", Required: false},
			},
		}, nil
	case "dynamic":
		return &StrategyMetadata{
			Name:        "Dynamic Market Making",
			Description: "Market making with volatility-adjusted spreads",
			RiskLevel:   "MEDIUM",
			Complexity:  "INTERMEDIATE",
			Version:     "1.0",
			Parameters: []ParameterDefinition{
				{Name: "base_spread", Type: "float", Default: 0.001, MinValue: 0.0001, MaxValue: 0.01, Description: "Base spread before adjustments", Required: true},
				{Name: "vol_factor", Type: "float", Default: 0.5, MinValue: 0.1, MaxValue: 2.0, Description: "Volatility scaling factor", Required: true},
				{Name: "size", Type: "float", Default: 100.0, MinValue: 1.0, MaxValue: 10000.0, Description: "Order size", Required: true},
				{Name: "lookback_period", Type: "int", Default: 20, MinValue: 5, MaxValue: 100, Description: "Volatility calculation window", Required: false},
			},
		}, nil
	case "inventory-skew":
		return &StrategyMetadata{
			Name:        "Inventory Skew Strategy",
			Description: "Market making with inventory-based spread skewing",
			RiskLevel:   "MEDIUM",
			Complexity:  "INTERMEDIATE",
			Version:     "1.0",
			Parameters: []ParameterDefinition{
				{Name: "base_spread", Type: "float", Default: 0.001, MinValue: 0.0001, MaxValue: 0.01, Description: "Base spread before skewing", Required: true},
				{Name: "inv_factor", Type: "float", Default: 0.001, MinValue: 0.0001, MaxValue: 0.01, Description: "Inventory skewing factor", Required: true},
				{Name: "size", Type: "float", Default: 100.0, MinValue: 1.0, MaxValue: 10000.0, Description: "Order size", Required: true},
				{Name: "max_skew", Type: "float", Default: 0.5, MinValue: 0.1, MaxValue: 1.0, Description: "Maximum skew ratio", Required: false},
			},
		}, nil
	case "predictive":
		return &StrategyMetadata{
			Name:        "Predictive Market Making",
			Description: "Advanced strategy using price prediction models",
			RiskLevel:   "HIGH",
			Complexity:  "ADVANCED",
			Version:     "1.0",
			Parameters: []ParameterDefinition{
				{Name: "base_spread", Type: "float", Default: 0.001, MinValue: 0.0001, MaxValue: 0.01, Description: "Base spread", Required: true},
				{Name: "max_inventory", Type: "float", Default: 1000.0, MinValue: 100.0, MaxValue: 100000.0, Description: "Maximum inventory", Required: true},
				{Name: "prediction_alpha", Type: "float", Default: 0.1, MinValue: 0.01, MaxValue: 0.5, Description: "Prediction weight factor", Required: false},
				{Name: "lookback_window", Type: "int", Default: 50, MinValue: 20, MaxValue: 200, Description: "Historical data window", Required: false},
			},
		}, nil
	default:
		return &StrategyMetadata{
			Name:        name,
			Description: "Strategy description not available",
			RiskLevel:   "UNKNOWN",
			Complexity:  "UNKNOWN",
			Version:     "1.0",
			Parameters:  []ParameterDefinition{},
		}, nil
	}
}

// mergeParams merges user parameters with defaults
func (sf *StrategyFactory) mergeParams(strategyName string, userParams map[string]interface{}) map[string]interface{} {
	merged := make(map[string]interface{})

	// Start with defaults
	if defaults, exists := sf.defaultParams[strategyName]; exists {
		for k, v := range defaults {
			merged[k] = v
		}
	}

	// Override with user parameters
	if userParams != nil {
		for k, v := range userParams {
			merged[k] = v
		}
	}

	return merged
}

// Strategy creation functions
func (sf *StrategyFactory) createBasicStrategy(params map[string]interface{}) (Strategy, error) {
	spread, _ := params["spread"].(float64)
	size, _ := params["size"].(float64)

	return &BasicStrategy{
		Spread: spread,
		Size:   size,
	}, nil
}

func (sf *StrategyFactory) createDynamicStrategy(params map[string]interface{}) (Strategy, error) {
	baseSpread, _ := params["base_spread"].(float64)
	volFactor, _ := params["vol_factor"].(float64)
	size, _ := params["size"].(float64)

	return &DynamicStrategy{
		BaseSpread: baseSpread,
		VolFactor:  volFactor,
		Size:       size,
	}, nil
}

func (sf *StrategyFactory) createInventorySkewStrategy(params map[string]interface{}) (Strategy, error) {
	baseSpread, _ := params["base_spread"].(float64)
	invFactor, _ := params["inv_factor"].(float64)
	size, _ := params["size"].(float64)

	return &InventorySkewStrategy{
		BaseSpread: baseSpread,
		InvFactor:  invFactor,
		Size:       size,
	}, nil
}

func (sf *StrategyFactory) createPredictiveStrategy(params map[string]interface{}) (Strategy, error) {
	baseSpread, _ := params["base_spread"].(float64)
	maxInventory, _ := params["max_inventory"].(float64)

	return NewPredictiveStrategy(baseSpread, maxInventory), nil
}

func (sf *StrategyFactory) createVolSurfaceStrategy(params map[string]interface{}) (Strategy, error) {
	baseSpread, _ := params["base_spread"].(float64)
	maxInventory, _ := params["max_inventory"].(float64)

	return NewVolatilitySurfaceStrategy(baseSpread, maxInventory), nil
}

func (sf *StrategyFactory) createMicroStructureStrategy(params map[string]interface{}) (Strategy, error) {
	baseSpread, _ := params["base_spread"].(float64)
	maxInventory, _ := params["max_inventory"].(float64)
	tickSize, _ := params["tick_size"].(float64)

	return NewMicroStructureStrategy(baseSpread, maxInventory, tickSize), nil
}

func (sf *StrategyFactory) createCrossExchangeStrategy(params map[string]interface{}) (Strategy, error) {
	baseSpread, _ := params["base_spread"].(float64)
	maxInventory, _ := params["max_inventory"].(float64)
	arbThreshold, _ := params["arb_threshold"].(float64)

	return NewCrossExchangeArbitrageStrategy(baseSpread, maxInventory, arbThreshold), nil
}

func (sf *StrategyFactory) createMeanReversionStrategy(params map[string]interface{}) (Strategy, error) {
	baseSpread, _ := params["base_spread"].(float64)
	reversionSpeed, _ := params["reversion_speed"].(float64)
	lookbackPeriod, _ := params["lookback_period"].(int)
	size, _ := params["size"].(float64)

	return &MeanReversionStrategy{
		BaseSpread:     baseSpread,
		ReversionSpeed: reversionSpeed,
		LookbackPeriod: lookbackPeriod,
		Size:           size,
		priceHistory:   make([]float64, 0, lookbackPeriod),
	}, nil
}

func (sf *StrategyFactory) createMomentumStrategy(params map[string]interface{}) (Strategy, error) {
	baseSpread, _ := params["base_spread"].(float64)
	momentumFactor, _ := params["momentum_factor"].(float64)
	trendWindow, _ := params["trend_window"].(int)
	size, _ := params["size"].(float64)

	return &MomentumStrategy{
		BaseSpread:     baseSpread,
		MomentumFactor: momentumFactor,
		TrendWindow:    trendWindow,
		Size:           size,
		priceHistory:   make([]float64, 0, trendWindow),
	}, nil
}

// Additional strategy implementations
type MeanReversionStrategy struct {
	BaseSpread     float64
	ReversionSpeed float64
	LookbackPeriod int
	Size           float64
	priceHistory   []float64
	lastUpdate     time.Time
}

func (s *MeanReversionStrategy) Quote(mid, volatility, inventory float64) (bid, ask, size float64) {
	// Update price history
	s.priceHistory = append(s.priceHistory, mid)
	if len(s.priceHistory) > s.LookbackPeriod {
		s.priceHistory = s.priceHistory[1:]
	}

	// Calculate mean price
	if len(s.priceHistory) < 2 {
		// Not enough data, use basic spread
		halfSpread := s.BaseSpread / 2
		return mid - halfSpread, mid + halfSpread, s.Size
	}

	// Calculate mean and deviation
	sum := 0.0
	for _, price := range s.priceHistory {
		sum += price
	}
	mean := sum / float64(len(s.priceHistory))

	// Mean reversion signal
	deviation := (mid - mean) / mean
	adjustment := deviation * s.ReversionSpeed

	// Adjust spread based on mean reversion
	spreadAdjustment := s.BaseSpread * (1.0 + adjustment)
	if spreadAdjustment < s.BaseSpread*0.5 {
		spreadAdjustment = s.BaseSpread * 0.5
	}
	if spreadAdjustment > s.BaseSpread*2.0 {
		spreadAdjustment = s.BaseSpread * 2.0
	}

	halfSpread := spreadAdjustment / 2

	// Inventory adjustment
	invAdjustment := inventory * 0.001

	return mid - halfSpread - invAdjustment, mid + halfSpread + invAdjustment, s.Size
}

type MomentumStrategy struct {
	BaseSpread     float64
	MomentumFactor float64
	TrendWindow    int
	Size           float64
	priceHistory   []float64
	lastUpdate     time.Time
}

func (s *MomentumStrategy) Quote(mid, volatility, inventory float64) (bid, ask, size float64) {
	// Update price history
	s.priceHistory = append(s.priceHistory, mid)
	if len(s.priceHistory) > s.TrendWindow {
		s.priceHistory = s.priceHistory[1:]
	}

	// Calculate momentum
	momentum := 0.0
	if len(s.priceHistory) >= 2 {
		recent := s.priceHistory[len(s.priceHistory)-1]
		older := s.priceHistory[0]
		momentum = (recent - older) / older
	}

	// Adjust spread based on momentum
	momentumAdjustment := momentum * s.MomentumFactor
	spreadAdjustment := s.BaseSpread * (1.0 + math.Abs(momentumAdjustment))

	halfSpread := spreadAdjustment / 2

	// Skew based on momentum direction
	skew := momentumAdjustment * 0.5

	// Inventory adjustment
	invAdjustment := inventory * 0.001

	return mid - halfSpread - invAdjustment + skew, mid + halfSpread + invAdjustment + skew, s.Size
}

// StrategyValidator validates strategy parameters
type StrategyValidator struct {
	factory *StrategyFactory
}

// NewStrategyValidator creates a new strategy validator
func NewStrategyValidator(factory *StrategyFactory) *StrategyValidator {
	return &StrategyValidator{factory: factory}
}

// ValidateStrategy validates strategy configuration
func (sv *StrategyValidator) ValidateStrategy(name string, params map[string]interface{}) error {
	metadata, err := sv.factory.GetStrategyMetadata(name)
	if err != nil {
		return err
	}

	// Validate required parameters
	for _, param := range metadata.Parameters {
		if param.Required {
			if _, exists := params[param.Name]; !exists {
				return fmt.Errorf("required parameter '%s' is missing", param.Name)
			}
		}

		// Validate parameter ranges
		if value, exists := params[param.Name]; exists {
			if err := sv.validateParameterRange(param, value); err != nil {
				return fmt.Errorf("parameter '%s': %w", param.Name, err)
			}
		}
	}

	return nil
}

// validateParameterRange validates that a parameter value is within acceptable range
func (sv *StrategyValidator) validateParameterRange(param ParameterDefinition, value interface{}) error {
	switch param.Type {
	case "float":
		floatVal, ok := value.(float64)
		if !ok {
			return fmt.Errorf("expected float64, got %T", value)
		}

		if param.MinValue != nil {
			if min, ok := param.MinValue.(float64); ok && floatVal < min {
				return fmt.Errorf("value %f is below minimum %f", floatVal, min)
			}
		}

		if param.MaxValue != nil {
			if max, ok := param.MaxValue.(float64); ok && floatVal > max {
				return fmt.Errorf("value %f is above maximum %f", floatVal, max)
			}
		}

	case "int":
		intVal, ok := value.(int)
		if !ok {
			// Try converting from float
			if floatVal, okFloat := value.(float64); okFloat {
				intVal = int(floatVal)
			} else {
				return fmt.Errorf("expected int, got %T", value)
			}
		}

		if param.MinValue != nil {
			if min, ok := param.MinValue.(int); ok && intVal < min {
				return fmt.Errorf("value %d is below minimum %d", intVal, min)
			}
		}

		if param.MaxValue != nil {
			if max, ok := param.MaxValue.(int); ok && intVal > max {
				return fmt.Errorf("value %d is above maximum %d", intVal, max)
			}
		}
	}

	return nil
}
