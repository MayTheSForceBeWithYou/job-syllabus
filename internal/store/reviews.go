package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// Stage 5 (docs/design.md §6): a Bedrock-discovered term that doesn't match
// any known skill lands here — `REVIEW#PENDING` / `TERM#<normalized>` — with
// an occurrence count and example evidence, instead of becoming a canonical
// skill automatically. `REVIEW#REJECTED` is an extension beyond the design's
// literal two-column table: without it, a term rejected as noise would just
// reappear the next time a new posting reuses the same phrase, defeating
// the point of triaging it at all.
const (
	reviewPendingPK  = "REVIEW#PENDING"
	reviewRejectedPK = "REVIEW#REJECTED"
)

// maxReviewEvidence caps how many example spans accumulate per pending
// term — enough to judge the term without letting the item grow unbounded
// while it waits to be triaged.
const maxReviewEvidence = 5

func reviewSK(term string) string { return "TERM#" + term }

// NormalizeTerm turns a raw Bedrock-discovered term (or a skill ID a
// reviewer types in) into the canonical, comparable form used as both the
// review queue's sort key suffix and — on approval — a starting point for
// the new skill's own ID. Lowercase, trim, collapse internal whitespace to
// single spaces; callers that need a DynamoDB-item-id-safe slug (skill IDs
// must match config.validSkillID, `^[a-z0-9-]+$`) run this through
// SlugifyTerm instead.
func NormalizeTerm(raw string) string {
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(raw)))
	return strings.Join(fields, " ")
}

// SlugifyTerm derives a skill-ID-safe slug from a normalized term —
// non-alphanumeric runs collapse to a single hyphen, matching every other
// hand-written entry in data/skills.yaml (e.g. "unreal-build-tool").
func SlugifyTerm(normalized string) string {
	var b strings.Builder
	prevDash := true // true so a leading non-alnum run doesn't start with a dash
	for _, r := range normalized {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

type reviewItem struct {
	PK         string   `dynamodbav:"PK"`
	SK         string   `dynamodbav:"SK"`
	EntityType string   `dynamodbav:"entityType"`
	Term       string   `dynamodbav:"term"`
	Category   string   `dynamodbav:"category"`
	Count      int      `dynamodbav:"count"`
	Evidence   []string `dynamodbav:"evidence"`
}

// ReviewTerm is one pending unknown term, ready for the operator to triage.
type ReviewTerm struct {
	Term        string
	Category    string
	Occurrences int
	Evidence    []string
}

// RecordReviewOccurrence is Stage 5's write path: a Bedrock finding that
// didn't resolve to any known skill (internal/review.Resolve) bumps this
// term's occurrence count and, while there's room, appends its evidence
// span. Already-rejected terms (REVIEW#REJECTED) are a no-op — re-surfacing
// noise the operator already dismissed would defeat the point of rejecting
// it. Returns whether the term is newly tracked in the pending queue vs.
// already known (pending or rejected) — informational only, not currently
// branched on by any caller.
func (s *Store) RecordReviewOccurrence(ctx context.Context, term, category, evidence string) (tracked bool, err error) {
	normalized := NormalizeTerm(term)
	if normalized == "" {
		return false, nil
	}

	rejected, err := s.termRejected(ctx, normalized)
	if err != nil {
		return false, err
	}
	if rejected {
		return false, nil
	}

	_, err = s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(TableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: reviewPendingPK},
			"SK": &types.AttributeValueMemberS{Value: reviewSK(normalized)},
		},
		UpdateExpression: aws.String("ADD #count :one SET entityType = :et, #term = :term, category = :cat"),
		ExpressionAttributeNames: map[string]string{
			"#count": "count",
			"#term":  "term",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":one":  &types.AttributeValueMemberN{Value: "1"},
			":et":   &types.AttributeValueMemberS{Value: "review"},
			":term": &types.AttributeValueMemberS{Value: normalized},
			":cat":  &types.AttributeValueMemberS{Value: category},
		},
	})
	if err != nil {
		return false, fmt.Errorf("record review occurrence %q: %w", normalized, err)
	}

	if evidence != "" {
		_, err = s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
			TableName: aws.String(TableName),
			Key: map[string]types.AttributeValue{
				"PK": &types.AttributeValueMemberS{Value: reviewPendingPK},
				"SK": &types.AttributeValueMemberS{Value: reviewSK(normalized)},
			},
			UpdateExpression:    aws.String("SET evidence = list_append(if_not_exists(evidence, :empty), :e)"),
			ConditionExpression: aws.String("attribute_not_exists(evidence) OR size(evidence) < :cap"),
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":empty": &types.AttributeValueMemberL{},
				":e": &types.AttributeValueMemberL{Value: []types.AttributeValue{
					&types.AttributeValueMemberS{Value: evidence},
				}},
				":cap": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", maxReviewEvidence)},
			},
		})
		if err != nil {
			var condFailed *types.ConditionalCheckFailedException
			if !errors.As(err, &condFailed) {
				return true, fmt.Errorf("append review evidence %q: %w", normalized, err)
			}
			// Evidence cap reached — the occurrence count above still
			// incremented, which is all that matters past this point.
		}
	}

	return true, nil
}

