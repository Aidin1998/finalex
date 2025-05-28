// Deprecated: Logic is now in internal/marketfeeds/service.go
package aggregator

import (
	"database/sql"
	"time"

	_ "github.com/lib/pq"
)

type DBWriter struct {
	db *sql.DB
}

func NewDBWriter(connStr string) (*DBWriter, error) {
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, err
	}
	return &DBWriter{db: db}, nil
}

func (w *DBWriter) InsertPrice(exchange, symbol string, price float64, receivedAt time.Time) error {
	_, err := w.db.Exec(
		`INSERT INTO prices (exchange, symbol, price, received_at) VALUES ($1, $2, $3, $4)`,
		exchange, symbol, price, receivedAt,
	)
	return err
}

func (w *DBWriter) Migrate() error {
	_, err := w.db.Exec(`
		CREATE TABLE IF NOT EXISTS prices (
			id           UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
			exchange     STRING        NOT NULL,
			symbol       STRING        NOT NULL,
			price        DECIMAL(30,10) NOT NULL,
			received_at  TIMESTAMPTZ   NOT NULL DEFAULT now()
		);
	`)
	return err
}
