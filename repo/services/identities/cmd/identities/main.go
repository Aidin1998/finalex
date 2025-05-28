package main

import (
	"context"
	"errors"
	"os"
	"os/signal"

	"github.com/litebittech/cex/common/cfg"
	"github.com/litebittech/cex/common/logger"
	"github.com/litebittech/cex/common/otel"
	"github.com/litebittech/cex/services/identities/api"
	"github.com/litebittech/cex/services/identities/auth"
	"github.com/litebittech/cex/services/identities/users"
	"github.com/litebittech/cex/services/identities/users/store"
)

var (
	config          = cfg.MustLoad[Config]()
	log, syncLogger = logger.New(!config.IsDev)
	db              = CRDB(config.CockroachDB.DSN, log.Handler())
)

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

	userStore := store.New(log, db)
	usersService := users.New(log, userStore)

	authService, err := auth.New(log, config.Auth0, usersService)
	if err != nil {
		log.Error("Failed to initialize auth service", "error", err)
		panic(err)
	}

	apiService := api.New(log, config.CaptchaEnabled, authService, usersService, config.Authorization)

	listenAddress := config.API.ListenAddress
	if listenAddress == "" {
		listenAddress = "localhost:3000"
	}

	if err := apiService.Serve(ctx, listenAddress); err != nil {
		panic(err)
	}
}
