package orderbook

import (
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/google/uuid"
)

// OrderBook represents an order book for a trading pair
type OrderBook struct {
	Symbol     string
	BuyOrders  *OrderTree
	SellOrders *OrderTree
	mutex      sync.RWMutex
	trades     []*models.Trade

	// Advanced: store conditional/trigger orders (stop, stop-limit, trailing, OCO)
	triggerOrders map[string]*Order   // orderID -> *Order
	ocoGroups     map[string][]*Order // OCOGroupID -> []*Order
}

// NewOrderTree creates a new order tree
func NewOrderTree() *OrderTree {
	ot := &OrderTree{}
	ot.PricePoints.Store([]float64{})
	return ot
}

// NewOrderBook creates a new order book
func NewOrderBook(symbol string) *OrderBook {
	return &OrderBook{
		Symbol:        symbol,
		BuyOrders:     NewOrderTree(),
		SellOrders:    NewOrderTree(),
		trades:        make([]*models.Trade, 0),
		triggerOrders: make(map[string]*Order),
		ocoGroups:     make(map[string][]*Order),
	}
}

// OrderList represents a list of orders at a price point
type OrderList struct {
	Price  float64
	Orders []*Order
	Volume float64
	mux    sync.Mutex
}

// OrderTree represents a tree of orders
type OrderTree struct {
	PriceMap    sync.Map     // price -> *OrderList
	PricePoints atomic.Value // []float64
	OrderMap    sync.Map     // orderID -> *Order
}

// Order represents an order in the order book
type Order struct {
	ID        string
	UserID    string
	Symbol    string
	Side      string
	Price     float64
	Quantity  float64
	Timestamp time.Time

	// Advanced order fields
	OrderType      string  // limit, market, stop, stop-limit, trailing-stop, OCO, etc.
	TriggerPrice   float64 // for stop/triggered orders
	TrailingOffset float64 // for trailing stops
	OCOGroupID     string  // for OCO linkage
	ReduceOnly     bool    // reduce-only flag
	PostOnly       bool    // post-only flag
	ParentOrderID  string  // for OCO/linked orders
	Status         string  // pending, active, triggered, canceled, filled, etc.
}

// AddOrder adds an order to the order book
func (ob *OrderBook) AddOrder(order *Order) ([]*models.Trade, error) {
	ob.mutex.Lock()
	defer ob.mutex.Unlock()

	// Validate order
	if order.Symbol != ob.Symbol {
		return nil, fmt.Errorf("invalid symbol: %s", order.Symbol)
	}
	if order.Side != "buy" && order.Side != "sell" {
		return nil, fmt.Errorf("invalid side: %s", order.Side)
	}
	if order.Quantity <= 0 {
		return nil, fmt.Errorf("invalid quantity: %f", order.Quantity)
	}

	// Advanced: handle conditional/trigger orders
	switch order.OrderType {
	case "stop", "stop-limit", "trailing-stop":
		order.Status = "pending"
		ob.triggerOrders[order.ID] = order
		if order.OCOGroupID != "" {
			ob.ocoGroups[order.OCOGroupID] = append(ob.ocoGroups[order.OCOGroupID], order)
		}
		return nil, nil // not matched until triggered
	case "oco":
		// OCO is a logical group, each leg should be a stop/limit order with OCOGroupID set
		order.Status = "pending"
		ob.triggerOrders[order.ID] = order
		if order.OCOGroupID != "" {
			ob.ocoGroups[order.OCOGroupID] = append(ob.ocoGroups[order.OCOGroupID], order)
		}
		return nil, nil
	}

	// Standard order matching
	var trades []*models.Trade
	var err error
	if order.Side == "buy" {
		trades, err = ob.matchBuyOrder(order)
	} else {
		trades, err = ob.matchSellOrder(order)
	}
	if err != nil {
		return nil, err
	}

	// Add remaining order to order book
	if order.Quantity > 0 {
		if order.Side == "buy" {
			ob.BuyOrders.AddOrder(order)
		} else {
			ob.SellOrders.AddOrder(order)
		}
	}

	// Add trades to order book
	ob.trades = append(ob.trades, trades...)

	// After each trade, check triggers
	ob.checkAndTriggerOrders()

	return trades, nil
}

