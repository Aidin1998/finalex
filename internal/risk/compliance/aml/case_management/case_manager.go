package case_management

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/compliance/aml"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// CaseManager handles investigation case management
type CaseManager struct {
	mu     sync.RWMutex
	logger *zap.SugaredLogger

	// Active cases
	cases map[string]*aml.InvestigationCase

	// Case assignments
	assignments map[string][]string // investigator_id -> case_ids

	// Case statistics
	stats *CaseStatistics

	// Workflow definitions
	workflows map[string]*CaseWorkflow
}

// CaseWorkflow defines the workflow for different case types
type CaseWorkflow struct {
	ID          string                   `json:"id"`
	Name        string                   `json:"name"`
	CaseType    string                   `json:"case_type"`
	Steps       []WorkflowStep           `json:"steps"`
	SLAs        map[string]time.Duration `json:"slas"`
	AutoActions map[string]interface{}   `json:"auto_actions"`
	CreatedAt   time.Time                `json:"created_at"`
	UpdatedAt   time.Time                `json:"updated_at"`
}

// WorkflowStep represents a step in the case workflow
type WorkflowStep struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Type        string                 `json:"type"` // "review", "investigate", "escalate", "close"
	Required    bool                   `json:"required"`
	SLA         time.Duration          `json:"sla"`
	Conditions  map[string]interface{} `json:"conditions"`
	Actions     []string               `json:"actions"`
	NextSteps   []string               `json:"next_steps"`
}

// CaseStatistics tracks case management metrics
type CaseStatistics struct {
	TotalCases       int                             `json:"total_cases"`
	OpenCases        int                             `json:"open_cases"`
	ClosedCases      int                             `json:"closed_cases"`
	EscalatedCases   int                             `json:"escalated_cases"`
	CasesByStatus    map[aml.InvestigationStatus]int `json:"cases_by_status"`
	CasesByPriority  map[aml.RiskLevel]int           `json:"cases_by_priority"`
	AverageCloseTime time.Duration                   `json:"average_close_time"`
	SLABreaches      int                             `json:"sla_breaches"`
	LastUpdated      time.Time                       `json:"last_updated"`
}

// CaseAssignment represents case assignment to investigators
type CaseAssignment struct {
	CaseID         string        `json:"case_id"`
	InvestigatorID string        `json:"investigator_id"`
	AssignedBy     string        `json:"assigned_by"`
	AssignedAt     time.Time     `json:"assigned_at"`
	DueDate        time.Time     `json:"due_date"`
	Priority       aml.RiskLevel `json:"priority"`
}

// NewCaseManager creates a new case management system
func NewCaseManager(logger *zap.SugaredLogger) *CaseManager {
	cm := &CaseManager{
		logger:      logger,
		cases:       make(map[string]*aml.InvestigationCase),
		assignments: make(map[string][]string),
		stats: &CaseStatistics{
			CasesByStatus:   make(map[aml.InvestigationStatus]int),
			CasesByPriority: make(map[aml.RiskLevel]int),
		},
		workflows: make(map[string]*CaseWorkflow),
	}

	// Initialize default workflows
	cm.initializeDefaultWorkflows()

	return cm
}

