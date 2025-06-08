// Package middleware provides a unified middleware infrastructure for Orbit CEX
// This consolidates all middleware functionality including rate limiting, authentication,
// RBAC, security headers, CORS, validation, metrics, logging, and tracing.
package middleware

import (
	"time"

	"github.com/Aidin1998/finalex/pkg/models"
)

// UnifiedMiddlewareConfig contains configuration for all middleware components
type UnifiedMiddlewareConfig struct {
	// Global settings
	Enabled         bool   `json:"enabled" yaml:"enabled" default:"true"`
	ServiceName     string `json:"service_name" yaml:"service_name"`
	Environment     string `json:"environment" yaml:"environment"`
	RequestIDHeader string `json:"request_id_header" yaml:"request_id_header" default:"X-Request-ID"`

	// Rate Limiting Configuration
	RateLimit *RateLimitConfig `json:"rate_limit" yaml:"rate_limit"`

	// Authentication Configuration
	Authentication *AuthenticationConfig `json:"authentication" yaml:"authentication"`

	// RBAC Configuration
	RBAC *RBACConfig `json:"rbac" yaml:"rbac"`

	// CORS Configuration
	CORS *CORSConfig `json:"cors" yaml:"cors"`

	// Security Headers Configuration
	Security *SecurityConfig `json:"security" yaml:"security"`

	// Validation Configuration
	Validation *ValidationConfig `json:"validation" yaml:"validation"`

	// Logging Configuration
	Logging *LoggingConfig `json:"logging" yaml:"logging"`

	// Metrics Configuration
	Metrics *MetricsConfig `json:"metrics" yaml:"metrics"`

	// Tracing Configuration
	Tracing *TracingConfig `json:"tracing" yaml:"tracing"`

	// Recovery Configuration
	Recovery *RecoveryConfig `json:"recovery" yaml:"recovery"`
}

// RateLimitConfig configures rate limiting behavior
type RateLimitConfig struct {
	Enabled               bool                                    `json:"enabled" yaml:"enabled" default:"true"`
	DefaultAlgorithm      string                                  `json:"default_algorithm" yaml:"default_algorithm" default:"sliding_window"`
	RedisAddr             string                                  `json:"redis_addr" yaml:"redis_addr"`
	RedisPassword         string                                  `json:"redis_password" yaml:"redis_password"`
	RedisDB               int                                     `json:"redis_db" yaml:"redis_db"`
	RedisKeyPrefix        string                                  `json:"redis_key_prefix" yaml:"redis_key_prefix" default:"orbit_ratelimit"`
	FailureMode           string                                  `json:"failure_mode" yaml:"failure_mode" default:"allow"`
	CleanupInterval       time.Duration                           `json:"cleanup_interval" yaml:"cleanup_interval" default:"1h"`
	MaxDataAge            time.Duration                           `json:"max_data_age" yaml:"max_data_age" default:"24h"`
	EnableMetrics         bool                                    `json:"enable_metrics" yaml:"enable_metrics" default:"true"`
	EnableDistributedLock bool                                    `json:"enable_distributed_lock" yaml:"enable_distributed_lock" default:"true"`
	LockTimeout           time.Duration                           `json:"lock_timeout" yaml:"lock_timeout" default:"5s"`
	TierConfigs           map[models.UserTier]TierRateLimitConfig `json:"tier_configs" yaml:"tier_configs"`
	EndpointConfigs       map[string]EndpointRateLimitConfig      `json:"endpoint_configs" yaml:"endpoint_configs"`
	IPConfigs             map[string]IPRateLimitConfig            `json:"ip_configs" yaml:"ip_configs"`
	GlobalLimits          *GlobalRateLimitConfig                  `json:"global_limits" yaml:"global_limits"`
}

