// Package hooks provides platform-wide integration hooks implementation
package hooks

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/Aidin1998/finalex/internal/compliance/interfaces"
	"github.com/Aidin1998/finalex/internal/compliance/risk"
)

// RegisterUserAuthHook registers a user authentication hook
func (hm *HookManager) RegisterUserAuthHook(hook UserAuthHook) {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	hm.userAuthHooks = append(hm.userAuthHooks, hook)
}

// RegisterTradingHook registers a trading hook
func (hm *HookManager) RegisterTradingHook(hook TradingHook) {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	hm.tradingHooks = append(hm.tradingHooks, hook)
}

// RegisterFiatHook registers a fiat currency hook
func (hm *HookManager) RegisterFiatHook(hook FiatHook) {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	hm.fiatHooks = append(hm.fiatHooks, hook)
}

// RegisterWalletHook registers a wallet hook
func (hm *HookManager) RegisterWalletHook(hook WalletHook) {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	hm.walletHooks = append(hm.walletHooks, hook)
}

// RegisterAccountsHook registers an accounts hook
func (hm *HookManager) RegisterAccountsHook(hook AccountsHook) {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	hm.accountsHooks = append(hm.accountsHooks, hook)
}

// RegisterMarketMakingHook registers a market making hook
func (hm *HookManager) RegisterMarketMakingHook(hook MarketMakingHook) {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	hm.marketMakingHooks = append(hm.marketMakingHooks, hook)
}

// Built-in compliance hooks implementation

// ComplianceUserAuthHook implements UserAuthHook for compliance checking
type ComplianceUserAuthHook struct {
	compliance interfaces.ComplianceService
	audit      interfaces.AuditService
	monitoring interfaces.MonitoringService
	risk       interfaces.RiskService
}

// NewComplianceUserAuthHook creates a new compliance user auth hook
func NewComplianceUserAuthHook(
	compliance interfaces.ComplianceService,
	audit interfaces.AuditService,
	monitoring interfaces.MonitoringService,
	risk interfaces.RiskService,
) *ComplianceUserAuthHook {
	return &ComplianceUserAuthHook{
		compliance: compliance,
		audit:      audit,
		monitoring: monitoring,
		risk:       risk,
	}
}

func (h *ComplianceUserAuthHook) OnUserRegistration(ctx context.Context, event *UserRegistrationEvent) error {
	// Audit logging
	if err := h.auditEvent(ctx, "user_registration", event.UserID, event); err != nil {
		log.Printf("Failed to audit user registration: %v", err)
	}

	// Compliance check for new user
	complianceReq := &interfaces.ComplianceRequest{
		UserID:           event.UserID,
		ActivityType:     interfaces.ActivityRegistration,
		IPAddress:        event.IPAddress,
		UserAgent:        event.UserAgent,
		DeviceID:         event.DeviceID,
		Country:          event.Country,
		Email:            event.Email,
		RequestTimestamp: event.Timestamp,
		Metadata:         event.Metadata,
	}

	result, err := h.compliance.CheckCompliance(ctx, complianceReq)
	if err != nil {
		return fmt.Errorf("compliance check failed: %w", err)
	}

	// Risk assessment for new user
	userRiskCtx := &risk.UserRiskContext{
		UserID:           event.UserID,
		Email:            event.Email,
		Country:          event.Country,
		RegistrationDate: event.Timestamp,
		Metadata:         event.Metadata,
	}

	riskAssessment, err := h.risk.AssessUserRisk(ctx, userRiskCtx)
	if err != nil {
		log.Printf("Risk assessment failed for user %s: %v", event.UserID, err)
	}

	// Generate alerts if needed
	if result.RiskLevel == interfaces.RiskLevelHigh || result.RiskLevel == interfaces.RiskLevelCritical {
		alert := &interfaces.MonitoringAlert{
			UserID:    event.UserID.String(),
			AlertType: "high_risk_registration",
			Severity:  interfaces.AlertSeverityHigh,
			Status:    interfaces.AlertStatusPending,
			Message:   fmt.Sprintf("High risk user registration detected: %s", result.Reason),
			Details: map[string]interface{}{
				"compliance_result": result,
				"risk_assessment":   riskAssessment,
				"registration_data": event,
			},
			Timestamp: event.Timestamp,
		}

		if err := h.monitoring.GenerateAlert(ctx, alert); err != nil {
			log.Printf("Failed to generate alert: %v", err)
		}
	}

	return nil
}

