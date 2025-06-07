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
	risk       risk.RiskService
}

// NewComplianceUserAuthHook creates a new compliance user auth hook
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
	risk         risk.RiskService
}

// NewComplianceTradingHook creates a new compliance trading hook
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

	manipResult, err := h.manipulation.DetectManipulation(ctx, *manipReq)
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

// TriggerHooks processes the given event through appropriate hooks based on event type
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
				if err := hook.OnAccountCreation(ctx, e); err != nil {
					return err
				}
			}
		case "account_suspended":
			if e, ok := event.(*AccountSuspendedEvent); ok {
				if err := hook.OnAccountSuspension(ctx, e); err != nil {
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
				if err := hook.OnAddressGenerated(ctx, e); err != nil {
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
