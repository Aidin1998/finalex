package types

import (
	"time"

	"github.com/google/uuid"
	"github.com/litebittech/cex/services/bookkeeper/store"
	"github.com/shopspring/decimal"
)

type TransactionIn struct {
	ID          uuid.UUID              `json:"tx_id"`
	AccountID   uuid.UUID              `json:"account_id"`
	Type        store.TransactionType  `json:"type"`
	Date        time.Time              `json:"date"`
	Description string                 `json:"description"`
	Entries     []EntryIn              `json:"entries"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

type EntryIn struct {
	AccountID       uuid.UUID       `json:"account_id"`
	IsSystemAccount bool            `json:"is_system_account"`
	Type            store.EntryType `json:"type"`
	Amount          decimal.Decimal `json:"amount"`
	Currency        string          `json:"currency"`
}
