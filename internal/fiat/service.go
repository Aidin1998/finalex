package fiat

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Aidin1998/pincex_unified/internal/bookkeeper"
	"github.com/Aidin1998/pincex_unified/internal/kyc"
	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/Aidin1998/pincex_unified/pkg/validation"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// FiatService defines fiat operations.
type FiatService interface {
	Start() error
	Stop() error
	InitiateDeposit(ctx context.Context, userID, currency string, amount float64, provider string) (*models.Transaction, error)
	InitiateWithdrawal(ctx context.Context, userID, currency string, amount float64, bankDetails map[string]interface{}) (*models.Transaction, error)
	GetDeposits(ctx context.Context, userID, currency string, limit, offset int) ([]*models.Transaction, int64, error)
	GetWithdrawals(ctx context.Context, userID, currency string, limit, offset int) ([]*models.Transaction, int64, error)
	CompleteDeposit(ctx context.Context, transactionID string) error
	CompleteWithdrawal(ctx context.Context, transactionID string) error
}

// Service implements FiatService
type Service struct {
	logger        *zap.Logger
	db            *gorm.DB
	bookkeeperSvc bookkeeper.BookkeeperService
	kycService    *kyc.KYCService // Compliance hooks
}

// NewService creates a new FiatService
func NewService(logger *zap.Logger, db *gorm.DB, bookkeeperSvc bookkeeper.BookkeeperService, kycService *kyc.KYCService) (FiatService, error) {
	// Create service
	svc := &Service{
		logger:        logger,
		db:            db,
		bookkeeperSvc: bookkeeperSvc,
		kycService:    kycService,
	}

	return svc, nil
}

// Start starts the fiat service
func (s *Service) Start() error {
	s.logger.Info("Fiat service started")
	return nil
}

// Stop stops the fiat service
func (s *Service) Stop() error {
	s.logger.Info("Fiat service stopped")
	return nil
}

// InitiateDeposit initiates a fiat deposit
func (s *Service) InitiateDeposit(ctx context.Context, userID string, currency string, amount float64, provider string) (*models.Transaction, error) {
	// Validate currency
	if !isSupportedFiatCurrency(currency) {
		return nil, fmt.Errorf("unsupported currency: %s", currency)
	}

	// Validate amount
	if amount <= 0 {
		return nil, fmt.Errorf("invalid amount: %f", amount)
	}

	// Validate provider
	if !isSupportedProvider(provider) {
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}

	// Create transaction
	transaction, err := s.bookkeeperSvc.CreateTransaction(ctx, userID, "deposit", amount, currency, provider, fmt.Sprintf("Fiat deposit via %s", provider))
	if err != nil {
		return nil, fmt.Errorf("failed to create transaction: %w", err)
	}
	// Compliance: Monitor deposit
	if s.kycService != nil {
		uid, err := uuid.Parse(userID)
		if err == nil {
			_, amlErr := s.kycService.MonitorTransaction(ctx, uid, "deposit", provider, amount)
			if amlErr != nil {
				s.logger.Warn("AML alert on deposit", zap.String("userID", userID), zap.Error(amlErr))
			}
		}
	}
	// In a real implementation, this would initiate a deposit with the provider
	s.logger.Info("Initiating fiat deposit", zap.String("userID", userID), zap.String("currency", currency), zap.Float64("amount", amount), zap.String("provider", provider))

	return transaction, nil
}

// InitiateWithdrawal initiates a fiat withdrawal
func (s *Service) InitiateWithdrawal(ctx context.Context, userID string, currency string, amount float64, bankDetails map[string]interface{}) (*models.Transaction, error) {
	// Validate currency
	if !isSupportedFiatCurrency(currency) {
		return nil, fmt.Errorf("unsupported currency: %s", currency)
	}

	// Validate amount
	if amount <= 0 {
		return nil, fmt.Errorf("invalid amount: %f", amount)
	}

	// Validate bank details
	if err := validateBankDetails(bankDetails); err != nil {
		return nil, fmt.Errorf("invalid bank details: %w", err)
	}

	// Create transaction
	transaction, err := s.bookkeeperSvc.CreateTransaction(ctx, userID, "withdrawal", amount, currency, "bank", "Fiat withdrawal to bank account")
	if err != nil {
		return nil, fmt.Errorf("failed to create transaction: %w", err)
	}
	// Compliance: Monitor withdrawal
	if s.kycService != nil {
		uid, err := uuid.Parse(userID)
		if err == nil {
			_, amlErr := s.kycService.MonitorTransaction(ctx, uid, "withdrawal", "bank", amount)
			if amlErr != nil {
				s.logger.Warn("AML alert on withdrawal", zap.String("userID", userID), zap.Error(amlErr))
			}
		}
	}
	// Lock funds
	if err := s.bookkeeperSvc.LockFunds(ctx, userID, currency, amount); err != nil {
		// Fail transaction
		if failErr := s.bookkeeperSvc.FailTransaction(ctx, transaction.ID.String()); failErr != nil {
			s.logger.Error("Failed to fail transaction", zap.Error(failErr))
		}
		return nil, fmt.Errorf("failed to lock funds: %w", err)
	}

	// In a real implementation, this would initiate a withdrawal with the bank
	s.logger.Info("Initiating fiat withdrawal", zap.String("userID", userID), zap.String("currency", currency), zap.Float64("amount", amount))

	return transaction, nil
}

