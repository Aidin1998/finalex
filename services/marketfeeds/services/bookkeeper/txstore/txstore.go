package txstore

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
	Create(ctx context.Context, tx *Transaction) error
	Query(ctx context.Context, opts Filter) (QueryResult, error)
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

// Create stores a new transaction in DynamoDB with idempotency check
func (s *StoreImp) Create(ctx context.Context, tx *Transaction) error {
	// Set the sort key for transaction
	tx.SK = fmt.Sprintf("TX#%d", tx.Sequence)

	// Create idempotency record
	idempotencyRecord := IdempotencyRecord{
		UserID:        tx.UserID,
		SK:            fmt.Sprintf("IDEMPOTENCY#%s", tx.ID.String()),
		TransactionID: tx.ID,
		Sequence:      tx.Sequence,
	}

	// Marshal transaction and idempotency record
	txItem, err := attributevalue.MarshalMap(tx)
	if err != nil {
		return errors.New("failed to marshal transaction").Wrap(err).Trace()
	}

	idempotencyItem, err := attributevalue.MarshalMap(idempotencyRecord)
	if err != nil {
		return errors.New("failed to marshal idempotency record").Wrap(err).Trace()
	}

	// Create condition expression to check if idempotency record doesn't exist
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

// Query retrieves transactions based on the provided filter options
func (s *StoreImp) Query(ctx context.Context, filter Filter) (QueryResult, error) {
	// Key expressions for partition key
	keyExpr := expression.Key("user_id").Equal(expression.Value(filter.UserID.String()))

	// Add sort key begins_with condition to only get transaction records
	keyExpr = keyExpr.And(expression.Key("sk").BeginsWith("TX#"))

	// Add filter for sequence if needed
	var filterExpr expression.ConditionBuilder
	if filter.AfterSequence > 0 {
		filterExpr = expression.Name("sequence").GreaterThan(expression.Value(filter.AfterSequence))
	}

	// Build expression
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
