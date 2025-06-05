// Package trading provides trading services and interfaces
package trading

import (
	"github.com/Aidin1998/pincex_unified/internal/trading/settlement"
)

// SettlementProcessor is a simple alias to the settlement.SettlementProcessor
// This maintains backwards compatibility while redirecting to the proper implementation
type SettlementProcessor = settlement.SettlementProcessor

// SettlementMessage is a simple alias to the settlement.SettlementMessage
type SettlementMessage = settlement.SettlementMessage

// SettlementStatus is a simple alias to the settlement.SettlementStatus
type SettlementStatus = settlement.SettlementStatus

// For convenience, re-export the settlement constants
const (
	SettlementPending    = settlement.SettlementPending
	SettlementProcessing = settlement.SettlementProcessing
	SettlementSettled    = settlement.SettlementSettled
	SettlementFailed     = settlement.SettlementFailed
)
