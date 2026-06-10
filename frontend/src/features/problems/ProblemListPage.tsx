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
  const filteredProblems = languageFilter
    ? problems.filter((problem) => problem.language === languageFilter)
    : problems;

  useEffect(() => {
    api
      .get<{ problems: ProblemSummary[]; total: number }>('/problems')
      .then((data) => setProblems(data.problems ?? []))
      .catch(console.error)
      .finally(() => setLoading(false));
  }, []);

  const languages = [...new Set(problems.map((p) => p.language))];

  return (
    <div className="max-w-3xl mx-auto px-6 py-10">
      <div className="flex items-center justify-between mb-8">
        <h1 className="text-2xl font-bold text-ink tracking-tight">Problems</h1>
        <select
          value={languageFilter}
          onChange={(e) => setLanguageFilter(e.target.value)}
          className="rounded-xl border border-chalk bg-white px-3 py-1.5 text-xs text-ink cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ink focus-visible:ring-offset-2 focus-visible:ring-offset-bone"
          style={{ boxShadow: '0 1px 3px rgba(0,0,0,0.04), 0 4px 12px rgba(0,0,0,0.03)' }}
        >
          <option value="">All Languages</option>
          {languages.map((l) => (
            <option key={l} value={l}>
              {l.charAt(0).toUpperCase() + l.slice(1)}
            </option>
          ))}
        </select>
      </div>

      {loading ? (
        <div className="flex justify-center py-16">
          <GridSpinner size="md" />
        </div>
      ) : filteredProblems.length === 0 ? (
        <div className="text-xs text-ash text-center py-16 tracking-[0.1em]">
          No problems found
        </div>
      ) : (
        <div className="flex flex-col gap-3">
          {filteredProblems.map((problem) => (
            <Link
              key={problem.id}
              to={`/problems/${problem.id}`}
              className="flex items-center justify-between gap-4 rounded-2xl border border-chalk bg-white px-5 py-4 no-underline hover:border-ash focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ink focus-visible:ring-offset-2 focus-visible:ring-offset-bone transition-colors"
              style={{ boxShadow: '0 1px 3px rgba(0,0,0,0.04), 0 4px 12px rgba(0,0,0,0.03)' }}
            >
              <div className="flex items-center gap-3">
                <span className="text-sm font-medium text-ink">{problem.title}</span>
                <span className="text-[10px] tracking-[0.1em] text-ash">
                  {difficultyLabels[problem.difficulty] ?? '\u2014'}
                </span>
              </div>
              <div className="flex shrink-0 items-center gap-2 text-[10px] tracking-[0.08em] text-ash">
                <span>{problem.language.toUpperCase()}</span>
                {problem.framework && <span>{problem.framework.toUpperCase()}</span>}
                <span className="text-chalk">/</span>
                <span>{problem.estimated_minutes}m</span>
              </div>
            </Link>
          ))}
        </div>
      )}
    </div>
  );
}
