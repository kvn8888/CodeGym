import { useEffect, useRef, useState } from 'react';
import { Link, Outlet, useLocation } from 'react-router-dom';
import { AnimatePresence, motion, useReducedMotion } from 'motion/react';
import {
  Brain,
  CircleHelp,
  Grid2X2,
  LayoutDashboard,
  LogOut,
  PanelLeftClose,
  PanelLeftOpen,
  Plus,
  Rows3,
  Settings,
  type LucideIcon,
} from 'lucide-react';
import { FloatingChat } from './FloatingChat';

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
  const [profileOpen, setProfileOpen] = useState(false);
  const profileRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const handleClick = (event: MouseEvent) => {
      if (profileRef.current && !profileRef.current.contains(event.target as Node)) {
        setProfileOpen(false);
      }
    };
    document.addEventListener('mousedown', handleClick);
    return () => document.removeEventListener('mousedown', handleClick);
  }, []);

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
        <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
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
                  className="w-full"
                >
                  <Link
                    to={item.path}
                    title={collapsed ? item.label : undefined}
                    className={`cg-focus relative flex h-10 items-center rounded-md text-sm no-underline ${
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
                      <span className="relative z-10 truncate font-medium">
                        {item.label}
                      </span>
                    )}
                  </Link>
                </motion.div>
              );
            })}
          </nav>
        </div>

        <div ref={profileRef} className="relative shrink-0 border-t border-gray-alpha-200 p-2">
          <motion.button
            type="button"
            onClick={() => setProfileOpen((open) => !open)}
            whileTap={shouldReduceMotion ? undefined : { scale: 0.98 }}
            className={`cg-focus flex h-11 w-full items-center rounded-md hover:bg-gray-alpha-100 ${
              collapsed ? 'justify-center px-0' : 'gap-3 px-2'
            }`}
          >
            <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-gray-1000 font-mono text-[11px] font-semibold text-background-100">
              KC
            </div>
            {!collapsed && (
              <span className="truncate text-left text-[13px] text-gray-900">
                kvn.c8888
              </span>
            )}
          </motion.button>

          <AnimatePresence>
            {profileOpen && (
              <motion.div
                initial={{ opacity: 0, y: 8, scale: 0.98 }}
                animate={{ opacity: 1, y: 0, scale: 1 }}
                exit={{ opacity: 0, y: 6, scale: 0.98 }}
                transition={shouldReduceMotion ? { duration: 0 } : { duration: 0.2, ease: [0.175, 0.885, 0.32, 1.1] }}
                className={`absolute z-50 overflow-hidden rounded-xl border border-gray-alpha-200 bg-background-100 ${
                  collapsed ? 'bottom-2 left-full ml-2 w-60' : 'bottom-full left-2 right-2 mb-2'
                }`}
                style={{ boxShadow: 'var(--cg-popover-shadow)' }}
              >
                <div className="border-b border-gray-alpha-200 px-4 py-3">
                  <div className="truncate text-[13px] font-medium text-gray-1000">
                    kvn.c8888@gmail.com
                  </div>
                  <div className="mt-0.5 text-xs text-gray-700">Personal workspace</div>
                </div>
                <div className="py-1">
                  <ProfileAction icon={Settings} label="Settings" />
                  <ProfileAction icon={CircleHelp} label="Get Help" />
                </div>
                <div className="border-t border-gray-alpha-200 py-1">
                  <ProfileAction icon={LogOut} label="Log Out" />
                </div>
              </motion.div>
            )}
          </AnimatePresence>
        </div>
      </motion.aside>

      <main className="min-w-0 flex-1">
        <Outlet />
      </main>

      <FloatingChat />
    </div>
  );
}

function ProfileAction({ icon: Icon, label }: { icon: LucideIcon; label: string }) {
  return (
    <button
      type="button"
      className="cg-focus flex w-full items-center gap-3 px-4 py-2.5 text-left text-[13px] text-gray-900 hover:bg-gray-alpha-100 hover:text-gray-1000"
    >
      <Icon size={16} strokeWidth={1.8} />
      {label}
    </button>
  );
}
