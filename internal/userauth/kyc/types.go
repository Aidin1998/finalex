package kyc

import "net/http"

// KYCStatus represents the status of a KYC verification
type KYCStatus string

const (
	KYCStatusPending  KYCStatus = "pending"
	KYCStatusApproved KYCStatus = "approved"
	KYCStatusRejected KYCStatus = "rejected"
	KYCStatusExpired  KYCStatus = "expired"
)

// KYCLevel represents the level of KYC verification
type KYCLevel int

const (
	KYCLevelNone     KYCLevel = 0
	KYCLevelBasic    KYCLevel = 1
	KYCLevelStandard KYCLevel = 2
	KYCLevelAdvanced KYCLevel = 3
)

// KYCData represents the data structure for KYC verification
type KYCData struct {
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name"`
	DateOfBirth  string `json:"date_of_birth"`
	Nationality  string `json:"nationality"`
	CountryCode  string `json:"country_code"`
	DocumentType string `json:"document_type"`
	DocumentID   string `json:"document_id"`
	Address      string `json:"address"`
	City         string `json:"city"`
	PostalCode   string `json:"postal_code"`
	PhoneNumber  string `json:"phone_number"`
}

// KYCProvider interface for external KYC providers
type KYCProvider interface {
	StartVerification(userID string, data *KYCData) (string, error)
	GetStatus(sessionID string) (KYCStatus, error)
	WebhookHandler(w http.ResponseWriter, r *http.Request)
}
