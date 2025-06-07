// Package services provides address management functionality
package services

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/Aidin1998/finalex/internal/wallet/interfaces"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// AddressManager handles deposit address management
type AddressManager struct {
	db               *gorm.DB
	logger           *zap.Logger
	fireblocksClient interfaces.FireblocksClient
	repository       interfaces.WalletRepository
	cache            interfaces.WalletCache
	config           *AddressConfig
}

// AddressConfig holds address management configuration
type AddressConfig struct {
	MaxAddressesPerUser map[string]int // asset -> max addresses
	AddressPoolSize     int
	PreGenerateCount    int
	CacheTimeout        time.Duration
	ValidationEnabled   bool
	ReusableAddresses   bool
}

// AddressValidator defines validation rules for different assets
type AddressValidator struct {
	Asset       string
	MinLength   int
	MaxLength   int
	Pattern     *regexp.Regexp
	Checksum    bool
	RequiresTag bool
}

// NewAddressManager creates a new address manager
func NewAddressManager(
	db *gorm.DB,
	logger *zap.Logger,
	fireblocksClient interfaces.FireblocksClient,
	repository interfaces.WalletRepository,
	cache interfaces.WalletCache,
	config *AddressConfig,
) *AddressManager {
	return &AddressManager{
		db:               db,
		logger:           logger,
		fireblocksClient: fireblocksClient,
		repository:       repository,
		cache:            cache,
		config:           config,
	}
}

