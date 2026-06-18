import { motion } from 'motion/react';

const CARD_SHADOW = 'var(--cg-card-shadow)';

const stats = [
  { value: 0, label: 'Solved', tint: 'var(--color-moss-tint)', accent: 'var(--color-moss)' },
  { value: 0, label: 'Streak', tint: 'var(--color-blue-tint)', accent: 'var(--color-blue)' },
  { value: 0, label: 'Submissions', tint: 'var(--color-cobalt-tint)', accent: 'var(--color-cobalt)' },
];

export function DashboardPage() {
  return (
    <div className="max-w-4xl mx-auto px-6 py-12">
      <h1 className="font-display text-4xl font-semibold tracking-tight text-ink mb-10">
        Dashboard<span className="text-blue">.</span>
      </h1>

      <div className="grid grid-cols-3 gap-5">
        {stats.map(({ value, label, tint, accent }, i) => (
          <motion.div
            key={label}
            initial={{ opacity: 0, y: 16 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ type: 'spring', stiffness: 300, damping: 26, delay: i * 0.07 }}
            className="rounded-2xl border border-chalk px-6 py-5"
            style={{ backgroundColor: tint, boxShadow: CARD_SHADOW }}
          >
            <div className="font-display text-5xl font-semibold text-ink">{value}</div>
            <div
              className="mt-3 inline-block rounded-full px-2.5 py-0.5 text-[10px] font-bold tracking-[0.15em] uppercase text-white"
              style={{ backgroundColor: accent }}
            >
              {label}
            </div>
          </motion.div>
        ))}
      </div>

      <div className="mt-10 rounded-2xl border-2 border-dashed border-chalk p-8 text-center text-xs text-graphite leading-6">
        Your training history lands here once auth and submission tracking ship.
        <br />
        Until then — <span className="font-bold text-blue">go lift something heavy</span> in Problems.
      </div>
    </div>
  );
}
