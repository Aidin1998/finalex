package marketfeeds

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// Top 10 global fiat currencies vs USDT/BTC
var Top10FiatCurrencies = []string{
	"USD", "EUR", "JPY", "GBP", "AUD", "CAD", "CHF", "CNY", "SEK", "NZD",
}

// ExchangeRatesAPIProvider implements FiatRateProvider using exchangerates-api.io
type ExchangeRatesAPIProvider struct {
	name       string
	baseURL    string
	apiKey     string
	httpClient *http.Client
	logger     *zap.Logger
	healthy    bool
	lastError  error
}

// NewExchangeRatesAPIProvider creates a new ExchangeRatesAPI provider
func NewExchangeRatesAPIProvider(logger *zap.Logger, apiKey string) *ExchangeRatesAPIProvider {
	return &ExchangeRatesAPIProvider{
		name:    "exchangerates-api",
		baseURL: "https://api.exchangerates-api.io/v1",
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger:  logger,
		healthy: true,
	}
}

func (e *ExchangeRatesAPIProvider) GetName() string {
	return e.name
}

func (e *ExchangeRatesAPIProvider) IsHealthy() bool {
	return e.healthy
}

func (e *ExchangeRatesAPIProvider) GetRates(ctx context.Context, currencies []string) (map[string]decimal.Decimal, error) {
	url := fmt.Sprintf("%s/latest?access_key=%s&base=USD", e.baseURL, e.apiKey)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		e.healthy = false
		e.lastError = err
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		e.healthy = false
		e.lastError = err
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		e.healthy = false
		e.lastError = err
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var apiResp struct {
		Success bool               `json:"success"`
		Error   map[string]string  `json:"error,omitempty"`
		Base    string             `json:"base"`
		Date    string             `json:"date"`
		Rates   map[string]float64 `json:"rates"`
	}

	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !apiResp.Success {
		e.healthy = false
		e.lastError = fmt.Errorf("API error: %v", apiResp.Error)
		return nil, e.lastError
	}

	e.healthy = true
	e.lastError = nil

	rates := make(map[string]decimal.Decimal)

	// USD is always 1.0 since it's the base
	rates["USD"] = decimal.NewFromInt(1)

	for _, currency := range currencies {
		if rate, exists := apiResp.Rates[currency]; exists {
			rates[currency] = decimal.NewFromFloat(rate)
		}
	}

	return rates, nil
}

func (e *ExchangeRatesAPIProvider) GetRate(ctx context.Context, currency string) (decimal.Decimal, error) {
	rates, err := e.GetRates(ctx, []string{currency})
	if err != nil {
		return decimal.Zero, err
	}

	if rate, exists := rates[currency]; exists {
		return rate, nil
	}

	return decimal.Zero, fmt.Errorf("rate not found for currency %s", currency)
}

// CoinGeckoFiatProvider implements FiatRateProvider using CoinGecko API for crypto-fiat rates
type CoinGeckoFiatProvider struct {
	name       string
	baseURL    string
	httpClient *http.Client
	logger     *zap.Logger
	healthy    bool
	lastError  error
}

// NewCoinGeckoFiatProvider creates a new CoinGecko fiat provider
func NewCoinGeckoFiatProvider(logger *zap.Logger) *CoinGeckoFiatProvider {
	return &CoinGeckoFiatProvider{
		name:    "coingecko-fiat",
		baseURL: "https://api.coingecko.com/api/v3",
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger:  logger,
		healthy: true,
	}
}

func (c *CoinGeckoFiatProvider) GetName() string {
	return c.name
}

func (c *CoinGeckoFiatProvider) IsHealthy() bool {
	return c.healthy
}

