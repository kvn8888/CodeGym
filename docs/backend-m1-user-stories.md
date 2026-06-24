# Backend M1 — Middleware & Memory: User Stories & Learning Path

Companion to [auth-identity-tenant.md](auth-identity-tenant.md) and issue
[#2 — Backend M1: Auth, tenancy, and memory foundation](https://github.com/kvn8888/CodeGym/issues/2).

This doc started as a learning workflow: hand-write the code (with
autocomplete), then have a separate review pass check it against the acceptance
criteria. US-2 and US-3 now have a completed v0, while US-1 and US-4 remain the
main follow-up work.

---

## What already works (don't rebuild it)

The middleware chain and the memory read/write paths are complete:

- **Auth → identity → tenant** middleware (`internal/auth`, `internal/identity`,
  `internal/tenant`), chained in `internal/api/router.go`. A request resolves to
  an `auth.Principal` and a `tenant.Scope` in context.
- **Memory storage**: `memory.Store` interface with both `InMemoryStore` and
  `PostgresStore`; model types (`Profile`, `Event`, `Note`, `SkillProficiency`).
- **Memory service** read/record: `GetProfile`, `RecordEvent`, `ListEvents`, all
  tenant+user scoped, plus the HTTP handlers under `/api/v1/memory/*`.
- **Memory profile refresh v0**: `Summarize`, `RefreshProfile`,
  `RefreshProfileFor`, `RefreshAllProfiles`, the manual refresh endpoint, and
  `memory.Worker.RunOnce` are implemented and tested.

## What's left (these stories)

The remaining backend gaps are:

1. **Real signed-token auth** — only `DevAuthenticator` exists today.
2. **Strict memory event naming** — events are accepted as flexible
   `source`/`type` strings until the event-name convention is finalized.

---

### US-1 — Authenticate requests with a verifiable signed token

> **As a** CodeGym user, **I want** my requests authenticated by a verifiable
> signed token, **so that** my memory rows are tied to a real identity instead of
> a shared dev token.

**Skeleton:** [`backend/internal/auth/jwt_authenticator.go`](../backend/internal/auth/jwt_authenticator.go)
· **TDD harness:** `backend/internal/auth/jwt_authenticator_test.go`

**Learning focus:** how a stateless JWT proves identity — base64url segments,
HMAC-SHA256 signing, constant-time verification, expiry checks.

**Your tasks (the 5 `TODO(you)` steps in the file):**
1. Split `header.payload.signature`.
2. Recompute the HMAC and constant-time compare (`hmac.Equal`) — reject tampered
   tokens and tokens signed with the wrong secret.
3. base64url-decode + `json.Unmarshal` the claims.
4. Reject expired / not-yet-valid tokens (`exp`, `nbf`).
5. Map claims → `auth.Principal`.

**Acceptance criteria:**
- [ ] Implements `auth.Authenticator` (so it drops into `auth.Middleware`).
- [ ] Valid token → `Principal{UserID, DefaultTenantID, TenantIDs}`.
- [ ] Missing / malformed / wrong-secret / tampered / expired → `ErrUnauthenticated`.
- [ ] `jwt_authenticator_test.go` passes with its `t.Skip` removed.
- [ ] Wirable from `main.go` behind config without breaking the dev-token flow.

**Done when:** `go test ./internal/auth/ -run TestJWTAuthenticator -v` is green.

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
identity/tenant scope* when there is no HTTP request.

**Completed v0:** `RefreshProfileFor(ctx, tenantID, userID)` handles explicit
scope refreshes, `RefreshAllProfiles` refreshes every tenant/user pair with
events, `memory.Worker.RunOnce` exposes the worker behavior for tests, and
`POST /api/v1/memory/profile/refresh` manually refreshes the current request
scope.

**Acceptance criteria:**
- [x] A summarization failure never blocks or fails the `RecordEvent` request path.
- [x] The worker shuts down cleanly on context cancellation.
- [x] Manual refresh endpoint is documented in OpenAPI.

---

### US-4 (stretch) — Validate event names against the naming guide

> **As a** maintainer, **I want** memory event `source`/`type` validated against a
> canonical list, **so that** events stay queryable as more surfaces emit them.

Depends on [#7 — memory event naming guide v0](https://github.com/kvn8888/CodeGym/issues/7).
Add a validation hook in `Service.RecordEvent` that rejects unknown
`source`/`type` (at least in a strict mode), and make `Summarize` rely on those
canonical names. This is what makes US-2's aggregation tractable.

---

## Suggested order

1. **US-1** (self-contained; TDD harness gives instant feedback).
2. **US-4** once the memory event naming convention is approved.

US-2 and US-3 map to issue #2's completed memory-refresh foundation. US-1
still depends on the auth provider decision, and US-4 depends on issue #7.
