package models

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID         uuid.UUID `gorm:"primaryKey"`
	ProviderID string    `gorm:"unique;not null"`
	Username   string    `gorm:"unique;not null"`
	Email      string    `gorm:"unique;not null"`
	FirstName  string
	LastName   string

	CreatedAt time.Time `gorm:"type:timestamptz"`
	UpdatedAt time.Time `gorm:"type:timestamptz"`
}

type UserFilter struct {
	Limit         int
	Offset        int
	CreatedBefore *time.Time
	CreatedAfter  *time.Time
}
