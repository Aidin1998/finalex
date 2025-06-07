// Package services provides a stub FireblocksClient implementation
package services

import (
	"context"

	"github.com/Aidin1998/finalex/internal/wallet/interfaces"
)

type FireblocksClientImpl struct{}

func NewFireblocksClient(baseURL, apiKey, privateKeyPath, vaultAccountID string, log interface{}) (interfaces.FireblocksClient, error) {
	return &FireblocksClientImpl{}, nil
}

func (f *FireblocksClientImpl) HealthCheck(ctx context.Context) error {
	return nil
}

func (f *FireblocksClientImpl) CancelTransaction(ctx context.Context, txID string) (*interfaces.FireblocksTransaction, error) {
	return nil, nil
}

func (f *FireblocksClientImpl) CreateTransaction(ctx context.Context, req *interfaces.FireblocksTransactionRequest) (*interfaces.FireblocksTransaction, error) {
	return nil, nil
}

func (f *FireblocksClientImpl) GenerateAddress(ctx context.Context, req *interfaces.FireblocksAddressRequest) (*interfaces.FireblocksAddress, error) {
	return nil, nil
}

func (f *FireblocksClientImpl) GenerateDepositAddress(ctx context.Context, asset, network string, params *interfaces.AddressParams) (*interfaces.FireblocksAddress, error) {
	return nil, nil
}

func (f *FireblocksClientImpl) GetAddresses(ctx context.Context, asset, network string) ([]*interfaces.FireblocksAddress, error) {
	return nil, nil
}