// checkAndTriggerOrders checks and triggers eligible stop/trailing/OCO orders
func (ob *OrderBook) checkAndTriggerOrders() {
	// For each trigger order, check if trigger condition is met
	for id, order := range ob.triggerOrders {
		triggered := false
		if order.OrderType == "stop" || order.OrderType == "stop-limit" {
			if order.Side == "buy" {
				// Trigger if best ask >= trigger price
				if len(ob.SellOrders.PricePoints.Load().([]float64)) > 0 {
					bestAsk := ob.SellOrders.PricePoints.Load().([]float64)[0]
					if bestAsk >= order.TriggerPrice {
						triggered = true
					}
				}
			} else {
				// Trigger if best bid <= trigger price
				if len(ob.BuyOrders.PricePoints.Load().([]float64)) > 0 {
					bestBid := ob.BuyOrders.PricePoints.Load().([]float64)[0]
					if bestBid <= order.TriggerPrice {
						triggered = true
					}
				}
			}
		}
		// TODO: trailing-stop logic (update trigger dynamically)
		if triggered {
			order.Status = "triggered"
			delete(ob.triggerOrders, id)
			// Place as market or limit order
			if order.OrderType == "stop" {
				order.OrderType = "market"
				ob.AddOrder(order)
			} else if order.OrderType == "stop-limit" {
				order.OrderType = "limit"
				ob.AddOrder(order)
			}
			// OCO: cancel sibling(s)
			if order.OCOGroupID != "" {
				for _, sibling := range ob.ocoGroups[order.OCOGroupID] {
					if sibling.ID != order.ID {
						sibling.Status = "canceled"
						delete(ob.triggerOrders, sibling.ID)
					}
				}
				delete(ob.ocoGroups, order.OCOGroupID)
			}
		}
	}
}

// CancelOrder cancels an order in the order book
func (ob *OrderBook) CancelOrder(orderID string) error {
	ob.mutex.Lock()
	defer ob.mutex.Unlock()

	// Check if order exists in buy orders
	if v, exists := ob.BuyOrders.OrderMap.Load(orderID); exists {
		order := v.(*Order)
		return ob.BuyOrders.RemoveOrder(order)
	}

	// Check if order exists in sell orders
	if v, exists := ob.SellOrders.OrderMap.Load(orderID); exists {
		order := v.(*Order)
		return ob.SellOrders.RemoveOrder(order)
	}

	return fmt.Errorf("order not found: %s", orderID)
}

// GetOrder gets an order from the order book
func (ob *OrderBook) GetOrder(orderID string) (*Order, error) {
	ob.mutex.RLock()
	defer ob.mutex.RUnlock()

	// Check if order exists in buy orders
	if v, exists := ob.BuyOrders.OrderMap.Load(orderID); exists {
		return v.(*Order), nil
	}

	// Check if order exists in sell orders
	if v, exists := ob.SellOrders.OrderMap.Load(orderID); exists {
		return v.(*Order), nil
	}

	return nil, fmt.Errorf("order not found: %s", orderID)
}

// GetOrderBook gets the order book
func (ob *OrderBook) GetOrderBook(depth int) (*models.OrderBookSnapshot, error) {
	ob.mutex.RLock()
	defer ob.mutex.RUnlock()

	// Get buy orders
	buyOrders := make([]models.OrderBookLevel, 0, depth)
	for i, price := range ob.BuyOrders.PricePoints.Load().([]float64) {
		if i >= depth {
			break
		}
		if v, ok := ob.BuyOrders.PriceMap.Load(price); ok {
			orderList := v.(*OrderList)
			buyOrders = append(buyOrders, models.OrderBookLevel{
				Price:  price,
				Volume: orderList.Volume,
			})
		}
	}

	// Get sell orders
	sellOrders := make([]models.OrderBookLevel, 0, depth)
	for i, price := range ob.SellOrders.PricePoints.Load().([]float64) {
		if i >= depth {
			break
		}
		if v, ok := ob.SellOrders.PriceMap.Load(price); ok {
			orderList := v.(*OrderList)
			sellOrders = append(sellOrders, models.OrderBookLevel{
				Price:  price,
				Volume: orderList.Volume,
			})
		}
	}

	// Create order book snapshot
	snapshot := &models.OrderBookSnapshot{
		Symbol:     ob.Symbol,
		Bids:       buyOrders,
		Asks:       sellOrders,
		UpdateTime: time.Now(),
	}

	return snapshot, nil
}

// GetTrades gets trades from the order book
func (ob *OrderBook) GetTrades(limit int) []*models.Trade {
	ob.mutex.RLock()
	defer ob.mutex.RUnlock()

	// Get trades
	trades := make([]*models.Trade, 0, limit)
	for i := len(ob.trades) - 1; i >= 0 && len(trades) < limit; i-- {
		trades = append(trades, ob.trades[i])
	}

	return trades
}

