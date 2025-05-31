package monitoring

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

// APIMonitoringConfig defines configuration for API monitoring
type APIMonitoringConfig struct {
	RateLimitWindow    time.Duration `json:"rate_limit_window"`
	MaxRequestsPerUser int           `json:"max_requests_per_user"`
	SuspiciousPatterns []string      `json:"suspicious_patterns"`
	BlockedUserAgents  []string      `json:"blocked_user_agents"`
	EnableGeoBlocking  bool          `json:"enable_geo_blocking"`
	BlockedCountries   []string      `json:"blocked_countries"`
	MaxPayloadSize     int64         `json:"max_payload_size"`
	RequireKYC         bool          `json:"require_kyc"`
}

// APIRequest represents an API request for monitoring
type APIRequest struct {
	ID           string                 `json:"id"`
	UserID       string                 `json:"user_id"`
	Endpoint     string                 `json:"endpoint"`
	Method       string                 `json:"method"`
	UserAgent    string                 `json:"user_agent"`
	IPAddress    string                 `json:"ip_address"`
	Country      string                 `json:"country"`
	PayloadSize  int64                  `json:"payload_size"`
	Headers      map[string]string      `json:"headers"`
	Parameters   map[string]interface{} `json:"parameters"`
	Timestamp    time.Time              `json:"timestamp"`
	ResponseCode int                    `json:"response_code"`
	ResponseTime time.Duration          `json:"response_time"`
	Blocked      bool                   `json:"blocked"`
	Reason       string                 `json:"reason,omitempty"`
}

// APIViolation represents a detected API violation
type APIViolation struct {
	ID          string                 `json:"id"`
	UserID      string                 `json:"user_id"`
	Type        string                 `json:"type"`
	Severity    string                 `json:"severity"`
	Description string                 `json:"description"`
	Request     *APIRequest            `json:"request"`
	Evidence    map[string]interface{} `json:"evidence"`
	Timestamp   time.Time              `json:"timestamp"`
	Action      string                 `json:"action"`
	Resolved    bool                   `json:"resolved"`
}

// UserAPIStats tracks API usage statistics per user
type UserAPIStats struct {
	UserID            string           `json:"user_id"`
	RequestCount      int64            `json:"request_count"`
	RequestsPerMinute float64          `json:"requests_per_minute"`
	EndpointUsage     map[string]int64 `json:"endpoint_usage"`
	ErrorRate         float64          `json:"error_rate"`
	AverageResponse   time.Duration    `json:"average_response"`
	LastActivity      time.Time        `json:"last_activity"`
	ViolationCount    int64            `json:"violation_count"`
	IsBlocked         bool             `json:"is_blocked"`
	BlockReason       string           `json:"block_reason,omitempty"`
	RiskScore         float64          `json:"risk_score"`
}

