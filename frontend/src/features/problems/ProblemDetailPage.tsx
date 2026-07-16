import {
  useEffect,
  useState,
  useCallback,
  useRef,
  type KeyboardEvent as ReactKeyboardEvent,
  type PointerEvent as ReactPointerEvent,
} from 'react';
import { useParams } from 'react-router-dom';
import Editor from '@monaco-editor/react';
import { api } from '../../shared/api/client';
import type { Problem, SubmissionFile, TestResult } from '../../shared/api/types';
import { GridSpinner } from '../../shared/components/GridSpinner';
import {
  ProblemFileToolbar,
  ProblemHeader,
  ProblemStatement,
  ProblemTestResults,
} from './components/ProblemWorkspaceComponents';

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

function clamp(value: number, min: number, max: number) {
  return Math.min(max, Math.max(min, value));
}

export function ProblemDetailPage() {
  const { id } = useParams<{ id: string }>();
  const pageRef = useRef<HTMLDivElement>(null);
  const rightPaneRef = useRef<HTMLDivElement>(null);
  const [problem, setProblem] = useState<Problem | null>(null);
  const [files, setFiles] = useState<SubmissionFile[]>([]);
  const [activeFile, setActiveFile] = useState(0);
  const [submitting, setSubmitting] = useState(false);
  const [result, setResult] = useState<TestResult | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [descriptionWidth, setDescriptionWidth] = useState(DEFAULT_DESCRIPTION_WIDTH);
  const [resultsHeight, setResultsHeight] = useState(DEFAULT_RESULTS_HEIGHT);

  useEffect(() => {
    if (!id) return;
    Promise.all([
      api.get<Problem>(`/problems/${id}`),
      api.get<{ files: SubmissionFile[] }>(`/problems/${id}/skeleton`),
    ]).then(([prob, skel]) => {
      setProblem(prob);
      setFiles(skel.files);
      setError(null);
    }).catch((err: unknown) => {
      setError(err instanceof Error ? err.message : 'Could not load this problem.');
    });
  }, [id]);

  const handleCodeChange = useCallback(
    (value: string | undefined) => {
      if (value === undefined) return;
      setFiles((prev) => prev.map((f, i) => (i === activeFile ? { ...f, content: value } : f)));
    },
    [activeFile],
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

  const handleSubmit = async () => {
    if (!problem) return;
    setSubmitting(true);
    setResult(null);
    setError(null);
    try {
      const res = await api.post<{ submission_id: string }>('/submissions', {
        problem_id: problem.id,
        files,
      });
      const poll = async () => {
        const sub = await api.get<{ status: string; result?: TestResult }>(
          `/submissions/${res.submission_id}`,
        );
        if (sub.status === 'pending' || sub.status === 'running') {
          setTimeout(poll, 1000);
        } else if (sub.result) {
          setResult(sub.result);
          setSubmitting(false);
        }
      };
      poll();
    } catch (err) {
      console.error(err);
      setError(err instanceof Error ? err.message : 'Submission failed.');
      setSubmitting(false);
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
  const hasTestPanel = submitting || Boolean(error) || Boolean(result);

  return (
    <div ref={pageRef} className="h-screen flex">
      {/* Left: Problem Description */}
      <div
        className="shrink-0 min-w-0 overflow-y-auto bg-background-100 p-6"
        style={{ width: `${descriptionWidth}%` }}
      >
        <ProblemHeader problem={problem} />
        <ProblemStatement description={problem.description} hints={problem.hints} />
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
        <ProblemFileToolbar
          files={files}
          activeFile={activeFile}
          onActiveFileChange={setActiveFile}
          onRun={handleSubmit}
          running={submitting}
        />

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
                readOnly: false,
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
              <ProblemTestResults
                state={submitting ? 'running' : error ? 'error' : result ? 'complete' : 'idle'}
                result={result}
                error={error}
                onRetry={handleSubmit}
              />
            </div>
          </>
        )}
      </div>
    </div>
  );
}
