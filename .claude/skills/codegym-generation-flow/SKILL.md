---
name: codegym-generation-flow
description: Architecture for the agent-driven learning flow. Covers the Claude agent question modal, Chat interface, MCQ Marathon format, recommendation engine, Gemini-powered user profile memory, and backend generation pipeline. Use when implementing or modifying any learning feature, user profile system, the multi-step question modal, chat, or marathon.
---

# Agent-Driven Learning Flow

## Overview

CodeGym uses **agent-driven, personalized learning** rather than static forms.
Three main learning formats are available, each powered by LLM agents:

1. **Problem Generation** — Claude interviews the user, then generates a
   LeetCode-style coding problem tailored to their skill level.
2. **Chat** — A conversational interface with Claude for learning goals,
   knowledge assessment, and memory management.
3. **MCQ Marathon** — Timed multiple-choice question sets that reinforce known
   concepts and expand into new territory.

A **persistent user memory file** is read/written by all agents, ensuring
continuity across formats. A **recommendation engine** suggests the best format
based on the user's prompt and profile.

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         FRONTEND                                 │
│                                                                 │
│  ┌──────────┐  ┌────────────┐  ┌─────────────┐  ┌───────────┐ │
│  │ Generate  │  │    Chat    │  │  Marathon    │  │ Dashboard │ │
│  │  Page     │  │   Page     │  │   Page      │  │   Page    │ │
│  └─────┬─────┘  └─────┬──────┘  └──────┬──────┘  └───────────┘ │
│        │              │               │                         │
│        ▼              ▼               ▼                         │
│  QuestionModal   Message List   TimedQuestion                   │
│  + Recommend     + Input Bar    + HelpFlashcard                 │
│                                 + Results                       │
│        │              │               │                         │
│        └──────────────┼───────────────┘                         │
│                       ▼                                         │
│              POST /api/v1/...                                   │
└───────────────────────┬─────────────────────────────────────────┘
                        │
┌───────────────────────▼─────────────────────────────────────────┐
│                        BACKEND                                   │
│                                                                 │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │  User Memory (profile_memory TEXT in users table)        │   │
│  │  - Goals, familiarity, progress, strengths, weaknesses   │   │
│  │  - Read by ALL agents; written by chat/marathon/generate │   │
│  │  - Updated by Gemini Flash on activity events            │   │
│  │  - Dormancy check: skip if inactive > 14 days            │   │
│  └──────────────────────────────────────────────────────────┘   │
│                                                                 │
│  Agents:                                                        │
│  ├─ Claude: question generation, problem creation, chat, recs  │
│  └─ Gemini Flash: profile memory maintenance (async)            │
└─────────────────────────────────────────────────────────────────┘
```

## 1. Problem Generation Flow

### Steps

1. User submits a prompt from the Generate page.
2. Backend calls Claude to produce a **recommendation JSON** and questions.
3. Frontend shows the QuestionModal (max 3 questions per series).
4. After answers, Claude generates the problem (YAML + skeleton + tests).
5. Claude's recommendation may suggest Marathon instead (see §4).

### Recommendation JSON

After analyzing the user's prompt + memory, Claude returns:

```json
{
  "recommendation": "problem",       // "problem" | "marathon"
  "confidence": 0.85,
  "reason": "User asked for a specific DSA topic — a focused problem is ideal.",
  "questions": [ ... ],              // QuestionModal questions
  "marathon_config": null            // populated if recommendation is "marathon"
}
```

The frontend shows the recommendation as the default selection but lets the
user switch formats.

## 2. Chat Interface

### Purpose

The Chat page provides a conversational interface with Claude whose primary
mission is to:

- **Learn the user's goals** (career, interview prep, learning for fun, etc.)
- **Assess existing knowledge** (languages, frameworks, DSA comfort)
- **Generate/adjust the user's memory file** based on the conversation

### Frontend Component: ChatPage

```
ChatPage
├── MessageList (scrollable, auto-scroll to bottom)
│   ├── UserMessage (right-aligned, bg-parchment)
│   └── AssistantMessage (left-aligned, bg-white, border)
├── InputBar (bottom-fixed, similar to Generate prompt bar)
│   ├── TextInput (multi-line)
│   └── SendButton (arrow icon)
└── MemoryIndicator (shows when memory was last updated)
```

### API

- `POST /api/v1/chat` — Send a message, receive Claude's response + updated
  memory (if applicable).
- `GET /api/v1/chat/history` — Retrieve past messages for the session.

### Memory Integration

Every Chat response may include a `memory_update` field:

```json
{
  "message": "Great! I see you're comfortable with Python but want to...",
  "memory_update": {
    "goals": ["interview prep for FAANG"],
    "familiar_with": ["python", "basic-dsa"],
    "learning_targets": ["system-design", "go"]
  }
}
```

## 3. MCQ Marathon

### Purpose

A timed multiple-choice quiz that reinforces concepts the user knows and
gradually expands into adjacent topics. The LLM generates questions based on:

- The user's **memory file** (strengths, weaknesses, history)
- **Previous answers** in the current marathon (adaptive difficulty)
- **Time spent** on each question (hesitation signals uncertainty)

### Frontend Component: MarathonPage

```
MarathonPage
├── MarathonHeader
│   ├── QuestionCounter ("3 of 10")
│   ├── Timer (seconds elapsed on current question)
│   └── ScoreBar (correct/incorrect tally)
├── QuestionCard
│   ├── QuestionText
│   ├── OptionButtons (4 choices, single-select)
│   └── HelpButton (opens HelpFlashcard modal)
├── HelpFlashcard (modal overlay)
│   ├── ConceptTitle
│   ├── Explanation (does NOT answer the question)
│   └── CloseButton
└── ResultsView (shown after all questions)
    ├── Score summary
    ├── Time breakdown per question
    ├── Weak areas identified
    └── Recommended next steps
