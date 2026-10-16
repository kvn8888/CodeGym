import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';

import { GridSpinner } from '../../shared/components/GridSpinner';
import {
  WorkspaceEmptyState,
  WorkspacePage,
  WorkspacePageHeader,
  WorkspaceSectionHeader,
} from '../../shared/components/WorkspacePage';
import { useCodeGymAuthState } from '../../shared/auth/authState';
import { useEnvironmentRequest } from '../../shared/hooks/useEnvironmentRequest';

/**
 * Agent-built practice environments (#192): request a technology and
 * learning objective, follow the async build through verification, and
 * land in the browser practice session when it is ready. Builds run for
 * minutes; the request returns immediately and this page polls.
 */
export function EnvironmentPage() {
  const { configured: authConfigured, isAuthenticated } = useCodeGymAuthState();
  const apiAuthReady = !authConfigured || isAuthenticated;
  const navigate = useNavigate();
  const { status, operation, error, sessionId, start, reset } = useEnvironmentRequest();
  const [technology, setTechnology] = useState('spring-boot');
  const [objective, setObjective] = useState('practice dependency injection with services');

  useEffect(() => {
    if (status === 'ready' && sessionId) {
      void navigate(`/practice?session=${encodeURIComponent(sessionId)}`);
    }
  }, [status, sessionId, navigate]);

  if (!apiAuthReady) {
    return (
      <WorkspacePage>
        <WorkspacePageHeader title="Environments" />
        <WorkspaceEmptyState
          title="Sign in required"
          description="Sign in to request an agent-built practice environment."
        />
      </WorkspacePage>
    );
  }

  const busy = status === 'requesting' || status === 'polling';
  const canSubmit = !busy && technology.trim() !== '' && objective.trim() !== '';

  return (
    <WorkspacePage>
      <WorkspacePageHeader
        title="Environments"
        description="Request coding practice for a technology of your choice. CodeGym prepares, verifies, and delivers a working browser exercise — builds take several minutes and run in the background."
      />

      <div className="max-w-2xl">
        <WorkspaceSectionHeader
          title="Request an environment"
          description="Technology plus a learning objective. If no verified artifact exists yet, CodeGym attempts agent preparation subject to platform budgets."
        />
        <form
          onSubmit={(event) => {
            event.preventDefault();
            if (canSubmit) void start(technology, objective);
          }}
          className="space-y-3 rounded-lg border p-4"
        >
          <div>
            <label htmlFor="env-technology" className="mb-1 block text-sm font-medium">
              Technology
            </label>
            <input
              id="env-technology"
              type="text"
              value={technology}
              onChange={(event) => setTechnology(event.target.value)}
              disabled={busy}
              placeholder="spring-boot"
              className="h-9 w-full rounded-md border px-3 font-mono text-sm disabled:opacity-50"
            />
          </div>
          <div>
            <label htmlFor="env-objective" className="mb-1 block text-sm font-medium">
              Learning objective
            </label>
            <input
              id="env-objective"
              type="text"
              value={objective}
              onChange={(event) => setObjective(event.target.value)}
              disabled={busy}
              placeholder="practice dependency injection with services"
              className="h-9 w-full rounded-md border px-3 text-sm disabled:opacity-50"
            />
          </div>
          <div className="flex items-center gap-2">
            <button
              type="submit"
              disabled={!canSubmit}
              className="h-9 rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground disabled:opacity-40"
            >
              {status === 'idle' || status === 'failed' ? 'Request environment' : 'Requesting…'}
            </button>
            {status === 'failed' && (
              <button
                type="button"
                onClick={reset}
                className="h-9 rounded-md border px-4 text-sm font-medium"
              >
                Start over
              </button>
            )}
          </div>
        </form>

        <div className="mt-7" aria-live="polite">
          {busy && (
            <div className="flex items-center gap-3 rounded-lg border px-4 py-3">
              <GridSpinner size="sm" />
              <div className="min-w-0">
                <p className="font-mono text-xs">
                  {operation?.stage || 'queued'} · {operation?.technology} — {operation?.objective}
                </p>
                <p className="text-xs text-muted-foreground">
                  Preparation runs for minutes. This page polls automatically; you can leave and
                  come back.
                </p>
              </div>
            </div>
          )}
          {status === 'failed' && (
            <div className="rounded-lg border border-destructive/40 bg-destructive/5 px-4 py-3">
              <p className="text-sm font-medium">Environment build failed</p>
              <p className="mt-1 font-mono text-xs text-muted-foreground">
                {error ?? operation?.error ?? 'The environment request could not be completed.'}
              </p>
              <p className="mt-2 text-xs text-muted-foreground">
                Missing dependency-cache prerequisites fail fast with a prerequisite message;
                recoverable setup errors are repaired within a fixed budget before failing.
              </p>
            </div>
          )}
        </div>
      </div>
    </WorkspacePage>
  );
}
