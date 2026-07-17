# Auth, Identity, and Workspace Scope

This document explains the current backend auth flow. The product model is
**multi-user personal workspaces with data isolation**, not business SaaS
organization tenancy.

Important split:

- `auth` identifies the caller for this request.
- `identity` ensures the caller has durable app identity rows.
- `workspace` chooses the caller's personal workspace for scoped data.

CodeGym uses **workspace** naming in packages, APIs, and schema. Do not expand
that into organization/team multi-tenancy unless product scope changes.

Auth0's own dashboard still uses the word "tenant" for its hosted identity
domain; that is separate from CodeGym's app data model.

## Request Pipeline

Protected routes under `/api/v1/` run through middleware in this order:

```text
Authorization header
  -> auth.Middleware
  -> identity.Middleware
  -> workspace.Middleware
  -> API handler
```

`GET /health` and `GET /ready` are intentionally outside this protected chain.

## Package Responsibilities

### `backend/internal/auth`

Auth answers: who is making this request?

Current implementation:

- Reads `Authorization: Bearer <token>`.
- Uses the `Authenticator` interface.
- Uses `Auth0Authenticator` when Auth0 is configured.
- Uses `DevAuthenticator` for local fallback.
- Stores an `auth.Principal` in request context.

```go
type Principal struct {
    UserID             string
    DefaultWorkspaceID string
    WorkspaceIDs       []string
    UserMetadata       UserMetadata
}
```

```text
UserID             -> authenticated CodeGym user
DefaultWorkspaceID -> default personal workspace ID
WorkspaceIDs       -> allowed workspace IDs (currently only the personal workspace)
UserMetadata       -> optional Auth0 profile metadata for CodeGym's app user row
```

For local development without a static token, use:

```http
Authorization: Bearer dev:<user-id>:<workspace-id>
```

Example:

```http
Authorization: Bearer dev:kevin:personal-kevin
```

That creates a principal equivalent to:

```text
UserID: kevin
DefaultWorkspaceID: personal-kevin
WorkspaceIDs: [personal-kevin]
```

If `CODEGYM_DEV_AUTH_TOKEN` is set, the backend accepts only that exact bearer
token and maps it to `CODEGYM_DEV_USER_ID` and `CODEGYM_DEV_WORKSPACE_ID`
(legacy env alias: `CODEGYM_DEV_TENANT_ID`).

For Auth0-backed development or production, configure:

```bash
CODEGYM_AUTH_MODE=auth0
AUTH0_DOMAIN=your-auth0-domain.us.auth0.com
AUTH0_AUDIENCE=https://api.codegym.example
```

`CODEGYM_AUTH0_DOMAIN` / `CODEGYM_AUTH0_AUDIENCE` can be used instead of the
plain Auth0 names if a more explicit prefix is preferred.

The backend validates Auth0 access tokens with RS256 JWKS, issuer, audience,
expiry, and not-before checks before mapping claims into `auth.Principal`.
Auth0 `sub` becomes `Principal.UserID`, and CodeGym derives a deterministic
personal workspace ID from `sub`, such as `personal-auth0-user-123`.

Important Auth0 setup boundaries:

- The backend validates **access tokens** whose `aud` matches the configured
  Auth0 API Identifier. Do not send frontend ID tokens to the backend.
- The backend needs the Auth0 domain or issuer URL plus audience. It does not
  need an Auth0 client ID or client secret.
- The frontend SPA needs its Auth0 domain, SPA client ID, and the same audience
  so it can request backend API access tokens.
- `CODEGYM_AUTH0_DOMAIN` / `CODEGYM_AUTH0_AUDIENCE` are the preferred Doppler
  names. `AUTH0_DOMAIN` / `AUTH0_AUDIENCE` remain supported aliases.
- `CODEGYM_AUTH0_ISSUER_URL` is only for non-standard issuer setups. A normal
  Auth0 domain is converted to `https://<domain>/` automatically.

Auth0 `email` and `name` are treated as optional profile metadata. The identity
layer stores them on `app_users` when present. Neither field is used as a durable
user key.

