package api

import (
	"errors"
	"net/http"
)

// Exchange represents a cryptocurrency exchange.
type Exchange struct {
	Name       string
	APIKey     string
	APISecret  string
	BaseURL    string
}

// NewExchange creates a new instance of an Exchange.
func NewExchange(name, apiKey, apiSecret, baseURL string) *Exchange {
	return &Exchange{
		Name:      name,
		APIKey:    apiKey,
		APISecret: apiSecret,
		BaseURL:   baseURL,
	}
}

// FetchMarketData retrieves market data from the exchange.
func (e *Exchange) FetchMarketData(symbol string) (interface{}, error) {
	resp, err := http.Get(e.BaseURL + "/marketdata/" + symbol)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("failed to fetch market data")
	}

	var data interface{}
	// Assume we unmarshal the response into data here

	return data, nil
}

// ExecuteTrade executes a trade on the exchange.
func (e *Exchange) ExecuteTrade(order interface{}) error {
	// Implementation for executing a trade
	return nil
}