package store

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type FiatWallet struct {
	ID        string
	UserID    string
	Currency  string
	Balance   float64
	CreatedAt string
	UpdatedAt string
}

type FiatWalletRepo struct {
	DB *pgxpool.Pool
}

func NewFiatWalletRepo(db *pgxpool.Pool) *FiatWalletRepo {
	return &FiatWalletRepo{DB: db}
}

func (r *FiatWalletRepo) GetByUserID(ctx context.Context, userID, currency string) (*FiatWallet, error) {
	// ...fetch wallet by user and currency...
	return nil, nil
}

func (r *FiatWalletRepo) UpdateBalanceTx(ctx context.Context, tx pgx.Tx, userID, currency string, delta float64) error {
	// ...update balance in transaction...
	return nil
}

func (r *FiatWalletRepo) CreateIfNotExistsTx(ctx context.Context, tx pgx.Tx, userID, currency string) error {
	// ...create wallet if not exists in transaction...
	return nil
}
