import {
  useEffect,
  useState,
  useCallback,
  useRef,
  type KeyboardEvent as ReactKeyboardEvent,
  type PointerEvent as ReactPointerEvent,
} from 'react';
import { useParams, useSearchParams } from 'react-router-dom';
import Editor from '@monaco-editor/react';
import { api, createMemoryEvent } from '../../shared/api/client';
import { buildWorkspaceEvent, memoryEventTypes } from '../../shared/api/memoryEvents';
import { useCodeGymAuthState } from '../../shared/auth/authState';
import { MarkdownContent } from '../../shared/components/MarkdownContent';
import type {
  PracticeSession,
  PracticeSessionSummary,
  Problem,
  PublicProblemCase,
  Submission,
  SubmissionFile,
  SubmissionMode,
  SubmissionStatus,
  TestCaseResult,
  TestResult,
} from '../../shared/api/types';
import { GridSpinner } from '../../shared/components/GridSpinner';

const languageMap: Record<string, string> = {
  go: 'go',
  javascript: 'javascript',
  typescript: 'typescript',
  python: 'python',
  java: 'java',
  cpp: 'cpp',
  c: 'c',
  rust: 'rust',
  swift: 'swift',
};

const DEFAULT_DESCRIPTION_WIDTH = 45;
const MIN_DESCRIPTION_WIDTH = 28;
const MAX_DESCRIPTION_WIDTH = 62;
const DEFAULT_RESULTS_HEIGHT = 32;
const MIN_RESULTS_HEIGHT = 18;
const MAX_RESULTS_HEIGHT = 65;
const RESIZE_KEY_STEP = 2;
const AUTOSAVE_DELAY_MS = 800;

function clamp(value: number, min: number, max: number) {
  return Math.min(max, Math.max(min, value));
}

function normalizeSessionState(value: unknown): Record<string, unknown> {
  if (value && typeof value === 'object' && !Array.isArray(value)) {
    return { ...(value as Record<string, unknown>) };
  }
  return { schema_version: 1 };
}

function restoredHintCount(state: Record<string, unknown>, availableHints: number) {
  const value = Number(state.hints_revealed ?? 0);
  if (!Number.isFinite(value)) return 0;
  return clamp(Math.floor(value), 0, availableHints);
}

interface ProblemDetailPageProps {
  initialSubmission?: Submission | null;
  initialMemoryUpdateStatus?: 'idle' | 'pending' | 'synced' | 'failed';
}

type TerminalFailureStatus = Extract<
  SubmissionStatus,
  'timeout' | 'out_of_memory' | 'crashed' | 'error'
>;

const terminalFailurePresentation: Record<
  TerminalFailureStatus,
  { label: string; fallback: string; tone: string }
> = {
  timeout: {
    label: 'TIME LIMIT',
    fallback: 'The submission exceeded its wall-clock limit.',
    tone: 'text-[#dcdcaa]',
  },
  out_of_memory: {
    label: 'MEMORY LIMIT',
    fallback: 'The submission exceeded its memory limit.',
    tone: 'text-[#ce9178]',
  },
  crashed: {
    label: 'PROCESS CRASHED',
    fallback: 'The submission process exited before the test harness completed.',
    tone: 'text-[#f14c4c]',
  },
  error: {
    label: 'PLATFORM ERROR',
    fallback: 'CodeGym could not complete this run. Try again.',
    tone: 'text-[#f14c4c]',
  },
};

function isTerminalFailureSubmission(
  submission: Submission,
): submission is Submission & { status: TerminalFailureStatus } {
  return submission.status in terminalFailurePresentation;
}

function submissionModeLabel(mode: SubmissionMode | undefined) {
  return mode === 'run' ? 'SAMPLE RUN' : 'GRADED SUBMIT';
}

function submissionResultSummary(submission: Submission, result?: TestResult) {
  const executedCount = submission.executed_count ?? result?.total ?? 0;
  const caseLabel = executedCount === 1 ? 'case' : 'cases';

  if (!result) {
    return submission.mode === 'run'
      ? `${executedCount} sample ${caseLabel} executed`
      : `${executedCount} ${caseLabel} executed`;
  }

  if (submission.mode === 'run') {
    return result.status === 'pass'
      ? `${executedCount} sample ${caseLabel} passed`
      : `${result.passed} of ${executedCount} sample ${caseLabel} passed`;
  }

  return `${result.passed} of ${executedCount} passed`;
}

function formatExampleValue(value: unknown) {
  if (typeof value === 'string') return value;
  return JSON.stringify(value, null, 2) ?? String(value);
}

