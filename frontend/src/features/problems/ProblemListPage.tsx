import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { api } from '../../shared/api/client';
import type { ProblemSummary } from '../../shared/api/types';
import { GridSpinner } from '../../shared/components/GridSpinner';

const difficultyLabels: Record<number, string> = {
  1: 'EASY',
  2: 'EASY',
  3: 'MED',
  4: 'HARD',
  5: 'EXPERT',
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
    <div className="max-w-5xl mx-auto px-6 py-8">
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-xs font-bold tracking-[0.2em] text-ink uppercase">Problems</h1>
        <select
          value={languageFilter}
          onChange={(e) => setLanguageFilter(e.target.value)}
          className="bg-transparent border border-ink rounded-lg px-3 py-1.5 text-xs text-ink cursor-pointer focus:outline-none"
        >
          <option value="">ALL</option>
          {languages.map((l) => (
            <option key={l} value={l}>
              {l.toUpperCase()}
            </option>
          ))}
        </select>
      </div>

      {loading ? (
        <div className="flex justify-center py-16">
          <GridSpinner size="md" />
        </div>
      ) : problems.length === 0 ? (
        <div className="text-xs text-ash text-center py-16 tracking-[0.1em]">
          NO PROBLEMS FOUND
        </div>
      ) : (
        <div className="border-t border-ink">
          {problems.map((problem) => (
            <Link
              key={problem.id}
              to={`/problems/${problem.id}`}
              className="flex items-center justify-between border-b border-chalk px-2 py-3 no-underline hover:bg-parchment transition-colors group"
            >
              <div className="flex items-center gap-4">
                <span className="text-sm text-ink">{problem.title}</span>
                <span className="text-[10px] tracking-[0.1em] text-graphite">
                  {difficultyLabels[problem.difficulty] ?? '\u2014'}
                </span>
              </div>
              <div className="flex items-center gap-3 text-[10px] tracking-[0.1em] text-ash">
                <span>{problem.language.toUpperCase()}</span>
                {problem.framework && <span>{problem.framework.toUpperCase()}</span>}
                <span>{problem.estimated_minutes}M</span>
              </div>
            </Link>
          ))}
        </div>
      )}
    </div>
  );
}
