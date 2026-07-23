package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/kvn8888/codegym/backend/internal/api/response"
	"github.com/kvn8888/codegym/backend/internal/execution"
	"github.com/kvn8888/codegym/backend/internal/problems"
	"github.com/kvn8888/codegym/backend/internal/session"
	"github.com/kvn8888/codegym/backend/internal/submission"
)

type SubmissionHandler struct {
	service *submission.Service
}

func NewSubmissionHandler(service *submission.Service) *SubmissionHandler {
	return &SubmissionHandler{service: service}
}

func (h *SubmissionHandler) Submit(w http.ResponseWriter, r *http.Request) {
	var input submission.SubmitInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON.")
		return
	}

	accepted, err := h.service.Submit(r.Context(), input)
	switch {
	case errors.Is(err, problems.ErrNotFound):
		response.Error(w, http.StatusNotFound, "problem_not_found", "Problem not found.")
	case errors.Is(err, session.ErrNotFound):
		response.Error(w, http.StatusNotFound, "session_not_found", "Session not found.")
	case errors.Is(err, execution.ErrRunnerUnavailable):
		response.Error(w, http.StatusServiceUnavailable, "execution_unavailable", "Code execution is not configured on this server.")
	case errors.Is(err, submission.ErrInvalidInput):
		response.Error(w, http.StatusBadRequest, "invalid_submission", err.Error())
	case err != nil:
		response.Error(w, http.StatusInternalServerError, "submission_failed", "Submission could not be started.")
	default:
		response.JSON(w, http.StatusAccepted, accepted)
	}
}

func (h *SubmissionHandler) Get(w http.ResponseWriter, r *http.Request) {
	found, err := h.service.Get(r.Context(), r.PathValue("id"))
	if errors.Is(err, execution.ErrRunNotFound) {
		response.Error(w, http.StatusNotFound, "submission_not_found", "Submission not found.")
		return
	}
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "submission_get_failed", "Submission could not be loaded.")
		return
	}
	response.JSON(w, http.StatusOK, found)
}
