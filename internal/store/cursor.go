package store

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// EncodeCursor/DecodeCursor turn a DynamoDB LastEvaluatedKey into the
// opaque cursor docs/design.md §7 calls for ("base64 of the DynamoDB
// LastEvaluatedKey, never offset"). Every key attribute in this table
// (PK/SK/GSI1PK/GSI1SK/GSI2PK/GSI2SK) is a plain string (S), so this
// converts explicitly via type assertion rather than reaching for
// attributevalue's generic (reflection-based) map decoding — if a future
// GSI ever used a non-string key, this needs to grow with it.
func EncodeCursor(key map[string]types.AttributeValue) (string, error) {
	if len(key) == 0 {
		return "", nil
	}
	plain := make(map[string]string, len(key))
	for k, av := range key {
		s, ok := av.(*types.AttributeValueMemberS)
		if !ok {
			return "", fmt.Errorf("cursor key %q is not a string attribute (got %T)", k, av)
		}
		plain[k] = s.Value
	}
	raw, err := json.Marshal(plain)
	if err != nil {
		return "", fmt.Errorf("marshal cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// DecodeCursor is EncodeCursor's inverse. An empty cursor decodes to a nil
// key (i.e. "start from the beginning"), matching DynamoDB's own
// ExclusiveStartKey semantics.
func DecodeCursor(cursor string) (map[string]types.AttributeValue, error) {
	if cursor == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return nil, fmt.Errorf("decode cursor: %w", err)
	}
	var plain map[string]string
	if err := json.Unmarshal(raw, &plain); err != nil {
		return nil, fmt.Errorf("unmarshal cursor: %w", err)
	}
	key := make(map[string]types.AttributeValue, len(plain))
	for k, v := range plain {
		key[k] = &types.AttributeValueMemberS{Value: v}
	}
	return key, nil
}
