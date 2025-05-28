package wallet

import "errors"

// KeyManager abstracts key management (HSM, KMS, etc.)
type KeyManager interface {
	GenerateKey(label string) (string, error)
	SignTransaction(keyID string, txData []byte) ([]byte, error)
	GetPublicKey(keyID string) ([]byte, error)
}

// CustodyProvider abstracts external custody providers (Fireblocks, BitGo, etc.)
type CustodyProvider interface {
	CreateWallet(asset string) (string, error)
	GetBalance(walletID string) (float64, error)
	CreateWithdrawal(walletID, toAddress string, amount float64) (string, error)
	GetTransactionStatus(txID string) (string, error)
}

// InMemoryKeyManager is a simple in-memory key manager for demo/testing
// In production, use HSM/KMS-backed implementation

type InMemoryKeyManager struct {
	keys map[string][]byte
}

func NewInMemoryKeyManager() *InMemoryKeyManager {
	return &InMemoryKeyManager{keys: make(map[string][]byte)}
}

func (m *InMemoryKeyManager) GenerateKey(label string) (string, error) {
	keyID := label + "-key"
	m.keys[keyID] = []byte("dummy-private-key")
	return keyID, nil
}

func (m *InMemoryKeyManager) SignTransaction(keyID string, txData []byte) ([]byte, error) {
	if _, ok := m.keys[keyID]; !ok {
		return nil, errors.New("key not found")
	}
	return []byte("signed-" + string(txData)), nil
}

func (m *InMemoryKeyManager) GetPublicKey(keyID string) ([]byte, error) {
	if _, ok := m.keys[keyID]; !ok {
		return nil, errors.New("key not found")
	}
	return []byte("dummy-public-key"), nil
}

// DummyCustodyProvider is a stub for external custody integration

type DummyCustodyProvider struct {
	balances map[string]float64
}

func NewDummyCustodyProvider() *DummyCustodyProvider {
	return &DummyCustodyProvider{balances: make(map[string]float64)}
}

func (c *DummyCustodyProvider) CreateWallet(asset string) (string, error) {
	walletID := asset + "-wallet"
	c.balances[walletID] = 0
	return walletID, nil
}

func (c *DummyCustodyProvider) GetBalance(walletID string) (float64, error) {
	bal, ok := c.balances[walletID]
	if !ok {
		return 0, errors.New("wallet not found")
	}
	return bal, nil
}

func (c *DummyCustodyProvider) CreateWithdrawal(walletID, toAddress string, amount float64) (string, error) {
	if c.balances[walletID] < amount {
		return "", errors.New("insufficient funds")
	}
	c.balances[walletID] -= amount
	return "txid-123", nil
}

func (c *DummyCustodyProvider) GetTransactionStatus(txID string) (string, error) {
	return "confirmed", nil
}
