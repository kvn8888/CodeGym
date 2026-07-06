package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/kvn8888/codegym/backend/internal/api/response"
	"github.com/kvn8888/codegym/backend/internal/session"
)

type SessionHandler struct {
	service *session.Service
}

func NewSessionHandler(service *session.Service) *SessionHandler {
	return &SessionHandler{service: service}
}

func (h *SessionHandler) List(w http.ResponseWriter, r *http.Request) {
	filter, err := sessionFilterFromQuery(r)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid_session_filter", err.Error())
		return
	}

	sessions, err := h.service.List(r.Context(), filter)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "session_list_failed", err.Error())
		return
	}
	response.JSON(w, http.StatusOK, sessions)
}

func (h *SessionHandler) Create(w http.ResponseWriter, r *http.Request) {
	var input session.CreateInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON.")
		return
	}

	created, err := h.service.Create(r.Context(), input)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid_session", err.Error())
		return
	}
	response.JSON(w, http.StatusCreated, created)
}

func (h *SessionHandler) Get(w http.ResponseWriter, r *http.Request) {
	found, err := h.service.Get(r.Context(), r.PathValue("id"))
	if errors.Is(err, session.ErrNotFound) {
		response.Error(w, http.StatusNotFound, "session_not_found", "Session not found.")
		return
	}
	if err != nil {
		response.Error(w, http.StatusBadRequest, "session_get_failed", err.Error())
		return
	}
	response.JSON(w, http.StatusOK, found)
}

func (h *SessionHandler) Patch(w http.ResponseWriter, r *http.Request) {
	var input session.UpdateInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON.")
		return
	}

	updated, err := h.service.Update(r.Context(), r.PathValue("id"), input)
	if errors.Is(err, session.ErrNotFound) {
		response.Error(w, http.StatusNotFound, "session_not_found", "Session not found.")
		return
	}
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid_session_update", err.Error())
		return
	}
	response.JSON(w, http.StatusOK, updated)
}

func (h *SessionHandler) UpsertFiles(w http.ResponseWriter, r *http.Request) {
	var input session.UpsertFilesInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON.")
		return
	}

	updated, err := h.service.UpsertFiles(r.Context(), r.PathValue("id"), input)
	if errors.Is(err, session.ErrNotFound) {
		response.Error(w, http.StatusNotFound, "session_not_found", "Session not found.")
		return
	}
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid_session_files", err.Error())
		return
	}
	response.JSON(w, http.StatusOK, updated)
}

func sessionFilterFromQuery(r *http.Request) (session.ListFilter, error) {
	query := r.URL.Query()
	filter := session.ListFilter{
		Kind:   session.Kind(query.Get("kind")),
		Status: session.Status(query.Get("status")),
	}

	if limitValue := query.Get("limit"); limitValue != "" {
		limit, err := strconv.Atoi(limitValue)
		if err != nil {
			return session.ListFilter{}, err
		}
		filter.Limit = limit
	}
	if cursor := query.Get("cursor"); cursor != "" {
		before, err := time.Parse(time.RFC3339, cursor)
		if err != nil {
			return session.ListFilter{}, err
		}
		filter.Before = before
	}
	return filter, nil
}
