# Auth, Identity, and Tenant Scope

This document explains the current M1 backend auth flow. The important split is:

- `auth` identifies the caller for this request.
- `identity` ensures the caller has durable app identity rows.
- `tenant` chooses and enforces the active tenant scope.

## Request Pipeline

Protected routes under `/api/v1/` run through middleware in this order:

```text
Authorization header
  -> auth.Middleware
  -> identity.Middleware
  -> tenant.Middleware
  -> API handler
```

`GET /health` is intentionally outside this protected chain.

## Package Responsibilities

### `backend/internal/auth`

Auth answers: who is making this request?

Current implementation:

- Reads `Authorization: Bearer <token>`.
- Uses the `Authenticator` interface.
- Uses `DevAuthenticator` for M1 local/backend work.
- Stores an `auth.Principal` in request context.

The principal shape is:

```go
type Principal struct {
    UserID          string
    DefaultTenantID string
    TenantIDs       []string
}
```

For local development without a static token, use:

```http
Authorization: Bearer dev:<user-id>:<tenant-id>
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
token and maps it to `CODEGYM_DEV_USER_ID` and `CODEGYM_DEV_TENANT_ID`.

### `backend/internal/identity`

Identity answers: does this authenticated caller exist in CodeGym's durable app
model?

The identity middleware runs after auth. It reads the `auth.Principal` from
context and calls:

```go
EnsurePersonalTenant(ctx, principal)
```

For M1, this bootstraps a personal tenant for the authenticated user. With
Postgres enabled, it creates or updates:

```text
app_users
tenants
tenant_memberships
```

Using the dev example above, identity ensures:

```text
app_users.id = kevin
tenants.id = personal-kevin
tenant_memberships = kevin owner of personal-kevin
```

The operation is idempotent. Repeated requests for the same user/tenant should
not create duplicate rows.

When no database URL is configured, the backend uses an in-memory identity store
so local development still works.

### `backend/internal/tenant`

Tenant answers: which tenant is this request allowed to operate on?

The tenant middleware runs after identity. It chooses the tenant scope from:

1. `X-CodeGym-Tenant-ID`, if present.
2. `principal.DefaultTenantID`, if the header is absent.

It then checks the selected tenant against `principal.TenantIDs`. If the tenant
is not allowed, the request fails with `403 tenant_forbidden`.

On success, it stores:

```go
tenant.Scope{TenantID: "..."}
```

in request context. Handlers and services use this scope to read and write
tenant-scoped data.

## Why Identity Is Separate From Auth

Auth should be replaceable. Today it is a dev bearer token. Later it might be
Clerk, Auth0, WorkOS, Google OAuth, or another provider.

The rest of CodeGym should not need to know provider details. It should depend
on CodeGym's own durable identity model:

```text
app_users
tenants
tenant_memberships
```

Keeping `identity` separate means a future auth provider only needs to produce
an `auth.Principal`. The identity layer can still bootstrap or reconcile the
CodeGym app user and tenant records.

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

- `tenants.tenant_type` is `personal` or `team`.
- `tenant_memberships.role` is `owner`, `admin`, or `member`.
- `user_memory_profiles(tenant_id, user_id)` references
  `tenant_memberships(tenant_id, user_id)`.
- `memory_events(tenant_id, user_id)` references
  `tenant_memberships(tenant_id, user_id)`.

Those memory foreign keys mean a memory row cannot be written for a user/tenant
pair unless the identity bootstrap has created a membership first.

## How Memory Uses This

Memory services require both:

- `auth.Principal.UserID`
- `tenant.Scope.TenantID`

That means memory reads and writes are scoped to:

```text
tenant_id + user_id
```

The memory store never guesses identity from the token directly. It only uses
the normalized context created by the auth and tenant middleware.

With Postgres enabled, memory rows are tied back to durable tenant membership:

```text
auth principal -> app_users
tenant scope -> tenants
principal + scope -> tenant_memberships
memory row -> tenant_memberships
```

## Current Limitations

- `DevAuthenticator` is only for local/M1 development.
- Tenant membership is currently represented in the principal, not loaded from
Postgres on each request.
- Personal tenant bootstrap always assigns the `owner` role.
- There is no organization/team tenant model yet.
- Schema bootstrap is idempotent but not versioned migrations.

## Expected Real Auth Upgrade Path

When replacing dev auth:

1. Add a new implementation of `auth.Authenticator`.
2. Verify the provider token or session.
3. Map provider identity into `auth.Principal`.
4. Keep `identity.Middleware` in the pipeline.
5. Load allowed tenant IDs from durable memberships when multi-tenant orgs are
   introduced.

The middleware order should stay:

```text
auth -> identity -> tenant -> handler
```
