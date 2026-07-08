# shadcn/ui Migration — Status & Remaining Work

Branch: `codegym-shadcn` (off `codegym-v2`).

**Goal:** full migration to the shadcn/ui default **"neutral"** theme — a new visual
identity replacing the previous Vercel Geist token system — with light + dark mode.

This doc is the handoff for coding agents to finish the migration. Pick any
unchecked task below; they are independent unless noted.

---

## ✅ Done & verified (light + dark, `tsc -b` + `vite build` pass)

**Foundation**
- Deps: `class-variance-authority`, `clsx`, `tailwind-merge`, `tw-animate-css`,
  `@radix-ui/react-*` (dialog, select, tabs, tooltip, accordion, progress,
  separator, scroll-area, avatar, dropdown-menu, label, slot).
- `@/` alias (`vite.config.ts` resolve.alias; `tsconfig.app.json` already had paths).
- `components.json`, `src/lib/utils.ts` (`cn`).
- `src/index.css`: shadcn neutral tokens (`:root` + `.dark`), `@theme inline`
  mapping, **Geist → shadcn bridge** (see Notes), `tw-animate-css`, accordion
  keyframes, re-themed transitions-dev snippets.
- UI primitives in `src/components/ui/`: button, card, badge, input, textarea,
  label, separator, skeleton, dialog, tooltip, progress, avatar, accordion,
  scroll-area, select, dropdown-menu.
- Dark mode: `src/components/theme-provider.tsx` + `mode-toggle.tsx`;
  `ThemeProvider` + `TooltipProvider` wired in `src/App.tsx`. Default light;
  persists to `localStorage['codegym-theme']`.

**Migrated**
- `shared/components/Layout.tsx` — sidebar tokens, `DropdownMenu` + `Avatar`
  profile menu, `Button` collapse toggle, `ModeToggle`. (Kept Framer collapse
  spring, nav active `layoutId`, and the CSS sidebar tooltip.)
- `features/dashboard/DashboardPage.tsx` — `Card`.
- `features/problems/ProblemListPage.tsx` — `Card`/`Badge`/`Select`/`Skeleton`.
- `features/memory/MemoryPage.tsx` — `Card`/`Progress`/`Badge` + **`Accordion`** (Problem Notes).
- `features/marathon/MarathonPage.tsx` — `Card`/`Button`/`Progress` + **success-check** on a correct answer.
- `features/generate/GeneratePage.tsx` — **`SlidingTabs`** on the Command/Spotlight switcher + the format control; language `Select`.
- `features/generate/QuestionModal.tsx` + `features/marathon/HelpFlashcard.tsx` — shadcn `Dialog`.

**Follow-up transitions (requested):** success-check (Marathon), accordion
(Memory notes), SlidingTabs (Command/Spotlight) — all done.

---

## 🔧 Remaining (to call the full migration done)

- [ ] **ProblemDetailPage** (`features/problems/ProblemDetailPage.tsx`): migrate
  non-editor chrome — `RUN` → `Button`, lang/min chips → `Badge`, file tabs →
  shadcn `Tabs` (add `components/ui/tabs.tsx`) or keep custom, hint reveals →
  `Button`. Monaco editor stays. The results panel hardcodes `#1e1e1e`/`#252526`/
  `#f14c4c` etc. — either keep as a deliberate "editor" surface or tokenize.
- [x] **Chat** (`shared/components/FloatingChat.tsx`, `features/chat/ChatPanel.tsx`):
  shadcn chat block — `Card` chrome, `InputGroup` composer, `MessageScroller` +
  `Bubble`/`Message` list, `Marker`/`Spinner` thinking state. Framer
  bubble↔window morph + `react-rnd` drag/resize preserved.
- [ ] **Finish GeneratePage**: convert `GenerateButton`, the Recent/Example
  chips, and the difficulty buttons to `Button`/`Badge` variants; the command
  `textarea` + spotlight `input` are still borderless inline (lightly themed) —
  decide whether to use shadcn `Textarea`/`Input`.
- [ ] **Semantic chip dark variants**: difficulty/status badges use fixed light
  tints (`bg-green-100` …). Add dark variants (e.g. `dark:bg-green-500/15
  dark:text-green-400`) in ProblemListPage, MemoryPage, MarathonPage so they're
  not over-bright on dark cards.
- [ ] **Storybook** (14 stories): components now rely on `ThemeProvider` +
  `TooltipProvider` and shadcn imports. Add both as global decorators in
  `.storybook/preview.tsx`, run `npm run storybook`, fix per-story props. (Not
  verified during this migration.)
- [ ] **Retire the Geist bridge**: once every component uses native shadcn
  classes, delete the `Geist → shadcn neutral bridge` block + colored compat
  aliases in `src/index.css`, plus unused `cg-*` helpers. Sweep:
  `rg 'gray-1000|background-100|gray-alpha|cg-focus|cg-card-shadow|cg-surface' src`.
- [ ] **Dead CSS**: `.t-modal*` in `index.css` is now unused (modals → `Dialog`).
  Still used: `.t-tabs*` (SlidingTabs), `.t-tt*` (sidebar tooltip), `.t-check`
  (success-check). Remove `.t-modal*` when convenient.
- [ ] *(optional)* Swap the custom `.t-tt` sidebar tooltip for shadcn `Tooltip`
  (Radix portal escapes overflow natively) for consistency.

---

## ⚠️ Notes / gotchas

- **The bridge is the migration strategy.** `src/index.css` remaps legacy
  utilities (`bg-background-100` → `--card`, `text-gray-1000` → `--foreground`,
  `border-gray-alpha-*` → `color-mix(... --foreground ...)`, etc.) onto shadcn
  semantic tokens. So **un-migrated components already render in the neutral
  palette and respond to dark mode** — migrate at your own pace, nothing breaks.
- **Vite was downgraded 8.0.0 → 7.3.5.** The shadcn install needed
  `--legacy-peer-deps` (pre-existing Storybook 10 ⇄ Vite peer conflict), and that
  re-resolved Vite to 7.3.5 to satisfy Storybook's peer range (`npm ls vite` →
  7.3.5 deduped). Build + dev work on 7.3.5, but `package.json` still lists
  `^8.0.0`. Reconcile if desired (pin 7.3.5, or wait for Storybook's Vite-8 support).
- `npm audit` reports pre-existing transitive vulnerabilities — not introduced by
  the core shadcn deps.
- **Don't commit the backend files.** The working tree has unrelated
  `backend/internal/auth/*` and `backend/internal/identity/*` modifications from
  separate work — left untouched. Stage only `frontend/**` (and this file) when
  committing the migration.
