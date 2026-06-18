import { useState, useEffect, useRef } from 'react';
import { motion } from 'motion/react';
import { HelpFlashcard } from './HelpFlashcard';

// ── Types ────────────────────────────────────────────────────────────────────

/** A single multiple-choice question in the marathon. */
interface MarathonQuestion {
  id: string;
  text: string;
  options: string[];
  correctIndex: number;
  concept: string;
  helpContent: string;
}

/** Result for a single answered question. */
interface QuestionResult {
  questionId: string;
  selectedIndex: number;
  correct: boolean;
  timeMs: number;
  usedHelp: boolean;
}

// ── Mock data ────────────────────────────────────────────────────────────────

/** Sample questions for the marathon demo. In production, these come from
 *  POST /api/v1/marathon/generate which reads the user's memory file. */
const MOCK_QUESTIONS: MarathonQuestion[] = [
  {
    id: 'mq1',
    text: 'What is the time complexity of binary search?',
    options: ['O(n)', 'O(log n)', 'O(n log n)', 'O(1)'],
    correctIndex: 1,
    concept: 'Binary Search Complexity',
    helpContent:
      'Binary search works by repeatedly halving the search space. Each comparison eliminates half of the remaining elements, so the number of steps is proportional to log₂(n).',
  },
  {
    id: 'mq2',
    text: 'Which data structure uses FIFO ordering?',
    options: ['Stack', 'Queue', 'Heap', 'Hash Map'],
    correctIndex: 1,
    concept: 'Queue Data Structure',
    helpContent:
      'FIFO stands for First-In, First-Out. Elements are removed in the same order they were added. Think of a line at a coffee shop — the first person in line is served first.',
  },
  {
    id: 'mq3',
    text: 'What does the "two pointer" technique typically optimize?',
    options: [
      'Space complexity from O(n) to O(1)',
      'Time complexity from O(n²) to O(n)',
      'Both time and space',
      'Neither — it simplifies code',
    ],
    correctIndex: 1,
    concept: 'Two Pointer Technique',
    helpContent:
      'The two pointer technique uses two references that move through the data structure, usually from opposite ends or at different speeds. It commonly reduces nested loops (O(n²)) to a single pass (O(n)).',
  },
  {
    id: 'mq4',
    text: 'In a hash table, what is a collision?',
    options: [
      'When two keys produce the same hash',
      'When the table runs out of space',
      'When a key is deleted',
      'When lookup takes O(n)',
    ],
    correctIndex: 0,
    concept: 'Hash Collisions',
    helpContent:
      'A collision occurs when two different keys are mapped to the same index by the hash function. Common resolution strategies include chaining (linked lists at each bucket) and open addressing (probing for the next open slot).',
  },
  {
    id: 'mq5',
    text: 'What traversal order does BFS use?',
    options: ['Depth-first', 'Level-order', 'In-order', 'Post-order'],
    correctIndex: 1,
    concept: 'Breadth-First Search',
    helpContent:
      'BFS explores nodes level by level, visiting all neighbors of a node before moving to the next depth. It uses a queue to track the frontier and is ideal for finding the shortest path in unweighted graphs.',
  },
];

// ── Component ────────────────────────────────────────────────────────────────

/**
 * MarathonPage — timed multiple-choice question marathon.
 *
 * The LLM generates a set of 5–10 questions based on the user's memory file.
 * Each question is timed. The results (right/wrong, time, help usage) feed
 * back into the user's memory to reinforce known concepts and expand into
 * new territory.
 *
 * Three states:
 * - idle: start screen (select topic, see previous scores)
 * - active: question display with timer + options + help button
 * - results: score summary + time breakdown + recommendations
 */
