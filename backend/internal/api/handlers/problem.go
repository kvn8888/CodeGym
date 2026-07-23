package handlers

import (
	"errors"
	"net/http"

	"github.com/kvn8888/codegym/backend/internal/api/response"
	"github.com/kvn8888/codegym/backend/internal/problems"
)

type ProblemHandler struct {
	service *problems.Service
}

func NewProblemHandler(service *problems.Service) *ProblemHandler {
	return &ProblemHandler{service: service}
}

func (h *ProblemHandler) List(w http.ResponseWriter, r *http.Request) {
	found, err := h.service.List(r.Context())
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "problem_list_failed", "Problems could not be loaded.")
		return
	}
	response.JSON(w, http.StatusOK, struct {
		Problems []problems.Summary `json:"problems"`
		Total    int                `json:"total"`
	}{Problems: found, Total: len(found)})
}

func (h *ProblemHandler) Get(w http.ResponseWriter, r *http.Request) {
	found, err := h.service.Get(r.Context(), r.PathValue("id"))
	if errors.Is(err, problems.ErrNotFound) {
		response.Error(w, http.StatusNotFound, "problem_not_found", "Problem not found.")
		return
	}
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "problem_get_failed", "Problem could not be loaded.")
		return
	}
	response.JSON(w, http.StatusOK, found)
}

func (h *ProblemHandler) Skeleton(w http.ResponseWriter, r *http.Request) {
	found, err := h.service.GetSkeleton(r.Context(), r.PathValue("id"))
	if errors.Is(err, problems.ErrNotFound) {
		response.Error(w, http.StatusNotFound, "problem_not_found", "Problem not found.")
		return
	}
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "problem_skeleton_failed", "Problem skeleton could not be loaded.")
		return
	}
	response.JSON(w, http.StatusOK, found)
}
