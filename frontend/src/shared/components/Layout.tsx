import { useEffect, useState } from 'react';
import { Link, Outlet, useLocation } from 'react-router-dom';
import { motion, useReducedMotion } from 'motion/react';
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
import { ModeToggle } from '@/components/mode-toggle';
import { Avatar, AvatarFallback } from '@/components/ui/avatar';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { cn } from '@/lib/utils';

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
    <div className="bg-background text-foreground flex min-h-screen">
      <motion.aside
        animate={{ width: collapsed ? 52 : 232 }}
        transition={motionTransition}
        className="bg-sidebar border-sidebar-border sticky top-0 z-40 flex h-screen shrink-0 flex-col border-r"
      >
        <div className="flex min-h-0 flex-1 flex-col">
          <div className={`flex h-16 shrink-0 items-center ${collapsed ? 'justify-center px-2' : 'gap-3 px-4'}`}>
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
                    className={cn(
                      't-tt-trigger relative flex h-10 items-center rounded-md text-sm no-underline outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50',
                      collapsed ? 'w-9 justify-center px-0' : 'w-full gap-3 px-3',
                      active
                        ? 'text-foreground'
                        : 'text-muted-foreground hover:bg-sidebar-accent hover:text-foreground',
                    )}
                  >
                    {active && (
                      <motion.span
                        layoutId="geist-nav-active"
                        transition={shouldReduceMotion ? { duration: 0 } : { type: 'spring', stiffness: 500, damping: 40 }}
                        className="bg-sidebar-accent border-sidebar-border absolute inset-0 rounded-md border"
                        aria-hidden="true"
                      />
                    )}
                    <Icon className="relative z-10 shrink-0" size={17} strokeWidth={1.8} />
                    {!collapsed && (
                      <span className="relative z-10 min-w-0 truncate font-medium">{item.label}</span>
                    )}
                  </Link>
                  {/* transitions-dev tooltip (17): only when collapsed (label hidden). */}
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

        <div className="border-sidebar-border shrink-0 border-t p-2">
          <div className={cn('flex', collapsed ? 'flex-col items-center gap-1' : 'items-center gap-1')}>
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <button
                  type="button"
                  className={cn(
                    'hover:bg-sidebar-accent flex h-11 items-center rounded-md outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50',
                    collapsed ? 'w-9 justify-center px-0' : 'min-w-0 flex-1 gap-3 px-2',
                  )}
                >
                  <Avatar className="size-7">
                    <AvatarFallback className="bg-primary text-primary-foreground font-mono text-[11px] font-semibold">
                      KC
                    </AvatarFallback>
                  </Avatar>
                  {!collapsed && (
                    <span className="text-muted-foreground truncate text-left text-[13px]">
                      kvn.c8888
                    </span>
                  )}
                </button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="start" side="top" className="w-60">
                <DropdownMenuLabel className="font-normal">
                  <div className="text-[13px] font-medium">kvn.c8888@gmail.com</div>
                  <div className="text-muted-foreground text-xs">Personal workspace</div>
                </DropdownMenuLabel>
                <DropdownMenuSeparator />
                <DropdownMenuItem>
                  <Settings />
                  Settings
                </DropdownMenuItem>
                <DropdownMenuItem>
                  <CircleHelp />
                  Get Help
                </DropdownMenuItem>
                <DropdownMenuSeparator />
                <DropdownMenuItem variant="destructive">
                  <LogOut />
                  Log Out
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>

            <ModeToggle className="size-9 shrink-0" />
          </div>
        </div>
      </motion.aside>

      <main className="min-w-0 flex-1">
        <Outlet />
      </main>

      <FloatingChat />
    </div>
  );
}
