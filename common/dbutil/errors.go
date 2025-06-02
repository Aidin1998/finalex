package dbutil

import (
	"github.com/Aidin1998/pincex_unified/common/errors"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

const DuplicateKeyErrorCode = "23505"

// WrapError wraps a gorm error.
func WrapError(err error) error {
	var pgErr *pgconn.PgError

	if err == nil {
		return nil
	} else if _, ok := err.(*errors.Error); ok {
		return err
	} else if errors.Is(err, gorm.ErrRecordNotFound) {
		return errors.NotFound
	} else if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case DuplicateKeyErrorCode:
			return errors.Conflict.
				Explain("duplication of key").
				Wrap(err)
		}
	}

	return err
}
