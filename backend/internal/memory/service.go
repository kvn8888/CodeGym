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
	"github.com/kvn8888/codegym/backend/internal/workspace"
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

	profile, err := s.store.GetProfile(ctx, identity.workspaceID, identity.userID)
	if errors.Is(err, ErrProfileNotFound) {
		return s.defaultProfile(), nil
	}
	if err != nil {
		return Profile{}, err
	}
	return normalizeProfileCollections(profile), nil
}

// ProfileInputs returns the current profile and its append-only evidence for a
// model-backed synthesis pass. The boolean reports whether the profile was
// already persisted, allowing callers to preserve a known-good profile when a
// provider is unavailable without persisting deterministic conclusions on a
// first refresh.
func (s *Service) ProfileInputs(ctx context.Context) (Profile, []Event, bool, error) {
	identity, err := identityFromContext(ctx)
	if err != nil {
		return Profile{}, nil, false, err
	}

	events, err := s.store.ListEvents(ctx, identity.workspaceID, identity.userID)
	if err != nil {
		return Profile{}, nil, false, err
	}
	profile, err := s.store.GetProfile(ctx, identity.workspaceID, identity.userID)
	if errors.Is(err, ErrProfileNotFound) {
		return s.defaultProfile(), events, false, nil
	}
	if err != nil {
		return Profile{}, nil, false, err
	}
	return normalizeProfileCollections(profile), events, true, nil
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
	if err := ValidateEventName(source, eventType); err != nil {
		return Event{}, err
	}

	now := s.now().UTC()
	occurredAt := input.OccurredAt.UTC()
	if occurredAt.IsZero() {
		occurredAt = now
	}

	event := Event{
		ID:          newID("mem_evt"),
		WorkspaceID: identity.workspaceID,
		UserID:      identity.userID,
		Source:      source,
		Type:        eventType,
		Summary:     strings.TrimSpace(input.Summary),
		Payload:     input.Payload,
		OccurredAt:  occurredAt,
		CreatedAt:   now,
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
	return s.RefreshProfileFor(ctx, id.workspaceID, id.userID)
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

	profile, err := s.store.GetProfile(ctx, id.workspaceID, id.userID)
	if errors.Is(err, ErrProfileNotFound) {
		profile = s.defaultProfile()
	} else if err != nil {
		return Profile{}, err
	}

	profile.Notes = notes
	profile = normalizeProfileCollections(profile)
	profile.UpdatedAt = s.now().UTC()
	if err := s.store.UpsertProfile(ctx, id.workspaceID, id.userID, profile); err != nil {
		return Profile{}, err
	}
	return profile, nil
}

// ReplaceProfile atomically persists a fully curated profile for the scoped
// user. Profile synthesis owns validation and server timestamps; this method
// only enforces scope and persistence.
func (s *Service) ReplaceProfile(ctx context.Context, profile Profile) (Profile, error) {
	id, err := identityFromContext(ctx)
	if err != nil {
		return Profile{}, err
	}
	profile = normalizeProfileCollections(profile)
	if err := s.store.UpsertProfile(ctx, id.workspaceID, id.userID, profile); err != nil {
		return Profile{}, err
	}
	return profile, nil
}

// RefreshProfileFor is the explicit-scope version of RefreshProfile. Use this
// outside HTTP request handling — workers, smoke tests, or GenAI orchestration
// code should not have to fabricate auth/workspace middleware context just to
// refresh memory.
func (s *Service) RefreshProfileFor(ctx context.Context, workspaceID, userID string) (Profile, error) {
	if strings.TrimSpace(workspaceID) == "" || strings.TrimSpace(userID) == "" {
		return Profile{}, errors.New("memory refresh requires workspace id and user id")
	}

	events, err := s.store.ListEvents(ctx, workspaceID, userID)
	if err != nil {
		return Profile{}, err
	}

	// Load the current profile so Summarize can preserve fields it doesn't
	// recompute. A missing profile is fine — start from the default.
	current, err := s.store.GetProfile(ctx, workspaceID, userID)
	if errors.Is(err, ErrProfileNotFound) {
		current = s.defaultProfile()
	} else if err != nil {
		return Profile{}, err
	}

	next := normalizeProfileCollections(Summarize(normalizeProfileCollections(current), events, s.now()))
	if err := s.store.UpsertProfile(ctx, workspaceID, userID, next); err != nil {
		return Profile{}, err
	}
	return next, nil
}

// RefreshAllProfiles re-derives profiles for every workspace/user pair that has
// memory events. It is intentionally small: the worker owns scheduling, while
// this service owns the memory semantics.
func (s *Service) RefreshAllProfiles(ctx context.Context) (int, error) {
	scopes, err := s.ListEventScopes(ctx)
	if err != nil {
		return 0, err
	}
	for _, scope := range scopes {
		if _, err := s.RefreshProfileFor(ctx, scope.WorkspaceID, scope.UserID); err != nil {
			return 0, err
		}
	}
	return len(scopes), nil
}

// ListEventScopes returns every workspace/user pair with memory evidence. It is
// used by background synthesis workers, which do not have request middleware
// to establish a scope for them.
func (s *Service) ListEventScopes(ctx context.Context) ([]Scope, error) {
	return s.store.ListEventScopes(ctx)
}

func (s *Service) ListEvents(ctx context.Context) ([]Event, error) {
	identity, err := identityFromContext(ctx)
	if err != nil {
		return nil, err
	}
	events, err := s.store.ListEvents(ctx, identity.workspaceID, identity.userID)
	if err != nil {
		return nil, err
	}
	if events == nil {
		return []Event{}, nil
	}
	return events, nil
}

func normalizeProfileCollections(profile Profile) Profile {
	if profile.Strengths == nil {
		profile.Strengths = []string{}
	}
	if profile.GrowthEdges == nil {
		profile.GrowthEdges = []string{}
	}
	if profile.Skills == nil {
		profile.Skills = []SkillProficiency{}
	}
	if profile.Notes == nil {
		profile.Notes = []Note{}
	}
	for index := range profile.Notes {
		if profile.Notes[index].Tags == nil {
			profile.Notes[index].Tags = []string{}
		}
	}
	return profile
}

func (s *Service) defaultProfile() Profile {
	now := s.now().UTC()
	return Profile{
		Summary:      "No memory summary yet. Complete a practice set to build one from your learning activity.",
		UpdatedAt:    now,
		NextReviewAt: now.Add(24 * time.Hour),
		Strengths:    []string{},
		GrowthEdges:  []string{},
		Skills:       []SkillProficiency{},
		Notes:        []Note{},
	}
}

type identity struct {
	workspaceID string
	userID      string
}

func identityFromContext(ctx context.Context) (identity, error) {
	principal, ok := auth.PrincipalFromContext(ctx)
	if !ok || principal.UserID == "" {
		return identity{}, errors.New("missing authenticated user")
	}

	scope, ok := workspace.ScopeFromContext(ctx)
	if !ok || scope.WorkspaceID == "" {
		return identity{}, errors.New("missing workspace scope")
	}

	return identity{workspaceID: scope.WorkspaceID, userID: principal.UserID}, nil
}

func newID(prefix string) string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(bytes[:])
}
