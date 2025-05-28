package api

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/litebittech/cex/services/bookkeeper/store"
)

// CreateAccountRequest represents the request body for creating an account
type CreateAccountRequest struct {
	ID          uuid.UUID              `json:"id" validate:"required"`
	UserID      uuid.UUID              `json:"userId" validate:"required"`
	Description string                 `json:"description"`
	Type        string                 `json:"type" validate:"required"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// AccountResponse represents the response for account operations
type AccountResponse struct {
	ID          uuid.UUID              `json:"id"`
	UserID      uuid.UUID              `json:"userId"`
	Description string                 `json:"description"`
	Type        string                 `json:"type"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// CreateAccount handles creating a new account
// @Summary Create a new account
// @Description Create a new account for a user
// @Tags accounts
// @Accept json
// @Produce json
// @Param request body CreateAccountRequest true "Account request"
// @Success 201 {object} AccountResponse "Account created"
// @Failure 400 {object} errors.Error "Invalid request"
// @Failure 401 {object} errors.Error "Unauthorized"
// @Failure 409 {object} errors.Error "Account already exists"
// @Failure 500 {object} errors.Error "Server error"
// @Router /accounts [post]
func (e *endpoints) CreateAccount(c echo.Context) error {
	var req CreateAccountRequest
	if err := c.Bind(&req); err != nil {
		c.Error(err)
		return nil
	}

	if err := c.Validate(&req); err != nil {
		c.Error(err)
		return nil
	}

	account := &store.Account{
		ID:          store.UUID{UUID: req.ID},
		UserID:      store.UUID{UUID: req.UserID},
		Description: req.Description,
		Type:        req.Type,
		Metadata:    req.Metadata,
	}

	if err := e.bookkeeper.CreateAccount(c.Request().Context(), account); err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, AccountResponse{
		ID:          req.ID,
		UserID:      req.UserID,
		Description: account.Description,
		Type:        account.Type,
		Metadata:    account.Metadata,
	})
}
