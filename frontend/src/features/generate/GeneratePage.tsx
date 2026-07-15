import { useState } from 'react';
import { AnimatePresence, motion, useReducedMotion } from 'motion/react';
import { ArrowRight, Command as CommandIcon, Sparkles } from 'lucide-react';
import { Link } from 'react-router-dom';
import { GridSpinner } from '../../shared/components/GridSpinner';
import { SlidingTabs } from '../../shared/components/SlidingTabs';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardFooter,
  CardHeader,
} from '@/components/ui/card';
import { Label } from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Spinner } from '@/components/ui/spinner';
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Textarea } from '@/components/ui/textarea';
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group';
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

const RECENT_PROMPTS = [
  'Implement an LRU cache',
  'Sliding-window rate limiter',
  'Level-order tree traversal',
];

const SPOTLIGHT_TOPICS = [
  { label: 'DSA', count: 128, color: 'var(--color-green-700)' },
  { label: 'API Patterns', count: 64, color: 'var(--color-blue-700)' },
  { label: 'System Design', count: 52, color: 'var(--color-purple-700)' },
  { label: 'Concurrency', count: 37, color: 'var(--color-pink-700)' },
];

const EXAMPLES = [
  'Pagination API pattern in Express',
  'Iterator pattern in Java',
  'Go goroutines for fan-out fan-in',
  'REST API with Python FastAPI',
  'Linked list implementation in C++',
  'Simple neural network with PyTorch',
];

const FORMAT_LABELS: Record<GenerateFormat, string> = {
  problem: 'Problem',
  mcq: 'MCQ',
  interview: 'Interview',
};

const DIFFICULTY_LABELS: Record<Difficulty, string> = {
  easy: 'Easy',
  medium: 'Medium',
  hard: 'Hard',
};

const DIFFICULTY_TOGGLE_CLASSES: Record<Difficulty, string> = {
  easy:
    'data-[state=on]:border-green-400 data-[state=on]:bg-green-100 data-[state=on]:text-green-900 dark:data-[state=on]:border-green-500/40 dark:data-[state=on]:bg-green-500/15 dark:data-[state=on]:text-green-400',
  medium:
    'data-[state=on]:border-amber-400 data-[state=on]:bg-amber-100 data-[state=on]:text-amber-900 dark:data-[state=on]:border-amber-500/40 dark:data-[state=on]:bg-amber-500/15 dark:data-[state=on]:text-amber-400',
  hard:
    'data-[state=on]:border-red-400 data-[state=on]:bg-red-100 data-[state=on]:text-red-900 dark:data-[state=on]:border-red-500/40 dark:data-[state=on]:bg-red-500/15 dark:data-[state=on]:text-red-400',
};

function currentHourBucket() {
  const now = new Date();
  return `${now.getFullYear()}-${now.getMonth()}-${now.getDate()}-${now.getHours()}`;
}

function chooseSessionPhrase() {
  if (typeof window === 'undefined') return SESSION_PHRASES[0];

  const bucket = currentHourBucket();

  try {
    const stored = window.sessionStorage.getItem(PHRASE_STORAGE_KEY);
    if (stored) {
      const parsed = JSON.parse(stored) as { bucket?: string; phrase?: string };
      if (parsed.bucket === bucket && parsed.phrase && SESSION_PHRASES.includes(parsed.phrase)) {
        return parsed.phrase;
      }
    }

    const phrase = SESSION_PHRASES[Math.floor(Math.random() * SESSION_PHRASES.length)];
    window.sessionStorage.setItem(PHRASE_STORAGE_KEY, JSON.stringify({ bucket, phrase }));
    return phrase;
  } catch {
    return SESSION_PHRASES[0];
  }
}

