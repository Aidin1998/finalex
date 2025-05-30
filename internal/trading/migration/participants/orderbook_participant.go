// =============================
// Order Book Migration Participant
// =============================
// This participant handles the actual order book migration process including
// snapshot validation, data migration, and consistency checks.

package participants

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/trading/migration"
	"github.com/Aidin1998/pincex_unified/internal/trading/orderbook"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// OrderBookParticipant handles order book migration operations
type OrderBookParticipant struct {
	id           string
	pair         string
	oldOrderBook orderbook.OrderBookInterface
	newOrderBook orderbook.OrderBookInterface
	adaptiveBook *orderbook.AdaptiveOrderBook
	logger       *zap.SugaredLogger

	// Migration state
	currentMigrationID uuid.UUID
	preparationData    *OrderBookPreparationData
	isHealthy          int64 // atomic bool
	lastHeartbeat      int64 // atomic timestamp

	// Synchronization
	mu          sync.RWMutex
	migrationMu sync.Mutex // prevents concurrent migrations
}

// OrderBookPreparationData contains data prepared during the prepare phase
type OrderBookPreparationData struct {
	Snapshot        *migration.OrdersSnapshot `json:"snapshot"`
	ValidationData  *ValidationData           `json:"validation_data"`
	MigrationPlan   *MigrationPlan            `json:"migration_plan"`
	BackupInfo      *BackupInfo               `json:"backup_info"`
	PreparationTime time.Time                 `json:"preparation_time"`
}

// ValidationData contains validation information
type ValidationData struct {
	OrderCount          int64                  `json:"order_count"`
	TotalVolume         decimal.Decimal        `json:"total_volume"`
	BidAskSpread        decimal.Decimal        `json:"bid_ask_spread"`
	TopBids             []orderbook.PriceLevel `json:"top_bids"`
	TopAsks             []orderbook.PriceLevel `json:"top_asks"`
	IntegrityChecksum   string                 `json:"integrity_checksum"`
	ConsistencyChecks   map[string]bool        `json:"consistency_checks"`
	PerformanceBaseline *PerformanceBaseline   `json:"performance_baseline"`
}

// MigrationPlan defines how the migration will be executed
type MigrationPlan struct {
	Strategy          string          `json:"strategy"`
	BatchSize         int             `json:"batch_size"`
	EstimatedDuration time.Duration   `json:"estimated_duration"`
	RiskAssessment    *RiskAssessment `json:"risk_assessment"`
	FallbackStrategy  string          `json:"fallback_strategy"`
	DowntimeEstimate  time.Duration   `json:"downtime_estimate"`
}

// BackupInfo contains backup information for rollback
type BackupInfo struct {
	BackupID        uuid.UUID `json:"backup_id"`
	BackupLocation  string    `json:"backup_location"`
	BackupSize      int64     `json:"backup_size"`
	BackupTime      time.Time `json:"backup_time"`
	BackupChecksum  string    `json:"backup_checksum"`
	RestoreCommands []string  `json:"restore_commands"`
}

// RiskAssessment evaluates migration risks
type RiskAssessment struct {
	OverallRisk        string            `json:"overall_risk"` // low, medium, high
	RiskFactors        []string          `json:"risk_factors"`
	MitigationActions  []string          `json:"mitigation_actions"`
	SuccessProbability float64           `json:"success_probability"`
	ImpactAssessment   *ImpactAssessment `json:"impact_assessment"`
}

// ImpactAssessment evaluates potential impact
type ImpactAssessment struct {
	PerformanceImpact  string `json:"performance_impact"`
	AvailabilityImpact string `json:"availability_impact"`
	DataIntegrityRisk  string `json:"data_integrity_risk"`
	UserExperienceRisk string `json:"user_experience_risk"`
	EstimatedDowntime  int64  `json:"estimated_downtime_ms"`
}

// PerformanceBaseline captures current performance metrics
type PerformanceBaseline struct {
	AvgLatencyMs    float64   `json:"avg_latency_ms"`
	OrdersPerSecond float64   `json:"orders_per_second"`
	MemoryUsageMB   float64   `json:"memory_usage_mb"`
	CPUUsagePercent float64   `json:"cpu_usage_percent"`
	ErrorRate       float64   `json:"error_rate"`
	MeasurementTime time.Time `json:"measurement_time"`
	SampleSize      int64     `json:"sample_size"`
}

