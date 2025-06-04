package models

import (
	"time"

	"github.com/google/uuid"
)

// WithdrawalRequest represents a withdrawal request
type WithdrawalRequest struct {
	ID        uuid.UUID `json:"id" gorm:"primaryKey;type:uuid"`
	UserID    uuid.UUID `json:"user_id" gorm:"type:uuid;index"`
	WalletID  string    `json:"wallet_id" gorm:"index"`
	Asset     string    `json:"asset"`
	Amount    float64   `json:"amount"`
	ToAddress string    `json:"to_address"`
	Status    string    `json:"status"` // pending, approved, rejected, broadcasted, confirmed, failed
	TxID      string    `json:"tx_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Approval represents an approval for a withdrawal request
type Approval struct {
	ID        uuid.UUID `json:"id" gorm:"primaryKey;type:uuid"`
	RequestID uuid.UUID `json:"request_id" gorm:"type:uuid;index"`
	Approver  string    `json:"approver"`
	Approved  bool      `json:"approved"`
	Timestamp time.Time `json:"timestamp"`
}

// WalletAudit represents an audit log entry for a wallet
type WalletAudit struct {
	ID        uuid.UUID `json:"id" gorm:"primaryKey;type:uuid"`
	WalletID  string    `json:"wallet_id"`
	Event     string    `json:"event"`
	Actor     string    `json:"actor"`
	Details   string    `json:"details"`
	CreatedAt time.Time `json:"created_at"`
}
