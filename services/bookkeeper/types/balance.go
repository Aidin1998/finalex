package types

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// BalanceKey represents the key for a specific currency balance
type BalanceKey struct {
	UserID    uuid.UUID `json:"userId"`
	AccountID uuid.UUID `json:"accountId"`
	Currency  string    `json:"currency"`
}

// Balance represents a currency balance with its metadata
type Balance struct {
	AccountID uuid.UUID
	Currency  string
	Amount    decimal.Decimal
}

type BalanceMap struct {
	m map[string]map[string]map[string]decimal.Decimal // userID -> accountID -> currency -> amount
}

func NewBalanceMap(m map[string]map[string]map[string]decimal.Decimal) *BalanceMap {
	if m == nil {
		m = make(map[string]map[string]map[string]decimal.Decimal)
	}
	return &BalanceMap{m}
}

func (m BalanceMap) Copy() *BalanceMap {
	newMap := make(map[string]map[string]map[string]decimal.Decimal)
	for userID, accounts := range m.m {
		newMap[userID] = make(map[string]map[string]decimal.Decimal)
		for accountID, currencies := range accounts {
			newMap[userID][accountID] = make(map[string]decimal.Decimal)
			for currency, amount := range currencies {
				newMap[userID][accountID][currency] = amount.Copy()
			}
		}
	}
	return &BalanceMap{newMap}
}

func (m BalanceMap) Set(key BalanceKey, amount decimal.Decimal) {
	userKey := key.UserID.String()
	accountKey := key.AccountID.String()

	if m.m[userKey] == nil {
		m.m[userKey] = make(map[string]map[string]decimal.Decimal)
	}
	if m.m[userKey][accountKey] == nil {
		m.m[userKey][accountKey] = make(map[string]decimal.Decimal)
	}
	m.m[userKey][accountKey][key.Currency] = amount
}

func (m BalanceMap) Get(key BalanceKey) *decimal.Decimal {
	userKey := key.UserID.String()
	accountKey := key.AccountID.String()

	accounts, ok := m.m[userKey]
	if !ok {
		return nil
	}

	currencies, ok := accounts[accountKey]
	if !ok {
		return nil
	}

	amount, ok := currencies[key.Currency]
	if !ok {
		return nil
	}

	return &amount
}

func (m BalanceMap) MustGet(key BalanceKey) decimal.Decimal {
	userKey := key.UserID.String()
	accountKey := key.AccountID.String()

	accounts, ok := m.m[userKey]
	if !ok {
		panic(fmt.Sprintf("user not found for userID: %s", key.UserID))
	}

	currencies, ok := accounts[accountKey]
	if !ok {
		panic(fmt.Sprintf("account not found for userID: %s, accountID: %s", key.UserID, key.AccountID))
	}

	amount, ok := currencies[key.Currency]
	if !ok {
		panic(fmt.Sprintf("amount not found for userID: %s, accountID: %s, currency: %s", key.UserID, key.AccountID, key.Currency))
	}

	return amount
}

// GetAll returns all currency balances for a specific user and account
func (m BalanceMap) GetAll(userID uuid.UUID) map[uuid.UUID]map[string]decimal.Decimal {
	userKey := userID.String()

	accounts, ok := m.m[userKey]
	if !ok {
		return nil
	}

	result := make(map[uuid.UUID]map[string]decimal.Decimal)
	for accountIDStr, currencies := range accounts {
		accountID, _ := uuid.Parse(accountIDStr)
		result[accountID] = make(map[string]decimal.Decimal)
		for currency, amount := range currencies {
			result[accountID][currency] = amount
		}
	}
	return result
}
