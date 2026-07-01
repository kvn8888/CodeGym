package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/kvn8888/codegym/backend/internal/api/response"
	"github.com/kvn8888/codegym/backend/internal/memory"
)

// MemoryHandler exposes HTTP handlers for memory profile and event operations.
type MemoryHandler struct {
	service *memory.Service
}

// NewMemoryHandler builds a memory handler bound to a memory service.
func NewMemoryHandler(service *memory.Service) *MemoryHandler {
	return &MemoryHandler{service: service}
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
	profile, err := h.service.RefreshProfile(r.Context())
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "memory_refresh_failed", "Could not refresh memory profile.")
		return
	}

	response.JSON(w, http.StatusOK, profile)
}

// ListEvents returns the scoped user's memory event stream.
// Example fields: id, source, type, summary, payload, occurred_at, created_at.
func (h *MemoryHandler) ListEvents(w http.ResponseWriter, r *http.Request) {
	events, err := h.service.ListEvents(r.Context())
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "memory_events_failed", "Could not load memory events.")
		return
	}

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
