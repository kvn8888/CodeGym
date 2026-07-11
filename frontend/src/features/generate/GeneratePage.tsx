import { useEffect, useMemo, useState, type FormEvent } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { ArrowRight, Brain, Check, Clock3, ListChecks } from 'lucide-react';

import { api } from '../../shared/api/client';
import type {
  NewPracticeConfig,
  PracticeSession,
  PracticeSessionSummary,
  UserMemoryProfile,
} from '../../shared/api/types';
import {
  WorkspacePage,
  WorkspacePageHeader,
  WorkspaceSectionHeader,
} from '../../shared/components/WorkspacePage';
import { Button } from '@/components/ui/button';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group';

const questionCounts = [5, 10, 15];
const difficulties: Array<{ value: NewPracticeConfig['difficulty']; label: string }> = [
  { value: 'easy', label: 'Easy' },
  { value: 'medium', label: 'Medium' },
  { value: 'hard', label: 'Hard' },
];

function sessionTitle(prompt: string) {
  const normalized = prompt.trim().replace(/\s+/g, ' ');
  if (!normalized) return 'Personalized MCQ practice';
  return normalized.length > 64 ? `${normalized.slice(0, 61)}...` : normalized;
}

function formatRelativeDate(value: string) {
  const minutes = Math.round((new Date(value).getTime() - Date.now()) / 60_000);
  const formatter = new Intl.RelativeTimeFormat('en', { numeric: 'auto' });
  if (Math.abs(minutes) < 60) return formatter.format(minutes, 'minute');
  const hours = Math.round(minutes / 60);
  if (Math.abs(hours) < 24) return formatter.format(hours, 'hour');
  return formatter.format(Math.round(hours / 24), 'day');
}