// matchBuyOrder matches a buy order with sell orders
func (ob *OrderBook) matchBuyOrder(order *Order) ([]*models.Trade, error) {
	trades := make([]*models.Trade, 0)

	// Match order with sell orders
	for len(ob.SellOrders.PricePoints.Load().([]float64)) > 0 {
		// Get best sell price
		bestPrice := ob.SellOrders.PricePoints.Load().([]float64)[0]
		if bestPrice > order.Price {
			break
		}

		// Get sell orders at best price
		v, _ := ob.SellOrders.PriceMap.Load(bestPrice)
		orderList := v.(*OrderList)
		for len(orderList.Orders) > 0 {
			// Get first sell order
			sellOrder := orderList.Orders[0]

			// Calculate trade quantity
			quantity := order.Quantity
			if sellOrder.Quantity < quantity {
				quantity = sellOrder.Quantity
			}

			// Create trade
			trade := &models.Trade{
				ID:             uuid.New(),
				OrderID:        uuid.MustParse(order.ID),
				CounterOrderID: uuid.MustParse(sellOrder.ID),
				UserID:         uuid.MustParse(order.UserID),
				CounterUserID:  uuid.MustParse(sellOrder.UserID),
				Symbol:         order.Symbol,
				Side:           "buy",
				Price:          sellOrder.Price,
				Quantity:       quantity,
				Fee:            sellOrder.Price * quantity * 0.001,
				FeeCurrency:    order.Symbol[:3],
				CreatedAt:      time.Now(),
			}
			trades = append(trades, trade)

			// Update order quantities
			order.Quantity -= quantity
			sellOrder.Quantity -= quantity
			orderList.Volume -= quantity

			// Remove sell order if fully matched
			if sellOrder.Quantity == 0 {
				ob.SellOrders.RemoveOrder(sellOrder)
			}

			// Break if buy order is fully matched
			if order.Quantity == 0 {
				break
			}
		}

		// Remove price point if no more orders
		if len(orderList.Orders) == 0 {
			ob.SellOrders.PriceMap.Delete(bestPrice)
			// remove from PricePoints atomically
			for {
				old := ob.SellOrders.PricePoints.Load().([]float64)
				new := make([]float64, 0, len(old)-1)
				for _, p := range old {
					if p != bestPrice {
						new = append(new, p)
					}
				}
				if atomic.CompareAndSwapPointer(
					(*unsafe.Pointer)(unsafe.Pointer(&ob.SellOrders.PricePoints)),
					unsafe.Pointer(&old),
					unsafe.Pointer(&new),
				) {
					break
				}
			}
		}

		// Break if buy order is fully matched
		if order.Quantity == 0 {
			break
		}
	}

	return trades, nil
}

// matchSellOrder matches a sell order with buy orders
func (ob *OrderBook) matchSellOrder(order *Order) ([]*models.Trade, error) {
	trades := make([]*models.Trade, 0)

	// Match order with buy orders
	for len(ob.BuyOrders.PricePoints.Load().([]float64)) > 0 {
		// Get best buy price
		bestPrice := ob.BuyOrders.PricePoints.Load().([]float64)[0]
		if bestPrice < order.Price {
			break
		}

		// Get buy orders at best price
		v, _ := ob.BuyOrders.PriceMap.Load(bestPrice)
		orderList := v.(*OrderList)
		for len(orderList.Orders) > 0 {
			// Get first buy order
			buyOrder := orderList.Orders[0]

			// Calculate trade quantity
			quantity := order.Quantity
			if buyOrder.Quantity < quantity {
				quantity = buyOrder.Quantity
			}

			// Create trade
			trade := &models.Trade{
				ID:             uuid.New(),
				OrderID:        uuid.MustParse(order.ID),
				CounterOrderID: uuid.MustParse(buyOrder.ID),
				UserID:         uuid.MustParse(order.UserID),
				CounterUserID:  uuid.MustParse(buyOrder.UserID),
				Symbol:         order.Symbol,
				Side:           "sell",
				Price:          buyOrder.Price,
				Quantity:       quantity,
				Fee:            buyOrder.Price * quantity * 0.001,
				FeeCurrency:    order.Symbol[:3],
				CreatedAt:      time.Now(),
			}
			trades = append(trades, trade)

			// Update order quantities
			order.Quantity -= quantity
			buyOrder.Quantity -= quantity
			orderList.Volume -= quantity

			// Remove buy order if fully matched
			if buyOrder.Quantity == 0 {
				ob.BuyOrders.RemoveOrder(buyOrder)
			}

			// Break if sell order is fully matched
			if order.Quantity == 0 {
				break
			}
		}

		// Remove price point if no more orders
		if len(orderList.Orders) == 0 {
			ob.BuyOrders.PriceMap.Delete(bestPrice)
			// remove from PricePoints atomically
			for {
				old := ob.BuyOrders.PricePoints.Load().([]float64)
				new := make([]float64, 0, len(old)-1)
				for _, p := range old {
					if p != bestPrice {
						new = append(new, p)
					}
				}
				if atomic.CompareAndSwapPointer(
					(*unsafe.Pointer)(unsafe.Pointer(&ob.BuyOrders.PricePoints)),
					unsafe.Pointer(&old),
					unsafe.Pointer(&new),
				) {
					break
				}
			}
		}

		// Break if sell order is fully matched
		if order.Quantity == 0 {
			break
		}
	}

	return trades, nil
}

