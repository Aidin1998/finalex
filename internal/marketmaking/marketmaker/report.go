package marketmaker

import (
	"time"
)

type LPReport struct {
	Provider  string
	Volume    float64
	PnL       float64
	Rebates   float64
	Timestamp time.Time
}

type ReportService struct{}

func (r *ReportService) GenerateDailyReport() []LPReport {
	// TODO: aggregate from DB/metrics
	return nil
}

// Generate provider and liquidity reports
func (r *ReportService) ProviderPerformanceReport(registry *ProviderRegistry) []LPReport {
	reports := []LPReport{}
	for _, lp := range registry.List() {
		reports = append(reports, LPReport{
			Provider:  lp.Name,
			Volume:    lp.Volume,
			PnL:       0, // TODO: aggregate PnL
			Rebates:   lp.Rebates,
			Timestamp: time.Now(),
		})
	}
	return reports
}

// ProviderPerformanceReportOptimized generates an optimized provider performance report
func (r *ReportService) ProviderPerformanceReportOptimized(registry *ProviderRegistry) []LPReport {
	if registry == nil {
		return nil
	}

	providers := registry.List()
	if len(providers) == 0 {
		return nil
	}

	// Pre-allocate slice for better performance
	reports := make([]LPReport, 0, len(providers))
	now := time.Now()

	for _, lp := range providers {
		reports = append(reports, LPReport{
			Provider:  lp.Name,
			Volume:    lp.Volume,
			PnL:       0, // TODO: aggregate PnL from metrics
			Rebates:   lp.Rebates,
			Timestamp: now,
		})
	}

	return reports
}

func (r *ReportService) LiquidityReport(orderBookDepth map[string]float64) map[string]float64 {
	// Example: return current depth per pair
	return orderBookDepth
}
