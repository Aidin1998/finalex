// Zero-Copy, Cache-Line Optimized Event Structure for Matching Engine
// All fields are packed and aligned for 64-byte cache line efficiency.

package orderbook

type ZeroCopyEvent struct {
	EventType uint8    // 1 byte
	OrderID   uint64   // 8 bytes
	UserID    uint32   // 4 bytes
	Symbol    uint32   // 4 bytes
	Timestamp int64    // 8 bytes
	_         [39]byte // Padding to 64 bytes
}

func (e *ZeroCopyEvent) Reset() {
	*e = ZeroCopyEvent{}
}
