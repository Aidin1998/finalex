// Add a testable wrapper for processSettlement
// This must be in the same package as the implementation, so we use a build tag for test-only export.
//go:build unit && settlement
// +build unit,settlement

package settlement

func ProcessSettlementForTest(sp *SettlementProcessor, sm SettlementMessage) (SettlementStatus, string, error) {
	return sp.processSettlement(sm)
}