func (h *ComplianceUserAuthHook) OnUserLogin(ctx context.Context, event *UserLoginEvent) error {
	// Audit logging
	if err := h.auditEvent(ctx, "user_login", event.UserID, event); err != nil {
		log.Printf("Failed to audit user login: %v", err)
	}

	// Check for suspicious login patterns
	if !event.Success {
		// Failed login attempt - increase monitoring
		alert := &interfaces.MonitoringAlert{
			UserID:    event.UserID.String(),
			AlertType: "failed_login_attempt",
			Severity:  interfaces.AlertSeverityMedium,
			Status:    interfaces.AlertStatusPending,
			Message:   fmt.Sprintf("Failed login attempt: %s", event.FailReason),
			Details: map[string]interface{}{
				"ip_address":   event.IPAddress,
				"user_agent":   event.UserAgent,
				"device_id":    event.DeviceID,
				"country":      event.Country,
				"fail_reason":  event.FailReason,
				"login_method": event.LoginMethod,
			},
			Timestamp: event.Timestamp,
		}

		return h.monitoring.GenerateAlert(ctx, alert)
	}

	// Successful login - check for geo anomalies, device changes, etc.
	// This would involve more sophisticated analysis
	return nil
}

func (h *ComplianceUserAuthHook) OnUserLogout(ctx context.Context, event *UserLogoutEvent) error {
	return h.auditEvent(ctx, "user_logout", event.UserID, event)
}

func (h *ComplianceUserAuthHook) OnPasswordChange(ctx context.Context, event *PasswordChangeEvent) error {
	if err := h.auditEvent(ctx, "password_change", event.UserID, event); err != nil {
		return err
	}

	if !event.Success {
		alert := &interfaces.MonitoringAlert{
			UserID:    event.UserID.String(),
			AlertType: "failed_password_change",
			Severity:  interfaces.AlertSeverityMedium,
			Status:    interfaces.AlertStatusPending,
			Message:   "Failed password change attempt",
			Details: map[string]interface{}{
				"ip_address": event.IPAddress,
			},
			Timestamp: event.Timestamp,
		}

		return h.monitoring.GenerateAlert(ctx, alert)
	}

	return nil
}

func (h *ComplianceUserAuthHook) OnEmailVerification(ctx context.Context, event *EmailVerificationEvent) error {
	return h.auditEvent(ctx, "email_verification", event.UserID, event)
}

func (h *ComplianceUserAuthHook) On2FAEnabled(ctx context.Context, event *TwoFAEvent) error {
	return h.auditEvent(ctx, "2fa_change", event.UserID, event)
}

func (h *ComplianceUserAuthHook) OnAccountLocked(ctx context.Context, event *AccountLockEvent) error {
	if err := h.auditEvent(ctx, "account_locked", event.UserID, event); err != nil {
		return err
	}

	alert := &interfaces.MonitoringAlert{
		UserID:    event.UserID.String(),
		AlertType: "account_locked",
		Severity:  interfaces.AlertSeverityHigh,
		Status:    interfaces.AlertStatusPending,
		Message:   fmt.Sprintf("Account locked: %s", event.Reason),
		Details: map[string]interface{}{
			"reason":    event.Reason,
			"locked_by": event.LockedBy,
			"duration":  event.Duration,
		},
		Timestamp: event.Timestamp,
	}

	return h.monitoring.GenerateAlert(ctx, alert)
}

// ComplianceTradingHook implements TradingHook for compliance checking
type ComplianceTradingHook struct {
	compliance   interfaces.ComplianceService
	audit        interfaces.AuditService
	monitoring   interfaces.MonitoringService
	manipulation interfaces.ManipulationService
	risk         interfaces.RiskService
}

// NewComplianceTradingHook creates a new compliance trading hook
func NewComplianceTradingHook(
	compliance interfaces.ComplianceService,
	audit interfaces.AuditService,
	monitoring interfaces.MonitoringService,
	manipulation interfaces.ManipulationService,
	risk interfaces.RiskService,
) *ComplianceTradingHook {
	return &ComplianceTradingHook{
		compliance:   compliance,
		audit:        audit,
		monitoring:   monitoring,
		manipulation: manipulation,
		risk:         risk,
	}
}

