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
}
