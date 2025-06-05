// =============================
// Market Data Migration Participant
// =============================
// This participant handles real-time market data synchronization during
// order book migration, ensuring continuous data feed availability.

package participants

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Aidin1998/finalex/internal/trading/migration"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// MarketDataParticipant handles market data synchronization during migration
type MarketDataParticipant struct {
	id          string
	pair        string
	dataFeeds   map[string]DataFeed
	subscribers map[string]DataSubscriber
	logger      *zap.SugaredLogger

	// Migration state
	currentMigrationID uuid.UUID
	preparationData    *MarketDataPreparationData
	isHealthy          int64 // atomic bool
	lastHeartbeat      int64 // atomic timestamp

	// Data streaming state
	isStreaming    int64 // atomic bool
	streamBuffer   *StreamBuffer
	fallbackStream *FallbackStream

	// Synchronization
	mu          sync.RWMutex
	migrationMu sync.Mutex // prevents concurrent migrations
}

// DataFeed represents a market data feed
type DataFeed interface {
	GetName() string
	IsConnected() bool
	GetLatestPrice() (*PriceData, error)
	GetOrderBookSnapshot() (*OrderBookData, error)
	Subscribe(symbol string) error
	Unsubscribe(symbol string) error
	Pause() error
	Resume() error
}

// DataSubscriber represents a market data subscriber
type DataSubscriber interface {
	GetID() string
	SendUpdate(update *MarketDataUpdate) error
	IsActive() bool
	Pause() error
	Resume() error
}

// MarketDataPreparationData contains data prepared during the prepare phase
type MarketDataPreparationData struct {
	DataSnapshot       *MarketDataSnapshot          `json:"data_snapshot"`
	FeedStatuses       map[string]*FeedStatus       `json:"feed_statuses"`
	SubscriberStatuses map[string]*SubscriberStatus `json:"subscriber_statuses"`
	SyncStrategy       *SyncStrategy                `json:"sync_strategy"`
	BackupConfig       *BackupStreamConfig          `json:"backup_config"`
	PreparationTime    time.Time                    `json:"preparation_time"`
}

// MarketDataSnapshot represents current market data state
type MarketDataSnapshot struct {
	Timestamp      time.Time           `json:"timestamp"`
	Pair           string              `json:"pair"`
	LastPrice      decimal.Decimal     `json:"last_price"`
	Volume24h      decimal.Decimal     `json:"volume_24h"`
	PriceChange24h decimal.Decimal     `json:"price_change_24h"`
	BidPrice       decimal.Decimal     `json:"bid_price"`
	AskPrice       decimal.Decimal     `json:"ask_price"`
	Spread         decimal.Decimal     `json:"spread"`
	OrderBookDepth int                 `json:"order_book_depth"`
	TickCount      int64               `json:"tick_count"`
	FeedSources    []string            `json:"feed_sources"`
	DataQuality    *DataQualityMetrics `json:"data_quality"`
	Checksum       string              `json:"checksum"`
}

// FeedStatus represents the status of a data feed
type FeedStatus struct {
	FeedName     string    `json:"feed_name"`
	IsConnected  bool      `json:"is_connected"`
	LastUpdate   time.Time `json:"last_update"`
	MessageRate  float64   `json:"message_rate"` // messages per second
	Latency      float64   `json:"latency_ms"`
	ErrorRate    float64   `json:"error_rate"`
	QueueDepth   int       `json:"queue_depth"`
	IsHealthy    bool      `json:"is_healthy"`
	ErrorMessage string    `json:"error_message,omitempty"`
}

// SubscriberStatus represents the status of a data subscriber
type SubscriberStatus struct {
	SubscriberID string    `json:"subscriber_id"`
	IsActive     bool      `json:"is_active"`
	LastDelivery time.Time `json:"last_delivery"`
	DeliveryRate float64   `json:"delivery_rate"` // deliveries per second
	BacklogSize  int       `json:"backlog_size"`
	DropRate     float64   `json:"drop_rate"`
	IsHealthy    bool      `json:"is_healthy"`
	ErrorMessage string    `json:"error_message,omitempty"`
}

