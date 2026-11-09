package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/kvn8888/codegym/backend/internal/api/response"
	"github.com/kvn8888/codegym/backend/internal/auth"
	"github.com/kvn8888/codegym/backend/internal/environmentregistry"
	"github.com/kvn8888/codegym/backend/internal/practice"
	"github.com/kvn8888/codegym/backend/internal/workspace"
)

// PracticeHandler serves the #191 browser practice flow: opening a sample
// verified exercise, editing allowlisted files, Run/Submit with safe
// feedback, and durable save/resume. Ownership always comes from the
// auth principal plus workspace scope; request bodies never supply owner
// or workspace fields.
type PracticeHandler struct {
	service *practice.Service
}

// NewPracticeHandler creates a practice handler. A nil service answers
// 503 so production without sample wiring fails explicitly, never silently.
func NewPracticeHandler(service *practice.Service) *PracticeHandler {
	return &PracticeHandler{service: service}
}

func practiceIdentity(r *http.Request) (userID, workspaceID string, ok bool) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok || strings.TrimSpace(principal.UserID) == "" {
		return "", "", false
	}
	scope, ok := workspace.ScopeFromContext(r.Context())
	if !ok || strings.TrimSpace(scope.WorkspaceID) == "" {
		return "", "", false
	}
	return principal.UserID, scope.WorkspaceID, true
}

type openPracticeInput struct {
	ArtifactID string `json:"artifact_id"`
	Version    string `json:"version"`
	Technology string `json:"technology"`
}

// Open creates a fresh browser session for an exact artifact-ID pin.
func (h *PracticeHandler) Open(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		response.Error(w, http.StatusServiceUnavailable, "practice_unavailable", "Browser practice is not configured on this server.")
		return
	}
	userID, workspaceID, ok := practiceIdentity(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthenticated", "Missing authenticated principal or workspace scope.")
		return
	}
	var input openPracticeInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON.")
		return
	}
	view, err := h.service.OpenBrowserSession(r.Context(), userID, workspaceID, practice.ArtifactRef{
		ArtifactID: strings.TrimSpace(input.ArtifactID),
		Technology: strings.TrimSpace(input.Technology),
		Version:    strings.TrimSpace(input.Version),
	})
	if err != nil {
		writePracticeError(w, r, err)
		return
	}
	response.JSON(w, http.StatusCreated, view)
}

// Get returns the learner-safe view of one session.
func (h *PracticeHandler) Get(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		response.Error(w, http.StatusServiceUnavailable, "practice_unavailable", "Browser practice is not configured on this server.")
		return
	}
	userID, workspaceID, ok := practiceIdentity(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthenticated", "Missing authenticated principal or workspace scope.")
		return
	}
	view, err := h.service.GetBrowserSession(r.PathValue("id"), userID, workspaceID)
	if err != nil {
		writePracticeError(w, r, err)
		return
	}
	response.JSON(w, http.StatusOK, view)
}

type writePracticeFilesInput struct {
	Files []struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	} `json:"files"`
}

// WriteFiles applies a bulk learner edit and returns the fresh view. The
// frontend must flush its current multi-file editor state through this
// endpoint before Run/Submit/Save so decisive operations never run stale
// content.
func (h *PracticeHandler) WriteFiles(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		response.Error(w, http.StatusServiceUnavailable, "practice_unavailable", "Browser practice is not configured on this server.")
		return
	}
	userID, workspaceID, ok := practiceIdentity(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthenticated", "Missing authenticated principal or workspace scope.")
		return
	}
	var input writePracticeFilesInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON.")
		return
	}
	files := make(map[string]string, len(input.Files))
	for _, file := range input.Files {
		files[file.Path] = file.Content
	}
	if err := h.service.WriteBrowserFiles(r.PathValue("id"), userID, workspaceID, files); err != nil {
		writePracticeError(w, r, err)
		return
	}
	view, err := h.service.GetBrowserSession(r.PathValue("id"), userID, workspaceID)
	if err != nil {
		writePracticeError(w, r, err)
		return
	}
	response.JSON(w, http.StatusOK, view)
}

// Run executes the manifest run (or test fallback) for one session.
func (h *PracticeHandler) Run(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		response.Error(w, http.StatusServiceUnavailable, "practice_unavailable", "Browser practice is not configured on this server.")
		return
	}
	userID, workspaceID, ok := practiceIdentity(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthenticated", "Missing authenticated principal or workspace scope.")
		return
	}
	// Workspace membership is enforced by resolving the session for this
	// owner/workspace before execution; the Run path re-resolves owner.
	// Verify workspace access explicitly so cross-workspace IDs fail closed.
	if _, err := h.service.GetBrowserSession(r.PathValue("id"), userID, workspaceID); err != nil {
		writePracticeError(w, r, err)
		return
	}
	result, err := h.service.Run(r.Context(), r.PathValue("id"), userID)
	if err != nil {
		writePracticeError(w, r, err)
		return
	}
	response.JSON(w, http.StatusOK, result)
}

// Submit executes build then test for one session.
func (h *PracticeHandler) Submit(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		response.Error(w, http.StatusServiceUnavailable, "practice_unavailable", "Browser practice is not configured on this server.")
		return
	}
	userID, workspaceID, ok := practiceIdentity(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthenticated", "Missing authenticated principal or workspace scope.")
		return
	}
	if _, err := h.service.GetBrowserSession(r.PathValue("id"), userID, workspaceID); err != nil {
		writePracticeError(w, r, err)
		return
	}
	result, err := h.service.Submit(r.Context(), r.PathValue("id"), userID)
	if err != nil {
		writePracticeError(w, r, err)
		return
	}
	response.JSON(w, http.StatusOK, result)
}

