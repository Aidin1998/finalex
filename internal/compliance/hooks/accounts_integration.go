package hooks

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// AccountsIntegration handles compliance integration for account operations
type AccountsIntegration struct {
	hookManager *HookManager
	logger      *zap.Logger
}

// NewAccountsIntegration creates a new accounts integration
func NewAccountsIntegration(hookManager *HookManager, logger *zap.Logger) *AccountsIntegration {
	return &AccountsIntegration{
		hookManager: hookManager,
		logger:      logger,
	}
}

// OnAccountCreation handles account creation events
func (a *AccountsIntegration) OnAccountCreation(ctx context.Context, userID string, accountType string, metadata map[string]interface{}) error {
	event := AccountCreationEvent{
		BaseEvent: BaseEvent{
			Type:      EventTypeAccountCreation,
			UserID:    userID,
			Timestamp: getCurrentTimestamp(),
			Module:    ModuleAccounts,
		},
		AccountType: accountType,
		Metadata:    metadata,
	}

	a.logger.Info("Processing account creation",
		zap.String("user_id", userID),
		zap.String("account_type", accountType),
		zap.Any("metadata", metadata),
	)

	if err := a.hookManager.TriggerHooks(ctx, event); err != nil {
		a.logger.Error("Failed to trigger account creation hooks",
			zap.Error(err),
			zap.String("user_id", userID),
			zap.String("account_type", accountType),
		)
		return fmt.Errorf("failed to process account creation compliance: %w", err)
	}

	return nil
}

// OnAccountUpdate handles account update events
func (a *AccountsIntegration) OnAccountUpdate(ctx context.Context, userID string, updateType string, oldData, newData map[string]interface{}) error {
	event := AccountUpdateEvent{
		BaseEvent: BaseEvent{
			Type:      EventTypeAccountUpdate,
			UserID:    userID,
			Timestamp: getCurrentTimestamp(),
			Module:    ModuleAccounts,
		},
		UpdateType: updateType,
		OldData:    oldData,
		NewData:    newData,
	}

	a.logger.Info("Processing account update",
		zap.String("user_id", userID),
		zap.String("update_type", updateType),
		zap.Any("old_data", oldData),
		zap.Any("new_data", newData),
	)

	if err := a.hookManager.TriggerHooks(ctx, event); err != nil {
		a.logger.Error("Failed to trigger account update hooks",
			zap.Error(err),
			zap.String("user_id", userID),
			zap.String("update_type", updateType),
		)
		return fmt.Errorf("failed to process account update compliance: %w", err)
	}

	return nil
}

// OnAccountSuspension handles account suspension events
func (a *AccountsIntegration) OnAccountSuspension(ctx context.Context, userID string, reason string, suspendedBy string, duration int64) error {
	// Parse UUID
	suspendedByUUID, err := uuid.Parse(suspendedBy)
	if err != nil {
		return fmt.Errorf("invalid suspended_by UUID: %w", err)
	}

	event := AccountSuspensionEvent{
		BaseEvent: BaseEvent{
			Type:      EventTypeAccountSuspension,
			UserID:    userID,
			Timestamp: getCurrentTimestamp(),
			Module:    ModuleAccounts,
			ID:        uuid.New(),
		},
		Reason:      reason,
		SuspendedBy: suspendedByUUID,
		Duration:    duration,
	}

	a.logger.Info("Processing account suspension",
		zap.String("user_id", userID),
		zap.String("reason", reason),
		zap.String("suspended_by", suspendedBy),
		zap.Int64("duration", duration),
	)

	if err := a.hookManager.TriggerHooks(ctx, event); err != nil {
		a.logger.Error("Failed to trigger account suspension hooks",
			zap.Error(err),
			zap.String("user_id", userID),
			zap.String("reason", reason),
		)
		return fmt.Errorf("failed to process account suspension compliance: %w", err)
	}

	return nil
}

// OnAccountReactivation handles account reactivation events
func (a *AccountsIntegration) OnAccountReactivation(ctx context.Context, userID string, reason string, reactivatedBy string) error {
	// Parse UUID
	reactivatedByUUID, err := uuid.Parse(reactivatedBy)
	if err != nil {
		return fmt.Errorf("invalid reactivated_by UUID: %w", err)
	}

	event := AccountReactivationEvent{
		BaseEvent: BaseEvent{
			Type:      EventTypeAccountReactivation,
			UserID:    userID,
			Timestamp: getCurrentTimestamp(),
			Module:    ModuleAccounts,
			ID:        uuid.New(),
		},
		Reason:        reason,
		ReactivatedBy: reactivatedByUUID,
	}

	a.logger.Info("Processing account reactivation",
		zap.String("user_id", userID),
		zap.String("reason", reason),
		zap.String("reactivated_by", reactivatedBy),
	)

	if err := a.hookManager.TriggerHooks(ctx, event); err != nil {
		a.logger.Error("Failed to trigger account reactivation hooks",
			zap.Error(err),
			zap.String("user_id", userID),
			zap.String("reason", reason),
		)
		return fmt.Errorf("failed to process account reactivation compliance: %w", err)
	}

	return nil
}