// SyncStrategy defines how market data will be synchronized during migration
type SyncStrategy struct {
	Strategy         string        `json:"strategy"` // pause_resume, dual_stream, buffer_replay
	BufferSize       int           `json:"buffer_size"`
	MaxPauseDuration time.Duration `json:"max_pause_duration"`
	FallbackEnabled  bool          `json:"fallback_enabled"`
	QualityThreshold float64       `json:"quality_threshold"`
	SyncTimeout      time.Duration `json:"sync_timeout"`
}

// BackupStreamConfig defines backup streaming configuration
type BackupStreamConfig struct {
	Enabled         bool          `json:"enabled"`
	BackupFeeds     []string      `json:"backup_feeds"`
	BufferDuration  time.Duration `json:"buffer_duration"`
	SwitchThreshold time.Duration `json:"switch_threshold"`
	QualityCheck    bool          `json:"quality_check"`
}

// DataQualityMetrics contains metrics about data quality
type DataQualityMetrics struct {
	Completeness   float64   `json:"completeness"`    // 0.0 - 1.0
	Accuracy       float64   `json:"accuracy"`        // 0.0 - 1.0
	Timeliness     float64   `json:"timeliness"`      // 0.0 - 1.0
	Consistency    float64   `json:"consistency"`     // 0.0 - 1.0
	OverallQuality float64   `json:"overall_quality"` // 0.0 - 1.0
	LastUpdated    time.Time `json:"last_updated"`
	SampleSize     int64     `json:"sample_size"`
}

// PriceData represents price information
type PriceData struct {
	Symbol    string          `json:"symbol"`
	Price     decimal.Decimal `json:"price"`
	Timestamp time.Time       `json:"timestamp"`
	Volume    decimal.Decimal `json:"volume"`
	Source    string          `json:"source"`
}

// OrderBookData represents order book information
type OrderBookData struct {
	Symbol    string       `json:"symbol"`
	Bids      []PriceLevel `json:"bids"`
	Asks      []PriceLevel `json:"asks"`
	Timestamp time.Time    `json:"timestamp"`
	Sequence  int64        `json:"sequence"`
	Source    string       `json:"source"`
}

// PriceLevel represents a price level in the order book
type PriceLevel struct {
	Price  decimal.Decimal `json:"price"`
	Volume decimal.Decimal `json:"volume"`
	Orders int             `json:"orders"`
}

// MarketDataUpdate represents a market data update
type MarketDataUpdate struct {
	Type      string      `json:"type"` // price, trade, orderbook
	Symbol    string      `json:"symbol"`
	Data      interface{} `json:"data"`
	Timestamp time.Time   `json:"timestamp"`
	Sequence  int64       `json:"sequence"`
}

// StreamBuffer manages buffered market data during migration
type StreamBuffer struct {
	buffer    []*MarketDataUpdate
	maxSize   int
	startTime time.Time
	mu        sync.RWMutex
}

// FallbackStream manages fallback data streaming
type FallbackStream struct {
	isActive    bool
	primaryFeed string
	backupFeeds []string
	currentFeed string
	mu          sync.RWMutex
}

// NewMarketDataParticipant creates a new market data migration participant
func NewMarketDataParticipant(
	pair string,
	dataFeeds map[string]DataFeed,
	subscribers map[string]DataSubscriber,
	logger *zap.SugaredLogger,
) *MarketDataParticipant {
	p := &MarketDataParticipant{
		id:          fmt.Sprintf("marketdata_%s", pair),
		pair:        pair,
		dataFeeds:   dataFeeds,
		subscribers: subscribers,
		logger:      logger,
		streamBuffer: &StreamBuffer{
			buffer:  make([]*MarketDataUpdate, 0),
			maxSize: 10000,
		},
		fallbackStream: &FallbackStream{
			isActive: false,
		},
	}

	// Set initial health status
	atomic.StoreInt64(&p.isHealthy, 1)
	atomic.StoreInt64(&p.lastHeartbeat, time.Now().UnixNano())
	atomic.StoreInt64(&p.isStreaming, 1)

	return p
}

// GetID returns the participant identifier
func (p *MarketDataParticipant) GetID() string {
	return p.id
}

// GetType returns the participant type
func (p *MarketDataParticipant) GetType() string {
	return "market_data"
}

