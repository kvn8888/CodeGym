import { useState, useRef, useEffect } from 'react';
import { Link, Outlet, useLocation } from 'react-router-dom';
import { FloatingChat } from './FloatingChat';

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
      <div className="flex items-center justify-center rounded-full w-[22px] h-[22px] bg-ash/15">
        <svg width="14" height="14" viewBox="0 0 20 20" fill="currentColor">
          <path d="M10 3C10.4142 3 10.75 3.33579 10.75 3.75V9.25H16.25C16.6642 9.25 17 9.58579 17 10C17 10.3882 16.7051 10.7075 16.3271 10.7461L16.25 10.75H10.75V16.25C10.75 16.6642 10.4142 17 10 17C9.58579 17 9.25 16.6642 9.25 16.25V10.75H3.75C3.33579 10.75 3 10.4142 3 10C3 9.58579 3.33579 9.25 3.75 9.25H9.25V3.75C9.25 3.33579 9.58579 3 10 3Z" />
        </svg>
      </div>
    ),
  },
  {
    path: '/chat',
    label: 'Chat',
    icon: (
      <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round">
        <path d="M21 15a2 2 0 01-2 2H7l-4 4V5a2 2 0 012-2h14a2 2 0 012 2z" />
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
  const [collapsed, setCollapsed] = useState(false);
  const [profileOpen, setProfileOpen] = useState(false);
  const profileRef = useRef<HTMLDivElement>(null);

  // Close profile popup on outside click
  useEffect(() => {
    const handleClick = (e: MouseEvent) => {
      if (profileRef.current && !profileRef.current.contains(e.target as Node)) {
        setProfileOpen(false);
      }
    };
    document.addEventListener('mousedown', handleClick);
    return () => document.removeEventListener('mousedown', handleClick);
  }, []);

  // Cmd+/ (Mac) or Ctrl+/ (Windows/Linux) toggles the sidebar
  useEffect(() => {
    const handleKeyboard = (e: KeyboardEvent) => {
      if (e.key === '/' && (e.metaKey || e.ctrlKey)) {
        e.preventDefault();
        setCollapsed((c) => !c);
      }
    };
    document.addEventListener('keydown', handleKeyboard);
    return () => document.removeEventListener('keydown', handleKeyboard);
  }, []);

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
        <div className={`flex items-center pt-5 pb-3 transition-all duration-300 ${
          collapsed ? 'justify-center px-0' : 'gap-2 px-3'
        }`}>
          {/* Logo wrapper — transitions to max-w-0 when collapsed so
              the toggle button auto-centers in the thin strip. */}
          <div
            className={`min-w-0 overflow-hidden whitespace-nowrap transition-all duration-200 ${
              collapsed ? 'max-w-0 opacity-0' : 'max-w-[10rem] opacity-100 flex-1'
            }`}
          >
            <Link to="/" className="text-sm font-bold tracking-[0.18em] text-ink no-underline">
              CODEGYM
            </Link>
          </div>

          <button
            onClick={() => setCollapsed((c) => !c)}
            aria-label={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
            className="shrink-0 w-7 h-7 flex items-center justify-center rounded-md text-ash hover:text-ink hover:bg-grain transition-colors"
          >
            <PanelIcon />
          </button>
        </div>

        {/* ── Navigation items ──────────────────────────────────────────── */}
        <nav className={`flex-1 py-2 flex flex-col gap-0.5 transition-all duration-300 ${
          collapsed ? 'px-1' : 'px-2'
        }`}>
          {navItems.map((item) => {
            const active =
              item.path === '/'
                ? location.pathname === '/' || location.pathname.startsWith('/problems')
                : location.pathname.startsWith(item.path);
            return (
              <Link
                key={item.path}
                to={item.path}
                title={collapsed ? item.label : undefined}
                className={`flex items-center py-2.5 rounded-xl text-sm no-underline transition-all duration-200 ${
                  collapsed ? 'justify-center px-0 gap-0' : 'px-3 gap-3'
                } ${
                  active
                    ? 'bg-white text-ink font-medium'
                    : 'text-graphite hover:bg-grain hover:text-ink'
                }`}
                style={active ? { boxShadow: '0 1px 3px rgba(0,0,0,0.06)' } : {}}
              >
                <span className={`shrink-0 ${active ? 'text-ink' : 'text-ash'}`}>
                  {item.icon}
                </span>
                {!collapsed && (
                  <span className="whitespace-nowrap overflow-hidden">
                    {item.label}
                  </span>
                )}
              </Link>
            );
          })}
        </nav>

        {/* ── Profile ───────────────────────────────────────────────────── */}
        <div ref={profileRef} className="relative border-t border-grain">
          <button
            onClick={() => setProfileOpen((o) => !o)}
            className={`flex items-center w-full py-3 hover:bg-grain transition-all duration-300 ${
              collapsed ? 'justify-center px-0' : 'gap-3 px-3'
            }`}
          >
            <div className="w-7 h-7 rounded-full bg-ink text-bone flex items-center justify-center text-[10px] font-bold shrink-0">
              KC
            </div>
            {!collapsed && (
              <span className="text-xs text-graphite truncate">kvn.c8888</span>
            )}
          </button>

          {/* Profile popup — anchored above the avatar */}
          {profileOpen && (
            <div
              className="absolute bottom-full left-1 mb-2 w-56 border border-chalk rounded-2xl bg-white overflow-hidden z-50"
              style={{ boxShadow: '0 4px 16px rgba(0,0,0,0.08)' }}
            >
              <div className="px-4 py-3 border-b border-chalk">
                <span className="text-xs text-ink font-medium">kvn.c8888@gmail.com</span>
              </div>
              <div className="py-1">
                <button className="flex items-center gap-3 w-full text-left text-xs text-graphite hover:text-ink hover:bg-parchment px-4 py-2.5 transition-colors">
                  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round"><circle cx="12" cy="12" r="3" /><path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z" /></svg>
                  Settings
                </button>
                <button className="flex items-center gap-3 w-full text-left text-xs text-graphite hover:text-ink hover:bg-parchment px-4 py-2.5 transition-colors">
                  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round"><circle cx="12" cy="12" r="10" /><path d="M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3" /><line x1="12" y1="17" x2="12.01" y2="17" /></svg>
                  Get help
                </button>
              </div>
              <div className="border-t border-chalk py-1">
                <button className="flex items-center gap-3 w-full text-left text-xs text-graphite hover:text-ink hover:bg-parchment px-4 py-2.5 transition-colors">
                  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round"><path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4" /><polyline points="16 17 21 12 16 7" /><line x1="21" y1="12" x2="9" y2="12" /></svg>
                  Log out
                </button>
              </div>
            </div>
          )}
        </div>

      </aside>

      {/* ── Main content ──────────────────────────────────────────────────────
          flex-1 fills whatever width the sidebar doesn't claim, so the
          expansion/contraction is automatically synchronized — no extra
          animation code needed here.                                        */}
      <main className="flex-1 min-w-0">
        <Outlet />
      </main>

      <FloatingChat />
    </div>
  );
}



