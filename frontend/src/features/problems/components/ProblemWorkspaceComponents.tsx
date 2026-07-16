import { useState } from 'react';
import {
  AlertTriangle,
  Check,
  ChevronRight,
  CircleX,
  Clock3,
  FileCode2,
  Lightbulb,
  LoaderCircle,
  Play,
  RotateCcw,
} from 'lucide-react';
import ReactMarkdown from 'react-markdown';

import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import type {
  Hint,
  Problem,
  SubmissionFile,
  TestCaseResult,
  TestResult,
} from '@/shared/api/types';

function difficultyLabel(difficulty: number) {
  if (difficulty <= 2) return 'Easy';
  if (difficulty === 3) return 'Medium';
  return 'Hard';
}

function difficultyClass(difficulty: number) {
  if (difficulty <= 2) return 'border-green-700/20 bg-green-100 text-green-900';
  if (difficulty === 3) return 'border-amber-700/20 bg-amber-100 text-amber-900';
  return 'border-red-700/20 bg-red-100 text-red-900';
}

export function ProblemHeader({ problem }: { problem: Problem }) {
  return (
    <header className="border-b pb-5">
      <div className="flex flex-wrap items-center gap-2 text-xs">
        <span
          className={cn(
            'inline-flex h-6 items-center rounded-md border px-2 font-medium',
            difficultyClass(problem.difficulty),
          )}
        >
          {difficultyLabel(problem.difficulty)}
        </span>
        <span className="bg-muted text-muted-foreground inline-flex h-6 items-center rounded-md px-2 font-mono font-medium uppercase">
          {problem.language}
        </span>
        {problem.framework && (
          <span className="bg-muted text-muted-foreground inline-flex h-6 items-center rounded-md px-2 font-mono font-medium uppercase">
            {problem.framework}
          </span>
        )}
        <span className="text-muted-foreground ml-auto inline-flex items-center gap-1.5">
          <Clock3 className="size-3.5" />
          {problem.estimated_minutes} min
        </span>
      </div>
      <h1 className="mt-4 text-2xl leading-8 font-semibold">{problem.title}</h1>
      <div className="mt-3 flex flex-wrap gap-1.5">
        {problem.tags.map((tag) => (
          <span
            key={tag}
            className="text-muted-foreground rounded-md border px-2 py-0.5 text-xs"
          >
            {tag}
          </span>
        ))}
      </div>
    </header>
  );
}

export function ProblemStatement({
  description,
  hints = [],
  initiallyRevealedHints = 0,
}: {
  description: string;
  hints?: Hint[];
  initiallyRevealedHints?: number;
}) {
  const [revealedHints, setRevealedHints] = useState(initiallyRevealedHints);

  return (
    <div className="py-5">
      <div className="prose-geist text-sm leading-6">
        <ReactMarkdown>{description}</ReactMarkdown>
      </div>

      {hints.length > 0 && (
        <section className="mt-7 border-t pt-5" aria-labelledby="problem-hints-title">
          <div className="mb-3 flex items-center gap-2">
            <Lightbulb className="text-muted-foreground size-4" />
            <h2 id="problem-hints-title" className="text-sm font-semibold">
              Hints
            </h2>
          </div>
          <div className="space-y-2">
            {hints.map((hint, index) => {
              const isRevealed = index < revealedHints;
              return (
                <div key={`${index}-${hint.text.slice(0, 20)}`}>
                  {isRevealed ? (
                    <div className="bg-muted/40 rounded-md border px-4 py-3 text-sm leading-6">
                      <span className="text-muted-foreground mr-2 font-mono text-xs">
                        {String(index + 1).padStart(2, '0')}
                      </span>
                      {hint.text}
                    </div>
                  ) : (
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      className="text-muted-foreground -ml-2"
                      onClick={() => setRevealedHints(index + 1)}
                    >
                      <ChevronRight />
                      Reveal hint {index + 1}
                      {hint.cost > 0 && ` (${hint.cost} credit)`}
                    </Button>
                  )}
                </div>
              );
            })}
          </div>
        </section>
      )}
    </div>
  );
}

