package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/kvn8888/codegym/backend/internal/api/response"
	"github.com/kvn8888/codegym/backend/internal/execution"
)

type ExecutionHandler struct {
	service *execution.Service
}

func NewExecutionHandler(service *execution.Service) *ExecutionHandler {
	return &ExecutionHandler{service: service}
}

func (h *ExecutionHandler) Submit(w http.ResponseWriter, r *http.Request) {
	var input execution.SubmitRunInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON.")
		return
	}

	run, err := h.service.SubmitRun(r.Context(), input)
	if errors.Is(err, execution.ErrRunnerUnavailable) {
		response.Error(w, http.StatusServiceUnavailable, "execution_unavailable", "Code execution is not configured on this server.")
		return
	}
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid_execution_request", err.Error())
		return
	}

	response.JSON(w, http.StatusCreated, run)
}

func (h *ExecutionHandler) Get(w http.ResponseWriter, r *http.Request) {
	run, err := h.service.GetRun(r.Context(), r.PathValue("id"))
	if errors.Is(err, execution.ErrRunNotFound) {
		response.Error(w, http.StatusNotFound, "execution_not_found", "No execution run with that id.")
		return
	}
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "execution_get_failed", "Could not load execution run.")
		return
	}

	response.JSON(w, http.StatusOK, run)
}

func (h *ExecutionHandler) List(w http.ResponseWriter, r *http.Request) {
	runs, err := h.service.ListRuns(r.Context())
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "execution_list_failed", "Could not load execution runs.")
		return
	}

	response.JSON(w, http.StatusOK, runs)
}
