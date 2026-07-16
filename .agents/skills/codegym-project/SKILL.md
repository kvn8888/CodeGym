---
name: codegym-project
description: Living project knowledge for CodeGym v2, a React/Vite frontend plus Go backend for generative coding practice. Consult this skill first when starting CodeGym work. Contains branch defaults, architecture decisions, validation paths, GitHub Projects workflow, current project scope, and conventions. Update this skill whenever architecture, requirements, project-board workflow, or durable implementation details change so agents inherit accurate context across sessions.
---

# CodeGym Project

This is the single source of truth for recurring CodeGym agent work. Read this
before changing code, issues, or the project board. Update it when product scope,
architecture, workflow, validation, or durable gotchas change.

## Live Project Memory Rule

Treat this skill as living project memory. When a code, docs, board, or
deployment change introduces or changes a durable repo fact, operational gotcha,
vendor setup assumption, architecture decision, validation rule, risk, or
debugging pattern, update this skill before finishing the task.

Keep updates concise and remove stale or contradictory guidance rather than
layering caveats. Do not record transient branch status, one-off command output,
or momentary CI failures unless they change the durable workflow. When this
skill conflicts with current code, live GitHub issues/PRs, or the project board,
trust the current implementation and live planning state, then repair this skill.

## Branch Default

`codegym-v2` is the default working branch for now.

This branch now contains the React/Vite frontend and the Go backend foundation.
Backend auth, personal workspace bootstrap, Neon/Postgres memory storage, and
the memory service v0 are in scope. Legacy execution infrastructure, Docker
runners, problem-pack fixtures, organization/team tenancy, and deploy automation
should still stay out unless the user explicitly asks to reintroduce them.

## Publishing and PR Discipline

Do not leave completed repo work only in the local checkout. Branches are
temporary PR heads, not the deliverable. Unless Kevin explicitly asks for
local-only exploration, every code, docs, process, or project-helper change that
should persist in the repository must end the session committed, pushed to the
remote, and attached to a GitHub PR.

Use this publish flow:
1. Check `git status --short --branch` and inspect the diff before staging.
2. Keep unrelated local/user changes out of the commit.
3. Commit the scoped changes with a terse message.
4. Push the active branch to `origin` and set upstream to the matching remote
   branch.
5. Open a draft PR against `codegym-v2`, or push updates to the existing PR for
   the branch if one already exists.
6. Move linked board work to `In review` once the PR exists. Move it to `Done`
   only after merge/landing and validation evidence.

If publishing is blocked by auth, network, failing validation, missing owner
approval, or mixed unrelated work, say exactly what is blocked and what must
happen next. Do not present local-only work as finished repository work.

## Current State (Last Updated: 2026-07-15)

