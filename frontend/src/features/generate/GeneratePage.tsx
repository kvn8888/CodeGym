import { useEffect, useMemo, useState, type FormEvent } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import { ArrowRight, Brain, Check, Clock3, Code2, ListChecks, MessagesSquare } from 'lucide-react';

import { api } from '../../shared/api/client';
import type {
  NewPracticeConfig,
  PracticeIntake,
  PracticeFormat,
  ProblemLanguage,
  InterviewMode,
  PracticeSession,
  PracticeSessionSummary,
  Problem,
  UserMemoryProfile,
} from '../../shared/api/types';
import { QuestionModal } from './QuestionModal';
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
];
const problemLanguages: Array<{ value: ProblemLanguage; label: string }> = [
  { value: 'python', label: 'Python' },
  { value: 'go', label: 'Go' },
];
const interviewModes: Array<{ value: InterviewMode; label: string }> = [
  { value: 'coding', label: 'Coding' },
  { value: 'system_design', label: 'System design' },
  { value: 'behavioral', label: 'Behavioral' },
  { value: 'open_coaching', label: 'Open coaching' },
];

/** Stable start-flow status for New Practice (not an implicit boolean/string). */
type StartStatus = 'idle' | 'pending' | 'failed';

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
  {
    value: 'interview',
    label: 'Conversational interview',
    shortLabel: 'Interview',
    description: 'A resumable, memory-personalized interview with a saved transcript.',
    icon: MessagesSquare,
  },
];

