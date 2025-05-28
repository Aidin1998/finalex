package api

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/shopspring/decimal"

	"github.com/litebittech/cex/services/bookkeeper/txstore"
	"github.com/litebittech/cex/services/bookkeeper/types"
)

// CreateTransactionRequest represents the request body for creating a transaction
type CreateTransactionRequest struct {
	UserID      uuid.UUID               `json:"userId" validate:"required"`
	Type        txstore.TransactionType `json:"type" validate:"required"`
	Date        time.Time               `json:"date" validate:"required"`
	Description string                  `json:"description" validate:"required"`
	Entries     []EntryRequest          `json:"entries" validate:"required,min=1"`
	Metadata    map[string]interface{}  `json:"metadata,omitempty"`
}

// EntryRequest represents an entry in a transaction request
type EntryRequest struct {
	AccountID       uuid.UUID         `json:"accountId" validate:"required"`
	IsSystemAccount bool              `json:"isSystemAccount"`
	Type            txstore.EntryType `json:"type" validate:"required"`
	Amount          decimal.Decimal   `json:"amount" validate:"required"`
	Currency        string            `json:"currency" validate:"required"`
}

// CreateTransaction handles creating a new transaction
// @Summary Create a new transaction
// @Description Create a new transaction with entries
// @Tags transactions
// @Accept json
// @Produce json
// @Param request body CreateTransactionRequest true "Transaction request"
// @Success 201 {object} string "Transaction created"
// @Failure 400 {object} errors.Error "Invalid request"
// @Failure 401 {object} errors.Error "Unauthorized"
// @Failure 500 {object} errors.Error "Server error"
// @Router /transactions [post]
func (e *endpoints) CreateTransaction(c echo.Context) error {
	var req CreateTransactionRequest
	if err := c.Bind(&req); err != nil {
		c.Error(err)
		return nil
	}

	if err := c.Validate(&req); err != nil {
		c.Error(err)
		return nil
	}

	// Create transaction using a session
	session := e.bookkeeper.NewSession()

	// Convert to txstore.Transaction
	transaction := &txstore.Transaction{
		UserID:      txstore.NewUUID(req.UserID), // Assuming user_id is in context
		Type:        req.Type,
		Date:        txstore.NewUnixMilli(req.Date),
		Description: req.Description,
		Metadata:    req.Metadata,
	}

	// Convert entries
	txEntries := make([]txstore.Entry, len(req.Entries))
	for i, entry := range req.Entries {
		txEntries[i] = txstore.Entry{
			AccountID:       txstore.UUID{UUID: entry.AccountID},
			IsSystemAccount: entry.IsSystemAccount,
			Type:            entry.Type,
			Amount:          entry.Amount,
			Currency:        entry.Currency,
		}
	}
	transaction.Entries = txstore.NewEntries(txEntries)

	if err := session.CreateTransaction(c.Request().Context(), transaction); err != nil {
		return err
	}

	return c.NoContent(http.StatusCreated)
}

// GetBalanceRequest represents the query parameters for getting a balance
type GetBalanceRequest struct {
	UserID    uuid.UUID `query:"userId" validate:"required"`
	AccountID uuid.UUID `query:"accountId" validate:"required"`
	Currency  string    `query:"currency" validate:"required"`
}

// BalanceResponse represents the response for a balance query
type BalanceResponse struct {
	UserID    uuid.UUID       `json:"userId"`
	AccountID uuid.UUID       `json:"accountId"`
	Currency  string          `json:"currency"`
	Amount    decimal.Decimal `json:"amount"`
}

// GetBalance handles retrieving a balance for a specific user, account, and currency
// @Summary Get account balance
// @Description Get the balance for a specific user, account, and currency
// @Tags transactions
// @Accept json
// @Produce json
// @Param userId query string true "User ID"
// @Param accountId query string true "Account ID"
// @Param currency query string true "Currency code"
// @Success 200 {object} BalanceResponse "Balance information"
// @Failure 400 {object} errors.Error "Invalid request"
// @Failure 404 {object} errors.Error "Balance not found"
// @Failure 500 {object} errors.Error "Server error"
// @Router /transactions/balance [get]
func (e *endpoints) GetBalance(c echo.Context) error {
	var req GetBalanceRequest
	if err := c.Bind(&req); err != nil {
		c.Error(err)
		return nil
	}

	if err := c.Validate(&req); err != nil {
		c.Error(err)
		return nil
	}

	// Create a session to get the balance
	session := e.bookkeeper.NewSession()

	// Create the balance key
	balanceKey := types.BalanceKey{
		UserID:    req.UserID,
		AccountID: req.AccountID,
		Currency:  req.Currency,
	}

	// Get the balance
	balance, err := session.GetBalance(c.Request().Context(), balanceKey)
	if err != nil {
		return err
	}

	// If balance is nil, return zero
	amount := decimal.Zero
	if balance != nil {
		amount = *balance
	}

	// Return the balance response
	return c.JSON(http.StatusOK, BalanceResponse{
		UserID:    req.UserID,
		AccountID: req.AccountID,
		Currency:  req.Currency,
		Amount:    amount,
	})
}
