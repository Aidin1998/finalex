package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// IPReputationService provides IP reputation scoring and threat intelligence
type IPReputationService struct {
	redis       *redis.Client
	logger      *zap.Logger
	threatFeeds map[string]*ThreatFeed
	asnDatabase *ASNDatabase
	config      *ReputationConfig
}

// ReputationConfig defines reputation service configuration
type ReputationConfig struct {
	DefaultScore      float64       `json:"default_score"`
	MinScore          float64       `json:"min_score"`
	MaxScore          float64       `json:"max_score"`
	DecayRate         float64       `json:"decay_rate"`
	UpdateInterval    time.Duration `json:"update_interval"`
	ThreatFeedEnabled bool          `json:"threat_feed_enabled"`
	ASNScoringEnabled bool          `json:"asn_scoring_enabled"`
	GeoScoringEnabled bool          `json:"geo_scoring_enabled"`
	BehaviorWeight    float64       `json:"behavior_weight"`
	ThreatWeight      float64       `json:"threat_weight"`
	ASNWeight         float64       `json:"asn_weight"`
	GeoWeight         float64       `json:"geo_weight"`
}

// ThreatFeed represents a threat intelligence feed
type ThreatFeed struct {
	Name           string                      `json:"name"`
	URL            string                      `json:"url"`
	APIKey         string                      `json:"api_key"`
	Enabled        bool                        `json:"enabled"`
	UpdateInterval time.Duration               `json:"update_interval"`
	LastUpdate     time.Time                   `json:"last_update"`
	Indicators     map[string]*ThreatIndicator `json:"indicators"`
	Weight         float64                     `json:"weight"`
}

