package wallet

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

// EVMAdapter implements CustodyProvider for EVM-compatible chains (e.g., Ethereum, L2s)
type EVMAdapter struct {
	client *ethclient.Client
	// In production, never store private keys in memory! Use HSM/KMS.
	privateKeys map[string]*ecdsa.PrivateKey // walletID -> key
}

func NewEVMAdapter(rpcURL string) (*EVMAdapter, error) {
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		return nil, err
	}
	return &EVMAdapter{
		client:      client,
		privateKeys: make(map[string]*ecdsa.PrivateKey),
	}, nil
}

func (e *EVMAdapter) CreateWallet(asset string) (string, error) {
	// Generate a new ECDSA key (for demo; use HSM in prod)
	key, err := crypto.GenerateKey()
	if err != nil {
		return "", err
	}
	address := crypto.PubkeyToAddress(key.PublicKey).Hex()
	e.privateKeys[address] = key
	return address, nil
}

func (e *EVMAdapter) GetBalance(walletID string) (float64, error) {
	addr := common.HexToAddress(walletID)
	bal, err := e.client.BalanceAt(context.Background(), addr, nil)
	if err != nil {
		return 0, err
	}
	// Return ETH balance in Ether
	fbal := new(big.Float).Quo(new(big.Float).SetInt(bal), big.NewFloat(1e18))
	val, _ := fbal.Float64()
	return val, nil
}

func (e *EVMAdapter) CreateWithdrawal(walletID, toAddress string, amount float64) (string, error) {
	key, ok := e.privateKeys[walletID]
	if !ok {
		return "", errors.New("private key not found")
	}
	from := common.HexToAddress(walletID)
	to := common.HexToAddress(toAddress)
	// Get nonce
	nonce, err := e.client.PendingNonceAt(context.Background(), from)
	if err != nil {
		return "", err
	}
	// Set gas price and limit
	gasPrice, err := e.client.SuggestGasPrice(context.Background())
	if err != nil {
		return "", err
	}
	value := new(big.Int)
	value.SetString(big.NewFloat(amount*1e18).Text('f', 0), 10)
	// Create transaction
	tx := types.NewTransaction(nonce, to, value, 21000, gasPrice, nil)
	chainID, err := e.client.NetworkID(context.Background())
	if err != nil {
		return "", err
	}
	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(chainID), key)
	if err != nil {
		return "", err
	}
	err = e.client.SendTransaction(context.Background(), signedTx)
	if err != nil {
		return "", err
	}
	return signedTx.Hash().Hex(), nil
}

func (e *EVMAdapter) GetTransactionStatus(txID string) (string, error) {
	hash := common.HexToHash(txID)
	receipt, err := e.client.TransactionReceipt(context.Background(), hash)
	if err != nil {
		return "pending", nil // Not found = pending
	}
	if receipt.Status == 1 {
		return "confirmed", nil
	}
	return "failed", nil
}
