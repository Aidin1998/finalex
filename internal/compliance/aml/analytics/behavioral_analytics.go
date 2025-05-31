package analytics

import (
	"context"
	"math"
	"sort"
	"time"

	"github.com/shopspring/decimal"
)

// BehavioralAnalytics performs user behavior analysis for AML detection
type BehavioralAnalytics struct {
	userProfiles      map[string]*UserBehaviorProfile
	baselineMetrics   map[string]*BaselineMetrics
	anomalyThresholds *AnomalyThresholds
}

// UserBehaviorProfile contains user behavior patterns and metrics
type UserBehaviorProfile struct {
	UserID            string
	CreatedAt         time.Time
	LastUpdated       time.Time
	TransactionStats  *TransactionStats
	TradingPatterns   *TradingPatterns
	TimePatterns      *TimePatterns
	GeographicData    *GeographicData
	DeviceFingerprint *DeviceFingerprint
	RiskFactors       []RiskFactor
	AnomalyHistory    []AnomalyEvent
}

// TransactionStats contains transaction-related statistics
type TransactionStats struct {
	TotalTransactions  int64
	TotalVolume        decimal.Decimal
	AverageAmount      decimal.Decimal
	MedianAmount       decimal.Decimal
	MaxAmount          decimal.Decimal
	MinAmount          decimal.Decimal
	StandardDeviation  decimal.Decimal
	DailyAverageCount  decimal.Decimal
	DailyAverageVolume decimal.Decimal
	FrequencyPattern   []FrequencyData
	VelocityMetrics    *VelocityMetrics
}

// TradingPatterns contains trading behavior patterns
type TradingPatterns struct {
	PreferredMarkets      []string
	TradingFrequency      map[string]int64           // market -> frequency
	OrderSizeDistribution map[string]decimal.Decimal // size_range -> percentage
	WinLossRatio          decimal.Decimal
	AverageHoldTime       time.Duration
	PreferredOrderTypes   []string
	LeverageUsage         *LeverageStats
}

// TimePatterns contains temporal behavior patterns
type TimePatterns struct {
	ActiveHours      []int // Hours of day (0-23)
	ActiveDays       []int // Days of week (0-6, Sunday=0)
	SessionDuration  time.Duration
	SessionFrequency decimal.Decimal
	TimezonePattern  string
	PeakActivityUTC  time.Time
}

// GeographicData contains location-based information
type GeographicData struct {
	CountryCode     string
	RegionCode      string
	IPAddresses     []string
	LocationHistory []LocationEntry
	VPNDetected     bool
	ProxyDetected   bool
	TorDetected     bool
	SuspiciousGeo   bool
}

// DeviceFingerprint contains device identification data
type DeviceFingerprint struct {
	DeviceIDs           []string
	UserAgents          []string
	BrowserFingerprints []string
	OperatingSystems    []string
	ScreenResolutions   []string
	DeviceChanges       int
	SuspiciousDevices   bool
}

// BaselineMetrics contains baseline behavior for comparison
type BaselineMetrics struct {
	UserType             string
	AvgTransactionCount  decimal.Decimal
	AvgTransactionVolume decimal.Decimal
	AvgSessionDuration   time.Duration
	TypicalMarkets       []string
	TypicalHours         []int
}

// AnomalyThresholds defines thresholds for anomaly detection
type AnomalyThresholds struct {
	VolumeMultiplier      decimal.Decimal
	FrequencyMultiplier   decimal.Decimal
	DeviationThreshold    decimal.Decimal
	GeographicRiskScore   decimal.Decimal
	DeviceChangeThreshold int
	TimeAnomalyThreshold  decimal.Decimal
}

// RiskFactor represents a behavioral risk factor
type RiskFactor struct {
	Type        string
	Description string
	Severity    string
	Score       decimal.Decimal
	DetectedAt  time.Time
	Evidence    map[string]interface{}
}

// AnomalyEvent represents a detected behavioral anomaly
type AnomalyEvent struct {
	ID          string
	Type        string
	Description string
	Severity    string
	Score       decimal.Decimal
	DetectedAt  time.Time
	Resolved    bool
	ResolvedAt  *time.Time
	Metadata    map[string]interface{}
}

// FrequencyData contains frequency analysis data
type FrequencyData struct {
	TimeWindow time.Duration
	Count      int64
	Volume     decimal.Decimal
}

// VelocityMetrics contains velocity-based metrics
type VelocityMetrics struct {
	TransactionsPerHour    decimal.Decimal
	VolumePerHour          decimal.Decimal
	BurstDetectionScore    decimal.Decimal
	SustainedActivityScore decimal.Decimal
}

