import { apiBaseUrl } from '../auth/config';
import type { Problem } from './types';

export class ApiError extends Error {
  status: number;
  detail?: string;

  constructor(problem: Problem) {
    super(problem.title);
    this.status = problem.status;
    this.detail = problem.detail;
  }
}

// Every /v1/* call needs a Bearer access token — API Gateway's Cognito JWT
// authorizer (infra/terraform/modules/api-gateway) rejects the request
// before it ever reaches service-api if this is missing or expired,
// returning a bare {"message":"Unauthorized"} that isn't RFC 7807 shaped
// (that's API Gateway itself, not internal/api/problem.go), hence the
// separate 401 short-circuit below rather than trying to parse it as one.
export async function apiFetch<T>(
  path: string,
  accessToken: string | null,
  init?: RequestInit,
): Promise<T> {
  const res = await fetch(`${apiBaseUrl}${path}`, {
    ...init,
    headers: {
      ...(accessToken ? { Authorization: `Bearer ${accessToken}` } : {}),
      'Content-Type': 'application/json',
      ...init?.headers,
    },
  });

  if (res.status === 401) {
    throw new ApiError({ title: 'Unauthorized', status: 401, detail: 'Sign in again.' });
  }

  if (!res.ok) {
    const problem: Problem = await res.json().catch(() => ({
      title: 'Request failed',
      status: res.status,
    }));
    throw new ApiError(problem);
  }

  return res.json() as Promise<T>;
}
