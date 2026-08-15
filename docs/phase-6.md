# Phase 6 — Cognito auth + Expo client

**Status: done, DoD validated against real production infrastructure — not
just a local dev server.** A real Cognito User Pool (admin-created users
only, no public self-signup) fronts `service-api` via an API Gateway JWT
authorizer, retiring Phase 3's IP-allowlist entirely — a mobile client
signing in from arbitrary IPs is the whole point, so an IP allowlist would
have actively worked against the feature, not just been redundant. A new
Expo Router app (TypeScript, nativewind) implements sign-in, the ranked
syllabus, postings, companies, and the review-queue triage screen, all
against the real deployed API. The web build (`expo export -p web`) is
served from S3 + CloudFront (Origin Access Control, not the legacy OAI) and
deployed by a new `client-build` Jenkins pipeline.

## DoD proof

`docs/design-full-reference.md`'s Phase 6 DoD: **"Sign in and browse the
syllabus on iOS, Android, and web."** Per an explicit operator decision
before building any of this (no Xcode/Android Studio/simulator available in
this environment): **web is verified by real, live testing against the
deployed CloudFront URL in a real browser; iOS and Android are verified by
code review against the current Expo SDK 57 docs, not by running on an
actual device or simulator.** That distinction is carried through honestly
below — nothing here claims device-level iOS/Android verification that
didn't happen.

**Web, verified live end-to-end** against `https://d3hsgmapammng4.cloudfront.net`:

1. Loaded the deployed site cold (no console errors, no missing-env-var
   crash) — real proof the CI-shaped export → S3 → CloudFront pipeline
   produces a working bundle, not just "it built."
2. Clicked "Sign in" → full-page redirect to Cognito's real Hosted UI
   (`job-syllabus-881811711506.auth.us-west-1.amazoncognito.com/login`)
   with the correct PKCE `code_challenge`, `client_id`, and `scope` params
   already attached.
3. Signed in with a real admin-created user
   (`maythesforcebewithyou@gmail.com`) → Hosted UI redirected back to
   CloudFront with an authorization `code` → the app exchanged it for
   tokens and rendered the authenticated shell.
4. All four tabs loaded real data from the real deployed API:

```
Syllabus — 158 active postings
C++            languages   17.7%   23 req · 5 nice
AWS            cloud       17.1%   20 req · 7 nice
Kubernetes     containers  16.5%   21 req · 5 nice
...

Postings — real listings with skill-match counts
  Staff Software Engineer, Build Platforms - VALORANT (riot-games) — 7 skills matched
  Animation Tools Programmer (rockstar-games) — 4 skills matched
  ...

Companies — 20 companies · 158 active postings · 54.4% skill coverage
  Riot Games (30) · Roblox (28) · Epic Games (13) · Rockstar Games (13) ...

Review queue — 57 pending, Create/Alias/Reject all present and wired
  Ci/Cd Pipelines (9×), Slate (8×), Umg (8×), Build Automation (6×) ...
```

The review queue tab rendering at all is itself proof both JWT scopes work
correctly end-to-end: `GET /v1/reviews` needs `job-syllabus-api/read`,
`POST /v1/reviews/{term}`'s Create/Alias/Reject buttons need
`job-syllabus-api/admin` — both were requested at sign-in
(`src/auth/config.ts`) and both routes' `authorization_scopes` accepted the
resulting token.

