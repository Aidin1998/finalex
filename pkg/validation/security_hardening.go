package validation

import (
	"crypto/md5"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// SecurityHardening provides advanced security validation and protection
type SecurityHardening struct {
	logger                 *zap.Logger
	suspiciousPatterns     []*regexp.Regexp
	maliciousIPRanges      []*net.IPNet
	requestFingerprints    map[string]time.Time
	maxRequestsPerIP       map[string]int
	maxRequestsPerEndpoint map[string]int
	cleanupInterval        time.Duration
}

// SecurityConfig holds security hardening configuration
type SecurityConfig struct {
	EnableIPBlacklisting       bool          `json:"enable_ip_blacklisting"`
	EnableFingerprintTracking  bool          `json:"enable_fingerprint_tracking"`
	EnableAdvancedSQLInjection bool          `json:"enable_advanced_sql_injection"`
	EnableXSSProtection        bool          `json:"enable_xss_protection"`
	EnableCSRFProtection       bool          `json:"enable_csrf_protection"`
	EnableRateLimiting         bool          `json:"enable_rate_limiting"`
	MaxRequestsPerMinute       int           `json:"max_requests_per_minute"`
	MaxRequestsPerEndpoint     int           `json:"max_requests_per_endpoint"`
	SuspiciousActivityWindow   time.Duration `json:"suspicious_activity_window"`
	BlockDuration              time.Duration `json:"block_duration"`
}

// DefaultSecurityConfig returns default security configuration
func DefaultSecurityConfig() *SecurityConfig {
	return &SecurityConfig{
		EnableIPBlacklisting:       true,
		EnableFingerprintTracking:  true,
		EnableAdvancedSQLInjection: true,
		EnableXSSProtection:        true,
		EnableCSRFProtection:       true,
		EnableRateLimiting:         true,
		MaxRequestsPerMinute:       300,
		MaxRequestsPerEndpoint:     100,
		SuspiciousActivityWindow:   5 * time.Minute,
		BlockDuration:              15 * time.Minute,
	}
}

// NewSecurityHardening creates a new security hardening instance
func NewSecurityHardening(logger *zap.Logger, config *SecurityConfig) *SecurityHardening {
	if config == nil {
		config = DefaultSecurityConfig()
	}

	sh := &SecurityHardening{
		logger:                 logger,
		requestFingerprints:    make(map[string]time.Time),
		maxRequestsPerIP:       make(map[string]int),
		maxRequestsPerEndpoint: make(map[string]int),
		cleanupInterval:        1 * time.Minute,
		maliciousIPRanges:      make([]*net.IPNet, 0),
	}

	sh.initializeSuspiciousPatterns()
	sh.initializeMaliciousIPRanges()

	return sh
}

// initializeSuspiciousPatterns sets up patterns for detecting malicious requests
func (sh *SecurityHardening) initializeSuspiciousPatterns() {
	patterns := []string{
		// Advanced SQL Injection patterns
		`(?i)(union\s+select|or\s+1\s*=\s*1|and\s+1\s*=\s*1)`,
		`(?i)(exec\s*\(|execute\s*\(|sp_executesql)`,
		`(?i)(xp_cmdshell|sp_oacreate|sp_oamethod)`,
		`(?i)(insert\s+into|update\s+set|delete\s+from|drop\s+table|truncate\s+table)`,
		`(?i)(information_schema|sysobjects|syscolumns|pg_tables)`,
		`(?i)(waitfor\s+delay|benchmark\s*\(|sleep\s*\()`,
		`(?i)(load_file\s*\(|into\s+outfile|into\s+dumpfile)`,
		`(?i)(\bchar\s*\(|\bascii\s*\(|\bord\s*\(|\bhex\s*\()`,

		// Advanced XSS patterns
		`(?i)(<script[^>]*>|</script>|javascript:|vbscript:)`,
		`(?i)(onload\s*=|onclick\s*=|onerror\s*=|onmouseover\s*=)`,
		`(?i)(eval\s*\(|setTimeout\s*\(|setInterval\s*\()`,
		`(?i)(<iframe[^>]*>|<embed[^>]*>|<object[^>]*>)`,
		`(?i)(document\.cookie|document\.write|window\.location)`,
		`(?i)(alert\s*\(|confirm\s*\(|prompt\s*\()`,
		`(?i)(<img[^>]*onerror|<svg[^>]*onload)`,
		// Command Injection patterns
		`(?i)(\|\s*nc\s|\|\s*netcat\s|\|\s*wget\s|\|\s*curl\s)`,
		`(?i)(\|\s*cat\s|\|\s*grep\s|\|\s*awk\s|\|\s*sed\s)`,
		`(?i)(;\s*rm\s|;\s*mv\s|;\s*cp\s|;\s*chmod\s)`,
		`(?i)(\$\(.*\)|` + "`" + `.*` + "`" + `|%\{.*\})`,
		`(?i)(&&\s*rm|&&\s*cat|&&\s*ls|&&\s*ps)`,

		// Directory Traversal patterns
		`(?i)(\.\.\/|\.\.\\|\.\./|\.\.\x5c)`,
		`(?i)(%2e%2e%2f|%2e%2e\x5c|%252e%252e%252f)`,
		`(?i)(\/etc\/passwd|\/etc\/shadow|\/etc\/hosts)`,
		`(?i)(\.\.%2f|\.\.%5c|%2e%2e%5c)`,

		// LDAP Injection patterns
		`(?i)(\*\)\(\||\*\)\(&|\)\(\||&\(\|)`,
		`(?i)(\|\(.*=\*\)|\*\)\(\*|\(\|.*\=\*)`,

		// XXE patterns
		`(?i)(<!ENTITY|<!DOCTYPE.*ENTITY|SYSTEM\s+\"file:)`,
		`(?i)(<\?xml.*encoding|<!ENTITY.*%|SYSTEM\s+\"http:)`,

		// NoSQL Injection patterns
		`(?i)(\$ne|\$gt|\$lt|\$regex|\$where)`,
		`(?i)(\$or\s*:\s*\[|\$and\s*:\s*\[)`,

		// CSRF patterns
		`(?i)(x-requested-with.*xmlhttprequest)`,

		// File Upload patterns
		`(?i)(\.php|\.jsp|\.asp|\.aspx|\.exe|\.sh|\.bat)$`,
		`(?i)(data:.*base64|data:text\/html)`,

		// Protocol violations
		`(?i)(file:\/\/|ftp:\/\/|gopher:\/\/|dict:\/\/)`,

		// Encoding evasion attempts
		`(?i)(%00|%0a|%0d|%1a|%2e%2e|%252e|%c0%af)`,
		`(?i)(\\x00|\\x0a|\\x0d|\\x1a|\\x2e|\\x2f)`,
		// Template injection
		`(?i)(\{\{.*\}\}|\[\[.*\]\]|<%.*%>|\$\{.*\})`,

		// Suspicious headers and user agents
		`(?i)(sqlmap|havij|pangolin|netsparker|acunetix)`,
		`(?i)(nikto|nmap|masscan|zap|burp)`,
	}

	sh.suspiciousPatterns = make([]*regexp.Regexp, len(patterns))
	for i, pattern := range patterns {
		sh.suspiciousPatterns[i] = regexp.MustCompile(pattern)
	}
}

// initializeMaliciousIPRanges sets up known malicious IP ranges
func (sh *SecurityHardening) initializeMaliciousIPRanges() {
	// Add known malicious IP ranges (examples - in production use threat intelligence feeds)
	maliciousRanges := []string{
		"10.0.0.0/8",     // Private networks (for demo)
		"192.168.0.0/16", // Private networks (for demo)
		"172.16.0.0/12",  // Private networks (for demo)
		// Add actual malicious IP ranges from threat intelligence
	}

	for _, cidr := range maliciousRanges {
		if _, ipnet, err := net.ParseCIDR(cidr); err == nil {
			sh.maliciousIPRanges = append(sh.maliciousIPRanges, ipnet)
		}
	}
}

// ValidateAdvancedSecurity performs comprehensive security validation
func (sh *SecurityHardening) ValidateAdvancedSecurity(c *gin.Context) []ValidationError {
	var errors []ValidationError

	clientIP := c.ClientIP()
	userAgent := c.GetHeader("User-Agent")
	referer := c.GetHeader("Referer")

	// 1. IP Blacklist Check
	if sh.isBlacklistedIP(clientIP) {
		errors = append(errors, ValidationError{
			Field:   "client_ip",
			Tag:     "blacklisted",
			Message: "Request from blacklisted IP address",
		})
	}

	// 2. Suspicious User Agent Check
	if sh.isSuspiciousUserAgent(userAgent) {
		errors = append(errors, ValidationError{
			Field:   "user_agent",
			Tag:     "suspicious",
			Message: "Suspicious user agent detected",
		})
	}

	// 3. Advanced Pattern Matching on all inputs
	inputs := sh.extractAllInputs(c)
	for field, value := range inputs {
		if violations := sh.detectAdvancedThreats(value); len(violations) > 0 {
			for _, violation := range violations {
				errors = append(errors, ValidationError{
					Field:   field,
					Tag:     "advanced_threat",
					Message: fmt.Sprintf("Advanced threat pattern detected: %s", violation),
				})
			}
		}
	}

	// 4. Request Fingerprinting and Rate Limiting
	fingerprint := sh.generateRequestFingerprint(c)
	if sh.isRateLimited(fingerprint, clientIP, c.Request.URL.Path) {
		errors = append(errors, ValidationError{
			Field:   "rate_limit",
			Tag:     "exceeded",
			Message: "Rate limit exceeded",
		})
	}

	// 5. CSRF Protection
	if sh.isCSRFVulnerable(c) {
		errors = append(errors, ValidationError{
			Field:   "csrf",
			Tag:     "vulnerable",
			Message: "Potential CSRF attack detected",
		})
	}

	// 6. Referer Validation
	if referer != "" && !sh.isValidReferer(referer, c.Request.Host) {
		errors = append(errors, ValidationError{
			Field:   "referer",
			Tag:     "invalid",
			Message: "Invalid referer header",
		})
	}

	// 7. Content-Length and Transfer-Encoding validation
	if sh.hasInvalidTransferEncoding(c) {
		errors = append(errors, ValidationError{
			Field:   "transfer_encoding",
			Tag:     "invalid",
			Message: "Invalid transfer encoding",
		})
	}

	return errors
}

// isBlacklistedIP checks if an IP is in the blacklist
func (sh *SecurityHardening) isBlacklistedIP(ip string) bool {
	clientIP := net.ParseIP(ip)
	if clientIP == nil {
		return false
	}

	for _, ipnet := range sh.maliciousIPRanges {
		if ipnet.Contains(clientIP) {
			return true
		}
	}
	return false
}

// isSuspiciousUserAgent checks for suspicious user agents
func (sh *SecurityHardening) isSuspiciousUserAgent(userAgent string) bool {
	suspiciousAgents := []string{
		"sqlmap", "havij", "pangolin", "netsparker", "acunetix",
		"nikto", "nmap", "masscan", "zap", "burp", "w3af",
		"dirb", "dirbuster", "gobuster", "wfuzz", "ffuf",
	}

	userAgentLower := strings.ToLower(userAgent)
	for _, suspicious := range suspiciousAgents {
		if strings.Contains(userAgentLower, suspicious) {
			return true
		}
	}

	// Check for empty or very short user agents
	if len(userAgent) < 10 {
		return true
	}

	// Check for non-printable characters
	for _, r := range userAgent {
		if !unicode.IsPrint(r) {
			return true
		}
	}

	return false
}

// extractAllInputs extracts all input values from request
func (sh *SecurityHardening) extractAllInputs(c *gin.Context) map[string]string {
	inputs := make(map[string]string)

	// Query parameters
	for key, values := range c.Request.URL.Query() {
		for i, value := range values {
			inputs[fmt.Sprintf("query.%s[%d]", key, i)] = value
		}
	}

	// Headers
	for key, values := range c.Request.Header {
		for i, value := range values {
			inputs[fmt.Sprintf("header.%s[%d]", key, i)] = value
		}
	}

	// Path parameters
	for _, param := range c.Params {
		inputs[fmt.Sprintf("path.%s", param.Key)] = param.Value
	}

	// Form data (if applicable)
	if c.Request.Method == "POST" && c.GetHeader("Content-Type") == "application/x-www-form-urlencoded" {
		if err := c.Request.ParseForm(); err == nil {
			for key, values := range c.Request.PostForm {
				for i, value := range values {
					inputs[fmt.Sprintf("form.%s[%d]", key, i)] = value
				}
			}
		}
	}

	return inputs
}

// detectAdvancedThreats checks for advanced threat patterns
func (sh *SecurityHardening) detectAdvancedThreats(input string) []string {
	var violations []string

	// URL decode the input for better detection
	decoded, _ := url.QueryUnescape(input)

	// Check both original and decoded input
	inputs := []string{input, decoded}

	for _, testInput := range inputs {
		for _, pattern := range sh.suspiciousPatterns {
			if pattern.MatchString(testInput) {
				violations = append(violations, pattern.String())
			}
		}
	}

	// Additional checks for encoding evasion
	if sh.hasEncodingEvasion(input) {
		violations = append(violations, "encoding_evasion")
	}

	// Check for binary data in text fields
	if sh.hasBinaryData(input) {
		violations = append(violations, "binary_data")
	}

	// Check for excessive special characters
	if sh.hasExcessiveSpecialChars(input) {
		violations = append(violations, "excessive_special_chars")
	}

	return violations
}

// hasEncodingEvasion detects encoding evasion attempts
func (sh *SecurityHardening) hasEncodingEvasion(input string) bool {
	// Check for multiple URL encoding layers
	decoded1, _ := url.QueryUnescape(input)
	decoded2, _ := url.QueryUnescape(decoded1)

	// If double decoding produces different results, it's suspicious
	if decoded1 != decoded2 && decoded1 != input {
		return true
	}

	// Check for mixed encoding schemes
	patterns := []string{
		`%[0-9a-fA-F]{2}.*\\x[0-9a-fA-F]{2}`, // Mixed URL and hex encoding
		`%[0-9a-fA-F]{2}.*\\u[0-9a-fA-F]{4}`, // Mixed URL and Unicode encoding
		`\\x[0-9a-fA-F]{2}.*%[0-9a-fA-F]{2}`, // Mixed hex and URL encoding
	}

	for _, pattern := range patterns {
		if matched, _ := regexp.MatchString(pattern, input); matched {
			return true
		}
	}

	return false
}

// hasBinaryData checks for binary data in text inputs
func (sh *SecurityHardening) hasBinaryData(input string) bool {
	if len(input) == 0 {
		return false
	}

	nonPrintableCount := 0
	for _, r := range input {
		if !unicode.IsPrint(r) && !unicode.IsSpace(r) {
			nonPrintableCount++
		}
	}

	// If more than 10% of characters are non-printable, it's suspicious
	return float64(nonPrintableCount)/float64(len(input)) > 0.1
}

// hasExcessiveSpecialChars checks for excessive special characters
func (sh *SecurityHardening) hasExcessiveSpecialChars(input string) bool {
	if len(input) < 20 {
		return false
	}

	specialCharCount := 0
	for _, r := range input {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && !unicode.IsSpace(r) {
			specialCharCount++
		}
	}

	// If more than 30% are special characters, it's suspicious
	return float64(specialCharCount)/float64(len(input)) > 0.3
}

// generateRequestFingerprint creates a unique fingerprint for the request
func (sh *SecurityHardening) generateRequestFingerprint(c *gin.Context) string {
	fingerprint := fmt.Sprintf("%s:%s:%s:%s",
		c.ClientIP(),
		c.Request.Method,
		c.Request.URL.Path,
		c.GetHeader("User-Agent"))

	return fmt.Sprintf("%x", md5.Sum([]byte(fingerprint)))
}

// isRateLimited checks if the request should be rate limited
func (sh *SecurityHardening) isRateLimited(fingerprint, clientIP, endpoint string) bool {
	now := time.Now()

	// Clean up old entries
	if len(sh.requestFingerprints)%100 == 0 {
		sh.cleanupOldFingerprints(now)
	}

	// Check fingerprint-based rate limiting
	if lastSeen, exists := sh.requestFingerprints[fingerprint]; exists {
		if now.Sub(lastSeen) < time.Second {
			return true
		}
	}
	sh.requestFingerprints[fingerprint] = now

	// Check IP-based rate limiting
	ipKey := fmt.Sprintf("ip:%s", clientIP)
	sh.maxRequestsPerIP[ipKey]++
	if sh.maxRequestsPerIP[ipKey] > 300 { // 300 requests per cleanup interval
		return true
	}

	// Check endpoint-based rate limiting
	endpointKey := fmt.Sprintf("endpoint:%s", endpoint)
	sh.maxRequestsPerEndpoint[endpointKey]++
	if sh.maxRequestsPerEndpoint[endpointKey] > 100 { // 100 requests per endpoint per cleanup interval
		return true
	}

	return false
}

// cleanupOldFingerprints removes old fingerprint entries
func (sh *SecurityHardening) cleanupOldFingerprints(now time.Time) {
	cutoff := now.Add(-sh.cleanupInterval)

	for fingerprint, timestamp := range sh.requestFingerprints {
		if timestamp.Before(cutoff) {
			delete(sh.requestFingerprints, fingerprint)
		}
	}

	// Reset counters
	sh.maxRequestsPerIP = make(map[string]int)
	sh.maxRequestsPerEndpoint = make(map[string]int)
}

// isCSRFVulnerable checks for CSRF vulnerabilities
func (sh *SecurityHardening) isCSRFVulnerable(c *gin.Context) bool {
	// Only check state-changing methods
	if c.Request.Method != "POST" && c.Request.Method != "PUT" &&
		c.Request.Method != "DELETE" && c.Request.Method != "PATCH" {
		return false
	}

	// Check for missing or invalid CSRF token
	csrfToken := c.GetHeader("X-CSRF-Token")
	if csrfToken == "" {
		csrfToken = c.Query("csrf_token")
	}

	// For this example, we'll check if token exists and has minimum length
	// In production, implement proper CSRF token validation
	if len(csrfToken) < 32 {
		return true
	}

	return false
}

// isValidReferer validates the referer header
func (sh *SecurityHardening) isValidReferer(referer, host string) bool {
	refererURL, err := url.Parse(referer)
	if err != nil {
		return false
	}

	// Check if referer hostname matches request hostname
	return refererURL.Hostname() == host
}

// hasInvalidTransferEncoding checks for invalid transfer encoding
func (sh *SecurityHardening) hasInvalidTransferEncoding(c *gin.Context) bool {
	transferEncoding := c.GetHeader("Transfer-Encoding")
	contentLength := c.GetHeader("Content-Length")

	// Both Transfer-Encoding and Content-Length should not be present
	if transferEncoding != "" && contentLength != "" {
		return true
	}

	// Validate Transfer-Encoding values
	if transferEncoding != "" {
		validEncodings := []string{"chunked", "compress", "deflate", "gzip", "identity"}
		found := false
		for _, valid := range validEncodings {
			if strings.Contains(strings.ToLower(transferEncoding), valid) {
				found = true
				break
			}
		}
		if !found {
			return true
		}
	}

	// Validate Content-Length
	if contentLength != "" {
		if _, err := strconv.ParseInt(contentLength, 10, 64); err != nil {
			return true
		}
	}

	return false
}

// SecurityHardeningMiddleware creates middleware for advanced security protection
func SecurityHardeningMiddleware(logger *zap.Logger, config *SecurityConfig) gin.HandlerFunc {
	hardening := NewSecurityHardening(logger, config)

	return func(c *gin.Context) {
		// Apply security validation
		if errors := hardening.ValidateAdvancedSecurity(c); len(errors) > 0 {
			logger.Warn("Advanced security validation failed",
				zap.String("client_ip", c.ClientIP()),
				zap.String("method", c.Request.Method),
				zap.String("path", c.Request.URL.Path),
				zap.String("user_agent", c.GetHeader("User-Agent")),
				zap.Any("security_errors", errors))

			c.JSON(429, gin.H{
				"error":     "Security validation failed",
				"message":   "Request blocked due to security policy violation",
				"timestamp": time.Now().UTC().Format(time.RFC3339),
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
