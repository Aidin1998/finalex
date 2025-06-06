// Zero-Copy, Cache-Line Optimized Order Structure for Matching Engine
// All fields are packed and aligned for 64-byte cache line efficiency.
// Use only in the matching engine hot path and ring buffer pipeline.

package orderbook

import (
	"unsafe"
)

type ZeroCopyOrder struct {
	ID       uint64  // 8 bytes
	UserID   uint32  // 4 bytes
	Symbol   uint32  // 4 bytes (symbol ID, not string)
	Price    uint64  // 8 bytes (fixed point, e.g. 1e8 = 1.00000000)
	Quantity uint64  // 8 bytes (fixed point)
	Side     uint8   // 1 byte
	Type     uint8   // 1 byte
	Status   uint8   // 1 byte
	_        [5]byte // Padding to 64 bytes (cache line)
}

//go:nosplit
func (o *ZeroCopyOrder) Reset() {
	*o = ZeroCopyOrder{}
}

// Unsafe conversion from []byte to *ZeroCopyOrder (memory-mapped)
func BytesToZeroCopyOrder(b []byte) *ZeroCopyOrder {
	return (*ZeroCopyOrder)(unsafe.Pointer(&b[0]))
}

// Unsafe conversion from *ZeroCopyOrder to []byte
func ZeroCopyOrderToBytes(o *ZeroCopyOrder) []byte {
	return (*[64]byte)(unsafe.Pointer(o))[:]
}