```

### Question Generation

Each marathon set is 5–10 questions. The backend generates them in a batch:

```json
{
  "questions": [
    {
      "id": "mq1",
      "text": "What is the time complexity of binary search?",
      "options": ["O(n)", "O(log n)", "O(n log n)", "O(1)"],
      "correct_index": 1,
      "concept": "binary-search-complexity",
      "help_content": "Binary search halves the search space each step..."
    }
  ],
  "topic_distribution": {
    "reinforcement": 0.6,
    "expansion": 0.4
  }
}
```

### Timer + Scoring

- Timer starts when the question is displayed.
- Answering stops the timer and records `time_ms`.
- After all questions, the results are sent to the backend:

```json
{
  "marathon_id": "uuid",
  "results": [
    {
      "question_id": "mq1",
      "selected_index": 1,
      "correct": true,
      "time_ms": 4200,
      "used_help": false
    }
  ]
}
```

### Memory Integration

After a marathon, Gemini Flash updates the user's memory with:

- Questions answered correctly/incorrectly
- Average response time per topic
- Whether help was used (signals the concept needs more reinforcement)
- New topics to introduce in the next marathon

### API Endpoints

- `POST /api/v1/marathon/generate` — Generate a question set.
- `POST /api/v1/marathon/submit` — Submit completed marathon results.

## 4. Recommendation Engine

After the QuestionModal in the Generate flow, Claude produces a
**recommendation JSON** that suggests the best learning format:

| Signal | Suggests |
|--------|----------|
| Specific topic + "practice" | Problem (default) |
| Broad topic + "review" | Marathon |
| "I don't know where to start" | Chat |
| Memory shows topic weakness | Marathon for that topic |
| Memory shows readiness | Problem with higher difficulty |

The frontend shows the recommendation with a toggle to switch formats.

## 5. User Memory File

### Purpose

A persistent profile that ALL agents in the project read and write. Ensures
continuity across problem generation, chat, and marathon sessions.

### What it stores

- **Goals**: career targets, interview prep, learning for fun, etc.
- **Familiarity**: languages, frameworks, DSA comfort level
- **Progress**: topics covered, completion rates, improvement trajectory
- **Strengths**: concepts the user consistently gets right quickly
- **Weaknesses**: concepts with low accuracy or high help usage

### Storage

Stored as a TEXT blob in the `users` table (or a dedicated `user_profiles`
table). Updated by Gemini Flash on activity events.

```sql
ALTER TABLE users ADD COLUMN profile_memory TEXT DEFAULT '';
ALTER TABLE users ADD COLUMN profile_updated_at TIMESTAMP;
```

### When it updates

Gemini Flash fires on user activity:
- Problem generated / submission graded
- Chat message exchanged
- Marathon completed (results + time + help usage)

**Dormancy check**: Skip Gemini updates if user inactive > 14 days.

## API Endpoints (Planned)

### Problem Generation

- `POST /api/v1/generate/questions` — Claude generates clarifying questions.
- `POST /api/v1/generate` — Claude generates problem from prompt + answers.
- `GET /api/v1/generate/status/:id` — Poll generation progress.

### Chat

- `POST /api/v1/chat` — Send message, receive response + optional memory update.
- `GET /api/v1/chat/history` — Retrieve past messages.

### Marathon

- `POST /api/v1/marathon/generate` — Generate a timed question set.
- `POST /api/v1/marathon/submit` — Submit completed marathon results.

## Implementation Order

1. ~~QuestionModal component~~ ✅ (committed 813086d)
2. **Chat page + sidebar nav** — conversational Claude interface
3. **Marathon page + HelpFlashcard** — timed MCQ with concept help
4. **POST /api/v1/generate/questions** — backend, Claude generates questions
5. **POST /api/v1/generate** — backend, Claude generates problem
6. **POST /api/v1/chat** — backend chat endpoint
7. **POST /api/v1/marathon/generate** — backend marathon endpoint
8. **User profile memory** — Gemini Flash integration, profile_memory column
9. **Recommendation engine** — Claude returns recommendation JSON after questions
10. **Dormancy check** — skip Gemini updates when user is inactive