function sessionTitle(format: PracticeFormat, prompt: string) {
  const normalized = prompt.trim().replace(/\s+/g, ' ');
  if (!normalized) {
    if (format === 'coding') return 'Personalized coding practice';
    if (format === 'interview') return 'Personalized interview';
    return 'Personalized MCQ practice';
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

function configFromIntake(
  intake: PracticeIntake,
): NewPracticeConfig & { format: PracticeFormat } {
  const seed = intake.practice_seed;
  const seedFormat =
    seed.format === 'coding' || seed.format === 'mcq' || seed.format === 'interview'
      ? seed.format
      : 'mcq';
  const seedMode =
    seed.interviewMode === 'coding' ||
    seed.interviewMode === 'system_design' ||
    seed.interviewMode === 'behavioral' ||
    seed.interviewMode === 'open_coaching'
      ? seed.interviewMode
      : 'coding';
  const seedDifficulty =
    seed.difficulty === 'easy' || seed.difficulty === 'hard' ? seed.difficulty : 'medium';
  const seedLanguage = seed.language === 'go' ? seed.language : 'python';
  const seedCount =
    typeof seed.count === 'number' && Number.isInteger(seed.count) && seed.count > 0
      ? seed.count
      : seedFormat === 'mcq'
        ? 5
        : 1;
  return {
    format: seedFormat,
    prompt: intake.original_topic,
    difficulty: seedDifficulty,
    count: seedCount,
    language: seedLanguage,
    intakeId: intake.id,
    interviewMode: seedMode,
  };
}

interface GeneratePageProps {
  initialIntake?: PracticeIntake | null;
  initialStarting?: boolean;
  initialFormat?: PracticeFormat;
  initialStartError?: string | null;
}

export function GeneratePage({
  initialIntake = null,
  initialStarting = false,
  initialFormat = 'mcq',
  initialStartError = null,
}: GeneratePageProps) {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const [format, setFormat] = useState<PracticeFormat>(initialFormat);
  const [prompt, setPrompt] = useState(
    () => searchParams.get('prompt') ?? initialIntake?.original_topic ?? '',
  );
  const [difficulty, setDifficulty] = useState<NewPracticeConfig['difficulty']>('medium');
  const [language, setLanguage] = useState<ProblemLanguage>('python');
  const [interviewMode, setInterviewMode] = useState<InterviewMode>('coding');
  const [count, setCount] = useState(5);
  const [profile, setProfile] = useState<UserMemoryProfile | null>(null);
  const [recentSessions, setRecentSessions] = useState<PracticeSessionSummary[]>([]);
  const [startStatus, setStartStatus] = useState<StartStatus>(
    initialStarting ? 'pending' : initialStartError ? 'failed' : 'idle',
  );
  const [error, setError] = useState<string | null>(initialStartError);
  const [intake, setIntake] = useState<PracticeIntake | null>(initialIntake);
  const [pendingConfig, setPendingConfig] = useState<NewPracticeConfig | null>(
    initialIntake ? configFromIntake(initialIntake) : null,
  );
  const [intakeSaving, setIntakeSaving] = useState(false);

  const selectedFormat = practiceFormats.find((item) => item.value === format) ?? practiceFormats[0];
  const FormatIcon = selectedFormat.icon;

  useEffect(() => {
    let cancelled = false;
    void Promise.allSettled([
      api.get<UserMemoryProfile>('/memory/profile'),
      api.get<PracticeSessionSummary[]>('/sessions?limit=8'),
      initialIntake
        ? Promise.resolve([] as PracticeIntake[])
        : api.get<PracticeIntake[]>('/practice-intakes?status=pending&limit=1'),
    ]).then(([profileResult, sessionsResult, intakeResult]) => {
      if (cancelled) return;
      if (profileResult.status === 'fulfilled') setProfile(profileResult.value);
      if (sessionsResult.status === 'fulfilled') setRecentSessions(sessionsResult.value);
      if (intakeResult.status === 'fulfilled' && intakeResult.value[0]) {
        const restored = intakeResult.value[0];
        const restoredConfig = configFromIntake(restored);
        setIntake(restored);
        setPendingConfig(restoredConfig);
        setPrompt(restoredConfig.prompt);
        setFormat(restoredConfig.format);
        setDifficulty(restoredConfig.difficulty);
        setLanguage(restoredConfig.language ?? 'python');
        setCount(restoredConfig.count);
        setInterviewMode(restoredConfig.interviewMode ?? 'coding');
      }
    });
    return () => {
      cancelled = true;
    };
  }, [initialIntake]);

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
      .filter((session) =>
        format === 'mcq'
          ? session.kind === 'mcq'
          : format === 'interview'
            ? session.kind === 'interview'
            : session.kind === 'workspace',
      )
      .slice(0, 4);
  }, [format, recentSessions]);

  const launchPractice = async (config: NewPracticeConfig) => {
    if (config.format === 'mcq') {
      const session = await api.post<PracticeSession>('/sessions', {
        kind: 'mcq',
        title: sessionTitle('mcq', config.prompt),
        state: {
          schema_version: 1,
          prompt: config.prompt,
          difficulty: config.difficulty,
          count: config.count,
          intake_id: config.intakeId,
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

    if (config.format === 'interview') {
      const session = await api.post<PracticeSession>('/sessions', {
        kind: 'interview',
        title: sessionTitle('interview', config.prompt),
        state: {
          schema_version: 1,
          format: 'interview',
          mode: config.interviewMode ?? 'coding',
          topic: config.prompt,
          intake_id: config.intakeId,
          memory_update_status: 'idle',
        },
      });
      navigate(`/interviews/${encodeURIComponent(session.id)}`);
      return;
    }

    const generated = await api.post<{
      kind: 'problem';
      problem_id: string;
      problem: Problem;
      provider: string;
      model: string;
    }>('/generate', {
      kind: 'problem',
      intake_id: config.intakeId,
      spec: {
        topic: config.prompt,
        prompt: config.prompt,
        difficulty: config.difficulty,
        language: config.language ?? 'python',
      },
    });
    const session = await api.post<PracticeSession>('/sessions', {
      kind: 'workspace',
      title: generated.problem.title || sessionTitle('coding', config.prompt),
      problem_id: generated.problem_id,
      state: {
        schema_version: 1,
        format: 'coding',
        prompt: config.prompt,
        difficulty: config.difficulty,
        intake_id: config.intakeId,
        problem_id: generated.problem_id,
        memory_update_status: 'idle',
      },
    });
    navigate(`/problems/${encodeURIComponent(generated.problem_id)}?session=${encodeURIComponent(session.id)}&from=generate`, {
      state: { newPractice: { sessionId: session.id, config } },
    });
  };

  const prepareIntake = async (config: NewPracticeConfig, restart = false) => {
    setStartStatus('pending');
    setError(null);
    setPendingConfig(config);
    try {
      if (!config.prompt.trim()) {
        await launchPractice(config);
        return;
      }
      const prepared = await api.post<PracticeIntake>('/practice-intakes', {
        topic: config.prompt,
        practice_seed: config,
        restart,
      });
      if (prepared.status !== 'pending') {
        await launchPractice({ ...config, intakeId: prepared.id });
        return;
      }
      setIntake(prepared);
      setStartStatus('idle');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not prepare the topic baseline.');
      setStartStatus('failed');
    }
  };

  const startPractice = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (startStatus === 'pending') return;
    const config: NewPracticeConfig = {
      format,
      prompt: prompt.trim(),
      difficulty,
      count: format === 'mcq' ? count : 1,
      language: format === 'coding' ? language : undefined,
      interviewMode: format === 'interview' ? interviewMode : undefined,
    };
    await prepareIntake(config);
  };

  const saveIntakeAnswer = async (questionId: string, optionId: string) => {
    if (!intake) return;
    setIntakeSaving(true);
    setError(null);
    try {
      const updated = await api.patch<PracticeIntake>(`/practice-intakes/${intake.id}`, {
        answers: { [questionId]: optionId },
      });
      setIntake(updated);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not save the baseline answer.');
      throw err;
    } finally {
      setIntakeSaving(false);
    }
  };

  const finishIntake = async (status: 'completed' | 'skipped') => {
    if (!intake || !pendingConfig) return;
    setIntakeSaving(true);
    setError(null);
    try {
      const updated = await api.patch<PracticeIntake>(`/practice-intakes/${intake.id}`, { status });
      setIntake(updated);
      await launchPractice({ ...pendingConfig, intakeId: updated.id });
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not save the topic baseline.');
      setIntakeSaving(false);
    }
  };

  const retryIntake = async () => {
    if (!pendingConfig) return;
    await prepareIntake(pendingConfig);
  };

  return (
    <WorkspacePage>
      <WorkspacePageHeader
        title="New practice"
        description={
          format === 'coding'
            ? 'Generate a LeetCode-style coding problem personalized from your memory.'
            : format === 'interview'
              ? 'Start a resumable interview personalized from demonstrated memory.'
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
                      : format === 'interview'
                        ? 'Backend system design, behavioral leadership examples, or Python algorithms...'
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

              <div className={`grid border-t ${format === 'mcq' || format === 'coding' ? 'sm:grid-cols-2' : ''}`}>
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

                {format === 'coding' && (
                  <fieldset className="border-b p-4 sm:border-r sm:border-b-0 sm:p-5">
                    <legend className="mb-2 text-sm font-medium">Language</legend>
                    <ToggleGroup
                      type="single"
                      value={language}
                      onValueChange={(value) => value && setLanguage(value as ProblemLanguage)}
                      variant="outline"
                      className="justify-start"
                      aria-label="Language"
                    >
                      {problemLanguages.map((item) => (
                        <ToggleGroupItem key={item.value} value={item.value} className="px-3">
                          {item.label}
                        </ToggleGroupItem>
                      ))}
                    </ToggleGroup>
                  </fieldset>
                )}

                {format === 'interview' ? (
                  <fieldset className="p-4 sm:p-5">
                    <legend className="mb-2 text-sm font-medium">Interview mode</legend>
                    <ToggleGroup
                      type="single"
                      value={interviewMode}
                      onValueChange={(value) => value && setInterviewMode(value as InterviewMode)}
                      variant="outline"
                      className="flex-wrap justify-start"
                      aria-label="Interview mode"
                    >
                      {interviewModes.map((item) => (
                        <ToggleGroupItem key={item.value} value={item.value} className="px-3">
                          {item.label}
                        </ToggleGroupItem>
                      ))}
                    </ToggleGroup>
                  </fieldset>
                ) : (
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
                )}
              </div>

              <div className="bg-muted/20 flex flex-col gap-3 border-t px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
                <div className="text-muted-foreground text-xs">
                  {format === 'coding'
                    ? 'Opens the coding workspace with an editor and tests.'
                    : format === 'interview'
                      ? 'Every turn is saved so you can resume from History.'
                      : 'Progress is saved after every answer.'}
                </div>
                {prompt.trim() && (
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    disabled={startStatus === 'pending'}
                    onClick={() =>
                      void prepareIntake(
                        {
                          format,
                          prompt: prompt.trim(),
                          difficulty,
                          count: format === 'mcq' ? count : 1,
                          language: format === 'coding' ? language : undefined,
                          interviewMode: format === 'interview' ? interviewMode : undefined,
                        },
                        true,
                      )
                    }
                  >
                    Update baseline
                  </Button>
                )}
                <Button
                  type="submit"
                  disabled={startStatus === 'pending'}
                  className="sm:min-w-36"
                >
                  {startStatus === 'pending'
                    ? 'Starting'
                    : startStatus === 'failed'
                      ? 'Retry'
                      : format === 'coding'
                        ? 'Start coding'
                        : format === 'interview'
                          ? 'Start interview'
                          : 'Start practice'}
                  {startStatus !== 'pending' && <ArrowRight data-icon="inline-end" />}
                </Button>
              </div>
            </section>

            {error && (
              <div
                className="border-destructive/40 bg-destructive/5 text-destructive mt-4 rounded-md border px-3 py-2 text-sm"
                role="alert"
              >
                <p>{error}</p>
                {startStatus === 'failed' && (
                  <p className="text-muted-foreground mt-1 text-xs">
                    Edit your prompt above, then retry. Submits are blocked while a start is pending.
                  </p>
                )}
              </div>
            )}

            {intake?.generation_error && (
              <div className="mt-4 rounded-md border px-4 py-3">
                <p className="text-sm font-medium">Baseline questions could not be prepared</p>
                <p className="text-muted-foreground mt-1 text-sm">{intake.generation_error}</p>
                <div className="mt-3 flex gap-2">
                  <Button type="button" size="sm" onClick={() => void retryIntake()}>
                    Retry
                  </Button>
                  <Button type="button" size="sm" variant="outline" onClick={() => void finishIntake('skipped')}>
                    Skip baseline
                  </Button>
                </div>
              </div>
            )}

            {intake && intake.status !== 'pending' && (
              <div className="bg-muted/20 mt-4 rounded-md border px-4 py-3">
                <p className="text-sm font-medium">
                  {intake.status === 'completed'
                    ? 'Baseline already on file'
                    : 'Baseline questionnaire skipped'}
                </p>
                <p className="text-muted-foreground mt-1 text-sm">
                  This topic will start without another automatic questionnaire. Use Update
                  baseline whenever you want to replace it.
                </p>
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
      {intake &&
        !intake.generation_error &&
        intake.status === 'pending' &&
        intake.questions.length > 0 && (
          <QuestionModal
            topic={intake.original_topic}
            questions={intake.questions}
            initialAnswers={intake.answers}
            saving={intakeSaving}
            onSaveAnswer={saveIntakeAnswer}
            onComplete={() => finishIntake('completed')}
            onSkip={() => finishIntake('skipped')}
          />
        )}
    </WorkspacePage>
  );
}
