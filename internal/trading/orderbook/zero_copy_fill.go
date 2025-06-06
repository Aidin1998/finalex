// Zero-Copy, Cache-Line Optimized Fill Structure for Matching Engine
// All fields are packed and aligned for 64-byte cache line efficiency.

package orderbook

type ZeroCopyFill struct {
	OrderID  uint64   // 8 bytes
	MatchID  uint64   // 8 bytes
	UserID   uint32   // 4 bytes
	Symbol   uint32   // 4 bytes
	Price    uint64   // 8 bytes (fixed point)
	Quantity uint64   // 8 bytes (fixed point)
	Side     uint8    // 1 byte
	_        [27]byte // Padding to 64 bytes
}

func (f *ZeroCopyFill) Reset() {
	*f = ZeroCopyFill{}
}
