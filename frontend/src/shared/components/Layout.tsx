import { useState } from 'react';
import { Link, Outlet, useLocation } from 'react-router-dom';

// ── Sidebar widths ─────────────────────────────────────────────────────────────
// Expanded  → w-56   (14rem / 224px)
// Collapsed → w-12   (3rem  /  48px) — thin icon-only strip
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

// Panel / sidebar-layout icon provided by the design.
// Looks like a split-panel rectangle — left strip + right body.
const PanelIcon = () => (
  <svg width="16" height="16" viewBox="0 0 20 20" fill="currentColor">
    <path d="M16.5 4C17.3284 4 18 4.67157 18 5.5V14.5C18 15.3284 17.3284 16 16.5 16H3.5C2.67157 16 2 15.3284 2 14.5V5.5C2 4.67157 2.67157 4 3.5 4H16.5ZM7 15H16.5C16.7761 15 17 14.7761 17 14.5V5.5C17 5.22386 16.7761 5 16.5 5H7V15ZM3.5 5C3.22386 5 3 5.22386 3 5.5V14.5C3 14.7761 3.22386 15 3.5 15H6V5H3.5Z" />
  </svg>
);

export function Layout() {
  const location = useLocation();
  // true → sidebar is the thin icon-only strip (w-12)
  // false → sidebar is fully expanded (w-56)
  const [collapsed, setCollapsed] = useState(false);

  return (
    <div className="min-h-screen flex bg-bone">
      {/* ── Sidebar ──────────────────────────────────────────────────────────
          Width transitions between w-56 (14rem) and w-12 (3rem).
          overflow-hidden clips all inner content as the frame shrinks so
          nothing bleeds into the main area during the animation.           */}
      <aside
        className={`${
          collapsed ? 'w-12' : 'w-56'
        } shrink-0 flex flex-col h-screen sticky top-0 bg-parchment border-r border-grain overflow-hidden transition-[width] duration-300 ease-in-out`}
      >
        {/* ── Header: logo + toggle ──────────────────────────────────────── */}
        <div className="flex items-center gap-2 px-3 pt-5 pb-3">
          {/* Logo text fades out at 150ms — quicker than the 300ms width
              transition — so it disappears before the frame is fully narrow. */}
          <div
            className={`flex-1 min-w-0 overflow-hidden whitespace-nowrap transition-opacity duration-150 ${
              collapsed ? 'opacity-0' : 'opacity-100'
            }`}
          >
            <Link to="/" className="text-sm font-bold tracking-[0.18em] text-ink no-underline">
              CODEGYM
            </Link>
          </div>

          {/* Panel toggle button — always visible in the sidebar.
              Uses the split-panel icon so it communicates "toggle a panel".  */}
          <button
            onClick={() => setCollapsed((c) => !c)}
            aria-label={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
            className="shrink-0 w-6 h-6 flex items-center justify-center rounded-md text-ash hover:text-ink hover:bg-grain transition-colors"
          >
            <PanelIcon />
          </button>
        </div>

        {/* ── Navigation items ──────────────────────────────────────────── */}
        <nav className="flex-1 px-2 py-2 flex flex-col gap-0.5">
          {navItems.map((item) => {
            const active =
              item.path === '/'
                ? location.pathname === '/' || location.pathname.startsWith('/problems')
                : location.pathname.startsWith(item.path);
            return (
              <Link
                key={item.path}
                to={item.path}
                // title shows as a native tooltip when collapsed — useful
                // accessibility hint since labels are invisible.
                title={collapsed ? item.label : undefined}
                className={`flex items-center gap-3 px-3 py-2.5 rounded-xl text-sm no-underline transition-colors ${
                  active
                    ? 'bg-white text-ink font-medium'
                    : 'text-graphite hover:bg-grain hover:text-ink'
                }`}
                style={active ? { boxShadow: '0 1px 3px rgba(0,0,0,0.06)' } : {}}
              >
                {/* Icon is always visible (shrink-0 prevents it from being squished) */}
                <span className={`shrink-0 ${active ? 'text-ink' : 'text-ash'}`}>
                  {item.icon}
                </span>

                {/* Label fades fast (100ms) and collapses to zero width so it
                    doesn't shift the icon position once the sidebar is narrow. */}
                <span
                  className={`whitespace-nowrap overflow-hidden transition-opacity duration-100 ${
                    collapsed ? 'opacity-0 max-w-0' : 'opacity-100'
                  }`}
                >
                  {item.label}
                </span>
              </Link>
            );
          })}
        </nav>

        {/* ── Footer ────────────────────────────────────────────────────── */}
        <div className="px-3 py-5 border-t border-grain">
          <span
            className={`text-xs text-ash tracking-wide whitespace-nowrap transition-opacity duration-150 ${
              collapsed ? 'opacity-0' : 'opacity-100'
            }`}
          >
            v0.1
          </span>
        </div>
      </aside>

      {/* ── Main content ──────────────────────────────────────────────────────
          flex-1 fills whatever width the sidebar doesn't claim, so the
          expansion/contraction is automatically synchronized — no extra
          animation code needed here.                                        */}
      <main className="flex-1 min-w-0">
        <Outlet />
      </main>
    </div>
  );
}



