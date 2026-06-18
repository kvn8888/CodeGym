import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { motion } from 'motion/react';
import { api } from '../../shared/api/client';
import type { ProblemSummary } from '../../shared/api/types';
import { GridSpinner } from '../../shared/components/GridSpinner';

const CARD_SHADOW = 'var(--cg-card-shadow)';

const difficultyChips: Record<number, { label: string; color: string; tint: string }> = {
  1: { label: 'EASY', color: 'var(--color-moss)', tint: 'var(--color-moss-tint)' },
  2: { label: 'EASY', color: 'var(--color-moss)', tint: 'var(--color-moss-tint)' },
  3: { label: 'MED', color: 'var(--color-amber)', tint: 'var(--color-amber-tint)' },
  4: { label: 'HARD', color: 'var(--color-rose)', tint: 'var(--color-rose-tint)' },
  5: { label: 'EXPERT', color: 'var(--color-violet)', tint: 'var(--color-violet-tint)' },
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
    <div className="max-w-3xl mx-auto px-6 py-12">
      <div className="flex items-end justify-between mb-10">
        <div>
          <h1 className="font-display text-4xl font-semibold tracking-tight text-ink">
            Problems<span className="text-blue">.</span>
          </h1>
          {!loading && (
            <p className="mt-2 text-xs text-graphite">
              <span className="font-bold text-ink">{filteredProblems.length}</span> on the rack
              {languageFilter ? ` — ${languageFilter}` : ''}
            </p>
          )}
        </div>
        <select
          value={languageFilter}
          onChange={(e) => setLanguageFilter(e.target.value)}
          className="rounded-xl border border-chalk bg-white px-3 py-2 text-xs text-ink cursor-pointer cg-focus"
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
        <div className="text-xs text-ash text-center py-16 tracking-[0.1em]">
          No problems found
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
                  className="group flex items-center justify-between gap-4 rounded-2xl border border-chalk bg-white px-5 py-4 no-underline cg-focus hover:border-ash transition-colors"
                  style={{ boxShadow: CARD_SHADOW }}
                >
                  <div className="flex min-w-0 items-center gap-3">
                    <span className="truncate text-sm font-bold text-ink group-hover:text-blue transition-colors">
                      {problem.title}
                    </span>
                    {chip && (
                      <span
                        className="shrink-0 rounded-full px-2.5 py-0.5 text-[10px] font-bold tracking-[0.1em]"
                        style={{ color: chip.color, backgroundColor: chip.tint, border: `1px solid ${chip.color}` }}
                      >
                        {chip.label}
                      </span>
                    )}
                  </div>
                  <div className="flex shrink-0 items-center gap-2 text-[10px] tracking-[0.08em] text-graphite">
                    <span className="rounded-md bg-grain px-2 py-1 font-bold">{problem.language.toUpperCase()}</span>
                    {problem.framework && (
                      <span className="rounded-md bg-grain px-2 py-1 font-bold">{problem.framework.toUpperCase()}</span>
                    )}
                    <span className="text-ash">{problem.estimated_minutes} min</span>
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
