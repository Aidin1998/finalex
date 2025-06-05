// Package trading provides trading services and interfaces
package trading

import (
	"github.com/Aidin1998/finalex/internal/trading/coordination"
)

// This file contains type aliases to break circular dependencies between
// trading and coordination packages

// TradeWorkflow is a simple alias to the coordination.TradeWorkflow
type TradeWorkflow = coordination.TradeWorkflow

// WorkflowStep is a simple alias to the coordination.WorkflowStep
type WorkflowStep = coordination.WorkflowStep

// CoordinationService is a simple alias to the coordination.CoordinationService
type CoordinationService = coordination.CoordinationService
