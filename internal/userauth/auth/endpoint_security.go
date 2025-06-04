package auth

import (
	"regexp"
)

// SecurityLevel defines the security requirements for different endpoint types
type SecurityLevel int

const (
	// SecurityLevelCritical - Trading operations requiring ultra-low latency
	SecurityLevelCritical SecurityLevel = iota
	// SecurityLevelHigh - User management operations
	SecurityLevelHigh
	// SecurityLevelMedium - General API operations
	SecurityLevelMedium
	// SecurityLevelLow - Public information endpoints
	SecurityLevelLow
)

// EndpointClassification defines security requirements for an endpoint
type EndpointClassification struct {
	SecurityLevel   SecurityLevel
	HashAlgorithm   HashAlgorithm
	MaxLatencyMs    int
	RequiresMFA     bool
	RequiresTLS     bool
	RateLimitTier   RateLimitTier
	DDoSProtection  bool
	IPWhitelisting  bool
	SessionRequired bool
}

// HashAlgorithm defines the hashing algorithm to use
type HashAlgorithm int

const (
	HashSHA256 HashAlgorithm = iota
	HashBcrypt
	HashArgon2
)

// RateLimitTier defines rate limiting severity
type RateLimitTier int

const (
	RateLimitTierStrict RateLimitTier = iota
	RateLimitTierModerate
	RateLimitTierLenient
	RateLimitTierPublic
)

// EndpointSecurityManager manages endpoint-specific security policies
type EndpointSecurityManager struct {
	classifications map[string]*EndpointClassification
	patterns        []*endpointPattern
}

type endpointPattern struct {
	pattern        *regexp.Regexp
	classification *EndpointClassification
	priority       int
}

// NewEndpointSecurityManager creates a new endpoint security manager
func NewEndpointSecurityManager() *EndpointSecurityManager {
	esm := &EndpointSecurityManager{
		classifications: make(map[string]*EndpointClassification),
		patterns:        make([]*endpointPattern, 0),
	}

	esm.initializeDefaultClassifications()
	return esm
}

// initializeDefaultClassifications sets up default security classifications
func (esm *EndpointSecurityManager) initializeDefaultClassifications() {
	// Critical Trading Endpoints - Ultra-low latency, SHA256
	tradingClassification := &EndpointClassification{
		SecurityLevel:   SecurityLevelCritical,
		HashAlgorithm:   HashSHA256,
		MaxLatencyMs:    10,
		RequiresMFA:     false,
		RequiresTLS:     true,
		RateLimitTier:   RateLimitTierStrict,
		DDoSProtection:  true,
		IPWhitelisting:  false,
		SessionRequired: true,
	}

	// High Security User Management - bcrypt/Argon2
	userManagementClassification := &EndpointClassification{
		SecurityLevel:   SecurityLevelHigh,
		HashAlgorithm:   HashBcrypt,
		MaxLatencyMs:    500,
		RequiresMFA:     true,
		RequiresTLS:     true,
		RateLimitTier:   RateLimitTierModerate,
		DDoSProtection:  true,
		IPWhitelisting:  false,
		SessionRequired: true,
	}

	// Admin Operations - Highest security
	adminClassification := &EndpointClassification{
		SecurityLevel:   SecurityLevelHigh,
		HashAlgorithm:   HashArgon2,
		MaxLatencyMs:    1000,
		RequiresMFA:     true,
		RequiresTLS:     true,
		RateLimitTier:   RateLimitTierStrict,
		DDoSProtection:  true,
		IPWhitelisting:  true,
		SessionRequired: true,
	}

	// Market Data - Low latency, moderate security
	marketDataClassification := &EndpointClassification{
		SecurityLevel:   SecurityLevelMedium,
		HashAlgorithm:   HashSHA256,
		MaxLatencyMs:    50,
		RequiresMFA:     false,
		RequiresTLS:     true,
		RateLimitTier:   RateLimitTierLenient,
		DDoSProtection:  true,
		IPWhitelisting:  false,
		SessionRequired: false,
	}

	// Public endpoints
	publicClassification := &EndpointClassification{
		SecurityLevel:   SecurityLevelLow,
		HashAlgorithm:   HashSHA256,
		MaxLatencyMs:    100,
		RequiresMFA:     false,
		RequiresTLS:     false,
		RateLimitTier:   RateLimitTierPublic,
		DDoSProtection:  true,
		IPWhitelisting:  false,
		SessionRequired: false,
	}

	// Register patterns with priorities (higher priority = more specific)
	esm.addPattern("^/api/v1/trade/order.*", tradingClassification, 100)
	esm.addPattern("^/api/v1/trade/cancel.*", tradingClassification, 100)
	esm.addPattern("^/api/v1/trade/orderbook.*", tradingClassification, 100)
	esm.addPattern("^/api/v1/trade/.*", tradingClassification, 90)

	esm.addPattern("^/api/v1/marketdata/.*", marketDataClassification, 80)

	esm.addPattern("^/api/v1/user/.*", userManagementClassification, 70)
	esm.addPattern("^/api/v1/kyc/.*", userManagementClassification, 70)
	esm.addPattern("^/api/v1/compliance/.*", userManagementClassification, 70)

	esm.addPattern("^/api/v1/admin/.*", adminClassification, 60)

	esm.addPattern("^/api/v1/public/.*", publicClassification, 10)
	esm.addPattern("^/health.*", publicClassification, 5)
	esm.addPattern("^/metrics.*", publicClassification, 5)
}

