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

type krakenTickerResp struct {
	Result map[string]struct {
		C []string `json:"c"`
	} `json:"result"`
}

// KrakenConnector implements Connector for Kraken
type KrakenConnector struct {
	cfg connector.ExchangeConfig
}

// NewKrakenConnector creates a new KrakenConnector
func NewKrakenConnector() connector.Connector {
	return &KrakenConnector{}
}

// Init initializes the connector with ExchangeConfig
func (c *KrakenConnector) Init(cfg connector.ExchangeConfig) error {
	c.cfg = cfg
	return nil
}

// Run starts the connector processing loop
func (c *KrakenConnector) Run(ctx context.Context) {
	// TODO: implement Run logic
}

// HealthCheck returns the connector health status
func (c *KrakenConnector) HealthCheck() (bool, string) {
	return true, "ok"
}

// GetKrakenAvgPrice fetches Kraken last price via REST
func GetKrakenAvgPrice(pair string) (float64, error) {
	url := fmt.Sprintf("https://api.kraken.com/0/public/Ticker?pair=%s", pair)
	client := http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var data krakenTickerResp
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return 0, err
	}
	for _, v := range data.Result {
		if len(v.C) > 0 {
			return strconv.ParseFloat(v.C[0], 64)
		}
	}
	return 0, fmt.Errorf("no data")
}

func init() {
	connector.Register("kraken", NewKrakenConnector)
}