export function GeneratePage() {
  const shouldReduceMotion = useReducedMotion();
  const [sessionPhrase] = useState(chooseSessionPhrase);
  const [view, setView] = useState<GenerateView>('command');
  const [format, setFormat] = useState<GenerateFormat>('problem');
  const [language, setLanguage] = useState('python');
  const [difficulty, setDifficulty] = useState<Difficulty>('medium');
  const [prompt, setPrompt] = useState('');
  const [generating, setGenerating] = useState(false);
  const [status, setStatus] = useState<string | null>(null);
  const [showQuestions, setShowQuestions] = useState(false);
  const [agentQuestions, setAgentQuestions] = useState<Question[]>([]);

  const recordGenerateEvent = async (input: Parameters<typeof buildGenerateEvent>[0]) => {
    try {
      await createMemoryEvent(buildGenerateEvent(input));
    } catch (error) {
      console.warn('[generate] failed to record memory event', error);
    }
  };

  const handleGenerate = async () => {
    if (!prompt.trim() || generating) return;
    setGenerating(true);
    setStatus(null);

    const requestContext = {
      prompt: prompt.trim(),
      format,
      language,
      difficulty,
      view,
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
  };

  return (
    <div className="min-h-screen bg-background-100 px-6 py-8">
      <div className="mx-auto flex w-full max-w-[1200px] justify-end">
        <div className="flex items-center gap-3">
          <span className="text-muted-foreground hidden text-xs sm:inline">Generate Style</span>
          <SlidingTabs
            ariaLabel="Generate style"
            value={view}
            onChange={setView}
            options={[
              { value: 'command', label: 'Command' },
              { value: 'spotlight', label: 'Spotlight' },
            ]}
          />
        </div>
      </div>

      <AnimatePresence mode="wait" initial={false}>
        {view === 'command' ? (
          <motion.div
            key="command"
            initial={shouldReduceMotion ? false : { opacity: 0, y: 10 }}
            animate={{ opacity: 1, y: 0 }}
            exit={shouldReduceMotion ? undefined : { opacity: 0, y: -6 }}
            transition={{ duration: shouldReduceMotion ? 0 : 0.18, ease: [0.175, 0.885, 0.32, 1.1] }}
          >
            <CommandGenerateView
              difficulty={difficulty}
              format={format}
              generating={generating}
              language={language}
              prompt={prompt}
              status={status}
              onDifficultyChange={setDifficulty}
              onFormatChange={setFormat}
              onGenerate={handleGenerate}
              onKeyDown={handleKeyDown}
              onLanguageChange={setLanguage}
              onPromptChange={setPrompt}
              onUsePrompt={usePrompt}
            />
          </motion.div>
        ) : (
          <motion.div
            key="spotlight"
            initial={shouldReduceMotion ? false : { opacity: 0, y: 10 }}
            animate={{ opacity: 1, y: 0 }}
            exit={shouldReduceMotion ? undefined : { opacity: 0, y: -6 }}
            transition={{ duration: shouldReduceMotion ? 0 : 0.18, ease: [0.175, 0.885, 0.32, 1.1] }}
          >
            <SpotlightGenerateView
              generating={generating}
              prompt={prompt}
              sessionPhrase={sessionPhrase}
              status={status}
              onGenerate={handleGenerate}
              onKeyDown={handleKeyDown}
              onPromptChange={setPrompt}
              onUsePrompt={usePrompt}
            />
          </motion.div>
        )}
      </AnimatePresence>

      {showQuestions && agentQuestions.length > 0 && (
        <QuestionModal
          questions={agentQuestions}
          onComplete={handleQuestionsComplete}
          onClose={() => setShowQuestions(false)}
        />
      )}
    </div>
  );
}

function CommandGenerateView({
  difficulty,
  format,
  generating,
  language,
  prompt,
  status,
  onDifficultyChange,
  onFormatChange,
  onGenerate,
  onKeyDown,
  onLanguageChange,
  onPromptChange,
  onUsePrompt,
}: {
  difficulty: Difficulty;
  format: GenerateFormat;
  generating: boolean;
  language: string;
  prompt: string;
  status: string | null;
  onDifficultyChange: (difficulty: Difficulty) => void;
  onFormatChange: (format: GenerateFormat) => void;
  onGenerate: () => void;
  onKeyDown: (event: React.KeyboardEvent) => void;
  onLanguageChange: (language: string) => void;
  onPromptChange: (prompt: string) => void;
  onUsePrompt: (prompt: string) => void;
}) {
  return (
    <main className="flex flex-col items-center px-0 pb-12 pt-14">
      <div className="w-full max-w-3xl">
        <div className="mb-8">
          <h1 className="text-[40px] font-semibold leading-[48px] tracking-[-2.4px] text-foreground">
            What do you want to practice?
          </h1>
          <div className="mt-5 flex flex-wrap items-center justify-between gap-3 md:flex-nowrap">
            <div className="flex min-w-0 flex-1 items-center gap-2 text-sm text-muted-foreground">
              <span className="relative flex h-3 w-3 items-center justify-center rounded-full bg-blue-100 dark:bg-blue-500/20">
                <span className="h-1.5 w-1.5 rounded-full bg-blue-700 dark:bg-blue-400" />
              </span>
              <span>
                Tuned to your history - lately you worked on{' '}
                <strong className="font-semibold text-foreground">Graphs</strong> and{' '}
                <strong className="font-semibold text-foreground">Concurrency</strong>.
              </span>
            </div>
            <Button variant="link" className="h-auto px-1 text-blue-700 dark:text-blue-400" asChild>
              <Link to="/memory">View Memory</Link>
            </Button>
          </div>
        </div>

        <Card className="gap-0 overflow-hidden py-0">
          <CardHeader className="flex-row flex-wrap items-center justify-between gap-3 border-b bg-muted/40 px-4 py-3">
            <Tabs
              value={format}
              onValueChange={(value: string) => onFormatChange(value as GenerateFormat)}
            >
              <TabsList aria-label="Problem format">
                {(['problem', 'mcq', 'interview'] as GenerateFormat[]).map((item) => (
                  <TabsTrigger key={item} value={item}>
                    {FORMAT_LABELS[item]}
                  </TabsTrigger>
                ))}
              </TabsList>
            </Tabs>

            <Select value={language} onValueChange={onLanguageChange}>
              <SelectTrigger className="h-9 w-[148px]" aria-label="Language">
                <span className="h-2 w-2 rounded-sm bg-blue-700 dark:bg-blue-400" aria-hidden="true" />
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="python">Python</SelectItem>
                <SelectItem value="typescript">TypeScript</SelectItem>
                <SelectItem value="go">Go</SelectItem>
                <SelectItem value="java">Java</SelectItem>
              </SelectContent>
            </Select>
          </CardHeader>

          <CardContent className="p-0">
            <Textarea
              value={prompt}
              onChange={(event) => onPromptChange(event.target.value)}
              onKeyDown={onKeyDown}
              placeholder="Describe a challenge - e.g. a hard problem on topological sort with cycle detection, with tricky edge cases..."
              rows={5}
              className="min-h-36 resize-none rounded-none border-0 bg-transparent px-4 py-5 text-sm leading-6 shadow-none focus-visible:ring-0"
            />
          </CardContent>

          <CardFooter className="flex-wrap items-center justify-between gap-4 border-t bg-muted/40 px-4 py-3">
            <div className="flex flex-wrap items-center gap-2">
              <Label className="text-xs text-muted-foreground">Level</Label>
              <ToggleGroup
                type="single"
                variant="outline"
                size="lg"
                spacing={0}
                value={difficulty}
                onValueChange={(value: string) => {
                  if (value) onDifficultyChange(value as Difficulty);
                }}
              >
                {(['easy', 'medium', 'hard'] as Difficulty[]).map((item) => (
                  <ToggleGroupItem
                    key={item}
                    value={item}
                    aria-label={DIFFICULTY_LABELS[item]}
                    className={cn('min-w-20', DIFFICULTY_TOGGLE_CLASSES[item])}
                  >
                    {DIFFICULTY_LABELS[item]}
                  </ToggleGroupItem>
                ))}
              </ToggleGroup>
            </div>

            <Button
              type="button"
              disabled={generating || !prompt.trim()}
              onClick={onGenerate}
              className="gap-2"
            >
              {generating ? (
                <Spinner className="size-4" />
              ) : (
                <>
                  <span>Generate</span>
                  <kbd className="pointer-events-none hidden h-5 select-none items-center gap-1 rounded border border-primary-foreground/20 bg-primary-foreground/10 px-1.5 font-mono text-[10px] font-medium sm:inline-flex">
                    <CommandIcon className="size-2.5" />
                    Enter
                  </kbd>
                </>
              )}
            </Button>
          </CardFooter>
        </Card>

        <div className="mt-6 flex flex-wrap items-center gap-2">
          <Label className="text-xs text-muted-foreground">Recent</Label>
          {RECENT_PROMPTS.map((item) => (
            <Button
              key={item}
              type="button"
              variant="outline"
              size="sm"
              onClick={() => onUsePrompt(item)}
            >
              {item}
            </Button>
          ))}
        </div>

        <CommandGenerationState generating={generating} status={status} />
      </div>
    </main>
  );
}

function SpotlightGenerateView({
  generating,
  prompt,
  sessionPhrase,
  status,
  onGenerate,
  onKeyDown,
  onPromptChange,
  onUsePrompt,
}: {
  generating: boolean;
  prompt: string;
  sessionPhrase: string;
  status: string | null;
  onGenerate: () => void;
  onKeyDown: (event: React.KeyboardEvent) => void;
  onPromptChange: (prompt: string) => void;
  onUsePrompt: (prompt: string) => void;
}) {
  return (
    <main className="flex items-start justify-center px-0 pt-16">
      <div className="w-full max-w-4xl text-center">
        <div className="mb-10">
          <div className="mb-5 font-mono text-xs tracking-[0.2em] text-gray-700 uppercase">
            AI Problem Engine
          </div>
          <h1 className="text-[48px] font-semibold leading-[56px] tracking-[-2.88px] text-gray-1000 sm:whitespace-nowrap xl:text-[64px] xl:leading-[64px] xl:tracking-[-3.84px]">
            {sessionPhrase}
          </h1>
          <p className="mx-auto mt-8 max-w-2xl text-xl leading-9 text-gray-900">
            One prompt becomes a unique problem, a test suite, and a sandbox to prove your
            solution in.
          </p>
        </div>

        <div
          className="mx-auto flex max-w-3xl items-center rounded-xl border border-gray-alpha-200 bg-background-100 p-2"
          style={{ boxShadow: 'var(--cg-card-shadow)' }}
        >
          <input
            type="text"
            value={prompt}
            onChange={(event) => onPromptChange(event.target.value)}
            onKeyDown={onKeyDown}
            placeholder="Describe what you want to practice..."
            className="min-w-0 flex-1 bg-transparent px-4 text-base text-gray-1000 placeholder-gray-700 focus-visible:outline-none"
          />
          <GenerateButton
            disabled={generating || !prompt.trim()}
            generating={generating}
            label="Generate"
            onClick={onGenerate}
            variant="blueprint"
          />
        </div>

        <div className="mt-10 flex flex-wrap justify-center gap-3">
          {SPOTLIGHT_TOPICS.map((topic) => (
            <motion.button
              key={topic.label}
              type="button"
              onClick={() => onUsePrompt(topic.label)}
              whileHover={{ y: -2 }}
              whileTap={{ scale: 0.98 }}
              className="cg-focus flex min-w-56 items-center justify-center gap-3 rounded-xl border border-gray-alpha-200 bg-background-100 px-5 py-4 text-sm text-gray-1000"
              style={{ boxShadow: '0 1px 2px rgba(0,0,0,0.04)' }}
            >
              <span
                className="h-2.5 w-2.5 rounded-sm"
                style={{ backgroundColor: topic.color }}
                aria-hidden="true"
              />
              <span className="font-semibold">{topic.label}</span>
              <span className="text-gray-700">.{topic.count}</span>
            </motion.button>
          ))}
        </div>

        <div className="mt-8 flex items-center justify-center gap-2 text-sm text-gray-700">
          <span className="relative flex h-3 w-3 items-center justify-center rounded-full bg-blue-100">
            <span className="h-1.5 w-1.5 rounded-full bg-blue-700" />
          </span>
          <span>Personalized from your recent activity - 3 skills tracked</span>
        </div>

        <div className="mt-8 flex flex-wrap justify-center gap-2">
          {EXAMPLES.slice(0, 3).map((example) => (
            <button
              key={example}
              type="button"
              onClick={() => onUsePrompt(example)}
              className="cg-focus h-9 rounded-full border border-gray-alpha-200 bg-background-100 px-4 text-sm text-gray-900 transition-colors hover:border-gray-alpha-400 hover:text-gray-1000"
            >
              {example}
            </button>
          ))}
        </div>

        <GenerationState generating={generating} status={status} />
      </div>
    </main>
  );
}

function GenerateButton({
  disabled,
  generating,
  label,
  onClick,
  variant,
}: {
  disabled: boolean;
  generating: boolean;
  label: string;
  onClick: () => void;
  variant: 'ink' | 'blueprint';
}) {
  const isBlueprint = variant === 'blueprint';
  const shouldReduceMotion = useReducedMotion();

  return (
    <motion.button
      type="button"
      onClick={onClick}
      disabled={disabled}
      whileHover={disabled || shouldReduceMotion ? undefined : { y: -1 }}
      whileTap={disabled || shouldReduceMotion ? undefined : { scale: 0.97 }}
      transition={{ type: 'spring', stiffness: 500, damping: 34 }}
      className={`cg-focus flex h-10 shrink-0 items-center gap-2 rounded-md px-4 text-sm font-medium transition-colors disabled:cursor-not-allowed disabled:bg-gray-100 disabled:text-gray-700 ${
        isBlueprint
          ? 'bg-blue-700 text-white hover:bg-blue-800'
          : 'bg-gray-1000 text-background-100 hover:bg-gray-900'
      }`}
      aria-label={label}
    >
      {generating ? (
        <GridSpinner size="sm" />
      ) : isBlueprint ? (
        <ArrowRight size={18} strokeWidth={2.2} />
      ) : (
        <>
          <span>{label}</span>
          <kbd className="font-mono rounded-[5px] bg-white/15 px-1.5 py-0.5 text-[11px] text-white/80">
            <CommandIcon size={10} strokeWidth={2} className="inline" /> Enter
          </kbd>
        </>
      )}
    </motion.button>
  );
}

function CommandGenerationState({
  generating,
  status,
}: {
  generating: boolean;
  status: string | null;
}) {
  if (generating) {
    return (
      <div className="cg-fade-in mt-12 flex flex-col items-center gap-4">
        <Spinner className="size-6" />
        <span className="font-mono text-xs tracking-[0.16em] text-muted-foreground uppercase">
          Generating
        </span>
      </div>
    );
  }

  if (!status) return null;

  return (
    <Card className="cg-fade-in mx-auto mt-6 max-w-xl gap-0 py-3">
      <CardContent className="flex items-start gap-2 px-4 py-0 text-sm text-muted-foreground">
        <Sparkles className="mt-0.5 size-3.5 shrink-0" />
        <span>{status}</span>
      </CardContent>
    </Card>
  );
}

function GenerationState({
  generating,
  status,
}: {
  generating: boolean;
  status: string | null;
}) {
  if (generating) {
    return (
      <div className="cg-fade-in mt-12 flex flex-col items-center gap-4">
        <GridSpinner size="md" />
        <span className="font-mono text-xs tracking-[0.16em] text-gray-700 uppercase">
          Generating
        </span>
      </div>
    );
  }

  if (!status) return null;

  return (
    <div className="cg-fade-in mx-auto mt-6 max-w-xl rounded-xl border border-gray-alpha-200 bg-background-100 px-4 py-3 text-sm text-gray-900">
      <Sparkles size={14} strokeWidth={1.8} className="mr-2 inline text-gray-700" />
      {status}
    </div>
  );
}
