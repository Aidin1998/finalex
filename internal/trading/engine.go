package trading

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// OrderEngine handles order processing and matching
type OrderEngine struct {
	logger    *zap.Logger
	db        *gorm.DB
	orderbook map[string]*MarketOrderbook
	mu        sync.RWMutex
	running   bool
}

// MarketOrderbook represents the order book for a specific market
type MarketOrderbook struct {
	MarketID string
	Bids     []*Order
	Asks     []*Order
	mu       sync.RWMutex
}

// NewOrderEngine creates a new order engine
func NewOrderEngine(logger *zap.Logger, db *gorm.DB) (*OrderEngine, error) {
	return &OrderEngine{
		logger:    logger,
		db:        db,
		orderbook: make(map[string]*MarketOrderbook),
	}, nil
}

// ProcessOrder processes an order through the matching engine
func (e *OrderEngine) ProcessOrder(ctx context.Context, order *Order) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Get or create market orderbook
	book, exists := e.orderbook[order.MarketID]
	if !exists {
		book = &MarketOrderbook{
			MarketID: order.MarketID,
			Bids:     []*Order{},
			Asks:     []*Order{},
		}
		e.orderbook[order.MarketID] = book
	}

	// Process the order
	return e.matchOrder(ctx, book, order)
}

// matchOrder attempts to match an order against the order book
func (e *OrderEngine) matchOrder(ctx context.Context, book *MarketOrderbook, order *Order) error {
	book.mu.Lock()
	defer book.mu.Unlock()

	if order.Type == "market" {
		return e.processMarketOrder(ctx, book, order)
	} else if order.Type == "limit" {
		return e.processLimitOrder(ctx, book, order)
	}

	return fmt.Errorf("unsupported order type: %s", order.Type)
}

// processMarketOrder processes a market order
func (e *OrderEngine) processMarketOrder(ctx context.Context, book *MarketOrderbook, order *Order) error {
	if order.Side == "buy" {
		// Match against asks
		for i, ask := range book.Asks {
			if order.FilledAmount.Equal(order.Amount) {
				break
			}

			fillAmount := order.Amount.Sub(order.FilledAmount)
			if ask.Amount.Sub(ask.FilledAmount).LessThan(fillAmount) {
				fillAmount = ask.Amount.Sub(ask.FilledAmount)
			}

			// Execute trade
			if err := e.executeTrade(ctx, order, ask, ask.Price, fillAmount); err != nil {
				return err
			}

			// Update fill amounts
			order.FilledAmount = order.FilledAmount.Add(fillAmount)
			ask.FilledAmount = ask.FilledAmount.Add(fillAmount)

			// Remove fully filled ask
			if ask.FilledAmount.Equal(ask.Amount) {
				book.Asks = append(book.Asks[:i], book.Asks[i+1:]...)
			}
		}
	} else {
		// Match against bids
		for i, bid := range book.Bids {
			if order.FilledAmount.Equal(order.Amount) {
				break
			}

			fillAmount := order.Amount.Sub(order.FilledAmount)
			if bid.Amount.Sub(bid.FilledAmount).LessThan(fillAmount) {
				fillAmount = bid.Amount.Sub(bid.FilledAmount)
			}

			// Execute trade
			if err := e.executeTrade(ctx, bid, order, bid.Price, fillAmount); err != nil {
				return err
			}

			// Update fill amounts
			order.FilledAmount = order.FilledAmount.Add(fillAmount)
			bid.FilledAmount = bid.FilledAmount.Add(fillAmount)

			// Remove fully filled bid
			if bid.FilledAmount.Equal(bid.Amount) {
				book.Bids = append(book.Bids[:i], book.Bids[i+1:]...)
			}
		}
	}

	// Update order status
	if order.FilledAmount.Equal(order.Amount) {
		order.Status = "filled"
	} else if order.FilledAmount.GreaterThan(order.Amount.Mul(decimal.NewFromFloat(0.0))) {
		order.Status = "partial"
	}

	return nil
}

