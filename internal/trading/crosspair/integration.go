package crosspair

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ServiceIntegration provides integration with existing platform services
type ServiceIntegration struct {
	balanceService   BalanceServiceInterface
	spotEngine       SpotEngineInterface
	pairRegistry     PairRegistryInterface
	feeEngine        FeeEngineInterface
	eventPublisher   EventPublisherInterface
	metricsCollector MetricsCollectorInterface
	coordService     CoordinationServiceInterface
}

// BalanceServiceInterface defines the interface for balance management
type BalanceServiceInterface interface {
	GetBalance(ctx context.Context, userID uuid.UUID, currency string) (float64, error)
	ReserveBalance(ctx context.Context, userID uuid.UUID, currency string, amount float64) error
	ReleaseReservation(ctx context.Context, userID uuid.UUID, currency string, amount float64) error
	TransferBalance(ctx context.Context, from, to uuid.UUID, currency string, amount float64) error
	AtomicMultiAssetTransfer(ctx context.Context, transfers []AssetTransfer) error
}

// SpotEngineInterface defines the interface for spot trading engine
type SpotEngineInterface interface {
	GetOrderbook(ctx context.Context, pair string) (*Orderbook, error)
	PlaceOrder(ctx context.Context, order *SpotOrder) (*SpotOrder, error)
	CancelOrder(ctx context.Context, orderID uuid.UUID) error
	GetOrderStatus(ctx context.Context, orderID uuid.UUID) (*SpotOrder, error)
	SubscribeToTrades(pair string, callback func(*Trade)) error
	UnsubscribeFromTrades(pair string) error
}

// PairRegistryInterface defines the interface for trading pair management
type PairRegistryInterface interface {
	GetPair(ctx context.Context, pair string) (*TradingPair, error)
	GetAllPairs(ctx context.Context) ([]*TradingPair, error)
	IsPairActive(ctx context.Context, pair string) (bool, error)
	GetPairsByBase(ctx context.Context, baseCurrency string) ([]*TradingPair, error)
	GetPairsByQuote(ctx context.Context, quoteCurrency string) ([]*TradingPair, error)
	FindRoute(ctx context.Context, from, to string) ([]*CrossPairRoute, error)
}

// FeeEngineInterface defines the interface for fee calculation
type FeeEngineInterface interface {
	CalculateFee(ctx context.Context, userID uuid.UUID, pair string, amount float64, feeType string) (float64, error)
	GetFeeSchedule(ctx context.Context, userID uuid.UUID) (*FeeSchedule, error)
	GetFeeDiscount(ctx context.Context, userID uuid.UUID) (float64, error)
}

// EventPublisherInterface defines the interface for event publishing
type EventPublisherInterface interface {
	PublishOrderEvent(ctx context.Context, event *OrderEvent) error
	PublishTradeEvent(ctx context.Context, event *TradeEvent) error
	PublishBalanceEvent(ctx context.Context, event *BalanceEvent) error
	PublishErrorEvent(ctx context.Context, event *ErrorEvent) error
}

// MetricsCollectorInterface defines the interface for metrics collection
type MetricsCollectorInterface interface {
	IncrementCounter(name string, tags map[string]string)
	RecordHistogram(name string, value float64, tags map[string]string)
	RecordGauge(name string, value float64, tags map[string]string)
	RecordTimer(name string, duration time.Duration, tags map[string]string)
}

// CoordinationServiceInterface defines the interface for distributed coordination
type CoordinationServiceInterface interface {
	AcquireLock(ctx context.Context, lockKey string, ttl time.Duration) (LockHandle, error)
	ReleaseLock(ctx context.Context, handle LockHandle) error
	GetDistributedSequence(ctx context.Context, sequenceName string) (int64, error)
}

// Data structures for integration

type AssetTransfer struct {
	UserID   uuid.UUID `json:"user_id"`
	Currency string    `json:"currency"`
	Amount   float64   `json:"amount"`
	Type     string    `json:"type"` // "debit" or "credit"
}

type Orderbook struct {
	Pair      string       `json:"pair"`
	Bids      []OrderLevel `json:"bids"`
	Asks      []OrderLevel `json:"asks"`
	Timestamp time.Time    `json:"timestamp"`
}

type OrderLevel struct {
	Price    float64 `json:"price"`
	Quantity float64 `json:"quantity"`
}

