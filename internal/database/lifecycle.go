package database

import (
	"context"
	"time"

	"github.com/Aidin1998/pincex_unified/pkg/models"
	"gorm.io/gorm"
)

// ArchiveOrders moves orders older than the threshold from CockroachDB to PostgreSQL.
func ArchiveOrders(ctx context.Context, crDB, pgDB *gorm.DB, olderThan time.Duration) error {
	cutoff := time.Now().Add(-olderThan)
	// Load old orders
	var orders []models.Order
	if err := crDB.WithContext(ctx).
		Where("created_at < ?", cutoff).
		Find(&orders).Error; err != nil {
		return err
	}
	if len(orders) == 0 {
		return nil
	}
	// Begin transaction on PostgreSQL
	tx := pgDB.WithContext(ctx).Begin()
	for _, o := range orders {
		// Insert into Postgres
		if err := tx.Create(&o).Error; err != nil {
			tx.Rollback()
			return err
		}
	}
	// Commit Postgres
	if err := tx.Commit().Error; err != nil {
		return err
	}
	// Delete from Cockroach
	ids := make([]string, len(orders))
	for i, o := range orders {
		ids[i] = o.ID.String()
	}
	if err := crDB.WithContext(ctx).
		Where("id IN ?", ids).
		Delete(&models.Order{}).Error; err != nil {
		return err
	}
	return nil
}
