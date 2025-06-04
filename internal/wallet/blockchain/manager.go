package blockchain

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// BlockchainManager handles blockchain interactions for wallet operations
type BlockchainManager struct {
	logger  *zap.Logger
	db      *gorm.DB
	mu      sync.RWMutex
	clients map[string]BlockchainClient
	config  *Config
	running bool
}

// BlockchainClient interface for different blockchain implementations
type BlockchainClient interface {
	GetBalance(ctx context.Context, address string) (decimal.Decimal, error)
	SendTransaction(ctx context.Context, from, to string, amount decimal.Decimal, gasPrice decimal.Decimal) (*Transaction, error)
	GetTransaction(ctx context.Context, txHash string) (*Transaction, error)
	GetTransactionHistory(ctx context.Context, address string, limit int) ([]*Transaction, error)
	EstimateGas(ctx context.Context, from, to string, amount decimal.Decimal) (decimal.Decimal, error)
	ValidateAddress(address string) bool
	GetBlockNumber(ctx context.Context) (uint64, error)
	Subscribe(ctx context.Context, address string, callback func(*Transaction)) error
}

// Transaction represents a blockchain transaction
type Transaction struct {
	ID            string          `json:"id" gorm:"primaryKey"`
	Hash          string          `json:"hash" gorm:"uniqueIndex"`
	From          string          `json:"from" gorm:"index"`
	To            string          `json:"to" gorm:"index"`
	Amount        decimal.Decimal `json:"amount" gorm:"type:decimal(36,18)"`
	Fee           decimal.Decimal `json:"fee" gorm:"type:decimal(36,18)"`
	GasPrice      decimal.Decimal `json:"gas_price" gorm:"type:decimal(36,18)"`
	GasUsed       uint64          `json:"gas_used"`
	BlockNumber   uint64          `json:"block_number" gorm:"index"`
	BlockHash     string          `json:"block_hash"`
	Status        string          `json:"status"` // pending, confirmed, failed
	Confirmations int             `json:"confirmations"`
	Asset         string          `json:"asset" gorm:"index"`
	Data          string          `json:"data"`
	Nonce         uint64          `json:"nonce"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
	ConfirmedAt   *time.Time      `json:"confirmed_at"`
}

// Config represents blockchain manager configuration
type Config struct {
	Networks             map[string]NetworkConfig `json:"networks"`
	DefaultConfirmations int                      `json:"default_confirmations"`
	GasPriceMultiplier   float64                  `json:"gas_price_multiplier"`
	MaxRetries           int                      `json:"max_retries"`
	RetryInterval        time.Duration            `json:"retry_interval"`
}

// NetworkConfig represents configuration for a specific blockchain network
type NetworkConfig struct {
	Name                  string `json:"name"`
	ChainID               int    `json:"chain_id"`
	RPC                   string `json:"rpc"`
	WSS                   string `json:"wss"`
	ExplorerURL           string `json:"explorer_url"`
	RequiredConfirmations int    `json:"required_confirmations"`
	GasLimit              uint64 `json:"gas_limit"`
	MinGasPrice           string `json:"min_gas_price"`
	MaxGasPrice           string `json:"max_gas_price"`
}

// NewBlockchainManager creates a new blockchain manager
func NewBlockchainManager(logger *zap.Logger, db *gorm.DB, config *Config) (*BlockchainManager, error) {
	if config == nil {
		config = &Config{
			Networks: map[string]NetworkConfig{
				"ethereum": {
					Name:                  "Ethereum",
					ChainID:               1,
					RPC:                   "https://mainnet.infura.io/v3/YOUR-PROJECT-ID",
					WSS:                   "wss://mainnet.infura.io/ws/v3/YOUR-PROJECT-ID",
					ExplorerURL:           "https://etherscan.io",
					RequiredConfirmations: 12,
					GasLimit:              21000,
					MinGasPrice:           "1000000000",   // 1 gwei
					MaxGasPrice:           "100000000000", // 100 gwei
				},
				"bitcoin": {
					Name:                  "Bitcoin",
					ChainID:               0,
					RPC:                   "https://bitcoin-rpc.example.com",
					RequiredConfirmations: 6,
				},
			},
			DefaultConfirmations: 6,
			GasPriceMultiplier:   1.2,
			MaxRetries:           3,
			RetryInterval:        30 * time.Second,
		}
	}

	manager := &BlockchainManager{
		logger:  logger,
		db:      db,
		clients: make(map[string]BlockchainClient),
		config:  config,
	}

	// Auto-migrate transaction table
	if err := db.AutoMigrate(&Transaction{}); err != nil {
		return nil, fmt.Errorf("failed to migrate transaction table: %w", err)
	}

	// Initialize blockchain clients
	for network := range config.Networks {
		client, err := manager.createClient(network)
		if err != nil {
			logger.Warn("Failed to create client for network",
				zap.String("network", network),
				zap.Error(err))
			continue
		}
		manager.clients[network] = client
	}

	return manager, nil
}

// createClient creates a blockchain client for the specified network
func (m *BlockchainManager) createClient(network string) (BlockchainClient, error) {
	switch network {
	case "ethereum":
		return NewEthereumClient(m.config.Networks[network], m.logger)
	case "bitcoin":
		return NewBitcoinClient(m.config.Networks[network], m.logger)
	default:
		return nil, fmt.Errorf("unsupported network: %s", network)
	}
}

// GetBalance gets the balance for an address on a specific network
func (m *BlockchainManager) GetBalance(ctx context.Context, network, address string) (decimal.Decimal, error) {
	m.mu.RLock()
	client, exists := m.clients[network]
	m.mu.RUnlock()

	if !exists {
		return decimal.Zero, fmt.Errorf("client not found for network: %s", network)
	}

	balance, err := client.GetBalance(ctx, address)
	if err != nil {
		return decimal.Zero, fmt.Errorf("failed to get balance: %w", err)
	}

	m.logger.Debug("Retrieved balance",
		zap.String("network", network),
		zap.String("address", address),
		zap.String("balance", balance.String()))

	return balance, nil
}

// SendTransaction sends a transaction on the specified network
func (m *BlockchainManager) SendTransaction(ctx context.Context, network, from, to string, amount decimal.Decimal) (*Transaction, error) {
	m.mu.RLock()
	client, exists := m.clients[network]
	m.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("client not found for network: %s", network)
	}

	// Estimate gas price
	gasPrice, err := m.estimateGasPrice(ctx, network)
	if err != nil {
		return nil, fmt.Errorf("failed to estimate gas price: %w", err)
	}

	// Send transaction
	tx, err := client.SendTransaction(ctx, from, to, amount, gasPrice)
	if err != nil {
		return nil, fmt.Errorf("failed to send transaction: %w", err)
	}

	// Save transaction to database
	tx.ID = uuid.New().String()
	tx.Asset = network
	tx.Status = "pending"
	tx.CreatedAt = time.Now()
	tx.UpdatedAt = time.Now()

	if err := m.db.WithContext(ctx).Create(tx).Error; err != nil {
		m.logger.Error("Failed to save transaction",
			zap.String("hash", tx.Hash),
			zap.Error(err))
	}

	m.logger.Info("Transaction sent",
		zap.String("network", network),
		zap.String("hash", tx.Hash),
		zap.String("from", from),
		zap.String("to", to),
		zap.String("amount", amount.String()))

	return tx, nil
}

// GetTransaction retrieves a transaction by hash
func (m *BlockchainManager) GetTransaction(ctx context.Context, network, txHash string) (*Transaction, error) {
	// Check database first
	var tx Transaction
	result := m.db.WithContext(ctx).Where("hash = ? AND asset = ?", txHash, network).First(&tx)
	if result.Error == nil {
		return &tx, nil
	}

	// If not in database, query blockchain
	m.mu.RLock()
	client, exists := m.clients[network]
	m.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("client not found for network: %s", network)
	}

	blockchainTx, err := client.GetTransaction(ctx, txHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction from blockchain: %w", err)
	}

	// Save to database
	blockchainTx.ID = uuid.New().String()
	blockchainTx.Asset = network
	blockchainTx.CreatedAt = time.Now()
	blockchainTx.UpdatedAt = time.Now()

	if err := m.db.WithContext(ctx).Create(blockchainTx).Error; err != nil {
		m.logger.Error("Failed to save transaction",
			zap.String("hash", txHash),
			zap.Error(err))
	}

	return blockchainTx, nil
}

// GetTransactionHistory retrieves transaction history for an address
func (m *BlockchainManager) GetTransactionHistory(ctx context.Context, network, address string, limit int) ([]*Transaction, error) {
	// Check database first
	var dbTxs []*Transaction
	result := m.db.WithContext(ctx).Where("(from_address = ? OR to_address = ?) AND asset = ?", address, address, network).
		Order("created_at DESC").
		Limit(limit).
		Find(&dbTxs)

	if result.Error == nil && len(dbTxs) > 0 {
		return dbTxs, nil
	}

	// If not sufficient data in database, query blockchain
	m.mu.RLock()
	client, exists := m.clients[network]
	m.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("client not found for network: %s", network)
	}

	blockchainTxs, err := client.GetTransactionHistory(ctx, address, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction history from blockchain: %w", err)
	}

	// Save new transactions to database
	for _, tx := range blockchainTxs {
		var existingTx Transaction
		result := m.db.WithContext(ctx).Where("hash = ?", tx.Hash).First(&existingTx)
		if result.Error == gorm.ErrRecordNotFound {
			tx.ID = uuid.New().String()
			tx.Asset = network
			tx.CreatedAt = time.Now()
			tx.UpdatedAt = time.Now()

			if err := m.db.WithContext(ctx).Create(tx).Error; err != nil {
				m.logger.Error("Failed to save transaction",
					zap.String("hash", tx.Hash),
					zap.Error(err))
			}
		}
	}

	return blockchainTxs, nil
}

// ValidateAddress validates if an address is valid for the specified network
func (m *BlockchainManager) ValidateAddress(network, address string) bool {
	m.mu.RLock()
	client, exists := m.clients[network]
	m.mu.RUnlock()

	if !exists {
		return false
	}

	return client.ValidateAddress(address)
}

// estimateGasPrice estimates gas price for a transaction
func (m *BlockchainManager) estimateGasPrice(ctx context.Context, network string) (decimal.Decimal, error) {
	// Simple implementation - in production would query network for current gas prices
	networkConfig := m.config.Networks[network]
	minGasPrice, err := decimal.NewFromString(networkConfig.MinGasPrice)
	if err != nil {
		return decimal.NewFromFloat(20000000000), nil // 20 gwei default
	}

	// Apply multiplier
	gasPrice := minGasPrice.Mul(decimal.NewFromFloat(m.config.GasPriceMultiplier))

	maxGasPrice, err := decimal.NewFromString(networkConfig.MaxGasPrice)
	if err == nil && gasPrice.GreaterThan(maxGasPrice) {
		gasPrice = maxGasPrice
	}

	return gasPrice, nil
}

// MonitorTransactions monitors pending transactions for confirmations
func (m *BlockchainManager) MonitorTransactions(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.updatePendingTransactions(ctx)
		}
	}
}

// updatePendingTransactions updates the status of pending transactions
func (m *BlockchainManager) updatePendingTransactions(ctx context.Context) {
	var pendingTxs []*Transaction
	result := m.db.WithContext(ctx).Where("status = ?", "pending").Find(&pendingTxs)
	if result.Error != nil {
		m.logger.Error("Failed to get pending transactions", zap.Error(result.Error))
		return
	}

	for _, tx := range pendingTxs {
		m.mu.RLock()
		client, exists := m.clients[tx.Asset]
		m.mu.RUnlock()

		if !exists {
			continue
		}

		// Get updated transaction status
		updatedTx, err := client.GetTransaction(ctx, tx.Hash)
		if err != nil {
			m.logger.Error("Failed to get transaction update",
				zap.String("hash", tx.Hash),
				zap.Error(err))
			continue
		}

		// Update database if status changed
		if updatedTx.Status != tx.Status || updatedTx.Confirmations != tx.Confirmations {
			tx.Status = updatedTx.Status
			tx.Confirmations = updatedTx.Confirmations
			tx.BlockNumber = updatedTx.BlockNumber
			tx.BlockHash = updatedTx.BlockHash
			tx.UpdatedAt = time.Now()

			if updatedTx.Status == "confirmed" && tx.ConfirmedAt == nil {
				now := time.Now()
				tx.ConfirmedAt = &now
			}

			if err := m.db.WithContext(ctx).Save(tx).Error; err != nil {
				m.logger.Error("Failed to update transaction",
					zap.String("hash", tx.Hash),
					zap.Error(err))
			}

			m.logger.Info("Transaction status updated",
				zap.String("hash", tx.Hash),
				zap.String("status", tx.Status),
				zap.Int("confirmations", tx.Confirmations))
		}
	}
}

// Start starts the blockchain manager
func (m *BlockchainManager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return fmt.Errorf("blockchain manager is already running")
	}

	m.logger.Info("Starting blockchain manager")

	// Start transaction monitoring
	go m.MonitorTransactions(ctx)

	m.running = true
	m.logger.Info("Blockchain manager started successfully")

	return nil
}

// Stop stops the blockchain manager
func (m *BlockchainManager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return fmt.Errorf("blockchain manager is not running")
	}

	m.logger.Info("Stopping blockchain manager")
	m.running = false
	m.logger.Info("Blockchain manager stopped")

	return nil
}
