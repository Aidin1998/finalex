package scoring

import (
	"context"
	"math"
	"time"

	"github.com/shopspring/decimal"
)

// DynamicRiskScorer provides advanced risk scoring with machine learning capabilities
type DynamicRiskScorer struct {
	models          map[string]*RiskModel
	featureEngine   *FeatureEngine
	scoreAggregator *ScoreAggregator
	modelUpdater    *ModelUpdater
	config          *ScoringConfig
}

// RiskModel represents a machine learning model for risk scoring
type RiskModel struct {
	ID          string
	Name        string
	Version     string
	ModelType   ModelType
	Features    []string
	Weights     map[string]decimal.Decimal
	Bias        decimal.Decimal
	Accuracy    decimal.Decimal
	LastTrained time.Time
	LastUpdated time.Time
	IsActive    bool
	Metadata    map[string]interface{}
}

// FeatureEngine extracts and processes features for risk scoring
type FeatureEngine struct {
	extractors   map[string]FeatureExtractor
	transformers map[string]FeatureTransformer
	selectors    map[string]FeatureSelector
	featureStore *FeatureStore
}

// ScoreAggregator combines scores from multiple models
type ScoreAggregator struct {
	aggregationStrategy AggregationStrategy
	modelWeights        map[string]decimal.Decimal
	ensembleConfig      *EnsembleConfig
}

// ModelUpdater handles model updates and retraining
type ModelUpdater struct {
	updateSchedule     *UpdateSchedule
	retrainingTriggers []RetrainingTrigger
	dataCollector      *DataCollector
	validator          *ModelValidator
}

// ScoringConfig contains configuration for risk scoring
type ScoringConfig struct {
	DefaultThresholds    map[string]decimal.Decimal
	ModelTimeout         time.Duration
	FeatureTimeout       time.Duration
	CacheTimeout         time.Duration
	EnableModelEnsemble  bool
	EnableFeatureCaching bool
	EnableRealTimeUpdate bool
}

// RiskScoreRequest contains request for risk scoring
type RiskScoreRequest struct {
	UserID             string
	TransactionID      string
	ContextData        map[string]interface{}
	RequiredModels     []string
	RealTimeFeatures   bool
	IncludeExplanation bool
}

// RiskScoreResponse contains the risk scoring result
type RiskScoreResponse struct {
	UserID          string
	TransactionID   string
	OverallScore    decimal.Decimal
	ModelScores     map[string]decimal.Decimal
	FeatureScores   map[string]decimal.Decimal
	RiskCategory    RiskCategory
	Confidence      decimal.Decimal
	Explanation     *ScoreExplanation
	ProcessingTime  time.Duration
	Timestamp       time.Time
	Recommendations []string
	Triggers        []string
}

// ScoreExplanation provides interpretability for risk scores
type ScoreExplanation struct {
	TopRiskFactors     []RiskFactor
	FeatureImportance  map[string]decimal.Decimal
	ModelContributions map[string]decimal.Decimal
	ThresholdAnalysis  *ThresholdAnalysis
	ComparisonBaseline *BaselineComparison
}

// FeatureExtractor extracts features from raw data
type FeatureExtractor interface {
	Extract(ctx context.Context, data map[string]interface{}) (*FeatureVector, error)
	GetFeatureNames() []string
	GetFeatureTypes() map[string]FeatureType
}

// FeatureTransformer transforms features for model input
type FeatureTransformer interface {
	Transform(ctx context.Context, features *FeatureVector) (*FeatureVector, error)
	Fit(ctx context.Context, features []*FeatureVector) error
	GetTransformationType() TransformationType
}

// FeatureSelector selects relevant features
type FeatureSelector interface {
	Select(ctx context.Context, features *FeatureVector) (*FeatureVector, error)
	GetSelectedFeatures() []string
	UpdateSelection(ctx context.Context, importanceScores map[string]decimal.Decimal) error
}

// Types and enums
type ModelType string
type RiskCategory string
type AggregationStrategy string
type FeatureType string
type TransformationType string

