package stripe

import (
	"context"
	"os"

	"github.com/stripe/stripe-go/v75"
	"github.com/stripe/stripe-go/v75/paymentintent"
)

type Client struct {
	apiKey string
}

func NewClient() *Client {
	return &Client{apiKey: os.Getenv("STRIPE_KEY")}
}

func (c *Client) CreatePaymentIntent(ctx context.Context, amount int64, currency, userID, idempotencyKey string) (*stripe.PaymentIntent, error) {
	stripe.Key = c.apiKey
	params := &stripe.PaymentIntentParams{
		Amount:   stripe.Int64(amount),
		Currency: stripe.String(currency),
		Metadata: map[string]string{"user_id": userID},
	}
	params.SetIdempotencyKey(idempotencyKey)
	// Stripe Go SDK v75 does not support context, so we cannot pass ctx directly.
	return paymentintent.New(params)
}

func (c *Client) ConfirmPaymentIntent(ctx context.Context, intentID, idempotencyKey string) (*stripe.PaymentIntent, error) {
	stripe.Key = c.apiKey
	params := &stripe.PaymentIntentConfirmParams{}
	params.SetIdempotencyKey(idempotencyKey)
	// Stripe Go SDK v75 does not support context, so we cannot pass ctx directly.
	return paymentintent.Confirm(intentID, params)
}
