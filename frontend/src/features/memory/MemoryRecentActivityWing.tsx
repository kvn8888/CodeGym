import { useState } from 'react';
import { Brain, CheckCircle2, Clock3, X } from 'lucide-react';

import type { MemoryEvent } from '../../shared/api/types';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog';

function formatRelativeDate(value: string) {
  const then = new Date(value).getTime();
  const now = Date.now();
  const minutes = Math.round((then - now) / 60_000);
  const formatter = new Intl.RelativeTimeFormat('en', { numeric: 'auto' });

  if (Math.abs(minutes) < 60) return formatter.format(minutes, 'minute');
  const hours = Math.round(minutes / 60);
  if (Math.abs(hours) < 24) return formatter.format(hours, 'hour');
  return formatter.format(Math.round(hours / 24), 'day');
}

function eventPresentation(event: MemoryEvent) {
  if (event.type.includes('completed')) return { label: 'Session', icon: CheckCircle2 };
  if (event.type.includes('note')) return { label: 'Memory', icon: Brain };
  return { label: 'Practice', icon: Clock3 };
}

function formatEventType(type: string) {
  return type
    .split('_')
    .map((word) => word[0]?.toUpperCase() + word.slice(1))
    .join(' ');
}

function formatExactDate(value: string) {
  return new Intl.DateTimeFormat('en', {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  }).format(new Date(value));
}

function stringifyPayload(payload: unknown) {
  if (!payload) return 'No additional metadata.';
  try {
    return JSON.stringify(payload, null, 2);
  } catch {
    return 'Could not render event metadata.';
  }
}

function getPayloadRecord(payload: unknown): Record<string, unknown> | null {
  if (!payload || typeof payload !== 'object' || Array.isArray(payload)) {
    return null;
  }

  return payload as Record<string, unknown>;
}

function normalizeLabel(key: string) {
  return key
    .split('_')
    .map((word) => word[0]?.toUpperCase() + word.slice(1))
    .join(' ');
}

function formatDetailValue(value: unknown): string {
  if (value === null || value === undefined) {
    return 'Unknown';
  }
  if (typeof value === 'string') {
    return value;
  }
  if (typeof value === 'number' || typeof value === 'boolean') {
    return String(value);
  }
  if (Array.isArray(value)) {
    return value.length === 0 ? 'None' : value.map((item) => formatDetailValue(item)).join(', ');
  }
  if (typeof value === 'object') {
    try {
      return JSON.stringify(value);
    } catch {
      return 'Complex object';
    }
  }

  return String(value);
}

function outcomePresentation(type: string) {
  if (type.includes('failed') || type.includes('incorrect')) {
    return {
      label: 'Needs attention',
      className: 'border-amber-300/70 bg-amber-50 text-amber-900',
    };
  }
  if (type.includes('solved') || type.includes('completed')) {
    return {
      label: 'Positive signal',
      className: 'border-emerald-300/70 bg-emerald-50 text-emerald-900',
    };
  }
  if (type.includes('started') || type.includes('opened')) {
    return {
      label: 'Progress marker',
      className: 'border-sky-300/70 bg-sky-50 text-sky-900',
    };
  }
  return {
    label: 'Informational',
    className: 'border-border bg-muted/70 text-foreground',
  };
}

type RecentEventPolicy = {
  includeTypes: string[];
  excludeTypes: string[];
};

const recentEventPolicy: RecentEventPolicy = {
  includeTypes: [
    'session_completed',
    'attempt_solved',
    'attempt_failed',
    'answer_incorrect',
    'question_skipped',
    'note_created',
    'note_updated',
    'note_pruned',
    'problem_generated',
    'mcq_set_generated',
    'interview_prompt_generated',
  ],
  excludeTypes: [
    'session_started',
    'attempt_started',
    'thread_opened',
    'thread_closed',
    'message_sent',
    'assistant_replied',
    'tests_run',
    'memory_api_checked',
  ],
};

function curateRecentEvents(events: MemoryEvent[]) {
  const includeTypes = new Set(recentEventPolicy.includeTypes);
  const excludeTypes = new Set(recentEventPolicy.excludeTypes);

  return events.filter((event) => includeTypes.has(event.type) && !excludeTypes.has(event.type));
}

