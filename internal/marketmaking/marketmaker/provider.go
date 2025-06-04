package marketmaker

import (
	"math"
	"sync"
	"time"
)

// Enhanced LiquidityProvider with sophisticated metrics
type LiquidityProvider struct {
	ID     string
	Name   string
	APIKey string
	Active bool
	Tier   ProviderTier

	// Volume and performance metrics
	Volume  float64
	Rebates float64

	// Advanced performance tracking
	Performance  *ProviderPerformance
	Incentives   *IncentiveStructure
	RiskLimits   *ProviderRiskLimits
	LastActivity time.Time

	mu sync.RWMutex
}

type ProviderTier int

const (
	RetailTier ProviderTier = iota
	ProfessionalTier
	InstitutionalTier
	MarketMakerTier
	PrimeTier
)

type ProviderPerformance struct {
	// Liquidity provision metrics
	AverageSpread     float64
	UptimePercentage  float64
	QuoteResponseTime time.Duration
	FillRate          float64

	// Market quality metrics
	SpreadTightness  float64
	DepthConsistency float64
	PriceImprovement float64

	// Risk and compliance
	ToxicOrderRatio      float64
	AdverseSelectionCost float64
	InventoryTurnover    float64

	// Performance scores (0-100)
	LiquidityScore     float64
	ReliabilityScore   float64
	ProfitabilityScore float64
	OverallScore       float64

	// Historical tracking
	DailyMetrics   []DailyPerformance
	WeeklyMetrics  []WeeklyPerformance
	MonthlyMetrics []MonthlyPerformance
}

type DailyPerformance struct {
	Date            time.Time
	Volume          float64
	Spread          float64
	Uptime          float64
	PnL             float64
	ToxicOrderRatio float64
}

type WeeklyPerformance struct {
	Week          time.Time
	AverageVolume float64
	AverageSpread float64
	AverageUptime float64
	TotalPnL      float64
	QualityScore  float64
}

type MonthlyPerformance struct {
	Month          time.Time
	TotalVolume    float64
	AverageSpread  float64
	AverageUptime  float64
	TotalPnL       float64
	TotalRebates   float64
	NetPnL         float64
	RankPercentile float64
}

type IncentiveStructure struct {
	// Base rebate structure
	BaseRebateRate float64

	// Volume-based tiers
	VolumeTiers map[float64]float64

	// Performance-based bonuses
	SpreadBonusRate      float64
	UptimeBonusRate      float64
	ConsistencyBonusRate float64

	// Market making incentives
	MinSpreadRequirement float64
	MinUptimeRequirement float64
	MinDepthRequirement  float64

	// Dynamic adjustments
	MarketConditionMultiplier float64
	CompetitionAdjustment     float64
	SpecialIncentives         map[string]float64
}

type ProviderRiskLimits struct {
	MaxDailyVolume       float64
	MaxPositionSize      float64
	MaxDrawdown          float64
	MaxToxicOrderRatio   float64
	MinUptimeRequirement float64
	MaxLatency           time.Duration
}

// Enhanced ProviderRegistry with sophisticated management
type ProviderRegistry struct {
	providers        map[string]*LiquidityProvider
	tierRequirements map[ProviderTier]*TierRequirements
	globalIncentives *GlobalIncentiveParameters

	// Performance analytics
	analytics  *ProviderAnalytics
	benchmarks *PerformanceBenchmarks

	// Market conditions
	marketConditions *MarketConditions

	mu sync.RWMutex
}

type TierRequirements struct {
	MinMonthlyVolume       float64
	MinUptimePercentage    float64
	MaxAverageSpread       float64
	MinLiquidityScore      float64
	RequiredCapital        float64
	ComplianceRequirements []string
}

type GlobalIncentiveParameters struct {
	BaseFeeRate         float64
	VolumeDiscountRate  float64
	MarketMakerDiscount float64
	LoyaltyBonusRate    float64
	NewProviderBonus    float64
	CompetitionBonus    float64
}

type ProviderAnalytics struct {
	TotalProviders  int
	ActiveProviders int
	ProvidersByTier map[ProviderTier]int

	// Aggregate metrics
	TotalVolume   float64
	AverageSpread float64
	AverageUptime float64

	// Performance distribution
	TopPerformers   []*LiquidityProvider
	UnderPerformers []*LiquidityProvider

	// Market impact
	MarketShareByProvider map[string]float64
	ConcentrationIndex    float64

	mu sync.RWMutex
}

