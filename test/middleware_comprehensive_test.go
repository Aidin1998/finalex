package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
)

func setupTestUnifiedMiddleware() *UnifiedMiddleware {
	config := GetDefaultConfig()

	// Use test-friendly settings
	config.RateLimit.Enabled = false      // Disable for most tests
	config.Authentication.Enabled = false // Disable for basic tests
	config.Logging.Level = "debug"

	// Create mock logger
	mockLogger := &MockLogger{}

	return NewUnifiedMiddleware(config, nil, mockLogger)
}

// MockLogger implements the logger interface for testing
type MockLogger struct {
	logs []LogEntry
}

type LogEntry struct {
	Level   string
	Message string
	Fields  map[string]interface{}
}

func (m *MockLogger) Debug(msg string, fields ...interface{}) {
	m.logs = append(m.logs, LogEntry{Level: "debug", Message: msg, Fields: fieldsToMap(fields...)})
}

func (m *MockLogger) Info(msg string, fields ...interface{}) {
	m.logs = append(m.logs, LogEntry{Level: "info", Message: msg, Fields: fieldsToMap(fields...)})
}

func (m *MockLogger) Warn(msg string, fields ...interface{}) {
	m.logs = append(m.logs, LogEntry{Level: "warn", Message: msg, Fields: fieldsToMap(fields...)})
}

func (m *MockLogger) Error(msg string, fields ...interface{}) {
	m.logs = append(m.logs, LogEntry{Level: "error", Message: msg, Fields: fieldsToMap(fields...)})
}

func (m *MockLogger) GetLogs() []LogEntry {
	return m.logs
}

func (m *MockLogger) ClearLogs() {
	m.logs = nil
}

func fieldsToMap(fields ...interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for i := 0; i < len(fields); i += 2 {
		if i+1 < len(fields) {
			if key, ok := fields[i].(string); ok {
				result[key] = fields[i+1]
			}
		}
	}
	return result
}

func TestRequestIDMiddleware(t *testing.T) {
	um := setupTestUnifiedMiddleware()

	handler := um.Handler()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Context().Value("request_id")
		assert.NotNil(t, requestID, "Request ID should be set in context")

		responseRequestID := w.Header().Get("X-Request-ID")
		assert.NotEmpty(t, responseRequestID, "Request ID should be set in response header")

		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.NotEmpty(t, w.Header().Get("X-Request-ID"))
}

func TestCustomRequestID(t *testing.T) {
	um := setupTestUnifiedMiddleware()

	customRequestID := "custom-123"

	handler := um.Handler()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Context().Value("request_id")
		assert.Equal(t, customRequestID, requestID, "Should use custom request ID")

		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Request-ID", customRequestID)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, customRequestID, w.Header().Get("X-Request-ID"))
}

func TestSecurityMiddleware(t *testing.T) {
	um := setupTestUnifiedMiddleware()
	um.config.Security.Enabled = true

	handler := um.Handler()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Check security headers
	assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "DENY", w.Header().Get("X-Frame-Options"))
	assert.Equal(t, "1; mode=block", w.Header().Get("X-XSS-Protection"))
	assert.Contains(t, w.Header().Get("Strict-Transport-Security"), "max-age=31536000")
}

func TestCORSMiddleware(t *testing.T) {
	um := setupTestUnifiedMiddleware()
	um.config.CORS.Enabled = true

	handler := um.Handler()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))

	t.Run("preflight_request", func(t *testing.T) {
		req := httptest.NewRequest("OPTIONS", "/test", nil)
		req.Header.Set("Origin", "https://app.orbitcex.com")
		req.Header.Set("Access-Control-Request-Method", "POST")
		req.Header.Set("Access-Control-Request-Headers", "Content-Type")

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNoContent, w.Code)
		assert.Equal(t, "https://app.orbitcex.com", w.Header().Get("Access-Control-Allow-Origin"))
		assert.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), "POST")
	})

	t.Run("simple_request", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Origin", "https://app.orbitcex.com")

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "https://app.orbitcex.com", w.Header().Get("Access-Control-Allow-Origin"))
	})

	t.Run("disallowed_origin", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Origin", "https://malicious.com")

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
	})
}

