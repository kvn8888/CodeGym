# From Stale Closures to Squircles: Fixing Marathon Selection and Adding Continuous Corners

Two bugs that looked simple. One CSS property that changes how every corner renders. A session that crossed from React state management into the cutting edge of CSS specifications — and a reminder that the smallest visual details are the hardest to get right.

## The Starting Point

CodeGym's MCQ Marathon page had just shipped — a timed quiz interface where users answer multiple-choice questions, get feedback, and see a score breakdown. The implementation used React's `useCallback` for the answer handler and Tailwind's default color palette (`emerald-400`, `red-500`) for correct/incorrect feedback.

Two problems surfaced immediately:

1. **Clicking one answer option would visually activate a different one** — a classic stale closure bug
2. **Feedback colors (green/red) weren't rendering at all** — because the project uses Tailwind v4's `@theme` which replaces the default color palette

On top of fixing these, the user wanted to adopt CSS `corner-shape: squircle` across the entire project — a brand-new CSS property (Chrome 139+) that creates Apple-style continuous corner smoothing.

## Step 1: The Stale Closure Bug

**Symptom**: Click Option A → Option B appears selected. Click Option B → Option A appears selected. The selections were inverted.

**Root cause**: The answer handler used `useCallback` with `selectedIndex` in the dependency array, but the memoized closure captured stale state. When React batched the state update and re-render, the callback still referenced the previous `selectedIndex` value.

```tsx
// BEFORE — stale closure problem
const handleAnswer = useCallback((i: number) => {
  if (selectedIndex !== null) return;   // ← captured old selectedIndex
  setSelectedIndex(i);
  // ... create result ...
}, [selectedIndex, questionIndex]);      // ← deps update too late
```

**Fix**: Remove `useCallback` entirely and split into two plain functions — `handleSelect` for radio-style picking and `handleConfirm` for locking in the answer:

```tsx
// AFTER — plain functions, no stale closure risk
const handleSelect = (index: number) => {
  if (confirmed || advancingRef.current) return;
  setSelectedIndex(index);
};

const handleConfirm = () => {
  if (selectedIndex === null || confirmed) return;
  setConfirmed(true);
  const result: QuestionResult = {
    questionId: currentQ.id,
    selectedIndex,
    correct: selectedIndex === currentQ.correctIndex,
    timeMs: elapsed * 1000,
    usedHelp: helpUsed,
  };
  setResults((prev) => [...prev, result]);
};
```

**Why not fix the `useCallback` deps?** The two-step flow (select → confirm) was the right UX anyway — it matches the existing QuestionModal pattern and prevents accidental answers. Removing `useCallback` was simpler than debugging the dependency array, and the handler isn't expensive enough to need memoization.

## The Gotcha: Ghost Clicks on Next

With the stale closure fixed, a second bug appeared: clicking "Next" would sometimes auto-select an option on the next question.

**Symptom**: Click Confirm → see feedback → click Next → the new question loads with an option already highlighted.

**Investigation**: The "Next" button sits in the same vertical column as the option buttons above it. When the button is clicked, React processes the state update synchronously — `handleNext` resets `selectedIndex` to `null` and `confirmed` to `false`. The button unmounts. But the browser's `mouseup` event hasn't fired yet. When it does, it lands on whatever DOM element is now under the cursor — which is an option button that just rendered.

**Root cause**: React's synchronous rendering means the Next button unmounts before the browser finishes processing the click event. The `mouseup` propagates to the newly-mounted option button underneath.

**Fix**: A `useRef` guard that blocks option selection for one animation frame after advancing:

```tsx
const advancingRef = useRef(false);

const handleSelect = (index: number) => {
  if (confirmed || advancingRef.current) return;  // ← blocked during transition
  setSelectedIndex(index);
};

const handleNext = () => {
  advancingRef.current = true;
  requestAnimationFrame(() => { advancingRef.current = false; });
  // ... advance to next question ...
};
```

This is a pattern worth remembering: **when buttons unmount during click handlers, the `mouseup` can land on whatever replaces them**. A one-frame ref guard is the cleanest fix — no `setTimeout`, no event stopPropagation hacks, no layout changes needed.

## Step 2: The Invisible Colors

With selection mechanics working, the next report: "I don't see any green or red. It's all black."

**Root cause**: The project uses Tailwind CSS v4's `@theme` directive to define a custom color palette:

```css
@theme {
  --color-bone: #FAF9F6;
  --color-ink: #1a1a1a;
  --color-moss: #2d5a27;
  --color-rust: #8b2500;
  /* ... */
}
```

