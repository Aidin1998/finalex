package api

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/litebittech/cex/common/apiutil"
	"github.com/litebittech/cex/services/bookkeeper"
	"github.com/litebittech/cex/services/bookkeeper/store"
	"github.com/shopspring/decimal"
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
)

type endpoints struct {
	log        *slog.Logger
	bookkeeper *bookkeeper.BookKeeper
}

// TransactionIn represents input for creating a transaction
type TransactionIn struct {
	AccountID   uuid.UUID              `json:"account_id"`
	Type        store.TransactionType  `json:"type"`
	Date        time.Time              `json:"date"`
	Description string                 `json:"description"`
	Entries     []EntryIn              `json:"entries"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// EntryIn represents an entry in a transaction input
type EntryIn struct {
	AccountID       uuid.UUID       `json:"account_id"`
	IsSystemAccount bool            `json:"is_system_account"`
	Type            store.EntryType `json:"type"`
	Amount          decimal.Decimal `json:"amount"`
	Currency        string          `json:"currency"`
}

func New(log *slog.Logger, bookkeeper *bookkeeper.BookKeeper) *Server {
	return &Server{
		log,
		&endpoints{log, bookkeeper},
	}
}

type Server struct {
	log       *slog.Logger
	endpoints *endpoints
}

func (s *Server) Serve(ctx context.Context, listenAddr string) error {
	e := echo.New()

	e.HideBanner = true
	e.HidePort = true
	e.HTTPErrorHandler = apiutil.ErrorHandler()
	e.Use(apiutil.LoggerMiddleware(s.log))
	e.Use(otelecho.Middleware("bookkeeper"))
	e.Validator = apiutil.NewValidator()

	// Register public routes
	v1 := e.Group("/api/v1")
	s.endpoints.registerRoutes(v1)

	go func() {
		<-ctx.Done()
		e.Shutdown(ctx)
	}()

	s.log.Info("http server listening on " + listenAddr)
	return e.Start(listenAddr)
}
