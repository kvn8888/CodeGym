# Auth, Identity, and Workspace Scope

This document explains the current backend auth flow. The product model is
**multi-user personal workspaces with data isolation**, not business SaaS
organization tenancy.

Important split:

- `auth` identifies the caller for this request.
- `identity` ensures the caller has durable app identity rows.
- workspace scope chooses the caller's personal workspace for scoped data.

Implementation note: current package, header, and database names still use
`tenant` / `tenant_id`. Treat those as internal scope names for now. Do not
expand them into organization/team tenancy unless product scope changes.

## Request Pipeline

Protected routes under `/api/v1/` run through middleware in this order:

```text
Authorization header
  -> auth.Middleware
  -> identity.Middleware
  -> tenant.Middleware   # internal name for personal workspace scope
  -> API handler
```

`GET /health` is intentionally outside this protected chain.

## Package Responsibilities

### `backend/internal/auth`

Auth answers: who is making this request?

Current implementation:

- Reads `Authorization: Bearer <token>`.
- Uses the `Authenticator` interface.
- Uses `Auth0Authenticator` when Auth0 is configured.
- Uses `DevAuthenticator` for local fallback.
- Stores an `auth.Principal` in request context.

The principal shape still uses tenant field names internally:

```go
type Principal struct {
    UserID          string
    DefaultTenantID string
    TenantIDs       []string
    UserMetadata    UserMetadata
}
```

For product reasoning, read those fields as:

```text
UserID          -> authenticated CodeGym user
DefaultTenantID -> default personal workspace ID
TenantIDs       -> allowed workspace IDs, currently only the personal workspace
UserMetadata    -> optional Auth0 profile metadata for CodeGym's app user row
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
DefaultTenantID: personal-kevin
TenantIDs: [personal-kevin]
```

If `CODEGYM_DEV_AUTH_TOKEN` is set, the backend accepts only that exact bearer
token and maps it to `CODEGYM_DEV_USER_ID` and `CODEGYM_DEV_TENANT_ID`. The env
var name is internal legacy naming; the value is the local personal workspace
ID.

For Auth0-backed development or production, configure:

```bash
CODEGYM_AUTH_MODE=auth0
AUTH0_DOMAIN=your-auth0-domain.us.auth0.com
AUTH0_AUDIENCE=https://api.codegym.example
```

`CODEGYM_AUTH0_DOMAIN` / `CODEGYM_AUTH0_AUDIENCE` can be used instead of the
plain Auth0 names if a more explicit prefix is preferred. Auth0's own dashboard
uses the word "tenant" for its hosted identity domain; that is separate from
CodeGym's app data model.

The backend validates Auth0 access tokens with RS256 JWKS, issuer, audience,
expiry, and not-before checks before mapping claims into `auth.Principal`.
Auth0 `sub` becomes `Principal.UserID`, and CodeGym derives a deterministic
personal workspace ID from `sub`, such as `personal-auth0-user-123`.

Auth0 `email` and `name` are treated as optional profile metadata. The identity
layer stores `email` on `app_users.email` when present and stores `name` as
`app_users.display_name` when present. If a later access token omits those
optional claims, the bootstrap keeps the existing stored values instead of
blanking them. Neither field is used as a durable user key.

Do not add Auth0 app tenant/workspace claims, organization claims, tenant
switching, or team roles unless there is a real product requirement. Today,
every Auth0 user maps to one personal workspace scope.

### `backend/internal/identity`

Identity answers: does this authenticated caller exist in CodeGym's durable app
model?

The identity middleware runs after auth. It reads the `auth.Principal` from
context and calls:

```go
EnsurePersonalTenant(ctx, principal)
```

The function name is internal legacy naming. Product behavior is: bootstrap a
personal workspace for the authenticated user. With Postgres enabled, it creates
or updates:

```text
app_users
tenants              # internal table name for workspace records
tenant_memberships   # internal table name for user/workspace links
```

Using the dev example above, identity ensures:

