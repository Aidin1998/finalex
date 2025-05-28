package store

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ExchangeLog struct {
	ID              string
	UserID          string
	FromCurrency    string
	ToCurrency      string
	Amount          float64
	AmountConverted float64
	Rate            float64
	Fee             float64
	Status          string
	CreatedAt       string
}

type ExchangeLogRepo struct {
	DB *pgxpool.Pool
}

func NewExchangeLogRepo(db *pgxpool.Pool) *ExchangeLogRepo {
	return &ExchangeLogRepo{DB: db}
}

func (r *ExchangeLogRepo) InsertTx(ctx context.Context, tx pgx.Tx, log *ExchangeLog) error {
	// ...insert exchange log in transaction...
	return nil
}