type PerformanceBenchmarks struct {
	// Industry benchmarks
	MedianSpread      float64
	TopQuartileSpread float64
	MedianUptime      float64
	TopQuartileUptime float64

	// Internal benchmarks
	BestPerformerMetrics *ProviderPerformance
	AverageMetrics       *ProviderPerformance

	LastUpdated time.Time
}

type MarketConditions struct {
	VolatilityLevel float64
	LiquidityLevel  float64
	TradingActivity float64
	MarketStress    float64

	// Incentive multipliers based on conditions
	VolatilityMultiplier float64
	LiquidityMultiplier  float64
	StressMultiplier     float64

	LastUpdated time.Time
}

func NewProviderRegistry() *ProviderRegistry {
	registry := &ProviderRegistry{
		providers:        make(map[string]*LiquidityProvider),
		tierRequirements: make(map[ProviderTier]*TierRequirements),
		analytics: &ProviderAnalytics{
			ProvidersByTier:       make(map[ProviderTier]int),
			MarketShareByProvider: make(map[string]float64),
		},
		benchmarks: &PerformanceBenchmarks{},
		marketConditions: &MarketConditions{
			VolatilityMultiplier: 1.0,
			LiquidityMultiplier:  1.0,
			StressMultiplier:     1.0,
		},
		globalIncentives: &GlobalIncentiveParameters{
			BaseFeeRate:         0.001,
			VolumeDiscountRate:  0.0001,
			MarketMakerDiscount: 0.0005,
			LoyaltyBonusRate:    0.0002,
			NewProviderBonus:    0.0003,
			CompetitionBonus:    0.0001,
		},
	}

	// Initialize tier requirements
	registry.initializeTierRequirements()

	return registry
}

func (r *ProviderRegistry) initializeTierRequirements() {
	r.tierRequirements[RetailTier] = &TierRequirements{
		MinMonthlyVolume:    10000,
		MinUptimePercentage: 95.0,
		MaxAverageSpread:    0.01,
		MinLiquidityScore:   60.0,
		RequiredCapital:     1000,
	}

	r.tierRequirements[ProfessionalTier] = &TierRequirements{
		MinMonthlyVolume:    100000,
		MinUptimePercentage: 97.0,
		MaxAverageSpread:    0.005,
		MinLiquidityScore:   75.0,
		RequiredCapital:     10000,
	}

	r.tierRequirements[InstitutionalTier] = &TierRequirements{
		MinMonthlyVolume:    1000000,
		MinUptimePercentage: 99.0,
		MaxAverageSpread:    0.003,
		MinLiquidityScore:   85.0,
		RequiredCapital:     100000,
	}

	r.tierRequirements[MarketMakerTier] = &TierRequirements{
		MinMonthlyVolume:    5000000,
		MinUptimePercentage: 99.5,
		MaxAverageSpread:    0.002,
		MinLiquidityScore:   90.0,
		RequiredCapital:     1000000,
	}

	r.tierRequirements[PrimeTier] = &TierRequirements{
		MinMonthlyVolume:    50000000,
		MinUptimePercentage: 99.9,
		MaxAverageSpread:    0.001,
		MinLiquidityScore:   95.0,
		RequiredCapital:     10000000,
	}
}

// Enhanced provider management methods

func (r *ProviderRegistry) Register(lp *LiquidityProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Initialize performance tracking if not exists
	if lp.Performance == nil {
		lp.Performance = &ProviderPerformance{
			DailyMetrics:   make([]DailyPerformance, 0),
			WeeklyMetrics:  make([]WeeklyPerformance, 0),
			MonthlyMetrics: make([]MonthlyPerformance, 0),
		}
	}

	// Initialize incentive structure based on tier
	if lp.Incentives == nil {
		lp.Incentives = r.createIncentiveStructure(lp.Tier)
	}

	// Set default risk limits
	if lp.RiskLimits == nil {
		lp.RiskLimits = r.createRiskLimits(lp.Tier)
	}

	lp.LastActivity = time.Now()
	r.providers[lp.ID] = lp

	// Update analytics
	r.updateAnalytics()
}

