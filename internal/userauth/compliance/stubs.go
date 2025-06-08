package compliance

import (
	"context"
)

// StubGeolocationService provides a stub implementation for testing/development
type StubGeolocationService struct{}

// NewStubGeolocationService creates a new stub geolocation service
func NewStubGeolocationService() GeolocationService {
	return &StubGeolocationService{}
}

// GetLocationInfo returns stub location information
func (s *StubGeolocationService) GetLocationInfo(ip string) (*GeolocationInfo, error) {
	return &GeolocationInfo{
		Country:   "US",
		Region:    "California",
		City:      "San Francisco",
		Latitude:  37.7749,
		Longitude: -122.4194,
		ISP:       "Unknown ISP",
		VPN:       false,
		Proxy:     false,
	}, nil
}

// StubSanctionCheckService provides a stub implementation for testing/development
type StubSanctionCheckService struct{}

// NewStubSanctionCheckService creates a new stub sanction check service
func NewStubSanctionCheckService() SanctionCheckService {
	return &StubSanctionCheckService{}
}

// CheckPerson returns stub sanction check results
func (s *StubSanctionCheckService) CheckPerson(ctx context.Context, req *SanctionCheckRequest) (*SanctionCheckResult, error) {
	return &SanctionCheckResult{
		IsMatch:         false,
		IsPossibleMatch: false,
		MatchDetails:    "No matches found",
		ListSources:     []string{},
	}, nil
}

// StubKYCProvider provides a stub implementation for testing/development
type StubKYCProvider struct{}

// NewStubKYCProvider creates a new stub KYC provider
func NewStubKYCProvider() KYCProvider {
	return &StubKYCProvider{}
}

// GetKYCRequirements returns stub KYC requirements
func (s *StubKYCProvider) GetKYCRequirements(country string) (*KYCRequirements, error) {
	return &KYCRequirements{
		Level:             "basic",
		RequiredDocuments: []string{"passport", "driver_license"},
		VerificationSteps: []string{"document_upload", "selfie_verification"},
	}, nil
}