// LeverageStats contains leverage usage statistics
type LeverageStats struct {
	AverageLeverage  decimal.Decimal
	MaxLeverage      decimal.Decimal
	HighLeverageFreq decimal.Decimal
	RiskScore        decimal.Decimal
}

// LocationEntry represents a geographic location entry
type LocationEntry struct {
	Timestamp   time.Time
	CountryCode string
	RegionCode  string
	IPAddress   string
	Confidence  decimal.Decimal
}

// BehavioralAnalysisResult contains the result of behavioral analysis
type BehavioralAnalysisResult struct {
	UserID            string
	AnalysisTime      time.Time
	OverallRiskScore  decimal.Decimal
	AnomaliesDetected []AnomalyEvent
	RiskFactors       []RiskFactor
	Recommendations   []string
	ProfileUpdated    bool
	TriggerActions    []string
}

// NewBehavioralAnalytics creates a new behavioral analytics engine
func NewBehavioralAnalytics() *BehavioralAnalytics {
	return &BehavioralAnalytics{
		userProfiles:    make(map[string]*UserBehaviorProfile),
		baselineMetrics: make(map[string]*BaselineMetrics),
		anomalyThresholds: &AnomalyThresholds{
			VolumeMultiplier:      decimal.NewFromFloat(3.0),
			FrequencyMultiplier:   decimal.NewFromFloat(2.5),
			DeviationThreshold:    decimal.NewFromFloat(2.0),
			GeographicRiskScore:   decimal.NewFromFloat(0.7),
			DeviceChangeThreshold: 3,
			TimeAnomalyThreshold:  decimal.NewFromFloat(0.8),
		},
	}
}

// AnalyzeUserBehavior performs comprehensive behavioral analysis
func (ba *BehavioralAnalytics) AnalyzeUserBehavior(ctx context.Context, userID string, transactions []Transaction) (*BehavioralAnalysisResult, error) {
	result := &BehavioralAnalysisResult{
		UserID:            userID,
		AnalysisTime:      time.Now(),
		OverallRiskScore:  decimal.Zero,
		AnomaliesDetected: []AnomalyEvent{},
		RiskFactors:       []RiskFactor{},
		Recommendations:   []string{},
		TriggerActions:    []string{},
	}

	// Get or create user profile
	profile := ba.getUserProfile(userID)

	// Update profile with new transaction data
	ba.updateUserProfile(profile, transactions)

	// Perform anomaly detection
	anomalies := ba.detectAnomalies(profile, transactions)
	result.AnomaliesDetected = anomalies

	// Calculate risk factors
	riskFactors := ba.calculateRiskFactors(profile)
	result.RiskFactors = riskFactors

	// Calculate overall risk score
	result.OverallRiskScore = ba.calculateOverallRiskScore(anomalies, riskFactors)

	// Generate recommendations
	result.Recommendations = ba.generateRecommendations(result.OverallRiskScore, anomalies, riskFactors)

	// Determine trigger actions
	result.TriggerActions = ba.determineTriggerActions(result.OverallRiskScore, anomalies)

	// Update profile timestamp
	profile.LastUpdated = time.Now()
	result.ProfileUpdated = true

	return result, nil
}

// getUserProfile retrieves or creates a user behavior profile
func (ba *BehavioralAnalytics) getUserProfile(userID string) *UserBehaviorProfile {
	if profile, exists := ba.userProfiles[userID]; exists {
		return profile
	}

	profile := &UserBehaviorProfile{
		UserID:            userID,
		CreatedAt:         time.Now(),
		LastUpdated:       time.Now(),
		TransactionStats:  &TransactionStats{},
		TradingPatterns:   &TradingPatterns{},
		TimePatterns:      &TimePatterns{},
		GeographicData:    &GeographicData{},
		DeviceFingerprint: &DeviceFingerprint{},
		RiskFactors:       []RiskFactor{},
		AnomalyHistory:    []AnomalyEvent{},
	}

	ba.userProfiles[userID] = profile
	return profile
}

// updateUserProfile updates the user profile with new transaction data
func (ba *BehavioralAnalytics) updateUserProfile(profile *UserBehaviorProfile, transactions []Transaction) {
	if len(transactions) == 0 {
		return
	}

	// Update transaction statistics
	ba.updateTransactionStats(profile.TransactionStats, transactions)

	// Update trading patterns
	ba.updateTradingPatterns(profile.TradingPatterns, transactions)

	// Update time patterns
	ba.updateTimePatterns(profile.TimePatterns, transactions)

	// Update geographic data
	ba.updateGeographicData(profile.GeographicData, transactions)

	// Update device fingerprint
	ba.updateDeviceFingerprint(profile.DeviceFingerprint, transactions)
}