// TierRateLimitConfig defines rate limits for different user tiers
type TierRateLimitConfig struct {
	APICallsPerMinute    int `json:"api_calls_per_minute" yaml:"api_calls_per_minute"`
	APICallsPerHour      int `json:"api_calls_per_hour" yaml:"api_calls_per_hour"`
	OrdersPerMinute      int `json:"orders_per_minute" yaml:"orders_per_minute"`
	OrdersPerHour        int `json:"orders_per_hour" yaml:"orders_per_hour"`
	TradesPerMinute      int `json:"trades_per_minute" yaml:"trades_per_minute"`
	WithdrawalsPerDay    int `json:"withdrawals_per_day" yaml:"withdrawals_per_day"`
	LoginAttemptsPerHour int `json:"login_attempts_per_hour" yaml:"login_attempts_per_hour"`
	BurstMultiplier      int `json:"burst_multiplier" yaml:"burst_multiplier" default:"2"`
}

// EndpointRateLimitConfig defines rate limits for specific endpoints
type EndpointRateLimitConfig struct {
	Name              string             `json:"name" yaml:"name"`
	Path              string             `json:"path" yaml:"path"`
	Method            string             `json:"method" yaml:"method"`
	Algorithm         string             `json:"algorithm" yaml:"algorithm" default:"sliding_window"`
	RequestsPerMinute int                `json:"requests_per_minute" yaml:"requests_per_minute"`
	RequestsPerHour   int                `json:"requests_per_hour" yaml:"requests_per_hour"`
	BurstLimit        int                `json:"burst_limit" yaml:"burst_limit"`
	RequiresAuth      bool               `json:"requires_auth" yaml:"requires_auth"`
	TierOverrides     map[string]int     `json:"tier_overrides" yaml:"tier_overrides"`
	IPLimits          *IPRateLimitConfig `json:"ip_limits" yaml:"ip_limits"`
	Enabled           bool               `json:"enabled" yaml:"enabled" default:"true"`
}

// IPRateLimitConfig defines IP-based rate limits
type IPRateLimitConfig struct {
	RequestsPerMinute int           `json:"requests_per_minute" yaml:"requests_per_minute"`
	RequestsPerHour   int           `json:"requests_per_hour" yaml:"requests_per_hour"`
	BurstLimit        int           `json:"burst_limit" yaml:"burst_limit"`
	Window            time.Duration `json:"window" yaml:"window" default:"1m"`
	Whitelist         []string      `json:"whitelist" yaml:"whitelist"`
	Blacklist         []string      `json:"blacklist" yaml:"blacklist"`
}

// GlobalRateLimitConfig defines global system-wide rate limits
type GlobalRateLimitConfig struct {
	RequestsPerSecond  int `json:"requests_per_second" yaml:"requests_per_second"`
	OrdersPerSecond    int `json:"orders_per_second" yaml:"orders_per_second"`
	MaxConcurrentConns int `json:"max_concurrent_connections" yaml:"max_concurrent_connections"`
}

// AuthenticationConfig configures authentication behavior
type AuthenticationConfig struct {
	Enabled               bool          `json:"enabled" yaml:"enabled" default:"true"`
	JWTSecret             string        `json:"jwt_secret" yaml:"jwt_secret"`
	JWTIssuer             string        `json:"jwt_issuer" yaml:"jwt_issuer"`
	JWTAudience           []string      `json:"jwt_audience" yaml:"jwt_audience"`
	TokenExpiry           time.Duration `json:"token_expiry" yaml:"token_expiry" default:"24h"`
	RefreshTokenExpiry    time.Duration `json:"refresh_token_expiry" yaml:"refresh_token_expiry" default:"168h"`
	RequireHTTPS          bool          `json:"require_https" yaml:"require_https" default:"true"`
	CookieSecure          bool          `json:"cookie_secure" yaml:"cookie_secure" default:"true"`
	CookieHTTPOnly        bool          `json:"cookie_http_only" yaml:"cookie_http_only" default:"true"`
	CookieSameSite        string        `json:"cookie_same_site" yaml:"cookie_same_site" default:"strict"`
	SessionTimeout        time.Duration `json:"session_timeout" yaml:"session_timeout" default:"30m"`
	AllowMultipleSessions bool          `json:"allow_multiple_sessions" yaml:"allow_multiple_sessions" default:"false"`
	MFARequired           bool          `json:"mfa_required" yaml:"mfa_required" default:"false"`
	APIKeyEnabled         bool          `json:"api_key_enabled" yaml:"api_key_enabled" default:"true"`
	SkipPathsRegex        []string      `json:"skip_paths_regex" yaml:"skip_paths_regex"`
	RedisSessionStore     bool          `json:"redis_session_store" yaml:"redis_session_store" default:"true"`
}

