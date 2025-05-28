package main

import (
	pkgauth "github.com/litebittech/cex/common/auth"
	"github.com/litebittech/cex/common/otel"
	"github.com/litebittech/cex/services/identities/auth"
)

type Config struct {
	IsDev bool

	CockroachDB struct {
		DSN string
	}

	Auth0 auth.Auth0Config

	Authorization pkgauth.AuthorizationConfig

	CaptchaEnabled bool

	API struct {
		ListenAddress string
	}

	Otel otel.Config
}