// GetRates gets crypto-fiat rates (BTC/USDT vs fiat currencies)
func (c *CoinGeckoFiatProvider) GetRates(ctx context.Context, currencies []string) (map[string]decimal.Decimal, error) {
	// Get BTC and USDT prices in various fiat currencies
	url := fmt.Sprintf("%s/simple/price?ids=bitcoin,tether&vs_currencies=%s",
		c.baseURL,
		joinCurrencies(currencies))

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		c.healthy = false
		c.lastError = err
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.healthy = false
		c.lastError = err
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.healthy = false
		c.lastError = err
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var apiResp struct {
		Bitcoin map[string]float64 `json:"bitcoin"`
		Tether  map[string]float64 `json:"tether"`
	}

	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	c.healthy = true
	c.lastError = nil

	rates := make(map[string]decimal.Decimal)

	// Store both BTC and USDT rates vs fiat currencies
	for _, currency := range currencies {
		currencyLower := toLower(currency)

		if btcRate, exists := apiResp.Bitcoin[currencyLower]; exists {
			rates["BTC-"+currency] = decimal.NewFromFloat(btcRate)
		}

		if usdtRate, exists := apiResp.Tether[currencyLower]; exists {
			rates["USDT-"+currency] = decimal.NewFromFloat(usdtRate)
		}
	}

	return rates, nil
}

func (c *CoinGeckoFiatProvider) GetRate(ctx context.Context, currency string) (decimal.Decimal, error) {
	rates, err := c.GetRates(ctx, []string{currency})
	if err != nil {
		return decimal.Zero, err
	}

	// Try to find USDT rate first, then BTC rate
	if rate, exists := rates["USDT-"+currency]; exists {
		return rate, nil
	}

	if rate, exists := rates["BTC-"+currency]; exists {
		return rate, nil
	}

	return decimal.Zero, fmt.Errorf("rate not found for currency %s", currency)
}

// CurrencyLayerProvider implements FiatRateProvider using CurrencyLayer API
type CurrencyLayerProvider struct {
	name       string
	baseURL    string
	apiKey     string
	httpClient *http.Client
	logger     *zap.Logger
	healthy    bool
	lastError  error
}

// NewCurrencyLayerProvider creates a new CurrencyLayer provider
func NewCurrencyLayerProvider(logger *zap.Logger, apiKey string) *CurrencyLayerProvider {
	return &CurrencyLayerProvider{
		name:    "currencylayer",
		baseURL: "http://api.currencylayer.com",
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger:  logger,
		healthy: true,
	}
}

func (c *CurrencyLayerProvider) GetName() string {
	return c.name
}

func (c *CurrencyLayerProvider) IsHealthy() bool {
	return c.healthy
}

func (c *CurrencyLayerProvider) GetRates(ctx context.Context, currencies []string) (map[string]decimal.Decimal, error) {
	url := fmt.Sprintf("%s/live?access_key=%s&source=USD", c.baseURL, c.apiKey)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		c.healthy = false
		c.lastError = err
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.healthy = false
		c.lastError = err
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.healthy = false
		c.lastError = err
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var apiResp struct {
		Success bool               `json:"success"`
		Error   map[string]string  `json:"error,omitempty"`
		Source  string             `json:"source"`
		Quotes  map[string]float64 `json:"quotes"`
	}

	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !apiResp.Success {
		c.healthy = false
		c.lastError = fmt.Errorf("API error: %v", apiResp.Error)
		return nil, c.lastError
	}

	c.healthy = true
	c.lastError = nil

	rates := make(map[string]decimal.Decimal)

	// USD is always 1.0 since it's the source
	rates["USD"] = decimal.NewFromInt(1)

	for _, currency := range currencies {
		quoteKey := "USD" + currency
		if rate, exists := apiResp.Quotes[quoteKey]; exists {
			rates[currency] = decimal.NewFromFloat(rate)
		}
	}

	return rates, nil
}

func (c *CurrencyLayerProvider) GetRate(ctx context.Context, currency string) (decimal.Decimal, error) {
	rates, err := c.GetRates(ctx, []string{currency})
	if err != nil {
		return decimal.Zero, err
	}

	if rate, exists := rates[currency]; exists {
		return rate, nil
	}

	return decimal.Zero, fmt.Errorf("rate not found for currency %s", currency)
}

// Helper functions
func joinCurrencies(currencies []string) string {
	result := ""
	for i, currency := range currencies {
		if i > 0 {
			result += ","
		}
		result += toLower(currency)
	}
	return result
}

func toLower(s string) string {
	return strings.ToLower(s)
}
