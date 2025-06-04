package orderqueue

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/dgraph-io/badger/v3"
)

// BadgerQueue is a disk-backed implementation of Queue using BadgerDB.
type BadgerQueue struct {
	db *badger.DB
}

// NewBadgerQueue initializes a new BadgerQueue at the given path.
func NewBadgerQueue(path string) (*BadgerQueue, error) {
	opts := badger.DefaultOptions(path)
	opts.Logger = nil // disable internal logging
	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("opening badger db: %w", err)
	}
	return &BadgerQueue{db: db}, nil
}

// key format: priority:timestamp:orderID
func formatKey(o Order) ([]byte, error) {
	// Use timestamp for FIFO within same priority
	t := o.CreatedAt.UnixNano()
	key := fmt.Sprintf("%04d:%020d:%s", o.Priority, t, o.ID)
	return []byte(key), nil
}

// Enqueue adds an order to the queue if not duplicate.
func (q *BadgerQueue) Enqueue(ctx context.Context, order Order) error {
	key, err := formatKey(order)
	if err != nil {
		return err
	}
	val, err := json.Marshal(order)
	if err != nil {
		return err
	}
	return q.db.Update(func(txn *badger.Txn) error {
		_, err := txn.Get(key)
		if err == nil {
			// duplicate
			return fmt.Errorf("order duplicate: %s", order.ID)
		}
		if err != badger.ErrKeyNotFound {
			return err
		}
		return txn.Set(key, val)
	})
}

// Dequeue retrieves the highest priority order (smallest key) and holds it, but not remove until Acknowledge.
func (q *BadgerQueue) Dequeue(ctx context.Context) (Order, error) {
	var result Order
	err := q.db.View(func(txn *badger.Txn) error {
		r := txn.NewIterator(badger.DefaultIteratorOptions)
		defer r.Close()
		for r.Rewind(); r.Valid(); r.Next() {
			item := r.Item()
			error := item.Value(func(v []byte) error {
				return json.Unmarshal(v, &result)
			})
			if error != nil {
				return error
			}
			return nil // got first
		}
		return fmt.Errorf("queue empty")
	})
	return result, err
}

// Acknowledge removes the processed order from storage.
func (q *BadgerQueue) Acknowledge(ctx context.Context, id string) error {
	// scan to find key containing id
	return q.db.Update(func(txn *badger.Txn) error {
		r := txn.NewIterator(badger.DefaultIteratorOptions)
		defer r.Close()
		for r.Rewind(); r.Valid(); r.Next() {
			item := r.Item()
			k := item.Key()
			if strings.HasSuffix(string(k), ":"+id) {
				return txn.Delete(k)
			}
		}
		return fmt.Errorf("order not found: %s", id)
	})
}

// ReplayPending returns all unacknowledged orders by priority order.
func (q *BadgerQueue) ReplayPending(ctx context.Context) ([]Order, error) {
	orders := make([]Order, 0)
	err := q.db.View(func(txn *badger.Txn) error {
		r := txn.NewIterator(badger.DefaultIteratorOptions)
		defer r.Close()
		for r.Rewind(); r.Valid(); r.Next() {
			item := r.Item()
			var o Order
			err := item.Value(func(v []byte) error { return json.Unmarshal(v, &o) })
			if err != nil {
				return err
			}
			orders = append(orders, o)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	// sort by priority and CreatedAt
	sort.Slice(orders, func(i, j int) bool {
		iKey, _ := formatKey(orders[i])
		jKey, _ := formatKey(orders[j])
		return string(iKey) < string(jKey)
	})
	return orders, nil
}

// Shutdown closes the underlying BadgerDB.
func (q *BadgerQueue) Shutdown(ctx context.Context) error {
	return q.db.Close()
}
