---
name: codegym-design
description: CodeGym-specific design system reference. Use when building or modifying any frontend UI in this repository — pages, components, layouts, or Storybook stories. Documents the product's quiet operational-workspace direction, density, typography, surfaces, responsive shell, and page composition rules.
---

# CodeGym Design System

CodeGym is a **quiet technical workspace** for deliberate coding practice. Its
primary family is a quiet operational workspace: compact, calm, border-led, and
organized around a persistent sidebar plus a focused work canvas. Active
practice may borrow guided-workflow behavior; the code editor may use a dense
split workbench. These are task-driven structures, not alternate visual themes.

The product should feel useful before it feels decorative. Fill space with real
session, memory, and practice context rather than oversized headings, stat
cards, marketing copy, or invented charts.

## Core Principles

1. One application shell is in charge.
2. Use operational density by default; vary density only when the task changes.
3. Prefer rows, dividers, alignment, and surface contrast over card grids.
4. Color communicates status, progress, difficulty, correctness, or selection.
5. AI belongs inside the practice or memory workflow, with visible context.
6. Empty states explain what is missing and provide one useful next action.
7. Routine product pages are top-aligned; do not vertically center sparse content.

## Color System

Use the shadcn semantic tokens defined in `frontend/src/index.css`. They support
light and dark themes and are the source of truth.

| Token | Use |
| --- | --- |
| `background` / `foreground` | Main canvas and primary text |
| `sidebar` / `sidebar-foreground` | Persistent navigation |
| `card` / `card-foreground` | Bounded task surfaces and repeated objects |
| `muted` / `muted-foreground` | Secondary regions and metadata |
| `border` / `input` / `ring` | Separation, controls, and focus |
| `primary` / `primary-foreground` | Primary commands |
| `destructive` | Errors and destructive actions |

The fixed green, amber, and red scales are reserved for semantic states. Avoid
assigning different bright colors to ordinary topics or navigation categories.
Legacy `bone`, `parchment`, and `ink` aliases exist only for compatibility.

## Typography

Geist is the interface font. Geist Mono is reserved for code, identifiers,
durations, compact indices, percentages, and technical metadata.

| Role | Scale |
| --- | --- |
| Page title | `text-2xl leading-8 font-semibold` |
| Active-task title | `text-xl leading-7 font-semibold` |
| Section heading | `text-sm leading-5 font-semibold` |
| Routine body | `text-sm leading-5` |
| Metadata | `text-xs leading-4 text-muted-foreground` |
| Meaningful metric | `text-2xl` to `text-[32px]`, only when warranted |

Do not use negative letter spacing. Do not use 40–64px type for routine app
pages. Do not use monospace for ordinary prose or navigation.

## Spacing And Geometry

- App page width: `max-w-[1180px]`.
- Page padding: `px-4 py-6`, rising to `lg:px-8 lg:py-8`.
- Page header to content: 24px.
- Major section separation: 28px.
- Panel padding: 12–20px based on density.
- Operational row height: 44–56px.
- Routine control height: 32–40px.
- Control radius: 6–8px.
- Ordinary panel radius: 8–12px.
- Circular geometry is for avatars, radio indicators, status dots, and icon-only
  floating controls, not text containers.

`Card` is flat by default: border, modest radius, no shadow. Shadows are for
popovers, menus, dialogs, and truly elevated layers. Do not nest card-like
panels inside cards.

## Application Shell

Desktop:

- 216px expandable sidebar, 52px collapsed.
- 36px navigation rows with 8px horizontal gutters.
- Slightly differentiated sidebar surface and one bordered active row.
- Account and theme controls remain in the sidebar footer.

Mobile:

- The desktop sidebar is removed below `md`.
- A 56px top bar contains the brand, account, and navigation trigger.
- Navigation opens as a full-height solid sheet below the top bar.
- Do not compress the desktop sidebar beside mobile content.

Floating chat is contextual. It is available during Marathon and problem-detail
practice, not globally over Dashboard, Memory, Settings, or New Practice.

## Shared Page Structure

Use `frontend/src/shared/components/WorkspacePage.tsx` for operational pages:

- `WorkspacePage`: stable page width and responsive padding.
- `WorkspacePageHeader`: economical title, description, and actions.
- `WorkspaceSectionHeader`: compact section context and controls.
- `WorkspaceEmptyState`: bounded explanation plus an optional next action.

Use bordered row groups for sessions, skills, notes, recommendations, and
problems. Column headers may appear on desktop and collapse into two-line rows
on mobile.

## Page Responsibilities

### Dashboard

Answers two questions: what can I resume, and what should I practice next.
Prioritize active sessions, recent completed sessions, memory-derived practice
recommendations, and a compact learning snapshot. Do not add metric cards until
the backend provides trustworthy aggregate data.

### Memory

Explains what CodeGym believes about the learner and makes that model
actionable. Use a focus queue, compact skill rows, filterable memory-note rows,
strengths, and recent memory activity. A memory observation should link directly
to a relevant practice setup when possible.

### New Practice

Configures a real session. Supported formats are MCQ, DSA coding, and
Interview; keep future formats disabled until their corresponding flow exists.
Keep configuration on the main column and memory-derived personalization
context in a secondary column. Starting practice creates a durable session and
hands execution to the selected practice workspace.

### Marathon

Uses guided-workflow behavior: one question, stable progress, visible timer,
clear answer states, and stable next actions. Session progress is persisted for
resume. Keep the active question surface focused and narrower than operational
history pages.

### Problem Detail

The editor is the valid dense-workbench exception. Preserve simultaneous
problem and code context. The Monaco zone remains dark and visually sealed from
the light application shell.

## Motion

Motion communicates transitions, selection, and completion. Keep it short and
respect `prefers-reduced-motion`. Avoid independent hover movement on every
routine row. GridSpinner remains the page-level loading indicator.

## Storybook And Review

Every rebuilt page needs realistic populated, empty, loading, and error states
where the state applies. Use the mock API scenarios in `frontend/src/mocks`.
Review at desktop and 390px mobile widths, in light and dark themes. Verify:

- no horizontal overflow;
- no overlapping shell controls;
- no clipped labels or buttons;
- useful content is visible above the fold;
- empty states contain a next action when one exists;
- focus rings and keyboard navigation remain visible.

## Anti-Patterns

- Marketing-sized app headings.
- Default three-stat dashboards.
- Card-per-record lists.
- Default shadows on routine surfaces.
- Arbitrary topic colors.
- Nested cards and repeated large radii.
- Decorative charts or fabricated numbers.
- Generic AI slogans, sparkle icons, or a global assistant bubble.
- Two visual modes for the same workflow.
- Hiding supported context merely to make a screen look sparse.

## Key Files

| Purpose | Path |
| --- | --- |
| Tokens and global styles | `frontend/src/index.css` |
| Shell and responsive navigation | `frontend/src/shared/components/Layout.tsx` |
| Operational page primitives | `frontend/src/shared/components/WorkspacePage.tsx` |
| Dashboard | `frontend/src/features/dashboard/DashboardPage.tsx` |
| Memory | `frontend/src/features/memory/MemoryPage.tsx` |
| New Practice | `frontend/src/features/generate/GeneratePage.tsx` |
| Marathon | `frontend/src/features/marathon/MarathonPage.tsx` |
| Problem workbench | `frontend/src/features/problems/ProblemDetailPage.tsx` |
