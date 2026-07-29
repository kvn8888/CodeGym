# Deploying the frontend to Vercel

Production frontend project: `code-gym` (alias example:
[code-gym-rho.vercel.app](https://code-gym-rho.vercel.app)).

The React/Vite app deploys on Vercel and calls the matching Render backend
through an explicit environment-scoped `VITE_API_BASE_URL`.

Build-time config (`VITE_*`) comes from **Doppler only**. Vercel holds a
Production-only `DOPPLER_TOKEN` for `codegym/prd_frontend` and a Preview-only
token for `codegym/stg_frontend`; the build runs
`doppler run -- npm run build`. Do not paste Auth0 or other `VITE_*` values
into the Vercel dashboard.

See also [secrets-and-local-env.md](./secrets-and-local-env.md).

## How the connection works

`vercel.json`:

1. **Build** — install Doppler CLI, then
   `doppler run -- npm run build` so Vite inlines `VITE_*` from
   matching Production or Preview config.
2. **Direct API routing** — Production embeds
   `https://codegym.onrender.com/api/v1`; Preview embeds
   `https://codegym-staging.onrender.com/api/v1`.
3. **Stable Auth origin** — `VITE_APP_ORIGIN` supplies the canonical
   Production URL or stable branch Preview alias. Opening an ephemeral Vercel
   deployment first redirects to that stable origin, so the Auth0 PKCE
   transaction and callback both remain on one browser origin.
4. **SPA fallback** `/*` → `/index.html` for client routes such as `/marathon`.

The Render services must allow their matching Vercel origin through CORS.
`vercel.json` intentionally has no backend rewrite so Preview cannot fall
through to Production.

## Environment variables on Vercel

| Key | Environments | Notes |
| --- | --- | --- |
| `DOPPLER_TOKEN` | Production | Service token for `codegym/prd_frontend` only. |
| `DOPPLER_TOKEN` | Preview | Different service token for `codegym/stg_frontend` only. |

Browser config lives in separate Doppler frontend configs:

| Doppler key | Purpose |
| --- | --- |
| `VITE_AUTH0_DOMAIN` | Auth0 SPA domain |
| `VITE_AUTH0_CLIENT_ID` | Auth0 SPA client id (public) |
| `VITE_AUTH0_AUDIENCE` | **API Identifier** of the CodeGym Auth0 API (must match `CODEGYM_AUTH0_AUDIENCE`). Not the Management API (`…/api/v2/`). |
| `VITE_API_BASE_URL` | Absolute URL of the environment's matching Render API |
| `VITE_USE_MOCK_API` | `false` for the real Daytona demo |
| `VITE_APP_ORIGIN` | Canonical Production URL or stable branch Preview alias used for Auth0 callbacks/logout |

Auth0 login mounts when `VITE_AUTH0_DOMAIN` and `VITE_AUTH0_CLIENT_ID` are both
set. API calls still need a matching audience: if login works but Settings
shows `Invalid bearer token`, fix the API Identifier on the matching Doppler
config and redeploy (see `backend/README.md`). The Production Auth0 SPA should
allow only the canonical Production URL. The staging SPA should allow only the
stable branch Preview alias; do not add every ephemeral Vercel deployment URL.

Without Auth0 values, the app can fall back to a localStorage bearer token for
manual testing:

```js
localStorage.setItem('codegym_token', '<token that the backend accepts>')
```

`VITE_USE_MOCK_API=false` documents that this deployment is the real-API demo.
Mocks are also gated on `import.meta.env.DEV`, so they cannot run in the
production build.

## Rotate `DOPPLER_TOKEN`

1. Create a new service token in Doppler (`prd_frontend` → Access → Service Tokens).
2. Update `DOPPLER_TOKEN` on Vercel (Production and Preview).
3. Redeploy so the next build pulls secrets with the new token.
4. Revoke the old token in Doppler.

## Verify

1. Open the production alias (e.g. `https://code-gym-rho.vercel.app`).
2. Confirm the shell loads (200 HTML).
3. In DevTools Network, confirm API calls reach the environment's matching
   Render hostname.
4. Open an ephemeral Preview deployment and confirm it redirects to the stable
   branch Preview alias before login. Then confirm sign-in returns to that same
   stable alias.
5. Quick check:

```sh
curl -sS -o /dev/null -w "%{http_code}\n" https://code-gym-rho.vercel.app/
curl -sS https://codegym.onrender.com/health
curl -sS https://codegym-staging.onrender.com/health
```