func (s *Store) termRejected(ctx context.Context, normalized string) (bool, error) {
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(TableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: reviewRejectedPK},
			"SK": &types.AttributeValueMemberS{Value: reviewSK(normalized)},
		},
	})
	if err != nil {
		return false, fmt.Errorf("check rejected term %q: %w", normalized, err)
	}
	return out.Item != nil, nil
}

// ListPendingReviews queries the REVIEW#PENDING partition directly — a real
// Query against a known key, not a Scan, since every pending term shares
// that one PK by construction. Sorted by occurrence count descending
// client-side: the expected item count here is small (a human-triaged
// queue, not a bulk table), so an extra GSI just to let DynamoDB do the
// sort isn't worth it — same tradeoff `rank.Skills` makes for the money
// endpoint.
func (s *Store) ListPendingReviews(ctx context.Context) ([]ReviewTerm, error) {
	keyCond := expression.Key("PK").Equal(expression.Value(reviewPendingPK))
	expr, err := expression.NewBuilder().WithKeyCondition(keyCond).Build()
	if err != nil {
		return nil, fmt.Errorf("build query expression: %w", err)
	}

	var terms []ReviewTerm
	var lastKey map[string]types.AttributeValue
	for {
		out, err := s.client.Query(ctx, &dynamodb.QueryInput{
			TableName:                 aws.String(TableName),
			KeyConditionExpression:    expr.KeyCondition(),
			ExpressionAttributeNames:  expr.Names(),
			ExpressionAttributeValues: expr.Values(),
			ExclusiveStartKey:         lastKey,
		})
		if err != nil {
			return nil, fmt.Errorf("query pending reviews: %w", err)
		}
		for _, rawItem := range out.Items {
			var item reviewItem
			if err := attributevalue.UnmarshalMap(rawItem, &item); err != nil {
				return nil, fmt.Errorf("unmarshal review item: %w", err)
			}
			terms = append(terms, ReviewTerm{
				Term:        item.Term,
				Category:    item.Category,
				Occurrences: item.Count,
				Evidence:    item.Evidence,
			})
		}
		if out.LastEvaluatedKey == nil {
			break
		}
		lastKey = out.LastEvaluatedKey
	}

	sortReviewTermsByOccurrenceDesc(terms)
	return terms, nil
}

func sortReviewTermsByOccurrenceDesc(terms []ReviewTerm) {
	for i := 1; i < len(terms); i++ {
		for j := i; j > 0 && terms[j].Occurrences > terms[j-1].Occurrences; j-- {
			terms[j], terms[j-1] = terms[j-1], terms[j]
		}
	}
}

// GetPendingReview loads one pending term, or (nil, nil) if it isn't
// (or is no longer) in the queue — a stale/double-submitted triage action
// against a term someone else already resolved.
func (s *Store) GetPendingReview(ctx context.Context, normalizedTerm string) (*ReviewTerm, error) {
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(TableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: reviewPendingPK},
			"SK": &types.AttributeValueMemberS{Value: reviewSK(normalizedTerm)},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("get pending review %q: %w", normalizedTerm, err)
	}
	if out.Item == nil {
		return nil, nil
	}
	var item reviewItem
	if err := attributevalue.UnmarshalMap(out.Item, &item); err != nil {
		return nil, fmt.Errorf("unmarshal review item %q: %w", normalizedTerm, err)
	}
	return &ReviewTerm{Term: item.Term, Category: item.Category, Occurrences: item.Count, Evidence: item.Evidence}, nil
}

// ResolvePendingReview removes a term from REVIEW#PENDING — the last step
// of every triage action (create/alias/reject), once its effect (a new
// canonical skill, an alias merged into an existing one, or a rejection
// marker) has already been written.
func (s *Store) ResolvePendingReview(ctx context.Context, normalizedTerm string) error {
	_, err := s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(TableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: reviewPendingPK},
			"SK": &types.AttributeValueMemberS{Value: reviewSK(normalizedTerm)},
		},
	})
	if err != nil {
		return fmt.Errorf("resolve pending review %q: %w", normalizedTerm, err)
	}
	return nil
}

// RejectTerm marks a term as permanently dismissed noise — no TTL,
// deliberately: unlike the Bedrock cache or the dedup marker, a rejection
// is meant to stick (docs/design.md §6: "reject as noise").
func (s *Store) RejectTerm(ctx context.Context, normalizedTerm string) error {
	_, err := s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(TableName),
		Item: map[string]types.AttributeValue{
			"PK":         &types.AttributeValueMemberS{Value: reviewRejectedPK},
			"SK":         &types.AttributeValueMemberS{Value: reviewSK(normalizedTerm)},
			"entityType": &types.AttributeValueMemberS{Value: "reviewRejected"},
			"rejectedAt": &types.AttributeValueMemberS{Value: time.Now().UTC().Format(time.RFC3339)},
		},
	})
	if err != nil {
		return fmt.Errorf("reject term %q: %w", normalizedTerm, err)
	}
	return nil
}