// NewOrderBookParticipant creates a new order book migration participant
func NewOrderBookParticipant(
	pair string,
	oldOrderBook orderbook.OrderBookInterface,
	newOrderBook orderbook.OrderBookInterface,
	logger *zap.SugaredLogger,
) *OrderBookParticipant {
	p := &OrderBookParticipant{
		id:           fmt.Sprintf("orderbook_%s", pair),
		pair:         pair,
		oldOrderBook: oldOrderBook,
		newOrderBook: newOrderBook,
		logger:       logger,
	}

	// Set initial health status
	atomic.StoreInt64(&p.isHealthy, 1)
	atomic.StoreInt64(&p.lastHeartbeat, time.Now().UnixNano())

	// If we have an adaptive order book, use it
	if adaptiveOB, ok := oldOrderBook.(*orderbook.AdaptiveOrderBook); ok {
		p.adaptiveBook = adaptiveOB
	}

	return p
}

// GetID returns the participant identifier
func (p *OrderBookParticipant) GetID() string {
	return p.id
}

// GetType returns the participant type
func (p *OrderBookParticipant) GetType() string {
	return "order_book"
}

// Prepare implements the prepare phase of two-phase commit
func (p *OrderBookParticipant) Prepare(ctx context.Context, migrationID uuid.UUID, config *migration.MigrationConfig) (*migration.ParticipantState, error) {
	p.migrationMu.Lock()
	defer p.migrationMu.Unlock()

	p.logger.Infow("Starting prepare phase for order book migration",
		"migration_id", migrationID,
		"pair", p.pair,
		"target_implementation", config.TargetImplementation,
	)

	startTime := time.Now()
	p.currentMigrationID = migrationID

	// Step 1: Perform comprehensive health check
	healthCheck, err := p.performDetailedHealthCheck(ctx)
	if err != nil {
		return p.createFailedState("health check failed", err), nil
	}

	if healthCheck.Status != "healthy" {
		return p.createFailedState("system not healthy for migration", fmt.Errorf("health check failed: %s", healthCheck.Message)), nil
	}

	// Step 2: Create performance baseline
	baseline, err := p.capturePerformanceBaseline(ctx)
	if err != nil {
		p.logger.Warnw("Failed to capture performance baseline", "error", err)
		// Continue anyway - this is not critical
	}

	// Step 3: Create order book snapshot
	snapshot, err := p.createOrderBookSnapshot(ctx)
	if err != nil {
		return p.createFailedState("snapshot creation failed", err), nil
	}

	// Step 4: Validate data integrity
	validationData, err := p.validateDataIntegrity(ctx, snapshot)
	if err != nil {
		return p.createFailedState("data validation failed", err), nil
	}

	// Step 5: Assess migration risks
	riskAssessment := p.assessMigrationRisks(config, validationData)
	if riskAssessment.OverallRisk == "high" && !config.ValidationEnabled {
		return p.createFailedState("high risk migration rejected", fmt.Errorf("risk assessment failed: %+v", riskAssessment)), nil
	}

	// Step 6: Create migration plan
	migrationPlan := p.createMigrationPlan(config, validationData, riskAssessment)

	// Step 7: Create backup
	backupInfo, err := p.createBackup(ctx, snapshot)
	if err != nil {
		return p.createFailedState("backup creation failed", err), nil
	}

	// Step 8: Store preparation data
	p.preparationData = &OrderBookPreparationData{
		Snapshot:        snapshot,
		ValidationData:  validationData,
		MigrationPlan:   migrationPlan,
		BackupInfo:      backupInfo,
		PreparationTime: startTime,
	}

	preparationDuration := time.Since(startTime)
	p.logger.Infow("Prepare phase completed successfully",
		"migration_id", migrationID,
		"pair", p.pair,
		"duration", preparationDuration,
		"orders_count", snapshot.TotalOrders,
		"risk_level", riskAssessment.OverallRisk,
	)

	// Return successful state
	state := &migration.ParticipantState{
		ID:              p.id,
		Type:            p.GetType(),
		Vote:            migration.VoteYes,
		VoteTime:        &startTime,
		LastHeartbeat:   time.Now(),
		IsHealthy:       true,
		OrdersSnapshot:  snapshot,
		PreparationData: p.preparationData,
	}

	return state, nil
}

