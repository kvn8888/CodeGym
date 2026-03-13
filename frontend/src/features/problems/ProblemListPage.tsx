import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { api } from '../../shared/api/client';
import type { ProblemSummary } from '../../shared/api/types';

const difficultyColors: Record<number, string> = {
  1: 'bg-green-900 text-green-300',
  2: 'bg-green-900 text-green-300',
  3: 'bg-yellow-900 text-yellow-300',
  4: 'bg-red-900 text-red-300',
  5: 'bg-red-900 text-red-300',
};

const difficultyLabels: Record<number, string> = {
  1: 'Easy',
  2: 'Easy',
  3: 'Medium',
  4: 'Hard',
  5: 'Expert',
};

export function ProblemListPage() {
  const [problems, setProblems] = useState<ProblemSummary[]>([]);
  const [loading, setLoading] = useState(true);
  const [languageFilter, setLanguageFilter] = useState('');

  useEffect(() => {
    const params = languageFilter ? `?language=${languageFilter}` : '';
    api
      .get<{ problems: ProblemSummary[]; total: number }>(`/problems${params}`)
      .then((data) => setProblems(data.problems ?? []))
      .catch(console.error)
      .finally(() => setLoading(false));
  }, [languageFilter]);

  const languages = [...new Set(problems.map((p) => p.language))];

  return (
    <div className="max-w-5xl mx-auto px-4 py-8">
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-bold">Problems</h1>
        <div className="flex gap-2">
          <select
            value={languageFilter}
            onChange={(e) => setLanguageFilter(e.target.value)}
            className="bg-slate-800 border border-slate-700 rounded-md px-3 py-1.5 text-sm text-slate-300"
          >
            <option value="">All Languages</option>
            {languages.map((l) => (
              <option key={l} value={l}>
                {l}
              </option>
            ))}
          </select>
        </div>
      </div>

      {loading ? (
        <div className="text-slate-400 text-center py-12">Loading problems...</div>
      ) : problems.length === 0 ? (
        <div className="text-slate-400 text-center py-12">
          No problems found. Try generating one!
        </div>
      ) : (
        <div className="space-y-2">
          {problems.map((problem) => (
            <Link
              key={problem.id}
              to={`/problems/${problem.id}`}
              className="block bg-slate-800/50 border border-slate-700 rounded-lg p-4 hover:border-blue-500 transition-colors no-underline"
            >
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <h3 className="text-lg font-medium text-slate-100">{problem.title}</h3>
                  <span
                    className={`px-2 py-0.5 rounded text-xs font-medium ${difficultyColors[problem.difficulty] ?? 'bg-slate-700 text-slate-300'}`}
                  >
                    {difficultyLabels[problem.difficulty] ?? 'Unknown'}
                  </span>
                </div>
                <div className="flex items-center gap-2 text-sm text-slate-400">
                  <span className="bg-slate-700 px-2 py-0.5 rounded">{problem.language}</span>
                  {problem.framework && (
                    <span className="bg-slate-700 px-2 py-0.5 rounded">{problem.framework}</span>
                  )}
                  <span>{problem.estimated_minutes}m</span>
                </div>
              </div>
              <div className="flex gap-1.5 mt-2">
                {problem.tags.map((tag) => (
                  <span
                    key={tag}
                    className="text-xs text-slate-500 bg-slate-800 px-1.5 py-0.5 rounded"
                  >
                    {tag}
                  </span>
                ))}
              </div>
            </Link>
          ))}
        </div>
      )}
    </div>
  );
}
