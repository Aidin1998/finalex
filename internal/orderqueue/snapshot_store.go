package orderqueue

import (
	"context"
	"fmt"
	"time"

	"github.com/dgraph-io/badger/v4"
)

// SnapshotStore defines interface for persisting order queue snapshots for fast recovery.
type SnapshotStore interface {
	// Save persists the given state blob.
	Save(ctx context.Context, state []byte) error

	// Load retrieves the latest persisted state blob.
	Load(ctx context.Context) ([]byte, error)
}

// BadgerSnapshotStore persists snapshots in BadgerDB.
type BadgerSnapshotStore struct {
	db *badger.DB
}

// NewBadgerSnapshotStore initializes snapshot store at path.
func NewBadgerSnapshotStore(path string) (*BadgerSnapshotStore, error) {
	opts := badger.DefaultOptions(path)
	opts.Logger = nil
	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open snapshot store: %w", err)
	}
	return &BadgerSnapshotStore{db: db}, nil
}

// Save stores snapshot with timestamp-based key.
func (s *BadgerSnapshotStore) Save(ctx context.Context, state []byte) error {
	key := []byte(fmt.Sprintf("snapshot:%d", time.Now().UnixNano()))
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, state)
	})
}

// Load returns the most recent snapshot.
func (s *BadgerSnapshotStore) Load(ctx context.Context) ([]byte, error) {
	var latestKey []byte

	err := s.db.View(func(txn *badger.Txn) error {
		r := txn.NewIterator(badger.DefaultIteratorOptions)
		defer r.Close()
		for r.Rewind(); r.Valid(); r.Next() {
			item := r.Item()
			k := item.Key()
			if len(latestKey) == 0 || string(k) > string(latestKey) {
				latestKey = append([]byte(nil), k...)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if latestKey == nil {
		return nil, fmt.Errorf("no snapshot found")
	}
	var state []byte
	err = s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(latestKey)
		if err != nil {
			return err
		}
		return item.Value(func(v []byte) error {
			state = append([]byte(nil), v...)
			return nil
		})
	})
	return state, err
}
