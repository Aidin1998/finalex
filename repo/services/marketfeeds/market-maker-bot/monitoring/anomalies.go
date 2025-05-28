package monitoring

import (
	"math"
	"sync"
)

// AnomalyType represents the type of detected anomaly.
type AnomalyType string

const (
	AnomalySpoofing   AnomalyType = "Spoofing"
	AnomalyLayering   AnomalyType = "Layering"
	AnomalyWideSpread AnomalyType = "WideSpread"
)

// AnomalyAlert represents an alert for a detected anomaly.
type AnomalyAlert struct {
	Type    AnomalyType
	OrderID string
	Details string
}

// AnomalyDetector monitors order patterns and spreads for manipulation.
type AnomalyDetector struct {
	mu          sync.Mutex
	lastSpreads []float64
	alerts      []AnomalyAlert
}

func NewAnomalyDetector() *AnomalyDetector {
	return &AnomalyDetector{
		lastSpreads: make([]float64, 0, 100),
		alerts:      make([]AnomalyAlert, 0),
	}
}

// DetectOrderPattern checks for spoofing/layering based on rapid order add/cancel.
func (ad *AnomalyDetector) DetectOrderPattern(orderEvents []OrderEvent) []AnomalyAlert {
	ad.mu.Lock()
	defer ad.mu.Unlock()
	for _, evt := range orderEvents {
		if evt.Type == "cancel" && evt.Lifetime < 2.0 {
			ad.alerts = append(ad.alerts, AnomalyAlert{
				Type:    AnomalySpoofing,
				OrderID: evt.OrderID,
				Details: "Order cancelled very quickly (possible spoofing)",
			})
		}
		if evt.Type == "add" && evt.Size > 10*evt.AvgSize {
			ad.alerts = append(ad.alerts, AnomalyAlert{
				Type:    AnomalyLayering,
				OrderID: evt.OrderID,
				Details: "Unusually large order placed (possible layering)",
			})
		}
	}
	return ad.alerts
}

// DetectWideSpread checks for abnormal spread widening.
func (ad *AnomalyDetector) DetectWideSpread(currentSpread float64) *AnomalyAlert {
	ad.mu.Lock()
	defer ad.mu.Unlock()
	ad.lastSpreads = append(ad.lastSpreads, currentSpread)
	if len(ad.lastSpreads) > 100 {
		ad.lastSpreads = ad.lastSpreads[1:]
	}
	mean, std := meanStd(ad.lastSpreads)
	if currentSpread > mean+3*std {
		alert := AnomalyAlert{
			Type:    AnomalyWideSpread,
			OrderID: "",
			Details: "Spread widened abnormally",
		}
		ad.alerts = append(ad.alerts, alert)
		return &alert
	}
	return nil
}

// meanStd computes mean and stddev of a slice.
func meanStd(vals []float64) (mean, std float64) {
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	mean = sum / float64(len(vals))
	variance := 0.0
	for _, v := range vals {
		variance += (v - mean) * (v - mean)
	}
	std = math.Sqrt(variance / float64(len(vals)))
	return
}

// OrderEvent represents a simplified order event for anomaly detection.
type OrderEvent struct {
	OrderID  string
	Type     string  // "add" or "cancel"
	Lifetime float64 // seconds order was live
	Size     float64
	AvgSize  float64
}
