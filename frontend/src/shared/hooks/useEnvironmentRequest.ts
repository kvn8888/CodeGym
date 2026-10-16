import { useCallback, useEffect, useRef, useState } from 'react';

import { api, ApiRequestError } from '../api/client';

export type EnvironmentRequestStatus =
  | 'idle'
  | 'requesting'
  | 'polling'
  | 'ready'
  | 'failed';

/** Public view of an async environment build operation (see EnvironmentsHandler). */
export interface EnvironmentOperation {
  id: string;
  status: 'pending' | 'ready' | 'failed';
  stage: string;
  session_id?: string;
  error?: string;
  technology: string;
  objective: string;
}

const POLL_INTERVAL_MS = 2000;

function friendlyError(err: unknown): string {
  if (err instanceof ApiRequestError) return err.message;
  if (err instanceof Error) return err.message;
  return 'The environment request could not be completed.';
}

/**
 * Requests an agent-built practice environment and polls the operation until
 * it is ready (exposing the ready session id) or fails. Builds run for minutes,
 * so the POST returns immediately and this hook never blocks on it.
 */
export function useEnvironmentRequest() {
  const [status, setStatus] = useState<EnvironmentRequestStatus>('idle');
  const [operation, setOperation] = useState<EnvironmentOperation | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [sessionId, setSessionId] = useState<string | null>(null);
  const pollTimerRef = useRef<number | null>(null);

  const clearTimer = useCallback(() => {
    if (pollTimerRef.current !== null) {
      window.clearTimeout(pollTimerRef.current);
      pollTimerRef.current = null;
    }
  }, []);

  useEffect(() => clearTimer, [clearTimer]);

  const start = useCallback(
    async (technology: string, objective: string) => {
      clearTimer();
      setStatus('requesting');
      setError(null);
      setOperation(null);
      setSessionId(null);
      try {
        const operation = await api.post<EnvironmentOperation>(
          '/practice-environments/requests',
          { technology: technology.trim(), objective: objective.trim() },
        );
        setOperation(operation);
        setStatus('polling');

        const poll = async () => {
          if (!operation.id) return;
          try {
            const current = await api.get<EnvironmentOperation>(
              `/practice-environments/requests/${encodeURIComponent(operation.id)}`,
            );
            setOperation(current);
            if (current.status === 'failed') {
              setStatus('failed');
              setError(current.error || 'Environment build failed.');
              clearTimer();
              return;
            }
            if (current.status === 'ready' && current.session_id) {
              setStatus('ready');
              setSessionId(current.session_id);
              clearTimer();
              return;
            }
            clearTimer();
            pollTimerRef.current = window.setTimeout(poll, POLL_INTERVAL_MS);
          } catch (err: unknown) {
            setStatus('failed');
            setError(friendlyError(err));
            clearTimer();
          }
        };
        void poll();
      } catch (err: unknown) {
        setStatus('failed');
        setError(friendlyError(err));
      }
    },
    [clearTimer],
  );

  const reset = useCallback(() => {
    clearTimer();
    setStatus('idle');
    setOperation(null);
    setError(null);
    setSessionId(null);
  }, [clearTimer]);

  return { status, operation, error, sessionId, start, reset };
}
