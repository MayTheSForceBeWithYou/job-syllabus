package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// bedrockCacheTTL is docs/design.md §6 Stage 4's "cache by sha256(bullet_text)
// ... with a 90-day TTL" — re-running extraction across the corpus (e.g. a
// reextract job triggered by a skills.yaml/prompt version bump) should cost
// ~nothing the second time for bullets it's already classified.
const bedrockCacheTTL = 90 * 24 * time.Hour

// BedrockFinding is the content-derived half of one Bedrock classification
// result for a single bullet — deliberately excludes isRequired: that's a
// property of *which posting section this bullet occurred in*, not of the
// bullet text itself, so it can't be safely cached across postings (see
// internal/bedrock's schema comment). The caller applies its own
// section-derived Required flag when turning this into a PostingSkill edge.
type BedrockFinding struct {
	Term       string  `dynamodbav:"term"`
	Category   string  `dynamodbav:"category"`
	Evidence   string  `dynamodbav:"evidence"`
	Confidence float32 `dynamodbav:"confidence"`
}

// HashBullet is the cache key function named directly in docs/design.md §6.
func HashBullet(bulletText string) string {
	sum := sha256.Sum256([]byte(bulletText))
	return hex.EncodeToString(sum[:])
}

func bedrockCachePK(bulletHash string) string { return "CACHE#" + bulletHash }

type bedrockCacheItem struct {
	PK       string           `dynamodbav:"PK"`
	SK       string           `dynamodbav:"SK"`
	Findings []BedrockFinding `dynamodbav:"findings"`
	TTL      int64            `dynamodbav:"ttl"`
}

// GetBedrockCache looks up a previously-classified bullet by content hash.
// hit=false covers both "never classified" and "TTL-expired" — DynamoDB
// lazily removes expired items (up to 48h after expiry per AWS docs), so a
// stale-but-not-yet-swept item is deliberately not special-cased here: it's
// rare, and worst case is one avoidable cache hit re-running Bedrock, not a
// correctness bug.
func (s *Store) GetBedrockCache(ctx context.Context, bulletHash string) (findings []BedrockFinding, hit bool, err error) {
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(TableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: bedrockCachePK(bulletHash)},
			"SK": &types.AttributeValueMemberS{Value: "META"},
		},
	})
	if err != nil {
		return nil, false, fmt.Errorf("get bedrock cache %s: %w", bulletHash, err)
	}
	if out.Item == nil {
		return nil, false, nil
	}
	var item bedrockCacheItem
	if err := attributevalue.UnmarshalMap(out.Item, &item); err != nil {
		return nil, false, fmt.Errorf("unmarshal bedrock cache %s: %w", bulletHash, err)
	}
	return item.Findings, true, nil
}

// PutBedrockCache records a bullet's classification result — possibly an
// empty slice, which is itself a meaningful, cacheable answer ("Bedrock
// looked at this and found no skill term"), not "no cache entry". Plain
// PutItem: two workers racing to classify the same bullet text (e.g. a
// common phrase like "Excellent communication skills" appearing in two
// postings processed concurrently) would just overwrite with equivalent
// content — harmless, not worth a conditional write.
func (s *Store) PutBedrockCache(ctx context.Context, bulletHash string, findings []BedrockFinding) error {
	item := bedrockCacheItem{
		PK:       bedrockCachePK(bulletHash),
		SK:       "META",
		Findings: findings,
		TTL:      time.Now().Add(bedrockCacheTTL).Unix(),
	}
	av, err := attributevalue.MarshalMap(item)
	if err != nil {
		return fmt.Errorf("marshal bedrock cache %s: %w", bulletHash, err)
	}
	if _, err := s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(TableName),
		Item:      av,
	}); err != nil {
		return fmt.Errorf("put bedrock cache %s: %w", bulletHash, err)
	}
	return nil
}