func (r *ProviderRegistry) createIncentiveStructure(tier ProviderTier) *IncentiveStructure {
	baseRates := map[ProviderTier]float64{
		RetailTier:        0.0005,
		ProfessionalTier:  0.0003,
		InstitutionalTier: 0.0002,
		MarketMakerTier:   0.0001,
		PrimeTier:         -0.0001, // Prime providers pay fees
	}

	return &IncentiveStructure{
		BaseRebateRate: baseRates[tier],
		VolumeTiers: map[float64]float64{
			100000:   0.0001,
			1000000:  0.0002,
			10000000: 0.0003,
		},
		SpreadBonusRate:           0.00005,
		UptimeBonusRate:           0.00002,
		ConsistencyBonusRate:      0.00003,
		MinSpreadRequirement:      0.005,
		MinUptimeRequirement:      95.0,
		MinDepthRequirement:       1000,
		MarketConditionMultiplier: 1.0,
		CompetitionAdjustment:     1.0,
		SpecialIncentives:         make(map[string]float64),
	}
}

func (r *ProviderRegistry) createRiskLimits(tier ProviderTier) *ProviderRiskLimits {
	limits := map[ProviderTier]*ProviderRiskLimits{
		RetailTier: {
			MaxDailyVolume:       50000,
			MaxPositionSize:      5000,
			MaxDrawdown:          1000,
			MaxToxicOrderRatio:   0.1,
			MinUptimeRequirement: 90.0,
			MaxLatency:           100 * time.Millisecond,
		},
		ProfessionalTier: {
			MaxDailyVolume:       500000,
			MaxPositionSize:      50000,
			MaxDrawdown:          10000,
			MaxToxicOrderRatio:   0.05,
			MinUptimeRequirement: 95.0,
			MaxLatency:           50 * time.Millisecond,
		},
		InstitutionalTier: {
			MaxDailyVolume:       5000000,
			MaxPositionSize:      500000,
			MaxDrawdown:          100000,
			MaxToxicOrderRatio:   0.03,
			MinUptimeRequirement: 98.0,
			MaxLatency:           25 * time.Millisecond,
		},
		MarketMakerTier: {
			MaxDailyVolume:       50000000,
			MaxPositionSize:      5000000,
			MaxDrawdown:          1000000,
			MaxToxicOrderRatio:   0.02,
			MinUptimeRequirement: 99.0,
			MaxLatency:           10 * time.Millisecond,
		},
		PrimeTier: {
			MaxDailyVolume:       500000000,
			MaxPositionSize:      50000000,
			MaxDrawdown:          10000000,
			MaxToxicOrderRatio:   0.01,
			MinUptimeRequirement: 99.5,
			MaxLatency:           5 * time.Millisecond,
		},
	}

	return limits[tier]
}

// Advanced performance tracking and analytics

func (r *ProviderRegistry) UpdateProviderPerformance(id string, metrics map[string]float64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	provider, exists := r.providers[id]
	if !exists {
		return
	}

	provider.mu.Lock()
	defer provider.mu.Unlock()

	perf := provider.Performance

	// Update real-time metrics
	if spread, ok := metrics["spread"]; ok {
		perf.AverageSpread = r.updateMovingAverage(perf.AverageSpread, spread, 0.1)
	}

	if uptime, ok := metrics["uptime"]; ok {
		perf.UptimePercentage = r.updateMovingAverage(perf.UptimePercentage, uptime, 0.05)
	}

	if responseTime, ok := metrics["response_time"]; ok {
		perf.QuoteResponseTime = time.Duration(responseTime) * time.Millisecond
	}

	if fillRate, ok := metrics["fill_rate"]; ok {
		perf.FillRate = r.updateMovingAverage(perf.FillRate, fillRate, 0.1)
	}

	if toxicRatio, ok := metrics["toxic_ratio"]; ok {
		perf.ToxicOrderRatio = r.updateMovingAverage(perf.ToxicOrderRatio, toxicRatio, 0.1)
	}

	// Calculate composite scores
	r.calculatePerformanceScores(perf)

	provider.LastActivity = time.Now()
}

func (r *ProviderRegistry) updateMovingAverage(current, new, alpha float64) float64 {
	if current == 0 {
		return new
	}
	return alpha*new + (1-alpha)*current
}

