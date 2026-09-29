import { useCallback, useEffect, useRef, useState } from 'react';

import {
  cancelWorkflowOperation,
  createWorkflowOperation,
  streamWorkflowEvents,
  type WorkflowEvent,
  type WorkflowKind,
  type WorkflowStatus,
} from '../api/client';

export type WorkflowConnectionState =
  | 'connected'
  | 'connecting'
  | 'reconnecting'
  | 'fallback';

export interface WorkflowProgressState {
  kind: WorkflowKind | null;
  operationId: string | null;
  events: WorkflowEvent[];
  connectionState: WorkflowConnectionState;
  visible: boolean;
}

type CurrentWorkflow = {
  operationId: string | null;
  streamController: AbortController | null;
  executionController: AbortController;
  terminal: boolean;
  executionStatus: 'running' | 'succeeded' | 'failed';
  streamFallback: boolean;
};

const MAX_STREAM_RECONNECTS = 3;

const EMPTY_STATE: WorkflowProgressState = {
  kind: null,
  operationId: null,
  events: [],
  connectionState: 'connected',
  visible: false,
};

export function mergeWorkflowEvent(
  current: WorkflowEvent[],
  incoming: WorkflowEvent,
): WorkflowEvent[] {
  if (current.some((event) => event.sequence === incoming.sequence)) {
    return current;
  }
  return [...current, incoming].sort((left, right) => left.sequence - right.sequence);
}

function fallbackEvent(kind: WorkflowKind, status: WorkflowStatus): WorkflowEvent {
  const label =
    kind === 'memory_reflection'
      ? 'Update memory'
      : kind === 'mcq_next_round'
        ? 'Update memory and prepare the next set'
        : kind === 'problem_generation'
          ? 'Generate coding problem'
          : 'Prepare question set';
  return {
    operation_id: 'non-streaming-fallback',
    sequence: 1,
    step_id: 'request',
    label,
    status,
    timestamp: new Date().toISOString(),
    metadata:
      status === 'succeeded' || status === 'failed'
        ? { terminal: true, ...(status === 'failed' ? { retryable: true } : {}) }
        : undefined,
  };
}

function isTerminal(event: WorkflowEvent) {
  return event.metadata?.terminal === true;
}

function isAbortError(error: unknown) {
  return error instanceof DOMException && error.name === 'AbortError';
}

export function useWorkflowProgress() {
  const [state, setState] = useState<WorkflowProgressState>(EMPTY_STATE);
  const currentRef = useRef<CurrentWorkflow | null>(null);

  const consume = useCallback(async (
    operationId: string,
    kind: WorkflowKind,
    signal: AbortSignal,
  ) => {
    let cursor = 0;
    let reconnects = 0;
    while (!signal.aborted) {
      setState((current) => ({
        ...current,
        connectionState: reconnects === 0 ? 'connecting' : 'reconnecting',
      }));
      try {
        await streamWorkflowEvents(
          operationId,
          cursor,
          (event) => {
            cursor = Math.max(cursor, event.sequence);
            if (isTerminal(event) && currentRef.current?.operationId === operationId) {
              currentRef.current.terminal = true;
            }
            setState((current) => ({
              ...current,
              events: mergeWorkflowEvent(current.events, event),
              connectionState: 'connected',
            }));
          },
          signal,
        );
        if (currentRef.current?.operationId !== operationId || currentRef.current.terminal) {
          return;
        }
      } catch (error) {
        if (signal.aborted || isAbortError(error)) return;
      }
      reconnects += 1;
      if (reconnects >= MAX_STREAM_RECONNECTS) {
        const current = currentRef.current;
        if (current?.operationId !== operationId || current.terminal) return;
        current.streamFallback = true;
        if (current.executionStatus !== 'running') current.terminal = true;
        setState((state) => ({
          ...state,
          events: [fallbackEvent(kind, current.executionStatus)],
          connectionState: 'fallback',
        }));
        return;
      }
      setState((current) => ({ ...current, connectionState: 'reconnecting' }));
      await new Promise((resolve) => window.setTimeout(resolve, Math.min(250 * 2 ** reconnects, 2000)));
    }
  }, []);

  const cancelCurrent = useCallback(async (hide: boolean) => {
    const current = currentRef.current;
    if (!current) return;
    current.executionController.abort();
    if (current.operationId && !current.terminal) {
      try {
        await cancelWorkflowOperation(current.operationId);
      } catch {
        // The backend may already have committed a terminal event.
      }
    }
    current.streamController?.abort();
    currentRef.current = null;
    if (hide) setState(EMPTY_STATE);
  }, []);

  const run = useCallback(
    async <T,>(
      kind: WorkflowKind,
      execute: (operationId: string | undefined, signal: AbortSignal) => Promise<T>,
    ): Promise<T> => {
      await cancelCurrent(false);
      const executionController = new AbortController();
      let operationId: string | null = null;
      let streamController: AbortController | null = null;
      try {
        const created = await createWorkflowOperation(kind);
        operationId = created.operation.id;
        streamController = new AbortController();
        currentRef.current = {
          operationId,
          streamController,
          executionController,
          terminal: false,
          executionStatus: 'running',
          streamFallback: false,
        };
        setState({
          kind,
          operationId,
          events: created.events,
          connectionState: 'connecting',
          visible: true,
        });
        void consume(operationId, kind, streamController.signal);
      } catch {
        currentRef.current = {
          operationId: null,
          streamController: null,
          executionController,
          terminal: false,
          executionStatus: 'running',
          streamFallback: true,
        };
        setState({
          kind,
          operationId: null,
          events: [fallbackEvent(kind, 'running')],
          connectionState: 'fallback',
          visible: true,
        });
      }

      try {
        const result = await execute(operationId ?? undefined, executionController.signal);
        const current = currentRef.current;
        if (current && current.operationId === operationId) {
          current.executionStatus = 'succeeded';
        }
        if (!operationId || current?.streamFallback) {
          if (current) current.terminal = true;
          setState((current) => ({
            ...current,
            events: [fallbackEvent(kind, 'succeeded')],
          }));
        }
        return result;
      } catch (error) {
        const current = currentRef.current;
        if (current && current.operationId === operationId) {
          current.executionStatus = 'failed';
        }
        if (operationId && !current?.terminal) {
          try {
            await cancelWorkflowOperation(operationId);
          } catch {
            // The request handler may already have written its terminal failure.
          }
        }
        if ((!operationId || current?.streamFallback) && !isAbortError(error)) {
          if (current) current.terminal = true;
          setState((current) => ({
            ...current,
            events: [fallbackEvent(kind, 'failed')],
          }));
        }
        throw error;
      }
    },
    [cancelCurrent, consume],
  );

  const dismiss = useCallback(() => {
    setState((current) => ({ ...current, visible: false }));
  }, []);

  useEffect(
    () => () => {
      const current = currentRef.current;
      current?.executionController.abort();
      current?.streamController?.abort();
      if (current?.operationId && !current.terminal) {
        void cancelWorkflowOperation(current.operationId).catch(() => {});
      }
    },
    [],
  );

  return {
    state,
    run,
    cancel: () => cancelCurrent(true),
    dismiss,
  };
}
