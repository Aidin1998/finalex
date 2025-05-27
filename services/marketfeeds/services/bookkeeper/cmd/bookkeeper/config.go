package main

import (
	"github.com/litebittech/cex/common/otel"
)

type Config struct {
	IsDev bool

	RedisURL string

	DynamoDB struct {
		TableName string
		Region    string
		Endpoint  string
	}

	API struct {
		ListenAddress string
	}

	Otel otel.Config
}
