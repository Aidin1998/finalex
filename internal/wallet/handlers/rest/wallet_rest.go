// Package rest provides REST API handlers for the wallet service
package rest

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/Aidin1998/finalex/internal/wallet/interfaces"
)

// WalletHandler handles REST API requests for wallet operations
type WalletHandler struct {
	walletService interfaces.WalletService
}

// NewWalletHandler creates a new wallet REST handler
func NewWalletHandler(walletService interfaces.WalletService) *WalletHandler {
	return &WalletHandler{
		walletService: walletService,
	}
}

// RegisterRoutes registers wallet routes with the Gin router
func (h *WalletHandler) RegisterRoutes(r *gin.RouterGroup) {
	wallet := r.Group("/wallet")
	{
		// Deposit operations
		wallet.POST("/deposit", h.RequestDeposit)
		wallet.GET("/deposit/address", h.GetDepositAddress)

		// Withdrawal operations
		wallet.POST("/withdrawal", h.RequestWithdrawal)

		// Transaction operations
		wallet.GET("/transaction/:id/status", h.GetTransactionStatus)
		wallet.GET("/transactions", h.ListTransactions)

		// Balance operations
		wallet.GET("/balance", h.GetBalance)

		// Address validation
		wallet.POST("/address/validate", h.ValidateAddress)

		// Health check
		wallet.GET("/health", h.HealthCheck)
	}
}

// RequestDepositRequest represents REST deposit request
type RequestDepositRequest struct {
	Asset           string `json:"asset" binding:"required"`
	Network         string `json:"network" binding:"required"`
	GenerateAddress bool   `json:"generate_address"`
}

// RequestDeposit handles deposit requests
func (h *WalletHandler) RequestDeposit(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req RequestDepositRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	uid, ok := userID.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	depositReq := interfaces.DepositRequest{
		UserID:          uid,
		Asset:           req.Asset,
		Network:         req.Network,
		GenerateAddress: req.GenerateAddress,
	}

	response, err := h.walletService.RequestDeposit(c.Request.Context(), depositReq)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, response)
}

// RequestWithdrawalRequest represents REST withdrawal request
type RequestWithdrawalRequest struct {
	Asset          string `json:"asset" binding:"required"`
	Amount         string `json:"amount" binding:"required"`
	ToAddress      string `json:"to_address" binding:"required"`
	Network        string `json:"network" binding:"required"`
	Tag            string `json:"tag,omitempty"`
	TwoFactorToken string `json:"two_factor_token" binding:"required"`
	Priority       string `json:"priority,omitempty"`
	Note           string `json:"note,omitempty"`
}

// RequestWithdrawal handles withdrawal requests
func (h *WalletHandler) RequestWithdrawal(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req RequestWithdrawalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	uid, ok := userID.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	amount, err := decimal.NewFromString(req.Amount)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid amount format"})
		return
	}

	priority := interfaces.PriorityMedium
	if req.Priority != "" {
		priority = interfaces.WithdrawalPriority(req.Priority)
	}

	withdrawalReq := interfaces.WithdrawalRequest{
		UserID:         uid,
		Asset:          req.Asset,
		Amount:         amount,
		ToAddress:      req.ToAddress,
		Network:        req.Network,
		Tag:            req.Tag,
		TwoFactorToken: req.TwoFactorToken,
		Priority:       priority,
		Note:           req.Note,
	}

	response, err := h.walletService.RequestWithdrawal(c.Request.Context(), withdrawalReq)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, response)
}

// GetTransactionStatus returns transaction status
func (h *WalletHandler) GetTransactionStatus(c *gin.Context) {
	txIDStr := c.Param("id")
	if txIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "transaction ID is required"})
		return
	}

	txID, err := uuid.Parse(txIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid transaction ID format"})
		return
	}

	status, err := h.walletService.GetTransactionStatus(c.Request.Context(), txID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, status)
}

// ListTransactions returns user transactions with pagination
func (h *WalletHandler) ListTransactions(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	uid, ok := userID.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	// Parse query parameters
	asset := c.Query("asset")
	direction := c.Query("direction")

	limitStr := c.DefaultQuery("limit", "50")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 || limit > 100 {
		limit = 50
	}

	offsetStr := c.DefaultQuery("offset", "0")
	offset, err := strconv.Atoi(offsetStr)
	if err != nil || offset < 0 {
		offset = 0
	}

	transactions, total, err := h.walletService.ListTransactions(
		c.Request.Context(), uid, asset, direction, limit, offset,
	)
	if err != nil {
		h.handleError(c, err)
		return
	}

	response := gin.H{
		"transactions": transactions,
		"total":        total,
		"limit":        limit,
		"offset":       offset,
	}

	c.JSON(http.StatusOK, response)
}

// GetBalance returns user balance
func (h *WalletHandler) GetBalance(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	uid, ok := userID.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	// Parse asset filter
	var assets []string
	if asset := c.Query("asset"); asset != "" {
		assets = strings.Split(asset, ",")
	}

	balance, err := h.walletService.GetBalance(c.Request.Context(), uid, assets)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, balance)
}

// GetDepositAddress returns or creates a deposit address
func (h *WalletHandler) GetDepositAddress(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	uid, ok := userID.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	asset := c.Query("asset")
	network := c.Query("network")

	if asset == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "asset is required"})
		return
	}
	if network == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "network is required"})
		return
	}

	address, err := h.walletService.GetDepositAddress(c.Request.Context(), uid, asset, network)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, address)
}

// ValidateAddressRequest represents address validation request
type ValidateAddressRequest struct {
	Address string `json:"address" binding:"required"`
	Asset   string `json:"asset" binding:"required"`
	Network string `json:"network" binding:"required"`
}

// ValidateAddress validates a withdrawal address
func (h *WalletHandler) ValidateAddress(c *gin.Context) {
	var req ValidateAddressRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	validationReq := interfaces.AddressValidationRequest{
		Address: req.Address,
		Asset:   req.Asset,
		Network: req.Network,
	}

	result, err := h.walletService.ValidateAddress(c.Request.Context(), validationReq)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, result)
}

// HealthCheck returns service health status
func (h *WalletHandler) HealthCheck(c *gin.Context) {
	// Check wallet service health
	if err := h.walletService.HealthCheck(c.Request.Context()); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "unhealthy",
			"error":  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "healthy",
		"service": "wallet",
	})
}

// handleError converts internal errors to HTTP responses
func (h *WalletHandler) handleError(c *gin.Context, err error) {
	switch {
	case err == interfaces.ErrInsufficientBalance:
		c.JSON(http.StatusBadRequest, gin.H{"error": "insufficient balance"})
	case err == interfaces.ErrInvalidAmount:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid amount"})
	case err == interfaces.ErrInvalidAddress:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid address"})
	case err == interfaces.ErrTransactionNotFound:
		c.JSON(http.StatusNotFound, gin.H{"error": "transaction not found"})
	case err == interfaces.ErrUserNotFound:
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
	case err == interfaces.ErrComplianceRejected:
		c.JSON(http.StatusForbidden, gin.H{"error": "compliance check failed"})
	case err == interfaces.ErrRateLimited:
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded"})
	case err == interfaces.ErrServiceUnavailable:
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "service temporarily unavailable"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	}
}
