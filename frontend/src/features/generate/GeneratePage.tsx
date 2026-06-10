import { useState } from 'react';
import { motion } from 'motion/react';
import {
  ArrowRight,
  ChevronDown,
  Command,
  Sparkles,
} from 'lucide-react';
import { Link } from 'react-router-dom';
import { GridSpinner } from '../../shared/components/GridSpinner';
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
  { label: 'DSA', count: 128, color: 'var(--color-moss)' },
  { label: 'API Patterns', count: 64, color: 'var(--color-cobalt)' },
  { label: 'System Design', count: 52, color: 'var(--color-violet)' },
  { label: 'Concurrency', count: 37, color: 'var(--color-tangerine)' },
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
    color: 'var(--color-moss)',
    background: 'var(--color-moss-tint)',
    borderColor: 'var(--color-moss)',
  },
  medium: {
    color: 'var(--color-honey)',
    background: 'var(--color-honey-tint)',
    borderColor: 'var(--color-honey)',
  },
  hard: {
    color: 'var(--color-rust)',
    background: 'var(--color-rust-tint)',
    borderColor: 'var(--color-rust)',
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
    <div className="min-h-screen bg-shell px-6 py-8">
      <div className="mx-auto flex w-full max-w-5xl justify-end">
        <div className="flex items-center gap-3">
          <span className="cg-mono hidden text-[10px] font-medium tracking-[0.2em] text-faint uppercase sm:inline">
            Generate style
          </span>
          <div className="flex items-center rounded-xl border bg-grain p-1 cg-border-subtle">
            {(['command', 'spotlight'] as GenerateView[]).map((mode) => (
              <button
                key={mode}
                type="button"
                onClick={() => setView(mode)}
                className={`cg-focus cg-transition rounded-lg px-3 py-1.5 text-xs font-medium capitalize ${
                  view === mode
                    ? 'bg-shell text-ink shadow-[0_1px_2px_rgba(0,0,0,0.04)]'
                    : 'text-ash hover:text-ink'
                }`}
              >
                {mode}
              </button>
            ))}
          </div>
        </div>
      </div>

      {view === 'command' ? (
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
      ) : (
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
      )}

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
          <h1 className="font-display text-4xl font-semibold tracking-tight text-ink">
            What do you want to <span className="marker-tangerine">practice</span>?
          </h1>
          <div className="mt-5 flex flex-wrap items-center justify-between gap-3 md:flex-nowrap">
            <div className="flex min-w-0 flex-1 items-center gap-2 text-xs text-graphite">
              <span className="relative flex h-3 w-3 items-center justify-center rounded-full bg-grain">
                <span className="h-1.5 w-1.5 rounded-full bg-blue" />
              </span>
              <span>
                Tuned to your history - lately you worked on{' '}
                <strong className="font-semibold text-ink">Graphs</strong> and{' '}
                <strong className="font-semibold text-ink">Concurrency</strong>.
              </span>
            </div>
            <Link
              to="/memory"
              className="cg-focus rounded-md px-1 text-xs font-medium text-blue underline-offset-4 hover:text-blue-hover hover:underline"
            >
              View memory
            </Link>
          </div>
        </div>

        <section
          className="overflow-hidden rounded-2xl bg-shell cg-surface"
        >
          <div className="flex flex-wrap items-center justify-between gap-3 border-b bg-grain px-4 py-3 cg-border-subtle">
            <div className="flex rounded-xl bg-grain p-1">
              {(['problem', 'mcq', 'interview'] as GenerateFormat[]).map((item) => (
                <button
                  key={item}
                  type="button"
                  onClick={() => onFormatChange(item)}
                  className={`cg-focus cg-mono cg-transition rounded-lg px-4 py-2 text-xs font-medium ${
                    format === item
                      ? 'bg-shell text-ink shadow-[0_1px_2px_rgba(0,0,0,0.04)]'
                      : 'text-ash hover:text-ink'
                  }`}
                >
                  {FORMAT_LABELS[item]}
                </button>
              ))}
            </div>

            <label className="flex items-center gap-2 rounded-xl border bg-shell px-3 py-2 text-xs text-graphite cg-border-subtle">
              <span className="h-2 w-2 rounded-sm bg-blue" aria-hidden="true" />
              <select
                value={language}
                onChange={(event) => onLanguageChange(event.target.value)}
                className="cg-mono appearance-none bg-transparent text-xs text-graphite focus-visible:outline-none"
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
            className="cg-scroll block min-h-36 w-full resize-none bg-shell px-4 py-5 text-sm leading-7 text-ink placeholder-ash focus-visible:outline-none"
          />

          <div className="flex flex-wrap items-center justify-between gap-4 border-t bg-grain px-4 py-3 cg-border-subtle">
            <div className="flex items-center gap-2">
              <span className="cg-mono mr-2 text-[10px] font-medium tracking-[0.18em] text-faint uppercase">
                Level
              </span>
              {(['easy', 'medium', 'hard'] as Difficulty[]).map((item) => (
                <button
                  key={item}
                  type="button"
                  onClick={() => onDifficultyChange(item)}
                  className={`cg-focus cg-transition rounded-xl border px-4 py-2 text-xs font-medium ${
                    difficulty === item
                      ? 'border-transparent'
                      : 'bg-shell text-graphite hover:text-ink cg-border-subtle'
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
          <span className="cg-mono mr-2 text-[10px] font-medium tracking-[0.18em] text-faint uppercase">
            Recent
          </span>
          {RECENT_PROMPTS.map((item) => (
            <button
              key={item}
              type="button"
              onClick={() => onUsePrompt(item)}
              className="cg-focus cg-transition rounded-xl border bg-shell px-4 py-2 text-xs text-graphite hover:text-ink cg-border-subtle"
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
          <div className="mb-5 text-[10px] font-bold tracking-[0.32em] text-tangerine uppercase">
            AI Problem Engine
          </div>
          <h1 className="font-display text-4xl font-semibold tracking-tight text-ink sm:whitespace-nowrap md:text-5xl xl:text-6xl">
            <em className="marker-tangerine not-italic">{sessionPhrase}</em>
          </h1>
          <p className="mx-auto mt-8 max-w-2xl text-lg leading-8 text-graphite">
            One prompt becomes a unique problem, a test suite, and a sandbox to prove your
            solution in.
          </p>
        </div>

        <div
          className="mx-auto flex max-w-3xl items-center rounded-2xl bg-shell p-2 cg-surface"
        >
          <input
            type="text"
            value={prompt}
            onChange={(event) => onPromptChange(event.target.value)}
            onKeyDown={onKeyDown}
            placeholder="Describe what you want to practice..."
            className="min-w-0 flex-1 bg-transparent px-4 text-sm text-ink placeholder-ash focus-visible:outline-none"
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
            <button
              key={topic.label}
              type="button"
              onClick={() => onUsePrompt(topic.label)}
              className="cg-focus cg-transition flex min-w-56 items-center justify-center gap-3 rounded-2xl border bg-shell px-5 py-4 text-sm text-ink hover:-translate-y-px cg-border-subtle"
              style={{ boxShadow: '0 1px 2px rgba(0,0,0,0.04)' }}
            >
              <span
                className="h-2.5 w-2.5 rounded-sm"
                style={{ backgroundColor: topic.color }}
                aria-hidden="true"
              />
              <span className="font-semibold">{topic.label}</span>
              <span className="text-ash">.{topic.count}</span>
            </button>
          ))}
        </div>

        <div className="mt-8 flex items-center justify-center gap-2 text-xs text-ash">
          <span className="relative flex h-3 w-3 items-center justify-center rounded-full bg-grain">
            <span className="h-1.5 w-1.5 rounded-full bg-blue" />
          </span>
          <span>Personalized from your recent activity - 3 skills tracked</span>
        </div>

        <div className="mt-8 flex flex-wrap justify-center gap-2">
          {EXAMPLES.slice(0, 3).map((example) => (
            <button
              key={example}
              type="button"
              onClick={() => onUsePrompt(example)}
              className="cg-focus cg-transition rounded-full border bg-shell px-4 py-2 text-xs text-graphite hover:text-ink cg-border-subtle"
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

  return (
    <motion.button
      type="button"
      onClick={onClick}
      disabled={disabled}
      whileTap={disabled ? undefined : { scale: 0.96 }}
      transition={{ type: 'spring', stiffness: 500, damping: 30 }}
      className={`cg-focus cg-transition flex h-11 shrink-0 items-center gap-2 rounded-xl px-4 text-sm font-semibold disabled:cursor-not-allowed disabled:bg-chalk disabled:text-ash ${
        isBlueprint
          ? 'bg-tangerine text-shell hover:bg-tangerine-deep'
          : 'bg-tangerine text-shell hover:bg-tangerine-deep'
      }`}
      style={{ boxShadow: disabled ? undefined : '2px 2px 0 0 var(--color-ink)' }}
      aria-label={label}
    >
      {generating ? (
        <GridSpinner size="sm" />
      ) : isBlueprint ? (
        <ArrowRight size={18} strokeWidth={2.2} />
      ) : (
        <>
          <span>{label}</span>
          <kbd className="cg-mono rounded-md bg-white/15 px-1.5 py-0.5 text-[10px] text-white/80">
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
        <span className="cg-mono text-[10px] font-medium tracking-[0.18em] text-ash uppercase">
          Generating
        </span>
      </div>
    );
  }

  if (!status) return null;

  return (
    <div className="cg-fade-in mx-auto mt-6 max-w-xl rounded-xl border bg-shell px-4 py-3 text-xs text-graphite cg-border-subtle">
      <Sparkles size={14} strokeWidth={1.8} className="mr-2 inline text-ash" />
      {status}
    </div>
  );
}
