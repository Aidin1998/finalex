package strategy

import (
	"fmt"
	"math/rand"
	"time"
)

// MarketMaker represents the market making strategy engine.
type MarketMaker struct {
	Spread          float64
	Inventory       float64
	MarketPrice     float64
	TradeExecution  func(price float64, quantity float64) error
}

// NewMarketMaker creates a new MarketMaker instance.
func NewMarketMaker(spread float64, inventory float64, tradeExecution func(price float64, quantity float64) error) *MarketMaker {
	return &MarketMaker{
		Spread:         spread,
		Inventory:      inventory,
		TradeExecution: tradeExecution,
	}
}

// CalculatePrice calculates the price based on market conditions and strategy.
func (mm *MarketMaker) CalculatePrice() float64 {
	mm.MarketPrice = mm.MarketPrice + (mm.Spread * (rand.Float64() - 0.5))
	return mm.MarketPrice
}

// ExecuteTrade executes a trade based on the calculated price and inventory.
func (mm *MarketMaker) ExecuteTrade(quantity float64) error {
	price := mm.CalculatePrice()
	if mm.Inventory < quantity {
		return fmt.Errorf("insufficient inventory to execute trade")
	}
	mm.Inventory -= quantity
	return mm.TradeExecution(price, quantity)
}

// UpdateInventory updates the inventory based on trade outcomes.
func (mm *MarketMaker) UpdateInventory(quantity float64) {
	mm.Inventory += quantity
}

// SimulateMarket simulates market conditions for testing purposes.
func (mm *MarketMaker) SimulateMarket() {
	for {
		time.Sleep(1 * time.Second)
		err := mm.ExecuteTrade(1) // Example trade quantity
		if err != nil {
			fmt.Println("Trade execution error:", err)
		} else {
			fmt.Println("Trade executed successfully at price:", mm.MarketPrice)
		}
	}
}