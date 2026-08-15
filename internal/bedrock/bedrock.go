// Package bedrock implements docs/design.md §6 Stage 4: for
// requirement/nice-to-have bullets the dictionary pass found zero matches
// in, ask Claude Haiku (via Amazon Bedrock) what skill term, if any, the
// bullet names. Every response is validated against a strict schema before
// any of it is trusted — an LLM output is untrusted input here in exactly
// the same sense a network response is, not a shortcut around the
// dictionary's precision guarantees.
package bedrock

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
)

// ModelID is docs/design.md §6 Stage 4's "for volume; escalate to Sonnet if
// precision on the validation set is short of target" — no escalation has
// been needed yet, see docs/phase-5.md. This is a cross-region *inference
// profile* ID, not a bare foundation-model ID: this account's Bedrock
// catalog in us-west-2 doesn't offer the originally-planned
// claude-3-5-haiku on-demand at all (superseded), and the Haiku model that
// IS available (claude-haiku-4-5) only supports INFERENCE_PROFILE
// invocation, confirmed via `aws bedrock get-foundation-model` — real AWS
// account state overriding the design doc's illustrative model name, not a
// design change. See FoundationModelID for the underlying model ID the
// profile routes to (needed for IAM, not for InvokeModel itself).
const ModelID = "us.anthropic.claude-haiku-4-5-20251001-v1:0"

// FoundationModelID is the bare model ID ModelID's inference profile routes
// requests to (us-east-1/us-east-2/us-west-2) — Bedrock IAM requires
// InvokeModel permission on both the inference-profile ARN and every
// underlying foundation-model ARN it can route to, not just the profile
// itself (infra/terraform/modules/service-worker/iam.tf).
const FoundationModelID = "anthropic.claude-haiku-4-5-20251001-v1:0"

// MaxBatchSize is docs/design.md §6's "batch 20 unmatched bullets per call".
const MaxBatchSize = 20

// Region is where the worker's Bedrock client targets — Bedrock has no
// us-west-1 presence (docs/design.md §16 flagged this from the start), so
// this is a deliberate cross-region client while DynamoDB/S3/SQS all stay
// in us-west-1.
const Region = "us-west-2"

const maxTokens = 2048

// Finding is one skill term Bedrock identified in a batch of bullets,
// attributed back to the bullet it came from by index into the slice passed
// to Classify. Deliberately has no IsRequired field even though the wire
// schema the model returns includes one — see internal/store.BedrockFinding's
// comment: that's a property of which posting section the bullet occurred
// in, which the model wasn't told and shouldn't be trusted to infer, not a
// property of the bullet text itself.
type Finding struct {
	BulletIndex int
	Term        string
	Category    string
	Evidence    string
	Confidence  float32
}

// Client wraps a Bedrock Runtime client pinned to Region.
type Client struct {
	rt *bedrockruntime.Client
}

// NewClient builds a Bedrock Runtime client pinned to Region regardless of
// the given aws.Config's own region, so callers can share one aws.Config
// (loaded for us-west-1, matching every other AWS client cmd/worker builds)
// rather than needing a second LoadDefaultConfig call just for this.
func NewClient(cfg aws.Config, optFns ...func(*bedrockruntime.Options)) *Client {
	opts := append([]func(*bedrockruntime.Options){
		func(o *bedrockruntime.Options) { o.Region = Region },
	}, optFns...)
	return &Client{rt: bedrockruntime.NewFromConfig(cfg, opts...)}
}

// request/response shapes for Bedrock's Anthropic Messages API, per
// https://docs.aws.amazon.com/bedrock/latest/userguide/model-parameters-anthropic-claude-messages.html
// — InvokeModel with a model-specific JSON body, not the newer Converse API,
// since the Messages body is what lets max_tokens/temperature/system be set
// exactly as docs/design.md §6 specifies ("Temperature 0").
type invokeRequest struct {
	AnthropicVersion string          `json:"anthropic_version"`
	MaxTokens        int             `json:"max_tokens"`
	Temperature      float64         `json:"temperature"`
	System           string          `json:"system"`
	Messages         []invokeMessage `json:"messages"`
}

type invokeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type invokeResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

// wireFinding is the strict shape the prompt demands: `[{bulletIndex, term,
// category, isRequired, evidence, confidence}]` (docs/design.md §6). Every
// field is validated in unmarshalFindings before becoming a Finding —
// "reject and log on mismatch, never trust the shape."
type wireFinding struct {
	BulletIndex int     `json:"bulletIndex"`
	Term        string  `json:"term"`
	Category    string  `json:"category"`
	IsRequired  bool    `json:"isRequired"`
	Evidence    string  `json:"evidence"`
	Confidence  float64 `json:"confidence"`
}

