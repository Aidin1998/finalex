// provider_binance.go: Binance rates provider
package rates

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
)

type BinanceProvider struct{}

func (b *BinanceProvider) GetRate(currency string) (float64, error) {
	baseUrls := []string{"https://api.binance.com", "https://api1.binance.com", "https://api-gcp.binance.com"}
	var lastErr error
	for _, base := range baseUrls {
		url := fmt.Sprintf("%s/api/v3/ticker/price?symbol=%sUSDT", base, currency)
		resp, err := http.Get(url)
		if err != nil {
			lastErr = err
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			lastErr = fmt.Errorf("binance: status %d", resp.StatusCode)
			continue
		}
		var data struct {
			Price string `json:"price"`
		}
		body, _ := ioutil.ReadAll(resp.Body)
		if err := json.Unmarshal(body, &data); err != nil {
			lastErr = err
			continue
		}
		var price float64
		fmt.Sscanf(data.Price, "%f", &price)
		return price, nil
	}
	return 0, lastErr
}
