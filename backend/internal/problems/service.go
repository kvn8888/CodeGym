package problems

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kvn8888/codegym/backend/internal/auth"
	"github.com/kvn8888/codegym/backend/internal/workspace"
)

type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

func (s *Service) EnsureSeed(ctx context.Context) error {
	if s == nil || s.store == nil {
		return errors.New("problem store is not configured")
	}
	httpDefinition, err := goHTTPItemsDefinition()
	if err != nil {
		return err
	}
	for _, definition := range []Definition{twoSumDefinition(), httpDefinition} {
		if err := s.store.Upsert(ctx, definition); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) List(ctx context.Context) ([]Summary, error) {
	if s == nil || s.store == nil {
		return nil, errors.New("problem store is not configured")
	}
	workspaceID, userID := scopeFromContext(ctx)
	return s.store.List(ctx, workspaceID, userID)
}

func (s *Service) Get(ctx context.Context, id string) (Problem, error) {
	definition, err := s.GetDefinition(ctx, id)
	if err != nil {
		return Problem{}, err
	}
	return definition.Problem, nil
}

func (s *Service) GetSkeleton(ctx context.Context, id string) (Skeleton, error) {
	definition, err := s.GetDefinition(ctx, id)
	if err != nil {
		return Skeleton{}, err
	}
	return Skeleton{Files: append([]File(nil), definition.SkeletonFiles...)}, nil
}

// GetDefinition is intentionally server-side: submission assembly needs the
// hidden test files and entrypoint, while HTTP problem handlers use Get and
// GetSkeleton so those fields never enter a response.
func (s *Service) GetDefinition(ctx context.Context, id string) (Definition, error) {
	if s == nil || s.store == nil {
		return Definition{}, errors.New("problem store is not configured")
	}
	workspaceID, userID := scopeFromContext(ctx)
	return s.store.Get(ctx, strings.TrimSpace(id), workspaceID, userID)
}

// PersistGenerated assigns ownership and a non-guessable ID before saving a
// model-generated definition. Hidden tests remain server-only.
func (s *Service) PersistGenerated(ctx context.Context, definition Definition) (Problem, error) {
	workspaceID, userID := scopeFromContext(ctx)
	if workspaceID == "" || userID == "" {
		return Problem{}, errors.New("missing authenticated workspace scope")
	}
	if definition.ID == "" {
		definition.ID = newGeneratedID()
	}
	definition.Visibility = VisibilityWorkspace
	definition.WorkspaceID = workspaceID
	definition.UserID = userID
	if err := s.store.Upsert(ctx, definition); err != nil {
		return Problem{}, err
	}
	return definition.Problem, nil
}

func scopeFromContext(ctx context.Context) (string, string) {
	principal, principalOK := auth.PrincipalFromContext(ctx)
	scope, scopeOK := workspace.ScopeFromContext(ctx)
	if !principalOK || !scopeOK {
		return "", ""
	}
	return scope.WorkspaceID, principal.UserID
}

func newGeneratedID() string {
	var value [12]byte
	if _, err := rand.Read(value[:]); err != nil {
		return fmt.Sprintf("generated-%d", time.Now().UnixNano())
	}
	return "generated-" + hex.EncodeToString(value[:])
}