**iOS/Android, verified by code review, not device execution:** the sign-in
and API-fetch code paths were checked against the current Expo SDK 57 docs
(`AGENTS.md`'s own standing instruction) for `expo-auth-session`'s native
`useAuthRequest`/`promptAsync` flow, `expo-secure-store` token persistence,
and the `jobsyllabus://` redirect scheme registered as a Cognito callback
URL. This is a real, meaningful check — it caught two of the bugs below
(the native redirect URI mismatch, the `react-native-worklets` peer
dependency) before they'd have surfaced on a device — but it is not the
same claim as "ran on a simulator and it worked." No iOS/Android build was
ever produced or executed this phase.

## What was built

**`infra/terraform/modules/auth`**: a Cognito User Pool
(`admin_create_user_config.allow_admin_create_user_only = true` — no public
self-signup, matches `docs/phase-5.md`'s "internal users only" answer on
the Bedrock use-case form), a Hosted UI domain, a Resource Server
(`job-syllabus-api`) defining `read`/`admin` custom OAuth scopes, and a
public PKCE app client (no secret — a mobile/web client can't keep one).
SSM outputs (`user_pool_id`, `user_pool_client_id`, `hosted_ui_domain`) for
the client build and Jenkins to consume.

**`infra/terraform/modules/web`**: S3 (fully private, all public access
blocked) + CloudFront with Origin Access Control, `custom_error_response`
mapping 403/404 → `/index.html` (200) for Expo Router's client-side SPA
routing, `PriceClass_100`.

**`infra/terraform/modules/api-gateway`**: a JWT authorizer against the
Cognito pool, plus two routes more specific than the existing catch-all —
`GET /v1/{proxy+}` requiring the `read` scope, `POST /v1/{proxy+}`
requiring `admin` — since every read in this API is a GET and every write
is a POST (still true after Phase 5's `POST /v1/reviews/{term}`), splitting
by method alone is sufficient without per-path logic. `internal/api`'s own
`ipallow.go` middleware and its test were deleted outright, not left as
dead code — an IP allowlist would actively break the point of a mobile
client signing in from arbitrary networks, not just be redundant with the
new JWT check.

**`mobile/`** (Expo Router, TypeScript, nativewind v4 + Tailwind v3.4.x):
- `src/auth/AuthContext.tsx` + `config.ts`: PKCE sign-in against Cognito
  Hosted UI, native via `expo-auth-session`'s `useAuthRequest`/
  `promptAsync`, web via a manually-constructed `AuthSession.AuthRequest` +
  full-page redirect (see bug #6 below for why not `promptAsync` on web).
  Tokens persisted via `expo-secure-store` on native, `localStorage` on web
  (docs/design.md §8 is explicit: never AsyncStorage).
- `src/api/{types,client,hooks}.ts`: a typed mirror of the Go DTOs, a
  fetch wrapper handling API Gateway's bare (non-RFC7807) 401 body, and
  `@tanstack/react-query` v5 hooks for every `service-api` endpoint.
- `app/`: `(auth)/sign-in`, `(tabs)/{syllabus,postings,companies,review}`,
  using SDK 53+'s `Stack.Protected` for guard-based route protection —
  redirects to the sign-in group when signed out, the tab group when
  signed in.

**`ci/Jenkinsfile.client-build`**: `npm ci` → `tsc --noEmit` → `expo export
-p web` (env vars sourced fresh from SSM, not committed) → `aws s3 sync` →
CloudFront invalidation → smoke test (`<title>Job Syllabus</title>` in the
raw HTML — the SPA itself is client-rendered, so that's the one piece of
real content `curl` can see). Runs on the existing `fargate-agent` label;
no new agent image needed, since Node 20 + eas-cli were added to
`ci/agent.Dockerfile` ahead of need during an earlier phase specifically
for this job.

## What broke

Nine real bugs, roughly in the order they were found. The last three are
the ones that actually blocked the DoD (the app rendering something) rather
than being development-environment friction.

1. **`expo install nativewind tailwindcss` resolved Tailwind to v4**, but
   nativewind v4.2.6's own docs (checked directly, not assumed) still
   target v3.4.x and don't document v4's CSS-first config at all. Fixed by
   explicitly pinning `tailwindcss@^3.4.19` as a devDependency.
2. **`expo-router`'s own dependency chain (`@expo/ui` → radix-ui → vaul)
   wanted `react@19.2.8`** against the SDK's pinned `react@19.2.3`, and
   separately **`expo start --web` needs `react-dom`/`react-native-web`
   installed explicitly** — Expo doesn't pull them in by default even
   though `app.json` already declared a `web` platform config. Both fixed
   via `npx expo install <pkg> -- --legacy-peer-deps`.
3. **`--legacy-peer-deps` had a side effect**: `babel-preset-expo` and
   `react-native-worklets` (a hard peer of `babel-preset-expo` in SDK 57,
   even with no direct Reanimated usage) ended up nested under
   `node_modules/expo/node_modules/...` instead of hoisted to the top
   level, where Metro/Babel's module resolution needs them — surfaced as
   `Cannot find module 'babel-preset-expo'` and, after fixing that,
   `Cannot find module 'react-native-worklets/plugin'`. Fixed by
   explicitly reinstalling both at the top level.
4. **`Stack.Protected`'s redirect-to-anchor-route mechanism needs a real,
   concretely-resolvable index route** — the app initially only had
   `(auth)`/`(tabs)` route groups, no bare `app/index.tsx`, and hit
   "Unmatched Route" (a real 404) at `/`. Fixed by adding
   `app/index.tsx` (a `<Redirect>` based on sign-in state) declared
   *outside* any `Stack.Protected` wrapper in `app/_layout.tsx`, so it's
   always reachable regardless of auth state.
5. **nativewind + react-native-web's known dark-mode interop bug**
   (nativewind/nativewind#1489) crashed the whole web bundle: "Cannot
   manually set color scheme... please use `StyleSheet.setFlag('darkMode',
   'class')`." The first fix attempt — calling that unconditionally —
   made it *worse*, crashing with `StyleSheet.default.setFlag is not a
   function` (that patched method only exists via native's css-interop
   layer, not this web bundle). Reverted to a `typeof` existence check
   before calling it, safe on both platforms.
6. **Cognito's registered native callback URL didn't match what
   `expo-auth-session` actually sends.** `makeRedirectUri({scheme:
   'jobsyllabus'})`'s `path` option is never appended on native (confirmed
   against the SDK docs directly) — native always sends the bare
   `jobsyllabus://` scheme, but Terraform had registered
   `jobsyllabus://redirect`. Fixed by registering the bare scheme.
7. **Web's `promptAsync()` popup reproducibly triggered the browser's own
   popup blocker** — "Popup window was blocked... invoked too long after a
   user input was fired" — confirmed against a real click in a real
   browser, not just reasoned about from docs. Rewrote the web sign-in
   path to a full-page redirect instead (`window.location.href`), manually
   building the `AuthSession.AuthRequest` and persisting the PKCE
   `code_verifier` across the navigation since it has to survive a full
   page reload. Native is unaffected — its in-app-browser flow has no
   popup exposure — so it was left on `promptAsync()`.
8. **`EXPO_PUBLIC_*` env vars never made it into the production bundle**,
   even though `expo export -p web`'s own log line
   (`env: load .env.production.local .env`) showed the file loading
   correctly — the deployed app threw `EXPO_PUBLIC_COGNITO_DOMAIN is not
   set` in the browser console. Root cause wasn't the env-loading
   mechanism at all: `src/auth/config.ts`'s `requireEnv(name)` read
   `process.env[name]` — a **dynamic** bracket-access lookup — but Expo's
   inliner (`babel-plugin-transform-inline-environment-variables`) only
   rewrites **static** `process.env.EXPO_PUBLIC_X` member expressions it
   can pattern-match at build time. The dynamic form survives untouched
   into the bundle as a real runtime property read against `process.env`,
   which is empty in a browser bundle — confirmed directly by grepping a
   real export's output JS for the literal Cognito domain string and
   finding it completely absent. Fixed by rewriting `config.ts` to pass
   each var as a literal static `process.env.EXPO_PUBLIC_X` expression
   into `requireEnv`, instead of looking it up by name inside the
   function. Re-verified by grepping the next export's JS and finding the
   literal domain string present this time.
9. **The deployed API Gateway had no CORS configuration at all** — the
   browser blocked every `fetch()` from the CloudFront origin at the
   preflight stage ("No 'Access-Control-Allow-Origin' header is present").
   Adding `cors_configuration` to the HTTP API fixed the *authorized*
   preflight response, but a **second** issue surfaced immediately after:
   the existing `ANY /{proxy+}` catch-all route matched the OPTIONS
   preflight request too and forwarded it straight to the Go backend
   (which has no OPTIONS handler and returned 405) — HTTP APIs only
   auto-generate the CORS preflight response when *no* route matches the
   OPTIONS request at all. Confirmed directly: `curl -X OPTIONS` against
   the live API returned 405 with the catch-all in place, 204 once it was
   narrowed from `ANY` to `GET` (every real method served today is GET or
   POST, and POST only ever appears under `/v1` where the more specific
   `v1_write` route already owns it, so nothing reachable before was
   dropped). **A third, related bug** then surfaced once CORS itself
   worked: `/v1/skills` came back `{"detail":"no route matches /skills"}`
   — a 404 from the Go backend itself, not API Gateway. The integration's
   `overwrite:path = "/$request.path.proxy"` template was shared across
   the catch-all and the new `v1_read`/`v1_write` routes, but `{proxy}`
   captures a *different* substring depending on which route matched — for
   `GET /v1/{proxy+}` it's only what's after `/v1/` (e.g. `skills`), not
   the full `v1/skills` the catch-all's own `{proxy}` used to capture. So
   the rewritten path silently dropped the `/v1` prefix chi's router
   requires. Fixed with a second, dedicated integration
   (`aws_apigatewayv2_integration.alb_v1`) whose template hardcodes the
   prefix back on: `overwrite:path = "/v1/$request.path.proxy"`.

## What would be done differently

- **Bugs #8 and #9's last two parts were only caught by testing the real
  deployed URL in a real browser, not by `tsc --noEmit` or a local dev
  server.** Local dev never exercises the CORS/API-Gateway-routing path
  at all (Metro's dev server proxies differently), and a shell-exported
  env var during local `expo start` behaves differently from a file-loaded
  one at `expo export` time. The lesson generalizes past this phase: for
  any feature spanning a browser-hosted client and a real deployed API,
  budget time to test the actual deployed artifact end-to-end before
  calling the phase done — "it built" and "`tsc` is clean" are necessary,
  not sufficient.
- **`process.env[dynamicName]` vs. `process.env.STATIC_NAME` is an easy
  trap with build-time env inliners generally** (Expo/Metro here, but the
  same class of bug exists in webpack's `DefinePlugin` and Next.js).
  Worth a standing habit: any helper wrapping `EXPO_PUBLIC_*`/similar
  build-time-inlined vars should take the already-resolved value as a
  parameter, never look it up by a variable name inside the helper.
- **The API Gateway integration path-rewrite bug (bug #9's third part)
  was a direct consequence of adding new routes that reused an existing
  integration without checking whether its `{proxy}`-dependent template
  still meant the same thing for the new route's own path shape.** Worth
  generalizing: whenever a new `{proxy+}` route is added alongside an
  existing one, verify by direct request (not just `terraform plan`)
  that the specific path arriving at the backend is what the backend
  actually expects — Terraform's own plan output has no way to catch a
  logically-wrong-but-syntactically-valid `request_parameters` template.