// OnPermissionChange handles permission change events
func (a *AccountsIntegration) OnPermissionChange(ctx context.Context, userID string, permission string, action string, changedBy string) error {
	// Parse UUID
	changedByUUID, err := uuid.Parse(changedBy)
	if err != nil {
		return fmt.Errorf("invalid changed_by UUID: %w", err)
	}

	event := AccountPermissionEvent{
		BaseEvent: BaseEvent{
			Type:      EventTypeAccountPermission,
			UserID:    userID,
			Timestamp: getCurrentTimestamp(),
			Module:    ModuleAccounts,
			ID:        uuid.New(),
		},
		Permission: permission,
		Action:     action,
		ChangedBy:  changedByUUID,
	}

	a.logger.Info("Processing permission change",
		zap.String("user_id", userID),
		zap.String("permission", permission),
		zap.String("action", action),
		zap.String("changed_by", changedBy),
	)

	if err := a.hookManager.TriggerHooks(ctx, event); err != nil {
		a.logger.Error("Failed to trigger permission change hooks",
			zap.Error(err),
			zap.String("user_id", userID),
			zap.String("permission", permission),
			zap.String("action", action),
		)
		return fmt.Errorf("failed to process permission change compliance: %w", err)
	}

	return nil
}

// OnKYCStatusChange handles KYC status change events
func (a *AccountsIntegration) OnKYCStatusChange(ctx context.Context, userID string, oldStatus, newStatus string, verifiedBy string, documents []string) error {
	// Parse UUID
	verifiedByUUID, err := uuid.Parse(verifiedBy)
	if err != nil {
		return fmt.Errorf("invalid verified_by UUID: %w", err)
	}

	event := AccountKYCEvent{
		BaseEvent: BaseEvent{
			Type:      EventTypeAccountKYC,
			UserID:    userID,
			Timestamp: getCurrentTimestamp(),
			Module:    ModuleAccounts,
			ID:        uuid.New(),
		},
		OldStatus:  oldStatus,
		NewStatus:  newStatus,
		VerifiedBy: verifiedByUUID,
		Documents:  documents,
	}

	a.logger.Info("Processing KYC status change",
		zap.String("user_id", userID),
		zap.String("old_status", oldStatus),
		zap.String("new_status", newStatus),
		zap.String("verified_by", verifiedBy),
		zap.Strings("documents", documents),
	)

	if err := a.hookManager.TriggerHooks(ctx, event); err != nil {
		a.logger.Error("Failed to trigger KYC status change hooks",
			zap.Error(err),
			zap.String("user_id", userID),
			zap.String("old_status", oldStatus),
			zap.String("new_status", newStatus),
		)
		return fmt.Errorf("failed to process KYC status change compliance: %w", err)
	}

	return nil
}

// OnTierChange handles account tier change events
func (a *AccountsIntegration) OnTierChange(ctx context.Context, userID string, oldTier, newTier string, changedBy string, reason string) error {
	// Parse UUID
	changedByUUID, err := uuid.Parse(changedBy)
	if err != nil {
		return fmt.Errorf("invalid changed_by UUID: %w", err)
	}

	// Convert tier strings to integers
	oldTierInt, err := strconv.Atoi(oldTier)
	if err != nil {
		return fmt.Errorf("invalid old_tier value: %w", err)
	}

	newTierInt, err := strconv.Atoi(newTier)
	if err != nil {
		return fmt.Errorf("invalid new_tier value: %w", err)
	}

	event := AccountTierEvent{
		BaseEvent: BaseEvent{
			Type:      EventTypeAccountTier,
			UserID:    userID,
			Timestamp: getCurrentTimestamp(),
			Module:    ModuleAccounts,
			ID:        uuid.New(),
		},
		OldTier:   oldTierInt,
		NewTier:   newTierInt,
		ChangedBy: changedByUUID,
		Reason:    reason,
	}

	a.logger.Info("Processing tier change",
		zap.String("user_id", userID),
		zap.String("old_tier", oldTier),
		zap.String("new_tier", newTier),
		zap.String("changed_by", changedBy),
		zap.String("reason", reason),
	)

	if err := a.hookManager.TriggerHooks(ctx, event); err != nil {
		a.logger.Error("Failed to trigger tier change hooks",
			zap.Error(err),
			zap.String("user_id", userID),
			zap.String("old_tier", oldTier),
			zap.String("new_tier", newTier),
		)
		return fmt.Errorf("failed to process tier change compliance: %w", err)
	}

	return nil
}

// OnDormancyStatusChange handles account dormancy status change events
func (a *AccountsIntegration) OnDormancyStatusChange(ctx context.Context, userID string, isDormant bool, lastActivity int64, reason string) error {
	// Convert int64 to time.Time
	lastActivityTime := time.Unix(lastActivity, 0)

	event := AccountDormancyEvent{
		BaseEvent: BaseEvent{
			Type:      EventTypeAccountDormancy,
			UserID:    userID,
			Timestamp: getCurrentTimestamp(),
			Module:    ModuleAccounts,
			ID:        uuid.New(),
		},
		IsDormant:    isDormant,
		LastActivity: lastActivityTime,
		Reason:       reason,
	}

	a.logger.Info("Processing dormancy status change",
		zap.String("user_id", userID),
		zap.Bool("is_dormant", isDormant),
		zap.Int64("last_activity", lastActivity),
		zap.String("reason", reason),
	)

	if err := a.hookManager.TriggerHooks(ctx, event); err != nil {
		a.logger.Error("Failed to trigger dormancy status change hooks",
			zap.Error(err),
			zap.String("user_id", userID),
			zap.Bool("is_dormant", isDormant),
		)
		return fmt.Errorf("failed to process dormancy status change compliance: %w", err)
	}

	return nil
}
