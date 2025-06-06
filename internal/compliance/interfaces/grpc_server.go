package interfaces

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/Aidin1998/finalex/pkg/proto/compliance"
)

// GRPCServer implements the gRPC compliance service
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

// HealthCheck returns the health status of the compliance service
func (s *GRPCServer) HealthCheck(ctx context.Context, req *emptypb.Empty) (*pb.HealthResponse, error) {
	// Check service health
	healthy := true
	details := make(map[string]string)

	// Check database connectivity
	if err := s.monitoringService.CheckHealth(ctx); err != nil {
		healthy = false
		details["database"] = err.Error()
	} else {
		details["database"] = "healthy"
	}

	// Check manipulation service
	if s.manipulationService != nil {
		details["manipulation_service"] = "healthy"
	}

	// Check audit service
	if s.auditService != nil {
		details["audit_service"] = "healthy"
	}

	status := "healthy"
	if !healthy {
		status = "unhealthy"
	}

	return &pb.HealthResponse{
		Status:    status,
		Timestamp: timestamppb.Now(),
		Details:   details,
	}, nil
}

// ReadinessCheck returns the readiness status
func (s *GRPCServer) ReadinessCheck(ctx context.Context, req *emptypb.Empty) (*pb.ReadinessResponse, error) {
	ready := true
	var failingServices []string

	// Check if monitoring service is ready
	if err := s.monitoringService.CheckHealth(ctx); err != nil {
		ready = false
		failingServices = append(failingServices, "monitoring")
	}

	return &pb.ReadinessResponse{
		Ready:           ready,
		FailingServices: failingServices,
		Timestamp:       timestamppb.Now(),
	}, nil
}

// PerformComplianceCheck performs a compliance check
func (s *GRPCServer) PerformComplianceCheck(ctx context.Context, req *pb.ComplianceCheckRequest) (*pb.ComplianceCheckResponse, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	// Convert request to internal format
	checkReq := ComplianceCheckRequest{
		UserID:        req.UserId,
		CheckType:     req.CheckType,
		Parameters:    req.Parameters,
		TransactionID: req.TransactionId,
	}

	// Perform compliance check
	result, err := s.complianceService.PerformCheck(ctx, checkReq)
	if err != nil {
		s.logger.Error("Failed to perform compliance check", zap.Error(err), zap.String("user_id", req.UserId))
		return nil, status.Error(codes.Internal, "failed to perform compliance check")
	}

	// Convert actions to protobuf format
	var actions []*pb.ComplianceAction
	for _, action := range result.RequiredActions {
		pbAction := &pb.ComplianceAction{
			Id:          action.ID,
			Type:        action.Type,
			Description: action.Description,
			Parameters:  action.Parameters,
			Required:    action.Required,
		}
		if !action.Deadline.IsZero() {
			pbAction.Deadline = timestamppb.New(action.Deadline)
		}
		actions = append(actions, pbAction)
	}

	return &pb.ComplianceCheckResponse{
		Compliant:       result.Compliant,
		RiskScore:       result.RiskScore,
		Status:          result.Status,
		Violations:      result.Violations,
		RequiredActions: actions,
		Metadata:        result.Metadata,
	}, nil
}

// GetUserComplianceStatus gets user compliance status
func (s *GRPCServer) GetUserComplianceStatus(ctx context.Context, req *pb.UserComplianceRequest) (*pb.UserComplianceResponse, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	status, err := s.complianceService.GetUserStatus(ctx, req.UserId)
	if err != nil {
		s.logger.Error("Failed to get user compliance status", zap.Error(err), zap.String("user_id", req.UserId))
		return nil, status.Error(codes.Internal, "failed to get user compliance status")
	}

	return &pb.UserComplianceResponse{
		UserId:             status.UserID,
		RiskScore:          status.RiskScore,
		Status:             status.Status,
		ActiveRestrictions: status.ActiveRestrictions,
		LastCheck:          timestamppb.New(status.LastCheck),
		ViolationCount:     int32(status.ViolationCount),
	}, nil
}

