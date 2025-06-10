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
	"github.com/google/uuid"
)

// RegisterUserAuthHook registers a user authentication hook implementation with the HookManager.
// This allows the compliance system to listen for user authentication-related events (e.g., registration, login)
// and apply compliance, audit, and risk checks as needed across the platform.
//
// Input: hook - an implementation of the UserAuthHook interface
// Output: none (modifies the HookManager in place)
func (hm *HookManager) RegisterUserAuthHook(hook UserAuthHook) {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	hm.userAuthHooks = append(hm.userAuthHooks, hook)
}

// RegisterTradingHook registers a trading hook implementation with the HookManager.
// This enables the compliance system to process trading-related events (e.g., order placement, execution)
// for risk, audit, and compliance checks throughout the exchange.
//
// Input: hook - an implementation of the TradingHook interface
// Output: none (modifies the HookManager in place)
func (hm *HookManager) RegisterTradingHook(hook TradingHook) {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	hm.tradingHooks = append(hm.tradingHooks, hook)
}

// RegisterFiatHook registers a fiat currency hook implementation with the HookManager.
// This allows the compliance system to monitor fiat-related events (e.g., deposits, withdrawals, transfers)
// and enforce compliance and risk controls for fiat operations.
//
// Input: hook - an implementation of the FiatHook interface
// Output: none (modifies the HookManager in place)
func (hm *HookManager) RegisterFiatHook(hook FiatHook) {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	hm.fiatHooks = append(hm.fiatHooks, hook)
}

// RegisterWalletHook registers a wallet hook implementation with the HookManager.
// This enables the compliance system to listen for wallet-related events (e.g., crypto deposits, withdrawals)
// and apply compliance, audit, and monitoring logic for wallet operations.
//
// Input: hook - an implementation of the WalletHook interface
// Output: none (modifies the HookManager in place)
func (hm *HookManager) RegisterWalletHook(hook WalletHook) {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	hm.walletHooks = append(hm.walletHooks, hook)
}

// RegisterAccountsHook registers an accounts hook implementation with the HookManager.
// This allows the compliance system to process account management events (e.g., account creation, suspension)
// and enforce compliance and audit requirements for user accounts.
//
// Input: hook - an implementation of the AccountsHook interface
// Output: none (modifies the HookManager in place)
func (hm *HookManager) RegisterAccountsHook(hook AccountsHook) {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	hm.accountsHooks = append(hm.accountsHooks, hook)
}

// RegisterMarketMakingHook registers a market making hook implementation with the HookManager.
// This enables the compliance system to monitor market making activities (e.g., strategy changes, inventory updates)
// and apply risk and compliance controls for market making operations.
//
// Input: hook - an implementation of the MarketMakingHook interface
// Output: none (modifies the HookManager in place)
func (hm *HookManager) RegisterMarketMakingHook(hook MarketMakingHook) {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	hm.marketMakingHooks = append(hm.marketMakingHooks, hook)
}

// Built-in compliance hooks implementation

// ComplianceUserAuthHook implements the UserAuthHook interface for compliance checking on user authentication events.
// It integrates with the compliance, audit, monitoring, and risk services to enforce platform policies
// during user registration and login events.
type ComplianceUserAuthHook struct {
	compliance interfaces.ComplianceService
	audit      interfaces.AuditService
	monitoring interfaces.MonitoringService
	risk       risk.RiskService
}

// NewComplianceUserAuthHook creates a new ComplianceUserAuthHook instance.
// This function wires together the compliance, audit, monitoring, and risk services for user authentication events.
//
// Inputs:
//   - compliance: ComplianceService for compliance checks
//   - audit: AuditService for audit logging
//   - monitoring: MonitoringService for alerting
//   - riskSvc: RiskService for risk assessment
//
// Output: pointer to a new ComplianceUserAuthHook
func NewComplianceUserAuthHook(
	compliance interfaces.ComplianceService,
	audit interfaces.AuditService,
	monitoring interfaces.MonitoringService,
	riskSvc risk.RiskService,
) *ComplianceUserAuthHook {
	return &ComplianceUserAuthHook{
		compliance: compliance,
		audit:      audit,
		monitoring: monitoring,
		risk:       riskSvc,
	}
}