function ExampleValue({ label, value }: { label: string; value: unknown }) {
  return (
    <div>
      <dt className="text-xs font-medium text-gray-700">{label}</dt>
      <dd className="mt-1">
        <pre className="overflow-x-auto rounded-md border border-gray-alpha-200 bg-gray-100 px-3 py-2 font-mono text-xs leading-5 text-gray-900">
          {formatExampleValue(value)}
        </pre>
      </dd>
    </div>
  );
}

export function WorkedExamples({ cases }: { cases: PublicProblemCase[] }) {
  if (cases.length === 0) return null;

  return (
    <section className="mt-6 border-t border-gray-alpha-200 pt-4" aria-labelledby="worked-examples-heading">
      <h2 id="worked-examples-heading" className="text-sm font-semibold text-gray-1000">
        Worked examples
      </h2>
      <div className="mt-3 border-y border-gray-alpha-200">
        {cases.map((problemCase, index) => (
          <article
            key={`${problemCase.strategy}-${problemCase.name}-${index}`}
            className="border-b border-gray-alpha-200 py-4 last:border-b-0"
          >
            <h3 className="font-mono text-xs font-semibold text-gray-900">
              Example {index + 1} · {problemCase.name}
            </h3>

            {problemCase.strategy === 'unit' ? (
              <dl className="mt-3 flex flex-col gap-3">
                <ExampleValue label="Arguments" value={problemCase.args} />
                <ExampleValue label="Expected output" value={problemCase.expected} />
              </dl>
            ) : (
              <div className="mt-3 flex flex-col gap-3">
                <div>
                  <p className="text-xs font-medium text-gray-700">Request</p>
                  <div className="mt-1 overflow-x-auto rounded-md border border-gray-alpha-200 bg-gray-100 px-3 py-2 font-mono text-xs leading-5 text-gray-900">
                    <span className="font-semibold">{problemCase.request.method.toUpperCase()}</span>{' '}
                    {problemCase.request.path}
                  </div>
                </div>
                {(problemCase.request.headers || problemCase.request.body !== undefined) && (
                  <dl className="flex flex-col gap-3">
                    {problemCase.request.headers && (
                      <ExampleValue label="Request headers" value={problemCase.request.headers} />
                    )}
                    {problemCase.request.body !== undefined && (
                      <ExampleValue label="Request body" value={problemCase.request.body} />
                    )}
                  </dl>
                )}
                <div>
                  <p className="text-xs font-medium text-gray-700">Expected response</p>
                  <div className="mt-1 rounded-md border border-gray-alpha-200 bg-gray-100 px-3 py-2 font-mono text-xs leading-5 text-gray-900">
                    HTTP {problemCase.expected.status}
                  </div>
                </div>
                {(problemCase.expected.headers ||
                  problemCase.expected.json !== undefined ||
                  problemCase.expected.body !== undefined) && (
                  <dl className="flex flex-col gap-3">
                    {problemCase.expected.headers && (
                      <ExampleValue label="Response headers" value={problemCase.expected.headers} />
                    )}
                    {problemCase.expected.json !== undefined && (
                      <ExampleValue label="Response body" value={problemCase.expected.json} />
                    )}
                    {problemCase.expected.body !== undefined && (
                      <ExampleValue label="Response body" value={problemCase.expected.body} />
                    )}
                  </dl>
                )}
              </div>
            )}

            {problemCase.explanation && (
              <p className="mt-3 text-sm leading-5 text-gray-700">
                <span className="font-semibold text-gray-900">Explanation:</span>{' '}
                {problemCase.explanation}
              </p>
            )}
          </article>
        ))}
      </div>
    </section>
  );
}

function CapturedStdout({ submission }: { submission: Submission }) {
  if (!submission.stdout && !submission.output_truncated) return null;
  return (
    <div className="border-t border-[#333]">
      <div className="flex items-center gap-2 border-b border-[#2a2a2a] px-4 py-1.5">
        <span className="font-mono text-[10px] font-semibold text-[#858585]">STDOUT</span>
        {submission.output_truncated && (
          <span className="font-mono text-[10px] text-[#dcdcaa]">OUTPUT TRUNCATED</span>
        )}
      </div>
      <pre className="max-h-36 overflow-auto whitespace-pre-wrap px-4 py-2 font-mono text-[11px] leading-5 text-[#b8b8b8]">
        {submission.stdout || '(no stdout captured)'}
      </pre>
    </div>
  );
}

