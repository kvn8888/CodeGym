# From Floating Button to Desktop-Style Chat Window

The chat started as a simple app surface, but the interaction we wanted was closer to a small desktop window: it should open from a button, move around, resize, snap to corners, and feel polished while doing it. This session turned that into a real floating chat system and, just as importantly, exposed where native-feeling UI polish depends on choosing the right library boundary.

## The Starting Point

The first requirement sounded small: make the chat window draggable and resizable. The extra details made it more interesting. The window needed to anchor cleanly to corners, preserve its last size and position during the session, avoid text selection while dragging, and eventually animate like Arc or Dia instead of popping between states.

That is a lot of pointer math if I hand-roll it: mouse capture, resize hit targets, bounds, snapping, drag cancellation, mobile viewport changes, and persistence. The better architecture was to split the problem:

- `react-rnd` owns the steady-state window behavior: drag, resize, bounds, and handle hit targets.
- A small geometry module owns CodeGym-specific behavior: corner anchors, viewport clamping, default size, and persisted bounds.
- Motion owns the button-to-window morph and the window-to-button return animation.

That split kept the custom code focused on product behavior instead of low-level pointer mechanics.

## Step 1: Use a Drag/Resize Library for the Window

I chose `react-rnd` because it combines draggable and resizable behavior into one React component. That matters here because the chat is not just draggable and not just resizable; the size and position need to update as one coherent window state.

The live window now delegates interaction mechanics to `Rnd`:

```tsx
<Rnd
  size={{ width: bounds.width, height: bounds.height }}
  position={{ x: bounds.x, y: bounds.y }}
  onDrag={handleDrag}
  onDragStop={handleDragStop}
  onResize={handleResize}
  onResizeStop={handleResizeStop}
  dragHandleClassName="chat-drag-handle"
  cancel=".chat-drag-cancel"
  bounds="window"
>
  <ChatChrome onClose={beginCloseMorph} draggable />
</Rnd>
```

The important part is not just `Rnd`; it is the contract around it. The chat header gets the drag handle class, while the close button gets a cancel class. That is what prevents the close button from becoming part of the draggable surface.

## The Gotcha: Dragging Selected Text Instead of Moving the Window

The first browser test showed the wrong behavior: dragging the top of the window highlighted the header text instead of moving the window. That looked like a CSS or handle-targeting bug at first, but the console showed the real root cause:

```txt
ReferenceError: process is not defined
```

The error came from the bundled drag library expecting `process.env.NODE_ENV` to exist. Vite does not provide Node's `process` global in the browser by default, so the drag code was failing before it could do its job.

The fix was a Vite compatibility shim scoped to the environment key the dependency actually needs:

```ts
export default defineConfig(({ mode }) => ({
  define: {
    'process.env.NODE_ENV': JSON.stringify(
      mode === 'production' ? 'production' : 'development',
    ),
  },
  plugins: [react(), tailwindcss()],
}));
```

After that, the drag handle behaved like a handle instead of selectable text. This was a good reminder that when an interaction does nothing, the browser console can be more useful than staring at JSX class names.

## Step 2: Move Product Geometry Out of the Component

The snap behavior is product-specific, so I did not want it buried inside event handlers. I moved the geometry into `floatingChatGeometry.ts`, where the rules are explicit:

```ts
export type AnchorCorner =
  | 'top-left'
  | 'top-right'
  | 'bottom-left'
  | 'bottom-right';

const WINDOW_ANCHORS: AnchorCorner[] = [
  'top-right',
  'top-left',
  'bottom-left',
  'bottom-right',
];
```

That file also clamps sizes to the viewport, calculates each corner position, finds the nearest corner, and stores the last bounds in `sessionStorage`. This made it straightforward to correct the missing bottom-right snap later, because the allowed anchors are just data.

The result is that release behavior stays short and readable:

```ts
const snapToBounds = useCallback((released: ChatBounds) => {
  const target = snapToNearestCorner(released);
  saveStoredBounds(target);
  stopSnapTransition();
  updateBounds(released);

  requestAnimationFrame(() => {
    requestAnimationFrame(() => {
      setSnapTransitioning(true);
      updateBounds(target);
    });
  });
}, [stopSnapTransition, updateBounds]);
```

The double `requestAnimationFrame` is intentional. It lets the browser paint the released position first, then transition to the snapped position. Without that separation, React can collapse the two states and the snap looks like a teleport.

## Step 3: Let Motion Own the Morph

The first snap polish used CSS transitions, which worked for dragging. But the bigger interaction was the button-to-window and window-to-button animation. For that, I switched to Motion, the modern package behind Framer Motion's React animation API.

The key idea is a shared layout id. The closed bubble and the temporary opening/closing window use the same `layoutId`, so Motion can interpolate one shape into the other:

```tsx
const CHAT_LAYOUT_ID = 'codegym-floating-chat-shell';

<motion.button layoutId={CHAT_LAYOUT_ID} ... />

<motion.div
  layoutId={CHAT_LAYOUT_ID}
  transition={{ type: 'spring', stiffness: 430, damping: 36, mass: 0.9 }}
>
  <ChatChrome onClose={beginCloseMorph} />
</motion.div>
```

I kept `react-rnd` out of the morph itself. During the opening and closing animation, Motion renders a temporary shell. Once the open animation completes, the component switches to the real `Rnd` window for interaction.

That state split is the main architecture choice:

```ts
type ChatStage = 'closed' | 'opening' | 'interactive' | 'closing';
```

`closed` renders the button. `opening` and `closing` render Motion's shared-layout shell. `interactive` renders the draggable, resizable `Rnd` window. This is slightly more state than a boolean, but it keeps each library in the phase where it is strongest.

## The Revision: Fix the Polished Details

The first Motion pass exposed two visual defects.

First, during the close animation, the button briefly looked gray before returning to black. The cause was Motion's shared-layout crossfade applying opacity to the outer shared element. I wanted the geometry to morph, but not the launcher shell color to fade through gray. The fix was to pin the shared shell opacity while keeping the inner content fade:

```css
.chat-motion-shell {
  opacity: 1 !important;
}
```

Second, the release animation initially bounced because I had added a separate scale-settle keyframe after the snap. That made sense when thinking "spring," but it felt strange in a utility window: dropping the chat should feel like it lands, not like it rebounds. The final snap keeps a short ease to the nearest corner and removes the scale overshoot.

Third, the resize handles had visible corner marks. They technically communicated resize affordance, but they made the open chat look like a debug overlay instead of a polished app window. The final version keeps the resize hit targets and cursors, but removes the visible marks:

```css
.react-rnd-handle {
  background: transparent;
}
```

Those two details are easy to dismiss as cosmetic, but they are exactly where the interface starts feeling native. The user should feel the spring and understand the window can resize without seeing implementation artifacts at all four corners.

## What's Next

There are two practical follow-ups I would consider.

First, Motion increased the production bundle enough for Vite to warn about a large chunk. That is not automatically a problem, but the floating chat is a good candidate for lazy loading if we want to keep the initial route lighter.

Second, the current chat panel still uses mock responses. The UI shell is now ready for real use, but the next product step is wiring the panel to the backend chat API and making memory updates visible in a way that matches the new floating interaction.

The best version of this feature is not custom pointer math everywhere. It is a clean handoff: libraries handle the hard interaction primitives, and our code owns the behavior that makes the product feel like CodeGym.
