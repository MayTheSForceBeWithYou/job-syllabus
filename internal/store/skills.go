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

// skillEdgeItem is the DynamoDB item shape for a Posting->Skill edge:
// PK=POSTING#<id>, SK=SKILL#<skillId>, per docs/design.md §4.
type skillEdgeItem struct {
	PK         string `dynamodbav:"PK"`
	SK         string `dynamodbav:"SK"`
	EntityType string `dynamodbav:"entityType"`
	model.PostingSkill
}

// PutSkillEdge upserts a Posting->Skill edge. A PutItem with the same
// PK/SK is naturally idempotent (last write wins on identical content),
// which is sufficient for Phase 1's single-process, no-queue ingest — the
// write-time-counter + TransactWriteItems idempotency mechanism §4
// describes matters once there's an at-least-once queue consumer that can
// process the same posting twice concurrently; that doesn't exist yet
// (see PROGRESS.md). report computes counts by scanning edges fresh each
// run instead of maintaining separate counters, so there's nothing to
// double-increment in the meantime.
func (s *Store) PutSkillEdge(ctx context.Context, edge model.PostingSkill) error {
	item := skillEdgeItem{
		PK:           postingPK(edge.PostingID),
		SK:           "SKILL#" + edge.SkillID,
		EntityType:   "postingSkill",
		PostingSkill: edge,
	}
	av, err := attributevalue.MarshalMap(item)
	if err != nil {
		return fmt.Errorf("marshal skill edge %s/%s: %w", edge.PostingID, edge.SkillID, err)
	}
	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(TableName),
		Item:      av,
	})
	if err != nil {
		return fmt.Errorf("put skill edge %s/%s: %w", edge.PostingID, edge.SkillID, err)
	}
	return nil
}

// ListAllSkillEdges scans every Posting->Skill edge. Same Scan-over-Phase-1
// approach as ListAllPostings — fine for a few-hundred-item corpus, not
// meant to survive to the "≥500 postings across ≥40 companies" scale G1
// targets.
func (s *Store) ListAllSkillEdges(ctx context.Context) ([]model.PostingSkill, error) {
	filter := expression.Name("entityType").Equal(expression.Value("postingSkill"))
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
			return nil, fmt.Errorf("scan skill edges: %w", err)
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
