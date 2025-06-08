package engine

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// FeeEngine provides centralized fee calculation logic for the trading module
// This replaces all hardcoded fee calculations throughout the system
type FeeEngine struct {
	mu     sync.RWMutex
	logger *zap.Logger

	// Fee configuration
	config *FeeEngineConfig

	// Fee tiers and overrides
	feeTiers         map[string]*FeeTier         // fee tiers by tier name
	pairOverrides    map[string]*PairFeeConfig   // per-pair fee overrides
	accountDiscounts map[string]*AccountDiscount // account-level discounts

	// Cross-pair fee configuration
	crossPairConfig *CrossPairFeeConfig

	// Fee calculation metrics
	metrics *FeeEngineMetrics
}

// FeeEngineConfig holds the main fee engine configuration
type FeeEngineConfig struct {
	// Default fees
	DefaultMakerFee decimal.Decimal `yaml:"default_maker_fee"`
	DefaultTakerFee decimal.Decimal `yaml:"default_taker_fee"`

	// Cross-pair trading fees
	CrossPairFeeMultiplier decimal.Decimal `yaml:"cross_pair_fee_multiplier"`
	CrossPairMinFee        decimal.Decimal `yaml:"cross_pair_min_fee"`
	CrossPairMaxFee        decimal.Decimal `yaml:"cross_pair_max_fee"`

	// Fee calculation settings
	MinFeeAmount        decimal.Decimal `yaml:"min_fee_amount"`
	MaxFeeAmount        decimal.Decimal `yaml:"max_fee_amount"`
	FeeDisplayPrecision int32           `yaml:"fee_display_precision"`
}

// FeeTier represents a fee tier with maker/taker rates
type FeeTier struct {
	Name               string          `json:"name"`
	MakerFee           decimal.Decimal `json:"maker_fee"`
	TakerFee           decimal.Decimal `json:"taker_fee"`
	MonthlyVolumeMin   decimal.Decimal `json:"monthly_volume_min"`
	MonthlyVolumeMax   decimal.Decimal `json:"monthly_volume_max"`
	HoldingRequirement decimal.Decimal `json:"holding_requirement,omitempty"`
	Description        string          `json:"description"`
}

// PairFeeConfig represents per-pair fee overrides
type PairFeeConfig struct {
	Pair      string          `json:"pair"`
	MakerFee  decimal.Decimal `json:"maker_fee"`
	TakerFee  decimal.Decimal `json:"taker_fee"`
	Enabled   bool            `json:"enabled"`
	ValidFrom time.Time       `json:"valid_from"`
	ValidTo   *time.Time      `json:"valid_to,omitempty"`
}

// AccountDiscount represents account-level fee discounts
type AccountDiscount struct {
	UserID      string          `json:"user_id"`
	DiscountPct decimal.Decimal `json:"discount_pct"`
	ValidFrom   time.Time       `json:"valid_from"`
	ValidTo     *time.Time      `json:"valid_to,omitempty"`
	Description string          `json:"description"`
}

// CrossPairFeeConfig holds configuration for cross-pair trading fees
type CrossPairFeeConfig struct {
	Enabled              bool            `json:"enabled"`
	BaseFeeMultiplier    decimal.Decimal `json:"base_fee_multiplier"`
	RoutingFee           decimal.Decimal `json:"routing_fee"`
	LiquidityProviderFee decimal.Decimal `json:"liquidity_provider_fee"`
	MaxHops              int             `json:"max_hops"`
}

// FeeCalculationRequest represents a fee calculation request
type FeeCalculationRequest struct {
	UserID      string          `json:"user_id"`
	Pair        string          `json:"pair"`
	Side        string          `json:"side"`
	OrderType   string          `json:"order_type"`
	Quantity    decimal.Decimal `json:"quantity"`
	Price       decimal.Decimal `json:"price"`
	TradedValue decimal.Decimal `json:"traded_value"`
	IsMaker     bool            `json:"is_maker"`
	Timestamp   time.Time       `json:"timestamp"`
}

