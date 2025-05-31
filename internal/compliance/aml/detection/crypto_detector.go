package detection

import (
	"context"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// CryptoDetector handles cryptocurrency-specific AML detection
type CryptoDetector struct {
	riskAddresses   map[string]RiskLevel
	mixerAddresses  map[string]bool
	exchangeRatings map[string]ExchangeRisk
}

// RiskLevel defines the risk level of an address
type RiskLevel int

const (
	RiskLevelLow RiskLevel = iota
	RiskLevelMedium
	RiskLevelHigh
	RiskLevelCritical
)

// ExchangeRisk defines the risk level of an exchange
type ExchangeRisk struct {
	Name       string
	RiskLevel  RiskLevel
	Regulated  bool
	Sanctioned bool
}

// CryptoTransaction represents a cryptocurrency transaction for analysis
type CryptoTransaction struct {
	TxHash        string
	FromAddress   string
	ToAddress     string
	Amount        decimal.Decimal
	Currency      string
	Timestamp     time.Time
	BlockHeight   int64
	Confirmations int
	Fee           decimal.Decimal
	Metadata      map[string]interface{}
}

// CryptoAnalysisResult contains the result of crypto-specific analysis
type CryptoAnalysisResult struct {
	TransactionID   string
	RiskScore       decimal.Decimal
	SuspiciousFlags []string
	AddressRisks    map[string]RiskLevel
	MixerDetected   bool
	ChainAnalysis   *ChainAnalysisResult
	TaintAnalysis   *TaintAnalysisResult
	Recommendations []string
}

// ChainAnalysisResult contains blockchain analysis results
type ChainAnalysisResult struct {
	HopCount        int
	KnownExchanges  []string
	KnownMixers     []string
	SuspiciousHops  []string
	TotalVolume     decimal.Decimal
	TimeSpan        time.Duration
	ClusterAnalysis *ClusterInfo
}

// TaintAnalysisResult contains taint analysis results
type TaintAnalysisResult struct {
	TaintScore     decimal.Decimal
	TaintedSources []string
	CleanSources   []string
	MixingDetected bool
	LayeringDepth  int
}

// ClusterInfo contains address cluster information
type ClusterInfo struct {
	ClusterID    string
	AddressCount int
	TotalVolume  decimal.Decimal
	RiskLevel    RiskLevel
	EntityType   string
}

// NewCryptoDetector creates a new crypto detector
func NewCryptoDetector() *CryptoDetector {
	return &CryptoDetector{
		riskAddresses:   make(map[string]RiskLevel),
		mixerAddresses:  make(map[string]bool),
		exchangeRatings: make(map[string]ExchangeRisk),
	}
}

// AnalyzeTransaction performs comprehensive crypto AML analysis
func (cd *CryptoDetector) AnalyzeTransaction(ctx context.Context, tx *CryptoTransaction) (*CryptoAnalysisResult, error) {
	result := &CryptoAnalysisResult{
		TransactionID:   tx.TxHash,
		RiskScore:       decimal.Zero,
		SuspiciousFlags: []string{},
		AddressRisks:    make(map[string]RiskLevel),
		Recommendations: []string{},
	}

	// Address risk analysis
	cd.analyzeAddressRisks(tx, result)

	// Mixer detection
	cd.detectMixers(tx, result)

	// Chain analysis
	result.ChainAnalysis = cd.performChainAnalysis(ctx, tx)

	// Taint analysis
	result.TaintAnalysis = cd.performTaintAnalysis(ctx, tx)

	// Calculate overall risk score
	cd.calculateRiskScore(result)

	// Generate recommendations
	cd.generateRecommendations(result)

	return result, nil
}

// analyzeAddressRisks checks addresses against risk databases
func (cd *CryptoDetector) analyzeAddressRisks(tx *CryptoTransaction, result *CryptoAnalysisResult) {
	// Check from address
	if risk, exists := cd.riskAddresses[tx.FromAddress]; exists {
		result.AddressRisks[tx.FromAddress] = risk
		if risk >= RiskLevelHigh {
			result.SuspiciousFlags = append(result.SuspiciousFlags, "high_risk_sender")
		}
	}

	// Check to address
	if risk, exists := cd.riskAddresses[tx.ToAddress]; exists {
		result.AddressRisks[tx.ToAddress] = risk
		if risk >= RiskLevelHigh {
			result.SuspiciousFlags = append(result.SuspiciousFlags, "high_risk_recipient")
		}
	}

	// Check for known exchange addresses
	cd.checkExchangeAddresses(tx, result)
}

// detectMixers checks for mixer and tumbler usage
func (cd *CryptoDetector) detectMixers(tx *CryptoTransaction, result *CryptoAnalysisResult) {
	// Check known mixer addresses
	if cd.mixerAddresses[tx.FromAddress] || cd.mixerAddresses[tx.ToAddress] {
		result.MixerDetected = true
		result.SuspiciousFlags = append(result.SuspiciousFlags, "mixer_detected")
	}

	// Check for mixer patterns
	cd.detectMixerPatterns(tx, result)
}

// detectMixerPatterns looks for mixing service patterns
func (cd *CryptoDetector) detectMixerPatterns(tx *CryptoTransaction, result *CryptoAnalysisResult) {
	// Check for round number amounts (common in mixers)
	if cd.isRoundAmount(tx.Amount) {
		result.SuspiciousFlags = append(result.SuspiciousFlags, "round_amount_pattern")
	}

	// Check for common mixer fee structures
	if cd.isMixerFeePattern(tx.Fee, tx.Amount) {
		result.SuspiciousFlags = append(result.SuspiciousFlags, "mixer_fee_pattern")
	}
}

// performChainAnalysis traces the transaction through the blockchain
func (cd *CryptoDetector) performChainAnalysis(ctx context.Context, tx *CryptoTransaction) *ChainAnalysisResult {
	// This would integrate with blockchain analysis services
	// For now, return a simplified analysis
	return &ChainAnalysisResult{
		HopCount:       1,
		KnownExchanges: []string{},
		KnownMixers:    []string{},
		TotalVolume:    tx.Amount,
		TimeSpan:       time.Duration(0),
		ClusterAnalysis: &ClusterInfo{
			ClusterID:    "unknown",
			AddressCount: 1,
			TotalVolume:  tx.Amount,
			RiskLevel:    RiskLevelLow,
			EntityType:   "individual",
		},
	}
}

// performTaintAnalysis analyzes fund sources and mixing
func (cd *CryptoDetector) performTaintAnalysis(ctx context.Context, tx *CryptoTransaction) *TaintAnalysisResult {
	// This would integrate with taint analysis services
	// For now, return a simplified analysis
	return &TaintAnalysisResult{
		TaintScore:     decimal.NewFromFloat(0.1),
		TaintedSources: []string{},
		CleanSources:   []string{tx.FromAddress},
		MixingDetected: false,
		LayeringDepth:  1,
	}
}

// calculateRiskScore computes overall risk score
func (cd *CryptoDetector) calculateRiskScore(result *CryptoAnalysisResult) {
	score := decimal.Zero

	// Address risk contribution
	for _, risk := range result.AddressRisks {
		switch risk {
		case RiskLevelLow:
			score = score.Add(decimal.NewFromFloat(0.1))
		case RiskLevelMedium:
			score = score.Add(decimal.NewFromFloat(0.3))
		case RiskLevelHigh:
			score = score.Add(decimal.NewFromFloat(0.6))
		case RiskLevelCritical:
			score = score.Add(decimal.NewFromFloat(1.0))
		}
	}

	// Mixer detection contribution
	if result.MixerDetected {
		score = score.Add(decimal.NewFromFloat(0.8))
	}

	// Taint analysis contribution
	if result.TaintAnalysis != nil {
		score = score.Add(result.TaintAnalysis.TaintScore)
	}

	// Chain analysis contribution
	if result.ChainAnalysis != nil && result.ChainAnalysis.ClusterAnalysis != nil {
		switch result.ChainAnalysis.ClusterAnalysis.RiskLevel {
		case RiskLevelHigh, RiskLevelCritical:
			score = score.Add(decimal.NewFromFloat(0.5))
		}
	}

	// Normalize score to 0-1 range
	if score.GreaterThan(decimal.NewFromInt(1)) {
		score = decimal.NewFromInt(1)
	}

	result.RiskScore = score
}

// generateRecommendations provides action recommendations
func (cd *CryptoDetector) generateRecommendations(result *CryptoAnalysisResult) {
	if result.RiskScore.GreaterThan(decimal.NewFromFloat(0.8)) {
		result.Recommendations = append(result.Recommendations, "Block transaction")
		result.Recommendations = append(result.Recommendations, "File SAR")
	} else if result.RiskScore.GreaterThan(decimal.NewFromFloat(0.5)) {
		result.Recommendations = append(result.Recommendations, "Manual review required")
		result.Recommendations = append(result.Recommendations, "Enhanced monitoring")
	} else if result.RiskScore.GreaterThan(decimal.NewFromFloat(0.3)) {
		result.Recommendations = append(result.Recommendations, "Automated monitoring")
	}

	if result.MixerDetected {
		result.Recommendations = append(result.Recommendations, "Investigate mixing activity")
	}
}

// checkExchangeAddresses identifies known exchange addresses
func (cd *CryptoDetector) checkExchangeAddresses(tx *CryptoTransaction, result *CryptoAnalysisResult) {
	// This would check against known exchange address databases
	// Implementation would vary based on the specific service used
}

// isRoundAmount checks if amount follows round number patterns
func (cd *CryptoDetector) isRoundAmount(amount decimal.Decimal) bool {
	// Check for round numbers like 1.0, 10.0, 100.0, etc.
	str := amount.String()
	return strings.HasSuffix(str, ".0") || strings.HasSuffix(str, "00")
}

// isMixerFeePattern checks for common mixer fee structures
func (cd *CryptoDetector) isMixerFeePattern(fee, amount decimal.Decimal) bool {
	if amount.IsZero() {
		return false
	}

	feePercent := fee.Div(amount).Mul(decimal.NewFromInt(100))

	// Common mixer fee ranges (1-5%)
	return feePercent.GreaterThanOrEqual(decimal.NewFromInt(1)) &&
		feePercent.LessThanOrEqual(decimal.NewFromInt(5))
}

// AddRiskAddress adds an address to the risk database
func (cd *CryptoDetector) AddRiskAddress(address string, risk RiskLevel) {
	cd.riskAddresses[address] = risk
}

// AddMixerAddress adds an address to the mixer database
func (cd *CryptoDetector) AddMixerAddress(address string) {
	cd.mixerAddresses[address] = true
}

// AddExchangeRating adds an exchange risk rating
func (cd *CryptoDetector) AddExchangeRating(exchange string, rating ExchangeRisk) {
	cd.exchangeRatings[exchange] = rating
}
