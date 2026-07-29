package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/kvn8888/codegym/backend/internal/api/response"
	"github.com/kvn8888/codegym/backend/internal/chat"
)

type ChatHandler struct {
	service *chat.Service
}

func NewChatHandler(service *chat.Service) *ChatHandler {
	return &ChatHandler{service: service}
}

func (h *ChatHandler) CreateThread(w http.ResponseWriter, r *http.Request) {
	var input chat.CreateThreadInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON.")
		return
	}
	result, err := h.service.CreateOrResume(r.Context(), input)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "chat_thread_failed", err.Error())
		return
	}
	response.JSON(w, http.StatusCreated, result)
}

func (h *ChatHandler) ListThreads(w http.ResponseWriter, r *http.Request) {
	filter := chat.ThreadFilter{
		Kind:      chat.Kind(r.URL.Query().Get("kind")),
		Status:    chat.Status(r.URL.Query().Get("status")),
		SessionID: r.URL.Query().Get("session_id"),
	}
	if value := r.URL.Query().Get("limit"); value != "" {
		limit, err := strconv.Atoi(value)
		if err != nil {
			response.Error(w, http.StatusBadRequest, "invalid_chat_filter", "limit must be a number")
			return
		}
		filter.Limit = limit
	}
	threads, err := h.service.List(r.Context(), filter)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "chat_list_failed", err.Error())
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"threads": threads})
}

func (h *ChatHandler) Messages(w http.ResponseWriter, r *http.Request) {
	messages, err := h.service.Messages(r.Context(), r.PathValue("id"))
	if errors.Is(err, chat.ErrNotFound) {
		response.Error(w, http.StatusNotFound, "chat_thread_not_found", "Conversation not found.")
		return
	}
	if err != nil {
		response.Error(w, http.StatusBadRequest, "chat_messages_failed", err.Error())
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"messages": messages})
}

func (h *ChatHandler) Turn(w http.ResponseWriter, r *http.Request) {
	var input chat.TurnInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON.")
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

	emit := func(event chat.SSEEvent) error {
		encoded, err := json.Marshal(event.Data)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, encoded); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}
	if err := h.service.Turn(r.Context(), r.PathValue("id"), input, emit); err != nil {
		_ = emit(chat.SSEEvent{Type: "error", Data: map[string]any{
			"message": "The response was interrupted. Retry this turn.", "retryable": true,
		}})
	}
}

func (h *ChatHandler) Reset(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.Reset(r.Context(), r.PathValue("id"))
	if errors.Is(err, chat.ErrNotFound) {
		response.Error(w, http.StatusNotFound, "chat_thread_not_found", "Conversation not found.")
		return
	}
	if err != nil {
		response.Error(w, http.StatusBadRequest, "chat_reset_failed", err.Error())
		return
	}
	response.JSON(w, http.StatusCreated, result)
}

func (h *ChatHandler) Finish(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.Finish(r.Context(), r.PathValue("id"))
	if errors.Is(err, chat.ErrNotFound) {
		response.Error(w, http.StatusNotFound, "chat_thread_not_found", "Conversation not found.")
		return
	}
	if err != nil {
		response.Error(w, http.StatusBadRequest, "chat_finish_failed", err.Error())
		return
	}
	response.JSON(w, http.StatusOK, result)
}

func (h *ChatHandler) Exit(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.Exit(r.Context(), r.PathValue("id"))
	if errors.Is(err, chat.ErrNotFound) {
		response.Error(w, http.StatusNotFound, "chat_thread_not_found", "Conversation not found.")
		return
	}
	if err != nil {
		response.Error(w, http.StatusBadRequest, "chat_exit_failed", err.Error())
		return
	}
	response.JSON(w, http.StatusOK, result)
}
