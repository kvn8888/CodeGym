import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { motion } from 'motion/react';
import { api } from '../../shared/api/client';
import type { ProblemSummary } from '../../shared/api/types';
import { GridSpinner } from '../../shared/components/GridSpinner';

const CARD_SHADOW = 'var(--cg-card-shadow)';

const difficultyChips: Record<number, { label: string; color: string; tint: string }> = {
  1: { label: 'Easy', color: 'var(--color-green-900)', tint: 'var(--color-green-100)' },
  2: { label: 'Easy', color: 'var(--color-green-900)', tint: 'var(--color-green-100)' },
  3: { label: 'Medium', color: 'var(--color-amber-900)', tint: 'var(--color-amber-100)' },
  4: { label: 'Hard', color: 'var(--color-red-900)', tint: 'var(--color-red-100)' },
  5: { label: 'Expert', color: 'var(--color-purple-700)', tint: 'var(--color-purple-100)' },
};

const listVariants = {
  hidden: {},
  visible: { transition: { staggerChildren: 0.05 } },
};

const itemVariants = {
  hidden: { opacity: 0, y: 14 },
  visible: { opacity: 1, y: 0, transition: { type: 'spring' as const, stiffness: 320, damping: 28 } },
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
    <div className="mx-auto max-w-4xl px-6 py-12">
      <div className="flex items-end justify-between mb-10">
        <div>
          <h1 className="text-[40px] font-semibold leading-[48px] tracking-[-2.4px] text-gray-1000">
            Problems
          </h1>
          {!loading && (
            <p className="mt-2 text-sm text-gray-900">
              <span className="font-semibold text-gray-1000">{filteredProblems.length}</span> available
              {languageFilter ? ` — ${languageFilter}` : ''}
            </p>
          )}
        </div>
        <select
          value={languageFilter}
          onChange={(e) => setLanguageFilter(e.target.value)}
          className="cg-focus h-10 cursor-pointer rounded-md border border-gray-alpha-200 bg-background-100 px-3 text-sm text-gray-1000"
          style={{ boxShadow: CARD_SHADOW }}
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
        <div className="py-16 text-center text-sm text-gray-700">
          No problems found.
        </div>
      ) : (
        <motion.div
          className="flex flex-col gap-4"
          variants={listVariants}
          initial="hidden"
          animate="visible"
        >
          {filteredProblems.map((problem) => {
            const chip = difficultyChips[problem.difficulty];
            return (
              <motion.div key={problem.id} variants={itemVariants}>
                <Link
                  to={`/problems/${problem.id}`}
                  className="group cg-focus flex items-center justify-between gap-4 rounded-xl border border-gray-alpha-200 bg-background-100 px-5 py-4 no-underline transition-colors hover:border-gray-alpha-400"
                  style={{ boxShadow: CARD_SHADOW }}
                >
                  <div className="flex min-w-0 items-center gap-3">
                    <span className="truncate text-sm font-semibold text-gray-1000 transition-colors group-hover:text-blue-700">
                      {problem.title}
                    </span>
                    {chip && (
                      <span
                        className="shrink-0 rounded-full px-2.5 py-0.5 text-xs font-medium"
                        style={{ color: chip.color, backgroundColor: chip.tint }}
                      >
                        {chip.label}
                      </span>
                    )}
                  </div>
                  <div className="flex shrink-0 items-center gap-2 font-mono text-xs text-gray-900">
                    <span className="rounded-md bg-gray-100 px-2 py-1">{problem.language.toUpperCase()}</span>
                    {problem.framework && (
                      <span className="rounded-md bg-gray-100 px-2 py-1">{problem.framework.toUpperCase()}</span>
                    )}
                    <span className="text-gray-700">{problem.estimated_minutes} min</span>
                  </div>
                </Link>
              </motion.div>
            );
          })}
        </motion.div>
      )}
    </div>
  );
}