const (
	// Model types
	ModelTypeLinear    ModelType = "linear"
	ModelTypeTreeBased ModelType = "tree"
	ModelTypeNeural    ModelType = "neural"
	ModelTypeEnsemble  ModelType = "ensemble"

	// Risk categories
	RiskCategoryLow      RiskCategory = "low"
	RiskCategoryMedium   RiskCategory = "medium"
	RiskCategoryHigh     RiskCategory = "high"
	RiskCategoryCritical RiskCategory = "critical"

	// Aggregation strategies
	AggregationWeighted AggregationStrategy = "weighted"
	AggregationAverage  AggregationStrategy = "average"
	AggregationMaximum  AggregationStrategy = "maximum"
	AggregationStacking AggregationStrategy = "stacking"

	// Feature types
	FeatureTypeNumerical   FeatureType = "numerical"
	FeatureTypeCategorical FeatureType = "categorical"
	FeatureTypeBinary      FeatureType = "binary"
	FeatureTypeText        FeatureType = "text"

	// Transformation types
	TransformNormalization   TransformationType = "normalization"
	TransformStandardization TransformationType = "standardization"
	TransformEncoding        TransformationType = "encoding"
	TransformDimensionality  TransformationType = "dimensionality"
)

// Supporting types
type FeatureVector struct {
	Features map[string]interface{}
	Metadata map[string]interface{}
}

type FeatureStore struct {
	cache           map[string]*FeatureVector
	persistentStore map[string]*FeatureVector
}

type EnsembleConfig struct {
	Strategy        AggregationStrategy
	ModelWeights    map[string]decimal.Decimal
	VotingThreshold decimal.Decimal
}

type UpdateSchedule struct {
	Frequency     time.Duration
	LastUpdate    time.Time
	NextUpdate    time.Time
	UpdateWindows []time.Duration
}

type RetrainingTrigger struct {
	Type      string
	Threshold decimal.Decimal
	Condition string
	Enabled   bool
}

type DataCollector struct {
	feedbackData    []FeedbackData
	performanceData []PerformanceData
}

type ModelValidator struct {
	validationMetrics map[string]decimal.Decimal
	testDatasets      []ValidationDataset
}

type FeedbackData struct {
	PredictionID  string
	ActualOutcome string
	Timestamp     time.Time
	Metadata      map[string]interface{}
}

type PerformanceData struct {
	ModelID   string
	Accuracy  decimal.Decimal
	Precision decimal.Decimal
	Recall    decimal.Decimal
	F1Score   decimal.Decimal
	Timestamp time.Time
}

type ValidationDataset struct {
	Name      string
	Data      []*FeatureVector
	Labels    []string
	CreatedAt time.Time
}

type RiskFactor struct {
	Feature      string
	Value        interface{}
	Contribution decimal.Decimal
	Impact       string
	Description  string
}

type ThresholdAnalysis struct {
	CurrentThresholds map[string]decimal.Decimal
	OptimalThresholds map[string]decimal.Decimal
	ThresholdImpact   map[string]decimal.Decimal
}

type BaselineComparison struct {
	UserBaseline       decimal.Decimal
	PopulationBaseline decimal.Decimal
	PeerGroupBaseline  decimal.Decimal
	DeviationScore     decimal.Decimal
}

// NewDynamicRiskScorer creates a new dynamic risk scorer
func NewDynamicRiskScorer(config *ScoringConfig) *DynamicRiskScorer {
	return &DynamicRiskScorer{
		models:          make(map[string]*RiskModel),
		featureEngine:   NewFeatureEngine(),
		scoreAggregator: NewScoreAggregator(),
		modelUpdater:    NewModelUpdater(),
		config:          config,
	}
}

// ScoreRisk performs comprehensive risk scoring
func (drs *DynamicRiskScorer) ScoreRisk(ctx context.Context, request *RiskScoreRequest) (*RiskScoreResponse, error) {
	startTime := time.Now()

	response := &RiskScoreResponse{
		UserID:        request.UserID,
		TransactionID: request.TransactionID,
		ModelScores:   make(map[string]decimal.Decimal),
		FeatureScores: make(map[string]decimal.Decimal),
		Timestamp:     time.Now(),
	}

	// Extract features
	features, err := drs.extractFeatures(ctx, request)
	if err != nil {
		return nil, err
	}

	// Score with individual models
	modelScores, err := drs.scoreWithModels(ctx, features, request.RequiredModels)
	if err != nil {
		return nil, err
	}
	response.ModelScores = modelScores

	// Aggregate scores
	overallScore, confidence := drs.scoreAggregator.Aggregate(modelScores)
	response.OverallScore = overallScore
	response.Confidence = confidence

	// Determine risk category
	response.RiskCategory = drs.categorizeRisk(overallScore)

	// Generate explanation if requested
	if request.IncludeExplanation {
		response.Explanation = drs.generateExplanation(ctx, features, modelScores, overallScore)
	}

	// Calculate feature contributions
	response.FeatureScores = drs.calculateFeatureContributions(features, modelScores)

	// Generate recommendations and triggers
	response.Recommendations = drs.generateRecommendations(response.RiskCategory, overallScore)
	response.Triggers = drs.generateTriggers(response.RiskCategory, overallScore)

	response.ProcessingTime = time.Since(startTime)

	return response, nil
}

