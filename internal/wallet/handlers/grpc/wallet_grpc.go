// Package grpc provides gRPC handlers for the wallet service
package grpc

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Aidin1998/finalex/internal/wallet/interfaces"
	pb "github.com/Aidin1998/finalex/proto/wallet"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// WalletHandler implements the gRPC wallet service
type WalletHandler struct {
	pb.UnimplementedWalletServiceServer
	walletService interfaces.WalletService
}

// NewWalletHandler creates a new wallet gRPC handler
func NewWalletHandler(walletService interfaces.WalletService) *WalletHandler {
	return &WalletHandler{
		walletService: walletService,
	}
}

// RequestDeposit handles deposit requests
func (h *WalletHandler) RequestDeposit(ctx context.Context, req *pb.RequestDepositRequest) (*pb.RequestDepositResponse, error) {
	// Validate request
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}
	if req.Asset == "" {
		return nil, status.Error(codes.InvalidArgument, "asset is required")
	}
	if req.Network == "" {
		return nil, status.Error(codes.InvalidArgument, "network is required")
	}

	userID, err := uuid.Parse(req.UserId)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid user_id format")
	}

	// Create deposit request
	depositReq := interfaces.DepositRequest{
		UserID:          userID,
		Asset:           req.Asset,
		Network:         req.Network,
		GenerateAddress: req.GenerateAddress,
	}

	// Process deposit
	response, err := h.walletService.RequestDeposit(ctx, &depositReq)
	if err != nil {
		return nil, h.handleError(err)
	}

	// Convert response (use only proto fields)
	pbResponse := &pb.RequestDepositResponse{
		TransactionId: response.TransactionID.String(),
		Status:        string(response.Status),
		Address:       response.Address.Address,
		QrCode:        response.QRCode,
		MinDeposit:    response.MinDeposit.String(),
		CreatedAt:     nil, // Set if available in response
	}

	return pbResponse, nil
}

// RequestWithdrawal handles withdrawal requests
func (h *WalletHandler) RequestWithdrawal(ctx context.Context, req *pb.RequestWithdrawalRequest) (*pb.RequestWithdrawalResponse, error) {
	// Validate request
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}
	if req.Asset == "" {
		return nil, status.Error(codes.InvalidArgument, "asset is required")
	}
	if req.Amount == "" {
		return nil, status.Error(codes.InvalidArgument, "amount is required")
	}
	if req.ToAddress == "" {
		return nil, status.Error(codes.InvalidArgument, "to_address is required")
	}
	if req.TwoFactorToken == "" {
		return nil, status.Error(codes.InvalidArgument, "two_factor_token is required")
	}

	userID, err := uuid.Parse(req.UserId)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid user_id format")
	}

	amount, err := decimal.NewFromString(req.Amount)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid amount format")
	}

	// Create withdrawal request
	withdrawalReq := interfaces.WithdrawalRequest{
		UserID:         userID,
		Asset:          req.Asset,
		Amount:         amount,
		ToAddress:      req.ToAddress,
		Network:        req.Network,
		Tag:            req.Tag,
		TwoFactorToken: req.TwoFactorToken,
		Priority:       interfaces.WithdrawalPriority(req.Priority),
		Note:           req.Note,
	}

	// Process withdrawal
	response, err := h.walletService.RequestWithdrawal(ctx, &withdrawalReq)
	if err != nil {
		return nil, h.handleError(err)
	}

	return &pb.RequestWithdrawalResponse{
		TransactionId: response.TransactionID.String(),
		Status:        string(response.Status),
		EstimatedTime: fmt.Sprintf("%ds", int(response.EstimatedTime.Seconds())), // Convert duration to string
		Fee:           response.Fee.String(),
		// NetAmount omitted (not present in response)
	}, nil
}

// GetTransactionStatus returns transaction status
func (h *WalletHandler) GetTransactionStatus(ctx context.Context, req *pb.GetTransactionStatusRequest) (*pb.GetTransactionStatusResponse, error) {
	if req.TransactionId == "" {
		return nil, status.Error(codes.InvalidArgument, "transaction_id is required")
	}

	txID, err := uuid.Parse(req.TransactionId)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid transaction_id format")
	}

	txStatus, err := h.walletService.GetTransactionStatus(ctx, txID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, status.Error(codes.NotFound, "transaction not found")
		}
		return nil, h.handleError(err)
	}

	return &pb.GetTransactionStatusResponse{
		TransactionId: txStatus.ID.String(),
		Status:        string(txStatus.Status),
		Confirmations: int32(txStatus.Confirmations),
		UpdatedAt:     nil, // Set if available
	}, nil
}

// GetBalance returns user balance
func (h *WalletHandler) GetBalance(ctx context.Context, req *pb.GetBalanceRequest) (*pb.GetBalanceResponse, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	userID, err := uuid.Parse(req.UserId)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid user_id format")
	}

	balance, err := h.walletService.GetBalance(ctx, userID, req.Asset)
	if err != nil {
		return nil, h.handleError(err)
	}

	return &pb.GetBalanceResponse{
		Asset:     balance.Asset,
		Available: balance.Available.String(),
		Locked:    balance.Locked.String(),
		Total:     balance.Total.String(),
		UpdatedAt: nil, // Set if available
	}, nil
}