// Save persists the current overlay as a durable saved exercise and
// returns the public receipt. Snapshot/build references stay server-side
// for resume checks and are never projected.
func (h *PracticeHandler) Save(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		response.Error(w, http.StatusServiceUnavailable, "practice_unavailable", "Browser practice is not configured on this server.")
		return
	}
	userID, workspaceID, ok := practiceIdentity(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthenticated", "Missing authenticated principal or workspace scope.")
		return
	}
	saved, err := h.service.SaveBrowserExercise(r.Context(), r.PathValue("id"), userID, workspaceID)
	if err != nil {
		writePracticeError(w, r, err)
		return
	}
	response.JSON(w, http.StatusCreated, saved.Receipt())
}

// ListSaved lists durable saved exercises for the caller.
func (h *PracticeHandler) ListSaved(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		response.Error(w, http.StatusServiceUnavailable, "practice_unavailable", "Browser practice is not configured on this server.")
		return
	}
	userID, workspaceID, ok := practiceIdentity(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthenticated", "Missing authenticated principal or workspace scope.")
		return
	}
	summaries, err := h.service.ListBrowserSaved(r.Context(), userID, workspaceID)
	if err != nil {
		writePracticeError(w, r, err)
		return
	}
	if summaries == nil {
		summaries = []practice.SavedSummary{}
	}
	response.JSON(w, http.StatusOK, summaries)
}

// Resume opens a fresh session from one saved exercise.
func (h *PracticeHandler) Resume(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		response.Error(w, http.StatusServiceUnavailable, "practice_unavailable", "Browser practice is not configured on this server.")
		return
	}
	userID, workspaceID, ok := practiceIdentity(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthenticated", "Missing authenticated principal or workspace scope.")
		return
	}
	view, err := h.service.ResumeBrowserSaved(r.Context(), userID, workspaceID, r.PathValue("id"))
	if err != nil {
		writePracticeError(w, r, err)
		return
	}
	response.JSON(w, http.StatusCreated, view)
}

// writePracticeError maps service errors onto stable public outcomes.
// Messages are fixed templates without internal metadata (no snapshot or
// build references, file contents, hidden paths, or provider internals)
// and without echoing request internals. The full error is logged
// server-side. Missing, version-mismatched, and replaced-build outcomes
// stay explicit and distinct; platform faults surface as 200 results with
// a platform-fault category, never as error text.
func writePracticeError(w http.ResponseWriter, r *http.Request, err error) {
	message := err.Error()
	status := http.StatusBadRequest
	code := "practice_request_failed"
	safe := "The practice request could not be completed."
	switch {
	case errors.Is(err, practice.ErrSavedNotFound) || errors.Is(err, environmentregistry.ErrArtifactNotFound) ||
		strings.Contains(message, "practice session not found") ||
		strings.Contains(message, "practice saved exercise not found") ||
		strings.Contains(message, "snapshot") && strings.Contains(message, "not found") ||
		strings.Contains(message, "has no file"):
		status, code, safe = http.StatusNotFound, "practice_not_found",
			"The requested practice session or saved exercise was not found."
	case errors.Is(err, practice.ErrSavedOwnerMismatch) ||
		strings.Contains(message, "belongs to another owner") ||
		strings.Contains(message, "belongs to another workspace"):
		status, code, safe = http.StatusForbidden, "practice_forbidden",
			"You do not have access to this practice session."
	case errors.Is(err, practice.ErrBuildChanged):
		status, code, safe = http.StatusConflict, "practice_build_changed",
			"The saved exercise no longer matches the current verified build. Open a fresh session instead of resuming."
	case errors.Is(err, practice.ErrArtifactMismatch) ||
		strings.Contains(message, "never substituted") ||
		strings.Contains(message, "does not match") ||
		strings.Contains(message, "mismatch") ||
		strings.Contains(message, "not verified"):
		status, code, safe = http.StatusConflict, "practice_artifact_mismatch",
			"The requested artifact or version does not match the verified exercise; versions are never substituted."
	case errors.Is(err, practice.ErrSavedStoreMissing):
		status, code, safe = http.StatusServiceUnavailable, "practice_saved_unavailable",
			"Saved exercise storage is not configured."
	case strings.Contains(message, "requires an artifact_id pin"):
		status, code, safe = http.StatusBadRequest, "practice_invalid_request",
			"An artifact_id pin is required; technology and version alone can select another exercise."
	case strings.Contains(message, "is not learner-editable"):
		status, code, safe = http.StatusBadRequest, "practice_invalid_request",
			"The requested file is not learner-editable."
	case strings.Contains(message, "exceeds the"):
		status, code, safe = http.StatusBadRequest, "practice_invalid_request",
			"One file exceeds the backend file-size limit."
	case strings.Contains(message, "file path is required") ||
		strings.Contains(message, "requires an owner") ||
		strings.Contains(message, "requires a workspace") ||
		strings.Contains(message, "artifact source is not configured") ||
		strings.Contains(message, "content store is not configured") ||
		strings.Contains(message, "runner is not configured") ||
		strings.Contains(message, "has no exact identity") ||
		strings.Contains(message, "is not learner-editable in the live artifact"):
		status, code, safe = http.StatusBadRequest, "practice_invalid_request",
			"The practice request was invalid."
	}
	log.Printf("practice %s %s %s: %v", r.Method, r.URL.Path, code, err)
	response.Error(w, status, code, safe)
}
