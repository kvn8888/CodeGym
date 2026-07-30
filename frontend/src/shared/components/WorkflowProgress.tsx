import { useId, useMemo, useState } from 'react';
import {
  AlertCircle,
  Check,
  ChevronDown,
  Circle,
  LoaderCircle,
  RefreshCw,
} from 'lucide-react';

import type { WorkflowEvent, WorkflowStatus } from '../api/client';
import type { WorkflowConnectionState } from '../hooks/useWorkflowProgress';
import { cn } from '@/lib/utils';

export interface WorkflowProgressProps {
  title: string;
  events: WorkflowEvent[];
  connectionState?: WorkflowConnectionState;
  defaultExpanded?: boolean;
  className?: string;
}

interface WorkflowStep {
  id: string;
  label: string;
  status: WorkflowStatus;
  skipped: boolean;
}

function latestSteps(events: WorkflowEvent[]): WorkflowStep[] {
  const latest = new Map<string, WorkflowStep>();
  const order: string[] = [];
  for (const event of [...events].sort((left, right) => left.sequence - right.sequence)) {
    if (!latest.has(event.step_id)) order.push(event.step_id);
    latest.set(event.step_id, {
      id: event.step_id,
      label: event.label,
      status: event.status,
      skipped: event.metadata?.skipped === true,
    });
  }
  return order.map((id) => latest.get(id)!);
}

function terminalEvent(events: WorkflowEvent[]) {
  return [...events]
    .reverse()
    .find((event) => event.metadata?.terminal === true);
}

function operationStatus(events: WorkflowEvent[], steps: WorkflowStep[]): WorkflowStatus {
  const terminal = terminalEvent(events);
  if (terminal) return terminal.status;
  if (steps.some((step) => step.status === 'running')) return 'running';
  return 'queued';
}

function activeLabel(events: WorkflowEvent[], steps: WorkflowStep[]) {
  const terminal = terminalEvent(events);
  if (terminal) return terminal.label;
  return (
    steps.find((step) => step.status === 'running')?.label ??
    [...steps].reverse().find((step) => step.status === 'succeeded' && !step.skipped)?.label ??
    steps.find((step) => step.status === 'queued')?.label ??
    'Waiting to start'
  );
}

function StepIcon({ status }: { status: WorkflowStatus }) {
  if (status === 'running') {
    return <LoaderCircle className="size-3.5 animate-spin" aria-hidden="true" />;
  }
  if (status === 'succeeded') {
    return <Check className="size-3.5" strokeWidth={3} aria-hidden="true" />;
  }
  if (status === 'failed') {
    return <AlertCircle className="size-3.5" aria-hidden="true" />;
  }
  return <Circle className="size-2.5" aria-hidden="true" />;
}

function connectionLabel(connectionState: WorkflowConnectionState) {
  if (connectionState === 'connecting') return 'Connecting';
  if (connectionState === 'reconnecting') return 'Reconnecting';
  if (connectionState === 'fallback') return 'Progress stream unavailable';
  return null;
}

export function WorkflowProgress({
  title,
  events,
  connectionState = 'connected',
  defaultExpanded = false,
  className,
}: WorkflowProgressProps) {
  const [expanded, setExpanded] = useState(defaultExpanded);
  const panelId = useId();
  const steps = useMemo(() => latestSteps(events), [events]);
  const status = operationStatus(events, steps);
  const connection = connectionLabel(connectionState);
  const summary = connection ?? activeLabel(events, steps);

  return (
    <section
      className={cn(
        'bg-popover text-popover-foreground w-full max-w-[360px] overflow-hidden rounded-lg border shadow-lg',
        className,
      )}
      aria-label={`${title} progress`}
      aria-live="polite"
    >
      <button
        type="button"
        className="hover:bg-muted/50 focus-visible:ring-ring/50 flex min-h-14 w-full items-center gap-3 px-3.5 py-2.5 text-left outline-none transition-colors focus-visible:ring-3 focus-visible:ring-inset"
        onClick={() => setExpanded((current) => !current)}
        aria-expanded={expanded}
        aria-controls={panelId}
      >
        <span
          className={cn(
            'flex size-7 shrink-0 items-center justify-center rounded-full border',
            status === 'failed' && 'border-red-700/25 bg-red-100 text-red-900',
            status === 'succeeded' && 'border-green-700/25 bg-green-100 text-green-900',
            (status === 'running' || status === 'queued') &&
              'border-blue-700/20 bg-blue-100 text-blue-900',
          )}
        >
          {connectionState === 'reconnecting' ? (
            <RefreshCw className="size-3.5 animate-spin" aria-hidden="true" />
          ) : (
            <StepIcon status={status} />
          )}
        </span>
        <span className="min-w-0 flex-1">
          <span className="block truncate text-sm font-medium">{title}</span>
          <span className="text-muted-foreground block truncate text-xs">{summary}</span>
        </span>
        <ChevronDown
          className={cn(
            'text-muted-foreground size-4 shrink-0 transition-transform duration-200',
            expanded && 'rotate-180',
          )}
          aria-hidden="true"
        />
      </button>

      {expanded && (
        <div id={panelId} className="border-t px-3.5 py-3">
          {connection && (
            <div className="bg-muted/50 text-muted-foreground mb-3 flex items-center gap-2 rounded-md border px-3 py-2 text-xs">
              {connectionState === 'reconnecting' ? (
                <RefreshCw className="size-3.5 animate-spin" aria-hidden="true" />
              ) : connectionState === 'fallback' ? (
                <Circle className="size-3" aria-hidden="true" />
              ) : (
                <LoaderCircle className="size-3.5 animate-spin" aria-hidden="true" />
              )}
              {connectionState === 'fallback'
                ? 'The request continues without live step updates.'
                : `${connection} to progress stream`}
            </div>
          )}

          <ol>
            {steps.map((step, index) => {
              const isLast = index === steps.length - 1;
              return (
                <li
                  key={step.id}
                  className="relative grid grid-cols-[24px_minmax(0,1fr)] gap-2.5 pb-4 last:pb-0"
                >
                  {!isLast && (
                    <span
                      className={cn(
                        'absolute top-6 bottom-0 left-[11px] w-px',
                        step.status === 'succeeded' ? 'bg-green-700/35' : 'bg-border',
                      )}
                      aria-hidden="true"
                    />
                  )}
                  <span
                    className={cn(
                      'relative z-10 mt-0.5 flex size-6 items-center justify-center rounded-full border bg-background',
                      step.status === 'queued' && 'text-muted-foreground',
                      step.status === 'running' && 'border-blue-700/30 bg-blue-100 text-blue-900',
                      step.status === 'succeeded' &&
                        'border-green-700/30 bg-green-100 text-green-900',
                      step.status === 'failed' && 'border-red-700/30 bg-red-100 text-red-900',
                    )}
                  >
                    <StepIcon status={step.status} />
                  </span>
                  <div className="min-w-0 pt-0.5">
                    <span
                      className={cn(
                        'text-sm leading-5 font-medium',
                        step.status === 'queued' && 'text-muted-foreground font-normal',
                        step.status === 'failed' && 'text-red-900',
                      )}
                    >
                      {step.label}
                      {step.skipped ? (
                        <span className="text-muted-foreground ml-1 font-normal">(not needed)</span>
                      ) : null}
                    </span>
                  </div>
                </li>
              );
            })}
          </ol>
        </div>
      )}
    </section>
  );
}
