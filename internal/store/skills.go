package store

import (
	"context"
	"errors"
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

func statPK(roleFamily model.RoleFamily) string { return "STAT#" + string(roleFamily) }

// PutSkillEdge records a Posting->Skill edge and idempotently increments
// its STAT#<roleFamily>/SKILL#<skillId> counter (docs/design.md §4, "the
// aggregation problem") in one TransactWriteItems call: the edge Put
// carries ConditionExpression attribute_not_exists(PK), so if this exact
// edge already exists — an at-least-once SQS redelivery processing the
// same posting twice, docs/design.md §9 — the whole transaction is
// cancelled and the counter is not double-incremented. That specific,
// expected cancellation is treated as a successful no-op, not an error;
// any other TransactWriteItems failure still propagates.
func (s *Store) PutSkillEdge(ctx context.Context, roleFamily model.RoleFamily, edge model.PostingSkill) error {
	edgeItem := skillEdgeItem{
		PK:           postingPK(edge.PostingID),
		SK:           "SKILL#" + edge.SkillID,
		EntityType:   "postingSkill",
		PostingSkill: edge,
	}
	edgeAV, err := attributevalue.MarshalMap(edgeItem)
	if err != nil {
		return fmt.Errorf("marshal skill edge %s/%s: %w", edge.PostingID, edge.SkillID, err)
	}

	_, err = s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{
			{
				Put: &types.Put{
					TableName:           aws.String(TableName),
					Item:                edgeAV,
					ConditionExpression: aws.String("attribute_not_exists(PK)"),
				},
			},
			{
				Update: &types.Update{
					TableName: aws.String(TableName),
					Key: map[string]types.AttributeValue{
						"PK": &types.AttributeValueMemberS{Value: statPK(roleFamily)},
						"SK": &types.AttributeValueMemberS{Value: "SKILL#" + edge.SkillID},
					},
					UpdateExpression: aws.String("ADD #count :one SET entityType = :et"),
					ExpressionAttributeNames: map[string]string{
						"#count": "count",
					},
					ExpressionAttributeValues: map[string]types.AttributeValue{
						":one": &types.AttributeValueMemberN{Value: "1"},
						":et":  &types.AttributeValueMemberS{Value: "skillStat"},
					},
				},
			},
		},
	})
	if err != nil {
		var canceled *types.TransactionCanceledException
		if errors.As(err, &canceled) &&
			len(canceled.CancellationReasons) > 0 &&
			aws.ToString(canceled.CancellationReasons[0].Code) == "ConditionalCheckFailed" {
			return nil
		}
		return fmt.Errorf("put skill edge %s/%s: %w", edge.PostingID, edge.SkillID, err)
	}
	return nil
}

// SkillStat is one STAT#<roleFamily>/SKILL#<skillId> counter.
type SkillStat struct {
	RoleFamily model.RoleFamily
	SkillID    string
	Count      int
}

type skillStatItem struct {
	PK    string `dynamodbav:"PK"`
	SK    string `dynamodbav:"SK"`
	Count int    `dynamodbav:"count"`
}

// ListAllSkillStats scans every write-time skill counter — cmd/rollup's
// reconciliation pass reads this to compare against a fresh recount from
// ListAllSkillEdges and correct drift (docs/design.md §4).
func (s *Store) ListAllSkillStats(ctx context.Context) ([]SkillStat, error) {
	filter := expression.Name("entityType").Equal(expression.Value("skillStat"))
	expr, err := expression.NewBuilder().WithFilter(filter).Build()
	if err != nil {
		return nil, fmt.Errorf("build scan expression: %w", err)
	}

	var stats []SkillStat
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
			return nil, fmt.Errorf("scan skill stats: %w", err)
		}
		for _, rawItem := range out.Items {
			var item skillStatItem
			if err := attributevalue.UnmarshalMap(rawItem, &item); err != nil {
				return nil, fmt.Errorf("unmarshal skill stat: %w", err)
			}
			roleFamily, skillID, ok := parseStatKey(item.PK, item.SK)
			if !ok {
				continue
			}
			stats = append(stats, SkillStat{RoleFamily: roleFamily, SkillID: skillID, Count: item.Count})
		}
		if out.LastEvaluatedKey == nil {
			break
		}
		lastKey = out.LastEvaluatedKey
	}
	return stats, nil
}

// SetSkillStat overwrites one counter outright — cmd/rollup's correction
// path, deliberately a plain PutItem (not a transaction) since
// reconciliation is a scan-then-fix pass, not a concurrent write racing
// against live traffic the way PutSkillEdge's increment is.
func (s *Store) SetSkillStat(ctx context.Context, roleFamily model.RoleFamily, skillID string, count int) error {
	item := skillStatItem{
		PK:    statPK(roleFamily),
		SK:    "SKILL#" + skillID,
		Count: count,
	}
	av, err := attributevalue.MarshalMap(item)
	if err != nil {
		return fmt.Errorf("marshal skill stat %s/%s: %w", roleFamily, skillID, err)
	}
	av["entityType"] = &types.AttributeValueMemberS{Value: "skillStat"}
	if _, err := s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(TableName),
		Item:      av,
	}); err != nil {
		return fmt.Errorf("put skill stat %s/%s: %w", roleFamily, skillID, err)
	}
	return nil
}

func parseStatKey(pk, sk string) (roleFamily model.RoleFamily, skillID string, ok bool) {
	const statPrefix, skillPrefix = "STAT#", "SKILL#"
	if len(pk) <= len(statPrefix) || pk[:len(statPrefix)] != statPrefix {
		return "", "", false
	}
	if len(sk) <= len(skillPrefix) || sk[:len(skillPrefix)] != skillPrefix {
		return "", "", false
	}
	return model.RoleFamily(pk[len(statPrefix):]), sk[len(skillPrefix):], true
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
