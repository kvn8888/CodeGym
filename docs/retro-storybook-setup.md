# Setting Up Storybook 10 on a Bleeding-Edge Stack (Vite 8 + React 19 + Tailwind 4)

The CodeGym frontend had a working dev server but no way to preview UI states without a running backend. This session wired up Storybook so we can iterate on component designs — problem cards, the split-pane code editor, test result panels — without spinning up Docker containers or a Go API.

## The Starting Point

CodeGym is a self-hosted coding practice platform: the frontend is React 19 + TypeScript + Vite 8, styled with Tailwind CSS 4. That last detail matters. Tailwind 4 completely changed how it integrates with build tools — instead of a `tailwind.config.js` file, it uses a Vite plugin (`@tailwindcss/vite`) and a single `@import "tailwindcss"` in CSS. Most Storybook tutorials assume Tailwind 3.

The components we needed to preview had real complexity:
- `ProblemDetailPage` — a Monaco code editor (Monaco is the same editor VS Code uses), a problem description panel, hint reveal, and a test results panel, all fetching from the backend
- `ProblemListPage` — fetches a list of problems and renders cards with difficulty badges and language tags
- `GeneratePage` and `DashboardPage` — simpler, but still router-dependent

The core challenge: every "page" component either calls a real API or depends on React Router hooks like `useParams`. Neither works in a basic Storybook setup out of the box.

## Step 1: Running `storybook init`

Storybook's init command (`npx storybook@latest init --yes`) auto-detects the framework and generates config. It correctly identified `react-vite` and installed Storybook 10.2.19 — which is the version that supports Vite 8.

The init left us with:
- `.storybook/main.ts` — addon list and story glob patterns
- `.storybook/preview.ts` — global decorators and parameters
- A `src/stories/` folder full of sample Button/Header/Page components we didn't need

It also modified `vite.config.ts` to add Storybook's vitest plugin — an integration for running story-based tests in CI. We weren't setting up testing, just the dev viewer, so those additions would cause problems.

## Step 2: Trimming the Fat

The init added a lot we didn't need:

**In `vite.config.ts`**, it injected imports for `@storybook/addon-vitest/vitest-plugin` and `@vitest/browser-playwright` — neither of which were installed. This caused Vite to fail immediately:

```
Error [ERR_MODULE_NOT_FOUND]: Cannot find package '@storybook/addon-vitest'
```

**In `package.json`**, it added `vitest`, `playwright`, `@vitest/browser-playwright`, `@chromatic-com/storybook`, and `@storybook/addon-onboarding` as devDependencies — about 300MB of packages for features we wouldn't use (automated browser testing, Chromatic visual diffing, the interactive onboarding tour).

The fix was to strip both down to the essentials:

```ts
// vite.config.ts — reverted to the original, nothing Storybook-specific
export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: { port: 3000, proxy: { '/api': '...', '/ws': '...' } },
});
```

```json
// Only the addons we actually wanted
"addons": ["@storybook/addon-docs", "@storybook/addon-a11y"]
```

Lesson: `storybook init` is opinionated about its full testing stack. If you're not setting up vitest integration, manually prune the output before installing.

## Step 3: Making Tailwind 4 Work in Storybook

Tailwind 4 doesn't need any special Storybook configuration — because it's already a Vite plugin. Storybook's `react-vite` framework reads your `vite.config.ts` and inherits its plugins, so Tailwind is automatically active in every story.

The only thing required was importing the CSS in Storybook's preview file:

```ts
// .storybook/preview.tsx
import '../src/index.css';  // pulls in @import "tailwindcss" + CSS custom props
```

That single import gives every story the dark background, CSS variables (`--bg-primary`, `--accent`, etc.), and all Tailwind utility classes.

## Step 4: Wrapping Every Story in a Router

React Router's hooks (`useParams`, `useLocation`, `Link`) throw if they're used outside a router context. Since every page component uses at least one of them, the Storybook preview needed a global `MemoryRouter` decorator (a decorator in Storybook is a wrapper that applies to every story in scope — think of it like a layout in Next.js).