// Prepare implements the prepare phase of two-phase commit
func (p *MarketDataParticipant) Prepare(ctx context.Context, migrationID uuid.UUID, config *migration.MigrationConfig) (*migration.ParticipantState, error) {
	p.migrationMu.Lock()
	defer p.migrationMu.Unlock()

	p.logger.Infow("Starting prepare phase for market data migration",
		"migration_id", migrationID,
		"pair", p.pair,
	)

	startTime := time.Now()
	p.currentMigrationID = migrationID

	// Step 1: Perform market data health check
	healthCheck, err := p.performMarketDataHealthCheck(ctx)
	if err != nil {
		return p.createFailedState("market data health check failed", err), nil
	}

	if healthCheck.Status != "healthy" {
		return p.createFailedState("market data not healthy for migration", fmt.Errorf("health check failed: %s", healthCheck.Message)), nil
	}

	// Step 2: Create market data snapshot
	snapshot, err := p.createMarketDataSnapshot(ctx)
	if err != nil {
		return p.createFailedState("market data snapshot creation failed", err), nil
	}

	// Step 3: Assess data quality
	qualityScore := snapshot.DataQuality.OverallQuality
	if qualityScore < 0.9 {
		return p.createFailedState("data quality insufficient for migration", fmt.Errorf("quality score: %.2f", qualityScore)), nil
	}

	// Step 4: Analyze feed statuses
	feedStatuses, err := p.analyzeFeedStatuses(ctx)
	if err != nil {
		return p.createFailedState("feed status analysis failed", err), nil
	}

	// Step 5: Analyze subscriber statuses
	subscriberStatuses, err := p.analyzeSubscriberStatuses(ctx)
	if err != nil {
		return p.createFailedState("subscriber status analysis failed", err), nil
	}

	// Step 6: Create synchronization strategy
	syncStrategy := p.createSyncStrategy(config, snapshot, feedStatuses)

	// Step 7: Configure backup streaming
	backupConfig := p.configureBackupStreaming(config, feedStatuses)

	// Step 8: Initialize streaming buffer
	err = p.initializeStreamBuffer(ctx, syncStrategy)
	if err != nil {
		return p.createFailedState("stream buffer initialization failed", err), nil
	}

	// Step 9: Validate streaming continuity
	err = p.validateStreamingContinuity(ctx)
	if err != nil {
		return p.createFailedState("streaming continuity validation failed", err), nil
	}

	// Step 10: Store preparation data
	p.preparationData = &MarketDataPreparationData{
		DataSnapshot:       snapshot,
		FeedStatuses:       feedStatuses,
		SubscriberStatuses: subscriberStatuses,
		SyncStrategy:       syncStrategy,
		BackupConfig:       backupConfig,
		PreparationTime:    startTime,
	}

	preparationDuration := time.Since(startTime)
	p.logger.Infow("Prepare phase completed successfully",
		"migration_id", migrationID,
		"pair", p.pair,
		"duration", preparationDuration,
		"quality_score", qualityScore,
		"sync_strategy", syncStrategy.Strategy,
	)

	// Create orders snapshot for coordinator
	ordersSnapshot := &migration.OrdersSnapshot{
		Timestamp:   snapshot.Timestamp,
		TotalOrders: snapshot.TickCount,
		TotalVolume: snapshot.Volume24h,
		Checksum:    snapshot.Checksum,
	}

	// Return successful state
	now := time.Now()
	state := &migration.ParticipantState{
		ID:              p.id,
		Type:            p.GetType(),
		Vote:            migration.VoteYes,
		VoteTime:        &now,
		LastHeartbeat:   now,
		IsHealthy:       true,
		OrdersSnapshot:  ordersSnapshot,
		PreparationData: p.preparationData,
	}

	return state, nil
}

// Commit implements the commit phase of two-phase commit
func (p *MarketDataParticipant) Commit(ctx context.Context, migrationID uuid.UUID) error {
	p.migrationMu.Lock()
	defer p.migrationMu.Unlock()

	if p.currentMigrationID != migrationID {
		return fmt.Errorf("migration ID mismatch: expected %s, got %s", p.currentMigrationID, migrationID)
	}

	if p.preparationData == nil {
		return fmt.Errorf("no preparation data available for migration %s", migrationID)
	}

	p.logger.Infow("Starting commit phase for market data migration",
		"migration_id", migrationID,
		"pair", p.pair,
	)

	startTime := time.Now()

	// Step 1: Execute synchronization strategy
	err := p.executeSyncStrategy(ctx, p.preparationData.SyncStrategy)
	if err != nil {
		return fmt.Errorf("sync strategy execution failed: %w", err)
	}

	// Step 2: Validate data continuity post-migration
	err = p.validateDataContinuity(ctx)
	if err != nil {
		return fmt.Errorf("data continuity validation failed: %w", err)
	}

	// Step 3: Update subscriber configurations
	err = p.updateSubscriberConfigurations(ctx)
	if err != nil {
		return fmt.Errorf("subscriber configuration update failed: %w", err)
	}

	// Step 4: Cleanup temporary resources
	err = p.cleanupTemporaryResources(ctx)
	if err != nil {
		p.logger.Warnw("Failed to cleanup temporary resources", "error", err)
		// Don't fail the migration for this
	}

	commitDuration := time.Since(startTime)
	p.logger.Infow("Commit phase completed successfully",
		"migration_id", migrationID,
		"pair", p.pair,
		"duration", commitDuration,
	)

	return nil
}