// Commit implements the commit phase of two-phase commit
func (p *OrderBookParticipant) Commit(ctx context.Context, migrationID uuid.UUID) error {
	p.migrationMu.Lock()
	defer p.migrationMu.Unlock()

	if p.currentMigrationID != migrationID {
		return fmt.Errorf("migration ID mismatch: expected %s, got %s", p.currentMigrationID, migrationID)
	}

	if p.preparationData == nil {
		return fmt.Errorf("no preparation data available for migration %s", migrationID)
	}

	p.logger.Infow("Starting commit phase for order book migration",
		"migration_id", migrationID,
		"pair", p.pair,
	)

	startTime := time.Now()

	// Step 1: Final health check
	if !p.isSystemHealthy() {
		return fmt.Errorf("system not healthy for commit")
	}

	// Step 2: Execute migration based on strategy
	switch p.preparationData.MigrationPlan.Strategy {
	case "atomic_switch":
		err := p.executeAtomicSwitch(ctx)
		if err != nil {
			return fmt.Errorf("atomic switch failed: %w", err)
		}
	case "gradual_rollout":
		err := p.executeGradualRollout(ctx)
		if err != nil {
			return fmt.Errorf("gradual rollout failed: %w", err)
		}
	default:
		return fmt.Errorf("unknown migration strategy: %s", p.preparationData.MigrationPlan.Strategy)
	}

	// Step 3: Validate migration success
	err := p.validateMigrationSuccess(ctx)
	if err != nil {
		return fmt.Errorf("migration validation failed: %w", err)
	}

	commitDuration := time.Since(startTime)
	p.logger.Infow("Commit phase completed successfully",
		"migration_id", migrationID,
		"pair", p.pair,
		"duration", commitDuration,
		"strategy", p.preparationData.MigrationPlan.Strategy,
	)

	return nil
}

// Abort implements the abort phase of two-phase commit
func (p *OrderBookParticipant) Abort(ctx context.Context, migrationID uuid.UUID) error {
	p.migrationMu.Lock()
	defer p.migrationMu.Unlock()

	p.logger.Infow("Starting abort phase for order book migration",
		"migration_id", migrationID,
		"pair", p.pair,
	)

	// Step 1: Restore from backup if needed
	if p.preparationData != nil && p.preparationData.BackupInfo != nil {
		err := p.restoreFromBackup(ctx, p.preparationData.BackupInfo)
		if err != nil {
			p.logger.Errorw("Failed to restore from backup", "error", err, "migration_id", migrationID)
			return fmt.Errorf("backup restoration failed: %w", err)
		}
	}

	// Step 2: Reset to original state
	if p.adaptiveBook != nil {
		p.adaptiveBook.SetMigrationPercentage(0)
		p.logger.Infow("Reset adaptive order book to original implementation")
	}

	// Step 3: Cleanup resources
	err := p.cleanup(ctx, migrationID)
	if err != nil {
		p.logger.Warnw("Cleanup failed during abort", "error", err)
	}

	p.logger.Infow("Abort phase completed", "migration_id", migrationID, "pair", p.pair)
	return nil
}

// HealthCheck performs a health check
func (p *OrderBookParticipant) HealthCheck(ctx context.Context) (*migration.HealthCheck, error) {
	atomic.StoreInt64(&p.lastHeartbeat, time.Now().UnixNano())

	// Perform comprehensive health check
	return p.performDetailedHealthCheck(ctx)
}

// GetSnapshot returns the current order book snapshot
func (p *OrderBookParticipant) GetSnapshot(ctx context.Context) (*migration.OrdersSnapshot, error) {
	return p.createOrderBookSnapshot(ctx)
}

// Cleanup cleans up resources after migration
func (p *OrderBookParticipant) Cleanup(ctx context.Context, migrationID uuid.UUID) error {
	return p.cleanup(ctx, migrationID)
}

// Helper methods

