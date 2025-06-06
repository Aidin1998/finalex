// Package audit provides database models and supporting structures
package audit

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/Aidin1998/finalex/internal/compliance/interfaces"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// AuditEventRecord represents the database model for audit events
type AuditEventRecord struct {
	ID             uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	UserID         *uuid.UUID `gorm:"type:uuid;index" json:"user_id,omitempty"`
	EventType      string     `gorm:"type:varchar(100);not null;index" json:"event_type"`
	Category       string     `gorm:"type:varchar(50);not null;index" json:"category"`
	Severity       string     `gorm:"type:varchar(20);not null;index" json:"severity"`
	Description    string     `gorm:"type:text;not null" json:"description"`
	IPAddress      string     `gorm:"type:varchar(45);index" json:"ip_address,omitempty"`
	UserAgent      string     `gorm:"type:text" json:"user_agent,omitempty"`
	Resource       string     `gorm:"type:varchar(100);index" json:"resource,omitempty"`
	ResourceID     string     `gorm:"type:varchar(255);index" json:"resource_id,omitempty"`
	Action         string     `gorm:"type:varchar(50);index" json:"action,omitempty"`
	OldValues      string     `gorm:"type:jsonb" json:"old_values,omitempty"`
	NewValues      string     `gorm:"type:jsonb" json:"new_values,omitempty"`
	Metadata       string     `gorm:"type:jsonb" json:"metadata,omitempty"`
	RequestID      *uuid.UUID `gorm:"type:uuid;index" json:"request_id,omitempty"`
	SessionID      *uuid.UUID `gorm:"type:uuid;index" json:"session_id,omitempty"`
	CorrelationID  string     `gorm:"type:varchar(255);index" json:"correlation_id,omitempty"`
	Hash           string     `gorm:"type:varchar(64);not null;unique" json:"hash"`
	PreviousHash   string     `gorm:"type:varchar(64);index" json:"previous_hash"`
	ChainPosition  int64      `gorm:"autoIncrement" json:"chain_position"`
	Timestamp      time.Time  `gorm:"not null;index" json:"timestamp"`
	ProcessedBy    string     `gorm:"type:varchar(100)" json:"processed_by"`
	ProcessedAt    time.Time  `gorm:"not null;default:now()" json:"processed_at"`
	EncryptedData  []byte     `gorm:"type:bytea" json:"encrypted_data,omitempty"`
	CompressedData []byte     `gorm:"type:bytea" json:"compressed_data,omitempty"`
	ArchivedAt     *time.Time `gorm:"index" json:"archived_at,omitempty"`
	CreatedAt      time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt      time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName returns the table name for GORM
func (AuditEventRecord) TableName() string {
	return "audit_events"
}

// ChainMetadata represents chain integrity metadata
type ChainMetadata struct {
	ID         uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()"`
	StartHash  string    `gorm:"type:varchar(64);not null"`
	EndHash    string    `gorm:"type:varchar(64);not null"`
	EventCount int64     `gorm:"not null"`
	StartTime  time.Time `gorm:"not null"`
	EndTime    time.Time `gorm:"not null"`
	VerifiedAt time.Time `gorm:"not null"`
	IsValid    bool      `gorm:"not null"`
	Errors     string    `gorm:"type:jsonb"`
	CreatedAt  time.Time `gorm:"autoCreateTime"`
}

// BatchProcessor handles batch processing of audit events
type BatchProcessor struct {
	config     *Config
	logger     *zap.Logger
	eventBatch []*interfaces.AuditEvent
	batchMutex sync.Mutex
	flushTimer *time.Timer
	processCh  chan []*interfaces.AuditEvent
	stopCh     chan struct{}
}

// NewBatchProcessor creates a new batch processor
func NewBatchProcessor(config *Config, logger *zap.Logger) (*BatchProcessor, error) {
	return &BatchProcessor{
		config:     config,
		logger:     logger,
		eventBatch: make([]*interfaces.AuditEvent, 0, config.BatchSize),
		processCh:  make(chan []*interfaces.AuditEvent, 100),
		stopCh:     make(chan struct{}),
	}, nil
}

// Start starts the batch processor
func (bp *BatchProcessor) Start(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	bp.logger.Info("Starting batch processor")

	for {
		select {
		case <-ctx.Done():
			bp.flushBatch()
			return
		case <-bp.stopCh:
			bp.flushBatch()
			return
		case batch := <-bp.processCh:
			// Process batch
			if err := bp.processBatch(ctx, batch); err != nil {
				bp.logger.Error("Failed to process batch", zap.Error(err))
			}
		}
	}
}

// Stop stops the batch processor
func (bp *BatchProcessor) Stop() error {
	close(bp.stopCh)
	return nil
}

// AddEvent adds an event to the current batch
func (bp *BatchProcessor) AddEvent(event *interfaces.AuditEvent) {
	bp.batchMutex.Lock()
	defer bp.batchMutex.Unlock()

	bp.eventBatch = append(bp.eventBatch, event)

	// Check if batch is full
	if len(bp.eventBatch) >= bp.config.BatchSize {
		bp.flushBatchLocked()
	} else if bp.flushTimer == nil {
		// Start flush timer if this is the first event in the batch
		bp.flushTimer = time.AfterFunc(bp.config.BatchTimeout, func() {
			bp.batchMutex.Lock()
			defer bp.batchMutex.Unlock()
			bp.flushBatchLocked()
		})
	}
}

// flushBatch flushes the current batch (thread-safe)
func (bp *BatchProcessor) flushBatch() {
	bp.batchMutex.Lock()
	defer bp.batchMutex.Unlock()
	bp.flushBatchLocked()
}

// flushBatchLocked flushes the current batch (caller must hold mutex)
func (bp *BatchProcessor) flushBatchLocked() {
	if len(bp.eventBatch) == 0 {
		return
	}

	// Copy batch and reset
	batch := make([]*interfaces.AuditEvent, len(bp.eventBatch))
	copy(batch, bp.eventBatch)
	bp.eventBatch = bp.eventBatch[:0]

	// Stop flush timer
	if bp.flushTimer != nil {
		bp.flushTimer.Stop()
		bp.flushTimer = nil
	}

	// Send batch for processing
	select {
	case bp.processCh <- batch:
	default:
		bp.logger.Warn("Batch processing channel full, dropping batch", zap.Int("batch_size", len(batch)))
	}
}

// processBatch processes a batch of events
func (bp *BatchProcessor) processBatch(ctx context.Context, batch []*interfaces.AuditEvent) error {
	bp.logger.Debug("Processing event batch", zap.Int("size", len(batch)))

	// Here we would insert the batch into the database
	// This is a placeholder for the actual database insertion logic

	return nil
}

// ChainManager manages the tamper-evident chain
type ChainManager struct {
	db     *gorm.DB
	logger *zap.Logger
	mutex  sync.RWMutex
}

// NewChainManager creates a new chain manager
func NewChainManager(db *gorm.DB, logger *zap.Logger) (*ChainManager, error) {
	return &ChainManager{
		db:     db,
		logger: logger,
	}, nil
}

// GetLastHash gets the hash of the last event in the chain
func (cm *ChainManager) GetLastHash(ctx context.Context) (string, error) {
	var lastEvent AuditEventRecord
	err := cm.db.WithContext(ctx).
		Order("chain_position DESC").
		First(&lastEvent).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", nil // No events yet
		}
		return "", fmt.Errorf("failed to get last event: %w", err)
	}

	return lastEvent.Hash, nil
}

