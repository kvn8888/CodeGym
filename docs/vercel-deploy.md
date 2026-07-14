# Deploying the frontend to Vercel

The React/Vite frontend deploys to Vercel and reaches the Go backend
(on Render) through a **rewrite proxy** defined in
[`frontend/vercel.json`](../frontend/vercel.json).

## How the connection works

`vercel.json` rewrites `/api/*` to the Render backend:

```json
{ "source": "/api/:path*", "destination": "https://codegym.onrender.com/api/:path*" }
```

So the browser only ever calls the Vercel origin (`/api/v1/...`), and Vercel
forwards those requests to Render **server-side**. Two consequences:

- **No CORS.** The browser request is same-origin; Vercel's fetch to Render is
  server-to-server and never triggers a browser preflight. You do not need the
  Vercel domain in the backend's `CODEGYM_CORS_ALLOWED_ORIGINS` for this path
  (it's still used for direct/local cross-origin calls).
- **`VITE_API_BASE_URL` can stay unset.** The default `/api/v1` resolves
  same-origin and hits the rewrite. This mirrors the local `vite dev` proxy in
  `vite.config.ts`.

The second rewrite (`/:path*` -> `/index.html`) is the SPA fallback so deep
links like `/marathon` load the app instead of 404ing. Real build assets under
`/assets/*` are served from the filesystem before rewrites apply.

If the backend host ever changes, update the `destination` in `vercel.json` —
it's the single source of truth (Vercel rewrites can't read env vars).

## Environment variables

### Interim (no Auth0)

None are required to connect. The frontend falls back to a localStorage bearer
token; set it once in the browser console to match the backend dev token:

```js
localStorage.setItem('codegym_token', '<the CODEGYM_DEV_AUTH_TOKEN you set on Render>')
```

### Full Auth0 (when issue #21 lands)

| Variable | Value |
| --- | --- |
| `VITE_AUTH0_DOMAIN` | `dev-qpevrkauua3p7j6l.us.auth0.com` |
| `VITE_AUTH0_CLIENT_ID` | `Z4LdZf8STLjtuUvkrdgrtGvwzwQlJbBZ` |
| `VITE_AUTH0_AUDIENCE` | the Auth0 API Identifier |

The Auth0 provider only mounts when `DOMAIN` and `CLIENT_ID` are both set;
that's the switch from localStorage-token to real login. Add the Vercel
production/preview URLs to the Auth0 app's allowed callback/logout/web-origin
lists before testing hosted login.

Do **not** set `VITE_USE_MOCK_API` on Vercel — mocks are gated on
`import.meta.env.DEV` and never run in a production build.

## Verify

1. Open the Vercel URL, start an MCQ marathon.
2. In devtools Network, requests go to `<vercel-domain>/api/v1/generate` and
   return 200 (not a CORS error, not a 404 from Vercel).
3. If generation is unconfigured on Render (no `CODEGYM_GENAI_API_KEY`), the
   marathon shows the built-in "practice set" badge instead of failing.
