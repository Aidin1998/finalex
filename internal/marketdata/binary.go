package marketdata

import (
	"bytes"
	"encoding/binary"
)

// Binary serialization utilities for market data

func EncodeOrderBookDelta(delta *OrderBookDelta) ([]byte, error) {
	buf := new(bytes.Buffer)
	// Symbol length + symbol
	symBytes := []byte(delta.Symbol)
	if err := binary.Write(buf, binary.LittleEndian, uint16(len(symBytes))); err != nil {
		return nil, err
	}
	buf.Write(symBytes)
	// Bids
	if err := binary.Write(buf, binary.LittleEndian, uint16(len(delta.Bids))); err != nil {
		return nil, err
	}
	for _, lvl := range delta.Bids {
		if err := binary.Write(buf, binary.LittleEndian, lvl.Price); err != nil {
			return nil, err
		}
		if err := binary.Write(buf, binary.LittleEndian, lvl.Volume); err != nil {
			return nil, err
		}
	}
	// Asks
	if err := binary.Write(buf, binary.LittleEndian, uint16(len(delta.Asks))); err != nil {
		return nil, err
	}
	for _, lvl := range delta.Asks {
		if err := binary.Write(buf, binary.LittleEndian, lvl.Price); err != nil {
			return nil, err
		}
		if err := binary.Write(buf, binary.LittleEndian, lvl.Volume); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

func DecodeOrderBookDelta(data []byte) (*OrderBookDelta, error) {
	buf := bytes.NewReader(data)
	var symLen uint16
	if err := binary.Read(buf, binary.LittleEndian, &symLen); err != nil {
		return nil, err
	}
	symBytes := make([]byte, symLen)
	if _, err := buf.Read(symBytes); err != nil {
		return nil, err
	}
	delta := &OrderBookDelta{Symbol: string(symBytes)}
	var nBids uint16
	if err := binary.Read(buf, binary.LittleEndian, &nBids); err != nil {
		return nil, err
	}
	delta.Bids = make([]LevelDelta, nBids)
	for i := 0; i < int(nBids); i++ {
		if err := binary.Read(buf, binary.LittleEndian, &delta.Bids[i].Price); err != nil {
			return nil, err
		}
		if err := binary.Read(buf, binary.LittleEndian, &delta.Bids[i].Volume); err != nil {
			return nil, err
		}
	}
	var nAsks uint16
	if err := binary.Read(buf, binary.LittleEndian, &nAsks); err != nil {
		return nil, err
	}
	delta.Asks = make([]LevelDelta, nAsks)
	for i := 0; i < int(nAsks); i++ {
		if err := binary.Read(buf, binary.LittleEndian, &delta.Asks[i].Price); err != nil {
			return nil, err
		}
		if err := binary.Read(buf, binary.LittleEndian, &delta.Asks[i].Volume); err != nil {
			return nil, err
		}
	}
	return delta, nil
}

// EncodeOrderBookSnapshot encodes a full snapshot in binary
func EncodeOrderBookSnapshot(snapshot *OrderBookSnapshot) ([]byte, error) {
	buf := new(bytes.Buffer)
	symBytes := []byte(snapshot.Symbol)
	if err := binary.Write(buf, binary.LittleEndian, uint16(len(symBytes))); err != nil {
		return nil, err
	}
	buf.Write(symBytes)
	if err := binary.Write(buf, binary.LittleEndian, uint16(len(snapshot.Bids))); err != nil {
		return nil, err
	}
	for _, lvl := range snapshot.Bids {
		if err := binary.Write(buf, binary.LittleEndian, lvl.Price); err != nil {
			return nil, err
		}
		if err := binary.Write(buf, binary.LittleEndian, lvl.Volume); err != nil {
			return nil, err
		}
	}
	if err := binary.Write(buf, binary.LittleEndian, uint16(len(snapshot.Asks))); err != nil {
		return nil, err
	}
	for _, lvl := range snapshot.Asks {
		if err := binary.Write(buf, binary.LittleEndian, lvl.Price); err != nil {
			return nil, err
		}
		if err := binary.Write(buf, binary.LittleEndian, lvl.Volume); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

// DecodeOrderBookSnapshot decodes a full snapshot from binary
func DecodeOrderBookSnapshot(data []byte) (*OrderBookSnapshot, error) {
	buf := bytes.NewReader(data)
	var symLen uint16
	if err := binary.Read(buf, binary.LittleEndian, &symLen); err != nil {
		return nil, err
	}
	symBytes := make([]byte, symLen)
	if _, err := buf.Read(symBytes); err != nil {
		return nil, err
	}
	snap := &OrderBookSnapshot{Symbol: string(symBytes)}
	var nBids uint16
	if err := binary.Read(buf, binary.LittleEndian, &nBids); err != nil {
		return nil, err
	}
	snap.Bids = make([]LevelDelta, nBids)
	for i := 0; i < int(nBids); i++ {
		if err := binary.Read(buf, binary.LittleEndian, &snap.Bids[i].Price); err != nil {
			return nil, err
		}
		if err := binary.Read(buf, binary.LittleEndian, &snap.Bids[i].Volume); err != nil {
			return nil, err
		}
	}
	var nAsks uint16
	if err := binary.Read(buf, binary.LittleEndian, &nAsks); err != nil {
		return nil, err
	}
	snap.Asks = make([]LevelDelta, nAsks)
	for i := 0; i < int(nAsks); i++ {
		if err := binary.Read(buf, binary.LittleEndian, &snap.Asks[i].Price); err != nil {
			return nil, err
		}
		if err := binary.Read(buf, binary.LittleEndian, &snap.Asks[i].Volume); err != nil {
			return nil, err
		}
	}
	return snap, nil
}
