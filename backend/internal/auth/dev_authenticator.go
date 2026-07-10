// # Bearer Token Model
//
// HTTP requests are authenticated via bearer tokens in the Authorization header:
//
//	Authorization: Bearer <token>
//
// The middleware extracts this token, passes it to an Authenticator, and receives
// back a Principal (authenticated user + workspace access scope). If validation fails,
// the request is rejected (401).
//
// # Development Token Format
//
// DevAuthenticator supports two token modes:
//
//  1. Static token (exact match):
//     If CODEGYM_DEV_AUTH_TOKEN is set, the backend accepts only that exact token.
//     Maps to CODEGYM_DEV_USER_ID and CODEGYM_DEV_WORKSPACE_ID.
//     Use this for simple testing with a fixed identity.
//
//  2. Dev format (flexible):
//     Without CODEGYM_DEV_AUTH_TOKEN, tokens of the form dev:<user-id>[:<workspace-id>]
//     are accepted. Examples:
//     - Bearer dev:kevin → user=kevin, workspace=personal-dev (default)
//     - Bearer dev:alice:team-math → user=alice, workspace=team-math
//     - Bearer dev:bob:personal-bob → user=bob, workspace=personal-bob
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
// (env: CODEGYM_DEV_AUTH_TOKEN). Otherwise, accepts dev:<user>:<workspace> format.
//
// DefaultUserID: User assigned when token is empty or lacks a user component
// (env: CODEGYM_DEV_USER_ID, default: "dev-user").
//
// DefaultWorkspaceID: Workspace assigned when token is empty or lacks a workspace component
// (env: CODEGYM_DEV_WORKSPACE_ID, default: "personal-dev").
type DevAuthenticatorConfig struct {
	StaticToken     string
	DefaultUserID   string
	DefaultWorkspaceID string
}

// DevAuthenticator implements local development authentication semantics.
// It parses bearer tokens and returns a Principal with the user/workspace pair.
type DevAuthenticator struct {
	staticToken     string
	defaultUserID   string
	defaultWorkspaceID string
}

// NewDevAuthenticator creates a dev authenticator with fallback defaults.
// If StaticToken is set, only that exact token is accepted (env: CODEGYM_DEV_AUTH_TOKEN).
// Otherwise, tokens must match dev:<user-id>[:<workspace-id>] format.
func NewDevAuthenticator(config DevAuthenticatorConfig) *DevAuthenticator {
	defaultUserID := config.DefaultUserID
	if defaultUserID == "" {
		defaultUserID = "dev-user"
	}

	defaultWorkspaceID := config.DefaultWorkspaceID
	if defaultWorkspaceID == "" {
		defaultWorkspaceID = "personal-dev"
	}

	return &DevAuthenticator{
		staticToken:     config.StaticToken,
		defaultUserID:   defaultUserID,
		defaultWorkspaceID: defaultWorkspaceID,
	}
}

/*
Authenticate validates a bearer token and returns the authenticated Principal.

If DevAuthenticator was configured with StaticToken, the token must match that exact value; otherwise returns ErrUnauthenticated.

Otherwise, the token must match the dev format: dev:<user-id>[:<workspace-id>]

If the format is invalid or token is empty, returns ErrUnauthenticated.
On success, returns a Principal with UserID and WorkspaceIDs populated.
*/
func (a *DevAuthenticator) Authenticate(_ context.Context, bearerToken string) (Principal, error) {
	token := strings.TrimSpace(bearerToken)
	if token == "" {
		return Principal{}, ErrUnauthenticated
	}

	userID := a.defaultUserID
	workspaceID := a.defaultWorkspaceID

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
			workspaceID = parts[1]
		}
	} else {
		return Principal{}, ErrUnauthenticated
	}

	return Principal{
		UserID:          userID,
		DefaultWorkspaceID: workspaceID,
		WorkspaceIDs:       []string{workspaceID},
	}, nil
}
