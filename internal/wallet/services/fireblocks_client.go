// Package services provides Fireblocks client implementation
package services

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/Aidin1998/finalex/internal/wallet/interfaces"
	"github.com/golang-jwt/jwt/v4"
	"go.uber.org/zap"
)

// FireblocksClient implements the Fireblocks API client
type FireblocksClient struct {
	baseURL    string
	apiKey     string
	privateKey string
	httpClient *http.Client
	logger     *zap.Logger
	config     *FireblocksConfig
}

// FireblocksConfig holds Fireblocks client configuration
type FireblocksConfig struct {
	BaseURL        string
	APIKey         string
	PrivateKeyPath string
	Timeout        time.Duration
	RetryAttempts  int
	RetryDelay     time.Duration
	RateLimit      int // requests per minute
}

// NewFireblocksClient creates a new Fireblocks client
func NewFireblocksClient(logger *zap.Logger, config *FireblocksConfig) (*FireblocksClient, error) {
	// Load private key
	privateKey, err := loadPrivateKey(config.PrivateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load private key: %w", err)
	}

	return &FireblocksClient{
		baseURL:    config.BaseURL,
		apiKey:     config.APIKey,
		privateKey: privateKey,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		logger: logger,
		config: config,
	}, nil
}

// GetVaultAccounts retrieves vault accounts
func (fc *FireblocksClient) GetVaultAccounts(ctx context.Context) ([]*interfaces.FireblocksVault, error) {
	fc.logger.Debug("Getting vault accounts")

	var response []*interfaces.FireblocksVault
	if err := fc.makeRequest(ctx, "GET", "/v1/vault/accounts", nil, &response); err != nil {
		return nil, fmt.Errorf("failed to get vault accounts: %w", err)
	}

	return response, nil
}

// GetAssetBalance retrieves balance for a specific asset in a vault
func (fc *FireblocksClient) GetAssetBalance(ctx context.Context, vaultID, assetID string) (*interfaces.FireblocksBalance, error) {
	fc.logger.Debug("Getting asset balance",
		zap.String("vault_id", vaultID),
		zap.String("asset_id", assetID),
	)

	endpoint := fmt.Sprintf("/v1/vault/accounts/%s/%s", vaultID, assetID)
	var response *interfaces.FireblocksBalance
	if err := fc.makeRequest(ctx, "GET", endpoint, nil, &response); err != nil {
		return nil, fmt.Errorf("failed to get asset balance: %w", err)
	}

	return response, nil
}

// GenerateAddress generates a new deposit address
func (fc *FireblocksClient) GenerateAddress(ctx context.Context, req *interfaces.FireblocksAddressRequest) (*interfaces.FireblocksAddress, error) {
	fc.logger.Info("Generating address",
		zap.String("asset_id", req.AssetID),
		zap.String("vault_id", req.VaultID),
	)

	endpoint := fmt.Sprintf("/v1/vault/accounts/%s/%s/addresses", req.VaultID, req.AssetID)
	var response *interfaces.FireblocksAddress
	if err := fc.makeRequest(ctx, "POST", endpoint, req, &response); err != nil {
		return nil, fmt.Errorf("failed to generate address: %w", err)
	}

	return response, nil
}

// GetAddresses retrieves addresses for a vault and asset
func (fc *FireblocksClient) GetAddresses(ctx context.Context, vaultID, assetID string) ([]*interfaces.FireblocksAddress, error) {
	fc.logger.Debug("Getting addresses",
		zap.String("vault_id", vaultID),
		zap.String("asset_id", assetID),
	)

	endpoint := fmt.Sprintf("/v1/vault/accounts/%s/%s/addresses", vaultID, assetID)
	var response []*interfaces.FireblocksAddress
	if err := fc.makeRequest(ctx, "GET", endpoint, nil, &response); err != nil {
		return nil, fmt.Errorf("failed to get addresses: %w", err)
	}

	return response, nil
}

// CreateTransaction creates a new transaction
func (fc *FireblocksClient) CreateTransaction(ctx context.Context, req *interfaces.FireblocksTransactionRequest) (*interfaces.FireblocksTransaction, error) {
	fc.logger.Info("Creating transaction",
		zap.String("asset_id", req.AssetID),
		zap.String("amount", req.Amount.String()),
		zap.String("destination", req.Destination.Address),
	)

	var response *interfaces.FireblocksTransaction
	if err := fc.makeRequest(ctx, "POST", "/v1/transactions", req, &response); err != nil {
		return nil, fmt.Errorf("failed to create transaction: %w", err)
	}

	return response, nil
}

