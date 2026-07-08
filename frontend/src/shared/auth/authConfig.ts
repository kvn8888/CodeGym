export interface Auth0ClientConfig {
  domain: string;
  clientId: string;
  audience: string;
}

export const auth0ClientConfig: Auth0ClientConfig = {
  domain: (import.meta.env.VITE_AUTH0_DOMAIN ?? '').trim(),
  clientId: (import.meta.env.VITE_AUTH0_CLIENT_ID ?? '').trim(),
  audience: (import.meta.env.VITE_AUTH0_AUDIENCE ?? '').trim(),
};

export function hasAuth0ClientConfig(config = auth0ClientConfig): boolean {
  return config.domain !== '' && config.clientId !== '';
}

export function hasAuth0ApiAudience(config = auth0ClientConfig): boolean {
  return config.audience !== '';
}

export function auth0AuthorizationParams(extra: Record<string, string> = {}) {
  return {
    ...(hasAuth0ApiAudience() ? { audience: auth0ClientConfig.audience } : {}),
    ...extra,
  };
}
