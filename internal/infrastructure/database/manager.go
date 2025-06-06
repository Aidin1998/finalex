package database

import (
	"context"

	"gorm.io/gorm"
)

type Connection interface {
	DB() *gorm.DB
	Close() error
	Ping(ctx context.Context) error
}

type Manager struct {
	db *gorm.DB
}

func NewManager(db *gorm.DB) *Manager {
	return &Manager{db: db}
}

func (m *Manager) DB() *gorm.DB {
	return m.db
}

func (m *Manager) Close() error {
	sqlDB, err := m.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func (m *Manager) Ping(ctx context.Context) error {
	sqlDB, err := m.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}
