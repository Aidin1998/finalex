package store

import (
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/shopspring/decimal"
)

type Account struct {
	ID          UUID                   `dynamodbav:"id"`
	UserID      UUID                   `dynamodbav:"user_id"`
	SK          string                 `dynamodbav:"sk"`
	Type        string                 `dynamodbav:"type"`
	CreatedAt   UnixMilli              `dynamodbav:"created_at"`
	UpdatedAt   UnixMilli              `dynamodbav:"updated_at"`
	Description string                 `dynamodbav:"description"`
	Metadata    map[string]interface{} `dynamodbav:"metadata"`
}

// TransactionType represents the type of transaction
type TransactionType string

const (
	DepositTransaction    TransactionType = "deposit"
	WithdrawalTransaction TransactionType = "withdrawal"
	SpotTradeTransaction  TransactionType = "trade"
	AdjustmentTransaction TransactionType = "adjustment"
)

type Transaction struct {
	ID          UUID            `dynamodbav:"id"`
	UserID      UUID            `dynamodbav:"user_id"`
	SK          string          `dynamodbav:"sk"`
	Type        TransactionType `dynamodbav:"type"`
	Date        UnixMilli       `dynamodbav:"date"`
	Description string          `dynamodbav:"description"`
	Entries     Entries         `dynamodbav:"entries"`
	CreatedAt   UnixMilli       `dynamodbav:"created_at"`
	Reconciled  bool            `dynamodbav:"recondiled"`
	Sequence    int             `dynamodbav:"sequence"`

	Metadata map[string]interface{} `dynamodbav:"metadata"`
}

// IdempotencyRecord represents a record to ensure transaction idempotency
type IdempotencyRecord struct {
	UserID        UUID   `dynamodbav:"user_id"`
	SK            string `dynamodbav:"sk"`
	TransactionID UUID   `dynamodbav:"transaction_id"`
	Sequence      int    `dynamodbav:"sequence"`
}

type EntryType string

func (e EntryType) SignForLiabilty() int {
	if e.IsDebit() {
		return -1
	}
	return 1
}

func (e EntryType) IsDebit() bool {
	return e == DebitEntry
}

func (e EntryType) IsCredit() bool {
	return e == CreditEntry
}

const (
	DebitEntry  EntryType = "debit"
	CreditEntry EntryType = "credit"
)

type Entry struct {
	ID              UUID            `json:"id"`
	AccountID       UUID            `json:"account_id"`
	Type            EntryType       `json:"type"`
	Amount          decimal.Decimal `json:"amount"`
	IsSystemAccount bool            `json:"is_system_account"`
	Currency        string          `json:"currency"`
	CreatedAt       UnixMilli       `json:"created_at"`
}

func (e Entry) IsDebit() bool {
	return e.Type.IsDebit()
}

func (e Entry) IsCredit() bool {
	return e.Type.IsCredit()
}

type Entries struct {
	entries []Entry
}

func NewEntries(entries []Entry) Entries {
	return Entries{entries}
}

func (e Entries) Slice() []Entry {
	return e.entries
}

func (e Entries) MarshalDynamoDBAttributeValue() (types.AttributeValue, error) {
	entriesJSON, err := json.Marshal(e.Slice())
	if err != nil {
		return nil, err
	}

	return &types.AttributeValueMemberS{Value: string(entriesJSON)}, nil
}

func (e *Entries) UnmarshalDynamoDBAttributeValue(av types.AttributeValue) error {
	avStr, ok := av.(*types.AttributeValueMemberS)
	if !ok {
		return fmt.Errorf("expected string AttributeValue, got %T", av)
	}

	var entries []Entry
	if err := json.Unmarshal([]byte(avStr.Value), &entries); err != nil {
		return err
	}

	*e = Entries{entries}
	return nil
}
