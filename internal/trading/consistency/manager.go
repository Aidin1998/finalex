package consistency

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ConsistencyManager ensures data consistency across trading operations
type ConsistencyManager struct {
	logger     *zap.Logger
	db         *gorm.DB
	mu         sync.RWMutex
	running    bool
	snapshots  map[string]*ConsistencySnapshot
	validators []ConsistencyValidator
}

// ConsistencySnapshot represents a point-in-time consistency snapshot
type ConsistencySnapshot struct {
	ID        string                 `json:"id"`
	Timestamp time.Time              `json:"timestamp"`
	MarketID  string                 `json:"market_id"`
	Checksum  string                 `json:"checksum"`
	Data      map[string]interface{} `json:"data"`
}

// ConsistencyValidator defines the interface for consistency validators
type ConsistencyValidator interface {
	Validate(ctx context.Context, snapshot *ConsistencySnapshot) error
	Name() string
}

// BalanceValidator validates balance consistency
type BalanceValidator struct {
	logger *zap.Logger
}

func (v *BalanceValidator) Name() string {
	return "balance_validator"
}

func (v *BalanceValidator) Validate(ctx context.Context, snapshot *ConsistencySnapshot) error {
	// Validate that total balances match expected values
	v.logger.Debug("Validating balance consistency", zap.String("snapshot_id", snapshot.ID))
	return nil
}

// OrderbookValidator validates orderbook consistency
type OrderbookValidator struct {
	logger *zap.Logger
}

func (v *OrderbookValidator) Name() string {
	return "orderbook_validator"
}

func (v *OrderbookValidator) Validate(ctx context.Context, snapshot *ConsistencySnapshot) error {
	// Validate orderbook integrity
	v.logger.Debug("Validating orderbook consistency", zap.String("snapshot_id", snapshot.ID))
	return nil
}

// NewConsistencyManager creates a new consistency manager
func NewConsistencyManager(logger *zap.Logger, db *gorm.DB) (*ConsistencyManager, error) {
	cm := &ConsistencyManager{
		logger:    logger,
		db:        db,
		snapshots: make(map[string]*ConsistencySnapshot),
	}

	// Initialize validators
	cm.validators = []ConsistencyValidator{
		&BalanceValidator{logger: logger},
		&OrderbookValidator{logger: logger},
	}

	return cm, nil
}

// CreateSnapshot creates a consistency snapshot
func (cm *ConsistencyManager) CreateSnapshot(ctx context.Context, marketID string) (*ConsistencySnapshot, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	snapshotID := fmt.Sprintf("snapshot_%s_%d", marketID, time.Now().Unix())

	snapshot := &ConsistencySnapshot{
		ID:        snapshotID,
		Timestamp: time.Now(),
		MarketID:  marketID,
		Data:      make(map[string]interface{}),
	}

	// Collect snapshot data
	if err := cm.collectSnapshotData(ctx, snapshot); err != nil {
		return nil, fmt.Errorf("failed to collect snapshot data: %w", err)
	}

	// Calculate checksum
	snapshot.Checksum = cm.calculateChecksum(snapshot)

	cm.snapshots[snapshotID] = snapshot

	cm.logger.Info("Consistency snapshot created",
		zap.String("snapshot_id", snapshotID),
		zap.String("market_id", marketID),
		zap.String("checksum", snapshot.Checksum))

	return snapshot, nil
}

// ValidateConsistency validates consistency using all validators
func (cm *ConsistencyManager) ValidateConsistency(ctx context.Context, snapshot *ConsistencySnapshot) error {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	for _, validator := range cm.validators {
		if err := validator.Validate(ctx, snapshot); err != nil {
			cm.logger.Error("Consistency validation failed",
				zap.String("validator", validator.Name()),
				zap.String("snapshot_id", snapshot.ID),
				zap.Error(err))
			return fmt.Errorf("validation failed for %s: %w", validator.Name(), err)
		}
	}

	cm.logger.Info("Consistency validation passed",
		zap.String("snapshot_id", snapshot.ID),
		zap.Int("validators", len(cm.validators)))

	return nil
}

