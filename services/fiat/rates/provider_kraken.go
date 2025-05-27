// provider_kraken.go: Kraken rates provider
package rates

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
)

type KrakenProvider struct{}

func (k *KrakenProvider) GetRate(currency string) (float64, error) {
	// Map currency to Kraken's pair symbol (e.g. USDUSDT)
	pair := strings.ToUpper(currency) + "USDT"
	url := fmt.Sprintf("https://api.kraken.com/0/public/Ticker?pair=%s", pair)
	resp, err := http.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return 0, fmt.Errorf("kraken: status %d", resp.StatusCode)
	}
	var data struct {
		Result map[string]struct {
			C []string `json:"c"`
		} `json:"result"`
	}
	body, _ := ioutil.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &data); err != nil {
		return 0, err
	}
	for _, v := range data.Result {
		if len(v.C) > 0 {
			var price float64
			fmt.Sscanf(v.C[0], "%f", &price)
			return price, nil
		}
	}
	return 0, fmt.Errorf("kraken: no price found")
}
