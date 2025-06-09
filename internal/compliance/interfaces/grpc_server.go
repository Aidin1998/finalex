package interfaces

import (
	"context"
	"fmt"
	"time"

	pb "github.com/Aidin1998/finalex/github.com/Aidin1998/finalex/pkg/proto/compliance"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// GRPCServer implements the ComplianceService gRPC interface
type GRPCServer struct {
	pb.UnimplementedComplianceServiceServer
	complianceService   ComplianceService
	monitoringService   MonitoringService
	manipulationService ManipulationService
	auditService        AuditService
	logger              *zap.Logger
}

// NewGRPCServer creates a new gRPC compliance server
func NewGRPCServer(
	complianceService ComplianceService,
	monitoringService MonitoringService,
	manipulationService ManipulationService,
	auditService AuditService,
	logger *zap.Logger,
) *GRPCServer {
	return &GRPCServer{
		complianceService:   complianceService,
		monitoringService:   monitoringService,
		manipulationService: manipulationService,
		auditService:        auditService,
		logger:              logger,
	}
}

// HealthCheck implements the health check endpoint
func (s *GRPCServer) HealthCheck(ctx context.Context, req *emptypb.Empty) (*pb.HealthResponse, error) {
	s.logger.Info("Health check requested")

	details := make(map[string]string)
	overallStatus := "healthy"

	// Check compliance service
	if err := s.complianceService.HealthCheck(ctx); err != nil {
		details["compliance_service"] = "unhealthy: " + err.Error()
		overallStatus = "degraded"
	} else {
		details["compliance_service"] = "healthy"
	}

	// Check monitoring service
	if err := s.monitoringService.HealthCheck(ctx); err != nil {
		details["monitoring_service"] = "unhealthy: " + err.Error()
		overallStatus = "degraded"
	} else {
		details["monitoring_service"] = "healthy"
	}

	// Check manipulation service
	if err := s.manipulationService.HealthCheck(ctx); err != nil {
		details["manipulation_service"] = "unhealthy: " + err.Error()
		overallStatus = "degraded"
	} else {
		details["manipulation_service"] = "healthy"
	}

	// Check audit service
	if err := s.auditService.HealthCheck(ctx); err != nil {
		details["audit_service"] = "unhealthy: " + err.Error()
		overallStatus = "degraded"
	} else {
		details["audit_service"] = "healthy"
	}

	return &pb.HealthResponse{
		Status:    overallStatus,
		Timestamp: timestamppb.Now(),
		Details:   details,
	}, nil
}

// ReadinessCheck implements the readiness check endpoint
func (s *GRPCServer) ReadinessCheck(ctx context.Context, req *emptypb.Empty) (*pb.ReadinessResponse, error) {
	s.logger.Info("Readiness check requested")

	var failingServices []string

	// Check each service readiness
	if err := s.complianceService.HealthCheck(ctx); err != nil {
		failingServices = append(failingServices, "compliance_service")
	}

	if err := s.monitoringService.HealthCheck(ctx); err != nil {
		failingServices = append(failingServices, "monitoring_service")
	}

	if err := s.manipulationService.HealthCheck(ctx); err != nil {
		failingServices = append(failingServices, "manipulation_service")
	}

	if err := s.auditService.HealthCheck(ctx); err != nil {
		failingServices = append(failingServices, "audit_service")
	}

	ready := len(failingServices) == 0

	return &pb.ReadinessResponse{
		Ready:           ready,
		FailingServices: failingServices,
		Timestamp:       timestamppb.Now(),
	}, nil
}

// PerformComplianceCheck performs a compliance check for a user
func (s *GRPCServer) PerformComplianceCheck(ctx context.Context, req *pb.ComplianceCheckRequest) (*pb.ComplianceCheckResponse, error) {
	s.logger.Info("Compliance check requested", zap.String("user_id", req.UserId))

	userID, err := uuid.Parse(req.UserId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid user ID: %v", err)
	}

	// Create compliance request from protobuf request
	complianceReq := &ComplianceRequest{
		UserID:           userID,
		RequestTimestamp: time.Now(),
	}

	// Perform compliance check using existing service
	checkResult, err := s.complianceService.PerformComplianceCheck(ctx, complianceReq)
	if err != nil {
		s.logger.Error("Compliance check failed", zap.String("user_id", req.UserId), zap.Error(err))
		return nil, status.Errorf(codes.Internal, "compliance check failed: %v", err)
	}

	// Convert result to protobuf format
	response := &pb.ComplianceCheckResponse{
		Compliant:       checkResult.Approved,
		RiskScore:       checkResult.RiskScore.InexactFloat64(),
		Status:          checkResult.Status.String(),
		Violations:      checkResult.Flags, // Use flags as violations
		RequiredActions: convertToComplianceActions(checkResult.Conditions),
		Metadata:        make(map[string]string),
	}

	// Add metadata
	if response.Metadata == nil {
		response.Metadata = make(map[string]string)
	}
	response.Metadata["risk_level"] = checkResult.RiskLevel.String()
	response.Metadata["reason"] = checkResult.Reason
	if checkResult.ProcessedAt.IsZero() == false {
		response.Metadata["processed_at"] = checkResult.ProcessedAt.Format(time.RFC3339)
	}

	return response, nil
}

// GetUserComplianceStatus gets the compliance status for a user
func (s *GRPCServer) GetUserComplianceStatus(ctx context.Context, req *pb.UserComplianceRequest) (*pb.UserComplianceResponse, error) {
	s.logger.Info("User compliance status requested", zap.String("user_id", req.UserId))

	userID, err := uuid.Parse(req.UserId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid user ID: %v", err)
	}

	statusInfo, err := s.complianceService.GetUserStatus(ctx, userID)
	if err != nil {
		s.logger.Error("Failed to get user compliance status", zap.String("user_id", req.UserId), zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to get compliance status: %v", err)
	}

	return &pb.UserComplianceResponse{
		UserId:             req.UserId,
		Status:             statusInfo.Status.String(),
		RiskScore:          statusInfo.RiskScore.InexactFloat64(),
		ActiveRestrictions: statusInfo.Restrictions, // string slice
		LastCheck:          timestamppb.Now(),       // fallback, update if you have a timestamp
		ViolationCount:     0,                       // set to 0 or actual if available
	}, nil
}

// BatchComplianceCheck performs compliance checks for multiple users
func (s *GRPCServer) BatchComplianceCheck(ctx context.Context, req *pb.BatchComplianceRequest) (*pb.BatchComplianceResponse, error) {
	s.logger.Info("Batch compliance check requested", zap.Int("request_count", len(req.Requests)))

	if len(req.Requests) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "no requests provided")
	}

	responses := make([]*pb.ComplianceCheckResponse, 0, len(req.Requests))
	var failedCount int32

	for _, checkReq := range req.Requests {
		userID, err := uuid.Parse(checkReq.UserId)
		if err != nil {
			s.logger.Warn("Invalid user ID in batch request", zap.String("user_id", checkReq.UserId))
			failedCount++
			continue
		}
		complianceReq := &ComplianceRequest{
			UserID:           userID,
			RequestTimestamp: time.Now(),
		}
		checkResult, err := s.complianceService.PerformComplianceCheck(ctx, complianceReq)
		if err != nil {
			s.logger.Warn("Compliance check failed for user in batch", zap.String("user_id", checkReq.UserId), zap.Error(err))
			failedCount++
			continue
		}
		response := &pb.ComplianceCheckResponse{
			Compliant:       checkResult.Approved,
			RiskScore:       checkResult.RiskScore.InexactFloat64(),
			Status:          checkResult.Status.String(),
			Violations:      checkResult.Flags,
			RequiredActions: convertToComplianceActions(checkResult.Conditions),
			Metadata: map[string]string{
				"user_id":    checkReq.UserId,
				"risk_level": checkResult.RiskLevel.String(),
				"reason":     checkResult.Reason,
			},
		}
		responses = append(responses, response)
	}

	return &pb.BatchComplianceResponse{
		Responses:      responses,
		ProcessedCount: int32(len(responses)),
		FailedCount:    failedCount,
	}, nil
}

