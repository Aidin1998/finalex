//go:build integration
// +build integration

package test

import (
	"services/marketfeeds/aggregator"
	"testing"
)

// TestFetchAverageAcrossExchanges_Live3APIs hits three live exchanges (binance, coinbase, okx)
// requires then be run with `go test -tags=integration`.
func TestFetchAverageAcrossExchanges_Live3APIs(t *testing.T) {
	// Disable outlier filtering so no live prices are dropped
	aggregator.SetOutlierThresholdPercent(100.0)

	avg, prices, err := aggregator.FetchAverageAcrossExchanges("BTC/USDT")
	if err != nil {
		t.Fatalf("expected no error fetching live prices, got %v", err)
	}
	if len(prices) < 3 {
		t.Fatalf("expected at least 3 live sources, got %d: %+v", len(prices), prices)
	}
	for exch, p := range prices {
		if p <= 0 {
			t.Errorf("exchange %q returned non-positive price %v", exch, p)
		}
	}
	t.Logf("live avg=%.2f prices=%+v", avg, prices)
}