- Branch: `codegym-v2`.
- Frontend: React 19 + Vite 7, Storybook 10, Motion/Framer-style animations, Monaco editor, and mock-friendly app routes. The visual system is a quiet operational workspace: 24px routine page titles, flat border-led surfaces, compact rows, a 216px desktop sidebar, and a dedicated mobile navigation sheet. Sidebar nav is Dashboard, New practice, History, Memory, Settings (Marathon is not a nav item). Dashboard composes active/recent sessions with memory-derived recommendations; Memory composes the profile, event history, focus queue, skills, and notes; `/generate` is labeled New Practice and lets users select practice format (multiple choice vs DSA-style coding problem). MCQ starts a marathon session; coding opens the problem workspace shell (AI coding generation still partial). New Practice creates a complete durable MCQ snapshot and hands it to Marathon, which restores the exact question, timer, and draft selection. Bare `/marathon` redirects to `/generate`; only launch state or `?session=` may enter the run UI. History (`/`) and New Practice surface persisted MCQ runs with resume/result links. Marathon Skip records neutral continuity state without answer correctness events, while Exit awaits an active-session save before leaving. Completing every MCQ question set shows a top-right `Updating memory` / `Memory updated` toast; Finished and Next Round both await the completion event plus notes maintenance, and the next set is generated only after that reflection succeeds. Floating chat is limited to active Marathon and problem-detail contexts. Settings exposes `/api/v1/me` and lets users save a custom display name.
- Backend: Go service with `auth -> identity -> personal workspace scope -> handler` request path, Auth0 RS256/JWKS bearer-token validation behind `auth.Authenticator`, dev-token auth fallback, Auth0 `sub` to personal workspace bootstrap, optional Auth0 `email`/`name` user-metadata reconciliation, `/api/v1/me` account profile APIs, memory profile/event APIs, session APIs, Neon/Postgres store support, deterministic memory summarization, manual profile refresh, and worker-ready profile refresh. Auth0/Google `name` seeds `app_users.display_name` with `display_name_source=oauth`; Settings updates set `display_name_source=user`, and future OAuth metadata must not overwrite user-edited names. Production Auth0 uses the custom API Identifier `https://codegym.onrender.com` (not the Auth0 Management API audience) in matching backend/frontend Doppler configs. The CodeGym SPA requires a `subject_type: user` client grant for that API. Render and Vercel were redeployed with this corrected setup on 2026-07-15.
- Generation: `generation.Orchestrator` composes the scoped memory profile with the provider-neutral `Generator` seam. `generation/openaicompat` is the first adapter (OpenAI chat-completions wire format; default base URL is the Vercel AI Gateway). `POST /api/v1/generate` serves MCQ sets with structural validation and one bounded repair retry; it answers 503 when `CODEGYM_GENAI_API_KEY` is unset. MCQ prompt text mirrors `docs/ai-prompts/05-mcq-marathon.md` — keep them in sync.
- Memory notes are the LLM-curated slice of the profile (agentic CRUD, like editing a living project doc): `POST /api/v1/memory/notes/maintain` runs deterministic profile refresh, then a best-effort LLM pass (`generation.MaintainNotes`, prompt mirrors `docs/ai-prompts/08-memory-notes.md`) that creates/updates/prunes notes from the round's events and emits `memory.note_created/updated/pruned` audit events. The deterministic `memory.Summarize` preserves notes by ID, so worker/cron refreshes never clobber LLM curation. The user-level summary stays deterministic until the daily cron work (#41) lands.
- Deploy: backend runs on Render (`https://codegym.onrender.com`); `render.yaml` is the blueprint and `docs/render-deploy.md` documents applying it. The server honors `PORT` as a fallback for `CODEGYM_PORT`; `CODEGYM_HOST=0.0.0.0` is required on Render.
- Secrets: Doppler is the preferred local secret runner; `NEON_CONNECTION_STRING` is checked before `DATABASE_URL`. GenAI: `CODEGYM_GENAI_BASE_URL` / `CODEGYM_GENAI_API_KEY` (alias `AI_GATEWAY_API_KEY`) / `CODEGYM_GENAI_MODEL`.
- CI: `.github/workflows/ci.yml` runs frontend `npm ci`, lint, build, and backend `go test ./...` on PRs/pushes to `codegym-v2`. `.github/workflows/openapi-lint.yml` runs `npm run api:lint` (Redocly) when `api/**` changes.
- Project board: GitHub Projects v2 project `#2` (`CodeGym v2`) is the active kanban unless the user says otherwise.

## Workflow Decision Tree

1. Identify the task surface area.
- Frontend route/page/API consumption: inspect `frontend/src/App.tsx`, `frontend/src/features/`, and `frontend/src/shared/api/`.
- Backend API/auth/workspace/memory work: inspect `backend/README.md`, `docs/auth-identity-workspace.md`, and `backend/internal/`.
- Project scope or product intent: read the "Scope Specification" section in this skill first.

2. Select the shortest local validation path.
- Frontend-only change: run `cd frontend && npm run lint` and `cd frontend && npm run build` when type-level confidence is needed.
- Backend-only change: run `cd backend && go test ./...`.
- CI parity check: run `cd frontend && CI=true npm ci --no-audit --progress=false`, then lint/build, plus backend tests.

3. Keep implementation aligned with the frontend-first branch.
- Stub backend-dependent flows behind clear interfaces or mock data when needed.
- Do not re-add legacy Docker execution, deploy automation, or problem-pack files unless the user explicitly asks.

4. Prefer minimal, task-focused edits.
- Avoid broad refactors unless required by the task.
- Preserve existing frontend naming, route, and component conventions.

## Project Conventions

### Frontend

- Start client with `cd frontend && npm run dev` (runs Vite).
- Start Storybook with `cd frontend && npm run storybook` (runs at port 6006).
- Define app routes in `frontend/src/App.tsx`.
- Keep pages grouped by feature under `frontend/src/features/`.
- Keep generated-problem, MCQ, conversational-practice, and memory/profile flows visible in the frontend UX.
- For new page components, add a `.stories.tsx` file next to the component file.

### Backend

- Start backend work in `backend/internal/` and keep HTTP routing thin.
- Preserve the request path conceptually: auth middleware establishes
  `auth.Principal`, identity middleware bootstraps/persists the user and a
  personal workspace, workspace-scope middleware establishes the active scope,
  and handlers read scoped services. Scope uses the `workspace` package and
  `workspace_id` fields for personal workspace isolation; do not expand that
  into organization/team SaaS tenancy unless product scope changes.
- Memory writes should append events quickly. Profile summarization should stay
  behind `Service.RefreshProfile` or a worker boundary so request paths do not
  block on derived-memory work.
- Memory event emitters should use the canonical `source` and `type` names in
  `docs/memory-event-naming-guide-v0.md`. Add new names there before frontend,
  backend, or script emitters start writing a new event shape.
- Auth0 is the selected production auth provider. Auth0 JWT validation now
  lives behind the existing `auth.Authenticator` boundary; preserve the
  `DevAuthenticator` path for local fallback and tests. Auth0 `sub` is the
  durable CodeGym user ID. Auth0 `email` and `name` are optional app-user
  metadata only; never use them for authorization or workspace selection.
