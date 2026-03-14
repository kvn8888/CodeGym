---
name: codegym-design
description: CodeGym-specific design system reference. Use when building or modifying any frontend UI in this repository — pages, components, layouts, or Storybook stories. Documents the exact color tokens, typography, component patterns, and animation conventions used in the project so new UI is visually consistent.
---

# CodeGym Design System

CodeGym uses a **Technical Brutalism → Refined Cards** aesthetic. The design started as raw brutalist (hard borders, uppercase labels, no rounding) and evolved into a warmer, card-based system that keeps the monospaced personality while adding softness through rounded corners and layered shadows. Every UI surface should feel like clean paper with ink — no gradients, no color splashes, no decorative chrome.

---

## Color System

Colors are defined as Tailwind theme variables in `frontend/src/index.css` using `@theme`. Always use tokens — never raw hex values inline.

| Token | Hex | Primary use |
|-------|-----|-------------|
| `bone` | `#FAF9F6` | Page/app background |
| `parchment` | `#F0EFEC` | Sidebar, secondary surfaces |
| `ink` | `#1a1a1a` | Primary text, CTA buttons, active states |
| `ink-soft` | `#333333` | Button hover, slightly muted ink |
| `graphite` | `#6b6b6b` | Body copy, prose text |
| `ash` | `#999999` | Labels, metadata, placeholders, inactive icons |
| `chalk` | `#d4d0c8` | Borders, dividers, disabled backgrounds |
| `grain` | `#e8e6e1` | Hover fills, subtle section backgrounds |
| `moss` | `#2d5a27` | Success accent (reserved, rarely used) |
| `rust` | `#8b2500` | Error/danger accent (reserved, rarely used) |

**Text selection**: `background-color: var(--color-ink); color: var(--color-bone)` — inverted ink-on-bone.

---

## Typography

The entire application uses a **monospace font stack** — no sans-serif, no serif. This is load-bearing to the aesthetic.

```css
font-family: 'SF Mono', 'Cascadia Code', 'JetBrains Mono', 'Fira Code', ui-monospace, 'Courier New', monospace;
```

### Scale in use

| Role | Tailwind classes | Notes |
|------|-----------------|-------|
| Page heading | `text-2xl font-bold text-ink tracking-tight` | Main page H1 (Problems, Generate) |
| Section heading (brutalist) | `text-xs font-bold tracking-[0.2em] text-ink uppercase` | Dashboard H1, older pattern |
| Sub-label | `text-[10px] font-bold tracking-[0.15em] text-ash uppercase` | Hints header, dropdown header |
| Body text | `text-sm text-ink` or `text-sm font-medium text-ink` | Problem titles, list items |
| Secondary body | `text-xs text-graphite` | Prose, descriptions |
| Metadata | `text-[10px] tracking-[0.08em] text-ash` | Language/framework/time tags |
| CTA button | `text-[10px] font-bold tracking-[0.15em] uppercase` | Run button |
| Hint text | `text-xs text-ash` or `text-xs text-graphite` | — |

---

## Layout

### App Shell

```
┌──────────────────────────────────────────────────────────┐
│  Sidebar (w-56, sticky, bg-parchment, border-r border-grain) │
│  ┌── Logo: text-sm font-bold tracking-[0.18em] text-ink  │
│  │   CODEGYM                                              │
│  │                                                        │
│  ├── Nav (px-3, flex col, gap-0.5)                       │
│  │   Active:  bg-white text-ink font-medium rounded-xl   │
│  │           + boxShadow: 0 1px 3px rgba(0,0,0,0.06)    │
│  │   Inactive: text-graphite hover:bg-grain hover:text-ink │
│  │                                                        │
│  └── Footer: text-xs text-ash tracking-wide v0.1         │
├──────────────────────────────────────────────────────────┤
│  Main (flex-1 min-w-0 bg-bone)                           │
│  └── Page content                                        │
└──────────────────────────────────────────────────────────┘
```

### Page content widths

