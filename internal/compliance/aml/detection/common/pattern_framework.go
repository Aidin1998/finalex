package common

import (
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// PatternDetector interface for all pattern detection implementations
type PatternDetector interface {
	Detect(activity DetectionActivity) *DetectionPattern
	GetName() string
	GetType() string
	UpdateConfig(config map[string]interface{}) error
}

// DetectionActivity represents a generalized activity for pattern detection
type DetectionActivity interface {
	GetUserID() string
	GetMarket() string
	GetTimeWindow() time.Duration
	GetMetadata() map[string]interface{}
}

// DetectionPattern represents a detected suspicious pattern
type DetectionPattern struct {
	Type        string                 `json:"type"`
	Description string                 `json:"description"`
	Confidence  decimal.Decimal        `json:"confidence"`
	Severity    string                 `json:"severity"`
	Evidence    []Evidence             `json:"evidence"`
	DetectedAt  time.Time              `json:"detected_at"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// Evidence represents supporting evidence for a detected pattern
type Evidence struct {
	Type        string                 `json:"type"`
	Description string                 `json:"description"`
	Data        map[string]interface{} `json:"data"`
	Timestamp   time.Time              `json:"timestamp"`
	Weight      decimal.Decimal        `json:"weight"`
}

// SeverityCalculator provides standardized severity determination
type SeverityCalculator struct {
	CriticalThreshold decimal.Decimal
	HighThreshold     decimal.Decimal
	MediumThreshold   decimal.Decimal
}

// NewDefaultSeverityCalculator creates a calculator with standard thresholds
func NewDefaultSeverityCalculator() *SeverityCalculator {
	return &SeverityCalculator{
		CriticalThreshold: decimal.NewFromFloat(90.0),
		HighThreshold:     decimal.NewFromFloat(75.0),
		MediumThreshold:   decimal.NewFromFloat(60.0),
	}
}

// DetermineSeverity calculates severity based on confidence score
func (sc *SeverityCalculator) DetermineSeverity(confidence decimal.Decimal) string {
	if confidence.GreaterThan(sc.CriticalThreshold) {
		return "critical"
	} else if confidence.GreaterThan(sc.HighThreshold) {
		return "high"
	} else if confidence.GreaterThan(sc.MediumThreshold) {
		return "medium"
	}
	return "low"
}

// EvidenceBuilder provides utilities for building evidence
type EvidenceBuilder struct {
	logger *zap.SugaredLogger
}

// NewEvidenceBuilder creates a new evidence builder
func NewEvidenceBuilder(logger *zap.SugaredLogger) *EvidenceBuilder {
	return &EvidenceBuilder{
		logger: logger,
	}
}

// AddEvidence creates an evidence entry with standardized structure
func (eb *EvidenceBuilder) AddEvidence(evidenceType, description string, data map[string]interface{}, weight decimal.Decimal) Evidence {
	return Evidence{
		Type:        evidenceType,
		Description: description,
		Data:        data,
		Timestamp:   time.Now(),
		Weight:      weight,
	}
}

// BuildTransactionEvidence creates evidence for transaction-based patterns
func (eb *EvidenceBuilder) BuildTransactionEvidence(pattern string, indicators map[string]decimal.Decimal) []Evidence {
	var evidence []Evidence

	for indicator, value := range indicators {
		evidence = append(evidence, eb.AddEvidence(
			"transaction_indicator",
			pattern+" indicator: "+indicator,
			map[string]interface{}{
				"indicator": indicator,
				"value":     value.String(),
				"pattern":   pattern,
			},
			eb.calculateWeight(value),
		))
	}

	return evidence
}

// BuildTimingEvidence creates evidence for timing-based patterns
func (eb *EvidenceBuilder) BuildTimingEvidence(pattern string, timingData map[string]interface{}) Evidence {
	return eb.AddEvidence(
		"timing",
		"Suspicious timing pattern detected for "+pattern,
		timingData,
		decimal.NewFromFloat(0.7), // Standard weight for timing evidence
	)
}

// BuildVolumeEvidence creates evidence for volume-based patterns
func (eb *EvidenceBuilder) BuildVolumeEvidence(pattern string, volumeData map[string]interface{}) Evidence {
	return eb.AddEvidence(
		"volume",
		"Suspicious volume pattern detected for "+pattern,
		volumeData,
		decimal.NewFromFloat(0.8), // Higher weight for volume evidence
	)
}

// calculateWeight determines evidence weight based on value magnitude
func (eb *EvidenceBuilder) calculateWeight(value decimal.Decimal) decimal.Decimal {
	// Simple weight calculation - can be made more sophisticated
	if value.GreaterThan(decimal.NewFromFloat(90)) {
		return decimal.NewFromFloat(1.0)
	} else if value.GreaterThan(decimal.NewFromFloat(75)) {
		return decimal.NewFromFloat(0.8)
	} else if value.GreaterThan(decimal.NewFromFloat(60)) {
		return decimal.NewFromFloat(0.6)
	}
	return decimal.NewFromFloat(0.4)
}

// ConfidenceCalculator provides standardized confidence calculation methods
type ConfidenceCalculator struct {
	BaseWeight     decimal.Decimal
	EvidenceWeight decimal.Decimal
	logger         *zap.SugaredLogger
}

// NewConfidenceCalculator creates a new confidence calculator
func NewConfidenceCalculator(logger *zap.SugaredLogger) *ConfidenceCalculator {
	return &ConfidenceCalculator{
		BaseWeight:     decimal.NewFromFloat(0.3),
		EvidenceWeight: decimal.NewFromFloat(0.7),
		logger:         logger,
	}
}

// CalculateConfidence computes confidence score based on evidence
func (cc *ConfidenceCalculator) CalculateConfidence(evidence []Evidence, baseScore decimal.Decimal) decimal.Decimal {
	if len(evidence) == 0 {
		return baseScore
	}

	// Calculate weighted evidence score
	var totalWeight, weightedScore decimal.Decimal
	for _, e := range evidence {
		totalWeight = totalWeight.Add(e.Weight)
		// Simplified scoring - in practice, this would be more sophisticated
		evidenceScore := decimal.NewFromFloat(50.0) // Base evidence score
		weightedScore = weightedScore.Add(evidenceScore.Mul(e.Weight))
	}

	if totalWeight.IsZero() {
		return baseScore
	}

	evidenceAverage := weightedScore.Div(totalWeight)

	// Combine base score with evidence score
	finalScore := baseScore.Mul(cc.BaseWeight).Add(evidenceAverage.Mul(cc.EvidenceWeight))

	// Cap at 100
	if finalScore.GreaterThan(decimal.NewFromFloat(100)) {
		finalScore = decimal.NewFromFloat(100)
	}

	return finalScore
}

// PatternRegistry manages available pattern detectors
type PatternRegistry struct {
	detectors map[string]PatternDetector
	logger    *zap.SugaredLogger
}

// NewPatternRegistry creates a new pattern registry
func NewPatternRegistry(logger *zap.SugaredLogger) *PatternRegistry {
	return &PatternRegistry{
		detectors: make(map[string]PatternDetector),
		logger:    logger,
	}
}

// RegisterDetector adds a detector to the registry
func (pr *PatternRegistry) RegisterDetector(detector PatternDetector) {
	pr.detectors[detector.GetName()] = detector
	pr.logger.Infow("Pattern detector registered", "name", detector.GetName(), "type", detector.GetType())
}

// GetDetector retrieves a detector by name
func (pr *PatternRegistry) GetDetector(name string) (PatternDetector, bool) {
	detector, exists := pr.detectors[name]
	return detector, exists
}

// GetAllDetectors returns all registered detectors
func (pr *PatternRegistry) GetAllDetectors() []PatternDetector {
	var detectors []PatternDetector
	for _, detector := range pr.detectors {
		detectors = append(detectors, detector)
	}
	return detectors
}

// RunAllDetectors runs all registered detectors on an activity
func (pr *PatternRegistry) RunAllDetectors(activity DetectionActivity) []*DetectionPattern {
	var patterns []*DetectionPattern

	for _, detector := range pr.detectors {
		if pattern := detector.Detect(activity); pattern != nil {
			patterns = append(patterns, pattern)
		}
	}

	return patterns
}
