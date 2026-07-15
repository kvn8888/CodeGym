# Deploying the frontend to Vercel

Production frontend project: `code-gym` (alias example:
[code-gym-rho.vercel.app](https://code-gym-rho.vercel.app)).

The React/Vite app deploys on Vercel and reaches the Go backend on Render
through a **rewrite proxy** in
[`frontend/vercel.json`](../frontend/vercel.json).

Build-time config (`VITE_*`) comes from **Doppler only**. Vercel holds a
single `DOPPLER_TOKEN` (project `codegym`, config `prd_frontend`) and the
build runs `doppler run -- npm run build`. Do not paste Auth0 or other
`VITE_*` values into the Vercel dashboard.

See also [secrets-and-local-env.md](./secrets-and-local-env.md).

## How the connection works

`vercel.json`:

1. **Build** — install Doppler CLI, then
   `doppler run -- npm run build` so Vite inlines `VITE_*` from
   `prd_frontend`.
2. **Rewrite** `/api/*` → `https://codegym.onrender.com/api/*` so the browser
   only talks to the Vercel origin.
3. **SPA fallback** `/*` → `/index.html` for client routes such as `/marathon`.

Same-origin API calls mean no browser CORS for the proxied path. Leave
`VITE_API_BASE_URL` empty in Doppler so the client uses `/api/v1` (mirrors the
local Vite proxy in `vite.config.ts`).

If the Render hostname changes, update the rewrite `destination` in
`vercel.json` — rewrites cannot read env vars.

## Environment variables on Vercel

| Key | Environments | Notes |
| --- | --- | --- |
| `DOPPLER_TOKEN` | Production, Preview | Service token for `codegym` / `prd_frontend` only. |

All browser config lives in Doppler `prd_frontend` (and preview can share the
same token/config unless you add a separate `stg_frontend` token later):

| Doppler key | Purpose |
| --- | --- |
| `VITE_AUTH0_DOMAIN` | Auth0 SPA domain |
| `VITE_AUTH0_CLIENT_ID` | Auth0 SPA client id (public) |
| `VITE_AUTH0_AUDIENCE` | API audience (must match backend) |
| `VITE_API_BASE_URL` | Leave empty for rewrite proxy |
| `VITE_APP_ORIGIN` | Canonical frontend origin (production Vercel URL) |

Auth0 login mounts when `VITE_AUTH0_DOMAIN` and `VITE_AUTH0_CLIENT_ID` are both
set. Add the Vercel production and preview URLs to the Auth0 app's allowed
callback, logout, and web-origin lists.

Without Auth0 values, the app can fall back to a localStorage bearer token for
manual testing:

```js
localStorage.setItem('codegym_token', '<token that the backend accepts>')
```

Do **not** set `VITE_USE_MOCK_API` on Vercel — mocks are gated on
`import.meta.env.DEV` and never run in a production build.

## Rotate `DOPPLER_TOKEN`

1. Create a new service token in Doppler (`prd_frontend` → Access → Service Tokens).
2. Update `DOPPLER_TOKEN` on Vercel (Production and Preview).
3. Redeploy so the next build pulls secrets with the new token.
4. Revoke the old token in Doppler.

## Verify

1. Open the production alias (e.g. `https://code-gym-rho.vercel.app`).
2. Confirm the shell loads (200 HTML).
3. In DevTools Network, a call to `/api/v1/...` should proxy to Render
   (not a Vercel 404). Unauthenticated routes may return 401 from the API —
   that still proves the rewrite works.
4. Quick check:

```sh
curl -sS -o /dev/null -w "%{http_code}\n" https://code-gym-rho.vercel.app/
curl -sS -o /dev/null -w "%{http_code}\n" https://code-gym-rho.vercel.app/api/v1/memory/profile
# expect 401 (or 200 with a token), not 404
```
