export function DashboardPage() {
  return (
    <div className="max-w-4xl mx-auto px-6 py-8">
      <h1 className="text-xs font-bold tracking-[0.2em] text-ink mb-8 uppercase">Dashboard</h1>

      <div className="grid grid-cols-3 gap-6">
        <div className="p-6">
          <div className="text-2xl font-bold text-ink">0</div>
          <div className="text-[10px] tracking-[0.15em] text-ash mt-1 uppercase">Solved</div>
        </div>
        <div className="p-6">
          <div className="text-2xl font-bold text-ink">0</div>
          <div className="text-[10px] tracking-[0.15em] text-ash mt-1 uppercase">Streak</div>
        </div>
        <div className="p-6">
          <div className="text-2xl font-bold text-ink">0</div>
          <div className="text-[10px] tracking-[0.15em] text-ash mt-1 uppercase">Submissions</div>
        </div>
      </div>

      <div className="mt-8 p-6 text-xs text-ash text-center">
        Dashboard features available after auth and submission tracking are implemented.
      </div>
    </div>
  );
}
