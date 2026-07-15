import { useCallback, useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import {
  ArrowRight,
  Brain,
  CheckCircle2,
  Clock3,
  MoreHorizontal,
  RefreshCw,
  RotateCcw,
  TrendingDown,
  TrendingUp,
} from 'lucide-react';

import { api } from '../../shared/api/client';
import type {
  MemoryEvent,
  MemoryNote,
  SkillProficiency,
  UserMemoryProfile,
} from '../../shared/api/types';
import { GridSpinner } from '../../shared/components/GridSpinner';
import {
  WorkspaceEmptyState,
  WorkspacePage,
  WorkspacePageHeader,
  WorkspaceSectionHeader,
} from '../../shared/components/WorkspacePage';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Progress } from '@/components/ui/progress';
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { cn } from '@/lib/utils';

type NoteFilter = 'all' | MemoryNote['action'];

const actionLabels: Record<MemoryNote['action'], string> = {
  keep: 'Keep',
  review: 'Review',
  prune: 'Prune',
};

const actionDots: Record<MemoryNote['action'], string> = {
  keep: 'bg-green-700',
  review: 'bg-amber-700',
  prune: 'bg-red-700',
};

function formatDate(value: string) {
  return new Intl.DateTimeFormat('en', {
    month: 'short',
    day: 'numeric',
    year: new Date(value).getFullYear() === new Date().getFullYear() ? undefined : 'numeric',
  }).format(new Date(value));
}

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

function trendPresentation(trend: SkillProficiency['trend']) {
  if (trend === 'up') return { label: 'Improving', icon: TrendingUp, className: 'text-green-800' };
  if (trend === 'down') return { label: 'Needs reps', icon: TrendingDown, className: 'text-amber-800' };
  return { label: 'Stable', icon: RotateCcw, className: 'text-muted-foreground' };
}

function eventPresentation(event: MemoryEvent) {
  if (event.type.includes('completed')) return { label: 'Session', icon: CheckCircle2 };
  if (event.type.includes('note')) return { label: 'Memory', icon: Brain };
  return { label: 'Practice', icon: Clock3 };
}