type SpotOrder struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	Pair      string    `json:"pair"`
	Side      string    `json:"side"`
	Type      string    `json:"type"`
	Quantity  float64   `json:"quantity"`
	Price     float64   `json:"price"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Trade struct {
	ID        uuid.UUID `json:"id"`
	Pair      string    `json:"pair"`
	Price     float64   `json:"price"`
	Quantity  float64   `json:"quantity"`
	Side      string    `json:"side"`
	Timestamp time.Time `json:"timestamp"`
}

type TradingPair struct {
	Pair         string  `json:"pair"`
	Base         string  `json:"base"`
	Quote        string  `json:"quote"`
	Active       bool    `json:"active"`
	MinQuantity  float64 `json:"min_quantity"`
	MaxQuantity  float64 `json:"max_quantity"`
	PriceStep    float64 `json:"price_step"`
	QuantityStep float64 `json:"quantity_step"`
}

type FeeSchedule struct {
	MakerFee       float64            `json:"maker_fee"`
	TakerFee       float64            `json:"taker_fee"`
	PairSpecific   map[string]float64 `json:"pair_specific"`
	VolumeDiscount float64            `json:"volume_discount"`
}

type OrderEvent struct {
	Type      string          `json:"type"`
	OrderID   uuid.UUID       `json:"order_id"`
	UserID    uuid.UUID       `json:"user_id"`
	Order     *CrossPairOrder `json:"order"`
	Timestamp time.Time       `json:"timestamp"`
}

type TradeEvent struct {
	Type      string          `json:"type"`
	TradeID   uuid.UUID       `json:"trade_id"`
	OrderID   uuid.UUID       `json:"order_id"`
	UserID    uuid.UUID       `json:"user_id"`
	Trade     *CrossPairTrade `json:"trade"`
	Timestamp time.Time       `json:"timestamp"`
}

type BalanceEvent struct {
	Type      string    `json:"type"`
	UserID    uuid.UUID `json:"user_id"`
	Currency  string    `json:"currency"`
	Amount    float64   `json:"amount"`
	Balance   float64   `json:"balance"`
	Timestamp time.Time `json:"timestamp"`
}

type ErrorEvent struct {
	Type      string     `json:"type"`
	Error     string     `json:"error"`
	Context   string     `json:"context"`
	UserID    *uuid.UUID `json:"user_id,omitempty"`
	OrderID   *uuid.UUID `json:"order_id,omitempty"`
	Timestamp time.Time  `json:"timestamp"`
}

type LockHandle interface {
	Key() string
	Acquired() time.Time
	TTL() time.Duration
}

// NewServiceIntegration creates a new service integration instance
func NewServiceIntegration(
	balanceService BalanceServiceInterface,
	spotEngine SpotEngineInterface,
	pairRegistry PairRegistryInterface,
	feeEngine FeeEngineInterface,
	eventPublisher EventPublisherInterface,
	metricsCollector MetricsCollectorInterface,
	coordService CoordinationServiceInterface,
) *ServiceIntegration {
	return &ServiceIntegration{
		balanceService:   balanceService,
		spotEngine:       spotEngine,
		pairRegistry:     pairRegistry,
		feeEngine:        feeEngine,
		eventPublisher:   eventPublisher,
		metricsCollector: metricsCollector,
		coordService:     coordService,
	}
}

// RealOrderbookProvider implements OrderbookProvider using the spot engine
type RealOrderbookProvider struct {
	spotEngine SpotEngineInterface
}

func NewRealOrderbookProvider(spotEngine SpotEngineInterface) *RealOrderbookProvider {
	return &RealOrderbookProvider{spotEngine: spotEngine}
}

func (p *RealOrderbookProvider) GetOrderbook(ctx context.Context, pair string) (*Orderbook, error) {
	return p.spotEngine.GetOrderbook(ctx, pair)
}

func (p *RealOrderbookProvider) Subscribe(pair string, callback func(*Orderbook)) error {
	// Subscribe to trades and reconstruct orderbook updates
	return p.spotEngine.SubscribeToTrades(pair, func(trade *Trade) {
		// This is a simplified approach - in reality, you'd need proper orderbook updates
		if orderbook, err := p.spotEngine.GetOrderbook(context.Background(), pair); err == nil {
			callback(orderbook)
		}
	})
}

func (p *RealOrderbookProvider) Unsubscribe(pair string) error {
	return p.spotEngine.UnsubscribeFromTrades(pair)
}

func (p *RealOrderbookProvider) GetBestBidAsk(ctx context.Context, pair string) (bid, ask decimal.Decimal, err error) {
	orderbook, err := p.spotEngine.GetOrderbook(ctx, pair)
	if err != nil {
		return decimal.Zero, decimal.Zero, err
	}

	if len(orderbook.Bids) == 0 || len(orderbook.Asks) == 0 {
		return decimal.Zero, decimal.Zero, fmt.Errorf("no liquidity available for pair %s", pair)
	}

	bid = decimal.NewFromFloat(orderbook.Bids[0].Price)
	ask = decimal.NewFromFloat(orderbook.Asks[0].Price)
	return bid, ask, nil
}

// RealEventPublisher implements EventPublisher using the platform's event system
type RealEventPublisher struct {
	eventPublisher EventPublisherInterface
}

func NewRealEventPublisher(eventPublisher EventPublisherInterface) *RealEventPublisher {
	return &RealEventPublisher{eventPublisher: eventPublisher}
}

func (p *RealEventPublisher) PublishOrderCreated(order *CrossPairOrder) error {
	event := &OrderEvent{
		Type:      "order_created",
		OrderID:   order.ID,
		UserID:    order.UserID,
		Order:     order,
		Timestamp: time.Now(),
	}
	return p.eventPublisher.PublishOrderEvent(context.Background(), event)
}

func (p *RealEventPublisher) PublishOrderCompleted(order *CrossPairOrder) error {
	event := &OrderEvent{
		Type:      "order_completed",
		OrderID:   order.ID,
		UserID:    order.UserID,
		Order:     order,
		Timestamp: time.Now(),
	}
	return p.eventPublisher.PublishOrderEvent(context.Background(), event)
}

func (p *RealEventPublisher) PublishOrderCancelled(order *CrossPairOrder) error {
	event := &OrderEvent{
		Type:      "order_cancelled",
		OrderID:   order.ID,
		UserID:    order.UserID,
		Order:     order,
		Timestamp: time.Now(),
	}
	return p.eventPublisher.PublishOrderEvent(context.Background(), event)
}

func (p *RealEventPublisher) PublishTradeExecuted(trade *CrossPairTrade) error {
	event := &TradeEvent{
		Type:      "trade_executed",
		TradeID:   trade.ID,
		OrderID:   trade.OrderID,
		UserID:    trade.UserID,
		Trade:     trade,
		Timestamp: time.Now(),
	}
	return p.eventPublisher.PublishTradeEvent(context.Background(), event)
}

func (p *RealEventPublisher) PublishOrderUpdated(order *CrossPairOrder) error {
	event := &OrderEvent{
		Type:      "order_updated",
		OrderID:   order.ID,
		UserID:    order.UserID,
		Order:     order,
		Timestamp: time.Now(),
	}
	return p.eventPublisher.PublishOrderEvent(context.Background(), event)
}

func (p *RealEventPublisher) PublishExecutionFailed(orderID uuid.UUID, reason string) error {
	event := &ErrorEvent{
		Type:      "execution_failed",
		Error:     reason,
		Context:   "cross_pair_order",
		OrderID:   &orderID,
		Timestamp: time.Now(),
	}
	return p.eventPublisher.PublishErrorEvent(context.Background(), event)
}

// RealMetricsCollector implements MetricsCollector using the platform's metrics system
type RealMetricsCollector struct {
	metricsCollector MetricsCollectorInterface
}

func NewRealMetricsCollector(metricsCollector MetricsCollectorInterface) *RealMetricsCollector {
	return &RealMetricsCollector{metricsCollector: metricsCollector}
}

func (m *RealMetricsCollector) RecordOrderCreated(pair string, orderType string) {
	m.metricsCollector.IncrementCounter("crosspair_orders_created_total", map[string]string{
		"pair": pair,
		"type": orderType,
	})
}

func (m *RealMetricsCollector) RecordOrderCompleted(pair string, executionTime time.Duration) {
	m.metricsCollector.IncrementCounter("crosspair_orders_completed_total", map[string]string{
		"pair": pair,
	})
	m.metricsCollector.RecordTimer("crosspair_order_execution_duration", executionTime, map[string]string{
		"pair": pair,
	})
}

func (m *RealMetricsCollector) RecordOrderFailed(fromAsset, toAsset string, reason string) {
	m.metricsCollector.IncrementCounter("crosspair_orders_failed_total", map[string]string{
		"from_asset": fromAsset,
		"to_asset":   toAsset,
		"reason":     reason,
	})
}

func (m *RealMetricsCollector) RecordTradeVolume(pair string, volume float64) {
	m.metricsCollector.RecordHistogram("crosspair_trade_volume", volume, map[string]string{
		"pair": pair,
	})
}

func (m *RealMetricsCollector) RecordRateCalculation(pair string, duration time.Duration, confidence float64) {
	m.metricsCollector.RecordTimer("crosspair_rate_calculation_duration", duration, map[string]string{
		"pair": pair,
	})
	m.metricsCollector.RecordGauge("crosspair_rate_confidence", confidence, map[string]string{
		"pair": pair,
	})
}

func (m *RealMetricsCollector) RecordActiveConnections(count int) {
	m.metricsCollector.RecordGauge("crosspair_websocket_connections", float64(count), nil)
}

func (m *RealMetricsCollector) RecordOrderExecuted(fromAsset, toAsset string, executionTime time.Duration) {
	m.metricsCollector.IncrementCounter("crosspair_orders_executed_total", map[string]string{
		"from_asset": fromAsset,
		"to_asset":   toAsset,
	})
	m.metricsCollector.RecordTimer("crosspair_order_execution_duration", executionTime, map[string]string{
		"from_asset": fromAsset,
		"to_asset":   toAsset,
	})
}

func (m *RealMetricsCollector) RecordSlippage(fromAsset, toAsset string, slippage decimal.Decimal) {
	slippageFloat, _ := slippage.Float64()
	m.metricsCollector.RecordHistogram("crosspair_slippage", slippageFloat, map[string]string{
		"from_asset": fromAsset,
		"to_asset":   toAsset,
	})
}

func (m *RealMetricsCollector) RecordVolume(fromAsset, toAsset string, volume decimal.Decimal) {
	volumeFloat, _ := volume.Float64()
	m.metricsCollector.RecordHistogram("crosspair_volume", volumeFloat, map[string]string{
		"from_asset": fromAsset,
		"to_asset":   toAsset,
	})
}

// RealAtomicExecutor implements AtomicExecutor using the platform's coordination service
type RealAtomicExecutor struct {
	balanceService BalanceServiceInterface
	spotEngine     SpotEngineInterface
	coordService   CoordinationServiceInterface
}

func NewRealAtomicExecutor(
	balanceService BalanceServiceInterface,
	spotEngine SpotEngineInterface,
	coordService CoordinationServiceInterface,
) *RealAtomicExecutor {
	return &RealAtomicExecutor{
		balanceService: balanceService,
		spotEngine:     spotEngine,
		coordService:   coordService,
	}
}

func (e *RealAtomicExecutor) ExecuteAtomic(ctx context.Context, execution *ExecutionPlan) (*ExecutionResult, error) {
	// Acquire distributed lock for atomic execution
	lockKey := fmt.Sprintf("crosspair_execution_%s", execution.OrderID)
	lock, err := e.coordService.AcquireLock(ctx, lockKey, 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire execution lock: %w", err)
	}
	defer e.coordService.ReleaseLock(ctx, lock)
	// Execute the two legs atomically
	leg1Order := &SpotOrder{
		ID:       uuid.New(),
		UserID:   execution.UserID,
		Pair:     execution.Leg1.Pair,
		Side:     execution.Leg1.Side,
		Type:     "market", // Force market orders for atomic execution
		Quantity: execution.Leg1.Quantity.InexactFloat64(),
	}

	leg2Order := &SpotOrder{
		ID:       uuid.New(),
		UserID:   execution.UserID,
		Pair:     execution.Leg2.Pair,
		Side:     execution.Leg2.Side,
		Type:     "market",
		Quantity: execution.Leg2.Quantity.InexactFloat64(),
	}

	// Execute leg 1
	executedLeg1, err := e.spotEngine.PlaceOrder(ctx, leg1Order)
	if err != nil {
		return nil, fmt.Errorf("failed to execute leg 1: %w", err)
	}

	// Execute leg 2
	executedLeg2, err := e.spotEngine.PlaceOrder(ctx, leg2Order)
	if err != nil {
		// Rollback leg 1 if possible
		e.spotEngine.CancelOrder(ctx, executedLeg1.ID)
		return nil, fmt.Errorf("failed to execute leg 2: %w", err)
	}

	result := &ExecutionResult{
		OrderID:     execution.OrderID,
		Success:     true,
		Leg1TradeID: executedLeg1.ID,
		Leg2TradeID: executedLeg2.ID,
		ExecutedAt:  time.Now(),
		TotalFee:    execution.EstimatedFee, // TODO: Calculate actual fee
	}

	return result, nil
}

func (e *RealAtomicExecutor) EstimateExecution(ctx context.Context, plan *ExecutionPlan) (*ExecutionEstimate, error) {
	// Get current orderbooks to estimate execution
	leg1Orderbook, err := e.spotEngine.GetOrderbook(ctx, plan.Leg1.Pair)
	if err != nil {
		return nil, fmt.Errorf("failed to get leg1 orderbook: %w", err)
	}

	leg2Orderbook, err := e.spotEngine.GetOrderbook(ctx, plan.Leg2.Pair)
	if err != nil {
		return nil, fmt.Errorf("failed to get leg2 orderbook: %w", err)
	}
	// Calculate estimated prices and slippage
	leg1Qty, _ := plan.Leg1.Quantity.Float64()
	leg2Qty, _ := plan.Leg2.Quantity.Float64()
	leg1Price, leg1Slippage := e.estimatePrice(leg1Orderbook, plan.Leg1.Side, leg1Qty)
	leg2Price, leg2Slippage := e.estimatePrice(leg2Orderbook, plan.Leg2.Side, leg2Qty)

	estimate := &ExecutionEstimate{
		EstimatedPrice:    decimal.NewFromFloat((leg1Price + leg2Price) / 2), // Simplified
		EstimatedSlippage: decimal.NewFromFloat((leg1Slippage + leg2Slippage) / 2),
		EstimatedFee:      plan.EstimatedFee,
		Confidence:        decimal.NewFromFloat(0.9), // TODO: Calculate based on orderbook depth
		CanExecute:        true,
	}

	return estimate, nil
}

func (e *RealAtomicExecutor) estimatePrice(orderbook *Orderbook, side string, quantity float64) (float64, float64) {
	var levels []OrderLevel
	if side == "buy" {
		levels = orderbook.Asks
	} else {
		levels = orderbook.Bids
	}

	if len(levels) == 0 {
		return 0, 1.0 // Maximum slippage if no liquidity
	}

	totalQuantity := 0.0
	weightedPrice := 0.0
	bestPrice := levels[0].Price

	for _, level := range levels {
		if totalQuantity >= quantity {
			break
		}

		fillQuantity := quantity - totalQuantity
		if fillQuantity > level.Quantity {
			fillQuantity = level.Quantity
		}

		weightedPrice += level.Price * fillQuantity
		totalQuantity += fillQuantity
	}

	if totalQuantity == 0 {
		return 0, 1.0
	}

	avgPrice := weightedPrice / totalQuantity
	slippage := abs(avgPrice-bestPrice) / bestPrice

	return avgPrice, slippage
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// CrossPairIntegrationAdapter adapts the cross-pair engine to use real services
type CrossPairIntegrationAdapter struct {
	services *ServiceIntegration
}

func NewCrossPairIntegrationAdapter(services *ServiceIntegration) *CrossPairIntegrationAdapter {
	return &CrossPairIntegrationAdapter{services: services}
}

// GetRealOrderbookProvider returns an orderbook provider using the spot engine
func (a *CrossPairIntegrationAdapter) GetRealOrderbookProvider() OrderbookProvider {
	return NewRealOrderbookProvider(a.services.spotEngine)
}

// GetRealEventPublisher returns an event publisher using the platform's event system
func (a *CrossPairIntegrationAdapter) GetRealEventPublisher() EventPublisher {
	return NewRealEventPublisher(a.services.eventPublisher)
}

// GetRealMetricsCollector returns a metrics collector using the platform's metrics system
func (a *CrossPairIntegrationAdapter) GetRealMetricsCollector() MetricsCollector {
	return NewRealMetricsCollector(a.services.metricsCollector)
}

// GetRealAtomicExecutor returns an atomic executor using the platform's coordination service
func (a *CrossPairIntegrationAdapter) GetRealAtomicExecutor() AtomicExecutor {
	return NewRealAtomicExecutor(
		a.services.balanceService,
		a.services.spotEngine,
		a.services.coordService,
	)
}

// GetRealStorage returns a storage implementation using the configured database
func (a *CrossPairIntegrationAdapter) GetRealStorage(config *StorageConfig) (Storage, error) {
	switch config.Type {
	case "postgres":
		// TODO: Create PostgreSQL connection and return PostgreSQLStorage
		return nil, fmt.Errorf("PostgreSQL storage not yet implemented")
	case "mysql":
		// TODO: Create MySQL connection and return MySQLStorage
		return nil, fmt.Errorf("MySQL storage not yet implemented")
	case "memory":
		return NewInMemoryStorage(), nil
	default:
		return nil, fmt.Errorf("unsupported storage type: %s", config.Type)
	}
}
