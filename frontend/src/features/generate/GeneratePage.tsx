import { useState } from 'react';

export function GeneratePage() {
  const [prompt, setPrompt] = useState('');
  const [generating, setGenerating] = useState(false);
  const [status, setStatus] = useState<string | null>(null);

  const handleGenerate = async () => {
    if (!prompt.trim()) return;
    setGenerating(true);
    setStatus('Generating problem...');
    // TODO: Call /api/v1/generate and poll for status
    setTimeout(() => {
      setStatus('Generation endpoint not yet implemented');
      setGenerating(false);
    }, 1000);
  };

  return (
    <div className="max-w-2xl mx-auto px-4 py-12">
      <h1 className="text-2xl font-bold mb-2">Generate a Problem</h1>
      <p className="text-slate-400 mb-6">
        Describe what you want to practice. The AI will generate a problem, tests, and skeleton
        code.
      </p>

      <div className="space-y-4">
        <textarea
          value={prompt}
          onChange={(e) => setPrompt(e.target.value)}
          placeholder="e.g., I want to try the pagination API pattern in Express"
          rows={4}
          className="w-full bg-slate-800 border border-slate-700 rounded-lg p-4 text-slate-200 placeholder-slate-500 resize-none focus:outline-none focus:border-blue-500"
        />

        <div className="flex items-center gap-4">
          <button
            onClick={handleGenerate}
            disabled={generating || !prompt.trim()}
            className="px-6 py-2 bg-blue-600 hover:bg-blue-700 disabled:bg-slate-700 disabled:text-slate-500 text-white font-medium rounded-lg transition-colors"
          >
            {generating ? 'Generating...' : 'Generate Problem'}
          </button>
          {status && <span className="text-sm text-slate-400">{status}</span>}
        </div>
      </div>

      <div className="mt-8 border-t border-slate-700 pt-6">
        <h3 className="text-sm font-medium text-slate-400 mb-3">Example prompts</h3>
        <div className="space-y-2">
          {[
            'I want to try the pagination API pattern in Express',
            'I want to practice the iterator pattern in Java',
            'I want to try using Go goroutines for fan-out/fan-in',
            'I want to practice REST API concepts in Python with FastAPI',
            'I want to try implementing a linked list in C++',
            'I want to practice using PyTorch for a simple neural network',
          ].map((example) => (
            <button
              key={example}
              onClick={() => setPrompt(example)}
              className="block w-full text-left text-sm text-slate-400 hover:text-blue-400 bg-slate-800/50 hover:bg-slate-800 border border-slate-700 rounded-md px-3 py-2 transition-colors"
            >
              {example}
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}
