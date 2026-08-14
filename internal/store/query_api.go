// Read-query methods added for cmd/api (docs/design.md §7, Phase 3). These
// sit alongside the Scan-based ListAllPostings/ListAllSkillEdges rather
// than replacing them — cmd/ingest's report still wants the whole corpus.
package store

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/model"
)

// Ping does a cheap DescribeTable call to confirm the table is reachable
// and ACTIVE — GET /readyz's dependency check, so a broken DynamoDB
// connection fails readiness instead of the ALB routing traffic to a task
// that can't actually serve anything.
func (s *Store) Ping(ctx context.Context) error {
	out, err := s.client.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(TableName),
	})
	if err != nil {
		return fmt.Errorf("describe table: %w", err)
	}
	if out.Table.TableStatus != types.TableStatusActive {
		return fmt.Errorf("table status is %s, not ACTIVE", out.Table.TableStatus)
	}
	return nil
}

const (
	defaultPostingsLimit = 25
	maxPostingsLimit     = 100
)

// PostingFilter is GET /v1/postings' query params (docs/design.md §7):
// company, roleFamily, since, cursor, limit.
type PostingFilter struct {
	Company    string
	RoleFamily string
	Since      time.Time // zero value = no filter
	Cursor     string
	Limit      int32 // 0 = defaultPostingsLimit
}

// PostingPage is one page of ListPostings.
type PostingPage struct {
	Postings   []model.Posting
	NextCursor string
}

// ListPostings returns one page of postings matching filter, via Scan +
// FilterExpression rather than a GSI query — the same "fine for a
// few-hundred-item corpus" tradeoff ListAllPostings already documents. A
// GSI1 (company) / GSI2 (roleFamily) query replaces this once the corpus
// is large enough for Scan-with-filter's read cost to matter
// (docs/design.md Phase 4, "Ingestion at scale").
//
// `since` is applied in Go after unmarshaling, not as part of the
// DynamoDB FilterExpression: attributevalue's time.Time encoding doesn't
// guarantee a fixed-width fractional-second format, so a hand-formatted
// RFC3339 comparison value could sort incorrectly against it (e.g.
// ".123Z" sorts lexicographically before "Z" despite being chronologically
// later at the same whole second). Comparing real parsed time.Time values
// avoids that class of bug entirely.
//
// Cursor pagination is a direct passthrough of DynamoDB's own
// LastEvaluatedKey (§7: "never offset"), so a page can legitimately
// return fewer than Limit items — even zero — while still carrying a
// NextCursor: DynamoDB's Limit bounds items *scanned* per page, not items
// that end up matching every filter (including the Go-side `since` one).
// Callers must keep paging on NextCursor being non-empty, not on the
// current page being non-empty.
func (s *Store) ListPostings(ctx context.Context, filter PostingFilter) (PostingPage, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = defaultPostingsLimit
	}
	if limit > maxPostingsLimit {
		limit = maxPostingsLimit
	}

	startKey, err := DecodeCursor(filter.Cursor)
	if err != nil {
		return PostingPage{}, fmt.Errorf("decode cursor: %w", err)
	}

	cond := expression.Name("entityType").Equal(expression.Value("posting"))
	if filter.Company != "" {
		cond = cond.And(expression.Name("companySlug").Equal(expression.Value(filter.Company)))
	}
	if filter.RoleFamily != "" {
		cond = cond.And(expression.Name("roleFamily").Equal(expression.Value(filter.RoleFamily)))
	}
	expr, err := expression.NewBuilder().WithFilter(cond).Build()
	if err != nil {
		return PostingPage{}, fmt.Errorf("build scan expression: %w", err)
	}

	out, err := s.client.Scan(ctx, &dynamodb.ScanInput{
		TableName:                 aws.String(TableName),
		FilterExpression:          expr.Filter(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		ExclusiveStartKey:         startKey,
		Limit:                     aws.Int32(limit),
	})
	if err != nil {
		return PostingPage{}, fmt.Errorf("scan postings: %w", err)
	}

	postings := make([]model.Posting, 0, len(out.Items))
	for _, rawItem := range out.Items {
		var item postingItem
		if err := attributevalue.UnmarshalMap(rawItem, &item); err != nil {
			return PostingPage{}, fmt.Errorf("unmarshal posting: %w", err)
		}
		if !filter.Since.IsZero() && item.PostedAt.Before(filter.Since) {
			continue
		}
		postings = append(postings, item.Posting)
	}

	nextCursor, err := EncodeCursor(out.LastEvaluatedKey)
	if err != nil {
		return PostingPage{}, fmt.Errorf("encode cursor: %w", err)
	}

	return PostingPage{Postings: postings, NextCursor: nextCursor}, nil
}

