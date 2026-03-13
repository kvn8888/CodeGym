import { useEffect, useState, useCallback } from 'react';
import { useParams } from 'react-router-dom';
import ReactMarkdown from 'react-markdown';
import Editor from '@monaco-editor/react';
import { api } from '../../shared/api/client';
import type { Problem, SubmissionFile, TestResult, TestCaseResult } from '../../shared/api/types';

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
      // Poll for result (TODO: replace with WebSocket)
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
    return <div className="text-slate-400 text-center py-12">Loading...</div>;
  }

  const monacoLang = languageMap[problem.language] ?? 'plaintext';

  return (
    <div className="h-[calc(100vh-3.5rem)] flex">
      {/* Left: Problem Description */}
      <div className="w-[45%] border-r border-slate-700 overflow-y-auto p-6">
        <h1 className="text-xl font-bold mb-1">{problem.title}</h1>
        <div className="flex gap-2 mb-4 text-sm">
          <span className="bg-slate-700 px-2 py-0.5 rounded">{problem.language}</span>
          {problem.framework && (
            <span className="bg-slate-700 px-2 py-0.5 rounded">{problem.framework}</span>
          )}
          <span className="bg-slate-700 px-2 py-0.5 rounded">
            {problem.estimated_minutes}m
          </span>
        </div>
        <div className="prose prose-invert prose-sm max-w-none">
          <ReactMarkdown>{problem.description}</ReactMarkdown>
        </div>

        {/* Hints */}
        {problem.hints && problem.hints.length > 0 && (
          <div className="mt-6 border-t border-slate-700 pt-4">
            <h3 className="text-sm font-medium text-slate-400 mb-2">Hints</h3>
            {problem.hints.map((hint, i) => (
              <div key={i} className="mb-2">
                {i < hintsRevealed ? (
                  <p className="text-sm text-slate-300 bg-slate-800 p-2 rounded">{hint.text}</p>
                ) : (
                  <button
                    onClick={() => setHintsRevealed(i + 1)}
                    className="text-sm text-blue-400 hover:text-blue-300"
                  >
                    Reveal hint {i + 1} {hint.cost > 0 ? `(${hint.cost} credit)` : '(free)'}
                  </button>
                )}
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Right: Editor + Results */}
      <div className="w-[55%] flex flex-col">
        {/* File tabs */}
        <div className="flex items-center border-b border-slate-700 bg-slate-900">
          {files.map((file, i) => (
            <button
              key={file.path}
              onClick={() => setActiveFile(i)}
              className={`px-4 py-2 text-sm border-r border-slate-700 ${
                i === activeFile
                  ? 'bg-slate-800 text-white'
                  : 'text-slate-400 hover:text-white hover:bg-slate-800/50'
              }`}
            >
              {file.path}
            </button>
          ))}
          <div className="flex-1" />
          <button
            onClick={handleSubmit}
            disabled={submitting}
            className="px-4 py-1.5 m-1 bg-green-600 hover:bg-green-700 disabled:bg-slate-700 text-white text-sm font-medium rounded transition-colors"
          >
            {submitting ? 'Running...' : 'Run Tests'}
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
                fontSize: 14,
                minimap: { enabled: false },
                scrollBeyondLastLine: false,
                padding: { top: 12 },
                lineNumbers: 'on',
                readOnly: false,
              }}
            />
          )}
        </div>

        {/* Results Panel */}
        {result && (
          <div className="border-t border-slate-700 bg-slate-900 max-h-[40%] overflow-y-auto">
            <div className="p-3 border-b border-slate-700 flex items-center gap-3">
              <span
                className={`font-bold text-sm ${
                  result.status === 'pass' ? 'text-green-400' : 'text-red-400'
                }`}
              >
                {result.status === 'pass' ? 'All Tests Passed' : 'Tests Failed'}
              </span>
              <span className="text-xs text-slate-400">
                {result.passed}/{result.total} passed &middot; {result.duration_ms}ms
              </span>
            </div>
            <div className="divide-y divide-slate-800">
              {result.test_cases.map((tc: TestCaseResult, i: number) => (
                <div key={i} className="px-3 py-2 flex items-start gap-2">
                  <span className={tc.status === 'pass' ? 'text-green-400' : 'text-red-400'}>
                    {tc.status === 'pass' ? '✓' : '✗'}
                  </span>
                  <div className="flex-1 min-w-0">
                    <span className="text-sm text-slate-300">{tc.name}</span>
                    {tc.error && (
                      <pre className="text-xs text-red-400 mt-1 whitespace-pre-wrap">
                        {tc.error}
                      </pre>
                    )}
                  </div>
                  <span className="text-xs text-slate-500">{tc.duration_ms}ms</span>
                </div>
              ))}
            </div>
            {result.compile_error && (
              <pre className="p-3 text-xs text-red-400 whitespace-pre-wrap bg-red-950/30">
                {result.compile_error}
              </pre>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
