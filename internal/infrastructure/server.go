package infrastructure

import (
	"github.com/Aidin1998/finalex/internal/infrastructure/server"
)

// Server is an alias for the main server.Server implementation.
type Server = server.Server

// NewServer delegates to the main server package.
var NewServer = server.NewServer
