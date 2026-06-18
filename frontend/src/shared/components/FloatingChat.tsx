import { useState, useRef, useCallback, useEffect } from 'react';
import {
  AnimatePresence,
  LayoutGroup,
  motion,
  useReducedMotion,
  type Transition,
} from 'motion/react';
import { Rnd } from 'react-rnd';
import { ChatPanel } from '../../features/chat/ChatPanel';
import {
  BUBBLE_SIZE,
  CHAT_MARGIN,
  MIN_CHAT_H,
  MIN_CHAT_W,
  defaultOpenBounds,
  loadStoredBounds,
  maxChatSize,
  saveStoredBounds,
  snapToNearestCorner,
  type ChatBounds,
} from './floatingChatGeometry';

const CARD_SHADOW = '3px 3px 0 0 var(--color-ink)';
const WINDOW_SHADOW = '5px 5px 0 0 var(--color-ink)';
const RADIUS_CLOSED = BUBBLE_SIZE / 2;
const RADIUS_OPEN = 16;
const CHAT_LAYOUT_ID = 'codegym-floating-chat-shell';
const OPEN_CONTENT_DELAY = 0.18;
const CONTENT_EASE = [0.16, 1, 0.3, 1] as [number, number, number, number];

type ChatStage = 'closed' | 'opening' | 'interactive' | 'closing';

function ChatBubbleIcon() {
  return (
    <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round">
      <path d="M21 15a2 2 0 01-2 2H7l-4 4V5a2 2 0 012-2h14a2 2 0 012 2z" />
    </svg>
  );
}

function ChatChrome({
  onClose,
  draggable = false,
}: {
  onClose: () => void;
  draggable?: boolean;
}) {
  return (
    <>
      <div
        className={`flex shrink-0 select-none items-center justify-between border-b border-chalk px-4 py-3 bg-bone ${
          draggable ? 'chat-drag-handle cursor-grab active:cursor-grabbing' : ''
        }`}
      >
        <div
          className={
            draggable
              ? 'pointer-events-none flex-1 min-w-0'
              : 'flex-1 min-w-0'
          }
        >
          <div className="font-display text-base font-semibold tracking-tight text-ink">
            Assistant<span className="text-blue">.</span>
          </div>
          <div className="text-[10px] text-graphite">Goals, skills, and memory</div>
        </div>
        <button
          onClick={onClose}
          className="chat-drag-cancel w-7 h-7 flex items-center justify-center rounded-lg text-ash hover:text-ink hover:bg-grain focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ink focus-visible:ring-offset-2 focus-visible:ring-offset-bone transition-colors"
          aria-label="Close chat"
          title="Close"
        >
          <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round">
            <path d="M4 4l8 8M12 4l-8 8" />
          </svg>
        </button>
      </div>
      <div className="flex-1 min-h-0 bg-bone">
        <ChatPanel />
      </div>
    </>
  );
}

function initialOpenBounds() {
  const stored = loadStoredBounds();
  return stored ? snapToNearestCorner(stored) : defaultOpenBounds();
}

/**
 * Floating chat launcher + window.
 * - Bubble/window morph uses Motion shared layout springs.
 * - Live window uses react-rnd for drag, resize, and corner snapping.
 */
