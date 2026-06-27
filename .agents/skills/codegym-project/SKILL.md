---
name: codegym-project
description: Living project knowledge for CodeGym v2, a React/Vite frontend plus Go backend for generative coding practice. Consult this skill first when starting CodeGym work. Contains branch defaults, architecture decisions, validation paths, GitHub Projects workflow, current project scope, and conventions. Update this skill whenever architecture, requirements, project-board workflow, or durable implementation details change so agents inherit accurate context across sessions.
---

# CodeGym Project

This is the single source of truth for recurring CodeGym agent work. Read this
before changing code, issues, or the project board. Update it when product scope,
architecture, workflow, validation, or durable gotchas change.

## Branch Default

`codegym-v2` is the default working branch for now.

This branch now contains the React/Vite frontend and the Go backend foundation.
Backend auth, personal workspace bootstrap, Neon/Postgres memory storage, and
the memory service v0 are in scope. Legacy execution infrastructure, Docker
runners, problem-pack fixtures, organization/team tenancy, and deploy automation
should still stay out unless the user explicitly asks to reintroduce them.

## Current State (Last Updated: 2026-06-26)

- Branch: `codegym-v2`.
- Frontend: React 19 + Vite 7, Storybook 10, Motion/Framer-style animations, Monaco editor, mock-friendly app routes.
- Backend: Go service with `auth -> identity -> personal workspace scope -> handler` request path, Auth0 RS256/JWKS bearer-token validation behind `auth.Authenticator`, dev-token auth fallback, personal workspace bootstrap, memory profile/event APIs, Neon/Postgres store support, deterministic memory summarization, manual profile refresh, and worker-ready profile refresh.
- Secrets: Doppler is the preferred local secret runner; `NEON_CONNECTION_STRING` is checked before `DATABASE_URL`.
- CI: `.github/workflows/ci.yml` runs frontend `npm ci`, lint, build, and backend `go test ./...` on PRs/pushes to `codegym-v2`. `.github/workflows/openapi-lint.yml` runs `npm run api:lint` (Redocly) when `api/**` changes.
- Project board: GitHub Projects v2 project `#2` (`CodeGym v2`) is the active kanban unless the user says otherwise.

## Workflow Decision Tree

1. Identify the task surface area.
- Frontend route/page/API consumption: inspect `frontend/src/App.tsx`, `frontend/src/features/`, and `frontend/src/shared/api/`.
- Backend API/auth/workspace/memory work: inspect `backend/README.md`, `docs/auth-identity-tenant.md`, and `backend/internal/`.
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
  and handlers read scoped services. The current implementation still uses
  `tenant` package/table/field names as internal scope names; do not expand that
  into organization/team SaaS tenancy unless product scope changes.
- Memory writes should append events quickly. Profile summarization should stay
  behind `Service.RefreshProfile` or a worker boundary so request paths do not
  block on derived-memory work.
- Auth0 is the selected production auth provider. Auth0 JWT validation now
  lives behind the existing `auth.Authenticator` boundary; preserve the
  `DevAuthenticator` path for local fallback and tests.
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

## Task Workflows

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