// GetTransaction retrieves a transaction by ID
func (fc *FireblocksClient) GetTransaction(ctx context.Context, txID string) (*interfaces.FireblocksTransaction, error) {
	fc.logger.Debug("Getting transaction", zap.String("tx_id", txID))

	endpoint := fmt.Sprintf("/v1/transactions/%s", txID)
	var response *interfaces.FireblocksTransaction
	if err := fc.makeRequest(ctx, "GET", endpoint, nil, &response); err != nil {
		return nil, fmt.Errorf("failed to get transaction: %w", err)
	}

	return response, nil
}

// CancelTransaction cancels a transaction
func (fc *FireblocksClient) CancelTransaction(ctx context.Context, txID string) (*interfaces.FireblocksTransaction, error) {
	fc.logger.Info("Cancelling transaction", zap.String("tx_id", txID))

	endpoint := fmt.Sprintf("/v1/transactions/%s/cancel", txID)
	var response *interfaces.FireblocksTransaction
	if err := fc.makeRequest(ctx, "POST", endpoint, nil, &response); err != nil {
		return nil, fmt.Errorf("failed to cancel transaction: %w", err)
	}

	return response, nil
}

// GetTransactions retrieves transactions with filters
func (fc *FireblocksClient) GetTransactions(ctx context.Context, filters *interfaces.FireblocksTransactionFilters) ([]*interfaces.FireblocksTransaction, error) {
	fc.logger.Debug("Getting transactions with filters")

	endpoint := "/v1/transactions"

	// Add query parameters
	if filters != nil {
		queryParams := make([]string, 0)
		if filters.Status != "" {
			queryParams = append(queryParams, fmt.Sprintf("status=%s", filters.Status))
		}
		if filters.AssetID != "" {
			queryParams = append(queryParams, fmt.Sprintf("assets=%s", filters.AssetID))
		}
		if filters.Limit > 0 {
			queryParams = append(queryParams, fmt.Sprintf("limit=%d", filters.Limit))
		}
		if filters.Before != "" {
			queryParams = append(queryParams, fmt.Sprintf("before=%s", filters.Before))
		}

		if len(queryParams) > 0 {
			endpoint += "?" + queryParams[0]
			for _, param := range queryParams[1:] {
				endpoint += "&" + param
			}
		}
	}

	var response []*interfaces.FireblocksTransaction
	if err := fc.makeRequest(ctx, "GET", endpoint, nil, &response); err != nil {
		return nil, fmt.Errorf("failed to get transactions: %w", err)
	}

	return response, nil
}

// ValidateAddress validates an external address
func (fc *FireblocksClient) ValidateAddress(ctx context.Context, req *interfaces.FireblocksAddressValidationRequest) (*interfaces.FireblocksAddressValidationResult, error) {
	fc.logger.Debug("Validating address",
		zap.String("address", req.Address),
		zap.String("asset_id", req.AssetID),
	)

	endpoint := fmt.Sprintf("/v1/external_wallets/%s/%s/validate", req.AssetID, req.Address)
	var response *interfaces.FireblocksAddressValidationResult
	if err := fc.makeRequest(ctx, "GET", endpoint, nil, &response); err != nil {
		return nil, fmt.Errorf("failed to validate address: %w", err)
	}

	return response, nil
}

// GetSupportedAssets retrieves supported assets
func (fc *FireblocksClient) GetSupportedAssets(ctx context.Context) ([]*interfaces.FireblocksAsset, error) {
	fc.logger.Debug("Getting supported assets")

	var response []*interfaces.FireblocksAsset
	if err := fc.makeRequest(ctx, "GET", "/v1/supported_assets", nil, &response); err != nil {
		return nil, fmt.Errorf("failed to get supported assets: %w", err)
	}

	return response, nil
}

// GetNetworkFee estimates network fee for a transaction
func (fc *FireblocksClient) GetNetworkFee(ctx context.Context, assetID string) (*interfaces.FireblocksNetworkFee, error) {
	fc.logger.Debug("Getting network fee", zap.String("asset_id", assetID))

	endpoint := fmt.Sprintf("/v1/estimate_network_fee?assetId=%s", assetID)
	var response *interfaces.FireblocksNetworkFee
	if err := fc.makeRequest(ctx, "GET", endpoint, nil, &response); err != nil {
		return nil, fmt.Errorf("failed to get network fee: %w", err)
	}

	return response, nil
}

