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
