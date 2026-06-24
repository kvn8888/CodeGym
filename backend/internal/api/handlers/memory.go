package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/kvn8888/codegym/backend/internal/api/response"
	"github.com/kvn8888/codegym/backend/internal/memory"
)

type MemoryHandler struct {
	service *memory.Service
}

func NewMemoryHandler(service *memory.Service) *MemoryHandler {
	return &MemoryHandler{service: service}
}

func (h *MemoryHandler) Profile(w http.ResponseWriter, r *http.Request) {
	profile, err := h.service.GetProfile(r.Context())
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "memory_profile_failed", "Could not load memory profile.")
		return
	}

	response.JSON(w, http.StatusOK, profile)
}

func (h *MemoryHandler) RefreshProfile(w http.ResponseWriter, r *http.Request) {
	profile, err := h.service.RefreshProfile(r.Context())
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "memory_refresh_failed", "Could not refresh memory profile.")
		return
	}

	response.JSON(w, http.StatusOK, profile)
}

func (h *MemoryHandler) ListEvents(w http.ResponseWriter, r *http.Request) {
	events, err := h.service.ListEvents(r.Context())
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "memory_events_failed", "Could not load memory events.")
		return
	}

	response.JSON(w, http.StatusOK, events)
}

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
