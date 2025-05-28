package models

import (
	"time"

	"github.com/google/uuid"
)

type Wallet struct {
	ID        uuid.UUID `json:"id" gorm:"primaryKey;type:uuid"`
	UserID    uuid.UUID `json:"user_id" gorm:"type:uuid;index"`
	Type      string    `json:"type"` // hot, warm, cold
	Asset     string    `json:"asset"`
	Address   string    `json:"address"`
	Balance   float64   `json:"balance"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Multi-sig fields
	Signers   []string `json:"signers" gorm:"type:text[]"` // user IDs, emails, or pubkeys
	Threshold int      `json:"threshold" gorm:"default:1"`

	// Address whitelisting
	AddressWhitelist []string `json:"address_whitelist" gorm:"type:text[]"`

	// Cold storage flag
	IsColdStorage bool `json:"is_cold_storage" gorm:"default:false"`
}
