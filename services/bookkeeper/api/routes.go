package api

import (
	"github.com/labstack/echo/v4"
)

// registerAuthorizedRoutes registers all authorized routes
func (e *endpoints) registerRoutes(g *echo.Group) {
	transactions := g.Group("/transactions")
	transactions.POST("/", e.CreateTransaction)
	transactions.GET("/balance", e.GetBalance)

	accounts := g.Group("/accounts")
	accounts.POST("/", e.CreateAccount)
}