| Page | Max width |
|------|-----------|
| Problems list | `max-w-3xl mx-auto px-6 py-10` |
| Generate | `max-w-2xl mx-auto px-6 py-24` |
| Dashboard | `max-w-4xl mx-auto px-6 py-8` |
| Problem detail (split) | Full height, `h-screen flex` |

---

## Component Patterns

### Card

```tsx
<div
  className="rounded-2xl border border-chalk bg-white px-5 py-4"
  style={{ boxShadow: '0 1px 3px rgba(0,0,0,0.04), 0 4px 12px rgba(0,0,0,0.03)' }}
>
```

The **layered shadow formula** is the design's fingerprint: a sharp 1px near-shadow + a soft diffuse far-shadow. Never use a single-layer box shadow on cards.

### Input / Search bar

```tsx
<div
  className="flex items-center rounded-2xl border border-chalk bg-white px-4 py-3"
  style={{ boxShadow: '0 1px 3px rgba(0,0,0,0.04), 0 4px 12px rgba(0,0,0,0.03)' }}
>
  <input className="flex-1 bg-transparent text-sm text-ink placeholder-ash focus:outline-none" />
```

### Primary button (CTA)

```tsx
<button className="w-9 h-9 bg-ink text-bone rounded-xl flex items-center justify-center hover:bg-ink-soft disabled:bg-chalk disabled:text-ash disabled:cursor-not-allowed transition-colors">
```

### Secondary / action button

```tsx
<button
  className="px-4 py-1.5 rounded-lg bg-white text-ink text-[10px] font-bold tracking-[0.15em] uppercase hover:bg-parchment disabled:opacity-40 transition-colors"
  style={{ boxShadow: '0 1px 3px rgba(0,0,0,0.08)' }}
>
  RUN
</button>
```

### Dropdown / popover

```tsx
<div
  className="absolute left-0 right-0 top-full mt-2 border border-chalk rounded-2xl bg-white overflow-hidden z-10"
  style={{ boxShadow: '0 4px 16px rgba(0,0,0,0.08)' }}
>
  <div className="px-4 py-2 border-b border-chalk">
    <span className="text-[10px] font-bold tracking-[0.2em] text-ash uppercase">Label</span>
  </div>
  <button className="block w-full text-left text-xs text-graphite hover:text-ink hover:bg-parchment px-4 py-2.5 transition-colors">
    Item
  </button>
</div>
```

### Select

```tsx
<select
  className="rounded-xl border border-chalk bg-white px-3 py-1.5 text-xs text-ink cursor-pointer focus:outline-none"
  style={{ boxShadow: '0 1px 3px rgba(0,0,0,0.04), 0 4px 12px rgba(0,0,0,0.03)' }}
>
```

### Stat card (Dashboard)

```tsx
<div
  className="rounded-2xl border border-chalk bg-white px-6 py-5"
  style={{ boxShadow: '0 1px 3px rgba(0,0,0,0.04), 0 4px 12px rgba(0,0,0,0.03)' }}
>
  <div className="text-3xl font-bold text-ink">{value}</div>
  <div className="text-[10px] tracking-[0.15em] text-ash mt-2 uppercase">{label}</div>
</div>
```

### List item / problem row (link card)

```tsx
<Link
  className="flex items-center justify-between rounded-2xl border border-chalk bg-white px-5 py-4 no-underline hover:border-ash transition-colors"
  style={{ boxShadow: '0 1px 3px rgba(0,0,0,0.04), 0 4px 12px rgba(0,0,0,0.03)' }}
>
```

### Nav link (sidebar)

```tsx
// Active
<Link className="flex items-center gap-3 px-3 py-2.5 rounded-xl text-sm no-underline bg-white text-ink font-medium transition-colors"
  style={{ boxShadow: '0 1px 3px rgba(0,0,0,0.06)' }}>

// Inactive
<Link className="flex items-center gap-3 px-3 py-2.5 rounded-xl text-sm no-underline text-graphite hover:bg-grain hover:text-ink transition-colors">
```

Icons are 18×18 SVGs with `strokeWidth="1.75"`.

---