// CrossPairFeeRequest represents a cross-pair fee calculation request
type CrossPairFeeRequest struct {
	UserID            string               `json:"user_id"`
	SourcePair        string               `json:"source_pair"`
	TargetPair        string               `json:"target_pair"`
	IntermediatePairs []string             `json:"intermediate_pairs"`
	Quantity          decimal.Decimal      `json:"quantity"`
	EstimatedValue    decimal.Decimal      `json:"estimated_value"`
	Route             []CrossPairRouteStep `json:"route"`
	Timestamp         time.Time            `json:"timestamp"`
}

// CrossPairRouteStep represents a single step in a cross-pair trade route
type CrossPairRouteStep struct {
	Pair     string          `json:"pair"`
	Side     string          `json:"side"`
	Quantity decimal.Decimal `json:"quantity"`
	Price    decimal.Decimal `json:"price"`
	IsMaker  bool            `json:"is_maker"`
}

// FeeCalculationResult represents the result of a fee calculation
type FeeCalculationResult struct {
	BaseFee         decimal.Decimal            `json:"base_fee"`
	DiscountedFee   decimal.Decimal            `json:"discounted_fee"`
	FinalFee        decimal.Decimal            `json:"final_fee"`
	Currency        string                     `json:"currency"`
	FeeRate         decimal.Decimal            `json:"fee_rate"`
	AppliedDiscount decimal.Decimal            `json:"applied_discount"`
	FeeTier         string                     `json:"fee_tier"`
	Breakdown       map[string]decimal.Decimal `json:"breakdown"`
	CalculatedAt    time.Time                  `json:"calculated_at"`
}

// CrossPairFeeResult represents the result of cross-pair fee calculation
type CrossPairFeeResult struct {
	TotalFee          decimal.Decimal            `json:"total_fee"`
	StepFees          []FeeCalculationResult     `json:"step_fees"`
	RoutingFee        decimal.Decimal            `json:"routing_fee"`
	LiquidityFee      decimal.Decimal            `json:"liquidity_fee"`
	EstimatedSlippage decimal.Decimal            `json:"estimated_slippage"`
	Currency          string                     `json:"currency"`
	Breakdown         map[string]decimal.Decimal `json:"breakdown"`
	CalculatedAt      time.Time                  `json:"calculated_at"`
}

// FeeEngineMetrics tracks fee engine performance and usage
type FeeEngineMetrics struct {
	mu                    sync.RWMutex
	TotalCalculations     int64            `json:"total_calculations"`
	CrossPairCalculations int64            `json:"cross_pair_calculations"`
	AverageLatencyNs      int64            `json:"average_latency_ns"`
	FeeTierUsage          map[string]int64 `json:"fee_tier_usage"`
	PairOverrideUsage     map[string]int64 `json:"pair_override_usage"`
	DiscountApplications  int64            `json:"discount_applications"`
	ErrorCount            int64            `json:"error_count"`
	LastUpdated           time.Time        `json:"last_updated"`
}

// NewFeeEngine creates a new centralized fee engine
func NewFeeEngine(logger *zap.Logger, config *FeeEngineConfig) *FeeEngine {
	if config == nil {
		config = getDefaultFeeEngineConfig()
	}

	return &FeeEngine{
		logger:           logger,
		config:           config,
		feeTiers:         make(map[string]*FeeTier),
		pairOverrides:    make(map[string]*PairFeeConfig),
		accountDiscounts: make(map[string]*AccountDiscount),
		crossPairConfig:  getDefaultCrossPairConfig(),
		metrics: &FeeEngineMetrics{
			FeeTierUsage:      make(map[string]int64),
			PairOverrideUsage: make(map[string]int64),
			LastUpdated:       time.Now(),
		},
	}
}