// GetDeposits gets fiat deposits for a user
func (s *Service) GetDeposits(ctx context.Context, userID string, currency string, limit, offset int) ([]*models.Transaction, int64, error) {
	// Build query
	query := s.db.Model(&models.Transaction{}).Where("user_id = ? AND type = ?", userID, "deposit")
	if currency != "" {
		query = query.Where("currency = ?", currency)
	}

	// Count total
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count deposits: %w", err)
	}

	// Get deposits
	var deposits []*models.Transaction
	if err := query.Order("created_at DESC").Limit(limit).Offset(offset).Find(&deposits).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to get deposits: %w", err)
	}

	return deposits, total, nil
}

// GetWithdrawals gets fiat withdrawals for a user
func (s *Service) GetWithdrawals(ctx context.Context, userID string, currency string, limit, offset int) ([]*models.Transaction, int64, error) {
	// Build query
	query := s.db.Model(&models.Transaction{}).Where("user_id = ? AND type = ?", userID, "withdrawal")
	if currency != "" {
		query = query.Where("currency = ?", currency)
	}

	// Count total
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count withdrawals: %w", err)
	}

	// Get withdrawals
	var withdrawals []*models.Transaction
	if err := query.Order("created_at DESC").Limit(limit).Offset(offset).Find(&withdrawals).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to get withdrawals: %w", err)
	}

	return withdrawals, total, nil
}

// CompleteDeposit completes a fiat deposit
func (s *Service) CompleteDeposit(ctx context.Context, transactionID string) error {
	// Complete transaction
	if err := s.bookkeeperSvc.CompleteTransaction(ctx, transactionID); err != nil {
		return fmt.Errorf("failed to complete transaction: %w", err)
	}

	return nil
}

// CompleteWithdrawal completes a fiat withdrawal
func (s *Service) CompleteWithdrawal(ctx context.Context, transactionID string) error {
	// Find transaction
	var transaction models.Transaction
	if err := s.db.Where("id = ?", transactionID).First(&transaction).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("transaction not found")
		}
		return fmt.Errorf("failed to find transaction: %w", err)
	}
	// Compliance: Monitor withdrawal completion
	if s.kycService != nil {
		_, amlErr := s.kycService.MonitorTransaction(ctx, transaction.UserID, "withdrawal_complete", "fiat", transaction.Amount)
		if amlErr != nil {
			s.logger.Warn("AML alert on withdrawal completion", zap.String("userID", transaction.UserID.String()), zap.Error(amlErr))
		}
	}
	// Check if transaction is a withdrawal
	if transaction.Type != "withdrawal" {
		return fmt.Errorf("transaction is not a withdrawal")
	}

	// Check if transaction is pending
	if transaction.Status != "pending" {
		return fmt.Errorf("transaction is not pending")
	}

	// Complete transaction
	if err := s.bookkeeperSvc.CompleteTransaction(ctx, transactionID); err != nil {
		return fmt.Errorf("failed to complete transaction: %w", err)
	}

	// Unlock and deduct funds
	if err := s.bookkeeperSvc.UnlockFunds(ctx, transaction.UserID.String(), transaction.Currency, transaction.Amount); err != nil {
		return fmt.Errorf("failed to unlock funds: %w", err)
	}

	return nil
}

// FailWithdrawal fails a fiat withdrawal
func (s *Service) FailWithdrawal(ctx context.Context, transactionID string) error {
	// Find transaction
	var transaction models.Transaction
	if err := s.db.Where("id = ?", transactionID).First(&transaction).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("transaction not found")
		}
		return fmt.Errorf("failed to find transaction: %w", err)
	}

	// Check if transaction is a withdrawal
	if transaction.Type != "withdrawal" {
		return fmt.Errorf("transaction is not a withdrawal")
	}

	// Check if transaction is pending
	if transaction.Status != "pending" {
		return fmt.Errorf("transaction is not pending")
	}

	// Fail transaction
	if err := s.bookkeeperSvc.FailTransaction(ctx, transactionID); err != nil {
		return fmt.Errorf("failed to fail transaction: %w", err)
	}

	// Unlock funds
	if err := s.bookkeeperSvc.UnlockFunds(ctx, transaction.UserID.String(), transaction.Currency, transaction.Amount); err != nil {
		return fmt.Errorf("failed to unlock funds: %w", err)
	}

	return nil
}