## Split-Pane Editor Layout (ProblemDetailPage)

```
┌──────────────────────────────────────────┐
│ Left (w-[45%]) border-r border-chalk     │
│ bg-white, overflow-y-auto, p-6           │
│ Problem description + hints              │
├──────────────────────────────────────────┤
│ Right (w-[55%]) bg-[#1e1e1e]            │
│ ┌── File tabs: bg-[#252526] border-[#333]│
│ │   Active tab: bg-[#1e1e1e] text-[#ccc]│
│ │   Inactive:   text-[#666]              │
│ │   Run button: bg-white text-ink        │
│ └── Monaco Editor (dark, #1e1e1e)       │
│ └── Results panel (bg-[#1e1e1e])        │
└──────────────────────────────────────────┘
```

The editor zone is always dark `#1e1e1e` regardless of the light app shell. Do not let bone/parchment creep into the editor or its tab bar.

---

## Animations & Motion

### Typewriter cursor

Used on GeneratePage headline. Defined in `index.css`:

```css
@keyframes blink {
  0%, 100% { opacity: 1; }
  50% { opacity: 0; }
}
```

Applied as:
```tsx
<span className="inline-block w-[2px] h-[1.1em] bg-ink ml-[2px] align-middle animate-[blink_1s_step-end_infinite]" aria-hidden="true" />
```

### GridSpinner

A 5×5 grid of cells that animate in a clockwise spiral sequence. Implemented in `frontend/src/shared/components/GridSpinner.tsx`. Sizes: `sm`, `md`, `lg`. Use as a loading state for page-level fetches.

```css
@keyframes grid-blink {
  0%, 100% { background-color: var(--color-grain); }
  3% { background-color: var(--color-ink); }
  25% { background-color: var(--color-grain); }
}
```

---

## Prose (Markdown rendering)

Markdown from the API (problem descriptions) is rendered with ReactMarkdown and styled via the `.prose-brutalist` class defined in `index.css`. Key traits:

- All fonts remain monospaced (no serif override)
- Code blocks: `background: #1e1e1e; color: #ccc`
- Inline code: `background: var(--color-grain)`
- Headings: H3 uppercase with `letter-spacing: 0.1em`
- Links: `color: ink`, underlined

---

## Scrollbar

```css
::-webkit-scrollbar { width: 6px; height: 6px; }
::-webkit-scrollbar-track { background: transparent; }
::-webkit-scrollbar-thumb { background: var(--color-chalk); }
::-webkit-scrollbar-thumb:hover { background: var(--color-ash); }
```

---

## Rules — What NOT to do

1. **No gradient backgrounds** — not even subtle ones on cards.
2. **No sans-serif fonts** — all text stays monospaced.
3. **No color accents on interactive states** — hover/active uses ink/parchment/grain, not moss/rust.
4. **No single-layer box shadows on cards** — always use the two-layer formula.
5. **No sharp corners on cards/inputs** — minimum `rounded-xl`; prefer `rounded-2xl`.
6. **No hard `border border-ink` on light-zone UI** — use `border-chalk` for light surfaces; `border-ink` was the older brutalist pattern and is no longer used for list rows or cards.
7. **No uppercase for main page titles** — page-level H1s use `text-2xl font-bold tracking-tight`, not small-caps shouts.
8. **No color in the Monaco editor zone** — the dark editor panel (#1e1e1e, #252526, #333) is its own sealed system.

---

## File locations

| Purpose | Path |
|---------|------|
| CSS tokens + global styles | `frontend/src/index.css` |
| App shell + sidebar | `frontend/src/shared/components/Layout.tsx` |
| GridSpinner | `frontend/src/shared/components/GridSpinner.tsx` |
| Problems list | `frontend/src/features/problems/ProblemListPage.tsx` |
| Problem detail (split editor) | `frontend/src/features/problems/ProblemDetailPage.tsx` |
| Generate page | `frontend/src/features/generate/GeneratePage.tsx` |
| Dashboard | `frontend/src/features/dashboard/DashboardPage.tsx` |
