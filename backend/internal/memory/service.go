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

// Service orchestrates memory profile and event behavior for scoped users.
type Service struct {
	store Store
	now   Clock
}

// NewService creates a memory service with a store and optional clock.
func NewService(store Store, clock Clock) *Service {
	if clock == nil {
		clock = time.Now
	}
	return &Service{store: store, now: clock}
}

// GetProfile returns the scoped user's profile or a generated default profile
// when no durable profile exists yet.
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

// RecordEvent validates and appends a scoped memory event.
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
	// NOTE: we intentionally do NOT summarize here — the request path stays fast.
	// Turning these append-only events into the derived Profile happens behind a
	// separate boundary: Service.RefreshProfile (synchronous) or memory.Worker
	// (async). See US-2/US-3 in docs/backend-m1-user-stories.md.
	return event, nil
}

// RefreshProfile re-derives this user's memory Profile from their event log and
// persists it. This is the isolated boundary where raw events become the summary
// that generation / chat / marathon read (issue #2, build step 4).
//
//	ListEvents (read evidence) -> Summarize (derive) -> UpsertProfile (save)
//
// It is safe to call repeatedly: Summarize is pure and idempotent, so calling
// this after a RecordEvent, on a timer (memory.Worker), or by hand all converge
// to the same profile for a given event log.
//
// Summarize owns the derivation rules. Keep this service focused on orchestration:
// read the event log, derive the next profile, then persist it.
func (s *Service) RefreshProfile(ctx context.Context) (Profile, error) {
	id, err := identityFromContext(ctx)
	if err != nil {
		return Profile{}, err
	}
	return s.RefreshProfileFor(ctx, id.tenantID, id.userID)
}

// ReplaceNotes persists a curated notes list on the scoped user's profile,
// preserving every other derived field. Notes are the LLM-maintained slice of
// memory (agentic CRUD after practice sessions); the deterministic Summarize
// keys existing notes by ID, so later refreshes preserve this curation.
func (s *Service) ReplaceNotes(ctx context.Context, notes []Note) (Profile, error) {
	id, err := identityFromContext(ctx)
	if err != nil {
		return Profile{}, err
	}

	profile, err := s.store.GetProfile(ctx, id.tenantID, id.userID)
	if errors.Is(err, ErrProfileNotFound) {
		profile = s.defaultProfile()
	} else if err != nil {
		return Profile{}, err
	}

	profile.Notes = notes
	profile.UpdatedAt = s.now().UTC()
	if err := s.store.UpsertProfile(ctx, id.tenantID, id.userID, profile); err != nil {
		return Profile{}, err
	}
	return profile, nil
}

// RefreshProfileFor is the explicit-scope version of RefreshProfile. Use this
// outside HTTP request handling — workers, smoke tests, or GenAI orchestration
// code should not have to fabricate auth/tenant middleware context just to
// refresh memory.
func (s *Service) RefreshProfileFor(ctx context.Context, tenantID, userID string) (Profile, error) {
	if strings.TrimSpace(tenantID) == "" || strings.TrimSpace(userID) == "" {
		return Profile{}, errors.New("memory refresh requires tenant id and user id")
	}

	events, err := s.store.ListEvents(ctx, tenantID, userID)
	if err != nil {
		return Profile{}, err
	}

	// Load the current profile so Summarize can preserve fields it doesn't
	// recompute. A missing profile is fine — start from the default.
	current, err := s.store.GetProfile(ctx, tenantID, userID)
	if errors.Is(err, ErrProfileNotFound) {
		current = s.defaultProfile()
	} else if err != nil {
		return Profile{}, err
	}

	next := Summarize(current, events, s.now())
	if err := s.store.UpsertProfile(ctx, tenantID, userID, next); err != nil {
		return Profile{}, err
	}
	return next, nil
}

// RefreshAllProfiles re-derives profiles for every tenant/user pair that has
// memory events. It is intentionally small: the worker owns scheduling, while
// this service owns the memory semantics.
func (s *Service) RefreshAllProfiles(ctx context.Context) (int, error) {
	scopes, err := s.store.ListEventScopes(ctx)
	if err != nil {
		return 0, err
	}
	for _, scope := range scopes {
		if _, err := s.RefreshProfileFor(ctx, scope.TenantID, scope.UserID); err != nil {
			return 0, err
		}
	}
	return len(scopes), nil
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