func (h *ComplianceTradingHook) OnOrderPlaced(ctx context.Context, event *OrderPlacedEvent) error {
	// Audit logging
	if err := h.auditEvent(ctx, "order_placed", event.UserID, event); err != nil {
		log.Printf("Failed to audit order placement: %v", err)
	}

	// Risk assessment for the transaction
	txRiskCtx := &risk.TransactionRiskContext{
		UserID:          event.UserID,
		TransactionID:   event.UserID, // Use appropriate transaction ID
		Amount:          event.Quantity.Mul(event.Price),
		Currency:        event.Market,
		TransactionType: "order",
		IPAddress:       event.IPAddress,
		Timestamp:       event.Timestamp,
		Metadata: map[string]interface{}{
			"order_id": event.OrderID,
			"market":   event.Market,
			"side":     event.Side,
			"type":     event.Type,
		},
	}

	riskAssessment, err := h.risk.AssessTransactionRisk(ctx, txRiskCtx)
	if err != nil {
		log.Printf("Risk assessment failed for order %s: %v", event.OrderID, err)
	}

	// Manipulation detection
	manipReq := &interfaces.ManipulationRequest{
		RequestID: event.OrderID,
		UserID:    event.UserID,
		Market:    event.Market,
		Orders: []interfaces.Order{
			{
				ID:        event.OrderID,
				UserID:    event.UserID,
				Market:    event.Market,
				Side:      event.Side,
				Type:      event.Type,
				Quantity:  event.Quantity,
				Price:     event.Price,
				Status:    "new",
				CreatedAt: event.Timestamp,
			},
		},
		Timestamp: event.Timestamp,
		IPAddress: event.IPAddress,
	}

	manipResult, err := h.manipulation.DetectManipulation(ctx, manipReq)
	if err != nil {
		log.Printf("Manipulation detection failed for order %s: %v", event.OrderID, err)
	}

	// Generate alerts for high-risk transactions
	if riskAssessment != nil && riskAssessment.Level >= risk.RiskLevelHigh {
		alert := &interfaces.MonitoringAlert{
			UserID:    event.UserID.String(),
			AlertType: "high_risk_order",
			Severity:  interfaces.AlertSeverityHigh,
			Status:    interfaces.AlertStatusPending,
			Message:   "High risk order detected",
			Details: map[string]interface{}{
				"order_event":         event,
				"risk_assessment":     riskAssessment,
				"manipulation_result": manipResult,
			},
			Timestamp: event.Timestamp,
		}

		if err := h.monitoring.GenerateAlert(ctx, alert); err != nil {
			log.Printf("Failed to generate alert: %v", err)
		}
	}

	return nil
}

func (h *ComplianceTradingHook) OnOrderExecuted(ctx context.Context, event *OrderExecutedEvent) error {
	return h.auditEvent(ctx, "order_executed", event.UserID, event)
}

func (h *ComplianceTradingHook) OnOrderCancelled(ctx context.Context, event *OrderCancelledEvent) error {
	return h.auditEvent(ctx, "order_cancelled", event.UserID, event)
}

func (h *ComplianceTradingHook) OnTradeExecuted(ctx context.Context, event *TradeExecutedEvent) error {
	// Audit both buyer and seller
	if err := h.auditEvent(ctx, "trade_executed_buyer", event.BuyerID, event); err != nil {
		log.Printf("Failed to audit trade for buyer: %v", err)
	}

	if err := h.auditEvent(ctx, "trade_executed_seller", event.SellerID, event); err != nil {
		log.Printf("Failed to audit trade for seller: %v", err)
	}

	// Check for potential wash trading
	if event.BuyerID == event.SellerID {
		alert := &interfaces.MonitoringAlert{
			UserID:    event.BuyerID.String(),
			AlertType: "potential_wash_trading",
			Severity:  interfaces.AlertSeverityCritical,
			Status:    interfaces.AlertStatusPending,
			Message:   "Potential wash trading detected - same user as buyer and seller",
			Details: map[string]interface{}{
				"trade_event": event,
			},
			Timestamp: event.Timestamp,
		}

		return h.monitoring.GenerateAlert(ctx, alert)
	}

	return nil
}