export function ProblemDetailPage({
  initialSubmission = null,
  initialMemoryUpdateStatus = 'idle',
}: ProblemDetailPageProps = {}) {
  const { configured: authConfigured, isAuthenticated } = useCodeGymAuthState();
  const apiAuthReady = !authConfigured || isAuthenticated;
  const { id } = useParams<{ id: string }>();
  const [searchParams, setSearchParams] = useSearchParams();
  const requestedSessionId = searchParams.get('session')?.trim() ?? '';
  const pageRef = useRef<HTMLDivElement>(null);
  const rightPaneRef = useRef<HTMLDivElement>(null);
  const filesRef = useRef<SubmissionFile[]>([]);
  const hintsRef = useRef(0);
  const sessionStateRef = useRef<Record<string, unknown>>({ schema_version: 1 });
  const autosaveTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const runRequestRef = useRef(0);
  const submissionInFlightRef = useRef(false);
  const [problem, setProblem] = useState<Problem | null>(null);
  const [files, setFiles] = useState<SubmissionFile[]>([]);
  const [activeFile, setActiveFile] = useState(0);
  const [sessionId, setSessionId] = useState<string | null>(null);
  const [resumedDraft, setResumedDraft] = useState(false);
  const [saveStatus, setSaveStatus] = useState<'idle' | 'saving' | 'saved' | 'error'>('idle');
  const [submitting, setSubmitting] = useState(false);
  const [activeSubmissionMode, setActiveSubmissionMode] = useState<SubmissionMode | null>(null);
  const [submission, setSubmission] = useState<Submission | null>(initialSubmission);
  const [hintsRevealed, setHintsRevealed] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const [descriptionWidth, setDescriptionWidth] = useState(DEFAULT_DESCRIPTION_WIDTH);
  const [resultsHeight, setResultsHeight] = useState(DEFAULT_RESULTS_HEIGHT);
  const [memoryUpdateStatus, setMemoryUpdateStatus] = useState<
    'idle' | 'pending' | 'synced' | 'failed'
  >(initialMemoryUpdateStatus);

  useEffect(() => {
    if (!id) return;
    if (!apiAuthReady) {
      setProblem(null);
      setError('Sign in to open this coding workspace.');
      return;
    }
    let cancelled = false;
    runRequestRef.current += 1;
    setProblem(null);
    setFiles([]);
    filesRef.current = [];
    setSessionId(null);
    setResumedDraft(false);
    setSaveStatus('idle');
    setSubmission(initialSubmission);
    setError(null);
    setActiveFile(0);

    const load = async () => {
      try {
        const [loadedProblem, skeleton, activeSessions] = await Promise.all([
          api.get<Problem>(`/problems/${id}`),
          api.get<{ files: SubmissionFile[] }>(`/problems/${id}/skeleton`),
          api.get<PracticeSessionSummary[]>(
            '/sessions?kind=workspace&status=active&limit=100',
          ),
        ]);

        let workspaceSession: PracticeSession;
        if (requestedSessionId) {
          workspaceSession = await api.get<PracticeSession>(
            `/sessions/${encodeURIComponent(requestedSessionId)}`,
          );
          if (
            workspaceSession.kind !== 'workspace' ||
            workspaceSession.problem_id !== loadedProblem.id
          ) {
            throw new Error('The requested workspace session does not match this problem.');
          }
        } else {
          const matchingSummary = activeSessions.find(
            (session) => session.problem_id === loadedProblem.id,
          );
          workspaceSession = matchingSummary
            ? await api.get<PracticeSession>(`/sessions/${matchingSummary.id}`)
            : await api.post<PracticeSession>('/sessions', {
                kind: 'workspace',
                title: loadedProblem.title,
                problem_id: loadedProblem.id,
                state: { schema_version: 1, hints_revealed: 0 },
              });
        }

        if (cancelled) return;

        const state = normalizeSessionState(workspaceSession.state);
        const restoredFiles =
          workspaceSession.files?.map((file) => ({
            path: file.file_path,
            content: file.content,
          })) ?? [];
        const initialFiles = restoredFiles.length > 0 ? restoredFiles : skeleton.files;
        const initialHintCount = restoredHintCount(
          state,
          loadedProblem.hints?.length ?? 0,
        );
        const restoredMemoryStatus =
          state.memory_update_status === 'pending' ||
          state.memory_update_status === 'synced' ||
          state.memory_update_status === 'failed'
            ? state.memory_update_status
            : 'idle';

        sessionStateRef.current = state;
        filesRef.current = initialFiles;
        hintsRef.current = initialHintCount;
        setProblem(loadedProblem);
        setFiles(initialFiles);
        setHintsRevealed(initialHintCount);
        setSessionId(workspaceSession.id);
        setResumedDraft(restoredFiles.length > 0);
        setSaveStatus(restoredFiles.length > 0 ? 'saved' : 'idle');
        setMemoryUpdateStatus(
          initialMemoryUpdateStatus === 'idle'
            ? restoredMemoryStatus
            : initialMemoryUpdateStatus,
        );
        if (typeof state.last_submission_id === 'string' && !initialSubmission) {
          void api
            .get<Submission>(
              `/submissions/${encodeURIComponent(state.last_submission_id)}`,
            )
            .then((submission) => {
              if (
                !cancelled &&
                submission.status !== 'pending' &&
                submission.status !== 'running'
              ) {
                setSubmission(submission);
              }
            })
            .catch(() => {
              // A missing historical run does not prevent draft resume.
            });
        }
        if (state.problem_opened_recorded !== true) {
          const openedState = { ...state, problem_opened_recorded: true };
          sessionStateRef.current = openedState;
          try {
            await Promise.all([
              createMemoryEvent(
                buildWorkspaceEvent({
                  type: memoryEventTypes.problemOpened,
                  summary: 'Opened a coding problem.',
                  payload: {
                    problem_id: loadedProblem.id,
                    session_id: workspaceSession.id,
                    concept: loadedProblem.subcategory ?? loadedProblem.category,
                    difficulty: loadedProblem.difficulty,
                    language: loadedProblem.language,
                    schema_version: 1,
                  },
                }),
              ),
              api.patch<PracticeSession>(`/sessions/${workspaceSession.id}`, {
                state: openedState,
              }),
            ]);
          } catch {
            sessionStateRef.current = state;
          }
        }
        if (!requestedSessionId) {
          const nextParams = new URLSearchParams(searchParams);
          nextParams.set('session', workspaceSession.id);
          setSearchParams(nextParams, { replace: true });
        }
      } catch (err: unknown) {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : 'Could not load this problem.');
        }
      }
    };

    void load();
    return () => {
      cancelled = true;
      runRequestRef.current += 1;
    };
  }, [
    apiAuthReady,
    id,
    initialMemoryUpdateStatus,
    initialSubmission,
    requestedSessionId,
    searchParams,
    setSearchParams,
  ]);

  const persistDraft = useCallback(
    async (currentFiles: SubmissionFile[], currentHints: number) => {
      if (!sessionId) return;
      setSaveStatus('saving');
      const state = {
        ...sessionStateRef.current,
        schema_version: Number(sessionStateRef.current.schema_version ?? 1),
        hints_revealed: currentHints,
      };
      sessionStateRef.current = state;
      try {
        await Promise.all([
          api.put<PracticeSession>(`/sessions/${sessionId}/files`, {
            files: currentFiles.map((file) => ({
              file_path: file.path,
              content: file.content,
            })),
          }),
          api.patch<PracticeSession>(`/sessions/${sessionId}`, { state }),
        ]);
        setSaveStatus('saved');
      } catch (err) {
        setSaveStatus('error');
        throw err;
      }
    },
    [sessionId],
  );

  useEffect(() => {
    if (!sessionId || files.length === 0) return;
    if (autosaveTimerRef.current) clearTimeout(autosaveTimerRef.current);
    autosaveTimerRef.current = setTimeout(() => {
      void persistDraft(files, hintsRevealed).catch(() => {
        // The compact save indicator communicates autosave failure without
        // replacing the editor with a blocking error state.
      });
    }, AUTOSAVE_DELAY_MS);
    return () => {
      if (autosaveTimerRef.current) clearTimeout(autosaveTimerRef.current);
    };
  }, [files, hintsRevealed, persistDraft, sessionId]);

  const handleCodeChange = useCallback(
    (value: string | undefined) => {
      if (value === undefined) return;
      setFiles((previous) => {
        const next = previous.map((file, index) =>
          index === activeFile ? { ...file, content: value } : file,
        );
        filesRef.current = next;
        return next;
      });
    },
    [activeFile],
  );

  const revealHint = useCallback(
    (count: number) => {
      hintsRef.current = count;
      setHintsRevealed(count);
      if (!problem || !sessionId) return;
      void createMemoryEvent(
        buildWorkspaceEvent({
          type: memoryEventTypes.hintRevealed,
          summary: 'Revealed a coding problem hint.',
          payload: {
            problem_id: problem.id,
            session_id: sessionId,
            concept: problem.subcategory ?? problem.category,
            difficulty: problem.difficulty,
            language: problem.language,
            hint_index: count,
            hint_count: problem.hints?.length ?? 0,
            schema_version: 1,
          },
        }),
      ).catch(() => {
        // Hint access is never blocked by best-effort memory persistence.
      });
    },
    [problem, sessionId],
  );

  const beginHorizontalResize = useCallback((event: ReactPointerEvent<HTMLButtonElement>) => {
    if (!pageRef.current) return;

    event.preventDefault();
    try {
      event.currentTarget.setPointerCapture(event.pointerId);
    } catch {
      // Some browsers can reject capture after synthetic pointer sequences.
    }

    const rect = pageRef.current.getBoundingClientRect();
    const previousCursor = document.body.style.cursor;
    const previousUserSelect = document.body.style.userSelect;
    document.body.style.cursor = 'col-resize';
    document.body.style.userSelect = 'none';

    const handlePointerMove = (moveEvent: PointerEvent) => {
      const nextWidth = ((moveEvent.clientX - rect.left) / rect.width) * 100;
      setDescriptionWidth(clamp(nextWidth, MIN_DESCRIPTION_WIDTH, MAX_DESCRIPTION_WIDTH));
    };

    const stopResize = () => {
      document.body.style.cursor = previousCursor;
      document.body.style.userSelect = previousUserSelect;
      window.removeEventListener('pointermove', handlePointerMove);
      window.removeEventListener('pointerup', stopResize);
      window.removeEventListener('pointercancel', stopResize);
    };

    window.addEventListener('pointermove', handlePointerMove);
    window.addEventListener('pointerup', stopResize);
    window.addEventListener('pointercancel', stopResize);
  }, []);

  const beginVerticalResize = useCallback((event: ReactPointerEvent<HTMLButtonElement>) => {
    if (!rightPaneRef.current) return;

    event.preventDefault();
    try {
      event.currentTarget.setPointerCapture(event.pointerId);
    } catch {
      // Some browsers can reject capture after synthetic pointer sequences.
    }

    const rect = rightPaneRef.current.getBoundingClientRect();
    const previousCursor = document.body.style.cursor;
    const previousUserSelect = document.body.style.userSelect;
    document.body.style.cursor = 'row-resize';
    document.body.style.userSelect = 'none';

    const handlePointerMove = (moveEvent: PointerEvent) => {
      const nextHeight = ((rect.bottom - moveEvent.clientY) / rect.height) * 100;
      setResultsHeight(clamp(nextHeight, MIN_RESULTS_HEIGHT, MAX_RESULTS_HEIGHT));
    };

    const stopResize = () => {
      document.body.style.cursor = previousCursor;
      document.body.style.userSelect = previousUserSelect;
      window.removeEventListener('pointermove', handlePointerMove);
      window.removeEventListener('pointerup', stopResize);
      window.removeEventListener('pointercancel', stopResize);
    };

    window.addEventListener('pointermove', handlePointerMove);
    window.addEventListener('pointerup', stopResize);
    window.addEventListener('pointercancel', stopResize);
  }, []);

  const handleHorizontalResizeKey = useCallback((event: ReactKeyboardEvent<HTMLButtonElement>) => {
    if (event.key === 'ArrowLeft') {
      event.preventDefault();
      setDescriptionWidth((current) =>
        clamp(current - RESIZE_KEY_STEP, MIN_DESCRIPTION_WIDTH, MAX_DESCRIPTION_WIDTH),
      );
    } else if (event.key === 'ArrowRight') {
      event.preventDefault();
      setDescriptionWidth((current) =>
        clamp(current + RESIZE_KEY_STEP, MIN_DESCRIPTION_WIDTH, MAX_DESCRIPTION_WIDTH),
      );
    } else if (event.key === 'Home') {
      event.preventDefault();
      setDescriptionWidth(MIN_DESCRIPTION_WIDTH);
    } else if (event.key === 'End') {
      event.preventDefault();
      setDescriptionWidth(MAX_DESCRIPTION_WIDTH);
    }
  }, []);

  const handleVerticalResizeKey = useCallback((event: ReactKeyboardEvent<HTMLButtonElement>) => {
    if (event.key === 'ArrowUp') {
      event.preventDefault();
      setResultsHeight((current) =>
        clamp(current + RESIZE_KEY_STEP, MIN_RESULTS_HEIGHT, MAX_RESULTS_HEIGHT),
      );
    } else if (event.key === 'ArrowDown') {
      event.preventDefault();
      setResultsHeight((current) =>
        clamp(current - RESIZE_KEY_STEP, MIN_RESULTS_HEIGHT, MAX_RESULTS_HEIGHT),
      );
    } else if (event.key === 'Home') {
      event.preventDefault();
      setResultsHeight(MIN_RESULTS_HEIGHT);
    } else if (event.key === 'End') {
      event.preventDefault();
      setResultsHeight(MAX_RESULTS_HEIGHT);
    }
  }, []);

  const handleSubmit = async (mode: SubmissionMode) => {
    if (!problem || !sessionId || submissionInFlightRef.current) return;
    submissionInFlightRef.current = true;
    const requestID = runRequestRef.current + 1;
    runRequestRef.current = requestID;
    const submittedFiles = filesRef.current;
    setSubmitting(true);
    setActiveSubmissionMode(mode);
    setSubmission(null);
    setError(null);
    try {
      if (autosaveTimerRef.current) clearTimeout(autosaveTimerRef.current);
      await persistDraft(submittedFiles, hintsRef.current);
      if (mode === 'submit') {
        setMemoryUpdateStatus('pending');
      }
      const res = await api.post<{
        submission_id: string;
        memory_update_status?: 'synced' | 'failed';
      }>('/submissions', {
        problem_id: problem.id,
        session_id: sessionId,
        files: submittedFiles,
        mode,
      });
      sessionStateRef.current = {
        ...sessionStateRef.current,
        last_submission_id: res.submission_id,
        ...(res.memory_update_status
          ? { memory_update_status: res.memory_update_status }
          : {}),
      };
      if (mode === 'submit' && res.memory_update_status) {
        setMemoryUpdateStatus(res.memory_update_status);
      }

      while (runRequestRef.current === requestID) {
        const sub = await api.get<Submission>(
          `/submissions/${res.submission_id}`,
        );
        if (sub.status === 'pending' || sub.status === 'running') {
          await new Promise((resolve) => setTimeout(resolve, 1000));
          continue;
        }
        if (sub.status === 'completed' && !sub.result) {
          throw new Error('The execution completed without test results.');
        }
        setSubmission(sub);
        await persistDraft(filesRef.current, hintsRef.current);
        break;
      }
    } catch (err) {
      if (runRequestRef.current === requestID) {
        setError(
          err instanceof Error
            ? err.message
            : mode === 'run'
              ? 'Run failed.'
              : 'Submission failed.',
        );
      }
    } finally {
      if (runRequestRef.current === requestID) {
        submissionInFlightRef.current = false;
        setSubmitting(false);
        setActiveSubmissionMode(null);
      }
    }
  };

  const retryMemoryUpdate = async () => {
    if (!sessionId) return;
    setMemoryUpdateStatus('pending');
    try {
      await api.post('/memory/profile/maintain', { session_id: sessionId });
      const state = { ...sessionStateRef.current, memory_update_status: 'synced' };
      sessionStateRef.current = state;
      await api.patch<PracticeSession>(`/sessions/${sessionId}`, { state });
      setMemoryUpdateStatus('synced');
    } catch {
      const state = { ...sessionStateRef.current, memory_update_status: 'failed' };
      sessionStateRef.current = state;
      void api.patch<PracticeSession>(`/sessions/${sessionId}`, { state }).catch(() => {});
      setMemoryUpdateStatus('failed');
    }
  };

  if (error && !problem) {
    return (
      <div className="max-w-xl mx-auto px-6 py-16">
        <div
          className="rounded-xl border border-red-400 bg-red-100 px-5 py-4 text-sm text-red-900"
          style={{ boxShadow: '0 1px 3px rgba(0,0,0,0.04), 0 4px 12px rgba(0,0,0,0.03)' }}
        >
          {error}
        </div>
      </div>
    );
  }

  if (!problem) {
    return (
      <div className="flex justify-center py-16">
        <GridSpinner size="md" />
      </div>
    );
  }

  const monacoLang = languageMap[problem.language] ?? 'plaintext';
  const editorWidth = 100 - descriptionWidth;
  const result = submission?.result ?? null;
  const terminalFailure =
    submission && isTerminalFailureSubmission(submission) ? submission : null;
  const hasTestPanel = submitting || Boolean(error) || Boolean(submission);

  return (
    <div ref={pageRef} className="h-screen flex">
      {/* Left: Problem Description */}
      <div
        className="shrink-0 min-w-0 overflow-y-auto bg-background-100 p-6"
        style={{ width: `${descriptionWidth}%` }}
      >
        <h1 className="mb-3 text-2xl leading-8 font-semibold text-gray-1000">{problem.title}</h1>
        <div className="mb-5 flex gap-2 font-mono text-xs">
          <span className="rounded-md bg-gray-100 px-2 py-1 text-gray-900">{problem.language.toUpperCase()}</span>
          {problem.framework && (
            <span className="rounded-md bg-gray-100 px-2 py-1 text-gray-900">{problem.framework.toUpperCase()}</span>
          )}
          <span className="rounded-md bg-amber-100 px-2 py-1 text-amber-900">
            {problem.estimated_minutes} MIN
          </span>
          {resumedDraft && (
            <span className="rounded-md border border-green-300 bg-green-100 px-2 py-1 text-green-900">
              DRAFT RESUMED
            </span>
          )}
          {saveStatus !== 'idle' && (
            <span
              className={`rounded-md border px-2 py-1 ${
                saveStatus === 'error'
                  ? 'border-red-300 bg-red-100 text-red-900'
                  : 'border-gray-alpha-200 bg-background-100 text-gray-700'
              }`}
              aria-live="polite"
            >
              {saveStatus === 'saving'
                ? 'SAVING'
                : saveStatus === 'saved'
                  ? 'SAVED'
                  : 'NOT SAVED'}
            </span>
          )}
        </div>
        <MarkdownContent className="text-sm text-gray-900">
          {problem.description}
        </MarkdownContent>

        <WorkedExamples cases={problem.public_cases ?? []} />

        {/* Hints */}
        {problem.hints && problem.hints.length > 0 && (
          <div className="mt-6 border-t border-gray-alpha-200 pt-4">
            <h3 className="mb-3 text-sm font-semibold text-gray-1000">
              Hints
            </h3>
            {problem.hints.map((hint, i) => (
              <div key={i} className="mb-2">
                {i < hintsRevealed ? (
                  <p
                    className="rounded-xl border border-gray-alpha-200 bg-gray-100 px-4 py-3 text-sm leading-6 text-gray-900"
                  >
                    {hint.text}
                  </p>
                ) : (
                  <button
                    onClick={() => revealHint(i + 1)}
                    className="text-sm font-medium text-blue-700 transition-colors hover:text-blue-800"
                  >
                    {'\u2192'} Reveal hint {i + 1}{' '}
                    {hint.cost > 0 ? `(${hint.cost} credit)` : ''}
                  </button>
                )}
              </div>
            ))}
          </div>
        )}
      </div>

      <button
        type="button"
        className="group relative z-10 w-2 shrink-0 cursor-col-resize border-x border-gray-alpha-200 bg-gray-100/80 hover:bg-gray-200 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-blue-700"
        aria-label="Resize editor pane"
        aria-orientation="vertical"
        aria-valuemin={100 - MAX_DESCRIPTION_WIDTH}
        aria-valuemax={100 - MIN_DESCRIPTION_WIDTH}
        aria-valuenow={Math.round(editorWidth)}
        aria-valuetext={`${Math.round(editorWidth)} percent editor width`}
        role="separator"
        onPointerDown={beginHorizontalResize}
        onKeyDown={handleHorizontalResizeKey}
      >
        <span
          aria-hidden="true"
          className="absolute inset-y-6 left-1/2 w-px -translate-x-1/2 bg-gray-alpha-400 transition-colors group-hover:bg-gray-alpha-600 group-focus-visible:bg-blue-700"
        />
      </button>

      {/* Right: Editor + Results */}
      <div ref={rightPaneRef} className="min-w-0 flex-1 flex flex-col bg-[#1e1e1e]">
        {/* File tabs */}
        <div className="border-b border-[#333] bg-[#252526]">
          <div className="flex items-center border-b border-[#333]">
            {files.map((file, i) => (
              <button
                key={file.path}
                onClick={() => setActiveFile(i)}
                className={`px-4 py-2 text-xs border-r border-[#333] transition-colors ${
                  i === activeFile
                    ? 'bg-[#1e1e1e] text-[#ccc]'
                    : 'text-[#666] hover:text-[#ccc]'
                }`}
              >
                {file.path}
              </button>
            ))}
          </div>
          <div className="flex items-center gap-2 px-3 py-2">
            <p className="flex-1 text-[10px] leading-4 text-[#858585]">
              Run checks sample cases. Submit grades the full hidden suite.
            </p>
            <button
              type="button"
              onClick={() => void handleSubmit('run')}
              disabled={submitting || !sessionId}
              className="rounded-md border border-[#555] bg-[#2d2d2d] px-4 py-1.5 text-sm font-medium text-[#ccc] transition-colors hover:bg-[#333] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-background-100 disabled:opacity-40"
            >
              {submitting && activeSubmissionMode === 'run' ? 'RUNNING' : 'RUN'}
            </button>
            <button
              type="button"
              onClick={() => void handleSubmit('submit')}
              disabled={submitting || !sessionId}
              className="rounded-md bg-background-100 px-4 py-1.5 text-sm font-medium text-gray-1000 transition-colors hover:bg-gray-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-background-100 disabled:opacity-40"
              style={{ boxShadow: '0 1px 3px rgba(0,0,0,0.3)' }}
            >
              {submitting && activeSubmissionMode === 'submit' ? 'SUBMITTING' : 'SUBMIT'}
            </button>
          </div>
        </div>

        {/* Editor */}
        <div className="flex-1 min-h-0">
          {files.length > 0 && (
            <Editor
              height="100%"
              language={monacoLang}
              theme="vs-dark"
              value={files[activeFile]?.content ?? ''}
              onChange={handleCodeChange}
              options={{
                fontSize: 13,
                fontFamily:
                  "'Geist Mono', 'SF Mono', 'Cascadia Code', ui-monospace, monospace",
                minimap: { enabled: false },
                scrollBeyondLastLine: false,
                automaticLayout: true,
                padding: { top: 12 },
                lineNumbers: 'on',
                readOnly: submitting,
              }}
            />
          )}
        </div>

        {hasTestPanel && (
          <>
            <button
              type="button"
              className="group relative h-2 shrink-0 cursor-row-resize border-y border-[#333] bg-[#252526] hover:bg-[#2d2d2d] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-background-100"
              aria-label="Resize test results panel"
              aria-orientation="horizontal"
              aria-valuemin={MIN_RESULTS_HEIGHT}
              aria-valuemax={MAX_RESULTS_HEIGHT}
              aria-valuenow={Math.round(resultsHeight)}
              aria-valuetext={`${Math.round(resultsHeight)} percent test results height`}
              role="separator"
              onPointerDown={beginVerticalResize}
              onKeyDown={handleVerticalResizeKey}
            >
              <span
                aria-hidden="true"
                className="absolute left-1/2 top-1/2 h-px w-12 -translate-x-1/2 -translate-y-1/2 bg-[#555] transition-colors group-hover:bg-[#888] group-focus-visible:bg-white"
              />
            </button>

            <div
              className="shrink-0 min-h-24 overflow-hidden bg-[#1e1e1e]"
              style={{ flexBasis: `${resultsHeight}%` }}
            >
              {submitting && (
                <div className="flex h-full items-center justify-center bg-[#252526] p-6">
                  <GridSpinner size="sm" />
                </div>
              )}

              {error && !submitting && (
                <div className="h-full overflow-y-auto px-4 py-3 text-[10px] text-[#f14c4c]">
                  {error}
                </div>
              )}

              {result && submission && !submitting && !error && (
                <div className="h-full overflow-y-auto">
                  <div className="px-4 py-2 border-b border-[#333] flex items-center gap-3">
                    <span
                      className={`text-xs font-bold ${
                        result.status === 'pass' ? 'text-[#4ec9b0]' : 'text-[#f14c4c]'
                      }`}
                    >
                      {result.status === 'pass' ? 'PASS' : 'FAIL'}
                    </span>
                    <span className="text-[10px] text-[#666]">
                      {submissionModeLabel(submission.mode)} {'\u00b7'}{' '}
                      {submissionResultSummary(submission, result)} {'\u00b7'}{' '}
                      {result.duration_ms}ms
                    </span>
                    {submission.mode !== 'run' && memoryUpdateStatus === 'pending' && (
                      <span className="ml-auto text-[10px] text-[#dcdcaa]">
                        UPDATING MEMORY
                      </span>
                    )}
                    {submission.mode !== 'run' && memoryUpdateStatus === 'synced' && (
                      <span className="ml-auto text-[10px] text-[#4ec9b0]">
                        MEMORY UPDATED
                      </span>
                    )}
                    {submission.mode !== 'run' && memoryUpdateStatus === 'failed' && (
                      <button
                        type="button"
                        onClick={() => void retryMemoryUpdate()}
                        className="ml-auto text-[10px] font-semibold text-[#f0c674] hover:text-white"
                      >
                        RESULT SAVED · RETRY MEMORY
                      </button>
                    )}
                  </div>
                  <div>
                    {result.test_cases.map((tc: TestCaseResult, i: number) => (
                      <div
                        key={i}
                        className="px-4 py-1.5 flex items-start gap-2 border-b border-[#252526]"
                      >
                        <span
                          className={`text-xs ${tc.status === 'pass' ? 'text-[#4ec9b0]' : 'text-[#f14c4c]'}`}
                        >
                          {tc.status === 'pass' ? '\u2713' : '\u2717'}
                        </span>
                        <div className="flex-1 min-w-0">
                          <span className="text-xs text-[#ccc]">{tc.name}</span>
                          {tc.error && (
                            <pre className="text-[10px] text-[#f14c4c] mt-1 whitespace-pre-wrap">
                              {tc.error}
                            </pre>
                          )}
                        </div>
                        <span className="text-[10px] text-[#555]">{tc.duration_ms}ms</span>
                      </div>
                    ))}
                  </div>
                  {result.compile_error && (
                    <pre className="p-3 text-[10px] text-[#f14c4c] whitespace-pre-wrap">
                      {result.compile_error}
                    </pre>
                  )}
                  <CapturedStdout submission={submission} />
                </div>
              )}

              {terminalFailure && !submitting && !error && (
                <div className="h-full overflow-y-auto">
                  <div className="flex items-center gap-3 border-b border-[#333] px-4 py-2">
                    <span
                      className={`text-xs font-bold ${terminalFailurePresentation[terminalFailure.status].tone}`}
                    >
                      {terminalFailurePresentation[terminalFailure.status].label}
                    </span>
                    <span className="text-[10px] text-[#666]">
                      {submissionModeLabel(terminalFailure.mode)} {'\u00b7'}{' '}
                      {submissionResultSummary(terminalFailure)}
                    </span>
                  </div>
                  <p className="px-4 py-3 font-mono text-[11px] leading-5 text-[#b8b8b8]">
                    {terminalFailure.failure_detail ||
                      terminalFailurePresentation[terminalFailure.status].fallback}
                  </p>
                  <CapturedStdout submission={terminalFailure} />
                </div>
              )}
            </div>
          </>
        )}
      </div>
    </div>
  );
}
