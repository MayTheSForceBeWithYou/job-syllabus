package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// dedupTTL is docs/design.md §5's 30-day window for collapsing the same
// job cross-posted under two different URLs into one posting.
const dedupTTL = 30 * 24 * time.Hour

func dedupPK(contentHash string) string { return "DEDUP#" + contentHash }

// ClaimContentHash atomically records that contentHash has been seen,
// returning true if this is the first time (the caller should keep
// whatever posting produced it) or false if some other posting already
// claimed the same content (the caller should treat this one as a
// cross-posted duplicate). The marker expires after 30 days via the
// table's `ttl` attribute (modules/data/dynamodb.tf), so a genuinely new
// posting that happens to reuse old, expired content text isn't
// permanently blocked.
func (s *Store) ClaimContentHash(ctx context.Context, contentHash string) (firstSeen bool, err error) {
	item := map[string]types.AttributeValue{
		"PK":  &types.AttributeValueMemberS{Value: dedupPK(contentHash)},
		"SK":  &types.AttributeValueMemberS{Value: "META"},
		"ttl": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", time.Now().Add(dedupTTL).Unix())},
	}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(TableName),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(PK)"),
	})
	if err == nil {
		return true, nil
	}

	var condFailed *types.ConditionalCheckFailedException
	if errors.As(err, &condFailed) {
		return false, nil
	}
	return false, fmt.Errorf("claim content hash: %w", err)
}
