package memory

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kvn8888/codegym/backend/internal/auth"
	"github.com/kvn8888/codegym/backend/internal/tenant"
)

type Clock func() time.Time

type Service struct {
	store Store
	now   Clock
}

func NewService(store Store, clock Clock) *Service {
	if clock == nil {
		clock = time.Now
	}
	return &Service{store: store, now: clock}
}

func (s *Service) GetProfile(ctx context.Context) (Profile, error) {
	identity, err := identityFromContext(ctx)
	if err != nil {
		return Profile{}, err
	}

	profile, err := s.store.GetProfile(ctx, identity.tenantID, identity.userID)
	if errors.Is(err, ErrProfileNotFound) {
		return s.defaultProfile(), nil
	}
	return profile, err
}

func (s *Service) RecordEvent(ctx context.Context, input RecordEventInput) (Event, error) {
	identity, err := identityFromContext(ctx)
	if err != nil {
		return Event{}, err
	}

	source := strings.TrimSpace(input.Source)
	eventType := strings.TrimSpace(input.Type)
	if source == "" || eventType == "" {
		return Event{}, errors.New("memory event source and type are required")
	}

	now := s.now().UTC()
	occurredAt := input.OccurredAt.UTC()
	if occurredAt.IsZero() {
		occurredAt = now
	}

	event := Event{
		ID:         newID("mem_evt"),
		TenantID:   identity.tenantID,
		UserID:     identity.userID,
		Source:     source,
		Type:       eventType,
		Summary:    strings.TrimSpace(input.Summary),
		Payload:    input.Payload,
		OccurredAt: occurredAt,
		CreatedAt:  now,
	}

	if err := s.store.AppendEvent(ctx, event); err != nil {
		return Event{}, err
	}
	return event, nil
}

func (s *Service) ListEvents(ctx context.Context) ([]Event, error) {
	identity, err := identityFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return s.store.ListEvents(ctx, identity.tenantID, identity.userID)
}

func (s *Service) defaultProfile() Profile {
	now := s.now().UTC()
	return Profile{
		Summary:      "No durable memory profile has been built yet. New chat, generation, marathon, and attempt events will update this profile once the summarizer worker is connected.",
		UpdatedAt:    now,
		NextReviewAt: now.Add(24 * time.Hour),
		Strengths:    []string{},
		GrowthEdges:  []string{},
		Skills:       []SkillProficiency{},
		Notes:        []Note{},
	}
}

type identity struct {
	tenantID string
	userID   string
}

func identityFromContext(ctx context.Context) (identity, error) {
	principal, ok := auth.PrincipalFromContext(ctx)
	if !ok || principal.UserID == "" {
		return identity{}, errors.New("missing authenticated user")
	}

	scope, ok := tenant.ScopeFromContext(ctx)
	if !ok || scope.TenantID == "" {
		return identity{}, errors.New("missing tenant scope")
	}

	return identity{tenantID: scope.TenantID, userID: principal.UserID}, nil
}

func newID(prefix string) string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(bytes[:])
}
