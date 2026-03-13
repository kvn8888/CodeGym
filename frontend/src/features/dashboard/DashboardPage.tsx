export function DashboardPage() {
  return (
    <div className="max-w-4xl mx-auto px-4 py-8">
      <h1 className="text-2xl font-bold mb-6">Dashboard</h1>

      <div className="grid grid-cols-3 gap-4 mb-8">
        <div className="bg-slate-800 border border-slate-700 rounded-lg p-4">
          <div className="text-3xl font-bold text-blue-400">0</div>
          <div className="text-sm text-slate-400 mt-1">Problems Solved</div>
        </div>
        <div className="bg-slate-800 border border-slate-700 rounded-lg p-4">
          <div className="text-3xl font-bold text-green-400">0</div>
          <div className="text-sm text-slate-400 mt-1">Day Streak</div>
        </div>
        <div className="bg-slate-800 border border-slate-700 rounded-lg p-4">
          <div className="text-3xl font-bold text-yellow-400">0</div>
          <div className="text-sm text-slate-400 mt-1">Submissions</div>
        </div>
      </div>

      <div className="bg-slate-800 border border-slate-700 rounded-lg p-6 text-center text-slate-400">
        Dashboard features will be available once auth and submission tracking are implemented.
      </div>
    </div>
  );
}
