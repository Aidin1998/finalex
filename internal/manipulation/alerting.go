package manipulation

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// =======================
// REAL-TIME ALERT SYSTEM
// =======================

// AlertSeverity represents alert severity levels
type AlertSeverity string

const (
	AlertSeverityLow      AlertSeverity = "low"
	AlertSeverityMedium   AlertSeverity = "medium"
	AlertSeverityHigh     AlertSeverity = "high"
	AlertSeverityCritical AlertSeverity = "critical"
)

// AlertChannel represents different alert delivery channels
type AlertChannel string

const (
	AlertChannelEmail     AlertChannel = "email"
	AlertChannelWebSocket AlertChannel = "websocket"
	AlertChannelSlack     AlertChannel = "slack"
	AlertChannelDatabase  AlertChannel = "database"
	AlertChannelSMS       AlertChannel = "sms"
)

// AlertNotification represents a real-time alert notification
type AlertNotification struct {
	ID             string                 `json:"id"`
	AlertID        string                 `json:"alert_id"`
	Type           string                 `json:"type"`
	Severity       AlertSeverity          `json:"severity"`
	Title          string                 `json:"title"`
	Message        string                 `json:"message"`
	UserID         string                 `json:"user_id"`
	Market         string                 `json:"market"`
	Timestamp      time.Time              `json:"timestamp"`
	Channels       []AlertChannel         `json:"channels"`
	Metadata       map[string]interface{} `json:"metadata"`
	Acknowledged   bool                   `json:"acknowledged"`
	AcknowledgedBy string                 `json:"acknowledged_by"`
	AcknowledgedAt *time.Time             `json:"acknowledged_at"`
}

