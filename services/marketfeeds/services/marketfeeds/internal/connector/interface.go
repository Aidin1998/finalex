package connector

import "context"

// ExchangeConfig holds configuration for a connector
// Extend fields as needed per exchange
type ExchangeConfig struct {
	Symbol     string
	WSURL      string
	APIKey     string
	Secret     string
	Passphrase string
}

// Connector defines the interface for exchange connectors
type Connector interface {
	Init(cfg ExchangeConfig) error
	Run(ctx context.Context)
	HealthCheck() (bool, string)
}

// ConnectorFactory creates a new Connector instance
type ConnectorFactory func() Connector
