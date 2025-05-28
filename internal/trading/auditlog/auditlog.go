package auditlog

import "context"

// AuditEvent represents an audit log event.
type AuditEvent struct {
	EventType string
	Entity    string
	Action    string
	// Optional fields for audit details
	OrderID  string
	UserID   string
	NewValue interface{}
}

// Audit event type constants
const (
	AuditConfigChange = "CONFIG_CHANGE"
	AuditAdminAction  = "ADMIN_ACTION"
)

// LogAuditEvent is a no-op stub for audit logging.
func LogAuditEvent(ctx context.Context, event AuditEvent) {
	// Implement audit logging or no-op stub
}
