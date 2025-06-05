// filepath: c:\Orbit CEX\Finalex\internal\userauth\notification\service.go
package notification

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"net/smtp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// NotificationType represents different types of notifications
type NotificationType string

const (
	NotificationEmailVerification NotificationType = "email_verification"
	NotificationPhoneVerification NotificationType = "phone_verification"
	NotificationPasswordReset     NotificationType = "password_reset"
	NotificationSecurityAlert     NotificationType = "security_alert"
	NotificationKYCStatus         NotificationType = "kyc_status"
	NotificationLogin             NotificationType = "login_notification"
	Notification2FABackup         NotificationType = "2fa_backup_codes"
)

// DeliveryMethod represents how notifications are delivered
type DeliveryMethod string

const (
	DeliveryEmail DeliveryMethod = "email"
	DeliverySMS   DeliveryMethod = "sms"
	DeliveryPush  DeliveryMethod = "push"
)

// NotificationTemplate represents a notification template
type NotificationTemplate struct {
	Type     NotificationType `json:"type"`
	Method   DeliveryMethod   `json:"method"`
	Subject  string           `json:"subject"`
	TextBody string           `json:"text_body"`
	HTMLBody string           `json:"html_body"`
	SMSBody  string           `json:"sms_body"`
	Language string           `json:"language"`
}

// VerificationCode represents a verification code
type VerificationCode struct {
	UserID    uuid.UUID        `json:"user_id"`
	Type      NotificationType `json:"type"`
	Code      string           `json:"code"`
	ExpiresAt time.Time        `json:"expires_at"`
	Attempts  int              `json:"attempts"`
	CreatedAt time.Time        `json:"created_at"`
}

