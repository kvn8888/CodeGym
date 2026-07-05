import { useEffect, useState } from 'react';
import { motion } from 'motion/react';
import { api } from '../../shared/api/client';
import type { MemoryEvent, UserMemoryProfile } from '../../shared/api/types';
import { GridSpinner } from '../../shared/components/GridSpinner';

const actionLabels: Record<string, string> = {
  keep: 'Keep',
  review: 'Review',
  prune: 'Prune',
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
        api.get<UserMemoryProfile>('/memory/profile'),
        api.get<MemoryEvent[]>('/memory/events'),
      ]);

      if (cancelled) {
        return;
      }

      if (profileResult.status === 'fulfilled') {
        setProfile(profileResult.value);
        setError(null);
      } else {
        setError(
          profileResult.reason instanceof Error
            ? profileResult.reason.message
            : 'Could not load memory profile.',
        );
      }

      if (eventsResult.status === 'fulfilled') {
        const sorted = [...eventsResult.value].sort(
          (a, b) => new Date(b.occurred_at).getTime() - new Date(a.occurred_at).getTime(),
        );
        setEvents(sorted);
        setEventsError(null);
      } else {
        setEvents([]);
        setEventsError(
          eventsResult.reason instanceof Error
            ? eventsResult.reason.message
            : 'Could not load memory events.',
        );
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
      <div className="max-w-xl mx-auto px-6 py-16">
        <div
          className="rounded-xl border border-red-400 bg-red-100 px-5 py-4 text-sm text-red-900"
        >
          {error ?? 'No memory profile available.'}
        </div>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-5xl px-6 py-10">
      <div className="mb-8 flex items-start justify-between gap-6">
        <div>
          <h1 className="text-[40px] font-semibold leading-[48px] tracking-[-2.4px] text-gray-1000">Memory</h1>
          <p className="mt-2 max-w-3xl text-sm leading-6 text-gray-900">{profile.summary}</p>
        </div>
        <div className="shrink-0 rounded-xl border border-gray-alpha-200 bg-background-100 px-4 py-3 text-right" style={{ boxShadow: 'var(--cg-card-shadow)' }}>
          <div className="text-xs text-gray-700">Next Review</div>
          <div className="mt-1 text-sm font-medium text-gray-1000">{formatDate(profile.next_review_at)}</div>
        </div>
      </div>

      <div className="flex flex-col gap-4 md:flex-row">
        <section
          className="flex-1 rounded-xl border border-gray-alpha-200 bg-background-100 p-5"
          style={{ boxShadow: 'var(--cg-card-shadow)' }}
        >
          <h2 className="text-sm font-semibold text-gray-1000">Strengths</h2>
          <div className="mt-4 flex flex-col gap-3">
            {profile.strengths.map((strength) => (
              <div key={strength} className="flex gap-3 text-sm leading-6 text-gray-900">
                <span className="mt-2 h-1.5 w-1.5 shrink-0 rounded-full bg-green-700" />
                <span>{strength}</span>
              </div>
            ))}
          </div>
        </section>

        <section
          className="flex-1 rounded-xl border border-gray-alpha-200 bg-background-100 p-5"
          style={{ boxShadow: 'var(--cg-card-shadow)' }}
        >
          <h2 className="text-sm font-semibold text-gray-1000">Growth Edges</h2>
          <div className="mt-4 flex flex-col gap-3">
            {profile.growth_edges.map((edge) => (
              <div key={edge} className="flex gap-3 text-sm leading-6 text-gray-900">
                <span className="mt-2 h-1.5 w-1.5 shrink-0 rounded-full bg-red-800" />
                <span>{edge}</span>
              </div>
            ))}
          </div>
        </section>
      </div>

      <section className="mt-6">
        <div className="mb-3 flex items-center justify-between">
          <h2 className="text-sm font-semibold text-gray-1000">Skill Profile</h2>
          <span className="text-xs text-gray-700">Updated {formatDate(profile.updated_at)}</span>
        </div>
        <div className="flex flex-col gap-3 md:flex-row md:flex-wrap">
          {profile.skills.map((skill, index) => (
            <motion.div
              key={skill.id}
              initial={{ opacity: 0, y: 12 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ type: 'spring', stiffness: 320, damping: 30, delay: index * 0.04 }}
              className="min-w-[260px] flex-1 rounded-xl border border-gray-alpha-200 bg-background-100 px-5 py-4"
              style={{ boxShadow: 'var(--cg-card-shadow)' }}
            >
              <div className="flex items-start justify-between gap-4">
                <div>
                  <div className="text-sm font-medium text-gray-1000">{skill.label}</div>
                  <div className="mt-1 text-xs text-gray-700">{skill.area}</div>
                </div>
                <div className="text-right">
                  <div className="font-mono text-xs text-gray-1000">L{skill.level}</div>
                  <div className="mt-1 text-xs text-gray-700">{trendLabel(skill.trend)}</div>
                </div>
              </div>
              <div className="mt-4 h-1.5 rounded-full bg-gray-100">
                <div
                  className="h-1.5 rounded-full bg-blue-700"
                  style={{ width: `${skill.confidence}%` }}
                />
              </div>
            </motion.div>
          ))}
        </div>
      </section>

      <section className="mt-6">
        <h2 className="mb-3 text-sm font-semibold text-gray-1000">Problem Notes</h2>
        <div className="flex flex-col gap-3">
          {profile.notes.map((note) => (
            <article
              key={note.id}
              className="rounded-xl border border-gray-alpha-200 bg-background-100 px-5 py-4"
              style={{ boxShadow: 'var(--cg-card-shadow)' }}
            >
              <div className="flex items-start justify-between gap-4">
                <div>
                  <h3 className="text-sm font-medium text-gray-1000">{note.title}</h3>
                  <p className="mt-2 text-sm leading-6 text-gray-900">{note.summary}</p>
                </div>
                <span
                  className="shrink-0 rounded-full px-3 py-1 text-xs font-medium"
                  style={{
                    color: note.action === 'keep' ? 'var(--color-green-900)' : note.action === 'review' ? 'var(--color-amber-900)' : 'var(--color-red-900)',
                    backgroundColor: note.action === 'keep' ? 'var(--color-green-100)' : note.action === 'review' ? 'var(--color-amber-100)' : 'var(--color-red-100)',
                  }}
                >
                  {actionLabels[note.action]}
                </span>
              </div>
              <div className="mt-3 flex flex-wrap gap-2">
                {note.tags.map((tag) => (
                  <span key={tag} className="rounded-full bg-gray-100 px-2.5 py-1 font-mono text-xs text-gray-900">
                    {tag}
                  </span>
                ))}
              </div>
            </article>
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
