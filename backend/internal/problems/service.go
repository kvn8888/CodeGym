package problems

import (
	"context"
	"errors"
	"strings"
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
	return s.store.Upsert(ctx, twoSumDefinition())
}

func (s *Service) List(ctx context.Context) ([]Summary, error) {
	if s == nil || s.store == nil {
		return nil, errors.New("problem store is not configured")
	}
	return s.store.List(ctx)
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
	return s.store.Get(ctx, strings.TrimSpace(id))
}
