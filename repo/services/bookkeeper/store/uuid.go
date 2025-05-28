package store

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
)

// UUID wraps uuid.UUID to provide custom marshaling/unmarshaling
type UUID struct {
	uuid.UUID
}

func NewUUID(id uuid.UUID) UUID {
	return UUID{id}
}

// MarshalDynamoDBAttributeValue implements attributevalue.Marshaler.
func (u UUID) MarshalDynamoDBAttributeValue() (types.AttributeValue, error) {
	return &types.AttributeValueMemberS{Value: u.String()}, nil
}

// UnmarshalDynamoDBAttributeValue implements attributevalue.Unmarshaler.
func (u *UUID) UnmarshalDynamoDBAttributeValue(av types.AttributeValue) error {
	s, ok := av.(*types.AttributeValueMemberS)
	if !ok {
		return fmt.Errorf("expected string AttributeValue, got %T", av)
	}

	// Trim any whitespace
	value := strings.TrimSpace(s.Value)

	// Parse the UUID
	id, err := uuid.Parse(value)
	if err != nil {
		return fmt.Errorf("invalid UUID value %q: %w", value, err)
	}

	*u = UUID{id}
	return nil
}