// GetDepositAddress returns or creates a deposit address
func (h *WalletHandler) GetDepositAddress(ctx context.Context, req *pb.GetDepositAddressRequest) (*pb.GetDepositAddressResponse, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}
	if req.Asset == "" {
		return nil, status.Error(codes.InvalidArgument, "asset is required")
	}
	if req.Network == "" {
		return nil, status.Error(codes.InvalidArgument, "network is required")
	}

	userID, err := uuid.Parse(req.UserId)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid user_id format")
	}

	address, err := h.walletService.GetDepositAddress(ctx, userID, req.Asset, req.Network)
	if err != nil {
		return nil, h.handleError(err)
	}

	return &pb.GetDepositAddressResponse{
		Address:   address.Address,
		QrCode:    "", // Not available in DepositAddress, set empty or map if available
		Tag:       address.Tag,
		Network:   address.Network,
		CreatedAt: nil, // Set if available
	}, nil
}

// ListTransactions returns user transactions
func (h *WalletHandler) ListTransactions(ctx context.Context, req *pb.ListTransactionsRequest) (*pb.ListTransactionsResponse, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	userID, err := uuid.Parse(req.UserId)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid user_id format")
	}

	// Set defaults
	limit := int(req.Limit)
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	offset := int(req.Offset)
	if offset < 0 {
		offset = 0
	}

	transactions, total, err := h.walletService.ListTransactions(ctx, userID, req.Asset, req.Direction, limit, offset)
	if err != nil {
		return nil, h.handleError(err)
	}

	pbTransactions := make([]*pb.GetTransactionResponse, len(transactions))
	for i, tx := range transactions {
		pbTransactions[i] = &pb.GetTransactionResponse{
			Id:            tx.ID.String(),
			UserId:        tx.UserID.String(),
			Asset:         tx.Asset,
			Amount:        tx.Amount.String(),
			Direction:     string(tx.Direction),
			Status:        string(tx.Status),
			TxHash:        tx.TxHash,
			FromAddress:   tx.FromAddress,
			ToAddress:     tx.ToAddress,
			Network:       tx.Network,
			Confirmations: int32(tx.Confirmations),
			CreatedAt:     nil, // Set if available
			UpdatedAt:     nil, // Set if available
		}
	}

	return &pb.ListTransactionsResponse{
		Transactions: pbTransactions,
		Total:        int32(total),
	}, nil
}

// ValidateAddress validates a withdrawal address
func (h *WalletHandler) ValidateAddress(ctx context.Context, req *pb.ValidateAddressRequest) (*pb.ValidateAddressResponse, error) {
	if req.Address == "" {
		return nil, status.Error(codes.InvalidArgument, "address is required")
	}
	if req.Asset == "" {
		return nil, status.Error(codes.InvalidArgument, "asset is required")
	}
	if req.Network == "" {
		return nil, status.Error(codes.InvalidArgument, "network is required")
	}

	validationReq := interfaces.AddressValidationRequest{
		Address: req.Address,
		Asset:   req.Asset,
		Network: req.Network,
	}

	result, err := h.walletService.ValidateAddress(ctx, &validationReq)
	if err != nil {
		return nil, h.handleError(err)
	}

	return &pb.ValidateAddressResponse{
		Valid:   result.Valid,
		Format:  result.Format,
		Network: result.Network,
		// Type and Message omitted (not present in result)
	}, nil
}

// handleError converts internal errors to gRPC status errors
func (h *WalletHandler) handleError(err error) error {
	switch {
	case errors.Is(err, interfaces.ErrInsufficientBalance):
		return status.Error(codes.FailedPrecondition, "insufficient balance")
	case errors.Is(err, interfaces.ErrInvalidAmount):
		return status.Error(codes.InvalidArgument, "invalid amount")
	case errors.Is(err, interfaces.ErrInvalidAddress):
		return status.Error(codes.InvalidArgument, "invalid address")
	case errors.Is(err, interfaces.ErrTransactionNotFound):
		return status.Error(codes.NotFound, "transaction not found")
	case errors.Is(err, interfaces.ErrUserNotFound):
		return status.Error(codes.NotFound, "user not found")
	case errors.Is(err, interfaces.ErrComplianceRejected):
		return status.Error(codes.PermissionDenied, "compliance check failed")
	case errors.Is(err, interfaces.ErrRateLimited):
		return status.Error(codes.ResourceExhausted, "rate limit exceeded")
	case errors.Is(err, interfaces.ErrServiceUnavailable):
		return status.Error(codes.Unavailable, "service temporarily unavailable")
	default:
		return status.Error(codes.Internal, fmt.Sprintf("internal error: %v", err))
	}
}