// isSupportedFiatCurrency returns true if the currency is supported
func isSupportedFiatCurrency(currency string) bool {
	supportedCurrencies := map[string]bool{
		"USD": true, "EUR": true, "GBP": true, "JPY": true, "CAD": true,
		"AUD": true, "CHF": true, "CNY": true, "SEK": true, "NZD": true,
		"MXN": true, "SGD": true, "HKD": true, "NOK": true, "TRY": true,
		"RUB": true, "INR": true, "BRL": true, "ZAR": true, "KRW": true,
	}

	// Validate and sanitize currency code
	if err := validation.ValidateCurrency(currency); err != nil {
		return false
	}

	return supportedCurrencies[strings.ToUpper(currency)]
}

// isSupportedProvider returns true if the provider is supported
func isSupportedProvider(provider string) bool {
	supportedProviders := map[string]bool{
		"stripe":    true,
		"plaid":     true,
		"dwolla":    true,
		"wise":      true,
		"revolut":   true,
		"paypal":    true,
		"adyen":     true,
		"checkout":  true,
		"square":    true,
		"braintree": true,
	}

	// Validate and sanitize provider name
	sanitized := validation.SanitizeInput(provider)
	if len(sanitized) == 0 || len(sanitized) > 50 {
		return false
	}

	// Check for SQL injection and XSS
	if validation.ContainsSQLInjection(provider) || validation.ContainsXSS(provider) {
		return false
	}

	return supportedProviders[strings.ToLower(sanitized)]
}

// validateBankDetails validates bank details map
func validateBankDetails(details map[string]interface{}) error {
	if details == nil {
		return errors.New("bank details cannot be nil")
	}

	// Required fields
	requiredFields := []string{"account_number", "routing_number", "account_type", "bank_name"}
	for _, field := range requiredFields {
		if _, exists := details[field]; !exists {
			return fmt.Errorf("required field '%s' is missing", field)
		}
	}

	// Validate account number
	if accountNum, ok := details["account_number"].(string); ok {
		if err := validation.ValidateBankAccount(accountNum); err != nil {
			return fmt.Errorf("invalid account number: %v", err)
		}
	} else {
		return errors.New("account_number must be a string")
	}

	// Validate routing number
	if routingNum, ok := details["routing_number"].(string); ok {
		if err := validation.ValidateRoutingNumber(routingNum); err != nil {
			return fmt.Errorf("invalid routing number: %v", err)
		}
	} else {
		return errors.New("routing_number must be a string")
	}

	// Validate account type
	if accountType, ok := details["account_type"].(string); ok {
		validTypes := map[string]bool{"checking": true, "savings": true, "business": true}
		sanitized := validation.SanitizeInput(accountType)
		if !validTypes[strings.ToLower(sanitized)] {
			return errors.New("account_type must be 'checking', 'savings', or 'business'")
		}
	} else {
		return errors.New("account_type must be a string")
	}

	// Validate bank name
	if bankName, ok := details["bank_name"].(string); ok {
		sanitized := validation.SanitizeInput(bankName)
		if len(sanitized) == 0 || len(sanitized) > 100 {
			return errors.New("bank_name must be between 1 and 100 characters")
		}

		// Check for malicious content
		if validation.ContainsSQLInjection(bankName) || validation.ContainsXSS(bankName) {
			return errors.New("bank_name contains invalid characters")
		}
	} else {
		return errors.New("bank_name must be a string")
	}

	// Validate optional IBAN if provided
	if iban, exists := details["iban"]; exists {
		if ibanStr, ok := iban.(string); ok && ibanStr != "" {
			if err := validation.ValidateIBAN(ibanStr); err != nil {
				return fmt.Errorf("invalid IBAN: %v", err)
			}
		}
	}

	// Validate optional BIC if provided
	if bic, exists := details["bic"]; exists {
		if bicStr, ok := bic.(string); ok && bicStr != "" {
			if err := validation.ValidateBIC(bicStr); err != nil {
				return fmt.Errorf("invalid BIC: %v", err)
			}
		}
	}

	// Validate optional beneficiary name
	if beneficiary, exists := details["beneficiary_name"]; exists {
		if beneficiaryStr, ok := beneficiary.(string); ok {
			sanitized := validation.SanitizeInput(beneficiaryStr)
			if len(sanitized) == 0 || len(sanitized) > 100 {
				return errors.New("beneficiary_name must be between 1 and 100 characters")
			}

			// Check for malicious content
			if validation.ContainsSQLInjection(beneficiaryStr) || validation.ContainsXSS(beneficiaryStr) {
				return errors.New("beneficiary_name contains invalid characters")
			}
		}
	}

	return nil
}
