// Extension methods for Service
package marketmaker

// --- BEGIN: Service and RiskManager stubs for extension compatibility ---
type Service struct{}
type RiskManager struct{}

// --- END: Service and RiskManager stubs for extension compatibility ---

// GetRiskManager returns the risk manager for the service
func (s *Service) GetRiskManager() *RiskManager {
	// No actual field, just return a stub for build compatibility
	return &RiskManager{}
}