// BatchComplianceCheck performs batch compliance checks
func (s *GRPCServer) BatchComplianceCheck(ctx context.Context, req *pb.BatchComplianceRequest) (*pb.BatchComplianceResponse, error) {
	if len(req.Requests) == 0 {
		return nil, status.Error(codes.InvalidArgument, "at least one request is required")
	}

	var responses []*pb.ComplianceCheckResponse
	var processedCount, failedCount int32

	for _, checkReq := range req.Requests {
		resp, err := s.PerformComplianceCheck(ctx, checkReq)
		if err != nil {
			failedCount++
			continue
		}
		responses = append(responses, resp)
		processedCount++
	}

	return &pb.BatchComplianceResponse{
		Responses:      responses,
		ProcessedCount: processedCount,
		FailedCount:    failedCount,
	}, nil
}

// GetMonitoringOverview returns monitoring overview
func (s *GRPCServer) GetMonitoringOverview(ctx context.Context, req *emptypb.Empty) (*pb.MonitoringOverviewResponse, error) {
	overview, err := s.monitoringService.GetOverview(ctx)
	if err != nil {
		s.logger.Error("Failed to get monitoring overview", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to get monitoring overview")
	}

	var recentAlerts []*pb.AlertSummary
	for _, alert := range overview.RecentAlerts {
		recentAlerts = append(recentAlerts, &pb.AlertSummary{
			Type:     alert.Type,
			Count:    int32(alert.Count),
			Severity: alert.Severity,
		})
	}

	return &pb.MonitoringOverviewResponse{
		ActiveAlerts:      int32(overview.ActiveAlerts),
		TotalPolicies:     int32(overview.TotalPolicies),
		EnabledPolicies:   int32(overview.EnabledPolicies),
		SystemHealthScore: overview.SystemHealthScore,
		LastUpdated:       timestamppb.New(overview.LastUpdated),
		RecentAlerts:      recentAlerts,
	}, nil
}

// GetActiveAlerts returns active alerts
func (s *GRPCServer) GetActiveAlerts(ctx context.Context, req *pb.AlertsRequest) (*pb.AlertsResponse, error) {
	alertReq := AlertsRequest{
		Severity: req.Severity,
		Status:   req.Status,
		Limit:    int(req.Limit),
		Offset:   int(req.Offset),
	}

	alerts, total, err := s.monitoringService.GetAlerts(ctx, alertReq)
	if err != nil {
		s.logger.Error("Failed to get active alerts", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to get active alerts")
	}

	var pbAlerts []*pb.MonitoringAlert
	for _, alert := range alerts {
		pbAlert := &pb.MonitoringAlert{
			Id:        alert.ID,
			Type:      alert.Type,
			Severity:  alert.Severity,
			Message:   alert.Message,
			Status:    alert.Status,
			CreatedAt: timestamppb.New(alert.CreatedAt),
			UpdatedAt: timestamppb.New(alert.UpdatedAt),
			Metadata:  alert.Metadata,
		}
		if alert.AcknowledgedBy != "" {
			pbAlert.AcknowledgedBy = alert.AcknowledgedBy
			pbAlert.AcknowledgedAt = timestamppb.New(*alert.AcknowledgedAt)
		}
		pbAlerts = append(pbAlerts, pbAlert)
	}

	return &pb.AlertsResponse{
		Alerts:     pbAlerts,
		TotalCount: int32(total),
	}, nil
}

// AcknowledgeAlert acknowledges an alert
func (s *GRPCServer) AcknowledgeAlert(ctx context.Context, req *pb.AcknowledgeAlertRequest) (*emptypb.Empty, error) {
	if req.AlertId == "" {
		return nil, status.Error(codes.InvalidArgument, "alert_id is required")
	}

	if req.AcknowledgedBy == "" {
		return nil, status.Error(codes.InvalidArgument, "acknowledged_by is required")
	}

	err := s.monitoringService.AcknowledgeAlert(ctx, req.AlertId, req.AcknowledgedBy, req.Notes)
	if err != nil {
		s.logger.Error("Failed to acknowledge alert", zap.Error(err), zap.String("alert_id", req.AlertId))
		return nil, status.Error(codes.Internal, "failed to acknowledge alert")
	}

	return &emptypb.Empty{}, nil
}

// GetSystemHealth returns system health information
func (s *GRPCServer) GetSystemHealth(ctx context.Context, req *emptypb.Empty) (*pb.SystemHealthResponse, error) {
	health, err := s.monitoringService.GetSystemHealth(ctx)
	if err != nil {
		s.logger.Error("Failed to get system health", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to get system health")
	}

	var services []*pb.ServiceHealth
	for _, svc := range health.Services {
		services = append(services, &pb.ServiceHealth{
			ServiceName:  svc.ServiceName,
			Status:       svc.Status,
			HealthScore:  svc.HealthScore,
			LastCheck:    timestamppb.New(svc.LastCheck),
			ErrorMessage: svc.ErrorMessage,
		})
	}

	var metrics []*pb.MetricSummary
	for _, metric := range health.Metrics {
		metrics = append(metrics, &pb.MetricSummary{
			Name:   metric.Name,
			Value:  metric.Value,
			Unit:   metric.Unit,
			Status: metric.Status,
		})
	}

	return &pb.SystemHealthResponse{
		HealthScore: health.HealthScore,
		Services:    services,
		Metrics:     metrics,
		Timestamp:   timestamppb.New(health.Timestamp),
	}, nil
}

// DetectManipulation performs manipulation detection
func (s *GRPCServer) DetectManipulation(ctx context.Context, req *pb.ManipulationDetectionRequest) (*pb.ManipulationDetectionResponse, error) {
	if s.manipulationService == nil {
		return nil, status.Error(codes.Unimplemented, "manipulation service not available")
	}

	detectionReq := ManipulationDetectionRequest{
		PatternType: req.PatternType,
		Parameters:  req.Parameters,
		StartTime:   req.StartTime.AsTime(),
		EndTime:     req.EndTime.AsTime(),
	}

	result, err := s.manipulationService.DetectPatterns(ctx, detectionReq)
	if err != nil {
		s.logger.Error("Failed to detect manipulation", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to detect manipulation")
	}

	var patterns []*pb.ManipulationPattern
	for _, pattern := range result.Patterns {
		patterns = append(patterns, &pb.ManipulationPattern{
			Id:          pattern.ID,
			Type:        pattern.Type,
			Confidence:  pattern.Confidence,
			Description: pattern.Description,
			Evidence:    pattern.Evidence,
			Parameters:  pattern.Parameters,
			DetectedAt:  timestamppb.New(pattern.DetectedAt),
		})
	}

	return &pb.ManipulationDetectionResponse{
		Patterns:           patterns,
		ConfidenceScore:    result.ConfidenceScore,
		AnalysisSummary:    result.AnalysisSummary,
		RecommendedActions: result.RecommendedActions,
	}, nil
}

// GetManipulationAlerts returns manipulation alerts
func (s *GRPCServer) GetManipulationAlerts(ctx context.Context, req *pb.ManipulationAlertsRequest) (*pb.ManipulationAlertsResponse, error) {
	if s.manipulationService == nil {
		return nil, status.Error(codes.Unimplemented, "manipulation service not available")
	}

	alertReq := ManipulationAlertsRequest{
		PatternType: req.PatternType,
		Status:      req.Status,
		Limit:       int(req.Limit),
		Offset:      int(req.Offset),
	}

	alerts, total, err := s.manipulationService.GetAlerts(ctx, alertReq)
	if err != nil {
		s.logger.Error("Failed to get manipulation alerts", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to get manipulation alerts")
	}

	var pbAlerts []*pb.ManipulationAlert
	for _, alert := range alerts {
		pbAlerts = append(pbAlerts, &pb.ManipulationAlert{
			Id:                  alert.ID,
			PatternType:         alert.PatternType,
			Confidence:          alert.Confidence,
			Status:              alert.Status,
			Description:         alert.Description,
			AffectedUsers:       alert.AffectedUsers,
			AffectedInstruments: alert.AffectedInstruments,
			CreatedAt:           timestamppb.New(alert.CreatedAt),
			UpdatedAt:           timestamppb.New(alert.UpdatedAt),
			Metadata:            alert.Metadata,
		})
	}

	return &pb.ManipulationAlertsResponse{
		Alerts:     pbAlerts,
		TotalCount: int32(total),
	}, nil
}

// ResolveManipulationAlert resolves a manipulation alert
func (s *GRPCServer) ResolveManipulationAlert(ctx context.Context, req *pb.ResolveAlertRequest) (*emptypb.Empty, error) {
	if s.manipulationService == nil {
		return nil, status.Error(codes.Unimplemented, "manipulation service not available")
	}

	if req.AlertId == "" {
		return nil, status.Error(codes.InvalidArgument, "alert_id is required")
	}

	err := s.manipulationService.ResolveAlert(ctx, req.AlertId, req.Resolution, req.ResolvedBy, req.Notes)
	if err != nil {
		s.logger.Error("Failed to resolve manipulation alert", zap.Error(err), zap.String("alert_id", req.AlertId))
		return nil, status.Error(codes.Internal, "failed to resolve manipulation alert")
	}

	return &emptypb.Empty{}, nil
}

// CreateInvestigation creates a new investigation
func (s *GRPCServer) CreateInvestigation(ctx context.Context, req *pb.CreateInvestigationRequest) (*pb.Investigation, error) {
	if req.Title == "" {
		return nil, status.Error(codes.InvalidArgument, "title is required")
	}

	createReq := CreateInvestigationRequest{
		Title:       req.Title,
		Description: req.Description,
		Severity:    req.Severity,
		AssignedTo:  req.AssignedTo,
		AlertIDs:    req.AlertIds,
		Metadata:    req.Metadata,
	}

	investigation, err := s.auditService.CreateInvestigation(ctx, createReq)
	if err != nil {
		s.logger.Error("Failed to create investigation", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to create investigation")
	}

	var notes []*pb.InvestigationNote
	for _, note := range investigation.Notes {
		notes = append(notes, &pb.InvestigationNote{
			Id:        note.ID,
			Content:   note.Content,
			CreatedBy: note.CreatedBy,
			CreatedAt: timestamppb.New(note.CreatedAt),
		})
	}

	return &pb.Investigation{
		Id:          investigation.ID,
		Title:       investigation.Title,
		Description: investigation.Description,
		Status:      investigation.Status,
		Severity:    investigation.Severity,
		AssignedTo:  investigation.AssignedTo,
		CreatedBy:   investigation.CreatedBy,
		CreatedAt:   timestamppb.New(investigation.CreatedAt),
		UpdatedAt:   timestamppb.New(investigation.UpdatedAt),
		AlertIds:    investigation.AlertIDs,
		Notes:       notes,
		Metadata:    investigation.Metadata,
	}, nil
}

// GetInvestigation retrieves an investigation
func (s *GRPCServer) GetInvestigation(ctx context.Context, req *pb.GetInvestigationRequest) (*pb.Investigation, error) {
	if req.InvestigationId == "" {
		return nil, status.Error(codes.InvalidArgument, "investigation_id is required")
	}

	investigation, err := s.auditService.GetInvestigation(ctx, req.InvestigationId)
	if err != nil {
		s.logger.Error("Failed to get investigation", zap.Error(err), zap.String("investigation_id", req.InvestigationId))
		return nil, status.Error(codes.Internal, "failed to get investigation")
	}

	if investigation == nil {
		return nil, status.Error(codes.NotFound, "investigation not found")
	}

	var notes []*pb.InvestigationNote
	for _, note := range investigation.Notes {
		notes = append(notes, &pb.InvestigationNote{
			Id:        note.ID,
			Content:   note.Content,
			CreatedBy: note.CreatedBy,
			CreatedAt: timestamppb.New(note.CreatedAt),
		})
	}

	return &pb.Investigation{
		Id:          investigation.ID,
		Title:       investigation.Title,
		Description: investigation.Description,
		Status:      investigation.Status,
		Severity:    investigation.Severity,
		AssignedTo:  investigation.AssignedTo,
		CreatedBy:   investigation.CreatedBy,
		CreatedAt:   timestamppb.New(investigation.CreatedAt),
		UpdatedAt:   timestamppb.New(investigation.UpdatedAt),
		AlertIds:    investigation.AlertIDs,
		Notes:       notes,
		Metadata:    investigation.Metadata,
	}, nil
}

// UpdateInvestigation updates an investigation
func (s *GRPCServer) UpdateInvestigation(ctx context.Context, req *pb.UpdateInvestigationRequest) (*pb.Investigation, error) {
	if req.InvestigationId == "" {
		return nil, status.Error(codes.InvalidArgument, "investigation_id is required")
	}

	updateReq := UpdateInvestigationRequest{
		InvestigationID: req.InvestigationId,
		Status:          req.Status,
		Notes:           req.Notes,
		AssignedTo:      req.AssignedTo,
		Metadata:        req.Metadata,
	}

	investigation, err := s.auditService.UpdateInvestigation(ctx, updateReq)
	if err != nil {
		s.logger.Error("Failed to update investigation", zap.Error(err), zap.String("investigation_id", req.InvestigationId))
		return nil, status.Error(codes.Internal, "failed to update investigation")
	}

	var notes []*pb.InvestigationNote
	for _, note := range investigation.Notes {
		notes = append(notes, &pb.InvestigationNote{
			Id:        note.ID,
			Content:   note.Content,
			CreatedBy: note.CreatedBy,
			CreatedAt: timestamppb.New(note.CreatedAt),
		})
	}

	return &pb.Investigation{
		Id:          investigation.ID,
		Title:       investigation.Title,
		Description: investigation.Description,
		Status:      investigation.Status,
		Severity:    investigation.Severity,
		AssignedTo:  investigation.AssignedTo,
		CreatedBy:   investigation.CreatedBy,
		CreatedAt:   timestamppb.New(investigation.CreatedAt),
		UpdatedAt:   timestamppb.New(investigation.UpdatedAt),
		AlertIds:    investigation.AlertIDs,
		Notes:       notes,
		Metadata:    investigation.Metadata,
	}, nil
}

// ListInvestigations lists investigations
func (s *GRPCServer) ListInvestigations(ctx context.Context, req *pb.ListInvestigationsRequest) (*pb.ListInvestigationsResponse, error) {
	listReq := ListInvestigationsRequest{
		Status:     req.Status,
		AssignedTo: req.AssignedTo,
		Limit:      int(req.Limit),
		Offset:     int(req.Offset),
	}

	investigations, total, err := s.auditService.ListInvestigations(ctx, listReq)
	if err != nil {
		s.logger.Error("Failed to list investigations", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to list investigations")
	}

	var pbInvestigations []*pb.Investigation
	for _, investigation := range investigations {
		var notes []*pb.InvestigationNote
		for _, note := range investigation.Notes {
			notes = append(notes, &pb.InvestigationNote{
				Id:        note.ID,
				Content:   note.Content,
				CreatedBy: note.CreatedBy,
				CreatedAt: timestamppb.New(note.CreatedAt),
			})
		}

		pbInvestigations = append(pbInvestigations, &pb.Investigation{
			Id:          investigation.ID,
			Title:       investigation.Title,
			Description: investigation.Description,
			Status:      investigation.Status,
			Severity:    investigation.Severity,
			AssignedTo:  investigation.AssignedTo,
			CreatedBy:   investigation.CreatedBy,
			CreatedAt:   timestamppb.New(investigation.CreatedAt),
			UpdatedAt:   timestamppb.New(investigation.UpdatedAt),
			AlertIds:    investigation.AlertIDs,
			Notes:       notes,
			Metadata:    investigation.Metadata,
		})
	}

	return &pb.ListInvestigationsResponse{
		Investigations: pbInvestigations,
		TotalCount:     int32(total),
	}, nil
}

// GetAuditLogs retrieves audit logs
func (s *GRPCServer) GetAuditLogs(ctx context.Context, req *pb.AuditLogsRequest) (*pb.AuditLogsResponse, error) {
	auditReq := AuditLogsRequest{
		EntityType: req.EntityType,
		EntityID:   req.EntityId,
		Action:     req.Action,
		StartTime:  req.StartTime.AsTime(),
		EndTime:    req.EndTime.AsTime(),
		Limit:      int(req.Limit),
		Offset:     int(req.Offset),
	}

	logs, total, err := s.auditService.GetAuditLogs(ctx, auditReq)
	if err != nil {
		s.logger.Error("Failed to get audit logs", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to get audit logs")
	}

	var pbLogs []*pb.AuditEntry
	for _, log := range logs {
		pbLogs = append(pbLogs, &pb.AuditEntry{
			Id:          log.ID,
			EntityType:  log.EntityType,
			EntityId:    log.EntityID,
			Action:      log.Action,
			PerformedBy: log.PerformedBy,
			Timestamp:   timestamppb.New(log.Timestamp),
			Details:     log.Details,
			IpAddress:   log.IPAddress,
			UserAgent:   log.UserAgent,
		})
	}

	return &pb.AuditLogsResponse{
		Logs:       pbLogs,
		TotalCount: int32(total),
	}, nil
}

// CreateAuditEntry creates an audit entry
func (s *GRPCServer) CreateAuditEntry(ctx context.Context, req *pb.CreateAuditEntryRequest) (*emptypb.Empty, error) {
	if req.EntityType == "" || req.EntityId == "" || req.Action == "" {
		return nil, status.Error(codes.InvalidArgument, "entity_type, entity_id, and action are required")
	}

	entry := AuditEntry{
		ID:          uuid.New().String(),
		EntityType:  req.EntityType,
		EntityID:    req.EntityId,
		Action:      req.Action,
		PerformedBy: req.PerformedBy,
		Timestamp:   time.Now(),
		Details:     req.Details,
	}

	err := s.auditService.CreateAuditEntry(ctx, entry)
	if err != nil {
		s.logger.Error("Failed to create audit entry", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to create audit entry")
	}

	return &emptypb.Empty{}, nil
}

// GetMonitoringPolicies retrieves monitoring policies
func (s *GRPCServer) GetMonitoringPolicies(ctx context.Context, req *emptypb.Empty) (*pb.MonitoringPoliciesResponse, error) {
	policies, err := s.monitoringService.GetPolicies(ctx)
	if err != nil {
		s.logger.Error("Failed to get monitoring policies", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to get monitoring policies")
	}

	var pbPolicies []*pb.MonitoringPolicy
	for _, policy := range policies {
		pbPolicies = append(pbPolicies, &pb.MonitoringPolicy{
			Id:          policy.ID,
			Name:        policy.Name,
			Description: policy.Description,
			Enabled:     policy.Enabled,
			Condition:   policy.Condition,
			Actions:     policy.Actions,
			Parameters:  policy.Parameters,
			CreatedAt:   timestamppb.New(policy.CreatedAt),
			UpdatedAt:   timestamppb.New(policy.UpdatedAt),
		})
	}

	return &pb.MonitoringPoliciesResponse{
		Policies: pbPolicies,
	}, nil
}

// UpdateMonitoringPolicy updates a monitoring policy
func (s *GRPCServer) UpdateMonitoringPolicy(ctx context.Context, req *pb.UpdatePolicyRequest) (*emptypb.Empty, error) {
	if req.PolicyId == "" {
		return nil, status.Error(codes.InvalidArgument, "policy_id is required")
	}

	updateReq := UpdatePolicyRequest{
		PolicyID:   req.PolicyId,
		Enabled:    req.Enabled,
		Parameters: req.Parameters,
	}

	err := s.monitoringService.UpdatePolicy(ctx, updateReq)
	if err != nil {
		s.logger.Error("Failed to update monitoring policy", zap.Error(err), zap.String("policy_id", req.PolicyId))
		return nil, status.Error(codes.Internal, "failed to update monitoring policy")
	}

	return &emptypb.Empty{}, nil
}
