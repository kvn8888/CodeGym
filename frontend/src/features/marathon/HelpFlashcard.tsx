// ── Types ────────────────────────────────────────────────────────────────────

interface HelpFlashcardProps {
  /** The concept being explained (e.g. "Binary Search Complexity"). */
  concept: string;
  /** An explanation of the concept. Does NOT answer the question directly. */
  explanation: string;
  /** Called when the user closes the flashcard. */
  onClose: () => void;
}

// ── Component ────────────────────────────────────────────────────────────────

/**
 * HelpFlashcard — a modal overlay that explains a concept without revealing
 * the answer to the current marathon question. Triggered by the "Help" button
 * during a marathon.
 *
 * Design matches the QuestionModal: bg-ink/30 backdrop with centered card.
 */
export function HelpFlashcard({ concept, explanation, onClose }: HelpFlashcardProps) {
  return (
    /* Backdrop overlay — click outside to close */
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-ink/30 backdrop-blur-sm"
      onClick={onClose}
    >
      {/* Flashcard */}
      <div
        className="w-full max-w-md mx-4 rounded-2xl border border-chalk bg-white overflow-hidden"
        style={{ boxShadow: '0 8px 32px rgba(0,0,0,0.12)' }}
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className="flex items-center justify-between px-6 pt-5 pb-3">
          <div className="flex items-center gap-2">
            {/* Lightbulb icon */}
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" style={{ color: '#d4a017' }}>
              <path d="M9 18h6M10 22h4M12 2a7 7 0 00-4 12.7V17h8v-2.3A7 7 0 0012 2z" />
            </svg>
            <h2 className="text-sm font-bold tracking-wide text-ink">{concept}</h2>
          </div>
          {/* Close button */}
          <button
            onClick={onClose}
            className="w-6 h-6 flex items-center justify-center rounded-md text-ash hover:text-ink hover:bg-grain transition-colors"
            aria-label="Close"
          >
            <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round">
              <path d="M4 4l8 8M12 4l-8 8" />
            </svg>
          </button>
        </div>

        {/* Explanation */}
        <div className="px-6 pb-6">
          <p className="text-sm text-graphite leading-relaxed">{explanation}</p>

          {/* Disclaimer */}
          <div className="mt-4 flex items-center gap-1.5 text-[10px] text-ash">
            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <circle cx="12" cy="12" r="10" />
              <line x1="12" y1="16" x2="12" y2="12" />
              <line x1="12" y1="8" x2="12.01" y2="8" />
            </svg>
            This explains the concept — it won't give away the answer.
          </div>
        </div>
      </div>
    </div>
  );
}
