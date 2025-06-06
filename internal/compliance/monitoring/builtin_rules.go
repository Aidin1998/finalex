// builtin_rules.go
// Example built-in compliance rules for the real-time compliance engine
package monitoring

import (
	"strings"
)

// LargeTransactionRule flags transactions over a threshold

type LargeTransactionRule struct {
	Threshold float64
}

func (r *LargeTransactionRule) Evaluate(event *ComplianceEvent) (bool, string) {
	if event.EventType == "transaction" && event.Amount > r.Threshold {
		return true, "Large transaction detected"
	}
	return false, ""
}

// SuspiciousLoginRule flags logins from suspicious locations (stub)
type SuspiciousLoginRule struct{}

func (r *SuspiciousLoginRule) Evaluate(event *ComplianceEvent) (bool, string) {
	if event.EventType == "login" {
		if loc, ok := event.Details["location"].(string); ok && strings.Contains(loc, "TOR") {
			return true, "Login from suspicious location (TOR)"
		}
	}
	return false, ""
}
