package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
)

func main() {
	// Simple OpenAPI spec for demonstration
	spec := map[string]interface{}{
		"openapi": "3.0.0",
		"info": map[string]interface{}{
			"title":       "Finalex Cryptocurrency Exchange API",
			"version":     "1.0.0",
			"description": "Comprehensive API for the Finalex cryptocurrency exchange platform",
		},
		"servers": []map[string]interface{}{
			{
				"url":         "https://api.finalex.io",
				"description": "Production server",
			},
			{
				"url":         "https://staging-api.finalex.io",
				"description": "Staging server",
			},
			{
				"url":         "http://localhost:8080",
				"description": "Development server",
			},
		},
		"paths": map[string]interface{}{
			"/health": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"System"},
					"summary":     "Health check",
					"description": "Check API health and status",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "API is healthy",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"status": map[string]interface{}{
												"type":    "string",
												"example": "healthy",
											},
											"timestamp": map[string]interface{}{
												"type":    "integer",
												"example": 1672531200,
											},
										},
									},
								},
							},
						},
					},
				},
			},
			"/api/v1/auth/register": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"Authentication"},
					"summary":     "Register new user",
					"description": "Create a new user account",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type":     "object",
									"required": []string{"email", "password", "username"},
									"properties": map[string]interface{}{
										"email": map[string]interface{}{
											"type":    "string",
											"format":  "email",
											"example": "user@example.com",
										},
										"password": map[string]interface{}{
											"type":      "string",
											"minLength": 8,
											"example":   "SecurePass123!",
										},
										"username": map[string]interface{}{
											"type":      "string",
											"minLength": 3,
											"maxLength": 50,
											"example":   "john_doe",
										},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"201": map[string]interface{}{
							"description": "User registered successfully",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"success": map[string]interface{}{
												"type":    "boolean",
												"example": true,
											},
											"message": map[string]interface{}{
												"type":    "string",
												"example": "User registered successfully",
											},
											"user_id": map[string]interface{}{
												"type":    "string",
												"example": "123e4567-e89b-12d3-a456-426614174000",
											},
										},
									},
								},
							},
						},
						"400": map[string]interface{}{
							"description": "Bad request",
						},
						"409": map[string]interface{}{
							"description": "User already exists",
						},
					},
				},
			},
		},
		"components": map[string]interface{}{
			"securitySchemes": map[string]interface{}{
				"BearerAuth": map[string]interface{}{
					"type":         "http",
					"scheme":       "bearer",
					"bearerFormat": "JWT",
					"description":  "JWT token obtained from login endpoint",
				},
				"ApiKeyAuth": map[string]interface{}{
					"type":        "apiKey",
					"in":          "header",
					"name":        "X-API-Key",
					"description": "API key for programmatic access",
				},
			},
		},
		"tags": []map[string]interface{}{
			{
				"name":        "System",
				"description": "Health checks and system information",
			},
			{
				"name":        "Authentication",
				"description": "User authentication and authorization",
			},
			{
				"name":        "Trading",
				"description": "Spot trading operations",
			},
			{
				"name":        "Wallet",
				"description": "Wallet and cryptocurrency operations",
			},
		},
	}

	// Convert to JSON
	jsonData, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal JSON: %v", err)
	}

	// Write swagger.json
	err = os.WriteFile("docs/swagger/swagger.json", jsonData, 0644)
	if err != nil {
		log.Fatalf("Failed to write swagger.json: %v", err)
	}

	// Create docs.go file
	docsContent := fmt.Sprintf(`// Code generated by swagger-gen. DO NOT EDIT.

package swagger

import "github.com/swaggo/swag"

const docTemplate = %q

// SwaggerInfo holds exported Swagger Info so clients can modify it
var SwaggerInfo = &swag.Spec{
	Version:          "1.0.0",
	Host:             "localhost:8080",
	BasePath:         "/",
	Schemes:          []string{"http", "https"},
	Title:            "Finalex Cryptocurrency Exchange API",
	Description:      "Comprehensive API for the Finalex cryptocurrency exchange platform",
	InfoInstanceName: "swagger",
	SwaggerTemplate:  docTemplate,
}

func init() {
	swag.Register(SwaggerInfo.InstanceName(), SwaggerInfo)
}
`, string(jsonData))

	err = os.WriteFile("docs/swagger/docs.go", []byte(docsContent), 0644)
	if err != nil {
		log.Fatalf("Failed to write docs.go: %v", err)
	}

	fmt.Println("✅ Successfully generated Swagger documentation!")
	fmt.Println("📁 Files created:")
	fmt.Println("   - docs/swagger/swagger.json")
	fmt.Println("   - docs/swagger/docs.go")
}