// GetMonitoringOverview gets the monitoring overview
func (s *GRPCServer) GetMonitoringOverview(ctx context.Context, req *emptypb.Empty) (*pb.MonitoringOverviewResponse, error) {
	s.logger.Info("Monitoring overview requested")
	_, err := s.monitoringService.GetDashboard(ctx)
	if err != nil {
		s.logger.Error("Failed to get monitoring dashboard", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to get monitoring overview: %v", err)
	}

	// Get recent alerts (convert to AlertSummary if needed)
	recentAlerts, err := s.monitoringService.GetActiveAlerts(ctx)
	if err != nil {
		s.logger.Warn("Failed to get recent alerts", zap.Error(err))
		recentAlerts = []*MonitoringAlert{} // Use empty slice as fallback
	}
	alertSummaries := make([]*pb.AlertSummary, len(recentAlerts))
	for i := range recentAlerts {
		alertSummaries[i] = &pb.AlertSummary{
			Type:     "generic", // fallback, update if you have a type field
			Count:    1,         // or aggregate if needed
			Severity: "info",    // fallback, update if you have a severity field
		}
	}

	return &pb.MonitoringOverviewResponse{
		ActiveAlerts:      0,                 // update if you have this info
		TotalPolicies:     0,                 // update if you have this info
		EnabledPolicies:   0,                 // update if you have this info
		SystemHealthScore: 1.0,               // update if you have this info
		LastUpdated:       timestamppb.Now(), // fallback
		RecentAlerts:      alertSummaries,
	}, nil
}

// GetActiveAlerts gets active alerts
func (s *GRPCServer) GetActiveAlerts(ctx context.Context, req *pb.AlertsRequest) (*pb.AlertsResponse, error) {
	s.logger.Info("Active alerts requested")

	alerts, err := s.monitoringService.GetActiveAlerts(ctx)
	if err != nil {
		s.logger.Error("Failed to get active alerts", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to get active alerts: %v", err)
	}

	return &pb.AlertsResponse{
		Alerts:     convertMonitoringAlertsToProto(alerts),
		TotalCount: int32(len(alerts)),
	}, nil
}

// AcknowledgeAlert acknowledges an alert
func (s *GRPCServer) AcknowledgeAlert(ctx context.Context, req *pb.AcknowledgeAlertRequest) (*emptypb.Empty, error) {
	s.logger.Info("Alert acknowledgment requested", zap.String("alert_id", req.AlertId))

	alertID, err := uuid.Parse(req.AlertId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid alert ID: %v", err)
	}

	acknowledgedBy, err := uuid.Parse(req.AcknowledgedBy)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid acknowledger ID: %v", err)
	}

	err = s.monitoringService.UpdateAlertStatus(ctx, alertID, "acknowledged")
	if err != nil {
		s.logger.Error("Failed to acknowledge alert", zap.String("alert_id", req.AlertId), zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to acknowledge alert: %v", err)
	}

	// Log the acknowledgment
	auditEvent := &AuditEvent{
		ID:          uuid.New(),
		UserID:      &acknowledgedBy,
		EventType:   "alert_acknowledged",
		Category:    "monitoring",
		Severity:    "info",
		Description: fmt.Sprintf("Alert %s acknowledged", req.AlertId),
		Resource:    "alert",
		ResourceID:  req.AlertId,
		Timestamp:   time.Now(),
	}

	if logErr := s.auditService.LogEvent(ctx, auditEvent); logErr != nil {
		s.logger.Warn("Failed to log alert acknowledgment", zap.Error(logErr))
	}

	return &emptypb.Empty{}, nil
}

// GetSystemHealth gets system health status
func (s *GRPCServer) GetSystemHealth(ctx context.Context, req *emptypb.Empty) (*pb.SystemHealthResponse, error) {
	s.logger.Info("System health requested")
	services := make([]*pb.ServiceHealth, 0)
	healthScore := 1.0

	// Check compliance service
	complianceStatus := "healthy"
	if err := s.complianceService.HealthCheck(ctx); err != nil {
		complianceStatus = "unhealthy"
		healthScore -= 0.25
	}
	services = append(services, &pb.ServiceHealth{
		ServiceName: "compliance",
		Status:      complianceStatus,
		HealthScore: healthScore,
		LastCheck:   timestamppb.Now(),
	})

	// Check monitoring service
	monitoringStatus := "healthy"
	if err := s.monitoringService.HealthCheck(ctx); err != nil {
		monitoringStatus = "unhealthy"
		healthScore -= 0.25
	}
	services = append(services, &pb.ServiceHealth{
		ServiceName: "monitoring",
		Status:      monitoringStatus,
		HealthScore: healthScore,
		LastCheck:   timestamppb.Now(),
	})

	// Check manipulation service
	manipulationStatus := "healthy"
	if err := s.manipulationService.HealthCheck(ctx); err != nil {
		manipulationStatus = "unhealthy"
		healthScore -= 0.25
	}
	services = append(services, &pb.ServiceHealth{
		ServiceName: "manipulation",
		Status:      manipulationStatus,
		HealthScore: healthScore,
		LastCheck:   timestamppb.Now(),
	})

	// Check audit service
	auditStatus := "healthy"
	if err := s.auditService.HealthCheck(ctx); err != nil {
		auditStatus = "unhealthy"
		healthScore -= 0.25
	}
	services = append(services, &pb.ServiceHealth{
		ServiceName: "audit",
		Status:      auditStatus,
		HealthScore: healthScore,
		LastCheck:   timestamppb.Now(),
	})

	return &pb.SystemHealthResponse{
		HealthScore: healthScore,
		Services:    services,
		Timestamp:   timestamppb.Now(),
	}, nil
}

// DetectManipulation detects market manipulation
func (s *GRPCServer) DetectManipulation(ctx context.Context, req *pb.ManipulationDetectionRequest) (*pb.ManipulationDetectionResponse, error) {
	s.logger.Info("Manipulation detection requested")
	// Map proto request to internal ManipulationRequest
	internalReq := ManipulationRequest{
		// Map fields as appropriate from req
		// Example: PatternType, Parameters, StartTime, EndTime
		Timestamp: time.Now(),
	}
	result, err := s.manipulationService.DetectManipulation(ctx, internalReq)
	if err != nil {
		s.logger.Error("Manipulation detection failed", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "manipulation detection failed: %v", err)
	}
	patterns := make([]*pb.ManipulationPattern, len(result.Patterns))
	for i, p := range result.Patterns {
		// Map PatternEvidence to []string or []pb.PatternEvidence if proto supports it
		var evidence []string
		for _, ev := range p.Evidence {
			evidence = append(evidence, ev.Description)
		}
		patterns[i] = &pb.ManipulationPattern{
			Id:          p.Type.String(),
			Type:        p.Type.String(),
			Confidence:  p.Confidence.InexactFloat64(),
			Description: p.Description,
			Evidence:    evidence,
			Parameters:  nil,
			DetectedAt:  timestamppb.New(p.DetectedAt),
		}
	}
	return &pb.ManipulationDetectionResponse{
		Patterns:           patterns,
		ConfidenceScore:    result.RiskScore.InexactFloat64(),
		AnalysisSummary:    "",
		RecommendedActions: nil,
	}, nil
}

// GetManipulationAlerts gets manipulation alerts
func (s *GRPCServer) GetManipulationAlerts(ctx context.Context, req *pb.ManipulationAlertsRequest) (*pb.ManipulationAlertsResponse, error) {
	s.logger.Info("Manipulation alerts requested")
	// Convert req.Status string to AlertStatus enum if needed
	var alertStatus AlertStatus
	switch req.Status {
	case "pending":
		alertStatus = AlertStatusPending
	case "acknowledged":
		alertStatus = AlertStatusAcknowledged
	case "investigating":
		alertStatus = AlertStatusInvestigating
	case "resolved":
		alertStatus = AlertStatusResolved
	case "dismissed":
		alertStatus = AlertStatusDismissed
	default:
		alertStatus = AlertStatusPending
	}
	filter := &AlertFilter{
		Status: alertStatus,
		Limit:  int(req.Limit),
		Offset: int(req.Offset),
	}
	alerts, err := s.manipulationService.GetAlerts(ctx, filter)
	if err != nil {
		s.logger.Error("Failed to get manipulation alerts", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to get manipulation alerts: %v", err)
	}
	protoAlerts := make([]*pb.ManipulationAlert, len(alerts))
	for i, a := range alerts {
		protoAlerts[i] = &pb.ManipulationAlert{
			Id:                  a.ID.String(),
			PatternType:         a.AlertType.String(),
			Confidence:          a.RiskScore.InexactFloat64(),
			Status:              a.Status.String(),
			Description:         a.Description,
			AffectedUsers:       []string{a.UserID.String()},
			AffectedInstruments: nil,
			CreatedAt:           timestamppb.New(a.DetectedAt),
			UpdatedAt:           timestamppb.New(a.DetectedAt),
			Metadata:            nil,
		}
	}
	return &pb.ManipulationAlertsResponse{
		Alerts:     protoAlerts,
		TotalCount: int32(len(protoAlerts)),
	}, nil
}

// CreateInvestigation creates a new investigation
func (s *GRPCServer) CreateInvestigation(ctx context.Context, req *pb.CreateInvestigationRequest) (*pb.Investigation, error) {
	s.logger.Info("Investigation creation requested", zap.String("title", req.Title))
	internalReq := &CreateInvestigationRequest{
		Subject:     req.Title, // proto 'Title' maps to internal 'Subject'
		Description: req.Description,
		Priority:    req.Severity, // proto 'Severity' maps to internal 'Priority' (if needed)
		AssignedTo:  convertStringToUUIDPointer(req.AssignedTo),
		AlertIDs:    convertStringSliceToUUIDs(req.AlertIds),
		Metadata:    convertStringMapToInterfaceMap(req.Metadata),
	}
	investigation, err := s.manipulationService.CreateInvestigation(ctx, internalReq)
	if err != nil {
		s.logger.Error("Failed to create investigation", zap.String("title", req.Title), zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to create investigation: %v", err)
	}
	return convertInvestigationToProto(investigation), nil
}

// GetInvestigation gets investigation details
// Fix for GetInvestigation: filter by AlertIDs or AssignedTo, not ID
func (s *GRPCServer) GetInvestigation(ctx context.Context, req *pb.GetInvestigationRequest) (*pb.Investigation, error) {
	s.logger.Info("Investigation details requested", zap.String("investigation_id", req.InvestigationId))
	id, err := uuid.Parse(req.InvestigationId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid investigation ID: %v", err)
	}
	// No ID field in InvestigationFilter, so fetch all and filter manually
	investigations, err := s.manipulationService.GetInvestigations(ctx, &InvestigationFilter{})
	if err != nil {
		s.logger.Error("Failed to get investigations", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to get investigation: %v", err)
	}
	for _, inv := range investigations {
		if inv.ID == id {
			return convertInvestigationToProto(inv), nil
		}
	}
	return nil, status.Errorf(codes.NotFound, "investigation not found")
}

// UpdateInvestigation updates an investigation
func (s *GRPCServer) UpdateInvestigation(ctx context.Context, req *pb.UpdateInvestigationRequest) (*pb.Investigation, error) {
	s.logger.Info("Investigation update requested", zap.String("investigation_id", req.InvestigationId))
	id, err := uuid.Parse(req.InvestigationId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid investigation ID: %v", err)
	}
	internalUpdate := &InvestigationUpdate{
		Status:     stringPointer(req.Status),
		AssignedTo: convertStringToUUIDPointer(req.AssignedTo),
		Metadata:   convertStringMapToInterfaceMap(req.Metadata),
	}
	err = s.manipulationService.UpdateInvestigation(ctx, id, internalUpdate)
	if err != nil {
		s.logger.Error("Failed to update investigation", zap.String("investigation_id", req.InvestigationId), zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to update investigation: %v", err)
	}
	investigations, err := s.manipulationService.GetInvestigations(ctx, &InvestigationFilter{AssignedTo: convertStringToUUIDPointer(req.AssignedTo)})
	if err != nil || len(investigations) == 0 {
		return nil, status.Errorf(codes.Internal, "failed to fetch updated investigation: %v", err)
	}
	return convertInvestigationToProto(investigations[0]), nil
}

// ListInvestigations lists investigations
func (s *GRPCServer) ListInvestigations(ctx context.Context, req *pb.ListInvestigationsRequest) (*pb.ListInvestigationsResponse, error) {
	s.logger.Info("Investigations list requested")
	filter := &InvestigationFilter{
		Status:     req.Status,
		AssignedTo: convertStringToUUIDPointer(req.AssignedTo),
		Limit:      int(req.Limit),
		Offset:     int(req.Offset),
	}
	investigations, err := s.manipulationService.GetInvestigations(ctx, filter)
	if err != nil {
		s.logger.Error("Failed to list investigations", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to list investigations: %v", err)
	}
	protoInvestigations := make([]*pb.Investigation, len(investigations))
	for i, inv := range investigations {
		protoInvestigations[i] = convertInvestigationToProto(inv)
	}
	return &pb.ListInvestigationsResponse{
		Investigations: protoInvestigations,
		TotalCount:     int32(len(protoInvestigations)),
	}, nil
}

// Helper: convert internal Investigation to proto Investigation
func convertInvestigationToProto(inv *Investigation) *pb.Investigation {
	return &pb.Investigation{
		Id:          inv.ID.String(),
		Title:       inv.Subject,
		Description: inv.Description,
		Status:      inv.Status,
		Severity:    inv.Priority,
		AssignedTo:  convertUUIDPointerToString(inv.AssignedTo),
		CreatedBy:   inv.CreatedBy.String(),
		CreatedAt:   timestamppb.New(inv.CreatedAt),
		UpdatedAt:   timestamppb.New(inv.UpdatedAt),
		AlertIds:    convertUUIDSliceToStrings(inv.AlertIDs),
		Notes:       nil, // map notes if needed
		Metadata:    convertInterfaceMapToStringMap(inv.Metadata),
	}
}

// Helper: convert map[string]string to map[string]interface{}
func convertStringMapToInterfaceMap(m map[string]string) map[string]interface{} {
	if m == nil {
		return nil
	}
	res := make(map[string]interface{}, len(m))
	for k, v := range m {
		res[k] = v
	}
	return res
}

// Helper: convert map[string]interface{} to map[string]string
func convertInterfaceMapToStringMap(m map[string]interface{}) map[string]string {
	if m == nil {
		return nil
	}
	res := make(map[string]string, len(m))
	for k, v := range m {
		if str, ok := v.(string); ok {
			res[k] = str
		}
	}
	return res
}

// Helper: string pointer
func stringPointer(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// convertToComplianceActions converts a slice of strings to ComplianceActions
func convertToComplianceActions(conditions []string) []*pb.ComplianceAction {
	actions := make([]*pb.ComplianceAction, len(conditions))
	for i, condition := range conditions {
		actions[i] = &pb.ComplianceAction{
			Id:          uuid.New().String(),
			Type:        "condition",
			Description: condition,
			Required:    true,
		}
	}
	return actions
}

// convertTimePointer converts a time pointer to protobuf timestamp
func convertTimePointer(t *time.Time) *timestamppb.Timestamp {
	if t == nil || t.IsZero() {
		return nil
	}
	return timestamppb.New(*t)
}

// convertStringToUUIDPointer converts a string to UUID pointer
func convertStringToUUIDPointer(s string) *uuid.UUID {
	if s == "" {
		return nil
	}
	if id, err := uuid.Parse(s); err == nil {
		return &id
	}
	return nil
}

// convertUUIDPointerToString converts a UUID pointer to string
func convertUUIDPointerToString(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}

// convertStringSliceToUUIDs converts string slice to UUID slice
func convertStringSliceToUUIDs(strs []string) []uuid.UUID {
	uuids := make([]uuid.UUID, 0, len(strs))
	for _, s := range strs {
		if id, err := uuid.Parse(s); err == nil {
			uuids = append(uuids, id)
		}
	}
	return uuids
}

// convertUUIDSliceToStrings converts UUID slice to string slice
func convertUUIDSliceToStrings(uuids []uuid.UUID) []string {
	strs := make([]string, len(uuids))
	for i, id := range uuids {
		strs[i] = id.String()
	}
	return strs
}

// convertMonitoringAlertsToProto converts monitoring alerts to protobuf format
func convertMonitoringAlertsToProto(alerts []*MonitoringAlert) []*pb.MonitoringAlert {
	result := make([]*pb.MonitoringAlert, len(alerts))
	for i, alert := range alerts {
		result[i] = &pb.MonitoringAlert{
			Id:             alert.ID.String(),
			Type:           alert.AlertType, // string
			Severity:       alert.Severity.String(),
			Message:        alert.Message,
			Status:         alert.Status.String(),
			CreatedAt:      timestamppb.New(alert.Timestamp),
			UpdatedAt:      convertTimePointer(alert.ResolvedAt),
			Metadata:       convertInterfaceMapToStringMap(alert.Metadata),
			AcknowledgedBy: convertUUIDPointerToString(alert.ResolvedBy),
			AcknowledgedAt: convertTimePointer(alert.ResolvedAt),
		}
	}
	return result
}