// RBACConfig configures role-based access control
type RBACConfig struct {
	Enabled              bool          `json:"enabled" yaml:"enabled" default:"true"`
	DefaultRole          string        `json:"default_role" yaml:"default_role" default:"user"`
	SuperAdminRole       string        `json:"super_admin_role" yaml:"super_admin_role" default:"super_admin"`
	CachePermissions     bool          `json:"cache_permissions" yaml:"cache_permissions" default:"true"`
	CacheTTL             time.Duration `json:"cache_ttl" yaml:"cache_ttl" default:"5m"`
	PermissionDeniedCode int           `json:"permission_denied_code" yaml:"permission_denied_code" default:"403"`
	LogAccessAttempts    bool          `json:"log_access_attempts" yaml:"log_access_attempts" default:"true"`
	LogDeniedRequests    bool          `json:"log_denied_requests" yaml:"log_denied_requests" default:"true"`
}

// CORSConfig configures Cross-Origin Resource Sharing
type CORSConfig struct {
	Enabled          bool          `json:"enabled" yaml:"enabled" default:"true"`
	AllowOrigins     []string      `json:"allow_origins" yaml:"allow_origins"`
	AllowMethods     []string      `json:"allow_methods" yaml:"allow_methods"`
	AllowHeaders     []string      `json:"allow_headers" yaml:"allow_headers"`
	ExposeHeaders    []string      `json:"expose_headers" yaml:"expose_headers"`
	AllowCredentials bool          `json:"allow_credentials" yaml:"allow_credentials" default:"true"`
	MaxAge           time.Duration `json:"max_age" yaml:"max_age" default:"12h"`
	OptionsPassthru  bool          `json:"options_passthru" yaml:"options_passthru" default:"false"`
}

// SecurityConfig configures security headers and policies
type SecurityConfig struct {
	Enabled               bool     `json:"enabled" yaml:"enabled" default:"true"`
	ContentTypeNoSniff    bool     `json:"content_type_no_sniff" yaml:"content_type_no_sniff" default:"true"`
	FrameOptions          string   `json:"frame_options" yaml:"frame_options" default:"DENY"`
	XSSProtection         string   `json:"xss_protection" yaml:"xss_protection" default:"1; mode=block"`
	ContentSecurityPolicy string   `json:"content_security_policy" yaml:"content_security_policy"`
	ReferrerPolicy        string   `json:"referrer_policy" yaml:"referrer_policy" default:"strict-origin-when-cross-origin"`
	HSTSEnabled           bool     `json:"hsts_enabled" yaml:"hsts_enabled" default:"true"`
	HSTSMaxAge            int      `json:"hsts_max_age" yaml:"hsts_max_age" default:"31536000"`
	HSTSIncludeSubdomains bool     `json:"hsts_include_subdomains" yaml:"hsts_include_subdomains" default:"true"`
	HSTSPreload           bool     `json:"hsts_preload" yaml:"hsts_preload" default:"false"`
	ExpectCTEnabled       bool     `json:"expect_ct_enabled" yaml:"expect_ct_enabled" default:"false"`
	ExpectCTMaxAge        int      `json:"expect_ct_max_age" yaml:"expect_ct_max_age" default:"86400"`
	ExpectCTEnforce       bool     `json:"expect_ct_enforce" yaml:"expect_ct_enforce" default:"false"`
	PermittedCrossDomain  string   `json:"permitted_cross_domain" yaml:"permitted_cross_domain" default:"none"`
	HidePoweredBy         bool     `json:"hide_powered_by" yaml:"hide_powered_by" default:"true"`
	IPRestrictions        []string `json:"ip_restrictions" yaml:"ip_restrictions"`
	GeoRestrictions       []string `json:"geo_restrictions" yaml:"geo_restrictions"`
	DDoSProtectionEnabled bool     `json:"ddos_protection_enabled" yaml:"ddos_protection_enabled" default:"true"`
	BruteForceProtection  bool     `json:"brute_force_protection" yaml:"brute_force_protection" default:"true"`
}

