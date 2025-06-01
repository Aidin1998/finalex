package apiutil

import (
	"github.com/gin-gonic/gin"
)

// ErrorResponse is the standard error response structure for all APIs
//
// Example:
//
//	{
//	  "error": "invalid_parameter",
//	  "message": "'quantity' parameter must be a positive decimal with up to 8 decimal places.",
//	  "details": "quantity: -5.0 provided"
//	}
type ErrorResponse struct {
	Error   string      `json:"error"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
}

// WriteErrorResponse writes a consistent error response to the client
func WriteErrorResponse(c *gin.Context, status int, code, message string, details interface{}) {
	c.JSON(status, ErrorResponse{
		Error:   code,
		Message: message,
		Details: details,
	})
}
