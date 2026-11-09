import { useCallback, useEffect, useRef, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import Editor from '@monaco-editor/react';
import { api, ApiRequestError } from '../../shared/api/client';
import { useCodeGymAuthState } from '../../shared/auth/authState';
import { MarkdownContent } from '../../shared/components/MarkdownContent';
import { GridSpinner } from '../../shared/components/GridSpinner';
import type {
  PracticeBrowserSession,
  PracticeResult,
  PracticeSavedSummary,
} from '../../shared/api/types';

type SaveStatus = 'idle' | 'unsaved' | 'saving' | 'saved' | 'error';
type OpStatus = 'idle' | 'running';

const SAMPLE_REQUEST = {
  artifact_id: 'greet-basics-2026-10-01',
  technology: 'python',
  version: '0.1.0',
} as const;

const CATEGORY_LABEL: Record<string, string> = {
  passed: 'PASSED',
  'build-error': 'BUILD ERROR',
  'test-failure': 'TEST FAILURE',
  'malformed-manifest': 'MALFORMED MANIFEST',
  'validation-failed': 'VALIDATION FAILED',
  timeout: 'TIMEOUT',
  'platform-fault': 'PLATFORM FAULT',
};

const EDITOR_LANGUAGES: Record<string, string> = {
  py: 'python',
  java: 'java',
  kt: 'kotlin',
  ts: 'typescript',
  tsx: 'typescript',
  js: 'javascript',
  jsx: 'javascript',
  go: 'go',
  rs: 'rust',
  md: 'markdown',
  json: 'json',
  xml: 'xml',
  yml: 'yaml',
  yaml: 'yaml',
  html: 'html',
  css: 'css',
};

function editorLanguage(path: string | undefined): string {
  const extension = path?.split('.').pop()?.toLowerCase() ?? '';
  return EDITOR_LANGUAGES[extension] ?? 'plaintext';
}

// Open on the first code file; agent-built exercises list EXERCISE.md first,
// and its text already shows as the instructions.
function initialFileIndex(files: Array<{ path: string }>): number {
  const index = files.findIndex((file) => !file.path.toLowerCase().endsWith('.md'));
  return index >= 0 ? index : 0;
}

function friendlyError(err: unknown): string {
  if (err instanceof ApiRequestError) return err.message;
  if (err instanceof Error) return err.message;
  return 'The practice workspace could not be loaded.';
}

export function PracticePage() {
  const { configured: authConfigured, isAuthenticated } = useCodeGymAuthState();
  const apiAuthReady = !authConfigured || isAuthenticated;
  // ?session=<id> opens an existing session, such as one an agent-built
  // environment request just created; otherwise the sample opens.
  const [searchParams] = useSearchParams();
  const requestedSessionId = searchParams.get('session');
  const [session, setSession] = useState<PracticeBrowserSession | null>(null);
  const [files, setFiles] = useState<Array<{ path: string; content: string }>>([]);
  const [activeFile, setActiveFile] = useState(0);
  const [savedList, setSavedList] = useState<PracticeSavedSummary[]>([]);
  const [result, setResult] = useState<PracticeResult | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [saveStatus, setSaveStatus] = useState<SaveStatus>('idle');
  // savingUnavailable is set when the backend reports durable save storage
  // is not configured (503 practice_saved_unavailable). Saves are then
  // refused explicitly instead of reporting success a restart would lose.
  const [savingUnavailable, setSavingUnavailable] = useState(false);
  const [opStatus, setOpStatus] = useState<OpStatus>('idle');
  const [opKind, setOpKind] = useState<'run' | 'submit' | null>(null);
  const filesRef = useRef<Array<{ path: string; content: string }>>([]);
  const sessionIdRef = useRef<string | null>(null);
  // Edit revisions distinguish flushed session state from durable saves:
  // Run/Submit flush the overlay but never mark SAVED. Only a successful
  // Save advances savedRev, and only to the revision flushed at its start,
  // so edits made during Save remain UNSAVED.
  const editRevRef = useRef(0);
  const savedRevRef = useRef(0);
  // Single-flight operation guard: Save/Run/Submit/Resume serialize, and
  // every async response is checked against the issuing sequence and
  // session so an old flush/save never replaces a newer session or marks
  // newer edits saved.
  const opSeqRef = useRef(0);
  const busyRef = useRef(false);

  const isStale = useCallback((seq: number, sessionId: string | null) => {
    return opSeqRef.current !== seq || sessionIdRef.current !== sessionId;
  }, []);

  const statusForRevisions = useCallback((editRev: number, savedRev: number): SaveStatus => {
    if (editRev !== savedRev) return 'unsaved';
    return editRev === 0 ? 'idle' : 'saved';
  }, []);

  const refreshSavedList = useCallback(async () => {
    try {
      const listed = await api.get<PracticeSavedSummary[]>('/practice/saved');
      setSavedList(listed);
    } catch (err: unknown) {
      // Saved list is best-effort; the open session remains usable. A
      // 503 here means durable saves are unavailable on this server.
      if (err instanceof ApiRequestError && err.code === 'practice_saved_unavailable') {
        setSavingUnavailable(true);
      }
    }
  }, []);

  useEffect(() => {
    if (!apiAuthReady) {
      setError('Sign in to open the browser practice workspace.');
      return;
    }
    let cancelled = false;
    const load = async () => {
      try {
        const opened = requestedSessionId
          ? await api.get<PracticeBrowserSession>(
              `/practice/sessions/${encodeURIComponent(requestedSessionId)}`,
            )
          : await api.post<PracticeBrowserSession>('/practice/sessions', {
              ...SAMPLE_REQUEST,
            });
        if (cancelled) return;
        sessionIdRef.current = opened.session_id;
        setSession(opened);
        setFiles(opened.files);
        setActiveFile(initialFileIndex(opened.files));
        filesRef.current = opened.files;
        editRevRef.current = 0;
        savedRevRef.current = 0;
        setSaveStatus('idle');
        setResult(null);
        setError(null);
        await refreshSavedList();
      } catch (err: unknown) {
        if (!cancelled) setError(friendlyError(err));
      }
    };
    void load();
    return () => {
      cancelled = true;
    };
  }, [apiAuthReady, refreshSavedList, requestedSessionId]);

  const handleCodeChange = useCallback(
    (value: string | undefined) => {
      if (value === undefined) return;
      editRevRef.current += 1;
      setFiles((previous) => {
        const next = previous.map((file, index) =>
          index === activeFile ? { ...file, content: value } : file,
        );
        filesRef.current = next;
        return next;
      });
      setSaveStatus('unsaved');
    },
    [activeFile],
  );

  const handleRun = useCallback(
    async (kind: 'run' | 'submit') => {
      const sessionId = sessionIdRef.current;
      if (!sessionId || busyRef.current) return;
      busyRef.current = true;
      const seq = opSeqRef.current + 1;
      opSeqRef.current = seq;
      const mySession = sessionId;
      // Capture the latest editor state now: edits made after this point
      // belong to a later revision and must not be claimed by this run.
      const snapshot = filesRef.current.map((file) => ({ ...file }));
      setOpStatus('running');
      setOpKind(kind);
      setResult(null);
      setError(null);
      try {
        const updated = await api.put<PracticeBrowserSession>(
          `/practice/sessions/${encodeURIComponent(mySession)}/files`,
          { files: snapshot },
        );
        if (isStale(seq, mySession)) return;
        // The flush echo carries the pre-flight snapshot; the local editor
        // may already hold newer edits, so never overwrite filesRef here.
        sessionIdRef.current = updated.session_id;
        setSession(updated);
        const outcome =
          kind === 'run'
            ? await api.post<PracticeResult>(
                `/practice/sessions/${encodeURIComponent(mySession)}/run`,
                {},
              )
            : await api.post<PracticeResult>(
                `/practice/sessions/${encodeURIComponent(mySession)}/submit`,
                {},
              );
        if (isStale(seq, mySession)) return;
        setResult(outcome);
        // A flush is not a durable save: Run/Submit never mark SAVED.
        setSaveStatus(statusForRevisions(editRevRef.current, savedRevRef.current));
      } catch (err: unknown) {
        if (isStale(seq, mySession)) return;
        setError(friendlyError(err));
        setSaveStatus('error');
      } finally {
        if (opSeqRef.current === seq) {
          busyRef.current = false;
          setOpStatus('idle');
          setOpKind(null);
        }
      }
    },
    [isStale, statusForRevisions],
  );

  const handleSave = useCallback(async () => {
    const sessionId = sessionIdRef.current;
    if (!sessionId || busyRef.current) return;
    busyRef.current = true;
    const seq = opSeqRef.current + 1;
    opSeqRef.current = seq;
    const mySession = sessionId;
    // Only the revision flushed at save start is covered by this save.
    const targetRev = editRevRef.current;
    const snapshot = filesRef.current.map((file) => ({ ...file }));
    setSaveStatus('saving');
    setError(null);
    try {
      const updated = await api.put<PracticeBrowserSession>(
        `/practice/sessions/${encodeURIComponent(mySession)}/files`,
        { files: snapshot },
      );
      if (isStale(seq, mySession)) return;
      sessionIdRef.current = updated.session_id;
      setSession(updated);
      await api.post(`/practice/sessions/${encodeURIComponent(mySession)}/save`, {});
      if (isStale(seq, mySession)) return;
      if (targetRev > savedRevRef.current) savedRevRef.current = targetRev;
      // Edits made during Save advanced editRev past targetRev, so the
      // status stays UNSAVED by construction.
      setSaveStatus(statusForRevisions(editRevRef.current, savedRevRef.current));
      await refreshSavedList();
    } catch (err: unknown) {
      if (isStale(seq, mySession)) return;
      if (err instanceof ApiRequestError && err.code === 'practice_saved_unavailable') {
        // Durable storage is not configured: surface the unavailable state
        // persistently and never mark SAVED.
        setSavingUnavailable(true);
      }
      setError(friendlyError(err));
      setSaveStatus('error');
    } finally {
      if (opSeqRef.current === seq) {
        busyRef.current = false;
        setOpStatus('idle');
      }
    }
  }, [isStale, refreshSavedList, statusForRevisions]);

  const handleResume = useCallback(
    async (savedId: string) => {
      if (busyRef.current) return;
      busyRef.current = true;
      const seq = opSeqRef.current + 1;
      opSeqRef.current = seq;
      setOpStatus('running');
      setError(null);
      setResult(null);
      try {
        const resumed = await api.post<PracticeBrowserSession>(
          `/practice/saved/${encodeURIComponent(savedId)}/resume`,
          {},
        );
        if (opSeqRef.current !== seq) return;
        sessionIdRef.current = resumed.session_id;
        setSession(resumed);
        setFiles(resumed.files);
        filesRef.current = resumed.files;
        editRevRef.current = 0;
        savedRevRef.current = 0;
        setActiveFile(initialFileIndex(resumed.files));
        setSaveStatus('saved');
      } catch (err: unknown) {
        if (opSeqRef.current !== seq) return;
        setError(friendlyError(err));
      } finally {
        if (opSeqRef.current === seq) {
          busyRef.current = false;
          setOpStatus('idle');
        }
      }
    },
    [],
  );

  if (error && !session) {
    return (
      <div className="mx-auto max-w-xl px-6 py-16">
        <div className="rounded-xl border border-red-400 bg-red-100 px-5 py-4 text-sm text-red-900">
          {error}
        </div>
      </div>
    );
  }

  if (!session) {
    return (
      <div className="flex justify-center py-16">
        <GridSpinner size="md" />
      </div>
    );
  }

  const running = opStatus === 'running';
  const saveLabel =
    saveStatus === 'saving'
      ? 'SAVING'
      : saveStatus === 'saved'
        ? 'SAVED'
        : saveStatus === 'unsaved'
          ? 'UNSAVED'
          : saveStatus === 'error'
            ? 'NOT SAVED'
            : 'IDLE';

  return (
    <div className="flex h-screen">
      <div className="w-[45%] shrink-0 overflow-y-auto bg-background-100 p-6">
        <h1 className="mb-3 text-2xl font-semibold leading-8 text-gray-1000">
          Browser practice
        </h1>
        <div className="mb-5 flex flex-wrap gap-2 font-mono text-xs">
          <span className="rounded-md bg-gray-100 px-2 py-1 text-gray-900">
            {session.technology.toUpperCase()} · {session.artifact_version}
          </span>
          <span className="rounded-md bg-gray-100 px-2 py-1 text-gray-900">
            {session.environment_base}
          </span>
          {session.simulated && (
            <span className="rounded-md border border-amber-300 bg-amber-100 px-2 py-1 text-amber-900">
              SIMULATED · DEVELOPMENT ONLY
            </span>
          )}
          <span
            className="rounded-md border border-gray-alpha-200 bg-background-100 px-2 py-1 text-gray-700"
            aria-live="polite"
          >
            {saveLabel}
          </span>
        </div>
        <p className="mb-2 font-mono text-xs text-gray-700">
          {session.artifact_id} · {session.network_policy}
        </p>
        <MarkdownContent className="text-sm text-gray-900">{session.instructions}</MarkdownContent>

        <div className="mt-6 border-t border-gray-alpha-200 pt-4">
          <h2 className="mb-2 text-sm font-semibold text-gray-1000">Saved exercises</h2>
          {savingUnavailable && (
            <p className="mb-2 rounded-lg border border-amber-300 bg-amber-100 px-3 py-2 text-sm text-amber-900">
              Saving is unavailable on this server (durable storage is not configured). Restart the
              backend with durable saves before relying on Save.
            </p>
          )}
          {savedList.length === 0 ? (
            <p className="text-sm text-gray-700">No saved exercises yet. Use Save to persist this session.</p>
          ) : (
            <ul className="space-y-2">
              {savedList.map((saved) => (
                <li
                  key={saved.id}
                  className="flex items-center justify-between gap-3 rounded-lg border border-gray-alpha-200 px-3 py-2"
                >
                  <span className="min-w-0 truncate font-mono text-xs text-gray-900">
                    {saved.artifact_id} · {saved.artifact_version}
                  </span>
                  <button
                    type="button"
                    onClick={() => void handleResume(saved.id)}
                    disabled={running}
                    className="shrink-0 text-sm font-medium text-blue-700 hover:text-blue-800 disabled:opacity-40"
                  >
                    Resume
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>
      </div>

      <div className="flex min-w-0 flex-1 flex-col bg-[#1e1e1e]">
        <div className="flex items-center border-b border-[#333] bg-[#252526]">
          {/* Tabs show file names (full path on hover) and scroll on their
              own, so deep package paths never push the actions off screen. */}
          <div className="flex min-w-0 flex-1 overflow-x-auto">
            {files.map((file, i) => (
              <button
                key={file.path}
                onClick={() => setActiveFile(i)}
                title={file.path}
                className={`shrink-0 border-r border-[#333] px-4 py-2 text-xs transition-colors ${
                  i === activeFile ? 'bg-[#1e1e1e] text-[#ccc]' : 'text-[#666] hover:text-[#ccc]'
                }`}
              >
                {file.path.split('/').pop()}
              </button>
            ))}
          </div>
          <button
            onClick={() => void handleSave()}
            disabled={running || savingUnavailable}
            title={savingUnavailable ? 'Saving is unavailable on this server' : undefined}
            className="m-1.5 rounded-md border border-[#555] px-3 py-1.5 text-sm font-medium text-[#ccc] hover:bg-[#333] disabled:opacity-40"
          >
            SAVE
          </button>
          <button
            onClick={() => void handleRun('run')}
            disabled={running}
            className="m-1.5 rounded-md bg-background-100 px-4 py-1.5 text-sm font-medium text-gray-1000 hover:bg-gray-100 disabled:opacity-40"
          >
            {running && opKind === 'run' ? 'RUNNING' : 'RUN'}
          </button>
          <button
            onClick={() => void handleRun('submit')}
            disabled={running}
            className="m-1.5 rounded-md bg-background-100 px-4 py-1.5 text-sm font-medium text-gray-1000 hover:bg-gray-100 disabled:opacity-40"
          >
            {running && opKind === 'submit' ? 'CHECKING' : 'SUBMIT'}
          </button>
        </div>

        <div className="min-h-0 flex-1">
          {files.length > 0 && (
            <Editor
              height="100%"
              path={files[activeFile]?.path}
              language={editorLanguage(files[activeFile]?.path)}
              theme="vs-dark"
              value={files[activeFile]?.content ?? ''}
              onChange={handleCodeChange}
              options={{
                fontSize: 13,
                minimap: { enabled: false },
                scrollBeyondLastLine: false,
                automaticLayout: true,
                padding: { top: 12 },
                readOnly: running,
              }}
            />
          )}
        </div>

        <div className="max-h-[38%] min-h-24 shrink-0 overflow-y-auto border-t border-[#333] bg-[#1e1e1e]">
          {running && (
            <div className="flex h-full items-center justify-center bg-[#252526] p-6">
              <GridSpinner size="sm" />
            </div>
          )}
          {!running && error && (
            <div className="px-4 py-3 font-mono text-[11px] text-[#f14c4c]">{error}</div>
          )}
          {!running && !error && result && (
            <div>
              <div className="flex items-center gap-3 border-b border-[#333] px-4 py-2">
                <span
                  className={`text-xs font-bold ${result.category === 'passed' ? 'text-[#4ec9b0]' : 'text-[#f14c4c]'}`}
                >
                  {CATEGORY_LABEL[result.category] ?? result.category.toUpperCase()}
                </span>
                <span className="text-[10px] text-[#888]">{result.summary}</span>
                {result.simulated && (
                  <span className="ml-auto text-[10px] text-[#dcdcaa]">SIMULATED</span>
                )}
              </div>
              {result.steps.length > 0 && (
                <ul className="border-b border-[#252526]">
                  {result.steps.map((step, index) => (
                    <li
                      key={`${step.stage}-${index}`}
                      className="flex items-center gap-3 px-4 py-1.5 font-mono text-[11px] text-[#b8b8b8]"
                    >
                      <span>{step.stage}</span>
                      <span>exit {step.exit_code}</span>
                    </li>
                  ))}
                </ul>
              )}
              {result.output && (
                <pre className="whitespace-pre-wrap px-4 py-2 font-mono text-[11px] leading-5 text-[#b8b8b8]">
                  {result.output}
                  {result.output_truncated && '\n…[truncated]'}
                </pre>
              )}
              <p className="px-4 py-2 font-mono text-[10px] text-[#666]">
                {result.artifact_id} · {result.artifact_version} · {result.environment_base}
              </p>
            </div>
          )}
          {!running && !error && !result && (
            <p className="px-4 py-3 font-mono text-[11px] text-[#666]">
              Edit the files, then use RUN for a smoke check or SUBMIT for backend checks.
            </p>
          )}
        </div>
      </div>
    </div>
  );
}