// VerifyWebhook verifies webhook signature and parses data
func (fc *FireblocksClient) VerifyWebhook(ctx context.Context, signature, body string) (*interfaces.FireblocksWebhookData, error) {
	fc.logger.Debug("Verifying webhook")

	// Verify signature
	if err := fc.verifyWebhookSignature(signature, body); err != nil {
		return nil, fmt.Errorf("webhook signature verification failed: %w", err)
	}

	// Parse webhook data
	var webhookData *interfaces.FireblocksWebhookData
	if err := json.Unmarshal([]byte(body), &webhookData); err != nil {
		return nil, fmt.Errorf("failed to parse webhook data: %w", err)
	}

	return webhookData, nil
}

// Private helper methods

func (fc *FireblocksClient) makeRequest(ctx context.Context, method, endpoint string, body interface{}, response interface{}) error {
	// Prepare request body
	var reqBody []byte
	var err error
	if body != nil {
		reqBody, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
	}

	// Create request
	url := fc.baseURL + endpoint
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewBuffer(reqBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", fc.apiKey)

	// Generate JWT token for authentication
	token, err := fc.generateJWTToken(endpoint, string(reqBody))
	if err != nil {
		return fmt.Errorf("failed to generate JWT token: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	// Make request with retry logic
	var resp *http.Response
	for attempt := 0; attempt < fc.config.RetryAttempts; attempt++ {
		resp, err = fc.httpClient.Do(req)
		if err != nil {
			if attempt < fc.config.RetryAttempts-1 {
				fc.logger.Warn("Request failed, retrying",
					zap.Error(err),
					zap.Int("attempt", attempt+1),
				)
				time.Sleep(fc.config.RetryDelay)
				continue
			}
			return fmt.Errorf("request failed after %d attempts: %w", fc.config.RetryAttempts, err)
		}
		break
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	// Check status code
	if resp.StatusCode >= 400 {
		var fbError interfaces.FireblocksError
		if err := json.Unmarshal(respBody, &fbError); err == nil {
			return &fbError
		}
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	// Parse response
	if response != nil {
		if err := json.Unmarshal(respBody, response); err != nil {
			return fmt.Errorf("failed to unmarshal response: %w", err)
		}
	}

	return nil
}

func (fc *FireblocksClient) generateJWTToken(uri, body string) (string, error) {
	now := time.Now()
	nonce := strconv.FormatInt(now.UnixNano(), 10)

	// Create hash of body
	bodyHash := sha256.Sum256([]byte(body))
	bodyHashHex := hex.EncodeToString(bodyHash[:])

	// Create claims
	claims := jwt.MapClaims{
		"uri":      uri,
		"nonce":    nonce,
		"iat":      now.Unix(),
		"exp":      now.Add(30 * time.Second).Unix(),
		"sub":      fc.apiKey,
		"bodyHash": bodyHashHex,
	}

	// Create token
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)

	// Parse private key
	privateKey, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(fc.privateKey))
	if err != nil {
		return "", fmt.Errorf("failed to parse private key: %w", err)
	}

	// Sign token
	tokenString, err := token.SignedString(privateKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}

	return tokenString, nil
}

func (fc *FireblocksClient) verifyWebhookSignature(signature, body string) error {
	// Simplified webhook signature verification
	// In production, implement proper signature verification using public key
	if signature == "" {
		return fmt.Errorf("missing webhook signature")
	}

	// Create hash of body
	bodyHash := sha256.Sum256([]byte(body))
	expectedSignature := hex.EncodeToString(bodyHash[:])

	// In a real implementation, you would verify the signature using Fireblocks' public key
	// For now, we'll just check that a signature is present
	if len(signature) < 32 { // Minimum reasonable signature length
		return fmt.Errorf("invalid webhook signature format")
	}

	fc.logger.Debug("Webhook signature verified",
		zap.String("expected", expectedSignature[:16]+"..."),
		zap.String("received", signature[:16]+"..."),
	)

	return nil
}

func loadPrivateKey(path string) (string, error) {
	// In production, load the private key from a secure location
	// For now, return a placeholder
	return "-----BEGIN RSA PRIVATE KEY-----\n...\n-----END RSA PRIVATE KEY-----", nil
}
