// util.go: Comprehensive helpers for key generation, time, request analysis, etc.
package ratelimit

import (
	"crypto/sha256"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// KeyFromRequest generates a unique key for rate limiting (user, IP, API key, etc)
func KeyFromRequest(r *http.Request) string {
	// Priority-based key selection
	if userID := extractUserID(r); userID != "" {
		return fmt.Sprintf("user:%s", userID)
	}

	if apiKey := extractAPIKey(r); apiKey != "" {
		return fmt.Sprintf("api:%s", hashKey(apiKey))
	}

	if ip := extractClientIP(r); ip != "" {
		return fmt.Sprintf("ip:%s", ip)
	}

	return "anonymous"
}

// GenerateCompositeKey generates a composite key from multiple request attributes
func GenerateCompositeKey(r *http.Request, keyTypes []string) string {
	parts := make([]string, 0, len(keyTypes))

	for _, keyType := range keyTypes {
		switch keyType {
		case "user":
			if userID := extractUserID(r); userID != "" {
				parts = append(parts, fmt.Sprintf("user:%s", userID))
			}
		case "ip":
			if ip := extractClientIP(r); ip != "" {
				parts = append(parts, fmt.Sprintf("ip:%s", ip))
			}
		case "endpoint":
			endpoint := fmt.Sprintf("%s:%s", r.Method, normalizeEndpoint(r.URL.Path))
			parts = append(parts, fmt.Sprintf("endpoint:%s", endpoint))
		case "api_key":
			if apiKey := extractAPIKey(r); apiKey != "" {
				parts = append(parts, fmt.Sprintf("api:%s", hashKey(apiKey)))
			}
		case "role":
			if role := extractUserRole(r); role != "" {
				parts = append(parts, fmt.Sprintf("role:%s", role))
			}
		case "tier":
			if tier := extractUserTier(r); tier != "" {
				parts = append(parts, fmt.Sprintf("tier:%s", tier))
			}
		}
	}

	if len(parts) == 0 {
		return "default"
	}

	return strings.Join(parts, ":")
}

// normalizeEndpoint normalizes API endpoints for consistent rate limiting
func normalizeEndpoint(path string) string {
	// Remove query parameters
	if idx := strings.Index(path, "?"); idx != -1 {
		path = path[:idx]
	}

	// Normalize path separators
	path = strings.Trim(path, "/")

	// Replace path parameters with placeholders
	pathParamRegex := regexp.MustCompile(`/[0-9a-fA-F-]{8,}`)
	path = pathParamRegex.ReplaceAllString(path, "/{id}")

	// Replace numeric IDs
	numericRegex := regexp.MustCompile(`/\d+`)
	path = numericRegex.ReplaceAllString(path, "/{id}")

	return path
}

// hashKey creates a hash of sensitive keys for logging/storage
func hashKey(key string) string {
	hash := sha256.Sum256([]byte(key))
	return fmt.Sprintf("%x", hash[:8]) // First 8 bytes for brevity
}

// IsPrivateIP checks if an IP address is private
func IsPrivateIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}

	privateRanges := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
		"169.254.0.0/16",
		"::1/128",
		"fc00::/7",
		"fe80::/10",
	}

	for _, rangeStr := range privateRanges {
		_, cidr, err := net.ParseCIDR(rangeStr)
		if err == nil && cidr.Contains(ip) {
			return true
		}
	}

	return false
}

// GetRequestSize estimates the size of an HTTP request
func GetRequestSize(r *http.Request) int64 {
	size := int64(0)

	// Method + URL + Protocol
	size += int64(len(r.Method) + len(r.URL.String()) + len(r.Proto))

	// Headers
	for name, values := range r.Header {
		size += int64(len(name))
		for _, value := range values {
			size += int64(len(value))
		}
	}

	// Content Length
	if r.ContentLength > 0 {
		size += r.ContentLength
	}

	return size
}

// ParseRateLimitHeaders extracts rate limit information from headers
func ParseRateLimitHeaders(headers http.Header) map[string]int {
	limits := make(map[string]int)

	if limit := headers.Get("X-RateLimit-Limit"); limit != "" {
		if val, err := strconv.Atoi(limit); err == nil {
			limits["limit"] = val
		}
	}

	if remaining := headers.Get("X-RateLimit-Remaining"); remaining != "" {
		if val, err := strconv.Atoi(remaining); err == nil {
			limits["remaining"] = val
		}
	}

	if reset := headers.Get("X-RateLimit-Reset"); reset != "" {
		if val, err := strconv.Atoi(reset); err == nil {
			limits["reset"] = val
		}
	}

	return limits
}

// CalculateRetryAfter calculates an appropriate retry-after value
func CalculateRetryAfter(algorithm string, limit int, window time.Duration, remaining int) time.Duration {
	switch algorithm {
	case LimiterTokenBucket:
		// For token bucket, calculate based on refill rate
		if remaining <= 0 {
			refillRate := float64(limit) / window.Seconds()
			return time.Duration(1.0/refillRate) * time.Second
		}
		return 0

	case LimiterSlidingWindow:
		// For sliding window, calculate based on window size
		if remaining <= 0 {
			return window / time.Duration(limit)
		}
		return 0

	case "leaky_bucket":
		// For leaky bucket, calculate based on leak rate
		if remaining <= 0 {
			leakRate := float64(limit) / window.Seconds()
			return time.Duration(1.0/leakRate) * time.Second
		}
		return 0

	default:
		// Default fallback
		return time.Minute
	}
}

