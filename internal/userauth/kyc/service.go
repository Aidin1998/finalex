// filepath: c:\Orbit CEX\Finalex\internal\userauth\kyc\service.go
package kyc

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	pkgmodels "github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// KYCLevel represents different levels of KYC verification
type KYCLevel int

const (
	KYCLevelNone KYCLevel = iota
	KYCLevelBasic
	KYCLevelIntermediate
	KYCLevelAdvanced
	KYCLevelInstitutional
)

// KYCStatus represents the current status of KYC process
type KYCStatus string

const (
	KYCStatusPending   KYCStatus = "pending"
	KYCStatusApproved  KYCStatus = "approved"
	KYCStatusRejected  KYCStatus = "rejected"
	KYCStatusExpired   KYCStatus = "expired"
	KYCStatusSuspended KYCStatus = "suspended"
)

// KYCRequirement represents requirements for different KYC levels
type KYCRequirement struct {
	Level                KYCLevel `json:"level"`
	RequiredDocuments    []string `json:"required_documents"`
	BiometricRequired    bool     `json:"biometric_required"`
	AddressProofRequired bool     `json:"address_proof_required"`
	IncomeProofRequired  bool     `json:"income_proof_required"`
	MaxTransactionLimit  float64  `json:"max_transaction_limit"`
	MaxDailyLimit        float64  `json:"max_daily_limit"`
	MaxMonthlyLimit      float64  `json:"max_monthly_limit"`
}

// InitiateKYC starts the KYC process for a user
func InitiateKYC(ctx context.Context, userID uuid.UUID, targetLevel KYCLevel, db *gorm.DB, logger *zap.Logger) (*pkgmodels.KYCRequest, error) {
	requirements := getKYCRequirements()

	_, exists := requirements[targetLevel]
	if !exists {
		return nil, fmt.Errorf("invalid KYC level: %d", targetLevel)
	}

	// Check if user already has pending or approved KYC
	var existingKYC pkgmodels.KYCRequest
	err := db.WithContext(ctx).Where("user_id = ? AND status IN ?", userID, []string{"pending", "approved"}).First(&existingKYC).Error
	if err == nil {
		return nil, fmt.Errorf("user already has active KYC process")
	}

	kycDoc := &pkgmodels.KYCRequest{
		ID:          uuid.New(),
		UserID:      userID,
		Level:       int(targetLevel),
		Status:      string(KYCStatusPending),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		CompletedAt: func() *time.Time { t := time.Now().AddDate(1, 0, 0); return &t }(), // 1 year validity
	}

	if err := db.WithContext(ctx).Create(kycDoc).Error; err != nil {
		logger.Error("Failed to create KYC document", zap.Error(err), zap.String("user_id", userID.String()))
		return nil, fmt.Errorf("failed to initiate KYC: %w", err)
	}

	logger.Info("KYC process initiated",
		zap.String("user_id", userID.String()),
		zap.Int("kyc_level", int(targetLevel)),
		zap.String("kyc_id", kycDoc.ID.String()))

	return kycDoc, nil
}

// SubmitDocument submits a document for KYC verification
func SubmitDocument(ctx context.Context, kycID uuid.UUID, documentType, documentPath string, metadata map[string]interface{}, db *gorm.DB, logger *zap.Logger) error {
	var kycDoc pkgmodels.KYCRequest
	if err := db.WithContext(ctx).First(&kycDoc, "id = ?", kycID).Error; err != nil {
		return fmt.Errorf("KYC document not found: %w", err)
	}

	if kycDoc.Status != string(KYCStatusPending) {
		return fmt.Errorf("KYC is not in pending status")
	}

	// Store metadata as JSON
	metadataJSON, _ := json.Marshal(metadata)

	updates := map[string]interface{}{
		"document_metadata": string(metadataJSON),
		"updated_at":        time.Now(),
	}

	if err := db.WithContext(ctx).Model(&kycDoc).Updates(updates).Error; err != nil {
		logger.Error("Failed to update KYC document", zap.Error(err), zap.String("kyc_id", kycID.String()))
		return fmt.Errorf("failed to submit document: %w", err)
	}

	logger.Info("Document submitted for KYC",
		zap.String("kyc_id", kycID.String()),
		zap.String("document_type", documentType))

	return nil
}

// ReviewKYC performs automated and manual review of KYC documents
func ReviewKYC(ctx context.Context, kycID uuid.UUID, reviewerID uuid.UUID, approved bool, comments string, db *gorm.DB, logger *zap.Logger) error {
	var kycDoc pkgmodels.KYCRequest
	if err := db.WithContext(ctx).First(&kycDoc, "id = ?", kycID).Error; err != nil {
		return fmt.Errorf("KYC document not found: %w", err)
	}

	status := KYCStatusRejected
	if approved {
		status = KYCStatusApproved
	}

	updates := map[string]interface{}{
		"status":       string(status),
		"reviewer_id":  reviewerID,
		"reviewed_at":  time.Now(),
		"review_notes": comments,
		"updated_at":   time.Now(),
	}

	if approved {
		updates["approved_at"] = time.Now()
	}

	if err := db.WithContext(ctx).Model(&kycDoc).Updates(updates).Error; err != nil {
		logger.Error("Failed to update KYC review", zap.Error(err), zap.String("kyc_id", kycID.String()))
		return fmt.Errorf("failed to review KYC: %w", err)
	}

	logger.Info("KYC reviewed",
		zap.String("kyc_id", kycID.String()),
		zap.String("reviewer_id", reviewerID.String()),
		zap.Bool("approved", approved))

	return nil
}

