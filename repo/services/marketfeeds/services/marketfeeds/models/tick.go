package models

import (
	"fmt"
	"time"
)

// Tick represents a normalized market update
type Tick struct {
	Symbol    string    `json:"symbol"`
	Price     float64   `json:"price"`
	Volume    float64   `json:"volume"`
	Sequence  int64     `json:"sequence"`
	Timestamp time.Time `json:"timestamp"`
}

// String serializes Tick to a string for publishing
func (t Tick) String() string {
	return fmt.Sprintf("%s|%f|%f|%d|%s", t.Symbol, t.Price, t.Volume, t.Sequence, t.Timestamp)
}

// Aggregate struct
