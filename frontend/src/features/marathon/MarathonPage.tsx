import { useState, useEffect, useRef } from 'react';
import { CircleHelpIcon } from 'lucide-react';
import { motion } from 'motion/react';

import { HelpFlashcard } from './HelpFlashcard';
import { api } from '../../shared/api/client';
import { Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import { Progress } from '@/components/ui/progress';
import { Spinner } from '@/components/ui/spinner';
import { cn } from '@/lib/utils';

// ── Types ────────────────────────────────────────────────────────────────────

/** A single multiple-choice question in the marathon. Matches the backend
 *  MCQQuestion shape returned by POST /api/v1/generate. */
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

/** Response envelope data for POST /api/v1/generate with kind "mcq". */
interface GenerateMcqResponse {
  kind: string;
  questions: MarathonQuestion[];
  provider: string;
  model: string;
}

/** How many questions to request per marathon. */
const QUESTION_COUNT = 5;

/** Fire-and-forget memory event emitter for the mcq source. Event writes must
 *  never block or break the practice flow, so failures are swallowed. Names
 *  follow docs/memory-event-naming-guide-v0.md. */
function emitMcqEvent(type: string, summary: string, payload: Record<string, unknown>) {
  void api
    .post('/memory/events', {
      source: 'mcq',
      type,
      summary,
      payload: { ...payload, schema_version: 1 },
    })
    .catch(() => {});
}

// ── Mock data ────────────────────────────────────────────────────────────────

/** Fallback questions when generation is unavailable (no backend, no GenAI key,
 *  or a provider error). The real set comes from POST /api/v1/generate, which
 *  reads the user's memory profile. */
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

// ── Success check (transitions-dev 10) ───────────────────────────────────────

/** Animated checkmark that draws itself in when a correct answer is confirmed. */
function SuccessCheck() {
  return (
    <svg
      className="t-check size-4 text-green-700"
      style={{ ['--t-check-len' as string]: 23 } as React.CSSProperties}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="3"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M20 6 9 17l-5-5" />
    </svg>
  );
}

// ── Component ────────────────────────────────────────────────────────────────

/**
 * MarathonPage — timed multiple-choice question marathon.
 *
 * Four states:
 * - idle: start screen (select topic, see previous scores)
 * - loading: generating a personalized set via POST /api/v1/generate
 * - active: question display with timer + options + help button
 * - results: score summary + time breakdown + recommendations
 */
export function MarathonPage() {
  // Which phase the marathon is in.
  const [phase, setPhase] = useState<'idle' | 'loading' | 'active' | 'results'>('idle');

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

  // The question set for this run: generated when the backend + GenAI are
  // available, otherwise the built-in practice set.
  const [questions, setQuestions] = useState<MarathonQuestion[]>(MOCK_QUESTIONS);

  // True when this run fell back to the built-in practice set.
  const [usingFallback, setUsingFallback] = useState(false);

  // Client-side id tying this run's memory events together.
  const sessionIdRef = useRef('');

  const currentQ = questions[questionIndex];

  // ── Timer logic ──────────────────────────────────────────────────────────
  useEffect(() => {
    // Timer runs only while the question is active and not yet confirmed.
    if (phase !== 'active' || confirmed) return;
    const interval = setInterval(() => setElapsed((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, [phase, confirmed]);

  /** Start the marathon: generate a personalized set, falling back to the
   *  built-in practice set when generation is unavailable. */
  const handleStart = async () => {
    setPhase('loading');
    setQuestionIndex(0);
    setResults([]);
    setElapsed(0);
    setSelectedIndex(null);
    setConfirmed(false);
    setHelpUsed(false);
    setShowHelp(false);

    let nextQuestions = MOCK_QUESTIONS;
    let fallback = true;
    try {
      const generated = await api.post<GenerateMcqResponse>('/generate', {
        kind: 'mcq',
        spec: { topic: '', count: QUESTION_COUNT },
      });
      if (generated.questions?.length) {
        nextQuestions = generated.questions;
        fallback = false;
      }
    } catch (err) {
      console.warn('MCQ generation unavailable; using the built-in practice set.', err);
    }

    sessionIdRef.current = `mcq_${Date.now().toString(36)}`;
    setQuestions(nextQuestions);
    setUsingFallback(fallback);
    emitMcqEvent('session_started', `Started a ${nextQuestions.length}-question MCQ marathon.`, {
      session_id: sessionIdRef.current,
      question_count: nextQuestions.length,
      generated: !fallback,
    });
    setPhase('active');
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

    // question_answered for correct answers; answer_incorrect feeds growth
    // edges for misses (one event per answer, per the naming guide).
    if (result.correct) {
      emitMcqEvent('question_answered', `Answered a ${currentQ.concept} question correctly.`, {
        session_id: sessionIdRef.current,
        topic: currentQ.concept,
        correct: true,
        duration_ms: result.timeMs,
        used_help: result.usedHelp,
      });
    } else {
      emitMcqEvent('answer_incorrect', `Missed a ${currentQ.concept} question.`, {
        session_id: sessionIdRef.current,
        topic: currentQ.concept,
        correct: false,
        duration_ms: result.timeMs,
        used_help: result.usedHelp,
      });
    }
  };

  /** Advance to the next question or show results. */
  const handleNext = () => {
    // Block option clicks until the next frame to prevent the mouseup
    // from the disappearing Next button from selecting an option.
    advancingRef.current = true;
    requestAnimationFrame(() => {
      advancingRef.current = false;
    });

    if (questionIndex < questions.length - 1) {
      setQuestionIndex((i) => i + 1);
      setElapsed(0);
      setSelectedIndex(null);
      setConfirmed(false);
      setShowHelp(false);
      setHelpUsed(false);
    } else {
      const correctCount = results.filter((r) => r.correct).length;
      emitMcqEvent(
        'session_completed',
        `Finished a ${results.length}-question MCQ marathon with ${correctCount} correct.`,
        {
          session_id: sessionIdRef.current,
          question_count: results.length,
          correct_count: correctCount,
        },
      );
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
      <div className="mx-auto mt-12 max-w-lg">
        <Card className="gap-0 px-6 py-16 text-center">
          <h1 className="mb-4 text-[48px] font-semibold leading-[56px] tracking-[-2.88px]">
            MCQ Marathon
          </h1>
          <p className="text-muted-foreground mx-auto mb-10 max-w-sm text-sm leading-6">
            Timed multiple-choice reps. The AI adapts by reinforcing what you know, stretching where
            you don't.
          </p>
          <motion.div whileHover={{ scale: 1.02 }} whileTap={{ scale: 0.97 }} className="inline-block">
            <Button size="lg" onClick={handleStart}>
              Start Marathon ({QUESTION_COUNT} questions)
            </Button>
          </motion.div>
        </Card>
      </div>
    );
  }

  // ── Loading state: generating a personalized set ─────────────────────────
  if (phase === 'loading') {
    return (
      <div className="mx-auto mt-12 max-w-lg">
        <Card className="gap-0 px-6 py-16 text-center">
          <div className="mx-auto mb-6 flex size-10 items-center justify-center">
            <Spinner className="text-muted-foreground size-8" />
          </div>
          <h1 className="mb-3 text-2xl font-semibold tracking-tight">Building your set</h1>
          <p className="text-muted-foreground mx-auto max-w-sm text-sm leading-6">
            Picking {QUESTION_COUNT} questions from your growth edges…
          </p>
        </Card>
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
        <h1 className="mb-8 text-center text-[40px] font-semibold leading-[48px] tracking-[-2.4px]">
          Results
        </h1>

        <motion.div
          initial={{ opacity: 0, y: 20, scale: 0.96 }}
          animate={{ opacity: 1, y: 0, scale: 1 }}
          transition={{ type: 'spring', stiffness: 320, damping: 26 }}
          className="mb-6"
        >
          <Card className="gap-0 p-6">
            <div className="mb-4 flex items-center justify-between">
              <div>
                <div className="text-[48px] font-semibold leading-[56px] tracking-[-2.88px]">
                  {correct}
                  <span className="text-muted-foreground text-3xl">/{results.length}</span>
                </div>
                <div className="text-muted-foreground mt-2 text-sm">correct answers</div>
              </div>
              <div className="text-right">
                <div className="text-[48px] font-semibold leading-[56px] tracking-[-2.88px] text-blue-700">
                  {avgTime}s
                </div>
                <div className="text-muted-foreground mt-2 text-sm">avg per question</div>
              </div>
            </div>

            <div className="mt-4 flex flex-col gap-2 border-t pt-4">
              {results.map((r, i) => (
                <div key={r.questionId} className="flex items-center gap-3 text-xs">
                  <span className="text-muted-foreground w-4">{i + 1}.</span>
                  <span
                    className={cn(
                      'flex size-4 items-center justify-center rounded-full text-[10px] font-bold text-white',
                      r.correct ? 'bg-green-700' : 'bg-red-800',
                    )}
                  >
                    {r.correct ? '✓' : '✗'}
                  </span>
                  <span className="text-muted-foreground flex-1 truncate">
                    {questions.find((q) => q.id === r.questionId)?.concept}
                  </span>
                  <span className="text-muted-foreground">{Math.round(r.timeMs / 1000)}s</span>
                  {r.usedHelp && (
                    <span className="rounded-full bg-amber-100 px-2 py-0.5 text-[10px] font-bold text-amber-900">
                      help
                    </span>
                  )}
                </div>
              ))}
            </div>
          </Card>
        </motion.div>

        <div className="flex justify-center gap-3">
          <Button onClick={handleStart}>Try Again</Button>
          <Button variant="outline" onClick={() => setPhase('idle')}>
            Back
          </Button>
        </div>
      </div>
    );
  }

  // ── Active state: question display ───────────────────────────────────────
  return (
    <div className="mx-auto max-w-lg px-6 py-12">
      <div className="mb-8 flex items-center justify-between">
        <span className="text-muted-foreground text-sm font-medium">
          {questionIndex + 1} of {questions.length}
          {usingFallback && (
            <span className="text-muted-foreground/70 ml-2 rounded-full border px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide">
              practice set
            </span>
          )}
        </span>
        <div className="flex items-center gap-4">
          <span className="text-muted-foreground font-mono text-sm tabular-nums">{elapsed}s</span>
          <span className="text-muted-foreground text-sm">
            {results.filter((r) => r.correct).length}✓ {results.filter((r) => !r.correct).length}✗
          </span>
        </div>
      </div>

      <Progress value={((questionIndex + 1) / questions.length) * 100} className="mb-8" />

      <h2 className="mb-6 text-2xl font-semibold leading-8 tracking-[-0.96px]">{currentQ.text}</h2>

      {/* Option buttons — radio-style selection */}
      <div className="mb-8 flex flex-col gap-3">
        {currentQ.options.map((option, i) => {
          const isSelected = selectedIndex === i;
          const isCorrect = i === currentQ.correctIndex;

          let stateClasses =
            'border bg-background text-muted-foreground hover:bg-accent/50';
          if (confirmed) {
            if (isCorrect) stateClasses = 'border-green-400 bg-green-100 text-green-900';
            else if (isSelected) stateClasses = 'border-red-400 bg-red-100 text-red-900';
            else stateClasses = 'border bg-background text-muted-foreground';
          } else if (isSelected) {
            stateClasses = 'border-primary bg-accent text-foreground font-medium';
          }

          return (
            <button
              key={i}
              onClick={() => handleSelect(i)}
              disabled={confirmed}
              className={cn(
                'w-full rounded-lg border px-4 py-3 text-left text-sm transition-all duration-150',
                stateClasses,
              )}
            >
              <div className="flex items-center gap-3">
                <div
                  className={cn(
                    'flex size-4 shrink-0 items-center justify-center rounded-full border-2 transition-colors',
                    confirmed
                      ? isCorrect
                        ? 'border-green-700'
                        : isSelected
                          ? 'border-red-800'
                          : 'border-input'
                      : isSelected
                        ? 'border-primary'
                        : 'border-input',
                  )}
                >
                  {(isSelected || (confirmed && isCorrect)) && (
                    <div
                      className={cn(
                        'size-2 rounded-full',
                        confirmed
                          ? isCorrect
                            ? 'bg-green-700'
                            : 'bg-red-800'
                          : 'bg-primary',
                      )}
                    />
                  )}
                </div>
                {option}
                {confirmed && isCorrect && (
                  <span className="ml-auto flex items-center gap-1 text-xs font-semibold text-green-700">
                    <SuccessCheck /> Correct
                  </span>
                )}
                {confirmed && isSelected && !isCorrect && (
                  <span className="ml-auto text-xs font-semibold text-red-900">✗ Wrong</span>
                )}
              </div>
            </button>
          );
        })}
      </div>

      {/* Footer: Help + Confirm/Next */}
      <div className="flex items-center justify-between">
        <Button
          variant="ghost"
          size="sm"
          onClick={handleHelp}
          disabled={confirmed}
          className="text-muted-foreground hover:text-foreground"
        >
          <CircleHelpIcon />
          Help
        </Button>

        {!confirmed && selectedIndex !== null && (
          <motion.div initial={{ opacity: 0, y: 6 }} animate={{ opacity: 1, y: 0 }}>
            <Button onClick={handleConfirm}>Confirm</Button>
          </motion.div>
        )}
        {confirmed && (
          <motion.div initial={{ opacity: 0, y: 6 }} animate={{ opacity: 1, y: 0 }}>
            <Button onClick={handleNext} className="bg-blue-700 text-white hover:bg-blue-800">
              {questionIndex < questions.length - 1 ? 'Next' : 'See Results'}
            </Button>
          </motion.div>
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
