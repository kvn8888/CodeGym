package identity

import (
	"context"
	"errors"
	"strings"

	"github.com/kvn8888/codegym/backend/internal/auth"
)

type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

func (s *Service) EnsurePersonalTenant(ctx context.Context, principal auth.Principal) error {
	if s == nil || s.store == nil {
		return nil
	}

	userID := strings.TrimSpace(principal.UserID)
	tenantID := strings.TrimSpace(principal.DefaultTenantID)
	if userID == "" {
		return errors.New("personal tenant bootstrap requires a user id")
	}
	if tenantID == "" {
		return errors.New("personal tenant bootstrap requires a default tenant id")
	}

	return s.store.EnsurePersonalTenant(ctx, PersonalTenant{
		UserID:   userID,
		TenantID: tenantID,
		Role:     "owner",
	})
}
