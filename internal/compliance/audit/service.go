// Package audit provides high-throughput tamper-evident audit logging
package audit

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/Aidin1998/finalex/internal/compliance/interfaces"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Service implements high-throughput tamper-evident audit logging
type Service struct {
	db             *gorm.DB
	logger         *zap.Logger
	config         *Config
	batchProcessor *BatchProcessor
	chainManager   *ChainManager
	encryptor      *Encryptor
	compressionMgr *CompressionManager
	mu             sync.RWMutex
	isRunning      bool
	shutdownCh     chan struct{}
	eventQueue     chan *interfaces.AuditEvent
	processingWg   sync.WaitGroup
	metrics        *Metrics
}

// Config holds audit service configuration
type Config struct {
	BatchSize            int           `yaml:"batch_size" json:"batch_size"`
	BatchTimeout         time.Duration `yaml:"batch_timeout" json:"batch_timeout"`
	WorkerCount          int           `yaml:"worker_count" json:"worker_count"`
	QueueSize            int           `yaml:"queue_size" json:"queue_size"`
	EnableEncryption     bool          `yaml:"enable_encryption" json:"enable_encryption"`
	EnableCompression    bool          `yaml:"enable_compression" json:"enable_compression"`
	ArchiveAfterDays     int           `yaml:"archive_after_days" json:"archive_after_days"`
	VerificationInterval time.Duration `yaml:"verification_interval" json:"verification_interval"`
	RetentionDays        int           `yaml:"retention_days" json:"retention_days"`
	EnableMetrics        bool          `yaml:"enable_metrics" json:"enable_metrics"`
}

// DefaultConfig returns default audit service configuration
func DefaultConfig() *Config {
	return &Config{
		BatchSize:            1000,
		BatchTimeout:         5 * time.Second,
		WorkerCount:          8,
		QueueSize:            10000,
		EnableEncryption:     true,
		EnableCompression:    true,
		ArchiveAfterDays:     30,
		VerificationInterval: time.Hour,
		RetentionDays:        2555, // 7 years
		EnableMetrics:        true,
	}
}

// NewService creates a new audit service
func NewService(db *gorm.DB, logger *zap.Logger, config *Config) (*Service, error) {
	if db == nil {
		return nil, fmt.Errorf("database connection is required")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}
	if config == nil {
		config = DefaultConfig()
	}

	service := &Service{
		db:         db,
		logger:     logger,
		config:     config,
		shutdownCh: make(chan struct{}),
		eventQueue: make(chan *interfaces.AuditEvent, config.QueueSize),
		metrics:    NewMetrics(),
	}

	// Initialize components
	var err error
	service.batchProcessor, err = NewBatchProcessor(config, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create batch processor: %w", err)
	}

	service.chainManager, err = NewChainManager(db, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create chain manager: %w", err)
	}

	if config.EnableEncryption {
		service.encryptor, err = NewEncryptor()
		if err != nil {
			return nil, fmt.Errorf("failed to create encryptor: %w", err)
		}
	}

	if config.EnableCompression {
		service.compressionMgr = NewCompressionManager()
	}

	// Create database tables
	if err := service.createTables(); err != nil {
		return nil, fmt.Errorf("failed to create tables: %w", err)
	}

	return service, nil
}

// Start starts the audit service
func (s *Service) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isRunning {
		return fmt.Errorf("audit service is already running")
	}

	s.logger.Info("Starting audit service",
		zap.Int("worker_count", s.config.WorkerCount),
		zap.Int("batch_size", s.config.BatchSize),
		zap.Duration("batch_timeout", s.config.BatchTimeout))

	// Start workers
	for i := 0; i < s.config.WorkerCount; i++ {
		s.processingWg.Add(1)
		go s.eventWorker(ctx, i)
	}

	// Start batch processor
	s.processingWg.Add(1)
	go s.batchProcessor.Start(ctx, &s.processingWg)

	// Start chain verification routine
	if s.config.VerificationInterval > 0 {
		s.processingWg.Add(1)
		go s.chainVerificationWorker(ctx)
	}

	// Start metrics collection
	if s.config.EnableMetrics {
		s.processingWg.Add(1)
		go s.metricsWorker(ctx)
	}

	s.isRunning = true
	s.logger.Info("Audit service started successfully")
	return nil
}

