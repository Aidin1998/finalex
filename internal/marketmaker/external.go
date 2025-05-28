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