// ValidationConfig configures input validation
type ValidationConfig struct {
	Enabled                bool     `json:"enabled" yaml:"enabled" default:"true"`
	MaxRequestSize         int64    `json:"max_request_size" yaml:"max_request_size" default:"10485760"` // 10MB
	MaxHeaderSize          int      `json:"max_header_size" yaml:"max_header_size" default:"8192"`       // 8KB
	MaxQueryParams         int      `json:"max_query_params" yaml:"max_query_params" default:"100"`
	MaxFormFields          int      `json:"max_form_fields" yaml:"max_form_fields" default:"100"`
	RequiredHeaders        []string `json:"required_headers" yaml:"required_headers"`
	ForbiddenHeaders       []string `json:"forbidden_headers" yaml:"forbidden_headers"`
	AllowedContentTypes    []string `json:"allowed_content_types" yaml:"allowed_content_types"`
	ValidateJSON           bool     `json:"validate_json" yaml:"validate_json" default:"true"`
	ValidateXML            bool     `json:"validate_xml" yaml:"validate_xml" default:"false"`
	SQLInjectionProtection bool     `json:"sql_injection_protection" yaml:"sql_injection_protection" default:"true"`
	XSSProtection          bool     `json:"xss_protection" yaml:"xss_protection" default:"true"`
	CSRFProtection         bool     `json:"csrf_protection" yaml:"csrf_protection" default:"true"`
	SanitizeInput          bool     `json:"sanitize_input" yaml:"sanitize_input" default:"true"`
	LogValidationErrors    bool     `json:"log_validation_errors" yaml:"log_validation_errors" default:"true"`
}

// LoggingConfig configures request/response logging
type LoggingConfig struct {
	Enabled              bool          `json:"enabled" yaml:"enabled" default:"true"`
	Level                string        `json:"level" yaml:"level" default:"info"`
	RequestBody          bool          `json:"request_body" yaml:"request_body" default:"false"`
	ResponseBody         bool          `json:"response_body" yaml:"response_body" default:"false"`
	Headers              bool          `json:"headers" yaml:"headers" default:"true"`
	QueryParams          bool          `json:"query_params" yaml:"query_params" default:"true"`
	UserAgent            bool          `json:"user_agent" yaml:"user_agent" default:"true"`
	ClientIP             bool          `json:"client_ip" yaml:"client_ip" default:"true"`
	RequestID            bool          `json:"request_id" yaml:"request_id" default:"true"`
	Duration             bool          `json:"duration" yaml:"duration" default:"true"`
	StatusCode           bool          `json:"status_code" yaml:"status_code" default:"true"`
	ErrorDetails         bool          `json:"error_details" yaml:"error_details" default:"true"`
	SlowRequestThreshold time.Duration `json:"slow_request_threshold" yaml:"slow_request_threshold" default:"1s"`
	SkipHealthCheck      bool          `json:"skip_health_check" yaml:"skip_health_check" default:"true"`
	SkipPaths            []string      `json:"skip_paths" yaml:"skip_paths"`
	SensitiveFields      []string      `json:"sensitive_fields" yaml:"sensitive_fields"`
	MaxBodyLogSize       int           `json:"max_body_log_size" yaml:"max_body_log_size" default:"1024"`
	CompressLogs         bool          `json:"compress_logs" yaml:"compress_logs" default:"false"`
	StructuredLogging    bool          `json:"structured_logging" yaml:"structured_logging" default:"true"`
	AuditLogging         bool          `json:"audit_logging" yaml:"audit_logging" default:"true"`
}