func (p *OrderBookParticipant) createFailedState(reason string, err error) *migration.ParticipantState {
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

func (p *OrderBookParticipant) isSystemHealthy() bool {
	return atomic.LoadInt64(&p.isHealthy) == 1
}

func (p *OrderBookParticipant) performDetailedHealthCheck(ctx context.Context) (*migration.HealthCheck, error) {
	checks := make(map[string]bool)
	issues := make([]string, 0)

	// Check order book availability
	checks["order_book_available"] = p.oldOrderBook != nil
	if !checks["order_book_available"] {
		issues = append(issues, "old order book not available")
	}

	// Check new implementation availability
	checks["new_implementation_available"] = p.newOrderBook != nil
	if !checks["new_implementation_available"] {
		issues = append(issues, "new order book implementation not available")
	}

	// Check memory usage (simplified)
	checks["memory_usage_ok"] = true // Would implement actual memory check

	// Check system load (simplified)
	checks["system_load_ok"] = true // Would implement actual load check

	// Calculate overall health
	healthyChecks := 0
	for _, isHealthy := range checks {
		if isHealthy {
			healthyChecks++
		}
	}

	score := float64(healthyChecks) / float64(len(checks))
	status := "healthy"
	message := "All systems operational"

	if score < 1.0 {
		status = "warning"
		message = fmt.Sprintf("Some checks failed: %v", issues)
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
		Name:       fmt.Sprintf("order_book_%s", p.pair),
		Status:     status,
		Score:      score,
		Message:    message,
		LastCheck:  time.Now(),
		CheckCount: 1,
		FailCount:  int64(len(checks) - healthyChecks),
	}, nil
}

func (p *OrderBookParticipant) capturePerformanceBaseline(ctx context.Context) (*PerformanceBaseline, error) {
	// This would implement actual performance measurement
	// For now, return mock data
	return &PerformanceBaseline{
		AvgLatencyMs:    1.5,
		OrdersPerSecond: 1000.0,
		MemoryUsageMB:   256.0,
		CPUUsagePercent: 15.0,
		ErrorRate:       0.001,
		MeasurementTime: time.Now(),
		SampleSize:      10000,
	}, nil
}

func (p *OrderBookParticipant) createOrderBookSnapshot(ctx context.Context) (*migration.OrdersSnapshot, error) {
	if p.oldOrderBook == nil {
		return nil, fmt.Errorf("old order book not available")
	}

	// Get snapshot from order book
	bids, asks := p.oldOrderBook.GetSnapshot(100) // Get deep snapshot

	// Count total orders
	totalOrders := int64(len(bids) + len(asks))

	// Calculate total volume
	totalVolume := decimal.Zero
	ordersByType := make(map[string]int64)
	priceLevels := make(map[string]*migration.PriceLevelSnapshot)

	// Process bids
	for _, bid := range bids {
		if len(bid) >= 2 {
			price, err := decimal.NewFromString(bid[0])
			if err != nil {
				continue
			}
			volume, err := decimal.NewFromString(bid[1])
			if err != nil {
				continue
			}

			totalVolume = totalVolume.Add(volume)
			ordersByType["buy"]++

			priceLevels[bid[0]] = &migration.PriceLevelSnapshot{
				Price:      price,
				Volume:     volume,
				OrderCount: 1, // Simplified
				FirstOrder: time.Now().Add(-time.Hour),
				LastOrder:  time.Now(),
			}
		}
	}

	// Process asks
	for _, ask := range asks {
		if len(ask) >= 2 {
			price, err := decimal.NewFromString(ask[0])
			if err != nil {
				continue
			}
			volume, err := decimal.NewFromString(ask[1])
			if err != nil {
				continue
			}

			totalVolume = totalVolume.Add(volume)
			ordersByType["sell"]++

			priceLevels[ask[0]] = &migration.PriceLevelSnapshot{
				Price:      price,
				Volume:     volume,
				OrderCount: 1, // Simplified
				FirstOrder: time.Now().Add(-time.Hour),
				LastOrder:  time.Now(),
			}
		}
	}

	// Create checksum
	checksumData := fmt.Sprintf("%s_%d_%s", p.pair, totalOrders, totalVolume.String())
	hash := md5.Sum([]byte(checksumData))
	checksum := hex.EncodeToString(hash[:])

	return &migration.OrdersSnapshot{
		Timestamp:    time.Now(),
		TotalOrders:  totalOrders,
		OrdersByType: ordersByType,
		TotalVolume:  totalVolume,
		PriceLevels:  priceLevels,
		Checksum:     checksum,
	}, nil
}

func (p *OrderBookParticipant) validateDataIntegrity(ctx context.Context, snapshot *migration.OrdersSnapshot) (*ValidationData, error) {
	// Calculate bid-ask spread (simplified)
	spread := decimal.Zero

	// Get top levels for validation
	bids, asks := p.oldOrderBook.GetSnapshot(5)

	// Perform consistency checks
	consistencyChecks := map[string]bool{
		"snapshot_not_empty": snapshot.TotalOrders > 0,
		"checksum_valid":     len(snapshot.Checksum) > 0,
		"orders_balanced":    snapshot.OrdersByType["buy"] > 0 && snapshot.OrdersByType["sell"] > 0,
		"volume_positive":    snapshot.TotalVolume.GreaterThan(decimal.Zero),
		"price_levels_valid": len(snapshot.PriceLevels) > 0,
	}

	// Create integrity checksum
	integrityData, _ := json.Marshal(snapshot)
	hash := md5.Sum(integrityData)
	integrityChecksum := hex.EncodeToString(hash[:])

	return &ValidationData{
		OrderCount:          snapshot.TotalOrders,
		TotalVolume:         snapshot.TotalVolume,
		BidAskSpread:        spread,
		IntegrityChecksum:   integrityChecksum,
		ConsistencyChecks:   consistencyChecks,
		PerformanceBaseline: nil, // Set externally
	}, nil
}

func (p *OrderBookParticipant) assessMigrationRisks(config *migration.MigrationConfig, validation *ValidationData) *RiskAssessment {
	riskFactors := make([]string, 0)
	mitigationActions := make([]string, 0)

	// Assess order volume risk
	if validation.OrderCount > 10000 {
		riskFactors = append(riskFactors, "high order volume")
		mitigationActions = append(mitigationActions, "use batch migration")
	}

	// Assess downtime risk
	if config.MaxDowntimeMs < 100 {
		riskFactors = append(riskFactors, "very strict downtime requirement")
		mitigationActions = append(mitigationActions, "use atomic switch strategy")
	}

	// Calculate overall risk
	overallRisk := "low"
	successProbability := 0.99

	if len(riskFactors) > 2 {
		overallRisk = "medium"
		successProbability = 0.95
	}

	if len(riskFactors) > 4 {
		overallRisk = "high"
		successProbability = 0.85
	}

	return &RiskAssessment{
		OverallRisk:        overallRisk,
		RiskFactors:        riskFactors,
		MitigationActions:  mitigationActions,
		SuccessProbability: successProbability,
		ImpactAssessment: &ImpactAssessment{
			PerformanceImpact:  "minimal",
			AvailabilityImpact: "minimal",
			DataIntegrityRisk:  "low",
			UserExperienceRisk: "low",
			EstimatedDowntime:  50, // milliseconds
		},
	}
}

func (p *OrderBookParticipant) createMigrationPlan(config *migration.MigrationConfig, validation *ValidationData, risk *RiskAssessment) *MigrationPlan {
	strategy := "atomic_switch"
	if validation.OrderCount > 5000 || risk.OverallRisk == "high" {
		strategy = "gradual_rollout"
	}

	batchSize := config.BatchSize
	if batchSize == 0 {
		batchSize = 1000
	}

	// Estimate duration based on order count and batch size
	estimatedDuration := time.Duration(validation.OrderCount/int64(batchSize)) * time.Millisecond * 10

	return &MigrationPlan{
		Strategy:          strategy,
		BatchSize:         batchSize,
		EstimatedDuration: estimatedDuration,
		RiskAssessment:    risk,
		FallbackStrategy:  "immediate_rollback",
		DowntimeEstimate:  time.Duration(risk.ImpactAssessment.EstimatedDowntime) * time.Millisecond,
	}
}

func (p *OrderBookParticipant) createBackup(ctx context.Context, snapshot *migration.OrdersSnapshot) (*BackupInfo, error) {
	backupID := uuid.New()
	backupTime := time.Now()

	// In a real implementation, this would create an actual backup
	// For now, we'll simulate it
	backupData, err := json.Marshal(snapshot)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize backup data: %w", err)
	}

	hash := md5.Sum(backupData)
	checksum := hex.EncodeToString(hash[:])

	return &BackupInfo{
		BackupID:       backupID,
		BackupLocation: fmt.Sprintf("/backups/orderbook_%s_%s.json", p.pair, backupID.String()),
		BackupSize:     int64(len(backupData)),
		BackupTime:     backupTime,
		BackupChecksum: checksum,
		RestoreCommands: []string{
			fmt.Sprintf("restore-orderbook --pair=%s --backup-id=%s", p.pair, backupID.String()),
		},
	}, nil
}

