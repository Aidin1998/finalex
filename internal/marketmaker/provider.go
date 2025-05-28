package marketmaker

// LiquidityProvider represents an external or internal LP
// Incentives, performance, and API keys tracked here
type LiquidityProvider struct {
	ID      string
	Name    string
	APIKey  string
	Active  bool
	Volume  float64
	Rebates float64
}

type ProviderRegistry struct {
	providers map[string]*LiquidityProvider
}

func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{providers: make(map[string]*LiquidityProvider)}
}

func (r *ProviderRegistry) Register(lp *LiquidityProvider) {
	r.providers[lp.ID] = lp
}

func (r *ProviderRegistry) Get(id string) *LiquidityProvider {
	return r.providers[id]
}
