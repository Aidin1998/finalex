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

// Add incentive logic and API stubs
func (r *ProviderRegistry) UpdateVolume(id string, volume float64) {
	if lp, ok := r.providers[id]; ok {
		lp.Volume += volume
	}
}

func (r *ProviderRegistry) AddRebate(id string, rebate float64) {
	if lp, ok := r.providers[id]; ok {
		lp.Rebates += rebate
	}
}

func (r *ProviderRegistry) List() []*LiquidityProvider {
	lps := make([]*LiquidityProvider, 0, len(r.providers))
	for _, lp := range r.providers {
		lps = append(lps, lp)
	}
	return lps
}

// API stubs for onboarding and status
func (r *ProviderRegistry) OnboardProvider(lp *LiquidityProvider) {
	lp.Active = true
	r.Register(lp)
}

func (r *ProviderRegistry) GetStatus(id string) (active bool, volume float64, rebates float64) {
	lp := r.Get(id)
	if lp == nil {
		return false, 0, 0
	}
	return lp.Active, lp.Volume, lp.Rebates
}
