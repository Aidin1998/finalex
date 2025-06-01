// Package docs provides Swagger API documentation for PinEx Crypto Exchange
// This file contains the main Swagger configuration and initialization
//
//	@title			PinEx Crypto Exchange API
//	@version		2.0.0
//	@description	Comprehensive REST API for PinEx cryptocurrency exchange platform
//	@description
//	@description	## Features
//	@description	- **High-Performance Trading**: 75,000+ TPS matching engine
//	@description	- **Advanced Security**: Multi-layer authentication with 2FA
//	@description	- **Real-time Data**: WebSocket streaming for market data
//	@description	- **Comprehensive AML**: Advanced compliance and risk management
//	@description	- **Multi-Currency**: Support for 5000+ trading pairs
//	@description	- **Institutional Grade**: Enterprise-ready infrastructure
//	@description
//	@description	## Authentication
//	@description	The API uses JWT Bearer tokens for authentication. Include the token in the Authorization header:
//	@description	`Authorization: Bearer <your_jwt_token>`
//	@description
//	@description	## Rate Limiting
//	@description	API endpoints are rate limited based on user tiers:
//	@description	- **Basic**: Standard limits for retail users
//	@description	- **Premium**: Higher limits for verified users
//	@description	- **VIP**: Maximum limits for institutional clients
//	@description
//	@description	## Error Handling
//	@description	The API returns standard HTTP status codes and JSON error responses:
//	@description	```json
//	@description	{
//	@description		"error": "Error description",
//	@description		"details": "Additional error details",
//	@description		"code": "ERROR_CODE",
//	@description		"timestamp": "2025-06-01T12:00:00Z"
//	@description	}
//	@description	```
//	@description
//	@description	## WebSocket Endpoints
//	@description	Real-time data is available via WebSocket connections:
//	@description	- **Market Data**: `/ws/marketdata`
//	@description	- **Order Updates**: `/ws/orders` (authenticated)
//	@description	- **Account Updates**: `/ws/account` (authenticated)
//	@description
//	@termsOfService	https://api.pincex.com/terms
//	@contact.name	PinEx API Support
//	@contact.url	https://support.pincex.com
//	@contact.email	api-support@pincex.com
//	@license.name	MIT
//	@license.url	https://opensource.org/licenses/MIT
//	@host			api.pincex.com
//	@basePath		/api/v1
//	@schemes		https wss
//
// @securityDefinitions.apikey	BearerAuth
// @in							header
// @name						Authorization
// @description				JWT Bearer token authentication. Format: 'Bearer <token>'
//
// @securityDefinitions.oauth2.application	OAuth2Application
// @tokenUrl								https://api.pincex.com/oauth/token
// @scope.read								Read access to account data
// @scope.write								Write access to place orders
// @scope.admin								Administrative access
//
// @x-logo	{"url": "https://api.pincex.com/logo.png", "altText": "PinEx Logo"}
package docs

import (
	"github.com/swaggo/swag"
)

const docTemplate = `{
    "schemes": {{ marshal .Schemes }},
    "swagger": "2.0",
    "info": {
        "description": "{{escape .Description}}",
        "title": "{{.Title}}",
        "termsOfService": "{{.TermsOfService}}",
        "contact": {
            "name": "{{.Contact.Name}}",
            "url": "{{.Contact.URL}}",
            "email": "{{.Contact.Email}}"
        },
        "license": {
            "name": "{{.License.Name}}",
            "url": "{{.License.URL}}"
        },
        "version": "{{.Version}}"
    },
    "host": "{{.Host}}",
    "basePath": "{{.BasePath}}",
    "paths": {},
    "definitions": {}
}`

func init() {
	swag.Register("swagger", &swag.Spec{
		Version:          "2.0.0",
		Host:             "api.pincex.com",
		BasePath:         "/api/v1",
		Schemes:          []string{"https", "wss"},
		Title:            "PinEx Crypto Exchange API",
		Description:      "Comprehensive REST API for PinEx cryptocurrency exchange platform",
		InfoInstanceName: "swagger",
		SwaggerTemplate:  docTemplate,
	})
}
