package aggregator

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"services/marketfeeds/clients"
	"services/marketfeeds/publisher"

	"github.com/spf13/viper"
)

// Publisher defines methods for publishing alerts and aggregated data
type NewKafkaPublisher interface {
	PublishAlert(exchange, symbol, errStr string)
	PublishAggregated(symbol string, avg float64, ts time.Time, fallback bool)
}

// pub is the global Publisher, replaceable for tests
var pub NewKafkaPublisher = publisher.NewKafkaPublisher("localhost:9092")

const disableTTL = time.Hour

var (
	disabled   = make(map[string]time.Time)
	disabledMu sync.RWMutex

	lastAgg   = make(map[string]float64)
	lastAggMu sync.RWMutex

	// exchangeFns maps exchange names to their price fetch functions; override in tests as needed
	exchangeFns = map[string]func(string) (float64, error){
		"binance":  clients.GetBinanceAvgPrice,
		"coinbase": clients.GetCoinbaseAvgPrice,
		"okx":      clients.GetOKXAvgPrice,
		"bybit":    clients.GetBybitAvgPrice,
		"kraken":   clients.GetKrakenAvgPrice,
	}
)

// FetchAverageAcrossExchanges concurrently fetches price, excludes disabled exchanges
func FetchAverageAcrossExchanges(symbol string) (float64, map[string]float64, error) {
	// use package-level exchangeFns
	exchFns := exchangeFns

	var wg sync.WaitGroup
	mu := &sync.Mutex{}
	prices := make(map[string]float64)
	errs := make([]error, 0)

	now := time.Now()
	// clean expired disabled

	disabledMu.Lock()
	for ex, until := range disabled {
		if now.After(until) {
			delete(disabled, ex)
		}
	}
	disabledMu.Unlock()

	for name, fn := range exchFns {
		// skip disabled
		disabledMu.RLock()
		exp, isDis := disabled[name]
		disabledMu.RUnlock()
		if isDis && now.Before(exp) {
			continue
		}

		wg.Add(1)
		go func(exchange string, f func(string) (float64, error)) {
			defer wg.Done()
			res := make(chan float64, 1)
			errCh := make(chan error, 1)
			go func() {
				p, err := f(symbol)
				if err != nil {
					errCh <- err
					return
				}
				res <- p
			}()

			select {
			case p := <-res:
				mu.Lock()
				prices[exchange] = p
				mu.Unlock()
			case err := <-errCh:
				if strings.Contains(err.Error(), "no data") {
					// unsupported symbol: skip without disabling or alert
				} else {
					// disable exchange and alert
					disabledMu.Lock()
					disabled[exchange] = time.Now().Add(disableTTL)
					disabledMu.Unlock()
					pub.PublishAlert(exchange, symbol, err.Error())
				}
			case <-time.After(5 * time.Second):
				// timeout
				disabledMu.Lock()
				disabled[exchange] = time.Now().Add(disableTTL)
				disabledMu.Unlock()
				pub.PublishAlert(exchange, symbol, "timeout")
				mu.Lock()
				errs = append(errs, fmt.Errorf("%s: timeout", exchange))
				mu.Unlock()
			}
		}(name, fn)
	}

	wg.Wait()

	if len(prices) == 0 {
		return 0, prices, fmt.Errorf("all fetches failed: %v", errs)
	}

	// preliminary average
	sum := 0.0
	for _, v := range prices {
		sum += v
	}
	prelimAvg := sum / float64(len(prices))

	// detect outliers
	threshold := viper.GetFloat64("OUTLIER_THRESHOLD_PERCENT") / 100.0
	for exch, price := range prices {
		if price < prelimAvg*(1-threshold) || price > prelimAvg*(1+threshold) {
			// emit outlier alert and remove
			pub.PublishAlert(exch, symbol,
				fmt.Sprintf("outlier detected: price %.4f vs avg %.4f", price, prelimAvg),
			)
			delete(prices, exch)
		}
	}

	// warning if few sources
	if len(prices) > 0 && len(prices) < 2 {
		pub.PublishAlert("system", symbol, fmt.Sprintf("warning: only %d sources available", len(prices)))
	}

	// if all removed after outlier filter, fallback
	if len(prices) == 0 {
		// fetch last known or emergency price
		lastAggMu.RLock()
		fallbackPrice, ok := lastAgg[symbol]
		lastAggMu.RUnlock()
		if !ok {
			fallbackPrice = viper.GetFloat64("EMERGENCY_PRICE") // default zero if not set
		}
		// send critical alert
		pub.PublishAlert("system", symbol, fmt.Sprintf("fallback used: price %.4f", fallbackPrice))
		// publish aggregated with fallback flag
		pub.PublishAggregated(symbol, fallbackPrice, time.Now(), true)
		return fallbackPrice, prices, fmt.Errorf("all sources unavailable or outliers; fallback price: %.4f", fallbackPrice)
	}

	// final average
	sum = 0.0
	for _, v := range prices {
		sum += v
	}
	finalAvg := sum / float64(len(prices))
	// publish aggregated price for market-maker feed
	pub.PublishAggregated(symbol, finalAvg, time.Now(), false)
	// cache last aggregated per symbol
	lastAggMu.Lock()
	lastAgg[symbol] = finalAvg
	lastAggMu.Unlock()
	return finalAvg, prices, nil
}

// SetOutlierThresholdPercent updates the outlier threshold percent at runtime
func SetOutlierThresholdPercent(p float64) {
	viper.Set("OUTLIER_THRESHOLD_PERCENT", p)
}

// SetMarketMakerRangePercent updates the market maker range percent at runtime
func SetMarketMakerRangePercent(p float64) {
	viper.Set("MAKER_RANGE_PERCENT", p)
}

// DisableExchange manually disables an exchange until TTL expires
func DisableExchange(exchange string) {
	disabledMu.Lock()
	disabled[exchange] = time.Now().Add(disableTTL)
	disabledMu.Unlock()
}

// EnableExchange re-enables a previously disabled exchange
func EnableExchange(exchange string) {
	disabledMu.Lock()
	delete(disabled, exchange)
	disabledMu.Unlock()
}
