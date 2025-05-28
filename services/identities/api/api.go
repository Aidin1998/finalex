// Deprecated: Logic is now in internal/identities/service.go
package api

import (
	"context"
	"log/slog"

	"github.com/labstack/echo/v4"
	"github.com/litebittech/cex/common/apiutil"
	pkgauth "github.com/litebittech/cex/common/auth"
	"github.com/litebittech/cex/services/identities/auth"
	"github.com/litebittech/cex/services/identities/users"
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
)

type endpoints struct {
	log            *slog.Logger
	captchaEnabled bool
	auth           *auth.Auth
	users          *users.Users
}

func New(log *slog.Logger, captchaEnabled bool, auth *auth.Auth, users *users.Users, authorization pkgauth.AuthorizationConfig) *Server {
	return &Server{
		log,
		&endpoints{log, captchaEnabled, auth, users},
		authorization,
	}
}

type Server struct {
	log           *slog.Logger
	endpoints     *endpoints
	authorization pkgauth.AuthorizationConfig
}

func (s *Server) Serve(ctx context.Context, listenAddr string) error {
	e := echo.New()

	e.HideBanner = true
	e.HidePort = true
	e.HTTPErrorHandler = apiutil.ErrorHandler()
	e.Use(apiutil.LoggerMiddleware(s.log))
	e.Use(otelecho.Middleware("identities"))
	e.Validator = apiutil.NewValidator()

	// e.GET("/docs/*", echoSwagger.WrapHandler)

	// Register public routes
	v1 := e.Group("/api/v1")
	s.endpoints.registerPublicRoutes(v1)

	// Register authorized routes
	authorized := v1.Group("", pkgauth.Middleware(s.log, s.authorization))
	s.endpoints.registerAuthorizedRoutes(authorized)

	go func() {
		<-ctx.Done()
		e.Shutdown(ctx)
	}()

	s.log.Info("http server listening on " + listenAddr)
	return e.Start(listenAddr)
}
