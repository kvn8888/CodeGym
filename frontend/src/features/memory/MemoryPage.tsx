import { useEffect, useState } from 'react';
import { motion } from 'motion/react';

import { api } from '../../shared/api/client';
import type { MemoryEvent, UserMemoryProfile } from '../../shared/api/types';
import { GridSpinner } from '../../shared/components/GridSpinner';
import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from '@/components/ui/accordion';
import { Badge } from '@/components/ui/badge';
import { Card } from '@/components/ui/card';
import { Progress } from '@/components/ui/progress';
import { cn } from '@/lib/utils';

const actionLabels: Record<string, string> = {
  keep: 'Keep',
  review: 'Review',
  prune: 'Prune',
};

const actionBadgeClass: Record<string, string> = {
  keep: 'bg-green-100 text-green-900',
  review: 'bg-amber-100 text-amber-900',
  prune: 'bg-red-100 text-red-900',
};

function formatDate(value: string) {
  return new Intl.DateTimeFormat('en', {
    month: 'short',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  }).format(new Date(value));
}

function trendLabel(trend: string) {
  if (trend === 'up') return 'Improving';
  if (trend === 'down') return 'Needs reps';
  return 'Stable';
}

function formatEventType(value: string) {
  return value
    .split('_')
    .filter(Boolean)
    .map((chunk) => `${chunk[0]?.toUpperCase() ?? ''}${chunk.slice(1)}`)
    .join(' ');
}