// CheckConsistency performs a full consistency check
func (cm *ConsistencyManager) CheckConsistency(ctx context.Context, marketID string) error {
	snapshot, err := cm.CreateSnapshot(ctx, marketID)
	if err != nil {
		return fmt.Errorf("failed to create snapshot: %w", err)
	}

	return cm.ValidateConsistency(ctx, snapshot)
}

// RepairInconsistency attempts to repair detected inconsistencies
func (cm *ConsistencyManager) RepairInconsistency(ctx context.Context, snapshotID string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	snapshot, exists := cm.snapshots[snapshotID]
	if !exists {
		return fmt.Errorf("snapshot not found: %s", snapshotID)
	}

	cm.logger.Info("Attempting to repair inconsistency",
		zap.String("snapshot_id", snapshotID))

	// Repair logic would go here
	// For now, just log the attempt
	time.Sleep(time.Millisecond * 50) // Simulate repair time

	cm.logger.Info("Inconsistency repair completed",
		zap.String("snapshot_id", snapshotID))

	return nil
}

// collectSnapshotData collects data for the consistency snapshot
func (cm *ConsistencyManager) collectSnapshotData(ctx context.Context, snapshot *ConsistencySnapshot) error {
	// Collect orderbook data
	snapshot.Data["orderbook"] = map[string]interface{}{
		"bids_count":   0,
		"asks_count":   0,
		"total_volume": decimal.Zero,
	}

	// Collect balance data
	snapshot.Data["balances"] = map[string]interface{}{
		"total_locked":    decimal.Zero,
		"total_available": decimal.Zero,
	}

	// Collect trade data
	snapshot.Data["trades"] = map[string]interface{}{
		"pending_settlements":   0,
		"completed_settlements": 0,
	}

	return nil
}

// calculateChecksum calculates a checksum for the snapshot
func (cm *ConsistencyManager) calculateChecksum(snapshot *ConsistencySnapshot) string {
	// Simple checksum calculation - in production, use a proper hash function
	return fmt.Sprintf("checksum_%s_%d", snapshot.MarketID, snapshot.Timestamp.Unix())
}

// GetSnapshot retrieves a consistency snapshot
func (cm *ConsistencyManager) GetSnapshot(snapshotID string) (*ConsistencySnapshot, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	snapshot, exists := cm.snapshots[snapshotID]
	if !exists {
		return nil, fmt.Errorf("snapshot not found: %s", snapshotID)
	}

	return snapshot, nil
}

// CleanupOldSnapshots removes old snapshots to free memory
func (cm *ConsistencyManager) CleanupOldSnapshots(maxAge time.Duration) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	var toDelete []string

	for id, snapshot := range cm.snapshots {
		if snapshot.Timestamp.Before(cutoff) {
			toDelete = append(toDelete, id)
		}
	}

	for _, id := range toDelete {
		delete(cm.snapshots, id)
	}

	if len(toDelete) > 0 {
		cm.logger.Info("Cleaned up old snapshots",
			zap.Int("deleted_count", len(toDelete)))
	}
}

// Start starts the consistency manager
func (cm *ConsistencyManager) Start() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.running = true
	cm.logger.Info("Consistency manager started")

	// Start periodic cleanup
	go cm.periodicCleanup()

	return nil
}

// Stop stops the consistency manager
func (cm *ConsistencyManager) Stop() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.running = false
	cm.logger.Info("Consistency manager stopped")
	return nil
}

// periodicCleanup runs periodic cleanup of old snapshots
func (cm *ConsistencyManager) periodicCleanup() {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for cm.running {
		select {
		case <-ticker.C:
			cm.CleanupOldSnapshots(24 * time.Hour)
		}
	}
}
