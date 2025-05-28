// aggregator.go: fetch & average top-10 fiat↔USDT rates
package rates

import (
	"sync"
	"time"
)

type RateProvider interface {
	GetRate(currency string) (float64, error)
}

type Aggregator struct {
	Providers []RateProvider
	Cache     map[string]float64
	CacheTTL  time.Duration
	CacheTime map[string]time.Time
	Mu        sync.Mutex
}

func NewAggregator(providers []RateProvider, ttl time.Duration) *Aggregator {
	return &Aggregator{
		Providers: providers,
		Cache:     make(map[string]float64),
		CacheTTL:  ttl,
		CacheTime: make(map[string]time.Time),
	}
}

func (a *Aggregator) GetAvgRate(currency string) (float64, error) {
	a.Mu.Lock()
	if val, ok := a.Cache[currency]; ok && time.Since(a.CacheTime[currency]) < a.CacheTTL {
		a.Mu.Unlock()
		return val, nil
	}
	a.Mu.Unlock()

	var sum float64
	var count int
	for _, p := range a.Providers {
		rate, err := p.GetRate(currency)
		if err == nil && rate > 0 {
			sum += rate
			count++
		}
	}
	if count == 0 {
		return 0, nil
	}
	avg := sum / float64(count)
	a.Mu.Lock()
	a.Cache[currency] = avg
	a.CacheTime[currency] = time.Now()
	a.Mu.Unlock()
	return avg, nil
}
