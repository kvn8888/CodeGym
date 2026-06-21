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
      <div className="mx-auto mt-12 max-w-lg rounded-xl border border-gray-alpha-200 bg-background-100 px-6 py-16 text-center" style={{ boxShadow: 'var(--cg-card-shadow)' }}>
        <h1 className="mb-4 text-[48px] font-semibold leading-[56px] tracking-[-2.88px] text-gray-1000">
          MCQ Marathon
        </h1>
        <p className="mx-auto mb-10 max-w-sm text-sm leading-6 text-gray-900">
          Timed multiple-choice reps. The AI adapts by reinforcing what you know,
          stretching where you don't.
        </p>
        <motion.button
          onClick={handleStart}
          whileHover={{ scale: 1.03 }}
          whileTap={{ scale: 0.96 }}
          transition={{ type: 'spring', stiffness: 460, damping: 26 }}
          className="cg-focus h-12 rounded-md bg-gray-1000 px-7 text-base font-medium text-background-100 transition-colors hover:bg-gray-900"
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
      <div className="mx-auto max-w-lg px-6 py-24">
        <h1 className="mb-8 text-center text-[40px] font-semibold leading-[48px] tracking-[-2.4px] text-gray-1000">
          Results
        </h1>

        {/* Score card */}
        <motion.div
          initial={{ opacity: 0, y: 20, scale: 0.96 }}
          animate={{ opacity: 1, y: 0, scale: 1 }}
          transition={{ type: 'spring', stiffness: 320, damping: 26 }}
          className="mb-6 rounded-xl border border-gray-alpha-200 bg-background-100 p-6"
          style={{ boxShadow: 'var(--cg-card-shadow)' }}
        >
          <div className="flex items-center justify-between mb-4">
            <div>
              <div className="text-[48px] font-semibold leading-[56px] tracking-[-2.88px] text-gray-1000">
                {correct}<span className="text-3xl text-gray-700">/{results.length}</span>
              </div>
              <div className="mt-2 text-sm text-gray-900">correct answers</div>
            </div>
            <div className="text-right">
              <div className="text-[48px] font-semibold leading-[56px] tracking-[-2.88px] text-blue-700">{avgTime}s</div>
              <div className="mt-2 text-sm text-gray-900">avg per question</div>
            </div>
          </div>

          {/* Per-question breakdown */}
          <div className="mt-4 flex flex-col gap-2 border-t border-gray-alpha-200 pt-4">
            {results.map((r, i) => (
              <div key={r.questionId} className="flex items-center gap-3 text-xs">
                <span className="w-4 text-gray-700">{i + 1}.</span>
                <span
                  className="flex h-4 w-4 items-center justify-center rounded-full text-[10px] font-bold"
                  style={{
                    color: 'var(--color-background-100)',
                    backgroundColor: r.correct ? 'var(--color-green-700)' : 'var(--color-red-800)',
                  }}
                >
                  {r.correct ? '✓' : '✗'}
                </span>
                <span className="flex-1 truncate text-gray-900">
                  {questions.find((q) => q.id === r.questionId)?.concept}
                </span>
                <span className="text-gray-700">{Math.round(r.timeMs / 1000)}s</span>
                {r.usedHelp && (
                  <span
                    className="rounded-full px-2 py-0.5 text-[10px] font-bold"
                    style={{ color: 'var(--color-amber-900)', backgroundColor: 'var(--color-amber-100)' }}
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
            className="cg-focus h-10 rounded-md bg-gray-1000 px-5 text-sm font-medium text-background-100 transition-colors hover:bg-gray-900"
          >
            Try Again
          </button>
          <button
            onClick={() => setPhase('idle')}
            className="cg-focus h-10 rounded-md border border-gray-alpha-200 bg-background-100 px-5 text-sm font-medium text-gray-900 transition-colors hover:border-gray-alpha-400 hover:text-gray-1000"
          >
            Back
          </button>
        </div>
      </div>
    );
  }

  // ── Active state: question display ───────────────────────────────────────
  return (
    <div className="mx-auto max-w-lg px-6 py-12">
      {/* Header: question counter + timer + score */}
      <div className="flex items-center justify-between mb-8">
        <span className="text-sm font-medium text-gray-700">
          {questionIndex + 1} of {questions.length}
        </span>
        <div className="flex items-center gap-4">
          {/* Timer display */}
          <span className="font-mono text-sm tabular-nums text-gray-900">{elapsed}s</span>
          {/* Score tally */}
          <span className="text-sm text-gray-700">
            {results.filter((r) => r.correct).length}✓ {results.filter((r) => !r.correct).length}✗
          </span>
        </div>
      </div>

      {/* Progress bar */}
      <div className="mb-8 h-2 rounded-full border border-gray-alpha-200 bg-gray-100">
        <motion.div
          className="h-full rounded-full bg-gray-1000"
          animate={{ width: `${((questionIndex + 1) / questions.length) * 100}%` }}
          transition={{ type: 'spring', stiffness: 200, damping: 26 }}
        />
      </div>

      {/* Question text */}
      <h2 className="mb-6 text-2xl font-semibold leading-8 tracking-[-0.96px] text-gray-1000">{currentQ.text}</h2>

      {/* Option buttons — radio-style selection (same behavior as QuestionModal) */}
      <div className="flex flex-col gap-3 mb-8">
        {currentQ.options.map((option, i) => {
          const isSelected = selectedIndex === i;
          const isCorrect = i === currentQ.correctIndex;

          // Before confirm: radio highlight on the selected option only.
          // After confirm: moss (correct) and rust (wrong) feedback states.
          const baseStyle = 'border-gray-alpha-200 bg-background-100 text-gray-900 hover:border-gray-alpha-400 hover:bg-gray-100';
          const selectedStyle = 'border-blue-400 bg-blue-100 text-gray-1000 font-medium';
          const mutedStyle = 'border-gray-alpha-200 bg-background-100 text-gray-700';

          let feedbackStyle: React.CSSProperties = {};
          let classes = baseStyle;

          if (confirmed) {
            if (isCorrect) {
              classes = mutedStyle; // base, override with inline
              feedbackStyle = {
                borderColor: 'var(--color-green-400)',
                backgroundColor: 'var(--color-green-100)',
                color: 'var(--color-green-900)',
              };
            } else if (isSelected) {
              classes = mutedStyle;
              feedbackStyle = {
                borderColor: 'var(--color-red-400)',
                backgroundColor: 'var(--color-red-100)',
                color: 'var(--color-red-900)',
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
              className={`w-full rounded-xl border px-4 py-3 text-left text-sm transition-all duration-150 ${classes}`}
            >
              <div className="flex items-center gap-3">
                {/* Radio circle — filled for selected option and correct answer after confirm */}
                <div
                  className="w-4 h-4 rounded-full border-2 shrink-0 flex items-center justify-center transition-colors"
                  style={
                    confirmed
                      ? {
                          borderColor: isCorrect
                            ? 'var(--color-green-700)'
                            : isSelected
                              ? 'var(--color-red-800)'
                              : undefined,
                        }
                      : { borderColor: isSelected ? 'var(--color-blue-700)' : undefined }
                  }
                >
                  {(isSelected || (confirmed && isCorrect)) && (
                    <div
                      className="w-2 h-2 rounded-full"
                      style={{
                        backgroundColor: confirmed
                          ? (isCorrect ? 'var(--color-green-700)' : 'var(--color-red-800)')
                          : 'var(--color-blue-700)',
                      }}
                    />
                  )}
                </div>
                {option}
                {/* Small label after confirming */}
                {confirmed && isCorrect && (
                  <span className="ml-auto text-xs font-semibold" style={{ color: 'var(--color-green-900)' }}>✓ Correct</span>
                )}
                {confirmed && isSelected && !isCorrect && (
                  <span className="ml-auto text-xs font-semibold" style={{ color: 'var(--color-red-900)' }}>✗ Wrong</span>
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
          className="flex items-center gap-1.5 text-sm text-gray-900 transition-colors hover:text-gray-1000 disabled:cursor-not-allowed disabled:text-gray-700"
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
            className="cg-focus h-10 rounded-md bg-gray-1000 px-5 text-sm font-medium text-background-100 transition-colors hover:bg-gray-900"
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
            className="cg-focus h-10 rounded-md bg-blue-700 px-5 text-sm font-medium text-white transition-colors hover:bg-blue-800"
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