// extractFeatures extracts and processes features for scoring
func (drs *DynamicRiskScorer) extractFeatures(ctx context.Context, request *RiskScoreRequest) (*FeatureVector, error) {
	// Use the feature engine to extract features
	features, err := drs.featureEngine.ExtractFeatures(ctx, request.ContextData)
	if err != nil {
		return nil, err
	}

	// Transform features if needed
	transformedFeatures, err := drs.featureEngine.TransformFeatures(ctx, features)
	if err != nil {
		return nil, err
	}

	// Select relevant features
	selectedFeatures, err := drs.featureEngine.SelectFeatures(ctx, transformedFeatures)
	if err != nil {
		return nil, err
	}

	return selectedFeatures, nil
}

// scoreWithModels scores using individual models
func (drs *DynamicRiskScorer) scoreWithModels(ctx context.Context, features *FeatureVector, requiredModels []string) (map[string]decimal.Decimal, error) {
	scores := make(map[string]decimal.Decimal)

	modelsToUse := requiredModels
	if len(modelsToUse) == 0 {
		// Use all active models
		for modelID, model := range drs.models {
			if model.IsActive {
				modelsToUse = append(modelsToUse, modelID)
			}
		}
	}

	for _, modelID := range modelsToUse {
		model, exists := drs.models[modelID]
		if !exists || !model.IsActive {
			continue
		}

		score, err := drs.scoreWithModel(ctx, model, features)
		if err != nil {
			continue // Skip this model on error
		}

		scores[modelID] = score
	}

	return scores, nil
}

// scoreWithModel scores using a specific model
func (drs *DynamicRiskScorer) scoreWithModel(ctx context.Context, model *RiskModel, features *FeatureVector) (decimal.Decimal, error) {
	switch model.ModelType {
	case ModelTypeLinear:
		return drs.scoreLinearModel(model, features)
	case ModelTypeTreeBased:
		return drs.scoreTreeModel(model, features)
	case ModelTypeNeural:
		return drs.scoreNeuralModel(model, features)
	case ModelTypeEnsemble:
		return drs.scoreEnsembleModel(model, features)
	default:
		return decimal.Zero, nil
	}
}

// scoreLinearModel implements linear model scoring
func (drs *DynamicRiskScorer) scoreLinearModel(model *RiskModel, features *FeatureVector) (decimal.Decimal, error) {
	score := model.Bias

	for featureName, weight := range model.Weights {
		if value, exists := features.Features[featureName]; exists {
			if numValue, ok := drs.convertToDecimal(value); ok {
				score = score.Add(weight.Mul(numValue))
			}
		}
	}

	// Apply sigmoid to get probability
	return drs.sigmoid(score), nil
}

// scoreTreeModel implements tree-based model scoring (simplified)
func (drs *DynamicRiskScorer) scoreTreeModel(model *RiskModel, features *FeatureVector) (decimal.Decimal, error) {
	// Simplified tree scoring - in practice, this would traverse actual decision trees
	score := decimal.Zero
	featureCount := decimal.Zero

	for featureName, weight := range model.Weights {
		if value, exists := features.Features[featureName]; exists {
			if numValue, ok := drs.convertToDecimal(value); ok {
				score = score.Add(weight.Mul(numValue))
				featureCount = featureCount.Add(decimal.NewFromInt(1))
			}
		}
	}

	if featureCount.GreaterThan(decimal.Zero) {
		score = score.Div(featureCount)
	}

	return drs.normalizeScore(score), nil
}

