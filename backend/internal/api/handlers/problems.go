package handlers

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/kevinc/codegym/internal/api/response"
	"github.com/kevinc/codegym/internal/store/problemstore"
)

type ProblemsHandler struct {
	store *problemstore.Store
}

func NewProblemsHandler(store *problemstore.Store) *ProblemsHandler {
	return &ProblemsHandler{store: store}
}

// List returns problems matching optional query filters.
func (h *ProblemsHandler) List(w http.ResponseWriter, r *http.Request) {
	difficulty, _ := strconv.Atoi(r.URL.Query().Get("difficulty"))

	filters := problemstore.ProblemFilters{
		Language:   r.URL.Query().Get("language"),
		Category:   r.URL.Query().Get("category"),
		Difficulty: difficulty,
		Tag:        r.URL.Query().Get("tag"),
		Framework:  r.URL.Query().Get("framework"),
	}

	problems := h.store.List(filters)
	response.JSON(w, http.StatusOK, map[string]any{
		"problems": problems,
		"total":    len(problems),
	})
}

// Get returns a single problem by ID (without solution/test code).
func (h *ProblemsHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	prob, err := h.store.Get(id)
	if err != nil {
		response.Err(w, http.StatusNotFound, "not_found", err.Error())
		return
	}

	response.JSON(w, http.StatusOK, prob)
}

// GetSkeleton returns the skeleton files for a problem.
func (h *ProblemsHandler) GetSkeleton(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	files, err := h.store.GetSkeletonFiles(id)
	if err != nil {
		response.Err(w, http.StatusNotFound, "not_found", err.Error())
		return
	}

	response.JSON(w, http.StatusOK, map[string]any{
		"files": files,
	})
}
