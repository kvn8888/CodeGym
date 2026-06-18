---
name: codegym-project
description: Repository-specific workflow and product scope guidance for CodeGym v2, a frontend-first Generative AI Problem Generator branch. Use when implementing frontend pages, user flows, design system work, problem-generation UX, memory UX, MCQ/conversation practice UX, or updating project scope in this repository.
---

# CodeGym Project

Use this skill to execute CodeGym tasks without re-discovering project conventions.

## Branch Default

`codegym-v2` is the default working branch for now.

This branch is intentionally frontend-focused. Backend services, Docker execution infrastructure, database code, problem-pack fixtures, deploy files, and legacy automation are excluded from this branch until explicitly reintroduced.

## Workflow Decision Tree

1. Identify the task surface area.
- Frontend route/page/API consumption: inspect `frontend/src/App.tsx`, `frontend/src/features/`, and `frontend/src/shared/api/`.
- Project scope or product intent: read the "Scope Specification" section in this skill first.

2. Select the shortest local validation path.
- Frontend-only change: run `cd frontend && npm run lint` and `cd frontend && npm run build` when type-level confidence is needed.

3. Keep implementation aligned with the frontend-first branch.
- Stub backend-dependent flows behind clear interfaces or mock data when needed.
- Do not re-add backend, Docker, database, deploy, or problem-pack files unless the user explicitly asks.

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

## Task Workflows

### Manage the GitHub Project Board

Use `scripts/github_project_board.py` when an agent needs to inspect or update
the CodeGym GitHub Projects v2 kanban board. This script exists because some
Codex GitHub connectors can create/read issues but do not expose Project column
mutation.

Requirements:
- Set `GH_TOKEN` or `GITHUB_TOKEN` with repository and Projects v2 permissions.
- Default owner/repo: `kvn8888/CodeGym`.
- Default project title lookup: `CodeGym`.
- If the owner has multiple matching projects, pass `--project-number`.

Common commands:
```bash
python .agents/skills/codegym-project/scripts/github_project_board.py projects
python .agents/skills/codegym-project/scripts/github_project_board.py columns --project-number <number>
python .agents/skills/codegym-project/scripts/github_project_board.py list --project-number <number>
python .agents/skills/codegym-project/scripts/github_project_board.py list --project-number <number> --status Ready
python .agents/skills/codegym-project/scripts/github_project_board.py show --project-number <number> 5
python .agents/skills/codegym-project/scripts/github_project_board.py add-issue --project-number <number> 5 --status Ready
python .agents/skills/codegym-project/scripts/github_project_board.py add-draft --project-number <number> "Task title" --status Ready --body "Task details"
python .agents/skills/codegym-project/scripts/github_project_board.py move --project-number <number> 5 "In Progress"
python .agents/skills/codegym-project/scripts/github_project_board.py set-field --project-number <number> 5 Priority High
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
- Mock `globalThis.fetch` inside story-level decorators (not `beforeEach` — `@storybook/test`'s lifecycle hooks depend on vitest, which is not installed).
- Tailwind styles work automatically — the `@tailwindcss/vite` plugin is inherited from `vite.config.ts` by the `react-vite` Storybook framework. No extra config needed.
- `@storybook/test` is installed but do NOT use `beforeEach` from it — use decorators instead.

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
