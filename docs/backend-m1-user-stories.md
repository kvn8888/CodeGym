# Backend M1 — Middleware & Memory: User Stories & Learning Path

Companion to [auth-identity-workspace.md](auth-identity-workspace.md) and issue
[#2 — Week 5: Finish auth, personal workspace, and memory foundation](https://github.com/kvn8888/CodeGym/issues/2).

This doc started as a learning workflow: hand-write the code (with
autocomplete), then have a separate review pass check it against the acceptance
criteria. US-1, US-2, and US-3 now have a completed v0, while US-4 remains the
main follow-up work.

---

## What already works (don't rebuild it)

The middleware chain and the memory read/write paths are complete:

- **Auth → identity → personal workspace scope** middleware (`internal/auth`,
  `internal/identity`, `internal/workspace`), chained in `internal/api/router.go`.
  A request resolves to an `auth.Principal` and an internal `workspace.Scope` in
  context.
- **Auth0 identity reconciliation**: Auth0 `sub` maps to the durable CodeGym
  user id, each user gets a deterministic personal workspace, and optional
  Auth0 `email`/`name` claims are reconciled into `app_users` metadata.
- **Memory storage**: `memory.Store` interface with both `InMemoryStore` and
  `PostgresStore`; model types (`Profile`, `Event`, `Note`, `SkillProficiency`).
- **Memory service** read/record: `GetProfile`, `RecordEvent`, `ListEvents`, all
  workspace+user scoped, plus the HTTP handlers under `/api/v1/memory/*`.
- **Memory profile refresh v0**: `Summarize`, `RefreshProfile`,
  `RefreshProfileFor`, `RefreshAllProfiles`, the manual refresh endpoint, and
  `memory.Worker.RunOnce` are implemented and tested.

## What's left (these stories)

The remaining backend implementation gap is:

1. **Strict memory event validation** — the naming guide exists, but events are
   still accepted as flexible `source`/`type` strings until the service adds a
   strict validation hook.

---

### US-1 — Authenticate requests with a verifiable signed token

> **As a** CodeGym user, **I want** my requests authenticated by a verifiable
> signed token, **so that** my memory rows are tied to a real identity instead of
> a shared dev token.

**Implementation:** [`backend/internal/auth/jwt_authenticator.go`](../backend/internal/auth/jwt_authenticator.go)
· **Test harness:** `backend/internal/auth/jwt_authenticator_test.go`

**Learning focus:** how a stateless Auth0 access token proves identity with
issuer, audience, RS256 signature, expiry, and not-before validation.

**Implemented path:**
1. Configure Auth0 issuer/audience from env.
2. Fetch and cache Auth0 JWKS for RS256 verification.
3. Let Auth0's Go validator reject malformed, tampered, wrong-issuer,
   wrong-audience, expired, and not-yet-valid tokens.
4. Map validated claims into `auth.Principal`.
5. Map each Auth0 user to one deterministic personal workspace.
6. Reconcile Auth0 `email` and `name` as optional app-user metadata.
7. Preserve `DevAuthenticator` as the local fallback.

**Acceptance criteria:**
- [x] Implements `auth.Authenticator` (so it drops into `auth.Middleware`).
- [x] Valid token → `Principal{UserID, DefaultWorkspaceID, WorkspaceIDs}` for the
  user's personal workspace.
- [x] Optional Auth0 `email` and `name` metadata flows through identity
  middleware into the durable app-user bootstrap.
- [x] Missing / malformed / wrong-audience / wrong-issuer / tampered / expired
  -> `ErrUnauthenticated`.
- [x] `jwt_authenticator_test.go` passes.
- [x] Wired from `main.go` behind config without breaking the dev-token flow.

**Done when:** `go test ./internal/auth/ -run TestAuth0Authenticator -v` is green.

---

### US-2 — Derive the memory profile from events (the summarizer)

> **As a** returning user, **I want** my memory profile to reflect what I have
> actually done, **so that** generated problems match my real level and gaps.

**Skeleton:** [`backend/internal/memory/summarizer.go`](../backend/internal/memory/summarizer.go)
(the pure `Summarize` function) · wired by `Service.RefreshProfile` in
`service.go` · baseline test `summarizer_test.go`.

**Learning focus:** turning an append-only event log into a derived view; keeping
the core a **pure, idempotent** function so it is easy to test and safe to re-run.

**Completed v0:** `Summarize` aggregates `events` into `Summary`, `Strengths`,
`GrowthEdges`, `Skills`, and `Notes`. It reads common structured payload fields
such as `skill`, `skills`, `topic`, `problem_id`, `passed`, and `correct`, while
falling back to event source and summary keywords when payloads are sparse.

**Acceptance criteria:**
- [x] `Summarize` is pure (no DB / clock / globals) and never mutates its inputs.
- [x] Empty event log → valid, timestamped profile (no panic).
- [x] Re-running over the same events yields the same profile (idempotent).
- [x] `Service.RefreshProfile` persists the derived profile.
- [x] `summarizer_test.go` has event-derived assertions and stays green.

**Done when:** record a few events, call `RefreshProfile`, then `GET /api/v1/memory/profile`
returns a summary derived from those events (not the default placeholder).

---

### US-3 — Update profiles on an isolated / async boundary

> **As the** system, **I want** profile updates to happen behind an isolated
> boundary, **so that** product requests stay fast and raw events are preserved
> even if a summary is wrong and needs regenerating.

**Skeleton:** [`backend/internal/memory/worker.go`](../backend/internal/memory/worker.go)

**Learning focus:** background work in Go — goroutine + `time.Ticker` + context
cancellation; and the real design question of *how a background job gets an
identity/workspace scope* when there is no HTTP request.

**Completed v0:** `RefreshProfileFor(ctx, workspaceID, userID)` handles explicit
internal workspace-scope refreshes, `RefreshAllProfiles` refreshes every
workspace/user pair with events, `memory.Worker.RunOnce` exposes the worker
behavior for tests, and `POST /api/v1/memory/profile/refresh` manually refreshes
the current request scope.

**Acceptance criteria:**
- [x] A summarization failure never blocks or fails the `RecordEvent` request path.
- [x] The worker shuts down cleanly on context cancellation.
- [x] Manual refresh endpoint is documented in OpenAPI.

---

### US-4 (stretch) — Validate event names against the naming guide

> **As a** maintainer, **I want** memory event `source`/`type` validated against a
> canonical list, **so that** events stay queryable as more surfaces emit them.

Depends on the [memory event naming guide v0](memory-event-naming-guide-v0.md),
created for [#7](https://github.com/kvn8888/CodeGym/issues/7). Add a validation
hook in `Service.RecordEvent` that rejects unknown `source`/`type` (at least in
a strict mode), and make `Summarize` rely on those canonical names. This is what
makes US-2's aggregation tractable.

---

## Suggested order

1. **US-4** strict validation now that the memory event naming guide exists.

US-1, US-2, and US-3 map to issue #2's foundation. US-4 depends on the naming
guide from issue #7.
