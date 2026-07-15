import { useCallback, useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import {
  ArrowRight,
  Brain,
  CheckCircle2,
  CircleDot,
  Code2,
  History,
  ListChecks,
  Plus,
  RefreshCw,
} from 'lucide-react';

import { api } from '../../shared/api/client';
import type {
  PracticeSessionSummary,
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
import { Progress } from '@/components/ui/progress';
import { cn } from '@/lib/utils';

function formatRelativeDate(value: string) {
  const minutes = Math.round((new Date(value).getTime() - Date.now()) / 60_000);
  const formatter = new Intl.RelativeTimeFormat('en', { numeric: 'auto' });
  if (Math.abs(minutes) < 60) return formatter.format(minutes, 'minute');
  const hours = Math.round(minutes / 60);
  if (Math.abs(hours) < 24) return formatter.format(hours, 'hour');
  return formatter.format(Math.round(hours / 24), 'day');
}

function sessionKindLabel(session: PracticeSessionSummary) {
  if (session.kind === 'mcq') return 'MCQ';
  if (session.kind === 'interview') return 'Interview';
  return 'Workspace';
}

function sessionIcon(session: PracticeSessionSummary) {
  return session.kind === 'mcq' ? ListChecks : Code2;
}

function sessionDestination(session: PracticeSessionSummary) {
  if (session.kind === 'mcq') return `/marathon?session=${encodeURIComponent(session.id)}`;
  if (session.problem_id) return `/problems/${encodeURIComponent(session.problem_id)}`;
  return '/';
}

function trendLabel(skill: SkillProficiency) {
  if (skill.trend === 'up') return 'Improving';
  if (skill.trend === 'down') return 'Needs reps';
  return 'Stable';
}

export function DashboardPage() {
  const [sessions, setSessions] = useState<PracticeSessionSummary[]>([]);
  const [profile, setProfile] = useState<UserMemoryProfile | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const loadDashboard = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [nextSessions, nextProfile] = await Promise.all([
        api.get<PracticeSessionSummary[]>('/sessions?limit=12'),
        api.get<UserMemoryProfile>('/memory/profile'),
      ]);
      setSessions(nextSessions);
      setProfile(nextProfile);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not load your dashboard.');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    let cancelled = false;
    void Promise.all([
      api.get<PracticeSessionSummary[]>('/sessions?limit=12'),
      api.get<UserMemoryProfile>('/memory/profile'),
    ])
      .then(([nextSessions, nextProfile]) => {
        if (cancelled) return;
        setSessions(nextSessions);
        setProfile(nextProfile);
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : 'Could not load your dashboard.');
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const activeSessions = useMemo(
    () => sessions.filter((session) => session.status === 'active').slice(0, 3),
    [sessions],
  );
  const recentSessions = useMemo(
    () => sessions.filter((session) => session.status !== 'active').slice(0, 7),
    [sessions],
  );
  const focusAreas = profile?.growth_edges.slice(0, 3) ?? [];
  const skills = useMemo(
    () => [...(profile?.skills ?? [])].sort((a, b) => a.confidence - b.confidence).slice(0, 4),
    [profile],
  );

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
        <WorkspacePageHeader title="Today" />
        <WorkspaceEmptyState
          icon={<History size={20} strokeWidth={1.8} />}
          title="Your workspace is unavailable"
          description={error ?? 'CodeGym could not load your sessions and learning profile.'}
          action={
            <Button variant="outline" size="sm" onClick={() => void loadDashboard()}>
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
        title="Today"
        description="Resume unfinished work or start with the concepts that need attention."
        actions={
          <Button size="sm" asChild>
            <Link to="/generate">
              <Plus data-icon="inline-start" />
              New practice
            </Link>
          </Button>
        }
      />

      <section>
        <WorkspaceSectionHeader
          title="Continue"
          description={activeSessions.length > 0 ? `${activeSessions.length} active ${activeSessions.length === 1 ? 'session' : 'sessions'}` : undefined}
        />
        {activeSessions.length > 0 ? (
          <div className="overflow-hidden rounded-lg border">
            {activeSessions.map((session) => {
              const SessionIcon = sessionIcon(session);
              return (
                <div
                  key={session.id}
                  className="flex min-h-14 items-center gap-3 border-b px-3 py-2.5 last:border-b-0"
                >
                  <span className="bg-muted flex size-8 shrink-0 items-center justify-center rounded-md">
                    <SessionIcon size={16} strokeWidth={1.8} />
                  </span>
                  <div className="min-w-0 flex-1">
                    <div className="truncate text-sm font-medium">{session.title}</div>
                    <div className="text-muted-foreground mt-0.5 flex items-center gap-2 text-xs">
                      <span>{sessionKindLabel(session)}</span>
                      <span aria-hidden="true">·</span>
                      <span>Last active {formatRelativeDate(session.last_activity_at)}</span>
                    </div>
                  </div>
                  <Badge variant="outline" className="hidden rounded-md font-normal sm:inline-flex">
                    <CircleDot className="text-amber-700" />
                    Active
                  </Badge>
                  <Button size="sm" asChild>
                    <Link to={sessionDestination(session)}>
                      Resume
                      <ArrowRight data-icon="inline-end" />
                    </Link>
                  </Button>
                </div>
              );
            })}
          </div>
        ) : (
          <WorkspaceEmptyState
            icon={<CheckCircle2 size={20} strokeWidth={1.8} />}
            title="Nothing waiting for you"
            description="Start a focused practice session and CodeGym will save your progress here."
            action={
              <Button size="sm" asChild>
                <Link to="/generate">Start practice</Link>
              </Button>
            }
            className="min-h-32"
          />
        )}
      </section>

      <div className="mt-7 grid gap-7 lg:grid-cols-[minmax(0,1fr)_320px]">
        <section className="min-w-0">
          <WorkspaceSectionHeader title="Recent sessions" description="Completed practice across every format." />
          {recentSessions.length > 0 ? (
            <div className="overflow-hidden rounded-lg border">
              <div className="text-muted-foreground bg-muted/30 hidden grid-cols-[minmax(0,1fr)_100px_104px_28px] gap-3 border-b px-3 py-2 text-xs sm:grid">
                <span>Session</span>
                <span>Format</span>
                <span>Completed</span>
                <span />
              </div>
              {recentSessions.map((session) => {
                const SessionIcon = sessionIcon(session);
                return (
                  <Link
                    key={session.id}
                    to={sessionDestination(session)}
                    className="hover:bg-muted/35 grid min-h-12 grid-cols-[minmax(0,1fr)_auto] items-center gap-3 border-b px-3 py-2.5 no-underline last:border-b-0 sm:grid-cols-[minmax(0,1fr)_100px_104px_28px]"
                  >
                    <span className="flex min-w-0 items-center gap-2.5">
                      <SessionIcon className="text-muted-foreground shrink-0" size={15} strokeWidth={1.8} />
                      <span className="truncate text-sm font-medium">{session.title}</span>
                    </span>
                    <span className="text-muted-foreground text-xs sm:text-sm">{sessionKindLabel(session)}</span>
                    <span className="text-muted-foreground hidden text-xs sm:block">
                      {formatRelativeDate(session.completed_at ?? session.last_activity_at)}
                    </span>
                    <ArrowRight className="text-muted-foreground hidden sm:block" size={14} strokeWidth={1.8} />
                  </Link>
                );
              })}
            </div>
          ) : (
            <WorkspaceEmptyState
              title="No completed sessions yet"
              description="Your completed practice history will collect here."
              className="min-h-32"
            />
          )}
        </section>

        <aside className="min-w-0 lg:border-l lg:pl-7">
          <section>
            <WorkspaceSectionHeader
              title="Practice next"
              description="Recommended from your learning memory."
            />
            {focusAreas.length > 0 ? (
              <div className="overflow-hidden rounded-lg border">
                {focusAreas.map((area, index) => (
                  <Link
                    key={area}
                    to={`/generate?prompt=${encodeURIComponent(area)}`}
                    className="hover:bg-muted/35 flex gap-3 border-b px-3 py-3 no-underline last:border-b-0"
                  >
                    <span className="text-muted-foreground w-5 shrink-0 font-mono text-xs">
                      {String(index + 1).padStart(2, '0')}
                    </span>
                    <span className="min-w-0 flex-1 text-sm leading-5">{area}</span>
                    <ArrowRight className="text-muted-foreground mt-0.5 shrink-0" size={14} strokeWidth={1.8} />
                  </Link>
                ))}
              </div>
            ) : (
              <WorkspaceEmptyState
                title="No recommendations yet"
                description="Complete your first session to build a practice queue."
                action={
                  <Button variant="outline" size="sm" asChild>
                    <Link to="/generate">Start practice</Link>
                  </Button>
                }
                className="min-h-32"
              />
            )}
          </section>

          <section className="mt-7">
            <WorkspaceSectionHeader
              title="Learning snapshot"
              actions={
                <Button variant="ghost" size="sm" asChild className="h-7 px-2 text-xs">
                  <Link to="/memory">
                    <Brain data-icon="inline-start" />
                    Memory
                  </Link>
                </Button>
              }
            />
            {skills.length > 0 ? (
              <div className="flex flex-col gap-3">
                {skills.map((skill) => (
                  <div key={skill.id}>
                    <div className="mb-1.5 flex items-center justify-between gap-3 text-xs">
                      <span className="truncate font-medium">{skill.label}</span>
                      <span
                        className={cn(
                          'shrink-0',
                          skill.trend === 'down' ? 'text-amber-800' : 'text-muted-foreground',
                        )}
                      >
                        {trendLabel(skill)}
                      </span>
                    </div>
                    <div className="flex items-center gap-2">
                      <Progress value={skill.confidence} className="h-1.5 flex-1" />
                      <span className="text-muted-foreground w-8 text-right font-mono text-xs tabular-nums">
                        {skill.confidence}%
                      </span>
                    </div>
                  </div>
                ))}
              </div>
            ) : (
              <p className="text-muted-foreground text-sm leading-5">
                Skill confidence appears after CodeGym has enough practice evidence.
              </p>
            )}
          </section>
        </aside>
      </div>
    </WorkspacePage>
  );
}