func (h *ComplianceTradingHook) OnPositionUpdated(ctx context.Context, event *PositionUpdateEvent) error {
	return h.auditEvent(ctx, "position_updated", event.UserID, event)
}

func (h *ComplianceTradingHook) OnMarketDataReceived(ctx context.Context, event *MarketDataEvent) error {
	// This could be used for market manipulation detection
	// For now, just basic logging
	return nil
}

// Helper function for audit logging
func (h *ComplianceUserAuthHook) auditEvent(ctx context.Context, eventType string, userID interface{}, data interface{}) error {
	auditEvent := &interfaces.AuditEvent{
		EventType:   eventType,
		Category:    "user_auth",
		Severity:    "info",
		Description: fmt.Sprintf("User auth event: %s", eventType),
		Metadata: map[string]interface{}{
			"event_data": data,
		},
		Timestamp: time.Now(),
	}

	if uid, ok := userID.(string); ok {
		auditEvent.UserID = &uid
	}

	return h.audit.CreateEvent(ctx, auditEvent)
}

func (h *ComplianceTradingHook) auditEvent(ctx context.Context, eventType string, userID interface{}, data interface{}) error {
	auditEvent := &interfaces.AuditEvent{
		EventType:   eventType,
		Category:    "trading",
		Severity:    "info",
		Description: fmt.Sprintf("Trading event: %s", eventType),
		Metadata: map[string]interface{}{
			"event_data": data,
		},
		Timestamp: time.Now(),
	}

	if uid, ok := userID.(string); ok {
		auditEvent.UserID = &uid
	}

	return h.audit.CreateEvent(ctx, auditEvent)
}

// Initialize default hooks
func (hm *HookManager) InitializeDefaultHooks() {
	// Register built-in compliance hooks
	hm.RegisterUserAuthHook(NewComplianceUserAuthHook(
		hm.compliance,
		hm.audit,
		hm.monitoring,
		hm.risk,
	))

	hm.RegisterTradingHook(NewComplianceTradingHook(
		hm.compliance,
		hm.audit,
		hm.monitoring,
		hm.manipulation,
		hm.risk,
	))

	// Additional built-in hooks would be registered here
}

// ProcessEvent processes events through all registered hooks
func (hm *HookManager) ProcessUserAuthEvent(ctx context.Context, eventType string, event interface{}) error {
	hm.mu.RLock()
	hooks := make([]UserAuthHook, len(hm.userAuthHooks))
	copy(hooks, hm.userAuthHooks)
	hm.mu.RUnlock()

	var wg sync.WaitGroup
	errChan := make(chan error, len(hooks))

	for _, hook := range hooks {
		wg.Add(1)
		go func(h UserAuthHook) {
			defer wg.Done()

			var err error
			switch eventType {
			case "user_registration":
				if e, ok := event.(*UserRegistrationEvent); ok {
					err = h.OnUserRegistration(ctx, e)
				}
			case "user_login":
				if e, ok := event.(*UserLoginEvent); ok {
					err = h.OnUserLogin(ctx, e)
				}
			case "user_logout":
				if e, ok := event.(*UserLogoutEvent); ok {
					err = h.OnUserLogout(ctx, e)
				}
			case "password_change":
				if e, ok := event.(*PasswordChangeEvent); ok {
					err = h.OnPasswordChange(ctx, e)
				}
			case "email_verification":
				if e, ok := event.(*EmailVerificationEvent); ok {
					err = h.OnEmailVerification(ctx, e)
				}
			case "2fa_enabled":
				if e, ok := event.(*TwoFAEvent); ok {
					err = h.On2FAEnabled(ctx, e)
				}
			case "account_locked":
				if e, ok := event.(*AccountLockEvent); ok {
					err = h.OnAccountLocked(ctx, e)
				}
			}

			if err != nil {
				errChan <- err
			}
		}(hook)
	}

	wg.Wait()
	close(errChan)

	// Collect any errors
	var errors []error
	for err := range errChan {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		return fmt.Errorf("hook processing errors: %v", errors)
	}

	return nil
}

// Similar ProcessEvent methods for other event types would be implemented here...

// Start starts the hook manager
func (hm *HookManager) Start(ctx context.Context) error {
	hm.InitializeDefaultHooks()
	return nil
}

// Stop stops the hook manager
func (hm *HookManager) Stop(ctx context.Context) error {
	return nil
}