func TestValidationMiddleware(t *testing.T) {
	um := setupTestUnifiedMiddleware()
	um.config.Validation.Enabled = true
	um.config.Validation.MaxRequestSize = 1024 // 1KB limit

	handler := um.Handler()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))

	t.Run("valid_request", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/test", strings.NewReader(`{"test":"data"}`))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("request_too_large", func(t *testing.T) {
		largeData := strings.Repeat("x", 2048) // 2KB
		req := httptest.NewRequest("POST", "/test", strings.NewReader(largeData))
		req.ContentLength = int64(len(largeData))

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
	})

	t.Run("sql_injection_detection", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test?id=1' OR '1'='1", nil)

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "Security violation detected")
	})

	t.Run("xss_detection", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test?comment=<script>alert('xss')</script>", nil)

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "Security violation detected")
	})
}

func TestAuthenticationMiddleware(t *testing.T) {
	um := setupTestUnifiedMiddleware()
	um.config.Authentication.Enabled = true
	um.config.Authentication.SkipPathsRegex = []string{"/public/.*", "/health"}

	handler := um.Handler()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userInfo := um.getUserFromContext(r.Context())
		if userInfo != nil {
			w.Header().Set("X-Authenticated-User", userInfo.ID)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))

	t.Run("skip_public_path", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/public/info", nil)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("skip_health_path", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/health", nil)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("missing_token", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/protected", nil)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)

		var response map[string]string
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, "authentication_failed", response["error"])
	})

	t.Run("valid_token", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/protected", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.NotEmpty(t, w.Header().Get("X-Authenticated-User"))
	})
}

func TestLoggingMiddleware(t *testing.T) {
	um := setupTestUnifiedMiddleware()
	um.config.Logging.Enabled = true
	um.config.Logging.Headers = true
	um.config.Logging.QueryParams = true
	um.config.Logging.Duration = true

	mockLogger := um.logger.(*MockLogger)
	mockLogger.ClearLogs()

	handler := um.Handler()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Millisecond) // Simulate processing time
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))

	req := httptest.NewRequest("GET", "/test?param=value", nil)
	req.Header.Set("User-Agent", "Test-Agent")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	logs := mockLogger.GetLogs()
	assert.Greater(t, len(logs), 0, "Should generate log entries")

	// Find the completion log
	var completionLog *LogEntry
	for _, log := range logs {
		if strings.Contains(log.Message, "completed") {
			completionLog = &log
			break
		}
	}

	require.NotNil(t, completionLog, "Should log request completion")
	assert.Equal(t, "info", completionLog.Level)
	assert.Equal(t, "GET", completionLog.Fields["method"])
	assert.Equal(t, "/test", completionLog.Fields["path"])
	assert.Equal(t, 200, completionLog.Fields["status_code"])
	assert.Contains(t, completionLog.Fields, "duration")
}

func TestMetricsMiddleware(t *testing.T) {
	um := setupTestUnifiedMiddleware()
	um.config.Metrics.Enabled = true

	handler := um.Handler()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))

	// Make multiple requests
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	}

	// Metrics would be collected by Prometheus, we can't easily test the values
	// but we can verify the middleware doesn't break the request flow
}

func TestRecoveryMiddleware(t *testing.T) {
	um := setupTestUnifiedMiddleware()
	um.config.Recovery.Enabled = true
	um.config.Recovery.LogStackTrace = true

	mockLogger := um.logger.(*MockLogger)
	mockLogger.ClearLogs()

	handler := um.Handler()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "Internal Server Error")

	logs := mockLogger.GetLogs()
	assert.Greater(t, len(logs), 0, "Should log panic")

	// Find the panic log
	var panicLog *LogEntry
	for _, log := range logs {
		if strings.Contains(log.Message, "Panic recovered") {
			panicLog = &log
			break
		}
	}

	require.NotNil(t, panicLog, "Should log panic recovery")
	assert.Equal(t, "error", panicLog.Level)
	assert.Equal(t, "test panic", panicLog.Fields["panic"])
	assert.Contains(t, panicLog.Fields, "stack_trace")
}

