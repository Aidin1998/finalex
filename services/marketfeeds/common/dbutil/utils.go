package dbutil

import (
	"github.com/litebittech/cex/common/errors"
	"gorm.io/gorm"
)

func FindOne[T any](db *gorm.DB) (*T, error) {
	var item T
	result := db.Find(&item)
	if result.Error != nil {
		return &item, WrapError(result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, errors.NotFound
	}
	return &item, nil
}