// ListSkillEdgesForPosting returns every skill edge for one posting via a
// Query (PK=POSTING#<id>, SK begins_with "SKILL#"). Unlike the Scan-based
// lookups elsewhere in this file, this is a real, efficient, at-any-scale
// query — the posting ID is already known, so no filter/GSI tradeoff
// applies here.
func (s *Store) ListSkillEdgesForPosting(ctx context.Context, postingID string) ([]model.PostingSkill, error) {
	keyCond := expression.Key("PK").Equal(expression.Value(postingPK(postingID))).
		And(expression.Key("SK").BeginsWith("SKILL#"))
	expr, err := expression.NewBuilder().WithKeyCondition(keyCond).Build()
	if err != nil {
		return nil, fmt.Errorf("build query expression: %w", err)
	}

	out, err := s.client.Query(ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(TableName),
		KeyConditionExpression:    expr.KeyCondition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	})
	if err != nil {
		return nil, fmt.Errorf("query skill edges for posting %s: %w", postingID, err)
	}

	edges := make([]model.PostingSkill, 0, len(out.Items))
	for _, rawItem := range out.Items {
		var item skillEdgeItem
		if err := attributevalue.UnmarshalMap(rawItem, &item); err != nil {
			return nil, fmt.Errorf("unmarshal skill edge: %w", err)
		}
		edges = append(edges, item.PostingSkill)
	}
	return edges, nil
}

// ListSkillEdgesForSkill returns every edge for one skill, across every
// posting — the reverse direction of ListSkillEdgesForPosting. No GSI
// exists for this access pattern (skillEdgeItem carries no GSI key
// attributes), so this is a full Scan+Filter, the same tradeoff
// ListAllSkillEdges already documents. Add a GSI keyed by skillId if/when
// this needs to scale past a few hundred edges (docs/design.md Phase 4).
func (s *Store) ListSkillEdgesForSkill(ctx context.Context, skillID string) ([]model.PostingSkill, error) {
	filter := expression.Name("entityType").Equal(expression.Value("postingSkill")).
		And(expression.Name("skillId").Equal(expression.Value(skillID)))
	expr, err := expression.NewBuilder().WithFilter(filter).Build()
	if err != nil {
		return nil, fmt.Errorf("build scan expression: %w", err)
	}

	var edges []model.PostingSkill
	var lastKey map[string]types.AttributeValue
	for {
		out, err := s.client.Scan(ctx, &dynamodb.ScanInput{
			TableName:                 aws.String(TableName),
			FilterExpression:          expr.Filter(),
			ExpressionAttributeNames:  expr.Names(),
			ExpressionAttributeValues: expr.Values(),
			ExclusiveStartKey:         lastKey,
		})
		if err != nil {
			return nil, fmt.Errorf("scan skill edges for skill %s: %w", skillID, err)
		}
		for _, rawItem := range out.Items {
			var item skillEdgeItem
			if err := attributevalue.UnmarshalMap(rawItem, &item); err != nil {
				return nil, fmt.Errorf("unmarshal skill edge: %w", err)
			}
			edges = append(edges, item.PostingSkill)
		}
		if out.LastEvaluatedKey == nil {
			break
		}
		lastKey = out.LastEvaluatedKey
	}
	return edges, nil
}
