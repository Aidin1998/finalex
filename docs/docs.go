// Package docs provides OpenAPI documentation for the Finalex API
//
// This package contains automatically generated OpenAPI documentation
// using swaggo/swag. The documentation is served via Swagger UI.
//
// @title           Finalex Cryptocurrency Exchange API
// @version         1.0.0
// @description     Comprehensive API for the Finalex cryptocurrency exchange platform
// @description
// @description     This API provides complete access to all platform functionality including:
// @description     - User authentication and account management
// @description     - Trading operations (spot, cross-pair, advanced orders)
// @description     - Wallet and balance management
// @description     - Market data and analytics
// @description     - Compliance and risk management
// @description     - Administrative functions
// @description
// @description     ## Authentication
// @description
// @description     The API uses multiple authentication methods:
// @description     - **Bearer Token**: JWT tokens for user authentication
// @description     - **API Key**: For programmatic access
// @description     - **HMAC Signature**: For sensitive operations
// @description
// @description     ## Rate Limiting
// @description
// @description     All endpoints are rate limited based on user tier:
// @description     - **Basic**: 100 requests/minute
// @description     - **Premium**: 1000 requests/minute
// @description     - **VIP**: 10000 requests/minute
// @description
// @description     ## Error Handling
// @description
// @description     All errors follow RFC 7807 Problem Details standard
//
// @contact.name   Finalex API Support
// @contact.url    https://finalex.io/support
// @contact.email  api-support@finalex.io
//
// @license.name  MIT
// @license.url   https://opensource.org/licenses/MIT
//
// @host      api.finalex.io
// @BasePath  /
//
// @securityDefinitions.apikey  BearerAuth
// @in                          header
// @name                        Authorization
// @description                 JWT token obtained from login endpoint. Format: "Bearer {token}"
//
// @securityDefinitions.apikey  ApiKeyAuth
// @in                          header
// @name                        X-API-Key
// @description                 API key for programmatic access
//
// @securityDefinitions.apikey  HmacAuth
// @in                          header
// @name                        X-Signature
// @description                 HMAC signature for sensitive operations
//
// @tag.name System
// @tag.description Health checks and system information
//
// @tag.name Authentication
// @tag.description User authentication and authorization
//
// @tag.name Accounts
// @tag.description Account and balance management
//
// @tag.name Trading
// @tag.description Spot trading operations
//
// @tag.name Cross-Pair Trading
// @tag.description Cross-pair trading operations
//
// @tag.name Market Data
// @tag.description Market prices and data feeds
//
// @tag.name Wallet
// @tag.description Wallet and cryptocurrency operations
//
// @tag.name Fiat
// @tag.description Fiat currency operations
//
// @tag.name Compliance
// @tag.description Compliance and risk management
//
// @tag.name Admin
// @tag.description Administrative functions
package docs
