// Package store implements the single-table DynamoDB persistence layer
// described in docs/design.md §4. It targets DynamoDB Local in Phase 1;
// nothing here talks to real AWS.
package store

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

const TableName = "jobsyllabus"

const tableWaitTimeout = 60 * time.Second

type Store struct {
	client *dynamodb.Client
}

// New builds a Store against a DynamoDB endpoint. For DynamoDB Local, pass
// e.g. "http://localhost:8000" — real AWS endpoints/credentials are a later
// phase (docs/design.md §0.2: never invent AWS resource config outside
// Terraform).
func New(ctx context.Context, endpoint string) (*Store, error) {
	// Explicit HTTP client so the SDK's own requests (DescribeTable,
	// PutItem, etc.) can't hang indefinitely either — same audit finding
	// as the ATS connectors: never rely on a client without a stated
	// Timeout.
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion("us-west-2"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("local", "local", "")),
		config.WithHTTPClient(&http.Client{Timeout: 10 * time.Second}),
	)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}
	slog.Info("store: connecting", "endpoint", endpoint)

	client := dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		if endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
		}
	})

	return &Store{client: client}, nil
}

// NewFromClient wraps an already-configured DynamoDB client. New's fake
// static credentials and hardcoded region are a DynamoDB Local-only
// convenience (Phase 1); callers that need real AWS credentials/region —
// cmd/api and cmd/rollup — build their own client via
// config.LoadDefaultConfig (with the same DYNAMODB_ENDPOINT override for
// local testing) and hand it in here instead.
func NewFromClient(client *dynamodb.Client) *Store {
	return &Store{client: client}
}

// EnsureTable creates the single table with its two GSIs if it doesn't
// already exist. This is a DynamoDB Local convenience for Phase 1 — the
// real table is Terraform-managed starting Phase 2 (docs/design.md §9).
func (s *Store) EnsureTable(ctx context.Context) error {
	start := time.Now()
	_, err := s.client.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(TableName),
	})
	if err == nil {
		slog.Info("store: table exists", "table", TableName, "elapsed", time.Since(start).String())
		return nil // already exists
	}

	var notFound *types.ResourceNotFoundException
	if !errors.As(err, &notFound) {
		return fmt.Errorf("describe table: %w", err)
	}

	slog.Info("store: creating table", "table", TableName)

	_, err = s.client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName:   aws.String(TableName),
		BillingMode: types.BillingModePayPerRequest,
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("PK"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("SK"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("GSI1PK"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("GSI1SK"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("GSI2PK"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("GSI2SK"), AttributeType: types.ScalarAttributeTypeS},
		},
		KeySchema: []types.KeySchemaElement{
			{AttributeName: aws.String("PK"), KeyType: types.KeyTypeHash},
			{AttributeName: aws.String("SK"), KeyType: types.KeyTypeRange},
		},
		GlobalSecondaryIndexes: []types.GlobalSecondaryIndex{
			{
				IndexName: aws.String("GSI1"),
				KeySchema: []types.KeySchemaElement{
					{AttributeName: aws.String("GSI1PK"), KeyType: types.KeyTypeHash},
					{AttributeName: aws.String("GSI1SK"), KeyType: types.KeyTypeRange},
				},
				Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll},
			},
			{
				IndexName: aws.String("GSI2"),
				KeySchema: []types.KeySchemaElement{
					{AttributeName: aws.String("GSI2PK"), KeyType: types.KeyTypeHash},
					{AttributeName: aws.String("GSI2SK"), KeyType: types.KeyTypeRange},
				},
				Projection: &types.Projection{ProjectionType: types.ProjectionTypeKeysOnly},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("create table: %w", err)
	}

	waiter := dynamodb.NewTableExistsWaiter(s.client)
	if err := waiter.Wait(ctx, &dynamodb.DescribeTableInput{TableName: aws.String(TableName)}, tableWaitTimeout); err != nil {
		return fmt.Errorf("wait for table active: %w", err)
	}
	slog.Info("store: table created", "table", TableName, "elapsed", time.Since(start).String())
	return nil
}
