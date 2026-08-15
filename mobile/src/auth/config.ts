// Real, apply-specific values (Cognito domain/client ID, API base URL) —
// never hardcoded here. Expo inlines any env var prefixed EXPO_PUBLIC_ into
// the bundle at build time; see .env.example for the full set and
// ci/Jenkinsfile.client-build for how the real values get there in CI
// (read from SSM, same pattern the Go side uses throughout this project).
function requireEnv(name: string): string {
  const value = process.env[name];
  if (!value) {
    throw new Error(
      `${name} is not set — copy .env.example to .env and fill in real values (see docs/phase-6.md), or check ci/Jenkinsfile.client-build's SSM injection if this is a CI build.`,
    );
  }
  return value;
}

export const authConfig = {
  // e.g. "job-syllabus-881811711506.auth.us-west-1.amazoncognito.com" —
  // no https:// prefix, matching Terraform's own auth_hosted_ui_domain
  // output.
  hostedUiDomain: requireEnv('EXPO_PUBLIC_COGNITO_DOMAIN'),
  clientId: requireEnv('EXPO_PUBLIC_COGNITO_CLIENT_ID'),
  // docs/design.md §7: read scope for queries, admin scope for writes —
  // both requested for every sign-in since this is a single-operator tool
  // (docs/phase-5.md's Bedrock use-case form answer: "internal users
  // only") with no per-user role distinction to make.
  scopes: ['openid', 'email', 'profile', 'job-syllabus-api/read', 'job-syllabus-api/admin'],
  // The exact web origin registered as a Cognito callback URL
  // (infra/terraform/envs/dev-compute/main.tf's module "auth" block) —
  // expo-auth-session's makeRedirectUri() derives a web redirect from
  // window.location, but Cognito requires an *exact* match against a
  // registered callback URL, and the current SDK's own docs recommend
  // hard-coding the production web URL rather than relying on
  // auto-detection. Native (iOS/Android) doesn't use this at all — see
  // AuthContext.tsx.
  webRedirectUri: requireEnv('EXPO_PUBLIC_WEB_REDIRECT_URI'),
};

export const apiBaseUrl = requireEnv('EXPO_PUBLIC_API_URL');
