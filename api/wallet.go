package api

import (
	"net/http"

	"github.com/Aidin1998/pincex_unified/internal/wallet"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// POST /api/v1/wallet/withdraw
func (s *Server) createWithdrawal(c *gin.Context) {
	userID, _ := c.Get("userID")
	var req struct {
		WalletID  string  `json:"wallet_id"`
		Asset     string  `json:"asset"`
		Amount    float64 `json:"amount"`
		ToAddress string  `json:"to_address"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		s.writeError(c, err)
		return
	}
	ws, ok := s.walletService.(*wallet.WalletService)
	if !ok {
		s.writeError(c, http.ErrNotSupported)
		return
	}
	wr, err := ws.CreateWithdrawalRequest(c.Request.Context(), userID.(uuid.UUID), req.WalletID, req.Asset, req.ToAddress, req.Amount)
	if err != nil {
		s.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, wr)
}

// POST /api/v1/wallet/withdraw/:id/approve
func (s *Server) approveWithdrawal(c *gin.Context) {
	requestID := c.Param("id")
	approver, _ := c.Get("userID")
	ws, ok := s.walletService.(*wallet.WalletService)
	if !ok {
		s.writeError(c, http.ErrNotSupported)
		return
	}
	uuidReq, err := uuid.Parse(requestID)
	if err != nil {
		s.writeError(c, err)
		return
	}
	if err := ws.ApproveWithdrawal(c.Request.Context(), uuidReq, approver.(string)); err != nil {
		s.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "approved"})
}

// GET /api/v1/wallet/balance
func (s *Server) getWalletBalance(c *gin.Context) {
	walletID := c.Query("wallet_id")
	ws, ok := s.walletService.(*wallet.WalletService)
	if !ok {
		s.writeError(c, http.ErrNotSupported)
		return
	}
	bal, err := ws.GetBalance(c.Request.Context(), walletID)
	if err != nil {
		s.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"balance": bal})
}

// GET /api/v1/wallet/transactions
func (s *Server) listWalletTransactions(c *gin.Context) {
	// List transactions (stub)
	c.JSON(http.StatusOK, gin.H{"transactions": []interface{}{}})
}