export function MarathonPage() {
  // Which phase the marathon is in.
  const [phase, setPhase] = useState<'idle' | 'active' | 'results'>('idle');

  // Guard: prevents accidental option selection when Next button unmounts
  // and mouseup lands on an option button underneath.
  const advancingRef = useRef(false);

  // Index of the current question (0-based).
  const [questionIndex, setQuestionIndex] = useState(0);

  // Accumulated results for each answered question.
  const [results, setResults] = useState<QuestionResult[]>([]);

  // Timer: seconds elapsed on the current question.
  const [elapsed, setElapsed] = useState(0);

  // Which option the user has selected (before confirming). null = none.
  const [selectedIndex, setSelectedIndex] = useState<number | null>(null);

  // Whether the user has confirmed their answer (locks in + shows feedback).
  const [confirmed, setConfirmed] = useState(false);

  // Whether the help flashcard is visible.
  const [showHelp, setShowHelp] = useState(false);

  // Whether help was used on the current question.
  const [helpUsed, setHelpUsed] = useState(false);

  const questions = MOCK_QUESTIONS;
  const currentQ = questions[questionIndex];

  // ── Timer logic ──────────────────────────────────────────────────────────
  useEffect(() => {
    // Timer runs only while the question is active and not yet confirmed.
    if (phase !== 'active' || confirmed) return;
    const interval = setInterval(() => setElapsed((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, [phase, confirmed]);

  /** Start the marathon. */
  const handleStart = () => {
    setPhase('active');
    setQuestionIndex(0);
    setResults([]);
    setElapsed(0);
    setSelectedIndex(null);
    setConfirmed(false);
    setHelpUsed(false);
  };

  /** User selects an answer option (radio-style, can change before confirming). */
  const handleSelect = (index: number) => {
    if (confirmed || advancingRef.current) return; // locked or transitioning
    setSelectedIndex(index);
  };

  /** User confirms their selection — locks in the answer and shows feedback. */
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

  /** Advance to the next question or show results. */
  const handleNext = () => {
    // Block option clicks until the next frame to prevent the mouseup
    // from the disappearing Next button from selecting an option.
    advancingRef.current = true;
    requestAnimationFrame(() => { advancingRef.current = false; });

    if (questionIndex < questions.length - 1) {
      setQuestionIndex((i) => i + 1);
      setElapsed(0);
      setSelectedIndex(null);
      setConfirmed(false);
      setShowHelp(false);
      setHelpUsed(false);
    } else {
      setPhase('results');
    }
  };

  /** Open the help flashcard. */
  const handleHelp = () => {
    setShowHelp(true);
    setHelpUsed(true);
  };

  // ── Idle state: start screen ─────────────────────────────────────────────
  if (phase === 'idle') {
    return (
      <div className="max-w-lg mx-auto px-6 py-24 text-center bg-dots rounded-3xl mt-12">
        <h1 className="font-display text-5xl font-semibold text-ink tracking-tight mb-4">
          MCQ <span className="marker-accent">Marathon</span>
        </h1>
        <p className="text-sm text-graphite mb-10 leading-relaxed max-w-sm mx-auto">
          Timed multiple-choice reps. The AI adapts — reinforcing what you know,
          stretching where you don't.
        </p>
        <motion.button
          onClick={handleStart}
          whileHover={{ scale: 1.03 }}
          whileTap={{ scale: 0.96 }}
          transition={{ type: 'spring', stiffness: 460, damping: 26 }}
          className="px-7 py-3.5 bg-ink text-bone text-sm font-bold rounded-xl hover:bg-ink-soft transition-colors"
          style={{ boxShadow: 'var(--cg-card-shadow)' }}
        >
          Start Marathon ({questions.length} questions)
        </motion.button>
      </div>
    );
  }

  // ── Results state: summary ───────────────────────────────────────────────
  if (phase === 'results') {
    const correct = results.filter((r) => r.correct).length;
    const totalTime = results.reduce((sum, r) => sum + r.timeMs, 0);
    const avgTime = Math.round(totalTime / results.length / 1000);

    return (
      <div className="max-w-lg mx-auto px-6 py-24">
        <h1 className="font-display text-4xl font-semibold text-ink tracking-tight mb-8 text-center">
          Results<span className="text-blue">.</span>
        </h1>

        {/* Score card */}
        <motion.div
          initial={{ opacity: 0, y: 20, scale: 0.96 }}
          animate={{ opacity: 1, y: 0, scale: 1 }}
          transition={{ type: 'spring', stiffness: 320, damping: 26 }}
          className="rounded-2xl border border-chalk bg-white p-6 mb-6"
          style={{ boxShadow: 'var(--cg-card-shadow)' }}
        >
          <div className="flex items-center justify-between mb-4">
            <div>
              <div className="font-display text-5xl font-semibold text-ink">
                {correct}<span className="text-ash text-3xl">/{results.length}</span>
              </div>
              <div className="text-xs text-graphite mt-2">correct answers</div>
            </div>
            <div className="text-right">
              <div className="font-display text-5xl font-semibold text-blue">{avgTime}s</div>
              <div className="text-xs text-graphite mt-2">avg per question</div>
            </div>
          </div>

          {/* Per-question breakdown */}
          <div className="flex flex-col gap-2 mt-4 border-t-2 border-grain pt-4">
            {results.map((r, i) => (
              <div key={r.questionId} className="flex items-center gap-3 text-xs">
                <span className="text-ash w-4">{i + 1}.</span>
                <span
                  className="flex h-4 w-4 items-center justify-center rounded-full text-[10px] font-bold"
                  style={{
                    color: 'var(--color-shell)',
                    backgroundColor: r.correct ? 'var(--color-moss)' : 'var(--color-rust)',
                  }}
                >
                  {r.correct ? '✓' : '✗'}
                </span>
                <span className="text-graphite flex-1 truncate">
                  {questions.find((q) => q.id === r.questionId)?.concept}
                </span>
                <span className="text-ash">{Math.round(r.timeMs / 1000)}s</span>
                {r.usedHelp && (
                  <span
                    className="rounded-full px-2 py-0.5 text-[10px] font-bold"
                    style={{ color: 'var(--color-honey)', backgroundColor: 'var(--color-honey-tint)' }}
                  >
                    help
                  </span>
                )}
              </div>
            ))}
          </div>
        </motion.div>

        <div className="flex gap-3 justify-center">
          <button
            onClick={handleStart}
            className="px-5 py-2.5 bg-ink text-bone text-xs font-bold rounded-xl hover:bg-ink-soft transition-colors"
          >
            Try Again
          </button>
          <button
            onClick={() => setPhase('idle')}
            className="px-5 py-2.5 border border-chalk bg-white text-xs font-medium text-graphite rounded-xl hover:border-ash hover:text-ink transition-colors"
          >
            Back
          </button>
        </div>
      </div>
    );
  }

  // ── Active state: question display ───────────────────────────────────────
  return (
    <div className="max-w-lg mx-auto px-6 py-12">
      {/* Header: question counter + timer + score */}
      <div className="flex items-center justify-between mb-8">
        <span className="text-xs text-ash font-medium">
          {questionIndex + 1} of {questions.length}
        </span>
        <div className="flex items-center gap-4">
          {/* Timer display */}
          <span className="text-xs text-graphite tabular-nums">{elapsed}s</span>
          {/* Score tally */}
          <span className="text-xs text-ash">
            {results.filter((r) => r.correct).length}✓ {results.filter((r) => !r.correct).length}✗
          </span>
        </div>
      </div>

      {/* Progress bar */}
      <div className="h-2 rounded-full bg-grain mb-8 border border-ink/20">
        <motion.div
          className="h-full rounded-full bg-ink"
          animate={{ width: `${((questionIndex + 1) / questions.length) * 100}%` }}
          transition={{ type: 'spring', stiffness: 200, damping: 26 }}
        />
      </div>

      {/* Question text */}
      <h2 className="font-display text-2xl font-semibold text-ink mb-6 leading-snug">{currentQ.text}</h2>

      {/* Option buttons — radio-style selection (same behavior as QuestionModal) */}
      <div className="flex flex-col gap-3 mb-8">
        {currentQ.options.map((option, i) => {
          const isSelected = selectedIndex === i;
          const isCorrect = i === currentQ.correctIndex;

          // Before confirm: radio highlight on the selected option only.
          // After confirm: moss (correct) and rust (wrong) feedback states.
          const baseStyle = 'border-chalk bg-shell text-graphite hover:border-ash hover:bg-bone';
          const selectedStyle = 'border-blue bg-blue-tint text-ink font-medium';
          const mutedStyle = 'border-chalk bg-shell text-ash';

          let feedbackStyle: React.CSSProperties = {};
          let classes = baseStyle;

          if (confirmed) {
            if (isCorrect) {
              classes = mutedStyle; // base, override with inline
              feedbackStyle = {
                borderColor: 'var(--color-moss)',
                backgroundColor: 'var(--color-moss-tint)',
                color: 'var(--color-moss)',
              };
            } else if (isSelected) {
              classes = mutedStyle;
              feedbackStyle = {
                borderColor: 'var(--color-rust)',
                backgroundColor: 'var(--color-rust-tint)',
                color: 'var(--color-rust)',
              };
            } else {
              classes = mutedStyle;
            }
          } else if (isSelected) {
            classes = selectedStyle;
          }

          return (
            <button
              key={i}
              onClick={() => handleSelect(i)}
              disabled={confirmed}
              style={feedbackStyle}
              className={`w-full text-left text-sm px-4 py-3 rounded-xl border-2 transition-all duration-150 ${classes}`}
            >
              <div className="flex items-center gap-3">
                {/* Radio circle — filled for selected option and correct answer after confirm */}
                <div
                  className="w-4 h-4 rounded-full border-2 shrink-0 flex items-center justify-center transition-colors"
                  style={
                    confirmed
                      ? {
                          borderColor: isCorrect
                            ? 'var(--color-moss)'
                            : isSelected
                              ? 'var(--color-rust)'
                              : undefined,
                        }
                      : { borderColor: isSelected ? 'var(--color-blue)' : undefined }
                  }
                >
                  {(isSelected || (confirmed && isCorrect)) && (
                    <div
                      className="w-2 h-2 rounded-full"
                      style={{
                        backgroundColor: confirmed
                          ? (isCorrect ? 'var(--color-moss)' : 'var(--color-rust)')
                          : 'var(--color-blue)',
                      }}
                    />
                  )}
                </div>
                {option}
                {/* Small label after confirming */}
                {confirmed && isCorrect && (
                  <span className="ml-auto text-xs font-bold" style={{ color: 'var(--color-moss)' }}>✓ Correct</span>
                )}
                {confirmed && isSelected && !isCorrect && (
                  <span className="ml-auto text-xs font-bold" style={{ color: 'var(--color-rust)' }}>✗ Wrong</span>
                )}
              </div>
            </button>
          );
        })}
      </div>

      {/* Footer: Help + Confirm/Next */}
      <div className="flex items-center justify-between">
        {/* Help button (only enabled before confirming) */}
        <button
          onClick={handleHelp}
          disabled={confirmed}
          className="flex items-center gap-1.5 text-xs text-graphite hover:text-ink disabled:text-ash disabled:cursor-not-allowed transition-colors"
        >
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <circle cx="12" cy="12" r="10" />
            <path d="M9.09 9a3 3 0 015.83 1c0 2-3 3-3 3M12 17h.01" />
          </svg>
          Help
        </button>

        {/* Confirm button (before confirming) or Next button (after confirming) */}
        {!confirmed && selectedIndex !== null && (
          <motion.button
            onClick={handleConfirm}
            initial={{ opacity: 0, y: 6 }}
            animate={{ opacity: 1, y: 0 }}
            whileTap={{ scale: 0.95 }}
            className="px-5 py-2.5 bg-ink text-bone text-xs font-bold rounded-xl hover:bg-ink-soft transition-colors"
          >
            Confirm
          </motion.button>
        )}
        {confirmed && (
          <motion.button
            onClick={handleNext}
            initial={{ opacity: 0, y: 6 }}
            animate={{ opacity: 1, y: 0 }}
            whileTap={{ scale: 0.95 }}
            className="px-5 py-2.5 bg-blue text-white text-xs font-bold rounded-xl hover:bg-blue-hover transition-colors"
          >
            {questionIndex < questions.length - 1 ? 'Next' : 'See Results'}
          </motion.button>
        )}
      </div>

      {/* Help flashcard modal */}
      {showHelp && (
        <HelpFlashcard
          concept={currentQ.concept}
          explanation={currentQ.helpContent}
          onClose={() => setShowHelp(false)}
        />
      )}
    </div>
  );
}
