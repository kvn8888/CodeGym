import { useCallback, useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import {
  ArrowRight,
  CheckCircle2,
  CircleDot,
  CircleSlash2,
  Code2,
  History,
  ListChecks,
  RefreshCw,
} from 'lucide-react';
import { motion } from 'motion/react';

import { api } from '../../shared/api/client';
import { useCodeGymAuthState } from '../../shared/auth/authState';
import type { PracticeSessionSummary, ProblemSummary } from '../../shared/api/types';
import {
  WorkspaceEmptyState,
  WorkspacePage,
  WorkspacePageHeader,
  WorkspaceSectionHeader,
} from '../../shared/components/WorkspacePage';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { cn } from '@/lib/utils';

const difficultyChips: Record<number, { label: string; className: string }> = {
  1: { label: 'Easy', className: 'bg-green-100 text-green-900' },
  2: { label: 'Easy', className: 'bg-green-100 text-green-900' },
  3: { label: 'Medium', className: 'bg-amber-100 text-amber-900' },
  4: { label: 'Hard', className: 'bg-red-100 text-red-900' },
  5: { label: 'Expert', className: 'bg-purple-100 text-purple-700' },
};

const listVariants = {
  hidden: {},
  visible: { transition: { staggerChildren: 0.05 } },
};

const itemVariants = {
  hidden: { opacity: 0, y: 14 },
  visible: { opacity: 1, y: 0, transition: { type: 'spring' as const, stiffness: 320, damping: 28 } },
};

const sessionStatus = {
  active: {
    label: 'Active',
    action: 'Resume',
    icon: CircleDot,
    iconClassName: 'text-amber-700',
  },
  completed: {
    label: 'Completed',
    action: 'Results',
    icon: CheckCircle2,
    iconClassName: 'text-green-700',
  },
  abandoned: {
    label: 'Abandoned',
    action: 'Results',
    icon: CircleSlash2,
    iconClassName: 'text-muted-foreground',
  },
} as const;

function formatSessionDate(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return 'Unknown date';
  return new Intl.DateTimeFormat('en', {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  }).format(date);
}

function sessionDateLabel(session: PracticeSessionSummary) {
  if (session.status === 'completed') {
    return `Completed ${formatSessionDate(session.completed_at ?? session.last_activity_at)}`;
  }
  return `${session.status === 'active' ? 'Updated' : 'Last active'} ${formatSessionDate(session.last_activity_at)}`;
}

export function ProblemListPage() {
  const { configured: authConfigured, isAuthenticated } = useCodeGymAuthState();
  const apiAuthReady = !authConfigured || isAuthenticated;
  const [problems, setProblems] = useState<ProblemSummary[]>([]);
  const [sessions, setSessions] = useState<PracticeSessionSummary[]>([]);
  const [workspaceSessions, setWorkspaceSessions] = useState<PracticeSessionSummary[]>([]);
  const [problemsLoading, setProblemsLoading] = useState(true);
  const [sessionsLoading, setSessionsLoading] = useState(true);
  const [workspaceSessionsLoading, setWorkspaceSessionsLoading] = useState(true);
  const [problemsError, setProblemsError] = useState<string | null>(null);
  const [sessionsError, setSessionsError] = useState<string | null>(null);
  const [workspaceSessionsError, setWorkspaceSessionsError] = useState<string | null>(null);
  const [languageFilter, setLanguageFilter] = useState('');

  const loadProblems = useCallback(async () => {
    setProblemsLoading(true);
    setProblemsError(null);
    try {
      const data = await api.get<{ problems: ProblemSummary[]; total: number }>('/problems');
      setProblems(data.problems ?? []);
    } catch (error) {
      setProblemsError(error instanceof Error ? error.message : 'Could not load the problem library.');
    } finally {
      setProblemsLoading(false);
    }
  }, []);

  const loadSessions = useCallback(async () => {
    setSessionsLoading(true);
    setSessionsError(null);
    try {
      const data = await api.get<PracticeSessionSummary[]>('/sessions?kind=mcq&limit=50');
      setSessions(data);
    } catch (error) {
      setSessionsError(error instanceof Error ? error.message : 'Could not load your MCQ history.');
    } finally {
      setSessionsLoading(false);
    }
  }, []);

  const loadWorkspaceSessions = useCallback(async () => {
    setWorkspaceSessionsLoading(true);
    setWorkspaceSessionsError(null);
    try {
      const data = await api.get<PracticeSessionSummary[]>(
        '/sessions?kind=workspace&status=active&limit=50',
      );
      setWorkspaceSessions(data);
    } catch (error) {
      setWorkspaceSessionsError(
        error instanceof Error ? error.message : 'Could not load your coding drafts.',
      );
    } finally {
      setWorkspaceSessionsLoading(false);
    }
  }, []);

  useEffect(() => {
    if (!apiAuthReady) {
      const message = 'Sign in to load your practice data.';
      setProblemsLoading(false);
      setSessionsLoading(false);
      setWorkspaceSessionsLoading(false);
      setProblemsError(message);
      setSessionsError(message);
      setWorkspaceSessionsError(message);
      return;
    }
    void loadProblems();
    void loadSessions();
    void loadWorkspaceSessions();
  }, [apiAuthReady, loadProblems, loadSessions, loadWorkspaceSessions]);

  const filteredProblems = useMemo(
    () =>
      languageFilter
        ? problems.filter((problem) => problem.language === languageFilter)
        : problems,
    [languageFilter, problems],
  );
  const sortedSessions = useMemo(
    () =>
      [...sessions].sort(
        (a, b) =>
          Number(a.status !== 'active') - Number(b.status !== 'active') ||
          b.last_activity_at.localeCompare(a.last_activity_at),
      ),
    [sessions],
  );
  const sortedWorkspaceSessions = useMemo(
    () =>
      [...workspaceSessions]
        .filter((session) => session.problem_id)
        .sort((a, b) => b.last_activity_at.localeCompare(a.last_activity_at)),
    [workspaceSessions],
  );
  const languages = useMemo(
    () => [...new Set(problems.map((problem) => problem.language))].sort(),
    [problems],
  );

  return (
    <WorkspacePage>
      <WorkspacePageHeader
        title="History"
        description="Resume a coding draft or multiple-choice run, or open a problem."
      />

      <section>
        <WorkspaceSectionHeader
          title="Coding drafts"
          description={
            !workspaceSessionsLoading &&
            !workspaceSessionsError &&
            sortedWorkspaceSessions.length > 0
              ? `${sortedWorkspaceSessions.length} active ${
                  sortedWorkspaceSessions.length === 1 ? 'draft' : 'drafts'
                }`
              : undefined
          }
        />
        {workspaceSessionsLoading ? (
          <div className="overflow-hidden rounded-lg border" aria-label="Loading coding drafts">
            {Array.from({ length: 2 }).map((_, index) => (
              <div key={index} className="flex min-h-14 items-center gap-3 border-b px-3 py-2.5 last:border-b-0">
                <Skeleton className="size-8 shrink-0 rounded-md" />
                <div className="min-w-0 flex-1 space-y-1.5">
                  <Skeleton className="h-4 w-40 max-w-full" />
                  <Skeleton className="h-3 w-28 max-w-full" />
                </div>
                <Skeleton className="h-8 w-20 rounded-md" />
              </div>
            ))}
          </div>
        ) : workspaceSessionsError ? (
          <WorkspaceEmptyState
            icon={<Code2 size={20} strokeWidth={1.8} />}
            title="Coding drafts are unavailable"
            description={workspaceSessionsError}
            action={
              <Button variant="outline" size="sm" onClick={() => void loadWorkspaceSessions()}>
                <RefreshCw data-icon="inline-start" />
                Try again
              </Button>
            }
            className="min-h-32"
          />
        ) : sortedWorkspaceSessions.length === 0 ? (
          <WorkspaceEmptyState
            icon={<Code2 size={20} strokeWidth={1.8} />}
            title="No coding drafts"
            description="Edits in an active problem workspace will appear here automatically."
            action={
              <Button size="sm" asChild>
                <Link to="/generate">Start coding practice</Link>
              </Button>
            }
            className="min-h-32"
          />
        ) : (
          <div className="overflow-hidden rounded-lg border">
            {sortedWorkspaceSessions.map((session) => (
              <div
                key={session.id}
                className="flex min-h-14 items-center gap-3 border-b px-3 py-2.5 last:border-b-0"
              >
                <span className="bg-muted flex size-8 shrink-0 items-center justify-center rounded-md">
                  <Code2 size={16} strokeWidth={1.8} />
                </span>
                <div className="min-w-0 flex-1">
                  <div className="truncate text-sm font-medium">{session.title}</div>
                  <div className="text-muted-foreground mt-0.5 truncate text-xs">
                    Updated {formatSessionDate(session.last_activity_at)}
                  </div>
                </div>
                <Badge variant="outline" className="hidden rounded-md font-normal sm:inline-flex">
                  <CircleDot className="text-amber-700" />
                  Active
                </Badge>
                <Button size="sm" asChild>
                  <Link
                    to={`/problems/${encodeURIComponent(
                      session.problem_id ?? '',
                    )}?session=${encodeURIComponent(session.id)}`}
                  >
                    Resume
                    <ArrowRight data-icon="inline-end" />
                  </Link>
                </Button>
              </div>
            ))}
          </div>
        )}
      </section>

      <section className="mt-7">
        <WorkspaceSectionHeader
          title="MCQ history"
          description={
            !sessionsLoading && !sessionsError && sortedSessions.length > 0
              ? `${sortedSessions.length} saved ${sortedSessions.length === 1 ? 'run' : 'runs'}`
              : undefined
          }
        />
        {sessionsLoading ? (
          <div className="overflow-hidden rounded-lg border" aria-label="Loading MCQ history">
            {Array.from({ length: 3 }).map((_, index) => (
              <div key={index} className="flex min-h-14 items-center gap-3 border-b px-3 py-2.5 last:border-b-0">
                <Skeleton className="size-8 shrink-0 rounded-md" />
                <div className="min-w-0 flex-1 space-y-1.5">
                  <Skeleton className="h-4 w-48 max-w-full" />
                  <Skeleton className="h-3 w-28 max-w-full" />
                </div>
                <Skeleton className="h-8 w-20 rounded-md" />
              </div>
            ))}
          </div>
        ) : sessionsError ? (
          <WorkspaceEmptyState
            icon={<History size={20} strokeWidth={1.8} />}
            title="MCQ history is unavailable"
            description={sessionsError}
            action={
              <Button variant="outline" size="sm" onClick={() => void loadSessions()}>
                <RefreshCw data-icon="inline-start" />
                Try again
              </Button>
            }
            className="min-h-32"
          />
        ) : sortedSessions.length === 0 ? (
          <WorkspaceEmptyState
            icon={<ListChecks size={20} strokeWidth={1.8} />}
            title="No MCQ runs yet"
            description="Active and completed multiple-choice practice will collect here."
            action={
              <Button size="sm" asChild>
                <Link to="/generate">Start MCQ practice</Link>
              </Button>
            }
            className="min-h-32"
          />
        ) : (
          <div className="overflow-hidden rounded-lg border">
            {sortedSessions.map((session) => {
              const status = sessionStatus[session.status];
              const StatusIcon = status.icon;
              return (
                <div
                  key={session.id}
                  className="flex min-h-14 items-center gap-3 border-b px-3 py-2.5 last:border-b-0"
                >
                  <span className="bg-muted flex size-8 shrink-0 items-center justify-center rounded-md">
                    <ListChecks size={16} strokeWidth={1.8} />
                  </span>
                  <div className="min-w-0 flex-1">
                    <div className="truncate text-sm font-medium">{session.title}</div>
                    <div className="text-muted-foreground mt-0.5 truncate text-xs">
                      <span className="sm:hidden">{status.label} · </span>
                      <span>{sessionDateLabel(session)}</span>
                    </div>
                  </div>
                  <Badge variant="outline" className="hidden rounded-md font-normal sm:inline-flex">
                    <StatusIcon className={status.iconClassName} />
                    {status.label}
                  </Badge>
                  <Button variant={session.status === 'active' ? 'default' : 'outline'} size="sm" asChild>
                    <Link to={`/marathon?session=${encodeURIComponent(session.id)}`}>
                      {status.action}
                      <ArrowRight data-icon="inline-end" />
                    </Link>
                  </Button>
                </div>
              );
            })}
          </div>
        )}
      </section>

      <section className="mt-7">
        <WorkspaceSectionHeader
          title="Coding problem library"
          description={
            !problemsLoading && !problemsError
              ? `${filteredProblems.length} available${languageFilter ? ` in ${languageFilter}` : ''}`
              : undefined
          }
          actions={
            <Select
              value={languageFilter || 'all'}
              onValueChange={(value) => setLanguageFilter(value === 'all' ? '' : value)}
              disabled={problemsLoading || Boolean(problemsError)}
            >
              <SelectTrigger className="w-[164px] sm:w-[180px]" aria-label="Filter by language">
                <SelectValue placeholder="All languages" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All languages</SelectItem>
                {languages.map((language) => (
                  <SelectItem key={language} value={language}>
                    {language.charAt(0).toUpperCase() + language.slice(1)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          }
        />

        {problemsLoading ? (
          <div className="flex flex-col gap-2" aria-label="Loading coding problems">
            {Array.from({ length: 4 }).map((_, index) => (
              <Skeleton key={index} className="h-13 w-full rounded-lg" />
            ))}
          </div>
        ) : problemsError ? (
          <WorkspaceEmptyState
            icon={<Code2 size={20} strokeWidth={1.8} />}
            title="Problem library is unavailable"
            description={problemsError}
            action={
              <Button variant="outline" size="sm" onClick={() => void loadProblems()}>
                <RefreshCw data-icon="inline-start" />
                Try again
              </Button>
            }
            className="min-h-32"
          />
        ) : filteredProblems.length === 0 ? (
          <WorkspaceEmptyState
            icon={<Code2 size={20} strokeWidth={1.8} />}
            title={languageFilter ? `No ${languageFilter} problems` : 'No coding problems yet'}
            description={
              languageFilter
                ? 'Choose another language to continue browsing the library.'
                : 'Coding problems will appear here when they are available.'
            }
            action={
              languageFilter ? (
                <Button variant="outline" size="sm" onClick={() => setLanguageFilter('')}>
                  Show all languages
                </Button>
              ) : undefined
            }
            className="min-h-32"
          />
        ) : (
          <motion.div
            className="overflow-hidden rounded-lg border"
            variants={listVariants}
            initial="hidden"
            animate="visible"
          >
            {filteredProblems.map((problem) => {
              const chip = difficultyChips[problem.difficulty];
              return (
                <motion.div key={problem.id} variants={itemVariants} className="border-b last:border-b-0">
                  <Link
                    to={`/problems/${problem.id}`}
                    className="group bg-card hover:bg-muted/35 focus-visible:ring-ring/50 flex min-h-13 items-center justify-between gap-4 px-3 py-2.5 no-underline transition-colors outline-none focus-visible:ring-[3px]"
                  >
                    <div className="flex min-w-0 items-center gap-3">
                      <span className="group-hover:text-primary truncate text-sm font-semibold transition-colors">
                        {problem.title}
                      </span>
                      {chip && (
                        <Badge variant="secondary" className={cn('hidden border-transparent sm:inline-flex', chip.className)}>
                          {chip.label}
                        </Badge>
                      )}
                    </div>
                    <div className="text-muted-foreground flex shrink-0 items-center gap-2 font-mono text-xs">
                      <span className="bg-muted rounded-md px-2 py-1">{problem.language.toUpperCase()}</span>
                      {problem.framework && (
                        <span className="bg-muted hidden rounded-md px-2 py-1 md:inline">
                          {problem.framework.toUpperCase()}
                        </span>
                      )}
                      <span className="hidden sm:inline">{problem.estimated_minutes} min</span>
                    </div>
                  </Link>
                </motion.div>
              );
            })}
          </motion.div>
        )}
      </section>
    </WorkspacePage>
  );
}
