// High-Performance Order Pool for Zero-Copy Order Structures
// Uses buffered channels for lock-free pooling, fallback to stack allocation

package orderbook

const orderPoolSize = 4096

// OrderPool manages pools for orders, fills, and events
// All pools are pre-allocated and use buffered channels for zero-allocation hot path

type OrderPool struct {
	orders chan *ZeroCopyOrder
}

func NewOrderPool() *OrderPool {
	pool := &OrderPool{
		orders: make(chan *ZeroCopyOrder, orderPoolSize),
	}
	// Pre-allocate pool
	for i := 0; i < orderPoolSize; i++ {
		pool.orders <- &ZeroCopyOrder{}
	}
	return pool
}

func (p *OrderPool) GetOrder() *ZeroCopyOrder {
	select {
	case order := <-p.orders:
		return order
	default:
		return &ZeroCopyOrder{} // fallback if pool exhausted
	}
}

func (p *OrderPool) PutOrder(order *ZeroCopyOrder) {
	order.Reset()
	select {
	case p.orders <- order:
		// returned to pool
	default:
		// pool full, let GC reclaim
	}
}
