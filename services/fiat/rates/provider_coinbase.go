// provider_coinbase.go: Coinbase rates provider
package rates

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
)

type CoinbaseProvider struct{}

func (c *CoinbaseProvider) GetRate(currency string) (float64, error) {
	url := fmt.Sprintf("https://api.coinbase.com/v2/exchange-rates?currency=%s", currency)
	resp, err := http.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return 0, fmt.Errorf("coinbase: status %d", resp.StatusCode)
	}
	var data struct {
		Data struct {
			Rates map[string]string `json:"rates"`
		} `json:"data"`
	}
	body, _ := ioutil.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &data); err != nil {
		return 0, err
	}
	priceStr, ok := data.Data.Rates["USDT"]
	if !ok {
		return 0, fmt.Errorf("coinbase: no USDT rate")
	}
	var price float64
	fmt.Sscanf(priceStr, "%f", &price)
	return price, nil
}
