// Health monitor extensions for compatibility
package marketmaker

import (
	"context"
)

// CheckAll runs all health checks and returns the results
func (hm *HealthMonitor) CheckAll() map[string]*HealthCheckResult {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	results := make(map[string]*HealthCheckResult)

	for key, checker := range hm.checkers {
		result := checker.Check(context.Background())
		results[key] = &result
	}

	return results
}

// GetAllHealthResults returns the latest results of all health checks
func (hm *HealthMonitor) GetAllHealthResults() map[string]*HealthCheckResult {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	results := make(map[string]*HealthCheckResult)

	for key, result := range hm.results {
		// Make a copy to avoid race conditions
		resultCopy := *result
		results[key] = &resultCopy
	}

	return results
}

// StartMonitoring begins periodic health checking (renamed to avoid conflict)
func (hm *HealthMonitor) StartMonitoring(ctx context.Context) {
	// Implementation details
	// This would typically start a goroutine that runs periodic health checks
}

// RegisterChecker registers a health checker by name
func (hm *HealthMonitor) RegisterChecker(name string, checker HealthChecker) {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	hm.checkers[name] = checker
}

// Stop stops the health monitor
func (hm *HealthMonitor) Stop() {
	// Implementation details
	// This would typically signal the health check goroutine to stop
}
