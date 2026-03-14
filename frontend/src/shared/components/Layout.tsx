import { useState } from 'react';
import { Link, Outlet, useLocation } from 'react-router-dom';

// ── Sidebar width constant ─────────────────────────────────────────────────────
// Must match the Tailwind class "w-56" (14rem = 224px at default scale).
// The toggle button uses this value to ride along the sidebar's right edge
// during the collapse/expand animation via transform: translateX().
const SIDEBAR_WIDTH_PX = 224; // 14rem × 16px/rem

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
  // Controls whether the sidebar is fully collapsed (w-0) or expanded (w-56).
  const [collapsed, setCollapsed] = useState(false);

  return (
    <div className="min-h-screen flex bg-bone">
      {/* ── Sidebar ──────────────────────────────────────────────────────────
          Width transitions between w-56 and w-0.  overflow-hidden clips
          the contents as the frame shrinks so nothing bleeds out.         */}
      <aside
        className={`${
          collapsed ? 'w-0' : 'w-56'
        } shrink-0 flex flex-col h-screen sticky top-0 bg-parchment border-r border-grain overflow-hidden transition-[width] duration-300 ease-in-out`}
      >
        {/* Inner wrapper fades out quickly (150 ms) so the text/icons
            disappear before the sidebar frame finishes collapsing.
            pointer-events-none prevents clicks on invisible elements.    */}
        <div
          className={`flex flex-col flex-1 min-w-[14rem] transition-opacity duration-150 ${
            collapsed ? 'opacity-0 pointer-events-none' : 'opacity-100'
          }`}
        >
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
        </div>
      </aside>

      {/* ── Sidebar toggle button ─────────────────────────────────────────────
          Fixed to the viewport and translated along the X axis to keep it
          sitting on the sidebar's right border at all times. The same
          easing/duration (300 ms ease-in-out) as the sidebar width transition
          makes the button appear to ride the collapsing edge.

          Layout:
            collapsed=false  → translateX(SIDEBAR_WIDTH_PX - 10px)
                                 centre of the 20px button ≈ right border
            collapsed=true   → translateX(4px)
                                 flush with left edge of viewport             */}
      <button
        onClick={() => setCollapsed((c) => !c)}
        aria-label={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
        className="fixed top-5 left-0 z-50 w-5 h-5 rounded-full bg-white border border-chalk flex items-center justify-center transition-transform duration-300 ease-in-out hover:bg-grain"
        style={{
          transform: `translateX(${collapsed ? 4 : SIDEBAR_WIDTH_PX - 10}px)`,
          boxShadow: '0 1px 3px rgba(0,0,0,0.08)',
        }}
      >
        {/* Chevron flips direction: pointing left when expanded (to collapse),
            pointing right when collapsed (to expand).                        */}
        <svg
          width="10"
          height="10"
          viewBox="0 0 10 10"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.75"
          strokeLinecap="round"
          strokeLinejoin="round"
          className={`text-graphite transition-transform duration-300 ${collapsed ? '' : 'rotate-180'}`}
        >
          <path d="M6.5 2L3.5 5l3 3" />
        </svg>
      </button>

      {/* ── Main content ──────────────────────────────────────────────────────
          flex-1 automatically fills whichever width the sidebar vacates,
          so the expansion is a free side-effect of the sidebar's transition. */}
      <main className="flex-1 min-w-0">
        <Outlet />
      </main>
    </div>
  );
}