// MetricsConfig configures Prometheus metrics collection
type MetricsConfig struct {
	Enabled               bool          `json:"enabled" yaml:"enabled" default:"true"`
	Path                  string        `json:"path" yaml:"path" default:"/metrics"`
	Namespace             string        `json:"namespace" yaml:"namespace" default:"orbit"`
	Subsystem             string        `json:"subsystem" yaml:"subsystem" default:"middleware"`
	RequestDuration       bool          `json:"request_duration" yaml:"request_duration" default:"true"`
	RequestCount          bool          `json:"request_count" yaml:"request_count" default:"true"`
	RequestSize           bool          `json:"request_size" yaml:"request_size" default:"true"`
	ResponseSize          bool          `json:"response_size" yaml:"response_size" default:"true"`
	ActiveConnections     bool          `json:"active_connections" yaml:"active_connections" default:"true"`
	RateLimitMetrics      bool          `json:"rate_limit_metrics" yaml:"rate_limit_metrics" default:"true"`
	AuthenticationMetrics bool          `json:"authentication_metrics" yaml:"authentication_metrics" default:"true"`
	SecurityMetrics       bool          `json:"security_metrics" yaml:"security_metrics" default:"true"`
	ValidationMetrics     bool          `json:"validation_metrics" yaml:"validation_metrics" default:"true"`
	HistogramBuckets      []float64     `json:"histogram_buckets" yaml:"histogram_buckets"`
	CollectionInterval    time.Duration `json:"collection_interval" yaml:"collection_interval" default:"15s"`
	MaxMetricsAge         time.Duration `json:"max_metrics_age" yaml:"max_metrics_age" default:"24h"`
	PushGatewayEnabled    bool          `json:"push_gateway_enabled" yaml:"push_gateway_enabled" default:"false"`
	PushGatewayURL        string        `json:"push_gateway_url" yaml:"push_gateway_url"`
	PushInterval          time.Duration `json:"push_interval" yaml:"push_interval" default:"15s"`
}

// TracingConfig configures OpenTelemetry tracing
type TracingConfig struct {
	Enabled                 bool              `json:"enabled" yaml:"enabled" default:"true"`
	ServiceName             string            `json:"service_name" yaml:"service_name"`
	ServiceVersion          string            `json:"service_version" yaml:"service_version"`
	Environment             string            `json:"environment" yaml:"environment"`
	Endpoint                string            `json:"endpoint" yaml:"endpoint"`
	Headers                 map[string]string `json:"headers" yaml:"headers"`
	Insecure                bool              `json:"insecure" yaml:"insecure" default:"false"`
	SamplingRate            float64           `json:"sampling_rate" yaml:"sampling_rate" default:"1.0"`
	MaxSpansPerTrace        int               `json:"max_spans_per_trace" yaml:"max_spans_per_trace" default:"1000"`
	SpanAttributeCountLimit int               `json:"span_attribute_count_limit" yaml:"span_attribute_count_limit" default:"128"`
	SpanEventCountLimit     int               `json:"span_event_count_limit" yaml:"span_event_count_limit" default:"128"`
	SpanLinkCountLimit      int               `json:"span_link_count_limit" yaml:"span_link_count_limit" default:"128"`
	BatchTimeout            time.Duration     `json:"batch_timeout" yaml:"batch_timeout" default:"5s"`
	ExportTimeout           time.Duration     `json:"export_timeout" yaml:"export_timeout" default:"30s"`
	MaxExportBatchSize      int               `json:"max_export_batch_size" yaml:"max_export_batch_size" default:"512"`
	MaxQueueSize            int               `json:"max_queue_size" yaml:"max_queue_size" default:"2048"`
	TraceRequests           bool              `json:"trace_requests" yaml:"trace_requests" default:"true"`
	TraceResponses          bool              `json:"trace_responses" yaml:"trace_responses" default:"true"`
	TraceHeaders            bool              `json:"trace_headers" yaml:"trace_headers" default:"true"`
	TraceErrors             bool              `json:"trace_errors" yaml:"trace_errors" default:"true"`
	TraceDatabaseCalls      bool              `json:"trace_database_calls" yaml:"trace_database_calls" default:"true"`
	TraceRedisOps           bool              `json:"trace_redis_ops" yaml:"trace_redis_ops" default:"true"`
	TraceExternalAPIs       bool              `json:"trace_external_apis" yaml:"trace_external_apis" default:"true"`
	PropagateTraceContext   bool              `json:"propagate_trace_context" yaml:"propagate_trace_context" default:"true"`
}

