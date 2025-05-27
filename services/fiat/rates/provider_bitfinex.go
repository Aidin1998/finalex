package rates

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
)

type BitfinexProvider struct{}

func (b *BitfinexProvider) GetRate(currency string) (float64, error) {
	symbol := fmt.Sprintf("%sUSDT", currency)
	url := fmt.Sprintf("https://api.bitfinex.com/v1/pubticker/%s", symbol)
	resp, err := http.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return 0, fmt.Errorf("bitfinex: status %d", resp.StatusCode)
	}
	var data struct {
		LastPrice string `json:"last_price"`
	}
	body, _ := ioutil.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &data); err != nil {
		return 0, err
	}
	var price float64
	fmt.Sscanf(data.LastPrice, "%f", &price)
	return price, nil
}
