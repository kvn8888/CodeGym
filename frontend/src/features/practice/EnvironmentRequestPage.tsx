import { useCallback, useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { api, ApiRequestError } from '../../shared/api/client';
import { useCodeGymAuthState } from '../../shared/auth/authState';
import { GridSpinner } from '../../shared/components/GridSpinner';
import type { PracticeEnvironmentRequest } from '../../shared/api/types';

const POLL_INTERVAL_MS = 2000;

// Stage labels reported by the backend pipeline, in order. Unknown stages
// still render as the current step; this list only orders the checklist.
const STAGES = [
  'queued',
  'looking for a verified environment',
  'agent building project and resolving dependencies',
  'verifying build, tests, and starter',
  'promotion checks on a fresh copy',
  'opening practice session',
] as const;

function friendlyError(err: unknown): string {
  if (err instanceof ApiRequestError) return err.message;
  if (err instanceof Error) return err.message;
  return 'The environment request failed.';
}

function formatElapsed(ms: number): string {
  const seconds = Math.floor(ms / 1000);
  return `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, '0')}`;
}

export function EnvironmentRequestPage() {
  const navigate = useNavigate();
  const { configured: authConfigured, isAuthenticated } = useCodeGymAuthState();
  const apiAuthReady = !authConfigured || isAuthenticated;
  const [technology, setTechnology] = useState('Spring Boot');
  const [objective, setObjective] = useState('practice dependency injection with services');
  const [request, setRequest] = useState<PracticeEnvironmentRequest | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [startedAt, setStartedAt] = useState<number | null>(null);
  const [now, setNow] = useState(() => Date.now());
  // Poll while the request is pending: each response re-runs this effect,
  // which schedules the next poll or navigates once the session is ready.
  useEffect(() => {
    if (!request || request.status !== 'pending') return;
    let cancelled = false;
    const timer = window.setTimeout(async () => {
      try {
        const latest = await api.get<PracticeEnvironmentRequest>(
          `/practice-environments/requests/${encodeURIComponent(request.id)}`,
        );
        if (cancelled) return;
        if (latest.status === 'ready' && latest.session_id) {
          navigate(`/practice?session=${encodeURIComponent(latest.session_id)}`);
          return;
        }
        setRequest(latest);
      } catch (err: unknown) {
        if (!cancelled) setError(friendlyError(err));
      }
    }, POLL_INTERVAL_MS);
    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [navigate, request]);

  useEffect(() => {
    if (!startedAt || request?.status !== 'pending') return;
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [startedAt, request?.status]);

  const handleSubmit = useCallback(
    async (event: React.FormEvent) => {
      event.preventDefault();
      setError(null);
      setRequest(null);
      try {
        const started = await api.post<PracticeEnvironmentRequest>('/practice-environments/requests', {
          technology: technology.trim(),
          objective: objective.trim(),
        });
        setStartedAt(Date.now());
        setNow(Date.now());
        setRequest(started);
      } catch (err: unknown) {
        setError(friendlyError(err));
      }
    },
    [objective, technology],
  );

  const pending = request?.status === 'pending';
  const currentIndex = request ? STAGES.indexOf(request.stage as (typeof STAGES)[number]) : -1;

  return (
    <div className="mx-auto max-w-2xl px-6 py-12">
      <h1 className="mb-2 text-2xl font-semibold leading-8 text-gray-1000">Build a practice environment</h1>
      <p className="mb-8 text-sm text-gray-700">
        Name any technology and what you want to practice. An agent builds the project, resolves its
        dependencies, and writes an exercise; the backend verifies it before you get the question.
      </p>

      {!apiAuthReady && (
        <div className="mb-6 rounded-xl border border-red-400 bg-red-100 px-5 py-4 text-sm text-red-900">
          Sign in to request a practice environment.
        </div>
      )}

      <form onSubmit={(event) => void handleSubmit(event)} className="space-y-4">
        <label className="block">
          <span className="mb-1 block text-sm font-medium text-gray-1000">Technology</span>
          <input
            value={technology}
            onChange={(event) => setTechnology(event.target.value)}
            disabled={pending}
            required
            className="w-full rounded-lg border border-gray-alpha-200 bg-background-100 px-3 py-2 text-sm text-gray-1000 disabled:opacity-60"
          />
        </label>
        <label className="block">
          <span className="mb-1 block text-sm font-medium text-gray-1000">What do you want to practice?</span>
          <input
            value={objective}
            onChange={(event) => setObjective(event.target.value)}
            disabled={pending}
            className="w-full rounded-lg border border-gray-alpha-200 bg-background-100 px-3 py-2 text-sm text-gray-1000 disabled:opacity-60"
          />
        </label>
        <button
          type="submit"
          disabled={pending || !apiAuthReady || technology.trim() === ''}
          className="rounded-md bg-gray-1000 px-4 py-2 text-sm font-medium text-background-100 hover:opacity-90 disabled:opacity-40"
        >
          {pending ? 'BUILDING' : 'BUILD EXERCISE'}
        </button>
      </form>

      {request && (
        <section className="mt-8 rounded-xl border border-gray-alpha-200 bg-background-100 p-5" aria-live="polite">
          <div className="mb-4 flex items-center justify-between gap-3">
            <h2 className="text-sm font-semibold text-gray-1000">
              {request.technology}
              {request.objective ? ` · ${request.objective}` : ''}
            </h2>
            {startedAt && (
              <span className="font-mono text-xs text-gray-700">{formatElapsed(now - startedAt)}</span>
            )}
          </div>
          <ol className="space-y-2">
            {STAGES.map((stage, index) => {
              const done = request.status === 'ready' || (currentIndex >= 0 && index < currentIndex);
              const active = pending && index === currentIndex;
              const failedHere = request.status === 'failed' && index === currentIndex;
              return (
                <li key={stage} className="flex items-center gap-3 text-sm">
                  <span className="flex w-5 justify-center">
                    {active ? (
                      <GridSpinner size="sm" />
                    ) : failedHere ? (
                      <span className="text-red-700">✕</span>
                    ) : done ? (
                      <span className="text-green-700">✓</span>
                    ) : (
                      <span className="text-gray-500">·</span>
                    )}
                  </span>
                  <span className={done || active || failedHere ? 'text-gray-1000' : 'text-gray-600'}>{stage}</span>
                </li>
              );
            })}
            {currentIndex < 0 && request.stage && (
              <li className="text-sm text-gray-700">Current step: {request.stage}</li>
            )}
          </ol>
          {request.status === 'failed' && request.error && (
            <pre className="mt-4 whitespace-pre-wrap rounded-lg border border-red-300 bg-red-100 px-3 py-2 font-mono text-xs text-red-900">
              {request.error}
            </pre>
          )}
        </section>
      )}

      {error && (
        <div className="mt-6 rounded-xl border border-red-400 bg-red-100 px-5 py-4 text-sm text-red-900">{error}</div>
      )}
    </div>
  );
}