// updateTransactionStats updates transaction statistics
func (ba *BehavioralAnalytics) updateTransactionStats(stats *TransactionStats, transactions []Transaction) {
	if stats == nil {
		stats = &TransactionStats{}
	}

	stats.TotalTransactions += int64(len(transactions))

	amounts := make([]decimal.Decimal, len(transactions))
	totalVolume := decimal.Zero

	for i, tx := range transactions {
		amounts[i] = tx.Amount
		totalVolume = totalVolume.Add(tx.Amount)
	}

	stats.TotalVolume = stats.TotalVolume.Add(totalVolume)

	if len(amounts) > 0 {
		// Calculate statistics
		sort.Slice(amounts, func(i, j int) bool {
			return amounts[i].LessThan(amounts[j])
		})

		stats.MinAmount = amounts[0]
		stats.MaxAmount = amounts[len(amounts)-1]
		stats.MedianAmount = amounts[len(amounts)/2]

		// Calculate average
		if stats.TotalTransactions > 0 {
			stats.AverageAmount = stats.TotalVolume.Div(decimal.NewFromInt(stats.TotalTransactions))
		}

		// Calculate standard deviation
		stats.StandardDeviation = ba.calculateStandardDeviation(amounts, stats.AverageAmount)
	}

	// Update velocity metrics
	ba.updateVelocityMetrics(stats, transactions)
}

// updateTradingPatterns updates trading behavior patterns
func (ba *BehavioralAnalytics) updateTradingPatterns(patterns *TradingPatterns, transactions []Transaction) {
	if patterns.TradingFrequency == nil {
		patterns.TradingFrequency = make(map[string]int64)
	}
	if patterns.OrderSizeDistribution == nil {
		patterns.OrderSizeDistribution = make(map[string]decimal.Decimal)
	}

	marketCounts := make(map[string]int64)

	for _, tx := range transactions {
		if market, ok := tx.Metadata["market"].(string); ok {
			marketCounts[market]++
			patterns.TradingFrequency[market]++
		}
	}

	// Update preferred markets
	patterns.PreferredMarkets = ba.getTopMarkets(marketCounts, 5)
}

// updateTimePatterns updates temporal behavior patterns
func (ba *BehavioralAnalytics) updateTimePatterns(patterns *TimePatterns, transactions []Transaction) {
	hourCounts := make(map[int]int)
	dayCounts := make(map[int]int)

	for _, tx := range transactions {
		hour := tx.Timestamp.Hour()
		day := int(tx.Timestamp.Weekday())

		hourCounts[hour]++
		dayCounts[day]++
	}

	// Update active hours and days
	patterns.ActiveHours = ba.getActiveTimeSlots(hourCounts, 8) // Top 8 hours
	patterns.ActiveDays = ba.getActiveTimeSlots(dayCounts, 7)   // All days with activity
}

// updateGeographicData updates location-based information
func (ba *BehavioralAnalytics) updateGeographicData(geoData *GeographicData, transactions []Transaction) {
	for _, tx := range transactions {
		if ipAddr, ok := tx.Metadata["ip_address"].(string); ok {
			// Add IP address if not already present
			found := false
			for _, existingIP := range geoData.IPAddresses {
				if existingIP == ipAddr {
					found = true
					break
				}
			}
			if !found {
				geoData.IPAddresses = append(geoData.IPAddresses, ipAddr)
			}
		}

		if country, ok := tx.Metadata["country_code"].(string); ok {
			geoData.CountryCode = country
		}
	}
}

// updateDeviceFingerprint updates device identification data
func (ba *BehavioralAnalytics) updateDeviceFingerprint(fingerprint *DeviceFingerprint, transactions []Transaction) {
	for _, tx := range transactions {
		if deviceID, ok := tx.Metadata["device_id"].(string); ok {
			fingerprint.DeviceIDs = ba.addUniqueString(fingerprint.DeviceIDs, deviceID)
		}

		if userAgent, ok := tx.Metadata["user_agent"].(string); ok {
			fingerprint.UserAgents = ba.addUniqueString(fingerprint.UserAgents, userAgent)
		}
	}

	fingerprint.DeviceChanges = len(fingerprint.DeviceIDs)
	fingerprint.SuspiciousDevices = fingerprint.DeviceChanges > ba.anomalyThresholds.DeviceChangeThreshold
}