func (r *ProviderRegistry) calculatePerformanceScores(perf *ProviderPerformance) {
	// Liquidity Score (0-100)
	spreadScore := math.Max(0, 100-perf.AverageSpread*10000) // Lower spread = higher score
	depthScore := perf.DepthConsistency * 100
	perf.LiquidityScore = (spreadScore + depthScore) / 2

	// Reliability Score (0-100)
	uptimeScore := perf.UptimePercentage
	responseScore := math.Max(0, 100-float64(perf.QuoteResponseTime.Milliseconds()))
	fillScore := perf.FillRate * 100
	perf.ReliabilityScore = (uptimeScore + responseScore + fillScore) / 3

	// Profitability Score (0-100)
	toxicScore := math.Max(0, 100-perf.ToxicOrderRatio*1000)
	adverseSelectionScore := math.Max(0, 100-perf.AdverseSelectionCost*1000)
	turnoverScore := math.Min(100, perf.InventoryTurnover*10)
	perf.ProfitabilityScore = (toxicScore + adverseSelectionScore + turnoverScore) / 3

	// Overall Score
	perf.OverallScore = (perf.LiquidityScore + perf.ReliabilityScore + perf.ProfitabilityScore) / 3
}

// Dynamic rebate calculation
func (r *ProviderRegistry) CalculateDynamicRebate(id string, volume float64) float64 {
	r.mu.RLock()
	defer r.mu.RUnlock()

	provider, exists := r.providers[id]
	if !exists {
		return 0
	}

	provider.mu.RLock()
	defer provider.mu.RUnlock()

	incentives := provider.Incentives
	baseRebate := incentives.BaseRebateRate * volume

	// Volume tier bonus
	volumeBonus := 0.0
	for threshold, bonus := range incentives.VolumeTiers {
		if volume >= threshold {
			volumeBonus = bonus * volume
		}
	}

	// Performance bonuses
	perf := provider.Performance
	spreadBonus := incentives.SpreadBonusRate * volume * math.Max(0, (0.005-perf.AverageSpread)/0.005)
	uptimeBonus := incentives.UptimeBonusRate * volume * math.Max(0, (perf.UptimePercentage-95)/5)
	consistencyBonus := incentives.ConsistencyBonusRate * volume * (perf.OverallScore / 100)

	// Market condition adjustments
	marketMultiplier := r.marketConditions.VolatilityMultiplier *
		r.marketConditions.LiquidityMultiplier *
		r.marketConditions.StressMultiplier

	totalRebate := (baseRebate + volumeBonus + spreadBonus + uptimeBonus + consistencyBonus) * marketMultiplier

	return math.Max(0, totalRebate) // Ensure non-negative
}

// Tier management and promotions
func (r *ProviderRegistry) EvaluateTierPromotion(id string) ProviderTier {
	r.mu.RLock()
	defer r.mu.RUnlock()

	provider, exists := r.providers[id]
	if !exists {
		return RetailTier
	}

	provider.mu.RLock()
	defer provider.mu.RUnlock()

	// Get monthly performance metrics
	monthlyMetrics := r.getRecentMonthlyMetrics(provider, 3) // Last 3 months
	if len(monthlyMetrics) == 0 {
		return provider.Tier
	}

	avgVolume := r.calculateAverageMonthlyVolume(monthlyMetrics)
	avgUptime := r.calculateAverageUptime(monthlyMetrics)
	avgSpread := r.calculateAverageSpread(monthlyMetrics)
	liquidityScore := provider.Performance.LiquidityScore

	// Check each tier from highest to lowest
	for tier := PrimeTier; tier >= RetailTier; tier-- {
		requirements := r.tierRequirements[tier]
		if avgVolume >= requirements.MinMonthlyVolume &&
			avgUptime >= requirements.MinUptimePercentage &&
			avgSpread <= requirements.MaxAverageSpread &&
			liquidityScore >= requirements.MinLiquidityScore {
			return tier
		}
	}

	return RetailTier
}

func (r *ProviderRegistry) getRecentMonthlyMetrics(provider *LiquidityProvider, months int) []MonthlyPerformance {
	metrics := provider.Performance.MonthlyMetrics
	if len(metrics) <= months {
		return metrics
	}
	return metrics[len(metrics)-months:]
}

