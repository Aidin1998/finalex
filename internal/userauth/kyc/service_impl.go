package kyc

import (
	"context"

	"github.com/Aidin1998/finalex/internal/userauth/models"
	pkgmodels "github.com/Aidin1998/finalex/pkg/models"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ServiceImpl implements the KYC Service interface
type ServiceImpl struct {
	logger *zap.Logger
	db     *gorm.DB
}

// NewService creates a new KYC service implementation
func NewService(logger *zap.Logger, db *gorm.DB) Service {
	return &ServiceImpl{
		logger: logger,
		db:     db,
	}
}

// convertKYCRequestToDocument converts pkgmodels.KYCRequest to models.KYCDocument
func convertKYCRequestToDocument(req *pkgmodels.KYCRequest) *models.KYCDocument {
	if req == nil {
		return nil
	}

	return &models.KYCDocument{
		ID:             req.ID,
		UserID:         req.UserID,
		DocumentStatus: req.Status,
		CreatedAt:      req.CreatedAt,
		UpdatedAt:      req.UpdatedAt,
		// Map other fields as needed
	}
}

// convertKYCRequestsToDocuments converts slice of pkgmodels.KYCRequest to slice of models.KYCDocument
func convertKYCRequestsToDocuments(requests []pkgmodels.KYCRequest) []models.KYCDocument {
	documents := make([]models.KYCDocument, len(requests))
	for i, req := range requests {
		documents[i] = *convertKYCRequestToDocument(&req)
	}
	return documents
}

// InitiateKYC starts the KYC process for a user
func (s *ServiceImpl) InitiateKYC(ctx context.Context, userID uuid.UUID, targetLevel KYCLevel) (*models.KYCDocument, error) {
	// Use the existing InitiateKYC function
	kycRequest, err := InitiateKYC(ctx, userID, targetLevel, s.db, s.logger)
	if err != nil {
		return nil, err
	}
	return convertKYCRequestToDocument(kycRequest), nil
}

// SubmitDocument submits a KYC document
func (s *ServiceImpl) SubmitDocument(ctx context.Context, kycID uuid.UUID, documentType, documentPath string, metadata map[string]interface{}) error {
	// Use the existing SubmitDocument function
	return SubmitDocument(ctx, kycID, documentType, documentPath, metadata, s.db, s.logger)
}

// ReviewKYC reviews a KYC submission
func (s *ServiceImpl) ReviewKYC(ctx context.Context, kycID uuid.UUID, reviewerID uuid.UUID, approved bool, comments string) error {
	// Use the existing ReviewKYC function
	return ReviewKYC(ctx, kycID, reviewerID, approved, comments, s.db, s.logger)
}

// GetKYCStatus gets the KYC status for a user
func (s *ServiceImpl) GetKYCStatus(ctx context.Context, userID uuid.UUID) (*models.KYCDocument, error) {
	// Use the existing GetKYCStatus function
	kycRequest, err := GetKYCStatus(ctx, userID, s.db)
	if err != nil {
		return nil, err
	}
	return convertKYCRequestToDocument(kycRequest), nil
}

// GetKYCRequirements gets requirements for a KYC level
func (s *ServiceImpl) GetKYCRequirements(level KYCLevel) (KYCRequirement, error) {
	// Use the existing GetKYCRequirements function
	return GetKYCRequirements(level)
}

// ValidateTransactionLimits validates transaction limits for a user
func (s *ServiceImpl) ValidateTransactionLimits(ctx context.Context, userID uuid.UUID, amount float64) error {
	// Use the existing ValidateTransactionLimits function
	return ValidateTransactionLimits(ctx, userID, amount, s.db)
}

// ExpireKYC expires old KYC records
func (s *ServiceImpl) ExpireKYC(ctx context.Context) error {
	// Use the existing ExpireKYC function
	return ExpireKYC(ctx, s.db, s.logger)
}

// GetPendingKYCs gets pending KYC submissions
func (s *ServiceImpl) GetPendingKYCs(ctx context.Context, limit, offset int) ([]models.KYCDocument, error) {
	// Use the existing GetPendingKYCs function
	kycRequests, err := GetPendingKYCs(ctx, limit, offset, s.db, s.logger)
	if err != nil {
		return nil, err
	}
	return convertKYCRequestsToDocuments(kycRequests), nil
}