// CalculateFee calculates fees for a single trading pair order
func (fe *FeeEngine) CalculateFee(ctx context.Context, req *FeeCalculationRequest) (*FeeCalculationResult, error) {
	start := time.Now()
	defer func() {
		fe.recordMetrics(time.Since(start), false)
	}()

	fe.mu.RLock()
	defer fe.mu.RUnlock()

	if req == nil {
		return nil, fmt.Errorf("fee calculation request is nil")
	}

	// Validate request
	if err := fe.validateFeeRequest(req); err != nil {
		fe.recordError()
		return nil, fmt.Errorf("invalid fee request: %w", err)
	}

	// Determine base fee rate
	baseFeeRate, feeTierName, err := fe.getBaseFeeRate(req)
	if err != nil {
		fe.recordError()
		return nil, fmt.Errorf("failed to get base fee rate: %w", err)
	}

	// Calculate base fee
	baseFee := req.TradedValue.Mul(baseFeeRate)

	// Apply account-level discounts
	discount, discountedFee := fe.applyAccountDiscount(req.UserID, baseFee)

	// Apply minimum and maximum fee limits
	finalFee := fe.applyFeeLimits(discountedFee)

	// Round fee according to configuration
	finalFee = finalFee.Round(fe.config.FeeDisplayPrecision)

	// Determine fee currency (typically quote currency)
	feeCurrency := fe.getFeeCurrency(req.Pair)

	// Create breakdown
	breakdown := map[string]decimal.Decimal{
		"base_fee":        baseFee,
		"discount_amount": baseFee.Sub(discountedFee),
		"final_fee":       finalFee,
		"fee_rate":        baseFeeRate,
	}

	result := &FeeCalculationResult{
		BaseFee:         baseFee,
		DiscountedFee:   discountedFee,
		FinalFee:        finalFee,
		Currency:        feeCurrency,
		FeeRate:         baseFeeRate,
		AppliedDiscount: discount,
		FeeTier:         feeTierName,
		Breakdown:       breakdown,
		CalculatedAt:    time.Now(),
	}

	fe.logger.Debug("Fee calculated",
		zap.String("user_id", req.UserID),
		zap.String("pair", req.Pair),
		zap.String("final_fee", finalFee.String()),
		zap.String("fee_tier", feeTierName),
	)

	return result, nil
}

// CalculateCrossPairFee calculates fees for cross-pair trading
func (fe *FeeEngine) CalculateCrossPairFee(ctx context.Context, req *CrossPairFeeRequest) (*CrossPairFeeResult, error) {
	start := time.Now()
	defer func() {
		fe.recordMetrics(time.Since(start), true)
	}()

	fe.mu.RLock()
	defer fe.mu.RUnlock()

	if req == nil {
		return nil, fmt.Errorf("cross-pair fee calculation request is nil")
	}

	if !fe.crossPairConfig.Enabled {
		return nil, fmt.Errorf("cross-pair trading is disabled")
	}

	// Validate cross-pair request
	if err := fe.validateCrossPairRequest(req); err != nil {
		fe.recordError()
		return nil, fmt.Errorf("invalid cross-pair request: %w", err)
	}

	var stepFees []FeeCalculationResult
	var totalFee decimal.Decimal

	// Calculate fees for each step in the route
	for i, step := range req.Route {
		stepReq := &FeeCalculationRequest{
			UserID:      req.UserID,
			Pair:        step.Pair,
			Side:        step.Side,
			OrderType:   "LIMIT", // Assume limit orders for cross-pair
			Quantity:    step.Quantity,
			Price:       step.Price,
			TradedValue: step.Quantity.Mul(step.Price),
			IsMaker:     step.IsMaker,
			Timestamp:   req.Timestamp,
		}

		stepFee, err := fe.CalculateFee(ctx, stepReq)
		if err != nil {
			fe.recordError()
			return nil, fmt.Errorf("failed to calculate fee for step %d: %w", i, err)
		}

		stepFees = append(stepFees, *stepFee)
		totalFee = totalFee.Add(stepFee.FinalFee)
	}

	// Apply cross-pair multiplier
	totalFee = totalFee.Mul(fe.crossPairConfig.BaseFeeMultiplier)

	// Add routing fee
	routingFee := req.EstimatedValue.Mul(fe.crossPairConfig.RoutingFee)
	totalFee = totalFee.Add(routingFee)

	// Add liquidity provider fee
	liquidityFee := req.EstimatedValue.Mul(fe.crossPairConfig.LiquidityProviderFee)
	totalFee = totalFee.Add(liquidityFee)

	// Estimate slippage (simplified model)
	estimatedSlippage := fe.estimateCrossPairSlippage(req)

	// Apply fee limits
	totalFee = fe.applyFeeLimits(totalFee)

	// Create breakdown
	breakdown := map[string]decimal.Decimal{
		"base_trading_fees":  totalFee.Sub(routingFee).Sub(liquidityFee),
		"routing_fee":        routingFee,
		"liquidity_fee":      liquidityFee,
		"total_fee":          totalFee,
		"estimated_slippage": estimatedSlippage,
	}

	result := &CrossPairFeeResult{
		TotalFee:          totalFee,
		StepFees:          stepFees,
		RoutingFee:        routingFee,
		LiquidityFee:      liquidityFee,
		EstimatedSlippage: estimatedSlippage,
		Currency:          fe.getFeeCurrency(req.SourcePair),
		Breakdown:         breakdown,
		CalculatedAt:      time.Now(),
	}

	fe.logger.Debug("Cross-pair fee calculated",
		zap.String("user_id", req.UserID),
		zap.String("source_pair", req.SourcePair),
		zap.String("target_pair", req.TargetPair),
		zap.String("total_fee", totalFee.String()),
		zap.Int("route_steps", len(req.Route)),
	)

	return result, nil
}