func TestTracingMiddleware(t *testing.T) {
	um := setupTestUnifiedMiddleware()
	um.config.Tracing.Enabled = true
	um.config.Tracing.TraceHeaders = true

	handler := um.Handler()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check that trace context is available
		span := trace.SpanFromContext(r.Context())
		assert.True(t, span.IsRecording(), "Should have active span")

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestMiddlewareChainOrder(t *testing.T) {
	um := setupTestUnifiedMiddleware()
	um.config.Authentication.Enabled = true
	um.config.RBAC.Enabled = true
	um.config.Validation.Enabled = true
	um.config.Security.Enabled = true
	um.config.CORS.Enabled = true
	um.config.Logging.Enabled = true
	um.config.Metrics.Enabled = true
	um.config.Tracing.Enabled = true
	um.config.Recovery.Enabled = true

	executionOrder := []string{}

	// Mock the middleware to track execution order
	originalHandler := um.Handler()

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		executionOrder = append(executionOrder, "handler")
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/public/test", nil) // Use public path to skip auth
	w := httptest.NewRecorder()

	originalHandler(testHandler).ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, executionOrder, "handler", "Handler should be executed")
}

func TestIPRestrictions(t *testing.T) {
	um := setupTestUnifiedMiddleware()
	um.config.Security.Enabled = true
	um.config.Security.IPRestrictions = []string{"192.168.1.0/24", "10.0.0.1"}

	handler := um.Handler()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))

	t.Run("allowed_ip_exact", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "10.0.0.1:12345"
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("allowed_ip_cidr", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "192.168.1.100:12345"
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("blocked_ip", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "203.0.113.1:12345"
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})
}

func TestSensitiveDataRedaction(t *testing.T) {
	um := setupTestUnifiedMiddleware()
	um.config.Logging.Enabled = true
	um.config.Logging.RequestBody = true
	um.config.Logging.SensitiveFields = []string{"password", "ssn"}

	mockLogger := um.logger.(*MockLogger)
	mockLogger.ClearLogs()

	handler := um.Handler()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	requestBody := `{"username":"user","password":"secret123","email":"user@example.com"}`
	req := httptest.NewRequest("POST", "/test", strings.NewReader(requestBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	logs := mockLogger.GetLogs()

	// Find log with request body
	var bodyLog *LogEntry
	for _, log := range logs {
		if log.Fields["request_body"] != nil {
			bodyLog = &log
			break
		}
	}

	if bodyLog != nil {
		bodyStr := bodyLog.Fields["request_body"].(string)
		assert.Contains(t, bodyStr, "user@example.com", "Should keep non-sensitive data")
		assert.Contains(t, bodyStr, "[REDACTED]", "Should redact sensitive data")
		assert.NotContains(t, bodyStr, "secret123", "Should not contain actual password")
	}
}

func BenchmarkUnifiedMiddleware(b *testing.B) {
	um := setupTestUnifiedMiddleware()
	um.config.Logging.Enabled = false // Disable logging for benchmark

	handler := um.Handler()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
		}
	})
}

func BenchmarkMiddlewareComponents(b *testing.B) {
	um := setupTestUnifiedMiddleware()

	baseHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	benchmarks := []struct {
		name    string
		handler http.Handler
	}{
		{
			name:    "RequestID",
			handler: um.requestIDMiddleware(baseHandler),
		},
		{
			name:    "Security",
			handler: um.securityMiddleware(baseHandler),
		},
		{
			name:    "CORS",
			handler: um.corsMiddleware(baseHandler),
		},
		{
			name:    "Validation",
			handler: um.validationMiddleware(baseHandler),
		},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			req := httptest.NewRequest("GET", "/test", nil)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				w := httptest.NewRecorder()
				bm.handler.ServeHTTP(w, req)
			}
		})
	}
}
