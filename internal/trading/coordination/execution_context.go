package coordination

import (
	"time"

	"github.com/Aidin1998/finalex/internal/trading/consensus"
	"github.com/google/uuid"
)

// ExecutionContext provides context for atomic trade execution
type ExecutionContext struct {
	TransactionID     uuid.UUID              `json:"transaction_id"`
	InitiatedAt       time.Time              `json:"initiated_at"`
	ExpiresAt         time.Time              `json:"expires_at"`
	Operations        []consensus.Operation  `json:"operations"`
	Metadata          map[string]interface{} `json:"metadata,omitempty"`
	Priority          int                    `json:"priority"`
	RequiresConsensus bool                   `json:"requires_consensus"`
}

// Operation represents a coordination operation (alias for consensus.Operation)
type Operation = consensus.Operation

// NewExecutionContext creates a new execution context
func NewExecutionContext(transactionID uuid.UUID) *ExecutionContext {
	now := time.Now()
	return &ExecutionContext{
		TransactionID:     transactionID,
		InitiatedAt:       now,
		ExpiresAt:         now.Add(30 * time.Minute), // Default 30 minute timeout
		Operations:        make([]consensus.Operation, 0),
		Metadata:          make(map[string]interface{}),
		Priority:          3, // Normal priority
		RequiresConsensus: false,
	}
}

// AddOperation adds an operation to the execution context
func (ec *ExecutionContext) AddOperation(op consensus.Operation) {
	ec.Operations = append(ec.Operations, op)
}

// IsExpired checks if the execution context has expired
func (ec *ExecutionContext) IsExpired() bool {
	return time.Now().After(ec.ExpiresAt)
}
