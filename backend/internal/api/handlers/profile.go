package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/kvn8888/codegym/backend/internal/api/response"
	"github.com/kvn8888/codegym/backend/internal/auth"
	"github.com/kvn8888/codegym/backend/internal/identity"
)

// ProfileHandler exposes the authenticated user's editable account profile.
type ProfileHandler struct {
	service *identity.Service
}

// NewProfileHandler builds a profile handler bound to the identity service.
func NewProfileHandler(service *identity.Service) *ProfileHandler {
	return &ProfileHandler{service: service}
}

// Me returns the authenticated user's account profile.
func (h *ProfileHandler) Me(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", "Missing authenticated user.")
		return
	}

	profile, err := h.service.GetUserProfile(r.Context(), principal)
	if err != nil {
		writeProfileError(w, err)
		return
	}

	response.JSON(w, http.StatusOK, profile)
}

// UpdateMe updates user-editable account profile fields.
func (h *ProfileHandler) UpdateMe(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", "Missing authenticated user.")
		return
	}

	var input struct {
		DisplayName string `json:"display_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON.")
		return
	}

	profile, err := h.service.UpdateDisplayName(r.Context(), principal, input.DisplayName)
	if err != nil {
		writeProfileError(w, err)
		return
	}

	response.JSON(w, http.StatusOK, profile)
}

func writeProfileError(w http.ResponseWriter, err error) {
	if errors.Is(err, identity.ErrUserNotFound) {
		response.Error(w, http.StatusNotFound, "profile_not_found", "User profile was not found.")
		return
	}
	if errors.Is(err, identity.ErrInvalidProfile) {
		message := strings.TrimPrefix(err.Error(), identity.ErrInvalidProfile.Error()+": ")
		response.Error(w, http.StatusBadRequest, "invalid_profile", message)
		return
	}

	response.Error(w, http.StatusInternalServerError, "profile_failed", "Could not load user profile.")
}