// ThreatIndicator represents a threat intelligence indicator
type ThreatIndicator struct {
	IP          string                 `json:"ip"`
	ThreatType  string                 `json:"threat_type"`
	Confidence  float64                `json:"confidence"`
	FirstSeen   time.Time              `json:"first_seen"`
	LastSeen    time.Time              `json:"last_seen"`
	Source      string                 `json:"source"`
	Description string                 `json:"description"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// ASNDatabase provides ASN-based reputation scoring
type ASNDatabase struct {
	asnReputations map[int]*ASNReputation
	lastUpdate     time.Time
}

// ASNReputation represents reputation information for an ASN
type ASNReputation struct {
	ASN               int       `json:"asn"`
	Organization      string    `json:"organization"`
	Country           string    `json:"country"`
	ReputationScore   float64   `json:"reputation_score"`
	ThreatLevel       string    `json:"threat_level"`
	LastIncident      time.Time `json:"last_incident"`
	IncidentCount     int64     `json:"incident_count"`
	IsHostingProvider bool      `json:"is_hosting_provider"`
	IsResidential     bool      `json:"is_residential"`
	IsTor             bool      `json:"is_tor"`
	IsVPN             bool      `json:"is_vpn"`
	IsProxy           bool      `json:"is_proxy"`
}

// IPReputation represents comprehensive IP reputation information
type IPReputation struct {
	IP               string                 `json:"ip"`
	OverallScore     float64                `json:"overall_score"`
	BehaviorScore    float64                `json:"behavior_score"`
	ThreatScore      float64                `json:"threat_score"`
	ASNScore         float64                `json:"asn_score"`
	GeoScore         float64                `json:"geo_score"`
	LastUpdated      time.Time              `json:"last_updated"`
	ThreatIndicators []*ThreatIndicator     `json:"threat_indicators"`
	BehaviorMetrics  *BehaviorMetrics       `json:"behavior_metrics"`
	ASNInfo          *ASNReputation         `json:"asn_info"`
	GeoInfo          *GeoInfo               `json:"geo_info"`
	Metadata         map[string]interface{} `json:"metadata"`
}

// BehaviorMetrics tracks IP behavior patterns
type BehaviorMetrics struct {
	RequestCount          int64     `json:"request_count"`
	FailedAuthAttempts    int64     `json:"failed_auth_attempts"`
	SuspiciousActivities  int64     `json:"suspicious_activities"`
	RateLimitViolations   int64     `json:"rate_limit_violations"`
	FirstSeen             time.Time `json:"first_seen"`
	LastSeen              time.Time `json:"last_seen"`
	AverageRequestRate    float64   `json:"average_request_rate"`
	PeakRequestRate       float64   `json:"peak_request_rate"`
	UserAgentDiversity    int       `json:"user_agent_diversity"`
	EndpointDiversity     int       `json:"endpoint_diversity"`
	TimePatternAnomaly    float64   `json:"time_pattern_anomaly"`
	RequestPatternAnomaly float64   `json:"request_pattern_anomaly"`
}

// GeoInfo provides geographical information about an IP
type GeoInfo struct {
	Country      string  `json:"country"`
	CountryCode  string  `json:"country_code"`
	Region       string  `json:"region"`
	City         string  `json:"city"`
	Latitude     float64 `json:"latitude"`
	Longitude    float64 `json:"longitude"`
	ASN          int     `json:"asn"`
	ISP          string  `json:"isp"`
	Organization string  `json:"organization"`
	IsVPN        bool    `json:"is_vpn"`
	IsProxy      bool    `json:"is_proxy"`
	IsTor        bool    `json:"is_tor"`
	IsHosting    bool    `json:"is_hosting"`
	ThreatLevel  string  `json:"threat_level"`
}

// NewIPReputationService creates a new IP reputation service
func NewIPReputationService(redis *redis.Client, logger *zap.Logger) *IPReputationService {
	config := &ReputationConfig{
		DefaultScore:      0.5,
		MinScore:          0.0,
		MaxScore:          1.0,
		DecayRate:         0.01,
		UpdateInterval:    time.Hour,
		ThreatFeedEnabled: true,
		ASNScoringEnabled: true,
		GeoScoringEnabled: true,
		BehaviorWeight:    0.4,
		ThreatWeight:      0.3,
		ASNWeight:         0.2,
		GeoWeight:         0.1,
	}

	service := &IPReputationService{
		redis:       redis,
		logger:      logger,
		threatFeeds: make(map[string]*ThreatFeed),
		asnDatabase: &ASNDatabase{
			asnReputations: make(map[int]*ASNReputation),
		},
		config: config,
	}

	// Initialize threat feeds
	service.initializeThreatFeeds()

	return service
}

// GetReputationScore returns the overall reputation score for an IP
func (irs *IPReputationService) GetReputationScore(ctx context.Context, ip string) float64 {
	reputation := irs.GetIPReputation(ctx, ip)
	return reputation.OverallScore
}

// GetIPReputation returns comprehensive reputation information for an IP
func (irs *IPReputationService) GetIPReputation(ctx context.Context, ip string) *IPReputation {
	// Try to get cached reputation first
	cached := irs.getCachedReputation(ctx, ip)
	if cached != nil && time.Since(cached.LastUpdated) < irs.config.UpdateInterval {
		return cached
	}

	// Calculate fresh reputation
	reputation := &IPReputation{
		IP:               ip,
		LastUpdated:      time.Now(),
		ThreatIndicators: make([]*ThreatIndicator, 0),
		Metadata:         make(map[string]interface{}),
	}

	// Get behavior metrics
	reputation.BehaviorMetrics = irs.getBehaviorMetrics(ctx, ip)
	reputation.BehaviorScore = irs.calculateBehaviorScore(reputation.BehaviorMetrics)

	// Get threat intelligence
	reputation.ThreatIndicators = irs.getThreatIndicators(ctx, ip)
	reputation.ThreatScore = irs.calculateThreatScore(reputation.ThreatIndicators)

	// Get ASN information and score
	if irs.config.ASNScoringEnabled {
		reputation.ASNInfo = irs.getASNInfo(ip)
		reputation.ASNScore = irs.calculateASNScore(reputation.ASNInfo)
	} else {
		reputation.ASNScore = irs.config.DefaultScore
	}

	// Get geographical information and score
	if irs.config.GeoScoringEnabled {
		reputation.GeoInfo = irs.getGeoInfo(ip)
		reputation.GeoScore = irs.calculateGeoScore(reputation.GeoInfo)
	} else {
		reputation.GeoScore = irs.config.DefaultScore
	}

	// Calculate overall score
	reputation.OverallScore = irs.calculateOverallScore(reputation)

	// Cache the reputation
	irs.cacheReputation(ctx, reputation)

	return reputation
}

// calculateOverallScore calculates the weighted overall reputation score
func (irs *IPReputationService) calculateOverallScore(reputation *IPReputation) float64 {
	score := (reputation.BehaviorScore * irs.config.BehaviorWeight) +
		(reputation.ThreatScore * irs.config.ThreatWeight) +
		(reputation.ASNScore * irs.config.ASNWeight) +
		(reputation.GeoScore * irs.config.GeoWeight)

	// Ensure score is within bounds
	if score < irs.config.MinScore {
		score = irs.config.MinScore
	}
	if score > irs.config.MaxScore {
		score = irs.config.MaxScore
	}

	return score
}

// calculateBehaviorScore calculates reputation based on behavioral patterns
func (irs *IPReputationService) calculateBehaviorScore(metrics *BehaviorMetrics) float64 {
	if metrics == nil {
		return irs.config.DefaultScore
	}

	score := irs.config.DefaultScore

	// Positive factors
	if metrics.RequestCount > 0 {
		// Consistent usage patterns are good
		consistency := 1.0 - metrics.TimePatternAnomaly
		if consistency > 0.8 {
			score += 0.1
		}

		// Diverse but reasonable endpoint usage is good
		if metrics.EndpointDiversity > 1 && metrics.EndpointDiversity < 50 {
			score += 0.05
		}

		// Reasonable request rates are good
		if metrics.AverageRequestRate > 0 && metrics.AverageRequestRate < 10 {
			score += 0.05
		}
	}

	// Negative factors
	if metrics.FailedAuthAttempts > 0 {
		failureRate := float64(metrics.FailedAuthAttempts) / float64(metrics.RequestCount)
		score -= failureRate * 0.5
	}

	if metrics.SuspiciousActivities > 0 {
		suspiciousRate := float64(metrics.SuspiciousActivities) / float64(metrics.RequestCount)
		score -= suspiciousRate * 0.7
	}

	if metrics.RateLimitViolations > 0 {
		violationRate := float64(metrics.RateLimitViolations) / float64(metrics.RequestCount)
		score -= violationRate * 0.3
	}

	// High request rate anomalies are suspicious
	if metrics.RequestPatternAnomaly > 0.7 {
		score -= 0.2
	}

	// Ensure score is within bounds
	if score < irs.config.MinScore {
		score = irs.config.MinScore
	}
	if score > irs.config.MaxScore {
		score = irs.config.MaxScore
	}

	return score
}

// calculateThreatScore calculates reputation based on threat intelligence
func (irs *IPReputationService) calculateThreatScore(indicators []*ThreatIndicator) float64 {
	if len(indicators) == 0 {
		return irs.config.DefaultScore
	}

	score := irs.config.DefaultScore

	for _, indicator := range indicators {
		// Recent threats have more impact
		age := time.Since(indicator.LastSeen)
		ageFactor := 1.0
		if age > time.Hour*24*30 { // Older than 30 days
			ageFactor = 0.5
		} else if age > time.Hour*24*7 { // Older than 7 days
			ageFactor = 0.7
		} else if age > time.Hour*24 { // Older than 1 day
			ageFactor = 0.9
		}

		// Apply threat impact based on type and confidence
		threatImpact := indicator.Confidence * ageFactor

		switch strings.ToLower(indicator.ThreatType) {
		case "malware", "botnet", "c2":
			score -= threatImpact * 0.8
		case "scanning", "bruteforce":
			score -= threatImpact * 0.6
		case "spam", "phishing":
			score -= threatImpact * 0.4
		case "suspicious":
			score -= threatImpact * 0.2
		default:
			score -= threatImpact * 0.3
		}
	}

	// Ensure score is within bounds
	if score < irs.config.MinScore {
		score = irs.config.MinScore
	}
	if score > irs.config.MaxScore {
		score = irs.config.MaxScore
	}

	return score
}

// calculateASNScore calculates reputation based on ASN information
func (irs *IPReputationService) calculateASNScore(asnInfo *ASNReputation) float64 {
	if asnInfo == nil {
		return irs.config.DefaultScore
	}

	score := asnInfo.ReputationScore

	// Adjust based on ASN characteristics
	if asnInfo.IsTor {
		score -= 0.3 // Tor exit nodes are higher risk
	}

	if asnInfo.IsVPN || asnInfo.IsProxy {
		score -= 0.1 // VPNs and proxies are slightly higher risk
	}

	if asnInfo.IsHostingProvider {
		score -= 0.05 // Hosting providers can be abused
	}

	if asnInfo.IsResidential {
		score += 0.05 // Residential IPs are generally lower risk
	}

	// Adjust based on recent incidents
	if asnInfo.LastIncident.After(time.Now().Add(-time.Hour * 24 * 7)) {
		score -= 0.1 // Recent incidents reduce score
	}

	// Ensure score is within bounds
	if score < irs.config.MinScore {
		score = irs.config.MinScore
	}
	if score > irs.config.MaxScore {
		score = irs.config.MaxScore
	}

	return score
}

// calculateGeoScore calculates reputation based on geographical information
func (irs *IPReputationService) calculateGeoScore(geoInfo *GeoInfo) float64 {
	if geoInfo == nil {
		return irs.config.DefaultScore
	}

	score := irs.config.DefaultScore

	// Country-based scoring (this would be configurable in practice)
	switch strings.ToUpper(geoInfo.CountryCode) {
	case "US", "CA", "GB", "DE", "FR", "AU", "JP", "KR", "SG":
		score += 0.1 // Generally trusted countries
	case "CN", "RU", "IR", "KP":
		score -= 0.2 // Higher risk countries
	}

	// Service type adjustments
	if geoInfo.IsVPN || geoInfo.IsProxy {
		score -= 0.1
	}

	if geoInfo.IsTor {
		score -= 0.3
	}

	if geoInfo.IsHosting {
		score -= 0.05
	}

	// Threat level adjustments
	switch strings.ToLower(geoInfo.ThreatLevel) {
	case "low":
		score += 0.05
	case "medium":
		// No adjustment
	case "high":
		score -= 0.1
	case "critical":
		score -= 0.2
	}

	// Ensure score is within bounds
	if score < irs.config.MinScore {
		score = irs.config.MinScore
	}
	if score > irs.config.MaxScore {
		score = irs.config.MaxScore
	}

	return score
}

// RecordBehavior records behavioral metrics for an IP
func (irs *IPReputationService) RecordBehavior(ctx context.Context, ip string, behaviorType string, metadata map[string]interface{}) error {
	key := fmt.Sprintf("reputation:behavior:%s", ip)

	// Get current metrics
	metrics := irs.getBehaviorMetrics(ctx, ip)

	// Update metrics based on behavior type
	switch behaviorType {
	case "request":
		metrics.RequestCount++
		metrics.LastSeen = time.Now()
		if metrics.FirstSeen.IsZero() {
			metrics.FirstSeen = time.Now()
		}

		// Update request rate
		duration := time.Since(metrics.FirstSeen)
		if duration > 0 {
			metrics.AverageRequestRate = float64(metrics.RequestCount) / duration.Seconds()
		}

	case "failed_auth":
		metrics.FailedAuthAttempts++

	case "suspicious":
		metrics.SuspiciousActivities++

	case "rate_limit":
		metrics.RateLimitViolations++

	case "user_agent":
		if userAgent, ok := metadata["user_agent"].(string); ok && userAgent != "" {
			// This would require more sophisticated tracking
			metrics.UserAgentDiversity++
		}

	case "endpoint":
		if endpoint, ok := metadata["endpoint"].(string); ok && endpoint != "" {
			// This would require more sophisticated tracking
			metrics.EndpointDiversity++
		}
	}

	// Store updated metrics
	metricsJSON, err := json.Marshal(metrics)
	if err != nil {
		return fmt.Errorf("failed to marshal behavior metrics: %w", err)
	}

	err = irs.redis.Set(ctx, key, metricsJSON, time.Hour*24*30).Err() // 30 days retention
	if err != nil {
		return fmt.Errorf("failed to store behavior metrics: %w", err)
	}

	return nil
}

// getBehaviorMetrics retrieves behavioral metrics for an IP
func (irs *IPReputationService) getBehaviorMetrics(ctx context.Context, ip string) *BehaviorMetrics {
	key := fmt.Sprintf("reputation:behavior:%s", ip)

	data, err := irs.redis.Get(ctx, key).Result()
	if err != nil {
		// Return empty metrics if not found
		return &BehaviorMetrics{
			RequestCount:          0,
			FailedAuthAttempts:    0,
			SuspiciousActivities:  0,
			RateLimitViolations:   0,
			FirstSeen:             time.Time{},
			LastSeen:              time.Time{},
			AverageRequestRate:    0,
			PeakRequestRate:       0,
			UserAgentDiversity:    0,
			EndpointDiversity:     0,
			TimePatternAnomaly:    0,
			RequestPatternAnomaly: 0,
		}
	}

	var metrics BehaviorMetrics
	err = json.Unmarshal([]byte(data), &metrics)
	if err != nil {
		irs.logger.Error("Failed to unmarshal behavior metrics", zap.Error(err))
		return &BehaviorMetrics{}
	}

	return &metrics
}

// getThreatIndicators retrieves threat intelligence indicators for an IP
func (irs *IPReputationService) getThreatIndicators(ctx context.Context, ip string) []*ThreatIndicator {
	indicators := make([]*ThreatIndicator, 0)

	// Check each threat feed
	for feedName, feed := range irs.threatFeeds {
		if !feed.Enabled {
			continue
		}

		if indicator, exists := feed.Indicators[ip]; exists {
			// Check if indicator is still fresh
			if time.Since(indicator.LastSeen) < time.Hour*24*30 { // 30 days
				indicators = append(indicators, indicator)
			}
		}

		irs.logger.Debug("Checked threat feed",
			zap.String("feed", feedName),
			zap.String("ip", ip),
			zap.Int("indicators_found", len(indicators)))
	}

	return indicators
}

// getASNInfo retrieves ASN information for an IP
func (irs *IPReputationService) getASNInfo(ip string) *ASNReputation {
	// In a real implementation, this would query an ASN database
	// For now, we'll return a basic implementation

	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return nil
	}

	// Mock ASN lookup - in practice this would use a real ASN database
	asn := irs.mockASNLookup(ip)

	if asnInfo, exists := irs.asnDatabase.asnReputations[asn]; exists {
		return asnInfo
	}

	// Return default ASN info if not found
	return &ASNReputation{
		ASN:               asn,
		Organization:      "Unknown",
		Country:           "Unknown",
		ReputationScore:   irs.config.DefaultScore,
		ThreatLevel:       "medium",
		LastIncident:      time.Time{},
		IncidentCount:     0,
		IsHostingProvider: false,
		IsResidential:     true,
		IsTor:             false,
		IsVPN:             false,
		IsProxy:           false,
	}
}

// mockASNLookup provides a mock ASN lookup (replace with real implementation)
func (irs *IPReputationService) mockASNLookup(ip string) int {
	// Simple hash-based mock ASN assignment
	// In practice, this would query a real ASN database
	hash := 0
	for _, b := range []byte(ip) {
		hash = (hash * 31) + int(b)
	}
	return (hash % 65535) + 1 // ASN range 1-65535
}

// getGeoInfo retrieves geographical information for an IP
func (irs *IPReputationService) getGeoInfo(ip string) *GeoInfo {
	// Mock geo lookup - in practice this would use a real geo IP service
	return &GeoInfo{
		Country:      "Unknown",
		CountryCode:  "XX",
		Region:       "Unknown",
		City:         "Unknown",
		Latitude:     0,
		Longitude:    0,
		ASN:          0,
		ISP:          "Unknown",
		Organization: "Unknown",
		IsVPN:        false,
		IsProxy:      false,
		IsTor:        false,
		IsHosting:    false,
		ThreatLevel:  "medium",
	}
}

// cacheReputation caches IP reputation information
func (irs *IPReputationService) cacheReputation(ctx context.Context, reputation *IPReputation) error {
	key := fmt.Sprintf("reputation:cache:%s", reputation.IP)

	data, err := json.Marshal(reputation)
	if err != nil {
		return fmt.Errorf("failed to marshal reputation: %w", err)
	}

	err = irs.redis.Set(ctx, key, data, irs.config.UpdateInterval*2).Err()
	if err != nil {
		return fmt.Errorf("failed to cache reputation: %w", err)
	}

	return nil
}

// getCachedReputation retrieves cached IP reputation information
func (irs *IPReputationService) getCachedReputation(ctx context.Context, ip string) *IPReputation {
	key := fmt.Sprintf("reputation:cache:%s", ip)

	data, err := irs.redis.Get(ctx, key).Result()
	if err != nil {
		return nil
	}

	var reputation IPReputation
	err = json.Unmarshal([]byte(data), &reputation)
	if err != nil {
		irs.logger.Error("Failed to unmarshal cached reputation", zap.Error(err))
		return nil
	}

	return &reputation
}

// initializeThreatFeeds initializes threat intelligence feeds
func (irs *IPReputationService) initializeThreatFeeds() {
	// Example threat feeds - in practice these would be real threat intelligence sources
	irs.threatFeeds["internal"] = &ThreatFeed{
		Name:           "Internal Threat Feed",
		Enabled:        true,
		UpdateInterval: time.Hour,
		LastUpdate:     time.Now(),
		Indicators:     make(map[string]*ThreatIndicator),
		Weight:         1.0,
	}

	irs.threatFeeds["abuse_ch"] = &ThreatFeed{
		Name:           "Abuse.ch",
		URL:            "https://feodotracker.abuse.ch/downloads/ipblocklist.txt",
		Enabled:        false, // Disabled by default - would need real implementation
		UpdateInterval: time.Hour * 6,
		LastUpdate:     time.Time{},
		Indicators:     make(map[string]*ThreatIndicator),
		Weight:         0.8,
	}
}