// addPattern adds a new endpoint pattern with classification
func (esm *EndpointSecurityManager) addPattern(pattern string, classification *EndpointClassification, priority int) {
	regex := regexp.MustCompile(pattern)
	esm.patterns = append(esm.patterns, &endpointPattern{
		pattern:        regex,
		classification: classification,
		priority:       priority,
	})
}

// GetEndpointClassification returns security classification for an endpoint
func (esm *EndpointSecurityManager) GetEndpointClassification(path string) *EndpointClassification {
	// Check exact matches first
	if classification, exists := esm.classifications[path]; exists {
		return classification
	}

	// Check patterns, prioritizing higher priority patterns
	var bestMatch *EndpointClassification
	highestPriority := -1

	for _, pattern := range esm.patterns {
		if pattern.pattern.MatchString(path) && pattern.priority > highestPriority {
			bestMatch = pattern.classification
			highestPriority = pattern.priority
		}
	}

	if bestMatch != nil {
		return bestMatch
	}

	// Default classification for unknown endpoints
	return &EndpointClassification{
		SecurityLevel:   SecurityLevelMedium,
		HashAlgorithm:   HashBcrypt,
		MaxLatencyMs:    200,
		RequiresMFA:     true,
		RequiresTLS:     true,
		RateLimitTier:   RateLimitTierModerate,
		DDoSProtection:  true,
		IPWhitelisting:  false,
		SessionRequired: true,
	}
}

// SetEndpointClassification sets a specific classification for an endpoint
func (esm *EndpointSecurityManager) SetEndpointClassification(path string, classification *EndpointClassification) {
	esm.classifications[path] = classification
}

// IsHighLatencyEndpoint checks if an endpoint can tolerate higher latency
func (esm *EndpointSecurityManager) IsHighLatencyEndpoint(path string) bool {
	classification := esm.GetEndpointClassification(path)
	return classification.MaxLatencyMs >= 100
}

// IsCriticalTradingEndpoint checks if an endpoint is critical for trading
func (esm *EndpointSecurityManager) IsCriticalTradingEndpoint(path string) bool {
	classification := esm.GetEndpointClassification(path)
	return classification.SecurityLevel == SecurityLevelCritical
}

// GetRequiredHashAlgorithm returns the required hash algorithm for an endpoint
func (esm *EndpointSecurityManager) GetRequiredHashAlgorithm(path string) HashAlgorithm {
	classification := esm.GetEndpointClassification(path)
	return classification.HashAlgorithm
}

// GetRateLimitTier returns the rate limit tier for an endpoint
func (esm *EndpointSecurityManager) GetRateLimitTier(path string) RateLimitTier {
	classification := esm.GetEndpointClassification(path)
	return classification.RateLimitTier
}

// RequiresMFA checks if an endpoint requires multi-factor authentication
func (esm *EndpointSecurityManager) RequiresMFA(path string) bool {
	classification := esm.GetEndpointClassification(path)
	return classification.RequiresMFA
}

// RequiresDDoSProtection checks if an endpoint requires DDoS protection
func (esm *EndpointSecurityManager) RequiresDDoSProtection(path string) bool {
	classification := esm.GetEndpointClassification(path)
	return classification.DDoSProtection
}

// RequiresIPWhitelisting checks if an endpoint requires IP whitelisting
func (esm *EndpointSecurityManager) RequiresIPWhitelisting(path string) bool {
	classification := esm.GetEndpointClassification(path)
	return classification.IPWhitelisting
}

// String returns string representation of security level
func (sl SecurityLevel) String() string {
	switch sl {
	case SecurityLevelCritical:
		return "critical"
	case SecurityLevelHigh:
		return "high"
	case SecurityLevelMedium:
		return "medium"
	case SecurityLevelLow:
		return "low"
	default:
		return "unknown"
	}
}

// String returns string representation of hash algorithm
func (ha HashAlgorithm) String() string {
	switch ha {
	case HashSHA256:
		return "sha256"
	case HashBcrypt:
		return "bcrypt"
	case HashArgon2:
		return "argon2"
	default:
		return "unknown"
	}
}

// String returns string representation of rate limit tier
func (rlt RateLimitTier) String() string {
	switch rlt {
	case RateLimitTierStrict:
		return "strict"
	case RateLimitTierModerate:
		return "moderate"
	case RateLimitTierLenient:
		return "lenient"
	case RateLimitTierPublic:
		return "public"
	default:
		return "unknown"
	}
}