func (p *OrderBookParticipant) executeAtomicSwitch(ctx context.Context) error {
	if p.adaptiveBook == nil {
		return fmt.Errorf("adaptive order book not available for atomic switch")
	}

	p.logger.Infow("Executing atomic switch", "pair", p.pair)

	// Temporarily halt trading
	p.adaptiveBook.HaltTrading()
	defer p.adaptiveBook.ResumeTrading()

	// Switch to new implementation
	p.adaptiveBook.SetMigrationPercentage(100)

	p.logger.Infow("Atomic switch completed", "pair", p.pair)
	return nil
}

func (p *OrderBookParticipant) executeGradualRollout(ctx context.Context) error {
	if p.adaptiveBook == nil {
		return fmt.Errorf("adaptive order book not available for gradual rollout")
	}

	p.logger.Infow("Executing gradual rollout", "pair", p.pair)

	// Gradually increase percentage over time
	percentages := []int{10, 25, 50, 75, 100}

	for _, percentage := range percentages {
		p.adaptiveBook.SetMigrationPercentage(percentage)
		p.logger.Infow("Rollout progress", "pair", p.pair, "percentage", percentage)

		// Wait and monitor
		time.Sleep(time.Second * 2)

		// Check health
		if !p.isSystemHealthy() {
			return fmt.Errorf("system became unhealthy during rollout at %d%%", percentage)
		}
	}

	p.logger.Infow("Gradual rollout completed", "pair", p.pair)
	return nil
}