// RecoveryConfig configures panic recovery
type RecoveryConfig struct {
	Enabled            bool     `json:"enabled" yaml:"enabled" default:"true"`
	LogStackTrace      bool     `json:"log_stack_trace" yaml:"log_stack_trace" default:"true"`
	PrintStack         bool     `json:"print_stack" yaml:"print_stack" default:"false"`
	IncludeHeaders     bool     `json:"include_headers" yaml:"include_headers" default:"true"`
	IncludeQueryParams bool     `json:"include_query_params" yaml:"include_query_params" default:"true"`
	IncludeRequestBody bool     `json:"include_request_body" yaml:"include_request_body" default:"false"`
	DisableStackAll    bool     `json:"disable_stack_all" yaml:"disable_stack_all" default:"false"`
	StackSize          int      `json:"stack_size" yaml:"stack_size" default:"4096"`
	RecoveryHandler    string   `json:"recovery_handler" yaml:"recovery_handler" default:"default"`
	NotifyChannels     []string `json:"notify_channels" yaml:"notify_channels"`
	CriticalEndpoints  []string `json:"critical_endpoints" yaml:"critical_endpoints"`
}

// GetDefaultConfig returns a default configuration with sensible defaults
func GetDefaultConfig() *UnifiedMiddlewareConfig {
	return &UnifiedMiddlewareConfig{
		Enabled:         true,
		ServiceName:     "orbit-cex",
		Environment:     "production",
		RequestIDHeader: "X-Request-ID",

		RateLimit: &RateLimitConfig{
			Enabled:               true,
			DefaultAlgorithm:      "sliding_window",
			RedisKeyPrefix:        "orbit_ratelimit",
			FailureMode:           "allow",
			CleanupInterval:       time.Hour,
			MaxDataAge:            24 * time.Hour,
			EnableMetrics:         true,
			EnableDistributedLock: true,
			LockTimeout:           5 * time.Second,
			TierConfigs: map[models.UserTier]TierRateLimitConfig{
				models.TierBasic: {
					APICallsPerMinute:    60,
					APICallsPerHour:      1000,
					OrdersPerMinute:      10,
					OrdersPerHour:        100,
					TradesPerMinute:      5,
					WithdrawalsPerDay:    3,
					LoginAttemptsPerHour: 5,
					BurstMultiplier:      2,
				},
				models.TierPremium: {
					APICallsPerMinute:    300,
					APICallsPerHour:      10000,
					OrdersPerMinute:      50,
					OrdersPerHour:        1000,
					TradesPerMinute:      25,
					WithdrawalsPerDay:    10,
					LoginAttemptsPerHour: 10,
					BurstMultiplier:      3,
				},
				models.TierVIP: {
					APICallsPerMinute:    1000,
					APICallsPerHour:      50000,
					OrdersPerMinute:      200,
					OrdersPerHour:        5000,
					TradesPerMinute:      100,
					WithdrawalsPerDay:    50,
					LoginAttemptsPerHour: 20,
					BurstMultiplier:      5,
				},
			},
			GlobalLimits: &GlobalRateLimitConfig{
				RequestsPerSecond:  10000,
				OrdersPerSecond:    1000,
				MaxConcurrentConns: 50000,
			},
		},

		Authentication: &AuthenticationConfig{
			Enabled:               true,
			TokenExpiry:           24 * time.Hour,
			RefreshTokenExpiry:    7 * 24 * time.Hour,
			RequireHTTPS:          true,
			CookieSecure:          true,
			CookieHTTPOnly:        true,
			CookieSameSite:        "strict",
			SessionTimeout:        30 * time.Minute,
			AllowMultipleSessions: false,
			MFARequired:           false,
			APIKeyEnabled:         true,
			RedisSessionStore:     true,
		},

		RBAC: &RBACConfig{
			Enabled:              true,
			DefaultRole:          "user",
			SuperAdminRole:       "super_admin",
			CachePermissions:     true,
			CacheTTL:             5 * time.Minute,
			PermissionDeniedCode: 403,
			LogAccessAttempts:    true,
			LogDeniedRequests:    true,
		},

		CORS: &CORSConfig{
			Enabled: true,
			AllowOrigins: []string{
				"https://app.orbitcex.com",
				"https://admin.orbitcex.com",
			},
			AllowMethods: []string{
				"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH",
			},
			AllowHeaders: []string{
				"Origin", "Content-Type", "Accept", "Authorization",
				"X-Requested-With", "X-Request-ID", "X-API-Key",
			},
			ExposeHeaders: []string{
				"X-Request-ID", "X-RateLimit-Limit", "X-RateLimit-Remaining",
				"X-RateLimit-Reset", "X-Total-Count",
			},
			AllowCredentials: true,
			MaxAge:           12 * time.Hour,
			OptionsPassthru:  false,
		},

		Security: &SecurityConfig{
			Enabled:               true,
			ContentTypeNoSniff:    true,
			FrameOptions:          "DENY",
			XSSProtection:         "1; mode=block",
			ContentSecurityPolicy: "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'",
			ReferrerPolicy:        "strict-origin-when-cross-origin",
			HSTSEnabled:           true,
			HSTSMaxAge:            31536000,
			HSTSIncludeSubdomains: true,
			HSTSPreload:           false,
			ExpectCTEnabled:       false,
			ExpectCTMaxAge:        86400,
			ExpectCTEnforce:       false,
			PermittedCrossDomain:  "none",
			HidePoweredBy:         true,
			DDoSProtectionEnabled: true,
			BruteForceProtection:  true,
		},

		Validation: &ValidationConfig{
			Enabled:                true,
			MaxRequestSize:         10 * 1024 * 1024, // 10MB
			MaxHeaderSize:          8 * 1024,         // 8KB
			MaxQueryParams:         100,
			MaxFormFields:          100,
			ValidateJSON:           true,
			ValidateXML:            false,
			SQLInjectionProtection: true,
			XSSProtection:          true,
			CSRFProtection:         true,
			SanitizeInput:          true,
			LogValidationErrors:    true,
		},

		Logging: &LoggingConfig{
			Enabled:              true,
			Level:                "info",
			RequestBody:          false,
			ResponseBody:         false,
			Headers:              true,
			QueryParams:          true,
			UserAgent:            true,
			ClientIP:             true,
			RequestID:            true,
			Duration:             true,
			StatusCode:           true,
			ErrorDetails:         true,
			SlowRequestThreshold: 1 * time.Second,
			SkipHealthCheck:      true,
			MaxBodyLogSize:       1024,
			CompressLogs:         false,
			StructuredLogging:    true,
			AuditLogging:         true,
		},

		Metrics: &MetricsConfig{
			Enabled:               true,
			Path:                  "/metrics",
			Namespace:             "orbit",
			Subsystem:             "middleware",
			RequestDuration:       true,
			RequestCount:          true,
			RequestSize:           true,
			ResponseSize:          true,
			ActiveConnections:     true,
			RateLimitMetrics:      true,
			AuthenticationMetrics: true,
			SecurityMetrics:       true,
			ValidationMetrics:     true,
			HistogramBuckets: []float64{
				0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10,
			},
			CollectionInterval: 15 * time.Second,
			MaxMetricsAge:      24 * time.Hour,
			PushGatewayEnabled: false,
			PushInterval:       15 * time.Second,
		},

		Tracing: &TracingConfig{
			Enabled:                 true,
			SamplingRate:            1.0,
			MaxSpansPerTrace:        1000,
			SpanAttributeCountLimit: 128,
			SpanEventCountLimit:     128,
			SpanLinkCountLimit:      128,
			BatchTimeout:            5 * time.Second,
			ExportTimeout:           30 * time.Second,
			MaxExportBatchSize:      512,
			MaxQueueSize:            2048,
			TraceRequests:           true,
			TraceResponses:          true,
			TraceHeaders:            true,
			TraceErrors:             true,
			TraceDatabaseCalls:      true,
			TraceRedisOps:           true,
			TraceExternalAPIs:       true,
			PropagateTraceContext:   true,
		},

		Recovery: &RecoveryConfig{
			Enabled:            true,
			LogStackTrace:      true,
			PrintStack:         false,
			IncludeHeaders:     true,
			IncludeQueryParams: true,
			IncludeRequestBody: false,
			DisableStackAll:    false,
			StackSize:          4096,
			RecoveryHandler:    "default",
		},
	}
}
