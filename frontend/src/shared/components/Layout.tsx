import { useEffect, useState } from 'react';
import { Link, Outlet, useLocation } from 'react-router-dom';
import { motion, useReducedMotion } from 'motion/react';
import {
  Brain,
  Grid2X2,
  LayoutDashboard,
  PanelLeftClose,
  PanelLeftOpen,
  Plus,
  Rows3,
  type LucideIcon,
} from 'lucide-react';
import { FloatingChat } from './FloatingChat';
import { AuthAccountMenu } from '../auth/AuthAccountMenu';

type NavItem = {
  path: string;
  label: string;
  icon: LucideIcon;
  match?: (pathname: string) => boolean;
};

const navItems: NavItem[] = [
  { path: '/dashboard', label: 'Dashboard', icon: LayoutDashboard },
  { path: '/generate', label: 'Generate', icon: Plus },
  {
    path: '/',
    label: 'Problems',
    icon: Grid2X2,
    match: (pathname: string) => pathname === '/' || pathname.startsWith('/problems'),
  },
  { path: '/marathon', label: 'Marathon', icon: Rows3 },
  { path: '/memory', label: 'Memory', icon: Brain },
];

const sidebarTransition = {
  type: 'spring' as const,
  stiffness: 420,
  damping: 38,
  mass: 0.9,
};

export function Layout() {
  const location = useLocation();
  const shouldReduceMotion = useReducedMotion();
  const [collapsed, setCollapsed] = useState(false);

  useEffect(() => {
    const handleKeyboard = (event: KeyboardEvent) => {
      if (event.key === '/' && (event.metaKey || event.ctrlKey)) {
        event.preventDefault();
        setCollapsed((current) => !current);
      }
    };
    document.addEventListener('keydown', handleKeyboard);
    return () => document.removeEventListener('keydown', handleKeyboard);
  }, []);

  const motionTransition = shouldReduceMotion ? { duration: 0 } : sidebarTransition;

  return (
    <div className="flex min-h-screen bg-background-200 text-gray-1000">
      <motion.aside
        animate={{ width: collapsed ? 52 : 232 }}
        transition={motionTransition}
        className="sticky top-0 z-40 flex h-screen shrink-0 flex-col border-r border-gray-alpha-200 bg-background-100"
      >
        <div className="flex min-h-0 flex-1 flex-col">
          <div className={`flex h-16 shrink-0 items-center ${collapsed ? 'justify-center px-2' : 'gap-3 px-4'}`}>
            <motion.div
              animate={{
                opacity: collapsed ? 0 : 1,
                width: collapsed ? 0 : 132,
              }}
              transition={shouldReduceMotion ? { duration: 0 } : { duration: 0.15 }}
              className="min-w-0 overflow-hidden whitespace-nowrap"
            >
              <Link to="/" className="font-mono text-[13px] font-semibold tracking-[0.08em] text-gray-1000 no-underline">
                CODEGYM
              </Link>
            </motion.div>

            <motion.button
              type="button"
              onClick={() => setCollapsed((current) => !current)}
              whileTap={shouldReduceMotion ? undefined : { scale: 0.96 }}
              aria-label={collapsed ? 'Expand Sidebar' : 'Collapse Sidebar'}
              className="cg-focus flex h-8 w-8 shrink-0 items-center justify-center rounded-md text-gray-900 hover:bg-gray-alpha-100 hover:text-gray-1000"
            >
              {collapsed ? <PanelLeftOpen size={16} strokeWidth={1.8} /> : <PanelLeftClose size={16} strokeWidth={1.8} />}
            </motion.button>
          </div>

          <nav className={`flex flex-1 flex-col gap-1 px-2 py-2 ${collapsed ? 'items-center' : ''}`}>
            {navItems.map((item) => {
              const active = item.match ? item.match(location.pathname) : location.pathname.startsWith(item.path);
              const Icon = item.icon;

              return (
                <motion.div
                  key={item.path}
                  whileHover={shouldReduceMotion ? undefined : { y: -1 }}
                  whileTap={shouldReduceMotion ? undefined : { scale: 0.98 }}
                  className="t-tt-wrap w-full"
                >
                  <Link
                    to={item.path}
                    className={`t-tt-trigger cg-focus relative flex h-10 items-center rounded-md text-sm no-underline ${
                      collapsed ? 'w-9 justify-center px-0' : 'w-full gap-3 px-3'
                    } ${active ? 'text-gray-1000' : 'text-gray-900 hover:bg-gray-alpha-100 hover:text-gray-1000'}`}
                  >
                    {active && (
                      <motion.span
                        layoutId="geist-nav-active"
                        transition={shouldReduceMotion ? { duration: 0 } : { type: 'spring', stiffness: 500, damping: 40 }}
                        className="absolute inset-0 rounded-md border border-gray-alpha-200 bg-gray-100"
                        aria-hidden="true"
                      />
                    )}
                    <Icon className="relative z-10 shrink-0" size={17} strokeWidth={1.8} />
                    {!collapsed && (
                      <span className="relative z-10 min-w-0 truncate font-medium">
                        {item.label}
                      </span>
                    )}
                  </Link>
                  {/* transitions-dev tooltip (17): only when collapsed, since the
                      label is hidden. Pure CSS — shows on hover/focus of the row. */}
                  {collapsed && (
                    <span className="t-tt" role="tooltip">
                      {item.label}
                    </span>
                  )}
                </motion.div>
              );
            })}
          </nav>
        </div>

        <AuthAccountMenu collapsed={collapsed} />
      </motion.aside>

      <main className="min-w-0 flex-1">
        <Outlet />
      </main>

      <FloatingChat />
    </div>
  );
}
