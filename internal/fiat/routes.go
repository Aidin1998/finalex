package fiat

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Routes configures all fiat-related routes
func Routes(
	router *gin.RouterGroup,
	db *gorm.DB,
	logger *zap.Logger,
	signatureKey []byte,
	providers map[string]ProviderInfo,
	allowedIPs []string,
) {
	// Initialize fiat service
	fiatService := NewFiatService(db, logger, signatureKey)
	handler := NewHandler(fiatService, logger)

	// Initialize security middleware
	securityMiddleware := NewSecurityMiddleware(logger, providers)

	// Apply global security middleware
	fiatGroup := router.Group("/fiat")
	fiatGroup.Use(securityMiddleware.SecurityHeadersMiddleware())
	fiatGroup.Use(securityMiddleware.RequestValidationMiddleware())

	// Health check endpoint (no auth required)
	fiatGroup.GET("/health", handler.HealthCheckHandler)

	// Provider receipt endpoints (external provider access)
	providerGroup := fiatGroup.Group("/receipts")
	providerGroup.Use(securityMiddleware.ProviderAuthMiddleware())
	providerGroup.Use(securityMiddleware.PayloadValidationMiddleware())
	providerGroup.Use(securityMiddleware.RateLimitMiddleware())

	// Optional: Add IP whitelist if provided
	if len(allowedIPs) > 0 {
		providerGroup.Use(securityMiddleware.IPWhitelistMiddleware(allowedIPs))
	}

	// Optional: Add mTLS validation (uncomment for production with mTLS)
	// providerGroup.Use(securityMiddleware.MTLSMiddleware())

	{
		// External provider endpoints
		providerGroup.POST("", handler.DepositReceiptHandler)               // Process deposit receipt
		providerGroup.GET("/:receipt_id", handler.GetDepositReceiptHandler) // Get receipt status
	}

	// User endpoints (require user authentication)
	// Note: These would typically use the main auth middleware from userauth module
	userGroup := fiatGroup.Group("/user")
	// userGroup.Use(authMiddleware) // This would be the main user auth middleware
	{
		userGroup.GET("/receipts", handler.GetUserDepositsHandler) // Get user's deposit receipts
	}
}

// ProviderConfig represents provider configuration
type ProviderConfig struct {
	Providers    map[string]ProviderInfo `json:"providers"`
	SignatureKey string                  `json:"signature_key"`
	AllowedIPs   []string                `json:"allowed_ips,omitempty"`
}

// DefaultProviderConfig returns a default configuration for testing
func DefaultProviderConfig() ProviderConfig {
	return ProviderConfig{
		Providers: map[string]ProviderInfo{
			"test_provider": {
				ID:        "test_provider",
				Name:      "Test Payment Provider",
				PublicKey: "test_key_placeholder",
				Endpoint:  "https://api.testprovider.com",
				Active:    true,
			},
			"bank_transfer": {
				ID:        "bank_transfer",
				Name:      "Bank Transfer Provider",
				PublicKey: "bank_key_placeholder",
				Endpoint:  "https://api.bankprovider.com",
				Active:    true,
			},
		},
		SignatureKey: "default_test_signature_key_change_in_production",
		AllowedIPs:   []string{}, // Empty means no IP restrictions
	}
}
