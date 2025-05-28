package orderbook

import (
	"fmt"
	"sync"
	"testing"

	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestConcurrentOrderBook_Concurrency(t *testing.T) {
	ob := NewOrderBook("BTCUSD") // Fix: use NewOrderBook for exported API
	wg := sync.WaitGroup{}
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			order := &models.Order{
				ID:       uuid.New(),
				UserID:   uuid.New(),
				Symbol:   "BTCUSD",
				Side:     "buy",
				Price:    10000 + float64(i%10),
				Quantity: 1,
			}
			_, err := ob.AddOrder(order)
			assert.NoError(t, err)
		}(i)
	}
	wg.Wait()
}

func TestConcurrentOrderBook_ConcurrentAddAndCancel(t *testing.T) {
	ob := NewOrderBook("BTCUSD") // Fix: use NewOrderBook for exported API
	wg := sync.WaitGroup{}
	orderCount := 10000
	cancelCount := 5000
	ids := make([]string, orderCount)

	// Add orders concurrently
	for i := 0; i < orderCount; i++ {
		wg.Add(1)
		id := fmt.Sprintf("order-%d", i)
		ids[i] = id
		go func(i int, id string) {
			defer wg.Done()
			order := &models.Order{
				ID:       uuid.New(),
				UserID:   uuid.New(),
				Symbol:   "BTCUSD",
				Side:     "buy",
				Price:    10000 + float64(i%10),
				Quantity: 1,
			}
			_, err := ob.AddOrder(order)
			assert.NoError(t, err)
		}(i, id)
	}
	wg.Wait()

	// Cancel half the orders concurrently
	wg = sync.WaitGroup{}
	for i := 0; i < cancelCount; i++ {
		wg.Add(1)
		id := ids[i]
		go func(id string) {
			defer wg.Done()
			err := ob.CancelOrder(id)
			assert.True(t, err == nil || err.Error() == fmt.Sprintf("order not found: %s", id))
		}(id)
	}
	wg.Wait()
}

func BenchmarkConcurrentOrderBook_AddOrder(b *testing.B) {
	ob := NewOrderBook("BTCUSD") // Fix: use NewOrderBook for exported API
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			order := &models.Order{
				ID:       uuid.New(),
				UserID:   uuid.New(),
				Symbol:   "BTCUSD",
				Side:     "buy",
				Price:    10000,
				Quantity: 1,
			}
			ob.AddOrder(order)
		}
	})
}
