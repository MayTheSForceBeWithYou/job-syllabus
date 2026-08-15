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

// skillItem is the DynamoDB item shape for a canonical Skill (docs/design.md
// §4's `SKILL#<skillId>` / `META` item) — this is Phase 5's live-dictionary
// override store: data/skills.yaml stays the git-tracked seed loaded at
// startup, and a skill approved via the review queue is written here
// instead of opening a git commit (operator decision — no GitHub write
// credential exists in this project yet, see docs/phase-5.md). Callers
// merge yaml ∪ DynamoDB at load time (config.MergeSkills), with DynamoDB
// winning on ID collision since it represents the most recently approved
// definition.
type skillItem struct {
	PK         string `dynamodbav:"PK"`
	SK         string `dynamodbav:"SK"`
	EntityType string `dynamodbav:"entityType"`
	model.Skill
}

func skillPK(id string) string { return "SKILL#" + id }

// PutSkill writes (or overwrites) one canonical skill definition. Plain
// PutItem, not conditional — an operator re-approving/editing the same
// skill ID (e.g. adding another alias via the `alias` review action) is
// expected to overwrite, not fail.
func (s *Store) PutSkill(ctx context.Context, sk model.Skill) error {
	item := skillItem{PK: skillPK(sk.ID), SK: "META", EntityType: "skill", Skill: sk}
	av, err := attributevalue.MarshalMap(item)
	if err != nil {
		return fmt.Errorf("marshal skill %s: %w", sk.ID, err)
	}
	if _, err := s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(TableName),
		Item:      av,
	}); err != nil {
		return fmt.Errorf("put skill %s: %w", sk.ID, err)
	}
	return nil
}

// GetSkill loads one canonical skill by ID, or (nil, nil) if this ID has no
// DynamoDB-approved definition (i.e. it only exists in the yaml seed, or
// doesn't exist at all — callers distinguish those two cases against their
// own yaml-loaded map).
func (s *Store) GetSkill(ctx context.Context, id string) (*model.Skill, error) {
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(TableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: skillPK(id)},
			"SK": &types.AttributeValueMemberS{Value: "META"},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("get skill %s: %w", id, err)
	}
	if out.Item == nil {
		return nil, nil
	}
	var item skillItem
	if err := attributevalue.UnmarshalMap(out.Item, &item); err != nil {
		return nil, fmt.Errorf("unmarshal skill %s: %w", id, err)
	}
	return &item.Skill, nil
}

// ListDynamicSkills scans every DynamoDB-approved canonical skill — expected
// to stay small (this is a human-triaged review queue's output, not a bulk
// import path), so a Scan is the same accepted Phase-1-scale tradeoff as
// ListAllPostings/ListAllSkillEdges rather than a premature GSI.
func (s *Store) ListDynamicSkills(ctx context.Context) ([]model.Skill, error) {
	filter := expression.Name("entityType").Equal(expression.Value("skill"))
	expr, err := expression.NewBuilder().WithFilter(filter).Build()
	if err != nil {
		return nil, fmt.Errorf("build scan expression: %w", err)
	}

	var skills []model.Skill
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
			return nil, fmt.Errorf("scan skills: %w", err)
		}
		for _, rawItem := range out.Items {
			var item skillItem
			if err := attributevalue.UnmarshalMap(rawItem, &item); err != nil {
				return nil, fmt.Errorf("unmarshal skill: %w", err)
			}
			skills = append(skills, item.Skill)
		}
		if out.LastEvaluatedKey == nil {
			break
		}
		lastKey = out.LastEvaluatedKey
	}
	return skills, nil
}
