package marketdata

// Delta encoding utilities for order book updates

// OrderBookSnapshot and Delta types would be defined elsewhere or imported

type OrderBookSnapshot struct {
	Symbol string
	Bids   []LevelDelta
	Asks   []LevelDelta
}

type OrderBookDelta struct {
	Symbol string
	Bids   []LevelDelta
	Asks   []LevelDelta
}

type LevelDelta struct {
	Price  float64
	Volume float64
}

// ComputeDelta returns the difference between two order book snapshots
func ComputeDelta(prev, curr *OrderBookSnapshot) *OrderBookDelta {
	delta := &OrderBookDelta{Symbol: curr.Symbol}
	// Map price to volume for quick lookup
	prevBids := make(map[float64]float64)
	for _, lvl := range prev.Bids {
		prevBids[lvl.Price] = lvl.Volume
	}
	currBids := make(map[float64]float64)
	for _, lvl := range curr.Bids {
		currBids[lvl.Price] = lvl.Volume
	}
	// Bids: changed or removed
	for price, currVol := range currBids {
		if prevVol, ok := prevBids[price]; !ok || prevVol != currVol {
			delta.Bids = append(delta.Bids, LevelDelta{Price: price, Volume: currVol})
		}
	}
	for price := range prevBids {
		if _, ok := currBids[price]; !ok {
			delta.Bids = append(delta.Bids, LevelDelta{Price: price, Volume: 0}) // 0 = remove
		}
	}
	// Asks: changed or removed
	prevAsks := make(map[float64]float64)
	for _, lvl := range prev.Asks {
		prevAsks[lvl.Price] = lvl.Volume
	}
	currAsks := make(map[float64]float64)
	for _, lvl := range curr.Asks {
		currAsks[lvl.Price] = lvl.Volume
	}
	for price, currVol := range currAsks {
		if prevVol, ok := prevAsks[price]; !ok || prevVol != currVol {
			delta.Asks = append(delta.Asks, LevelDelta{Price: price, Volume: currVol})
		}
	}
	for price := range prevAsks {
		if _, ok := currAsks[price]; !ok {
			delta.Asks = append(delta.Asks, LevelDelta{Price: price, Volume: 0})
		}
	}
	return delta
}
