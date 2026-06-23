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
Backend auth, tenant bootstrap, Neon/Postgres memory storage, and memory service
plumbing are in scope. Legacy execution infrastructure, Docker runners,
problem-pack fixtures, and deploy automation should still stay out unless the
user explicitly asks to reintroduce them.

## Current State (Last Updated: 2026-06-22)

- Branch: `codegym-v2`.
- Frontend: React 19 + Vite 8, Storybook 10, Motion/Framer-style animations, Monaco editor, mock-friendly app routes.
- Backend: Go service with `auth -> identity -> tenant -> handler` request path, dev-token auth, tenant bootstrap, memory profile/event APIs, and Postgres store support via Neon.
- Secrets: Doppler is the preferred local secret runner; `NEON_CONNECTION_STRING` is checked before `DATABASE_URL`.
- CI: `.github/workflows/ci.yml` runs frontend `npm ci`, lint, build, and backend `go test ./...` on PRs/pushes to `codegym-v2`.
- Project board: GitHub Projects v2 project `#2` (`CodeGym v2`) is the active kanban unless the user says otherwise.

## Workflow Decision Tree

1. Identify the task surface area.
- Frontend route/page/API consumption: inspect `frontend/src/App.tsx`, `frontend/src/features/`, and `frontend/src/shared/api/`.
- Backend API/auth/tenant/memory work: inspect `backend/README.md`, `docs/auth-identity-tenant.md`, and `backend/internal/`.
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
- Preserve the request path: auth middleware establishes `auth.Principal`,
  identity middleware bootstraps/persists the user and personal tenant, tenant
  middleware establishes `tenant.Scope`, and handlers read scoped services.
- Memory writes should append events quickly. Profile summarization should stay
  behind `Service.RefreshProfile` or a worker boundary so request paths do not
  block on derived-memory work.
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

Use `scripts/github_project_board.py` when an agent needs to inspect or update
the CodeGym GitHub Projects v2 kanban board. This script exists because some
Codex GitHub connectors can create/read issues but do not expose every Projects
v2 field mutation. Treat the board as live planning state.

**Script maintenance**: Treat the repo-local path above as the stable command
entrypoint, but do not assume it is the canonical source file forever. This
helper may later become a symlink to a shared GitHub Projects utility used by
CodeGym and OpenFoodJournal. Before changing the script, resolve the real file
with `realpath .agents/skills/codegym-project/scripts/github_project_board.py`
or `readlink`, then modify the resolved canonical file and update this skill if
the shared location changes. Do not patch a stale copied script while another
project points at the shared target. If this is centralized, prefer a neutral
shared-tools location near the sibling repos instead of making one app repo own
the other app repo's utility.

Requirements:
- Set `GH_TOKEN` or `GITHUB_TOKEN` with repository and Projects v2 permissions.
- Default owner/repo: `kvn8888/CodeGym`.
- Default project title lookup: `CodeGym`.
- Active project: pass `--project-number 2` or set `CODEGYM_GITHUB_PROJECT_NUMBER=2`.
- After each work session, update relevant GitHub issues and board fields so
  the board reflects reality. Move actively worked issues to `In Progress`,
  verified completed issues to `Done`, and add issue comments/details for
  blockers, validation, or deferred follow-up.

Common commands:
```bash
python .agents/skills/codegym-project/scripts/github_project_board.py projects
python .agents/skills/codegym-project/scripts/github_project_board.py columns --project-number <number>
python .agents/skills/codegym-project/scripts/github_project_board.py fields --project-number <number>
python .agents/skills/codegym-project/scripts/github_project_board.py list --project-number <number>
python .agents/skills/codegym-project/scripts/github_project_board.py list --project-number <number> --status Ready
python .agents/skills/codegym-project/scripts/github_project_board.py show --project-number <number> 5
python .agents/skills/codegym-project/scripts/github_project_board.py add-issue --project-number <number> 5 --status Ready --priority P1 --size M
python .agents/skills/codegym-project/scripts/github_project_board.py add-draft --project-number <number> "Task title" --status Ready --priority P2 --size S --body "Task details"
python .agents/skills/codegym-project/scripts/github_project_board.py create-issue --project-number <number> --title "Task title" --body-file /tmp/body.md --label area:ci,type:task --status Ready --priority P1 --size M
python .agents/skills/codegym-project/scripts/github_project_board.py edit-issue --project-number <number> 10 --body-file /tmp/body.md --add-label area:backend --status "In Progress" --priority P0
python .agents/skills/codegym-project/scripts/github_project_board.py move --project-number <number> 5 "In Progress"
python .agents/skills/codegym-project/scripts/github_project_board.py set-field --project-number <number> 5 Priority P1
python .agents/skills/codegym-project/scripts/github_project_board.py set-fields --project-number <number> 5 --status Done --priority P1 --size M
python .agents/skills/codegym-project/scripts/github_project_board.py rename --project-number <number> 5 "Better issue title"
python .agents/skills/codegym-project/scripts/github_project_board.py delete --project-number <number> 5
```

The script prints issue/project views as Markdown so agents can paste or
summarize board state directly in responses. It can resolve items by project
item ID, issue number, URL, or exact title.

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

The single- or multi-tenant system automates creation of LeetCode/HackerRank-style coding problems and their corresponding test cases using generative AI.

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

Render will be used for production using hackathon credits.

Claude through Vercel AI Gateway or Gemini will be used for LLMs using monthly credits.

TursoDB will be used for persistence.

### 10. Licensing

AGPL 3 will be used to discourage freeloading from an open-source project.

## Generation Architecture

Problem generation should use an agent-driven flow:
- A Codex agent interviews the user via a multi-choice question modal (max 3 questions per series).
- A Gemini Flash model maintains a persistent user profile memory (skill level, history, preferences).
- The generation agent reads the user profile when creating personalized problems.
- In this branch, frontend UX and interaction contracts should be built first; backend implementation can be reintroduced later.
