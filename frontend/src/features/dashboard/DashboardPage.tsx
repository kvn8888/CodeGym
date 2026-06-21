import { motion } from 'motion/react';

const CARD_SHADOW = 'var(--cg-card-shadow)';

const stats = [
  { value: 0, label: 'Solved', helper: 'Completed problems' },
  { value: 0, label: 'Streak', helper: 'Active practice days' },
  { value: 0, label: 'Submissions', helper: 'Attempts recorded' },
];

export function DashboardPage() {
  return (
    <div className="mx-auto max-w-5xl px-6 py-12">
      <h1 className="mb-10 text-[40px] font-semibold leading-[48px] tracking-[-2.4px] text-gray-1000">
        Dashboard
      </h1>

      <div className="flex flex-col gap-4 md:flex-row">
        {stats.map(({ value, label, helper }, i) => (
          <motion.div
            key={label}
            initial={{ opacity: 0, y: 16 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ type: 'spring', stiffness: 300, damping: 26, delay: i * 0.07 }}
            className="min-w-0 flex-1 rounded-xl border border-gray-alpha-200 bg-background-100 px-6 py-5"
            style={{ boxShadow: CARD_SHADOW }}
          >
            <div className="text-[48px] font-semibold leading-[56px] tracking-[-2.88px] text-gray-1000">{value}</div>
            <div className="mt-3 text-sm font-medium text-gray-1000">{label}</div>
            <div className="mt-1 text-sm text-gray-700">{helper}</div>
          </motion.div>
        ))}
      </div>

      <div className="mt-10 rounded-xl border border-dashed border-gray-alpha-400 bg-background-100 p-8 text-center text-sm leading-6 text-gray-900">
        Your training history lands here once auth and submission tracking ship.
        <br />
        Until then, start a focused session from Problems.
      </div>
    </div>
  );
}
