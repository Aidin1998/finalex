package marketmaker

import (
	"context"
	"log"
	"math/rand"
	"strconv"
	"time"

	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	OrderBookDepth = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "pincex_orderbook_depth",
			Help: "Order book depth at top N levels",
		},
		[]string{"pair", "side", "level"},
	)
	Spread = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "pincex_orderbook_spread",
			Help: "Order book spread for each pair",
		},
		[]string{"pair"},
	)
	Inventory = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "pincex_mm_inventory",
			Help: "Market maker inventory per pair",
		},
		[]string{"pair"},
	)
)

func init() {
	prometheus.MustRegister(OrderBookDepth, Spread, Inventory)
}

// TradingAPI abstracts trading operations needed by the market maker
// (should be injected in production)
type TradingAPI interface {
	PlaceOrder(ctx context.Context, order *models.Order) (*models.Order, error)
	CancelOrder(ctx context.Context, orderID string) error
	GetOrderBook(pair string, depth int) (*models.OrderBookSnapshot, error)
	GetInventory(pair string) (float64, error)
}

type MarketMaker interface {
	Start(ctx context.Context) error
	Stop() error
	UpdateConfig(cfg MarketMakerConfig) error
}

type MarketMakerConfig struct {
	Pairs        []string
	MinDepth     float64
	TargetSpread float64
	MaxInventory float64
	Strategy     string // e.g. "basic", "dynamic", "inventory-skew"
}

type Service struct {
	cfg      MarketMakerConfig
	quit     chan struct{}
	trading  TradingAPI
	strategy Strategy
}

func NewService(cfg MarketMakerConfig, trading TradingAPI, strategy Strategy) *Service {
	return &Service{cfg: cfg, quit: make(chan struct{}), trading: trading, strategy: strategy}
}

func (s *Service) Start(ctx context.Context) error {
	go s.run(ctx)
	return nil
}

func (s *Service) Stop() error {
	close(s.quit)
	return nil
}

func (s *Service) UpdateConfig(cfg MarketMakerConfig) error {
	s.cfg = cfg
	return nil
}

func (s *Service) run(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			for _, pair := range s.cfg.Pairs {
				// 1. Fetch real order book snapshot
				var mid, vol float64
				var bestBid, bestAsk float64
				if s.trading != nil {
					ob, err := s.trading.GetOrderBook(pair, 5)
					if err == nil && len(ob.Bids) > 0 && len(ob.Asks) > 0 {
						bestBid = ob.Bids[0].Price
						bestAsk = ob.Asks[0].Price
						mid = (bestBid + bestAsk) / 2
						// Simple volatility estimate (stub: random for now)
						vol = 0.01 + 0.01*rand.Float64()
					} else {
						mid = 100.0 + rand.Float64()
						vol = 0.01 + 0.01*rand.Float64()
					}
				} else {
					mid = 100.0 + rand.Float64()
					vol = 0.01 + 0.01*rand.Float64()
				}
				// 2. Get inventory
				inv := 0.0
				if s.trading != nil {
					var err error
					inv, err = s.trading.GetInventory(pair)
					if err != nil {
						log.Printf("inventory fetch error: %v", err)
					}
				}
				// 3. Generate quotes
				bid, ask, size := s.strategy.Quote(mid, vol, inv)
				// 4. Place/cancel/update orders using real trading API
				if s.trading != nil {
					buyOrder := &models.Order{
						Symbol:      pair,
						Side:        "buy",
						Type:        "limit",
						Price:       bid,
						Quantity:    size,
						TimeInForce: "GTC",
					}
					_, _ = s.trading.PlaceOrder(ctx, buyOrder)
					sellOrder := &models.Order{
						Symbol:      pair,
						Side:        "sell",
						Type:        "limit",
						Price:       ask,
						Quantity:    size,
						TimeInForce: "GTC",
					}
					_, _ = s.trading.PlaceOrder(ctx, sellOrder)
				}
				// 5. Record metrics
				Spread.WithLabelValues(pair).Set(ask - bid)
				Inventory.WithLabelValues(pair).Set(inv)
				for i := 0; i < 3; i++ {
					level := strconv.Itoa(i)
					OrderBookDepth.WithLabelValues(pair, "bid", level).Set(bestBid)
					OrderBookDepth.WithLabelValues(pair, "ask", level).Set(bestAsk)
				}
			}
		case <-s.quit:
			return
		}
	}
}
