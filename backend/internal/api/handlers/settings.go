package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/kvn8888/codegym/backend/internal/api/response"
	"github.com/kvn8888/codegym/backend/internal/settings"
)

// SettingsHandler exposes workspace-scoped operator controls. There is no
// administrator role in CodeGym today, so the protected route can only read or
// write the authenticated caller's personal workspace.
type SettingsHandler struct {
	service *settings.Service
}

func NewSettingsHandler(service *settings.Service) *SettingsHandler {
	return &SettingsHandler{service: service}
}

func (h *SettingsHandler) Get(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.service == nil {
		response.Error(w, http.StatusServiceUnavailable, "settings_unavailable", "Runtime settings are not configured on this server.")
		return
	}
	setting, err := h.service.Get(r.Context(), r.PathValue("key"))
	if errors.Is(err, settings.ErrUnknownKey) {
		response.Error(w, http.StatusNotFound, "setting_not_found", "No registered runtime setting has that key.")
		return
	}
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "setting_get_failed", "Could not resolve runtime setting.")
		return
	}
	response.JSON(w, http.StatusOK, setting)
}

func (h *SettingsHandler) Put(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.service == nil {
		response.Error(w, http.StatusServiceUnavailable, "settings_unavailable", "Runtime settings are not configured on this server.")
		return
	}
	var input struct {
		Value json.RawMessage `json:"value"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid_json", "Request body must be a JSON object containing only value.")
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		response.Error(w, http.StatusBadRequest, "invalid_json", "Request body must contain exactly one JSON object.")
		return
	}
	setting, err := h.service.Set(r.Context(), r.PathValue("key"), input.Value)
	if errors.Is(err, settings.ErrUnknownKey) {
		response.Error(w, http.StatusNotFound, "setting_not_found", "No registered runtime setting has that key.")
		return
	}
	if errors.Is(err, settings.ErrInvalidValue) {
		response.Error(w, http.StatusBadRequest, "invalid_setting_value", err.Error())
		return
	}
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "setting_update_failed", "Could not update runtime setting.")
		return
	}
	response.JSON(w, http.StatusOK, setting)
}
