package store

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
	"github.com/litebittech/cex/common/errors"
)

type Filter struct {
	UserID        uuid.UUID
	Limit         int
	AfterSequence int
}

type QueryResult struct {
	Transactions []Transaction
	LastSequence int
}

type Store interface {
	CreateTransaction(ctx context.Context, tx *Transaction) error
	QueryTransactions(ctx context.Context, opts Filter) (QueryResult, error)
	CreateAccount(ctx context.Context, account *Account) error
	GetUserAccount(ctx context.Context, userID uuid.UUID, accountID uuid.UUID) (*Account, error)
	GetUserAccounts(ctx context.Context, userID uuid.UUID, accountIDs []uuid.UUID) ([]Account, error)
}

type StoreImp struct {
	client    *dynamodb.Client
	tableName string
}

// NewStore creates a new DynamoDB transaction store
func NewStore(client *dynamodb.Client, tableName string) Store {
	return &StoreImp{
		client:    client,
		tableName: tableName,
	}
}

// CreateTransaction stores a new transaction in DynamoDB with idempotency check
func (s *StoreImp) CreateTransaction(ctx context.Context, tx *Transaction) error {
	tx.SK = fmt.Sprintf("TX#%d", tx.Sequence)

	idempotencyRecord := IdempotencyRecord{
		UserID:        tx.UserID,
		SK:            fmt.Sprintf("IDEMPOTENCY#%s", tx.ID.String()),
		TransactionID: tx.ID,
		Sequence:      tx.Sequence,
	}

	txItem, err := attributevalue.MarshalMap(tx)
	if err != nil {
		return errors.New("failed to marshal transaction").Wrap(err).Trace()
	}
	idempotencyItem, err := attributevalue.MarshalMap(idempotencyRecord)
	if err != nil {
		return errors.New("failed to marshal idempotency record").Wrap(err).Trace()
	}

	condExpr := expression.AttributeNotExists(expression.Name("user_id")).And(
		expression.AttributeNotExists(expression.Name("sk")))

	expr, err := expression.NewBuilder().WithCondition(condExpr).Build()
	if err != nil {
		return errors.New("failed to build condition expression").Wrap(err).Trace()
	}

	// Use TransactWriteItems to ensure atomicity
	_, err = s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{
			{
				Put: &types.Put{
					TableName:                 aws.String(s.tableName),
					Item:                      idempotencyItem,
					ConditionExpression:       expr.Condition(),
					ExpressionAttributeNames:  expr.Names(),
					ExpressionAttributeValues: expr.Values(),
				},
			},
			{
				Put: &types.Put{
					TableName: aws.String(s.tableName),
					Item:      txItem,
				},
			},
		},
	})
	if err != nil {
		// Check if this is a conditional check failure (idempotency record already exists)
		var txConditionalCheckFailed *types.TransactionCanceledException
		if errors.As(err, &txConditionalCheckFailed) {
			for _, reason := range txConditionalCheckFailed.CancellationReasons {
				if *reason.Code == "ConditionalCheckFailed" {
					return errors.Conflict.Explain("transaction already processed")
				}
			}
		}
		return errors.New("failed to store transaction").Wrap(err).Trace()
	}

	return nil
}

// CreateAccount stores a new account in DynamoDB
func (s *StoreImp) CreateAccount(ctx context.Context, account *Account) error {
	account.SK = fmt.Sprintf("ACC#%s", account.ID.String())

	accountItem, err := attributevalue.MarshalMap(account)
	if err != nil {
		return errors.New("failed to marshal account").Wrap(err).Trace()
	}

	// Create condition expression to check if account doesn't exist
	condExpr := expression.AttributeNotExists(expression.Name("user_id")).And(
		expression.AttributeNotExists(expression.Name("sk")))

	expr, err := expression.NewBuilder().WithCondition(condExpr).Build()
	if err != nil {
		return errors.New("failed to build condition expression").Wrap(err).Trace()
	}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:                 aws.String(s.tableName),
		Item:                      accountItem,
		ConditionExpression:       expr.Condition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	})
	if err != nil {
		var conditionalCheckFailed *types.ConditionalCheckFailedException
		if errors.As(err, &conditionalCheckFailed) {
			return errors.Conflict.Explain("account already exists")
		}
		return errors.New("failed to store account").Wrap(err).Trace()
	}

	return nil
}