Do not add Auth0 organization claims, workspace switching, or team roles unless
there is a real product requirement. Today every Auth0 user maps to one personal
workspace.

### `backend/internal/identity`

Identity answers: does this authenticated caller exist in CodeGym's durable app
model?

The identity middleware runs after auth. It reads the `auth.Principal` from
context and calls:

```go
EnsurePersonalWorkspace(ctx, principal)
```

With Postgres enabled, it creates or updates:

```text
app_users
workspaces
workspace_memberships
```

Using the dev example above, identity ensures:

```text
app_users.id = kevin
workspaces.id = personal-kevin
workspace_memberships = kevin owner of personal-kevin
```

The operation is idempotent. When no database URL is configured, the backend uses
an in-memory identity store so local development still works.

### `backend/internal/workspace`

This package answers: which personal workspace scope is this request allowed to
operate on?

The middleware runs after identity. It chooses the workspace scope from:

1. `X-CodeGym-Workspace-ID`, if present.
2. Legacy `X-CodeGym-Tenant-ID`, if present (compatibility alias).
3. `principal.DefaultWorkspaceID`, if no header is set.

Product flows should omit the header and use the authenticated user's default
personal workspace. Keep the header for low-level tests and future flexibility,
not as a user-facing workspace switcher.

It checks the selected scope against `principal.WorkspaceIDs`. If the scope is
not allowed, the request fails with `403 workspace_forbidden`.

On success, it stores:

```go
workspace.Scope{WorkspaceID: "..."}
```

in request context. Handlers and services use this scope to read and write
workspace-scoped data.

## Why Identity Is Separate From Auth

Auth should be replaceable. Today it can be Auth0 or a dev bearer token. Later
it might be another provider.

The rest of CodeGym should not need to know provider details. It should depend
on CodeGym's own durable identity model:

```text
app_users
workspaces
workspace_memberships
```

## Database Behavior

The backend reads database configuration in this order:

1. `NEON_CONNECTION_STRING`
2. `DATABASE_URL`

If neither is present, it uses in-memory stores.

If a database URL is present, startup calls `EnsureSchema` for identity, memory,
and sessions. This creates tables with `CREATE TABLE IF NOT EXISTS` and renames
legacy `tenant*` tables/columns when present. This is not a full migration
framework yet.

Identity tables:

```text
app_users
  id text primary key
  email text
  display_name text
  created_at timestamptz
  updated_at timestamptz

workspaces
  id text primary key
  name text
  workspace_type text
  created_at timestamptz
  updated_at timestamptz

workspace_memberships
  workspace_id text references workspaces(id)
  user_id text references app_users(id)
  role text
  created_at timestamptz
  updated_at timestamptz
  primary key (workspace_id, user_id)
```

Memory and session rows also key on `workspace_id + user_id` and reference
`workspace_memberships`.

The tables currently have `workspace_type` and `role` fields from an earlier
generalized design. Do not build organization/team behavior on top of them
without an explicit product decision.

## How Memory Uses This

Memory services require both:

- `auth.Principal.UserID`
- `workspace.Scope.WorkspaceID`

That means memory reads and writes are scoped to:

```text
workspace_id + user_id
```

## Current Limitations

- `DevAuthenticator` is only for local development.
- Workspace membership is currently represented in the principal, not loaded
  from Postgres on each request.
- Personal workspace bootstrap always assigns the internal `owner` role.
- Organization/team workspaces are not in product scope.
- Schema bootstrap is idempotent but not versioned migrations.

## Auth Upgrade Path

Auth0 is the selected production auth boundary. Remaining follow-up work:

1. Keep `identity.Middleware` in the pipeline.
2. Keep reconciling Auth0 `email` and `name` as optional app-user metadata.
3. Keep every authenticated user mapped to one personal workspace until product
   scope explicitly requires shared/team workspaces.

The middleware order should stay:

```text
auth -> identity -> personal workspace scope -> handler
```
