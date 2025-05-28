package api

import (
	"context"
	"net/http"

	"github.com/Aidin1998/pincex_unified/internal/kyc"
	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// POST /api/v1/user/kyc/start
func (s *Server) startKYC(c *gin.Context) {
	userID, _ := c.Get("userID")
	var req kyc.KYCData
	if err := c.ShouldBindJSON(&req); err != nil {
		s.writeError(c, err)
		return
	}
	service, ok := s.kycProvider.(interface {
		StartKYCRequest(ctx context.Context, userID uuid.UUID, data *kyc.KYCData) (string, error)
	})
	if !ok {
		s.writeError(c, http.ErrNotSupported)
		return
	}
	sessionID, err := service.StartKYCRequest(c.Request.Context(), userID.(uuid.UUID), &req)
	if err != nil {
		s.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"session_id": sessionID})
}

// GET /api/v1/user/kyc/status
func (s *Server) getKYCStatus(c *gin.Context) {
	userID, _ := c.Get("userID")
	service, ok := s.kycProvider.(interface {
		GetKYCStatus(ctx context.Context, userID uuid.UUID) (string, error)
	})
	if !ok {
		s.writeError(c, http.ErrNotSupported)
		return
	}
	status, err := service.GetKYCStatus(c.Request.Context(), userID.(uuid.UUID))
	if err != nil {
		s.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": status})
}

// POST /api/v1/user/kyc/document
func (s *Server) uploadKYCDocument(c *gin.Context) {
	userID, _ := c.Get("userID")
	_ = userID // silence unused var warning
	var doc models.KYCDocument
	if err := c.ShouldBindJSON(&doc); err != nil {
		s.writeError(c, err)
		return
	}
	// Save document (stub)
	c.JSON(http.StatusOK, gin.H{"status": "uploaded"})
}

// GET /api/v1/admin/kyc/cases
func (s *Server) listKYCCases(c *gin.Context) {
	// List KYC requests (stub)
	c.JSON(http.StatusOK, gin.H{"cases": []interface{}{}})
}

// POST /api/v1/admin/kyc/case/:id/approve
func (s *Server) approveKYC(c *gin.Context) {
	caseID := c.Param("id")
	// Approve KYC (stub)
	c.JSON(http.StatusOK, gin.H{"case_id": caseID, "status": "approved"})
}

// POST /api/v1/admin/kyc/case/:id/reject
func (s *Server) rejectKYC(c *gin.Context) {
	caseID := c.Param("id")
	// Reject KYC (stub)
	c.JSON(http.StatusOK, gin.H{"case_id": caseID, "status": "rejected"})
}
