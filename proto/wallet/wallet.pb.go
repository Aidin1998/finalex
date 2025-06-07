// Package wallet provides gRPC types for wallet service
package wallet

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// WalletServiceServer is the server API for WalletService service.
type WalletServiceServer interface {
	// Deposit operations
	RequestDeposit(context.Context, *DepositRequest) (*DepositResponse, error)
	GetDepositAddress(context.Context, *GetDepositAddressRequest) (*GetDepositAddressResponse, error)

	// Withdrawal operations
	RequestWithdrawal(context.Context, *WithdrawalRequest) (*WithdrawalResponse, error)
	CancelWithdrawal(context.Context, *CancelWithdrawalRequest) (*CancelWithdrawalResponse, error)

	// Balance operations
	GetBalance(context.Context, *GetBalanceRequest) (*GetBalanceResponse, error)
	GetBalances(context.Context, *GetBalancesRequest) (*GetBalancesResponse, error)

	// Transaction operations
	GetTransaction(context.Context, *GetTransactionRequest) (*GetTransactionResponse, error)
	GetTransactions(context.Context, *GetTransactionsRequest) (*GetTransactionsResponse, error)
}

// UnimplementedWalletServiceServer can be embedded to have forward compatible implementations.
type UnimplementedWalletServiceServer struct{}

func (UnimplementedWalletServiceServer) RequestDeposit(context.Context, *DepositRequest) (*DepositResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method RequestDeposit not implemented")
}

func (UnimplementedWalletServiceServer) GetDepositAddress(context.Context, *GetDepositAddressRequest) (*GetDepositAddressResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetDepositAddress not implemented")
}

func (UnimplementedWalletServiceServer) RequestWithdrawal(context.Context, *WithdrawalRequest) (*WithdrawalResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method RequestWithdrawal not implemented")
}

func (UnimplementedWalletServiceServer) CancelWithdrawal(context.Context, *CancelWithdrawalRequest) (*CancelWithdrawalResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method CancelWithdrawal not implemented")
}

func (UnimplementedWalletServiceServer) GetBalance(context.Context, *GetBalanceRequest) (*GetBalanceResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetBalance not implemented")
}

func (UnimplementedWalletServiceServer) GetBalances(context.Context, *GetBalancesRequest) (*GetBalancesResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetBalances not implemented")
}

func (UnimplementedWalletServiceServer) GetTransaction(context.Context, *GetTransactionRequest) (*GetTransactionResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetTransaction not implemented")
}

func (UnimplementedWalletServiceServer) GetTransactions(context.Context, *GetTransactionsRequest) (*GetTransactionsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetTransactions not implemented")
}

// Request/Response message types
type DepositRequest struct {
	UserId  string `json:"user_id"`
	Asset   string `json:"asset"`
	Network string `json:"network"`
}

type DepositResponse struct {
	TransactionId string                 `json:"transaction_id"`
	Address       string                 `json:"address"`
	QrCode        string                 `json:"qr_code"`
	ExpiresAt     *timestamppb.Timestamp `json:"expires_at"`
}

type GetDepositAddressRequest struct {
	UserId  string `json:"user_id"`
	Asset   string `json:"asset"`
	Network string `json:"network"`
}

type GetDepositAddressResponse struct {
	Address   string                 `json:"address"`
	QrCode    string                 `json:"qr_code"`
	Tag       string                 `json:"tag"`
	Network   string                 `json:"network"`
	CreatedAt *timestamppb.Timestamp `json:"created_at"`
}

type WithdrawalRequest struct {
	UserId  string `json:"user_id"`
	Asset   string `json:"asset"`
	Amount  string `json:"amount"`
	Address string `json:"address"`
	Tag     string `json:"tag"`
	Network string `json:"network"`
}

type WithdrawalResponse struct {
	TransactionId string `json:"transaction_id"`
	Status        string `json:"status"`
	EstimatedTime string `json:"estimated_time"`
	Fee           string `json:"fee"`
	NetAmount     string `json:"net_amount"`
}

type CancelWithdrawalRequest struct {
	TransactionId string `json:"transaction_id"`
	Reason        string `json:"reason"`
}

type CancelWithdrawalResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type GetBalanceRequest struct {
	UserId string `json:"user_id"`
	Asset  string `json:"asset"`
}

