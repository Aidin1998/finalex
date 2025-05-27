// service.go: core convert & fee-calc logic
package exchange

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/yourorg/fiat/accounts"
	"github.com/yourorg/fiat/internal/store"
	"github.com/yourorg/fiat/rates"
)

type SpotTickerClient interface {
	GetLastPrice(symbol string) (float64, error)
}

type ExchangeService struct {
	Aggregator *rates.Aggregator
	SpotClient SpotTickerClient
	FeePercent float64
}

func (s *ExchangeService) ConvertFiatToCrypto(fromCurrency, toSymbol string, amount float64) (float64, float64, error) {
	// e.g. fromCurrency = "USD", toSymbol = "BTC"
	rate, err := s.Aggregator.GetAvgRate(fromCurrency)
	if err != nil || rate == 0 {
		return 0, 0, fmt.Errorf("no fiat rate")
	}
	// Get USDT→BTC price from spot
	lastPrice, err := s.SpotClient.GetLastPrice(toSymbol + "USDT")
	if err != nil || lastPrice == 0 {
		return 0, 0, fmt.Errorf("no spot price")
	}
	// Convert USD→USDT, then USDT→BTC
	gross := amount * rate / lastPrice
	net := gross * (1 - s.FeePercent)
	return roundToPrecision(net, 8), lastPrice, nil
}

func (s *ExchangeService) ConvertFiatToFiat(fromCurrency, toCurrency string, amount float64) (float64, float64, error) {
	fromRate, err := s.Aggregator.GetAvgRate(fromCurrency)
	if err != nil || fromRate == 0 {
		return 0, 0, fmt.Errorf("no from rate")
	}
	toRate, err := s.Aggregator.GetAvgRate(toCurrency)
	if err != nil || toRate == 0 {
		return 0, 0, fmt.Errorf("no to rate")
	}
	gross := amount * fromRate / toRate
	net := gross * (1 - s.FeePercent)
	return roundToPrecision(net, 2), toRate, nil
}

func roundToPrecision(val float64, prec int) float64 {
	factor := math.Pow(10, float64(prec))
	return math.Round(val*factor) / factor
}

// --- SpotTickerClient implementation using HTTP ---
type HttpSpotClient struct {
	BaseURL string
}

func (c *HttpSpotClient) GetLastPrice(symbol string) (float64, error) {
	url := fmt.Sprintf("%s/api/spot/market/ticker?symbol=%s", c.BaseURL, symbol)
	resp, err := http.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	var data struct {
		LastPrice string `json:"lastPrice"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return 0, err
	}
	var price float64
	fmt.Sscanf(data.LastPrice, "%f", &price)
	return price, nil
}

type AtomicExchangeService struct {
	ExchangeService
	AccountsClient *accounts.Client
	LogRepo        *store.ExchangeLogRepo
	DB             *pgxpool.Pool
}

func (s *AtomicExchangeService) ExchangeAtomic(ctx context.Context, fromUser, toUser, fromCurrency, toCurrency string, amount float64, idempotencyKey string) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// 1. Debit fromWallet
	if err := s.AccountsClient.Debit(ctx, accounts.WalletOpRequest{
		UserID: fromUser, Currency: fromCurrency, Amount: amount, IdempotencyKey: idempotencyKey + ":debit",
	}); err != nil {
		return fmt.Errorf("debit failed: %w", err)
	}

	// 2. Calculate conversion
	net, rate, err := s.ConvertFiatToFiat(fromCurrency, toCurrency, amount)
	if err != nil {
		// Compensate: credit back if needed
		s.AccountsClient.Credit(ctx, accounts.WalletOpRequest{
			UserID: fromUser, Currency: fromCurrency, Amount: amount, IdempotencyKey: idempotencyKey + ":compensate",
		})
		return fmt.Errorf("conversion failed: %w", err)
	}

	// 3. Credit toWallet
	if err := s.AccountsClient.Credit(ctx, accounts.WalletOpRequest{
		UserID: toUser, Currency: toCurrency, Amount: net, IdempotencyKey: idempotencyKey + ":credit",
	}); err != nil {
		// Compensate: credit back to fromWallet
		s.AccountsClient.Credit(ctx, accounts.WalletOpRequest{
			UserID: fromUser, Currency: fromCurrency, Amount: amount, IdempotencyKey: idempotencyKey + ":compensate",
		})
		return fmt.Errorf("credit failed: %w", err)
	}

	// 4. Insert exchange log
	exLog := &store.ExchangeLog{
		ID:              uuid.NewString(),
		UserID:          fromUser,
		FromCurrency:    fromCurrency,
		ToCurrency:      toCurrency,
		Amount:          amount,
		AmountConverted: net,
		Rate:            rate,
		Fee:             s.FeePercent,
		Status:          "completed",
	}
	if err := s.LogRepo.InsertTx(ctx, tx, exLog); err != nil {
		return fmt.Errorf("log insert failed: %w", err)
	}

	return tx.Commit(ctx)
}