// OnUserRegistration handles user registration events for compliance, audit, and risk assessment.
//
// Inputs:
//   - ctx: context for request-scoped values and cancellation
//   - event: UserRegistrationEvent containing registration details
//
// Outputs:
//   - error: error if compliance or risk checks fail, or if audit/monitoring fails
//
// This function parses the user ID, logs the registration event for auditing, performs compliance checks,
// assesses user risk, and generates alerts if the user is deemed high risk. It integrates with the compliance,
// audit, monitoring, and risk services to ensure new users meet platform requirements.
func (h *ComplianceUserAuthHook) OnUserRegistration(ctx context.Context, event *UserRegistrationEvent) error {
	// Parse UserID to UUID
	userUUID, err := uuid.Parse(event.UserID)
	if err != nil {
		log.Printf("Failed to parse UserID as UUID: %v", err)
		userUUID = uuid.New() // Generate a new UUID if parsing fails
	}

	// Audit logging
	if err := h.auditEvent(ctx, "user_registration", event.UserID, event); err != nil {
		log.Printf("Failed to audit user registration: %v", err)
	}

	// Compliance check for new user
	complianceReq := &interfaces.ComplianceRequest{
		UserID:           userUUID,
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
		UserID:           userUUID,
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
			UserID:    event.UserID,
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

// OnUserLogin handles user login events for audit logging and suspicious activity monitoring.
//
// Inputs:
//   - ctx: context for request-scoped values and cancellation
//   - event: UserLoginEvent containing login details
//
// Outputs:
//   - error: error if audit logging or alert generation fails
//
// This function logs the login event for auditing and generates alerts for failed login attempts or suspicious patterns.
// It helps the compliance and monitoring systems detect and respond to potential account compromise or abuse.
func (h *ComplianceUserAuthHook) OnUserLogin(ctx context.Context, event *UserLoginEvent) error {
	// Audit logging
	if err := h.auditEvent(ctx, "user_login", event.UserID, event); err != nil {
		log.Printf("Failed to audit user login: %v", err)
	}

	// Check for suspicious login patterns
	if !event.Success {
		// Failed login attempt - increase monitoring
		alert := &interfaces.MonitoringAlert{
			UserID:    event.UserID,
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

// OnUserLogout handles user logout events for audit logging.
//
// Inputs:
//   - ctx: context for request-scoped values and cancellation
//   - event: UserLogoutEvent containing logout details
//
// Output:
//   - error: error if audit logging fails
//
// This function logs the logout event for auditing purposes, supporting compliance and security monitoring.
func (h *ComplianceUserAuthHook) OnUserLogout(ctx context.Context, event *UserLogoutEvent) error {
	return h.auditEvent(ctx, "user_logout", event.UserID, event)
}

// OnPasswordChange handles password change events for audit logging and alerting on failures.
//
// Inputs:
//   - ctx: context for request-scoped values and cancellation
//   - event: PasswordChangeEvent containing password change details
//
// Output:
//   - error: error if audit logging or alert generation fails
//
// This function logs password change attempts and generates alerts for failed attempts, helping detect account compromise attempts.
func (h *ComplianceUserAuthHook) OnPasswordChange(ctx context.Context, event *PasswordChangeEvent) error {
	if err := h.auditEvent(ctx, "password_change", event.UserID, event); err != nil {
		return err
	}

	if !event.Success {
		alert := &interfaces.MonitoringAlert{
			UserID:    event.UserID,
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

// OnEmailVerification handles email verification events for audit logging.
//
// Inputs:
//   - ctx: context for request-scoped values and cancellation
//   - event: EmailVerificationEvent containing verification details
//
// Output:
//   - error: error if audit logging fails
//
// This function logs email verification events for compliance and user activity tracking.
func (h *ComplianceUserAuthHook) OnEmailVerification(ctx context.Context, event *EmailVerificationEvent) error {
	return h.auditEvent(ctx, "email_verification", event.UserID, event)
}

// On2FAEnabled handles 2FA enablement events for audit logging.
//
// Inputs:
//   - ctx: context for request-scoped values and cancellation
//   - event: TwoFAEvent containing 2FA enablement details
//
// Output:
//   - error: error if audit logging fails
//
// This function logs 2FA enablement events, supporting compliance and security monitoring.
func (h *ComplianceUserAuthHook) On2FAEnabled(ctx context.Context, event *TwoFAEvent) error {
	return h.auditEvent(ctx, "2fa_change", event.UserID, event)
}

// OnAccountLocked handles account lock events for audit logging and alerting.
//
// Inputs:
//   - ctx: context for request-scoped values and cancellation
//   - event: AccountLockEvent containing lock details
//
// Output:
//   - error: error if audit logging or alert generation fails
//
// This function logs account lock events and can trigger alerts, supporting compliance and security incident response.
func (h *ComplianceUserAuthHook) OnAccountLocked(ctx context.Context, event *AccountLockEvent) error {
	if err := h.auditEvent(ctx, "account_locked", event.UserID, event); err != nil {
		return err
	}

	alert := &interfaces.MonitoringAlert{
		UserID:    event.UserID,
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

// ComplianceTradingHook implements the TradingHook interface for compliance checking on trading events.
// It integrates with the compliance, audit, monitoring, manipulation, and risk services to enforce platform policies
// during trading operations such as order placement, execution, and cancellation.
type ComplianceTradingHook struct {
	compliance   interfaces.ComplianceService
	audit        interfaces.AuditService
	monitoring   interfaces.MonitoringService
	manipulation interfaces.ManipulationService
	risk         risk.RiskService
}

// NewComplianceTradingHook creates a new ComplianceTradingHook instance.
// This function wires together the compliance, audit, monitoring, manipulation, and risk services for trading events.
//
// Inputs:
//   - compliance: ComplianceService for compliance checks
//   - audit: AuditService for audit logging
//   - monitoring: MonitoringService for alerting
//   - manipulation: ManipulationService for market manipulation detection
//   - riskSvc: RiskService for risk assessment
//
// Output: pointer to a new ComplianceTradingHook
func NewComplianceTradingHook(
	compliance interfaces.ComplianceService,
	audit interfaces.AuditService,
	monitoring interfaces.MonitoringService,
	manipulation interfaces.ManipulationService,
	riskSvc risk.RiskService,
) *ComplianceTradingHook {
	return &ComplianceTradingHook{
		compliance:   compliance,
		audit:        audit,
		monitoring:   monitoring,
		manipulation: manipulation,
		risk:         riskSvc,
	}
}

// OnOrderPlaced handles order placement events for compliance, audit, and risk assessment.
//
// Inputs:
//   - ctx: context for request-scoped values and cancellation
//   - event: OrderPlacedEvent containing order placement details
//
// Output:
//   - error: error if compliance or risk checks fail, or if audit/monitoring fails
//
// This function parses the user ID, logs the order placement event for auditing, performs risk assessment
// for the transaction, and integrates with the compliance, audit, monitoring, and risk services to ensure
// all new orders meet platform requirements and risk controls.
func (h *ComplianceTradingHook) OnOrderPlaced(ctx context.Context, event *OrderPlacedEvent) error {
	// Parse UserID to UUID for risk assessment
	userUUID, err := uuid.Parse(event.UserID)
	if err != nil {
		log.Printf("Failed to parse UserID as UUID: %v", err)
		userUUID = uuid.New() // Generate a new UUID if parsing fails
	}

	// Audit logging
	if err := h.auditEvent(ctx, "order_placed", event.UserID, event); err != nil {
		log.Printf("Failed to audit order placement: %v", err)
	}

	// Risk assessment for the transaction
	txRiskCtx := &risk.TransactionRiskContext{
		UserID:          userUUID,
		TransactionID:   uuid.New(), // Generate a proper transaction ID
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
		UserID:    userUUID,
		Market:    event.Market,
		Orders: []interfaces.Order{
			{
				ID:        event.OrderID,
				UserID:    userUUID,
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

	manipResult, err := h.manipulation.DetectManipulation(ctx, *manipReq)
	if err != nil {
		log.Printf("Manipulation detection failed for order %s: %v", event.OrderID, err)
	}

	// Generate alerts for high-risk transactions
	if riskAssessment != nil && riskAssessment.Level >= risk.RiskLevelHigh {
		alert := &interfaces.MonitoringAlert{
			UserID:    event.UserID,
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

// OnOrderExecuted handles order execution events for audit logging.
//
// Inputs:
//   - ctx: context for request-scoped values and cancellation
//   - event: OrderExecutedEvent containing execution details
//
// Output:
//   - error: error if audit logging fails
//
// This function logs order execution events for compliance and trading activity tracking.
func (h *ComplianceTradingHook) OnOrderExecuted(ctx context.Context, event *OrderExecutedEvent) error {
	return h.auditEvent(ctx, "order_executed", event.UserID, event)
}

// OnOrderCancelled handles order cancellation events for audit logging.
//
// Inputs:
//   - ctx: context for request-scoped values and cancellation
//   - event: OrderCancelledEvent containing cancellation details
//
// Output:
//   - error: error if audit logging fails
//
// This function logs order cancellation events for compliance and trading activity tracking.
func (h *ComplianceTradingHook) OnOrderCancelled(ctx context.Context, event *OrderCancelledEvent) error {
	return h.auditEvent(ctx, "order_cancelled", event.UserID, event)
}

// OnTradeExecuted handles trade execution events for audit logging and wash trading detection.
//
// Inputs:
//   - ctx: context for request-scoped values and cancellation
//   - event: TradeExecutedEvent containing trade details
//
// Output:
//   - error: error if audit logging or alert generation fails
//
// This function logs trade execution events for both buyer and seller, and generates alerts if the same user
// is both buyer and seller (potential wash trading). It supports compliance, monitoring, and market integrity.
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
			UserID:    event.BuyerID,
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

// OnPositionUpdated handles position update events for audit logging.
//
// Inputs:
//   - ctx: context for request-scoped values and cancellation
//   - event: PositionUpdateEvent containing position update details
//
// Output:
//   - error: error if audit logging fails
//
// This function logs position update events for compliance and trading activity tracking.
func (h *ComplianceTradingHook) OnPositionUpdated(ctx context.Context, event *PositionUpdateEvent) error {
	return h.auditEvent(ctx, "position_updated", event.UserID, event)
}

// OnMarketDataReceived handles market data events for manipulation detection or logging.
//
// Inputs:
//   - ctx: context for request-scoped values and cancellation
//   - event: MarketDataEvent containing market data details
//
// Output:
//   - error: error if processing fails (currently always returns nil)
//
// This function can be extended to detect market manipulation or anomalies based on market data.
func (h *ComplianceTradingHook) OnMarketDataReceived(ctx context.Context, event *MarketDataEvent) error {
	// This could be used for market manipulation detection
	// For now, just basic logging
	return nil
}

// TriggerHooks processes the given event through the appropriate registered hooks based on event type.
//
// Inputs:
//   - ctx: context for request-scoped values and cancellation
//   - event: the event to process (type switch determines which hooks to call)
//
// Output:
//   - error: error if any hook fails to process the event
//
// This function acts as a central dispatcher for compliance, risk, and audit hooks, ensuring all relevant
// hooks are triggered for each event type across the platform.
func (hm *HookManager) TriggerHooks(ctx context.Context, event interface{}) error {
	switch e := event.(type) {
	// User Authentication Events
	case *UserRegistrationEvent:
		return hm.ProcessUserAuthEvent(ctx, "user_registration", e)
	case *UserLoginEvent:
		return hm.ProcessUserAuthEvent(ctx, "user_login", e)
	case *UserLogoutEvent:
		return hm.ProcessUserAuthEvent(ctx, "user_logout", e)
	case *PasswordChangeEvent:
		return hm.ProcessUserAuthEvent(ctx, "password_change", e)
	case *EmailVerificationEvent:
		return hm.ProcessUserAuthEvent(ctx, "email_verification", e)
	case *TwoFAEvent:
		return hm.ProcessUserAuthEvent(ctx, "2fa_enabled", e)
	case *AccountLockEvent:
		return hm.ProcessUserAuthEvent(ctx, "account_locked", e)

	// Account Events
	case *AccountCreatedEvent:
		return hm.ProcessAccountEvent(ctx, "account_created", e)
	case *AccountSuspendedEvent:
		return hm.ProcessAccountEvent(ctx, "account_suspended", e)
	case *AccountKYCEvent:
		return hm.ProcessAccountEvent(ctx, "account_kyc", e)
	case *AccountTierEvent:
		return hm.ProcessAccountEvent(ctx, "account_tier", e)
	case *AccountDormancyEvent:
		return hm.ProcessAccountEvent(ctx, "account_dormancy", e)

	// Trading Events
	case *OrderPlacedEvent:
		return hm.ProcessTradingEvent(ctx, "order_placed", e)
	case *OrderExecutedEvent:
		return hm.ProcessTradingEvent(ctx, "order_executed", e)
	case *TradeExecutedEvent:
		return hm.ProcessTradingEvent(ctx, "trade_executed", e)

	// Fiat Events
	case *FiatDepositEvent:
		return hm.ProcessFiatEvent(ctx, "fiat_deposit", e)
	case *FiatWithdrawalEvent:
		return hm.ProcessFiatEvent(ctx, "fiat_withdrawal", e)
	case *FiatTransferEvent:
		return hm.ProcessFiatEvent(ctx, "fiat_transfer", e)

	// Wallet Events
	case *CryptoDepositEvent:
		return hm.ProcessWalletEvent(ctx, "crypto_deposit", e)
	case *WalletBalanceUpdateEvent:
		return hm.ProcessWalletEvent(ctx, "wallet_balance_update", e)
	case *WalletAddressEvent:
		return hm.ProcessWalletEvent(ctx, "wallet_address", e)
	case *WalletStakingEvent:
		return hm.ProcessWalletEvent(ctx, "wallet_staking", e)

	// Market Making Events
	case *MMStrategyEvent:
		return hm.ProcessMarketMakingEvent(ctx, "mm_strategy", e)
	case *MMQuoteEvent:
		return hm.ProcessMarketMakingEvent(ctx, "mm_quote", e)
	case *MMOrderEvent:
		return hm.ProcessMarketMakingEvent(ctx, "mm_order", e)
	case *MMInventoryEvent:
		return hm.ProcessMarketMakingEvent(ctx, "mm_inventory", e)
	case *MMRiskEvent:
		return hm.ProcessMarketMakingEvent(ctx, "mm_risk", e)
	case *MMPnLEvent:
		return hm.ProcessMarketMakingEvent(ctx, "mm_pnl", e)
	case *MMPerformanceEvent:
		return hm.ProcessMarketMakingEvent(ctx, "mm_performance", e)

	default:
		return fmt.Errorf("unsupported event type: %T", event)
	}
}

// Helper methods for processing different event categories
func (hm *HookManager) ProcessAccountEvent(ctx context.Context, eventType string, event interface{}) error {
	hm.mu.RLock()
	hooks := make([]AccountsHook, len(hm.accountsHooks))
	copy(hooks, hm.accountsHooks)
	hm.mu.RUnlock()

	for _, hook := range hooks {
		switch eventType {
		case "account_created":
			if e, ok := event.(*AccountCreatedEvent); ok {
				if err := hook.OnAccountCreated(ctx, e); err != nil {
					return err
				}
			}
		case "account_suspended":
			if e, ok := event.(*AccountSuspendedEvent); ok {
				if err := hook.OnAccountSuspended(ctx, e); err != nil {
					return err
				}
			}
		case "account_kyc":
			if e, ok := event.(*AccountKYCEvent); ok {
				if err := hook.OnKYCStatusChange(ctx, e); err != nil {
					return err
				}
			}
		case "account_tier":
			if e, ok := event.(*AccountTierEvent); ok {
				if err := hook.OnTierChange(ctx, e); err != nil {
					return err
				}
			}
		case "account_dormancy":
			if e, ok := event.(*AccountDormancyEvent); ok {
				if err := hook.OnDormancyStatusChange(ctx, e); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// ProcessTradingEvent dispatches trading-related events (order placed, executed, trade executed, etc.) to all registered TradingHooks.
//
// Inputs:
//   - ctx: context for request-scoped values and cancellation
//   - eventType: string identifier for the trading event type (e.g., "order_placed")
//   - event: event payload (should match the expected struct for the eventType)
//
// Output:
//   - error: error if any hook fails to process the event
//
// This function ensures that all trading compliance, risk, and monitoring hooks are invoked for each trading event.
// It is used by the trading module and other platform components to enforce compliance on trading operations.
func (hm *HookManager) ProcessTradingEvent(ctx context.Context, eventType string, event interface{}) error {
	hm.mu.RLock()
	hooks := make([]TradingHook, len(hm.tradingHooks))
	copy(hooks, hm.tradingHooks)
	hm.mu.RUnlock()

	for _, hook := range hooks {
		switch eventType {
		case "order_placed":
			if e, ok := event.(*OrderPlacedEvent); ok {
				if err := hook.OnOrderPlaced(ctx, e); err != nil {
					return err
				}
			}
		case "order_executed":
			if e, ok := event.(*OrderExecutedEvent); ok {
				if err := hook.OnOrderExecuted(ctx, e); err != nil {
					return err
				}
			}
		case "trade_executed":
			if e, ok := event.(*TradeExecutedEvent); ok {
				if err := hook.OnTradeExecuted(ctx, e); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// ProcessFiatEvent dispatches fiat-related events (deposit, withdrawal, transfer) to all registered FiatHooks.
//
// Inputs:
//   - ctx: context for request-scoped values and cancellation
//   - eventType: string identifier for the fiat event type (e.g., "fiat_deposit")
//   - event: event payload (should match the expected struct for the eventType)
//
// Output:
//   - error: error if any hook fails to process the event
//
// This function ensures that all fiat compliance, risk, and monitoring hooks are invoked for each fiat event.
// It is used by the fiat module and other platform components to enforce compliance on fiat operations.
func (hm *HookManager) ProcessFiatEvent(ctx context.Context, eventType string, event interface{}) error {
	hm.mu.RLock()
	hooks := make([]FiatHook, len(hm.fiatHooks))
	copy(hooks, hm.fiatHooks)
	hm.mu.RUnlock()

	for _, hook := range hooks {
		switch eventType {
		case "fiat_deposit":
			if e, ok := event.(*FiatDepositEvent); ok {
				if err := hook.OnFiatDeposit(ctx, e); err != nil {
					return err
				}
			}
		case "fiat_withdrawal":
			if e, ok := event.(*FiatWithdrawalEvent); ok {
				if err := hook.OnFiatWithdrawal(ctx, e); err != nil {
					return err
				}
			}
		case "fiat_transfer":
			if e, ok := event.(*FiatTransferEvent); ok {
				if err := hook.OnFiatTransfer(ctx, e); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// ProcessWalletEvent dispatches wallet-related events (crypto deposit, balance update, address, staking, etc.) to all registered WalletHooks.
//
// Inputs:
//   - ctx: context for request-scoped values and cancellation
//   - eventType: string identifier for the wallet event type (e.g., "crypto_deposit")
//   - event: event payload (should match the expected struct for the eventType)
//
// Output:
//   - error: error if any hook fails to process the event
//
// This function ensures that all wallet compliance, risk, and monitoring hooks are invoked for each wallet event.
// It is used by the wallet module and other platform components to enforce compliance on wallet operations.
func (hm *HookManager) ProcessWalletEvent(ctx context.Context, eventType string, event interface{}) error {
	hm.mu.RLock()
	hooks := make([]WalletHook, len(hm.walletHooks))
	copy(hooks, hm.walletHooks)
	hm.mu.RUnlock()

	for _, hook := range hooks {
		switch eventType {
		case "crypto_deposit":
			if e, ok := event.(*CryptoDepositEvent); ok {
				if err := hook.OnCryptoDeposit(ctx, e); err != nil {
					return err
				}
			}
		case "wallet_balance_update":
			if e, ok := event.(*WalletBalanceUpdateEvent); ok {
				if err := hook.OnBalanceUpdate(ctx, e); err != nil {
					return err
				}
			}
		case "wallet_address":
			if e, ok := event.(*WalletAddressEvent); ok {
				// Convert WalletAddressEvent to AddressGeneratedEvent
				addressEvent := &AddressGeneratedEvent{
					BaseEvent: e.BaseEvent,
					Currency:  e.Currency,
					Network:   e.Network,
					Address:   e.Address,
				}
				if err := hook.OnAddressGenerated(ctx, addressEvent); err != nil {
					return err
				}
			}
		case "wallet_staking":
			if e, ok := event.(*WalletStakingEvent); ok {
				if err := hook.OnStaking(ctx, e); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// ProcessMarketMakingEvent dispatches market making-related events (strategy, quote, order, inventory, risk, pnl, performance) to all registered MarketMakingHooks.
//
// Inputs:
//   - ctx: context for request-scoped values and cancellation
//   - eventType: string identifier for the market making event type (e.g., "mm_strategy")
//   - event: event payload (should match the expected struct for the eventType)
//
// Output:
//   - error: error if any hook fails to process the event
//
// This function ensures that all market making compliance, risk, and monitoring hooks are invoked for each market making event.
// It is used by the market making module and other platform components to enforce compliance on market making operations.
func (hm *HookManager) ProcessMarketMakingEvent(ctx context.Context, eventType string, event interface{}) error {
	hm.mu.RLock()
	hooks := make([]MarketMakingHook, len(hm.marketMakingHooks))
	copy(hooks, hm.marketMakingHooks)
	hm.mu.RUnlock()

	for _, hook := range hooks {
		switch eventType {
		case "mm_strategy":
			if e, ok := event.(*MMStrategyEvent); ok {
				if err := hook.OnStrategyChange(ctx, e); err != nil {
					return err
				}
			}
		case "mm_quote":
			if e, ok := event.(*MMQuoteEvent); ok {
				if err := hook.OnQuoteUpdate(ctx, e); err != nil {
					return err
				}
			}
		case "mm_order":
			if e, ok := event.(*MMOrderEvent); ok {
				if err := hook.OnOrderManagement(ctx, e); err != nil {
					return err
				}
			}
		case "mm_inventory":
			if e, ok := event.(*MMInventoryEvent); ok {
				if err := hook.OnInventoryRebalancing(ctx, e); err != nil {
					return err
				}
			}
		case "mm_risk":
			if e, ok := event.(*MMRiskEvent); ok {
				if err := hook.OnRiskBreach(ctx, e); err != nil {
					return err
				}
			}
		case "mm_pnl":
			if e, ok := event.(*MMPnLEvent); ok {
				if err := hook.OnPnLAlert(ctx, e); err != nil {
					return err
				}
			}
		case "mm_performance":
			if e, ok := event.(*MMPerformanceEvent); ok {
				if err := hook.OnPerformanceReport(ctx, e); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// auditEvent logs an audit event for compliance and monitoring purposes.
//
// Inputs:
//   - ctx: context for request-scoped values and cancellation
//   - eventType: string identifier for the event type (e.g., "user_login")
//   - userID: user identifier (string or uuid.UUID)
//   - data: event payload (arbitrary struct)
//
// Output:
//   - error: error if audit logging fails
//
// This function creates and sends an audit event to the platform's audit service, supporting compliance,
// monitoring, and traceability of user and system actions.
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

	// Handle different UserID types and convert to *uuid.UUID
	if uid, ok := userID.(string); ok {
		if parsedUUID, err := uuid.Parse(uid); err == nil {
			auditEvent.UserID = &parsedUUID
		}
	} else if uid, ok := userID.(uuid.UUID); ok {
		auditEvent.UserID = &uid
	}

	return h.audit.CreateEvent(ctx, auditEvent)
}

// auditEvent logs a trading-related audit event for compliance and monitoring purposes.
//
// Inputs:
//   - ctx: context for request-scoped values and cancellation
//   - eventType: string identifier for the trading event type (e.g., "order_placed")
//   - userID: user identifier (string or uuid.UUID)
//   - data: event payload (arbitrary struct)
//
// Output:
//   - error: error if audit logging fails
//
// This function creates and sends a trading audit event to the platform's audit service, supporting compliance,
// monitoring, and traceability of trading actions.
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

	// Handle different UserID types and convert to *uuid.UUID
	if uid, ok := userID.(string); ok {
		if parsedUUID, err := uuid.Parse(uid); err == nil {
			auditEvent.UserID = &parsedUUID
		}
	} else if uid, ok := userID.(uuid.UUID); ok {
		auditEvent.UserID = &uid
	}
	return h.audit.CreateEvent(ctx, auditEvent)
}

// InitializeDefaultHooks registers all built-in compliance hooks with the HookManager.
// This function is called during platform startup to ensure that all core compliance, audit, monitoring,
// and risk hooks are active and ready to process events from user authentication, trading, and other modules.
//
// Input: none (uses HookManager dependencies)
// Output: none (modifies the HookManager in place)
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

// Start initializes and activates all built-in compliance, risk, and monitoring hooks for the platform.
//
// Input:
//   - ctx: context for request-scoped values and cancellation
//
// Output:
//   - error: error if initialization fails (currently always returns nil)
//
// This function should be called during platform startup to ensure all hooks are registered and ready to process events.
func (hm *HookManager) Start(ctx context.Context) error {
	hm.InitializeDefaultHooks()
	return nil
}

// Stop gracefully shuts down the hook manager and releases any resources if needed.
//
// Input:
//   - ctx: context for request-scoped values and cancellation
//
// Output:
//   - error: error if shutdown fails (currently always returns nil)
//
// This function can be extended to perform cleanup or shutdown logic for compliance hooks if required in the future.
func (hm *HookManager) Stop(ctx context.Context) error {
	return nil
}
