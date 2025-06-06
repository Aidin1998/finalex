// High-Performance Fill Pool for Zero-Copy Fill Structures
// Uses buffered channels for lock-free pooling, fallback to stack allocation

package orderbook

const fillPoolSize = 4096

type FillPool struct {
	fills chan *ZeroCopyFill
}

func NewFillPool() *FillPool {
	pool := &FillPool{
		fills: make(chan *ZeroCopyFill, fillPoolSize),
	}
	for i := 0; i < fillPoolSize; i++ {
		pool.fills <- &ZeroCopyFill{}
	}
	return pool
}

func (p *FillPool) GetFill() *ZeroCopyFill {
	select {
	case fill := <-p.fills:
		return fill
	default:
		return &ZeroCopyFill{}
	}
}

func (p *FillPool) PutFill(fill *ZeroCopyFill) {
	fill.Reset()
	select {
	case p.fills <- fill:
		// returned to pool
	default:
		// pool full, let GC reclaim
	}
}
