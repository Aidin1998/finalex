package store

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestFiatWalletTx(t *testing.T) {
	// TODO: Setup test DB pool (use testcontainers or CockroachDB test instance)
	var db *pgxpool.Pool // = ...
	_ = NewFiatWalletRepo(db)
	// TODO: Begin tx, test CreateIfNotExistsTx, UpdateBalanceTx, commit/rollback
}

func TestExchangeLogTx(t *testing.T) {
	// TODO: Setup test DB pool
	var db *pgxpool.Pool // = ...
	_ = NewExchangeLogRepo(db)
	// TODO: Begin tx, test InsertTx, commit/rollback
}