- GenAI provider code should live behind a provider client/adapter, but that
  client should not fetch memory directly. Generation/chat orchestration should
  compose `memory.Service` with the GenAI client, build the prompt/context, then
  call the provider adapter.
- Prefer explicit docs in `docs/` for durable backend contracts and decisions,
  then link them from `backend/README.md` when they become canonical.

### Static Analysis

Keep correctness CI and static-analysis CI separate. The default `CI` workflow is
the required compile/test gate. Add advisory static analysis first, then tighten
once the baseline is understood.

Recommended frontend path:
- Keep `npm run lint` in the main CI workflow.
- For React-specific review, start with React Doctor locally:
  `cd frontend && npx react-doctor@latest --verbose --diff --blocking none`.
- If adding React Doctor CI, use a separate workflow and point it at
  `directory: frontend`; start with `blocking: none` and promote to blocking
  only after noisy findings are triaged.

Recommended backend path:
- Keep `go test ./...` in the main CI workflow.
- Add `go vet ./...` as the first low-cost static check.
- Add `golangci-lint` in a separate job/workflow once the config is pinned.
- Add `govulncheck ./...` as an advisory dependency/security check; treat it as
  vulnerability reachability analysis, not a style linter.

## Validation Matrix

Use the narrowest validation path that covers the changed behavior. Record the
commands and results in the issue/PR when the work maps to the board.

| Surface | Blocking check | Advisory or situational check | Notes |
| --- | --- | --- | --- |
| Frontend route, component, or API consumption | `cd frontend && npm run lint`; `cd frontend && npm run build` | Storybook local review for visible component work | Add or update a `.stories.tsx` file for new page components. |
| Storybook-only work | `cd frontend && npm run storybook` long enough to load the changed story | Screenshot or browser check for visual regressions | Storybook is not a substitute for `npm run build` when app code changes. |
| Backend API/auth/workspace/memory code | `cd backend && go test ./...` | `go vet ./...`, `golangci-lint`, `govulncheck ./...` when the issue targets static/security checks | Go cache sandbox failures are environmental; rerun with an allowed cache path or approved escalation before calling tests broken. |
| OpenAPI or HTTP contract change | `npm run api:lint` when `api/**` changes | Compare backend routes, OpenAPI, and typed frontend helpers for drift | Do not mark API work done when docs/spec changed but handlers or client helpers did not. |
| Auth0 or external auth setup | Backend tests plus config/doc review | Owner verification in Auth0, Render, Vercel, and deployed preview/prod envs | External console work belongs in `agency:external-blocked` until Kevin or a permitted agent confirms it. |
| Memory worker/scheduling | Backend tests plus route/worker boundary review | Manual worker start/schedule check once worker runtime exists | Request paths append raw events; derived profiles and summaries belong behind `memory.Service.RefreshProfile` or the worker boundary. |
| Generation-memory orchestration | Backend tests and prompt/context unit coverage when available | Manual generated-problem flow check when wired end to end | GenAI provider adapters should not fetch memory directly. Orchestration composes memory with provider calls. |

## Operational Readiness / External Setup

Auth0 readiness requires the app's callback/logout URLs, allowed web origins,
frontend Auth0 envs, backend issuer/audience envs, and the Auth0 API Identifier
to agree. Auth0 `sub` is the durable user ID; `email` and `name` are metadata
only and must not drive authorization or workspace selection.

For the current Auth0 tenant, the CodeGym API Identifier is
`https://codegym.onrender.com`. Never use the tenant Management API audience
ending in `/api/v2/` for CodeGym access tokens. Because the API's user access
policy is `require_client_grant`, authorize the SPA with a user-delegated
client grant (`subject_type: user`); a machine grant (`subject_type: client`)
does not authorize logged-in SPA users.

Render readiness for the Go backend means the service has the expected branch,
start command, health check, Auth0 envs, database envs, and secret source wired.
Deployment is not complete just because `.env.example` or docs list the values.

Vercel/frontend readiness means preview and production environments have the
matching Auth0 domain/client/audience values and point at the intended backend
base URL. If frontend and backend are on different origins, confirm CORS and
Auth0 allowed origins together.

Doppler and Neon are the preferred local secret and Postgres targets. Prefer
`NEON_CONNECTION_STRING` over `DATABASE_URL` when both exist. Never print or
commit secret values while documenting setup.

Preprod and prod should stay distinguishable. Safe preprod can share code and
schema shape, but should use separate Auth0/Vercel/Render/Neon resources or
explicitly safe config. Do not mark external setup issues `Done` without a
comment that names the environment verified and the validation performed.

## Current Risk Register

Verified from the codebase and board reconciliation work around 2026-07-02.
Updated during the backend/session/generation implementation pass on 2026-07-06.
Treat exact PR/check status as time-sensitive, but keep the risk categories
current as implementation lands.

