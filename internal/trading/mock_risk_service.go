// Package trading provides mock implementations for testing
package trading

// Mock implementation of risk and compliance services

// SimpleMockRiskService is a simplified risk service implementation
type SimpleMockRiskService struct{}

// NewSimpleMockRiskService creates a new simplified mock risk service
func NewSimpleMockRiskService() *SimpleMockRiskService {
	return &SimpleMockRiskService{}
}

// CheckRisk is a mock risk check implementation
func (m *SimpleMockRiskService) CheckRisk(userID string, amount float64) (bool, error) {
	// Always approve in mock implementation
	return true, nil
}

// This can be used instead of aml.NewRiskService() when the original is not available
// Just change:
//   aml.NewRiskService()
// to:
//   NewSimpleMockRiskService()
