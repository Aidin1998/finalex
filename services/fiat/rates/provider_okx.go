package rates

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
)

type OKXProvider struct{}

func (o *OKXProvider) GetRate(currency string) (float64, error) {
	instId := fmt.Sprintf("%s-USDT", currency)
	url := fmt.Sprintf("https://www.okx.com/api/v5/market/ticker?instId=%s", instId)
	resp, err := http.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return 0, fmt.Errorf("okx: status %d", resp.StatusCode)
	}
	var data struct {
		Data []struct {
			Last string `json:"last"`
		} `json:"data"`
	}
	body, _ := ioutil.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &data); err != nil {
		return 0, err
	}
	if len(data.Data) == 0 {
		return 0, fmt.Errorf("okx: no data")
	}
	var price float64
	fmt.Sscanf(data.Data[0].Last, "%f", &price)
	return price, nil
}