// Stop stops the audit service gracefully
func (s *Service) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isRunning {
		return nil
	}

	s.logger.Info("Stopping audit service...")

	// Signal shutdown
	close(s.shutdownCh)

	// Close event queue
	close(s.eventQueue)

	// Wait for workers to finish with timeout
	done := make(chan struct{})
	go func() {
		s.processingWg.Wait()
		close(done)
	}()

	select {
	case <-done:
		s.logger.Info("All workers stopped successfully")
	case <-time.After(30 * time.Second):
		s.logger.Warn("Timeout waiting for workers to stop")
	}

	// Stop batch processor
	if err := s.batchProcessor.Stop(); err != nil {
		s.logger.Error("Error stopping batch processor", zap.Error(err))
	}

	s.isRunning = false
	s.logger.Info("Audit service stopped")
	return nil
}

// LogEvent logs a single audit event
func (s *Service) LogEvent(ctx context.Context, event *interfaces.AuditEvent) error {
	if event == nil {
		return fmt.Errorf("event cannot be nil")
	}

	// Validate event
	if err := s.validateEvent(event); err != nil {
		return fmt.Errorf("invalid event: %w", err)
	}

	// Set defaults
	if event.ID == uuid.Nil {
		event.ID = uuid.New()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}

	// Calculate hash
	if err := s.calculateEventHash(event); err != nil {
		return fmt.Errorf("failed to calculate event hash: %w", err)
	}

	// Queue event for processing
	select {
	case s.eventQueue <- event:
		s.metrics.IncrementEventsReceived()
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-s.shutdownCh:
		return fmt.Errorf("audit service is shutting down")
	default:
		s.metrics.IncrementEventsDropped()
		return fmt.Errorf("event queue is full")
	}
}

// LogBatch logs multiple audit events in a batch
func (s *Service) LogBatch(ctx context.Context, events []*interfaces.AuditEvent) error {
	if len(events) == 0 {
		return nil
	}

	// Validate and prepare events
	for _, event := range events {
		if err := s.validateEvent(event); err != nil {
			return fmt.Errorf("invalid event in batch: %w", err)
		}

		if event.ID == uuid.Nil {
			event.ID = uuid.New()
		}
		if event.Timestamp.IsZero() {
			event.Timestamp = time.Now().UTC()
		}

		if err := s.calculateEventHash(event); err != nil {
			return fmt.Errorf("failed to calculate event hash: %w", err)
		}
	}

	// Queue events
	for _, event := range events {
		select {
		case s.eventQueue <- event:
			s.metrics.IncrementEventsReceived()
		case <-ctx.Done():
			return ctx.Err()
		case <-s.shutdownCh:
			return fmt.Errorf("audit service is shutting down")
		default:
			s.metrics.IncrementEventsDropped()
			return fmt.Errorf("event queue is full")
		}
	}

	return nil
}

