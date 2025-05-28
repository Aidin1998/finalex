package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"services/marketfeeds/internal/connector"
)

type binanceAvgResp struct {
	Mins  int    `json:"mins"`
	Price string `json:"price"`
}

// GetBinanceAvgPrice fetches Binance's 1-minute average price
func GetBinanceAvgPrice(symbol string) (float64, error) {
	url := fmt.Sprintf("https://data-api.binance.vision/api/v3/avgPrice?symbol=%s", symbol)
	client := http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var data binanceAvgResp
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return 0, err
	}
	return strconv.ParseFloat(data.Price, 64)
}

// BinanceConnector implements Connector for Binance
type BinanceConnector struct {
	cfg connector.ExchangeConfig
}

// NewBinanceConnector creates a new BinanceConnector
func NewBinanceConnector() connector.Connector {
	return &BinanceConnector{}
}

// Init initializes the connector with ExchangeConfig
func (c *BinanceConnector) Init(cfg connector.ExchangeConfig) error {
	c.cfg = cfg
	return nil
}

// Run starts the connector processing loop
func (c *BinanceConnector) Run(ctx context.Context) {
	// ...existing logic...
}

// HealthCheck returns the connector health status
func (c *BinanceConnector) HealthCheck() (bool, string) {
	// Implement actual health logic or stub
	return true, "ok"
}

func init() {
	connector.Register("binance", NewBinanceConnector)
}
