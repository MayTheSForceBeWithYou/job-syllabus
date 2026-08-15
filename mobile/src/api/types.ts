// Mirrors internal/api/dto.go's JSON shapes exactly — field names, casing,
// and optionality all match what service-api actually serializes. Keep
// these two in sync by hand; there's no shared schema generation between
// the Go and TypeScript sides yet.

export interface Skill {
  id: string;
  display: string;
  category: string;
  count: number;
  required: number;
  niceToHave: number;
  pctOfPostings: number;
}

export interface ListSkillsResponse {
  skills: Skill[];
  totalPostings: number;
}

export interface SkillDetail {
  id: string;
  display: string;
  category: string;
  count: number;
  required: number;
  niceToHave: number;
  exampleEvidence: string[];
}

export interface PostingSummary {
  id: string;
  companySlug: string;
  title: string;
  roleFamily: string;
  location: string;
  url: string;
  source: string;
  postedAt: string;
  skillCount: number;
}

export interface ListPostingsResponse {
  postings: PostingSummary[];
  nextCursor?: string;
}

export interface PostingSkill {
  skillId: string;
  display: string;
  required: boolean;
  evidence: string;
  confidence: number;
  method: 'dict' | 'llm';
}

export interface PostingDetail extends PostingSummary {
  skills: PostingSkill[];
}

export interface Company {
  slug: string;
  name: string;
  tier: string;
  ats: string;
  postingCount: number;
}

export interface ListCompaniesResponse {
  companies: Company[];
}

export interface StatsOverview {
  postingCount: number;
  companyCount: number;
  skillEdgeCount: number;
  distinctSkillsMatched: number;
  lastIngestAt?: string;
  coveragePct: number;
}

export interface ReviewTerm {
  term: string;
  category: string;
  occurrences: number;
  evidence: string[];
}

export interface ListReviewsResponse {
  reviews: ReviewTerm[];
}

export type ReviewAction =
  | { action: 'create'; skillId?: string; display?: string; category?: string; aliases?: string[] }
  | { action: 'alias'; mergeIntoSkillId: string }
  | { action: 'reject' };

export interface ReviewActionResponse {
  term: string;
  action: string;
  skill?: Skill;
}

// docs/design.md §7's RFC 7807 problem+json error shape.
export interface Problem {
  title: string;
  status: number;
  detail?: string;
  instance?: string;
}
