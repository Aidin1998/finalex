package models

import (
	"time"

	"github.com/google/uuid"
)

// KYCRequest represents a KYC verification request
// Each user can have multiple requests (for re-verification, etc.)
type KYCRequest struct {
	ID          uuid.UUID  `json:"id" gorm:"primaryKey;type:uuid"`
	UserID      uuid.UUID  `json:"user_id" gorm:"type:uuid;index"`
	Provider    string     `json:"provider"`
	Status      string     `json:"status"`
	Level       int        `json:"level"` // 1=basic, 2=document, 3=EDD
	RiskScore   int        `json:"risk_score"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	CompletedAt *time.Time `json:"completed_at"`
	AuditTrail  []KYCAudit `json:"audit_trail" gorm:"foreignKey:RequestID"`
}

// KYCAudit represents an audit log entry for a KYC request
type KYCAudit struct {
	ID        uuid.UUID `json:"id" gorm:"primaryKey;type:uuid"`
	RequestID uuid.UUID `json:"request_id" gorm:"type:uuid;index"`
	Event     string    `json:"event"`
	Actor     string    `json:"actor"` // user, admin, system
	Details   string    `json:"details"`
	CreatedAt time.Time `json:"created_at"`
}

// AMLAlert represents a suspicious activity alert
type AMLAlert struct {
	ID        uuid.UUID  `json:"id" gorm:"primaryKey;type:uuid"`
	UserID    uuid.UUID  `json:"user_id" gorm:"type:uuid;index"`
	Type      string     `json:"type"` // transaction, login, etc.
	Reason    string     `json:"reason"`
	Status    string     `json:"status"` // open, closed, escalated
	CreatedAt time.Time  `json:"created_at"`
	ClosedAt  *time.Time `json:"closed_at"`
}