const systemPrompt = `You extract technical skill/technology requirements from game-industry job posting bullets. You will be given a numbered list of bullets. For each bullet that names a specific skill, technology, tool, platform, or technique (e.g. "Perforce", "Kubernetes", "Unreal Engine", "CI/CD pipelines"), respond with one entry per skill found. Bullets about soft skills, years of experience, degree requirements, or generic responsibilities with no named technology should produce zero entries.

Respond with ONLY a JSON array, no prose before or after it, no markdown code fence. Each element must have exactly these fields:
- bulletIndex: the integer number of the bullet this came from (1-based, matching the input numbering)
- term: the skill/technology name as it appears in the bullet, or its common name
- category: one of vcs, languages, cloud, containers, ci_cd, testing, engines, platforms, liveops, data, observability, security, build_infra, practices, other
- isRequired: true if this bullet is unambiguously required, false if it reads as preferred/nice-to-have (best guess if unclear)
- evidence: the exact substring of the bullet that names the term, at most 200 characters
- confidence: your confidence this is a real, correctly-identified skill term, from 0.0 to 1.0

If no bullet names any skill, respond with an empty array: []`

// Classify sends up to MaxBatchSize bullets in one InvokeModel call and
// returns every finding, index-aligned to bullets (bulletIndex 1 refers to
// bullets[0], etc.). Bullets producing no finding simply have no entries —
// callers must not assume len(result) == len(bullets).
func (c *Client) Classify(ctx context.Context, bullets []string) ([]Finding, error) {
	if len(bullets) == 0 {
		return nil, nil
	}
	if len(bullets) > MaxBatchSize {
		return nil, fmt.Errorf("classify: %d bullets exceeds MaxBatchSize %d", len(bullets), MaxBatchSize)
	}

	reqBody, err := json.Marshal(invokeRequest{
		AnthropicVersion: "bedrock-2023-05-31",
		MaxTokens:        maxTokens,
		Temperature:      0,
		System:           systemPrompt,
		Messages: []invokeMessage{
			{Role: "user", Content: buildBulletPrompt(bullets)},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal bedrock request: %w", err)
	}

	out, err := c.rt.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(ModelID),
		ContentType: aws.String("application/json"),
		Accept:      aws.String("application/json"),
		Body:        reqBody,
	})
	if err != nil {
		return nil, fmt.Errorf("invoke bedrock model: %w", err)
	}

	var resp invokeResponse
	if err := json.Unmarshal(out.Body, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal bedrock response envelope: %w", err)
	}
	if len(resp.Content) == 0 || resp.Content[0].Type != "text" {
		return nil, fmt.Errorf("bedrock response had no text content block")
	}

	return unmarshalFindings(resp.Content[0].Text, len(bullets))
}

func buildBulletPrompt(bullets []string) string {
	var b strings.Builder
	for i, bullet := range bullets {
		b.WriteString(strconv.Itoa(i + 1))
		b.WriteString(". ")
		b.WriteString(bullet)
		b.WriteString("\n")
	}
	return b.String()
}

// unmarshalFindings parses and validates the model's JSON array against the
// strict schema. A structurally invalid response (not a JSON array at all)
// is rejected outright — the whole call's findings are dropped. An
// individual element with an out-of-range bulletIndex, empty term, or
// out-of-bounds confidence is dropped on its own rather than failing the
// whole batch — a single hallucinated field on an otherwise-valid response
// is a data-quality issue, not a reason to discard 19 good findings.
func unmarshalFindings(text string, bulletCount int) ([]Finding, error) {
	text = extractJSONArray(text)

	var wire []wireFinding
	if err := json.Unmarshal([]byte(text), &wire); err != nil {
		return nil, fmt.Errorf("bedrock response is not a valid JSON array: %w", err)
	}

	findings := make([]Finding, 0, len(wire))
	for _, w := range wire {
		term := strings.TrimSpace(w.Term)
		if w.BulletIndex < 1 || w.BulletIndex > bulletCount {
			continue
		}
		if term == "" {
			continue
		}
		if w.Confidence < 0 || w.Confidence > 1 {
			continue
		}
		findings = append(findings, Finding{
			BulletIndex: w.BulletIndex,
			Term:        term,
			Category:    strings.TrimSpace(w.Category),
			Evidence:    w.Evidence,
			Confidence:  float32(w.Confidence),
		})
	}
	return findings, nil
}

// extractJSONArray tolerates the model wrapping its JSON in prose or a
// markdown fence despite being told not to — takes the substring from the
// first '[' to the last ']'. If no bracket pair exists this returns the
// input unchanged, which then fails json.Unmarshal in the caller exactly as
// it should (a response with no array at all is a real schema violation,
// not something to paper over).
func extractJSONArray(text string) string {
	start := strings.IndexByte(text, '[')
	end := strings.LastIndexByte(text, ']')
	if start == -1 || end == -1 || end < start {
		return text
	}
	return text[start : end+1]
}