// initializeDefaultWorkflows sets up standard case workflows
func (cm *CaseManager) initializeDefaultWorkflows() {
	// SAR workflow
	sarWorkflow := &CaseWorkflow{
		ID:       "sar_workflow_v1",
		Name:     "Suspicious Activity Report Workflow",
		CaseType: "sar",
		Steps: []WorkflowStep{
			{
				ID:          "initial_review",
				Name:        "Initial Review",
				Description: "Initial assessment of suspicious activity",
				Type:        "review",
				Required:    true,
				SLA:         24 * time.Hour,
				NextSteps:   []string{"detailed_investigation", "false_positive"},
			},
			{
				ID:          "detailed_investigation",
				Name:        "Detailed Investigation",
				Description: "Comprehensive investigation of the activity",
				Type:        "investigate",
				Required:    true,
				SLA:         72 * time.Hour,
				NextSteps:   []string{"sar_filing", "escalate", "close"},
			},
			{
				ID:          "sar_filing",
				Name:        "SAR Filing",
				Description: "File Suspicious Activity Report with authorities",
				Type:        "escalate",
				Required:    false,
				SLA:         48 * time.Hour,
				NextSteps:   []string{"close"},
			},
		},
		SLAs: map[string]time.Duration{
			"total_resolution": 7 * 24 * time.Hour,
			"initial_response": 4 * time.Hour,
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// High-risk user workflow
	highRiskWorkflow := &CaseWorkflow{
		ID:       "high_risk_user_workflow_v1",
		Name:     "High-Risk User Investigation Workflow",
		CaseType: "high_risk_user",
		Steps: []WorkflowStep{
			{
				ID:          "risk_assessment",
				Name:        "Risk Assessment",
				Description: "Assess user risk profile and activities",
				Type:        "review",
				Required:    true,
				SLA:         12 * time.Hour,
				NextSteps:   []string{"enhanced_monitoring", "account_restriction", "close"},
			},
			{
				ID:          "enhanced_monitoring",
				Name:        "Enhanced Monitoring",
				Description: "Place user under enhanced monitoring",
				Type:        "investigate",
				Required:    false,
				SLA:         1 * time.Hour,
				NextSteps:   []string{"close"},
			},
		},
		SLAs: map[string]time.Duration{
			"total_resolution": 3 * 24 * time.Hour,
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	cm.workflows[sarWorkflow.ID] = sarWorkflow
	cm.workflows[highRiskWorkflow.ID] = highRiskWorkflow
}

// CreateCase creates a new investigation case
func (cm *CaseManager) CreateCase(ctx context.Context, caseData *aml.InvestigationCase) (*aml.InvestigationCase, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Generate case ID and number if not provided
	if caseData.ID == uuid.Nil {
		caseData.ID = uuid.New()
	}

	if caseData.CaseNumber == "" {
		caseData.CaseNumber = cm.generateCaseNumber()
	}

	// Set creation timestamp
	caseData.CreatedAt = time.Now()
	caseData.UpdatedAt = time.Now()

	// Add initial timeline entry
	initialEntry := aml.CaseTimelineEntry{
		ID:          uuid.New(),
		CaseID:      caseData.ID,
		Action:      "case_created",
		Description: "Investigation case created",
		PerformedBy: caseData.AssignedBy,
		Timestamp:   time.Now(),
	}
	caseData.Timeline = []aml.CaseTimelineEntry{initialEntry}

	// Store the case
	cm.cases[caseData.ID.String()] = caseData

	// Update statistics
	cm.updateStatistics()

	cm.logger.Infow("Investigation case created",
		"case_id", caseData.ID,
		"case_number", caseData.CaseNumber,
		"user_id", caseData.UserID,
		"priority", caseData.Priority)

	return caseData, nil
}

// AssignCase assigns a case to an investigator
func (cm *CaseManager) AssignCase(ctx context.Context, caseID string, investigatorID string, assignedBy uuid.UUID) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	caseObj, exists := cm.cases[caseID]
	if !exists {
		return fmt.Errorf("case not found: %s", caseID)
	}

	// Update case assignment
	investigatorUUID, err := uuid.Parse(investigatorID)
	if err != nil {
		return fmt.Errorf("invalid investigator ID: %s", investigatorID)
	}

	caseObj.AssignedTo = &investigatorUUID
	caseObj.AssignedBy = assignedBy
	caseObj.UpdatedAt = time.Now()

	// Add to assignments map
	cm.assignments[investigatorID] = append(cm.assignments[investigatorID], caseID)

	// Add timeline entry
	timelineEntry := aml.CaseTimelineEntry{
		ID:          uuid.New(),
		CaseID:      caseObj.ID,
		Action:      "case_assigned",
		Description: fmt.Sprintf("Case assigned to investigator %s", investigatorID),
		PerformedBy: assignedBy,
		Timestamp:   time.Now(),
	}
	caseObj.Timeline = append(caseObj.Timeline, timelineEntry)

	cm.logger.Infow("Case assigned",
		"case_id", caseID,
		"investigator_id", investigatorID,
		"assigned_by", assignedBy)

	return nil
}

// UpdateCaseStatus updates the status of a case
func (cm *CaseManager) UpdateCaseStatus(ctx context.Context, caseID string, status aml.InvestigationStatus, updatedBy uuid.UUID, notes string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	caseObj, exists := cm.cases[caseID]
	if !exists {
		return fmt.Errorf("case not found: %s", caseID)
	}

	oldStatus := caseObj.Status
	caseObj.Status = status
	caseObj.UpdatedAt = time.Now()

	// If closing the case, set closed timestamp
	if status == aml.StatusClosed {
		now := time.Now()
		caseObj.ClosedAt = &now
		caseObj.ClosedBy = &updatedBy
	}

	// Add notes if provided
	if notes != "" {
		if caseObj.Notes != "" {
			caseObj.Notes += "\n\n"
		}
		caseObj.Notes += fmt.Sprintf("[%s] %s", time.Now().Format("2006-01-02 15:04:05"), notes)
	}

	// Add timeline entry
	timelineEntry := aml.CaseTimelineEntry{
		ID:          uuid.New(),
		CaseID:      caseObj.ID,
		Action:      "status_updated",
		Description: fmt.Sprintf("Status changed from %s to %s", oldStatus, status),
		PerformedBy: updatedBy,
		Timestamp:   time.Now(),
	}
	caseObj.Timeline = append(caseObj.Timeline, timelineEntry)

	// Update statistics
	cm.updateStatistics()

	cm.logger.Infow("Case status updated",
		"case_id", caseID,
		"old_status", oldStatus,
		"new_status", status,
		"updated_by", updatedBy)

	return nil
}

// AddEvidence adds evidence to a case
func (cm *CaseManager) AddEvidence(ctx context.Context, caseID string, evidenceType string, evidence map[string]interface{}, addedBy uuid.UUID) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	caseObj, exists := cm.cases[caseID]
	if !exists {
		return fmt.Errorf("case not found: %s", caseID)
	}

	// Initialize evidence map if nil
	if caseObj.Evidence == nil {
		caseObj.Evidence = make(map[string]interface{})
	}

	// Add evidence with timestamp
	evidenceEntry := map[string]interface{}{
		"type":     evidenceType,
		"data":     evidence,
		"added_by": addedBy,
		"added_at": time.Now(),
	}

	evidenceKey := fmt.Sprintf("evidence_%d", len(caseObj.Evidence)+1)
	caseObj.Evidence[evidenceKey] = evidenceEntry
	caseObj.UpdatedAt = time.Now()

	// Add timeline entry
	timelineEntry := aml.CaseTimelineEntry{
		ID:          uuid.New(),
		CaseID:      caseObj.ID,
		Action:      "evidence_added",
		Description: fmt.Sprintf("Evidence of type %s added", evidenceType),
		PerformedBy: addedBy,
		Timestamp:   time.Now(),
	}
	caseObj.Timeline = append(caseObj.Timeline, timelineEntry)

	cm.logger.Infow("Evidence added to case",
		"case_id", caseID,
		"evidence_type", evidenceType,
		"added_by", addedBy)

	return nil
}

// GetCase retrieves a case by ID
func (cm *CaseManager) GetCase(caseID string) (*aml.InvestigationCase, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	caseObj, exists := cm.cases[caseID]
	if !exists {
		return nil, fmt.Errorf("case not found: %s", caseID)
	}

	return caseObj, nil
}

// ListCases lists cases based on filters
func (cm *CaseManager) ListCases(ctx context.Context, filters map[string]interface{}) ([]*aml.InvestigationCase, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var result []*aml.InvestigationCase

	for _, caseObj := range cm.cases {
		if cm.matchesFilters(caseObj, filters) {
			result = append(result, caseObj)
		}
	}

	return result, nil
}

// GetCasesByInvestigator returns cases assigned to a specific investigator
func (cm *CaseManager) GetCasesByInvestigator(investigatorID string) ([]*aml.InvestigationCase, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var result []*aml.InvestigationCase

	caseIDs, exists := cm.assignments[investigatorID]
	if !exists {
		return result, nil
	}

	for _, caseID := range caseIDs {
		if caseObj, exists := cm.cases[caseID]; exists {
			result = append(result, caseObj)
		}
	}

	return result, nil
}

// GetCaseStatistics returns current case management statistics
func (cm *CaseManager) GetCaseStatistics() *CaseStatistics {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	// Create a copy to avoid race conditions
	statsCopy := *cm.stats
	statsCopy.CasesByStatus = make(map[aml.InvestigationStatus]int)
	statsCopy.CasesByPriority = make(map[aml.RiskLevel]int)

	for k, v := range cm.stats.CasesByStatus {
		statsCopy.CasesByStatus[k] = v
	}
	for k, v := range cm.stats.CasesByPriority {
		statsCopy.CasesByPriority[k] = v
	}

	return &statsCopy
}

// EscalateCase escalates a case to higher authority
func (cm *CaseManager) EscalateCase(ctx context.Context, caseID string, escalatedBy uuid.UUID, reason string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	caseObj, exists := cm.cases[caseID]
	if !exists {
		return fmt.Errorf("case not found: %s", caseID)
	}

	// Update case status and escalation info
	caseObj.Status = aml.StatusEscalated
	caseObj.IsEscalated = true
	now := time.Now()
	caseObj.EscalatedAt = &now
	caseObj.UpdatedAt = time.Now()

	// Add escalation note
	escalationNote := fmt.Sprintf("Case escalated by %s. Reason: %s", escalatedBy, reason)
	if caseObj.Notes != "" {
		caseObj.Notes += "\n\n"
	}
	caseObj.Notes += fmt.Sprintf("[%s] %s", time.Now().Format("2006-01-02 15:04:05"), escalationNote)

	// Add timeline entry
	timelineEntry := aml.CaseTimelineEntry{
		ID:          uuid.New(),
		CaseID:      caseObj.ID,
		Action:      "case_escalated",
		Description: escalationNote,
		PerformedBy: escalatedBy,
		Timestamp:   time.Now(),
	}
	caseObj.Timeline = append(caseObj.Timeline, timelineEntry)

	// Update statistics
	cm.updateStatistics()

	cm.logger.Infow("Case escalated",
		"case_id", caseID,
		"escalated_by", escalatedBy,
		"reason", reason)

	return nil
}

// generateCaseNumber generates a unique case number
func (cm *CaseManager) generateCaseNumber() string {
	return fmt.Sprintf("CASE-%s-%d", time.Now().Format("20060102"), len(cm.cases)+1)
}

// matchesFilters checks if a case matches the provided filters
func (cm *CaseManager) matchesFilters(caseObj *aml.InvestigationCase, filters map[string]interface{}) bool {
	for key, value := range filters {
		switch key {
		case "status":
			if status, ok := value.(aml.InvestigationStatus); ok {
				if caseObj.Status != status {
					return false
				}
			}
		case "priority":
			if priority, ok := value.(aml.RiskLevel); ok {
				if caseObj.Priority != priority {
					return false
				}
			}
		case "assigned_to":
			if assignedTo, ok := value.(string); ok {
				if caseObj.AssignedTo == nil || caseObj.AssignedTo.String() != assignedTo {
					return false
				}
			}
		case "user_id":
			if userID, ok := value.(string); ok {
				if caseObj.UserID.String() != userID {
					return false
				}
			}
		}
	}
	return true
}

// updateStatistics updates case management statistics
func (cm *CaseManager) updateStatistics() {
	stats := &CaseStatistics{
		CasesByStatus:   make(map[aml.InvestigationStatus]int),
		CasesByPriority: make(map[aml.RiskLevel]int),
		LastUpdated:     time.Now(),
	}

	var totalCloseTime time.Duration
	var closedCaseCount int

	for _, caseObj := range cm.cases {
		stats.TotalCases++

		// Count by status
		stats.CasesByStatus[caseObj.Status]++

		// Count by priority
		stats.CasesByPriority[caseObj.Priority]++

		// Calculate average close time
		if caseObj.Status == aml.StatusClosed && caseObj.ClosedAt != nil {
			closeTime := caseObj.ClosedAt.Sub(caseObj.CreatedAt)
			totalCloseTime += closeTime
			closedCaseCount++
		}

		// Count specific statuses
		switch caseObj.Status {
		case aml.StatusOpen, aml.StatusInProgress:
			stats.OpenCases++
		case aml.StatusClosed:
			stats.ClosedCases++
		case aml.StatusEscalated:
			stats.EscalatedCases++
		}
	}

	if closedCaseCount > 0 {
		stats.AverageCloseTime = totalCloseTime / time.Duration(closedCaseCount)
	}

	cm.stats = stats
}
