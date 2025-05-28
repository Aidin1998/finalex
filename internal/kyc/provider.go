package kyc

import (
	"net/http"
)

// KYCProvider defines the interface for external KYC provider integration
// (e.g., Jumio, Onfido, etc.)
type KYCProvider interface {
	StartVerification(userID string, data *KYCData) (sessionID string, err error)
	GetStatus(sessionID string) (KYCStatus, error)
	WebhookHandler(w http.ResponseWriter, r *http.Request)
}

// KYCData holds user info and documents for verification
// Extend as needed for provider requirements
type KYCData struct {
	FirstName   string
	LastName    string
	DOB         string
	Country     string
	IDType      string
	IDNumber    string
	IDImageURL  string
	SelfieURL   string
	Address     string
	ExtraFields map[string]string
}

type KYCStatus string

const (
	KYCStatusPending  KYCStatus = "pending"
	KYCStatusApproved KYCStatus = "approved"
	KYCStatusRejected KYCStatus = "rejected"
	KYCStatusReview   KYCStatus = "manual_review"
)
