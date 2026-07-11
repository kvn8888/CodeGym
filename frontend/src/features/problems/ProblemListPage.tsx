import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { motion } from 'motion/react';

import { api } from '../../shared/api/client';
import type { ProblemSummary } from '../../shared/api/types';
import { Badge } from '@/components/ui/badge';
import { Skeleton } from '@/components/ui/skeleton';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { cn } from '@/lib/utils';

const difficultyChips: Record<number, { label: string; className: string }> = {
  1: { label: 'Easy', className: 'bg-green-100 text-green-900' },
  2: { label: 'Easy', className: 'bg-green-100 text-green-900' },
  3: { label: 'Medium', className: 'bg-amber-100 text-amber-900' },
  4: { label: 'Hard', className: 'bg-red-100 text-red-900' },
  5: { label: 'Expert', className: 'bg-purple-100 text-purple-700' },
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
    <div className="mx-auto w-full max-w-[1180px] px-4 py-6 sm:px-6 lg:px-8 lg:py-8">
      <div className="mb-6 flex items-end justify-between border-b pb-5">
        <div>
          <h1 className="text-2xl leading-8 font-semibold">Problems</h1>
          {!loading && (
            <p className="mt-2 text-sm text-muted-foreground">
              <span className="font-semibold text-foreground">{filteredProblems.length}</span> available
              {languageFilter ? ` — ${languageFilter}` : ''}
            </p>
          )}
        </div>
        <Select
          value={languageFilter || 'all'}
          onValueChange={(value) => setLanguageFilter(value === 'all' ? '' : value)}
        >
          <SelectTrigger className="w-[180px]">
            <SelectValue placeholder="All Languages" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All Languages</SelectItem>
            {languages.map((l) => (
              <SelectItem key={l} value={l}>
                {l.charAt(0).toUpperCase() + l.slice(1)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {loading ? (
        <div className="flex flex-col gap-2">
          {Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className="h-13 w-full rounded-lg" />
          ))}
        </div>
      ) : filteredProblems.length === 0 ? (
        <div className="text-muted-foreground py-16 text-center text-sm">No problems found.</div>
      ) : (
        <motion.div className="overflow-hidden rounded-lg border" variants={listVariants} initial="hidden" animate="visible">
          {filteredProblems.map((problem) => {
            const chip = difficultyChips[problem.difficulty];
            return (
              <motion.div key={problem.id} variants={itemVariants} className="border-b last:border-b-0">
                <Link
                  to={`/problems/${problem.id}`}
                  className="group bg-card hover:bg-muted/35 focus-visible:ring-ring/50 flex min-h-13 items-center justify-between gap-4 px-3 py-2.5 no-underline transition-colors outline-none focus-visible:ring-[3px]"
                >
                  <div className="flex min-w-0 items-center gap-3">
                    <span className="group-hover:text-primary truncate text-sm font-semibold transition-colors">
                      {problem.title}
                    </span>
                    {chip && (
                      <Badge variant="secondary" className={cn('border-transparent', chip.className)}>
                        {chip.label}
                      </Badge>
                    )}
                  </div>
                  <div className="text-muted-foreground flex shrink-0 items-center gap-2 font-mono text-xs">
                    <span className="bg-muted rounded-md px-2 py-1">{problem.language.toUpperCase()}</span>
                    {problem.framework && (
                      <span className="bg-muted rounded-md px-2 py-1">{problem.framework.toUpperCase()}</span>
                    )}
                    <span>{problem.estimated_minutes} min</span>
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
