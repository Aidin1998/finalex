package marketmaker

// ExternalLiquiditySource represents an integration with an external venue
// (e.g., another exchange, OTC desk, aggregator)
type ExternalLiquiditySource interface {
	GetQuotes(pair string) (bid, ask, size float64, err error)
	PlaceOrder(pair string, side string, price, size float64) error
}

// Example stub for integration
type DummyExternalSource struct{}

func (d *DummyExternalSource) GetQuotes(pair string) (float64, float64, float64, error) {
	// TODO: implement real integration
	return 100.0, 100.2, 1.0, nil
}

func (d *DummyExternalSource) PlaceOrder(pair, side string, price, size float64) error {
	// TODO: implement real integration
	return nil
}

// Registry for multiple external sources

var externalSources = make(map[string]ExternalLiquiditySource)

func RegisterExternalSource(name string, src ExternalLiquiditySource) {
	externalSources[name] = src
}

func GetBestExternalQuote(pair string) (bid, ask, size float64, srcName string, err error) {
	bestSpread := 1e9
	for name, src := range externalSources {
		b, a, s, e := src.GetQuotes(pair)
		if e == nil && (a-b) < bestSpread {
			bid, ask, size, srcName, bestSpread = b, a, s, name, a-b
		}
	}
	if bestSpread < 1e9 {
		return bid, ask, size, srcName, nil
	}
	return 0, 0, 0, "", err
}
