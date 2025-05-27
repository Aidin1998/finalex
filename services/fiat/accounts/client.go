// client.go: gRPC/REST calls into Wallet service
package accounts

import (
	"context"
)

type Client struct {
	BaseURL string
}

type WalletOpRequest struct {
	UserID         string
	Currency       string
	Amount         float64
	IdempotencyKey string
}

func (c *Client) Debit(ctx context.Context, req WalletOpRequest) error {
	// TODO: Call REST/gRPC to debit wallet atomically
	// Example: POST /wallet/debit {user_id, currency, amount, idempotency_key}
	return nil
}

func (c *Client) Credit(ctx context.Context, req WalletOpRequest) error {
	// TODO: Call REST/gRPC to credit wallet atomically
	// Example: POST /wallet/credit {user_id, currency, amount, idempotency_key}
	return nil
}
