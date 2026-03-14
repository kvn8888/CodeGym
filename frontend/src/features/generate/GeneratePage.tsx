import { useState, useRef, useEffect, useCallback } from 'react';
import { GridSpinner } from '../../shared/components/GridSpinner';

const HEADLINES = [
  'Generate any problem.',
  'Practice on demand.',
  'Build real fluency.',
  'Learn by doing.',
  'Ship better code.',
];

function useTypewriter(phrases: string[], typingSpeed = 60, deletingSpeed = 35, pauseMs = 1800) {
  const [displayed, setDisplayed] = useState('');
  const [phraseIndex, setPhraseIndex] = useState(0);
  const [isDeleting, setIsDeleting] = useState(false);

  const tick = useCallback(() => {
    const current = phrases[phraseIndex];
    if (!isDeleting) {
      setDisplayed(current.slice(0, displayed.length + 1));
      if (displayed.length + 1 === current.length) {
        setTimeout(() => setIsDeleting(true), pauseMs);
        return;
      }
    } else {
      setDisplayed(current.slice(0, displayed.length - 1));
      if (displayed.length - 1 === 0) {
        setIsDeleting(false);
        setPhraseIndex((i) => (i + 1) % phrases.length);
        return;
      }
    }
  }, [displayed, isDeleting, phraseIndex, phrases, pauseMs]);

  useEffect(() => {
    const delay = isDeleting ? deletingSpeed : typingSpeed;
    const timer = setTimeout(tick, delay);
    return () => clearTimeout(timer);
  }, [tick, isDeleting, typingSpeed, deletingSpeed]);

  return displayed;
}

const EXAMPLES = [
  'Pagination API pattern in Express',
  'Iterator pattern in Java',
  'Go goroutines for fan-out/fan-in',
  'REST API with Python FastAPI',
  'Linked list implementation in C++',
  'Simple neural network with PyTorch',
];

// Topic chips shown below the prompt bar for quick-start generation.
const TOPIC_CHIPS = [
  { label: 'DSA', icon: '🧩' },
  { label: 'API Patterns', icon: '🔌' },
  { label: 'System Design', icon: '🏗️' },
  { label: 'Algorithms', icon: '⚡' },
  { label: 'Data Structures', icon: '📦' },
];

const DIFFICULTIES = ['Beginner', 'Junior', 'Senior'] as const;

// Maps difficulty level to a prompt suffix for the generation agent.
const DIFFICULTY_PROMPTS: Record<string, string> = {
  Beginner: 'Make this problem beginner-friendly: use simple inputs, provide detailed hints, and focus on fundamental concepts.',
  Junior: 'Target a junior developer level: moderate complexity, some edge cases, and practical real-world relevance.',
  Senior: 'Make this senior-level: complex edge cases, performance constraints, system-design considerations, and minimal hand-holding.',
};