// AddOrder adds an order to the order tree
func (ot *OrderTree) AddOrder(order *Order) {
	// Get or create OrderList
	v, loaded := ot.PriceMap.LoadOrStore(order.Price, &OrderList{Price: order.Price})
	ol := v.(*OrderList)
	if !loaded {
		// update price points
		pts := ot.PricePoints.Load().([]float64)
		// append and sort unique
		next := append(pts, order.Price)
		sort.Float64s(next)
		ot.PricePoints.Store(uniqueFloats(next))
	}
	// append order
	ol.mux.Lock()
	ol.Orders = append(ol.Orders, order)
	ol.Volume += order.Quantity
	ol.mux.Unlock()
	// store in OrderMap
	ot.OrderMap.Store(order.ID, order)
}

// RemoveOrder removes an order from the order tree
func (ot *OrderTree) RemoveOrder(order *Order) error {
	v, ok := ot.PriceMap.Load(order.Price)
	if !ok {
		return fmt.Errorf("price level not found: %f", order.Price)
	}
	ol := v.(*OrderList)
	// remove from list
	ol.mux.Lock()
	idx := -1
	for i, o := range ol.Orders {
		if o.ID == order.ID {
			idx = i
			break
		}
	}
	if idx < 0 {
		ol.mux.Unlock()
		return fmt.Errorf("order not found: %s", order.ID)
	}
	removedQty := ol.Orders[idx].Quantity
	ol.Orders = append(ol.Orders[:idx], ol.Orders[idx+1:]...)
	ol.Volume -= removedQty
	ol.mux.Unlock()
	// delete from OrderMap
	ot.OrderMap.Delete(order.ID)
	// if empty, remove price level
	ol.mux.Lock()
	empty := len(ol.Orders) == 0
	ol.mux.Unlock()
	if empty {
		ot.PriceMap.Delete(order.Price)
		pts := ot.PricePoints.Load().([]float64)
		next := removeFloat(pts, order.Price)
		ot.PricePoints.Store(next)
	}
	return nil
}

// GetOrder gets an order by ID
func (ot *OrderTree) GetOrder(orderID string) (*Order, error) {
	if v, ok := ot.OrderMap.Load(orderID); ok {
		return v.(*Order), nil
	}
	return nil, fmt.Errorf("order not found: %s", orderID)
}

// GetPricePoints returns the current price points
func (ot *OrderTree) GetPricePoints() []float64 {
	pts := ot.PricePoints.Load().([]float64)
	return append([]float64(nil), pts...)
}

// GetOrderMap returns all orders (for snapshot etc.)
func (ot *OrderTree) GetOrderMap() []*Order {
	orders := make([]*Order, 0)
	ot.OrderMap.Range(func(_, v interface{}) bool {
		orders = append(orders, v.(*Order))
		return true
	})
	return orders
}

// uniqueFloats removes duplicates from sorted slice
func uniqueFloats(in []float64) []float64 {
	if len(in) == 0 {
		return in
	}
	out := []float64{in[0]}
	for _, v := range in[1:] {
		if v != out[len(out)-1] {
			out = append(out, v)
		}
	}
	return out
}

// removeFloat removes a value from slice
func removeFloat(in []float64, val float64) []float64 {
	out := make([]float64, 0, len(in)-1)
	for _, v := range in {
		if v != val {
			out = append(out, v)
		}
	}
	return out
}
