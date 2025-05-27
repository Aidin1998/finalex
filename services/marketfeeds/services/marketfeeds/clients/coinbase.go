package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"net/url"
	"services/marketfeeds/internal/connector"

	"github.com/gorilla/websocket"
)

type coinbaseTickerResp struct {
	Price string `json:"price"`
}

// DialCoinbase establishes a websocket connection to the given URL.
func DialCoinbase(wsURL string) (*websocket.Conn, error) {
	u, err := url.Parse(wsURL)
	if err != nil {
		return nil, err
	}
	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	return c, err
}

// GetCoinbaseAvgPrice fetches the latest price via Coinbase REST ticker
func GetCoinbaseAvgPrice(symbol string) (float64, error) {
	url := fmt.Sprintf("https://api.exchange.coinbase.com/products/%s/ticker", symbol)
	client := http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var data coinbaseTickerResp
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return 0, err
	}
	return strconv.ParseFloat(data.Price, 64)
}

// CoinbaseConnector implements Connector for Coinbase
type CoinbaseConnector struct {
	cfg connector.ExchangeConfig
}

func NewCoinbaseConnector() connector.Connector {
	return &CoinbaseConnector{}
}

func (c *CoinbaseConnector) Init(cfg connector.ExchangeConfig) error {
	c.cfg = cfg
	return nil
}

func (c *CoinbaseConnector) Run(ctx context.Context) {
	// TODO: implement Run logic
}

func (c *CoinbaseConnector) HealthCheck() (bool, string) {
	return true, "ok"
}

func init() {
	connector.Register("coinbase", NewCoinbaseConnector)
}
