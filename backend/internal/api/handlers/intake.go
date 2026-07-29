package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/kvn8888/codegym/backend/internal/api/response"
	"github.com/kvn8888/codegym/backend/internal/intake"
)

type IntakeHandler struct{ service *intake.Service }

func NewIntakeHandler(service *intake.Service) *IntakeHandler {
	return &IntakeHandler{service: service}
}

func (h *IntakeHandler) Prepare(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		response.Error(w, http.StatusServiceUnavailable, "intake_unconfigured", "Practice intake is not configured on this server.")
		return
	}
	var input intake.PrepareInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON.")
		return
	}
	record, err := h.service.Prepare(r.Context(), input)
	if err != nil {
		if strings.Contains(err.Error(), "topic") {
			response.Error(w, http.StatusBadRequest, "invalid_intake", err.Error())
			return
		}
		response.Error(w, http.StatusInternalServerError, "intake_failed", "Could not prepare the topic baseline.")
		return
	}
	response.JSON(w, http.StatusOK, record)
}

func (h *IntakeHandler) List(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		response.Error(w, http.StatusServiceUnavailable, "intake_unconfigured", "Practice intake is not configured on this server.")
		return
	}
	if status := r.URL.Query().Get("status"); status != "" && status != string(intake.StatusPending) {
		response.Error(w, http.StatusBadRequest, "invalid_status", "Only status=pending is supported.")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	records, err := h.service.ListPending(r.Context(), limit)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "intake_list_failed", "Could not load pending baselines.")
		return
	}
	response.JSON(w, http.StatusOK, records)
}

func (h *IntakeHandler) Update(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		response.Error(w, http.StatusServiceUnavailable, "intake_unconfigured", "Practice intake is not configured on this server.")
		return
	}
	var input intake.UpdateInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON.")
		return
	}
	record, err := h.service.Update(r.Context(), r.PathValue("id"), input)
	if errors.Is(err, intake.ErrNotFound) {
		response.Error(w, http.StatusNotFound, "intake_not_found", "Practice intake not found.")
		return
	}
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid_intake_update", err.Error())
		return
	}
	response.JSON(w, http.StatusOK, record)
}
