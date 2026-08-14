package store

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/model"
)

// ListAllPostings scans every posting item. Fine for Phase 1's local,
// few-hundred-item dataset; a GSI1 query per company (or a real analytics
// path) replaces this once the corpus is large enough for Scan to matter.
func (s *Store) ListAllPostings(ctx context.Context) ([]model.Posting, error) {
	filter := expression.Name("entityType").Equal(expression.Value("posting"))
	expr, err := expression.NewBuilder().WithFilter(filter).Build()
	if err != nil {
		return nil, fmt.Errorf("build scan expression: %w", err)
	}

	var postings []model.Posting
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
			return nil, fmt.Errorf("scan postings: %w", err)
		}

		for _, rawItem := range out.Items {
			var item postingItem
			if err := attributevalue.UnmarshalMap(rawItem, &item); err != nil {
				return nil, fmt.Errorf("unmarshal posting: %w", err)
			}
			postings = append(postings, item.Posting)
		}

		if out.LastEvaluatedKey == nil {
			break
		}
		lastKey = out.LastEvaluatedKey
	}

	return postings, nil
}

// ListPostingsByCompany queries GSI1 (hash key COMPANY#<slug>) for every
// posting — open or closed — belonging to one company. Real Query, not a
// Scan+filter: this is exactly the access pattern GSI1 was built for
// (docs/design.md §4), and it's what cmd/ingest's posting-lifecycle
// handling needs to find previously-open postings a fresh fetch no longer
// lists (docs/design.md §4, "a posting disappearing from an ATS feed
// means it closed").
func (s *Store) ListPostingsByCompany(ctx context.Context, companySlug string) ([]model.Posting, error) {
	keyCond := expression.Key("GSI1PK").Equal(expression.Value("COMPANY#" + companySlug))
	expr, err := expression.NewBuilder().WithKeyCondition(keyCond).Build()
	if err != nil {
		return nil, fmt.Errorf("build query expression: %w", err)
	}

	var postings []model.Posting
	var lastKey map[string]types.AttributeValue

	for {
		out, err := s.client.Query(ctx, &dynamodb.QueryInput{
			TableName:                 aws.String(TableName),
			IndexName:                 aws.String("GSI1"),
			KeyConditionExpression:    expr.KeyCondition(),
			ExpressionAttributeNames:  expr.Names(),
			ExpressionAttributeValues: expr.Values(),
			ExclusiveStartKey:         lastKey,
		})
		if err != nil {
			return nil, fmt.Errorf("query postings for company %s: %w", companySlug, err)
		}

		for _, rawItem := range out.Items {
			var item postingItem
			if err := attributevalue.UnmarshalMap(rawItem, &item); err != nil {
				return nil, fmt.Errorf("unmarshal posting: %w", err)
			}
			postings = append(postings, item.Posting)
		}

		if out.LastEvaluatedKey == nil {
			break
		}
		lastKey = out.LastEvaluatedKey
	}

	return postings, nil
}