export function GeneratePage() {
  const headline = useTypewriter(HEADLINES);
  const [prompt, setPrompt] = useState('');
  const [difficulty, setDifficulty] = useState<string>('Junior');
  const [generating, setGenerating] = useState(false);
  const [status, setStatus] = useState<string | null>(null);
  const [showExamples, setShowExamples] = useState(false);
  const barRef = useRef<HTMLDivElement>(null);

  const handleGenerate = async () => {
    if (!prompt.trim() || generating) return;
    setGenerating(true);
    setStatus(null);
    setShowExamples(false);
    // Build the full prompt with difficulty context appended
    const fullPrompt = `${prompt.trim()} ${DIFFICULTY_PROMPTS[difficulty] ?? ''}`.trim();
    // TODO: Call /api/v1/generate with fullPrompt and poll for status
    console.log('[generate]', fullPrompt);
    setTimeout(() => {
      setStatus('Generation endpoint not yet implemented');
      setGenerating(false);
    }, 3000);
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
      handleGenerate();
    }
  };

  const handleSelectExample = (example: string) => {
    setPrompt(example);
    setShowExamples(false);
  };

  // Close dropdown on outside click
  useEffect(() => {
    const handleClick = (e: MouseEvent) => {
      if (barRef.current && !barRef.current.contains(e.target as Node)) {
        setShowExamples(false);
      }
    };
    document.addEventListener('mousedown', handleClick);
    return () => document.removeEventListener('mousedown', handleClick);
  }, []);

  return (
    <div className="max-w-2xl mx-auto px-6 py-24">
      <div className="mb-10 h-10 flex items-end" aria-live="polite" aria-label={headline}>
        <h1 className="text-2xl font-bold text-ink tracking-tight leading-tight text-balance">
          {headline}
          <span className="inline-block w-[2px] h-[1.1em] bg-ink ml-[2px] align-middle animate-[blink_1s_step-end_infinite]" aria-hidden="true" />
        </h1>
      </div>

      <div ref={barRef} className="relative">
        <div
          className="flex items-center rounded-2xl border border-chalk bg-white px-4 py-3"
          style={{ boxShadow: '0 1px 3px rgba(0, 0, 0, 0.04), 0 4px 12px rgba(0, 0, 0, 0.03)' }}
        >
          <input
            type="text"
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
            onKeyDown={handleKeyDown}
            onFocus={() => !prompt && setShowExamples(true)}
            placeholder="Describe what you want to practice..."
            className="flex-1 bg-transparent text-sm text-ink placeholder-ash focus:outline-none"
          />

          {/* Difficulty selector pill — styled like screen reference */}
          <select
            value={difficulty}
            onChange={(e) => setDifficulty(e.target.value)}
            className="shrink-0 rounded-full border border-chalk bg-parchment px-3 py-1.5 text-xs text-ink cursor-pointer focus:outline-none appearance-none pr-7 ml-2"
            style={{
              backgroundImage: `url("data:image/svg+xml,%3Csvg width='10' height='10' viewBox='0 0 10 10' fill='none' xmlns='http://www.w3.org/2000/svg'%3E%3Cpath d='M2.5 4L5 6.5L7.5 4' stroke='%236b6b6b' stroke-width='1.5' stroke-linecap='round' stroke-linejoin='round'/%3E%3C/svg%3E")`,
              backgroundRepeat: 'no-repeat',
              backgroundPosition: 'right 8px center',
            }}
          >
            {DIFFICULTIES.map((d) => (
              <option key={d} value={d}>{d}</option>
            ))}
          </select>

          {/* Generate button — send arrow */}
          <button
            onClick={handleGenerate}
            disabled={generating || !prompt.trim()}
            className="w-9 h-9 bg-ink text-bone rounded-xl flex items-center justify-center shrink-0 ml-3 hover:bg-ink-soft disabled:bg-chalk disabled:text-ash disabled:cursor-not-allowed transition-colors"
            aria-label="Generate"
          >
            <svg
              width="14"
              height="14"
              viewBox="0 0 16 16"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
            >
              <path d="M3 8h10M9 4l4 4-4 4" />
            </svg>
          </button>
        </div>

        {/* Example prompts dropdown */}
        {showExamples && (
          <div
            className="absolute left-0 right-0 top-full mt-2 border border-chalk rounded-2xl bg-white overflow-hidden z-10"
            style={{ boxShadow: '0 4px 16px rgba(0, 0, 0, 0.08)' }}
          >
            <div className="px-4 py-2 border-b border-chalk">
              <span className="text-[10px] font-bold tracking-[0.2em] text-ash uppercase">
                Examples
              </span>
            </div>
            {EXAMPLES.map((example) => (
              <button
                key={example}
                onClick={() => handleSelectExample(example)}
                className="block w-full text-left text-xs text-graphite hover:text-ink hover:bg-parchment px-4 py-2.5 transition-colors"
              >
                {example}
              </button>
            ))}
          </div>
        )}
      </div>

      {!showExamples && (
        <div className="mt-4 flex flex-wrap gap-2">
          {TOPIC_CHIPS.map((chip) => (
            <button
              key={chip.label}
              onClick={() => setPrompt(chip.label + ' ')}
              className="flex items-center gap-1.5 px-3.5 py-2 rounded-full border border-chalk bg-white text-xs text-graphite hover:text-ink hover:border-ash transition-colors"
              style={{ boxShadow: '0 1px 3px rgba(0,0,0,0.04)' }}
            >
              <span>{chip.icon}</span>
              {chip.label}
            </button>
          ))}
        </div>
      )}

      {generating && (
        <div className="mt-16 flex flex-col items-center gap-4">
          <GridSpinner size="md" />
          <span className="text-[10px] text-ash tracking-[0.15em]">GENERATING</span>
        </div>
      )}

      {status && !generating && (
        <div className="mt-6 text-xs text-graphite border border-chalk rounded-lg px-4 py-3">
          {status}
        </div>
      )}
    </div>
  );
}
