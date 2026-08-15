package main

import (
	"context"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/bedrock"
	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/extract"
	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/store"
)

// bulletRef locates one unmatched bullet within the batch of jobs currently
// being processed — the flat unit classifyUnmatched works over so bullets
// from different postings can share the same Bedrock call (docs/design.md
// §6: "Batch 20 unmatched bullets per call").
type bulletRef struct {
	jobIdx    int
	bulletIdx int
	text      string
}

// classifyUnmatched is Stage 4: for every job's Stage-3 misses, check the
// 90-day bullet cache first, then send whatever's left to Bedrock in
// batches of up to bedrock.MaxBatchSize, storing fresh results back to the
// cache as they come in. A nil bc means Stage 4 is disabled (BEDROCK_ENABLED
// unset to false — local dev against LocalStack, whose fake credentials
// can't call real Bedrock) — jobs simply get no llmFindings, same as any
// other best-effort miss.
//
// A failed Classify call for one chunk is logged and skipped, not fatal —
// Stage 4 is an enhancement over the dictionary pass, not required for a
// posting to finish this run. Those bullets' llmFindings stay nil; the
// posting still gets ExtractVer stamped, so a live Bedrock outage doesn't
// block ingestion, but it does mean those specific bullets won't be
// automatically retried until something re-enqueues them (a future
// `reextract` after the next Version bump, or a manual backfill) — see
// docs/phase-5.md.
func classifyUnmatched(ctx context.Context, s *store.Store, bc *bedrock.Client, jobs []*job, logger *slog.Logger) {
	if bc == nil {
		return
	}

	var refs []bulletRef
	for ji, j := range jobs {
		if j == nil || len(j.unmatched) == 0 {
			continue
		}
		j.llmFindings = make([][]bedrock.Finding, len(j.unmatched))
		for bi, ub := range j.unmatched {
			refs = append(refs, bulletRef{jobIdx: ji, bulletIdx: bi, text: ub.Text})
		}
	}
	if len(refs) == 0 {
		return
	}

	var toClassify []bulletRef
	for _, ref := range refs {
		hash := store.HashBullet(ref.text)
		cached, hit, err := s.GetBedrockCache(ctx, hash)
		if err != nil {
			logger.Error("worker: bedrock cache read failed, falling through to a live call", "error", err.Error())
			toClassify = append(toClassify, ref)
			continue
		}
		if hit {
			jobs[ref.jobIdx].llmFindings[ref.bulletIdx] = toFindings(cached, ref.bulletIdx+1)
			continue
		}
		toClassify = append(toClassify, ref)
	}

	for start := 0; start < len(toClassify); start += bedrock.MaxBatchSize {
		end := min(start+bedrock.MaxBatchSize, len(toClassify))
		chunk := toClassify[start:end]

		texts := make([]string, len(chunk))
		for i, ref := range chunk {
			texts[i] = ref.text
		}

		findings, err := bc.Classify(ctx, texts)
		if err != nil {
			logger.Error("worker: bedrock classify failed, skipping this chunk", "bullets", len(chunk), "error", err.Error())
			continue
		}

		byLocalIdx := make(map[int][]bedrock.Finding, len(chunk))
		for _, f := range findings {
			byLocalIdx[f.BulletIndex] = append(byLocalIdx[f.BulletIndex], f)
		}

		for i, ref := range chunk {
			fs := byLocalIdx[i+1] // Classify's bulletIndex is 1-based within this call
			jobs[ref.jobIdx].llmFindings[ref.bulletIdx] = fs
			if err := s.PutBedrockCache(ctx, store.HashBullet(ref.text), toCacheFindings(fs)); err != nil {
				logger.Error("worker: bedrock cache write failed", "error", err.Error())
			}
		}
	}
}

func toFindings(cached []store.BedrockFinding, bulletIndex int) []bedrock.Finding {
	if len(cached) == 0 {
		return nil
	}
	out := make([]bedrock.Finding, len(cached))
	for i, c := range cached {
		out[i] = bedrock.Finding{
			BulletIndex: bulletIndex,
			Term:        c.Term,
			Category:    c.Category,
			Evidence:    c.Evidence,
			Confidence:  c.Confidence,
		}
	}
	return out
}

func toCacheFindings(findings []bedrock.Finding) []store.BedrockFinding {
	if len(findings) == 0 {
		return []store.BedrockFinding{}
	}
	out := make([]store.BedrockFinding, len(findings))
	for i, f := range findings {
		out[i] = store.BedrockFinding{Term: f.Term, Category: f.Category, Evidence: f.Evidence, Confidence: f.Confidence}
	}
	return out
}

// resolveFinding checks a Bedrock finding against the current compiled
// dictionary before accepting it as "unknown" — the same regex matchers
// Stage 3 uses, so a paraphrase Bedrock resolves to text the dictionary
// would also match (e.g. the model says "Perforce" for a bullet reading
// "experience with Helix Core") is treated as a dictionary-equivalent hit,
// not routed to the review queue as if it were novel.
func resolveFinding(f bedrock.Finding, skills []extract.CompiledSkill) (skillID string, ok bool) {
	for _, cs := range skills {
		if _, matched := cs.FindEvidence(f.Evidence); matched {
			return cs.Skill.ID, true
		}
		if _, matched := cs.FindEvidence(f.Term); matched {
			return cs.Skill.ID, true
		}
	}
	return "", false
}

// newBedrockClientOrNil builds a Bedrock client unless explicitly disabled —
// BEDROCK_ENABLED=false is set by the local dev Makefile target, since
// LocalStack's fake credentials can't authenticate against real Bedrock.
func newBedrockClientOrNil(cfg aws.Config, enabled bool) *bedrock.Client {
	if !enabled {
		return nil
	}
	return bedrock.NewClient(cfg)
}