// detectAnomalies performs anomaly detection on user behavior
func (ba *BehavioralAnalytics) detectAnomalies(profile *UserBehaviorProfile, transactions []Transaction) []AnomalyEvent {
	var anomalies []AnomalyEvent

	// Volume anomalies
	volumeAnomalies := ba.detectVolumeAnomalies(profile, transactions)
	anomalies = append(anomalies, volumeAnomalies...)

	// Frequency anomalies
	frequencyAnomalies := ba.detectFrequencyAnomalies(profile, transactions)
	anomalies = append(anomalies, frequencyAnomalies...)

	// Time pattern anomalies
	timeAnomalies := ba.detectTimeAnomalies(profile, transactions)
	anomalies = append(anomalies, timeAnomalies...)

	// Geographic anomalies
	geoAnomalies := ba.detectGeographicAnomalies(profile, transactions)
	anomalies = append(anomalies, geoAnomalies...)

	// Device anomalies
	deviceAnomalies := ba.detectDeviceAnomalies(profile, transactions)
	anomalies = append(anomalies, deviceAnomalies...)

	return anomalies
}

// detectVolumeAnomalies detects unusual transaction volumes
func (ba *BehavioralAnalytics) detectVolumeAnomalies(profile *UserBehaviorProfile, transactions []Transaction) []AnomalyEvent {
	var anomalies []AnomalyEvent

	if profile.TransactionStats.AverageAmount.IsZero() {
		return anomalies
	}

	threshold := profile.TransactionStats.AverageAmount.Mul(ba.anomalyThresholds.VolumeMultiplier)

	for _, tx := range transactions {
		if tx.Amount.GreaterThan(threshold) {
			anomaly := AnomalyEvent{
				ID:          ba.generateAnomalyID(),
				Type:        "volume_anomaly",
				Description: "Transaction volume significantly exceeds user's typical pattern",
				Severity:    "medium",
				Score:       tx.Amount.Div(profile.TransactionStats.AverageAmount),
				DetectedAt:  time.Now(),
				Metadata: map[string]interface{}{
					"transaction_id": tx.ID,
					"amount":         tx.Amount.String(),
					"average_amount": profile.TransactionStats.AverageAmount.String(),
					"threshold":      threshold.String(),
				},
			}
			anomalies = append(anomalies, anomaly)
		}
	}

	return anomalies
}

// detectFrequencyAnomalies detects unusual transaction frequencies
func (ba *BehavioralAnalytics) detectFrequencyAnomalies(profile *UserBehaviorProfile, transactions []Transaction) []AnomalyEvent {
	var anomalies []AnomalyEvent

	if len(transactions) == 0 || profile.TransactionStats.DailyAverageCount.IsZero() {
		return anomalies
	}

	// Check if current frequency exceeds threshold
	currentFreq := decimal.NewFromInt(int64(len(transactions)))
	threshold := profile.TransactionStats.DailyAverageCount.Mul(ba.anomalyThresholds.FrequencyMultiplier)

	if currentFreq.GreaterThan(threshold) {
		anomaly := AnomalyEvent{
			ID:          ba.generateAnomalyID(),
			Type:        "frequency_anomaly",
			Description: "Transaction frequency significantly exceeds user's typical pattern",
			Severity:    "medium",
			Score:       currentFreq.Div(profile.TransactionStats.DailyAverageCount),
			DetectedAt:  time.Now(),
			Metadata: map[string]interface{}{
				"current_frequency": currentFreq.String(),
				"average_frequency": profile.TransactionStats.DailyAverageCount.String(),
				"threshold":         threshold.String(),
			},
		}
		anomalies = append(anomalies, anomaly)
	}

	return anomalies
}

