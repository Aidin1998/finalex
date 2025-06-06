// PerformanceMetrics extensions
package marketmaker

import (
	"time"
)

// Ensure PerformanceMetrics has all required fields
type PerformanceMetrics struct {
	// Existing fields
	TotalPnL       float64          `json:"total_pnl"`
	TotalTrades    int              `json:"total_trades"`
	SuccessRate    float64          `json:"success_rate"`
	LatencyMetrics map[string]int64 `json:"latency_metrics"`

	// Added fields
	TotalExposure    float64            `json:"total_exposure"`
	InventoryByAsset map[string]float64 `json:"inventory_by_asset"`
	LastUpdate       time.Time          `json:"last_update"`
	VolumeByPair     map[string]float64 `json:"volume_by_pair"`
	WinRatioByPair   map[string]float64 `json:"win_ratio_by_pair"`
}
