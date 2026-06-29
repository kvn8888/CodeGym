import { Auth0Provider, useAuth0 } from '@auth0/auth0-react';
import { useEffect, type ReactNode } from 'react';
import { setApiAccessTokenProvider } from '../api/client';
import {
  auth0AuthorizationParams,
  auth0ClientConfig,
  hasAuth0ApiAudience,
  hasAuth0ClientConfig,
} from './authConfig';

export function CodeGymAuthProvider({ children }: { children: ReactNode }) {
  if (!hasAuth0ClientConfig()) {
    return <>{children}</>;
  }

  return (
    <Auth0Provider
      domain={auth0ClientConfig.domain}
      clientId={auth0ClientConfig.clientId}
      authorizationParams={{
        redirect_uri: window.location.origin,
        ...auth0AuthorizationParams(),
      }}
    >
      <ApiTokenBridge />
      {children}
    </Auth0Provider>
  );
}

function ApiTokenBridge() {
  const { getAccessTokenSilently, isAuthenticated } = useAuth0();

  useEffect(() => {
    if (!isAuthenticated) {
      setApiAccessTokenProvider(null);
      return;
    }

    setApiAccessTokenProvider(async () =>
      getAccessTokenSilently({
        authorizationParams: hasAuth0ApiAudience() ? auth0AuthorizationParams() : undefined,
      }),
    );

    return () => setApiAccessTokenProvider(null);
  }, [getAccessTokenSilently, isAuthenticated]);

  return null;
}