// SetFeeTier adds or updates a fee tier
func (fe *FeeEngine) SetFeeTier(tier *FeeTier) error {
	if tier == nil {
		return fmt.Errorf("fee tier is nil")
	}

	fe.mu.Lock()
	defer fe.mu.Unlock()

	fe.feeTiers[tier.Name] = tier
	fe.logger.Info("Fee tier updated",
		zap.String("tier_name", tier.Name),
		zap.String("maker_fee", tier.MakerFee.String()),
		zap.String("taker_fee", tier.TakerFee.String()),
	)

	return nil
}

// SetPairOverride adds or updates a per-pair fee override
func (fe *FeeEngine) SetPairOverride(override *PairFeeConfig) error {
	if override == nil {
		return fmt.Errorf("pair fee override is nil")
	}

	fe.mu.Lock()
	defer fe.mu.Unlock()

	fe.pairOverrides[override.Pair] = override
	fe.logger.Info("Pair fee override updated",
		zap.String("pair", override.Pair),
		zap.String("maker_fee", override.MakerFee.String()),
		zap.String("taker_fee", override.TakerFee.String()),
	)

	return nil
}

// SetAccountDiscount adds or updates an account-level discount
func (fe *FeeEngine) SetAccountDiscount(discount *AccountDiscount) error {
	if discount == nil {
		return fmt.Errorf("account discount is nil")
	}

	fe.mu.Lock()
	defer fe.mu.Unlock()

	fe.accountDiscounts[discount.UserID] = discount
	fe.logger.Info("Account discount updated",
		zap.String("user_id", discount.UserID),
		zap.String("discount_pct", discount.DiscountPct.String()),
	)

	return nil
}

// GetFeeTiers returns all configured fee tiers
func (fe *FeeEngine) GetFeeTiers() map[string]*FeeTier {
	fe.mu.RLock()
	defer fe.mu.RUnlock()

	result := make(map[string]*FeeTier)
	for k, v := range fe.feeTiers {
		tier := *v // Copy
		result[k] = &tier
	}
	return result
}

// GetPairOverrides returns all pair fee overrides
func (fe *FeeEngine) GetPairOverrides() map[string]*PairFeeConfig {
	fe.mu.RLock()
	defer fe.mu.RUnlock()

	result := make(map[string]*PairFeeConfig)
	for k, v := range fe.pairOverrides {
		override := *v // Copy
		result[k] = &override
	}
	return result
}

