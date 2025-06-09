// ml_anomaly_detector.go
// Production ML Anomaly Detector for Compliance Engine
// Implements multiple anomaly detection algorithms and feature engineering
package monitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"go.uber.org/zap"
)

// MLConfig represents configuration for ML anomaly detection
type MLConfig struct {
	// Detection parameters
	AnomalyThreshold        float64            `json:"anomaly_threshold"`         // Default: 0.95
	StatisticalThreshold    float64            `json:"statistical_threshold"`     // Default: 3.0 (z-score)
	MinSamplesForTraining   int                `json:"min_samples_for_training"`  // Default: 100
	FeatureWindowSize       int                `json:"feature_window_size"`       // Default: 100
	ModelRetrainingInterval time.Duration      `json:"model_retraining_interval"` // Default: 1 hour
	EnabledAlgorithms       []string           `json:"enabled_algorithms"`        // isolation_forest, statistical, behavioral, ensemble
	FeatureWeights          map[string]float64 `json:"feature_weights"`           // Custom feature importance weights

	// Behavioral analysis
	UserBehaviorHistoryDays   int           `json:"user_behavior_history_days"`  // Default: 30
	TransactionVelocityWindow time.Duration `json:"transaction_velocity_window"` // Default: 1 hour

	// Model persistence
	ModelSaveInterval    time.Duration `json:"model_save_interval"`    // Default: 24 hours
	ModelBackupRetention int           `json:"model_backup_retention"` // Default: 7 days
}

// DefaultMLConfig returns default configuration
func DefaultMLConfig() *MLConfig {
	return &MLConfig{
		AnomalyThreshold:        0.95,
		StatisticalThreshold:    3.0,
		MinSamplesForTraining:   100,
		FeatureWindowSize:       100,
		ModelRetrainingInterval: time.Hour,
		EnabledAlgorithms:       []string{"statistical", "behavioral", "ensemble"},
		FeatureWeights: map[string]float64{
			"amount_zscore":      1.0,
			"frequency_zscore":   0.8,
			"velocity_score":     0.9,
			"time_pattern_score": 0.7,
			"behavior_deviation": 1.2,
		},
		UserBehaviorHistoryDays:   30,
		TransactionVelocityWindow: time.Hour,
		ModelSaveInterval:         24 * time.Hour,
		ModelBackupRetention:      7,
	}
}

// UserBehaviorProfile represents user's historical behavior patterns
type UserBehaviorProfile struct {
	UserID               string         `json:"user_id"`
	AvgTransactionAmount float64        `json:"avg_transaction_amount"`
	StdTransactionAmount float64        `json:"std_transaction_amount"`
	TransactionFrequency float64        `json:"transaction_frequency"` // per day
	CommonEventTypes     map[string]int `json:"common_event_types"`
	PreferredTimeWindows []TimeWindow   `json:"preferred_time_windows"`
	GeographicPatterns   []string       `json:"geographic_patterns"`
	LastUpdated          time.Time      `json:"last_updated"`
	SampleCount          int            `json:"sample_count"`
}

// TimeWindow represents a time pattern
type TimeWindow struct {
	StartHour int     `json:"start_hour"`
	EndHour   int     `json:"end_hour"`
	Weight    float64 `json:"weight"`
}

// FeatureVector represents extracted features from an event
type FeatureVector struct {
	// Amount-based features
	Amount           float64 `json:"amount"`
	AmountZScore     float64 `json:"amount_zscore"`
	AmountPercentile float64 `json:"amount_percentile"`

	// Temporal features
	HourOfDay        int     `json:"hour_of_day"`
	DayOfWeek        int     `json:"day_of_week"`
	TimePatternScore float64 `json:"time_pattern_score"`

	// Frequency features
	FrequencyZScore float64 `json:"frequency_zscore"`
	VelocityScore   float64 `json:"velocity_score"`

	// Behavioral features
	BehaviorDeviation  float64 `json:"behavior_deviation"`
	EventTypeFrequency float64 `json:"event_type_frequency"`

	// Context features
	EventType      string                 `json:"event_type"`
	UserID         string                 `json:"user_id"`
	AdditionalData map[string]interface{} `json:"additional_data"`
}