// VerifyChain verifies the integrity of the audit chain
func (cm *ChainManager) VerifyChain(ctx context.Context, from, to time.Time) (*interfaces.ChainVerification, error) {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	var events []AuditEventRecord
	err := cm.db.WithContext(ctx).
		Where("timestamp BETWEEN ? AND ?", from, to).
		Order("chain_position ASC").
		Find(&events).Error

	if err != nil {
		return nil, fmt.Errorf("failed to retrieve events for verification: %w", err)
	}

	verification := &interfaces.ChainVerification{
		Valid:            true,
		EventCount:       int64(len(events)),
		VerificationTime: time.Now(),
		Errors:           []string{},
	}

	if len(events) == 0 {
		return verification, nil
	}

	verification.StartHash = events[0].Hash
	verification.EndHash = events[len(events)-1].Hash

	// Verify chain integrity
	for i := 1; i < len(events); i++ {
		if events[i].PreviousHash != events[i-1].Hash {
			verification.Valid = false
			verification.Errors = append(verification.Errors,
				fmt.Sprintf("Chain break at position %d: expected previous hash %s, got %s",
					events[i].ChainPosition, events[i-1].Hash, events[i].PreviousHash))
		}
	}

	return verification, nil
}

// GetChainHash gets the chain hash at a specific timestamp
func (cm *ChainManager) GetChainHash(ctx context.Context, timestamp time.Time) (string, error) {
	var event AuditEventRecord
	err := cm.db.WithContext(ctx).
		Where("timestamp <= ?", timestamp).
		Order("timestamp DESC").
		First(&event).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", nil
		}
		return "", fmt.Errorf("failed to get event at timestamp: %w", err)
	}

	return event.Hash, nil
}

