import { useState, useEffect, useRef } from 'react';
import {
  CheckCircle2Icon,
  CircleAlertIcon,
  CircleHelpIcon,
  LoaderCircleIcon,
  LogOutIcon,
  SkipForwardIcon,
} from 'lucide-react';
import { AnimatePresence, motion } from 'motion/react';
import { Navigate, useLocation, useNavigate, useSearchParams } from 'react-router-dom';

import { HelpFlashcard } from './HelpFlashcard';
import { api } from '../../shared/api/client';
import type { NewPracticeConfig, PracticeSession } from '../../shared/api/types';
import { WorkspacePage } from '../../shared/components/WorkspacePage';
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

/** Result for a single answered question. Concept is denormalized so the
 *  aggregate results screen works across rounds with fresh question sets. */
interface QuestionResult {
  questionId: string;
  concept: string;
  round: number;
  selectedIndex: number;
  correct: boolean;
  timeMs: number;
  usedHelp: boolean;
}

/** A skipped question is persisted for resume/history without becoming an
 *  answer result or a correctness signal for memory. */
interface SkippedQuestion {
  questionId: string;
  concept: string;
  round: number;
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

interface MarathonSessionState {
  schema_version: 1;
  prompt: string;
  difficulty: NewPracticeConfig['difficulty'];
  count: number;
  round: number;
  question_index: number;
  elapsed: number;
  selected_index: number | null;
  confirmed: boolean;
  using_fallback: boolean;
  questions: MarathonQuestion[];
  results: QuestionResult[];
  skipped_questions: SkippedQuestion[];
}

interface MarathonLocationState {
  newPractice?: {
    sessionId: string;
    config: NewPracticeConfig;
  };
}

type MemoryUpdateStatus = 'updating' | 'updated' | 'failed' | null;

interface PendingMemoryEvent {
  write: () => Promise<void>;
  promise: Promise<void>;
}

const DEFAULT_CONFIG: NewPracticeConfig = {
  prompt: '',
  difficulty: 'medium',
  count: 5,
};

function normalizeMarathonSessionState(value: unknown): MarathonSessionState | null {
  if (!value || typeof value !== 'object') return null;
  const candidate = value as Partial<MarathonSessionState>;
  if (
    candidate.schema_version !== 1 ||
    typeof candidate.prompt !== 'string' ||
    (candidate.difficulty !== 'easy' &&
      candidate.difficulty !== 'medium' &&
      candidate.difficulty !== 'hard') ||
    typeof candidate.count !== 'number' ||
    typeof candidate.round !== 'number' ||
    typeof candidate.question_index !== 'number' ||
    typeof candidate.elapsed !== 'number' ||
    !Array.isArray(candidate.results)
  ) {
    return null;
  }

  return {
    schema_version: 1,
    prompt: candidate.prompt,
    difficulty: candidate.difficulty,
    count: candidate.count,
    round: candidate.round,
    question_index: candidate.question_index,
    elapsed: candidate.elapsed,
    selected_index:
      candidate.selected_index === null || typeof candidate.selected_index === 'number'
        ? candidate.selected_index
        : null,
    confirmed: candidate.confirmed ?? false,
    using_fallback: candidate.using_fallback ?? false,
    questions: Array.isArray(candidate.questions) ? candidate.questions : [],
    results: candidate.results,
    skipped_questions: Array.isArray(candidate.skipped_questions)
      ? candidate.skipped_questions
      : [],
  };
}

/** Memory event writer for the mcq source. Product interactions queue these
 *  without blocking; round completion flushes the queue before maintenance. */
function emitMcqEvent(
  type: string,
  summary: string,
  payload: Record<string, unknown>,
  occurredAt: string,
) {
  return api
    .post('/memory/events', {
      source: 'mcq',
      type,
      summary,
      payload: { ...payload, schema_version: 1 },
      occurred_at: occurredAt,
    })
    .then(() => undefined);
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

function MemoryUpdateToast({ status }: { status: MemoryUpdateStatus }) {
  const content =
    status === 'updating'
      ? {
          label: 'Updating memory',
          description: 'Saving this question set before continuing.',
          icon: <LoaderCircleIcon className="text-muted-foreground size-4 animate-spin" />,
        }
      : status === 'updated'
        ? {
            label: 'Memory updated',
            description: 'The next question set will use your latest progress.',
            icon: <CheckCircle2Icon className="size-4 text-green-700" />,
          }
        : status === 'failed'
          ? {
              label: 'Memory update failed',
              description: 'Retry before creating the next question set.',
              icon: <CircleAlertIcon className="text-destructive size-4" />,
            }
          : null;

  return (
    <AnimatePresence>
      {content && (
        <motion.div
          role="status"
          aria-live="polite"
          initial={{ opacity: 0, y: -8, scale: 0.98 }}
          animate={{ opacity: 1, y: 0, scale: 1 }}
          exit={{ opacity: 0, y: -6, scale: 0.98 }}
          transition={{ duration: 0.18 }}
          className="bg-popover text-popover-foreground fixed top-4 right-4 z-70 flex w-[min(22rem,calc(100vw-2rem))] items-start gap-3 rounded-lg border px-4 py-3 shadow-lg"
        >
          <span className="mt-0.5 shrink-0">{content.icon}</span>
          <span className="min-w-0">
            <span className="block text-sm font-medium">{content.label}</span>
            <span className="text-muted-foreground mt-0.5 block text-xs leading-4">
              {content.description}
            </span>
          </span>
        </motion.div>
      )}
    </AnimatePresence>
  );
}

// ── Component ────────────────────────────────────────────────────────────────

/**
 * MarathonPage — timed multiple-choice question marathon.
 *
 * Entry is only via New Practice launch state or `?session=` resume.
 * Bare `/marathon` redirects to `/generate`.
 *
 * States:
 * - loading: generating a personalized set via POST /api/v1/generate
 * - active: question display with timer + options + help button
 * - results: score summary + time breakdown + recommendations
 */
export function MarathonPage() {
  const location = useLocation();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const launch = (location.state as MarathonLocationState | null)?.newPractice;
  const requestedSessionId = searchParams.get('session') ?? '';

  // Which phase the marathon is in.
  const [phase, setPhase] = useState<'loading' | 'active' | 'results'>('loading');

  // Guard: prevents accidental option selection when Next button unmounts
  // and mouseup lands on an option button underneath.
  const advancingRef = useRef(false);

  // Index of the current question (0-based).
  const [questionIndex, setQuestionIndex] = useState(0);

  // Accumulated results for each answered question.
  const [results, setResults] = useState<QuestionResult[]>([]);

  // Skips are continuity data only and never count as correct/incorrect answers.
  const [skippedQuestions, setSkippedQuestions] = useState<SkippedQuestion[]>([]);

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

  // The question set for the CURRENT round: generated when the backend +
  // GenAI are available, otherwise the built-in practice set.
  const [questions, setQuestions] = useState<MarathonQuestion[]>(MOCK_QUESTIONS);

  // True when this round fell back to the built-in practice set.
  const [usingFallback, setUsingFallback] = useState(false);

  const [sessionError, setSessionError] = useState<string | null>(null);
  const [isExiting, setIsExiting] = useState(false);
  const [isFinishing, setIsFinishing] = useState(false);
  const [memoryUpdateStatus, setMemoryUpdateStatus] = useState<MemoryUpdateStatus>(null);

  // The user's free-text "what do you want to study?" ask; inserted into the
  // MCQ generation spec each round.
  const [studyPrompt, setStudyPrompt] = useState(launch?.config.prompt ?? '');

  const [difficulty, setDifficulty] = useState<NewPracticeConfig['difficulty']>(
    launch?.config.difficulty ?? DEFAULT_CONFIG.difficulty,
  );

  const [questionCount, setQuestionCount] = useState(launch?.config.count ?? DEFAULT_CONFIG.count);

  // 1-based round number in the continuous marathon loop.
  const [round, setRound] = useState(1);

  // Base id for the marathon run; each round derives `${base}_r${round}` so
  // the post-round reflection digests exactly one round of events.
  const baseIdRef = useRef('');

  // Durable practice session id used by the resume/history APIs.
  const practiceSessionIdRef = useRef(launch?.sessionId ?? requestedSessionId);

  // StrictMode-safe guard for the one-shot launch/resume effect.
  const initializedRef = useRef(false);

  // Serialize session writes so an awaited Exit save cannot be overtaken by an
  // older timer/selection write already in flight.
  const saveQueueRef = useRef<Promise<void>>(Promise.resolve());

  const memoryToastTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Answer/start events remain non-blocking during a question, but every write
  // must settle successfully before the round-level memory refresh can run.
  const pendingMemoryEventsRef = useRef<PendingMemoryEvent[]>([]);
  const completionEventRoundsRef = useRef(new Set<number>());
  const exitEventQueuedRef = useRef(false);

  // Session id for the current round's memory events.
  const sessionIdRef = useRef('');

  const currentQ = questions[questionIndex];

  const showMemoryUpdateStatus = (status: MemoryUpdateStatus) => {
    if (memoryToastTimerRef.current) clearTimeout(memoryToastTimerRef.current);
    setMemoryUpdateStatus(status);
    if (status === 'updated' || status === 'failed') {
      memoryToastTimerRef.current = setTimeout(
        () => setMemoryUpdateStatus(null),
        status === 'updated' ? 3000 : 5000,
      );
    }
  };

  const trackMcqEvent = (type: string, summary: string, payload: Record<string, unknown>) => {
    const occurredAt = new Date().toISOString();
    const write = () => emitMcqEvent(type, summary, payload, occurredAt);
    const event: PendingMemoryEvent = { write, promise: write() };
    pendingMemoryEventsRef.current.push(event);
    void event.promise.catch(() => {});
  };

  const flushPendingMemoryEvents = async () => {
    const pending = [...pendingMemoryEventsRef.current];
    const outcomes = await Promise.allSettled(pending.map((event) => event.promise));
    const failed = pending.filter((_, index) => outcomes[index].status === 'rejected');
    if (failed.length > 0) {
      for (const event of failed) {
        event.promise = event.write();
        void event.promise.catch(() => {});
      }
      throw new Error('Could not save all question activity. Retry the memory update.');
    }
    pendingMemoryEventsRef.current = pendingMemoryEventsRef.current.filter(
      (event) => !pending.includes(event),
    );
  };

  useEffect(
    () => () => {
      if (memoryToastTimerRef.current) clearTimeout(memoryToastTimerRef.current);
    },
    [],
  );

  // ── Timer logic ──────────────────────────────────────────────────────────
  useEffect(() => {
    // Timer runs only while the question is active and not yet confirmed.
    if (phase !== 'active' || confirmed) return;
    const interval = setInterval(() => setElapsed((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, [phase, confirmed]);

  /** Generate one round's question set; falls back to the built-in practice
   *  set when generation is unavailable. */
  const generateRound = async (
    roundNumber: number,
    config: NewPracticeConfig = { prompt: studyPrompt, difficulty, count: questionCount },
  ) => {
    let nextQuestions = MOCK_QUESTIONS;
    let fallback = true;
    try {
      const generated = await api.post<GenerateMcqResponse>('/generate', {
        kind: 'mcq',
        spec: {
          topic: '',
          prompt: config.prompt.trim(),
          count: config.count,
          difficulty: config.difficulty,
          round: roundNumber,
        },
      });
      if (generated.questions?.length) {
        nextQuestions = generated.questions;
        fallback = false;
      }
    } catch (err) {
      console.warn('MCQ generation unavailable; using the built-in practice set.', err);
    }
    return { nextQuestions, fallback };
  };

  /** Enter a round: reset per-question state and emit session_started. */
  const beginRound = (roundNumber: number, nextQuestions: MarathonQuestion[], fallback: boolean) => {
    sessionIdRef.current = `${baseIdRef.current}_r${roundNumber}`;
    setRound(roundNumber);
    setQuestions(nextQuestions);
    setUsingFallback(fallback);
    setQuestionIndex(0);
    setElapsed(0);
    setSelectedIndex(null);
    setConfirmed(false);
    setHelpUsed(false);
    setShowHelp(false);
    trackMcqEvent('session_started', `Started round ${roundNumber} of an MCQ marathon.`, {
      session_id: sessionIdRef.current,
      question_count: nextQuestions.length,
      generated: !fallback,
      round: roundNumber,
    });
    setPhase('active');
  };

  const buildSnapshot = (
    overrides: Partial<MarathonSessionState> = {},
  ): MarathonSessionState => ({
    schema_version: 1,
    prompt: studyPrompt,
    difficulty,
    count: questionCount,
    round,
    question_index: questionIndex,
    elapsed,
    selected_index: selectedIndex,
    confirmed,
    using_fallback: usingFallback,
    questions,
    results,
    skipped_questions: skippedQuestions,
    ...overrides,
  });

  const persistSession = (
    overrides: Partial<MarathonSessionState> = {},
    status?: 'active' | 'completed' | 'abandoned',
  ): Promise<void> => {
    const sessionId = practiceSessionIdRef.current;
    if (!sessionId) return Promise.resolve();
    const state = buildSnapshot(overrides);
    const request = saveQueueRef.current.then(() =>
      api.patch<PracticeSession>(`/sessions/${sessionId}`, {
        ...(status ? { status } : {}),
        state,
      }),
    );
    const completion = request.then(() => undefined);
    saveQueueRef.current = completion.catch(() => undefined);
    return completion;
  };

  const ensurePracticeSession = async (config: NewPracticeConfig) => {
    if (practiceSessionIdRef.current) return practiceSessionIdRef.current;
    const session = await api.post<PracticeSession>('/sessions', {
      kind: 'mcq',
      title: config.prompt.trim() || 'Personalized MCQ practice',
      state: {
        schema_version: 1,
        ...config,
        round: 1,
        question_index: 0,
        elapsed: 0,
        selected_index: null,
        confirmed: false,
        using_fallback: false,
        questions: [],
        results: [],
        skipped_questions: [],
      },
    });
    practiceSessionIdRef.current = session.id;
    return session.id;
  };

  /** Record the just-finished round, then await full profile synthesis so the
   *  next round reads the updated summary, skills, focus areas, and notes. */
  const reflectOnRound = async () => {
    const roundResults = results.filter((r) => r.round === round);
    const roundSkips = skippedQuestions.filter((item) => item.round === round);
    const correctCount = roundResults.filter((r) => r.correct).length;
    showMemoryUpdateStatus('updating');
    try {
      if (!completionEventRoundsRef.current.has(round)) {
        trackMcqEvent(
          'session_completed',
          `Finished round ${round} with ${correctCount} of ${roundResults.length} answered correctly and ${roundSkips.length} skipped.`,
          {
          session_id: sessionIdRef.current,
          question_count: roundResults.length + roundSkips.length,
          answered_count: roundResults.length,
          skipped_count: roundSkips.length,
          correct_count: correctCount,
          round,
          },
        );
        completionEventRoundsRef.current.add(round);
      }
      await flushPendingMemoryEvents();
      await api.post('/memory/profile/maintain', { session_id: sessionIdRef.current });
      showMemoryUpdateStatus('updated');
    } catch (err) {
      showMemoryUpdateStatus('failed');
      throw err;
    }
  };

  /** Start the marathon at round 1. */
  const handleStart = async (config?: NewPracticeConfig) => {
    const nextConfig = config ?? { prompt: studyPrompt, difficulty, count: questionCount };
    setPhase('loading');
    setIsFinishing(false);
    setSessionError(null);
    showMemoryUpdateStatus(null);
    pendingMemoryEventsRef.current = [];
    completionEventRoundsRef.current.clear();
    exitEventQueuedRef.current = false;
    setResults([]);
    setSkippedQuestions([]);
    setStudyPrompt(nextConfig.prompt);
    setDifficulty(nextConfig.difficulty);
    setQuestionCount(nextConfig.count);
    try {
      const durableSessionId = await ensurePracticeSession(nextConfig);
      baseIdRef.current = durableSessionId;
      setRound(1);
      const { nextQuestions, fallback } = await generateRound(1, nextConfig);
      beginRound(1, nextQuestions, fallback);
      await persistSession({
        prompt: nextConfig.prompt,
        difficulty: nextConfig.difficulty,
        count: nextConfig.count,
        round: 1,
        question_index: 0,
        elapsed: 0,
        selected_index: null,
        confirmed: false,
        using_fallback: fallback,
        questions: nextQuestions,
        results: [],
        skipped_questions: [],
      });
    } catch (err) {
      setSessionError(err instanceof Error ? err.message : 'Could not start the practice session.');
      navigate('/generate', { replace: true });
    }
  };

  useEffect(() => {
    if (initializedRef.current || (!launch && !requestedSessionId)) return;
    initializedRef.current = true;

    const initialize = async () => {
      if (launch) {
        await handleStart(launch.config);
        return;
      }

      try {
        const session = await api.get<PracticeSession>(`/sessions/${requestedSessionId}`);
        practiceSessionIdRef.current = session.id;
        baseIdRef.current = session.id;
        const snapshot = normalizeMarathonSessionState(session.state);
        if (session.status !== 'active') {
          setPhase('results');
          if (snapshot) {
            setStudyPrompt(snapshot.prompt);
            setDifficulty(snapshot.difficulty);
            setQuestionCount(snapshot.count);
            setRound(snapshot.round);
            setQuestions(snapshot.questions.length > 0 ? snapshot.questions : MOCK_QUESTIONS);
            setResults(snapshot.results);
            setSkippedQuestions(snapshot.skipped_questions);
          }
          return;
        }

        if (snapshot && snapshot.questions.length > 0) {
          setStudyPrompt(snapshot.prompt);
          setDifficulty(snapshot.difficulty);
          setQuestionCount(snapshot.count);
          setRound(snapshot.round);
          setQuestionIndex(Math.min(snapshot.question_index, snapshot.questions.length - 1));
          setElapsed(snapshot.elapsed);
          setSelectedIndex(snapshot.selected_index);
          setConfirmed(snapshot.confirmed);
          setUsingFallback(snapshot.using_fallback);
          setQuestions(snapshot.questions);
          setResults(snapshot.results);
          setSkippedQuestions(snapshot.skipped_questions);
          sessionIdRef.current = `${session.id}_r${snapshot.round}`;
          setPhase('active');
          return;
        }

        const fallbackConfig: NewPracticeConfig = snapshot
          ? {
              prompt: snapshot.prompt,
              difficulty: snapshot.difficulty,
              count: snapshot.count,
            }
          : DEFAULT_CONFIG;
        await handleStart(fallbackConfig);
      } catch {
        practiceSessionIdRef.current = '';
        baseIdRef.current = '';
        navigate('/generate', { replace: true });
      }
    };

    void initialize();
    // The launch payload and query id are intentionally consumed once.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  /** Round finished, user wants more: reflect (memory update), then build the
   *  next round from the just-updated notes. */
  const handleNextRound = async () => {
    setIsFinishing(false);
    setPhase('loading');
    setSessionError(null);
    try {
      await reflectOnRound();
      const nextRound = round + 1;
      const { nextQuestions, fallback } = await generateRound(nextRound);
      beginRound(nextRound, nextQuestions, fallback);
      void persistSession({
        round: nextRound,
        question_index: 0,
        elapsed: 0,
        selected_index: null,
        confirmed: false,
        using_fallback: fallback,
        questions: nextQuestions,
      }).catch(() => {});
    } catch (err) {
      setSessionError(
        err instanceof Error
          ? err.message
          : 'Could not update memory. Retry before creating the next question set.',
      );
      setPhase('active');
    }
  };

  /** Finish only after the final round is reflected into memory. */
  const handleFinish = async () => {
    setIsFinishing(true);
    setPhase('loading');
    setSessionError(null);
    try {
      await reflectOnRound();
    } catch (err) {
      setSessionError(
        err instanceof Error
          ? err.message
          : 'Could not update memory. Retry before finishing this run.',
      );
      setIsFinishing(false);
      setPhase('active');
      return;
    }

    try {
      await persistSession({}, 'completed');
      setPhase('results');
    } catch (err) {
      setSessionError(err instanceof Error ? err.message : 'Could not save the completed run.');
      setPhase('active');
    } finally {
      setIsFinishing(false);
    }
  };

  /** User selects an answer option (radio-style, can change before confirming). */
  const handleSelect = (index: number) => {
    if (confirmed || advancingRef.current) return; // locked or transitioning
    setSelectedIndex(index);
    void persistSession({ selected_index: index }).catch(() => {});
  };

  /** User confirms their selection — locks in the answer and shows feedback. */
  const handleConfirm = () => {
    if (selectedIndex === null || confirmed) return;
    setConfirmed(true);
    const result: QuestionResult = {
      questionId: currentQ.id,
      concept: currentQ.concept,
      round,
      selectedIndex,
      correct: selectedIndex === currentQ.correctIndex,
      timeMs: elapsed * 1000,
      usedHelp: helpUsed,
    };
    const nextResults = [...results, result];
    setResults(nextResults);
    void persistSession({
      selected_index: selectedIndex,
      confirmed: true,
      results: nextResults,
    }).catch(() => {});

    // question_answered for correct answers; answer_incorrect feeds growth
    // edges for misses (one event per answer, per the naming guide).
    if (result.correct) {
      trackMcqEvent('question_answered', `Answered a ${currentQ.concept} question correctly.`, {
        session_id: sessionIdRef.current,
        topic: currentQ.concept,
        correct: true,
        duration_ms: result.timeMs,
        used_help: result.usedHelp,
      });
    } else {
      trackMcqEvent('answer_incorrect', `Missed a ${currentQ.concept} question.`, {
        session_id: sessionIdRef.current,
        topic: currentQ.concept,
        correct: false,
        duration_ms: result.timeMs,
        used_help: result.usedHelp,
      });
    }
  };

  /** Advance without producing answer correctness data. */
  const handleSkip = () => {
    if (confirmed) return;
    const skipped: SkippedQuestion = {
      questionId: currentQ.id,
      concept: currentQ.concept,
      round,
      timeMs: elapsed * 1000,
      usedHelp: helpUsed,
    };
    const nextSkippedQuestions = [...skippedQuestions, skipped];
    setSelectedIndex(null);
    setConfirmed(true);
    setSkippedQuestions(nextSkippedQuestions);
    trackMcqEvent('question_skipped', `Skipped a ${currentQ.concept} question.`, {
      session_id: sessionIdRef.current,
      question_id: currentQ.id,
      topic: currentQ.concept,
      duration_ms: skipped.timeMs,
      used_help: skipped.usedHelp,
      round,
    });
    void persistSession({
      selected_index: null,
      confirmed: true,
      skipped_questions: nextSkippedQuestions,
    }).catch(() => {});
  };

  /** Save the exact current question state before leaving the active run. */
  const handleExit = async () => {
    if (isExiting) return;
    setIsExiting(true);
    setSessionError(null);
    if (!exitEventQueuedRef.current) {
      trackMcqEvent('session_exited', `Exited round ${round} of an MCQ marathon.`, {
        session_id: sessionIdRef.current,
        round,
        question_index: questionIndex,
        duration_ms: elapsed * 1000,
      });
      exitEventQueuedRef.current = true;
    }
    try {
      await persistSession({}, 'active');
      await flushPendingMemoryEvents();
      navigate('/');
    } catch (err) {
      setSessionError(err instanceof Error ? err.message : 'Could not save this run before exiting.');
      setIsExiting(false);
    }
  };

  /** Advance to the next question within the round. The last question's
   *  controls are Next Round / Finished instead (see the footer). */
  const handleNext = () => {
    // Block option clicks until the next frame to prevent the mouseup
    // from the disappearing Next button from selecting an option.
    advancingRef.current = true;
    requestAnimationFrame(() => {
      advancingRef.current = false;
    });

    if (questionIndex < questions.length - 1) {
      const nextQuestionIndex = questionIndex + 1;
      setQuestionIndex(nextQuestionIndex);
      setElapsed(0);
      setSelectedIndex(null);
      setConfirmed(false);
      setShowHelp(false);
      setHelpUsed(false);
      void persistSession({
        question_index: nextQuestionIndex,
        elapsed: 0,
        selected_index: null,
        confirmed: false,
      }).catch(() => {});
    }
  };

  useEffect(() => {
    if (phase !== 'active' || elapsed === 0 || elapsed % 5 !== 0) return;
    void persistSession({ elapsed }).catch(() => {});
    // Persisting every five seconds keeps resume timers close without writing every tick.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [elapsed, phase]);

  /** Open the help flashcard. */
  const handleHelp = () => {
    setShowHelp(true);
    setHelpUsed(true);
  };

  if (!launch && !requestedSessionId) {
    return <Navigate to="/generate" replace />;
  }

  // ── Loading state: memory reflection + next round generation ─────────────
  if (phase === 'loading') {
    const firstRound = round === 1 && results.length === 0;
    return (
      <WorkspacePage>
        <MemoryUpdateToast status={memoryUpdateStatus} />
        <Card className="mx-auto max-w-xl gap-0 px-6 py-8 text-center">
          <div className="mx-auto mb-6 flex size-10 items-center justify-center">
            <Spinner className="text-muted-foreground size-8" />
          </div>
          <h1 className="mb-2 text-xl font-semibold">
            {firstRound ? 'Building your set' : 'Updating memory'}
          </h1>
          <p className="text-muted-foreground mx-auto max-w-md text-sm leading-5">
            {firstRound
              ? studyPrompt.trim()
                ? `Writing ${questionCount} questions on “${studyPrompt.trim()}”…`
                : `Picking ${questionCount} questions from your growth edges…`
              : isFinishing
                ? `Reflecting on round ${round} before showing your results…`
              : `Reflecting on round ${round}, then building round ${round + 1} from the updated notes…`}
          </p>
        </Card>
      </WorkspacePage>
    );
  }

  // ── Results state: summary ───────────────────────────────────────────────
  if (phase === 'results') {
    const correct = results.filter((r) => r.correct).length;
    const skipped = skippedQuestions.length;
    const totalTime = results.reduce((sum, r) => sum + r.timeMs, 0);
    const avgTime = results.length > 0 ? Math.round(totalTime / results.length / 1000) : 0;

    return (
      <div className="mx-auto max-w-xl px-4 py-8 sm:px-6">
        <MemoryUpdateToast status={memoryUpdateStatus} />
        <h1 className="mb-1 text-center text-[28px] leading-9 font-semibold">
          Results
        </h1>
        <p className="text-muted-foreground mb-8 text-center text-sm">
          {round} {round === 1 ? 'round' : 'rounds'}
          {studyPrompt.trim() ? ` · “${studyPrompt.trim()}”` : ''} · memory notes updated
        </p>

        <motion.div
          initial={{ opacity: 0, y: 20, scale: 0.96 }}
          animate={{ opacity: 1, y: 0, scale: 1 }}
          transition={{ type: 'spring', stiffness: 320, damping: 26 }}
          className="mb-6"
        >
          <Card className="gap-0 p-6">
            <div className="mb-4 flex items-center justify-between">
              <div>
                <div className="text-[32px] leading-10 font-semibold tabular-nums">
                  {correct}
                  <span className="text-muted-foreground text-3xl">/{results.length}</span>
                </div>
                <div className="text-muted-foreground mt-2 text-sm">correct answers</div>
                {skipped > 0 && (
                  <div className="text-muted-foreground mt-1 text-xs">
                    {skipped} {skipped === 1 ? 'question' : 'questions'} skipped
                  </div>
                )}
              </div>
              <div className="text-right">
                <div className="text-[32px] leading-10 font-semibold tabular-nums">
                  {avgTime}s
                </div>
                <div className="text-muted-foreground mt-2 text-sm">avg per question</div>
              </div>
            </div>

            <div className="mt-4 flex flex-col gap-2 border-t pt-4">
              {results.map((r, i) => (
                <div key={`${r.round}-${r.questionId}`} className="flex items-center gap-3 text-xs">
                  <span className="text-muted-foreground w-4">{i + 1}.</span>
                  <span
                    className={cn(
                      'flex size-4 items-center justify-center rounded-full text-[10px] font-bold text-white',
                      r.correct ? 'bg-green-700' : 'bg-red-800',
                    )}
                  >
                    {r.correct ? '✓' : '✗'}
                  </span>
                  <span className="text-muted-foreground flex-1 truncate">{r.concept}</span>
                  <span className="text-muted-foreground">{Math.round(r.timeMs / 1000)}s</span>
                  {r.usedHelp && (
                    <span className="rounded-full bg-amber-100 px-2 py-0.5 text-[10px] font-bold text-amber-900">
                      help
                    </span>
                  )}
                </div>
              ))}
              {skippedQuestions.map((item, i) => (
                <div
                  key={`skipped-${item.round}-${item.questionId}`}
                  className="flex items-center gap-3 text-xs"
                >
                  <span className="text-muted-foreground w-4">{results.length + i + 1}.</span>
                  <span className="bg-muted text-muted-foreground flex size-4 items-center justify-center rounded-full text-[10px] font-bold">
                    -
                  </span>
                  <span className="text-muted-foreground flex-1 truncate">{item.concept}</span>
                  <span className="text-muted-foreground">skipped</span>
                </div>
              ))}
            </div>
          </Card>
        </motion.div>

        <div className="flex justify-center gap-3">
          <Button
            onClick={() => {
              practiceSessionIdRef.current = '';
              void handleStart();
            }}
          >
            Try Again
          </Button>
          <Button variant="outline" onClick={() => navigate('/generate')}>
            New practice
          </Button>
        </div>
      </div>
    );
  }

  // ── Active state: question display ───────────────────────────────────────
  return (
    <div className="mx-auto max-w-xl px-4 py-8 sm:px-6">
      <MemoryUpdateToast status={memoryUpdateStatus} />
      <div className="mb-5 flex flex-wrap items-center justify-between gap-3">
        <span className="text-muted-foreground text-sm font-medium">
          Round {round} · {questionIndex + 1} of {questions.length}
          {usingFallback && (
            <span className="text-muted-foreground/70 ml-2 rounded-md border px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide">
              practice set
            </span>
          )}
        </span>
        <div className="flex items-center gap-3">
          <span className="text-muted-foreground font-mono text-sm tabular-nums">{elapsed}s</span>
          <span className="text-muted-foreground text-sm">
            {results.filter((r) => r.correct).length}✓ {results.filter((r) => !r.correct).length}✗
            {skippedQuestions.length > 0 && ` · ${skippedQuestions.length} skipped`}
          </span>
          <Button
            variant="outline"
            size="sm"
            onClick={() => void handleExit()}
            disabled={isExiting}
          >
            <LogOutIcon />
            {isExiting ? 'Saving' : 'Exit'}
          </Button>
        </div>
      </div>

      {sessionError && (
        <div
          className="border-destructive/40 bg-destructive/5 text-destructive mb-4 rounded-md border px-3 py-2 text-sm"
          role="alert"
        >
          {sessionError}
        </div>
      )}

      <Progress value={((questionIndex + 1) / questions.length) * 100} className="mb-6" />

      <h2 className="mb-5 text-xl leading-7 font-semibold">{currentQ.text}</h2>

      {/* Option buttons — radio-style selection */}
      <div className="mb-6 flex flex-col gap-2.5">
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

      {/* Footer: Help/Skip + Confirm/Next */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-1">
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
          <Button
            variant="ghost"
            size="sm"
            onClick={handleSkip}
            disabled={confirmed}
            className="text-muted-foreground hover:text-foreground"
          >
            <SkipForwardIcon />
            Skip
          </Button>
        </div>

        {!confirmed && selectedIndex !== null && (
          <motion.div initial={{ opacity: 0, y: 6 }} animate={{ opacity: 1, y: 0 }}>
            <Button onClick={handleConfirm}>Confirm</Button>
          </motion.div>
        )}
        {confirmed &&
          (questionIndex < questions.length - 1 ? (
            <motion.div initial={{ opacity: 0, y: 6 }} animate={{ opacity: 1, y: 0 }}>
              <Button onClick={handleNext}>
                Next
              </Button>
            </motion.div>
          ) : (
            <motion.div
              initial={{ opacity: 0, y: 6 }}
              animate={{ opacity: 1, y: 0 }}
              className="flex items-center gap-3"
            >
              <Button variant="outline" onClick={() => void handleFinish()}>
                Finished
              </Button>
              <Button onClick={handleNextRound}>
                Next Round
              </Button>
            </motion.div>
          ))}
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
