export function DashboardPage() {
  return (
    <div className="max-w-4xl mx-auto px-6 py-8">
      <h1 className="text-xs font-bold tracking-[0.2em] text-ink mb-8 uppercase">Dashboard</h1>

      <div className="grid grid-cols-3 gap-4">
        {[
          { value: 0, label: 'Solved' },
          { value: 0, label: 'Streak' },
          { value: 0, label: 'Submissions' },
        ].map(({ value, label }) => (
          <div
            key={label}
            className="rounded-2xl border border-chalk bg-white px-6 py-5"
            style={{ boxShadow: '0 1px 3px rgba(0,0,0,0.04), 0 4px 12px rgba(0,0,0,0.03)' }}
          >
            <div className="text-3xl font-bold text-ink">{value}</div>
            <div className="text-[10px] tracking-[0.15em] text-ash mt-2 uppercase">{label}</div>
          </div>
        ))}
      </div>

      <div className="mt-8 p-6 text-xs text-ash text-center">
        Dashboard features available after auth and submission tracking are implemented.
      </div>
    </div>
  );
}