// scoreNeuralModel implements neural network scoring (simplified)
func (drs *DynamicRiskScorer) scoreNeuralModel(model *RiskModel, features *FeatureVector) (decimal.Decimal, error) {
	// Simplified neural network - in practice, this would use actual neural network inference
	hiddenScore := decimal.Zero

	for featureName, weight := range model.Weights {
		if value, exists := features.Features[featureName]; exists {
			if numValue, ok := drs.convertToDecimal(value); ok {
				hiddenScore = hiddenScore.Add(drs.relu(weight.Mul(numValue)))
			}
		}
	}

	// Apply final activation
	return drs.sigmoid(hiddenScore.Add(model.Bias)), nil
}

// scoreEnsembleModel implements ensemble model scoring
func (drs *DynamicRiskScorer) scoreEnsembleModel(model *RiskModel, features *FeatureVector) (decimal.Decimal, error) {
	// For ensemble models, we would combine multiple base models
	// This is a simplified implementation
	return drs.scoreLinearModel(model, features)
}

// categorizeRisk categorizes risk score into risk levels
func (drs *DynamicRiskScorer) categorizeRisk(score decimal.Decimal) RiskCategory {
	if score.GreaterThan(decimal.NewFromFloat(0.8)) {
		return RiskCategoryCritical
	} else if score.GreaterThan(decimal.NewFromFloat(0.6)) {
		return RiskCategoryHigh
	} else if score.GreaterThan(decimal.NewFromFloat(0.3)) {
		return RiskCategoryMedium
	}
	return RiskCategoryLow
}

// generateExplanation generates score explanation
func (drs *DynamicRiskScorer) generateExplanation(ctx context.Context, features *FeatureVector, modelScores map[string]decimal.Decimal, overallScore decimal.Decimal) *ScoreExplanation {
	explanation := &ScoreExplanation{
		TopRiskFactors:     []RiskFactor{},
		FeatureImportance:  make(map[string]decimal.Decimal),
		ModelContributions: modelScores,
		ThresholdAnalysis:  &ThresholdAnalysis{},
		ComparisonBaseline: &BaselineComparison{},
	}

	// Calculate feature importance
	for featureName, value := range features.Features {
		if numValue, ok := drs.convertToDecimal(value); ok {
			// Simplified importance calculation
			importance := numValue.Mul(decimal.NewFromFloat(0.1))
			explanation.FeatureImportance[featureName] = importance

			if importance.GreaterThan(decimal.NewFromFloat(0.1)) {
				explanation.TopRiskFactors = append(explanation.TopRiskFactors, RiskFactor{
					Feature:      featureName,
					Value:        value,
					Contribution: importance,
					Impact:       "high",
					Description:  "High-risk feature detected",
				})
			}
		}
	}

	return explanation
}

// calculateFeatureContributions calculates individual feature contributions
func (drs *DynamicRiskScorer) calculateFeatureContributions(features *FeatureVector, modelScores map[string]decimal.Decimal) map[string]decimal.Decimal {
	contributions := make(map[string]decimal.Decimal)

	for featureName, value := range features.Features {
		if numValue, ok := drs.convertToDecimal(value); ok {
			// Simplified contribution calculation
			avgModelScore := decimal.Zero
			modelCount := decimal.Zero

			for _, score := range modelScores {
				avgModelScore = avgModelScore.Add(score)
				modelCount = modelCount.Add(decimal.NewFromInt(1))
			}

			if modelCount.GreaterThan(decimal.Zero) {
				avgModelScore = avgModelScore.Div(modelCount)
				contributions[featureName] = numValue.Mul(avgModelScore)
			}
		}
	}

	return contributions
}

// generateRecommendations generates action recommendations
func (drs *DynamicRiskScorer) generateRecommendations(category RiskCategory, score decimal.Decimal) []string {
	var recommendations []string

	switch category {
	case RiskCategoryCritical:
		recommendations = append(recommendations, "Immediately block transaction")
		recommendations = append(recommendations, "File suspicious activity report")
		recommendations = append(recommendations, "Freeze account pending investigation")
	case RiskCategoryHigh:
		recommendations = append(recommendations, "Manual review required")
		recommendations = append(recommendations, "Request additional documentation")
		recommendations = append(recommendations, "Enhanced monitoring")
	case RiskCategoryMedium:
		recommendations = append(recommendations, "Automated enhanced monitoring")
		recommendations = append(recommendations, "Consider transaction limits")
	case RiskCategoryLow:
		recommendations = append(recommendations, "Standard monitoring")
	}

	return recommendations
}