export function MemoryPage() {
  const [profile, setProfile] = useState<UserMemoryProfile | null>(null);
  const [events, setEvents] = useState<MemoryEvent[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [eventsError, setEventsError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;

    const loadMemory = async () => {
      setLoading(true);

      const [profileResult, eventsResult] = await Promise.allSettled([
        api.memory.getProfile(),
        api.memory.listEvents(),
      ]);

      if (cancelled) {
        return;
      }

      if (profileResult.status === 'fulfilled') {
        setProfile(profileResult.value);
        setError(null);
      } else {
        setError( profileResult.reason instanceof Error ? profileResult.reason.message : 'Could not load memory profile.', );
      }

      if (eventsResult.status === 'fulfilled') {
        const sorted = [...eventsResult.value].sort(
          (a, b) => new Date(b.occurred_at).getTime() - new Date(a.occurred_at).getTime(),
        );
        setEvents(sorted);
        setEventsError(null);
      } else {
        setEvents([]);
        setEventsError( eventsResult.reason instanceof Error ? eventsResult.reason.message : 'Could not load memory events.', );
      }

      setLoading(false);
    };

    loadMemory();

    return () => {
      cancelled = true;
    };
  }, []);

  if (loading) {
    return (
      <div className="flex justify-center py-16">
        <GridSpinner size="md" />
      </div>
    );
  }

  if (error || !profile) {
    return (
      <div className="mx-auto max-w-xl px-6 py-16">
        <div className="border-destructive/40 bg-destructive/10 text-destructive rounded-xl border px-5 py-4 text-sm">
          {error ?? 'No memory profile available.'}
        </div>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-5xl px-6 py-10">
      <div className="mb-8 flex items-start justify-between gap-6">
        <div>
          <h1 className="text-[40px] font-semibold leading-[48px] tracking-[-2.4px]">Memory</h1>
          <p className="text-muted-foreground mt-2 max-w-3xl text-sm leading-6">{profile.summary}</p>
        </div>
        <Card className="shrink-0 gap-0 px-4 py-3 text-right">
          <div className="text-muted-foreground text-xs">Next Review</div>
          <div className="mt-1 text-sm font-medium">{formatDate(profile.next_review_at)}</div>
        </Card>
      </div>

      <div className="flex flex-col gap-4 md:flex-row">
        <Card className="flex-1 gap-0 p-5">
          <h2 className="text-sm font-semibold">Strengths</h2>
          <div className="mt-4 flex flex-col gap-3">
            {profile.strengths.map((strength) => (
              <div key={strength} className="text-muted-foreground flex gap-3 text-sm leading-6">
                <span className="mt-2 h-1.5 w-1.5 shrink-0 rounded-full bg-green-700" />
                <span>{strength}</span>
              </div>
            ))}
          </div>
        </Card>

        <Card className="flex-1 gap-0 p-5">
          <h2 className="text-sm font-semibold">Growth Edges</h2>
          <div className="mt-4 flex flex-col gap-3">
            {profile.growth_edges.map((edge) => (
              <div key={edge} className="text-muted-foreground flex gap-3 text-sm leading-6">
                <span className="mt-2 h-1.5 w-1.5 shrink-0 rounded-full bg-red-800" />
                <span>{edge}</span>
              </div>
            ))}
          </div>
        </Card>
      </div>

      <section className="mt-6">
        <div className="mb-3 flex items-center justify-between">
          <h2 className="text-sm font-semibold">Skill Profile</h2>
          <span className="text-muted-foreground text-xs">Updated {formatDate(profile.updated_at)}</span>
        </div>
        <div className="flex flex-col gap-3 md:flex-row md:flex-wrap">
          {profile.skills.map((skill, index) => (
            <motion.div
              key={skill.id}
              initial={{ opacity: 0, y: 12 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ type: 'spring', stiffness: 320, damping: 30, delay: index * 0.04 }}
              className="min-w-[260px] flex-1"
            >
              <Card className="gap-0 px-5 py-4">
                <div className="flex items-start justify-between gap-4">
                  <div>
                    <div className="text-sm font-medium">{skill.label}</div>
                    <div className="text-muted-foreground mt-1 text-xs">{skill.area}</div>
                  </div>
                  <div className="text-right">
                    <div className="font-mono text-xs">L{skill.level}</div>
                    <div className="text-muted-foreground mt-1 text-xs">{trendLabel(skill.trend)}</div>
                  </div>
                </div>
                <Progress value={skill.confidence} className="mt-4 h-1.5" />
              </Card>
            </motion.div>
          ))}
        </div>
      </section>

      <section className="mt-6">
        <h2 className="mb-3 text-sm font-semibold">Problem Notes</h2>
        <div className="flex flex-col gap-3">
          {profile.notes.map((note) => (
            <Card key={note.id} className="gap-0 px-5 py-0">
              <Accordion type="single" collapsible>
                <AccordionItem value={note.id} className="border-b-0">
                  <AccordionTrigger className="py-4 hover:no-underline">
                    <div className="flex flex-1 items-center justify-between gap-4">
                      <h3 className="text-sm font-medium">{note.title}</h3>
                      <Badge
                        variant="secondary"
                        className={cn('border-transparent', actionBadgeClass[note.action])}
                      >
                        {actionLabels[note.action]}
                      </Badge>
                    </div>
                  </AccordionTrigger>
                  <AccordionContent>
                    <p className="text-muted-foreground text-sm leading-6">{note.summary}</p>
                    <div className="mt-3 flex flex-wrap gap-2">
                      {note.tags.map((tag) => (
                        <span
                          key={tag}
                          className="bg-muted text-muted-foreground rounded-full px-2.5 py-1 font-mono text-xs"
                        >
                          {tag}
                        </span>
                      ))}
                    </div>
                  </AccordionContent>
                </AccordionItem>
              </Accordion>
            </Card>
          ))}
        </div>
      </section>

      <section className="mt-6">
        <div className="mb-3 flex items-center justify-between">
          <h2 className="text-sm font-semibold text-gray-1000">Recent Memory Events</h2>
          <span className="text-xs text-gray-700">{events.length} events</span>
        </div>

        {eventsError ? (
          <div className="mb-3 rounded-xl border border-amber-400 bg-amber-100 px-4 py-3 text-xs text-amber-900">
            {eventsError}
          </div>
        ) : null}

        <div className="flex flex-col gap-3">
          {events.map((event) => (
            <article
              key={event.id}
              className="rounded-xl border border-gray-alpha-200 bg-background-100 px-5 py-4"
              style={{ boxShadow: 'var(--cg-card-shadow)' }}
            >
              <div className="flex items-start justify-between gap-4">
                <div>
                  <h3 className="text-sm font-medium text-gray-1000">{event.summary}</h3>
                  <p className="mt-2 text-xs text-gray-700">
                    {event.source} • {formatEventType(event.type)}
                  </p>
                </div>
                <span className="shrink-0 text-xs text-gray-700">{formatDate(event.occurred_at)}</span>
              </div>
            </article>
          ))}

          {!events.length && !eventsError ? (
            <div className="rounded-xl border border-gray-alpha-200 bg-background-100 px-5 py-4 text-sm text-gray-900" style={{ boxShadow: 'var(--cg-card-shadow)' }}>
              No events yet. New attempts and memory updates will appear here.
            </div>
          ) : null}
        </div>
      </section>
    </div>
  );
}
