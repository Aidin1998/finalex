package compliance

import (
	"strings"
	"time"
)

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
		lists:    make(map[string]*SanctionsList),
		lastSync: time.Now(),
	}
}

// checkName checks if a name matches any sanctions list
func (sc *SanctionsChecker) checkName(name string) bool {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	normalizedName := strings.ToLower(strings.TrimSpace(name))

	for _, list := range sc.lists {
		for _, entry := range list.Entries {
			if strings.Contains(normalizedName, strings.ToLower(entry)) {
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
			if strings.Contains(normalizedAddress, strings.ToLower(entry)) {
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

	list.Updated = time.Now()
	sc.lists[list.ID] = list
	sc.lastSync = time.Now()
}

// NewKYCProcessor creates a new KYC processor
func NewKYCProcessor() *KYCProcessor {
	return &KYCProcessor{
		providers: make(map[string]KYCProvider),
		config: &KYCConfig{
			RequiredDocuments:     []string{"passport", "driver_license", "national_id"},
			MinAge:                18,
			RestrictedCountries:   []string{"US", "CN", "KP", "IR", "CU", "SY"},
			AutoApprovalLimit:     3.0,
			ManualReviewThreshold: 7.0,
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
				Pattern:   "amount > 10000",
				Threshold: 10000,
				Action:    "flag",
				Active:    true,
				Weight:    2.0,
			},
			{
				ID:        "frequent_transactions",
				Name:      "Frequent Transactions",
				Pattern:   "count > 100 AND timeframe < 24h",
				Threshold: 100,
				Action:    "flag",
				Active:    true,
				Weight:    1.5,
			},
			{
				ID:        "round_amount",
				Name:      "Round Amount Pattern",
				Pattern:   "amount % 1000 == 0",
				Threshold: 0,
				Action:    "monitor",
				Active:    true,
				Weight:    0.5,
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
