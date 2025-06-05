// Package trading provides mock implementations for testing
package trading

// Mock implementation of risk and compliance services

// MockRiskService is a simplified risk service implementation
type MockRiskService struct{}

// NewMockRiskService creates a new mock risk service
func NewMockRiskService() *MockRiskService {
	return &MockRiskService{}
}

// CheckRisk is a mock risk check implementation
func (m *MockRiskService) CheckRisk(userID string, amount float64) (bool, error) {
	// Always approve in mock implementation
	return true, nil
}

// This can be used instead of aml.NewRiskService() when the original is not available
// Just change:
//   aml.NewRiskService()
// to:
//   NewMockRiskService()