### Critical Open Risks

- The memory worker is scheduled from the API server with configurable interval,
  but production readiness still needs deployed schedule/env validation and
  observability for refresh failures.
- Session/resume now has backend schema/store and protected `/api/v1/sessions`
  routes, but frontend resume/history is still incomplete until typed client
  wiring and #74 land.
- Frontend memory/profile rendering does not finish typed API infrastructure.
  Shared API errors still need typed handling and memory-specific helpers before
  memory UX can rely on them broadly.
- Memory-aware generation has a provider-neutral interface and memory-aware
  orchestration scaffold, but provider adapters, HTTP routes, and frontend
  flows still need follow-on implementation.

### Medium Risks

- #85 tracks when the first authenticated backend request creates the persisted
  account/workspace record. This is separate from the verified Auth0 sign-in
  and token configuration.
- Project board state can drift from PR state and code reality, especially when
  draft/conflicting PRs or docs-only spikes are moved too far right.
- OpenAPI drift can appear when backend routes, `api/**`, and frontend helpers
  are changed independently.
- Agents may still treat "workspace" as multi-org SaaS tenancy. It is personal
  data scope only; legacy `tenant` env/header aliases are compatibility shims.
- Deployment/env drift can hide behind passing local tests because Auth0,
  Render, Vercel, Doppler, and Neon readiness depends on external state.

### Already Mitigated / Guardrails

- Auth0 validation is behind `auth.Authenticator`, with `DevAuthenticator` kept
  for local fallback and tests.
- Auth0 `sub` is the durable user identifier; optional email/name metadata is
  explicitly not an authorization boundary.
- Memory event names have a canonical naming guide. New event source/type names
  should be added there before emitters write new shapes.
- Session history uses `practice_sessions` plus `session_files`; memory events
  carry compact learning signals only and are not a resume data store.
- Generation provider adapters implement the provider-neutral `Generator`
  interface; the orchestration layer composes memory context before adapter
  calls.
- The GitHub Project helper can create/upsert/import issues, set Project fields,
  link sub-issues, verify field writes, and warn about sparse process metadata.

## Stale Docs / Source Of Truth

Docs in `docs/` are valuable for contracts and decisions, but implementation
truth is the current code plus live GitHub issue/PR/project-board state. A doc
or spike proves intent; it does not prove the feature is implemented.

When docs, board state, and code disagree:
- Trust the current code for runtime behavior.
- Trust live PR/issue state for review/merge status.
- Trust the project board for current planning only after checking the linked
  issue/PR and repairing obvious drift.
- Update stale docs or this skill when the mismatch is durable.

Session-history/resume docs do not mean session storage, sessions HTTP routes,
OpenAPI coverage, or frontend resume/history UX are complete. Auth0 setup docs
do not mean vendor-console configuration is complete. Memory architecture docs
do not mean the worker is scheduled or running.

## Common Pitfalls

1. Treating docs-only work as implementation completion.
2. Moving cards to `Done` because a PR exists, before merge/landing and
   validation evidence.
3. Using Auth0 `email` or `name` for authorization, identity durability, or
   workspace selection instead of Auth0 `sub`.
4. Expanding personal workspaces into organization/team tenancy without an
   explicit product-scope change.
5. Mixing raw memory events with derived profiles/notes; raw facts are written
   quickly, derived understanding belongs behind service/worker refresh.
6. Letting provider adapters fetch memory directly instead of composing memory
   in generation/chat orchestration.
7. Updating backend routes, OpenAPI, or frontend typed helpers without checking
   the other two surfaces for drift.
8. Assuming local tests prove Auth0/Render/Vercel/Doppler/Neon readiness.
9. Using the Auth0 Management API audience or a machine client grant for the
   SPA instead of the CodeGym API Identifier plus a user-delegated grant.

## Task Workflows

### Run an Implementation Goal Loop

Use this process when Kevin asks to "set a goal", "start a loop", reconcile a
track until complete, or run a multi-issue implementation cycle. The coordinator
agent owns the loop state, board truth, issue sequencing, validation evidence,
and pause/resume decisions.

Loop shape:
1. Read this skill, inspect the relevant GitHub issues/project fields, open PRs,
   and current code before editing.
2. Reconcile drift first: separate landed code, open PRs, board status, and
   docs-only artifacts. Repair obvious board/status mismatches before starting
   new implementation work.
3. Build or refresh a dependency-ordered issue sequence. Keep spikes and
   owner/architecture decisions ahead of stories that depend on them.
4. Move the active issue to `In progress`, implement only that issue's
   acceptance criteria, run the narrow validation path, and leave an issue
   comment with outcome, validation, links, and blockers.
5. Move work to `In review` when a PR exists or to `Done` only when the work is
   actually merged/landed and validated, with any validation gap documented.
6. Pick the next unblocked issue and repeat until the goal is complete or a
   real pause condition is hit.

Pause conditions:
- External setup is required in Auth0, Render, Vercel, Doppler, Neon, GitHub
  settings, or another vendor console.
