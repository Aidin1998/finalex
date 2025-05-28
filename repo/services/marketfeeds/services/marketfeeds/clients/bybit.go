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

type bybitTickerResp struct {
	Result []struct {
		Price string `json:"last_price"`
	} `json:"result"`
}

// GetBybitAvgPrice fetches Bybit last price via REST
func GetBybitAvgPrice(symbol string) (float64, error) {
	url := fmt.Sprintf("https://api.bybit.com/v2/public/tickers?symbol=%s", symbol)
	client := http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var data bybitTickerResp
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return 0, err
	}
	if len(data.Result) == 0 {
		return 0, fmt.Errorf("no data")
	}
	return strconv.ParseFloat(data.Result[0].Price, 64)
}

// BybitConnector implements Connector for Bybit
type BybitConnector struct {
	cfg connector.ExchangeConfig
}

// NewBybitConnector creates a new BybitConnector
func NewBybitConnector() connector.Connector {
	return &BybitConnector{}
}

// Init initializes the connector with ExchangeConfig
func (c *BybitConnector) Init(cfg connector.ExchangeConfig) error {
	c.cfg = cfg
	return nil
}

// Run starts the connector processing loop
func (c *BybitConnector) Run(ctx context.Context) {
	// TODO: implement Run logic
}

// HealthCheck returns the connector health status
func (c *BybitConnector) HealthCheck() (bool, string) {
	return true, "ok"
}

func init() {
	connector.Register("bybit", NewBybitConnector)
}