// AlertSubscription represents a subscription to specific alert types
type AlertSubscription struct {
	UserID     string         `json:"user_id"`
	AlertTypes []string       `json:"alert_types"`
	Channels   []AlertChannel `json:"channels"`
	Filters    AlertFilters   `json:"filters"`
	IsActive   bool           `json:"is_active"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

// AlertFilters represents filters for alert subscriptions
type AlertFilters struct {
	MinSeverity   AlertSeverity          `json:"min_severity"`
	Markets       []string               `json:"markets"`
	MinConfidence decimal.Decimal        `json:"min_confidence"`
	PatternTypes  []string               `json:"pattern_types"`
	BusinessHours bool                   `json:"business_hours_only"`
	CustomFilters map[string]interface{} `json:"custom_filters"`
}

// AlertingService manages real-time alerts
type AlertingService struct {
	mu            sync.RWMutex
	logger        *zap.SugaredLogger
	subscriptions map[string][]AlertSubscription // userID -> subscriptions
	notifications []AlertNotification
	channels      map[AlertChannel]AlertChannel
	rateLimiter   *AlertRateLimiter
	config        AlertingConfig

	// Delivery channels
	emailSender     EmailSender
	websocketSender WebSocketSender
	slackSender     SlackSender
	smsSender       SMSSender

	// Background processing
	alertQueue   chan AlertNotification
	processingWG sync.WaitGroup
	stopChan     chan struct{}
}

// AlertingConfig contains alerting service configuration
type AlertingConfig struct {
	Enabled               bool           `json:"enabled"`
	QueueSize             int            `json:"queue_size"`
	MaxRetries            int            `json:"max_retries"`
	RetryDelay            time.Duration  `json:"retry_delay"`
	RateLimitPerUser      int            `json:"rate_limit_per_user"`
	RateLimitWindow       time.Duration  `json:"rate_limit_window"`
	DefaultChannels       []AlertChannel `json:"default_channels"`
	RequireAcknowledgment bool           `json:"require_acknowledgment"`
	AcknowledgmentTimeout time.Duration  `json:"acknowledgment_timeout"`
}

// AlertRateLimiter prevents alert spam
type AlertRateLimiter struct {
	mu         sync.RWMutex
	userCounts map[string]int
	lastReset  time.Time
	maxPerUser int
	window     time.Duration
}

// Interface definitions for alert delivery channels
type EmailSender interface {
	SendEmail(to, subject, body string) error
}

type WebSocketSender interface {
	SendToUser(userID string, message interface{}) error
	Broadcast(message interface{}) error
}

type SlackSender interface {
	SendToChannel(channel, message string) error
	SendDirectMessage(userID, message string) error
}

type SMSSender interface {
	SendSMS(phoneNumber, message string) error
}

// NewAlertingService creates a new alerting service
func NewAlertingService(logger *zap.SugaredLogger, config AlertingConfig) *AlertingService {
	return &AlertingService{
		logger:        logger,
		subscriptions: make(map[string][]AlertSubscription),
		notifications: make([]AlertNotification, 0),
		channels:      make(map[AlertChannel]AlertChannel),
		config:        config,
		rateLimiter: &AlertRateLimiter{
			userCounts: make(map[string]int),
			lastReset:  time.Now(),
			maxPerUser: config.RateLimitPerUser,
			window:     config.RateLimitWindow,
		},
		alertQueue: make(chan AlertNotification, config.QueueSize),
		stopChan:   make(chan struct{}),
	}
}

// Start starts the alerting service
func (as *AlertingService) Start(ctx context.Context) error {
	if !as.config.Enabled {
		as.logger.Info("Alerting service is disabled")
		return nil
	}

	as.logger.Info("Starting alerting service")

	// Start alert processing workers
	for i := 0; i < 3; i++ { // 3 worker goroutines
		as.processingWG.Add(1)
		go as.alertProcessor(ctx, i)
	}

	// Start rate limiter reset routine
	as.processingWG.Add(1)
	go as.rateLimiterReset(ctx)

	return nil
}

// Stop stops the alerting service
func (as *AlertingService) Stop() {
	as.logger.Info("Stopping alerting service")
	close(as.stopChan)
	as.processingWG.Wait()
	close(as.alertQueue)
}

// SendAlert sends a manipulation alert notification
func (as *AlertingService) SendAlert(alert ManipulationAlert) error {
	if !as.config.Enabled {
		return nil
	}

	// Create notification
	notification := AlertNotification{
		ID:        uuid.New().String(),
		AlertID:   alert.ID,
		Type:      alert.Pattern.Type,
		Severity:  AlertSeverity(alert.Pattern.Severity),
		Title:     fmt.Sprintf("Manipulation Alert: %s", alert.Pattern.Type),
		Message:   as.formatAlertMessage(alert),
		UserID:    alert.UserID,
		Market:    alert.Market,
		Timestamp: time.Now(),
		Channels:  as.config.DefaultChannels,
		Metadata: map[string]interface{}{
			"confidence":       alert.Pattern.Confidence.String(),
			"risk_score":       alert.RiskScore.String(),
			"auto_suspended":   alert.AutoSuspended,
			"pattern_metadata": alert.Pattern.Metadata,
		},
	}

	// Queue for processing
	select {
	case as.alertQueue <- notification:
		return nil
	default:
		return fmt.Errorf("alert queue is full")
	}
}

// formatAlertMessage formats an alert message for notifications
func (as *AlertingService) formatAlertMessage(alert ManipulationAlert) string {
	return fmt.Sprintf(
		"Manipulation detected for user %s in market %s.\n"+
			"Pattern: %s\n"+
			"Confidence: %s%%\n"+
			"Risk Score: %s\n"+
			"Description: %s\n"+
			"Action Required: %s\n"+
			"Auto-suspended: %t",
		alert.UserID,
		alert.Market,
		alert.Pattern.Type,
		alert.Pattern.Confidence.String(),
		alert.RiskScore.String(),
		alert.Pattern.Description,
		alert.ActionRequired,
		alert.AutoSuspended,
	)
}

// alertProcessor processes alerts from the queue
func (as *AlertingService) alertProcessor(ctx context.Context, workerID int) {
	defer as.processingWG.Done()

	as.logger.Infow("Started alert processor", "worker_id", workerID)

	for {
		select {
		case <-ctx.Done():
			return
		case <-as.stopChan:
			return
		case notification := <-as.alertQueue:
			as.processAlert(notification)
		}
	}
}

// processAlert processes a single alert notification
func (as *AlertingService) processAlert(notification AlertNotification) {
	// Check rate limiting
	if !as.rateLimiter.Allow(notification.UserID) {
		as.logger.Warnw("Alert rate limited",
			"user_id", notification.UserID,
			"alert_type", notification.Type)
		return
	}

	// Find subscriptions for this alert
	subscriptions := as.getMatchingSubscriptions(notification)

	// Send to each matching subscription
	for _, subscription := range subscriptions {
		for _, channel := range subscription.Channels {
			err := as.sendToChannel(channel, notification)
			if err != nil {
				as.logger.Errorw("Failed to send alert",
					"channel", channel,
					"notification_id", notification.ID,
					"error", err)
			}
		}
	}

	// Store notification
	as.mu.Lock()
	as.notifications = append(as.notifications, notification)
	// Keep only last 10000 notifications
	if len(as.notifications) > 10000 {
		as.notifications = as.notifications[len(as.notifications)-10000:]
	}
	as.mu.Unlock()

	as.logger.Infow("Alert processed",
		"notification_id", notification.ID,
		"alert_type", notification.Type,
		"user_id", notification.UserID,
		"subscriptions_matched", len(subscriptions))
}

// getMatchingSubscriptions finds subscriptions that match an alert
func (as *AlertingService) getMatchingSubscriptions(notification AlertNotification) []AlertSubscription {
	as.mu.RLock()
	defer as.mu.RUnlock()

	var matching []AlertSubscription

	// Get subscriptions for the affected user
	if userSubs, exists := as.subscriptions[notification.UserID]; exists {
		for _, sub := range userSubs {
			if as.subscriptionMatches(sub, notification) {
				matching = append(matching, sub)
			}
		}
	}

	// Get global subscriptions (e.g., compliance team)
	if globalSubs, exists := as.subscriptions["*"]; exists {
		for _, sub := range globalSubs {
			if as.subscriptionMatches(sub, notification) {
				matching = append(matching, sub)
			}
		}
	}

	return matching
}

// subscriptionMatches checks if a subscription matches an alert
func (as *AlertingService) subscriptionMatches(subscription AlertSubscription, notification AlertNotification) bool {
	if !subscription.IsActive {
		return false
	}

	// Check alert type filter
	if len(subscription.AlertTypes) > 0 {
		matched := false
		for _, alertType := range subscription.AlertTypes {
			if alertType == notification.Type || alertType == "*" {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Check severity filter
	if subscription.Filters.MinSeverity != "" {
		if !as.severityMeetsMinimum(notification.Severity, subscription.Filters.MinSeverity) {
			return false
		}
	}

	// Check market filter
	if len(subscription.Filters.Markets) > 0 {
		matched := false
		for _, market := range subscription.Filters.Markets {
			if market == notification.Market || market == "*" {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Check confidence filter
	if !subscription.Filters.MinConfidence.IsZero() {
		if confidence, exists := notification.Metadata["confidence"]; exists {
			if confStr, ok := confidence.(string); ok {
				if confDec, err := decimal.NewFromString(confStr); err == nil {
					if confDec.LessThan(subscription.Filters.MinConfidence) {
						return false
					}
				}
			}
		}
	}

	// Check business hours filter
	if subscription.Filters.BusinessHours {
		now := time.Now()
		if now.Weekday() == time.Saturday || now.Weekday() == time.Sunday {
			return false
		}
		hour := now.Hour()
		if hour < 9 || hour > 17 { // Outside 9 AM - 5 PM
			return false
		}
	}

	return true
}

// severityMeetsMinimum checks if alert severity meets minimum requirement
func (as *AlertingService) severityMeetsMinimum(alertSeverity, minSeverity AlertSeverity) bool {
	severityLevels := map[AlertSeverity]int{
		AlertSeverityLow:      1,
		AlertSeverityMedium:   2,
		AlertSeverityHigh:     3,
		AlertSeverityCritical: 4,
	}

	alertLevel := severityLevels[alertSeverity]
	minLevel := severityLevels[minSeverity]

	return alertLevel >= minLevel
}

// sendToChannel sends a notification to a specific channel
func (as *AlertingService) sendToChannel(channel AlertChannel, notification AlertNotification) error {
	switch channel {
	case AlertChannelEmail:
		if as.emailSender != nil {
			return as.sendEmailAlert(notification)
		}
	case AlertChannelWebSocket:
		if as.websocketSender != nil {
			return as.sendWebSocketAlert(notification)
		}
	case AlertChannelSlack:
		if as.slackSender != nil {
			return as.sendSlackAlert(notification)
		}
	case AlertChannelSMS:
		if as.smsSender != nil {
			return as.sendSMSAlert(notification)
		}
	case AlertChannelDatabase:
		return as.sendDatabaseAlert(notification)
	}

	return fmt.Errorf("unsupported channel: %s", channel)
}

// sendEmailAlert sends an email alert
func (as *AlertingService) sendEmailAlert(notification AlertNotification) error {
	subject := fmt.Sprintf("[%s] %s", notification.Severity, notification.Title)
	body := notification.Message

	// In a real implementation, you would get the user's email address
	email := fmt.Sprintf("user-%s@example.com", notification.UserID)

	return as.emailSender.SendEmail(email, subject, body)
}

// sendWebSocketAlert sends a WebSocket alert
func (as *AlertingService) sendWebSocketAlert(notification AlertNotification) error {
	return as.websocketSender.SendToUser(notification.UserID, notification)
}

// sendSlackAlert sends a Slack alert
func (as *AlertingService) sendSlackAlert(notification AlertNotification) error {
	message := fmt.Sprintf("🚨 *%s*\n%s", notification.Title, notification.Message)
	return as.slackSender.SendToChannel("compliance-alerts", message)
}

// sendSMSAlert sends an SMS alert
func (as *AlertingService) sendSMSAlert(notification AlertNotification) error {
	message := fmt.Sprintf("ALERT: %s - %s", notification.Type, notification.Market)

	// In a real implementation, you would get the user's phone number
	phoneNumber := fmt.Sprintf("+1555%s", notification.UserID[len(notification.UserID)-7:])

	return as.smsSender.SendSMS(phoneNumber, message)
}

// sendDatabaseAlert stores alert in database
func (as *AlertingService) sendDatabaseAlert(notification AlertNotification) error {
	// In a real implementation, this would insert into a database
	as.logger.Infow("Database alert stored",
		"notification_id", notification.ID,
		"alert_type", notification.Type,
		"user_id", notification.UserID)
	return nil
}

// Allow checks if an alert is allowed for a user (rate limiting)
func (rl *AlertRateLimiter) Allow(userID string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Reset counts if window has passed
	if time.Since(rl.lastReset) > rl.window {
		rl.userCounts = make(map[string]int)
		rl.lastReset = time.Now()
	}

	// Check current count
	count := rl.userCounts[userID]
	if count >= rl.maxPerUser {
		return false
	}

	// Increment count
	rl.userCounts[userID] = count + 1
	return true
}

// rateLimiterReset resets rate limiter periodically
func (as *AlertingService) rateLimiterReset(ctx context.Context) {
	defer as.processingWG.Done()

	ticker := time.NewTicker(as.config.RateLimitWindow)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-as.stopChan:
			return
		case <-ticker.C:
			as.rateLimiter.mu.Lock()
			as.rateLimiter.userCounts = make(map[string]int)
			as.rateLimiter.lastReset = time.Now()
			as.rateLimiter.mu.Unlock()
		}
	}
}

// Subscription management methods

// Subscribe adds a new alert subscription
func (as *AlertingService) Subscribe(subscription AlertSubscription) error {
	as.mu.Lock()
	defer as.mu.Unlock()

	subscription.CreatedAt = time.Now()
	subscription.UpdatedAt = time.Now()
	subscription.IsActive = true

	as.subscriptions[subscription.UserID] = append(as.subscriptions[subscription.UserID], subscription)

	as.logger.Infow("Alert subscription added",
		"user_id", subscription.UserID,
		"alert_types", subscription.AlertTypes,
		"channels", subscription.Channels)

	return nil
}

// Unsubscribe removes an alert subscription
func (as *AlertingService) Unsubscribe(userID string, alertTypes []string) error {
	as.mu.Lock()
	defer as.mu.Unlock()

	if subscriptions, exists := as.subscriptions[userID]; exists {
		var remaining []AlertSubscription

		for _, sub := range subscriptions {
			shouldRemove := false
			for _, alertType := range alertTypes {
				for _, subType := range sub.AlertTypes {
					if subType == alertType {
						shouldRemove = true
						break
					}
				}
				if shouldRemove {
					break
				}
			}

			if !shouldRemove {
				remaining = append(remaining, sub)
			}
		}

		as.subscriptions[userID] = remaining
	}

	return nil
}

// AcknowledgeAlert acknowledges an alert notification
func (as *AlertingService) AcknowledgeAlert(notificationID, acknowledgedBy string) error {
	as.mu.Lock()
	defer as.mu.Unlock()

	for i, notification := range as.notifications {
		if notification.ID == notificationID {
			now := time.Now()
			as.notifications[i].Acknowledged = true
			as.notifications[i].AcknowledgedBy = acknowledgedBy
			as.notifications[i].AcknowledgedAt = &now

			as.logger.Infow("Alert acknowledged",
				"notification_id", notificationID,
				"acknowledged_by", acknowledgedBy)

			return nil
		}
	}

	return fmt.Errorf("notification not found: %s", notificationID)
}

// GetNotifications retrieves alert notifications with filtering
func (as *AlertingService) GetNotifications(userID string, limit int, unacknowledgedOnly bool) []AlertNotification {
	as.mu.RLock()
	defer as.mu.RUnlock()

	var filtered []AlertNotification

	for _, notification := range as.notifications {
		// Filter by user ID
		if userID != "" && notification.UserID != userID {
			continue
		}

		// Filter by acknowledgment status
		if unacknowledgedOnly && notification.Acknowledged {
			continue
		}

		filtered = append(filtered, notification)
	}

	// Sort by timestamp (newest first)
	for i := 0; i < len(filtered)-1; i++ {
		for j := i + 1; j < len(filtered); j++ {
			if filtered[i].Timestamp.Before(filtered[j].Timestamp) {
				filtered[i], filtered[j] = filtered[j], filtered[i]
			}
		}
	}

	// Apply limit
	if limit > 0 && len(filtered) > limit {
		filtered = filtered[:limit]
	}

	return filtered
}

// GetAlertingMetrics returns alerting service metrics
func (as *AlertingService) GetAlertingMetrics() map[string]interface{} {
	as.mu.RLock()
	defer as.mu.RUnlock()

	// Count notifications by type and severity
	typeCounts := make(map[string]int)
	severityCounts := make(map[AlertSeverity]int)
	acknowledgedCount := 0

	for _, notification := range as.notifications {
		typeCounts[notification.Type]++
		severityCounts[notification.Severity]++
		if notification.Acknowledged {
			acknowledgedCount++
		}
	}

	return map[string]interface{}{
		"total_notifications":  len(as.notifications),
		"acknowledged_count":   acknowledgedCount,
		"unacknowledged_count": len(as.notifications) - acknowledgedCount,
		"type_counts":          typeCounts,
		"severity_counts":      severityCounts,
		"active_subscriptions": len(as.subscriptions),
		"queue_size":           len(as.alertQueue),
		"rate_limiter_counts":  as.rateLimiter.userCounts,
	}
}