In Tailwind v4, `@theme` **replaces** the default palette entirely. Classes like `text-emerald-600`, `bg-red-500`, and `text-amber-500` don't exist — they compile to nothing. The only valid color classes are the custom tokens: `text-ink`, `bg-moss`, `border-rust`, etc.

**First attempt**: Switch to the project's `moss` and `rust` tokens:

```tsx
classes = 'border-moss bg-moss/10 text-moss';  // correct answer
classes = 'border-rust bg-rust/10 text-rust';   // wrong answer
```

Still invisible. Why? Because Storybook loads components in isolation and may not fully process the `@theme` block the same way Vite's production build does. The custom tokens weren't resolving in the Storybook iframe.

**Final fix**: Inline hex styles bypass both Tailwind and Storybook entirely:

```tsx
let feedbackStyle: React.CSSProperties = {};
if (confirmed) {
  if (isCorrect) {
    feedbackStyle = {
      borderColor: '#2d5a27',
      backgroundColor: '#2d5a270d',  // moss with 5% opacity
      color: '#2d5a27'
    };
  } else if (isSelected) {
    feedbackStyle = {
      borderColor: '#8b2500',
      backgroundColor: '#8b25000d',  // rust with 5% opacity
      color: '#8b2500'
    };
  }
}
```

**Lesson**: When your design system uses custom color tokens and you need colors to work in both production AND Storybook, inline styles are the most reliable path. The tokens are still the source of truth — the hex values come directly from them — but the rendering doesn't depend on build tooling.

## Step 3: Continuous Corners with CSS `corner-shape`

The user wanted Apple-style continuous corner smoothing across the entire project. `corner-shape` is a new CSS property (Chrome 139+, Edge 139+) that changes how `border-radius` curves transition into straight edges.

**Standard `border-radius`** uses a circular arc — the curve starts abruptly where it meets the straight edge. **`corner-shape: squircle`** uses a superellipse curve that gradually eases into the corner, creating a smoother, more organic shape. Apple uses this on every iOS app icon.

### The implementation decision

I had two options:

1. **Per-component**: Add `style={{ cornerShape: 'squircle' }}` to every card, button, input, and modal
2. **Global rule**: Apply it once in CSS to all elements

Option 2 wins decisively. `corner-shape` only takes effect when `border-radius` is set, so applying it to `*` is safe — elements without rounded corners are unaffected. And it's future-proof: new components automatically get continuous corners without remembering to add the style.

```css
/* In index.css — three lines that change every corner in the app */
*,
*::before,
*::after {
  corner-shape: squircle;
}
```

### Safari graceful degradation

Safari doesn't support `corner-shape` (as of March 2026). But there's nothing to guard against — browsers that don't understand the property simply ignore it. The `border-radius` continues rendering standard circular corners. No `@supports` block needed, no fallback, no polyfill. The page doesn't crash, doesn't look broken, just looks... slightly less smooth.

This is exactly how CSS progressive enhancement should work: browsers that support the feature get the enhanced experience, others get the existing one. Zero cost.

### What about `rounded-full`?

Elements with `border-radius: 9999px` (Tailwind's `rounded-full`) — dots, pills, avatars — remain perfectly circular. A squircle on a 9999px radius is visually indistinguishable from a circle, so no exception needed.

## What's Next

- **Tailwind v4 `@theme` + Storybook**: Need to investigate why custom tokens don't resolve in Storybook's iframe. If fixable, the inline hex workaround can be reverted to proper token classes.
- **`corner-shape` animation**: The property is animatable between values (`squircle` → `scoop` → `notch`). Could be used for micro-interactions — e.g., a card morphing its corners on hover.
- **Cross-browser tracking**: Firefox and Safari support will come eventually. When it does, every rounded element in the app will automatically upgrade to continuous corners — zero code changes needed because of the global rule.

## Commits (this session)

| SHA | Description |
|-----|-------------|
| `dd60ef4` | Two-step radio+confirm flow for Marathon options |
| `916894f` | Ref guard to prevent ghost option selection on Next |
| `82ac20f` | Only highlight selected option (reverted) |
| `99e453e` | Restore dual-highlight with stronger colors |
| `0dd1509` | Switch to moss/rust project tokens |
| `1d2ddcd` | Inline hex styles for reliable Storybook rendering |
| `f973c86` | Global `corner-shape: squircle` + skill doc update |

---

Three lines of CSS can change how every corner in your app feels. The hard part isn't writing them — it's fixing the five bugs you find along the way.
