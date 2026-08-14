package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/model"
)

// postingItem is the DynamoDB item shape for a Posting: model fields plus
// the single-table key attributes from docs/design.md §4.
type postingItem struct {
	PK         string `dynamodbav:"PK"`
	SK         string `dynamodbav:"SK"`
	GSI1PK     string `dynamodbav:"GSI1PK"`
	GSI1SK     string `dynamodbav:"GSI1SK"`
	GSI2PK     string `dynamodbav:"GSI2PK"`
	GSI2SK     string `dynamodbav:"GSI2SK"`
	EntityType string `dynamodbav:"entityType"`
	model.Posting
}

func postingPK(id string) string { return "POSTING#" + id }

func toPostingItem(p model.Posting) postingItem {
	return postingItem{
		PK:         postingPK(p.ID),
		SK:         "META",
		GSI1PK:     "COMPANY#" + p.CompanySlug,
		GSI1SK:     "POSTED#" + p.PostedAt.UTC().Format(time.RFC3339),
		GSI2PK:     "ROLE#" + string(p.RoleFamily),
		GSI2SK:     "POSTED#" + p.PostedAt.UTC().Format(time.RFC3339),
		EntityType: "posting",
		Posting:    p,
	}
}

// GetPosting returns a posting by ID, or (nil, nil) if it doesn't exist.
func (s *Store) GetPosting(ctx context.Context, id string) (*model.Posting, error) {
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(TableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: postingPK(id)},
			"SK": &types.AttributeValueMemberS{Value: "META"},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("get posting %s: %w", id, err)
	}
	if out.Item == nil {
		return nil, nil
	}

	var item postingItem
	if err := attributevalue.UnmarshalMap(out.Item, &item); err != nil {
		return nil, fmt.Errorf("unmarshal posting %s: %w", id, err)
	}
	return &item.Posting, nil
}

// PutPosting upserts a posting. Callers decide FirstSeenAt/LastSeenAt
// semantics (see UpsertPosting for the dedup-aware version cmd/ingest uses).
func (s *Store) PutPosting(ctx context.Context, p model.Posting) error {
	av, err := attributevalue.MarshalMap(toPostingItem(p))
	if err != nil {
		return fmt.Errorf("marshal posting %s: %w", p.ID, err)
	}
	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(TableName),
		Item:      av,
	})
	if err != nil {
		return fmt.Errorf("put posting %s: %w", p.ID, err)
	}
	return nil
}

// UpsertPosting implements the ingest dedup rule from docs/design.md §5: a
// posting seen before keeps its FirstSeenAt and just advances LastSeenAt
// (and ContentHash/ExtractVer if the content changed); a new posting is
// inserted as-is. Returns the final stored posting (so callers can layer
// extraction results — ExtractVer, SkillCount — on top of the correct
// FirstSeenAt/etc. rather than the caller's pre-upsert draft) and whether
// this was a newly-created posting.
func (s *Store) UpsertPosting(ctx context.Context, p model.Posting) (stored model.Posting, created bool, err error) {
	existing, err := s.GetPosting(ctx, p.ID)
	if err != nil {
		return model.Posting{}, false, err
	}
	if existing == nil {
		p.FirstSeenAt = p.LastSeenAt
		if err := s.PutPosting(ctx, p); err != nil {
			return model.Posting{}, false, err
		}
		return p, true, nil
	}

	existing.LastSeenAt = p.LastSeenAt
	existing.Title = p.Title
	existing.Location = p.Location
	existing.URL = p.URL
	if existing.ContentHash != p.ContentHash {
		existing.ContentHash = p.ContentHash
		existing.ExtractVer = 0 // force re-extraction; content changed
	}
	if err := s.PutPosting(ctx, *existing); err != nil {
		return model.Posting{}, false, err
	}
	return *existing, false, nil
}

// ErrNotFound is returned by lookups that expect the item to exist.
var ErrNotFound = errors.New("not found")
