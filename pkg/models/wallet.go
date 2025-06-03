package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// StringArray is a custom type for []string with JSON marshaling for GORM/SQLite
// Implements driver.Valuer and sql.Scanner
// Use for Signers and AddressWhitelist

type StringArray []string

func (a StringArray) Value() (driver.Value, error) {
	if len(a) == 0 {
		return "[]", nil
	}
	return json.Marshal(a)
}

func (a *StringArray) Scan(value interface{}) error {
	if value == nil {
		*a = StringArray{}
		return nil
	}
	var bytes []byte
	switch v := value.(type) {
	case string:
		bytes = []byte(v)
	case []byte:
		bytes = v
	default:
		return fmt.Errorf("unsupported type: %T", value)
	}
	return json.Unmarshal(bytes, a)
}

type Wallet struct {
	ID               uuid.UUID   `json:"id" gorm:"primaryKey;type:uuid"`
	UserID           uuid.UUID   `json:"user_id" gorm:"type:uuid;index"`
	Type             string      `json:"type"` // hot, warm, cold
	Asset            string      `json:"asset"`
	Address          string      `json:"address"`
	Balance          float64     `json:"balance"`
	CreatedAt        time.Time   `json:"created_at"`
	UpdatedAt        time.Time   `json:"updated_at"`
	Signers          StringArray `json:"signers" gorm:"type:text"`
	Threshold        int         `json:"threshold" gorm:"default:1"`
	AddressWhitelist StringArray `json:"address_whitelist" gorm:"type:text"`
	IsColdStorage    bool        `json:"is_cold_storage" gorm:"default:false"`
}
