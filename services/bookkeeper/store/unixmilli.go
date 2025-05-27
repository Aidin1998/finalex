package store

import (
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type UnixMilli struct {
	// TODO: Maybe int64 is safer choice, for now time.Time is used for ease of use.
	time.Time
}

func NewUnixMilli(t time.Time) UnixMilli {
	return UnixMilli{t}
}

// MarshalJSON implements json.Marshaler.
// It writes the time as a string of Unix milliseconds.
func (t UnixMilli) MarshalJSON() ([]byte, error) {
	ms := t.UnixMilli()
	return []byte(strconv.FormatInt(ms, 10)), nil
}

// UnmarshalJSON implements json.Unmarshaler.
// It parses a millisecond string or number back into time.Time.
func (t *UnixMilli) UnmarshalJSON(data []byte) error {
	var ms int64

	// Try to parse as string first
	s := string(data)
	if len(s) > 0 && s[0] == '"' && s[len(s)-1] == '"' {
		// Remove quotes
		s = s[1 : len(s)-1]
		var err error
		ms, err = strconv.ParseInt(s, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid unix-milli string value %q: %w", s, err)
		}
	} else {
		// Try to parse as number
		var err error
		ms, err = strconv.ParseInt(s, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid unix-milli numeric value %q: %w", s, err)
		}
	}

	*t = UnixMilli{time.Unix(0, ms*int64(time.Millisecond))}
	return nil
}

// MarshalDynamoDBAttributeValue implements attributevalue.Marshaler.
// It writes the time as a string of Unix milliseconds.
func (t UnixMilli) MarshalDynamoDBAttributeValue() (types.AttributeValue, error) {
	ms := t.UnixMilli()
	return &types.AttributeValueMemberS{Value: strconv.FormatInt(ms, 10)}, nil
}

// UnmarshalDynamoDBAttributeValue implements attributevalue.Unmarshaler.
// It parses a millisecond string back into time.Time.
func (t *UnixMilli) UnmarshalDynamoDBAttributeValue(av types.AttributeValue) error {
	s, ok := av.(*types.AttributeValueMemberS)
	if !ok {
		return fmt.Errorf("expected string AttributeValue, got %T", av)
	}
	ms, err := strconv.ParseInt(s.Value, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid unix-milli value %q: %w", s.Value, err)
	}
	*t = UnixMilli{time.Unix(0, ms*int64(time.Millisecond))}
	return nil
}
