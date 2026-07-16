import { useEffect, useState } from 'react';
import { Link, Outlet, useLocation } from 'react-router-dom';
import { motion, useReducedMotion } from 'motion/react';
import {
  Brain,
  History,
  LayoutDashboard,
  Menu,
  PanelLeftClose,
  PanelLeftOpen,
  Plus,
  Settings,
  X,
  type LucideIcon,
} from 'lucide-react';

import { FloatingChat } from './FloatingChat';
import { ModeToggle } from '@/components/mode-toggle';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import { AuthAccountMenu } from '../auth/AuthAccountMenu';

type NavItem = {
  path: string;
  label: string;
  icon: LucideIcon;
  match?: (pathname: string) => boolean;
};

const navItems: NavItem[] = [
  { path: '/dashboard', label: 'Dashboard', icon: LayoutDashboard },
  { path: '/generate', label: 'New practice', icon: Plus },
  {
    path: '/',
    label: 'History',
    icon: History,
    match: (pathname: string) => pathname === '/' || pathname.startsWith('/problems'),
  },
  { path: '/memory', label: 'Memory', icon: Brain },
  { path: '/settings', label: 'Settings', icon: Settings },
];

const sidebarTransition = {
  type: 'spring' as const,
  stiffness: 420,
  damping: 38,
  mass: 0.9,
};

function NavigationLinks({
  pathname,
  collapsed,
  shouldReduceMotion,
  onNavigate,
  layoutId,
}: {
  pathname: string;
  collapsed: boolean;
  shouldReduceMotion: boolean | null;
  onNavigate?: () => void;
  layoutId: string;
}) {
  return navItems.map((item) => {
    const active = item.match ? item.match(pathname) : pathname.startsWith(item.path);
    const Icon = item.icon;

    return (
      <motion.div
        key={item.path}
        whileTap={shouldReduceMotion ? undefined : { scale: 0.98 }}
        className="t-tt-wrap w-full"
      >
        <Link
          to={item.path}
          onClick={onNavigate}
          className={cn(
            't-tt-trigger relative flex h-9 items-center rounded-md text-sm no-underline outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50',
            collapsed ? 'w-9 justify-center px-0' : 'w-full gap-3 px-3',
            active
              ? 'text-foreground'
              : 'text-muted-foreground hover:bg-sidebar-accent hover:text-foreground',
          )}
        >
          {active && (
            <motion.span
              layoutId={layoutId}
              transition={shouldReduceMotion ? { duration: 0 } : { type: 'spring', stiffness: 500, damping: 40 }}
              className="bg-sidebar-accent border-sidebar-border absolute inset-0 rounded-md border"
              aria-hidden="true"
            />
          )}
          <Icon className="relative z-10 shrink-0" size={17} strokeWidth={1.8} />
          {!collapsed && <span className="relative z-10 min-w-0 truncate font-medium">{item.label}</span>}
        </Link>
        {collapsed && (
          <span className="t-tt" role="tooltip">
            {item.label}
          </span>
        )}
      </motion.div>
    );
  });
}

export function Layout() {
  const location = useLocation();
  const shouldReduceMotion = useReducedMotion();
  const [collapsed, setCollapsed] = useState(false);
  const [mobileOpen, setMobileOpen] = useState(false);
  const showContextualChat =
    location.pathname.startsWith('/problems/') || location.pathname.startsWith('/marathon');

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
    <div className="bg-background text-foreground min-h-screen md:flex">
      <header className="bg-background sticky top-0 z-40 flex h-14 items-center justify-between border-b px-3 md:hidden">
        <Link to="/dashboard" className="font-mono text-[13px] font-semibold tracking-[0.08em] no-underline">
          CODEGYM
        </Link>
        <div className="flex items-center gap-1">
          <AuthAccountMenu collapsed />
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="size-9"
            onClick={() => setMobileOpen((current) => !current)}
            aria-label={mobileOpen ? 'Close navigation' : 'Open navigation'}
            aria-expanded={mobileOpen}
          >
            {mobileOpen ? <X /> : <Menu />}
          </Button>
        </div>
      </header>

      {mobileOpen && (
        <div className="bg-background fixed inset-x-0 top-14 bottom-0 z-50 flex flex-col md:hidden">
          <nav className="flex flex-1 flex-col gap-1 px-3 py-4">
            <NavigationLinks
              pathname={location.pathname}
              collapsed={false}
              shouldReduceMotion={shouldReduceMotion}
              onNavigate={() => setMobileOpen(false)}
              layoutId="mobile-nav-active"
            />
          </nav>
          <div className="flex items-center justify-between border-t p-3">
            <AuthAccountMenu collapsed={false} />
            <ModeToggle className="size-9 shrink-0" />
          </div>
        </div>
      )}

      <motion.aside
        animate={{ width: collapsed ? 52 : 216 }}
        transition={motionTransition}
        className="bg-sidebar border-sidebar-border sticky top-0 z-40 hidden h-screen shrink-0 flex-col border-r md:flex"
      >
        <div className="flex min-h-0 flex-1 flex-col">
          <div className={`flex h-14 shrink-0 items-center ${collapsed ? 'justify-center px-2' : 'gap-3 px-3'}`}>
            <motion.div
              animate={{ opacity: collapsed ? 0 : 1, width: collapsed ? 0 : 132 }}
              transition={shouldReduceMotion ? { duration: 0 } : { duration: 0.15 }}
              className="min-w-0 overflow-hidden whitespace-nowrap"
            >
              <Link to="/" className="font-mono text-[13px] font-semibold tracking-[0.08em] no-underline">
                CODEGYM
              </Link>
            </motion.div>

            <Button
              type="button"
              variant="ghost"
              size="icon"
              onClick={() => setCollapsed((current) => !current)}
              aria-label={collapsed ? 'Expand Sidebar' : 'Collapse Sidebar'}
              className="text-muted-foreground hover:text-foreground size-8 shrink-0"
            >
              {collapsed ? <PanelLeftOpen strokeWidth={1.8} /> : <PanelLeftClose strokeWidth={1.8} />}
            </Button>
          </div>

          <nav className={`flex flex-1 flex-col gap-0.5 px-2 py-2 ${collapsed ? 'items-center' : ''}`}>
            <NavigationLinks
              pathname={location.pathname}
              collapsed={collapsed}
              shouldReduceMotion={shouldReduceMotion}
              layoutId="desktop-nav-active"
            />
          </nav>
        </div>

        <div
          className={cn(
            'border-sidebar-border flex shrink-0 border-t',
            collapsed ? 'flex-col items-center gap-1 p-2' : 'items-center gap-1 p-2',
          )}
        >
          <AuthAccountMenu collapsed={collapsed} />
          <ModeToggle className="size-9 shrink-0" />
        </div>
      </motion.aside>

      <main className="min-w-0 flex-1">
        <Outlet />
      </main>

      {showContextualChat && <FloatingChat />}
    </div>
  );
}