// Abort implements the abort phase of two-phase commit
func (p *MarketDataParticipant) Abort(ctx context.Context, migrationID uuid.UUID) error {
	p.migrationMu.Lock()
	defer p.migrationMu.Unlock()

	p.logger.Infow("Starting abort phase for market data migration",
		"migration_id", migrationID,
		"pair", p.pair,
	)

	// Step 1: Restore original streaming configuration
	err := p.restoreOriginalStreaming(ctx)
	if err != nil {
		p.logger.Errorw("Failed to restore original streaming", "error", err)
	}

	// Step 2: Resume normal data feeds if paused
	err = p.resumeAllFeeds(ctx)
	if err != nil {
		p.logger.Errorw("Failed to resume data feeds", "error", err)
	}

	// Step 3: Disable fallback streams
	err = p.disableFallbackStreams(ctx)
	if err != nil {
		p.logger.Errorw("Failed to disable fallback streams", "error", err)
	}

	// Step 4: Clear stream buffers
	p.clearStreamBuffers()

	// Step 5: Cleanup resources
	err = p.cleanup(ctx, migrationID)
	if err != nil {
		p.logger.Warnw("Cleanup failed during abort", "error", err)
	}

	p.logger.Infow("Abort phase completed", "migration_id", migrationID, "pair", p.pair)
	return nil
}

// HealthCheck performs a market data health check
func (p *MarketDataParticipant) HealthCheck(ctx context.Context) (*migration.HealthCheck, error) {
	atomic.StoreInt64(&p.lastHeartbeat, time.Now().UnixNano())

	return p.performMarketDataHealthCheck(ctx)
}

// GetSnapshot returns the current market data snapshot
func (p *MarketDataParticipant) GetSnapshot(ctx context.Context) (*migration.OrdersSnapshot, error) {
	snapshot, err := p.createMarketDataSnapshot(ctx)
	if err != nil {
		return nil, err
	}

	return &migration.OrdersSnapshot{
		Timestamp:   snapshot.Timestamp,
		TotalOrders: snapshot.TickCount,
		TotalVolume: snapshot.Volume24h,
		Checksum:    snapshot.Checksum,
	}, nil
}

// Cleanup cleans up resources after migration
func (p *MarketDataParticipant) Cleanup(ctx context.Context, migrationID uuid.UUID) error {
	return p.cleanup(ctx, migrationID)
}

// Helper methods

func (p *MarketDataParticipant) createFailedState(reason string, err error) *migration.ParticipantState {
	now := time.Now()
	return &migration.ParticipantState{
		ID:            p.id,
		Type:          p.GetType(),
		Vote:          migration.VoteNo,
		VoteTime:      &now,
		LastHeartbeat: now,
		IsHealthy:     false,
		ErrorMessage:  fmt.Sprintf("%s: %v", reason, err),
	}
}