export function ProblemFileToolbar({
  files,
  activeFile,
  onActiveFileChange,
  onRun,
  running = false,
}: {
  files: SubmissionFile[];
  activeFile: number;
  onActiveFileChange: (index: number) => void;
  onRun: () => void;
  running?: boolean;
}) {
  return (
    <div className="flex min-h-11 items-center border-b border-[#333] bg-[#252526]">
      <div className="min-w-0 flex-1 overflow-x-auto">
        <div className="flex min-w-max">
          {files.map((file, index) => (
            <button
              key={file.path}
              type="button"
              onClick={() => onActiveFileChange(index)}
              className={cn(
                'flex h-11 items-center gap-2 border-r border-[#333] px-4 text-xs transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-white/70',
                index === activeFile
                  ? 'bg-[#1e1e1e] text-[#ddd]'
                  : 'text-[#858585] hover:bg-[#2a2d2e] hover:text-[#ccc]',
              )}
              aria-pressed={index === activeFile}
            >
              <FileCode2 className="size-3.5" />
              {file.path}
            </button>
          ))}
        </div>
      </div>
      <Button
        type="button"
        size="sm"
        variant="secondary"
        className="mx-2 bg-white text-black hover:bg-[#e6e6e6]"
        onClick={onRun}
        disabled={running || files.length === 0}
      >
        {running ? <LoaderCircle className="animate-spin" /> : <Play className="fill-current" />}
        {running ? 'Running' : 'Run'}
      </Button>
    </div>
  );
}

function TestCaseRow({ testCase, index }: { testCase: TestCaseResult; index: number }) {
  const passed = testCase.status === 'pass';

  return (
    <div className="flex items-start gap-3 border-b border-[#2a2a2a] px-4 py-2.5 last:border-b-0">
      <span
        className={cn(
          'mt-0.5 flex size-4 shrink-0 items-center justify-center rounded-full',
          passed ? 'bg-[#244a3f] text-[#73daca]' : 'bg-[#542d32] text-[#ff7b72]',
        )}
      >
        {passed ? <Check className="size-3" strokeWidth={3} /> : <CircleX className="size-3" />}
      </span>
      <div className="min-w-0 flex-1">
        <div className="flex items-center justify-between gap-3">
          <span className="text-xs text-[#d4d4d4]">{testCase.name || `Case ${index + 1}`}</span>
          <span className="shrink-0 font-mono text-[10px] text-[#777]">
            {testCase.duration_ms}ms
          </span>
        </div>
        {testCase.error && (
          <pre className="mt-1.5 overflow-x-auto whitespace-pre-wrap font-mono text-[11px] leading-5 text-[#ff7b72]">
            {testCase.error}
          </pre>
        )}
      </div>
    </div>
  );
}

export function ProblemTestResults({
  state,
  result,
  error,
  onRetry,
}: {
  state: 'idle' | 'running' | 'complete' | 'error';
  result?: TestResult | null;
  error?: string | null;
  onRetry?: () => void;
}) {
  if (state === 'idle') {
    return (
      <div className="flex h-full min-h-28 items-center justify-center bg-[#1e1e1e] px-5 text-center text-xs text-[#777]">
        Run your solution to see test results.
      </div>
    );
  }

  if (state === 'running') {
    return (
      <div className="flex h-full min-h-28 items-center justify-center gap-2 bg-[#1e1e1e] text-xs text-[#aaa]">
        <LoaderCircle className="size-4 animate-spin" />
        Running tests
      </div>
    );
  }

  if (state === 'error' || !result) {
    return (
      <div className="flex h-full min-h-28 flex-col items-start justify-center bg-[#1e1e1e] px-4 py-3">
        <div className="flex items-center gap-2 text-xs font-medium text-[#ff7b72]">
          <AlertTriangle className="size-4" />
          Could not run tests
        </div>
        <p className="mt-1.5 max-w-xl text-xs leading-5 text-[#aaa]">
          {error || 'The execution service did not return a result.'}
        </p>
        {onRetry && (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="mt-2 -ml-2 text-[#ddd] hover:bg-[#2a2d2e] hover:text-white"
            onClick={onRetry}
          >
            <RotateCcw />
            Retry
          </Button>
        )}
      </div>
    );
  }

  const passed = result.status === 'pass';
  return (
    <div className="h-full overflow-y-auto bg-[#1e1e1e]">
      <div className="flex min-h-11 items-center gap-3 border-b border-[#333] px-4 py-2">
        <span className={cn('text-xs font-semibold', passed ? 'text-[#73daca]' : 'text-[#ff7b72]')}>
          {passed ? 'Accepted' : 'Tests failed'}
        </span>
        <span className="text-[11px] text-[#858585]">
          {result.passed}/{result.total} passed
        </span>
        <span className="ml-auto font-mono text-[10px] text-[#777]">{result.duration_ms}ms</span>
      </div>
      {result.compile_error ? (
        <pre className="overflow-x-auto whitespace-pre-wrap p-4 font-mono text-[11px] leading-5 text-[#ff7b72]">
          {result.compile_error}
        </pre>
      ) : (
        result.test_cases.map((testCase, index) => (
          <TestCaseRow key={`${testCase.name}-${index}`} testCase={testCase} index={index} />
        ))
      )}
    </div>
  );
}