// GetKYCStatus retrieves the current KYC status for a user
func GetKYCStatus(ctx context.Context, userID uuid.UUID, db *gorm.DB) (*pkgmodels.KYCRequest, error) {
	var kycDoc pkgmodels.KYCRequest
	err := db.WithContext(ctx).Where("user_id = ?", userID).Order("created_at DESC").First(&kycDoc).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil // No KYC process started
		}
		return nil, fmt.Errorf("failed to get KYC status: %w", err)
	}

	return &kycDoc, nil
}

// GetKYCRequirements returns the requirements for a specific KYC level
func GetKYCRequirements(level KYCLevel) (KYCRequirement, error) {
	requirement, exists := getKYCRequirements()[level]
	if !exists {
		return KYCRequirement{}, fmt.Errorf("invalid KYC level: %d", level)
	}
	return requirement, nil
}

// ValidateTransactionLimits checks if a transaction is within KYC limits
func ValidateTransactionLimits(ctx context.Context, userID uuid.UUID, amount float64, db *gorm.DB) error {
	kycDoc, err := GetKYCStatus(ctx, userID, db)
	if err != nil {
		return fmt.Errorf("failed to get KYC status: %w", err)
	}

	if kycDoc == nil || kycDoc.Status != string(KYCStatusApproved) {
		// Default limits for unverified users
		if amount > 100 {
			return fmt.Errorf("transaction amount exceeds limit for unverified users")
		}
		return nil
	}

	requirement, err := GetKYCRequirements(KYCLevel(kycDoc.Level))
	if err != nil {
		return fmt.Errorf("failed to get KYC requirements: %w", err)
	}

	if amount > requirement.MaxTransactionLimit {
		return fmt.Errorf("transaction amount exceeds KYC limit of %.2f", requirement.MaxTransactionLimit)
	}

	return nil
}

// ExpireKYC marks expired KYC documents as expired
func ExpireKYC(ctx context.Context, db *gorm.DB, logger *zap.Logger) error {
	result := db.WithContext(ctx).Model(&pkgmodels.KYCRequest{}).
		Where("status = ? AND completed_at < ?", string(KYCStatusApproved), time.Now()).
		Update("status", string(KYCStatusExpired))

	if result.Error != nil {
		logger.Error("Failed to expire KYC documents", zap.Error(result.Error))
		return fmt.Errorf("failed to expire KYC documents: %w", result.Error)
	}

	if result.RowsAffected > 0 {
		logger.Info("Expired KYC documents", zap.Int64("count", result.RowsAffected))
	}

	return nil
}

// GetPendingKYCs retrieves all pending KYC documents for review
func GetPendingKYCs(ctx context.Context, limit, offset int, db *gorm.DB, logger *zap.Logger) ([]pkgmodels.KYCRequest, error) {
	var kycDocs []pkgmodels.KYCRequest
	err := db.WithContext(ctx).
		Where("status = ?", string(KYCStatusPending)).
		Order("created_at ASC").
		Limit(limit).
		Offset(offset).
		Find(&kycDocs).Error

	if err != nil {
		logger.Error("Failed to get pending KYCs", zap.Error(err))
		return nil, fmt.Errorf("failed to get pending KYCs: %w", err)
	}

	return kycDocs, nil
}

func getKYCRequirements() map[KYCLevel]KYCRequirement {
	return map[KYCLevel]KYCRequirement{
		KYCLevelBasic: {
			Level:               KYCLevelBasic,
			RequiredDocuments:   []string{"government_id"},
			BiometricRequired:   false,
			MaxTransactionLimit: 1000,
			MaxDailyLimit:       5000,
			MaxMonthlyLimit:     50000,
		},
		KYCLevelIntermediate: {
			Level:                KYCLevelIntermediate,
			RequiredDocuments:    []string{"government_id", "address_proof"},
			BiometricRequired:    true,
			AddressProofRequired: true,
			MaxTransactionLimit:  10000,
			MaxDailyLimit:        50000,
			MaxMonthlyLimit:      500000,
		},
		KYCLevelAdvanced: {
			Level:                KYCLevelAdvanced,
			RequiredDocuments:    []string{"government_id", "address_proof", "income_proof"},
			BiometricRequired:    true,
			AddressProofRequired: true,
			IncomeProofRequired:  true,
			MaxTransactionLimit:  100000,
			MaxDailyLimit:        500000,
			MaxMonthlyLimit:      5000000,
		},
		KYCLevelInstitutional: {
			Level:                KYCLevelInstitutional,
			RequiredDocuments:    []string{"corporate_documents", "beneficial_ownership", "compliance_certificate"},
			BiometricRequired:    false,
			AddressProofRequired: true,
			IncomeProofRequired:  true,
			MaxTransactionLimit:  1000000,
			MaxDailyLimit:        10000000,
			MaxMonthlyLimit:      100000000,
		},
	}
}