type GetBalanceResponse struct {
	Asset     string                 `json:"asset"`
	Available string                 `json:"available"`
	Locked    string                 `json:"locked"`
	Total     string                 `json:"total"`
	UpdatedAt *timestamppb.Timestamp `json:"updated_at"`
}

type GetBalancesRequest struct {
	UserId string `json:"user_id"`
}

type GetBalancesResponse struct {
	Balances []*GetBalanceResponse `json:"balances"`
}

type GetTransactionRequest struct {
	TransactionId string `json:"transaction_id"`
}

type GetTransactionResponse struct {
	Id            string                 `json:"id"`
	UserId        string                 `json:"user_id"`
	Asset         string                 `json:"asset"`
	Amount        string                 `json:"amount"`
	Direction     string                 `json:"direction"`
	Status        string                 `json:"status"`
	TxHash        string                 `json:"tx_hash"`
	FromAddress   string                 `json:"from_address"`
	ToAddress     string                 `json:"to_address"`
	Network       string                 `json:"network"`
	Confirmations int32                  `json:"confirmations"`
	CreatedAt     *timestamppb.Timestamp `json:"created_at"`
	UpdatedAt     *timestamppb.Timestamp `json:"updated_at"`
}

type GetTransactionsRequest struct {
	UserId    string `json:"user_id"`
	Asset     string `json:"asset"`
	Direction string `json:"direction"`
	Status    string `json:"status"`
	Limit     int32  `json:"limit"`
	Offset    int32  `json:"offset"`
}

type GetTransactionsResponse struct {
	Transactions []*GetTransactionResponse `json:"transactions"`
	Total        int32                     `json:"total"`
}

// Missing request/response types

type RequestDepositRequest struct {
	UserId          string `json:"user_id"`
	Asset           string `json:"asset"`
	Network         string `json:"network"`
	GenerateAddress bool   `json:"generate_address"`
}

type RequestDepositResponse struct {
	TransactionId string                 `json:"transaction_id"`
	Status        string                 `json:"status"`
	Address       string                 `json:"address"`
	QrCode        string                 `json:"qr_code"`
	MinDeposit    string                 `json:"min_deposit"`
	CreatedAt     *timestamppb.Timestamp `json:"created_at"`
}

type RequestWithdrawalRequest struct {
	UserId         string `json:"user_id"`
	Asset          string `json:"asset"`
	Amount         string `json:"amount"`
	ToAddress      string `json:"to_address"`
	Network        string `json:"network"`
	Tag            string `json:"tag"`
	TwoFactorToken string `json:"two_factor_token"`
	Priority       string `json:"priority"`
	Note           string `json:"note"`
}

type RequestWithdrawalResponse struct {
	TransactionId string `json:"transaction_id"`
	Status        string `json:"status"`
	EstimatedTime string `json:"estimated_time"`
	Fee           string `json:"fee"`
	NetAmount     string `json:"net_amount"`
}

type GetTransactionStatusRequest struct {
	TransactionId string `json:"transaction_id"`
}

type GetTransactionStatusResponse struct {
	TransactionId string                 `json:"transaction_id"`
	Status        string                 `json:"status"`
	Confirmations int32                  `json:"confirmations"`
	UpdatedAt     *timestamppb.Timestamp `json:"updated_at"`
}

type ListTransactionsRequest struct {
	UserId    string `json:"user_id"`
	Asset     string `json:"asset"`
	Direction string `json:"direction"`
	Limit     int32  `json:"limit"`
	Offset    int32  `json:"offset"`
}

type ListTransactionsResponse struct {
	Transactions []*GetTransactionResponse `json:"transactions"`
	Total        int32                     `json:"total"`
}

type ValidateAddressRequest struct {
	Address string `json:"address"`
	Asset   string `json:"asset"`
	Network string `json:"network"`
}

type ValidateAddressResponse struct {
	Valid   bool   `json:"valid"`
	Format  string `json:"format"`
	Network string `json:"network"`
	Type    string `json:"type"`
	Message string `json:"message"`
}

// RegisterWalletServiceServer registers the server
func RegisterWalletServiceServer(s grpc.ServiceRegistrar, srv WalletServiceServer) {
	// Implementation stub
}
