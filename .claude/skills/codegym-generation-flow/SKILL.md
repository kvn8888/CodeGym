---
name: codegym-generation-flow
description: Architecture for the agent-driven problem generation flow. Covers the Claude agent question modal, Gemini-powered user profile memory, and backend generation pipeline. Use when implementing or modifying the generation feature, user profile system, or the multi-step question modal.
---

# Agent-Driven Problem Generation Flow

## Overview

Problem generation in CodeGym is **agent-driven, not form-driven**. Instead of a
static difficulty selector, an LLM agent (Claude) interviews the user with
context-aware questions before generating a problem. A secondary agent (Gemini
Flash) maintains a persistent user profile/memory that the generation agent reads
to personalize difficulty and topic selection.

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    FRONTEND                              │
│                                                         │
│  GeneratePage                                           │
│  ┌─────────────────────────────────────────────┐        │
│  │  Prompt bar: "Describe what to practice..." │        │
│  │  Topic chips: DSA, API Patterns, etc.       │        │
│  │  [Send] button                              │        │
│  └──────────────────┬──────────────────────────┘        │
│                     │                                    │
│                     ▼                                    │
│  ┌─────────────────────────────────────────────┐        │
│  │  QuestionModal (max 3 questions per series) │        │
│  │  ┌─────────────────────────────────────┐    │        │
│  │  │  Multiple-choice options            │    │        │
│  │  │  Last option: "Specify..." (free)   │    │        │
│  │  │  [Next] button                      │    │        │
│  │  └─────────────────────────────────────┘    │        │
│  │  Claude may ask another series if needed    │        │
│  └──────────────────┬──────────────────────────┘        │
│                     │                                    │
│                     ▼                                    │
│  POST /api/v1/generate                                  │
│  { prompt, answers[], userProfileId }                   │
└─────────────────────┬───────────────────────────────────┘
                      │
┌─────────────────────▼───────────────────────────────────┐
│                    BACKEND                               │
│                                                         │
│  1. Read user profile/memory from DB                    │
│  2. Call Claude with:                                   │
│     - User's prompt                                     │
│     - User's question answers                           │
│     - User's profile memory (progress, history, skills) │
│  3. Claude generates problem YAML + skeleton + tests    │
│  4. Store generated problem in filesystem               │
│  5. Return problem ID to frontend                       │
│                                                         │
│  Gemini Flash (async, triggered on activity):           │
│  - Reads recent: problems generated, submissions,      │
│    question answers, completion rates                   │
│  - Updates user profile memory/summary                  │
│  - Dormancy check: skip if user inactive > N days       │
└─────────────────────────────────────────────────────────┘
```

## Question Modal Component

### Behavior

1. User submits a prompt from the Generate page.
2. Frontend calls `POST /api/v1/generate/questions` with the prompt and user ID.
3. Backend responds with a series of **max 3 questions**.
4. A modal slides up with the first question.
5. Each question shows:
   - A question text (e.g., "What language do you prefer?")
   - Multiple-choice options (e.g., "Python", "Go", "JavaScript")
   - A final **"Specify..."** option → reveals a text input for free-form response
   - A **[Next]** button to advance to the next question
6. After all questions answered, frontend calls `POST /api/v1/generate` with the
   prompt + answers.
7. Claude may request **another series** of questions if it needs more context.
   The modal re-opens for the follow-up series.

### Component Structure

```
QuestionModal
├── QuestionCard (one per question)
│   ├── Question text
│   ├── OptionList (multiple-choice buttons)
│   │   ├── OptionButton (selectable, radio-style)
│   │   └── SpecifyOption (last item, expands to text input)
│   └── NextButton
├── ProgressDots (shows 1 of 3)
└── SkipButton (optional: "Generate without answering")
```

### State Flow

```typescript
interface QuestionSeries {
  questions: Question[];
  seriesIndex: number;      // which round of questions (0, 1, ...)
}

interface Question {
  id: string;
  text: string;
  options: string[];        // last option is always "Specify..."
}

interface Answer {
  questionId: string;
  selectedOption: string;   // the chosen option text
  freeText?: string;        // populated if "Specify..." was chosen
}
```

## User Profile Memory (Gemini Flash)

### What it stores

A Gemini Flash model maintains a **living summary** of each user's profile:

- **Skill assessment**: inferred skill level per language/topic based on
  submission history and completion rates
- **Problem history**: types of problems generated, topics covered, difficulty
  distribution
- **Question answers**: aggregated preferences from past generation question
  modals (preferred languages, domains, learning goals)
- **Progress trajectory**: improving, plateauing, or struggling in specific areas

### When it updates

Gemini Flash updates the profile memory on **user activity events**:

- Problem generated
- Submission graded (pass/fail)
- Question answers submitted

**Dormancy check**: If the user has been inactive for a configurable threshold
(e.g., 14 days), Gemini Flash does NOT run. The stale profile is still readable
by the generation agent — it just won't be refreshed until the user returns.

### Storage

The profile memory is stored as a text blob in the `users` table (or a dedicated
`user_profiles` table). The generation agent reads it as context when creating
problems.

```sql
ALTER TABLE users ADD COLUMN profile_memory TEXT DEFAULT '';
ALTER TABLE users ADD COLUMN profile_updated_at TIMESTAMP;
```

## API Endpoints (Planned)

### POST /api/v1/generate/questions

Request:
```json
{
  "prompt": "Two pointer problems in Go",
  "user_id": "uuid"
}
```

Response:
```json
{
  "data": {
    "series_id": "uuid",
    "questions": [
      {
        "id": "q1",
        "text": "What's your experience with Go?",
        "options": ["Beginner", "Intermediate", "Advanced", "Specify..."]
      },
      {
        "id": "q2",
        "text": "What aspect of two pointers do you want to focus on?",
        "options": ["Sliding window", "Fast/slow pointers", "Meeting in middle", "Specify..."]
      }
    ]
  }
}
```

### POST /api/v1/generate

Request:
```json
{
  "prompt": "Two pointer problems in Go",
  "answers": [
    { "question_id": "q1", "selected_option": "Intermediate" },
    { "question_id": "q2", "selected_option": "Sliding window" }
  ],
  "series_id": "uuid",
  "user_id": "uuid"
}
```

Response:
```json
{
  "data": {
    "problem_id": "go-two-pointer-sliding-window-abc123",
    "status": "generating"
  }
}
```

### POST /api/v1/generate/status/:id

Poll for generation completion.

## Implementation Order

1. **QuestionModal component** — frontend modal with multi-choice + specify + next
2. **POST /api/v1/generate/questions** — backend endpoint, Claude generates questions
3. **POST /api/v1/generate** — backend endpoint, Claude generates problem from prompt + answers + profile
4. **User profile memory** — Gemini Flash integration, profile_memory column, activity triggers
5. **Dormancy check** — skip Gemini updates when user is inactive
