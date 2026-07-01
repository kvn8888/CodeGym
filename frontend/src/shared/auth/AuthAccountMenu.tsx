import { useAuth0 } from '@auth0/auth0-react';
import { CircleHelp, LogIn, LogOut, Settings, UserRound } from 'lucide-react';
import { AnimatePresence, motion, useReducedMotion } from 'motion/react';
import { useEffect, useRef, useState, type ComponentType } from 'react';
import {
  auth0AuthorizationParams,
  hasAuth0ApiAudience,
  hasAuth0ClientConfig,
} from './authConfig';

export function AuthAccountMenu({ collapsed }: { collapsed: boolean }) {
  if (!hasAuth0ClientConfig()) {
    return <DevAccountMenu collapsed={collapsed} />;
  }

  return <Auth0AccountMenu collapsed={collapsed} />;
}

function Auth0AccountMenu({ collapsed }: { collapsed: boolean }) {
  const {
    error,
    isAuthenticated,
    isLoading,
    loginWithRedirect,
    logout,
    user,
  } = useAuth0();
  const shouldReduceMotion = useReducedMotion();
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const handleClick = (event: MouseEvent) => {
      if (ref.current && !ref.current.contains(event.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener('mousedown', handleClick);
    return () => document.removeEventListener('mousedown', handleClick);
  }, []);

  const label = isLoading
    ? 'Loading...'
    : isAuthenticated
      ? user?.email ?? user?.name ?? 'Signed in'
      : 'Sign in';
  const initials = initialsFor(user?.name ?? user?.email ?? 'CodeGym');

  return (
    <div ref={ref} className="relative shrink-0 border-t border-gray-alpha-200 p-2">
      <motion.button
        type="button"
        onClick={() => setOpen((current) => !current)}
        whileTap={shouldReduceMotion ? undefined : { scale: 0.98 }}
        className={`cg-focus flex h-11 w-full items-center rounded-md hover:bg-gray-alpha-100 ${
          collapsed ? 'justify-center px-0' : 'gap-3 px-2'
        }`}
      >
        <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-gray-1000 font-mono text-[11px] font-semibold text-background-100">
          {isAuthenticated ? initials : <UserRound size={15} strokeWidth={1.8} />}
        </div>
        {!collapsed && (
          <span className="truncate text-left text-[13px] text-gray-900">
            {label}
          </span>
        )}
      </motion.button>

      <AnimatePresence>
        {open && (
          <motion.div
            initial={{ opacity: 0, y: 8, scale: 0.98 }}
            animate={{ opacity: 1, y: 0, scale: 1 }}
            exit={{ opacity: 0, y: 6, scale: 0.98 }}
            transition={shouldReduceMotion ? { duration: 0 } : { duration: 0.2, ease: [0.175, 0.885, 0.32, 1.1] }}
            className={`absolute z-50 overflow-hidden rounded-xl border border-gray-alpha-200 bg-background-100 ${
              collapsed ? 'bottom-2 left-full ml-2 w-72' : 'bottom-full left-2 right-2 mb-2'
            }`}
            style={{ boxShadow: 'var(--cg-popover-shadow)' }}
          >
            <div className="border-b border-gray-alpha-200 px-4 py-3">
              <div className="truncate text-[13px] font-medium text-gray-1000">
                {label}
              </div>
              <div className="mt-0.5 text-xs text-gray-700">
                {isAuthenticated ? 'Personal workspace' : 'Auth0 Single Page App'}
              </div>
              {!hasAuth0ApiAudience() && (
                <div className="mt-2 rounded-md bg-amber-50 px-2 py-1.5 text-xs text-amber-900">
                  Set VITE_AUTH0_AUDIENCE before calling protected backend routes.
                </div>
              )}
              {error && (
                <div className="mt-2 rounded-md bg-red-50 px-2 py-1.5 text-xs text-red-900">
                  {error.message}
                </div>
              )}
            </div>

            <div className="py-1">
              {isAuthenticated ? (
                <>
                  <ProfileAction icon={Settings} label="Settings" />
                  <ProfileAction icon={CircleHelp} label="Get Help" />
                  <ProfileAction
                    icon={LogOut}
                    label="Log Out"
                    onClick={() => logout({ logoutParams: { returnTo: window.location.origin } })}
                  />
                </>
              ) : (
                <>
                  <ProfileAction
                    icon={LogIn}
                    label="Log In"
                    onClick={() =>
                      loginWithRedirect({ authorizationParams: auth0AuthorizationParams() })
                    }
                  />
                  <ProfileAction
                    icon={UserRound}
                    label="Sign Up"
                    onClick={() =>
                      loginWithRedirect({
                        authorizationParams: auth0AuthorizationParams({ screen_hint: 'signup' }),
                      })
                    }
                  />
                </>
              )}
            </div>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  );
}

function DevAccountMenu({ collapsed }: { collapsed: boolean }) {
  return (
    <div className="relative shrink-0 border-t border-gray-alpha-200 p-2">
      <div
        className={`flex h-11 w-full items-center rounded-md ${
          collapsed ? 'justify-center px-0' : 'gap-3 px-2'
        }`}
      >
        <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-gray-1000 font-mono text-[11px] font-semibold text-background-100">
          CG
        </div>
        {!collapsed && (
          <span className="truncate text-left text-[13px] text-gray-900">
            Dev auth
          </span>
        )}
      </div>
    </div>
  );
}

function ProfileAction({
  icon: Icon,
  label,
  onClick,
}: {
  icon: ComponentType<{ size?: number; strokeWidth?: number }>;
  label: string;
  onClick?: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="cg-focus flex w-full items-center gap-3 px-4 py-2.5 text-left text-[13px] text-gray-900 hover:bg-gray-alpha-100 hover:text-gray-1000"
    >
      <Icon size={16} strokeWidth={1.8} />
      {label}
    </button>
  );
}

function initialsFor(value: string) {
  const words = value
    .split(/[^a-zA-Z0-9]+/)
    .filter(Boolean)
    .slice(0, 2);

  if (words.length === 0) {
    return 'CG';
  }

  return words.map((word) => word[0]).join('').toUpperCase();
}
