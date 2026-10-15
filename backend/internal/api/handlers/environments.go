package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/kvn8888/codegym/backend/internal/api/response"
	"github.com/kvn8888/codegym/backend/internal/auth"
	"github.com/kvn8888/codegym/backend/internal/envintegration"
	"github.com/kvn8888/codegym/backend/internal/workspace"
)

// EnvironmentsHandler fronts async environment requests: a learner names a
// technology plus objective and polls the operation until a practice session
// is ready or the failure is reported. Builds run for minutes, so the POST
// returns immediately and never holds the connection.
type EnvironmentsHandler struct {
	operations *envintegration.OperationStore
}

// NewEnvironmentsHandler builds the handler. A nil store answers 503 so an
// unconfigured server fails explicitly rather than accepting requests it
// cannot run.
func NewEnvironmentsHandler(operations *envintegration.OperationStore) *EnvironmentsHandler {
	return &EnvironmentsHandler{operations: operations}
}

func environmentIdentity(r *http.Request) (userID, workspaceID string, ok bool) {
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

type requestEnvironmentInput struct {
	Technology string `json:"technology"`
	Objective  string `json:"objective"`
}

type environmentOperationView struct {
	ID         string `json:"id"`
	Technology string `json:"technology"`
	Objective  string `json:"objective"`
	Status     string `json:"status"`
	Stage      string `json:"stage"`
	SessionID  string `json:"session_id,omitempty"`
	Error      string `json:"error,omitempty"`
}

func operationView(operation *envintegration.EnvironmentOperation) environmentOperationView {
	return environmentOperationView{
		ID:         operation.ID,
		Technology: operation.Technology,
		Objective:  operation.Objective,
		Status:     string(operation.Status),
		Stage:      operation.Stage,
		SessionID:  operation.SessionID,
		Error:      operation.Error,
	}
}

// Request starts an environment build and returns the operation immediately.
func (h *EnvironmentsHandler) Request(w http.ResponseWriter, r *http.Request) {
	if h.operations == nil {
		response.Error(w, http.StatusServiceUnavailable, "environments_unavailable", "Environment requests are not configured on this server.")
		return
	}
	userID, workspaceID, ok := environmentIdentity(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthenticated", "Missing authenticated principal or workspace scope.")
		return
	}
	var input requestEnvironmentInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON.")
		return
	}
	operation, err := h.operations.Start(userID, workspaceID, strings.TrimSpace(input.Technology), strings.TrimSpace(input.Objective))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	response.JSON(w, http.StatusAccepted, operationView(operation))
}

// Status reports one environment request. Unknown IDs and other owners'
// operations both answer 404 so identifiers cannot be probed across owners.
func (h *EnvironmentsHandler) Status(w http.ResponseWriter, r *http.Request) {
	if h.operations == nil {
		response.Error(w, http.StatusServiceUnavailable, "environments_unavailable", "Environment requests are not configured on this server.")
		return
	}
	userID, _, ok := environmentIdentity(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthenticated", "Missing authenticated principal or workspace scope.")
		return
	}
	operation, ok := h.operations.Get(r.PathValue("id"))
	if !ok || operation.Owner != userID {
		response.Error(w, http.StatusNotFound, "operation_not_found", "No such environment request.")
		return
	}
	response.JSON(w, http.StatusOK, operationView(operation))
}