// generateTriggers generates automated triggers
func (drs *DynamicRiskScorer) generateTriggers(category RiskCategory, score decimal.Decimal) []string {
	var triggers []string

	switch category {
	case RiskCategoryCritical:
		triggers = append(triggers, "block_transaction")
		triggers = append(triggers, "generate_sar")
		triggers = append(triggers, "freeze_account")
	case RiskCategoryHigh:
		triggers = append(triggers, "create_case")
		triggers = append(triggers, "alert_compliance_team")
	case RiskCategoryMedium:
		triggers = append(triggers, "enhanced_monitoring")
	}

	return triggers
}

// Helper functions

func (drs *DynamicRiskScorer) convertToDecimal(value interface{}) (decimal.Decimal, bool) {
	switch v := value.(type) {
	case decimal.Decimal:
		return v, true
	case float64:
		return decimal.NewFromFloat(v), true
	case int:
		return decimal.NewFromInt(int64(v)), true
	case int64:
		return decimal.NewFromInt(v), true
	case string:
		if d, err := decimal.NewFromString(v); err == nil {
			return d, true
		}
	}
	return decimal.Zero, false
}

func (drs *DynamicRiskScorer) sigmoid(x decimal.Decimal) decimal.Decimal {
	// Sigmoid function: 1 / (1 + e^(-x))
	negX := x.Neg()
	exp := decimal.NewFromFloat(math.Exp(negX.InexactFloat64()))
	return decimal.NewFromInt(1).Div(decimal.NewFromInt(1).Add(exp))
}

func (drs *DynamicRiskScorer) relu(x decimal.Decimal) decimal.Decimal {
	// ReLU function: max(0, x)
	if x.GreaterThan(decimal.Zero) {
		return x
	}
	return decimal.Zero
}

func (drs *DynamicRiskScorer) normalizeScore(score decimal.Decimal) decimal.Decimal {
	// Normalize score to 0-1 range
	if score.LessThan(decimal.Zero) {
		return decimal.Zero
	}
	if score.GreaterThan(decimal.NewFromInt(1)) {
		return decimal.NewFromInt(1)
	}
	return score
}

// Constructor functions for supporting components

func NewFeatureEngine() *FeatureEngine {
	return &FeatureEngine{
		extractors:   make(map[string]FeatureExtractor),
		transformers: make(map[string]FeatureTransformer),
		selectors:    make(map[string]FeatureSelector),
		featureStore: &FeatureStore{
			cache:           make(map[string]*FeatureVector),
			persistentStore: make(map[string]*FeatureVector),
		},
	}
}

func NewScoreAggregator() *ScoreAggregator {
	return &ScoreAggregator{
		aggregationStrategy: AggregationWeighted,
		modelWeights:        make(map[string]decimal.Decimal),
		ensembleConfig: &EnsembleConfig{
			Strategy:        AggregationWeighted,
			ModelWeights:    make(map[string]decimal.Decimal),
			VotingThreshold: decimal.NewFromFloat(0.5),
		},
	}
}

func NewModelUpdater() *ModelUpdater {
	return &ModelUpdater{
		updateSchedule: &UpdateSchedule{
			Frequency:  24 * time.Hour,
			LastUpdate: time.Now(),
		},
		retrainingTriggers: []RetrainingTrigger{},
		dataCollector:      &DataCollector{},
		validator:          &ModelValidator{},
	}
}

// FeatureEngine methods

