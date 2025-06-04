// types.go: Core types for market data distribution
package distribution

import (
	"time"
)

// Update represents a market data update message
type Update struct {
	Symbol     string    // Trading pair symbol
	PriceLevel int       // Price level index
	Bid        float64   // Bid price
	Ask        float64   // Ask price
	Timestamp  time.Time // Update timestamp
}

// Snapshot represents a full order book snapshot
type Snapshot struct {
	Symbol    string      // Trading pair symbol
	Bids      [][]float64 // [[price, volume], ...]
	Asks      [][]float64 // [[price, volume], ...]
	Timestamp time.Time   // Snapshot timestamp
}

// Subscription defines parameters for a client subscription
type Subscription struct {
	ClientID    string        // Unique client identifier
	Symbol      string        // Trading pair symbol
	PriceLevels []int         // Specific price levels to subscribe
	Frequency   time.Duration // Minimum update interval
	Compression bool          // Enable delta compression
}

// TransportType defines the type of subscription transport
type TransportType string

const (
	TransportWebSocket TransportType = "websocket"
	TransportGRPC      TransportType = "grpc"
)
