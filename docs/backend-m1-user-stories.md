# Backend M1 — Middleware & Memory: User Stories & Learning Path

Companion to [auth-identity-tenant.md](auth-identity-tenant.md) and issue
[#2 — Backend M1: Auth, tenancy, and memory foundation](https://github.com/kvn8888/CodeGym/issues/2).

This doc is written for a learning workflow: hand-write the code (with
autocomplete), then have a separate review pass check it against the acceptance
criteria. Each story points at a **guided skeleton** already in the tree — the
plumbing compiles and runs; the interesting logic is left as `TODO(you)`.

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

## What's left (these stories)

Two real gaps, both left as guided skeletons:

1. **Real signed-token auth** — only `DevAuthenticator` exists today.
2. **The summarizer** — events accumulate (append-only) but nothing derives the
   `Profile` from them. `UpsertProfile` exists in every store and is never called.

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

**Your tasks:** inside `Summarize`, aggregate `events` into `Summary`,
`Strengths`, `GrowthEdges`, `Skills` (and optionally `Notes`). Start with a
`map[string]int` over `Type`/`Source`; unmarshal `event.Payload` when you need
structured signal.

**Acceptance criteria:**
- [ ] `Summarize` is pure (no DB / clock / globals) and never mutates its inputs.
- [ ] Empty event log → valid, timestamped profile (no panic).
- [ ] Re-running over the same events yields the same profile (idempotent).
- [ ] `Service.RefreshProfile` persists the derived profile (plumbing already done).
- [ ] `summarizer_test.go` grows real assertions and stays green.

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

**Your tasks:** solve the "no request context" problem (the file lays out two
options — recommend option (b): a `RefreshProfileFor(ctx, tenantID, userID)`),
then make the tick refresh active profiles. Optionally add a manual trigger:
`POST /api/v1/memory/profile/refresh` → `Service.RefreshProfile`.

**Acceptance criteria:**
- [ ] A summarization failure never blocks or fails the `RecordEvent` request path.
- [ ] The worker shuts down cleanly on context cancellation.
- [ ] (If added) the manual refresh endpoint is covered by the smoke test (issue #6).

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
2. **US-2** (the core memory feature; plumbing is already wired for you).
3. **US-3** then **US-4** (stretch / depends on #7).

All four map back to issue #2's acceptance criteria and its "Open decisions"
(auth provider → US-1; summarizer as job vs. service → US-3).
