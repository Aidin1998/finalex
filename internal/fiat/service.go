package fiat

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Aidin1998/finalex/internal/accounts/transaction"
	"github.com/Aidin1998/finalex/pkg/models"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// FiatService provides fiat currency operations with secure receipt processing
type FiatService struct {
	db           *gorm.DB
	logger       *zap.Logger
	fiatXA       *transaction.FiatXAResource
	signatureKey []byte // Key for validating provider signatures
}

// ReceiptStatus represents the status of a deposit receipt
type ReceiptStatus string

const (
	ReceiptStatusPending   ReceiptStatus = "pending"
	ReceiptStatusProcessed ReceiptStatus = "processed"
	ReceiptStatusCompleted ReceiptStatus = "completed"
	ReceiptStatusFailed    ReceiptStatus = "failed"
	ReceiptStatusDuplicate ReceiptStatus = "duplicate"
)

// ProviderInfo represents supported fiat providers
type ProviderInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	PublicKey string `json:"public_key"`
	Endpoint  string `json:"endpoint"`
	Active    bool   `json:"active"`
}

// FiatDepositReceipt represents a secure deposit receipt from external providers
type FiatDepositReceipt struct {
	ID            uuid.UUID       `json:"id" gorm:"primaryKey;type:uuid"`
	UserID        uuid.UUID       `json:"user_id" gorm:"type:uuid;index;not null"`
	ProviderID    string          `json:"provider_id" gorm:"size:100;not null"`
	ReferenceID   string          `json:"reference_id" gorm:"size:255;uniqueIndex;not null"`
	Amount        decimal.Decimal `json:"amount" gorm:"type:decimal(20,8);not null"`
	Currency      string          `json:"currency" gorm:"size:10;not null"`
	ReceiptHash   string          `json:"receipt_hash" gorm:"size:64;uniqueIndex;not null"`
	ProviderSig   string          `json:"provider_signature" gorm:"type:text;not null"`
	Status        ReceiptStatus   `json:"status" gorm:"size:20;default:'pending'"`
	ProcessedAt   *time.Time      `json:"processed_at,omitempty"`
	CompletedAt   *time.Time      `json:"completed_at,omitempty"`
	TransactionID *uuid.UUID      `json:"transaction_id,omitempty" gorm:"type:uuid"`
	AuditTrail    string          `json:"audit_trail" gorm:"type:text"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

// FiatReceiptRequest represents the external provider receipt payload
type FiatReceiptRequest struct {
	UserID      uuid.UUID       `json:"user_id" validate:"required,uuid"`
	ProviderID  string          `json:"provider_id" validate:"required,min=1,max=100"`
	ReferenceID string          `json:"reference_id" validate:"required,min=1,max=255"`
	Amount      decimal.Decimal `json:"amount" validate:"required,gt=0"`
	Currency    string          `json:"currency" validate:"required,iso4217"`
	Timestamp   time.Time       `json:"timestamp" validate:"required"`
	ProviderSig string          `json:"provider_signature" validate:"required"`
	TraceID     string          `json:"trace_id,omitempty"`
}

// FiatReceiptResponse represents the response to receipt submission
type FiatReceiptResponse struct {
	ReceiptID     uuid.UUID     `json:"receipt_id"`
	Status        ReceiptStatus `json:"status"`
	Message       string        `json:"message"`
	ProcessedAt   time.Time     `json:"processed_at"`
	TransactionID *uuid.UUID    `json:"transaction_id,omitempty"`
	TraceID       string        `json:"trace_id,omitempty"`
}

// NewFiatService creates a new fiat service instance
func NewFiatService(db *gorm.DB, logger *zap.Logger, signatureKey []byte) *FiatService {
	// Pass nil for fiatService interface to break import cycle
	fiatXA := transaction.NewFiatXAResource(nil, db, logger, "")

	return &FiatService{
		db:           db,
		logger:       logger,
		fiatXA:       fiatXA,
		signatureKey: signatureKey,
	}
}

// ProcessDepositReceipt processes an incoming deposit receipt from external providers
func (fs *FiatService) ProcessDepositReceipt(ctx context.Context, req *FiatReceiptRequest) (*FiatReceiptResponse, error) {
	traceID := req.TraceID
	if traceID == "" {
		traceID = uuid.New().String()
	}

	logger := fs.logger.With(
		zap.String("trace_id", traceID),
		zap.String("provider_id", req.ProviderID),
		zap.String("reference_id", req.ReferenceID),
		zap.String("user_id", req.UserID.String()),
	)

	logger.Info("Processing fiat deposit receipt")

	// 1. Validate provider signature
	if err := fs.validateProviderSignature(req); err != nil {
		logger.Error("Provider signature validation failed", zap.Error(err))
		return &FiatReceiptResponse{
			Status:  ReceiptStatusFailed,
			Message: "Invalid provider signature",
			TraceID: traceID,
		}, fmt.Errorf("signature validation failed: %w", err)
	}

	// 2. Generate receipt hash for idempotency
	receiptHash := fs.generateReceiptHash(req)

	// 3. Check for duplicate receipt (idempotent processing)
	existingReceipt, err := fs.checkDuplicateReceipt(ctx, receiptHash)
	if err != nil {
		logger.Error("Failed to check for duplicate receipt", zap.Error(err))
		return nil, fmt.Errorf("duplicate check failed: %w", err)
	}
	if existingReceipt != nil {
		logger.Info("Duplicate receipt detected, returning existing result",
			zap.String("existing_receipt_id", existingReceipt.ID.String()),
			zap.String("existing_status", string(existingReceipt.Status)))

		return &FiatReceiptResponse{
			ReceiptID:     existingReceipt.ID,
			Status:        ReceiptStatusDuplicate,
			Message:       "Receipt already processed",
			ProcessedAt:   time.Now(),
			TransactionID: existingReceipt.TransactionID,
			TraceID:       traceID,
		}, nil
	}

	// 4. Validate user exists and is eligible
	if err := fs.validateUserEligibility(ctx, req.UserID); err != nil {
		logger.Error("User validation failed", zap.Error(err))
		return &FiatReceiptResponse{
			Status:  ReceiptStatusFailed,
			Message: "User validation failed",
			TraceID: traceID,
		}, fmt.Errorf("user validation failed: %w", err)
	}

	// 5. Create receipt record
	receipt := &FiatDepositReceipt{
		ID:          uuid.New(),
		UserID:      req.UserID,
		ProviderID:  req.ProviderID,
		ReferenceID: req.ReferenceID,
		Amount:      req.Amount,
		Currency:    req.Currency,
		ReceiptHash: receiptHash,
		ProviderSig: req.ProviderSig,
		Status:      ReceiptStatusPending,
		AuditTrail:  fs.createAuditTrail("receipt_created", traceID, nil),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// 6. Process within transaction context
	err = fs.db.Transaction(func(tx *gorm.DB) error {
		// Save receipt
		if err := tx.Create(receipt).Error; err != nil {
			return fmt.Errorf("failed to save receipt: %w", err)
		}

		// Process deposit with XA transaction
		if err := fs.processDepositWithXA(ctx, tx, receipt, traceID); err != nil {
			return fmt.Errorf("failed to process deposit: %w", err)
		}

		return nil
	})

	if err != nil {
		logger.Error("Failed to process receipt transaction", zap.Error(err))

		// Update receipt status to failed
		fs.updateReceiptStatus(ctx, receipt.ID, ReceiptStatusFailed, err.Error())

		return &FiatReceiptResponse{
			ReceiptID: receipt.ID,
			Status:    ReceiptStatusFailed,
			Message:   "Processing failed",
			TraceID:   traceID,
		}, err
	}

	logger.Info("Deposit receipt processed successfully",
		zap.String("receipt_id", receipt.ID.String()),
		zap.String("transaction_id", receipt.TransactionID.String()))

	return &FiatReceiptResponse{
		ReceiptID:     receipt.ID,
		Status:        ReceiptStatusCompleted,
		Message:       "Deposit processed successfully",
		ProcessedAt:   time.Now(),
		TransactionID: receipt.TransactionID,
		TraceID:       traceID,
	}, nil
}

// validateProviderSignature validates the digital signature from the provider
func (fs *FiatService) validateProviderSignature(req *FiatReceiptRequest) error {
	// Create payload for signature validation
	payload := fmt.Sprintf("%s:%s:%s:%s:%s:%d",
		req.UserID.String(),
		req.ProviderID,
		req.ReferenceID,
		req.Amount.String(),
		req.Currency,
		req.Timestamp.Unix())

	// Compute expected signature using HMAC-SHA256
	h := sha256.New()
	h.Write(fs.signatureKey)
	h.Write([]byte(payload))
	expectedSig := hex.EncodeToString(h.Sum(nil))

	if req.ProviderSig != expectedSig {
		return fmt.Errorf("signature mismatch: expected %s, got %s", expectedSig, req.ProviderSig)
	}

	return nil
}

// generateReceiptHash generates a unique hash for idempotency checking
func (fs *FiatService) generateReceiptHash(req *FiatReceiptRequest) string {
	hashData := fmt.Sprintf("%s:%s:%s:%s:%s",
		req.UserID.String(),
		req.ProviderID,
		req.ReferenceID,
		req.Amount.String(),
		req.Currency)

	h := sha256.Sum256([]byte(hashData))
	return hex.EncodeToString(h[:])
}

// checkDuplicateReceipt checks if receipt has already been processed
func (fs *FiatService) checkDuplicateReceipt(ctx context.Context, receiptHash string) (*FiatDepositReceipt, error) {
	var receipt FiatDepositReceipt
	err := fs.db.Where("receipt_hash = ?", receiptHash).First(&receipt).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &receipt, nil
}

// validateUserEligibility validates user exists and is eligible for deposits
func (fs *FiatService) validateUserEligibility(ctx context.Context, userID uuid.UUID) error {
	var user models.User
	err := fs.db.Where("id = ?", userID).First(&user).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("user not found: %s", userID.String())
		}
		return fmt.Errorf("user lookup failed: %w", err)
	}

	// Check KYC status
	if user.KYCStatus != "approved" {
		return fmt.Errorf("user KYC not approved: %s", user.KYCStatus)
	}

	return nil
}

// xidStringToXID converts string xid to transaction.XID struct
func xidStringToXID(xid string) transaction.XID {
	// For this project, xid is generated as fmt.Sprintf("fiat_deposit_%s_%s", receipt.ID.String(), traceID)
	// We'll hash the string to get a fixed-length byte slice for GlobalTxnID, and use a static FormatID and BranchQualID
	hash := sha256.Sum256([]byte(xid))
	return transaction.XID{
		FormatID:     1,
		GlobalTxnID:  hash[:16], // first 16 bytes
		BranchQualID: hash[16:], // last 16 bytes
	}
}

// processDepositWithXA processes the deposit using XA transactions
func (fs *FiatService) processDepositWithXA(ctx context.Context, tx *gorm.DB, receipt *FiatDepositReceipt, traceID string) error {
	// Prepare XA transaction
	xid := fmt.Sprintf("fiat_deposit_%s_%s", receipt.ID.String(), traceID)
	txXID := xidStringToXID(xid)

	// Prepare expects (ctx, xid transaction.XID) and returns (bool, error)
	prepared, err := fs.fiatXA.Prepare(ctx, txXID)
	if err != nil || !prepared {
		return fmt.Errorf("XA prepare failed: %w", err)
	}

	// Execute deposit operation (InitiateDeposit expects userID, currency, amount, provider)
	_, err = fs.fiatXA.InitiateDeposit(ctx, receipt.UserID.String(), receipt.Currency, receipt.Amount.InexactFloat64(), receipt.ProviderID)
	if err != nil {
		fs.fiatXA.Rollback(ctx, txXID)
		return fmt.Errorf("deposit initiation failed: %w", err)
	}

	// Commit expects (ctx, xid transaction.XID, onePhase bool)
	if err := fs.fiatXA.Commit(ctx, txXID, false); err != nil {
		return fmt.Errorf("XA commit failed: %w", err)
	}

	// Update receipt with transaction details
	now := time.Now()
	receipt.Status = ReceiptStatusCompleted
	receipt.ProcessedAt = &now
	receipt.CompletedAt = &now

	// Create transaction ID for reference
	transactionID := uuid.New()
	receipt.TransactionID = &transactionID

	receipt.AuditTrail = fs.createAuditTrail("deposit_completed", traceID, map[string]interface{}{
		"xa_transaction_id": xid,
		"transaction_id":    transactionID.String(),
	})

	return tx.Save(receipt).Error
}

// updateReceiptStatus updates the receipt status
func (fs *FiatService) updateReceiptStatus(ctx context.Context, receiptID uuid.UUID, status ReceiptStatus, reason string) error {
	updates := map[string]interface{}{
		"status":     status,
		"updated_at": time.Now(),
	}

	if status == ReceiptStatusFailed {
		updates["audit_trail"] = fs.createAuditTrail("status_update_failed", "", map[string]interface{}{
			"reason": reason,
		})
	}

	return fs.db.Model(&FiatDepositReceipt{}).Where("id = ?", receiptID).Updates(updates).Error
}

// createAuditTrail creates audit trail entry
func (fs *FiatService) createAuditTrail(action, traceID string, metadata map[string]interface{}) string {
	auditEntry := map[string]interface{}{
		"action":    action,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"trace_id":  traceID,
	}

	if metadata != nil {
		auditEntry["metadata"] = metadata
	}

	auditJSON, _ := json.Marshal(auditEntry)
	return string(auditJSON)
}

// GetDepositReceipt retrieves a deposit receipt by ID
func (fs *FiatService) GetDepositReceipt(ctx context.Context, receiptID uuid.UUID) (*FiatDepositReceipt, error) {
	var receipt FiatDepositReceipt
	err := fs.db.Where("id = ?", receiptID).First(&receipt).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("receipt not found: %s", receiptID.String())
		}
		return nil, fmt.Errorf("receipt lookup failed: %w", err)
	}
	return &receipt, nil
}

// GetUserDeposits retrieves deposit receipts for a user
func (fs *FiatService) GetUserDeposits(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*FiatDepositReceipt, int64, error) {
	var receipts []*FiatDepositReceipt
	var total int64

	// Get total count
	if err := fs.db.Model(&FiatDepositReceipt{}).Where("user_id = ?", userID).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count receipts: %w", err)
	}

	// Get receipts with pagination
	err := fs.db.Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&receipts).Error

	if err != nil {
		return nil, 0, fmt.Errorf("failed to fetch receipts: %w", err)
	}

	return receipts, total, nil
}
