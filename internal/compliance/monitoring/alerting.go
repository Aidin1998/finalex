package monitoring

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/pkg/errors"
	"go.uber.org/zap"

	"github.com/Aidin1998/finalex/internal/compliance/interfaces"
)

// AlertChannel defines the interface for alert notification channels
type AlertChannel interface {
	SendAlert(ctx context.Context, alert interfaces.MonitoringAlert) error
	GetChannelType() string
	IsEnabled() bool
}

// WebhookChannel implements webhook-based alert notifications
type WebhookChannel struct {
	URL        string
	Method     string
	Headers    map[string]string
	Timeout    time.Duration
	RetryCount int
	Enabled    bool
	logger     *zap.Logger
}

// NewWebhookChannel creates a new webhook alert channel
func NewWebhookChannel(url, method string, headers map[string]string, timeout time.Duration, logger *zap.Logger) *WebhookChannel {
	if method == "" {
		method = "POST"
	}
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &WebhookChannel{
		URL:        url,
		Method:     method,
		Headers:    headers,
		Timeout:    timeout,
		RetryCount: 3,
		Enabled:    true,
		logger:     logger,
	}
}

// SendAlert sends alert via webhook
func (wc *WebhookChannel) SendAlert(ctx context.Context, alert interfaces.MonitoringAlert) error {
	if !wc.Enabled {
		return nil
	}

	payload := map[string]interface{}{
		"id":         alert.ID,
		"user_id":    alert.UserID,
		"alert_type": alert.AlertType,
		"severity":   alert.Severity.String(),
		"status":     alert.Status.String(),
		"message":    alert.Message,
		"details":    alert.Details,
		"timestamp":  alert.Timestamp,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return errors.Wrap(err, "failed to marshal alert payload")
	}

	return wc.sendWithRetry(ctx, jsonData)
}

