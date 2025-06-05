package shared

// EnterpriseRegistrationRequest is a shared type for registration requests
// TODO: Fill in the actual fields as needed by admin/api.go and userauth
// Example placeholder:
type EnterpriseRegistrationRequest struct {
	Email    string                 `json:"email"`
	Password string                 `json:"password"`
	Phone    string                 `json:"phone"`
	Country  string                 `json:"country"`
	DOB      string                 `json:"dob"`
	FullName string                 `json:"full_name"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// AuditContext is a shared type for audit logging context
// Example placeholder:
type AuditContext struct {
	UserID    string                 `json:"user_id,omitempty"`
	IPAddress string                 `json:"ip_address,omitempty"`
	UserAgent string                 `json:"user_agent,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}