// EmailConfig represents email server configuration
type EmailConfig struct {
	SMTPHost    string `json:"smtp_host"`
	SMTPPort    int    `json:"smtp_port"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	FromAddress string `json:"from_address"`
	FromName    string `json:"from_name"`
	TLSEnabled  bool   `json:"tls_enabled"`
}

// SMSConfig represents SMS provider configuration
type SMSConfig struct {
	Provider   string `json:"provider"` // "twilio", "aws_sns", etc.
	APIKey     string `json:"api_key"`
	APISecret  string `json:"api_secret"`
	FromNumber string `json:"from_number"`
	Region     string `json:"region"`
}

// Service provides notification services
type Service struct {
	db          *gorm.DB
	redis       *redis.Client
	logger      *zap.Logger
	emailConfig EmailConfig
	smsConfig   SMSConfig
	templates   map[string]NotificationTemplate
}

// NewService creates a new notification service
func NewService(logger *zap.Logger, db *gorm.DB, redisClient *redis.Client, emailConfig EmailConfig, smsConfig SMSConfig) *Service {
	service := &Service{
		db:          db,
		redis:       redisClient,
		logger:      logger,
		emailConfig: emailConfig,
		smsConfig:   smsConfig,
		templates:   make(map[string]NotificationTemplate),
	}

	// Initialize default templates
	service.initializeTemplates()

	return service
}

// SendEmailVerification sends an email verification code
func (s *Service) SendEmailVerification(ctx context.Context, userID uuid.UUID, email string) error {
	code, err := s.generateVerificationCode()
	if err != nil {
		return fmt.Errorf("failed to generate verification code: %w", err)
	}

	// Store verification code in Redis with 10 minute expiration
	if err := s.storeVerificationCode(ctx, userID, NotificationEmailVerification, code, time.Minute*10); err != nil {
		return fmt.Errorf("failed to store verification code: %w", err)
	}

	// Get template
	template, err := s.getTemplate(NotificationEmailVerification, DeliveryEmail, "en")
	if err != nil {
		return fmt.Errorf("failed to get email template: %w", err)
	}

	// Replace placeholders
	subject := strings.ReplaceAll(template.Subject, "{code}", code)
	htmlBody := strings.ReplaceAll(template.HTMLBody, "{code}", code)
	textBody := strings.ReplaceAll(template.TextBody, "{code}", code)

	// Send email
	if err := s.sendEmail(email, subject, textBody, htmlBody); err != nil {
		return fmt.Errorf("failed to send verification email: %w", err)
	}

	s.logger.Info("Email verification sent",
		zap.String("user_id", userID.String()),
		zap.String("email", email))

	return nil
}

// SendSMSVerification sends an SMS verification code
func (s *Service) SendSMSVerification(ctx context.Context, userID uuid.UUID, phoneNumber string) error {
	code, err := s.generateVerificationCode()
	if err != nil {
		return fmt.Errorf("failed to generate verification code: %w", err)
	}

	// Store verification code in Redis with 10 minute expiration
	if err := s.storeVerificationCode(ctx, userID, NotificationPhoneVerification, code, time.Minute*10); err != nil {
		return fmt.Errorf("failed to store verification code: %w", err)
	}

	// Get template
	template, err := s.getTemplate(NotificationPhoneVerification, DeliverySMS, "en")
	if err != nil {
		return fmt.Errorf("failed to get SMS template: %w", err)
	}

	// Replace placeholders
	message := strings.ReplaceAll(template.SMSBody, "{code}", code)

	// Send SMS
	if err := s.sendSMS(phoneNumber, message); err != nil {
		return fmt.Errorf("failed to send verification SMS: %w", err)
	}

	s.logger.Info("SMS verification sent",
		zap.String("user_id", userID.String()),
		zap.String("phone", phoneNumber))

	return nil
}

// VerifyCode verifies a verification code
func (s *Service) VerifyCode(ctx context.Context, userID uuid.UUID, notificationType NotificationType, code string) (bool, error) {
	// Get stored code from Redis
	key := fmt.Sprintf("verification:%s:%s:%s", userID.String(), string(notificationType), code)

	result, err := s.redis.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			// Check attempts
			attemptsKey := fmt.Sprintf("verification_attempts:%s:%s", userID.String(), string(notificationType))
			attempts, _ := s.redis.Incr(ctx, attemptsKey).Result()
			s.redis.Expire(ctx, attemptsKey, time.Hour) // Reset attempts after 1 hour

			if attempts > 5 {
				s.logger.Warn("Too many verification attempts",
					zap.String("user_id", userID.String()),
					zap.String("type", string(notificationType)))
				return false, fmt.Errorf("too many verification attempts")
			}

			return false, fmt.Errorf("invalid verification code")
		}
		return false, fmt.Errorf("failed to get verification code: %w", err)
	}

	// Code found and valid
	s.redis.Del(ctx, key) // Remove used code

	// Clear attempts counter
	attemptsKey := fmt.Sprintf("verification_attempts:%s:%s", userID.String(), string(notificationType))
	s.redis.Del(ctx, attemptsKey)

	s.logger.Info("Verification code verified",
		zap.String("user_id", userID.String()),
		zap.String("type", string(notificationType)))

	return true, nil
}

// SendPasswordResetEmail sends a password reset email
func (s *Service) SendPasswordResetEmail(ctx context.Context, userID uuid.UUID, email string) error {
	// Generate a secure reset token (longer than verification codes)
	token, err := s.generateResetToken()
	if err != nil {
		return fmt.Errorf("failed to generate reset token: %w", err)
	}

	// Store reset token in Redis with 30 minute expiration
	key := fmt.Sprintf("password_reset:%s", userID.String())
	if err := s.redis.Set(ctx, key, token, time.Minute*30).Err(); err != nil {
		return fmt.Errorf("failed to store reset token: %w", err)
	}

	// Get template
	template, err := s.getTemplate(NotificationPasswordReset, DeliveryEmail, "en")
	if err != nil {
		return fmt.Errorf("failed to get password reset template: %w", err)
	}

	// Create reset URL
	resetURL := fmt.Sprintf("https://your-domain.com/reset-password?token=%s", token)

	// Replace placeholders
	subject := template.Subject
	htmlBody := strings.ReplaceAll(template.HTMLBody, "{reset_url}", resetURL)
	textBody := strings.ReplaceAll(template.TextBody, "{reset_url}", resetURL)

	// Send email
	if err := s.sendEmail(email, subject, textBody, htmlBody); err != nil {
		return fmt.Errorf("failed to send password reset email: %w", err)
	}

	s.logger.Info("Password reset email sent",
		zap.String("user_id", userID.String()),
		zap.String("email", email))

	return nil
}

// SendSecurityAlert sends security alert notifications
func (s *Service) SendSecurityAlert(ctx context.Context, userID uuid.UUID, email, alertType, details string) error {
	// Get template
	template, err := s.getTemplate(NotificationSecurityAlert, DeliveryEmail, "en")
	if err != nil {
		return fmt.Errorf("failed to get security alert template: %w", err)
	}

	// Replace placeholders
	subject := strings.ReplaceAll(template.Subject, "{alert_type}", alertType)
	htmlBody := strings.ReplaceAll(template.HTMLBody, "{alert_type}", alertType)
	htmlBody = strings.ReplaceAll(htmlBody, "{details}", details)
	textBody := strings.ReplaceAll(template.TextBody, "{alert_type}", alertType)
	textBody = strings.ReplaceAll(textBody, "{details}", details)

	// Send email
	if err := s.sendEmail(email, subject, textBody, htmlBody); err != nil {
		return fmt.Errorf("failed to send security alert: %w", err)
	}

	s.logger.Info("Security alert sent",
		zap.String("user_id", userID.String()),
		zap.String("alert_type", alertType))

	return nil
}

// Send2FABackupCodes sends backup codes via email
func (s *Service) Send2FABackupCodes(ctx context.Context, userID uuid.UUID, email string, backupCodes []string) error {
	// Get template
	template, err := s.getTemplate(Notification2FABackup, DeliveryEmail, "en")
	if err != nil {
		return fmt.Errorf("failed to get 2FA backup template: %w", err)
	}

	// Format backup codes
	codesFormatted := strings.Join(backupCodes, "\n")

	// Replace placeholders
	subject := template.Subject
	htmlBody := strings.ReplaceAll(template.HTMLBody, "{backup_codes}", codesFormatted)
	textBody := strings.ReplaceAll(template.TextBody, "{backup_codes}", codesFormatted)

	// Send email
	if err := s.sendEmail(email, subject, textBody, htmlBody); err != nil {
		return fmt.Errorf("failed to send 2FA backup codes: %w", err)
	}

	s.logger.Info("2FA backup codes sent",
		zap.String("user_id", userID.String()))

	return nil
}

// ValidatePasswordResetToken validates a password reset token
func (s *Service) ValidatePasswordResetToken(ctx context.Context, userID uuid.UUID, token string) (bool, error) {
	key := fmt.Sprintf("password_reset:%s", userID.String())

	storedToken, err := s.redis.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return false, fmt.Errorf("invalid or expired reset token")
		}
		return false, fmt.Errorf("failed to validate reset token: %w", err)
	}

	return storedToken == token, nil
}

// ConsumePasswordResetToken consumes a password reset token
func (s *Service) ConsumePasswordResetToken(ctx context.Context, userID uuid.UUID, token string) error {
	valid, err := s.ValidatePasswordResetToken(ctx, userID, token)
	if err != nil {
		return err
	}

	if !valid {
		return fmt.Errorf("invalid reset token")
	}

	// Remove the token
	key := fmt.Sprintf("password_reset:%s", userID.String())
	s.redis.Del(ctx, key)

	s.logger.Info("Password reset token consumed",
		zap.String("user_id", userID.String()))

	return nil
}

// Helper methods

func (s *Service) generateVerificationCode() (string, error) {
	// Generate 6-digit numeric code
	max := big.NewInt(999999)
	min := big.NewInt(100000)

	n, err := rand.Int(rand.Reader, max.Sub(max, min).Add(max, big.NewInt(1)))
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%06d", n.Add(n, min).Int64()), nil
}

func (s *Service) generateResetToken() (string, error) {
	// Generate 32-byte random token
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", b), nil
}

func (s *Service) storeVerificationCode(ctx context.Context, userID uuid.UUID, notificationType NotificationType, code string, expiration time.Duration) error {
	key := fmt.Sprintf("verification:%s:%s:%s", userID.String(), string(notificationType), code)
	return s.redis.Set(ctx, key, "valid", expiration).Err()
}

func (s *Service) sendEmail(to, subject, textBody, htmlBody string) error {
	// Simple SMTP implementation
	auth := smtp.PlainAuth("", s.emailConfig.Username, s.emailConfig.Password, s.emailConfig.SMTPHost)

	msg := fmt.Sprintf("From: %s <%s>\r\n", s.emailConfig.FromName, s.emailConfig.FromAddress)
	msg += fmt.Sprintf("To: %s\r\n", to)
	msg += fmt.Sprintf("Subject: %s\r\n", subject)
	msg += "MIME-Version: 1.0\r\n"
	msg += "Content-Type: multipart/alternative; boundary=\"boundary\"\r\n\r\n"
	msg += "--boundary\r\n"
	msg += "Content-Type: text/plain; charset=UTF-8\r\n\r\n"
	msg += textBody + "\r\n"
	msg += "--boundary\r\n"
	msg += "Content-Type: text/html; charset=UTF-8\r\n\r\n"
	msg += htmlBody + "\r\n"
	msg += "--boundary--\r\n"

	addr := fmt.Sprintf("%s:%d", s.emailConfig.SMTPHost, s.emailConfig.SMTPPort)
	return smtp.SendMail(addr, auth, s.emailConfig.FromAddress, []string{to}, []byte(msg))
}

func (s *Service) sendSMS(to, message string) error {
	// Placeholder for SMS implementation
	// This would integrate with providers like Twilio, AWS SNS, etc.
	s.logger.Info("SMS would be sent",
		zap.String("to", to),
		zap.String("message", message))
	return nil
}

func (s *Service) getTemplate(notificationType NotificationType, method DeliveryMethod, language string) (NotificationTemplate, error) {
	key := fmt.Sprintf("%s:%s:%s", string(notificationType), string(method), language)
	template, exists := s.templates[key]
	if !exists {
		return NotificationTemplate{}, fmt.Errorf("template not found: %s", key)
	}
	return template, nil
}

func (s *Service) initializeTemplates() {
	// Email verification template
	s.templates["email_verification:email:en"] = NotificationTemplate{
		Type:     NotificationEmailVerification,
		Method:   DeliveryEmail,
		Subject:  "Verify Your Email Address",
		TextBody: "Your verification code is: {code}. This code will expire in 10 minutes.",
		HTMLBody: `<h2>Email Verification</h2><p>Your verification code is: <strong>{code}</strong></p><p>This code will expire in 10 minutes.</p>`,
		Language: "en",
	}

	// SMS verification template
	s.templates["phone_verification:sms:en"] = NotificationTemplate{
		Type:     NotificationPhoneVerification,
		Method:   DeliverySMS,
		SMSBody:  "Your verification code is: {code}. Valid for 10 minutes.",
		Language: "en",
	}

	// Password reset template
	s.templates["password_reset:email:en"] = NotificationTemplate{
		Type:     NotificationPasswordReset,
		Method:   DeliveryEmail,
		Subject:  "Password Reset Request",
		TextBody: "Click the following link to reset your password: {reset_url}. This link will expire in 30 minutes.",
		HTMLBody: `<h2>Password Reset</h2><p>Click the following link to reset your password:</p><p><a href="{reset_url}">Reset Password</a></p><p>This link will expire in 30 minutes.</p>`,
		Language: "en",
	}

	// Security alert template
	s.templates["security_alert:email:en"] = NotificationTemplate{
		Type:     NotificationSecurityAlert,
		Method:   DeliveryEmail,
		Subject:  "Security Alert: {alert_type}",
		TextBody: "Security Alert: {alert_type}. Details: {details}",
		HTMLBody: `<h2>Security Alert</h2><p><strong>Alert Type:</strong> {alert_type}</p><p><strong>Details:</strong> {details}</p>`,
		Language: "en",
	}

	// 2FA backup codes template
	s.templates["2fa_backup_codes:email:en"] = NotificationTemplate{
		Type:     Notification2FABackup,
		Method:   DeliveryEmail,
		Subject:  "Your 2FA Backup Codes",
		TextBody: "Your 2FA backup codes:\n\n{backup_codes}\n\nStore these codes safely. Each code can only be used once.",
		HTMLBody: `<h2>Your 2FA Backup Codes</h2><p>Store these codes safely. Each code can only be used once:</p><pre>{backup_codes}</pre>`,
		Language: "en",
	}
}