func (r *ProviderRegistry) calculateAverageMonthlyVolume(metrics []MonthlyPerformance) float64 {
	if len(metrics) == 0 {
		return 0
	}

	total := 0.0
	for _, metric := range metrics {
		total += metric.TotalVolume
	}
	return total / float64(len(metrics))
}

func (r *ProviderRegistry) calculateAverageUptime(metrics []MonthlyPerformance) float64 {
	if len(metrics) == 0 {
		return 0
	}

	total := 0.0
	for _, metric := range metrics {
		total += metric.AverageUptime
	}
	return total / float64(len(metrics))
}

func (r *ProviderRegistry) calculateAverageSpread(metrics []MonthlyPerformance) float64 {
	if len(metrics) == 0 {
		return 0
	}

	total := 0.0
	for _, metric := range metrics {
		total += metric.AverageSpread
	}
	return total / float64(len(metrics))
}

// Analytics and reporting
func (r *ProviderRegistry) updateAnalytics() {
	r.analytics.mu.Lock()
	defer r.analytics.mu.Unlock()

	r.analytics.TotalProviders = len(r.providers)
	r.analytics.ActiveProviders = 0
	r.analytics.TotalVolume = 0

	// Reset tier counts
	for tier := range r.analytics.ProvidersByTier {
		r.analytics.ProvidersByTier[tier] = 0
	}

	totalSpread := 0.0
	totalUptime := 0.0
	activeCount := 0

	for _, provider := range r.providers {
		provider.mu.RLock()

		if provider.Active {
			r.analytics.ActiveProviders++
			activeCount++
			totalSpread += provider.Performance.AverageSpread
			totalUptime += provider.Performance.UptimePercentage
		}

		r.analytics.ProvidersByTier[provider.Tier]++
		r.analytics.TotalVolume += provider.Volume

		provider.mu.RUnlock()
	}

	if activeCount > 0 {
		r.analytics.AverageSpread = totalSpread / float64(activeCount)
		r.analytics.AverageUptime = totalUptime / float64(activeCount)
	}

	// Update top and underperformers
	r.updatePerformerLists()
}

func (r *ProviderRegistry) updatePerformerLists() {
	// Sort providers by overall score
	allProviders := make([]*LiquidityProvider, 0, len(r.providers))
	for _, provider := range r.providers {
		if provider.Active {
			allProviders = append(allProviders, provider)
		}
	}

	// Simple bubble sort by overall score (descending)
	for i := 0; i < len(allProviders); i++ {
		for j := i + 1; j < len(allProviders); j++ {
			if allProviders[i].Performance.OverallScore < allProviders[j].Performance.OverallScore {
				allProviders[i], allProviders[j] = allProviders[j], allProviders[i]
			}
		}
	}
	// Top 10% as top performers
	topCount := int(math.Max(1, float64(len(allProviders))*0.1))
	r.analytics.TopPerformers = allProviders[:int(math.Min(float64(topCount), float64(len(allProviders))))]

	// Bottom 10% as underperformers
	underCount := int(math.Max(1, float64(len(allProviders))*0.1))
	startIdx := int(math.Max(0, float64(len(allProviders)-underCount)))
	r.analytics.UnderPerformers = allProviders[startIdx:]
}

// Market condition updates
func (r *ProviderRegistry) UpdateMarketConditions(volatility, liquidity, activity, stress float64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.marketConditions.VolatilityLevel = volatility
	r.marketConditions.LiquidityLevel = liquidity
	r.marketConditions.TradingActivity = activity
	r.marketConditions.MarketStress = stress

	// Calculate dynamic multipliers
	r.marketConditions.VolatilityMultiplier = 1.0 + volatility*0.5
	r.marketConditions.LiquidityMultiplier = 1.0 + (1.0-liquidity)*0.3
	r.marketConditions.StressMultiplier = 1.0 + stress*0.2

	r.marketConditions.LastUpdated = time.Now()
}

// Existing methods preserved for compatibility
func (r *ProviderRegistry) Get(id string) *LiquidityProvider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.providers[id]
}

func (r *ProviderRegistry) UpdateVolume(id string, volume float64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if lp, ok := r.providers[id]; ok {
		lp.mu.Lock()
		lp.Volume += volume
		lp.LastActivity = time.Now()
		lp.mu.Unlock()

		// Update daily metrics
		r.updateDailyMetrics(lp, volume)
	}
}

