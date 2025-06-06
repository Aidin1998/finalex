// Async Write-Behind Database Writer for Trading Engine
// Implements non-blocking, batched, atomic DB operations for trades/orders
// All writes are performed outside the matching engine hot path

package persistence

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/Aidin1998/finalex/internal/trading/dbutil"
	"github.com/Aidin1998/finalex/internal/trading/model"
)

type DatabaseOperation struct {
	OpType  string // "trade", "order", etc.
	Payload interface{}
	Retry   int
}

type DatabaseWriter struct {
	writeQueue    chan DatabaseOperation
	batchSize     int
	flushInterval time.Duration
	workers       int
	stopCh        chan struct{}
	deadLetter    chan DatabaseOperation
	wg            sync.WaitGroup
}

func NewDatabaseWriter(batchSize int, flushInterval time.Duration, workers int, queueSize int) *DatabaseWriter {
	dw := &DatabaseWriter{
		writeQueue:    make(chan DatabaseOperation, queueSize),
		batchSize:     batchSize,
		flushInterval: flushInterval,
		workers:       workers,
		stopCh:        make(chan struct{}),
		deadLetter:    make(chan DatabaseOperation, queueSize),
	}
	for i := 0; i < workers; i++ {
		dw.wg.Add(1)
		go dw.worker()
	}
	return dw
}

func (dw *DatabaseWriter) QueueWrite(op DatabaseOperation) {
	select {
	case dw.writeQueue <- op:
		// queued
	default:
		log.Printf("Database write queue full, dropping op: %v", op.OpType)
		// Optionally send to dead letter
		select {
		case dw.deadLetter <- op:
		default:
		}
	}
}

func (dw *DatabaseWriter) worker() {
	defer dw.wg.Done()
	batch := make([]DatabaseOperation, 0, dw.batchSize)
	ticker := time.NewTicker(dw.flushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-dw.stopCh:
			return
		case op := <-dw.writeQueue:
			batch = append(batch, op)
			if len(batch) >= dw.batchSize {
				dw.flush(batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				dw.flush(batch)
				batch = batch[:0]
			}
		}
	}
}

func (dw *DatabaseWriter) flush(batch []DatabaseOperation) {
	// Group by OpType for bulk ops
	trades := make([]*model.Trade, 0)
	orders := make([]*model.Order, 0)
	for _, op := range batch {
		switch op.OpType {
		case "trade":
			if t, ok := op.Payload.(*model.Trade); ok {
				trades = append(trades, t)
			}
		case "order":
			if o, ok := op.Payload.(*model.Order); ok {
				orders = append(orders, o)
			}
		}
	}
	ctx := context.Background()
	if len(trades) > 0 {
		if err := PersistTradesBatch(ctx, trades); err != nil {
			log.Printf("Trade batch persist failed: %v", err)
			// Retry or dead letter
		}
	}
	// TODO: Add order batch persistence
}

// Atomic batch persistence for trades
func PersistTradesBatch(ctx context.Context, trades []*model.Trade) error {
	if len(trades) == 0 {
		return nil
	}
	// Use sharding if needed, here just use first trade's user for demo
	shardKey := int64(0)
	if len(trades) > 0 {
		shardKey = int64(trades[0].UserID.ID()) // Use the full uint32 as int64
	}
	db := dbutil.GetDBForKey(shardKey)
	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()
	for _, trade := range trades {
		if err := tx.Create(trade).Error; err != nil {
			tx.Rollback()
			return err
		}
	}
	// TODO: update positions atomically
	return tx.Commit().Error
}

func (dw *DatabaseWriter) Stop() {
	close(dw.stopCh)
	dw.wg.Wait()
}