// AnomalyResult represents the result of anomaly detection
type AnomalyResult struct {
	IsAnomaly        bool               `json:"is_anomaly"`
	AnomalyScore     float64            `json:"anomaly_score"`
	Features         FeatureVector      `json:"features"`
	DetectionMethods map[string]float64 `json:"detection_methods"`
	Explanation      string             `json:"explanation"`
	Confidence       float64            `json:"confidence"`
	Timestamp        time.Time          `json:"timestamp"`
}

// AdvancedMLAnomalyDetector implements production-ready ML anomaly detection
type AdvancedMLAnomalyDetector struct {
	config *MLConfig
	logger *zap.Logger

	// User behavior tracking
	userProfiles  map[string]*UserBehaviorProfile
	profilesMutex sync.RWMutex

	// Historical data for statistical analysis
	transactionHistory []ComplianceEvent
	historyMutex       sync.RWMutex

	// Feature extraction and normalization
	featureStats map[string]FeatureStats
	statsMutex   sync.RWMutex

	// Model state
	modelVersion    int
	lastModelUpdate time.Time
	isModelTrained  bool

	// Background processing
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// FeatureStats represents statistical information about features
type FeatureStats struct {
	Mean   float64 `json:"mean"`
	StdDev float64 `json:"std_dev"`
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
	Count  int     `json:"count"`
}

// NewAdvancedMLAnomalyDetector creates a new production ML anomaly detector
func NewAdvancedMLAnomalyDetector(config *MLConfig, logger *zap.Logger) *AdvancedMLAnomalyDetector {
	if config == nil {
		config = DefaultMLConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	detector := &AdvancedMLAnomalyDetector{
		config:             config,
		logger:             logger,
		userProfiles:       make(map[string]*UserBehaviorProfile),
		transactionHistory: make([]ComplianceEvent, 0, config.FeatureWindowSize*2),
		featureStats:       make(map[string]FeatureStats),
		modelVersion:       1,
		lastModelUpdate:    time.Now(),
		isModelTrained:     false,
		ctx:                ctx,
		cancel:             cancel,
	}

	// Start background processes
	detector.wg.Add(2)
	go detector.modelRetrainingWorker()
	go detector.profileUpdateWorker()

	logger.Info("Advanced ML Anomaly Detector initialized",
		zap.Any("config", config),
		zap.Int("model_version", detector.modelVersion))

	return detector
}

// DetectAnomaly performs comprehensive anomaly detection on a compliance event
func (d *AdvancedMLAnomalyDetector) DetectAnomaly(event *ComplianceEvent) (bool, float64, map[string]interface{}) {
	startTime := time.Now()

	// Extract comprehensive features
	features := d.extractFeatures(event)

	// Perform multi-algorithm anomaly detection
	result := d.performAnomalyDetection(features)

	// Update user profile and historical data
	d.updateUserProfile(event, features)
	d.addToHistory(event)

	// Prepare response in the expected format
	featuresMap := d.featureVectorToMap(features)
	featuresMap["detection_methods"] = result.DetectionMethods
	featuresMap["explanation"] = result.Explanation
	featuresMap["confidence"] = result.Confidence
	featuresMap["model_version"] = d.modelVersion
	featuresMap["processing_time_ms"] = float64(time.Since(startTime).Nanoseconds()) / 1e6

	d.logger.Debug("Anomaly detection completed",
		zap.String("user_id", event.UserID),
		zap.String("event_type", event.EventType),
		zap.Bool("is_anomaly", result.IsAnomaly),
		zap.Float64("score", result.AnomalyScore),
		zap.Duration("processing_time", time.Since(startTime)))

	return result.IsAnomaly, result.AnomalyScore, featuresMap
}

// extractFeatures performs comprehensive feature extraction
func (d *AdvancedMLAnomalyDetector) extractFeatures(event *ComplianceEvent) FeatureVector {
	features := FeatureVector{
		Amount:         event.Amount,
		EventType:      event.EventType,
		UserID:         event.UserID,
		AdditionalData: event.Details,
	}

	// Temporal features
	features.HourOfDay = event.Timestamp.Hour()
	features.DayOfWeek = int(event.Timestamp.Weekday())

	// Calculate amount-based features
	d.calculateAmountFeatures(event, &features)

	// Calculate temporal pattern features
	d.calculateTemporalFeatures(event, &features)

	// Calculate frequency and velocity features
	d.calculateFrequencyFeatures(event, &features)

	// Calculate behavioral deviation features
	d.calculateBehavioralFeatures(event, &features)

	return features
}

// calculateAmountFeatures calculates amount-related anomaly features
func (d *AdvancedMLAnomalyDetector) calculateAmountFeatures(event *ComplianceEvent, features *FeatureVector) {
	d.statsMutex.RLock()
	defer d.statsMutex.RUnlock()

	if stats, exists := d.featureStats["amount"]; exists && stats.Count > 0 {
		if stats.StdDev > 0 {
			features.AmountZScore = math.Abs((event.Amount - stats.Mean) / stats.StdDev)
		}

		// Calculate percentile rank
		features.AmountPercentile = d.calculatePercentile(event.Amount, "amount")
	}
}

// calculateTemporalFeatures calculates time-based pattern features
func (d *AdvancedMLAnomalyDetector) calculateTemporalFeatures(event *ComplianceEvent, features *FeatureVector) {
	d.profilesMutex.RLock()
	profile, exists := d.userProfiles[event.UserID]
	d.profilesMutex.RUnlock()

	if !exists || len(profile.PreferredTimeWindows) == 0 {
		features.TimePatternScore = 0.5 // Neutral score for new users
		return
	}

	currentHour := event.Timestamp.Hour()
	maxScore := 0.0

	for _, window := range profile.PreferredTimeWindows {
		if d.isInTimeWindow(currentHour, window) {
			if window.Weight > maxScore {
				maxScore = window.Weight
			}
		}
	}

	features.TimePatternScore = maxScore
}

// calculateFrequencyFeatures calculates transaction frequency and velocity features
func (d *AdvancedMLAnomalyDetector) calculateFrequencyFeatures(event *ComplianceEvent, features *FeatureVector) {
	// Calculate recent transaction velocity for this user
	recentCount := d.countRecentTransactions(event.UserID, d.config.TransactionVelocityWindow)

	d.profilesMutex.RLock()
	profile, exists := d.userProfiles[event.UserID]
	d.profilesMutex.RUnlock()

	if exists && profile.TransactionFrequency > 0 {
		expectedCount := profile.TransactionFrequency * (float64(d.config.TransactionVelocityWindow) / float64(24*time.Hour))
		features.VelocityScore = float64(recentCount) / math.Max(expectedCount, 1.0)

		if expectedCount > 0 {
			features.FrequencyZScore = math.Abs((float64(recentCount) - expectedCount) / math.Sqrt(expectedCount))
		}
	} else {
		features.VelocityScore = 0.5 // Neutral for new users
		features.FrequencyZScore = 0.0
	}
}

// calculateBehavioralFeatures calculates behavioral deviation features
func (d *AdvancedMLAnomalyDetector) calculateBehavioralFeatures(event *ComplianceEvent, features *FeatureVector) {
	d.profilesMutex.RLock()
	profile, exists := d.userProfiles[event.UserID]
	d.profilesMutex.RUnlock()

	if !exists {
		features.BehaviorDeviation = 0.0
		features.EventTypeFrequency = 0.0
		return
	}

	// Calculate behavioral deviation based on amount patterns
	amountDeviation := 0.0
	if profile.StdTransactionAmount > 0 {
		amountDeviation = math.Abs((event.Amount - profile.AvgTransactionAmount) / profile.StdTransactionAmount)
	}

	// Calculate event type frequency deviation
	totalEvents := 0
	for _, count := range profile.CommonEventTypes {
		totalEvents += count
	}

	eventTypeFreq := 0.0
	if count, exists := profile.CommonEventTypes[event.EventType]; exists && totalEvents > 0 {
		eventTypeFreq = float64(count) / float64(totalEvents)
	}

	features.BehaviorDeviation = amountDeviation
	features.EventTypeFrequency = eventTypeFreq
}

// performAnomalyDetection runs multiple detection algorithms and combines results
func (d *AdvancedMLAnomalyDetector) performAnomalyDetection(features FeatureVector) AnomalyResult {
	result := AnomalyResult{
		Features:         features,
		DetectionMethods: make(map[string]float64),
		Timestamp:        time.Now(),
	}

	scores := make([]float64, 0, len(d.config.EnabledAlgorithms))
	explanations := make([]string, 0)

	// Statistical anomaly detection
	if d.containsAlgorithm("statistical") {
		score, explanation := d.statisticalAnomalyDetection(features)
		result.DetectionMethods["statistical"] = score
		scores = append(scores, score)
		if score > d.config.StatisticalThreshold {
			explanations = append(explanations, explanation)
		}
	}

	// Behavioral anomaly detection
	if d.containsAlgorithm("behavioral") {
		score, explanation := d.behavioralAnomalyDetection(features)
		result.DetectionMethods["behavioral"] = score
		scores = append(scores, score)
		if score > d.config.AnomalyThreshold {
			explanations = append(explanations, explanation)
		}
	}

	// Ensemble detection (combines multiple methods)
	if d.containsAlgorithm("ensemble") {
		score, explanation := d.ensembleAnomalyDetection(features, result.DetectionMethods)
		result.DetectionMethods["ensemble"] = score
		scores = append(scores, score)
		if score > d.config.AnomalyThreshold {
			explanations = append(explanations, explanation)
		}
	}

	// Calculate final anomaly score and decision
	if len(scores) > 0 {
		result.AnomalyScore = d.calculateWeightedScore(result.DetectionMethods)
		result.IsAnomaly = result.AnomalyScore > d.config.AnomalyThreshold
		result.Confidence = d.calculateConfidence(scores)
	}

	// Create explanation
	if len(explanations) > 0 {
		result.Explanation = fmt.Sprintf("Anomaly detected: %v", explanations)
	} else {
		result.Explanation = "Normal behavior pattern"
	}

	return result
}

// statisticalAnomalyDetection performs statistical analysis for anomaly detection
func (d *AdvancedMLAnomalyDetector) statisticalAnomalyDetection(features FeatureVector) (float64, string) {
	anomalyScore := 0.0
	reasons := make([]string, 0)

	// Amount Z-score analysis
	if features.AmountZScore > d.config.StatisticalThreshold {
		anomalyScore += features.AmountZScore / 10.0 // Normalize
		reasons = append(reasons, fmt.Sprintf("unusual amount (z-score: %.2f)", features.AmountZScore))
	}

	// Frequency Z-score analysis
	if features.FrequencyZScore > d.config.StatisticalThreshold {
		anomalyScore += features.FrequencyZScore / 10.0
		reasons = append(reasons, fmt.Sprintf("unusual frequency (z-score: %.2f)", features.FrequencyZScore))
	}

	// Velocity analysis
	if features.VelocityScore > 3.0 { // 3x normal velocity
		anomalyScore += (features.VelocityScore - 1.0) / 5.0
		reasons = append(reasons, fmt.Sprintf("high transaction velocity (%.2fx normal)", features.VelocityScore))
	}

	explanation := ""
	if len(reasons) > 0 {
		explanation = fmt.Sprintf("Statistical anomalies: %v", reasons)
	}

	return math.Min(anomalyScore, 1.0), explanation
}

// behavioralAnomalyDetection performs behavioral pattern analysis
func (d *AdvancedMLAnomalyDetector) behavioralAnomalyDetection(features FeatureVector) (float64, string) {
	anomalyScore := 0.0
	reasons := make([]string, 0)

	// Behavioral deviation analysis
	if features.BehaviorDeviation > 2.0 {
		anomalyScore += math.Min(features.BehaviorDeviation/5.0, 0.4)
		reasons = append(reasons, "unusual transaction pattern")
	}

	// Time pattern analysis
	if features.TimePatternScore < 0.1 { // Very unusual time
		anomalyScore += 0.3
		reasons = append(reasons, "unusual time pattern")
	}

	// Event type frequency analysis
	if features.EventTypeFrequency < 0.05 { // Very rare event type for this user
		anomalyScore += 0.2
		reasons = append(reasons, "unusual event type for user")
	}

	explanation := ""
	if len(reasons) > 0 {
		explanation = fmt.Sprintf("Behavioral anomalies: %v", reasons)
	}

	return math.Min(anomalyScore, 1.0), explanation
}

// ensembleAnomalyDetection combines multiple detection methods
func (d *AdvancedMLAnomalyDetector) ensembleAnomalyDetection(features FeatureVector, methods map[string]float64) (float64, string) {
	weights := map[string]float64{
		"statistical": 0.6,
		"behavioral":  0.4,
	}

	weightedSum := 0.0
	totalWeight := 0.0

	for method, score := range methods {
		if weight, exists := weights[method]; exists {
			weightedSum += score * weight
			totalWeight += weight
		}
	}

	ensembleScore := 0.0
	if totalWeight > 0 {
		ensembleScore = weightedSum / totalWeight
	}

	explanation := fmt.Sprintf("Ensemble score combining %d methods", len(methods))
	return ensembleScore, explanation
}

// Helper methods

func (d *AdvancedMLAnomalyDetector) calculateWeightedScore(methods map[string]float64) float64 {
	weightedSum := 0.0
	totalWeight := 0.0

	for feature, score := range methods {
		weight := 1.0
		if w, exists := d.config.FeatureWeights[feature]; exists {
			weight = w
		}
		weightedSum += score * weight
		totalWeight += weight
	}

	if totalWeight > 0 {
		return weightedSum / totalWeight
	}
	return 0.0
}

func (d *AdvancedMLAnomalyDetector) calculateConfidence(scores []float64) float64 {
	if len(scores) == 0 {
		return 0.0
	}

	// Calculate standard deviation of scores as confidence measure
	mean := 0.0
	for _, score := range scores {
		mean += score
	}
	mean /= float64(len(scores))

	variance := 0.0
	for _, score := range scores {
		variance += (score - mean) * (score - mean)
	}
	variance /= float64(len(scores))

	// Lower variance means higher confidence
	confidence := 1.0 / (1.0 + math.Sqrt(variance))
	return confidence
}

func (d *AdvancedMLAnomalyDetector) containsAlgorithm(algorithm string) bool {
	for _, enabled := range d.config.EnabledAlgorithms {
		if enabled == algorithm {
			return true
		}
	}
	return false
}

func (d *AdvancedMLAnomalyDetector) calculatePercentile(value float64, featureName string) float64 {
	d.historyMutex.RLock()
	defer d.historyMutex.RUnlock()

	values := make([]float64, 0, len(d.transactionHistory))
	for _, event := range d.transactionHistory {
		switch featureName {
		case "amount":
			values = append(values, event.Amount)
		}
	}

	if len(values) == 0 {
		return 0.5
	}

	sort.Float64s(values)

	// Find percentile
	count := 0
	for _, v := range values {
		if v <= value {
			count++
		}
	}

	return float64(count) / float64(len(values))
}

func (d *AdvancedMLAnomalyDetector) isInTimeWindow(hour int, window TimeWindow) bool {
	if window.StartHour <= window.EndHour {
		return hour >= window.StartHour && hour <= window.EndHour
	} else {
		// Window crosses midnight
		return hour >= window.StartHour || hour <= window.EndHour
	}
}

func (d *AdvancedMLAnomalyDetector) countRecentTransactions(userID string, window time.Duration) int {
	d.historyMutex.RLock()
	defer d.historyMutex.RUnlock()

	cutoff := time.Now().Add(-window)
	count := 0

	for _, event := range d.transactionHistory {
		if event.UserID == userID && event.Timestamp.After(cutoff) {
			count++
		}
	}

	return count
}

func (d *AdvancedMLAnomalyDetector) featureVectorToMap(features FeatureVector) map[string]interface{} {
	data, _ := json.Marshal(features)
	result := make(map[string]interface{})
	json.Unmarshal(data, &result)
	return result
}

// Background processing methods

func (d *AdvancedMLAnomalyDetector) updateUserProfile(event *ComplianceEvent, features FeatureVector) {
	d.profilesMutex.Lock()
	defer d.profilesMutex.Unlock()

	profile, exists := d.userProfiles[event.UserID]
	if !exists {
		profile = &UserBehaviorProfile{
			UserID:               event.UserID,
			CommonEventTypes:     make(map[string]int),
			PreferredTimeWindows: make([]TimeWindow, 0),
			LastUpdated:          time.Now(),
		}
		d.userProfiles[event.UserID] = profile
	}

	// Update transaction statistics
	oldCount := float64(profile.SampleCount)
	newCount := oldCount + 1

	profile.AvgTransactionAmount = (profile.AvgTransactionAmount*oldCount + event.Amount) / newCount

	// Update variance for standard deviation
	if profile.SampleCount > 0 {
		oldVariance := profile.StdTransactionAmount * profile.StdTransactionAmount
		newMean := profile.AvgTransactionAmount
		oldMean := (newMean*newCount - event.Amount) / oldCount

		newVariance := (oldVariance*oldCount + (event.Amount-oldMean)*(event.Amount-newMean)) / newCount
		profile.StdTransactionAmount = math.Sqrt(newVariance)
	}

	// Update event type frequency
	profile.CommonEventTypes[event.EventType]++

	// Update time windows (simplified approach)
	hour := event.Timestamp.Hour()
	d.updateTimeWindows(profile, hour)

	profile.SampleCount = int(newCount)
	profile.LastUpdated = time.Now()
}

func (d *AdvancedMLAnomalyDetector) updateTimeWindows(profile *UserBehaviorProfile, hour int) {
	// Find existing time window or create new one
	for i := range profile.PreferredTimeWindows {
		window := &profile.PreferredTimeWindows[i]
		if d.isInTimeWindow(hour, *window) {
			window.Weight = math.Min(window.Weight+0.1, 1.0)
			return
		}
	}

	// Create new time window
	if len(profile.PreferredTimeWindows) < 5 { // Limit to 5 windows
		newWindow := TimeWindow{
			StartHour: hour - 1,
			EndHour:   hour + 1,
			Weight:    0.3,
		}
		if newWindow.StartHour < 0 {
			newWindow.StartHour = 23
		}
		if newWindow.EndHour > 23 {
			newWindow.EndHour = 0
		}
		profile.PreferredTimeWindows = append(profile.PreferredTimeWindows, newWindow)
	}
}

func (d *AdvancedMLAnomalyDetector) addToHistory(event *ComplianceEvent) {
	d.historyMutex.Lock()
	defer d.historyMutex.Unlock()

	d.transactionHistory = append(d.transactionHistory, *event)

	// Maintain maximum size
	if len(d.transactionHistory) > d.config.FeatureWindowSize*2 {
		d.transactionHistory = d.transactionHistory[len(d.transactionHistory)-d.config.FeatureWindowSize:]
	}

	// Update feature statistics
	d.updateFeatureStats()
}

func (d *AdvancedMLAnomalyDetector) updateFeatureStats() {
	d.statsMutex.Lock()
	defer d.statsMutex.Unlock()

	if len(d.transactionHistory) == 0 {
		return
	}

	// Calculate amount statistics
	amounts := make([]float64, len(d.transactionHistory))
	for i, event := range d.transactionHistory {
		amounts[i] = event.Amount
	}

	d.featureStats["amount"] = d.calculateStats(amounts)
}

func (d *AdvancedMLAnomalyDetector) calculateStats(values []float64) FeatureStats {
	if len(values) == 0 {
		return FeatureStats{}
	}

	// Calculate mean
	sum := 0.0
	min := values[0]
	max := values[0]

	for _, v := range values {
		sum += v
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}

	mean := sum / float64(len(values))

	// Calculate standard deviation
	variance := 0.0
	for _, v := range values {
		variance += (v - mean) * (v - mean)
	}
	variance /= float64(len(values))
	stddev := math.Sqrt(variance)

	return FeatureStats{
		Mean:   mean,
		StdDev: stddev,
		Min:    min,
		Max:    max,
		Count:  len(values),
	}
}

// Background workers

func (d *AdvancedMLAnomalyDetector) modelRetrainingWorker() {
	defer d.wg.Done()

	ticker := time.NewTicker(d.config.ModelRetrainingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			d.retrainModel()
		}
	}
}

func (d *AdvancedMLAnomalyDetector) profileUpdateWorker() {
	defer d.wg.Done()

	ticker := time.NewTicker(time.Minute * 10) // Clean up every 10 minutes
	defer ticker.Stop()

	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			d.cleanupOldProfiles()
		}
	}
}