func (r *ProviderRegistry) updateDailyMetrics(provider *LiquidityProvider, volume float64) {
	today := time.Now().Truncate(24 * time.Hour)
	metrics := &provider.Performance.DailyMetrics

	// Find or create today's metrics
	var todayMetric *DailyPerformance
	for i := len(*metrics) - 1; i >= 0; i-- {
		if (*metrics)[i].Date.Equal(today) {
			todayMetric = &(*metrics)[i]
			break
		}
	}

	if todayMetric == nil {
		*metrics = append(*metrics, DailyPerformance{
			Date:   today,
			Volume: 0,
		})
		todayMetric = &(*metrics)[len(*metrics)-1]
	}

	todayMetric.Volume += volume
	todayMetric.Spread = provider.Performance.AverageSpread
	todayMetric.Uptime = provider.Performance.UptimePercentage
	todayMetric.ToxicOrderRatio = provider.Performance.ToxicOrderRatio

	// Keep only recent daily metrics
	if len(*metrics) > 90 { // Keep 90 days
		*metrics = (*metrics)[len(*metrics)-90:]
	}
}

func (r *ProviderRegistry) AddRebate(id string, rebate float64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if lp, ok := r.providers[id]; ok {
		lp.mu.Lock()
		lp.Rebates += rebate
		lp.mu.Unlock()
	}
}

func (r *ProviderRegistry) List() []*LiquidityProvider {
	r.mu.RLock()
	defer r.mu.RUnlock()

	lps := make([]*LiquidityProvider, 0, len(r.providers))
	for _, lp := range r.providers {
		lps = append(lps, lp)
	}
	return lps
}

func (r *ProviderRegistry) OnboardProvider(lp *LiquidityProvider) {
	lp.Active = true
	r.Register(lp)
}

func (r *ProviderRegistry) GetStatus(id string) (active bool, volume float64, rebates float64) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	lp := r.providers[id]
	if lp == nil {
		return false, 0, 0
	}

	lp.mu.RLock()
	defer lp.mu.RUnlock()

	return lp.Active, lp.Volume, lp.Rebates
}

// Advanced analytics and reporting methods
func (r *ProviderRegistry) GetProviderAnalytics() *ProviderAnalytics {
	r.analytics.mu.RLock()
	defer r.analytics.mu.RUnlock()

	// Return a copy to prevent race conditions
	analyticsCopy := *r.analytics
	return &analyticsCopy
}

func (r *ProviderRegistry) GetPerformanceBenchmarks() *PerformanceBenchmarks {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Calculate current benchmarks
	r.calculateBenchmarks()

	benchmarksCopy := *r.benchmarks
	return &benchmarksCopy
}

func (r *ProviderRegistry) calculateBenchmarks() {
	if len(r.providers) == 0 {
		return
	}

	spreads := make([]float64, 0)
	uptimes := make([]float64, 0)

	for _, provider := range r.providers {
		if provider.Active {
			provider.mu.RLock()
			spreads = append(spreads, provider.Performance.AverageSpread)
			uptimes = append(uptimes, provider.Performance.UptimePercentage)
			provider.mu.RUnlock()
		}
	}

	if len(spreads) > 0 {
		r.benchmarks.MedianSpread = r.calculateMedian(spreads)
		r.benchmarks.TopQuartileSpread = r.calculatePercentile(spreads, 0.75)
		r.benchmarks.MedianUptime = r.calculateMedian(uptimes)
		r.benchmarks.TopQuartileUptime = r.calculatePercentile(uptimes, 0.75)
	}

	r.benchmarks.LastUpdated = time.Now()
}

func (r *ProviderRegistry) calculateMedian(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	// Simple bubble sort
	sorted := make([]float64, len(values))
	copy(sorted, values)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	mid := len(sorted) / 2
	if len(sorted)%2 == 0 {
		return (sorted[mid-1] + sorted[mid]) / 2
	}
	return sorted[mid]
}

func (r *ProviderRegistry) calculatePercentile(values []float64, percentile float64) float64 {
	if len(values) == 0 {
		return 0
	}

	// Simple bubble sort
	sorted := make([]float64, len(values))
	copy(sorted, values)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	index := int(float64(len(sorted)-1) * percentile)
	return sorted[index]
}
