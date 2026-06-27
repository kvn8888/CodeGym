// # Bearer Token Model
//
// HTTP requests are authenticated via bearer tokens in the Authorization header:
//
//	Authorization: Bearer <token>
//
// The middleware extracts this token, passes it to an Authenticator, and receives
// back a Principal (authenticated user + tenant access scope). If validation fails,
// the request is rejected (401).
//
// # Development Token Format
//
// DevAuthenticator supports two token modes:
//
//  1. Static token (exact match):
//     If CODEGYM_DEV_AUTH_TOKEN is set, the backend accepts only that exact token.
//     Maps to CODEGYM_DEV_USER_ID and CODEGYM_DEV_TENANT_ID.
//     Use this for simple testing with a fixed identity.
//
//  2. Dev format (flexible):
//     Without CODEGYM_DEV_AUTH_TOKEN, tokens of the form dev:<user-id>[:<tenant-id>]
//     are accepted. Examples:
//     - Bearer dev:kevin → user=kevin, tenant=personal-dev (default)
//     - Bearer dev:alice:team-math → user=alice, tenant=team-math
//     - Bearer dev:bob:personal-bob → user=bob, tenant=personal-bob
//
// The dev format enables rapid testing of different user/workspace combinations
// without restarting the backend. In production, replace DevAuthenticator with
// a real provider (JWT, OAuth2, etc.).
package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ErrUnauthenticated indicates bearer token validation failed.
var ErrUnauthenticated = errors.New("unauthenticated")

// Authenticator validates bearer tokens and returns an authenticated Principal.
type Authenticator interface {
	Authenticate(ctx context.Context, bearerToken string) (Principal, error)
}

// DevAuthenticatorConfig configures development token validation behavior.
//
// StaticToken: If set, the backend only accepts this exact token
// (env: CODEGYM_DEV_AUTH_TOKEN). Otherwise, accepts dev:<user>:<tenant> format.
//
// DefaultUserID: User assigned when token is empty or lacks a user component
// (env: CODEGYM_DEV_USER_ID, default: "dev-user").
//
// DefaultTenantID: Workspace assigned when token is empty or lacks a tenant component
// (env: CODEGYM_DEV_TENANT_ID, default: "personal-dev").
type DevAuthenticatorConfig struct {
	StaticToken     string
	DefaultUserID   string
	DefaultTenantID string
}

// DevAuthenticator implements local development authentication semantics.
// It parses bearer tokens and returns a Principal with the user/tenant pair.
type DevAuthenticator struct {
	staticToken     string
	defaultUserID   string
	defaultTenantID string
}

// NewDevAuthenticator creates a dev authenticator with fallback defaults.
// If StaticToken is set, only that exact token is accepted (env: CODEGYM_DEV_AUTH_TOKEN).
// Otherwise, tokens must match dev:<user-id>[:<tenant-id>] format.
func NewDevAuthenticator(config DevAuthenticatorConfig) *DevAuthenticator {
	defaultUserID := config.DefaultUserID
	if defaultUserID == "" {
		defaultUserID = "dev-user"
	}

	defaultTenantID := config.DefaultTenantID
	if defaultTenantID == "" {
		defaultTenantID = "personal-dev"
	}

	return &DevAuthenticator{
		staticToken:     config.StaticToken,
		defaultUserID:   defaultUserID,
		defaultTenantID: defaultTenantID,
	}
}

/*
Authenticate validates a bearer token and returns the authenticated Principal.

If DevAuthenticator was configured with StaticToken, the token must match that exact value; otherwise returns ErrUnauthenticated.

Otherwise, the token must match the dev format: dev:<user-id>[:<tenant-id>]

If the format is invalid or token is empty, returns ErrUnauthenticated.
On success, returns a Principal with UserID and TenantIDs populated.
*/
func (a *DevAuthenticator) Authenticate(_ context.Context, bearerToken string) (Principal, error) {
	token := strings.TrimSpace(bearerToken)
	if token == "" {
		return Principal{}, ErrUnauthenticated
	}

	userID := a.defaultUserID
	tenantID := a.defaultTenantID

	if a.staticToken != "" {
		if token != a.staticToken {
			return Principal{}, ErrUnauthenticated
		}
	} else if strings.HasPrefix(token, "dev:") {
		parts := strings.Split(strings.TrimPrefix(token, "dev:"), ":")
		if len(parts) > 2 || parts[0] == "" {
			return Principal{}, fmt.Errorf("%w: invalid dev token shape", ErrUnauthenticated)
		}
		userID = parts[0]
		if len(parts) == 2 && parts[1] != "" {
			tenantID = parts[1]
		}
	} else {
		return Principal{}, ErrUnauthenticated
	}

	return Principal{
		UserID:          userID,
		DefaultTenantID: tenantID,
		TenantIDs:       []string{tenantID},
	}, nil
}
