import { useState } from 'react';
import { GridSpinner } from '../../shared/components/GridSpinner';

export function GeneratePage() {
  const [prompt, setPrompt] = useState('');
  const [generating, setGenerating] = useState(false);
  const [status, setStatus] = useState<string | null>(null);

  const handleGenerate = async () => {
    if (!prompt.trim() || generating) return;
    setGenerating(true);
    setStatus(null);
    // TODO: Call /api/v1/generate and poll for status
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

  return (
    <div className="max-w-2xl mx-auto px-6 py-24">
      <h1 className="text-xs font-bold tracking-[0.2em] text-ink mb-8 uppercase">Generate</h1>

      <div className="flex items-end gap-3">
        <textarea
          value={prompt}
          onChange={(e) => setPrompt(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder="Describe what you want to practice..."
          rows={3}
          className="flex-1 bg-transparent border border-ink rounded-xl px-4 py-3 text-sm text-ink placeholder-ash resize-none focus:outline-none focus:border-ink"
        />
        <button
          onClick={handleGenerate}
          disabled={generating || !prompt.trim()}
          className="w-11 h-11 bg-ink text-bone rounded-xl flex items-center justify-center shrink-0 hover:bg-ink-soft disabled:bg-chalk disabled:cursor-not-allowed transition-colors"
          aria-label="Generate"
        >
          <svg
            width="16"
            height="16"
            viewBox="0 0 16 16"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.5"
            strokeLinecap="round"
            strokeLinejoin="round"
          >
            <path d="M3 8h10M9 4l4 4-4 4" />
          </svg>
        </button>
      </div>

      <p className="text-[10px] text-ash mt-2 tracking-wide">CMD+ENTER TO SUBMIT</p>

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

      <div className="mt-16 border-t border-chalk pt-6">
        <h3 className="text-[10px] font-bold tracking-[0.2em] text-ash mb-4 uppercase">
          Example prompts
        </h3>
        <div className="space-y-1">
          {[
            'Pagination API pattern in Express',
            'Iterator pattern in Java',
            'Go goroutines for fan-out/fan-in',
            'REST API with Python FastAPI',
            'Linked list implementation in C++',
            'Simple neural network with PyTorch',
          ].map((example) => (
            <button
              key={example}
              onClick={() => setPrompt(example)}
              className="block w-full text-left text-xs text-graphite hover:text-ink px-2 py-1.5 transition-colors"
            >
              {'\u2192'} {example}
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}