// QuickIntegrityCheck performs a quick integrity check
func (cm *ChainManager) QuickIntegrityCheck(ctx context.Context) (bool, error) {
	// Check last 1000 events for integrity
	var events []AuditEventRecord
	err := cm.db.WithContext(ctx).
		Order("chain_position DESC").
		Limit(1000).
		Find(&events).Error

	if err != nil {
		return false, fmt.Errorf("failed to retrieve events for quick check: %w", err)
	}

	if len(events) <= 1 {
		return true, nil
	}

	// Reverse the slice to check from oldest to newest
	for i := len(events)/2 - 1; i >= 0; i-- {
		opp := len(events) - 1 - i
		events[i], events[opp] = events[opp], events[i]
	}

	// Check chain integrity
	for i := 1; i < len(events); i++ {
		if events[i].PreviousHash != events[i-1].Hash {
			return false, nil
		}
	}

	return true, nil
}

// Encryptor handles encryption of sensitive audit data
type Encryptor struct {
	key []byte
	gcm cipher.AEAD
}

// NewEncryptor creates a new encryptor
func NewEncryptor() (*Encryptor, error) {
	// In production, this key should be loaded from a secure key management system
	key := make([]byte, 32) // AES-256
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("failed to generate encryption key: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	return &Encryptor{
		key: key,
		gcm: gcm,
	}, nil
}

// Encrypt encrypts data
func (e *Encryptor) Encrypt(data []byte) ([]byte, error) {
	nonce := make([]byte, e.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := e.gcm.Seal(nonce, nonce, data, nil)
	return ciphertext, nil
}

// Decrypt decrypts data
func (e *Encryptor) Decrypt(data []byte) ([]byte, error) {
	nonceSize := e.gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := e.gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}

	return plaintext, nil
}

// CompressionManager handles compression of audit data
type CompressionManager struct {
	// Placeholder for compression logic
	// Could use gzip, zstd, or other compression algorithms
}

// NewCompressionManager creates a new compression manager
func NewCompressionManager() *CompressionManager {
	return &CompressionManager{}
}

// Compress compresses data
func (cm *CompressionManager) Compress(data []byte) ([]byte, error) {
	// Placeholder - implement actual compression
	return data, nil
}

// Decompress decompresses data
func (cm *CompressionManager) Decompress(data []byte) ([]byte, error) {
	// Placeholder - implement actual decompression
	return data, nil
}

// Metrics tracks audit service metrics
type Metrics struct {
	eventsReceived   int64
	eventsProcessed  int64
	eventsDropped    int64
	processingErrors int64
	totalLatency     time.Duration
	latencyCount     int64
	mutex            sync.RWMutex
	lastMetricsReset time.Time
}

// NewMetrics creates new metrics
func NewMetrics() *Metrics {
	return &Metrics{
		lastMetricsReset: time.Now(),
	}
}

// IncrementEventsReceived increments events received counter
func (m *Metrics) IncrementEventsReceived() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.eventsReceived++
}

// IncrementEventsProcessed increments events processed counter
func (m *Metrics) IncrementEventsProcessed() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.eventsProcessed++
}

// IncrementEventsDropped increments events dropped counter
func (m *Metrics) IncrementEventsDropped() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.eventsDropped++
}

// IncrementProcessingErrors increments processing errors counter
func (m *Metrics) IncrementProcessingErrors() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.processingErrors++
}

// RecordLatency records processing latency
func (m *Metrics) RecordLatency(latency time.Duration) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.totalLatency += latency
	m.latencyCount++
}

// GetEventsPerSecond returns events per second rate
func (m *Metrics) GetEventsPerSecond() float64 {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	duration := time.Since(m.lastMetricsReset).Seconds()
	if duration == 0 {
		return 0
	}
	return float64(m.eventsProcessed) / duration
}

// GetAverageLatency returns average processing latency
func (m *Metrics) GetAverageLatency() time.Duration {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if m.latencyCount == 0 {
		return 0
	}
	return m.totalLatency / time.Duration(m.latencyCount)
}

// GetErrorRate returns error rate as percentage
func (m *Metrics) GetErrorRate() float64 {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	total := m.eventsReceived
	if total == 0 {
		return 0
	}
	return float64(m.processingErrors) / float64(total) * 100
}