// sendWithRetry sends webhook with retry logic
func (wc *WebhookChannel) sendWithRetry(ctx context.Context, data []byte) error {
	var lastErr error

	for i := 0; i <= wc.RetryCount; i++ {
		if err := wc.sendWebhook(ctx, data); err != nil {
			lastErr = err
			wc.logger.Warn("Webhook send failed, retrying",
				zap.Int("attempt", i+1),
				zap.Error(err))

			if i < wc.RetryCount {
				select {
				case <-time.After(time.Duration(i+1) * time.Second):
					continue
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		} else {
			return nil
		}
	}

	return errors.Wrap(lastErr, "webhook send failed after retries")
}

// sendWebhook sends the actual webhook request
func (wc *WebhookChannel) sendWebhook(ctx context.Context, data []byte) error {
	client := &http.Client{Timeout: wc.Timeout}

	req, err := http.NewRequestWithContext(ctx, wc.Method, wc.URL, bytes.NewBuffer(data))
	if err != nil {
		return errors.Wrap(err, "failed to create webhook request")
	}

	req.Header.Set("Content-Type", "application/json")
	for key, value := range wc.Headers {
		req.Header.Set(key, value)
	}

	resp, err := client.Do(req)
	if err != nil {
		return errors.Wrap(err, "failed to send webhook request")
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	return nil
}

// GetChannelType returns the channel type
func (wc *WebhookChannel) GetChannelType() string {
	return "webhook"
}

// IsEnabled returns whether the channel is enabled
func (wc *WebhookChannel) IsEnabled() bool {
	return wc.Enabled
}

// EmailChannel implements email-based alert notifications
type EmailChannel struct {
	SMTPHost  string
	SMTPPort  int
	Username  string
	Password  string
	FromEmail string
	ToEmails  []string
	Subject   string
	Enabled   bool
	logger    *zap.Logger
}

// NewEmailChannel creates a new email alert channel
func NewEmailChannel(smtpHost string, smtpPort int, username, password, fromEmail string, toEmails []string, logger *zap.Logger) *EmailChannel {
	return &EmailChannel{
		SMTPHost:  smtpHost,
		SMTPPort:  smtpPort,
		Username:  username,
		Password:  password,
		FromEmail: fromEmail,
		ToEmails:  toEmails,
		Subject:   "Compliance Alert",
		Enabled:   true,
		logger:    logger,
	}
}

// SendAlert sends alert via email
func (ec *EmailChannel) SendAlert(ctx context.Context, alert interfaces.MonitoringAlert) error {
	if !ec.Enabled {
		return nil
	}

	body := fmt.Sprintf(`
Compliance Alert

Alert ID: %s
User ID: %s
Type: %s
Severity: %s
Status: %s
Message: %s
Timestamp: %s

Details: %+v
`, alert.ID, alert.UserID, alert.AlertType, alert.Severity.String(),
		alert.Status.String(), alert.Message, alert.Timestamp.Format(time.RFC3339), alert.Details)

	// For production, implement actual SMTP sending
	// This is a placeholder implementation
	ec.logger.Info("Email alert would be sent",
		zap.String("to", fmt.Sprintf("%v", ec.ToEmails)),
		zap.String("subject", ec.Subject),
		zap.String("alert_id", alert.ID.String()))

	return nil
}

// GetChannelType returns the channel type
func (ec *EmailChannel) GetChannelType() string {
	return "email"
}

// IsEnabled returns whether the channel is enabled
func (ec *EmailChannel) IsEnabled() bool {
	return ec.Enabled
}

// SlackChannel implements Slack-based alert notifications
type SlackChannel struct {
	WebhookURL string
	Channel    string
	Username   string
	IconEmoji  string
	Enabled    bool
	logger     *zap.Logger
}

// NewSlackChannel creates a new Slack alert channel
func NewSlackChannel(webhookURL, channel, username, iconEmoji string, logger *zap.Logger) *SlackChannel {
	if username == "" {
		username = "Compliance Bot"
	}
	if iconEmoji == "" {
		iconEmoji = ":warning:"
	}

	return &SlackChannel{
		WebhookURL: webhookURL,
		Channel:    channel,
		Username:   username,
		IconEmoji:  iconEmoji,
		Enabled:    true,
		logger:     logger,
	}
}

// SendAlert sends alert via Slack
func (sc *SlackChannel) SendAlert(ctx context.Context, alert interfaces.MonitoringAlert) error {
	if !sc.Enabled {
		return nil
	}

	color := sc.getSeverityColor(alert.Severity)

	payload := map[string]interface{}{
		"channel":    sc.Channel,
		"username":   sc.Username,
		"icon_emoji": sc.IconEmoji,
		"attachments": []map[string]interface{}{
			{
				"color": color,
				"title": fmt.Sprintf("Compliance Alert: %s", alert.AlertType),
				"text":  alert.Message,
				"fields": []map[string]interface{}{
					{
						"title": "Alert ID",
						"value": alert.ID.String(),
						"short": true,
					},
					{
						"title": "User ID",
						"value": alert.UserID,
						"short": true,
					},
					{
						"title": "Severity",
						"value": alert.Severity.String(),
						"short": true,
					},
					{
						"title": "Status",
						"value": alert.Status.String(),
						"short": true,
					},
					{
						"title": "Timestamp",
						"value": alert.Timestamp.Format(time.RFC3339),
						"short": false,
					},
				},
				"footer": "Compliance Monitoring System",
				"ts":     alert.Timestamp.Unix(),
			},
		},
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return errors.Wrap(err, "failed to marshal Slack payload")
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Post(sc.WebhookURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return errors.Wrap(err, "failed to send Slack webhook")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Slack webhook returned status %d", resp.StatusCode)
	}

	return nil
}

// getSeverityColor returns color based on alert severity
func (sc *SlackChannel) getSeverityColor(severity interfaces.AlertSeverity) string {
	switch severity {
	case interfaces.AlertSeverityLow:
		return "good"
	case interfaces.AlertSeverityMedium:
		return "warning"
	case interfaces.AlertSeverityHigh:
		return "danger"
	case interfaces.AlertSeverityCritical:
		return "#FF0000"
	default:
		return "#CCCCCC"
	}
}

// GetChannelType returns the channel type
func (sc *SlackChannel) GetChannelType() string {
	return "slack"
}

// IsEnabled returns whether the channel is enabled
func (sc *SlackChannel) IsEnabled() bool {
	return sc.Enabled
}

// AlertingManager manages multiple alert channels
type AlertingManager struct {
	channels []AlertChannel
	logger   *zap.Logger
}

// NewAlertingManager creates a new alerting manager
func NewAlertingManager(logger *zap.Logger) *AlertingManager {
	return &AlertingManager{
		channels: make([]AlertChannel, 0),
		logger:   logger,
	}
}

// AddChannel adds an alert channel
func (am *AlertingManager) AddChannel(channel AlertChannel) {
	am.channels = append(am.channels, channel)
}

// SendAlert sends alert to all enabled channels
func (am *AlertingManager) SendAlert(ctx context.Context, alert interfaces.MonitoringAlert) error {
	var errors []error

	for _, channel := range am.channels {
		if !channel.IsEnabled() {
			continue
		}

		if err := channel.SendAlert(ctx, alert); err != nil {
			am.logger.Error("Failed to send alert via channel",
				zap.String("channel_type", channel.GetChannelType()),
				zap.String("alert_id", alert.ID.String()),
				zap.Error(err))
			errors = append(errors, err)
		} else {
			am.logger.Info("Alert sent successfully",
				zap.String("channel_type", channel.GetChannelType()),
				zap.String("alert_id", alert.ID.String()))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to send alerts via %d channels", len(errors))
	}

	return nil
}

// GetEnabledChannels returns list of enabled channels
func (am *AlertingManager) GetEnabledChannels() []string {
	var enabled []string
	for _, channel := range am.channels {
		if channel.IsEnabled() {
			enabled = append(enabled, channel.GetChannelType())
		}
	}
	return enabled
}