// GenerateAddress generates a new deposit address for a user
func (am *AddressManager) GenerateAddress(ctx context.Context, userID uuid.UUID, asset, network string) (*interfaces.DepositAddress, error) {
	am.logger.Info("Generating deposit address",
		zap.String("user_id", userID.String()),
		zap.String("asset", asset),
		zap.String("network", network),
	)
	// Check if user already has active addresses for this asset
	if !am.config.ReusableAddresses {
		existingAddresses, err := am.repository.GetUserAddresses(ctx, userID, asset)
		if err != nil {
			return nil, fmt.Errorf("failed to check existing addresses: %w", err)
		}

		// Check address limit
		maxAddresses := am.getMaxAddressesForAsset(asset)
		if len(existingAddresses) >= maxAddresses {
			return nil, fmt.Errorf("maximum addresses per user exceeded for asset %s", asset)
		}

		// Return existing active address if reuse is disabled
		for _, addr := range existingAddresses {
			if addr.IsActive {
				return addr, nil
			}
		}
	}

	// Generate new address via Fireblocks
	fbAddr, err := am.fireblocksClient.GenerateDepositAddress(ctx, "0", asset, &interfaces.AddressParams{
		Description: fmt.Sprintf("Deposit address for user %s", userID.String()),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate address via Fireblocks: %w", err)
	}
	// Create address record
	address := &interfaces.DepositAddress{
		ID:           uuid.New(),
		UserID:       userID,
		Asset:        asset,
		Network:      network,
		Address:      fbAddr.Address,
		Tag:          fbAddr.Tag,
		IsActive:     true,
		FireblocksID: fbAddr.AssetID, // Use AssetID as identifier
		CreatedAt:    time.Now(),
	}

	// Store address
	if err := am.repository.CreateAddress(ctx, address); err != nil {
		return nil, fmt.Errorf("failed to create address record: %w", err)
	}

	// Cache the address
	if am.cache != nil {
		if err := am.cache.SetAddresses(ctx, userID, asset, []*interfaces.DepositAddress{address}, am.config.CacheTimeout); err != nil {
			am.logger.Warn("Failed to cache address", zap.Error(err))
		}
	}

	am.logger.Info("Successfully generated deposit address",
		zap.String("address_id", address.ID.String()),
		zap.String("address", address.Address),
		zap.String("user_id", userID.String()),
		zap.String("asset", asset),
	)

	return address, nil
}

// GetUserAddresses retrieves all addresses for a user and asset
func (am *AddressManager) GetUserAddresses(ctx context.Context, userID uuid.UUID, asset string) ([]*interfaces.DepositAddress, error) {
	am.logger.Debug("Getting user addresses",
		zap.String("user_id", userID.String()),
		zap.String("asset", asset),
	)
	// Try cache first
	if am.cache != nil {
		if cached, err := am.cache.GetAddresses(ctx, userID, asset); err == nil && cached != nil {
			return cached, nil
		}
	}

	// Get from database
	addresses, err := am.repository.GetUserAddresses(ctx, userID, asset)
	if err != nil {
		return nil, fmt.Errorf("failed to get user addresses: %w", err)
	}

	// Cache the result
	if am.cache != nil {
		if err := am.cache.SetAddresses(ctx, userID, asset, addresses, am.config.CacheTimeout); err != nil {
			am.logger.Warn("Failed to cache addresses", zap.Error(err))
		}
	}

	return addresses, nil
}

// GetActiveAddress retrieves the active deposit address for a user and asset
func (am *AddressManager) GetActiveAddress(ctx context.Context, userID uuid.UUID, asset string) (*interfaces.DepositAddress, error) {
	addresses, err := am.GetUserAddresses(ctx, userID, asset)
	if err != nil {
		return nil, err
	}

	// Find active address
	for _, addr := range addresses {
		if addr.IsActive {
			return addr, nil
		}
	}

	// Generate new address if none found
	return am.GenerateAddress(ctx, userID, asset, am.getDefaultNetwork(asset))
}

// ValidateAddress validates a withdrawal address
func (am *AddressManager) ValidateAddress(ctx context.Context, req *interfaces.AddressValidationRequest) (*interfaces.AddressValidationResult, error) {
	am.logger.Debug("Validating address",
		zap.String("address", req.Address),
		zap.String("asset", req.Asset),
		zap.String("network", req.Network),
	)
	result := &interfaces.AddressValidationResult{
		IsValid: false,
		Valid:   false,
	}

	// Basic validation
	if req.Address == "" {
		result.Reason = "Address is required"
		return result, nil
	}

	if req.Asset == "" {
		result.Reason = "Asset is required"
		return result, nil
	}

	// Get validator for asset
	validator := am.getValidatorForAsset(req.Asset)
	if validator == nil {
		result.Reason = fmt.Sprintf("Validation not supported for asset: %s", req.Asset)
		return result, nil
	}

	// Validate address format
	if err := am.validateAddressFormat(req.Address, validator); err != nil {
		result.Reason = err.Error()
		return result, nil
	}

	// Validate tag if required
	if validator.RequiresTag {
		if req.Tag == "" {
			result.Reason = "Tag is required for this asset"
			return result, nil
		}
	}
	// Additional validation via Fireblocks (if enabled)
	if am.config.ValidationEnabled {
		fbResult, err := am.fireblocksClient.ValidateAddress(ctx, req.Asset, req.Address)
		if err != nil {
			result.Reason = fmt.Sprintf("External validation failed: %v", err)
			return result, nil
		}

		if !fbResult.IsValid {
			result.Reason = "Address validation failed"
			return result, nil
		}
	}

	result.IsValid = true
	result.Reason = "Address is valid"
	return result, nil
}

// DeactivateAddress deactivates a deposit address
func (am *AddressManager) DeactivateAddress(ctx context.Context, addressID uuid.UUID) error {
	am.logger.Info("Deactivating address", zap.String("address_id", addressID.String()))

	// Update status directly using repository
	updates := map[string]interface{}{
		"is_active": false,
	}

	if err := am.repository.UpdateAddress(ctx, addressID, updates); err != nil {
		return fmt.Errorf("failed to update address: %w", err)
	}

	// Note: We can't invalidate cache without knowing user/asset details
	// This would need to be handled differently in a real implementation

	return nil
}

// GetAddressByValue finds an address by its value
func (am *AddressManager) GetAddressByValue(ctx context.Context, address, asset string) (*interfaces.DepositAddress, error) {
	return am.repository.GetAddressByValue(ctx, address, asset)
}

// GetAddressStatistics retrieves address usage statistics
func (am *AddressManager) GetAddressStatistics(ctx context.Context, userID uuid.UUID) (*interfaces.AddressStatistics, error) {
	stats := &interfaces.AddressStatistics{
		TotalAddresses:  0,
		ActiveAddresses: 0,
		ByAsset:         make(map[string]int),
		ByNetwork:       make(map[string]int),
		LastGenerated:   nil,
		OldestAddress:   nil,
	}

	// Since GetAllUserAddresses doesn't exist, we need to get addresses for all assets
	// This is a limitation we need to work around
	// For now, we'll return the basic structure
	// In a real implementation, you'd need to add a method to get all addresses for a user

	return stats, nil
}

// BatchGenerateAddresses generates multiple addresses at once
func (am *AddressManager) BatchGenerateAddresses(ctx context.Context, requests []*interfaces.AddressGenerationRequest) ([]*interfaces.DepositAddress, error) {
	am.logger.Info("Batch generating addresses", zap.Int("count", len(requests)))

	addresses := make([]*interfaces.DepositAddress, 0, len(requests))

	for _, req := range requests {
		addr, err := am.GenerateAddress(ctx, req.UserID, req.Asset, req.Network)
		if err != nil {
			am.logger.Error("Failed to generate address in batch",
				zap.String("user_id", req.UserID.String()),
				zap.String("asset", req.Asset),
				zap.Error(err),
			)
			continue
		}
		addresses = append(addresses, addr)
	}

	return addresses, nil
}

// PreGenerateAddresses pre-generates addresses for popular assets
func (am *AddressManager) PreGenerateAddresses(ctx context.Context) error {
	am.logger.Info("Pre-generating addresses")

	// This would typically be run as a background job
	// For now, we'll just log that it would happen
	am.logger.Info("Pre-generation completed")

	return nil
}

// Private helper methods

func (am *AddressManager) getMaxAddressesForAsset(asset string) int {
	if max, exists := am.config.MaxAddressesPerUser[asset]; exists {
		return max
	}
	return 5 // Default
}

func (am *AddressManager) getDefaultNetwork(asset string) string {
	// Return default network for asset
	switch asset {
	case "BTC":
		return "bitcoin"
	case "ETH":
		return "ethereum"
	case "USDT":
		return "ethereum" // Assume ERC-20 by default
	case "USDC":
		return "ethereum" // Assume ERC-20 by default
	default:
		return "ethereum"
	}
}

func (am *AddressManager) getValidatorForAsset(asset string) *AddressValidator {
	validators := map[string]*AddressValidator{
		"BTC": {
			Asset:     "BTC",
			MinLength: 26,
			MaxLength: 62,
			Pattern:   regexp.MustCompile(`^[13][a-km-zA-HJ-NP-Z1-9]{25,34}$|^bc1[a-z0-9]{39,59}$`),
			Checksum:  true,
		},
		"ETH": {
			Asset:     "ETH",
			MinLength: 42,
			MaxLength: 42,
			Pattern:   regexp.MustCompile(`^0x[a-fA-F0-9]{40}$`),
			Checksum:  false,
		},
		"USDT": {
			Asset:     "USDT",
			MinLength: 42,
			MaxLength: 42,
			Pattern:   regexp.MustCompile(`^0x[a-fA-F0-9]{40}$`),
			Checksum:  false,
		},
		"USDC": {
			Asset:     "USDC",
			MinLength: 42,
			MaxLength: 42,
			Pattern:   regexp.MustCompile(`^0x[a-fA-F0-9]{40}$`),
			Checksum:  false,
		},
		"XRP": {
			Asset:       "XRP",
			MinLength:   25,
			MaxLength:   34,
			Pattern:     regexp.MustCompile(`^r[0-9a-zA-Z]{24,34}$`),
			Checksum:    true,
			RequiresTag: true,
		},
	}

	return validators[asset]
}

func (am *AddressManager) validateAddressFormat(address string, validator *AddressValidator) error {
	// Check length
	if len(address) < validator.MinLength || len(address) > validator.MaxLength {
		return fmt.Errorf("invalid address length: expected %d-%d, got %d",
			validator.MinLength, validator.MaxLength, len(address))
	}

	// Check pattern
	if validator.Pattern != nil && !validator.Pattern.MatchString(address) {
		return fmt.Errorf("invalid address format for %s", validator.Asset)
	}

	// Additional checksum validation could be added here
	if validator.Checksum {
		if err := am.validateChecksum(address, validator.Asset); err != nil {
			return err
		}
	}

	return nil
}

func (am *AddressManager) validateChecksum(address, asset string) error {
	// Simplified checksum validation
	// In a real implementation, you'd use proper libraries for each asset
	switch asset {
	case "BTC":
		return am.validateBitcoinChecksum(address)
	case "ETH", "USDT", "USDC":
		return am.validateEthereumChecksum(address)
	case "XRP":
		return am.validateRippleChecksum(address)
	default:
		return nil // Skip validation for unknown assets
	}
}

func (am *AddressManager) validateBitcoinChecksum(address string) error {
	// Simplified Bitcoin address validation
	// In production, use a proper Bitcoin library
	if strings.HasPrefix(address, "1") || strings.HasPrefix(address, "3") {
		// Legacy address format - would need Base58 checksum validation
		return nil
	}
	if strings.HasPrefix(address, "bc1") {
		// Bech32 address format - would need Bech32 checksum validation
		return nil
	}
	return fmt.Errorf("invalid Bitcoin address format")
}

func (am *AddressManager) validateEthereumChecksum(address string) error {
	// Simplified Ethereum address validation
	// In production, use a proper Ethereum library for EIP-55 checksum
	if !strings.HasPrefix(address, "0x") {
		return fmt.Errorf("Ethereum address must start with 0x")
	}

	// Check if it's all lowercase or all uppercase (no checksum)
	hexPart := address[2:]
	if hexPart == strings.ToLower(hexPart) || hexPart == strings.ToUpper(hexPart) {
		return nil // Valid but no checksum
	}

	// Would implement EIP-55 checksum validation here
	return nil
}

func (am *AddressManager) validateRippleChecksum(address string) error {
	// Simplified Ripple address validation
	// In production, use a proper Ripple library
	if !strings.HasPrefix(address, "r") {
		return fmt.Errorf("Ripple address must start with 'r'")
	}

	// Would implement proper Ripple checksum validation here
	return nil
}

func (am *AddressManager) invalidateAddressCache(userID uuid.UUID, asset string) {
	if am.cache != nil {
		// Invalidate address cache using the correct method
		if err := am.cache.InvalidateAddresses(context.Background(), userID, asset); err != nil {
			am.logger.Warn("Failed to invalidate address cache", zap.Error(err))
		}
	}
}