// SanitizeKeyComponent sanitizes a key component for safe storage
func SanitizeKeyComponent(component string) string {
	// Remove potentially dangerous characters
	sanitized := regexp.MustCompile(`[^a-zA-Z0-9\-_\.]`).ReplaceAllString(component, "_")

	// Limit length
	if len(sanitized) > 100 {
		sanitized = sanitized[:100]
	}

	return sanitized
}

// GetRequestFingerprint creates a unique fingerprint for a request
func GetRequestFingerprint(r *http.Request) string {
	components := []string{
		r.Method,
		normalizeEndpoint(r.URL.Path),
		extractClientIP(r),
		r.UserAgent(),
	}

	fingerprint := strings.Join(components, "|")
	return hashKey(fingerprint)
}

// IsHealthCheckRequest determines if a request is a health check
func IsHealthCheckRequest(r *http.Request) bool {
	healthPaths := []string{
		"/health",
		"/healthz",
		"/ping",
		"/status",
		"/.well-known/ready",
		"/.well-known/live",
	}

	path := strings.ToLower(r.URL.Path)
	for _, healthPath := range healthPaths {
		if path == healthPath || strings.HasSuffix(path, healthPath) {
			return true
		}
	}

	// Check User-Agent for common health check agents
	userAgent := strings.ToLower(r.UserAgent())
	healthAgents := []string{
		"kube-probe",
		"health",
		"ping",
		"monitor",
		"check",
	}

	for _, agent := range healthAgents {
		if strings.Contains(userAgent, agent) {
			return true
		}
	}

	return false
}

// ShouldBypassRateLimit determines if a request should bypass rate limiting
func ShouldBypassRateLimit(r *http.Request) bool {
	// Bypass health checks
	if IsHealthCheckRequest(r) {
		return true
	}

	// Bypass internal requests
	if r.Header.Get("X-Internal-Request") == "true" {
		return true
	}

	// Bypass admin requests (with proper authentication)
	if r.Header.Get("X-Admin-Request") == "true" {
		// TODO: Verify admin authentication
		return true
	}

	// Bypass requests from private IPs (configurable)
	if ip := extractClientIP(r); ip != "" && IsPrivateIP(ip) {
		// TODO: Make this configurable
		return false // For now, don't bypass private IPs
	}

	return false
}

// TimeWindow represents a time window for rate limiting
type TimeWindow struct {
	Start    time.Time
	Duration time.Duration
}

// Contains checks if a time is within the window
func (tw *TimeWindow) Contains(t time.Time) bool {
	return t.After(tw.Start) && t.Before(tw.Start.Add(tw.Duration))
}

// Remaining returns the remaining time in the window
func (tw *TimeWindow) Remaining(now time.Time) time.Duration {
	end := tw.Start.Add(tw.Duration)
	if now.After(end) {
		return 0
	}
	return end.Sub(now)
}

// Progress returns the progress through the window (0.0 to 1.0)
func (tw *TimeWindow) Progress(now time.Time) float64 {
	if now.Before(tw.Start) {
		return 0.0
	}

	elapsed := now.Sub(tw.Start)
	if elapsed >= tw.Duration {
		return 1.0
	}

	return float64(elapsed) / float64(tw.Duration)
}

// NewTimeWindow creates a new time window
func NewTimeWindow(start time.Time, duration time.Duration) *TimeWindow {
	return &TimeWindow{
		Start:    start,
		Duration: duration,
	}
}

// GetCurrentWindow returns the current time window for a given duration
func GetCurrentWindow(duration time.Duration) *TimeWindow {
	now := time.Now()
	// Align to window boundaries
	windowStart := now.Truncate(duration)
	return NewTimeWindow(windowStart, duration)
}

// FormatDuration formats a duration in a human-readable way
func FormatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d/time.Millisecond)
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%.1fm", d.Minutes())
	}
	return fmt.Sprintf("%.1fh", d.Hours())
}

// ValidateRateLimitConfig validates a rate limit configuration
func ValidateRateLimitConfig(config *RateLimitConfig) error {
	if config.Limit <= 0 {
		return fmt.Errorf("limit must be positive, got %d", config.Limit)
	}

	if config.Window <= 0 {
		return fmt.Errorf("window must be positive, got %v", config.Window)
	}

	if config.Type != LimiterTokenBucket &&
		config.Type != LimiterSlidingWindow &&
		config.Type != "leaky_bucket" {
		return fmt.Errorf("unsupported limiter type: %s", config.Type)
	}

	if config.Type == LimiterTokenBucket && config.Burst <= 0 {
		return fmt.Errorf("token bucket burst must be positive, got %d", config.Burst)
	}

	return nil
}
