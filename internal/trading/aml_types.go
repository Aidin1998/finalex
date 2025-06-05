// Package trading provides trading services and interfaces
package trading

import (
	"github.com/Aidin1998/pincex_unified/internal/risk/compliance/aml"
)

// This file contains type aliases for risk and compliance types
// to help break circular dependencies

// RiskMetrics is a simple alias to the aml.RiskMetrics
type RiskMetrics = aml.RiskMetrics

// ComplianceResult is a simple alias to the aml.ComplianceResult
type ComplianceResult = aml.ComplianceResult

// RiskService is a simple alias to the aml.RiskService
type RiskService = aml.RiskService