func (p *MarketDataParticipant) performMarketDataHealthCheck(ctx context.Context) (*migration.HealthCheck, error) {
	issues := make([]string, 0)
	score := 1.0

	// Check feed connections
	connectedFeeds := 0
	for name, feed := range p.dataFeeds {
		if feed.IsConnected() {
			connectedFeeds++
		} else {
			issues = append(issues, fmt.Sprintf("feed %s disconnected", name))
			score -= 0.2
		}
	}

	if connectedFeeds == 0 {
		issues = append(issues, "no data feeds connected")
		score -= 0.5
	}

	// Check streaming status
	if atomic.LoadInt64(&p.isStreaming) == 0 {
		issues = append(issues, "data streaming is stopped")
		score -= 0.3
	}

	// Check subscriber health
	activeSubscribers := 0
	for id, subscriber := range p.subscribers {
		if subscriber.IsActive() {
			activeSubscribers++
		} else {
			issues = append(issues, fmt.Sprintf("subscriber %s inactive", id))
			score -= 0.1
		}
	}

	// Determine status
	status := "healthy"
	message := "Market data is healthy"

	if len(issues) > 0 {
		status = "warning"
		message = fmt.Sprintf("Issues found: %v", issues)
		if score < 0.5 {
			status = "critical"
		}
	}

	// Update health status
	if score >= 0.8 {
		atomic.StoreInt64(&p.isHealthy, 1)
	} else {
		atomic.StoreInt64(&p.isHealthy, 0)
	}

	return &migration.HealthCheck{
		Name:       fmt.Sprintf("market_data_%s", p.pair),
		Status:     status,
		Score:      score,
		Message:    message,
		LastCheck:  time.Now(),
		CheckCount: 1,
		FailCount:  int64(len(issues)),
	}, nil
}

func (p *MarketDataParticipant) createMarketDataSnapshot(ctx context.Context) (*MarketDataSnapshot, error) {
	snapshot := &MarketDataSnapshot{
		Timestamp:   time.Now(),
		Pair:        p.pair,
		FeedSources: make([]string, 0),
	}

	// Get data from primary feed if available
	for name, feed := range p.dataFeeds {
		if feed.IsConnected() {
			snapshot.FeedSources = append(snapshot.FeedSources, name)

			// Get latest price
			if priceData, err := feed.GetLatestPrice(); err == nil {
				snapshot.LastPrice = priceData.Price
				snapshot.Volume24h = priceData.Volume
			}

			// Get order book data
			if orderBookData, err := feed.GetOrderBookSnapshot(); err == nil {
				if len(orderBookData.Bids) > 0 {
					snapshot.BidPrice = orderBookData.Bids[0].Price
				}
				if len(orderBookData.Asks) > 0 {
					snapshot.AskPrice = orderBookData.Asks[0].Price
				}
				snapshot.OrderBookDepth = len(orderBookData.Bids) + len(orderBookData.Asks)
			}

			break // Use first connected feed
		}
	}

	// Calculate spread
	if snapshot.BidPrice.GreaterThan(decimal.Zero) && snapshot.AskPrice.GreaterThan(decimal.Zero) {
		snapshot.Spread = snapshot.AskPrice.Sub(snapshot.BidPrice)
	}

	// Calculate data quality metrics
	snapshot.DataQuality = p.calculateDataQuality()

	// Mock some values for demonstration
	snapshot.PriceChange24h = decimal.NewFromFloat(1.5)
	snapshot.TickCount = 10000

	// Calculate checksum
	checksumData := fmt.Sprintf("%s_%s_%s_%d",
		p.pair,
		snapshot.LastPrice.String(),
		snapshot.Volume24h.String(),
		snapshot.TickCount)
	hash := md5.Sum([]byte(checksumData))
	snapshot.Checksum = hex.EncodeToString(hash[:])

	return snapshot, nil
}

func (p *MarketDataParticipant) analyzeFeedStatuses(ctx context.Context) (map[string]*FeedStatus, error) {
	statuses := make(map[string]*FeedStatus)

	for name, feed := range p.dataFeeds {
		status := &FeedStatus{
			FeedName:    name,
			IsConnected: feed.IsConnected(),
			LastUpdate:  time.Now(),
			MessageRate: 100.0, // Mock data
			Latency:     5.0,   // Mock data
			ErrorRate:   0.001, // Mock data
			QueueDepth:  10,    // Mock data
			IsHealthy:   feed.IsConnected(),
		}

		if !status.IsConnected {
			status.ErrorMessage = "feed disconnected"
		}

		statuses[name] = status
	}

	return statuses, nil
}

func (p *MarketDataParticipant) analyzeSubscriberStatuses(ctx context.Context) (map[string]*SubscriberStatus, error) {
	statuses := make(map[string]*SubscriberStatus)

	for id, subscriber := range p.subscribers {
		status := &SubscriberStatus{
			SubscriberID: id,
			IsActive:     subscriber.IsActive(),
			LastDelivery: time.Now(),
			DeliveryRate: 95.0,  // Mock data
			BacklogSize:  5,     // Mock data
			DropRate:     0.001, // Mock data
			IsHealthy:    subscriber.IsActive(),
		}

		if !status.IsActive {
			status.ErrorMessage = "subscriber inactive"
		}

		statuses[id] = status
	}

	return statuses, nil
}