func (p *OrderBookParticipant) validateMigrationSuccess(ctx context.Context) error {
	// Get new snapshot
	newSnapshot, err := p.createOrderBookSnapshot(ctx)
	if err != nil {
		return fmt.Errorf("failed to create post-migration snapshot: %w", err)
	}

	// Compare with original
	if p.preparationData != nil && p.preparationData.Snapshot != nil {
		original := p.preparationData.Snapshot

		// Check order count consistency (allowing for some variance due to new orders)
		if newSnapshot.TotalOrders < original.TotalOrders/2 {
			return fmt.Errorf("significant order count decrease: %d -> %d", original.TotalOrders, newSnapshot.TotalOrders)
		}

		// Check total volume consistency (allowing for some variance)
		if newSnapshot.TotalVolume.LessThan(original.TotalVolume.Mul(decimal.NewFromFloat(0.5))) {
			return fmt.Errorf("significant volume decrease: %s -> %s", original.TotalVolume, newSnapshot.TotalVolume)
		}
	}

	return nil
}

func (p *OrderBookParticipant) restoreFromBackup(ctx context.Context, backupInfo *BackupInfo) error {
	p.logger.Infow("Restoring from backup", "backup_id", backupInfo.BackupID, "pair", p.pair)

	// In a real implementation, this would restore actual backup data
	// For now, we'll simulate it

	// Reset adaptive book to original state
	if p.adaptiveBook != nil {
		p.adaptiveBook.SetMigrationPercentage(0)
	}

	p.logger.Infow("Backup restoration completed", "backup_id", backupInfo.BackupID, "pair", p.pair)
	return nil
}

func (p *OrderBookParticipant) cleanup(ctx context.Context, migrationID uuid.UUID) error {
	p.logger.Infow("Cleaning up migration resources", "migration_id", migrationID, "pair", p.pair)

	// Reset migration state
	p.mu.Lock()
	p.currentMigrationID = uuid.Nil
	p.preparationData = nil
	p.mu.Unlock()

	return nil
}