// detectTimeAnomalies detects unusual timing patterns
func (ba *BehavioralAnalytics) detectTimeAnomalies(profile *UserBehaviorProfile, transactions []Transaction) []AnomalyEvent {
	var anomalies []AnomalyEvent

	if len(profile.TimePatterns.ActiveHours) == 0 {
		return anomalies
	}

	unusualHours := 0
	for _, tx := range transactions {
		hour := tx.Timestamp.Hour()
		isUsual := false
		for _, activeHour := range profile.TimePatterns.ActiveHours {
			if hour == activeHour {
				isUsual = true
				break
			}
		}
		if !isUsual {
			unusualHours++
		}
	}

	if len(transactions) > 0 {
		unusualRatio := decimal.NewFromInt(int64(unusualHours)).Div(decimal.NewFromInt(int64(len(transactions))))
		if unusualRatio.GreaterThan(ba.anomalyThresholds.TimeAnomalyThreshold) {
			anomaly := AnomalyEvent{
				ID:          ba.generateAnomalyID(),
				Type:        "time_anomaly",
				Description: "Transactions occurring outside typical time patterns",
				Severity:    "low",
				Score:       unusualRatio,
				DetectedAt:  time.Now(),
				Metadata: map[string]interface{}{
					"unusual_hours":      unusualHours,
					"total_transactions": len(transactions),
					"unusual_ratio":      unusualRatio.String(),
				},
			}
			anomalies = append(anomalies, anomaly)
		}
	}

	return anomalies
}

// detectGeographicAnomalies detects unusual geographic patterns
func (ba *BehavioralAnalytics) detectGeographicAnomalies(profile *UserBehaviorProfile, transactions []Transaction) []AnomalyEvent {
	var anomalies []AnomalyEvent

	// Check for VPN/Proxy usage
	if profile.GeographicData.VPNDetected || profile.GeographicData.ProxyDetected || profile.GeographicData.TorDetected {
		anomaly := AnomalyEvent{
			ID:          ba.generateAnomalyID(),
			Type:        "geographic_anomaly",
			Description: "VPN/Proxy/Tor usage detected",
			Severity:    "medium",
			Score:       ba.anomalyThresholds.GeographicRiskScore,
			DetectedAt:  time.Now(),
			Metadata: map[string]interface{}{
				"vpn_detected":   profile.GeographicData.VPNDetected,
				"proxy_detected": profile.GeographicData.ProxyDetected,
				"tor_detected":   profile.GeographicData.TorDetected,
			},
		}
		anomalies = append(anomalies, anomaly)
	}

	return anomalies
}

// detectDeviceAnomalies detects unusual device patterns
func (ba *BehavioralAnalytics) detectDeviceAnomalies(profile *UserBehaviorProfile, transactions []Transaction) []AnomalyEvent {
	var anomalies []AnomalyEvent

	if profile.DeviceFingerprint.SuspiciousDevices {
		anomaly := AnomalyEvent{
			ID:          ba.generateAnomalyID(),
			Type:        "device_anomaly",
			Description: "Excessive device changes detected",
			Severity:    "medium",
			Score:       decimal.NewFromInt(int64(profile.DeviceFingerprint.DeviceChanges)),
			DetectedAt:  time.Now(),
			Metadata: map[string]interface{}{
				"device_changes": profile.DeviceFingerprint.DeviceChanges,
				"threshold":      ba.anomalyThresholds.DeviceChangeThreshold,
			},
		}
		anomalies = append(anomalies, anomaly)
	}

	return anomalies
}

// Helper functions

func (ba *BehavioralAnalytics) calculateStandardDeviation(amounts []decimal.Decimal, mean decimal.Decimal) decimal.Decimal {
	if len(amounts) <= 1 {
		return decimal.Zero
	}

	variance := decimal.Zero
	for _, amount := range amounts {
		diff := amount.Sub(mean)
		variance = variance.Add(diff.Mul(diff))
	}

	variance = variance.Div(decimal.NewFromInt(int64(len(amounts) - 1)))

	// Simple square root approximation
	sqrt, _ := decimal.NewFromFloat(math.Sqrt(variance.InexactFloat64())).Float64()
	return decimal.NewFromFloat(sqrt)
}

func (ba *BehavioralAnalytics) updateVelocityMetrics(stats *TransactionStats, transactions []Transaction) {
	if stats.VelocityMetrics == nil {
		stats.VelocityMetrics = &VelocityMetrics{}
	}

	if len(transactions) > 0 {
		duration := transactions[len(transactions)-1].Timestamp.Sub(transactions[0].Timestamp)
		if duration > 0 {
			hours := decimal.NewFromFloat(duration.Hours())
			stats.VelocityMetrics.TransactionsPerHour = decimal.NewFromInt(int64(len(transactions))).Div(hours)

			totalVolume := decimal.Zero
			for _, tx := range transactions {
				totalVolume = totalVolume.Add(tx.Amount)
			}
			stats.VelocityMetrics.VolumePerHour = totalVolume.Div(hours)
		}
	}
}