func (p *MarketDataParticipant) createSyncStrategy(config *migration.MigrationConfig, snapshot *MarketDataSnapshot, feedStatuses map[string]*FeedStatus) *SyncStrategy {
	strategy := "buffer_replay"

	// Choose strategy based on downtime requirements
	if config.MaxDowntimeMs < 50 {
		strategy = "dual_stream"
	} else if config.MaxDowntimeMs < 100 {
		strategy = "buffer_replay"
	} else {
		strategy = "pause_resume"
	}

	return &SyncStrategy{
		Strategy:         strategy,
		BufferSize:       10000,
		MaxPauseDuration: time.Duration(config.MaxDowntimeMs) * time.Millisecond,
		FallbackEnabled:  true,
		QualityThreshold: 0.95,
		SyncTimeout:      30 * time.Second,
	}
}

func (p *MarketDataParticipant) configureBackupStreaming(config *migration.MigrationConfig, feedStatuses map[string]*FeedStatus) *BackupStreamConfig {
	backupFeeds := make([]string, 0)
	for name, status := range feedStatuses {
		if status.IsHealthy {
			backupFeeds = append(backupFeeds, name)
		}
	}

	return &BackupStreamConfig{
		Enabled:         len(backupFeeds) > 1,
		BackupFeeds:     backupFeeds,
		BufferDuration:  time.Duration(config.MaxDowntimeMs*2) * time.Millisecond,
		SwitchThreshold: time.Duration(config.MaxDowntimeMs/2) * time.Millisecond,
		QualityCheck:    true,
	}
}

func (p *MarketDataParticipant) initializeStreamBuffer(ctx context.Context, strategy *SyncStrategy) error {
	p.streamBuffer.mu.Lock()
	defer p.streamBuffer.mu.Unlock()

	p.streamBuffer.buffer = make([]*MarketDataUpdate, 0, strategy.BufferSize)
	p.streamBuffer.maxSize = strategy.BufferSize
	p.streamBuffer.startTime = time.Now()

	return nil
}

func (p *MarketDataParticipant) validateStreamingContinuity(ctx context.Context) error {
	// Validate that streaming can continue during migration
	for name, feed := range p.dataFeeds {
		if !feed.IsConnected() {
			return fmt.Errorf("feed %s is not connected", name)
		}
	}

	return nil
}

func (p *MarketDataParticipant) executeSyncStrategy(ctx context.Context, strategy *SyncStrategy) error {
	p.logger.Infow("Executing sync strategy", "strategy", strategy.Strategy, "pair", p.pair)

	switch strategy.Strategy {
	case "pause_resume":
		return p.executePauseResumeStrategy(ctx, strategy)
	case "dual_stream":
		return p.executeDualStreamStrategy(ctx, strategy)
	case "buffer_replay":
		return p.executeBufferReplayStrategy(ctx, strategy)
	default:
		return fmt.Errorf("unknown sync strategy: %s", strategy.Strategy)
	}
}

func (p *MarketDataParticipant) executePauseResumeStrategy(ctx context.Context, strategy *SyncStrategy) error {
	// Temporarily pause all feeds
	for name, feed := range p.dataFeeds {
		if err := feed.Pause(); err != nil {
			p.logger.Warnw("Failed to pause feed", "feed", name, "error", err)
		}
	}

	// Wait for migration to complete (simulated)
	time.Sleep(strategy.MaxPauseDuration)

	// Resume all feeds
	for name, feed := range p.dataFeeds {
		if err := feed.Resume(); err != nil {
			p.logger.Warnw("Failed to resume feed", "feed", name, "error", err)
		}
	}

	return nil
}

func (p *MarketDataParticipant) executeDualStreamStrategy(ctx context.Context, strategy *SyncStrategy) error {
	// Enable fallback streaming
	p.fallbackStream.mu.Lock()
	p.fallbackStream.isActive = true
	p.fallbackStream.mu.Unlock()

	// Continue both old and new streams simultaneously
	// Switch over after migration completes

	return nil
}

