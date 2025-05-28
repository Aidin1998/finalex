package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"time"

	"github.com/litebittech/cex/common/cfg"
	"github.com/litebittech/cex/common/logger"
	"github.com/litebittech/cex/common/otel"
	"github.com/litebittech/cex/services/bookkeeper"
	"github.com/litebittech/cex/services/bookkeeper/api"
	"github.com/litebittech/cex/services/bookkeeper/cache"
	"github.com/litebittech/cex/services/bookkeeper/txstore"
)

var (
	config          = cfg.MustLoad[Config]()
	log, syncLogger = logger.New(!config.IsDev)
)

func initializeBookKeeper(ctx context.Context) *bookkeeper.BookKeeper {
	redisClient := NewRedis(config.RedisURL)

	balanceCache := cache.NewBalanceCache(redisClient, time.Hour*24)

	dynamoClient := NewDynamoDB(ctx, config.DynamoDB.Region, config.DynamoDB.Endpoint, log)
	txStore := txstore.NewStore(dynamoClient, config.DynamoDB.TableName)

	return bookkeeper.New(
		txStore,
		balanceCache,
	)
}

func main() {
	defer syncLogger()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	otelShutdown, err := otel.Setup(ctx, config.Otel)
	if err != nil {
		panic(err)
	}
	defer func() {
		err = errors.Join(err, otelShutdown(context.Background()))
	}()

	bookkeeper := initializeBookKeeper(ctx)
	apiService := api.New(log.With("component", "api"), bookkeeper)

	listenAddress := config.API.ListenAddress
	if listenAddress == "" {
		listenAddress = "localhost:3000"
	}

	if err := apiService.Serve(ctx, listenAddress); err != nil {
		panic(err)
	}
}