The tricky part: `useParams` only works if the story is rendered inside a matching `Route`. Just wrapping in `MemoryRouter` wasn't enough for `ProblemDetailPage`, which reads `const { id } = useParams<{ id: string }>()`.

The solution was a global decorator that renders each story inside a proper `Routes`/`Route` tree, with the path configurable per-story via `parameters`:

```tsx
// .storybook/preview.tsx
decorators: [
  (Story, context) => {
    const initialPath = context.parameters.initialPath ?? '/';
    const routePath = context.parameters.routePath ?? '*';
    return (
      <MemoryRouter initialEntries={[initialPath]}>
        <Routes>
          <Route path={routePath} element={<Story />} />
        </Routes>
      </MemoryRouter>
    );
  },
],
```

Then each story that needs `useParams` sets its own path:

```tsx
// ProblemDetailPage.stories.tsx
parameters: {
  initialPath: '/problems/two-sum',
  routePath: '/problems/:id',   // ← useParams sees { id: 'two-sum' }
},
```

## The Gotcha: `@storybook/test` and the Vitest Dependency

The original stories imported `beforeEach` from `@storybook/test` to set up fetch mocks:

```ts
import { beforeEach } from '@storybook/test';

export const WithProblems: Story = {
  beforeEach() {
    globalThis.fetch = mockFetch(sampleProblems) as typeof fetch;
  },
};
```

This is the documented Storybook 8+ API for story-level lifecycle hooks. The package was installed, but Storybook's Vite environment couldn't resolve it:

```
[UNRESOLVED_IMPORT] @storybook/test (imported by ProblemDetailPage.stories.tsx)
```

The root cause: `@storybook/test` depends on `vitest` internally — and we had removed vitest from the project when we stripped the testing addons. Vite's module resolver was failing trying to trace that dependency chain.

The fix was to move the fetch mock setup into Storybook **decorators** instead. Decorators run as React component wrappers before each story renders — no lifecycle hook API needed, no vitest dependency:

```tsx
export const WithProblems: Story = {
  decorators: [
    (Story) => {
      globalThis.fetch = makeFetchMock(sampleProblems) as typeof fetch;
      return <Story />;
    },
  ],
};
```

This is actually cleaner: the mock is scoped to a specific story, defined right next to it, and readable at a glance. The only tradeoff is that it doesn't tear down after the story renders — but since each story replaces `globalThis.fetch` with its own mock before rendering, there's no cross-contamination in the Storybook dev viewer.

## What Got Built

Fourteen stories covering every meaningful UI state:

| Group | Stories |
|-------|---------|
| Shell/Layout | On Problems, On Generate, On Dashboard (tests active nav state) |
| Pages/ProblemList | With Problems, No Problems, Go Only, Loading |
| Pages/ProblemDetail | Editor (blank), All Tests Passed, Tests Failed, Compile Error, JS/Express multi-file |
| Pages/Generate | Empty prompt |
| Pages/Dashboard | Empty state |

The `ProblemDetail` stories are the most valuable for iteration. They let you see exactly what the test results panel looks like for passing/failing/compile-error states without running a single Docker container.

## What's Next

The stories currently cover the pages as they exist — functional but visually sparse. The whole point of having Storybook is to use it for rapid visual iteration. Some concrete things to do now:

- **Better problem cards** — the list view is functional but plain; Storybook makes it easy to try different card layouts side-by-side
- **Test results panel** — the pass/fail display needs better visual hierarchy; compare states are now trivial with the dedicated stories
- **Dashboard stats** — once auth lands, replace the hardcoded zeros with real data and design the stat cards properly
- **Component extraction** — as the UI matures, pull reusable pieces (difficulty badge, language chip, hint accordion) into their own components with their own stories

---

*Tooling that reduces friction compounds over time — fifteen minutes of Storybook setup saves hours of "let me spin up the backend real quick" every day from here on.*