func (p *MarketDataParticipant) executeBufferReplayStrategy(ctx context.Context, strategy *SyncStrategy) error {
	// Start buffering
	atomic.StoreInt64(&p.isStreaming, 0)

	// Simulate migration time
	time.Sleep(strategy.MaxPauseDuration)

	// Replay buffered data
	err := p.replayBufferedData(ctx)
	if err != nil {
		return fmt.Errorf("buffer replay failed: %w", err)
	}

	// Resume streaming
	atomic.StoreInt64(&p.isStreaming, 1)

	return nil
}

func (p *MarketDataParticipant) replayBufferedData(ctx context.Context) error {
	p.streamBuffer.mu.RLock()
	defer p.streamBuffer.mu.RUnlock()

	p.logger.Infow("Replaying buffered data",
		"buffer_size", len(p.streamBuffer.buffer),
		"pair", p.pair)

	// Replay all buffered updates
	for _, update := range p.streamBuffer.buffer {
		for _, subscriber := range p.subscribers {
			if subscriber.IsActive() {
				err := subscriber.SendUpdate(update)
				if err != nil {
					p.logger.Warnw("Failed to replay update to subscriber",
						"subscriber", subscriber.GetID(), "error", err)
				}
			}
		}
	}

	return nil
}

func (p *MarketDataParticipant) validateDataContinuity(ctx context.Context) error {
	// Validate that data flow is continuous post-migration
	snapshot, err := p.createMarketDataSnapshot(ctx)
	if err != nil {
		return fmt.Errorf("failed to create post-migration snapshot: %w", err)
	}

	if snapshot.DataQuality.OverallQuality < 0.9 {
		return fmt.Errorf("data quality degraded after migration: %.2f", snapshot.DataQuality.OverallQuality)
	}

	return nil
}

func (p *MarketDataParticipant) updateSubscriberConfigurations(ctx context.Context) error {
	// Update subscriber configurations post-migration
	for id, subscriber := range p.subscribers {
		if !subscriber.IsActive() {
			err := subscriber.Resume()
			if err != nil {
				p.logger.Warnw("Failed to resume subscriber", "subscriber", id, "error", err)
			}
		}
	}

	return nil
}

func (p *MarketDataParticipant) cleanupTemporaryResources(ctx context.Context) error {
	// Clear stream buffers
	p.clearStreamBuffers()

	// Disable fallback streams
	p.fallbackStream.mu.Lock()
	p.fallbackStream.isActive = false
	p.fallbackStream.mu.Unlock()

	return nil
}

func (p *MarketDataParticipant) restoreOriginalStreaming(ctx context.Context) error {
	// Restore original streaming configuration
	atomic.StoreInt64(&p.isStreaming, 1)

	return nil
}

func (p *MarketDataParticipant) resumeAllFeeds(ctx context.Context) error {
	for name, feed := range p.dataFeeds {
		if err := feed.Resume(); err != nil {
			p.logger.Warnw("Failed to resume feed during abort", "feed", name, "error", err)
		}
	}

	return nil
}

func (p *MarketDataParticipant) disableFallbackStreams(ctx context.Context) error {
	p.fallbackStream.mu.Lock()
	p.fallbackStream.isActive = false
	p.fallbackStream.mu.Unlock()

	return nil
}

func (p *MarketDataParticipant) clearStreamBuffers() {
	p.streamBuffer.mu.Lock()
	p.streamBuffer.buffer = p.streamBuffer.buffer[:0]
	p.streamBuffer.mu.Unlock()
}

func (p *MarketDataParticipant) cleanup(ctx context.Context, migrationID uuid.UUID) error {
	p.logger.Infow("Cleaning up market data resources", "migration_id", migrationID, "pair", p.pair)

	// Clear buffers
	p.clearStreamBuffers()

	// Reset migration state
	p.mu.Lock()
	p.currentMigrationID = uuid.Nil
	p.preparationData = nil
	p.mu.Unlock()

	return nil
}

func (p *MarketDataParticipant) calculateDataQuality() *DataQualityMetrics {
	// In a real implementation, this would calculate actual data quality metrics
	return &DataQualityMetrics{
		Completeness:   0.998,
		Accuracy:       0.999,
		Timeliness:     0.995,
		Consistency:    0.997,
		OverallQuality: 0.997,
		LastUpdated:    time.Now(),
		SampleSize:     10000,
	}
}
