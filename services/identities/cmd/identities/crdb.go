package main

import (
	"log/slog"

	sloggorm "github.com/orandin/slog-gorm"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// CockrochDB
func CRDB(dsn string, logHandler slog.Handler) *gorm.DB {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: sloggorm.New(sloggorm.WithHandler(logHandler)),
	})
	if err != nil {
		panic("failed to connect to database: " + err.Error())
	}

	return db
}
