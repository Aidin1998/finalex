package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// CreateSession creates a new user session
func (s *Service) CreateSession(ctx context.Context, userID uuid.UUID, deviceFingerprint string) (*Session, error) {
	session := &Session{
		ID:                uuid.New(),
		UserID:            userID,
		DeviceFingerprint: deviceFingerprint,
		IsActive:          true,
		LastActivityAt:    time.Now(),
		ExpiresAt:         time.Now().Add(24 * time.Hour), // Default 24-hour expiration
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	// Extract IP and User-Agent from context if available
	if ip, ok := ctx.Value("ip_address").(string); ok {
		session.IPAddress = ip
	}
	if ua, ok := ctx.Value("user_agent").(string); ok {
		session.UserAgent = ua
	}
	// Save session to database
	if err := s.db.Create(session).Error; err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	s.logger.Info("Session created",
		zap.String("user_id", userID.String()),
		zap.String("session_id", session.ID.String()),
	)

	return session, nil
}

// ValidateSession validates a session and updates last activity
func (s *Service) ValidateSession(ctx context.Context, sessionID uuid.UUID) (*Session, error) {
	var session Session
	err := s.db.Where("id = ? AND is_active = ? AND expires_at > ?",
		sessionID, true, time.Now()).First(&session).Error
	if err != nil {
		return nil, fmt.Errorf("session not found or expired: %w", err)
	}

	// Update last activity
	now := time.Now()
	s.db.Model(&session).Updates(map[string]interface{}{
		"last_activity_at": now,
		"updated_at":       now,
	})

	return &session, nil
}

// InvalidateSession invalidates a specific session
func (s *Service) InvalidateSession(ctx context.Context, sessionID uuid.UUID) error {
	result := s.db.Model(&Session{}).Where("id = ?", sessionID).Updates(map[string]interface{}{
		"is_active":  false,
		"updated_at": time.Now(),
	})

	if result.Error != nil {
		return fmt.Errorf("failed to invalidate session: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("session not found")
	}

	s.logger.Info("Session invalidated",
		zap.String("session_id", sessionID.String()),
	)

	return nil
}

// InvalidateAllSessions invalidates all sessions for a user
func (s *Service) InvalidateAllSessions(ctx context.Context, userID uuid.UUID) error {
	result := s.db.Model(&Session{}).Where("user_id = ?", userID).Updates(map[string]interface{}{
		"is_active":  false,
		"updated_at": time.Now(),
	})
	if result.Error != nil {
		return fmt.Errorf("failed to invalidate sessions: %w", result.Error)
	}
	s.logger.Info("All sessions invalidated",
		zap.String("user_id", userID.String()),
		zap.Int64("sessions_count", result.RowsAffected),
	)

	return nil
}

// GetActiveSessions returns all active sessions for a user
func (s *Service) GetActiveSessions(ctx context.Context, userID uuid.UUID) ([]*Session, error) {
	var sessions []*Session
	err := s.db.Where("user_id = ? AND is_active = ? AND expires_at > ?",
		userID, true, time.Now()).Find(&sessions).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get sessions: %w", err)
	}

	return sessions, nil
}

// CleanupExpiredSessions removes expired sessions from the database
func (s *Service) CleanupExpiredSessions(ctx context.Context) error {
	result := s.db.Where("expires_at < ? OR (is_active = false AND updated_at < ?)",
		time.Now(), time.Now().Add(-7*24*time.Hour)).Delete(&Session{})

	if result.Error != nil {
		return fmt.Errorf("failed to cleanup expired sessions: %w", result.Error)
	}

	if result.RowsAffected > 0 {
		s.logger.Info("Cleaned up expired sessions",
			zap.Int64("count", result.RowsAffected),
		)
	}

	return nil
}

// ExtendSession extends the expiration time of a session
func (s *Service) ExtendSession(ctx context.Context, sessionID uuid.UUID, duration time.Duration) error {
	newExpiresAt := time.Now().Add(duration)

	result := s.db.Model(&Session{}).Where("id = ? AND is_active = ?", sessionID, true).Updates(map[string]interface{}{
		"expires_at":       newExpiresAt,
		"last_activity_at": time.Now(),
		"updated_at":       time.Now(),
	})

	if result.Error != nil {
		return fmt.Errorf("failed to extend session: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("session not found or inactive")
	}

	return nil
}

// GetSessionInfo returns detailed session information
func (s *Service) GetSessionInfo(ctx context.Context, sessionID uuid.UUID) (*SessionInfo, error) {
	var session Session
	err := s.db.Where("id = ?", sessionID).First(&session).Error
	if err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}

	return &SessionInfo{
		ID:                session.ID,
		UserID:            session.UserID,
		DeviceFingerprint: session.DeviceFingerprint,
		IPAddress:         session.IPAddress,
		UserAgent:         session.UserAgent,
		IsActive:          session.IsActive,
		LastActivityAt:    session.LastActivityAt,
		ExpiresAt:         session.ExpiresAt,
		CreatedAt:         session.CreatedAt,
		IsCurrent:         false, // Will be set by caller if this is the current session
	}, nil
}

// SessionInfo represents detailed session information
type SessionInfo struct {
	ID                uuid.UUID `json:"id"`
	UserID            uuid.UUID `json:"user_id"`
	DeviceFingerprint string    `json:"device_fingerprint,omitempty"`
	IPAddress         string    `json:"ip_address,omitempty"`
	UserAgent         string    `json:"user_agent,omitempty"`
	IsActive          bool      `json:"is_active"`
	LastActivityAt    time.Time `json:"last_activity_at"`
	ExpiresAt         time.Time `json:"expires_at"`
	CreatedAt         time.Time `json:"created_at"`
	IsCurrent         bool      `json:"is_current"`
}

// IsSessionActive checks if a session is currently active
func (s *Service) IsSessionActive(ctx context.Context, sessionID uuid.UUID) (bool, error) {
	var count int64
	err := s.db.Model(&Session{}).Where("id = ? AND is_active = ? AND expires_at > ?",
		sessionID, true, time.Now()).Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("failed to check session status: %w", err)
	}

	return count > 0, nil
}

// CountActiveSessions returns the number of active sessions for a user
func (s *Service) CountActiveSessions(ctx context.Context, userID uuid.UUID) (int64, error) {
	var count int64
	err := s.db.Model(&Session{}).Where("user_id = ? AND is_active = ? AND expires_at > ?",
		userID, true, time.Now()).Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("failed to count active sessions: %w", err)
	}

	return count, nil
}

// InvalidateOtherSessions invalidates all sessions except the current one
func (s *Service) InvalidateOtherSessions(ctx context.Context, userID uuid.UUID, currentSessionID uuid.UUID) error {
	result := s.db.Model(&Session{}).Where("user_id = ? AND id != ?", userID, currentSessionID).Updates(map[string]interface{}{
		"is_active":  false,
		"updated_at": time.Now(),
	})
	if result.Error != nil {
		return fmt.Errorf("failed to invalidate other sessions: %w", result.Error)
	}

	s.logger.Info("Other sessions invalidated",
		zap.String("user_id", userID.String()),
		zap.String("current_session", currentSessionID.String()),
		zap.Int64("sessions_count", result.RowsAffected),
	)

	return nil
}
