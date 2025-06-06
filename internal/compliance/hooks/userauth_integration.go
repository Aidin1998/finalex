// Package hooks provides userauth module integration with compliance
package hooks

import (
	"context"
	"time"

	"github.com/Aidin1998/finalex/internal/compliance/hooks"
	"github.com/google/uuid"
)

// UserAuthIntegration provides integration between userauth and compliance modules
type UserAuthIntegration struct {
	hookManager *hooks.HookManager
}

// NewUserAuthIntegration creates a new userauth integration
func NewUserAuthIntegration(hookManager *hooks.HookManager) *UserAuthIntegration {
	return &UserAuthIntegration{
		hookManager: hookManager,
	}
}

// OnUserRegistration should be called when a user registers
func (ui *UserAuthIntegration) OnUserRegistration(ctx context.Context, userID uuid.UUID, email, country, ipAddress, userAgent, deviceID string, metadata map[string]interface{}) error {
	event := &hooks.UserRegistrationEvent{
		UserID:    userID,
		Email:     email,
		Country:   country,
		IPAddress: ipAddress,
		UserAgent: userAgent,
		DeviceID:  deviceID,
		Timestamp: time.Now(),
		Metadata:  metadata,
	}

	return ui.hookManager.ProcessUserAuthEvent(ctx, "user_registration", event)
}

// OnUserLogin should be called when a user attempts to login
func (ui *UserAuthIntegration) OnUserLogin(ctx context.Context, userID, sessionID uuid.UUID, ipAddress, userAgent, deviceID, country, loginMethod string, success bool, failReason string, metadata map[string]interface{}) error {
	event := &hooks.UserLoginEvent{
		UserID:      userID,
		SessionID:   sessionID,
		IPAddress:   ipAddress,
		UserAgent:   userAgent,
		DeviceID:    deviceID,
		Country:     country,
		LoginMethod: loginMethod,
		Success:     success,
		FailReason:  failReason,
		Timestamp:   time.Now(),
		Metadata:    metadata,
	}

	return ui.hookManager.ProcessUserAuthEvent(ctx, "user_login", event)
}

// OnUserLogout should be called when a user logs out
func (ui *UserAuthIntegration) OnUserLogout(ctx context.Context, userID, sessionID uuid.UUID) error {
	event := &hooks.UserLogoutEvent{
		UserID:    userID,
		SessionID: sessionID,
		Timestamp: time.Now(),
	}

	return ui.hookManager.ProcessUserAuthEvent(ctx, "user_logout", event)
}

// OnPasswordChange should be called when a user changes their password
func (ui *UserAuthIntegration) OnPasswordChange(ctx context.Context, userID uuid.UUID, ipAddress string, success bool) error {
	event := &hooks.PasswordChangeEvent{
		UserID:    userID,
		IPAddress: ipAddress,
		Success:   success,
		Timestamp: time.Now(),
	}

	return ui.hookManager.ProcessUserAuthEvent(ctx, "password_change", event)
}

// OnEmailVerification should be called when a user verifies their email
func (ui *UserAuthIntegration) OnEmailVerification(ctx context.Context, userID uuid.UUID, email string, verified bool) error {
	event := &hooks.EmailVerificationEvent{
		UserID:    userID,
		Email:     email,
		Verified:  verified,
		Timestamp: time.Now(),
	}

	return ui.hookManager.ProcessUserAuthEvent(ctx, "email_verification", event)
}

// On2FAEnabled should be called when 2FA is enabled/disabled
func (ui *UserAuthIntegration) On2FAEnabled(ctx context.Context, userID uuid.UUID, method string, enabled bool) error {
	event := &hooks.TwoFAEvent{
		UserID:    userID,
		Method:    method,
		Enabled:   enabled,
		Timestamp: time.Now(),
	}

	return ui.hookManager.ProcessUserAuthEvent(ctx, "2fa_enabled", event)
}

// OnAccountLocked should be called when an account is locked
func (ui *UserAuthIntegration) OnAccountLocked(ctx context.Context, userID, lockedBy uuid.UUID, reason string, duration *time.Duration) error {
	event := &hooks.AccountLockEvent{
		UserID:    userID,
		Reason:    reason,
		LockedBy:  lockedBy,
		Duration:  duration,
		Timestamp: time.Now(),
	}

	return ui.hookManager.ProcessUserAuthEvent(ctx, "account_locked", event)
}