func (ba *BehavioralAnalytics) getTopMarkets(marketCounts map[string]int64, limit int) []string {
	type marketCount struct {
		market string
		count  int64
	}

	var markets []marketCount
	for market, count := range marketCounts {
		markets = append(markets, marketCount{market, count})
	}

	sort.Slice(markets, func(i, j int) bool {
		return markets[i].count > markets[j].count
	})

	var result []string
	for i, market := range markets {
		if i >= limit {
			break
		}
		result = append(result, market.market)
	}

	return result
}

func (ba *BehavioralAnalytics) getActiveTimeSlots(counts map[int]int, limit int) []int {
	type timeCount struct {
		time  int
		count int
	}

	var times []timeCount
	for t, count := range counts {
		if count > 0 {
			times = append(times, timeCount{t, count})
		}
	}

	sort.Slice(times, func(i, j int) bool {
		return times[i].count > times[j].count
	})

	var result []int
	for i, tc := range times {
		if i >= limit {
			break
		}
		result = append(result, tc.time)
	}

	return result
}

func (ba *BehavioralAnalytics) addUniqueString(slice []string, item string) []string {
	for _, existing := range slice {
		if existing == item {
			return slice
		}
	}
	return append(slice, item)
}

func (ba *BehavioralAnalytics) generateAnomalyID() string {
	return "anomaly_" + time.Now().Format("20060102_150405_000")
}

func (ba *BehavioralAnalytics) calculateRiskFactors(profile *UserBehaviorProfile) []RiskFactor {
	var factors []RiskFactor

	// Add risk factors based on profile analysis
	if profile.DeviceFingerprint.SuspiciousDevices {
		factors = append(factors, RiskFactor{
			Type:        "device_risk",
			Description: "Multiple device changes detected",
			Severity:    "medium",
			Score:       decimal.NewFromFloat(0.6),
			DetectedAt:  time.Now(),
		})
	}

	if profile.GeographicData.VPNDetected {
		factors = append(factors, RiskFactor{
			Type:        "geographic_risk",
			Description: "VPN usage detected",
			Severity:    "medium",
			Score:       decimal.NewFromFloat(0.5),
			DetectedAt:  time.Now(),
		})
	}

	return factors
}

func (ba *BehavioralAnalytics) calculateOverallRiskScore(anomalies []AnomalyEvent, riskFactors []RiskFactor) decimal.Decimal {
	score := decimal.Zero

	for _, anomaly := range anomalies {
		switch anomaly.Severity {
		case "low":
			score = score.Add(decimal.NewFromFloat(0.1))
		case "medium":
			score = score.Add(decimal.NewFromFloat(0.3))
		case "high":
			score = score.Add(decimal.NewFromFloat(0.6))
		case "critical":
			score = score.Add(decimal.NewFromFloat(1.0))
		}
	}

	for _, factor := range riskFactors {
		score = score.Add(factor.Score)
	}

	// Normalize to 0-1 range
	if score.GreaterThan(decimal.NewFromInt(1)) {
		score = decimal.NewFromInt(1)
	}

	return score
}

func (ba *BehavioralAnalytics) generateRecommendations(riskScore decimal.Decimal, anomalies []AnomalyEvent, riskFactors []RiskFactor) []string {
	var recommendations []string

	if riskScore.GreaterThan(decimal.NewFromFloat(0.8)) {
		recommendations = append(recommendations, "Immediate manual review required")
		recommendations = append(recommendations, "Consider account restrictions")
	} else if riskScore.GreaterThan(decimal.NewFromFloat(0.5)) {
		recommendations = append(recommendations, "Enhanced monitoring required")
		recommendations = append(recommendations, "Request additional verification")
	} else if riskScore.GreaterThan(decimal.NewFromFloat(0.3)) {
		recommendations = append(recommendations, "Continued automated monitoring")
	}

	return recommendations
}

func (ba *BehavioralAnalytics) determineTriggerActions(riskScore decimal.Decimal, anomalies []AnomalyEvent) []string {
	var actions []string

	if riskScore.GreaterThan(decimal.NewFromFloat(0.8)) {
		actions = append(actions, "generate_sar")
		actions = append(actions, "freeze_account")
	} else if riskScore.GreaterThan(decimal.NewFromFloat(0.5)) {
		actions = append(actions, "create_investigation_case")
		actions = append(actions, "enhance_monitoring")
	}

	return actions
}

// Transaction represents a transaction for behavioral analysis
type Transaction struct {
	ID        string
	UserID    string
	Type      string
	Amount    decimal.Decimal
	Currency  string
	Timestamp time.Time
	Metadata  map[string]interface{}
}
