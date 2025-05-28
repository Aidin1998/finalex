package trading

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/trading/engine"
	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/google/uuid"
)

func TestEngineOrderBook_Stress_Matching(t *testing.T) {
	const (
		orderCount  = 50000
		marketRatio = 0.4
		startPrice  = 100000.0
		priceRange  = 100.0
		symbol      = "BTCUSD"
	)

	// TODO: Provide real orderRepo, tradeRepo, config, eventJournal as needed
	var orderRepo interface{} = nil             // Replace with actual repo
	var tradeRepo interface{} = nil             // Replace with actual repo
	var config *engine.Config = nil             // Replace with actual config
	var eventJournal *engine.EventJournal = nil // Replace with actual journal if needed
	e := engine.NewMatchingEngine(orderRepo, tradeRepo, nil, config, eventJournal)
	_ = e.AddPair(symbol)
	// e.AddTradingPairForTest(symbol) // replaced with AddPair
	// e.Start() // If needed, call Start
	// defer e.Stop() // If needed, call Stop

	var wg sync.WaitGroup
	orders := make([]*models.Order, orderCount)
	var matched int64
	start := time.Now()

	rand.Seed(time.Now().UnixNano())
	for i := 0; i < orderCount; i++ {
		orderType := "limit"
		if rand.Float64() < marketRatio {
			orderType = "market"
		}
		price := startPrice + rand.Float64()*priceRange - priceRange/2
		if orderType == "market" {
			price = 0 // market order
		}
		qty := float64(rand.Intn(5) + 1) // 1-5 BTC
		side := "buy"
		if rand.Intn(2) == 0 {
			side = "sell"
		}
		order := &models.Order{
			ID:       uuid.New(),
			UserID:   uuid.New(),
			Symbol:   symbol,
			Side:     side,
			Type:     orderType,
			Price:    price,
			Quantity: qty,
			Status:   "new",
		}
		orders[i] = order
	}

	// Place orders concurrently
	for i := 0; i < orderCount; i++ {
		wg.Add(1)
		go func(order *models.Order) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			res, _, _, err := e.ProcessOrder(ctx, order, "stress")
			if err == nil && res.Status == "filled" {
				atomic.AddInt64(&matched, 1)
			}
		}(orders[i])
	}
	wg.Wait()
	dur := time.Since(start)
	matchRate := float64(matched) / dur.Seconds()

	fmt.Printf("\n--- Order Book Stress Test Results ---\n")
	fmt.Printf("Total Orders: %d\n", orderCount)
	fmt.Printf("Matched (filled) Orders: %d\n", matched)
	fmt.Printf("Match Rate: %.2f orders/sec\n", matchRate)
	fmt.Printf("Duration: %s\n", dur)

	// Print partial fill and book state
	ob := e.GetOrderBook(symbol)
	if ob != nil {
		buyLevels, buyVol := ob.BuyStats()
		sellLevels, sellVol := ob.SellStats()
		fmt.Printf("Buy Levels: %d, Total Buy Volume: %.2f\n", buyLevels, buyVol)
		fmt.Printf("Sell Levels: %d, Total Sell Volume: %.2f\n", sellLevels, sellVol)
	}
}