```text
app_users.id = kevin
app_users.email = optional profile email when Auth0 supplies it
app_users.display_name = optional Auth0 name, otherwise the user id fallback
tenants.id = personal-kevin
tenant_memberships = kevin owner of personal-kevin
```

The operation is idempotent. Repeated requests for the same user/workspace
should not create duplicate rows.

When no database URL is configured, the backend uses an in-memory identity store
so local development still works.

### `backend/internal/tenant`

This package currently answers: which personal workspace scope is this request
allowed to operate on?

The middleware runs after identity. It chooses the workspace scope from:

1. `X-CodeGym-Tenant-ID`, if present.
2. `principal.DefaultTenantID`, if the header is absent.

The header name is internal legacy naming. Product flows should omit it and use
the authenticated user's default personal workspace. Keep the header for
low-level tests and future migration flexibility, not as a user-facing tenant
switcher.

It checks the selected scope against `principal.TenantIDs`. If the scope is not
allowed, the request fails with `403 tenant_forbidden`.

On success, it stores:

```go
tenant.Scope{TenantID: "..."}
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
tenants
tenant_memberships
```

Keeping `identity` separate means an auth provider only needs to produce an
`auth.Principal`. The identity layer can still bootstrap or reconcile the
CodeGym app user and personal workspace records.

## Database Behavior

The backend reads database configuration in this order:

1. `NEON_CONNECTION_STRING`
2. `DATABASE_URL`

If neither is present, it uses in-memory stores.

If a database URL is present, startup calls `EnsureSchema` for identity and
memory. This creates the current M1 tables with `CREATE TABLE IF NOT EXISTS`.
This is not a migration framework yet.

Identity tables:

```text
app_users
  id text primary key
  email text
  display_name text
  created_at timestamptz
  updated_at timestamptz

tenants
  id text primary key
  name text
  tenant_type text
  created_at timestamptz
  updated_at timestamptz

tenant_memberships
  tenant_id text references tenants(id)
  user_id text references app_users(id)
  role text
  created_at timestamptz
  updated_at timestamptz
  primary key (tenant_id, user_id)
```

The current bootstrap also enforces:

- `user_memory_profiles(tenant_id, user_id)` references
  `tenant_memberships(tenant_id, user_id)`.
- `memory_events(tenant_id, user_id)` references
  `tenant_memberships(tenant_id, user_id)`.

Those memory foreign keys mean a memory row cannot be written for a
user/workspace pair unless the identity bootstrap has created the membership
first.

The tables currently have `tenant_type` and `role` fields from an earlier
generalized design. Do not build organization/team behavior on top of them
without an explicit product decision.

## How Memory Uses This

Memory services require both:

- `auth.Principal.UserID`
- `tenant.Scope.TenantID` as the internal workspace scope ID

That means memory reads and writes are scoped to:

```text
workspace_id + user_id
```

Internally, current tables still spell that as:

```text
tenant_id + user_id
```

The memory store never guesses identity from the token directly. It only uses
the normalized context created by the auth and workspace-scope middleware.

With Postgres enabled, memory rows are tied back to durable user/workspace
membership:

```text
auth principal -> app_users
workspace scope -> tenants
principal + scope -> tenant_memberships
memory row -> tenant_memberships
```

## Current Limitations

- `DevAuthenticator` is only for local development.
- Workspace membership is currently represented in the principal, not loaded
  from Postgres on each request.
- Personal workspace bootstrap always assigns the internal `owner` role.
- Organization/team workspaces are not in product scope.
- Schema bootstrap is idempotent but not versioned migrations.
- Internal package/table/field names still say tenant; rename only in a focused
  schema/code cleanup task.

## Auth Upgrade Path

Auth0 is now the selected production auth boundary. Remaining follow-up work:

1. Keep `identity.Middleware` in the pipeline.
2. Keep reconciling Auth0 `email` and `name` as optional app-user metadata.
3. Keep every authenticated user mapped to one personal workspace until product
   scope explicitly requires shared/team workspaces.

The middleware order should stay conceptually:

```text
auth -> identity -> personal workspace scope -> handler
```
