import { useId, useState, type ReactNode } from 'react';
import {
  AlertCircle,
  Check,
  ChevronDown,
  Circle,
  LoaderCircle,
  RefreshCw,
  Wrench,
} from 'lucide-react';

import { cn } from '@/lib/utils';

export type WorkflowStepStatus = 'queued' | 'running' | 'succeeded' | 'failed';
export type WorkflowConnectionState = 'connected' | 'connecting' | 'reconnecting';

export interface WorkflowStep {
  id: string;
  label: string;
  status: WorkflowStepStatus;
  detail?: string;
  tool?: string;
  duration?: string;
}

export interface WorkflowProgressProps {
  title: string;
  steps: WorkflowStep[];
  connectionState?: WorkflowConnectionState;
  defaultExpanded?: boolean;
  className?: string;
  footer?: ReactNode;
}

function operationStatus(steps: WorkflowStep[]) {
  if (steps.some((step) => step.status === 'failed')) return 'failed';
  if (steps.length > 0 && steps.every((step) => step.status === 'succeeded')) return 'succeeded';
  if (steps.some((step) => step.status === 'running')) return 'running';
  return 'queued';
}

function activeLabel(steps: WorkflowStep[]) {
  return (
    steps.find((step) => step.status === 'running')?.label ??
    steps.find((step) => step.status === 'failed')?.label ??
    [...steps].reverse().find((step) => step.status === 'succeeded')?.label ??
    steps[0]?.label ??
    'Waiting to start'
  );
}

function StepIcon({ status }: { status: WorkflowStepStatus }) {
  if (status === 'running') {
    return <LoaderCircle className="size-3.5 animate-spin" aria-hidden="true" />;
  }
  if (status === 'succeeded') return <Check className="size-3.5" strokeWidth={3} aria-hidden="true" />;
  if (status === 'failed') return <AlertCircle className="size-3.5" aria-hidden="true" />;
  return <Circle className="size-2.5" aria-hidden="true" />;
}

function connectionLabel(connectionState: WorkflowConnectionState) {
  if (connectionState === 'connecting') return 'Connecting';
  if (connectionState === 'reconnecting') return 'Reconnecting';
  return null;
}

export function WorkflowProgress({
  title,
  steps,
  connectionState = 'connected',
  defaultExpanded = false,
  className,
  footer,
}: WorkflowProgressProps) {
  const [expanded, setExpanded] = useState(defaultExpanded);
  const panelId = useId();
  const status = operationStatus(steps);
  const connection = connectionLabel(connectionState);
  const summary = connection ?? activeLabel(steps);

  return (
    <section
      className={cn(
        'bg-popover text-popover-foreground w-full max-w-[380px] overflow-hidden rounded-lg border shadow-lg',
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
            <RefreshCw className="size-3.5 animate-spin" />
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
                <RefreshCw className="size-3.5 animate-spin" />
              ) : (
                <LoaderCircle className="size-3.5 animate-spin" />
              )}
              {connection} to progress stream
            </div>
          )}

          <ol className="space-y-0">
            {steps.map((step, index) => {
              const isLast = index === steps.length - 1;
              return (
                <li key={step.id} className="relative grid grid-cols-[24px_minmax(0,1fr)] gap-2.5 pb-4 last:pb-0">
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
                    <div className="flex items-start justify-between gap-3">
                      <span
                        className={cn(
                          'text-sm leading-5 font-medium',
                          step.status === 'queued' && 'text-muted-foreground font-normal',
                          step.status === 'failed' && 'text-red-900',
                        )}
                      >
                        {step.label}
                      </span>
                      {step.duration && (
                        <span className="text-muted-foreground shrink-0 font-mono text-[10px] leading-5">
                          {step.duration}
                        </span>
                      )}
                    </div>
                    {step.detail && (
                      <p className="text-muted-foreground mt-0.5 text-xs leading-4">{step.detail}</p>
                    )}
                    {step.tool && (
                      <span className="bg-muted text-muted-foreground mt-1.5 inline-flex items-center gap-1 rounded px-1.5 py-0.5 font-mono text-[10px]">
                        <Wrench className="size-2.5" />
                        {step.tool}
                      </span>
                    )}
                  </div>
                </li>
              );
            })}
          </ol>
          {footer && <div className="mt-3 border-t pt-3">{footer}</div>}
        </div>
      )}
    </section>
  );
}
