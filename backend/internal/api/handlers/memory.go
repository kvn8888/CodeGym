package handlers

import (
	"encoding/json"
	"net/http"
	"sort"

	"github.com/kvn8888/codegym/backend/internal/api/response"
	"github.com/kvn8888/codegym/backend/internal/generation"
	"github.com/kvn8888/codegym/backend/internal/memory"
)

// MemoryHandler exposes HTTP handlers for memory profile and event operations.
type MemoryHandler struct {
	service  *memory.Service
	profiles *generation.ProfileSynthesizer
}

// NewMemoryHandler builds a memory handler bound to a memory service.
func NewMemoryHandler(service *memory.Service, profiles *generation.ProfileSynthesizer) *MemoryHandler {
	return &MemoryHandler{service: service, profiles: profiles}
}

// Profile returns the scoped user's memory profile.
// Example fields: summary, updated_at, next_review_at, strengths, growth_edges, skills, notes.
func (h *MemoryHandler) Profile(w http.ResponseWriter, r *http.Request) {
	profile, err := h.service.GetProfile(r.Context())
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "memory_profile_failed", "Could not load memory profile.")
		return
	}

	response.JSON(w, http.StatusOK, profile)
}

// RefreshProfile rebuilds the scoped user's memory profile from their event stream.
func (h *MemoryHandler) RefreshProfile(w http.ResponseWriter, r *http.Request) {
	result, err := h.profiles.RefreshProfile(r.Context(), generation.ProfileRefreshInput{Trigger: "manual"})
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "memory_refresh_failed", "Could not refresh memory profile.")
		return
	}

	response.JSON(w, http.StatusOK, result.Profile)
}

// ListEvents returns the scoped user's memory event stream.
// Example fields: id, source, type, summary, payload, occurred_at, created_at.
func (h *MemoryHandler) ListEvents(w http.ResponseWriter, r *http.Request) {
	events, err := h.service.ListEvents(r.Context())
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "memory_events_failed", "Could not load memory events.")
		return
	}

	// Internal aggregation intentionally consumes oldest-first evidence. The
	// public activity feed returns a sorted copy so recent user actions appear
	// first without changing that service contract.
	events = append([]memory.Event(nil), events...)
	for index := range events {
		events[index].OccurredAt = events[index].OccurredAt.UTC()
		events[index].CreatedAt = events[index].CreatedAt.UTC()
	}
	sort.SliceStable(events, func(i, j int) bool {
		if !events[i].OccurredAt.Equal(events[j].OccurredAt) {
			return events[i].OccurredAt.After(events[j].OccurredAt)
		}
		if !events[i].CreatedAt.Equal(events[j].CreatedAt) {
			return events[i].CreatedAt.After(events[j].CreatedAt)
		}
		return events[i].ID > events[j].ID
	})
	response.JSON(w, http.StatusOK, events)
}

// RecordEvent validates and appends a memory event for the scoped user.
// Example input: source=generate, type=problem_attempted, summary=Tried DFS, payload={...}, occurred_at=2026-06-24T12:30:00Z.
func (h *MemoryHandler) RecordEvent(w http.ResponseWriter, r *http.Request) {
	var input memory.RecordEventInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON.")
		return
	}

	event, err := h.service.RecordEvent(r.Context(), input)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid_memory_event", err.Error())
		return
	}

	response.JSON(w, http.StatusCreated, event)
}