func (d *AdvancedMLAnomalyDetector) retrainModel() {
	d.logger.Info("Starting model retraining",
		zap.Int("history_size", len(d.transactionHistory)),
		zap.Int("user_profiles", len(d.userProfiles)))

	// Update feature statistics
	d.historyMutex.Lock()
	d.updateFeatureStats()
	d.historyMutex.Unlock()

	// Increment model version
	d.modelVersion++
	d.lastModelUpdate = time.Now()
	d.isModelTrained = len(d.transactionHistory) >= d.config.MinSamplesForTraining

	d.logger.Info("Model retraining completed",
		zap.Int("new_model_version", d.modelVersion),
		zap.Bool("is_trained", d.isModelTrained))
}

func (d *AdvancedMLAnomalyDetector) cleanupOldProfiles() {
	d.profilesMutex.Lock()
	defer d.profilesMutex.Unlock()

	cutoff := time.Now().AddDate(0, 0, -d.config.UserBehaviorHistoryDays)
	removed := 0

	for userID, profile := range d.userProfiles {
		if profile.LastUpdated.Before(cutoff) {
			delete(d.userProfiles, userID)
			removed++
		}
	}

	if removed > 0 {
		d.logger.Info("Cleaned up old user profiles", zap.Int("removed", removed))
	}
}

// Shutdown gracefully stops the detector
func (d *AdvancedMLAnomalyDetector) Shutdown() {
	d.logger.Info("Shutting down ML Anomaly Detector")
	d.cancel()
	d.wg.Wait()
}

// Legacy interface implementation for backward compatibility
type BuiltinMLAnomalyDetector struct {
	advanced *AdvancedMLAnomalyDetector
}

// NewBuiltinMLAnomalyDetector creates a legacy detector that wraps the advanced one
func NewBuiltinMLAnomalyDetector(logger *zap.Logger) *BuiltinMLAnomalyDetector {
	return &BuiltinMLAnomalyDetector{
		advanced: NewAdvancedMLAnomalyDetector(DefaultMLConfig(), logger),
	}
}

func (d *BuiltinMLAnomalyDetector) DetectAnomaly(event *ComplianceEvent) (bool, float64, map[string]interface{}) {
	return d.advanced.DetectAnomaly(event)
}