- Kevin must make a product, academic-scope, risk, launch, or architecture
  decision before implementation would be meaningful.
- The coordinator needs explicit permission for a write action outside normal
  repo edits, such as merging clean PRs, pushing branches, modifying secrets, or
  changing production/preprod settings.

Parallel-agent rules:
- Spawn subagents only after the coordinator has a current board/code/PR read
  and has assigned each subagent one issue or one narrow review task.
- Do not parallelize issues that modify the same API contract, generated client,
  migration/schema boundary, auth middleware path, or memory service boundary
  unless one branch is explicitly the base for the other.
- Good parallel lanes are: PR/CI closeout, isolated docs/process updates,
  backend worker scheduling, frontend memory UX, and session-store spike work
  after its architecture doc is accepted.
- The coordinator remains the only agent that reconciles the board globally,
  declares the goal complete, or pauses for Kevin.
- Each subagent must report issue number, files changed, validation command and
  result, branch/PR if any, and remaining blockers. The coordinator folds that
  into issue comments and board moves.

For auth/backend/memory completion loops, keep the tracks distinct:
- Auth/backend follow-ups: eager account-bootstrap timing (#85), deployment
  drift monitoring, backend static checks, OpenAPI drift checks, and lint baseline.
- Memory services: typed frontend API helpers, memory UI/backend wiring, product
  flow memory events, worker scheduling/backfill, and memory-aware generation
  orchestration.
- Session/resume backend: session schema/store, sessions API/OpenAPI, then
  frontend resume/history integration. Do not treat session docs as
  implementation completion.

### Manage the GitHub Project Board

Use `.agents/skills/codegym-project/scripts/github_project_board.py` when an agent needs to inspect or update the CodeGym GitHub Projects v2 kanban board. The helper talks directly to GitHub GraphQL Projects v2 because ordinary connector surfaces do not expose every project field mutation. Treat the board as live planning state, not a static backlog.

**Script maintenance**: Treat the repo-local path above as the stable command entrypoint, but do not assume it is the canonical source file forever. Before changing the script, resolve the real file with `realpath .agents/skills/codegym-project/scripts/github_project_board.py` or `readlink`, then modify the resolved canonical file and update this skill if the shared location changes. Do not patch a stale copied script while another project points at the shared target.

Requirements:
- Set `GH_TOKEN` or `GITHUB_TOKEN` with repository and Projects v2 permissions. In local Codex sessions, `GH_TOKEN="$(gh auth token)"` is usually enough when `gh auth status` is healthy.
- Default owner/repo: `kvn8888/CodeGym`.
- Default project title lookup: `CodeGym`.
- Active project: pass `--project-number 2` or set `CODEGYM_GITHUB_PROJECT_NUMBER=2`.
- Current Status columns are `Backlog`, `Ready`, `In progress`, `In review`, and `Done`. Use `Ready` for autonomous work that is queued; use `Backlog` for work that is known but not ready to start.
- Current Category options are `Spikes`, `Frontend / UX`, `Backend / API`, `AI / Generation`, `Execution / Sandbox`, `Memory / Personalization`, and `Project / Process`.
- Current Priority options are `P0`, `P1`, and `P2`; Size options are `XS`, `S`, `M`, `L`, and `XL`; `Source` is a text field for the origin of the task.
- GitHub labels are used as an agent work router. Every planned issue should carry exactly one primary `agency:*` label, exactly one `agent:*` label, and optionally an `output:*` label:
  - `agency:ready` — a coding agent can implement or validate from repo context with little owner input.
  - `agency:investigate` — a coding agent can independently produce useful diagnosis, evidence, or a plan; implementation may or may not follow.
  - `agency:needs-owner-decision` — Kevin needs to decide product, academic scope, risk, launch posture, or tradeoffs before implementation.
  - `agency:needs-architecture-decision` — system shape must be decided before code should move.
  - `agency:external-blocked` — action depends on GitHub/Vercel/Doppler/Neon/vendor/access/settings outside the repo; this can be combined with another agency label when both apply.
  - `agent:standard` — suitable for cheaper/medium coding agents: scoped UI, docs, tests, scripts, small backend changes.
  - `agent:strong` — use a strong coding agent for cross-layer frontend/backend work, auth/workspace/memory behavior, generation orchestration, or tricky tests.
  - `agent:frontier` — reserve for ambiguous architecture/product/security decisions where a frontier model is worth the cost.
  - `output:plan-only` — expected output is a decision memo, issue comment, or implementation plan before code.
- Routing rule of thumb: queue `agency:ready` and `agency:investigate` work for autonomous overnight agents; save `agency:needs-owner-decision` and `agency:needs-architecture-decision` for interactive planning sessions. Do not mark work `agency:ready` if a product, academic, vendor, or architecture decision is still missing.
- After work sessions that touch planned board items, update the relevant issue/project fields so the board reflects reality. Move actively worked issues to `In progress`, review-bound work to `In review`, verified completed issues to `Done`, and leave blockers or validation notes in the issue body/comment when appropriate.
- Treat the Kanban board as a living operating system. When a user assigns a GitHub Project issue or a task clearly tied to an existing issue, the coding agent should:
  1. Inspect the issue and linked project fields before editing code.
  2. Move the issue to `In progress` when substantial work begins.
  3. Keep implementation scoped to the issue's acceptance criteria.
  4. Before finishing, update the issue with outcome, validation, blockers, or links to the relevant commit/PR when available.
  5. Move the issue to `Done` only after the requested work is genuinely complete and validation has run or the validation gap is documented.
- Repopulate the board during normal work. If an agent discovers real follow-up work outside the current scope, create a new GitHub Issue instead of expanding the task: production bugs, missing tests, product ambiguity, architecture decisions, external setup, retrospective "what remains" items, AI/generation incidents, or refactors too large for the current change. Do not create issues for tiny fixes that can be safely included in the active task.
- New agent-created issues should include context/source, acceptance criteria, likely starting files, risk or user impact, `Status`, `Category`, `Priority`, `Size`, `Source`, one primary `agency:*` label, one `agent:*` label, and `output:plan-only` when the expected next step is a memo rather than code.
- CodeGym uses Project fields, not `category:*` or `priority:*` labels, as the
  durable source for category and priority. Do not copy another repo's label
  taxonomy blindly. Add optional type labels such as `bug`, `enhancement`,
  `documentation`, or `technical-debt` only when the issue text supports them
  and those labels exist in this repository.
- Do not pad issues with inaccurate labels or fields to silence tooling, but do
  avoid sparse issue metadata when the title/body clearly gives enough evidence
  to set routing, category, priority, size, source, and expected output.
- The board helper emits non-blocking warnings when issue creation/import/upsert
  metadata is missing a primary agency route, agent route, decision
  `output:plan-only`, or required Project fields. Treat warnings as a prompt to
  improve issue metadata before dispatching agents, not as a reason to invent
  inaccurate labels or fields.
- Draft Project cards are inbox items. Convert durable work to real GitHub Issues when it needs labels, comments, links, or agent routing; leave rough brainstorms as drafts until they are actionable.
- Model large capabilities as **epics with sub-issues** (GitHub-native parent/child, with a `subIssuesSummary` rollup). An epic is a parent issue (title prefix `Epic:`) that opens with a gating **spike** wherever uncertainty is real, then **stories**. Spikes carry `agency:investigate` or `agency:needs-architecture-decision` + `output:plan-only`, and their Definition of Done is *"the implementation stories now exist, each with acceptance criteria"* — progressive elaboration: do not pre-write story acceptance criteria a spike will change. Stories carry `agency:ready`. Link children with `create-issue --parent <#>` or `add-sub-issue <parent> <child>`; `list`/`show` surface the rollup and parent/child. The M2 scope features are tracked as epics #35 (generation), #36 (execution sandbox), #38 (verification), #40 (personalization & memory), and #42 (MCQ).
- Do not use `M1:` as an active kanban bucket. M1 is historical/foundation scope; active work should be named as `Week N: ...` issues under the relevant epic, while the academic M1/M2/M3/M4 milestone language stays in scope docs and presentation planning.

Common commands:
```bash
GH_TOKEN="$(gh auth token)" python .agents/skills/codegym-project/scripts/github_project_board.py projects
GH_TOKEN="$(gh auth token)" python .agents/skills/codegym-project/scripts/github_project_board.py columns --project-number 2
GH_TOKEN="$(gh auth token)" python .agents/skills/codegym-project/scripts/github_project_board.py fields --project-number 2
GH_TOKEN="$(gh auth token)" python .agents/skills/codegym-project/scripts/github_project_board.py list --project-number 2
GH_TOKEN="$(gh auth token)" python .agents/skills/codegym-project/scripts/github_project_board.py list --project-number 2 --status Ready
GH_TOKEN="$(gh auth token)" python .agents/skills/codegym-project/scripts/github_project_board.py show --project-number 2 5
GH_TOKEN="$(gh auth token)" python .agents/skills/codegym-project/scripts/github_project_board.py add-issue --project-number 2 5 --status Ready --category "Project / Process" --priority P1 --size M --source "manual triage"
GH_TOKEN="$(gh auth token)" python .agents/skills/codegym-project/scripts/github_project_board.py add-draft --project-number 2 "Task title" --status Backlog --category "AI / Generation" --priority P2 --size S --body "Task details"
GH_TOKEN="$(gh auth token)" python .agents/skills/codegym-project/scripts/github_project_board.py create-issue --project-number 2 --title "Task title" --body-file /tmp/body.md --label agency:ready,agent:standard --status Ready --category "Frontend / UX" --priority P1 --size M --source "conversation import"
GH_TOKEN="$(gh auth token)" python .agents/skills/codegym-project/scripts/github_project_board.py create-issue --project-number 2 --title "Story under an epic" --body-file /tmp/body.md --label agency:ready,agent:standard --status Backlog --category "AI / Generation" --priority P1 --size M --source "epic breakdown" --parent 35
GH_TOKEN="$(gh auth token)" python .agents/skills/codegym-project/scripts/github_project_board.py add-sub-issue --project-number 2 35 12
GH_TOKEN="$(gh auth token)" python .agents/skills/codegym-project/scripts/github_project_board.py add-sub-issue --project-number 2 35 12 --remove
GH_TOKEN="$(gh auth token)" python .agents/skills/codegym-project/scripts/github_project_board.py upsert-issue --project-number 2 --title "Task title" --body-file /tmp/body.md --status Ready --category "Frontend / UX" --priority P1 --size M --source "conversation import" --verify
GH_TOKEN="$(gh auth token)" python .agents/skills/codegym-project/scripts/github_project_board.py import-issues --project-number 2 --json '[{"title":"Task title","body":"Details","labels":["agency:ready","agent:standard"],"status":"Ready","category":"Frontend / UX","priority":"P1","size":"M"}]' --source "conversation import" --verify
GH_TOKEN="$(gh auth token)" python .agents/skills/codegym-project/scripts/github_project_board.py import-issues --project-number 2 --source "conversation import" --verify <<'EOF'
{"issues":[{"title":"Task title","body":"Details","labels":["agency:ready","agent:standard"],"status":"Ready","category":"Frontend / UX","priority":"P1","size":"M"}]}
EOF
GH_TOKEN="$(gh auth token)" python .agents/skills/codegym-project/scripts/github_project_board.py import-issues --project-number 2 --file /tmp/issues.json --source "conversation import" --verify
GH_TOKEN="$(gh auth token)" python .agents/skills/codegym-project/scripts/github_project_board.py complete --project-number 2 5 --source "validated in PR 12" --verify
GH_TOKEN="$(gh auth token)" python .agents/skills/codegym-project/scripts/github_project_board.py edit-issue --project-number 2 10 --body-file /tmp/body.md --add-label agency:investigate,agent:strong --status "In progress" --category "Backend / API" --priority P0
GH_TOKEN="$(gh auth token)" python .agents/skills/codegym-project/scripts/github_project_board.py move --project-number 2 5 "In progress"
GH_TOKEN="$(gh auth token)" python .agents/skills/codegym-project/scripts/github_project_board.py set-field --project-number 2 5 Category "Project / Process"
GH_TOKEN="$(gh auth token)" python .agents/skills/codegym-project/scripts/github_project_board.py set-fields --project-number 2 5 --status Done --category "Project / Process" --priority P1 --size M --source "validated"
GH_TOKEN="$(gh auth token)" python .agents/skills/codegym-project/scripts/github_project_board.py rename --project-number 2 5 "Better issue title"
GH_TOKEN="$(gh auth token)" python .agents/skills/codegym-project/scripts/github_project_board.py delete --project-number 2 5
```

The helper prints issue/project views as Markdown so agents can paste or summarize board state directly in responses. It resolves items by project item ID, issue number, URL, or exact title. Prefer `upsert-issue` or `import-issues` for conversation imports so reruns update exact-title matches instead of creating duplicates. Use `--source` on imports and `--verify` after bulk field writes when the next step depends on project metadata; verification tolerates GitHub's short delay before newly-created project items appear in readback queries. `import-issues` accepts a JSON list or `{ "issues": [...] }` from `--json '<inline>'`, stdin (a heredoc, or `--file -`), or `--file <path>`; prefer inline/stdin so you do not have to create temp JSON files. Entries support `title`, `body`, `bodyFile`, `labels`, `status`, `category`, `priority`, `size`, `source`, `state`, and `close`.

### Add or Update a Story (Storybook)

Storybook 10 runs at http://localhost:6006. Stories live next to their component files as `ComponentName.stories.tsx`.

Key conventions:
- The global decorator in `.storybook/preview.tsx` wraps every story in a `MemoryRouter` + `Routes` + `Route`. Stories that need `useParams` must set `parameters.initialPath` and `parameters.routePath` so the router populates params correctly.
- Mock `globalThis.fetch` inside story-level decorators (not `beforeEach` — vitest is not installed in this project).
- Tailwind styles work automatically — the `@tailwindcss/vite` plugin is inherited from `vite.config.ts` by the `react-vite` Storybook framework. No extra config needed.
- Use decorators for per-story setup; do not rely on vitest lifecycle hooks in stories.

Example story with fetch mock and route params:
```tsx
export const MyStory: Story = {
  parameters: {
    initialPath: '/problems/two-sum',
    routePath: '/problems/:id',
  },
  decorators: [
    (Story) => {
      globalThis.fetch = myMockFetch as typeof fetch;
      return <Story />;
    },
  ],
};
```

## Scope Specification

Project Name: Generative AI Problem Generator
Version: 1.0
Date: Date
Prepared By: Kevin Chen
Presented To: Andy Meneely
License: AGPL 3

This is the live product scope for the project.

### 1. System Concept

The multi-user system automates creation of LeetCode/HackerRank-style coding problems and their corresponding test cases using generative AI. Each authenticated user has a personal workspace for data isolation and memory; business SaaS organization tenancy is not in scope unless explicitly added later.

The primary goal is to help software engineers at any level prepare for job interviews by generating unique challenges where generated test cases are executed against user attempts inside a secure, isolated Docker container or microVM.

Other features include MCQs and long-form conversational interview practice.

A memory feature, similar to Claude or ChatGPT memory, personalizes generated questions. It captures user proficiency across technical skills such as web development, cloud, DevOps, embedded programming, and ML. The system memorizes how users attempt problems and where they need improvement.

System memory has two parts:
- A user-level summary.
- Problem-specific notes.

A daily cron job updates the summary when the user attempts problems. For each solved problem, notes can be generated or pruned.

### 2. Project Vision and Business Case

Vision statement: The primary use case is to streamline interview preparation for software engineers. The secondary use case is personal practice on a desired language, concept, library, or framework.

Business and academic justification: This helps new grads and students practice LeetCode-style problems beyond DSA, including skills not covered in HackerRank.

### 3. Scope Definition

In-scope features:
- AI problem generation: A prompt-based interface to generate coding problem descriptions.
- Test case generation: Automatic creation of valid input/output pairs or unit tests for the generated problem.
- Secure execution environment: Docker containers or microVMs to run user-submitted or AI-generated solutions against test cases.
- Backend services: A robust Go backend to handle concurrent generation and execution tasks.
- Personalization and memory: User proficiency tracking and personalized question generation, including a user-level summary and problem-specific notes.
- Alternative problem formats: MCQs and long-form conversational interview practice.
- Problem verification: System-level running of a hidden reference solution against generated test cases to ensure problem validity.

Out-of-scope non-goals:
- Integration with external learning management systems like Canvas in Phase 1.
- Support for non-coding academic subjects.
- Production deployment for thousands of concurrent users.
- A full-featured in-browser IDE with integrated debugging and advanced code completion.
- Automated generation of reference solutions in Phase 1; manual creation is required for problem verification.
- Comprehensive progress reporting, user analytics dashboards, or monetization features.
- Support for more than three programming languages in the execution environment during Phase 1.

### 4. Typical Use Case and Flow of Events

1. Input prompt and context: The user enters a topic and difficulty. The system retrieves relevant personalization data from memory.
2. AI generation: The LLM generates a coding problem, MCQ, or long-form conversational prompt with corresponding test cases or solutions.
3. Verification: The system runs a hidden reference solution against generated test cases in a Docker container to ensure problem validity.
4. Output delivery: The user receives the verified problem set, reference solution, and test suite.
5. Memory update: The memory feature updates the user's proficiency summary and adds problem-specific notes based on engagement.

### 5. Technical Constraints and Architecture

Backend: Go for high-performance service orchestration. Goroutines are useful here, and a Go backend has a smaller memory footprint than Node or Python.

Environment: Docker or microVMs for secure, isolated code execution.

Frontend: React and Vite for a lightweight user interface.

Development standard: Follow a simple-to-complex implementation approach, delivering core functionality first.

### 6. Project Milestones

Progress is tracked using binary milestones: 100% done or 100% not done.

Milestones:
- M1 Project Kickoff: Detailed scope and architecture document.
- M2 MVP: Create frontend and backend, meet primary project goals.
- M3 Polish: Continue fixing defects.
- M4 Final Release: Polish and complete all user stories.

### 7. Risks and Mitigation

AI hallucinations: AI may generate invalid test cases. Mitigation: create a solution for LeetCode/HackerRank-style problems, run test cases against it, and rewrite failed test cases.

Resource constraints: Running many Docker containers may exceed local hardware limits. Mitigation: implement a queue system for execution tasks or explore microVMs such as Firecracker.

Security: Malicious code execution within containers. Mitigation: strict resource limits and no network access for Docker containers.

### 8. Stakeholders

Primary stakeholder: TBD

Project lead: Kevin Chen

Team member: Person

### 9. External Services and Deployment

Render will be used for the Go backend for now.

Claude through Vercel AI Gateway or Gemini will be used for LLMs using monthly credits.

Neon/Postgres is the current persistence target.

### 10. Licensing

AGPL 3 will be used to discourage freeloading from an open-source project.

## Generation Architecture

Problem generation should use an agent-driven flow:
- A Codex agent interviews the user via a multi-choice question modal (max 3 questions per series).
- The Go memory service maintains a persistent user profile memory (skill level, history, preferences).
- The generation/chat service reads the memory profile, builds provider context, and calls the GenAI client/adapter.
- Frontend UX stays mock-friendly, but backend memory APIs are now available for integration.
