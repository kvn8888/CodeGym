import { useEffect, useState, useCallback } from 'react';
import { useParams } from 'react-router-dom';
import ReactMarkdown from 'react-markdown';
import Editor from '@monaco-editor/react';
import { api } from '../../shared/api/client';
import type { Problem, SubmissionFile, TestResult, TestCaseResult } from '../../shared/api/types';
import { GridSpinner } from '../../shared/components/GridSpinner';

const languageMap: Record<string, string> = {
  go: 'go',
  javascript: 'javascript',
  typescript: 'typescript',
  python: 'python',
  java: 'java',
  cpp: 'cpp',
  c: 'c',
  rust: 'rust',
  swift: 'swift',
};

export function ProblemDetailPage() {
  const { id } = useParams<{ id: string }>();
  const [problem, setProblem] = useState<Problem | null>(null);
  const [files, setFiles] = useState<SubmissionFile[]>([]);
  const [activeFile, setActiveFile] = useState(0);
  const [submitting, setSubmitting] = useState(false);
  const [result, setResult] = useState<TestResult | null>(null);
  const [hintsRevealed, setHintsRevealed] = useState(0);

  useEffect(() => {
    if (!id) return;
    Promise.all([
      api.get<Problem>(`/problems/${id}`),
      api.get<{ files: SubmissionFile[] }>(`/problems/${id}/skeleton`),
    ]).then(([prob, skel]) => {
      setProblem(prob);
      setFiles(skel.files);
    });
  }, [id]);

  const handleCodeChange = useCallback(
    (value: string | undefined) => {
      if (value === undefined) return;
      setFiles((prev) => prev.map((f, i) => (i === activeFile ? { ...f, content: value } : f)));
    },
    [activeFile],
  );

  const handleSubmit = async () => {
    if (!problem) return;
    setSubmitting(true);
    setResult(null);
    try {
      const res = await api.post<{ submission_id: string }>('/submissions', {
        problem_id: problem.id,
        files,
      });
      const poll = async () => {
        const sub = await api.get<{ status: string; result?: TestResult }>(
          `/submissions/${res.submission_id}`,
        );
        if (sub.status === 'pending' || sub.status === 'running') {
          setTimeout(poll, 1000);
        } else if (sub.result) {
          setResult(sub.result);
          setSubmitting(false);
        }
      };
      poll();
    } catch (err) {
      console.error(err);
      setSubmitting(false);
    }
  };

  if (!problem) {
    return (
      <div className="flex justify-center py-16">
        <GridSpinner size="md" />
      </div>
    );
  }

  const monacoLang = languageMap[problem.language] ?? 'plaintext';

  return (
    <div className="h-[calc(100vh-3rem)] flex">
      {/* Left: Problem Description */}
      <div className="w-[45%] border-r border-ink overflow-y-auto p-6 bg-bone">
        <h1 className="text-sm font-bold text-ink mb-1">{problem.title}</h1>
        <div className="flex gap-3 mb-4 text-[10px] tracking-[0.1em] text-ash">
          <span>{problem.language.toUpperCase()}</span>
          {problem.framework && <span>{problem.framework.toUpperCase()}</span>}
          <span>{problem.estimated_minutes}M</span>
        </div>
        <div className="prose-brutalist text-xs text-graphite">
          <ReactMarkdown>{problem.description}</ReactMarkdown>
        </div>

        {/* Hints */}
        {problem.hints && problem.hints.length > 0 && (
          <div className="mt-6 border-t border-chalk pt-4">
            <h3 className="text-[10px] font-bold tracking-[0.2em] text-ash mb-3 uppercase">
              Hints
            </h3>
            {problem.hints.map((hint, i) => (
              <div key={i} className="mb-2">
                {i < hintsRevealed ? (
                  <p className="text-xs text-graphite border border-chalk px-3 py-2">{hint.text}</p>
                ) : (
                  <button
                    onClick={() => setHintsRevealed(i + 1)}
                    className="text-xs text-ash hover:text-ink transition-colors"
                  >
                    {'\u2192'} Reveal hint {i + 1}{' '}
                    {hint.cost > 0 ? `(${hint.cost} credit)` : ''}
                  </button>
                )}
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Right: Editor + Results */}
      <div className="w-[55%] flex flex-col bg-[#1e1e1e]">
        {/* File tabs */}
        <div className="flex items-center border-b border-[#333] bg-[#252526]">
          {files.map((file, i) => (
            <button
              key={file.path}
              onClick={() => setActiveFile(i)}
              className={`px-4 py-2 text-xs border-r border-[#333] transition-colors ${
                i === activeFile
                  ? 'bg-[#1e1e1e] text-[#ccc]'
                  : 'text-[#666] hover:text-[#ccc]'
              }`}
            >
              {file.path}
            </button>
          ))}
          <div className="flex-1" />
          <button
            onClick={handleSubmit}
            disabled={submitting}
            className="px-4 py-1 m-1.5 bg-bone text-ink text-[10px] font-bold tracking-[0.15em] uppercase hover:bg-parchment disabled:opacity-40 transition-colors"
          >
            {submitting ? 'RUNNING' : 'RUN'}
          </button>
        </div>

        {/* Editor */}
        <div className="flex-1">
          {files.length > 0 && (
            <Editor
              height="100%"
              language={monacoLang}
              theme="vs-dark"
              value={files[activeFile]?.content ?? ''}
              onChange={handleCodeChange}
              options={{
                fontSize: 13,
                fontFamily:
                  "'SF Mono', 'Cascadia Code', 'JetBrains Mono', 'Fira Code', ui-monospace, monospace",
                minimap: { enabled: false },
                scrollBeyondLastLine: false,
                padding: { top: 12 },
                lineNumbers: 'on',
                readOnly: false,
              }}
            />
          )}
        </div>

        {/* Running indicator */}
        {submitting && (
          <div className="border-t border-[#333] bg-[#252526] p-6 flex justify-center">
            <GridSpinner size="sm" />
          </div>
        )}

        {/* Results Panel */}
        {result && (
          <div className="border-t border-[#333] bg-[#1e1e1e] max-h-[40%] overflow-y-auto">
            <div className="px-4 py-2 border-b border-[#333] flex items-center gap-3">
              <span
                className={`text-xs font-bold ${
                  result.status === 'pass' ? 'text-[#4ec9b0]' : 'text-[#f14c4c]'
                }`}
              >
                {result.status === 'pass' ? 'PASS' : 'FAIL'}
              </span>
              <span className="text-[10px] text-[#666]">
                {result.passed}/{result.total} {'\u2014'} {result.duration_ms}ms
              </span>
            </div>
            <div>
              {result.test_cases.map((tc: TestCaseResult, i: number) => (
                <div
                  key={i}
                  className="px-4 py-1.5 flex items-start gap-2 border-b border-[#252526]"
                >
                  <span
                    className={`text-xs ${tc.status === 'pass' ? 'text-[#4ec9b0]' : 'text-[#f14c4c]'}`}
                  >
                    {tc.status === 'pass' ? '\u2713' : '\u2717'}
                  </span>
                  <div className="flex-1 min-w-0">
                    <span className="text-xs text-[#ccc]">{tc.name}</span>
                    {tc.error && (
                      <pre className="text-[10px] text-[#f14c4c] mt-1 whitespace-pre-wrap">
                        {tc.error}
                      </pre>
                    )}
                  </div>
                  <span className="text-[10px] text-[#555]">{tc.duration_ms}ms</span>
                </div>
              ))}
            </div>
            {result.compile_error && (
              <pre className="p-3 text-[10px] text-[#f14c4c] whitespace-pre-wrap">
                {result.compile_error}
              </pre>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
