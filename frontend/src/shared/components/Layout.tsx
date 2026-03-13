import { Link, Outlet, useLocation } from 'react-router-dom';

const navItems = [
  { path: '/', label: 'PROBLEMS' },
  { path: '/generate', label: 'GENERATE' },
  { path: '/dashboard', label: 'DASHBOARD' },
];

export function Layout() {
  const location = useLocation();

  return (
    <div className="min-h-screen flex flex-col bg-bone">
      <header className="border-b border-ink bg-bone sticky top-0 z-50">
        <div className="max-w-7xl mx-auto px-6 h-12 flex items-center justify-between">
          <Link to="/" className="text-sm font-bold tracking-[0.2em] text-ink no-underline">
            CODEGYM
          </Link>
          <nav className="flex items-center gap-6">
            {navItems.map((item) => (
              <Link
                key={item.path}
                to={item.path}
                className={`text-xs tracking-[0.1em] no-underline px-2 py-1 transition-colors ${
                  location.pathname === item.path
                    ? 'text-bone bg-ink'
                    : 'text-ash hover:text-ink'
                }`}
              >
                {item.label}
              </Link>
            ))}
          </nav>
        </div>
      </header>
      <main className="flex-1">
        <Outlet />
      </main>
    </div>
  );
}