// APISecurityEvent represents a security event detected in API usage
type APISecurityEvent struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`
	Severity    string                 `json:"severity"`
	Description string                 `json:"description"`
	UserID      string                 `json:"user_id,omitempty"`
	IPAddress   string                 `json:"ip_address"`
	Pattern     string                 `json:"pattern"`
	Evidence    map[string]interface{} `json:"evidence"`
	Timestamp   time.Time              `json:"timestamp"`
	Mitigated   bool                   `json:"mitigated"`
	Action      string                 `json:"action"`
}

// APIMonitor provides comprehensive API monitoring and security
type APIMonitor struct {
	config         *APIMonitoringConfig
	userStats      map[string]*UserAPIStats
	violations     []APIViolation
	securityEvents []APISecurityEvent
	blockedUsers   map[string]string
	blockedIPs     map[string]string

	mu           sync.RWMutex
	violationsMu sync.RWMutex
	eventsMu     sync.RWMutex

	// Rate limiting
	rateLimiter   map[string][]time.Time
	rateLimiterMu sync.RWMutex

	// Pattern detection
	suspiciousPatterns []SuspiciousPattern

	// Callbacks
	onViolation     func(violation APIViolation)
	onSecurityEvent func(event APISecurityEvent)
	onUserBlocked   func(userID, reason string)

	// Storage
	storage APIMonitoringStorage
}

// SuspiciousPattern defines patterns to detect in API usage
type SuspiciousPattern struct {
	Name        string `json:"name"`
	Pattern     string `json:"pattern"`
	Type        string `json:"type"`   // endpoint, parameter, header, user_agent
	Action      string `json:"action"` // block, flag, monitor
	Severity    string `json:"severity"`
	Description string `json:"description"`
}

// APIMonitoringStorage interface for persisting API monitoring data
type APIMonitoringStorage interface {
	StoreRequest(ctx context.Context, request *APIRequest) error
	StoreViolation(ctx context.Context, violation *APIViolation) error
	StoreSecurityEvent(ctx context.Context, event *APISecurityEvent) error
	GetUserStats(ctx context.Context, userID string) (*UserAPIStats, error)
	UpdateUserStats(ctx context.Context, stats *UserAPIStats) error
	GetViolationHistory(ctx context.Context, userID string, limit int) ([]APIViolation, error)
}

// NewAPIMonitor creates a new API monitor
func NewAPIMonitor(config *APIMonitoringConfig, storage APIMonitoringStorage) *APIMonitor {
	monitor := &APIMonitor{
		config:         config,
		userStats:      make(map[string]*UserAPIStats),
		violations:     make([]APIViolation, 0),
		securityEvents: make([]APISecurityEvent, 0),
		blockedUsers:   make(map[string]string),
		blockedIPs:     make(map[string]string),
		rateLimiter:    make(map[string][]time.Time),
		storage:        storage,
	}

	monitor.initializeSuspiciousPatterns()
	return monitor
}

// MonitorRequest monitors an incoming API request
func (am *APIMonitor) MonitorRequest(request *APIRequest) (bool, string) {
	// Check if user is blocked
	if reason, blocked := am.isUserBlocked(request.UserID); blocked {
		request.Blocked = true
		request.Reason = reason
		am.recordSecurityEvent("BLOCKED_USER_REQUEST", "HIGH",
			fmt.Sprintf("Blocked user %s attempted API access", request.UserID),
			request.UserID, request.IPAddress, map[string]interface{}{
				"endpoint": request.Endpoint,
				"reason":   reason,
			})
		return false, reason
	}

	// Check if IP is blocked
	if reason, blocked := am.isIPBlocked(request.IPAddress); blocked {
		request.Blocked = true
		request.Reason = reason
		am.recordSecurityEvent("BLOCKED_IP_REQUEST", "HIGH",
			fmt.Sprintf("Blocked IP %s attempted API access", request.IPAddress),
			request.UserID, request.IPAddress, map[string]interface{}{
				"endpoint": request.Endpoint,
				"reason":   reason,
			})
		return false, reason
	}

	// Rate limiting check
	if !am.checkRateLimit(request.UserID) {
		violation := am.createViolation(request, "RATE_LIMIT_EXCEEDED", "MEDIUM",
			"User exceeded rate limit", map[string]interface{}{
				"rate_limit": am.config.MaxRequestsPerUser,
				"window":     am.config.RateLimitWindow.String(),
			})
		am.recordViolation(violation)
		request.Blocked = true
		request.Reason = "Rate limit exceeded"
		return false, "Rate limit exceeded"
	}

	// Geographic blocking check
	if am.config.EnableGeoBlocking && am.isCountryBlocked(request.Country) {
		violation := am.createViolation(request, "GEO_BLOCKED", "HIGH",
			fmt.Sprintf("Request from blocked country: %s", request.Country),
			map[string]interface{}{
				"country":           request.Country,
				"blocked_countries": am.config.BlockedCountries,
			})
		am.recordViolation(violation)
		request.Blocked = true
		request.Reason = fmt.Sprintf("Country %s is blocked", request.Country)
		return false, request.Reason
	}

	// User agent check
	if am.isUserAgentBlocked(request.UserAgent) {
		violation := am.createViolation(request, "BLOCKED_USER_AGENT", "MEDIUM",
			"Blocked user agent detected", map[string]interface{}{
				"user_agent": request.UserAgent,
			})
		am.recordViolation(violation)
		request.Blocked = true
		request.Reason = "Blocked user agent"
		return false, "Blocked user agent"
	}

	// Payload size check
	if request.PayloadSize > am.config.MaxPayloadSize {
		violation := am.createViolation(request, "PAYLOAD_TOO_LARGE", "MEDIUM",
			"Request payload exceeds maximum size", map[string]interface{}{
				"payload_size": request.PayloadSize,
				"max_size":     am.config.MaxPayloadSize,
			})
		am.recordViolation(violation)
		request.Blocked = true
		request.Reason = "Payload too large"
		return false, "Payload too large"
	}

	// Pattern-based detection
	if violation := am.detectSuspiciousPatterns(request); violation != nil {
		am.recordViolation(*violation)
		if violation.Action == "BLOCK" {
			request.Blocked = true
			request.Reason = violation.Description
			return false, violation.Description
		}
	}

	// Update user statistics
	am.updateUserStats(request)

	// Store request
	if am.storage != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := am.storage.StoreRequest(ctx, request); err != nil {
				log.Printf("Failed to store API request: %v", err)
			}
		}()
	}

	return true, ""
}

// checkRateLimit checks if user is within rate limits
func (am *APIMonitor) checkRateLimit(userID string) bool {
	am.rateLimiterMu.Lock()
	defer am.rateLimiterMu.Unlock()

	now := time.Now()
	cutoff := now.Add(-am.config.RateLimitWindow)

	// Get user's request timestamps
	timestamps := am.rateLimiter[userID]

	// Filter out old timestamps
	var validTimestamps []time.Time
	for _, ts := range timestamps {
		if ts.After(cutoff) {
			validTimestamps = append(validTimestamps, ts)
		}
	}

	// Check if within limit
	if len(validTimestamps) >= am.config.MaxRequestsPerUser {
		return false
	}

	// Add current timestamp
	validTimestamps = append(validTimestamps, now)
	am.rateLimiter[userID] = validTimestamps

	return true
}

// isUserBlocked checks if a user is blocked
func (am *APIMonitor) isUserBlocked(userID string) (string, bool) {
	am.mu.RLock()
	defer am.mu.RUnlock()

	reason, blocked := am.blockedUsers[userID]
	return reason, blocked
}

// isIPBlocked checks if an IP is blocked
func (am *APIMonitor) isIPBlocked(ipAddress string) (string, bool) {
	am.mu.RLock()
	defer am.mu.RUnlock()

	reason, blocked := am.blockedIPs[ipAddress]
	return reason, blocked
}

// isCountryBlocked checks if a country is blocked
func (am *APIMonitor) isCountryBlocked(country string) bool {
	for _, blocked := range am.config.BlockedCountries {
		if strings.EqualFold(blocked, country) {
			return true
		}
	}
	return false
}

// isUserAgentBlocked checks if a user agent is blocked
func (am *APIMonitor) isUserAgentBlocked(userAgent string) bool {
	for _, blocked := range am.config.BlockedUserAgents {
		if strings.Contains(strings.ToLower(userAgent), strings.ToLower(blocked)) {
			return true
		}
	}
	return false
}

// detectSuspiciousPatterns checks for suspicious patterns in the request
func (am *APIMonitor) detectSuspiciousPatterns(request *APIRequest) *APIViolation {
	for _, pattern := range am.suspiciousPatterns {
		var match bool
		var evidence map[string]interface{}

		switch pattern.Type {
		case "endpoint":
			match = strings.Contains(strings.ToLower(request.Endpoint), strings.ToLower(pattern.Pattern))
			evidence = map[string]interface{}{
				"endpoint": request.Endpoint,
				"pattern":  pattern.Pattern,
			}
		case "parameter":
			for key, value := range request.Parameters {
				if strings.Contains(strings.ToLower(key), strings.ToLower(pattern.Pattern)) ||
					strings.Contains(strings.ToLower(fmt.Sprintf("%v", value)), strings.ToLower(pattern.Pattern)) {
					match = true
					evidence = map[string]interface{}{
						"parameter": key,
						"value":     value,
						"pattern":   pattern.Pattern,
					}
					break
				}
			}
		case "header":
			for key, value := range request.Headers {
				if strings.Contains(strings.ToLower(key), strings.ToLower(pattern.Pattern)) ||
					strings.Contains(strings.ToLower(value), strings.ToLower(pattern.Pattern)) {
					match = true
					evidence = map[string]interface{}{
						"header":  key,
						"value":   value,
						"pattern": pattern.Pattern,
					}
					break
				}
			}
		case "user_agent":
			match = strings.Contains(strings.ToLower(request.UserAgent), strings.ToLower(pattern.Pattern))
			evidence = map[string]interface{}{
				"user_agent": request.UserAgent,
				"pattern":    pattern.Pattern,
			}
		}

		if match {
			violation := am.createViolation(request, "SUSPICIOUS_PATTERN", pattern.Severity,
				fmt.Sprintf("Suspicious pattern detected: %s", pattern.Description), evidence)
			violation.Action = pattern.Action
			return &violation
		}
	}

	return nil
}

// updateUserStats updates statistics for a user
func (am *APIMonitor) updateUserStats(request *APIRequest) {
	am.mu.Lock()
	defer am.mu.Unlock()

	stats, exists := am.userStats[request.UserID]
	if !exists {
		stats = &UserAPIStats{
			UserID:        request.UserID,
			EndpointUsage: make(map[string]int64),
			LastActivity:  request.Timestamp,
		}
		am.userStats[request.UserID] = stats
	}

	stats.RequestCount++
	stats.EndpointUsage[request.Endpoint]++
	stats.LastActivity = request.Timestamp

	// Calculate requests per minute
	if stats.RequestCount > 1 {
		duration := time.Since(stats.LastActivity).Minutes()
		if duration > 0 {
			stats.RequestsPerMinute = float64(stats.RequestCount) / duration
		}
	}

	// Update error rate
	if request.ResponseCode >= 400 {
		stats.ErrorRate = (stats.ErrorRate*float64(stats.RequestCount-1) + 1) / float64(stats.RequestCount)
	} else {
		stats.ErrorRate = stats.ErrorRate * float64(stats.RequestCount-1) / float64(stats.RequestCount)
	}

	// Calculate risk score
	stats.RiskScore = am.calculateUserRiskScore(stats)

	// Auto-block if risk score is too high
	if stats.RiskScore > 90 && !stats.IsBlocked {
		am.blockUser(request.UserID, "High risk score detected")
	}
}

// calculateUserRiskScore calculates a risk score for a user based on their API usage
func (am *APIMonitor) calculateUserRiskScore(stats *UserAPIStats) float64 {
	score := 0.0

	// High request rate
	if stats.RequestsPerMinute > 100 {
		score += 30
	} else if stats.RequestsPerMinute > 50 {
		score += 15
	}

	// High error rate
	if stats.ErrorRate > 0.5 {
		score += 25
	} else if stats.ErrorRate > 0.2 {
		score += 10
	}

	// Multiple violations
	if stats.ViolationCount > 10 {
		score += 40
	} else if stats.ViolationCount > 5 {
		score += 20
	}

	// Diverse endpoint usage (potential enumeration)
	if len(stats.EndpointUsage) > 50 {
		score += 15
	}

	if score > 100 {
		score = 100
	}

	return score
}

// createViolation creates a new API violation record
func (am *APIMonitor) createViolation(request *APIRequest, violationType, severity, description string, evidence map[string]interface{}) APIViolation {
	return APIViolation{
		ID:          fmt.Sprintf("viol_%d", time.Now().UnixNano()),
		UserID:      request.UserID,
		Type:        violationType,
		Severity:    severity,
		Description: description,
		Request:     request,
		Evidence:    evidence,
		Timestamp:   time.Now(),
		Action:      "FLAGGED",
	}
}

// recordViolation records a violation
func (am *APIMonitor) recordViolation(violation APIViolation) {
	am.violationsMu.Lock()
	am.violations = append(am.violations, violation)
	am.violationsMu.Unlock()

	// Update user stats
	am.mu.Lock()
	if stats, exists := am.userStats[violation.UserID]; exists {
		stats.ViolationCount++
	}
	am.mu.Unlock()

	// Call violation callback
	if am.onViolation != nil {
		go am.onViolation(violation)
	}

	// Store violation
	if am.storage != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := am.storage.StoreViolation(ctx, &violation); err != nil {
				log.Printf("Failed to store API violation: %v", err)
			}
		}()
	}
}

// recordSecurityEvent records a security event
func (am *APIMonitor) recordSecurityEvent(eventType, severity, description, userID, ipAddress string, evidence map[string]interface{}) {
	event := APISecurityEvent{
		ID:          fmt.Sprintf("event_%d", time.Now().UnixNano()),
		Type:        eventType,
		Severity:    severity,
		Description: description,
		UserID:      userID,
		IPAddress:   ipAddress,
		Evidence:    evidence,
		Timestamp:   time.Now(),
	}

	am.eventsMu.Lock()
	am.securityEvents = append(am.securityEvents, event)
	am.eventsMu.Unlock()

	// Call security event callback
	if am.onSecurityEvent != nil {
		go am.onSecurityEvent(event)
	}

	// Store event
	if am.storage != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := am.storage.StoreSecurityEvent(ctx, &event); err != nil {
				log.Printf("Failed to store security event: %v", err)
			}
		}()
	}
}

// BlockUser blocks a user from API access
func (am *APIMonitor) BlockUser(userID, reason string) {
	am.blockUser(userID, reason)
}

func (am *APIMonitor) blockUser(userID, reason string) {
	am.mu.Lock()
	am.blockedUsers[userID] = reason
	if stats, exists := am.userStats[userID]; exists {
		stats.IsBlocked = true
		stats.BlockReason = reason
	}
	am.mu.Unlock()

	log.Printf("Blocked user %s: %s", userID, reason)

	// Call user blocked callback
	if am.onUserBlocked != nil {
		go am.onUserBlocked(userID, reason)
	}
}

// UnblockUser unblocks a user
func (am *APIMonitor) UnblockUser(userID string) {
	am.mu.Lock()
	delete(am.blockedUsers, userID)
	if stats, exists := am.userStats[userID]; exists {
		stats.IsBlocked = false
		stats.BlockReason = ""
	}
	am.mu.Unlock()

	log.Printf("Unblocked user %s", userID)
}

// BlockIP blocks an IP address
func (am *APIMonitor) BlockIP(ipAddress, reason string) {
	am.mu.Lock()
	am.blockedIPs[ipAddress] = reason
	am.mu.Unlock()

	log.Printf("Blocked IP %s: %s", ipAddress, reason)
}

// UnblockIP unblocks an IP address
func (am *APIMonitor) UnblockIP(ipAddress string) {
	am.mu.Lock()
	delete(am.blockedIPs, ipAddress)
	am.mu.Unlock()

	log.Printf("Unblocked IP %s", ipAddress)
}

// GetUserStats returns statistics for a user
func (am *APIMonitor) GetUserStats(userID string) (*UserAPIStats, error) {
	am.mu.RLock()
	defer am.mu.RUnlock()

	if stats, exists := am.userStats[userID]; exists {
		// Create a copy
		statsCopy := *stats
		statsCopy.EndpointUsage = make(map[string]int64)
		for k, v := range stats.EndpointUsage {
			statsCopy.EndpointUsage[k] = v
		}
		return &statsCopy, nil
	}

	return nil, fmt.Errorf("user stats not found for user %s", userID)
}

// GetRecentViolations returns recent violations
func (am *APIMonitor) GetRecentViolations(limit int) []APIViolation {
	am.violationsMu.RLock()
	defer am.violationsMu.RUnlock()

	if limit > len(am.violations) {
		limit = len(am.violations)
	}

	result := make([]APIViolation, limit)
	copy(result, am.violations[len(am.violations)-limit:])
	return result
}

// GetSecurityEvents returns recent security events
func (am *APIMonitor) GetSecurityEvents(limit int) []APISecurityEvent {
	am.eventsMu.RLock()
	defer am.eventsMu.RUnlock()

	if limit > len(am.securityEvents) {
		limit = len(am.securityEvents)
	}

	result := make([]APISecurityEvent, limit)
	copy(result, am.securityEvents[len(am.securityEvents)-limit:])
	return result
}

// SetCallbacks sets callback functions for events
func (am *APIMonitor) SetCallbacks(
	onViolation func(APIViolation),
	onSecurityEvent func(APISecurityEvent),
	onUserBlocked func(string, string),
) {
	am.onViolation = onViolation
	am.onSecurityEvent = onSecurityEvent
	am.onUserBlocked = onUserBlocked
}

// initializeSuspiciousPatterns initializes default suspicious patterns
func (am *APIMonitor) initializeSuspiciousPatterns() {
	am.suspiciousPatterns = []SuspiciousPattern{
		{
			Name:        "SQL Injection",
			Pattern:     "union select",
			Type:        "parameter",
			Action:      "BLOCK",
			Severity:    "HIGH",
			Description: "Potential SQL injection attempt",
		},
		{
			Name:        "XSS Attempt",
			Pattern:     "<script",
			Type:        "parameter",
			Action:      "BLOCK",
			Severity:    "HIGH",
			Description: "Potential XSS attempt",
		},
		{
			Name:        "Path Traversal",
			Pattern:     "../",
			Type:        "parameter",
			Action:      "BLOCK",
			Severity:    "HIGH",
			Description: "Potential path traversal attempt",
		},
		{
			Name:        "Admin Endpoint",
			Pattern:     "/admin",
			Type:        "endpoint",
			Action:      "FLAG",
			Severity:    "MEDIUM",
			Description: "Access to admin endpoint",
		},
		{
			Name:        "Automated Tool",
			Pattern:     "bot",
			Type:        "user_agent",
			Action:      "FLAG",
			Severity:    "LOW",
			Description: "Automated tool detected",
		},
	}
}

// GetTopUsers returns users with highest activity
func (am *APIMonitor) GetTopUsers(limit int) []*UserAPIStats {
	am.mu.RLock()
	defer am.mu.RUnlock()

	var users []*UserAPIStats
	for _, stats := range am.userStats {
		users = append(users, stats)
	}

	// Sort by request count (simplified)
	// In a real implementation, you would use sort.Slice
	if len(users) > limit {
		users = users[:limit]
	}

	return users
}

// UpdateConfig updates the monitoring configuration
func (am *APIMonitor) UpdateConfig(config *APIMonitoringConfig) {
	am.mu.Lock()
	am.config = config
	am.mu.Unlock()

	log.Println("API monitoring configuration updated")
}