// GetAccount retrieves an account by userID and accountID
func (s *StoreImp) GetUserAccount(ctx context.Context, userID, accountID uuid.UUID) (*Account, error) {
	sk := fmt.Sprintf("ACC#%s", accountID.String())

	keyExpr := expression.Key("user_id").Equal(expression.Value(userID.String())).And(
		expression.Key("sk").Equal(expression.Value(sk)))

	expr, err := expression.NewBuilder().WithKeyCondition(keyExpr).Build()
	if err != nil {
		return nil, errors.New("failed to build expression").Wrap(err).Trace()
	}

	result, err := s.client.Query(ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(s.tableName),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		KeyConditionExpression:    expr.KeyCondition(),
		Limit:                     aws.Int32(1),
	})
	if err != nil {
		return nil, errors.New("failed to query account").Wrap(err).Trace()
	}

	if len(result.Items) == 0 {
		return nil, errors.NotFound.Explain("account not found")
	}

	var account Account
	if err := attributevalue.UnmarshalMap(result.Items[0], &account); err != nil {
		return nil, errors.New("failed to unmarshal account").Wrap(err).Trace()
	}

	return &account, nil
}

// GetUserAccounts retrieves accounts for a specific user, filtered by accountIDs if provided
func (s *StoreImp) GetUserAccounts(ctx context.Context, userID uuid.UUID, accountIDs []uuid.UUID) ([]Account, error) {
	keyExpr := expression.Key("user_id").Equal(expression.Value(userID.String())).
		And(expression.Key("sk").BeginsWith("ACC#"))
	builder := expression.NewBuilder().WithKeyCondition(keyExpr)

	var skValues []string
	for _, id := range accountIDs {
		skValues = append(skValues, fmt.Sprintf("ACC#%s", id.String()))
	}
	builder = builder.WithFilter(expression.Name("sk").In(expression.Value(skValues)))

	expr, err := builder.Build()
	if err != nil {
		return nil, errors.New("failed to build expression").Wrap(err).Trace()
	}

	input := &dynamodb.QueryInput{
		TableName:                 aws.String(s.tableName),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		KeyConditionExpression:    expr.KeyCondition(),
		FilterExpression:          expr.Filter(),
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, errors.New("failed to query accounts").Wrap(err).Trace()
	}

	if len(result.Items) == 0 {
		return []Account{}, nil
	}

	var accounts []Account
	if err := attributevalue.UnmarshalListOfMaps(result.Items, &accounts); err != nil {
		return nil, errors.New("failed to unmarshal accounts").Wrap(err).Trace()
	}

	return accounts, nil
}

// QueryTransactions retrieves transactions based on the provided filter options
func (s *StoreImp) QueryTransactions(ctx context.Context, filter Filter) (QueryResult, error) {
	keyExpr := expression.Key("user_id").Equal(expression.Value(filter.UserID.String())).And(expression.Key("sk").BeginsWith("TX#"))

	// Add filter for sequence if needed
	var filterExpr expression.ConditionBuilder
	if filter.AfterSequence > 0 {
		filterExpr = expression.Name("sequence").GreaterThan(expression.Value(filter.AfterSequence))
	}

	builder := expression.NewBuilder().WithKeyCondition(keyExpr)
	if filter.AfterSequence > 0 {
		builder = builder.WithFilter(filterExpr)
	}

	expr, err := builder.Build()
	if err != nil {
		return QueryResult{}, errors.New("failed to build expression").Wrap(err).Trace()
	}

	input := &dynamodb.QueryInput{
		TableName:                 aws.String(s.tableName),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		KeyConditionExpression:    expr.KeyCondition(),
	}

	if filter.AfterSequence > 0 {
		input.FilterExpression = expr.Filter()
	}

	if filter.Limit > 0 {
		input.Limit = aws.Int32(int32(filter.Limit))
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return QueryResult{}, errors.New("failed to query transactions").Wrap(err).Trace()
	}

	var transactions []Transaction
	if err := attributevalue.UnmarshalListOfMaps(result.Items, &transactions); err != nil {
		return QueryResult{}, errors.New("failed to unmarshal transactions").Wrap(err).Trace()
	}

	lastSequence := filter.AfterSequence
	if len(transactions) > 0 {
		lastSequence = transactions[len(transactions)-1].Sequence
	}

	return QueryResult{
		Transactions: transactions,
		LastSequence: lastSequence,
	}, nil
}
