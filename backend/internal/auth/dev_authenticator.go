package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var ErrUnauthenticated = errors.New("unauthenticated")

type Authenticator interface {
	Authenticate(ctx context.Context, bearerToken string) (Principal, error)
}

type DevAuthenticatorConfig struct {
	StaticToken     string
	DefaultUserID   string
	DefaultTenantID string
}

type DevAuthenticator struct {
	staticToken     string
	defaultUserID   string
	defaultTenantID string
}

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
