import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { useAuth } from '../auth/AuthContext';
import { apiFetch } from './client';
import type {
  Company,
  ListCompaniesResponse,
  ListPostingsResponse,
  ListReviewsResponse,
  ListSkillsResponse,
  PostingDetail,
  ReviewAction,
  ReviewActionResponse,
  SkillDetail,
  StatsOverview,
} from './types';

// Every hook takes the current access token from useAuth() and disables
// itself (`enabled: !!accessToken`) until one exists — screens render
// under (tabs), which is itself gated on isSignedIn (see app/_layout.tsx),
// so in practice this only matters for the brief window between mount and
// the auth restore effect resolving.

export function useSkills(params?: { roleFamily?: string; tier?: string; required?: 'true' | 'false' }) {
  const { accessToken } = useAuth();
  const query = new URLSearchParams(params as Record<string, string>).toString();
  return useQuery({
    queryKey: ['skills', params],
    queryFn: () => apiFetch<ListSkillsResponse>(`/v1/skills${query ? `?${query}` : ''}`, accessToken),
    enabled: !!accessToken,
  });
}

export function useSkill(id: string) {
  const { accessToken } = useAuth();
  return useQuery({
    queryKey: ['skill', id],
    queryFn: () => apiFetch<SkillDetail>(`/v1/skills/${encodeURIComponent(id)}`, accessToken),
    enabled: !!accessToken && !!id,
  });
}

export function usePostings(params?: { company?: string; roleFamily?: string; cursor?: string }) {
  const { accessToken } = useAuth();
  const query = new URLSearchParams(params as Record<string, string>).toString();
  return useQuery({
    queryKey: ['postings', params],
    queryFn: () => apiFetch<ListPostingsResponse>(`/v1/postings${query ? `?${query}` : ''}`, accessToken),
    enabled: !!accessToken,
  });
}

export function usePosting(id: string) {
  const { accessToken } = useAuth();
  return useQuery({
    queryKey: ['posting', id],
    queryFn: () => apiFetch<PostingDetail>(`/v1/postings/${encodeURIComponent(id)}`, accessToken),
    enabled: !!accessToken && !!id,
  });
}

export function useCompanies() {
  const { accessToken } = useAuth();
  return useQuery({
    queryKey: ['companies'],
    queryFn: () => apiFetch<ListCompaniesResponse>('/v1/companies', accessToken),
    enabled: !!accessToken,
  });
}

export function useStatsOverview() {
  const { accessToken } = useAuth();
  return useQuery({
    queryKey: ['stats-overview'],
    queryFn: () => apiFetch<StatsOverview>('/v1/stats/overview', accessToken),
    enabled: !!accessToken,
  });
}

export function useReviews() {
  const { accessToken } = useAuth();
  return useQuery({
    queryKey: ['reviews'],
    queryFn: () => apiFetch<ListReviewsResponse>('/v1/reviews', accessToken),
    enabled: !!accessToken,
  });
}

export function useReviewAction() {
  const { accessToken } = useAuth();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ term, action }: { term: string; action: ReviewAction }) =>
      apiFetch<ReviewActionResponse>(`/v1/reviews/${encodeURIComponent(term)}`, accessToken, {
        method: 'POST',
        body: JSON.stringify(action),
      }),
    onSuccess: () => {
      // The review queue and the skill list both change on a successful
      // triage (a new/aliased skill can immediately affect ranked counts
      // once re-extraction catches up) — invalidate both rather than
      // hand-patching the cache.
      queryClient.invalidateQueries({ queryKey: ['reviews'] });
      queryClient.invalidateQueries({ queryKey: ['skills'] });
    },
  });
}

export type { Company };
