import { Auth0Provider, useAuth0 } from '@auth0/auth0-react';
import { useCallback, useEffect, useLayoutEffect, useState, type ReactNode } from 'react';
import { setApiAccessTokenProvider } from '../api/client';
import { CodeGymAuthContext } from './authState';
import {
  auth0AppOrigin,
  auth0AuthorizationParams,
  auth0ClientConfig,
  hasAuth0ApiAudience,
  hasAuth0ClientConfig,
} from './authConfig';

export function CodeGymAuthProvider({ children }: { children: ReactNode }) {
  if (!hasAuth0ClientConfig()) {
    return (
      <CodeGymAuthContext.Provider value={{ configured: false, isAuthenticated: true }}>
        {children}
      </CodeGymAuthContext.Provider>
    );
  }

  const appOrigin = auth0AppOrigin();
  if (window.location.origin !== appOrigin) {
    return <Auth0OriginRedirect appOrigin={appOrigin} />;
  }

  return (
    <Auth0Provider
      domain={auth0ClientConfig.domain}
      clientId={auth0ClientConfig.clientId}
      authorizationParams={{
        redirect_uri: auth0AppOrigin(),
        ...auth0AuthorizationParams(),
      }}
    >
      <AuthSessionBridge>{children}</AuthSessionBridge>
    </Auth0Provider>
  );
}

function Auth0OriginRedirect({ appOrigin }: { appOrigin: string }) {
  useEffect(() => {
    const destination = new URL(window.location.href);
    const canonicalOrigin = new URL(appOrigin);
    destination.protocol = canonicalOrigin.protocol;
    destination.host = canonicalOrigin.host;
    window.location.replace(destination);
  }, [appOrigin]);

  return (
    <div className="grid min-h-screen place-items-center bg-background font-mono text-xs uppercase tracking-[0.18em] text-muted-foreground">
      Opening secure sign in
    </div>
  );
}

function AuthSessionBridge({ children }: { children: ReactNode }) {
  const { getAccessTokenSilently, isAuthenticated, isLoading } = useAuth0();
  const expectedAuthState = isAuthenticated ? 'authenticated' : 'anonymous';
  const [installedAuthState, setInstalledAuthState] = useState<string | null>(null);
  const tokenProvider = useCallback(
    () =>
      getAccessTokenSilently({
        authorizationParams: hasAuth0ApiAudience() ? auth0AuthorizationParams() : undefined,
      }),
    [getAccessTokenSilently],
  );

  // Route loaders start in passive effects. Install the bearer-token source
  // in the preceding layout phase, then render route content on the next
  // commit so loaders cannot race Auth0 callback processing.
  useLayoutEffect(() => {
    if (isLoading) return;
    let active = true;
    setApiAccessTokenProvider(isAuthenticated ? tokenProvider : null);
    queueMicrotask(() => {
      if (active) setInstalledAuthState(expectedAuthState);
    });
    return () => {
      active = false;
      setApiAccessTokenProvider(null);
    };
  }, [expectedAuthState, isAuthenticated, isLoading, tokenProvider]);

  if (isLoading || installedAuthState !== expectedAuthState) {
    return (
      <div className="grid min-h-screen place-items-center bg-background font-mono text-xs uppercase tracking-[0.18em] text-muted-foreground">
        Restoring account
      </div>
    );
  }

  // OAuth navigation may restore the pre-login page from the browser's
  // back-forward cache. Remount route content when auth state flips so stale
  // unauthenticated loaders rerun with the newly installed token provider.
  return (
    <CodeGymAuthContext.Provider value={{ configured: true, isAuthenticated }}>
      <AuthContent key={isAuthenticated ? 'authenticated' : 'anonymous'}>
        {children}
      </AuthContent>
    </CodeGymAuthContext.Provider>
  );
}

function AuthContent({ children }: { children: ReactNode }) {
  return <>{children}</>;
}