export function GeneratePage() {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const [prompt, setPrompt] = useState(() => searchParams.get('prompt') ?? '');
  const [difficulty, setDifficulty] = useState<NewPracticeConfig['difficulty']>('medium');
  const [count, setCount] = useState(5);
  const [profile, setProfile] = useState<UserMemoryProfile | null>(null);
  const [recentSessions, setRecentSessions] = useState<PracticeSessionSummary[]>([]);
  const [starting, setStarting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    void Promise.allSettled([
      api.get<UserMemoryProfile>('/memory/profile'),
      api.get<PracticeSessionSummary[]>('/sessions?kind=mcq&limit=5'),
    ]).then(([profileResult, sessionsResult]) => {
      if (cancelled) return;
      if (profileResult.status === 'fulfilled') setProfile(profileResult.value);
      if (sessionsResult.status === 'fulfilled') setRecentSessions(sessionsResult.value);
    });
    return () => {
      cancelled = true;
    };
  }, []);

  const contextItems = useMemo(() => {
    if (!profile) return [];
    return [
      ...profile.growth_edges.slice(0, 2).map((label) => ({ label, source: 'Focus area' })),
      ...profile.notes
        .filter((note) => note.action === 'review')
        .slice(0, 2)
        .map((note) => ({ label: note.title, source: 'Memory note' })),
    ].slice(0, 4);
  }, [profile]);

  const startPractice = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (starting) return;

    const config: NewPracticeConfig = {
      prompt: prompt.trim(),
      difficulty,
      count,
    };

    setStarting(true);
    setError(null);
    try {
      const session = await api.post<PracticeSession>('/sessions', {
        kind: 'mcq',
        title: sessionTitle(config.prompt),
        state: {
          schema_version: 1,
          ...config,
          round: 1,
          question_index: 0,
          elapsed: 0,
          results: [],
        },
      });
      navigate(`/marathon?session=${encodeURIComponent(session.id)}`, {
        state: { newPractice: { sessionId: session.id, config } },
      });
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not start practice.');
      setStarting(false);
    }
  };

  return (
    <WorkspacePage>
      <WorkspacePageHeader
        title="New practice"
        description="Set the focus for a personalized multiple-choice session."
      />

      <form onSubmit={(event) => void startPractice(event)}>
        <div className="grid gap-7 lg:grid-cols-[minmax(0,1fr)_320px]">
          <div className="min-w-0">
            <section className="overflow-hidden rounded-lg border">
              <div className="bg-muted/25 flex items-center justify-between border-b px-4 py-3">
                <div className="flex items-center gap-2 text-sm font-medium">
                  <ListChecks size={16} strokeWidth={1.8} />
                  Multiple choice
                </div>
                <span className="text-muted-foreground flex items-center gap-1.5 text-xs">
                  <Check size={14} strokeWidth={2} />
                  Available
                </span>
              </div>

              <div className="p-4 sm:p-5">
                <Label htmlFor="practice-prompt" className="text-sm font-medium">
                  What should this session focus on?
                </Label>
                <Textarea
                  id="practice-prompt"
                  value={prompt}
                  onChange={(event) => setPrompt(event.target.value)}
                  placeholder="Go concurrency patterns, channel ownership, and cancellation..."
                  maxLength={500}
                  rows={5}
                  className="mt-2 min-h-32 resize-none text-sm leading-5"
                  autoFocus
                />
                <div className="text-muted-foreground mt-1.5 flex justify-between gap-3 text-xs">
                  <span>Leave blank to practice your current focus areas.</span>
                  <span className="font-mono tabular-nums">{prompt.length}/500</span>
                </div>
              </div>

              <div className="grid border-t sm:grid-cols-2">
                <fieldset className="border-b p-4 sm:border-r sm:border-b-0 sm:p-5">
                  <legend className="mb-2 text-sm font-medium">Questions</legend>
                  <ToggleGroup
                    type="single"
                    value={String(count)}
                    onValueChange={(value) => value && setCount(Number(value))}
                    variant="outline"
                    className="justify-start"
                    aria-label="Question count"
                  >
                    {questionCounts.map((value) => (
                      <ToggleGroupItem key={value} value={String(value)} className="min-w-11">
                        {value}
                      </ToggleGroupItem>
                    ))}
                  </ToggleGroup>
                </fieldset>

                <fieldset className="p-4 sm:p-5">
                  <legend className="mb-2 text-sm font-medium">Difficulty</legend>
                  <ToggleGroup
                    type="single"
                    value={difficulty}
                    onValueChange={(value) => value && setDifficulty(value as NewPracticeConfig['difficulty'])}
                    variant="outline"
                    className="justify-start"
                    aria-label="Difficulty"
                  >
                    {difficulties.map((item) => (
                      <ToggleGroupItem key={item.value} value={item.value} className="px-3">
                        {item.label}
                      </ToggleGroupItem>
                    ))}
                  </ToggleGroup>
                </fieldset>
              </div>

              <div className="bg-muted/20 flex flex-col gap-3 border-t px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
                <div className="text-muted-foreground text-xs">
                  Progress is saved after every answer.
                </div>
                <Button type="submit" disabled={starting} className="sm:min-w-36">
                  {starting ? 'Starting' : 'Start practice'}
                  {!starting && <ArrowRight data-icon="inline-end" />}
                </Button>
              </div>
            </section>

            {error && (
              <div className="border-destructive/40 bg-destructive/5 text-destructive mt-4 rounded-md border px-3 py-2 text-sm">
                {error}
              </div>
            )}

            {recentSessions.length > 0 && (
              <section className="mt-7">
                <WorkspaceSectionHeader
                  title="Recent topics"
                  description="Reuse a previous focus without rebuilding the setup."
                />
                <div className="overflow-hidden rounded-lg border">
                  {recentSessions.slice(0, 4).map((session) => (
                    <button
                      key={session.id}
                      type="button"
                      onClick={() => setPrompt(session.title)}
                      className="hover:bg-muted/35 flex min-h-11 w-full items-center gap-3 border-b px-3 py-2 text-left last:border-b-0"
                    >
                      <Clock3 className="text-muted-foreground shrink-0" size={15} strokeWidth={1.8} />
                      <span className="min-w-0 flex-1 truncate text-sm">{session.title}</span>
                      <span className="text-muted-foreground shrink-0 text-xs">
                        {formatRelativeDate(session.last_activity_at)}
                      </span>
                    </button>
                  ))}
                </div>
              </section>
            )}
          </div>

          <aside className="min-w-0 lg:border-l lg:pl-7">
            <WorkspaceSectionHeader
              title="Personalization"
              description="Included automatically when CodeGym builds the set."
            />
            {contextItems.length > 0 ? (
              <div className="overflow-hidden rounded-lg border">
                {contextItems.map((item, index) => (
                  <button
                    key={`${item.source}-${item.label}`}
                    type="button"
                    onClick={() => setPrompt(item.label)}
                    className="hover:bg-muted/35 flex w-full gap-3 border-b px-3 py-3 text-left last:border-b-0"
                  >
                    <span className="text-muted-foreground mt-0.5 w-5 shrink-0 font-mono text-xs">
                      {String(index + 1).padStart(2, '0')}
                    </span>
                    <span className="min-w-0">
                      <span className="block text-sm leading-5">{item.label}</span>
                      <span className="text-muted-foreground mt-0.5 block text-xs">{item.source}</span>
                    </span>
                  </button>
                ))}
              </div>
            ) : (
              <div className="bg-muted/20 rounded-lg border border-dashed px-4 py-5">
                <Brain className="text-muted-foreground" size={18} strokeWidth={1.8} />
                <p className="text-muted-foreground mt-3 text-sm leading-5">
                  Finish a session to build personalized focus areas and memory notes.
                </p>
              </div>
            )}

            <div className="mt-5 border-t pt-4">
              <div className="text-muted-foreground text-xs leading-5">
                Difficulty and question count steer this session. Your learning history determines which concepts receive emphasis.
              </div>
            </div>
          </aside>
        </div>
      </form>
    </WorkspacePage>
  );
}