type DetailRow = {
  label: string;
  value: string;
};

type DetailSection = {
  title: string;
  rows: DetailRow[];
};

function pushPayloadRows(
  rows: DetailRow[],
  payload: Record<string, unknown> | null,
  keys: string[],
  used: Set<string>,
  labels?: Record<string, string>,
) {
  if (!payload) return;
  keys.forEach((key) => {
    if (payload[key] === undefined) {
      return;
    }
    rows.push({
      label: labels?.[key] ?? normalizeLabel(key),
      value: formatDetailValue(payload[key]),
    });
    used.add(key);
  });
}

function eventDetails(event: MemoryEvent) {
  const payload = getPayloadRecord(event.payload);
  const usedKeys = new Set<string>();

  const contextRows: DetailRow[] = [
    { label: 'Event ID', value: event.id },
    { label: 'Workspace ID', value: event.workspace_id },
    { label: 'User ID', value: event.user_id },
  ];
  pushPayloadRows(
    contextRows,
    payload,
    ['session_id', 'problem_id', 'attempt_id', 'thread_id', 'note_id', 'generation_job_id', 'message_id'],
    usedKeys,
  );

  const metricRows: DetailRow[] = [];
  pushPayloadRows(
    metricRows,
    payload,
    [
      'passed',
      'total',
      'score',
      'message_length',
      'answer_length',
      'duration_ms',
      'latency_ms',
      'retry_count',
      'question_count',
      'correct_count',
      'incorrect_count',
      'test_case_count',
    ],
    usedKeys,
  );

  const classificationRows: DetailRow[] = [];
  pushPayloadRows(
    classificationRows,
    payload,
    ['format', 'difficulty', 'language', 'topic', 'area', 'concept', 'tags', 'action', 'result', 'status'],
    usedKeys,
  );

  const timingRows: DetailRow[] = [
    { label: 'Occurred At', value: formatExactDate(event.occurred_at) },
    { label: 'Persisted At', value: formatExactDate(event.created_at) },
  ];
  pushPayloadRows(timingRows, payload, ['started_at', 'completed_at'], usedKeys);

  const provenanceRows: DetailRow[] = [
    { label: 'Source', value: event.source },
    { label: 'Type', value: event.type },
  ];
  pushPayloadRows(provenanceRows, payload, ['schema_version', 'provider', 'model', 'emitter'], usedKeys);

  const extraRows: DetailRow[] = [];
  if (payload) {
    Object.keys(payload)
      .filter((key) => !usedKeys.has(key))
      .sort()
      .forEach((key) => {
        extraRows.push({ label: normalizeLabel(key), value: formatDetailValue(payload[key]) });
      });
  }

  const sections: DetailSection[] = [
    { title: 'Timing', rows: timingRows },
    { title: 'Context', rows: contextRows },
    { title: 'Outcome Metrics', rows: metricRows },
    { title: 'Classification', rows: classificationRows },
    { title: 'Provenance', rows: provenanceRows },
    { title: 'Additional Metadata', rows: extraRows },
  ].filter((section) => section.rows.length > 0);

  return {
    sections,
    outcome: outcomePresentation(event.type),
  };
}

type MemoryRecentActivityWingProps = {
  events: MemoryEvent[];
};

