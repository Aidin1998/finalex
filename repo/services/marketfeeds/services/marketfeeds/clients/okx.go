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

type okxTickerResp struct {
	Data []struct {
		Last string `json:"last"`
	} `json:"data"`
}

// OKXConnector implements Connector for OKX
type OKXConnector struct {
	cfg connector.ExchangeConfig
}

// NewOKXConnector creates a new OKXConnector
func NewOKXConnector() connector.Connector {
	return &OKXConnector{}
}

// Init initializes the connector with ExchangeConfig
func (c *OKXConnector) Init(cfg connector.ExchangeConfig) error {
	c.cfg = cfg
	return nil
}

// Run starts the connector processing loop
func (c *OKXConnector) Run(ctx context.Context) {
	// TODO: implement Run logic
}

// HealthCheck returns the connector health status
func (c *OKXConnector) HealthCheck() (bool, string) {
	return true, "ok"
}

// GetOKXAvgPrice fetches OKX last price via REST
func GetOKXAvgPrice(symbol string) (float64, error) {
	url := fmt.Sprintf("https://www.okx.com/api/v5/market/ticker?instId=%s", symbol)
	client := http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var data okxTickerResp
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return 0, err
	}
	if len(data.Data) == 0 {
		return 0, fmt.Errorf("no data")
	}
	return strconv.ParseFloat(data.Data[0].Last, 64)
}

func init() {
	connector.Register("okx", NewOKXConnector)
}