// GetMetrics returns fee engine metrics
func (fe *FeeEngine) GetMetrics() *FeeEngineMetrics {
	fe.metrics.mu.RLock()
	defer fe.metrics.mu.RUnlock()

	// Return a copy
	return &FeeEngineMetrics{
		TotalCalculations:     fe.metrics.TotalCalculations,
		CrossPairCalculations: fe.metrics.CrossPairCalculations,
		AverageLatencyNs:      fe.metrics.AverageLatencyNs,
		FeeTierUsage:          copyMap(fe.metrics.FeeTierUsage),
		PairOverrideUsage:     copyMap(fe.metrics.PairOverrideUsage),
		DiscountApplications:  fe.metrics.DiscountApplications,
		ErrorCount:            fe.metrics.ErrorCount,
		LastUpdated:           fe.metrics.LastUpdated,
	}
}

// Private helper methods

func (fe *FeeEngine) validateFeeRequest(req *FeeCalculationRequest) error {
	if req.UserID == "" {
		return fmt.Errorf("user_id is required")
	}
	if req.Pair == "" {
		return fmt.Errorf("pair is required")
	}
	if req.Quantity.IsZero() || req.Quantity.IsNegative() {
		return fmt.Errorf("quantity must be positive")
	}
	if req.TradedValue.IsNegative() {
		return fmt.Errorf("traded_value cannot be negative")
	}
	return nil
}

func (fe *FeeEngine) validateCrossPairRequest(req *CrossPairFeeRequest) error {
	if req.UserID == "" {
		return fmt.Errorf("user_id is required")
	}
	if req.SourcePair == "" {
		return fmt.Errorf("source_pair is required")
	}
	if req.TargetPair == "" {
		return fmt.Errorf("target_pair is required")
	}
	if len(req.Route) == 0 {
		return fmt.Errorf("route cannot be empty")
	}
	if len(req.Route) > fe.crossPairConfig.MaxHops {
		return fmt.Errorf("route exceeds maximum hops (%d)", fe.crossPairConfig.MaxHops)
	}
	return nil
}

func (fe *FeeEngine) getBaseFeeRate(req *FeeCalculationRequest) (decimal.Decimal, string, error) {
	// Check for pair-specific overrides first
	if override, exists := fe.pairOverrides[req.Pair]; exists && fe.isOverrideValid(override) {
		fe.recordPairOverrideUsage(req.Pair)
		if req.IsMaker {
			return override.MakerFee, "pair_override", nil
		}
		return override.TakerFee, "pair_override", nil
	}

	// Get user's fee tier
	tier := fe.getUserFeeTier(req.UserID)
	fe.recordFeeTierUsage(tier.Name)

	if req.IsMaker {
		return tier.MakerFee, tier.Name, nil
	}
	return tier.TakerFee, tier.Name, nil
}

func (fe *FeeEngine) getUserFeeTier(userID string) *FeeTier {
	// TODO: Implement logic to determine user's fee tier based on:
	// - Monthly trading volume
	// - Token holdings
	// - Account type
	// For now, return default tier

	if defaultTier, exists := fe.feeTiers["default"]; exists {
		return defaultTier
	}

	// Fallback to config defaults
	return &FeeTier{
		Name:     "default",
		MakerFee: fe.config.DefaultMakerFee,
		TakerFee: fe.config.DefaultTakerFee,
	}
}

func (fe *FeeEngine) applyAccountDiscount(userID string, baseFee decimal.Decimal) (decimal.Decimal, decimal.Decimal) {
	if discount, exists := fe.accountDiscounts[userID]; exists && fe.isDiscountValid(discount) {
		fe.recordDiscountApplication()
		discountAmount := baseFee.Mul(discount.DiscountPct).Div(decimal.NewFromInt(100))
		return discount.DiscountPct, baseFee.Sub(discountAmount)
	}
	return decimal.Zero, baseFee
}

func (fe *FeeEngine) applyFeeLimits(fee decimal.Decimal) decimal.Decimal {
	if fee.LessThan(fe.config.MinFeeAmount) {
		return fe.config.MinFeeAmount
	}
	if !fe.config.MaxFeeAmount.IsZero() && fee.GreaterThan(fe.config.MaxFeeAmount) {
		return fe.config.MaxFeeAmount
	}
	return fee
}