// processLimitOrder processes a limit order
func (e *OrderEngine) processLimitOrder(ctx context.Context, book *MarketOrderbook, order *Order) error {
	// First try to match against existing orders
	if order.Side == "buy" {
		// Try to match against asks at or below our price
		for i := len(book.Asks) - 1; i >= 0; i-- {
			ask := book.Asks[i]
			if ask.Price.GreaterThan(order.Price) {
				continue
			}

			if order.FilledAmount.Equal(order.Amount) {
				break
			}

			fillAmount := order.Amount.Sub(order.FilledAmount)
			if ask.Amount.Sub(ask.FilledAmount).LessThan(fillAmount) {
				fillAmount = ask.Amount.Sub(ask.FilledAmount)
			}

			// Execute trade
			if err := e.executeTrade(ctx, order, ask, ask.Price, fillAmount); err != nil {
				return err
			}

			// Update fill amounts
			order.FilledAmount = order.FilledAmount.Add(fillAmount)
			ask.FilledAmount = ask.FilledAmount.Add(fillAmount)

			// Remove fully filled ask
			if ask.FilledAmount.Equal(ask.Amount) {
				book.Asks = append(book.Asks[:i], book.Asks[i+1:]...)
			}
		}

		// Add remaining amount to bid book if not fully filled
		if !order.FilledAmount.Equal(order.Amount) {
			book.Bids = append(book.Bids, order)
			e.sortBids(book.Bids)
		}
	} else {
		// Try to match against bids at or above our price
		for i := len(book.Bids) - 1; i >= 0; i-- {
			bid := book.Bids[i]
			if bid.Price.LessThan(order.Price) {
				continue
			}

			if order.FilledAmount.Equal(order.Amount) {
				break
			}

			fillAmount := order.Amount.Sub(order.FilledAmount)
			if bid.Amount.Sub(bid.FilledAmount).LessThan(fillAmount) {
				fillAmount = bid.Amount.Sub(bid.FilledAmount)
			}

			// Execute trade
			if err := e.executeTrade(ctx, bid, order, bid.Price, fillAmount); err != nil {
				return err
			}

			// Update fill amounts
			order.FilledAmount = order.FilledAmount.Add(fillAmount)
			bid.FilledAmount = bid.FilledAmount.Add(fillAmount)

			// Remove fully filled bid
			if bid.FilledAmount.Equal(bid.Amount) {
				book.Bids = append(book.Bids[:i], book.Bids[i+1:]...)
			}
		}

		// Add remaining amount to ask book if not fully filled
		if !order.FilledAmount.Equal(order.Amount) {
			book.Asks = append(book.Asks, order)
			e.sortAsks(book.Asks)
		}
	}

	// Update order status
	if order.FilledAmount.Equal(order.Amount) {
		order.Status = "filled"
	} else if order.FilledAmount.GreaterThan(decimal.Zero) {
		order.Status = "partial"
	}

	return nil
}

// executeTrade executes a trade between two orders
func (e *OrderEngine) executeTrade(ctx context.Context, buyOrder, sellOrder *Order, price, amount decimal.Decimal) error {
	tradeID := uuid.New().String()

	trade := &Trade{
		ID:          tradeID,
		MarketID:    buyOrder.MarketID,
		BuyOrderID:  buyOrder.ID,
		SellOrderID: sellOrder.ID,
		BuyerID:     buyOrder.UserID,
		SellerID:    sellOrder.UserID,
		Price:       price,
		Amount:      amount,
		Fee:         amount.Mul(decimal.NewFromFloat(0.001)), // 0.1% fee
		CreatedAt:   time.Now(),
	}

	e.logger.Info("Trade executed",
		zap.String("trade_id", tradeID),
		zap.String("market", buyOrder.MarketID),
		zap.String("buyer", buyOrder.UserID),
		zap.String("seller", sellOrder.UserID),
		zap.String("price", price.String()),
		zap.String("amount", amount.String()))

	return nil
}

// sortBids sorts bids by price (highest first)
func (e *OrderEngine) sortBids(bids []*Order) {
	// Simple bubble sort - in production use a more efficient algorithm
	for i := 0; i < len(bids)-1; i++ {
		for j := 0; j < len(bids)-i-1; j++ {
			if bids[j].Price.LessThan(bids[j+1].Price) {
				bids[j], bids[j+1] = bids[j+1], bids[j]
			}
		}
	}
}

// sortAsks sorts asks by price (lowest first)
func (e *OrderEngine) sortAsks(asks []*Order) {
	// Simple bubble sort - in production use a more efficient algorithm
	for i := 0; i < len(asks)-1; i++ {
		for j := 0; j < len(asks)-i-1; j++ {
			if asks[j].Price.GreaterThan(asks[j+1].Price) {
				asks[j], asks[j+1] = asks[j+1], asks[j]
			}
		}
	}
}

// Start starts the order engine
func (e *OrderEngine) Start() error {
	e.running = true
	e.logger.Info("Order engine started")
	return nil
}

// Stop stops the order engine
func (e *OrderEngine) Stop() error {
	e.running = false
	e.logger.Info("Order engine stopped")
	return nil
}