// GetEvents retrieves audit events based on filter
func (s *Service) GetEvents(ctx context.Context, filter *interfaces.AuditFilter) ([]*interfaces.AuditEvent, error) {
	if filter == nil {
		return nil, fmt.Errorf("filter cannot be nil")
	}

	query := s.db.WithContext(ctx).Model(&AuditEventRecord{})

	// Apply filters
	if filter.UserID != nil {
		query = query.Where("user_id = ?", *filter.UserID)
	}
	if filter.EventType != "" {
		query = query.Where("event_type = ?", filter.EventType)
	}
	if filter.Category != "" {
		query = query.Where("category = ?", filter.Category)
	}
	if filter.Severity != "" {
		query = query.Where("severity = ?", filter.Severity)
	}
	if filter.IPAddress != "" {
		query = query.Where("ip_address = ?", filter.IPAddress)
	}
	if filter.Resource != "" {
		query = query.Where("resource = ?", filter.Resource)
	}
	if filter.Action != "" {
		query = query.Where("action = ?", filter.Action)
	}

	// Time range
	if !filter.From.IsZero() {
		query = query.Where("timestamp >= ?", filter.From)
	}
	if !filter.To.IsZero() {
		query = query.Where("timestamp <= ?", filter.To)
	}

	// Pagination
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	} else {
		query = query.Limit(1000) // Default limit
	}
	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}

	// Order by timestamp
	query = query.Order("timestamp DESC")

	var records []AuditEventRecord
	if err := query.Find(&records).Error; err != nil {
		return nil, fmt.Errorf("failed to query events: %w", err)
	}

	// Convert to domain objects
	events := make([]*interfaces.AuditEvent, len(records))
	for i, record := range records {
		event, err := s.recordToEvent(&record)
		if err != nil {
			s.logger.Error("Failed to convert record to event", zap.Error(err), zap.String("event_id", record.ID.String()))
			continue
		}
		events[i] = event
	}

	return events, nil
}

// GetEventsByUser retrieves events for a specific user
func (s *Service) GetEventsByUser(ctx context.Context, userID uuid.UUID, from, to time.Time) ([]*interfaces.AuditEvent, error) {
	filter := &interfaces.AuditFilter{
		UserID: &userID,
		From:   from,
		To:     to,
		Limit:  10000,
	}
	return s.GetEvents(ctx, filter)
}

// GetEventsByType retrieves events of a specific type
func (s *Service) GetEventsByType(ctx context.Context, eventType string, from, to time.Time) ([]*interfaces.AuditEvent, error) {
	filter := &interfaces.AuditFilter{
		EventType: eventType,
		From:      from,
		To:        to,
		Limit:     10000,
	}
	return s.GetEvents(ctx, filter)
}

// VerifyChain verifies the integrity of the audit chain
func (s *Service) VerifyChain(ctx context.Context, from, to time.Time) (*interfaces.ChainVerification, error) {
	return s.chainManager.VerifyChain(ctx, from, to)
}

// GetChainHash gets the chain hash at a specific timestamp
func (s *Service) GetChainHash(ctx context.Context, timestamp time.Time) (string, error) {
	return s.chainManager.GetChainHash(ctx, timestamp)
}

// ArchiveEvents archives old events
func (s *Service) ArchiveEvents(ctx context.Context, before time.Time) error {
	s.logger.Info("Starting event archival", zap.Time("before", before))

	// Move old events to archive table
	result := s.db.WithContext(ctx).Exec(`
		INSERT INTO audit_event_archive 
		SELECT * FROM audit_events 
		WHERE timestamp < ? AND archived_at IS NULL
	`, before)

	if result.Error != nil {
		return fmt.Errorf("failed to archive events: %w", result.Error)
	}

	s.logger.Info("Events archived", zap.Int64("count", result.RowsAffected))

	// Mark events as archived
	if err := s.db.WithContext(ctx).
		Model(&AuditEventRecord{}).
		Where("timestamp < ? AND archived_at IS NULL", before).
		Update("archived_at", time.Now()).Error; err != nil {
		return fmt.Errorf("failed to mark events as archived: %w", err)
	}

	return nil
}

