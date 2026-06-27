import { useState } from 'react';
import { AnimatePresence, motion, useReducedMotion } from 'motion/react';
import {
  ArrowRight,
  ChevronDown,
  Command,
  Sparkles,
} from 'lucide-react';
import { Link } from 'react-router-dom';
import { GridSpinner } from '../../shared/components/GridSpinner';
import { SlidingTabs } from '../../shared/components/SlidingTabs';
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

const DIFFICULTY_STYLES: Record<Difficulty, React.CSSProperties> = {
  easy: {
    color: 'var(--color-green-900)',
    background: 'var(--color-green-100)',
    borderColor: 'var(--color-green-400)',
  },
  medium: {
    color: 'var(--color-amber-900)',
    background: 'var(--color-amber-100)',
    borderColor: 'var(--color-amber-400)',
  },
  hard: {
    color: 'var(--color-red-900)',
    background: 'var(--color-red-100)',
    borderColor: 'var(--color-red-400)',
  },
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
          <span className="hidden text-xs text-gray-700 sm:inline">
            Generate Style
          </span>
          <div className="flex items-center rounded-md border border-gray-alpha-200 bg-gray-100 p-0.5">
            {(['command', 'spotlight'] as GenerateView[]).map((mode) => (
              <motion.button
                key={mode}
                type="button"
                onClick={() => setView(mode)}
                whileTap={shouldReduceMotion ? undefined : { scale: 0.98 }}
                className={`cg-focus rounded-[5px] px-3 py-1.5 text-sm font-medium capitalize transition-colors ${
                  view === mode
                    ? 'bg-background-100 text-gray-1000 shadow-[0_1px_1px_rgba(0,0,0,0.04)]'
                    : 'text-gray-700 hover:text-gray-1000'
                }`}
              >
                {mode}
              </motion.button>
            ))}
          </div>
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
          <h1 className="text-[40px] font-semibold leading-[48px] tracking-[-2.4px] text-gray-1000">
            What do you want to practice?
          </h1>
          <div className="mt-5 flex flex-wrap items-center justify-between gap-3 md:flex-nowrap">
            <div className="flex min-w-0 flex-1 items-center gap-2 text-sm text-gray-900">
              <span className="relative flex h-3 w-3 items-center justify-center rounded-full bg-blue-100">
                <span className="h-1.5 w-1.5 rounded-full bg-blue-700" />
              </span>
              <span>
                Tuned to your history - lately you worked on{' '}
                <strong className="font-semibold text-gray-1000">Graphs</strong> and{' '}
                <strong className="font-semibold text-gray-1000">Concurrency</strong>.
              </span>
            </div>
            <Link
              to="/memory"
              className="cg-focus rounded-md px-1 text-sm font-medium text-blue-700 underline-offset-4 hover:text-blue-800 hover:underline"
            >
              View Memory
            </Link>
          </div>
        </div>

        <section
          className="overflow-hidden rounded-xl border border-gray-alpha-200 bg-background-100"
          style={{ boxShadow: 'var(--cg-card-shadow)' }}
        >
          <div className="flex flex-wrap items-center justify-between gap-3 border-b border-gray-alpha-200 bg-background-200 px-4 py-3">
            <SlidingTabs
              ariaLabel="Problem format"
              value={format}
              onChange={onFormatChange}
              options={(['problem', 'mcq', 'interview'] as GenerateFormat[]).map((item) => ({
                value: item,
                label: FORMAT_LABELS[item],
              }))}
            />

            <label className="flex h-10 items-center gap-2 rounded-md border border-gray-alpha-200 bg-background-100 px-3 text-sm text-gray-900">
              <span className="h-2 w-2 rounded-sm bg-blue-700" aria-hidden="true" />
              <select
                value={language}
                onChange={(event) => onLanguageChange(event.target.value)}
                className="appearance-none bg-transparent text-sm text-gray-900 focus-visible:outline-none"
                aria-label="Language"
              >
                <option value="python">Python</option>
                <option value="typescript">TypeScript</option>
                <option value="go">Go</option>
                <option value="java">Java</option>
              </select>
              <ChevronDown size={14} strokeWidth={1.8} aria-hidden="true" />
            </label>
          </div>

          <textarea
            value={prompt}
            onChange={(event) => onPromptChange(event.target.value)}
            onKeyDown={onKeyDown}
            placeholder="Describe a challenge - e.g. a hard problem on topological sort with cycle detection, with tricky edge cases..."
            rows={5}
            className="cg-scroll block min-h-36 w-full resize-none bg-background-100 px-4 py-5 text-sm leading-6 text-gray-1000 placeholder-gray-700 focus-visible:outline-none"
          />

          <div className="flex flex-wrap items-center justify-between gap-4 border-t border-gray-alpha-200 bg-background-200 px-4 py-3">
            <div className="flex items-center gap-2">
              <span className="mr-2 text-xs text-gray-700">
                Level
              </span>
              {(['easy', 'medium', 'hard'] as Difficulty[]).map((item) => (
                <button
                  key={item}
                  type="button"
                  onClick={() => onDifficultyChange(item)}
                  className={`cg-focus h-10 rounded-md border px-4 text-sm font-medium transition-colors ${
                    difficulty === item
                      ? 'border-transparent'
                      : 'border-gray-alpha-200 bg-background-100 text-gray-900 hover:border-gray-alpha-400 hover:text-gray-1000'
                  }`}
                  style={difficulty === item ? DIFFICULTY_STYLES[item] : undefined}
                >
                  {DIFFICULTY_LABELS[item]}
                </button>
              ))}
            </div>

            <GenerateButton
              disabled={generating || !prompt.trim()}
              generating={generating}
              label="Generate"
              onClick={onGenerate}
              variant="ink"
            />
          </div>
        </section>

        <div className="mt-6 flex flex-wrap items-center gap-2">
          <span className="mr-2 text-xs text-gray-700">
            Recent
          </span>
          {RECENT_PROMPTS.map((item) => (
            <button
              key={item}
              type="button"
              onClick={() => onUsePrompt(item)}
              className="cg-focus h-10 rounded-md border border-gray-alpha-200 bg-background-100 px-4 text-sm text-gray-900 transition-colors hover:border-gray-alpha-400 hover:text-gray-1000"
            >
              {item}
            </button>
          ))}
        </div>

        <GenerationState generating={generating} status={status} />
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
            <Command size={10} strokeWidth={2} className="inline" /> Enter
          </kbd>
        </>
      )}
    </motion.button>
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
