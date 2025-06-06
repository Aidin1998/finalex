// api.go: HTTP API for MarketFeeds (MarketMaker module)
package marketfeeds

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// RegisterMarketFeedsAPI registers HTTP handlers for marketfeeds endpoints
func RegisterMarketFeedsAPI(mux *http.ServeMux, svc EnhancedMarketFeedService) {
	mux.HandleFunc("/api/marketfeeds/price/", func(w http.ResponseWriter, r *http.Request) {
		symbol := strings.TrimPrefix(r.URL.Path, "/api/marketfeeds/price/")
		if symbol == "" {
			http.Error(w, "symbol required", http.StatusBadRequest)
			return
		}
		ctx := r.Context()
		ticker, err := svc.GetAggregatedTicker(ctx, symbol)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		vol, _ := getVolatility(svc, ctx, symbol)
		resp := map[string]interface{}{
			"symbol":     ticker.Symbol,
			"price":      ticker.Price,
			"bid":        ticker.BidPrice,
			"ask":        ticker.AskPrice,
			"mid":        ticker.Price,
			"min":        ticker.LowPrice,
			"max":        ticker.HighPrice,
			"volatility": vol,
			"updated_at": ticker.UpdatedAt,
		}
		json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("/api/marketfeeds/prices", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		prices, err := svc.GetMarketPrices(ctx)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		resp := make([]map[string]interface{}, 0, len(prices))
		for _, p := range prices {
			vol, _ := getVolatility(svc, ctx, p.Symbol)
			resp = append(resp, map[string]interface{}{
				"symbol":     p.Symbol,
				"price":      p.Price,
				"min":        p.Low24h,
				"max":        p.High24h,
				"volatility": vol,
				"updated_at": p.UpdatedAt,
			})
		}
		json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("/api/marketfeeds/orderbook/", func(w http.ResponseWriter, r *http.Request) {
		symbol := strings.TrimPrefix(r.URL.Path, "/api/marketfeeds/orderbook/")
		if symbol == "" {
			http.Error(w, "symbol required", http.StatusBadRequest)
			return
		}
		ctx := r.Context()
		orderbook, err := svc.GetAggregatedOrderBook(ctx, symbol, 20)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(orderbook)
	})

	// Prometheus metrics endpoint (assumes metrics are registered elsewhere)
	mux.Handle("/api/marketfeeds/metrics", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// This should be hooked to prometheus HTTP handler in main server
		http.NotFound(w, r)
	}))
}

// getVolatility is a placeholder for volatility calculation (to be implemented)
func getVolatility(svc EnhancedMarketFeedService, ctx context.Context, symbol string) (float64, error) {
	// TODO: Implement rolling volatility calculation using price history
	return 0, nil
}