func (fe *FeeEngine) getFeeCurrency(pair string) string {
	// TODO: Extract quote currency from pair
	// For now, assume USDT for most pairs
	if len(pair) >= 6 && pair[len(pair)-4:] == "USDT" {
		return "USDT"
	}
	return "USD"
}

func (fe *FeeEngine) estimateCrossPairSlippage(req *CrossPairFeeRequest) decimal.Decimal {
	// Simplified slippage estimation
	// In a real implementation, this would consider:
	// - Order book depth
	// - Market volatility
	// - Route complexity
	baseSlippage := decimal.NewFromFloat(0.001) // 0.1% base
	routeMultiplier := decimal.NewFromInt(int64(len(req.Route)))
	return baseSlippage.Mul(routeMultiplier)
}

func (fe *FeeEngine) isOverrideValid(override *PairFeeConfig) bool {
	if !override.Enabled {
		return false
	}
	now := time.Now()
	if now.Before(override.ValidFrom) {
		return false
	}
	if override.ValidTo != nil && now.After(*override.ValidTo) {
		return false
	}
	return true
}

func (fe *FeeEngine) isDiscountValid(discount *AccountDiscount) bool {
	now := time.Now()
	if now.Before(discount.ValidFrom) {
		return false
	}
	if discount.ValidTo != nil && now.After(*discount.ValidTo) {
		return false
	}
	return true
}

func (fe *FeeEngine) recordMetrics(duration time.Duration, isCrossPair bool) {
	fe.metrics.mu.Lock()
	defer fe.metrics.mu.Unlock()

	fe.metrics.TotalCalculations++
	if isCrossPair {
		fe.metrics.CrossPairCalculations++
	}

	// Update average latency (simple moving average)
	if fe.metrics.AverageLatencyNs == 0 {
		fe.metrics.AverageLatencyNs = duration.Nanoseconds()
	} else {
		fe.metrics.AverageLatencyNs = (fe.metrics.AverageLatencyNs + duration.Nanoseconds()) / 2
	}

	fe.metrics.LastUpdated = time.Now()
}

func (fe *FeeEngine) recordFeeTierUsage(tierName string) {
	fe.metrics.mu.Lock()
	defer fe.metrics.mu.Unlock()
	fe.metrics.FeeTierUsage[tierName]++
}

func (fe *FeeEngine) recordPairOverrideUsage(pair string) {
	fe.metrics.mu.Lock()
	defer fe.metrics.mu.Unlock()
	fe.metrics.PairOverrideUsage[pair]++
}

func (fe *FeeEngine) recordDiscountApplication() {
	fe.metrics.mu.Lock()
	defer fe.metrics.mu.Unlock()
	fe.metrics.DiscountApplications++
}

func (fe *FeeEngine) recordError() {
	fe.metrics.mu.Lock()
	defer fe.metrics.mu.Unlock()
	fe.metrics.ErrorCount++
}

func getDefaultFeeEngineConfig() *FeeEngineConfig {
	return &FeeEngineConfig{
		DefaultMakerFee:        decimal.NewFromFloat(0.001), // 0.1%
		DefaultTakerFee:        decimal.NewFromFloat(0.001), // 0.1%
		CrossPairFeeMultiplier: decimal.NewFromFloat(1.5),   // 1.5x multiplier
		CrossPairMinFee:        decimal.NewFromFloat(0.0001),
		CrossPairMaxFee:        decimal.NewFromFloat(0.01),
		MinFeeAmount:           decimal.NewFromFloat(0.00000001),
		MaxFeeAmount:           decimal.Zero, // No max limit
		FeeDisplayPrecision:    8,
	}
}

func getDefaultCrossPairConfig() *CrossPairFeeConfig {
	return &CrossPairFeeConfig{
		Enabled:              true,
		BaseFeeMultiplier:    decimal.NewFromFloat(1.2),
		RoutingFee:           decimal.NewFromFloat(0.0001),
		LiquidityProviderFee: decimal.NewFromFloat(0.0002),
		MaxHops:              3,
	}
}

func copyMap(src map[string]int64) map[string]int64 {
	dst := make(map[string]int64)
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
