package monitoring

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
)

// MetricType defines the type of metric
type MetricType string

const (
	// MetricTypeCounter is a cumulative metric that only increases
	MetricTypeCounter MetricType = "counter"

	// MetricTypeGauge is a metric that can increase and decrease
	MetricTypeGauge MetricType = "gauge"

	// MetricTypeHistogram is a metric that samples observations
	MetricTypeHistogram MetricType = "histogram"
)

// Metric represents a monitoring metric
type Metric struct {
	Name      string            `json:"name"`
	Type      MetricType        `json:"type"`
	Value     float64           `json:"value"`
	Tags      map[string]string `json:"tags"`
	Timestamp time.Time         `json:"timestamp"`
}

// Alert represents a monitoring alert
type Alert struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	Severity     string                 `json:"severity"`
	Description  string                 `json:"description"`
	Data         map[string]interface{} `json:"data"`
	Timestamp    time.Time              `json:"timestamp"`
	Acknowledged bool                   `json:"acknowledged"`
}

// Service handles monitoring operations for the risk module
type Service struct {
	logger  *zap.Logger
	metrics map[string][]*Metric
	alerts  []*Alert
	mu      sync.RWMutex
}

// NewService creates a new monitoring service
func NewService(logger *zap.Logger) *Service {
	return &Service{
		logger:  logger,
		metrics: make(map[string][]*Metric),
		alerts:  make([]*Alert, 0),
	}
}

// RegisterMetric registers a metric
func (s *Service) RegisterMetric(name string, metricType MetricType, value float64, tags map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	metric := &Metric{
		Name:      name,
		Type:      metricType,
		Value:     value,
		Tags:      tags,
		Timestamp: time.Now().UTC(),
	}

	// Store metric
	if _, ok := s.metrics[name]; !ok {
		s.metrics[name] = make([]*Metric, 0)
	}
	s.metrics[name] = append(s.metrics[name], metric)

	// Truncate metrics if there are too many
	if len(s.metrics[name]) > 1000 {
		s.metrics[name] = s.metrics[name][len(s.metrics[name])-1000:]
	}
}

// GetMetrics gets metrics by name
func (s *Service) GetMetrics(ctx context.Context, name string, from, to time.Time) ([]*Metric, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if metrics, ok := s.metrics[name]; ok {
		// Filter metrics by time range
		var result []*Metric
		for _, m := range metrics {
			if (m.Timestamp.Equal(from) || m.Timestamp.After(from)) &&
				(m.Timestamp.Equal(to) || m.Timestamp.Before(to)) {
				result = append(result, m)
			}
		}
		return result, nil
	}

	return []*Metric{}, nil
}

// CreateAlert creates an alert
func (s *Service) CreateAlert(name, severity, description string, data map[string]interface{}) *Alert {
	s.mu.Lock()
	defer s.mu.Unlock()

	alert := &Alert{
		ID:           name + "-" + time.Now().Format(time.RFC3339),
		Name:         name,
		Severity:     severity,
		Description:  description,
		Data:         data,
		Timestamp:    time.Now().UTC(),
		Acknowledged: false,
	}

	s.alerts = append(s.alerts, alert)

	// Truncate alerts if there are too many
	if len(s.alerts) > 1000 {
		s.alerts = s.alerts[len(s.alerts)-1000:]
	}

	return alert
}

// GetAlerts gets alerts
func (s *Service) GetAlerts(ctx context.Context, severity string, from, to time.Time) ([]*Alert, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*Alert
	for _, a := range s.alerts {
		if (severity == "" || a.Severity == severity) &&
			(a.Timestamp.Equal(from) || a.Timestamp.After(from)) &&
			(a.Timestamp.Equal(to) || a.Timestamp.Before(to)) {
			result = append(result, a)
		}
	}

	return result, nil
}

// AcknowledgeAlert acknowledges an alert
func (s *Service) AcknowledgeAlert(ctx context.Context, alertID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, a := range s.alerts {
		if a.ID == alertID {
			a.Acknowledged = true
			break
		}
	}

	return nil
}

// GetSystemStatus gets the system status
func (s *Service) GetSystemStatus(ctx context.Context) (map[string]interface{}, error) {
	// In a real implementation, this would gather system metrics

	status := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().UTC(),
		"metrics": map[string]interface{}{
			"cpu_usage":    0.12,
			"memory_usage": 0.34,
			"disk_usage":   0.56,
		},
	}

	return status, nil
}
