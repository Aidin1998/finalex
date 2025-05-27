package api

import (
	"github.com/labstack/echo/v4"
)

// registerPublicRoutes registers all public routes
func (e *endpoints) registerPublicRoutes(g *echo.Group) {
	// Auth routes
	auth := g.Group("/auth")
	auth.POST("/signup/", e.Signup)
	auth.POST("/login/", e.Login)
	auth.GET("/authorize/", e.Authorize)
}

// registerAuthorizedRoutes registers all authorized routes
func (e *endpoints) registerAuthorizedRoutes(g *echo.Group) {
	g.GET("/users/me/", e.Me)
	g.PATCH("/users/me/names", e.UpdateNames)
}
