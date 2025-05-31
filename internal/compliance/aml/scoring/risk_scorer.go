package scoring

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/compliance/aml"
	"github.com/Aidin1998/pincex_unified/internal/risk"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// RiskScorer integrates with existing risk calculation to provide AML-specific scoring
type RiskScorer struct {
	mu          sync.RWMutex
	logger      *zap.SugaredLogger
	riskService risk.RiskService
	calculator  *risk.RiskCalculator

	// Scoring models
	models map[string]*ScoringModel

	// Historical data for model improvement
	scoringHistory map[string]*ScoringHistory
}

// ScoringModel defines a risk scoring model
type ScoringModel struct {
	ID              string                 `json:"id"`
	Name            string                 `json:"name"`
	Version         string                 `json:"version"`
	ModelType       string                 `json:"model_type"` // "rule_based", "ml", "hybrid"
	Weights         map[string]float64     `json:"weights"`
	Thresholds      map[string]float64     `json:"thresholds"`
	Parameters      map[string]interface{} `json:"parameters"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
	IsActive        bool                   `json:"is_active"`
	AccuracyMetrics *AccuracyMetrics       `json:"accuracy_metrics"`
}

// AccuracyMetrics tracks model performance
type AccuracyMetrics struct {
	TotalPredictions   int       `json:"total_predictions"`
	CorrectPredictions int       `json:"correct_predictions"`
	Accuracy           float64   `json:"accuracy"`
	Precision          float64   `json:"precision"`
	Recall             float64   `json:"recall"`
	F1Score            float64   `json:"f1_score"`
	LastUpdated        time.Time `json:"last_updated"`
}

// ScoringHistory tracks historical scoring data
type ScoringHistory struct {
	UserID         uuid.UUID              `json:"user_id"`
	Scores         []HistoricalScore      `json:"scores"`
	RiskTrend      string                 `json:"risk_trend"` // "increasing", "decreasing", "stable"
	AverageScore   float64                `json:"average_score"`
	MaxScore       float64                `json:"max_score"`
	LastScored     time.Time              `json:"last_scored"`
	ValidationData map[string]interface{} `json:"validation_data"`
}

// HistoricalScore represents a historical risk score
type HistoricalScore struct {
	Score     float64                `json:"score"`
	RiskLevel aml.RiskLevel          `json:"risk_level"`
	Factors   map[string]float64     `json:"factors"`
	ModelUsed string                 `json:"model_used"`
	ScoredAt  time.Time              `json:"scored_at"`
	Context   map[string]interface{} `json:"context"`
}

// NewRiskScorer creates a new AML risk scorer
func NewRiskScorer(riskService risk.RiskService, logger *zap.SugaredLogger) *RiskScorer {
	scorer := &RiskScorer{
		logger:         logger,
		riskService:    riskService,
		models:         make(map[string]*ScoringModel),
		scoringHistory: make(map[string]*ScoringHistory),
	}

	// Initialize default scoring models
	scorer.initializeDefaultModels()

	return scorer
}

// initializeDefaultModels sets up default AML scoring models
func (rs *RiskScorer) initializeDefaultModels() {
	// Rule-based model integrating with existing risk calculator
	ruleBasedModel := &ScoringModel{
		ID:        "aml_rule_based_v1",
		Name:      "AML Rule-Based Scoring Model",
		Version:   "1.0",
		ModelType: "rule_based",
		Weights: map[string]float64{
			"transaction_volume":    0.25,
			"transaction_frequency": 0.20,
			"geographic_risk":       0.15,
			"customer_profile":      0.20,
			"behavior_anomaly":      0.20,
		},
		Thresholds: map[string]float64{
			"low_risk":      25.0,
			"medium_risk":   50.0,
			"high_risk":     75.0,
			"critical_risk": 90.0,
		},
		Parameters: map[string]interface{}{
			"lookback_days":       30,
			"velocity_weight":     1.5,
			"pattern_multiplier":  2.0,
			"jurisdiction_factor": 1.2,
		},
		IsActive:  true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		AccuracyMetrics: &AccuracyMetrics{
			LastUpdated: time.Now(),
		},
	}

	rs.models[ruleBasedModel.ID] = ruleBasedModel
}

// CalculateRiskScore calculates AML risk score using existing risk service integration
func (rs *RiskScorer) CalculateRiskScore(ctx context.Context, userID uuid.UUID, amlUser *aml.AMLUser) (*AMLRiskScore, error) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	// Get active scoring model
	model := rs.getActiveModel()
	if model == nil {
		return nil, fmt.Errorf("no active scoring model found")
	}

	// Calculate base risk using existing risk service
	riskFactors := map[string]interface{}{
		"user_id":         userID.String(),
		"kyc_status":      amlUser.KYCStatus,
		"country_code":    amlUser.CountryCode,
		"risk_level":      string(amlUser.RiskLevel),
		"pep_status":      amlUser.PEPStatus,
		"sanction_status": amlUser.SanctionStatus,
	}

	// Use existing risk calculation if available
	var baseRiskScore float64 = amlUser.RiskScore

	// Apply AML-specific scoring factors
	score := rs.applyAMLFactors(model, amlUser, baseRiskScore)

	// Determine risk level based on score
	riskLevel := rs.determineRiskLevel(model, score)

	// Create risk score result
	riskScore := &AMLRiskScore{
		UserID:       userID,
		Score:        score,
		RiskLevel:    riskLevel,
		ModelID:      model.ID,
		Factors:      rs.calculateFactorBreakdown(model, amlUser, baseRiskScore),
		CalculatedAt: time.Now(),
		ExpiresAt:    time.Now().Add(24 * time.Hour), // Scores valid for 24 hours
		Metadata: map[string]interface{}{
			"base_risk_score":     baseRiskScore,
			"model_version":       model.Version,
			"calculation_context": riskFactors,
		},
	}

	// Update scoring history
	rs.updateScoringHistory(userID, riskScore, model)

	rs.logger.Infow("AML risk score calculated",
		"user_id", userID,
		"score", score,
		"risk_level", riskLevel,
		"model", model.ID,
	)

	return riskScore, nil
}

// applyAMLFactors applies AML-specific factors to base risk score
func (rs *RiskScorer) applyAMLFactors(model *ScoringModel, amlUser *aml.AMLUser, baseScore float64) float64 {
	score := baseScore

	// Apply geographic risk factors
	if amlUser.IsHighRiskCountry {
		score *= model.Weights["geographic_risk"] + 1.0
	}

	// Apply PEP status factor
	if amlUser.PEPStatus {
		score *= 1.3
	}

	// Apply sanction status factor
	if amlUser.SanctionStatus {
		score *= 2.0 // High multiplier for sanctioned entities
	}

	// Apply blacklist/whitelist factors
	if amlUser.IsBlacklisted {
		score = 100.0 // Maximum risk score
	} else if amlUser.IsWhitelisted {
		score *= 0.5 // Reduce risk for whitelisted users
	}

	// Apply customer type factors
	customerTypeMultiplier := rs.getCustomerTypeMultiplier(amlUser.CustomerType)
	score *= customerTypeMultiplier

	// Ensure score is within bounds
	if score > 100.0 {
		score = 100.0
	} else if score < 0.0 {
		score = 0.0
	}

	return score
}

// getCustomerTypeMultiplier returns risk multiplier based on customer type
func (rs *RiskScorer) getCustomerTypeMultiplier(customerType string) float64 {
	multipliers := map[string]float64{
		"individual":            1.0,
		"corporate":             1.1,
		"trust":                 1.2,
		"foundation":            1.3,
		"government":            0.8,
		"financial_institution": 0.9,
	}

	if multiplier, exists := multipliers[customerType]; exists {
		return multiplier
	}
	return 1.0 // Default multiplier
}

// calculateFactorBreakdown calculates detailed factor breakdown
func (rs *RiskScorer) calculateFactorBreakdown(model *ScoringModel, amlUser *aml.AMLUser, baseScore float64) map[string]float64 {
	factors := make(map[string]float64)

	factors["base_risk"] = baseScore
	factors["geographic_risk"] = rs.calculateGeographicRiskFactor(amlUser)
	factors["customer_profile"] = rs.calculateCustomerProfileFactor(amlUser)
	factors["compliance_status"] = rs.calculateComplianceStatusFactor(amlUser)
	factors["historical_behavior"] = rs.calculateHistoricalBehaviorFactor(amlUser)

	return factors
}

// calculateGeographicRiskFactor calculates geographic risk component
func (rs *RiskScorer) calculateGeographicRiskFactor(amlUser *aml.AMLUser) float64 {
	score := 0.0

	if amlUser.IsHighRiskCountry {
		score += 25.0
	}

	// Add more sophisticated geographic risk logic here
	// Could integrate with sanctions lists, FATF lists, etc.

	return score
}

// calculateCustomerProfileFactor calculates customer profile risk component
func (rs *RiskScorer) calculateCustomerProfileFactor(amlUser *aml.AMLUser) float64 {
	score := 0.0

	if amlUser.PEPStatus {
		score += 20.0
	}

	// Business type risk factors
	businessTypeRisk := map[string]float64{
		"money_services":  15.0,
		"casino":          12.0,
		"crypto_exchange": 10.0,
		"jewelry":         8.0,
		"real_estate":     6.0,
	}

	if risk, exists := businessTypeRisk[amlUser.BusinessType]; exists {
		score += risk
	}

	return score
}

// calculateComplianceStatusFactor calculates compliance status risk component
func (rs *RiskScorer) calculateComplianceStatusFactor(amlUser *aml.AMLUser) float64 {
	score := 0.0

	// KYC status impact
	switch amlUser.KYCStatus {
	case "verified":
		score -= 5.0 // Reduce risk for verified customers
	case "pending":
		score += 10.0
	case "failed":
		score += 25.0
	case "incomplete":
		score += 15.0
	}

	if amlUser.SanctionStatus {
		score += 50.0 // High impact for sanctions
	}

	return score
}

// calculateHistoricalBehaviorFactor calculates historical behavior risk component
func (rs *RiskScorer) calculateHistoricalBehaviorFactor(amlUser *aml.AMLUser) float64 {
	// This would integrate with transaction history and behavior analysis
	// For now, use the existing risk score as a proxy
	return amlUser.RiskScore * 0.3
}

// determineRiskLevel determines risk level based on score and model thresholds
func (rs *RiskScorer) determineRiskLevel(model *ScoringModel, score float64) aml.RiskLevel {
	if score >= model.Thresholds["critical_risk"] {
		return aml.RiskLevelCritical
	} else if score >= model.Thresholds["high_risk"] {
		return aml.RiskLevelHigh
	} else if score >= model.Thresholds["medium_risk"] {
		return aml.RiskLevelMedium
	}
	return aml.RiskLevelLow
}

// getActiveModel returns the currently active scoring model
func (rs *RiskScorer) getActiveModel() *ScoringModel {
	for _, model := range rs.models {
		if model.IsActive {
			return model
		}
	}
	return nil
}

// updateScoringHistory updates historical scoring data
func (rs *RiskScorer) updateScoringHistory(userID uuid.UUID, score *AMLRiskScore, model *ScoringModel) {
	key := userID.String()

	history, exists := rs.scoringHistory[key]
	if !exists {
		history = &ScoringHistory{
			UserID: userID,
			Scores: make([]HistoricalScore, 0),
		}
		rs.scoringHistory[key] = history
	}

	// Add new score to history
	historicalScore := HistoricalScore{
		Score:     score.Score,
		RiskLevel: score.RiskLevel,
		Factors:   score.Factors,
		ModelUsed: model.ID,
		ScoredAt:  time.Now(),
		Context:   score.Metadata,
	}

	history.Scores = append(history.Scores, historicalScore)
	history.LastScored = time.Now()

	// Keep only last 100 scores
	if len(history.Scores) > 100 {
		history.Scores = history.Scores[len(history.Scores)-100:]
	}

	// Update statistics
	rs.updateHistoryStatistics(history)
}

// updateHistoryStatistics updates historical statistics
func (rs *RiskScorer) updateHistoryStatistics(history *ScoringHistory) {
	if len(history.Scores) == 0 {
		return
	}

	// Calculate average score
	totalScore := 0.0
	maxScore := 0.0
	for _, score := range history.Scores {
		totalScore += score.Score
		if score.Score > maxScore {
			maxScore = score.Score
		}
	}

	history.AverageScore = totalScore / float64(len(history.Scores))
	history.MaxScore = maxScore

	// Determine trend (simple implementation)
	if len(history.Scores) >= 3 {
		recent := history.Scores[len(history.Scores)-3:]
		if recent[2].Score > recent[1].Score && recent[1].Score > recent[0].Score {
			history.RiskTrend = "increasing"
		} else if recent[2].Score < recent[1].Score && recent[1].Score < recent[0].Score {
			history.RiskTrend = "decreasing"
		} else {
			history.RiskTrend = "stable"
		}
	}
}

// AMLRiskScore represents the result of AML risk scoring
type AMLRiskScore struct {
	UserID       uuid.UUID              `json:"user_id"`
	Score        float64                `json:"score"`
	RiskLevel    aml.RiskLevel          `json:"risk_level"`
	ModelID      string                 `json:"model_id"`
	Factors      map[string]float64     `json:"factors"`
	CalculatedAt time.Time              `json:"calculated_at"`
	ExpiresAt    time.Time              `json:"expires_at"`
	Metadata     map[string]interface{} `json:"metadata"`
}

// GetScoringHistory returns scoring history for a user
func (rs *RiskScorer) GetScoringHistory(userID uuid.UUID) (*ScoringHistory, error) {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	history, exists := rs.scoringHistory[userID.String()]
	if !exists {
		return nil, fmt.Errorf("no scoring history found for user: %s", userID.String())
	}

	return history, nil
}

// UpdateModelAccuracy updates model accuracy metrics based on feedback
func (rs *RiskScorer) UpdateModelAccuracy(modelID string, actualOutcome bool, predictedRisk aml.RiskLevel) error {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	model, exists := rs.models[modelID]
	if !exists {
		return fmt.Errorf("model not found: %s", modelID)
	}

	model.AccuracyMetrics.TotalPredictions++

	// Simple accuracy tracking - in practice this would be more sophisticated
	if actualOutcome {
		model.AccuracyMetrics.CorrectPredictions++
	}

	model.AccuracyMetrics.Accuracy = float64(model.AccuracyMetrics.CorrectPredictions) /
		float64(model.AccuracyMetrics.TotalPredictions)
	model.AccuracyMetrics.LastUpdated = time.Now()

	rs.logger.Infow("Model accuracy updated",
		"model_id", modelID,
		"accuracy", model.AccuracyMetrics.Accuracy,
		"total_predictions", model.AccuracyMetrics.TotalPredictions,
	)

	return nil
}
