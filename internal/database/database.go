// Package database provides database utilities and interfaces
package database

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// DB represents a database connection interface
type DB interface {
	Create(value interface{}) error
	Find(dest interface{}, conditions ...interface{}) error
	Update(values interface{}) error
	Delete(value interface{}, conditions ...interface{}) error
	Raw(sql string, values ...interface{}) *gorm.DB
	Transaction(fc func(*gorm.DB) error) error
}

// DatabaseConnection wraps a GORM database connection
type DatabaseConnection struct {
	*gorm.DB
}

// NewDatabaseConnection creates a new database connection wrapper
func NewDatabaseConnection(db *gorm.DB) *DatabaseConnection {
	return &DatabaseConnection{DB: db}
}

// TransactionModel represents a basic transaction record
type TransactionModel struct {
	ID        uuid.UUID       `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id"`
	UserID    uuid.UUID       `gorm:"type:uuid;not null" json:"user_id"`
	Amount    decimal.Decimal `gorm:"type:decimal(20,8);not null" json:"amount"`
	Currency  string          `gorm:"type:varchar(10);not null" json:"currency"`
	Type      string          `gorm:"type:varchar(50);not null" json:"type"`
	Status    string          `gorm:"type:varchar(20);not null" json:"status"`
	CreatedAt time.Time       `gorm:"default:current_timestamp" json:"created_at"`
	UpdatedAt time.Time       `gorm:"default:current_timestamp" json:"updated_at"`
}

// OrderModel represents a basic order record
type OrderModel struct {
	ID        uuid.UUID       `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id"`
	UserID    uuid.UUID       `gorm:"type:uuid;not null" json:"user_id"`
	Symbol    string          `gorm:"type:varchar(20);not null" json:"symbol"`
	Side      string          `gorm:"type:varchar(10);not null" json:"side"`
	Type      string          `gorm:"type:varchar(20);not null" json:"type"`
	Quantity  decimal.Decimal `gorm:"type:decimal(20,8);not null" json:"quantity"`
	Price     decimal.Decimal `gorm:"type:decimal(20,8)" json:"price"`
	Status    string          `gorm:"type:varchar(20);not null" json:"status"`
	CreatedAt time.Time       `gorm:"default:current_timestamp" json:"created_at"`
	UpdatedAt time.Time       `gorm:"default:current_timestamp" json:"updated_at"`
}

// Repository provides common database operations
type Repository struct {
	db *gorm.DB
}

// NewRepository creates a new repository instance
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// WithContext returns repository with context
func (r *Repository) WithContext(ctx context.Context) *Repository {
	return &Repository{db: r.db.WithContext(ctx)}
}

// GetDB returns the underlying database connection
func (r *Repository) GetDB() *gorm.DB {
	return r.db
}