export function MemoryPage() {
  const [profile, setProfile] = useState<UserMemoryProfile | null>(null);
  const [events, setEvents] = useState<MemoryEvent[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [noteFilter, setNoteFilter] = useState<NoteFilter>('all');

  const loadMemory = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [nextProfile, nextEvents] = await Promise.all([
        api.get<UserMemoryProfile>('/memory/profile'),
        api.get<MemoryEvent[]>('/memory/events'),
      ]);
      setProfile(nextProfile);
      setEvents(nextEvents);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not load memory.');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    let cancelled = false;
    void Promise.all([
      api.get<UserMemoryProfile>('/memory/profile'),
      api.get<MemoryEvent[]>('/memory/events'),
    ])
      .then(([nextProfile, nextEvents]) => {
        if (cancelled) return;
        setProfile(nextProfile);
        setEvents(nextEvents);
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(err instanceof Error ? err.message : 'Could not load memory.');
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const refreshMemory = async () => {
    setRefreshing(true);
    setError(null);
    try {
      const nextProfile = await api.post<UserMemoryProfile>('/memory/profile/refresh', {});
      setProfile(nextProfile);
      const nextEvents = await api.get<MemoryEvent[]>('/memory/events');
      setEvents(nextEvents);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not refresh memory.');
    } finally {
      setRefreshing(false);
    }
  };

  const filteredNotes = useMemo(() => {
    if (!profile) return [];
    if (noteFilter === 'all') return profile.notes;
    return profile.notes.filter((note) => note.action === noteFilter);
  }, [noteFilter, profile]);

  if (loading) {
    return (
      <WorkspacePage className="flex min-h-[60vh] items-center justify-center">
        <GridSpinner size="md" />
      </WorkspacePage>
    );
  }

  if (error || !profile) {
    return (
      <WorkspacePage>
        <WorkspacePageHeader title="Memory" />
        <WorkspaceEmptyState
          icon={<Brain size={20} strokeWidth={1.8} />}
          title="Memory is unavailable"
          description={error ?? 'CodeGym could not load your learning profile.'}
          action={
            <Button variant="outline" size="sm" onClick={() => void loadMemory()}>
              <RefreshCw data-icon="inline-start" />
              Try again
            </Button>
          }
        />
      </WorkspacePage>
    );
  }

  return (
    <WorkspacePage>
      <WorkspacePageHeader
        title="Memory"
        description={profile.summary}
        actions={
          <>
            <span className="text-muted-foreground hidden text-xs sm:inline">
              Updated {formatRelativeDate(profile.updated_at)}
            </span>
            <Button variant="outline" size="sm" onClick={() => void refreshMemory()} disabled={refreshing}>
              <RefreshCw data-icon="inline-start" className={cn(refreshing && 'animate-spin')} />
              {refreshing ? 'Refreshing' : 'Refresh'}
            </Button>
          </>
        }
      />

      {error && (
        <div className="border-destructive/40 bg-destructive/5 text-destructive mb-5 rounded-md border px-3 py-2 text-sm">
          {error}
        </div>
      )}

      <div className="grid gap-7 lg:grid-cols-[minmax(0,1fr)_320px]">
        <div className="min-w-0">
          <section>
            <WorkspaceSectionHeader
              title="Focus next"
              description="The highest-value concepts to reinforce in your next session."
            />
            {profile.growth_edges.length > 0 ? (
              <div className="overflow-hidden rounded-lg border">
                {profile.growth_edges.map((edge, index) => (
                  <div
                    key={edge}
                    className="group flex min-h-12 items-center gap-3 border-b px-3 py-2.5 last:border-b-0"
                  >
                    <span className="text-muted-foreground w-5 shrink-0 font-mono text-xs tabular-nums">
                      {String(index + 1).padStart(2, '0')}
                    </span>
                    <span className="min-w-0 flex-1 text-sm leading-5">{edge}</span>
                    <Button variant="ghost" size="sm" asChild className="shrink-0">
                      <Link to={`/generate?prompt=${encodeURIComponent(edge)}`}>
                        Practice
                        <ArrowRight data-icon="inline-end" />
                      </Link>
                    </Button>
                  </div>
                ))}
              </div>
            ) : (
              <WorkspaceEmptyState
                title="No focus areas yet"
                description="Finish a practice session and CodeGym will identify what deserves another pass."
                className="min-h-32"
              />
            )}
          </section>

          <section className="mt-7">
            <WorkspaceSectionHeader
              title="Skill profile"
              description={`${profile.skills.length} tracked ${profile.skills.length === 1 ? 'skill' : 'skills'}`}
            />
            {profile.skills.length > 0 ? (
              <div className="overflow-hidden rounded-lg border">
                <div className="text-muted-foreground bg-muted/30 hidden grid-cols-[minmax(150px,1.3fr)_64px_minmax(120px,1fr)_112px_96px] gap-3 border-b px-3 py-2 text-xs lg:grid">
                  <span>Skill</span>
                  <span>Level</span>
                  <span>Confidence</span>
                  <span>Trend</span>
                  <span>Last practiced</span>
                </div>
                {profile.skills.map((skill) => {
                  const trend = trendPresentation(skill.trend);
                  const TrendIcon = trend.icon;
                  return (
                    <div
                      key={skill.id}
                      className="grid min-h-14 grid-cols-[minmax(0,1fr)_auto] items-center gap-3 border-b px-3 py-2.5 last:border-b-0 lg:grid-cols-[minmax(150px,1.3fr)_64px_minmax(120px,1fr)_112px_96px]"
                    >
                      <div className="min-w-0">
                        <div className="truncate text-sm font-medium">{skill.label}</div>
                        <div className="text-muted-foreground mt-0.5 text-xs capitalize">{skill.area}</div>
                      </div>
                      <div className="font-mono text-xs lg:text-sm">L{skill.level}</div>
                      <div className="col-span-2 flex min-w-0 items-center gap-2 lg:col-span-1">
                        <Progress value={skill.confidence} className="h-1.5 flex-1" />
                        <span className="text-muted-foreground w-8 text-right font-mono text-xs tabular-nums">
                          {skill.confidence}%
                        </span>
                      </div>
                      <div className={cn('flex items-center gap-1.5 text-xs', trend.className)}>
                        <TrendIcon size={14} strokeWidth={1.8} />
                        {trend.label}
                      </div>
                      <div className="text-muted-foreground hidden text-xs lg:block">
                        {formatDate(skill.last_practiced)}
                      </div>
                    </div>
                  );
                })}
              </div>
            ) : (
              <WorkspaceEmptyState
                title="No skills tracked yet"
                description="Skill confidence and trends appear after your first completed session."
                className="min-h-32"
              />
            )}
          </section>

          <section className="mt-7">
            <WorkspaceSectionHeader
              title="Memory notes"
              description="Durable observations CodeGym carries into future practice."
              actions={
                <Tabs value={noteFilter} onValueChange={(value) => setNoteFilter(value as NoteFilter)}>
                  <TabsList className="h-8" aria-label="Filter memory notes">
                    <TabsTrigger value="all" className="px-2.5 text-xs">All</TabsTrigger>
                    <TabsTrigger value="review" className="px-2.5 text-xs">Review</TabsTrigger>
                    <TabsTrigger value="keep" className="px-2.5 text-xs">Keep</TabsTrigger>
                  </TabsList>
                </Tabs>
              }
            />
            {filteredNotes.length > 0 ? (
              <div className="overflow-hidden rounded-lg border">
                {filteredNotes.map((note) => (
                  <div key={note.id} className="flex gap-3 border-b px-3 py-3 last:border-b-0">
                    <span className={cn('mt-1.5 size-2 shrink-0 rounded-full', actionDots[note.action])} />
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                        <h3 className="text-sm font-medium">{note.title}</h3>
                        <span className="text-muted-foreground text-xs">{actionLabels[note.action]}</span>
                      </div>
                      <p className="text-muted-foreground mt-1 text-sm leading-5">{note.summary}</p>
                      <div className="mt-2 flex flex-wrap items-center gap-1.5">
                        {note.tags.map((tag) => (
                          <Badge key={tag} variant="outline" className="rounded-md px-1.5 py-0 font-mono text-[10px] font-normal">
                            {tag}
                          </Badge>
                        ))}
                        <span className="text-muted-foreground ml-1 text-xs">{formatDate(note.created_at)}</span>
                      </div>
                    </div>
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <Button variant="ghost" size="icon" className="size-8 shrink-0" aria-label={`Actions for ${note.title}`}>
                          <MoreHorizontal />
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end">
                        <DropdownMenuGroup>
                          <DropdownMenuItem asChild>
                            <Link to={`/generate?prompt=${encodeURIComponent(note.title)}`}>Practice this</Link>
                          </DropdownMenuItem>
                          {note.problem_id && (
                            <DropdownMenuItem asChild>
                              <Link to={`/problems/${note.problem_id}`}>Open source problem</Link>
                            </DropdownMenuItem>
                          )}
                        </DropdownMenuGroup>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </div>
                ))}
              </div>
            ) : (
              <WorkspaceEmptyState
                title="No notes in this view"
                description="Choose another filter or finish more practice to grow your memory."
                className="min-h-32"
              />
            )}
          </section>
        </div>

        <aside className="min-w-0 lg:border-l lg:pl-7">
          <section>
            <WorkspaceSectionHeader title="What is working" />
            {profile.strengths.length > 0 ? (
              <div className="flex flex-col divide-y rounded-lg border">
                {profile.strengths.map((strength) => (
                  <div key={strength} className="flex gap-2.5 px-3 py-3 text-sm leading-5">
                    <CheckCircle2 className="mt-0.5 shrink-0 text-green-800" size={15} strokeWidth={1.8} />
                    <span>{strength}</span>
                  </div>
                ))}
              </div>
            ) : (
              <p className="text-muted-foreground text-sm leading-5">
                Strengths appear after CodeGym observes repeated success.
              </p>
            )}
          </section>

          <section className="mt-7">
            <WorkspaceSectionHeader
              title="Recent activity"
              description={`Next review ${formatRelativeDate(profile.next_review_at)}`}
            />
            {events.length > 0 ? (
              <div className="flex flex-col">
                {events.slice(0, 6).map((event) => {
                  const presentation = eventPresentation(event);
                  const EventIcon = presentation.icon;
                  return (
                    <div key={event.id} className="relative flex gap-3 border-l pb-5 pl-4 last:pb-0">
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
                    </div>
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
      </div>
    </WorkspacePage>
  );
}
