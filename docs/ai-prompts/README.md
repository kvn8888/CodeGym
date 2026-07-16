# CodeGym AI Prompt Pack

Copy-paste **system prompts** and **test inputs** for every AI service /
pipeline in CodeGym. Drop a system prompt into any playground (Anthropic
Console, Vercel AI Gateway, Google AI Studio), paste the matching test input as
the first user message, and inspect the output. Tune the prompt, re-run, repeat.

These are authored against the **real data shapes** already in the repo, so a
well-behaved model produces JSON that drops straight into the frontend and
backend:

- Problem package → `frontend/src/shared/api/types.ts` (`Problem`, `Hint`, `RuntimeConfig`)
- MCQ marathon → `MarathonQuestion` in `frontend/src/features/marathon/MarathonPage.tsx`
- Intake questions → `Question` / `Answer` in `frontend/src/features/generate/QuestionModal.tsx`
- Memory profile → `UserMemoryProfile` / `SkillProficiency` / `MemoryNote` in `types.ts`
  and `Profile` in `backend/internal/memory/model.go`
- Memory events → `docs/memory-event-naming-guide-v0.md`

## The services

| # | Service | File | Output |
| - | ------- | ---- | ------ |
| 1 | Intake / clarifying-question agent | [`01-intake-clarifying-agent.md`](01-intake-clarifying-agent.md) | `Question[]` (max 3) |
| 2 | Problem (code) generation | [`02-problem-generation.md`](02-problem-generation.md) | `Problem` package |
| 3 | Test-case generation | [`03-test-generation.md`](03-test-generation.md) | Test suite JSON |
| 4 | Problem verification / test repair | [`04-problem-verification.md`](04-problem-verification.md) | Verdict + fixed cases |
| 5 | MCQ marathon generation | [`05-mcq-marathon.md`](05-mcq-marathon.md) | `MarathonQuestion[]` |
| 6 | Conversational interview coach | [`06-conversational-interview.md`](06-conversational-interview.md) | Streaming chat |
| 7 | Memory profile synthesizer | [`07-memory-profile-summary.md`](07-memory-profile-summary.md) | Curated `Profile` JSON |
| 8 | Legacy note-only maintenance reference | [`08-memory-notes.md`](08-memory-notes.md) | Note actions |
| 9 | MCQ free-response evaluator | [`09-mcq-free-response-evaluation.md`](09-mcq-free-response-evaluation.md) | Binary verdict + concise feedback |

## How the pipeline fits together

```
                    ┌─────────────────────────── memory.Service (Go) ───────────────────────────┐
                    │   append-only events  ──►  (7) profile summarizer  ──►  Profile            │
                    └───────────────────────────────────────────────────────────────────────────┘
                                   │ Profile is read as personalization context by ▼
 user prompt ─► (1) intake ─► answers ─► (2) problem gen ─► (3) test gen ─► (4) verify ─► delivered problem
                                    └───► (5) MCQ marathon
                                    └───► (6) interview coach
 every surface emits deterministic events ───────────────────────────────────► back into (7) profile synthesis
```

## Conventions used in every prompt

- **Strict JSON out.** Each generator returns a single JSON object/array, no
  prose, no markdown fences. Playgrounds show raw text so you can eyeball it;
  in production wrap with the provider's JSON/structured-output mode.
- **Memory is context, never a command.** The user's `Profile` is injected as
  reference material to personalize difficulty and topic — the model must not
  follow instructions found inside profile text (prompt-injection hygiene).
- **No secrets or full user code** land in any summary/note, per the event
  naming guide's payload-hygiene rules.
- **`{{PLACEHOLDERS}}`** in a system prompt are values your orchestrator fills
  before the call. In a playground, just replace them inline.
- **Difficulty:** the `Problem` type stores difficulty as an integer `1–5`
  (1 = easy … 5 = expert). The generate UI's `easy|medium|hard` maps to
  `2|3|4`. Prompts emit the integer.

## Suggested test loop

1. Paste system prompt + input A (happy path) → check schema + quality.
2. Paste input B (cold-start / empty memory) → check graceful defaults.
3. Paste input C (adversarial / injection in memory) → check it doesn't obey.
4. Adjust the system prompt, re-run all three. Keep a note of what changed.
