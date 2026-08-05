# Auth0 Tokens, Audience, and CodeGym API Access

This guide explains why a user can be signed in through Auth0 while CodeGym's
protected API still rejects requests with `Invalid bearer token`.

## Authentication and API authorization are separate

Signing in answers one question:

```text
Who is this user?
```

Calling a protected backend answers another:

```text
May this application call this API on behalf of that user?
```

Auth0 represents those answers with different tokens. A successful login does
not, by itself, prove that the browser has an access token intended for the
CodeGym backend.

## ID tokens and access tokens

An **ID token** describes the authenticated session to the frontend. It can
contain identity claims such as the user's name, email, and Auth0 subject.
CodeGym can therefore show the signed-in account even when backend calls fail.

An **access token** authorizes calls to an API. The frontend sends it in the
request header:

```http
Authorization: Bearer <access-token>
```

CodeGym's Go backend accepts access tokens, not ID tokens, on protected routes.

## The audience claim

A JWT access token contains claims similar to:

```json
{
  "iss": "https://dev-qpevrkauua3p7j6l.us.auth0.com/",
  "sub": "auth0|user-id",
  "aud": "https://codegym.onrender.com",
  "exp": 1780000000
}
```

The important claims are:

| Claim | Meaning |
| --- | --- |
| `iss` | The Auth0 tenant that issued the token. |
| `sub` | The authenticated user identity. CodeGym uses this as the durable user ID. |
| `aud` | The API for which the token was issued. |
| `exp` | The time after which the token is no longer valid. |

The audience is comparable to the destination printed on a ticket. A real,
properly signed, unexpired ticket can still be rejected when it is presented at
the wrong destination.

The current CodeGym API Identifier and expected audience are:

```text
https://codegym.onrender.com
```

It does not have to be a routable URL in Auth0's general model, but it must be a
stable identifier and the frontend and backend must use exactly the same value.

## Why the Management API audience is wrong

Every Auth0 tenant includes an Auth0 Management API with an audience like:

```text
https://dev-qpevrkauua3p7j6l.us.auth0.com/api/v2/
```

That API manages Auth0 resources such as users, applications, grants, and tenant
settings. It is Auth0's administrative API, not the CodeGym Go API.

If the SPA requests a token for the Management API and sends it to CodeGym, the
token may still have a valid Auth0 signature, issuer, user, and expiration. The
CodeGym backend rejects it because its `aud` claim names a different API.

Conceptually, the failure looks like this:

```text
Auth0 login                     successful
User identity                   valid
Token signature and issuer      valid
Token destination               wrong
CodeGym API result              401 Invalid bearer token
```

## What the backend validates

Before a request reaches CodeGym's identity and workspace middleware, the Go
authenticator checks:

1. The token is a JWT signed with Auth0's RS256 key.
2. The signing key is present in the tenant's JWKS.
3. The issuer matches the configured Auth0 tenant.
4. The audience matches `CODEGYM_AUTH0_AUDIENCE` exactly.
5. The token is not expired and is not used before its valid time.

Only after these checks pass does Auth0 `sub` become the CodeGym user ID and
personal workspace scope.

## What the SPA requests

The frontend Auth0 SDK uses these build-time values:

```text
VITE_AUTH0_DOMAIN
VITE_AUTH0_CLIENT_ID
VITE_AUTH0_AUDIENCE
```

`VITE_AUTH0_AUDIENCE` tells Auth0 which API access token the SPA needs. It must
equal the backend's `CODEGYM_AUTH0_AUDIENCE`:

```text
Frontend requests: https://codegym.onrender.com
Backend accepts:   https://codegym.onrender.com
```

A login can still succeed when this value is absent or wrong because the ID
token flow can complete independently of CodeGym API authorization.

## User grants and machine grants

The CodeGym Auth0 API uses the `require_client_grant` access policy. Auth0 must
therefore know that the CodeGym SPA is allowed to request tokens for this API.

Client grants distinguish two subjects:

| Grant subject | Intended use |
| --- | --- |
| `subject_type: user` | A browser or application calls an API on behalf of a signed-in user. |
| `subject_type: client` | A server or automated process calls an API as itself with the client-credentials flow. |

CodeGym's SPA needs a user-delegated grant. A machine grant does not authorize
requests made on behalf of a signed-in user.

## Why this can appear suddenly

This problem is often hidden until an application starts enforcing a complete
custom-API flow. Earlier development may have used one or more of these paths:

- A local development bearer token instead of Auth0.
- Public routes that did not require an access token.
- Frontend screens that displayed only ID-token identity data.
- A backend that did not yet enforce an exact audience.
- An Auth0 API access policy that allowed applications without an explicit
  client grant.
- A cached token issued under an older configuration.

Stricter audience validation is desirable. It prevents a token issued for an
unrelated API from being replayed against CodeGym.

## Why each service must be refreshed

The audience is consumed at three different times:

| Layer | When it reads the value | Required refresh |
| --- | --- | --- |
| Render backend | When the Go process starts through Doppler `prd` | Restart or redeploy Render. |
| Vercel frontend | When Vite builds through Doppler `prd_frontend` | Rebuild and redeploy Vercel. |
| Browser session | When Auth0 issues and caches the access token | Log out, clear stale site data if needed, and log in again. |

Changing Doppler alone is therefore insufficient. Render must load the new
backend expectation, Vercel must compile the new frontend request, and the
browser must obtain a new token containing the new `aud` claim.

## Interpreting common responses

| Response | Meaning |
| --- | --- |
| `Missing bearer token` | The request reached the API, but the frontend did not attach a token. |
| `Invalid bearer token` | A token was attached, but signature, issuer, audience, time, or token format validation failed. |
| `200` with profile or cost data | Authentication, API authorization, identity bootstrap, and workspace scoping passed. |
| HTML from an `/api/` request | The frontend rewrite or routing layer returned the SPA instead of proxying to the API. |

## Safe verification

Use browser DevTools to inspect the `/api/v1/me` or `/api/v1/cost` request. If
you decode a JWT payload, do it locally and do not paste or share the complete
token. The expected claims are:

```text
iss = https://dev-qpevrkauua3p7j6l.us.auth0.com/
aud = https://codegym.onrender.com
sub = the signed-in Auth0 user ID
```

The unauthenticated proxy check should return a backend JSON response rather
than HTML:

```bash
curl -i https://code-gym-rho.vercel.app/api/v1/me
```

Without a token, the expected result is `401 Missing bearer token`. That result
proves the Vercel rewrite reached the Go API; it does not test a logged-in token.

## Related documentation

- [Backend Auth0 setup](../backend/README.md#auth0-api-audience-setup--required-for-settings--protected-apis)
- [Auth, identity, and workspace scope](./auth-identity-workspace.md)
- [Secrets and local environments](./secrets-and-local-env.md)
- [Render deployment](./render-deploy.md)
- [Vercel deployment](./vercel-deploy.md)
- [Auth0 application access and client grants](https://auth0.com/docs/get-started/applications/application-access-to-apis-client-grants)
