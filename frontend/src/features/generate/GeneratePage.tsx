import { useEffect, useMemo, useState, type FormEvent } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import { ArrowRight, Brain, Check, Clock3, Code2, ListChecks } from 'lucide-react';

import { api } from '../../shared/api/client';
import type {
  NewPracticeConfig,
  PracticeFormat,
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Textarea } from '@/components/ui/textarea';
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group';

const questionCounts = [5, 10, 15];
const difficulties: Array<{ value: NewPracticeConfig['difficulty']; label: string }> = [
  { value: 'easy', label: 'Easy' },
  { value: 'medium', label: 'Medium' },
  { value: 'hard', label: 'Hard' },
import { cn } from '@/lib/utils';
import { createMemoryEvent } from '../../shared/api/client';
import { buildGenerateEvent, memoryEventTypes } from '../../shared/api/memoryEvents';
import { QuestionModal, type Question, type Answer } from './QuestionModal';

type GenerateView = 'command' | 'spotlight';
type GenerateFormat = 'problem' | 'mcq' | 'interview';
type Difficulty = 'easy' | 'medium' | 'hard';

const PHRASE_STORAGE_KEY = 'codegym.generate.sessionPhrase';

const SESSION_PHRASES = [
  'Build real fluency.',
  'Practice with intent.',
  'Turn gaps into reps.',
  'Make hard topics familiar.',
  'Train the parts that matter.',
];

const practiceFormats: Array<{
  value: PracticeFormat;
  label: string;
  shortLabel: string;
  description: string;
  icon: typeof ListChecks;
}> = [
  {
    value: 'mcq',
    label: 'Question set',
    shortLabel: 'MCQ',
    description: 'Timed concept checks with explanations after each answer.',
    icon: ListChecks,
  },
  {
    value: 'coding',
    label: 'Coding problem',
    shortLabel: 'DSA',
    description: 'LeetCode / HackerRank-style problem with an editor and test cases.',
    icon: Code2,
  },
];

function sessionTitle(format: PracticeFormat, prompt: string) {
  const normalized = prompt.trim().replace(/\s+/g, ' ');
  if (!normalized) {
    return format === 'coding' ? 'Personalized coding practice' : 'Personalized MCQ practice';
  }
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
  const [format, setFormat] = useState<PracticeFormat>('mcq');
  const [prompt, setPrompt] = useState(() => searchParams.get('prompt') ?? '');
  const [difficulty, setDifficulty] = useState<NewPracticeConfig['difficulty']>('medium');
  const [count, setCount] = useState(5);
  const [profile, setProfile] = useState<UserMemoryProfile | null>(null);
  const [recentSessions, setRecentSessions] = useState<PracticeSessionSummary[]>([]);
  const [starting, setStarting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const selectedFormat = practiceFormats.find((item) => item.value === format) ?? practiceFormats[0];
  const FormatIcon = selectedFormat.icon;

  useEffect(() => {
    let cancelled = false;
    void Promise.allSettled([
      api.get<UserMemoryProfile>('/memory/profile'),
      api.get<PracticeSessionSummary[]>('/sessions?limit=8'),
    ]).then(([profileResult, sessionsResult]) => {
      if (cancelled) return;
      if (profileResult.status === 'fulfilled') setProfile(profileResult.value);
      if (sessionsResult.status === 'fulfilled') setRecentSessions(sessionsResult.value);
    });
    return () => {
      cancelled = true;
    };
  }, []);

  const recordGenerateEvent = async (input: Parameters<typeof buildGenerateEvent>[0]) => {
    try {
      await createMemoryEvent(buildGenerateEvent(input));
    } catch (error) {
      console.warn('[generate] failed to record memory event', error);
    }
  };

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

  const recentForFormat = useMemo(() => {
    return recentSessions
      .filter((session) => (format === 'mcq' ? session.kind === 'mcq' : session.kind === 'workspace'))
      .slice(0, 4);
  }, [format, recentSessions]);

  const startPractice = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (starting) return;

    const config: NewPracticeConfig = {
      format,
      prompt: prompt.trim(),
      language,
      difficulty,
      count: format === 'mcq' ? count : 1,
    };

    void recordGenerateEvent({
      type: memoryEventTypes.intakeStarted,
      summary: `Started a ${difficulty} ${language} ${format} generation request.`,
      payload: {
        prompt: requestContext.prompt,
        format,
        language,
        difficulty,
        view,
        schema_version: 1,
      },
    });

    // TODO: Call /api/v1/generate/questions with requestContext to get real questions.
    console.log('[generate]', requestContext);

    const mockQuestions: Question[] = [
      {
        id: 'q1',
        text: 'What programming language would you like to use?',
        options: ['Python', 'Go', 'JavaScript', 'TypeScript', 'Specify...'],
      },
      {
        id: 'q2',
        text: 'What area should this practice focus on?',
        options: ['Core algorithm', 'Data structure design', 'API integration', 'Specify...'],
      },
      {
        id: 'q3',
        text: 'How challenging should this be?',
        options: ['Beginner friendly', 'Moderate complexity', 'Senior-level challenge', 'Specify...'],
      },
    ];

    setAgentQuestions(mockQuestions);
    setShowQuestions(true);
    setGenerating(false);
  };

  const handleQuestionsComplete = (answers: Answer[]) => {
    setShowQuestions(false);
    setGenerating(true);
    console.log('[generate] answers:', answers);

    void recordGenerateEvent({
      type: memoryEventTypes.clarifyingQuestionsAnswered,
      summary: `Answered ${answers.length} clarifying questions for ${format} generation.`,
      payload: {
        format,
        language,
        difficulty,
        view,
        answer_count: answers.length,
        question_ids: answers.map((answer) => answer.questionId),
        schema_version: 1,
      },
    });

    // TODO: Call POST /api/v1/generate with prompt + answers + view context.
    setTimeout(() => {
      setStatus('Generation endpoint not yet implemented');
      setGenerating(false);
    }, 3000);
  };

  const handleKeyDown = (event: React.KeyboardEvent) => {
    if (event.key === 'Enter' && (event.metaKey || event.ctrlKey)) {
      handleGenerate();
    }
  };

  const usePrompt = (value: string) => {
    setPrompt(value);
    setStarting(true);
    setError(null);
    try {
      if (format === 'mcq') {
        const session = await api.post<PracticeSession>('/sessions', {
          kind: 'mcq',
          title: sessionTitle('mcq', config.prompt),
          state: {
            schema_version: 1,
            prompt: config.prompt,
            difficulty: config.difficulty,
            count: config.count,
            round: 1,
            question_index: 0,
            elapsed: 0,
            selected_index: null,
            selected_indices: [],
            response_text: '',
            evaluation_result: null,
            confirmed: false,
            using_fallback: false,
            questions: [],
            results: [],
            skipped_questions: [],
          },
        });
        navigate(`/marathon?session=${encodeURIComponent(session.id)}`, {
          state: { newPractice: { sessionId: session.id, config } },
        });
        return;
      }

      // DSA / LeetCode-style coding session. Full AI problem generation is still
      // landing; we open the coding workspace shell with a practice problem and
      // persist a workspace session for history/resume.
      const session = await api.post<PracticeSession>('/sessions', {
        kind: 'workspace',
        title: sessionTitle('coding', config.prompt),
        problem_id: 'two-sum',
        state: {
          schema_version: 1,
          format: 'coding',
          prompt: config.prompt,
          difficulty: config.difficulty,
          problem_id: 'two-sum',
        },
      });
      navigate(
        `/problems/two-sum?session=${encodeURIComponent(session.id)}&from=generate`,
        {
          state: {
            newPractice: { sessionId: session.id, config },
          },
        },
      );
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not start practice.');
      setStarting(false);
    }
  };

  return (
    <WorkspacePage>
      <WorkspacePageHeader
        title="New practice"
        description={
          format === 'coding'
            ? 'Generate a LeetCode-style coding problem personalized from your memory.'
            : 'Set the focus for a personalized question session.'
        }
      />

      <form onSubmit={(event) => void startPractice(event)}>
        <div className="grid gap-7 lg:grid-cols-[minmax(0,1fr)_320px]">
          <div className="min-w-0">
            <section className="overflow-hidden rounded-lg border">
              <div className="bg-muted/25 flex flex-col gap-3 border-b px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
                <div className="flex min-w-0 flex-1 items-center gap-3">
                  <FormatIcon className="text-muted-foreground shrink-0" size={16} strokeWidth={1.8} />
                  <div className="min-w-0 flex-1">
                    <Label htmlFor="practice-format" className="sr-only">
                      Practice format
                    </Label>
                    <Select
                      value={format}
                      onValueChange={(value) => setFormat(value as PracticeFormat)}
                    >
                      <SelectTrigger
                        id="practice-format"
                        size="sm"
                        className="h-8 w-full max-w-xs border-transparent bg-transparent px-2 text-sm font-medium shadow-none hover:bg-muted/50 sm:w-56"
                        aria-label="Practice format"
                      >
                        <SelectValue placeholder="Choose format" />
                      </SelectTrigger>
                      <SelectContent align="start">
                        {practiceFormats.map((item) => (
                          <SelectItem key={item.value} value={item.value}>
                            <span className="flex flex-col gap-0.5 py-0.5 text-left">
                              <span>{item.label}</span>
                              <span className="text-muted-foreground text-xs font-normal">
                                {item.shortLabel} · {item.description}
                              </span>
                            </span>
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                </div>
                <span className="text-muted-foreground flex items-center gap-1.5 text-xs">
                  <Check size={14} strokeWidth={2} />
                  Available
                </span>
              </div>

              <div className="text-muted-foreground border-b px-4 py-2 text-xs leading-5 sm:px-5">
                {selectedFormat.description}
              </div>

              <div className="p-4 sm:p-5">
                <Label htmlFor="practice-prompt" className="text-sm font-medium">
                  What should this session focus on?
                </Label>
                <Textarea
                  id="practice-prompt"
                  value={prompt}
                  onChange={(event) => setPrompt(event.target.value)}
                  placeholder={
                    format === 'coding'
                      ? 'Two-pointer array problems, hash maps, or graph BFS in Go...'
                      : 'Go concurrency patterns, channel ownership, and cancellation...'
                  }
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

              <div className={`grid border-t ${format === 'mcq' ? 'sm:grid-cols-2' : ''}`}>
                {format === 'mcq' && (
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
                )}

                <fieldset className="p-4 sm:p-5">
                  <legend className="mb-2 text-sm font-medium">Difficulty</legend>
                  <ToggleGroup
                    type="single"
                    value={difficulty}
                    onValueChange={(value) =>
                      value && setDifficulty(value as NewPracticeConfig['difficulty'])
                    }
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
                  {format === 'coding'
                    ? 'Opens the coding workspace with an editor and tests.'
                    : 'Progress is saved after every answer.'}
                </div>
                <Button type="submit" disabled={starting} className="sm:min-w-36">
                  {starting ? 'Starting' : format === 'coding' ? 'Start coding' : 'Start practice'}
                  {!starting && <ArrowRight data-icon="inline-end" />}
                </Button>
              </div>
            </section>

            {error && (
              <div className="border-destructive/40 bg-destructive/5 text-destructive mt-4 rounded-md border px-3 py-2 text-sm">
                {error}
              </div>
            )}

            {recentForFormat.length > 0 && (
              <section className="mt-7">
                <WorkspaceSectionHeader
                  title="Recent topics"
                  description={
                    format === 'mcq'
                      ? 'Resume active runs or review completed results.'
                      : 'Reuse a previous focus without rebuilding the setup.'
                  }
                />
                <div className="overflow-hidden rounded-lg border">
                  {recentForFormat.map((session) =>
                    format === 'mcq' ? (
                      <Link
                        key={session.id}
                        to={`/marathon?session=${encodeURIComponent(session.id)}`}
                        className="hover:bg-muted/35 flex min-h-11 w-full items-center gap-3 border-b px-3 py-2 text-left last:border-b-0"
                      >
                        <Clock3
                          className="text-muted-foreground shrink-0"
                          size={15}
                          strokeWidth={1.8}
                        />
                        <span className="min-w-0 flex-1 truncate text-sm">{session.title}</span>
                        <span className="text-muted-foreground shrink-0 text-xs">
                          {session.status === 'active' ? 'Resume' : 'Results'}
                        </span>
                        <ArrowRight className="text-muted-foreground shrink-0" size={14} />
                      </Link>
                    ) : (
                      <button
                        key={session.id}
                        type="button"
                        onClick={() => setPrompt(session.title)}
                        className="hover:bg-muted/35 flex min-h-11 w-full items-center gap-3 border-b px-3 py-2 text-left last:border-b-0"
                      >
                        <Clock3
                          className="text-muted-foreground shrink-0"
                          size={15}
                          strokeWidth={1.8}
                        />
                        <span className="min-w-0 flex-1 truncate text-sm">{session.title}</span>
                        <span className="text-muted-foreground shrink-0 text-xs">
                          {formatRelativeDate(session.last_activity_at)}
                        </span>
                      </button>
                    ),
                  )}
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
                {format === 'coding'
                  ? 'Difficulty steers problem selection. Memory notes decide which algorithms and data structures to emphasize.'
                  : 'Difficulty and question count steer this session. Your learning history determines which concepts receive emphasis.'}
              </div>
            </div>
          </aside>
        </div>
      </form>
    </WorkspacePage>
  );
}
