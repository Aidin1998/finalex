// Symbol Interning for Zero-Copy Order Processing
// Maps string symbols to uint32 IDs and vice versa, with thread-safe registration

package orderbook

import (
	"sync"
)

type SymbolInterning struct {
	symbols map[string]uint32
	ids     []string
	mu      sync.RWMutex
}

func NewSymbolInterning() *SymbolInterning {
	return &SymbolInterning{
		symbols: make(map[string]uint32),
		ids:     make([]string, 0, 128),
	}
}

// RegisterSymbol returns the symbol ID, registering if new
func (si *SymbolInterning) RegisterSymbol(symbol string) uint32 {
	si.mu.RLock()
	id, ok := si.symbols[symbol]
	si.mu.RUnlock()
	if ok {
		return id
	}
	si.mu.Lock()
	defer si.mu.Unlock()
	id, ok = si.symbols[symbol]
	if ok {
		return id
	}
	id = uint32(len(si.ids))
	si.symbols[symbol] = id
	si.ids = append(si.ids, symbol)
	return id
}

// SymbolFromID returns the string symbol for a given ID
func (si *SymbolInterning) SymbolFromID(id uint32) string {
	si.mu.RLock()
	defer si.mu.RUnlock()
	if int(id) < len(si.ids) {
		return si.ids[id]
	}
	return ""
}
