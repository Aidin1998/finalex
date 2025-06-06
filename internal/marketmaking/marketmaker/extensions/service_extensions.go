// Extension methods for Service
package marketmaker

// GetRiskManager returns the risk manager for the service
func (s *Service) GetRiskManager() *RiskManager {
	return s.riskManager
}