export function FloatingChat() {
  const shouldReduceMotion = useReducedMotion();
  const [stage, setStage] = useState<ChatStage>('closed');
  const [snapTransitioning, setSnapTransitioning] = useState(false);
  const [bounds, setBounds] = useState<ChatBounds>(initialOpenBounds);
  const boundsRef = useRef(bounds);
  const snapTimerRef = useRef<number | null>(null);
  const maxSize = maxChatSize();

  const layoutTransition: Transition = shouldReduceMotion
    ? { duration: 0 }
    : { type: 'spring', stiffness: 430, damping: 36, mass: 0.9 };

  const contentTransition: Transition = shouldReduceMotion
    ? { duration: 0 }
    : { duration: 0.18, ease: CONTENT_EASE };

  const beginOpenMorph = useCallback(() => {
    const target = initialOpenBounds();
    setBounds(target);
    boundsRef.current = target;
    setStage('opening');
  }, []);

  const beginCloseMorph = useCallback(() => {
    setStage('closing');
    requestAnimationFrame(() => {
      requestAnimationFrame(() => setStage('closed'));
    });
  }, []);

  const finishOpening = useCallback(() => {
    setStage((current) => (current === 'opening' ? 'interactive' : current));
  }, []);

  const clearSnapTimer = useCallback(() => {
    if (snapTimerRef.current !== null) {
      window.clearTimeout(snapTimerRef.current);
      snapTimerRef.current = null;
    }
  }, []);

  const updateBounds = useCallback((next: ChatBounds) => {
    boundsRef.current = next;
    setBounds(next);
  }, []);

  const stopSnapTransition = useCallback(() => {
    clearSnapTimer();
    setSnapTransitioning(false);
  }, [clearSnapTimer]);

  const snapToBounds = useCallback(
    (released: ChatBounds) => {
      const target = snapToNearestCorner(released);
      saveStoredBounds(target);
      stopSnapTransition();
      updateBounds(released);

      requestAnimationFrame(() => {
        requestAnimationFrame(() => {
          setSnapTransitioning(true);
          updateBounds(target);
          snapTimerRef.current = window.setTimeout(() => {
            setSnapTransitioning(false);
            snapTimerRef.current = null;
          }, 300);
        });
      });
    },
    [stopSnapTransition, updateBounds],
  );

  const handleDragStop = useCallback(
    (_: unknown, data: { x: number; y: number }) => {
      snapToBounds({ ...boundsRef.current, x: data.x, y: data.y });
    },
    [snapToBounds],
  );

  const handleResizeStop = useCallback(
    (
      _: unknown,
      __: unknown,
      ref: HTMLElement,
      ___: unknown,
      position: { x: number; y: number },
    ) => {
      snapToBounds({
        x: position.x,
        y: position.y,
        width: ref.offsetWidth,
        height: ref.offsetHeight,
      });
    },
    [snapToBounds],
  );

  const handleDrag = useCallback(
    (_: unknown, data: { x: number; y: number }) => {
      updateBounds({ ...boundsRef.current, x: data.x, y: data.y });
    },
    [updateBounds],
  );

  const handleResize = useCallback(
    (
      _: unknown,
      __: unknown,
      ref: HTMLElement,
      ___: unknown,
      position: { x: number; y: number },
    ) => {
      updateBounds({
        x: position.x,
        y: position.y,
        width: ref.offsetWidth,
        height: ref.offsetHeight,
      });
    },
    [updateBounds],
  );

  useEffect(() => {
    const handleViewportResize = () => {
      if (stage !== 'interactive') return;
      setBounds((current) => {
        const next = snapToNearestCorner(current);
        boundsRef.current = next;
        saveStoredBounds(next);
        return next;
      });
    };
    window.addEventListener('resize', handleViewportResize);
    return () => window.removeEventListener('resize', handleViewportResize);
  }, [stage]);

  useEffect(() => () => clearSnapTimer(), [clearSnapTimer]);

  return (
    <LayoutGroup>
      {/* Launcher bubble */}
      <AnimatePresence initial={false}>
        {stage === 'closed' && (
          <motion.button
            key="chat-bubble"
            type="button"
            layoutId={CHAT_LAYOUT_ID}
            transition={layoutTransition}
            onClick={beginOpenMorph}
            whileHover={shouldReduceMotion ? undefined : { scale: 1.04 }}
            whileTap={shouldReduceMotion ? undefined : { scale: 0.96 }}
            className="chat-motion-shell fixed z-50 overflow-hidden rounded-full bg-transparent text-bone focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ink focus-visible:ring-offset-2 focus-visible:ring-offset-bone"
            style={{
              right: CHAT_MARGIN,
              bottom: CHAT_MARGIN,
              width: BUBBLE_SIZE,
              height: BUBBLE_SIZE,
              borderRadius: RADIUS_CLOSED,
              boxShadow: CARD_SHADOW,
            }}
            aria-label="Open chat"
          >
            <motion.span
              className="absolute inset-0 flex items-center justify-center rounded-full bg-ink hover:bg-ink-soft"
              initial={false}
              animate={{ opacity: 1, scale: 1 }}
              exit={{ opacity: 0, scale: 0.72 }}
              transition={contentTransition}
            >
              <ChatBubbleIcon />
            </motion.span>
          </motion.button>
        )}

        {(stage === 'opening' || stage === 'closing') && (
          <motion.div
            key="chat-motion-window"
            layoutId={CHAT_LAYOUT_ID}
            transition={layoutTransition}
            onLayoutAnimationComplete={stage === 'opening' ? finishOpening : undefined}
            className="chat-motion-shell fixed z-50 overflow-hidden rounded-2xl bg-transparent"
            style={{
              left: bounds.x,
              top: bounds.y,
              width: bounds.width,
              height: bounds.height,
              borderRadius: RADIUS_OPEN,
            }}
            role="dialog"
            aria-label="Chat assistant"
          >
            <motion.div
              className="flex h-full w-full flex-col overflow-hidden rounded-2xl border-2 border-ink bg-shell"
              style={{ boxShadow: WINDOW_SHADOW }}
              initial={false}
              animate={{
                opacity: stage === 'opening' ? 1 : 0,
                scale: stage === 'opening' ? 1 : 0.985,
              }}
              transition={{
                ...contentTransition,
                delay: stage === 'opening' && !shouldReduceMotion ? OPEN_CONTENT_DELAY : 0,
              }}
            >
              <ChatChrome onClose={beginCloseMorph} />
            </motion.div>
          </motion.div>
        )}
      </AnimatePresence>

      {/* Live window - draggable, resizable, corner-anchored */}
      {stage === 'interactive' && (
        <Rnd
          size={{ width: bounds.width, height: bounds.height }}
          position={{ x: bounds.x, y: bounds.y }}
          onDragStart={stopSnapTransition}
          onDrag={handleDrag}
          onDragStop={handleDragStop}
          onResizeStart={stopSnapTransition}
          onResize={handleResize}
          onResizeStop={handleResizeStop}
          dragHandleClassName="chat-drag-handle"
          cancel=".chat-drag-cancel"
          bounds="window"
          minWidth={MIN_CHAT_W}
          minHeight={MIN_CHAT_H}
          maxWidth={maxSize.width}
          maxHeight={maxSize.height}
          enableResizing={{
            // Edge handles render 10px injected strips that can show as slivers
            // during the Motion-to-Rnd handoff. Corners keep resize affordance.
            top: false,
            right: false,
            bottom: false,
            left: false,
            topRight: true,
            bottomRight: true,
            bottomLeft: true,
            topLeft: true,
          }}
          resizeHandleStyles={{
            bottomRight: { right: 0, bottom: 0, width: 18, height: 18, cursor: 'nwse-resize' },
            bottomLeft: { left: 0, bottom: 0, width: 18, height: 18, cursor: 'nesw-resize' },
            topRight: { right: 0, top: 0, width: 18, height: 18, cursor: 'nesw-resize' },
            topLeft: { left: 0, top: 0, width: 18, height: 18, cursor: 'nwse-resize' },
          }}
          resizeHandleClasses={{
            bottomRight: 'react-rnd-handle react-rnd-handle-br',
            bottomLeft: 'react-rnd-handle react-rnd-handle-bl',
            topRight: 'react-rnd-handle react-rnd-handle-tr',
            topLeft: 'react-rnd-handle react-rnd-handle-tl',
          }}
          style={{
            zIndex: 50,
            boxShadow: WINDOW_SHADOW,
          }}
          className={`react-rnd overflow-hidden rounded-2xl border-2 border-ink bg-shell ${
            snapTransitioning ? 'chat-snap-transition' : ''
          }`}
        >
          <div className="flex h-full min-h-0 flex-col">
            <ChatChrome onClose={beginCloseMorph} draggable />
          </div>
        </Rnd>
      )}
    </LayoutGroup>
  );
}
