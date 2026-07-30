package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kvn8888/codegym/backend/internal/api/response"
	"github.com/kvn8888/codegym/backend/internal/workflow"
)

type WorkflowHandler struct {
	service      *workflow.Service
	pollInterval time.Duration
}

func NewWorkflowHandler(service *workflow.Service) *WorkflowHandler {
	return &WorkflowHandler{service: service, pollInterval: 150 * time.Millisecond}
}

func (h *WorkflowHandler) Create(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		response.Error(w, http.StatusServiceUnavailable, "workflow_unconfigured", "Workflow progress is not configured.")
		return
	}
	var input workflow.CreateInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON.")
		return
	}
	result, err := h.service.Create(r.Context(), input)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "workflow_create_failed", err.Error())
		return
	}
	response.JSON(w, http.StatusCreated, result)
}

func (h *WorkflowHandler) Events(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		response.Error(w, http.StatusServiceUnavailable, "workflow_unconfigured", "Workflow progress is not configured.")
		return
	}
	operationID := strings.TrimSpace(r.PathValue("id"))
	after, err := workflowCursor(r)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid_workflow_cursor", "Workflow cursor must be a non-negative integer.")
		return
	}
	operation, err := h.service.Get(r.Context(), operationID)
	if errors.Is(err, workflow.ErrNotFound) {
		response.Error(w, http.StatusNotFound, "workflow_not_found", "Workflow operation not found.")
		return
	}
	if err != nil {
		response.Error(w, http.StatusBadRequest, "workflow_read_failed", "Workflow operation could not be read.")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		response.Error(w, http.StatusInternalServerError, "streaming_unsupported", "Streaming is not available.")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	cursor := after
	ticker := time.NewTicker(h.pollInterval)
	defer ticker.Stop()
	for {
		events, listErr := h.service.Events(r.Context(), operationID, cursor)
		if listErr != nil {
			_ = writeWorkflowSSE(w, flusher, "error", 0, map[string]any{
				"code": "workflow_stream_failed", "retryable": true,
			})
			return
		}
		for _, event := range events {
			if event.Sequence <= cursor {
				continue
			}
			if err := writeWorkflowSSE(w, flusher, "progress", event.Sequence, event); err != nil {
				return
			}
			cursor = event.Sequence
		}

		operation, err = h.service.Get(r.Context(), operationID)
		if err != nil {
			return
		}
		if (operation.Status == workflow.StatusSucceeded || operation.Status == workflow.StatusFailed) &&
			cursor >= operation.LastSequence {
			return
		}

		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

func (h *WorkflowHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		response.Error(w, http.StatusServiceUnavailable, "workflow_unconfigured", "Workflow progress is not configured.")
		return
	}
	operation, event, err := h.service.Cancel(r.Context(), r.PathValue("id"))
	if errors.Is(err, workflow.ErrNotFound) {
		response.Error(w, http.StatusNotFound, "workflow_not_found", "Workflow operation not found.")
		return
	}
	if errors.Is(err, workflow.ErrTerminal) {
		response.Error(w, http.StatusConflict, "workflow_terminal", "Workflow operation is already complete.")
		return
	}
	if err != nil {
		response.Error(w, http.StatusBadRequest, "workflow_cancel_failed", "Workflow operation could not be cancelled.")
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"operation": operation, "event": event})
}

func workflowCursor(r *http.Request) (int64, error) {
	value := strings.TrimSpace(r.URL.Query().Get("after"))
	if value == "" {
		value = strings.TrimSpace(r.Header.Get("Last-Event-ID"))
	}
	if value == "" {
		return 0, nil
	}
	cursor, err := strconv.ParseInt(value, 10, 64)
	if err != nil || cursor < 0 {
		return 0, errors.New("invalid cursor")
	}
	return cursor, nil
}

func writeWorkflowSSE(
	w http.ResponseWriter,
	flusher http.Flusher,
	eventType string,
	sequence int64,
	data any,
) error {
	encoded, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if sequence > 0 {
		if _, err := fmt.Fprintf(w, "id: %d\n", sequence); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, encoded); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}
