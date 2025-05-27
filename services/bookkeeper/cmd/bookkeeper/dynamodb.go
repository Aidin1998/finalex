package main

import (
	"context"
	"fmt"
	"log/slog"

	dynamocfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

// NewDynamoDB initializes and returns a NewDynamoDB client
func NewDynamoDB(ctx context.Context, region string, endpoint string, logger *slog.Logger) *dynamodb.Client {
	cfg, err := dynamocfg.LoadDefaultConfig(ctx,
		dynamocfg.WithBaseEndpoint(endpoint),
		dynamocfg.WithRegion(region),
	)
	if err != nil {
		panic(fmt.Sprintf("failed to load config: %v", err))
	}

	db := dynamodb.NewFromConfig(cfg)

	logger.Info("DynamoDB client initialized", "region", region, "endpoint", endpoint)

	return db
}
