package compliance

import (
	"strings"
	"sync"
	"time"
)

// PolicyManager manages compliance policies
type PolicyManager struct {
	mu       sync.RWMutex
	policies map[string]*Policy
	version  string
	updated  time.Time
}

// Policy represents a compliance policy
type Policy struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Active      bool                   `json:"active"`
	Version     string                 `json:"version"`
	Rules       []PolicyRule           `json:"rules"`
	Metadata    map[string]interface{} `json:"metadata"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	ExpiresAt   *time.Time             `json:"expires_at,omitempty"`
}

// PolicyRule represents a single rule within a policy
type PolicyRule struct {
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	Condition  string                 `json:"condition"`
	Action     string                 `json:"action"`
	Parameters map[string]interface{} `json:"parameters"`
	Active     bool                   `json:"active"`
}

// SanctionsChecker manages sanctions list checking
type SanctionsChecker struct {
	mu    sync.RWMutex
	lists map[string]*SanctionsList
}

// SanctionsList represents a sanctions list
type SanctionsList struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Source      string    `json:"source"`
	Entries     []Entry   `json:"entries"`
	LastUpdated time.Time `json:"last_updated"`
	Version     string    `json:"version"`
}

// Entry represents a sanctions list entry
type Entry struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	DateOfBirth string `json:"date_of_birth,omitempty"`
	Nationality string `json:"nationality,omitempty"`
	Address     string `json:"address,omitempty"`
}

// KYCProcessor manages KYC verification processes
type KYCProcessor struct {
	mu        sync.RWMutex
	providers map[string]KYCProvider
	config    *KYCConfig
}

// KYCProvider represents a KYC verification provider
type KYCProvider interface {
	VerifyIdentity(data map[string]interface{}) (*KYCResult, error)
	GetStatus(requestID string) (*KYCStatus, error)
}

// KYCConfig represents KYC configuration
type KYCConfig struct {
	RequiredDocuments   []string `json:"required_documents"`
	RestrictedCountries []string `json:"restricted_countries"`
	MinimumAge          int      `json:"minimum_age"`
	EnableFaceMatch     bool     `json:"enable_face_match"`
}

// KYCResult represents the result of KYC verification
type KYCResult struct {
	Status    string  `json:"status"`
	Score     float64 `json:"score"`
	Passed    bool    `json:"passed"`
	Reason    string  `json:"reason,omitempty"`
	RequestID string  `json:"request_id"`
}

// KYCStatus represents the status of a KYC request
type KYCStatus struct {
	RequestID string `json:"request_id"`
	Status    string `json:"status"`
	Progress  int    `json:"progress"`
}

// AMLProcessor manages AML (Anti-Money Laundering) processing
type AMLProcessor struct {
	mu        sync.RWMutex
	rules     []AMLRule
	patterns  map[string]*TransactionPattern
	threshold float64
}

// AMLRule represents an AML rule
type AMLRule struct {
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	Type       string                 `json:"type"`
	Condition  string                 `json:"condition"`
	Threshold  float64                `json:"threshold"`
	Action     string                 `json:"action"`
	Active     bool                   `json:"active"`
	Parameters map[string]interface{} `json:"parameters"`
}

// TransactionPattern represents a transaction pattern for AML analysis
type TransactionPattern struct {
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	Pattern    string                 `json:"pattern"`
	RiskScore  float64                `json:"risk_score"`
	Frequency  int                    `json:"frequency"`
	TimeWindow time.Duration          `json:"time_window"`
	Metadata   map[string]interface{} `json:"metadata"`
	CreatedAt  time.Time              `json:"created_at"`
	UpdatedAt  time.Time              `json:"updated_at"`
}

// NewPolicyManager creates a new policy manager
func NewPolicyManager() *PolicyManager {
	return &PolicyManager{
		policies: make(map[string]*Policy),
		version:  "1.0.0",
		updated:  time.Now(),
	}
}

// UpdatePolicy updates or adds a policy
func (pm *PolicyManager) UpdatePolicy(policy *Policy) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.policies[policy.ID] = policy
	pm.updated = time.Now()
}

// GetPolicy retrieves a policy by ID
func (pm *PolicyManager) GetPolicy(id string) (*Policy, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	policy, exists := pm.policies[id]
	return policy, exists
}

// GetActivePolicies returns all active policies
func (pm *PolicyManager) GetActivePolicies() []*Policy {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	var active []*Policy
	for _, policy := range pm.policies {
		if policy.Active {
			// Check if policy is expired
			if policy.ExpiresAt != nil && policy.ExpiresAt.Before(time.Now()) {
				continue
			}
			active = append(active, policy)
		}
	}
	return active
}

// NewSanctionsChecker creates a new sanctions checker
func NewSanctionsChecker() *SanctionsChecker {
	return &SanctionsChecker{
		lists: make(map[string]*SanctionsList),
	}
}

// checkName checks if a name matches any sanctions list
func (sc *SanctionsChecker) checkName(name string) bool {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	normalizedName := strings.ToLower(strings.TrimSpace(name))
	for _, list := range sc.lists {
		for _, entry := range list.Entries {
			if strings.Contains(normalizedName, strings.ToLower(entry.Name)) {
				return true
			}
		}
	}
	return false
}

// checkAddress checks if an address matches any sanctions list
func (sc *SanctionsChecker) checkAddress(address string) bool {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	normalizedAddress := strings.ToLower(strings.TrimSpace(address))
	for _, list := range sc.lists {
		for _, entry := range list.Entries {
			if entry.Address != "" && strings.Contains(normalizedAddress, strings.ToLower(entry.Address)) {
				return true
			}
		}
	}
	return false
}

// UpdateSanctionsList updates a sanctions list
func (sc *SanctionsChecker) UpdateSanctionsList(list *SanctionsList) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	list.LastUpdated = time.Now()
	sc.lists[list.ID] = list
}

// NewKYCProcessor creates a new KYC processor
func NewKYCProcessor() *KYCProcessor {
	return &KYCProcessor{
		providers: make(map[string]KYCProvider),
		config: &KYCConfig{
			RequiredDocuments:   []string{"passport", "driver_license", "national_id"},
			RestrictedCountries: []string{"US", "CN", "KP", "IR", "CU", "SY"},
			MinimumAge:          18,
			EnableFaceMatch:     true,
		},
	}
}

// hasRequiredDocuments checks if all required documents are provided
func (kp *KYCProcessor) hasRequiredDocuments(data map[string]interface{}) bool {
	documents, ok := data["documents"].(map[string]interface{})
	if !ok {
		return false
	}

	for _, required := range kp.config.RequiredDocuments {
		if _, exists := documents[required]; !exists {
			return false
		}
	}
	return true
}

// isRestrictedCountry checks if a country is in the restricted list
func (kp *KYCProcessor) isRestrictedCountry(country string) bool {
	for _, restricted := range kp.config.RestrictedCountries {
		if strings.EqualFold(country, restricted) {
			return true
		}
	}
	return false
}

// calculateKYCRiskScore calculates risk score based on KYC data
func (kp *KYCProcessor) calculateKYCRiskScore(data map[string]interface{}) float64 {
	score := 0.0

	// Check document quality
	if documents, ok := data["documents"].(map[string]interface{}); ok {
		for _, doc := range documents {
			if docMap, ok := doc.(map[string]interface{}); ok {
				if quality, ok := docMap["quality"].(float64); ok {
					if quality < 0.8 {
						score += 1.0
					}
				}
			}
		}
	}

	// Check address verification
	if addressVerified, ok := data["address_verified"].(bool); ok && !addressVerified {
		score += 1.5
	}

	// Check phone verification
	if phoneVerified, ok := data["phone_verified"].(bool); ok && !phoneVerified {
		score += 1.0
	}

	// Check employment status
	if employment, ok := data["employment"].(string); ok {
		if employment == "unemployed" || employment == "unknown" {
			score += 0.5
		}
	}

	// Check income level
	if income, ok := data["income"].(float64); ok {
		if income < 10000 { // Low income threshold
			score += 0.5
		}
	}

	return score
}

// NewAMLProcessor creates a new AML processor
func NewAMLProcessor() *AMLProcessor {
	return &AMLProcessor{
		rules: []AMLRule{
			{
				ID:        "large_transaction",
				Name:      "Large Transaction",
				Type:      "amount_threshold",
				Condition: "amount > 10000",
				Threshold: 10000,
				Action:    "flag",
				Active:    true,
				Parameters: map[string]interface{}{
					"weight": 2.0,
				},
			},
			{
				ID:        "frequent_transactions",
				Name:      "Frequent Transactions",
				Type:      "frequency_check",
				Condition: "count > 100 AND timeframe < 24h",
				Threshold: 100,
				Action:    "flag",
				Active:    true,
				Parameters: map[string]interface{}{
					"weight": 1.5,
				},
			},
			{
				ID:        "round_amount",
				Name:      "Round Amount Pattern",
				Type:      "pattern_detection",
				Condition: "amount % 1000 == 0",
				Threshold: 0,
				Action:    "monitor",
				Active:    true,
				Parameters: map[string]interface{}{
					"weight": 0.5,
				},
			},
		},
		patterns:  make(map[string]*TransactionPattern),
		threshold: 5.0,
	}
}

// analyzeTransactionPattern analyzes transaction patterns for AML risks
func (ap *AMLProcessor) analyzeTransactionPattern(txData map[string]interface{}) float64 {
	score := 0.0

	// Check transaction amount
	if amount, ok := txData["amount"].(float64); ok {
		if amount > 10000 {
			score += 2.0
		} else if amount > 5000 {
			score += 1.0
		}

		// Check for round amounts (potential structuring)
		if amount > 1000 && int(amount)%1000 == 0 {
			score += 0.5
		}
	}

	// Check transaction frequency
	if frequency, ok := txData["daily_frequency"].(float64); ok {
		if frequency > 50 {
			score += 2.0
		} else if frequency > 20 {
			score += 1.0
		}
	}

	// Check for cash transactions
	if txType, ok := txData["type"].(string); ok {
		if txType == "cash" || txType == "cash_equivalent" {
			score += 1.5
		}
	}

	// Check for cross-border transactions
	if crossBorder, ok := txData["cross_border"].(bool); ok && crossBorder {
		score += 1.0

		// Additional risk for high-risk countries
		if destCountry, ok := txData["destination_country"].(string); ok {
			if ap.isHighRiskJurisdiction(destCountry) {
				score += 2.0
			}
		}
	}

	// Check transaction timing (unusual hours)
	if hour, ok := txData["hour"].(float64); ok {
		if hour < 6 || hour > 22 { // Late night or early morning
			score += 0.5
		}
	}

	return score
}

// isHighRiskJurisdiction checks if a country is considered high-risk for AML
func (ap *AMLProcessor) isHighRiskJurisdiction(country string) bool {
	highRiskCountries := []string{
		"AF", "BD", "BO", "GH", "IR", "KP", "LK", "MM", "NI", "PK", "LK", "SY", "UG", "VU", "YE", "ZW",
	}

	for _, highRisk := range highRiskCountries {
		if strings.EqualFold(country, highRisk) {
			return true
		}
	}
	return false
}

// AddRule adds a new AML rule
func (ap *AMLProcessor) AddRule(rule AMLRule) {
	ap.mu.Lock()
	defer ap.mu.Unlock()

	ap.rules = append(ap.rules, rule)
}

// UpdatePattern updates a transaction pattern
func (ap *AMLProcessor) UpdatePattern(pattern *TransactionPattern) {
	ap.mu.Lock()
	defer ap.mu.Unlock()

	pattern.UpdatedAt = time.Now()
	ap.patterns[pattern.ID] = pattern
}

// GetMetrics returns current AML processing metrics
func (ap *AMLProcessor) GetMetrics() map[string]interface{} {
	ap.mu.RLock()
	defer ap.mu.RUnlock()

	return map[string]interface{}{
		"active_rules":       len(ap.rules),
		"monitored_patterns": len(ap.patterns),
		"threshold":          ap.threshold,
	}
}
