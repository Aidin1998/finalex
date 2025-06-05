// filepath: c:\Orbit CEX\Finalex\internal\userauth\kyc\interface.go
package kyc

import (
	"context"

	"github.com/Aidin1998/pincex_unified/internal/userauth/models"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Service interface for KYC operations
type Service interface {
	InitiateKYC(ctx context.Context, userID uuid.UUID, targetLevel KYCLevel) (*models.KYCDocument, error)
	SubmitDocument(ctx context.Context, kycID uuid.UUID, documentType, documentPath string, metadata map[string]interface{}) error
	ReviewKYC(ctx context.Context, kycID uuid.UUID, reviewerID uuid.UUID, approved bool, comments string) error
	GetKYCStatus(ctx context.Context, userID uuid.UUID) (*models.KYCDocument, error)
	GetKYCRequirements(level KYCLevel) (KYCRequirement, error)
	ValidateTransactionLimits(ctx context.Context, userID uuid.UUID, amount float64) error
	ExpireKYC(ctx context.Context) error
	GetPendingKYCs(ctx context.Context, limit, offset int) ([]models.KYCDocument, error)
}

// Ensure our service implements the interface
var _ Service = (*service)(nil)

// Rename the concrete service to lowercase
type service struct {
	db           *gorm.DB
	logger       *zap.Logger
	requirements map[KYCLevel]KYCRequirement
}

// NewService creates a new KYC service that implements the Service interface
func NewService(logger *zap.Logger, db *gorm.DB) Service {
	return &service{
		db:     db,
		logger: logger,
		requirements: map[KYCLevel]KYCRequirement{
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
		},
	}
}
