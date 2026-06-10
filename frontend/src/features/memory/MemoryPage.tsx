import { useEffect, useState } from 'react';
import { api } from '../../shared/api/client';
import type { UserMemoryProfile } from '../../shared/api/types';
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

export function MemoryPage() {
  const [profile, setProfile] = useState<UserMemoryProfile | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api
      .get<UserMemoryProfile>('/memory/profile')
      .then((data) => {
        setProfile(data);
        setError(null);
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : 'Could not load memory profile.');
      })
      .finally(() => setLoading(false));
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
          className="rounded-2xl bg-shell px-5 py-4 text-xs text-rust cg-surface"
        >
          {error ?? 'No memory profile available.'}
        </div>
      </div>
    );
  }

  return (
    <div className="max-w-5xl mx-auto px-6 py-8">
      <div className="mb-8 flex items-start justify-between gap-6">
        <div>
          <h1 className="font-display text-4xl font-semibold text-ink tracking-tight">Memory<span className="text-tangerine">.</span></h1>
          <p className="mt-2 max-w-3xl text-xs text-graphite leading-7">{profile.summary}</p>
        </div>
        <div className="shrink-0 rounded-2xl bg-shell px-4 py-3 text-right cg-surface">
          <div className="text-[10px] font-bold tracking-[0.15em] text-tangerine-deep uppercase">Next review</div>
          <div className="mt-1 text-xs text-ink">{formatDate(profile.next_review_at)}</div>
        </div>
      </div>

      <div className="grid grid-cols-2 gap-4">
        <section
          className="rounded-2xl bg-shell p-5 cg-surface"
        >
          <h2 className="text-[10px] font-bold tracking-[0.15em] text-tangerine-deep uppercase">Strengths</h2>
          <div className="mt-4 flex flex-col gap-3">
            {profile.strengths.map((strength) => (
              <div key={strength} className="flex gap-3 text-xs text-graphite leading-6">
                <span className="mt-2 h-1.5 w-1.5 shrink-0 rounded-full bg-moss" />
                <span>{strength}</span>
              </div>
            ))}
          </div>
        </section>

        <section
          className="rounded-2xl bg-shell p-5 cg-surface"
        >
          <h2 className="text-[10px] font-bold tracking-[0.15em] text-tangerine-deep uppercase">Growth edges</h2>
          <div className="mt-4 flex flex-col gap-3">
            {profile.growth_edges.map((edge) => (
              <div key={edge} className="flex gap-3 text-xs text-graphite leading-6">
                <span className="mt-2 h-1.5 w-1.5 shrink-0 rounded-full bg-rust" />
                <span>{edge}</span>
              </div>
            ))}
          </div>
        </section>
      </div>

      <section className="mt-6">
        <div className="mb-3 flex items-center justify-between">
          <h2 className="text-[10px] font-bold tracking-[0.15em] text-tangerine-deep uppercase">Skill profile</h2>
          <span className="text-[10px] text-ash">Updated {formatDate(profile.updated_at)}</span>
        </div>
        <div className="grid grid-cols-2 gap-3">
          {profile.skills.map((skill) => (
            <div
              key={skill.id}
              className="rounded-2xl bg-shell px-5 py-4 cg-surface"
            >
              <div className="flex items-start justify-between gap-4">
                <div>
                  <div className="text-sm font-medium text-ink">{skill.label}</div>
                  <div className="mt-1 text-[10px] tracking-[0.1em] text-ash uppercase">{skill.area}</div>
                </div>
                <div className="text-right">
                  <div className="text-xs text-ink">L{skill.level}</div>
                  <div className="mt-1 text-[10px] text-ash">{trendLabel(skill.trend)}</div>
                </div>
              </div>
              <div className="mt-4 h-1.5 rounded-full bg-grain">
                <div
                  className="h-1.5 rounded-full bg-tangerine"
                  style={{ width: `${skill.confidence}%` }}
                />
              </div>
            </div>
          ))}
        </div>
      </section>

      <section className="mt-6">
        <h2 className="mb-3 text-[10px] font-bold tracking-[0.15em] text-tangerine-deep uppercase">Problem notes</h2>
        <div className="flex flex-col gap-3">
          {profile.notes.map((note) => (
            <article
              key={note.id}
              className="rounded-2xl bg-shell px-5 py-4 cg-surface"
            >
              <div className="flex items-start justify-between gap-4">
                <div>
                  <h3 className="text-sm font-medium text-ink">{note.title}</h3>
                  <p className="mt-2 text-xs leading-6 text-graphite">{note.summary}</p>
                </div>
                <span
                  className="shrink-0 rounded-full px-3 py-1 text-[10px] font-bold"
                  style={{
                    color: note.action === 'keep' ? 'var(--color-moss)' : note.action === 'review' ? 'var(--color-honey)' : 'var(--color-rust)',
                    backgroundColor: note.action === 'keep' ? 'var(--color-moss-tint)' : note.action === 'review' ? 'var(--color-honey-tint)' : 'var(--color-rust-tint)',
                    border: '1px solid currentColor',
                  }}
                >
                  {actionLabels[note.action]}
                </span>
              </div>
              <div className="mt-3 flex flex-wrap gap-2">
                {note.tags.map((tag) => (
                  <span key={tag} className="rounded-full bg-grain px-2.5 py-1 text-[10px] text-graphite">
                    {tag}
                  </span>
                ))}
              </div>
            </article>
          ))}
        </div>
      </section>
    </div>
  );
}