export function RecentActivity({ events }: MemoryRecentActivityWingProps) {
  const [allEventsOpen, setAllEventsOpen] = useState(false);
  const [activeAllEventId, setActiveAllEventId] = useState<string | null>(null);
  const visibleEvents = curateRecentEvents(events).slice(0, 6);

  return (
    <>
      <aside className="min-w-0 lg:border-l lg:pl-7">
        <section>
          <div className="mb-3">
            <div className="flex min-h-8 items-center justify-between gap-3">
              <h2 className="text-sm leading-5 font-semibold">Recent Activity</h2>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => setAllEventsOpen(true)}
                disabled={events.length === 0}
                className="shrink-0 font-medium hover:cursor-grab"
              >
                View more
              </Button>
            </div>
            <div className="text-muted-foreground mt-0.5 text-xs leading-4">
              Profile-shaping signals from your recent practice.
            </div>
          </div>
          {events.length > 0 ? (
            <div className="flex flex-col gap-1.5">
              {visibleEvents.map((event) => {
                const presentation = eventPresentation(event);
                const EventIcon = presentation.icon;
                const active = allEventsOpen && activeAllEventId === event.id;
                return (
                  <button
                    type="button"
                    key={event.id}
                    onClick={() => {
                      setActiveAllEventId(event.id);
                      setAllEventsOpen(true);
                    }}
                    className="group relative rounded-lg text-left outline-none focus-visible:ring-2 focus-visible:ring-ring/60"
                  >
                    <div
                      className={[
                        'relative flex gap-3 rounded-lg border-l px-2 py-2.5 pl-4 transition-colors',
                        active ? 'bg-muted/45 ring-1 ring-border/70' : 'hover:bg-muted/35 hover:ring-1 hover:ring-border/60',
                      ].join(' ')}
                    >
                      <span className="bg-background absolute -left-[7px] top-0 flex size-3.5 items-center justify-center rounded-full border">
                        <span className="bg-muted-foreground size-1 rounded-full" />
                      </span>
                      <EventIcon className="text-muted-foreground mt-0.5 shrink-0" size={15} strokeWidth={1.8} />
                      <div className="min-w-0">
                        <div className="text-xs font-medium">{presentation.label}</div>
                        <p className="text-muted-foreground mt-0.5 text-xs leading-4">{event.summary}</p>
                        <div className="text-muted-foreground mt-1 text-[11px]">
                          {formatRelativeDate(event.occurred_at)}
                        </div>
                      </div>
                      <span className="text-muted-foreground/0 group-hover:text-muted-foreground ml-auto mt-0.5 text-[11px] transition-colors">
                        Details
                      </span>
                    </div>
                  </button>
                );
              })}
            </div>
          ) : (
            <p className="text-muted-foreground text-sm leading-5">
              Activity appears here after you answer questions and finish sessions.
            </p>
          )}
        </section>
      </aside>

      <Dialog
        open={allEventsOpen}
        onOpenChange={(open) => {
          setAllEventsOpen(open);
          if (!open) {
            setActiveAllEventId(null);
          }
        }}
      >
        <DialogContent
          showCloseButton={false}
          className="left-auto top-0 right-0 h-full w-full max-w-none translate-x-0 translate-y-0 gap-0 rounded-none border-l p-0 sm:w-[540px] sm:max-w-none lg:w-[600px]"
        >
          <DialogHeader className="border-b px-5 py-4 text-left">
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0">
                <DialogTitle className="text-base leading-6 font-semibold">All memory events</DialogTitle>
                <DialogDescription className="mt-1 text-xs leading-4">
                  {events.length} total {events.length === 1 ? 'event' : 'events'}
                </DialogDescription>
              </div>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                className="size-8 shrink-0"
                onClick={() => setAllEventsOpen(false)}
                aria-label="Close all memory events"
              >
                <X size={16} />
              </Button>
            </div>
          </DialogHeader>

          <div className="min-h-0 flex-1 overflow-y-auto">
            {events.length > 0 ? (
              <div className="divide-y">
                {events.map((event) => {
                  const presentation = eventPresentation(event);
                  const EventIcon = presentation.icon;
                  const expanded = activeAllEventId === event.id;
                  const payloadText = stringifyPayload(event.payload);
                  const detail = eventDetails(event);

                  return (
                    <div
                      key={`all-${event.id}`}
                      onClick={() =>
                        setActiveAllEventId((current) => (current === event.id ? null : event.id))
                      }
                      role="button"
                      tabIndex={0}
                      onKeyDown={(eventKey) => {
                        if (eventKey.key === 'Enter' || eventKey.key === ' ') {
                          eventKey.preventDefault();
                          setActiveAllEventId((current) => (current === event.id ? null : event.id));
                        }
                      }}
                      aria-expanded={expanded}
                      className={[
                        'group cursor-default hover:cursor-grab rounded-md border border-transparent px-4 transition-all duration-200 ease-out outline-none',
                        expanded
                          ? 'border-border bg-muted/45 py-3.5 shadow-sm'
                          : 'py-2.5 hover:border-border/70 hover:bg-muted/35 hover:shadow-sm',
                      ].join(' ')}
                    >
                      <div className="flex gap-3">
                        <EventIcon
                          className={[
                            'mt-0.5 shrink-0 transition-colors duration-200',
                            expanded ? 'text-foreground' : 'text-muted-foreground',
                          ].join(' ')}
                          size={15}
                          strokeWidth={1.8}
                        />
                        <div className="min-w-0 flex-1">
                          <div className="flex items-start justify-between gap-3">
                            <div className="min-w-0">
                              <div className="text-xs font-semibold">{presentation.label}</div>
                              <p className="text-muted-foreground mt-0.5 line-clamp-2 text-xs leading-4">{event.summary}</p>
                              <div className="mt-1.5 flex flex-wrap items-center gap-1.5">
                                <span className="rounded-full border bg-background px-2 py-0.5 text-[10px] font-medium">
                                  {formatEventType(event.type)}
                                </span>
                                <span className="rounded-full border bg-background px-2 py-0.5 text-[10px] font-medium capitalize">
                                  {event.source}
                                </span>
                                <span
                                  className={[
                                    'rounded-full border px-2 py-0.5 text-[10px] font-medium',
                                    detail.outcome.className,
                                  ].join(' ')}
                                >
                                  {detail.outcome.label}
                                </span>
                              </div>
                            </div>
                            <div className="text-muted-foreground shrink-0 pt-0.5 text-[11px]">
                              {formatRelativeDate(event.occurred_at)}
                            </div>
                          </div>

                          <div
                            className={[
                              'grid transition-all duration-200 ease-out',
                              expanded ? 'mt-2.5 grid-rows-[1fr] opacity-100' : 'mt-0 grid-rows-[0fr] opacity-0',
                            ].join(' ')}
                          >
                            <div className="overflow-hidden">
                              <div className="space-y-2.5">
                                {detail.sections.map((section) => (
                                  <div key={`${event.id}-${section.title}`} className="rounded-md border bg-background/70 px-3 py-2">
                                    <div className="text-muted-foreground text-[11px] font-semibold tracking-wide uppercase">
                                      {section.title}
                                    </div>
                                    <dl className="mt-1.5 space-y-1.5 text-[11px] leading-4">
                                      {section.rows.map((row) => {
                                        const idLike = row.label.endsWith('ID');
                                        return (
                                          <div
                                            key={`${event.id}-${section.title}-${row.label}`}
                                            className="grid grid-cols-[max-content_minmax(0,1fr)] items-start gap-2"
                                          >
                                            <dt className="text-muted-foreground shrink-0">{row.label}</dt>
                                            <dd
                                              className={[
                                                'min-w-0 text-right font-medium',
                                                idLike
                                                  ? 'font-mono text-[10.5px] truncate'
                                                  : 'break-words',
                                              ].join(' ')}
                                              title={row.value}
                                            >
                                              {row.value}
                                            </dd>
                                          </div>
                                        );
                                      })}
                                    </dl>
                                  </div>
                                ))}

                                <div className="rounded-md border bg-background/70 px-3 py-2" onClick={(e) => e.stopPropagation()}>
                                  <div className="text-muted-foreground text-[11px] font-semibold tracking-wide uppercase">
                                    Raw payload JSON
                                  </div>
                                  <pre className="mt-1.5 rounded-sm border border-neutral-700 bg-neutral-800 px-2 py-1.5 text-[11px] leading-4 whitespace-pre-wrap text-neutral-100 shadow-[inset_0_1px_0_rgba(255,255,255,0.06)]">
                                    {payloadText}
                                  </pre>
                                </div>
                              </div>
                            </div>
                          </div>
                        </div>
                      </div>
                    </div>
                  );
                })}
              </div>
            ) : (
              <div className="px-5 py-6">
                <p className="text-muted-foreground text-sm leading-5">
                  Activity appears here after you answer questions and finish sessions.
                </p>
              </div>
            )}
          </div>
        </DialogContent>
      </Dialog>
    </>
  );
}