// GetMetrics returns audit service metrics
func (s *Service) GetMetrics(ctx context.Context) (*interfaces.AuditMetrics, error) {
	var totalEvents int64
	if err := s.db.WithContext(ctx).Model(&AuditEventRecord{}).Count(&totalEvents).Error; err != nil {
		return nil, fmt.Errorf("failed to count total events: %w", err)
	}

	chainIntegrity, err := s.chainManager.QuickIntegrityCheck(ctx)
	if err != nil {
		s.logger.Warn("Failed to check chain integrity", zap.Error(err))
		chainIntegrity = false
	}

	return &interfaces.AuditMetrics{
		TotalEvents:       totalEvents,
		EventsPerSecond:   s.metrics.GetEventsPerSecond(),
		ChainIntegrity:    chainIntegrity,
		StorageUsage:      s.getStorageUsage(ctx),
		ProcessingLatency: s.metrics.GetAverageLatency(),
		ErrorRate:         s.metrics.GetErrorRate(),
	}, nil
}

// HealthCheck checks the health of the audit service
func (s *Service) HealthCheck(ctx context.Context) error {
	s.mu.RLock()
	isRunning := s.isRunning
	s.mu.RUnlock()

	if !isRunning {
		return fmt.Errorf("audit service is not running")
	}

	// Check database connectivity
	if err := s.db.WithContext(ctx).Exec("SELECT 1").Error; err != nil {
		return fmt.Errorf("database connectivity check failed: %w", err)
	}

	// Check queue health
	queueDepth := len(s.eventQueue)
	if queueDepth > s.config.QueueSize*90/100 { // 90% threshold
		return fmt.Errorf("event queue is nearly full: %d/%d", queueDepth, s.config.QueueSize)
	}

	return nil
}

// Helper methods

func (s *Service) validateEvent(event *interfaces.AuditEvent) error {
	if event.EventType == "" {
		return fmt.Errorf("event_type is required")
	}
	if event.Category == "" {
		return fmt.Errorf("category is required")
	}
	if event.Severity == "" {
		return fmt.Errorf("severity is required")
	}
	if event.Description == "" {
		return fmt.Errorf("description is required")
	}
	return nil
}

func (s *Service) calculateEventHash(event *interfaces.AuditEvent) error {
	// Create hash input
	hashInput := struct {
		UserID       *uuid.UUID             `json:"user_id,omitempty"`
		EventType    string                 `json:"event_type"`
		Category     string                 `json:"category"`
		Severity     string                 `json:"severity"`
		Description  string                 `json:"description"`
		IPAddress    string                 `json:"ip_address,omitempty"`
		Resource     string                 `json:"resource,omitempty"`
		ResourceID   string                 `json:"resource_id,omitempty"`
		Action       string                 `json:"action,omitempty"`
		OldValues    map[string]interface{} `json:"old_values,omitempty"`
		NewValues    map[string]interface{} `json:"new_values,omitempty"`
		Metadata     map[string]interface{} `json:"metadata,omitempty"`
		Timestamp    time.Time              `json:"timestamp"`
		PreviousHash string                 `json:"previous_hash"`
	}{
		UserID:       event.UserID,
		EventType:    event.EventType,
		Category:     event.Category,
		Severity:     event.Severity,
		Description:  event.Description,
		IPAddress:    event.IPAddress,
		Resource:     event.Resource,
		ResourceID:   event.ResourceID,
		Action:       event.Action,
		OldValues:    event.OldValues,
		NewValues:    event.NewValues,
		Metadata:     event.Metadata,
		Timestamp:    event.Timestamp,
		PreviousHash: event.PreviousHash,
	}

	// Convert to JSON
	data, err := json.Marshal(hashInput)
	if err != nil {
		return fmt.Errorf("failed to marshal event for hashing: %w", err)
	}

	// Calculate SHA-256 hash
	hash := sha256.Sum256(data)
	event.Hash = fmt.Sprintf("%x", hash)

	return nil
}

func (s *Service) getStorageUsage(ctx context.Context) int64 {
	var size int64
	s.db.WithContext(ctx).Raw(`
		SELECT pg_total_relation_size('audit_events') + 
		       COALESCE(pg_total_relation_size('audit_event_archive'), 0) as total_size
	`).Scan(&size)
	return size
}
