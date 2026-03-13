import { Link, Outlet, useLocation } from 'react-router-dom';

const navItems = [
  {
    path: '/',
    label: 'Problems',
    icon: (
      <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round">
        <rect x="3" y="3" width="7" height="7" rx="1" />
        <rect x="14" y="3" width="7" height="7" rx="1" />
        <rect x="3" y="14" width="7" height="7" rx="1" />
        <rect x="14" y="14" width="7" height="7" rx="1" />
      </svg>
    ),
  },
  {
    path: '/generate',
    label: 'Generate',
    icon: (
      <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round">
        <path d="M12 3l1.9 5.8H20l-4.9 3.6 1.9 5.8L12 14.6l-5 3.6 1.9-5.8L4 8.8h6.1z" />
      </svg>
    ),
  },
  {
    path: '/dashboard',
    label: 'Dashboard',
    icon: (
      <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round">
        <path d="M3 12l2-2m0 0l7-7 7 7M5 10v10a1 1 0 001 1h3m10-11l2 2m-2-2v10a1 1 0 01-1 1h-3m-6 0a1 1 0 001-1v-4a1 1 0 011-1h2a1 1 0 011 1v4a1 1 0 001 1m-6 0h6" />
      </svg>
    ),
  },
];

export function Layout() {
  const location = useLocation();

  return (
    <div className="min-h-screen flex bg-bone">
      {/* Sidebar */}
      <aside className="w-56 shrink-0 flex flex-col h-screen sticky top-0 bg-parchment border-r border-grain">
        {/* Logo */}
        <div className="px-5 pt-6 pb-4">
          <Link to="/" className="text-sm font-bold tracking-[0.18em] text-ink no-underline">
            CODEGYM
          </Link>
        </div>

        {/* Nav */}
        <nav className="flex-1 px-3 py-2 flex flex-col gap-0.5">
          {navItems.map((item) => {
            const active =
              item.path === '/'
                ? location.pathname === '/' || location.pathname.startsWith('/problems')
                : location.pathname.startsWith(item.path);
            return (
              <Link
                key={item.path}
                to={item.path}
                className={`flex items-center gap-3 px-3 py-2.5 rounded-xl text-sm no-underline transition-colors ${
                  active
                    ? 'bg-white text-ink font-medium'
                    : 'text-graphite hover:bg-grain hover:text-ink'
                }`}
                style={active ? { boxShadow: '0 1px 3px rgba(0,0,0,0.06)' } : {}}
              >
                <span className={active ? 'text-ink' : 'text-ash'}>{item.icon}</span>
                {item.label}
              </Link>
            );
          })}
        </nav>

        {/* Footer */}
        <div className="px-5 py-5 border-t border-grain">
          <span className="text-xs text-ash tracking-wide">v0.1</span>
        </div>
      </aside>

      {/* Main content */}
      <main className="flex-1 min-w-0">
        <Outlet />
      </main>
    </div>
  );
}
