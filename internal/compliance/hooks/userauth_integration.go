// Package hooks provides userauth module integration with compliance
package hooks

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// UserAuthIntegration provides integration between userauth and compliance modules
type UserAuthIntegration struct {
	hookManager *HookManager
}

// NewUserAuthIntegration creates a new userauth integration
func NewUserAuthIntegration(hookManager *HookManager) *UserAuthIntegration {
	return &UserAuthIntegration{
		hookManager: hookManager,
	}
}

// OnUserRegistration should be called when a user registers
func (ui *UserAuthIntegration) OnUserRegistration(ctx context.Context, userID uuid.UUID, email, country, ipAddress, userAgent, deviceID string, metadata map[string]interface{}) error {
	event := &UserRegistrationEvent{
		BaseEvent: BaseEvent{
			Type:      EventTypeUserRegistration,
			UserID:    userID.String(),
			Timestamp: time.Now(),
			Module:    ModuleUserAuth,
			ID:        uuid.New(),
		},
		Email:     email,
		Country:   country,
		IPAddress: ipAddress,
		UserAgent: userAgent,
		DeviceID:  deviceID,
		Metadata:  metadata,
	}

	return ui.hookManager.TriggerHooks(ctx, event)
}

// OnUserLogin should be called when a user attempts to login
func (ui *UserAuthIntegration) OnUserLogin(ctx context.Context, userID, sessionID uuid.UUID, ipAddress, userAgent, deviceID, country, loginMethod string, success bool, failReason string, metadata map[string]interface{}) error {
	event := &UserLoginEvent{
		BaseEvent: BaseEvent{
			Type:      EventTypeUserLogin,
			UserID:    userID.String(),
			Timestamp: time.Now(),
			Module:    ModuleUserAuth,
			ID:        uuid.New(),
		},
		IPAddress:   ipAddress,
		UserAgent:   userAgent,
		Success:     success,
		DeviceID:    deviceID,
		Country:     country,
		FailReason:  failReason,
		LoginMethod: loginMethod,
	}

	return ui.hookManager.TriggerHooks(ctx, event)
}

// OnUserLogout should be called when a user logs out
func (ui *UserAuthIntegration) OnUserLogout(ctx context.Context, userID, sessionID uuid.UUID) error {
	event := &UserLogoutEvent{
		BaseEvent: BaseEvent{
			Type:      EventTypeUserLogout,
			UserID:    userID.String(),
			Timestamp: time.Now(),
			Module:    ModuleUserAuth,
			ID:        uuid.New(),
		},
		IPAddress: "", // This will need to be passed from the caller if needed
		Duration:  0,  // This will need to be calculated if needed
	}

	return ui.hookManager.TriggerHooks(ctx, event)
}

// OnPasswordChange should be called when a user changes their password
func (ui *UserAuthIntegration) OnPasswordChange(ctx context.Context, userID uuid.UUID, ipAddress string, success bool) error {
	event := &PasswordChangeEvent{
		BaseEvent: BaseEvent{
			Type:      EventTypePasswordChange,
			UserID:    userID.String(),
			Timestamp: time.Now(),
			Module:    ModuleUserAuth,
			ID:        uuid.New(),
		},
		IPAddress: ipAddress,
		Reason:    "", // This could be passed as a parameter if needed
		Success:   success,
	}

	return ui.hookManager.TriggerHooks(ctx, event)
}

// OnEmailVerification should be called when a user verifies their email
func (ui *UserAuthIntegration) OnEmailVerification(ctx context.Context, userID uuid.UUID, email string, verified bool) error {
	event := &EmailVerificationEvent{
		BaseEvent: BaseEvent{
			Type:      EventTypeEmailVerification,
			UserID:    userID.String(),
			Timestamp: time.Now(),
			Module:    ModuleUserAuth,
			ID:        uuid.New(),
		},
		Email:      email,
		Verified:   verified,
		VerifiedAt: time.Now(),
	}

	return ui.hookManager.TriggerHooks(ctx, event)
}

// On2FAEnabled should be called when 2FA is enabled/disabled
func (ui *UserAuthIntegration) On2FAEnabled(ctx context.Context, userID uuid.UUID, method string, enabled bool) error {
	event := &TwoFAEvent{
		BaseEvent: BaseEvent{
			Type:      EventTypeTwoFA,
			UserID:    userID.String(),
			Timestamp: time.Now(),
			Module:    ModuleUserAuth,
			ID:        uuid.New(),
		},
		Method:    method,
		Enabled:   enabled,
		IPAddress: "", // This could be passed as a parameter if needed
	}

	return ui.hookManager.TriggerHooks(ctx, event)
}

// OnAccountLocked should be called when an account is locked
func (ui *UserAuthIntegration) OnAccountLocked(ctx context.Context, userID, lockedBy uuid.UUID, reason string, duration *time.Duration) error {
	event := &AccountLockEvent{
		BaseEvent: BaseEvent{
			Type:      EventTypeAccountLock,
			UserID:    userID.String(),
			Timestamp: time.Now(),
			Module:    ModuleUserAuth,
			ID:        uuid.New(),
		},
		Reason:   reason,
		LockedBy: lockedBy,
		Duration: duration,
	}

	return ui.hookManager.TriggerHooks(ctx, event)
}
