// Extension methods for risk-related types
package marketmaker

// String method for RiskSignalType
func (r RiskSignalType) String() string {
	switch r {
	case InventoryBreach:
		return "inventory_breach"
	case PnLBreach:
		return "pnl_breach"
	case VaRBreach:
		return "var_breach"
	case CorrelationBreach:
		return "correlation_breach"
	case LiquidityBreach:
		return "liquidity_breach"
	case VolatilitySpike:
		return "volatility_spike"
	case DrawdownBreach:
		return "drawdown_breach"
	default:
		return "unknown"
	}
}

// String method for RiskSeverity
func (r RiskSeverity) String() string {
	switch r {
	case LowRisk:
		return "low"
	case MediumRisk:
		return "medium"
	case HighRisk:
		return "high"
	case CriticalRisk:
		return "critical"
	default:
		return "unknown"
	}
}
