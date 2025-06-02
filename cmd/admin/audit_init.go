package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/Aidin1998/pincex_unified/internal/audit"
	"github.com/Aidin1998/pincex_unified/internal/database"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// initAuditSystem initializes the complete audit logging system
func initAuditSystem(db *gorm.DB, logger *zap.Logger, config *audit.AuditConfig) (*audit.AuditService, *audit.Handlers, error) {
	// Get encryption key from environment
	encryptionKey := os.Getenv("AUDIT_ENCRYPTION_KEY")
	if encryptionKey == "" {
		logger.Warn("AUDIT_ENCRYPTION_KEY not set, using default key - NOT FOR PRODUCTION")
		encryptionKey = "pincex-audit-default-key-change-in-production"
	}

	// Initialize encryption service
	encryptionSvc, err := audit.NewEncryptionService(encryptionKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize encryption service: %w", err)
	}

	// Initialize sensitive data processor
	dataProcessor := audit.NewSensitiveDataProcessor(encryptionSvc)

	// Initialize audit service
	auditSvc, err := audit.NewAuditService(db, logger, encryptionSvc, config)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize audit service: %w", err)
	}

	// Initialize forensic engine
	forensicEngine := audit.NewForensicEngine(db, logger, dataProcessor)

	// Initialize audit handlers
	auditHandlers := audit.NewHandlers(logger, auditSvc, forensicEngine)

	// Start the audit service
	ctx := context.Background()
	if err := auditSvc.Start(ctx); err != nil {
		return nil, nil, fmt.Errorf("failed to start audit service: %w", err)
	}

	logger.Info("Audit system initialized successfully",
		zap.Bool("encryption_enabled", config.EnableEncryption),
		zap.Bool("tamper_proof", config.TamperProofStorage),
		zap.Bool("async_logging", config.AsyncLogging),
		zap.Int("batch_size", config.BatchSize),
	)

	return auditSvc, auditHandlers, nil
}

// createDefaultAuditConfig creates a default audit configuration
func createDefaultAuditConfig() *audit.AuditConfig {
	return &audit.AuditConfig{
		EnableEncryption:         true,
		TamperProofStorage:       true,
		AsyncLogging:             true,
		BatchSize:                100,
		FlushInterval:            30 * 1000000000,               // 30 seconds in nanoseconds
		RetentionPeriod:          2555 * 24 * 3600 * 1000000000, // 7 years in nanoseconds
		ComplianceMode:           true,
		DetailLevel:              audit.DetailLevelStandard,
		EnableForensicCapability: true,
	}
}

// setupAuditTables ensures audit tables exist and are properly configured
func setupAuditTables(db *gorm.DB) error {
	// Auto-migrate audit tables
	err := db.AutoMigrate(
		&audit.AuditEvent{},
		&audit.AuditSummary{},
		&audit.AuditConfig{},
		&audit.AuditIntegrityLog{},
	)
	if err != nil {
		return fmt.Errorf("failed to migrate audit tables: %w", err)
	}

	// Create indexes for performance
	// Note: In production, these should be created via migration scripts
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_audit_events_timestamp ON audit_events(timestamp)",
		"CREATE INDEX IF NOT EXISTS idx_audit_events_user_id ON audit_events(user_id)",
		"CREATE INDEX IF NOT EXISTS idx_audit_events_action_type ON audit_events(action_type)",
		"CREATE INDEX IF NOT EXISTS idx_audit_events_risk_score ON audit_events(risk_score)",
		"CREATE INDEX IF NOT EXISTS idx_audit_events_user_timestamp ON audit_events(user_id, timestamp)",
	}

	for _, indexSQL := range indexes {
		if err := db.Exec(indexSQL).Error; err != nil {
			log.Printf("Warning: Failed to create index: %v", err)
		}
	}

	return nil
}

// Example usage in main application
func main() {
	// Initialize logger
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	// Initialize database (example - replace with your actual DB initialization)
	db, err := database.Connect(database.Config{
		Host:     os.Getenv("DB_HOST"),
		Port:     os.Getenv("DB_PORT"),
		User:     os.Getenv("DB_USER"),
		Password: os.Getenv("DB_PASSWORD"),
		Database: os.Getenv("DB_NAME"),
		SSLMode:  "require",
	})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Setup audit tables
	if err := setupAuditTables(db); err != nil {
		log.Fatalf("Failed to setup audit tables: %v", err)
	}

	// Create audit configuration
	config := createDefaultAuditConfig()

	// Initialize audit system
	auditSvc, auditHandlers, err := initAuditSystem(db, logger, config)
	if err != nil {
		log.Fatalf("Failed to initialize audit system: %v", err)
	}

	// Example: Log an audit event
	ctx := context.Background()
	event := &audit.AuditEvent{
		EventType:       audit.EventSystemStartup,
		Category:        audit.CategorySystemAdmin,
		ActorID:         "system",
		ActorType:       audit.ActorTypeSystem,
		Resource:        "audit_system",
		Action:          "startup",
		Description:     "Audit system started successfully",
		RiskScore:       10,
		BusinessImpact:  audit.BusinessImpactLow,
		ComplianceFlags: []string{"SOX", "SOC2"},
	}

	if err := auditSvc.LogEvent(ctx, event); err != nil {
		logger.Error("Failed to log startup event", zap.Error(err))
	}

	logger.Info("Audit system startup completed successfully")

	// In a real application, you would integrate these into your server
	// server := NewServer(logger, authSvc, ..., auditSvc, auditHandlers)
	// server.Router()

	// Example of accessing audit handlers for testing
	_ = auditHandlers
}

// Example environment variables for production deployment
/*
Required Environment Variables:

# Database Configuration
DB_HOST=localhost
DB_PORT=5432
DB_USER=pincex_audit
DB_PASSWORD=secure_password
DB_NAME=pincex_audit

# Audit Configuration
AUDIT_ENCRYPTION_KEY=your-32-character-encryption-key-here
AUDIT_BATCH_SIZE=100
AUDIT_FLUSH_INTERVAL=30s
AUDIT_DETAIL_LEVEL=standard

# Optional Configuration
AUDIT_RETENTION_DAYS=2555
AUDIT_COMPLIANCE_MODE=true
AUDIT_ASYNC_LOGGING=true
AUDIT_ENABLE_FORENSICS=true

# Performance Tuning
AUDIT_WORKER_COUNT=2
AUDIT_QUEUE_SIZE=10000
AUDIT_CONNECTION_POOL=10

# Security
AUDIT_ENABLE_ENCRYPTION=true
AUDIT_TAMPER_PROOF=true
AUDIT_INTEGRITY_CHECKS=true
*/
