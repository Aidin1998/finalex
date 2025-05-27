// handler.go: HTTP endpoints for /fiat/deposit, /fiat/exchange, /fiat/rates
package api

import (
	"encoding/json"
	"net/http"

	"github.com/yourorg/fiat/exchange"
	"github.com/yourorg/fiat/rates"
)

// TODO: Implement HTTP handlers

type RateHandler struct {
	Aggregator *rates.Aggregator
}

func (h *RateHandler) GetRates(w http.ResponseWriter, r *http.Request) {
	currencies := []string{"USD", "EUR", "GBP", "JPY", "AUD", "CAD", "CHF", "SGD", "HKD", "NZD"}
	result := make(map[string]float64)
	for _, c := range currencies {
		avg, _ := h.Aggregator.GetAvgRate(c)
		result[c] = avg
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

type ExchangeHandler struct {
	Service *exchange.ExchangeService
}

type exchangeRequest struct {
	FromCurrency string  `json:"fromCurrency"`
	ToCurrency   string  `json:"toCurrency"`
	Amount       float64 `json:"amount"`
}

type exchangeResponse struct {
	NetAmount float64 `json:"netAmount"`
	Rate      float64 `json:"rate"`
}

func (h *ExchangeHandler) Exchange(w http.ResponseWriter, r *http.Request) {
	var req exchangeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var net, rate float64
	var err error
	if req.ToCurrency == "BTC" || req.ToCurrency == "ETH" {
		net, rate, err = h.Service.ConvertFiatToCrypto(req.FromCurrency, req.ToCurrency, req.Amount)
	} else {
		net, rate, err = h.Service.ConvertFiatToFiat(req.FromCurrency, req.ToCurrency, req.Amount)
	}
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(exchangeResponse{NetAmount: net, Rate: rate})
}
