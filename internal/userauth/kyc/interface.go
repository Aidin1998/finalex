// filepath: c:\Orbit CEX\Finalex\internal\userauth\kyc\interface.go
package kyc

import (
	"context"

	"github.com/Aidin1998/finalex/internal/userauth/models"
	"github.com/google/uuid"
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
