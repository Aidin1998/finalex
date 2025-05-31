package detection

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/compliance/aml"
	"github.com/Aidin1998/pincex_unified/internal/risk"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// PatternEngine integrates existing compliance engine with focused AML pattern detection
type PatternEngine struct {
	mu     sync.RWMutex
	logger *zap.SugaredLogger

	// Integration with existing compliance engine
	complianceEngine *risk.ComplianceEngine

	// Pattern detection configuration
	config PatternConfig

	// Pattern detection results
	detectedPatterns map[string]*aml.SuspiciousActivity
	patternStats     map[string]*PatternStats
}

// PatternConfig defines configuration for pattern detection
type PatternConfig struct {
	StructuringThreshold    decimal.Decimal `json:"structuring_threshold"`
	VelocityThreshold       int             `json:"velocity_threshold"`
	RoundAmountThreshold    decimal.Decimal `json:"round_amount_threshold"`
	RapidMovementTimeWindow time.Duration   `json:"rapid_movement_time_window"`
	MinPatternConfidence    float64         `json:"min_pattern_confidence"`
}

// PatternStats tracks pattern detection statistics
type PatternStats struct {
	TotalDetections int       `json:"total_detections"`
	TruePositives   int       `json:"true_positives"`
	FalsePositives  int       `json:"false_positives"`
	LastDetectionAt time.Time `json:"last_detection_at"`
	AccuracyRate    float64   `json:"accuracy_rate"`
	PatternType     string    `json:"pattern_type"`
}

// NewPatternEngine creates a new AML pattern detection engine
func NewPatternEngine(complianceEngine *risk.ComplianceEngine, logger *zap.SugaredLogger) *PatternEngine {
	return &PatternEngine{
		logger:           logger,
		complianceEngine: complianceEngine,
		config: PatternConfig{
			StructuringThreshold:    decimal.NewFromFloat(9500),
			VelocityThreshold:       10,
			RoundAmountThreshold:    decimal.NewFromFloat(1000),
			RapidMovementTimeWindow: 30 * time.Minute,
			MinPatternConfidence:    0.75,
		},
		detectedPatterns: make(map[string]*aml.SuspiciousActivity),
		patternStats:     make(map[string]*PatternStats),
	}
}

// DetectPatterns analyzes transactions for suspicious patterns using existing compliance engine
func (pe *PatternEngine) DetectPatterns(ctx context.Context, userID uuid.UUID, transactions []risk.TransactionRecord) ([]*aml.SuspiciousActivity, error) {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	var suspiciousActivities []*aml.SuspiciousActivity

	// Use existing compliance engine to run checks
	for _, transaction := range transactions {
		err := pe.complianceEngine.RunComplianceChecks(ctx, userID.String(), &transaction)
		if err != nil {
			pe.logger.Errorw("Failed to run compliance checks", "error", err, "user_id", userID)
			continue
		}
	}

	// Get alerts from compliance engine
	alerts, err := pe.complianceEngine.GetActiveAlerts(ctx)
	if err != nil {
		return nil, err
	}

	// Convert compliance alerts to suspicious activities
	for _, alert := range alerts {
		if alert.UserID == userID.String() {
			activity := pe.convertAlertToSuspiciousActivity(alert)
			suspiciousActivities = append(suspiciousActivities, activity)

			// Store for tracking
			pe.detectedPatterns[activity.ID.String()] = activity
			pe.updatePatternStats(activity.Pattern)
		}
	}

	return suspiciousActivities, nil
}

// convertAlertToSuspiciousActivity converts compliance alert to suspicious activity
func (pe *PatternEngine) convertAlertToSuspiciousActivity(alert risk.ComplianceAlert) *aml.SuspiciousActivity {
	// Map severity to risk level
	var riskLevel aml.RiskLevel
	switch alert.Severity {
	case "low":
		riskLevel = aml.RiskLevelLow
	case "medium":
		riskLevel = aml.RiskLevelMedium
	case "high":
		riskLevel = aml.RiskLevelHigh
	case "critical":
		riskLevel = aml.RiskLevelCritical
	default:
		riskLevel = aml.RiskLevelMedium
	}

	// Map rule name to activity type
	var activityType aml.ActivityType
	switch alert.RuleName {
	case "Transaction Structuring Detection":
		activityType = aml.ActivityTypeStructuring
	case "Transaction Velocity Check":
		activityType = aml.ActivityTypeUnusualVolume
	case "Round Amount Pattern Detection":
		activityType = aml.ActivityTypePatternMatching
	case "Rapid Deposit-Withdrawal Pattern":
		activityType = aml.ActivityTypeRapidMovement
	default:
		activityType = aml.ActivityTypePatternMatching
	}

	userID, _ := uuid.Parse(alert.UserID)

	return &aml.SuspiciousActivity{
		ID:           uuid.New(),
		UserID:       userID,
		ActivityType: activityType,
		RiskScore:    alert.RiskScore.InexactFloat64(),
		Severity:     riskLevel,
		Description:  alert.Description,
		Pattern:      alert.RuleName,
		Indicators: map[string]interface{}{
			"alert_id":          alert.ID,
			"rule_id":           alert.RuleID,
			"risk_score":        alert.RiskScore.String(),
			"detected_patterns": alert.DetectedPatterns,
		},
		DetectedAt: alert.CreatedAt,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
}

// updatePatternStats updates statistics for pattern detection
func (pe *PatternEngine) updatePatternStats(patternType string) {
	stats, exists := pe.patternStats[patternType]
	if !exists {
		stats = &PatternStats{
			PatternType: patternType,
		}
		pe.patternStats[patternType] = stats
	}

	stats.TotalDetections++
	stats.LastDetectionAt = time.Now()

	// Calculate accuracy rate (would be updated based on manual review)
	if stats.TotalDetections > 0 {
		stats.AccuracyRate = float64(stats.TruePositives) / float64(stats.TotalDetections)
	}
}

// GetPatternStats returns pattern detection statistics
func (pe *PatternEngine) GetPatternStats() map[string]*PatternStats {
	pe.mu.RLock()
	defer pe.mu.RUnlock()

	result := make(map[string]*PatternStats)
	for k, v := range pe.patternStats {
		result[k] = v
	}
	return result
}

// UpdatePatternFeedback updates pattern accuracy based on manual review
func (pe *PatternEngine) UpdatePatternFeedback(ctx context.Context, activityID uuid.UUID, isTruePositive bool) error {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	activity, exists := pe.detectedPatterns[activityID.String()]
	if !exists {
		return fmt.Errorf("suspicious activity not found: %s", activityID.String())
	}

	stats := pe.patternStats[activity.Pattern]
	if stats != nil {
		if isTruePositive {
			stats.TruePositives++
		} else {
			stats.FalsePositives++
		}

		// Recalculate accuracy rate
		stats.AccuracyRate = float64(stats.TruePositives) / float64(stats.TotalDetections)
	}

	pe.logger.Infow("Pattern feedback updated",
		"activity_id", activityID,
		"pattern", activity.Pattern,
		"is_true_positive", isTruePositive,
		"accuracy_rate", stats.AccuracyRate,
	)

	return nil
}