func (fe *FeatureEngine) ExtractFeatures(ctx context.Context, data map[string]interface{}) (*FeatureVector, error) {
	features := &FeatureVector{
		Features: make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	// Extract features using registered extractors
	for name, extractor := range fe.extractors {
		extractedFeatures, err := extractor.Extract(ctx, data)
		if err != nil {
			continue // Skip this extractor on error
		}

		// Merge features
		for featureName, featureValue := range extractedFeatures.Features {
			features.Features[name+"_"+featureName] = featureValue
		}
	}

	return features, nil
}

func (fe *FeatureEngine) TransformFeatures(ctx context.Context, features *FeatureVector) (*FeatureVector, error) {
	transformedFeatures := &FeatureVector{
		Features: make(map[string]interface{}),
		Metadata: features.Metadata,
	}

	// Apply transformations
	for name, transformer := range fe.transformers {
		transformed, err := transformer.Transform(ctx, features)
		if err != nil {
			continue // Skip this transformer on error
		}

		// Merge transformed features
		for featureName, featureValue := range transformed.Features {
			transformedFeatures.Features[name+"_"+featureName] = featureValue
		}
	}

	// If no transformers, return original features
	if len(transformedFeatures.Features) == 0 {
		return features, nil
	}

	return transformedFeatures, nil
}

func (fe *FeatureEngine) SelectFeatures(ctx context.Context, features *FeatureVector) (*FeatureVector, error) {
	selectedFeatures := &FeatureVector{
		Features: make(map[string]interface{}),
		Metadata: features.Metadata,
	}

	// Apply feature selection
	for name, selector := range fe.selectors {
		selected, err := selector.Select(ctx, features)
		if err != nil {
			continue // Skip this selector on error
		}

		// Merge selected features
		for featureName, featureValue := range selected.Features {
			selectedFeatures.Features[name+"_"+featureName] = featureValue
		}
	}

	// If no selectors, return original features
	if len(selectedFeatures.Features) == 0 {
		return features, nil
	}

	return selectedFeatures, nil
}

// ScoreAggregator methods

func (sa *ScoreAggregator) Aggregate(modelScores map[string]decimal.Decimal) (decimal.Decimal, decimal.Decimal) {
	if len(modelScores) == 0 {
		return decimal.Zero, decimal.Zero
	}

	switch sa.aggregationStrategy {
	case AggregationWeighted:
		return sa.weightedAggregation(modelScores)
	case AggregationAverage:
		return sa.averageAggregation(modelScores)
	case AggregationMaximum:
		return sa.maximumAggregation(modelScores)
	default:
		return sa.averageAggregation(modelScores)
	}
}

func (sa *ScoreAggregator) weightedAggregation(modelScores map[string]decimal.Decimal) (decimal.Decimal, decimal.Decimal) {
	weightedSum := decimal.Zero
	totalWeight := decimal.Zero

	for modelID, score := range modelScores {
		weight := decimal.NewFromFloat(1.0) // Default weight
		if w, exists := sa.modelWeights[modelID]; exists {
			weight = w
		}

		weightedSum = weightedSum.Add(score.Mul(weight))
		totalWeight = totalWeight.Add(weight)
	}

	if totalWeight.IsZero() {
		return decimal.Zero, decimal.Zero
	}

	aggregatedScore := weightedSum.Div(totalWeight)
	confidence := sa.calculateConfidence(modelScores)

	return aggregatedScore, confidence
}

func (sa *ScoreAggregator) averageAggregation(modelScores map[string]decimal.Decimal) (decimal.Decimal, decimal.Decimal) {
	sum := decimal.Zero
	for _, score := range modelScores {
		sum = sum.Add(score)
	}

	averageScore := sum.Div(decimal.NewFromInt(int64(len(modelScores))))
	confidence := sa.calculateConfidence(modelScores)

	return averageScore, confidence
}

func (sa *ScoreAggregator) maximumAggregation(modelScores map[string]decimal.Decimal) (decimal.Decimal, decimal.Decimal) {
	maxScore := decimal.Zero
	for _, score := range modelScores {
		if score.GreaterThan(maxScore) {
			maxScore = score
		}
	}

	confidence := sa.calculateConfidence(modelScores)
	return maxScore, confidence
}

func (sa *ScoreAggregator) calculateConfidence(modelScores map[string]decimal.Decimal) decimal.Decimal {
	if len(modelScores) <= 1 {
		return decimal.NewFromFloat(0.5)
	}

	// Calculate variance as a measure of disagreement
	mean := decimal.Zero
	for _, score := range modelScores {
		mean = mean.Add(score)
	}
	mean = mean.Div(decimal.NewFromInt(int64(len(modelScores))))

	variance := decimal.Zero
	for _, score := range modelScores {
		diff := score.Sub(mean)
		variance = variance.Add(diff.Mul(diff))
	}
	variance = variance.Div(decimal.NewFromInt(int64(len(modelScores))))

	// Lower variance means higher confidence
	confidence := decimal.NewFromInt(1).Sub(variance)
	if confidence.LessThan(decimal.Zero) {
		confidence = decimal.Zero
	}

	return confidence
}
